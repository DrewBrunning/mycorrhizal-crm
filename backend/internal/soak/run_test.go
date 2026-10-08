package soak

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFaults(t *testing.T) {
	fs, err := ParseFaults("")
	require.NoError(t, err)
	assert.Empty(t, fs)
	fs, err = ParseFaults("goroutines, heap,wal")
	require.NoError(t, err)
	assert.Equal(t, []Fault{FaultGoroutines, FaultHeap, FaultWAL}, fs)
	_, err = ParseFaults("goroutines,bogus")
	assert.Error(t, err)
}

func TestInjector_EachFaultLeaksAndCloseReleases(t *testing.T) {
	in, err := newInjector(context.Background(), []Fault{FaultGoroutines, FaultHeap, FaultFDs, FaultLimiter}, nil)
	require.NoError(t, err)
	in.Tick()
	in.Tick()
	assert.Len(t, in.retain, 2)
	assert.Len(t, in.files, 24)
	assert.True(t, in.has(FaultHeap))
	assert.False(t, in.has(FaultWAL))
	in.Close()
	in.Close() // idempotent
	assert.Nil(t, in.retain)
}

func TestInjector_WALNeedsADatabase(t *testing.T) {
	_, err := newInjector(context.Background(), []Fault{FaultWAL}, nil)
	assert.Error(t, err)
}

func TestRun_RejectsBadConfig(t *testing.T) {
	_, err := Run(context.Background(), Config{})
	assert.ErrorIs(t, err, ErrRun)
	_, err = Run(context.Background(), Config{Duration: time.Second, Target: &Target{BaseURL: "http://x"}, Faults: []Fault{FaultHeap}})
	assert.ErrorIs(t, err, ErrRun)
	_, err = Run(context.Background(), Config{Duration: time.Second, Target: &Target{BaseURL: "not a url"}})
	assert.ErrorIs(t, err, ErrRun)
	_, err = Run(context.Background(), Config{Duration: time.Second, Target: &Target{BaseURL: "http://127.0.0.1:1", MetricsToken: "x"}})
	assert.ErrorIs(t, err, ErrRun, "an unreachable target cannot register users")
}

// smokeConfig is the per-PR form: short, but long enough for the 2/3-tail fit
// to have MinTailPoints samples.
//
// Latency-degradation budgets are advisory here. They compare the tail's p95 to
// the first third's over only ~14 s on a runner shared with the rest of the
// package shard under -race, where p95 comes from coarse histogram buckets
// (…1, 2.5, 5 s) and a single contention burst moves it a whole bucket — a
// healthy server failed this twice in a row. The committed budgets still gate
// the long soak, where the window is minutes, not seconds.
func smokeConfig(t *testing.T) Config {
	budgets := DefaultBudgets()
	for i := range budgets {
		if budgets[i].Kind == KindDegradation {
			budgets[i].Advisory = true
		}
	}
	return Config{
		Duration: 14 * time.Second, SampleEvery: time.Second, Users: 4, Rate: 25, Seed: 5,
		MinOps: 100, Dir: t.TempDir(), Budgets: budgets,
	}
}

// The per-PR smoke: a healthy server under the mixed workload must pass every
// budget and end-of-run check. This is the green half of the harness's
// self-verification; the red half is the fault test below.
func TestRun_HealthyServerPasses(t *testing.T) {
	if testing.Short() {
		t.Skip("soak smoke boots a real server for ~15s")
	}
	rep, err := Run(context.Background(), smokeConfig(t))
	require.NoError(t, err)
	assert.True(t, rep.OK(), rep.Markdown())
	assert.Equal(t, "in-process", rep.Mode)
	for _, sig := range RequiredSignals {
		assert.NotEmpty(t, rep.Series[sig], sig)
	}
	names := map[string]bool{}
	for _, c := range rep.Checks {
		names[c.Name] = c.OK
	}
	for _, want := range []string{"scheduler", "search freshness probe", "PRAGMA integrity_check / foreign_key_check", "FTS index vs base tables", "doctor (data integrity)"} {
		assert.True(t, names[want], "%s must run and pass", want)
	}
	assert.Greater(t, rep.Workload.Ops[opLogin], int64(0))
}

// Hand-verification, kept as a regression test: with leaks injected the soak
// must fail, and name each leaked signal. A soak that cannot fail proves
// nothing.
func TestRun_InjectedLeaksAreDetected(t *testing.T) {
	if testing.Short() {
		t.Skip("soak fault test boots a real server for ~15s")
	}
	cfg := smokeConfig(t)
	cfg.Faults = []Fault{FaultGoroutines, FaultHeap, FaultFDs, FaultLimiter}
	rep, err := Run(context.Background(), cfg)
	require.NoError(t, err)
	require.False(t, rep.OK(), "injected leaks must fail the soak")
	breached := map[string]bool{}
	for _, v := range rep.Verdicts {
		if v.Failed() {
			breached[v.Budget.Signal] = true
		}
	}
	for _, sig := range []string{SigGoroutines, SigHeapInuse, SigOpenFDs, SigLimiterEntries} {
		assert.True(t, breached[sig], "%s leak was not detected: %s", sig, rep.Markdown())
	}
}

