package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// recordingDeliverer captures every dispatched alert instead of delivering it,
// so the transition logic can be tested without real HTTP.
type recordingDeliverer struct{ alerts []operationalAlert }

func (r *recordingDeliverer) fn(_ context.Context, _ *gorm.DB, _ config.Config, a operationalAlert) {
	r.alerts = append(r.alerts, a)
}
func (r *recordingDeliverer) reset() { r.alerts = nil }

func alertTestConfig(dbPath string) config.Config {
	return config.Config{
		DBPath:                        dbPath,
		AlertingEnabled:               true,
		AlertEvalIntervalMinutes:      15,
		AlertDiskUsagePercent:         90,
		AlertSyncFailureThreshold:     3,
		AlertNotifyFailureThreshold:   3,
		AlertBackupMaxAgeHours:        0,
		AlertJobStaleMultiplier:       3,
		AlertIncidentQuietHours:       6,
		AlertBackupEnabled:            true,
		AlertDBIntegrityEnabled:       true,
		AlertJobStoppedEnabled:        true,
		CalDAVSyncIntervalHours:       6,
		ImmichSyncIntervalHours:       6,
		DBIntegrityCheckIntervalHours: 24,
		DBRestoreDrillIntervalHours:   168,
	}
}

func alertStateFor(t *testing.T, db *gorm.DB, key string) (models.AlertState, bool) {
	t.Helper()
	var row models.AlertState
	err := db.Where("condition_key = ?", key).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return models.AlertState{}, false
	}
	require.NoError(t, err)
	return row, true
}

