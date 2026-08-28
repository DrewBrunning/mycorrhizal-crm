package services

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newJobRunHealthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)
	return db
}

// seedRun inserts one job_runs row at a fixed time with the given result and
// duration. n<0 means "no items count".
func seedRun(t *testing.T, db *gorm.DB, job string, at time.Time, result string, durMS int64, n int) {
	t.Helper()
	row := models.JobRun{
		JobName:    job,
		Trigger:    models.JobTriggerScheduled,
		StartedAt:  at,
		FinishedAt: at.Add(time.Duration(durMS) * time.Millisecond),
		DurationMS: durMS,
		Result:     result,
		CreatedAt:  at,
	}
	if n >= 0 {
		row.ItemsProcessed = &n
	}
	require.NoError(t, db.Create(&row).Error)
}

func findHealth(t *testing.T, hs []JobRunHealth, job string) JobRunHealth {
	t.Helper()
	for _, h := range hs {
		if h.JobName == job {
			return h
		}
	}
	t.Fatalf("no health entry for %q", job)
	return JobRunHealth{}
}

func TestComputeJobRunHealth_UnknownWhenNoRuns(t *testing.T) {
	db := newJobRunHealthTestDB(t)

	hs, err := ComputeJobRunHealth(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, hs, len(models.KnownJobNames), "one entry per known job, in order")
	for i, h := range hs {
		assert.Equal(t, models.KnownJobNames[i], h.JobName)
		assert.Equal(t, JobRunStatusUnknown, h.Status)
		assert.Nil(t, h.LastRunAt)
		assert.Zero(t, h.ConsecutiveFailures)
	}
}

func TestComputeJobRunHealth_HealthyAfterSuccess(t *testing.T) {
	db := newJobRunHealthTestDB(t)
	base := time.Now().Add(-time.Hour).UTC()

	seedRun(t, db, models.JobNameDailyReminders, base, models.JobRunResultFailure, 900, -1)
	seedRun(t, db, models.JobNameDailyReminders, base.Add(10*time.Minute), models.JobRunResultSuccess, 1100, 4)

	h := findHealth(t, mustHealth(t, db), models.JobNameDailyReminders)
	assert.Equal(t, JobRunStatusHealthy, h.Status)
	assert.Equal(t, models.JobRunResultSuccess, h.LastResult)
	assert.Zero(t, h.ConsecutiveFailures)
	assert.Nil(t, h.IncidentFirstFailureAt)
	require.NotNil(t, h.LastSuccessAt)
	require.NotNil(t, h.LastFailureAt, "the earlier failure is still recorded")
	require.NotNil(t, h.LastItemsProcessed)
	assert.Equal(t, 4, *h.LastItemsProcessed)
}

func TestComputeJobRunHealth_FailingRunTracksIncident(t *testing.T) {
	db := newJobRunHealthTestDB(t)
	base := time.Now().Add(-3 * time.Hour).UTC()

	// A failure that predates the last success must NOT be counted in the
	// current incident.
	seedRun(t, db, models.JobNameCalendarSync, base.Add(-time.Hour), models.JobRunResultFailure, 200, -1)
	seedRun(t, db, models.JobNameCalendarSync, base, models.JobRunResultSuccess, 500, -1)
	firstFail := base.Add(30 * time.Minute)
	seedRun(t, db, models.JobNameCalendarSync, firstFail, models.JobRunResultFailure, 200, -1)
	// A skipped run between failures must be transparent to the streak.
	seedRun(t, db, models.JobNameCalendarSync, base.Add(45*time.Minute), models.JobRunResultSkipped, 1, -1)
	seedRun(t, db, models.JobNameCalendarSync, base.Add(60*time.Minute), models.JobRunResultFailure, 200, -1)
	last := base.Add(90 * time.Minute)
	seedRunWithError(t, db, models.JobNameCalendarSync, last, "auth rejected", 200)

	h := findHealth(t, mustHealth(t, db), models.JobNameCalendarSync)
	assert.Equal(t, JobRunStatusFailing, h.Status)
	assert.Equal(t, 3, h.ConsecutiveFailures, "3 failures since the last success; the skipped run does not break it")
	require.NotNil(t, h.IncidentFirstFailureAt)
	assert.WithinDuration(t, firstFail, *h.IncidentFirstFailureAt, time.Second)
	assert.Equal(t, "auth rejected", h.LastError)
}

