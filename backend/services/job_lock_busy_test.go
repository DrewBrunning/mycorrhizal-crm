package services

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1176: a transient SQLITE_BUSY on the job-lock *release* write left the
// job marked locked (the transaction rolled back and the error was surfaced on
// the first attempt), so the next scheduled run stayed suppressed until the
// stale-lock window reclaimed it. The lock transactions now retry a busy with a
// short bounded backoff (retryJobLockOnBusy). These tests pin that policy and
// the acquire/release wiring.

// failUpdatesWithBusy installs an Update callback that fails each UPDATE with
// SQLite's lock-busy backstop until `failures` updates have been rejected, then
// lets them through. The counter records every update the callback saw, so a
// test can assert how many attempts the retry loop actually made. It is
// registered after acquireJobLock so only the release's Save is affected.
func failUpdatesWithBusy(t *testing.T, db *gorm.DB, failures int32) *int32 {
	t.Helper()
	var seen int32
	require.NoError(t, db.Callback().Update().Before("gorm:update").
		Register("job_lock_busy_test_fail_updates", func(tx *gorm.DB) {
			if atomic.AddInt32(&seen, 1) <= failures {
				tx.AddError(errLockBusy)
			}
		}))
	return &seen
}

// TestRetryJobLockOnBusy pins the retry policy directly: only SQLite's
// lock-busy backstop is retried; a busy that survives the bounded attempt
// count is surfaced; and the lock's own non-busy sentinels (errJobRanTooRecently,
// "locked by another instance") are never retried.
func TestRetryJobLockOnBusy(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name     string
		attempts []error // successive op() results; the last repeats once exhausted
		wantErr  string  // substring of the returned error; "" = want nil
		wantCall int
	}{
		{name: "success is not retried", attempts: []error{nil}, wantCall: 1},
		{name: "busy then success is retried", attempts: []error{errLockBusy, nil}, wantCall: 2},
		{name: "busy spelled sqlite_busy only is retried", attempts: []error{errors.New("SQLITE_BUSY"), nil}, wantCall: 2},
		{name: "busy then success after several retries", attempts: []error{errLockBusy, errLockBusy, nil}, wantCall: 3},
		{name: "sustained busy is bounded and surfaced", attempts: []error{errLockBusy}, wantErr: "sqlite_busy", wantCall: jobLockBusyMaxAttempts},
		{name: "non-busy error is not retried", attempts: []error{boom}, wantErr: "boom", wantCall: 1},
		{name: "dedup sentinel is not retried", attempts: []error{errJobRanTooRecently}, wantErr: errJobRanTooRecently.Error(), wantCall: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			op := func() error {
				idx := calls
				if idx >= len(tt.attempts) {
					idx = len(tt.attempts) - 1
				}
				calls++
				return tt.attempts[idx]
			}
			got := retryJobLockOnBusy(op)
			if tt.wantErr == "" {
				assert.NoError(t, got)
			} else {
				require.Error(t, got)
				assert.Contains(t, strings.ToLower(got.Error()), strings.ToLower(tt.wantErr))
			}
			assert.Equal(t, tt.wantCall, calls, "attempt() invocation count")
		})
	}
}

// TestReleaseJobLock_RetriesTransientBusy is the issue #1176 regression test: a
// release whose UPDATE hits SQLITE_BUSY must be retried and eventually clear
// locked_at, rather than surfacing on the first busy and leaving the job
// marked locked.
func TestReleaseJobLock_RetriesTransientBusy(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_release_busy_retry"
	window := JobCatchupWindow(time.Hour)

	ok, err := acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, jobExec(t, db, job).LockedAt, "precondition: the job is locked")

	// Fail the first two release UPDATEs with a busy, then succeed on the third.
	seen := failUpdatesWithBusy(t, db, 2)

	require.NoError(t, releaseJobLock(db, job, true))

	after := jobExec(t, db, job)
	assert.Nil(t, after.LockedAt, "the lock must be released after the retry succeeds")
	assert.Empty(t, after.LockedBy)
	assert.Equal(t, models.JobOutcomeRan, after.LastOutcome)
	assert.Equal(t, int32(3), atomic.LoadInt32(seen),
		"the release must have been attempted 3 times (2 busy + 1 success)")
}

