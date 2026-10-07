// Package clock is the injectable source of "now" for time-dependent
// business logic (issue #1494): token / session / lockout / idempotency
// expiry, reminder scheduling, cadence, retention.
//
// Production code takes a Clock instead of calling time.Now() so a test can
// pin or advance time deterministically — no real time.Sleep, and day-
// boundary / DST / expiry-edge behaviour is exercised at exact instants. The
// repo-local analyzer internal/lint/rawtime fails a raw time.Now() in
// controllers, services and middleware outside a reason-bearing allowlist.
package clock

import (
	"sync"
	"time"
)

// Clock reports the current instant.
type Clock interface {
	Now() time.Time
}

// System is the production Clock: the wall clock.
type System struct{}

// Now returns time.Now().
func (System) Now() time.Time { return time.Now() }

// Func adapts a plain func() time.Time (e.g. an existing `time.Now` seam) to
// a Clock.
type Func func() time.Time

// Now calls the function.
func (f Func) Now() time.Time { return f() }

// Fake is a manually-driven Clock for tests. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake pinned at t.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now returns the pinned instant.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set pins the clock at t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// Advance moves the clock by d (a negative d moves it back).
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Or returns c, or System{} when c is nil — for struct fields whose zero
// value must mean "the real clock".
func Or(c Clock) Clock {
	if c == nil {
		return System{}
	}
	return c
}

// ContextKey is the gin context key Install stores the request's Clock
// under, the same way "db" and the config are injected by routes.go.
const ContextKey = "clock"

type getter interface {
	Get(key any) (any, bool)
}

type setter interface {
	Set(key any, value any)
}

// FromContext returns the Clock stored under ContextKey (by Install), or
// System{} when none was installed — a handler mounted without the routes.go
// wiring, or a test that does not care about time, gets the real clock. It
// takes an interface so this package does not import gin; *gin.Context
// satisfies it.
func FromContext(g getter) Clock {
	if v, ok := g.Get(ContextKey); ok {
		if c, ok := v.(Clock); ok && c != nil {
			return c
		}
	}
	return System{}
}

// Install stores c (System{} when nil) under ContextKey.
func Install(s setter, c Clock) { s.Set(ContextKey, Or(c)) }

// InstallIfAbsent stores c (System{} when nil) under ContextKey unless a
// Clock is already installed — so a test router can pre-install a Fake before
// routes.RegisterRoutes wires the production default.
func InstallIfAbsent(g interface {
	getter
	setter
}, c Clock) {
	if _, ok := g.Get(ContextKey); ok {
		return
	}
	Install(g, c)
}
