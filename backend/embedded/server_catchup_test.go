package embedded

import (
	"context"
	"sync"
	"testing"
	"time"

	"mycorrhizal/config"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// jobObservation is what the jobFinishedObserver seam saw for one job run.
type jobObservation struct {
	job, trigger string
	dbOpen       bool
}

type jobRecorder struct {
	mu    sync.Mutex
	runs  []jobObservation
	first chan struct{} // closed on the first initial-trigger observation
	once  sync.Once
}

// observeJobs installs the jobFinishedObserver seam for the test. dbOpen is
// sampled at the *end* of each job run: a job that finished after Stop closed
// the database (issue #1311) is recorded with dbOpen=false.
func observeJobs(t *testing.T) *jobRecorder {
	t.Helper()
	r := &jobRecorder{first: make(chan struct{})}
	obs := func(db *gorm.DB, job, trigger string) {
		open := false
		if sqlDB, err := db.DB(); err == nil {
			open = sqlDB.Ping() == nil
		}
		r.mu.Lock()
		r.runs = append(r.runs, jobObservation{job, trigger, open})
		r.mu.Unlock()
		if trigger == "initial" {
			r.once.Do(func() { close(r.first) })
		}
	}
	jobFinishedObserver.Store(&obs)
	t.Cleanup(func() { jobFinishedObserver.Store(nil) })
	return r
}

func (r *jobRecorder) snapshot() []jobObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]jobObservation(nil), r.runs...)
}

func (r *jobRecorder) requireAllDBOpen(t *testing.T) {
	t.Helper()
	for _, o := range r.snapshot() {
		require.True(t, o.dbOpen, "job %s (%s) finished against a closed database", o.job, o.trigger)
	}
}

func startEmbedded(t *testing.T, delay time.Duration) *Server {
	t.Helper()
	cfg := newTestConfig(t, config.DeploymentEmbedded)
	ln, _ := listenUnix(t, "s.sock")
	srv, err := Start(context.Background(), cfg, Options{Listener: ln, CatchUpDelay: delay})
	require.NoError(t, err)
	return srv
}

// fakeCatchUpTimer stands in for the deferred catch-up timer: it records the
// callback and whether Stop cancelled it, and the test fires the callback by
// hand. Nothing here depends on the wall clock.
type fakeCatchUpTimer struct {
	mu      sync.Mutex
	fn      func()
	stopped bool
}

func (f *fakeCatchUpTimer) Stop() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	was := !f.stopped
	f.stopped = true
	return was
}

func (f *fakeCatchUpTimer) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

// Issue #1311 reproduction: Stop before the deferred catch-up fires must
// cancel the burst — no job runs, in particular none against a closed DB.
//
// Deterministic (issue #1689): the timer is a fake the test fires by hand. The
// earlier version used a real 300ms delay and a sleep, and on a loaded runner
// Start's tail (SLOW SQL 250-630ms) outlasted the delay, so the burst
// legitimately fired *before* Stop and the test misread it as a leak.
//
// Hand-verified: with bg.shutdown's timer.Stop removed the stopped assertion
// fails; with spawn's stopped guard removed the late-callback assertion fails.
func TestStop_BeforeDeferredCatchUp_CancelsBurst(t *testing.T) {
	rec := observeJobs(t)
	fake := &fakeCatchUpTimer{}
	cfg := newTestConfig(t, config.DeploymentEmbedded)
	ln, _ := listenUnix(t, "s.sock")
	srv, err := Start(context.Background(), cfg, Options{
		Listener:     ln,
		CatchUpDelay: time.Hour,
		afterFunc: func(_ time.Duration, fn func()) stoppableTimer {
			fake.fn = fn
			return fake
		},
	})
	require.NoError(t, err)
	require.NotNil(t, fake.fn, "the deferred catch-up timer was armed")

	require.NoError(t, srv.Stop(context.Background()))
	require.True(t, fake.isStopped(), "Stop must cancel the deferred catch-up timer")

	// A timer callback already running when Stop began lands after shutdown:
	// it must dispatch nothing.
	fake.fn()
	require.NoError(t, srv.bg.shutdown(context.Background())) // drain anything the late callback spawned
	for _, o := range rec.snapshot() {
		require.NotEqual(t, "initial", o.trigger, "job %s must not run after Stop cancelled the deferred burst", o.job)
	}
	rec.requireAllDBOpen(t)
}