// TestEvaluateAlerts drives the whole raise/recover cycle over one real
// migrated DB shared across the cases (each resets the operational tables
// first) — the services package is already near the CI -race timeout.
func TestEvaluateAlerts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alerting.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	rec := &recordingDeliverer{}
	origDeliverer := alertDeliverer
	alertDeliverer = rec.fn
	t.Cleanup(func() { alertDeliverer = origDeliverer })

	origDisk := diskUsageFn
	diskPct := 10
	diskUsageFn = func(string) (int, error) { return diskPct, nil }
	t.Cleanup(func() { diskUsageFn = origDisk })

	cfg := alertTestConfig(dbPath)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		for _, table := range []string{"system_events", "alert_states", "operational_check_results", "job_executions"} {
			require.NoError(t, db.Exec("DELETE FROM "+table).Error)
		}
		rec.reset()
		diskPct = 10
	}

	// baseline establishes the clean "ok" rows without any dispatch.
	baseline := func(t *testing.T) {
		t.Helper()
		RunAlertEvaluation(ctx, db, cfg)
		require.Empty(t, rec.alerts, "a clean baseline evaluation must not dispatch anything")
		rec.reset()
	}

	t.Run("backup failure raises once, repeats are silent, recovery clears", func(t *testing.T) {
		reset(t)
		baseline(t)
		now := time.Now()

		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, now.Add(-1*time.Hour), "snapshot failed")

		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.Equal(t, alertConditionKeyBackup, rec.alerts[0].conditionKey)
		assert.True(t, rec.alerts[0].firing)
		assert.Contains(t, rec.alerts[0].subject(), "Backup failed")

		row, ok := alertStateFor(t, db, alertConditionKeyBackup)
		require.True(t, ok)
		assert.Equal(t, models.AlertStateAlerting, row.State)
		assert.Equal(t, 1, row.FailureCount)
		since := row.Since

		// A second identical evaluation is the storm guard: no new dispatch,
		// and the row's "since" is unchanged.
		rec.reset()
		RunAlertEvaluation(ctx, db, cfg)
		assert.Empty(t, rec.alerts, "an unchanged failing condition must not re-alert")
		row2, _ := alertStateFor(t, db, alertConditionKeyBackup)
		assert.WithinDuration(t, since, row2.Since, 0)

		// Recovery.
		rec.reset()
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, now, "")
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.False(t, rec.alerts[0].firing)
		assert.Contains(t, rec.alerts[0].subject(), "recovered after 1 failure")
		row3, _ := alertStateFor(t, db, alertConditionKeyBackup)
		assert.Equal(t, models.AlertStateOK, row3.State)
		assert.Equal(t, 0, row3.FailureCount)
	})

	t.Run("sync needs the consecutive-failure threshold before it fires", func(t *testing.T) {
		reset(t)
		baseline(t)
		now := time.Now()

		// Two failures — below the threshold of 3.
		seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-2*time.Hour), "carddav 401")
		seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-1*time.Hour), "carddav 401")
		RunAlertEvaluation(ctx, db, cfg)
		_, ok := alertStateFor(t, db, alertConditionKeySyncContact)
		assert.True(t, ok, "baseline row exists")
		assert.Empty(t, rec.alerts, "two failures is under the threshold")

		// Third failure crosses it.
		seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now, "carddav 401")
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.Equal(t, alertConditionKeySyncContact, rec.alerts[0].conditionKey)
		assert.Equal(t, 3, rec.alerts[0].failureCount)
	})

	t.Run("disk space fires at the threshold and clears with hysteresis", func(t *testing.T) {
		reset(t)
		baseline(t)

		diskPct = 95
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.Equal(t, alertConditionKeyDiskSpace, rec.alerts[0].conditionKey)
		assert.True(t, rec.alerts[0].firing)

		// 88% is below the 90% threshold but above clear-below (85%): still
		// alerting, no new dispatch.
		rec.reset()
		diskPct = 88
		RunAlertEvaluation(ctx, db, cfg)
		assert.Empty(t, rec.alerts)
		row, _ := alertStateFor(t, db, alertConditionKeyDiskSpace)
		assert.Equal(t, models.AlertStateAlerting, row.State)

		// 80% is below clear-below: recovery.
		rec.reset()
		diskPct = 80
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.False(t, rec.alerts[0].firing)
	})

	t.Run("job_stopped fires for a stale enabled job, ignores disabled jobs", func(t *testing.T) {
		reset(t)
		baseline(t)

		// calendar_sync last completed 100h ago; interval 6h * multiplier 3 =
		// 18h stale threshold.
		require.NoError(t, db.Create(&models.JobExecution{
			JobName:   models.JobNameCalendarSync,
			LastRunAt: time.Now().Add(-100 * time.Hour),
		}).Error)
		// restore_drill is equally stale but disabled in this config, so it
		// must not contribute.
		require.NoError(t, db.Create(&models.JobExecution{
			JobName:   models.JobNameRestoreDrill,
			LastRunAt: time.Now().Add(-1000 * time.Hour),
		}).Error)

		staleCfg := cfg
		staleCfg.DBRestoreDrillEnabled = false

		RunAlertEvaluation(ctx, db, staleCfg)
		require.Len(t, rec.alerts, 1)
		assert.Equal(t, alertConditionKeyJobStopped, rec.alerts[0].conditionKey)
		assert.Contains(t, rec.alerts[0].detail, models.JobNameCalendarSync)
		assert.NotContains(t, rec.alerts[0].detail, models.JobNameRestoreDrill)

		// The job catches up: recovery.
		rec.reset()
		require.NoError(t, db.Model(&models.JobExecution{}).
			Where("job_name = ?", models.JobNameCalendarSync).
			Update("last_run_at", time.Now()).Error)
		RunAlertEvaluation(ctx, db, staleCfg)
		require.Len(t, rec.alerts, 1)
		assert.False(t, rec.alerts[0].firing)
	})

	t.Run("db_integrity reads the persisted check result", func(t *testing.T) {
		reset(t)
		baseline(t)

		RecordOperationalCheckResult(db, models.JobNameDBIntegrityCheck, models.OpCheckStatusFailed, "*** in database main *** Page 42: btreeInitPage() returns error code 11")
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.Equal(t, alertConditionKeyDBIntegrity, rec.alerts[0].conditionKey)
		assert.True(t, rec.alerts[0].firing)

		rec.reset()
		RecordOperationalCheckResult(db, models.JobNameDBIntegrityCheck, models.OpCheckStatusOK, "")
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		assert.False(t, rec.alerts[0].firing)
	})

	t.Run("ALERTING_ENABLED=false is a no-op", func(t *testing.T) {
		reset(t)
		off := cfg
		off.AlertingEnabled = false
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, time.Now(), "boom")
		EvaluateAlerts(db, off)
		assert.Empty(t, rec.alerts)
		_, ok := alertStateFor(t, db, alertConditionKeyBackup)
		assert.False(t, ok, "the evaluator must not touch alert_states when disabled")
	})
}
