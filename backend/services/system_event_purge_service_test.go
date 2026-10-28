package services

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPurgeExpiredSystemEvents pins the retention job: rows whose occurred_at
// is older than SYSTEM_EVENT_RETENTION_DAYS are removed, newer rows survive,
// and a non-positive retention disables purging entirely.
func TestPurgeExpiredSystemEvents(t *testing.T) {
	db := dbtest.New(t)
	// InitDB's migration run records a migration_completed event; clear it so
	// the retention assertions count only rows this test inserts.
	require.NoError(t, db.Exec("DELETE FROM system_events").Error)

	old := models.SystemEvent{
		EventType: models.SysEventJobCompleted, Severity: "info",
		OccurredAt: time.Now().AddDate(0, 0, -40), CreatedAt: time.Now(),
	}
	fresh := models.SystemEvent{
		EventType: models.SysEventJobCompleted, Severity: "info",
		OccurredAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Create(&fresh).Error)

	PurgeExpiredSystemEvents(context.Background(), db, config.Config{SystemEventRetentionDays: 30})

	var remaining []models.SystemEvent
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, fresh.ID, remaining[0].ID)

	// Retention <= 0 disables purging.
	require.NoError(t, db.Create(&models.SystemEvent{
		EventType: models.SysEventJobFailed, Severity: "error",
		OccurredAt: time.Now().AddDate(0, 0, -100), CreatedAt: time.Now(),
	}).Error)
	PurgeExpiredSystemEvents(context.Background(), db, config.Config{SystemEventRetentionDays: 0})
	var all []models.SystemEvent
	require.NoError(t, db.Find(&all).Error)
	assert.Len(t, all, 2, "a non-positive retention must disable purging, not delete everything")
}

// TestPurgeExpiredSystemEventsScheduled_JobLock proves the scheduled entry point takes the job lock: with
// a recent lock row it must leave an expired system_events row alone, and once the
// lock is gone it must purge it.
func TestPurgeExpiredSystemEventsScheduled_JobLock(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM system_events").Error)

	expired := models.SystemEvent{EventType: models.SysEventJobCompleted, Severity: "info", OccurredAt: time.Now().AddDate(0, 0, -40), CreatedAt: time.Now()}
	require.NoError(t, db.Create(&expired).Error)

	cfg := config.Config{SystemEventRetentionDays: 30}
	require.NoError(t, db.Create(&models.JobExecution{JobName: models.JobNameSystemEventPurge, LastRunAt: time.Now()}).Error)
	require.NotPanics(t, func() { PurgeExpiredSystemEventsScheduled(db, cfg) })
	var n int64
	require.NoError(t, db.Model(&models.SystemEvent{}).Where("id = ?", expired.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "a rate-limited run must not purge")

	require.NoError(t, db.Where("job_name = ?", models.JobNameSystemEventPurge).Unscoped().Delete(&models.JobExecution{}).Error)
	require.NoError(t, PurgeExpiredSystemEventsScheduled(db, cfg))
	require.NoError(t, db.Model(&models.SystemEvent{}).Where("id = ?", expired.ID).Count(&n).Error)
	assert.Zero(t, n, "an unlocked run must purge the expired row")

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameSystemEventPurge).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock released after the run")
}
