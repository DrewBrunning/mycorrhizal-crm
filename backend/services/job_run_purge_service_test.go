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

// TestPurgeExpiredJobRuns pins the retention job: rows whose started_at is
// older than JOB_RUN_RETENTION_DAYS are removed, newer rows survive, and a
// non-positive retention disables purging entirely.
func TestPurgeExpiredJobRuns(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)

	old := models.JobRun{
		JobName: models.JobNameDailyReminders, Trigger: models.JobTriggerScheduled,
		StartedAt: time.Now().AddDate(0, 0, -40), FinishedAt: time.Now().AddDate(0, 0, -40),
		DurationMS: 5, Result: models.JobRunResultSuccess, CreatedAt: time.Now(),
	}
	fresh := models.JobRun{
		JobName: models.JobNameDailyReminders, Trigger: models.JobTriggerScheduled,
		StartedAt: time.Now(), FinishedAt: time.Now(),
		DurationMS: 5, Result: models.JobRunResultSuccess, CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Create(&fresh).Error)

	PurgeExpiredJobRuns(context.Background(), db, config.Config{JobRunRetentionDays: 30})

	var remaining []models.JobRun
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, fresh.ID, remaining[0].ID)

	// Retention <= 0 disables purging.
	require.NoError(t, db.Create(&models.JobRun{
		JobName: models.JobNameAlertEval, Trigger: models.JobTriggerScheduled,
		StartedAt: time.Now().AddDate(0, 0, -100), FinishedAt: time.Now().AddDate(0, 0, -100),
		DurationMS: 5, Result: models.JobRunResultFailure, CreatedAt: time.Now(),
	}).Error)
	PurgeExpiredJobRuns(context.Background(), db, config.Config{JobRunRetentionDays: 0})
	var all []models.JobRun
	require.NoError(t, db.Find(&all).Error)
	assert.Len(t, all, 2, "a non-positive retention must disable purging, not delete everything")
}

// TestPurgeExpiredJobRunsScheduled_JobLock proves the scheduled entry point
// takes the job lock and does not panic on repeated invocation.
func TestPurgeExpiredJobRunsScheduled_JobLock(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)

	cfg := config.Config{JobRunRetentionDays: 30}
	require.NotPanics(t, func() {
		PurgeExpiredJobRunsScheduled(db, cfg)
		PurgeExpiredJobRunsScheduled(db, cfg) // second call: lock rate-limits it
	})

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameJobRunPurge).First(&job).Error)
}