// TestReleaseJobLock_SustainedBusyIsBounded pins the other half of the policy:
// if the busy never clears, releaseJobLock gives up after the bounded attempt
// count and returns the busy instead of retrying forever. The row stays locked
// — the unavoidable outcome of genuine sustained contention — which is exactly
// why the bounded retry must not be unbounded.
func TestReleaseJobLock_SustainedBusyIsBounded(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_release_busy_exhausted"
	window := JobCatchupWindow(time.Hour)

	ok, err := acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)

	// Every release UPDATE fails busy.
	seen := failUpdatesWithBusy(t, db, jobLockBusyMaxAttempts)

	err = releaseJobLock(db, job, true)
	require.Error(t, err, "a sustained busy must be surfaced, not swallowed")
	assert.True(t, isSQLiteBusy(err))
	assert.Equal(t, int32(jobLockBusyMaxAttempts), atomic.LoadInt32(seen),
		"no more attempts than the bounded maximum")
	assert.NotNil(t, jobExec(t, db, job).LockedAt, "a release that never succeeded leaves the row locked")
}

// TestReleaseJobLock_NonBusyErrorIsNotRetried pins that the retry is specific
// to SQLITE_BUSY: a different storage failure must be surfaced on the first
// attempt, preserving issue #975's contract that a failed release is reported
// (the existing purge tests rely on this via their own callback seam).
func TestReleaseJobLock_NonBusyErrorIsNotRetried(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_release_nonbusy"
	window := JobCatchupWindow(time.Hour)

	ok, err := acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)

	var seen int32
	require.NoError(t, db.Callback().Update().Before("gorm:update").
		Register("job_lock_busy_test_fail_updates_nonbusy", func(tx *gorm.DB) {
			atomic.AddInt32(&seen, 1)
			tx.AddError(errors.New("simulated release failure"))
		}))

	err = releaseJobLock(db, job, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated release failure")
	assert.Equal(t, int32(1), atomic.LoadInt32(&seen), "a non-busy error must not be retried")
}

// TestAcquireJobLock_RetriesTransientBusy covers the acquire path's exposure,
// which is the same as the release's: with _txlock=immediate the transaction's
// write (here the Save of an existing row) can surface a busy. A transient one
// must be retried so the job is not silently skipped.
func TestAcquireJobLock_RetriesTransientBusy(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_acquire_busy_retry"
	window := JobCatchupWindow(time.Hour)

	// A stale row routes acquire through its UPDATE (Save) path rather than
	// the first-run INSERT.
	require.NoError(t, db.Create(&models.JobExecution{
		JobName:     job,
		LastRunAt:   time.Now().Add(-100 * time.Hour),
		LastOutcome: models.JobOutcomeRan,
	}).Error)

	seen := failUpdatesWithBusy(t, db, 1)

	ok, err := acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok, "a transient busy must not suppress the run")
	assert.Equal(t, int32(2), atomic.LoadInt32(seen), "acquire must retry the busy once")
}

// TestAcquireJobLock_RetriesTransientBusyOnCreate is the first-run half of the
// acquire exposure: the INSERT (Create) can itself hit a busy. Retrying the
// whole transaction is safe because the failed one rolled back, so the row is
// not duplicated.
func TestAcquireJobLock_RetriesTransientBusyOnCreate(t *testing.T) {
	db := dbtest.New(t)
	const job = "test_acquire_busy_retry_create"
	window := JobCatchupWindow(time.Hour)

	var seen int32
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("job_lock_busy_test_fail_create", func(tx *gorm.DB) {
			if atomic.AddInt32(&seen, 1) <= 1 {
				tx.AddError(errLockBusy)
			}
		}))

	ok, err := acquireJobLock(db, job, window)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int32(2), atomic.LoadInt32(&seen), "the INSERT must be retried once")

	var count int64
	require.NoError(t, db.Model(&models.JobExecution{}).Where("job_name = ?", job).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the retried transaction must not leave a duplicate row")
}
