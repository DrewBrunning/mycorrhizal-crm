package middleware

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// velocityClock is a controllable clock for the velocity signal. The signal
// owns its own now() func, so tests can fast-forward the window, throttle and
// incident hold deterministically instead of sleeping.
type velocityClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *velocityClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *velocityClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// newVelocityLimiter returns an AccountRateLimiter isolated from the
// process-wide singleton with the velocity clock replaced by a fake. The
// per-pair/global backoff still uses real time (and long enough TTLs not to
// interfere); only the velocity signal is on the fake clock.
func newVelocityLimiter(cfg AuthVelocityConfig) (*AccountRateLimiter, *velocityClock) {
	clk := &velocityClock{t: time.Now()}
	a := NewAccountRateLimiter(time.Hour)
	a.mu.Lock()
	a.velocity = newAuthVelocityState(cfg, clk.Now)
	a.mu.Unlock()
	return a, clk
}

func velocityTestConfig() AuthVelocityConfig {
	return AuthVelocityConfig{
		Enabled:             true,
		Window:              60 * time.Second,
		FailureThreshold:    10,
		IdentifierThreshold: 5,
		Throttle:            2 * time.Minute,
		IncidentHold:        10 * time.Minute,
	}
}

// sprayFailures records n distinct-identifier, distinct-IP failures (one each).
func sprayFailures(a *AccountRateLimiter, clk *velocityClock, n int) {
	for i := 0; i < n; i++ {
		clk.Advance(time.Millisecond)
		a.RecordLoginFailure(fmt.Sprintf("victim%d@example.com", i), fmt.Sprintf("198.51.100.%d", i%250))
	}
}

// TestAuthVelocity_ManyIdentifiersManyIPsTrips is the issue #940 core case: a
// distributed password spray where every individual (identifier, IP) budget
// stays under its limit still crosses the instance-wide signal, and unknown
// sources are then refused.
func TestAuthVelocity_ManyIdentifiersManyIPsTrips(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	// 5 distinct identifiers from 5 distinct IPs, 2 failures each = 10 total.
	for round := 0; round < 2; round++ {
		for j := 0; j < 5; j++ {
			clk.Advance(time.Millisecond)
			a.RecordLoginFailure(fmt.Sprintf("victim%d@example.com", j), fmt.Sprintf("198.51.100.%d", j))
		}
	}

	// No single pair ever hit its per-pair limit, so the lock cannot be
	// credited to the existing per-identifier/IP defenses.
	for j := 0; j < 5; j++ {
		if got := a.GetFailedAttempts(LoginKey(fmt.Sprintf("victim%d@example.com", j), fmt.Sprintf("198.51.100.%d", j))); got >= MaxLoginAttempts {
			t.Fatalf("pair %d has %d failures; the test must stay under the per-pair limit", j, got)
		}
	}

	snap := a.AuthVelocity()
	if !snap.Incident {
		t.Fatalf("a many-identifier/many-IP spray must trip the instance signal: %+v", snap)
	}
	if !snap.Throttled {
		t.Fatalf("a tripped signal must engage the login throttle: %+v", snap)
	}
	if snap.Identifiers < 5 {
		t.Errorf("snapshot identifiers = %d, want >= 5", snap.Identifiers)
	}
	if locked, secs := a.IsLoginLocked("never-seen@example.com", "203.0.113.200"); !locked || secs <= 0 {
		t.Fatalf("a never-seen source must be refused while the throttle is active (locked=%v secs=%d)", locked, secs)
	}
}

// TestAuthVelocity_SingleAccountBruteForceDoesNotTrip: hammering one account
// from many source IPs is a targeted attack, not a spray. The distinct
// identifier count stays at 1, so the instance-wide signal must not fire — the
// per-identifier backstop already covers it, and a global lock here would be
// the #867 griefing footgun.
func TestAuthVelocity_SingleAccountBruteForceDoesNotTrip(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	for i := 0; i < 25; i++ { // > failureThreshold, but one identifier
		clk.Advance(time.Millisecond)
		a.RecordLoginFailure("victim@example.com", fmt.Sprintf("198.51.100.%d", i))
	}

	snap := a.AuthVelocity()
	if snap.Identifiers != 1 {
		t.Fatalf("snapshot identifiers = %d, want 1", snap.Identifiers)
	}
	if snap.Incident {
		t.Fatalf("one identifier failing from many IPs must not trip the instance signal: %+v", snap)
	}
}

