package fireandforget

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunAndWait(t *testing.T) {
	var n atomic.Int64
	Run(func() {
		time.Sleep(50 * time.Millisecond)
		n.Add(1)
	})
	Wait()
	if got := n.Load(); got != 1 {
		t.Fatalf("tracked goroutine did not run: n=%d", got)
	}
}

func TestWaitBlocksUntilInFlightRuns(t *testing.T) {
	var n atomic.Int64
	Run(func() {
		time.Sleep(100 * time.Millisecond)
		n.Add(1)
	})
	Wait()
	// Wait must not return while a tracked goroutine is still mid-flight.
	if got := n.Load(); got != 1 {
		t.Fatalf("Wait returned before the tracked goroutine finished: n=%d", got)
	}
}

func TestRunLaunchesConcurrently(t *testing.T) {
	started := make(chan struct{})
	Run(func() {
		close(started)
		time.Sleep(10 * time.Millisecond)
	})
	select {
	case <-started:
		// Run must return before the goroutine body completes.
	case <-time.After(time.Second):
		t.Fatal("tracked goroutine never started")
	}
	Wait()
}