// Stop must not return until the in-flight catch-up jobs have finished, and
// none may touch the database after it closes.
//
// Hand-verified: with the wg.Wait in shutdown removed this fails.
func TestStop_DrainsInFlightCatchUpJobs(t *testing.T) {
	rec := observeJobs(t)
	srv := startEmbedded(t, 0) // immediate burst

	require.NoError(t, srv.Stop(context.Background()))
	after := rec.snapshot()
	require.NotEmpty(t, after, "the burst dispatched jobs and Stop waited for them")
	rec.requireAllDBOpen(t)

	time.Sleep(200 * time.Millisecond)
	require.Len(t, rec.snapshot(), len(after), "no job may finish after Stop returned")
}

// Stop after the deferred burst has begun (some jobs may be mid-flight).
func TestStop_AfterDeferredBurstBegan(t *testing.T) {
	rec := observeJobs(t)
	srv := startEmbedded(t, 20*time.Millisecond)

	select {
	case <-rec.first:
	case <-time.After(20 * time.Second):
		t.Fatal("deferred catch-up never started")
	}
	require.NoError(t, srv.Stop(context.Background()))
	rec.requireAllDBOpen(t)
	n := len(rec.snapshot())
	time.Sleep(200 * time.Millisecond)
	require.Len(t, rec.snapshot(), n)
}

// Stop is idempotent, including when called concurrently.
func TestStop_Idempotent(t *testing.T) {
	rec := observeJobs(t)
	srv := startEmbedded(t, 0)

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = srv.Stop(context.Background()) }()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.NoError(t, srv.Stop(context.Background()))
	rec.requireAllDBOpen(t)
}

// The embedded lifecycle supports Start -> Stop -> Start in one process: the
// first instance's cancelled burst must not run against its closed handle
// while the second instance runs its own.
func TestStartStopStart_FirstBurstDoesNotLeak(t *testing.T) {
	rec := observeJobs(t)
	const delay = 200 * time.Millisecond

	first := startEmbedded(t, delay)
	firstDB := first.DB()
	require.NoError(t, first.Stop(context.Background()))

	second := startEmbedded(t, delay)
	select {
	case <-rec.first:
	case <-time.After(20 * time.Second):
		t.Fatal("second instance's catch-up never ran")
	}
	require.NoError(t, second.Stop(context.Background()))

	rec.requireAllDBOpen(t)
	require.NotSame(t, firstDB, second.DB())
}

func TestInitialRunner_ShutdownBoundedByContext(t *testing.T) {
	r := &initialRunner{}
	release := make(chan struct{})
	started := make(chan struct{})
	r.spawn(func() { close(started); <-release })
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.shutdown(ctx)
	require.ErrorIs(t, err, context.Canceled)

	// After shutdown a late spawn (a timer callback already running) is dropped.
	ran := false
	r.spawn(func() { ran = true })
	close(release)
	require.NoError(t, r.shutdown(context.Background()))
	require.False(t, ran)
}

// Stop returns ctx's error when catch-up jobs outlive its deadline, but still
// closes the database.
func TestStop_ContextExpiryStillCloses(t *testing.T) {
	srv := startEmbedded(t, 0)
	release := make(chan struct{})
	started := make(chan struct{})
	srv.bg.spawn(func() { close(started); <-release })
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := srv.Stop(ctx)
	require.ErrorIs(t, err, context.Canceled)
	sqlDB, dbErr := srv.db.DB()
	require.NoError(t, dbErr)
	require.Error(t, sqlDB.Ping())
	close(release)
	// The cancelled Stop returned without waiting, so the real catch-up jobs
	// (delay 0) are still running against the closed DB and logging. Drain them
	// before returning, or they race the next test's logger.InitLogger write to
	// zerolog's globals (a -race failure in TestStartStop_Lifecycle).
	require.NoError(t, srv.bg.shutdown(context.Background()))
}