// TestAuthVelocity_BelowIdentifierThresholdDoesNotTrip: enough total failures
// but too few distinct accounts is not the distributed shape.
func TestAuthVelocity_BelowIdentifierThresholdDoesNotTrip(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	// 4 identifiers (below the threshold of 5) x 5 failures = 20 total.
	for i := 0; i < 5; i++ {
		for j := 0; j < 4; j++ {
			clk.Advance(time.Millisecond)
			a.RecordLoginFailure(fmt.Sprintf("u%d@example.com", j), fmt.Sprintf("ip-%d", i))
		}
	}

	if snap := a.AuthVelocity(); snap.Incident {
		t.Fatalf("few distinct identifiers must not trip the signal even past the failure count: %+v", snap)
	}
}

// TestAuthVelocity_BelowFailureThresholdDoesNotTrip: many distinct accounts but
// under the failure count is normal failed-login noise.
func TestAuthVelocity_BelowFailureThresholdDoesNotTrip(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	for i := 0; i < 4; i++ { // 4 failures, 4 identifiers, threshold is 10
		clk.Advance(time.Millisecond)
		a.RecordLoginFailure(fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("198.51.100.%d", i))
	}

	if snap := a.AuthVelocity(); snap.Incident {
		t.Fatalf("sub-threshold noise must not trip the signal: %+v", snap)
	}
}

// TestAuthVelocity_ThrottleExemptsRecentAuthSource: the instance throttle must
// stop the botnet without denying the legitimate returning user. A source that
// recently authenticated — for the same identifier or any other — is exempt.
func TestAuthVelocity_ThrottleExemptsRecentAuthSource(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	a.RecordLoginSuccess("legit@example.com", "203.0.113.7")
	sprayFailures(a, clk, 10)
	if !a.AuthVelocity().Throttled {
		t.Fatal("precondition: the spray must trip the throttle")
	}

	if locked, _ := a.IsLoginLocked("legit@example.com", "203.0.113.7"); locked {
		t.Error("a source that just authenticated for this identifier must bypass the instance throttle")
	}
	if locked, _ := a.IsLoginLocked("someone-else@example.com", "203.0.113.7"); locked {
		t.Error("an IP that recently authenticated must bypass the instance throttle for any identifier")
	}
	if locked, _ := a.IsLoginLocked("someone-else@example.com", "203.0.113.8"); !locked {
		t.Error("an unrelated unseen source must still be refused")
	}
}

// TestAuthVelocity_ThrottleExpiresButIncidentHeldForEvaluator: the login
// throttle is short and self-clearing, but the incident latch outlives it so
// the polled alert evaluator cannot miss a spike that ends between runs.
func TestAuthVelocity_ThrottleExpiresButIncidentHeldForEvaluator(t *testing.T) {
	cfg := velocityTestConfig()
	a, clk := newVelocityLimiter(cfg)
	sprayFailures(a, clk, 10)

	clk.Advance(cfg.Throttle + time.Second)
	snap := a.AuthVelocity()
	if snap.Throttled {
		t.Fatalf("the throttle must clear after its duration: %+v", snap)
	}
	if !snap.Incident {
		t.Fatalf("the incident must outlive the throttle so the evaluator sees it: %+v", snap)
	}
	if locked, _ := a.IsLoginLocked("nobody@example.com", "203.0.113.99"); locked {
		t.Error("once the throttle clears, an unknown source is no longer refused by the instance signal")
	}

	clk.Advance(cfg.IncidentHold)
	if snap := a.AuthVelocity(); snap.Incident {
		t.Fatalf("the incident must clear after a quiet hold window: %+v", snap)
	}
}

