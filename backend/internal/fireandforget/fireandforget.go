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

var wg sync.WaitGroup

// Run executes fn in a new tracked goroutine. Wait blocks until every goroutine
// launched through Run has returned.
func Run(fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn()
	}()
}

// Wait blocks until every goroutine launched through Run has returned.
func Wait() { wg.Wait() }

// WaitContext is Wait bounded by ctx: it returns nil once every tracked
// goroutine has returned, or ctx.Err() if ctx ends first (the goroutines keep
// running; only the wait is abandoned).
func WaitContext(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
