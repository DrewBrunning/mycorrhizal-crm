package middleware

import "time"

// Reset drops every failure counter, lockout, known-good-IP record and the
// instance-wide velocity history, returning the limiter to its just-constructed
// state while keeping the configured velocity thresholds and clock.
//
// It exists for tests (issue #1492). The process-wide limiter behind
// GetAccountRateLimiter is keyed by username/IP, and a lockout it records
// outlives the test that caused it: under `go test -count=2` or `-shuffle` a
// later run of the same test (same deterministic username) starts already
// locked and fails. Test helpers call it at setup and again in t.Cleanup.
// Production code must not call it: it would silently clear a live brute-force
// lockout.
func (a *AccountRateLimiter) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accounts = make(map[string]*AccountLockoutEntry)
	a.global = make(map[string]*AccountLockoutEntry)
	a.knownGoodIPs = make(map[string]map[string]time.Time)
	a.knownGoodGlobalIPs = make(map[string]time.Time)
	if a.velocity != nil {
		a.velocity = newAuthVelocityState(a.velocity.cfg, a.velocity.now)
	}
}
