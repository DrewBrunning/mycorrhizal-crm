// Package fireandforget tracks the goroutines the application launches as
// fire-and-forget work (webhook fan-out, audit writes) so test teardown can
// wait for them.
//
// Production never waits on the tracker — the goroutines are asynchronous by
// design — but a test helper can call Wait before closing a per-test database:
// handlers spawn goroutines that hold the test's *gorm.DB, and if the database
// file is deleted by t.TempDir() while one of them still has a connection
// open, modernc/sqlite recreates a transient "001" temp directory and the
// cleanup fails with "directory not empty" (issue #703). Draining first
// guarantees no connection is in use when the file is removed.
//
// The package deliberately imports nothing else so every package that builds
// per-test databases — including internal/dbtest — can use it without an
// import cycle.
package fireandforget

import "sync"

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
