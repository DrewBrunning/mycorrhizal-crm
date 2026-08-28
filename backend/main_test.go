package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"mycorrhizal/metrics"

	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/require"
)

// TestRecoverJob_UnitRecoversPanic verifies recoverJob itself absorbs a
// panic thrown by the wrapped function instead of letting it propagate to
// the caller — the direct analogue of what safeGo already guarantees for
// each job's initial run.
func TestRecoverJob_UnitRecoversPanic(t *testing.T) {
	wrapped := recoverJob(nil, "test-panic-job", func() {
		panic("boom")
	})

	require.NotPanics(t, func() {
		wrapped()
	}, "recoverJob must absorb a panic from the wrapped function")
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

	_, err := s.Every(1).Second().Do(recoverJob(nil, "panic-job", func() {
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

// TestRunJob_RecordsJobMetrics pins that runJob feeds the Prometheus
// job_runs_total / job_duration_seconds families on both the success and the
// recovered-panic path (issue #389).
func TestRunJob_RecordsJobMetrics(t *testing.T) {
	runJob(nil, "metrics-unit-ok", func() {})
	require.NotPanics(t, func() {
		runJob(nil, "metrics-unit-boom", func() { panic("boom") })
	})

	var sb strings.Builder
	require.NoError(t, metrics.Default().WritePrometheus(&sb))
	out := sb.String()

	require.Contains(t, out, `job_runs_total{job="metrics-unit-ok",result="success"} 1`+"\n")
	require.Contains(t, out, `job_runs_total{job="metrics-unit-boom",result="failure"} 1`+"\n")
	require.Contains(t, out, `job_duration_seconds_count{job="metrics-unit-ok"} 1`+"\n")
}
