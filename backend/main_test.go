package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newMainTestDB builds the real migrated schema (CLAUDE.md backend trap 1) for
// the job-wrapper tests, then clears job_runs so each test controls its rows.
func newMainTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)
	return db
}

// TestRecoverJob_UnitRecoversPanic verifies recoverJob itself absorbs a
// panic thrown by the wrapped function instead of letting it propagate to
// the caller — the direct analogue of what safeGo already guarantees for
// each job's initial run.
func TestRecoverJob_UnitRecoversPanic(t *testing.T) {
	wrapped := recoverJob(nil, "test-panic-job", models.JobTriggerScheduled, func() error {
		panic("boom")
	})

	require.NotPanics(t, func() {
		wrapped()
	}, "recoverJob must absorb a panic from the wrapped function")
}

// TestRunJob_RecordsOutcome pins the job_runs recording (issue #391): a
// successful run, a returned error, a services.ErrJobSkipped, and a panic each
// land as one row with the right result — and the panic is still recovered.
func TestRunJob_RecordsOutcome(t *testing.T) {
	db := newMainTestDB(t)

	runJob(db, models.JobNameAlertEval, models.JobTriggerScheduled, func() error { return nil })
	runJob(db, models.JobNameAuditPurge, models.JobTriggerInitial, func() error { return errors.New("kaboom") })
	runJob(db, models.JobNameImmichSync, models.JobTriggerScheduled, func() error { return services.ErrJobSkipped })
	require.NotPanics(t, func() {
		runJob(db, models.JobNameCalendarSync, models.JobTriggerScheduled, func() error { panic("explode") })
	})
	runJobReport(db, models.JobNameDailyReminders, models.JobTriggerManual, func() (int, error) { return 5, nil })

	byJob := map[string]models.JobRun{}
	var rows []models.JobRun
	require.NoError(t, db.Find(&rows).Error)
	for _, r := range rows {
		byJob[r.JobName] = r
	}

	require.Equal(t, "success", byJob[models.JobNameAlertEval].Result)
	require.Equal(t, "failure", byJob[models.JobNameAuditPurge].Result)
	require.Equal(t, "kaboom", byJob[models.JobNameAuditPurge].Error)
	require.Equal(t, models.JobTriggerInitial, byJob[models.JobNameAuditPurge].Trigger)
	require.Equal(t, "skipped", byJob[models.JobNameImmichSync].Result)
	require.Equal(t, "failure", byJob[models.JobNameCalendarSync].Result)
	require.Contains(t, byJob[models.JobNameCalendarSync].Error, "panic: explode")

	rem := byJob[models.JobNameDailyReminders]
	require.Equal(t, "success", rem.Result)
	require.Equal(t, models.JobTriggerManual, rem.Trigger)
	require.NotNil(t, rem.ItemsProcessed)
	require.Equal(t, 5, *rem.ItemsProcessed)
}

// TestRecoverJob_SchedulerSurvivesPanic reproduces the exact pattern main.go
// uses — s.Every(...).Do(recoverJob(name, fn)) on a real gocron.Scheduler —
// and deliberately panics inside the scheduled job on every tick. It
// confirms the scheduler (and by extension the process) keeps running
// scheduled ticks afterward instead of crashing.
//
// Hand-verified per CLAUDE.md: removing the recoverJob wrapper below (i.e.
// registering the raw panicking func directly with .Do()) makes this test
// hang/fail — the scheduler's goroutine dies on the first tick, so neither
// panicJobRuns nor survivorJobRuns ever reaches 2, and require.Eventually
// times out. That confirms the test actually exercises the crash this
// change fixes, not just a tautology. Restored to the wrapped form below.
func TestRecoverJob_SchedulerSurvivesPanic(t *testing.T) {
	var mu sync.Mutex
	var panicJobRuns, survivorJobRuns int

	s := gocron.NewScheduler(time.UTC)

	_, err := s.Every(1).Second().Do(recoverJob(nil, "panic-job", models.JobTriggerScheduled, func() error {
		mu.Lock()
		panicJobRuns++
		mu.Unlock()
		panic("simulated scheduled-job panic")
	}))
	require.NoError(t, err)

	// A sibling job on the same scheduler — if the panic ever took down the
	// scheduler's own goroutine (not just the one job), this would stop
	// ticking too.
	_, err = s.Every(1).Second().Do(func() {
		mu.Lock()
		survivorJobRuns++
		mu.Unlock()
	})
	require.NoError(t, err)

	s.StartAsync()
	defer s.Stop()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return panicJobRuns >= 2 && survivorJobRuns >= 2
	}, 5*time.Second, 50*time.Millisecond,
		"scheduler must keep ticking both jobs after the panicking job panics repeatedly — "+
			"an unrecovered panic would crash the process on the job's first scheduled run")
}