func TestRun_WALStarvationIsDetected(t *testing.T) {
	if testing.Short() {
		t.Skip("soak fault test boots a real server for ~25s")
	}
	cfg := smokeConfig(t)
	cfg.Duration = 25 * time.Second
	cfg.Rate = 50
	cfg.Faults = []Fault{FaultWAL}
	// The committed 16 MiB ceiling is tuned for the 45 min run. Whether a
	// 25 s run gets past it depends on how many writes the runner manages
	// (~37 KiB of WAL per op measured: ~450 ops needed vs ~900-1250 achieved
	// on an idle machine), so on a contended -race CI runner this "must fail"
	// test could pass without detecting anything. Judge it against 8 MiB
	// instead: still 2x the ~4 MiB auto-checkpoint size a healthy run levels
	// off at (healthy max measured at 4.0-4.75 MiB), so only a starved
	// checkpoint reaches it, and it needs only ~220 ops.
	cfg.Budgets = DefaultBudgets()
	for i := range cfg.Budgets {
		if cfg.Budgets[i].Signal == SigWALBytes {
			cfg.Budgets[i].Limit = 8 * mib
		}
	}
	rep, err := Run(context.Background(), cfg)
	require.NoError(t, err)
	var wal Verdict
	for _, v := range rep.Verdicts {
		if v.Budget.Signal == SigWALBytes {
			wal = v
		}
	}
	assert.True(t, wal.Breached,
		"a pinned reader must starve the checkpoint and trip the WAL ceiling (observed %.2f MiB after %d ops; too few ops means the runner was too slow, not that detection broke): %s",
		wal.Observed/mib, rep.Workload.Total, rep.Markdown())
	assert.False(t, rep.OK())
}

// External-target mode against a server this test boots itself: exercises the
// attach path, the db-file checks and the "skipped" markers.
func TestRun_ExternalTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("soak external-target test boots a real server")
	}
	ctx := context.Background()
	inst, err := StartInstance(ctx, t.TempDir())
	require.NoError(t, err)
	defer func() { _ = inst.Stop(ctx) }()

	cfg := Config{
		Duration: 8 * time.Second, SampleEvery: time.Second, Users: 2, Rate: 20, Seed: 2, MinOps: 20,
		Budgets: []Budget{{Signal: SigGoroutines, Kind: KindGrowth, Limit: 1000, Unit: "g", Reason: "test"}},
		Target:  &Target{BaseURL: inst.BaseURL, MetricsToken: inst.MetricsToken, DBPath: inst.DBPath},
	}
	rep, err := Run(ctx, cfg)
	require.NoError(t, err)
	assert.Equal(t, "external", rep.Mode)
	skipped := map[string]bool{}
	for _, c := range rep.Checks {
		if c.Skipped {
			skipped[c.Name] = true
		}
	}
	assert.True(t, skipped["scheduler"])
	assert.True(t, skipped["doctor (data integrity)"])

	// Without a visible database file the integrity checks are skipped, not
	// silently passed.
	cfg.Target = &Target{BaseURL: inst.BaseURL, MetricsToken: inst.MetricsToken}
	cfg.Duration = 5 * time.Second
	rep, err = Run(ctx, cfg)
	require.NoError(t, err)
	skipped = map[string]bool{}
	for _, c := range rep.Checks {
		if c.Skipped {
			skipped[c.Name] = true
		}
	}
	assert.True(t, skipped["database integrity"])

	// A DB path that does not exist is a failed check.
	cfg.Target = &Target{BaseURL: inst.BaseURL, MetricsToken: inst.MetricsToken, DBPath: t.TempDir() + "/nope/x.db"}
	rep, err = Run(ctx, cfg)
	require.NoError(t, err)
	assert.False(t, rep.OK())
}

func TestJudge_AlwaysOnAssertions(t *testing.T) {
	r := &Report{Series: map[string]Series{}}
	r.Workload = Result{Total: 10, ServerErrs: 2, RateLimited: 5, Samples: []string{"s"}}
	r.judge(Config{MinOps: 100}, nil, Snapshot{Values: map[string]float64{SigServerErrors: 3, SigJobFailures: 1}})
	joined := ""
	for _, f := range r.Failures {
		joined += f + "\n"
	}
	for _, want := range []string{"never exported", "only 10 operations", "server errors", "5xx", "failed scheduled-job", "rate-limited"} {
		assert.Contains(t, joined, want)
	}
}
