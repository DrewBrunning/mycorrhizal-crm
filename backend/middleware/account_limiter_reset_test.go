package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountRateLimiter_Reset_ClearsEveryTable(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())
	const id, ip = "reset_target", "203.0.113.9"

	for i := 0; i < MaxLoginAttempts; i++ {
		a.RecordLoginFailure(id, ip)
	}
	a.RecordLoginSuccess("someone_else", "198.51.100.4") // seeds the known-good IP tables
	sprayFailures(a, clk, 3)

	locked, _ := a.IsLoginLocked(id, ip)
	require.True(t, locked, "precondition: lockout must be in force")
	require.Positive(t, a.EntryCount())
	require.Positive(t, a.GlobalEntryCount())
	require.Positive(t, a.KnownGoodIPCount("someone_else"))
	require.Positive(t, a.GlobalKnownGoodIPCount())
	require.Positive(t, a.AuthVelocity().Failures)

	a.Reset()

	locked, _ = a.IsLoginLocked(id, ip)
	assert.False(t, locked, "lockout must not survive Reset")
	assert.Zero(t, a.EntryCount())
	assert.Zero(t, a.GlobalEntryCount())
	assert.Zero(t, a.KnownGoodIPCount("someone_else"))
	assert.Zero(t, a.GlobalKnownGoodIPCount())
	assert.Zero(t, a.AuthVelocity().Failures, "velocity history must be dropped")

	// Still fully functional afterwards (maps re-made, not nil).
	for i := 0; i < MaxLoginAttempts; i++ {
		a.RecordLoginFailure(id, ip)
	}
	locked, _ = a.IsLoginLocked(id, ip)
	assert.True(t, locked, "limiter must keep enforcing after Reset")
}

func TestAccountRateLimiter_Reset_KeepsVelocityConfigAndClock(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())
	before := a.velocity
	a.Reset()
	assert.NotSame(t, before, a.velocity, "history must be fresh")
	assert.Equal(t, before.cfg, a.velocity.cfg, "configured thresholds must survive Reset")
	assert.Equal(t, clk.Now(), a.velocity.now(), "injected clock must survive Reset")
}

func TestAccountRateLimiter_Reset_NilVelocityIsSafe(t *testing.T) {
	a := NewAccountRateLimiter(time.Hour)
	a.velocity = nil
	assert.NotPanics(t, a.Reset)
}
