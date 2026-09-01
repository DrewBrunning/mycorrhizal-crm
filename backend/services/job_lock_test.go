package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestJobCatchupWindow pins the single derivation for every scheduled job's
// de-dup window (issue #526, ADR 0011): period minus the shared JobCatchupMargin,
// with the margin clamped to a quarter of the period for sub-hour jobs so the
// window stays close to the period instead of collapsing.
func TestJobCatchupWindow(t *testing.T) {
	cases := []struct {
		name   string
		period time.Duration
		want   time.Duration
	}{
		{"zero/unknown period -> bare margin", 0, JobCatchupMargin},
		{"negative period -> bare margin", -time.Hour, JobCatchupMargin},
		{"daily job", 24 * time.Hour, 24*time.Hour - 30*time.Minute},
		{"six-hourly job", 6 * time.Hour, 6*time.Hour - 30*time.Minute},
		{"two-hour job (margin still 30m, < period/4)", 2 * time.Hour, 2*time.Hour - 30*time.Minute},
		{"one-hour job (margin clamped to 15m = period/4)", time.Hour, time.Hour - 15*time.Minute},
		{"fifteen-minute job (margin clamped to 3m45s)", 15 * time.Minute, 15*time.Minute - (15*time.Minute)/4},
		{"five-minute job (margin clamped to 1m15s)", 5 * time.Minute, 5*time.Minute - (5*time.Minute)/4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, JobCatchupWindow(tc.period))
			// The window is always strictly inside (0, period] for a positive period.
			if tc.period > 0 {
				got := JobCatchupWindow(tc.period)
				assert.Greater(t, got, time.Duration(0))
				assert.LessOrEqual(t, got, tc.period)
			}
		})
	}
}

func jobExec(t *testing.T, db *gorm.DB, name string) models.JobExecution {
	t.Helper()
	var je models.JobExecution
	require.NoError(t, db.Where("job_name = ?", name).First(&je).Error)
	return je
}

// TestAcquireJobLock_RecordsOutcome exercises the ADR 0011 rule-4 outcome
// ledger on JobExecution.LastOutcome against the real migrated schema (dbtest,
// CLAUDE.md backend trap #1 — the last_outcome column comes from migration
// 000048, not AutoMigrate).
func TestAcquireJobLock_RecordsOutcome(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_catchup_job"
	window := JobCatchupWindow(24 * time.Hour)

	// First ever run: acquired, transiently "running", finalised to "ran".
	ok, err := acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, models.JobOutcomeRunning, jobExec(t, db, job).LastOutcome)

	require.NoError(t, releaseJobLock(db, job, true))
	after := jobExec(t, db, job)
	assert.Equal(t, models.JobOutcomeRan, after.LastOutcome)
	firstRunAt := after.LastRunAt

	// Immediate retry (inside the window): suppressed and recorded as deduped,
	// LastRunAt untouched.
	ok, err = acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.False(t, ok, "a run inside the de-dup window must be suppressed")
	deduped := jobExec(t, db, job)
	assert.Equal(t, models.JobOutcomeDeduped, deduped.LastOutcome)
	assert.Equal(t, firstRunAt.UnixMilli(), deduped.LastRunAt.UnixMilli(), "a deduped run must not advance LastRunAt")

	// Simulate the process having been down for >2 windows: the next run is a
	// catch-up, and the marker survives release.
	require.NoError(t, db.Model(&models.JobExecution{}).Where("job_name = ?", job).
		Update("last_run_at", time.Now().Add(-3*window)).Error)
	ok, err = acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, models.JobOutcomeCaughtUp, jobExec(t, db, job).LastOutcome)
	require.NoError(t, releaseJobLock(db, job, true))
	assert.Equal(t, models.JobOutcomeCaughtUp, jobExec(t, db, job).LastOutcome, "release keeps a caught_up marker")

	// A failing run is recorded as failed.
	require.NoError(t, db.Model(&models.JobExecution{}).Where("job_name = ?", job).
		Update("last_run_at", time.Now().Add(-3*window)).Error)
	ok, err = acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, releaseJobLock(db, job, false))
	assert.Equal(t, models.JobOutcomeFailed, jobExec(t, db, job).LastOutcome)
}

// TestAcquireJobLock_TwoMissedOccurrencesRunOnce is ADR 0011 rules 1+2 at the
// lock layer: however many scheduled occurrences the process slept through, the
// next start runs the job exactly once — the following retry inside the window
// is suppressed.
func TestAcquireJobLock_TwoMissedOccurrencesRunOnce(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_missed_occurrences"
	period := 24 * time.Hour
	window := JobCatchupWindow(period)

	// Seed a lock row whose last run was three periods ago — three missed
	// daily occurrences.
	require.NoError(t, db.Create(&models.JobExecution{
		JobName:   job,
		LastRunAt: time.Now().Add(-3 * period),
	}).Error)

	runs := 0
	for i := 0; i < 2; i++ {
		ok, err := acquireJobLock(db, job, window)
		require.NoError(t, err)
		if ok {
			runs++
			require.NoError(t, releaseJobLock(db, job, true))
		}
	}
	assert.Equal(t, 1, runs, "three missed daily occurrences must produce exactly one catch-up run, not three")
	assert.Equal(t, models.JobOutcomeDeduped, jobExec(t, db, job).LastOutcome, "the second attempt was suppressed")
}
