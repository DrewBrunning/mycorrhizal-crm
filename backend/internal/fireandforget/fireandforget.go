// Package fireandforget tracks the goroutines the application launches as
// fire-and-forget work (webhook fan-out, audit writes) so test teardown can
// wait for them.
//
// Requests never wait on the tracker — the goroutines are asynchronous by
// design. Shutdown does, bounded (WaitContext in embedded.Server.stop), so
// in-flight work such as a password-reset mail send (issue #1554) finishes
// before the database closes. A test helper calls Wait before closing a
// per-test database:
// handlers spawn goroutines that hold the test's *gorm.DB, and if the database
// file is deleted by t.TempDir() while one of them still has a connection
// open, modernc/sqlite recreates a transient "001" temp directory and the
// cleanup fails with "directory not empty" (issue #703). Draining first
// guarantees no connection is in use when the file is removed.
//
// The package deliberately imports nothing but the standard library so every
// package that builds
// per-test databases — including internal/dbtest — can use it without an
// import cycle.
package fireandforget

import (
	"context"
	"sync"
)

// The tracker is a counter plus a condition variable, not a sync.WaitGroup.
// WaitContext deliberately abandons its wait when the context ends (the
// goroutines keep running), and a WaitGroup must not be reused by a later Add
// while such an abandoned Wait is still in flight — the race detector catches
// exactly that when the next test (or the next embedded Start/Stop cycle) calls
// Run, because the abandoned Wait's internal state write races the new Add's
// read (issue #1638). A mutex+Cond has no reuse hazard: an abandoned waiter
// simply wakes on the next Broadcast, and Add/Wait never race.
var (
	mu     sync.Mutex
	cond   = sync.NewCond(&mu)
	active int
)

// Run executes fn in a new tracked goroutine. Wait blocks until every goroutine
// launched through Run has returned.
func Run(fn func()) {
	mu.Lock()
	active++
	mu.Unlock()
	go func() {
		defer finish()
		fn()
	}()
}

func finish() {
	mu.Lock()
	active--
	if active == 0 {
		cond.Broadcast()
	}
	mu.Unlock()
}

// Wait blocks until every goroutine launched through Run has returned.
func Wait() {
	mu.Lock()
	for active > 0 {
		cond.Wait()
	}
	mu.Unlock()
}

// WaitContext is Wait bounded by ctx: it returns nil once every tracked
// goroutine has returned, or ctx.Err() if ctx ends first (the goroutines keep
// running; only the wait is abandoned).
func WaitContext(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