// TestAuthVelocity_RetryAfterFloor: the reported retry_after never truncates to
// zero while the throttle is still active, so a 429 never advertises "retry in
// 0 seconds".
func TestAuthVelocity_RetryAfterFloor(t *testing.T) {
	cfg := velocityTestConfig()
	a, clk := newVelocityLimiter(cfg)
	sprayFailures(a, clk, 10)

	clk.Advance(cfg.Throttle - 500*time.Millisecond)
	snap := a.AuthVelocity()
	if !snap.Throttled {
		t.Fatal("precondition: the throttle should still be active")
	}
	if snap.RetryAfterSecs != 1 {
		t.Fatalf("retry_after = %d, want the 1s floor", snap.RetryAfterSecs)
	}
}

// TestAuthVelocity_NilStateIsSafe: the throttle check tolerates a nil tracker
// (never the case via the constructor, but the guard is cheap and pinned).
func TestAuthVelocity_NilStateIsSafe(t *testing.T) {
	a := NewAccountRateLimiter(time.Hour)
	a.mu.Lock()
	a.velocity = nil
	a.mu.Unlock()

	if locked, _ := a.IsLoginLocked("x@example.com", "203.0.113.9"); locked {
		t.Error("a nil velocity state must not refuse a login")
	}
}

// TestAuthVelocity_WindowEvictsStaleFailures: failures older than the window
// stop counting, so the signal is a rate, not a permanent counter.
func TestAuthVelocity_WindowEvictsStaleFailures(t *testing.T) {
	cfg := velocityTestConfig()
	a, clk := newVelocityLimiter(cfg)

	for i := 0; i < 9; i++ {
		clk.Advance(time.Millisecond)
		a.RecordLoginFailure("victim@example.com", fmt.Sprintf("198.51.100.%d", i))
	}
	if snap := a.AuthVelocity(); snap.Failures != 9 {
		t.Fatalf("snapshot failures = %d, want 9", snap.Failures)
	}

	clk.Advance(cfg.Window + time.Second)
	snap := a.AuthVelocity()
	if snap.Failures != 0 || snap.Identifiers != 0 || snap.SourceIPs != 0 {
		t.Fatalf("the window must evict stale failures and keys: %+v", snap)
	}
}

// TestAuthVelocity_TargetedIdentifiersReported: an identifier failing from many
// distinct IPs is surfaced as a greylist candidate. It is deliberately reported
// rather than auto-locked (the identifier is usually the victim).
func TestAuthVelocity_TargetedIdentifiersReported(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	for i := 0; i < authSprayTargetedIPThreshold; i++ {
		clk.Advance(time.Millisecond)
		a.RecordLoginFailure("target@example.com", fmt.Sprintf("203.0.113.%d", i))
	}

	snap := a.AuthVelocity()
	if len(snap.TargetedIdentifiers) != 1 || snap.TargetedIdentifiers[0] != "target@example.com" {
		t.Fatalf("targeted identifiers = %v, want [target@example.com]", snap.TargetedIdentifiers)
	}
}

// TestAuthVelocity_TargetedIdentifiersCapped keeps an alert payload bounded
// under a spray against a huge number of accounts.
func TestAuthVelocity_TargetedIdentifiersCapped(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())

	for id := 0; id < authSprayMaxReportedOffenders+5; id++ {
		for ip := 0; ip < authSprayTargetedIPThreshold; ip++ {
			clk.Advance(time.Millisecond)
			a.RecordLoginFailure(fmt.Sprintf("acct%d@example.com", id), fmt.Sprintf("10.%d.%d.1", id%250, ip))
		}
	}

	snap := a.AuthVelocity()
	if len(snap.TargetedIdentifiers) > authSprayMaxReportedOffenders {
		t.Fatalf("targeted list has %d entries, cap is %d", len(snap.TargetedIdentifiers), authSprayMaxReportedOffenders)
	}
}

