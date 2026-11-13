package services

// Failure paths of the alert evaluator (issue #2569, coverage restored for
// services/alerting_service.go): every bookkeeping step that can fail is
// logged and degrades safely, and a state transition that could not be
// recorded is never announced (so the next run does not double-alert).

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func alertErrFixture(t *testing.T) (*gorm.DB, *recordingDeliverer) {
	t.Helper()
	logtest.AllowWarnings(t, "drives the alert evaluator's logged failure paths on purpose")
	dbPath := filepath.Join(t.TempDir(), "alerting.db")
	db := dbtest.NewAt(t, dbPath)
	rec := &recordingDeliverer{deliver: true}
	orig := alertDeliverer
	alertDeliverer = rec.fn
	t.Cleanup(func() { alertDeliverer = orig })
	origDisk := diskUsageFn
	diskUsageFn = func(string) (int, error) { return 10, nil }
	t.Cleanup(func() { diskUsageFn = origDisk })
	return db, rec
}

func TestEvaluateAlerts_JobLockFailuresAreLoggedNotFatal(t *testing.T) {
	db, rec := alertErrFixture(t)
	cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))

	t.Run("the lock table is unreadable, so the job does not run", func(t *testing.T) {
		dbtest.HideTable(t, db, "job_executions")
		assert.NotPanics(t, func() { EvaluateAlerts(db, cfg) })
		assert.Empty(t, rec.alerts)
	})
}

func TestEvaluateAlerts_ReleaseFailureIsLogged(t *testing.T) {
	db, _ := alertErrFixture(t)
	cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
	// The release reads the lock row back; make that read fail once the
	// evaluation (which runs between acquire and release) has started.
	started := false
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_release", func(tx *gorm.DB) {
		if started && tx.Statement.Table == "job_executions" {
			_ = tx.AddError(gorm.ErrInvalidDB)
		}
	}))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:mark_started", func(tx *gorm.DB) {
		if tx.Statement.Table == "alert_states" {
			started = true
		}
	}))
	assert.NotPanics(t, func() { EvaluateAlerts(db, cfg) })
	assert.True(t, started, "the evaluation ran before the failing release")
}

func TestRunAlertEvaluation_HealthAndStateLoadFailuresStopTheRun(t *testing.T) {
	cases := map[string]string{
		"subsystem health cannot be computed": "system_events",
		"alert states cannot be loaded":       "alert_states",
	}
	for name, table := range cases {
		t.Run(name, func(t *testing.T) {
			db, rec := alertErrFixture(t)
			cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
			dbtest.HideTable(t, db, table)
			assert.NotPanics(t, func() { RunAlertEvaluation(context.Background(), db, cfg) })
			assert.Empty(t, rec.alerts)
		})
	}
}

// With no baseline row yet, a first-ever firing condition is raised and its
// row is created by the transition itself.
func TestRunAlertEvaluation_FirstObservationFiringCreatesTheRow(t *testing.T) {
	db, rec := alertErrFixture(t)
	cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
	seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, time.Now().Add(-time.Hour), "snapshot failed")

	RunAlertEvaluation(context.Background(), db, cfg)

	require.Len(t, rec.alerts, 1)
	row, ok := alertStateFor(t, db, alertConditionKeyBackup)
	require.True(t, ok)
	assert.Equal(t, models.AlertStateAlerting, row.State)
}

func TestRunAlertEvaluation_StateWriteFailuresAreLoggedAndNeverAnnounced(t *testing.T) {
	ctx := context.Background()
	t.Run("a transition that cannot be recorded is not notified", func(t *testing.T) {
		db, rec := alertErrFixture(t)
		cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
		RunAlertEvaluation(ctx, db, cfg) // baseline rows
		failStatement(t, db, "fail_alert_update", "BEFORE UPDATE ON alert_states")
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, time.Now().Add(-time.Hour), "snapshot failed")
		RunAlertEvaluation(ctx, db, cfg)
		assert.Empty(t, rec.alerts, "no recorded transition, no notification")
		row, _ := alertStateFor(t, db, alertConditionKeyBackup)
		assert.Equal(t, models.AlertStateOK, row.State)
	})
	t.Run("the first-ever clean observation cannot create its baseline row", func(t *testing.T) {
		db, rec := alertErrFixture(t)
		cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
		failStatement(t, db, "fail_alert_insert", "BEFORE INSERT ON alert_states")
		RunAlertEvaluation(ctx, db, cfg)
		assert.Empty(t, rec.alerts)
		_, ok := alertStateFor(t, db, alertConditionKeyBackup)
		assert.False(t, ok)
	})
	t.Run("a refreshed incident detail that cannot be written is logged", func(t *testing.T) {
		db, rec := alertErrFixture(t)
		cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
		RunAlertEvaluation(ctx, db, cfg)
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, time.Now().Add(-2*time.Hour), "first failure")
		RunAlertEvaluation(ctx, db, cfg)
		require.Len(t, rec.alerts, 1)
		before, _ := alertStateFor(t, db, alertConditionKeyBackup)

		failStatement(t, db, "fail_alert_update", "BEFORE UPDATE ON alert_states")
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, time.Now().Add(-time.Hour), "second failure")
		RunAlertEvaluation(ctx, db, cfg)
		after, _ := alertStateFor(t, db, alertConditionKeyBackup)
		assert.Equal(t, before.Detail, after.Detail, "the stale detail stays when the refresh fails")
	})
}