func TestComputeJobRunHealth_IncidentResetsAfterLaterSuccess(t *testing.T) {
	db := newJobRunHealthTestDB(t)
	base := time.Now().Add(-2 * time.Hour).UTC()

	seedRun(t, db, models.JobNameImmichSync, base, models.JobRunResultFailure, 100, -1)
	seedRun(t, db, models.JobNameImmichSync, base.Add(10*time.Minute), models.JobRunResultFailure, 100, -1)
	seedRun(t, db, models.JobNameImmichSync, base.Add(20*time.Minute), models.JobRunResultSuccess, 100, -1)

	h := findHealth(t, mustHealth(t, db), models.JobNameImmichSync)
	assert.Equal(t, JobRunStatusHealthy, h.Status)
	assert.Zero(t, h.ConsecutiveFailures)
	assert.Nil(t, h.IncidentFirstFailureAt)
	assert.Empty(t, h.LastError)
}

func TestComputeJobRunHealth_DurationTrendExcludesSkipped(t *testing.T) {
	db := newJobRunHealthTestDB(t)
	base := time.Now().Add(-time.Hour).UTC()

	seedRun(t, db, models.JobNameAuditPurge, base, models.JobRunResultSuccess, 1000, -1)
	seedRun(t, db, models.JobNameAuditPurge, base.Add(time.Minute), models.JobRunResultSuccess, 3000, -1)
	seedRun(t, db, models.JobNameAuditPurge, base.Add(2*time.Minute), models.JobRunResultSkipped, 1, -1)

	h := findHealth(t, mustHealth(t, db), models.JobNameAuditPurge)
	require.NotNil(t, h.AvgDurationMS)
	require.NotNil(t, h.MaxDurationMS)
	assert.Equal(t, 2, h.DurationSampleSize, "the skipped run is not sampled")
	assert.EqualValues(t, 2000, *h.AvgDurationMS)
	assert.EqualValues(t, 3000, *h.MaxDurationMS)
}

func TestComputeJobRunHealth_OnlySkippedIsUnknown(t *testing.T) {
	db := newJobRunHealthTestDB(t)
	seedRun(t, db, models.JobNameRestoreDrill, time.Now().Add(-time.Minute).UTC(), models.JobRunResultSkipped, 1, -1)

	h := findHealth(t, mustHealth(t, db), models.JobNameRestoreDrill)
	assert.Equal(t, JobRunStatusUnknown, h.Status, "a job that has only ever been skipped has not executed")
	require.NotNil(t, h.LastRunAt, "but the skipped run is still its last run")
	assert.Equal(t, models.JobRunResultSkipped, h.LastResult)
}

func TestListJobRuns_FiltersAndLimit(t *testing.T) {
	db := newJobRunHealthTestDB(t)
	base := time.Now().Add(-time.Hour).UTC()
	seedRun(t, db, models.JobNameDailyReminders, base, models.JobRunResultSuccess, 10, -1)
	seedRun(t, db, models.JobNameDailyReminders, base.Add(time.Minute), models.JobRunResultFailure, 10, -1)
	seedRun(t, db, models.JobNameCalendarSync, base.Add(2*time.Minute), models.JobRunResultSuccess, 10, -1)

	all, err := ListJobRuns(context.Background(), db, JobRunFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 3)
	assert.Equal(t, models.JobNameCalendarSync, all[0].JobName, "newest first")

	byJob, err := ListJobRuns(context.Background(), db, JobRunFilter{JobName: models.JobNameDailyReminders})
	require.NoError(t, err)
	assert.Len(t, byJob, 2)

	byResult, err := ListJobRuns(context.Background(), db, JobRunFilter{Result: models.JobRunResultFailure})
	require.NoError(t, err)
	require.Len(t, byResult, 1)
	assert.Equal(t, models.JobNameDailyReminders, byResult[0].JobName)

	capped, err := ListJobRuns(context.Background(), db, JobRunFilter{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, capped, 1)
}

func mustHealth(t *testing.T, db *gorm.DB) []JobRunHealth {
	t.Helper()
	hs, err := ComputeJobRunHealth(context.Background(), db)
	require.NoError(t, err)
	return hs
}

func seedRunWithError(t *testing.T, db *gorm.DB, job string, at time.Time, errMsg string, durMS int64) {
	t.Helper()
	require.NoError(t, db.Create(&models.JobRun{
		JobName:    job,
		Trigger:    models.JobTriggerScheduled,
		StartedAt:  at,
		FinishedAt: at.Add(time.Duration(durMS) * time.Millisecond),
		DurationMS: durMS,
		Result:     models.JobRunResultFailure,
		Error:      errMsg,
		CreatedAt:  at,
	}).Error)
}