// TestAuthVelocity_FailureLogIsCapped: the in-memory failure log cannot grow
// without bound under a flood far above the threshold.
func TestAuthVelocity_FailureLogIsCapped(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())
	a.mu.Lock()
	a.velocity.failureCap = 10
	a.mu.Unlock()

	for i := 0; i < 50; i++ {
		clk.Advance(time.Millisecond)
		a.RecordLoginFailure(fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("198.51.100.%d", i%250))
	}

	if got := a.AuthVelocity().Failures; got != 10 {
		t.Fatalf("live failure log = %d, want the cap 10", got)
	}
}

// TestAuthVelocity_DisabledNeverTrips: the master switch is a real off switch.
func TestAuthVelocity_DisabledNeverTrips(t *testing.T) {
	cfg := velocityTestConfig()
	cfg.Enabled = false
	a, clk := newVelocityLimiter(cfg)

	sprayFailures(a, clk, 50)

	snap := a.AuthVelocity()
	if snap.Enabled {
		t.Error("snapshot should report the signal disabled")
	}
	if snap.Incident || snap.Throttled {
		t.Fatalf("a disabled signal must never trip: %+v", snap)
	}
}

// TestAuthVelocity_DistinctKeysAcrossIdentifiers guards the snapshot's
// cardinality: one failure per identifier/IP is counted once each, not double.
func TestAuthVelocity_SnapshotCounts(t *testing.T) {
	a, clk := newVelocityLimiter(velocityTestConfig())
	sprayFailures(a, clk, 10)

	snap := a.AuthVelocity()
	if snap.Failures != 10 || snap.Identifiers != 10 || snap.SourceIPs != 10 {
		t.Fatalf("snapshot counts = failures:%d identifiers:%d ips:%d, want 10/10/10",
			snap.Failures, snap.Identifiers, snap.SourceIPs)
	}
	if snap.WindowSeconds != 60 {
		t.Errorf("window seconds = %d, want 60", snap.WindowSeconds)
	}
	if snap.PeakIdentifiers < 5 {
		t.Errorf("peak identifiers = %d, want >= 5", snap.PeakIdentifiers)
	}
}

// TestConfigureAuthVelocity_AppliesToSingleton: the startup hook installs the
// config on the process-wide limiter the login path uses.
func TestConfigureAuthVelocity_AppliesToSingleton(t *testing.T) {
	a := GetAccountRateLimiter()
	a.mu.Lock()
	orig := a.velocity
	a.mu.Unlock()
	t.Cleanup(func() {
		a.mu.Lock()
		a.velocity = orig
		a.mu.Unlock()
	})

	ConfigureAuthVelocity(AuthVelocityConfig{Enabled: false})
	if snap := a.AuthVelocity(); snap.Enabled {
		t.Fatal("ConfigureAuthVelocity did not apply the disabled config")
	}

	ConfigureAuthVelocity(velocityTestConfig())
	if snap := a.AuthVelocity(); !snap.Enabled || snap.WindowSeconds != 60 {
		t.Fatalf("ConfigureAuthVelocity did not apply the enabled config: %+v", snap)
	}
}

// TestCleanup_PrunesGlobalKnownGoodIP: the instance-wide known-good set ages
// out with the same TTL as the per-identifier set.
func TestCleanup_PrunesGlobalKnownGoodIP(t *testing.T) {
	a := NewAccountRateLimiter(time.Millisecond)
	a.RecordLoginSuccess("x@example.com", ipA)

	if a.GlobalKnownGoodIPCount() != 1 {
		t.Fatalf("known-good IP count = %d, want 1", a.GlobalKnownGoodIPCount())
	}

	// Age the entry past KnownGoodIPTTL and sweep.
	a.mu.Lock()
	a.knownGoodGlobalIPs[ipA] = time.Now().Add(-KnownGoodIPTTL - time.Hour)
	a.mu.Unlock()

	a.CleanupStaleAccountEntries()
	if a.GlobalKnownGoodIPCount() != 0 {
		t.Errorf("expired instance known-good IP survived cleanup: %d", a.GlobalKnownGoodIPCount())
	}
}
