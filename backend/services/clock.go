package services

import (
	"sync/atomic"
	"time"

	"mycorrhizal/internal/clock"
)

// clockHolder wraps the Clock so atomic.Value always stores one concrete type
// (it panics on mixed dynamic types, and a clock.Fake and clock.System differ).
type clockHolder struct{ c clock.Clock }

var serviceClock atomic.Value // clockHolder

// SetClock installs the process-wide Clock the services layer reads "now"
// from (issue #1494) and returns a function restoring the previous one. A nil
// Clock means the system clock. The services are free functions rather than
// constructed objects, so unlike middleware and controllers (which read the
// clock off the gin context) they share this one seam — the same shape as the
// controllers' old timeNow var, but concurrency-safe: background goroutines
// (job loops, async token touches) read it while a test swaps it.
func SetClock(c clock.Clock) (restore func()) {
	prev := serviceClock.Load()
	serviceClock.Store(clockHolder{c: clock.Or(c)})
	return func() {
		if prev == nil {
			serviceClock.Store(clockHolder{c: clock.System{}})
			return
		}
		serviceClock.Store(prev)
	}
}

// Now is the services layer's current instant: the clock installed by
// SetClock, else the wall clock.
func Now() time.Time {
	if h, ok := serviceClock.Load().(clockHolder); ok {
		return h.c.Now()
	}
	return time.Now() // rawtime:allow bootstrap default before any clock is installed; identical to clock.System
}
