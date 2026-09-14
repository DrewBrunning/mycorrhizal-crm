package middleware

import (
	"sort"
	"time"
)

// Instance-wide failed-authentication velocity detection (issue #940).
//
// The per-(identifier, source-IP) lockout and the per-identifier backstop in
// login_lockout.go both stop a *single-source* attack: one IP against one
// account, or one attacker rotating a handful of IPs against a known account.
// They share a blind spot — a distributed password spray (one common password
// tried against thousands of accounts from a botnet) keeps every individual
// budget under its threshold. No per-account or per-IP key ever crosses its
// limit, so the instance sees nothing and no alert fires.
//
// This file adds the missing dimension: failures-per-window aggregated across
// ALL identifiers. It is deliberately keyed on the *shape* of the traffic —
// how many distinct accounts failed, not just how many failures happened — so
// a single account being brute-forced (one identifier, however many IPs) does
// not trip the instance-wide signal, while accounts being sprayed in bulk do.
//
// When the signal trips it does two things:
//
//  1. Engages a short, instance-wide login throttle. `IsLoginLocked` refuses
//     attempts from sources that have not recently authenticated, so the
//     botnet is stopped without locking out the returning legitimate user
//     (both the per-identifier known-good set and an instance-wide known-good
//     IP set are honoured). The throttle is bounded and self-clearing — an
//     attacker who trips it deliberately can annoy users for its short
//     duration, but cannot sustain a denial of service from it (the same
//     bounded-amplification posture as the #867 per-identifier backstop).
//
//  2. Holds an "incident" latch long enough for the polled alert evaluator
//     (services.EvaluateAlerts, ALERT_EVAL_INTERVAL_MINUTES) to observe it and
//     dispatch the auth_spray raise — a spike that starts and ends between two
//     evaluations must still page the operator. The latch clears after a quiet
//     hold window, at which point the evaluator dispatches the recovery.
//
// State is in-memory and per-process like the rest of the rate limiter
// (single-process deployment, ADR 0004 family); a restart clears the signal.
// That is a deliberate trade — the alternative would put a write on the auth
// hot path — and the alert is still raised for any spray that continues past
// the restart. See docs/security/asvs-l2.md V11.1.7/V11.1.8.

// Defaults for the velocity signal. They are deliberately loose enough that
// ordinary failed-login noise never trips them, and the identifier count is
// the discriminator: a spray must touch many accounts, not just fail a lot.
const (
	// DefaultAuthSprayWindowSeconds is the sliding window velocity is measured
	// over.
	DefaultAuthSprayWindowSeconds = 60
	// DefaultAuthSprayFailureThreshold is the number of failures within the
	// window that arms the signal.
	DefaultAuthSprayFailureThreshold = 60
	// DefaultAuthSprayIdentifierThreshold is the number of DISTINCT identifiers
	// that must fail within the window for the signal to trip. This is what
	// separates a distributed spray from a single-account brute force.
	DefaultAuthSprayIdentifierThreshold = 15
	// DefaultAuthSprayThrottleSeconds is how long a tripped signal refuses
	// attempts from sources that have not recently authenticated.
	DefaultAuthSprayThrottleSeconds = 300

	// authSprayTargetedIPThreshold is the number of distinct source IPs at or
	// above which an identifier is reported as "targeted from many IPs" — the
	// greylist signal the issue asks us to surface (we report it; we do not
	// auto-lock it, because an identifier failing from many IPs is usually the
	// *victim*, and locking it would recreate the #867 griefing footgun).
	authSprayTargetedIPThreshold = 5
	// authSprayMaxReportedOffenders caps the targeted-identifier list a
	// snapshot carries so a huge spray cannot bloat an alert payload.
	authSprayMaxReportedOffenders = 20
	// authSprayFailureSliceCap bounds the number of live failure timestamps
	// held. It is far above any threshold that matters, so reaching it only
	// means the signal is already tripped; the cap stops a sustained flood
	// growing the log without bound.
	authSprayFailureSliceCap = 100000
	// authSprayPruneDenominator sets how often (window / this) the distinct-key
	// maps are swept. Pruning every map on every request would make the cost
	// O(distinct) per request — O(n^2) under a distributed flood — so it is
	// amortised. Between sweeps a stale key can inflate the distinct count
	// slightly, which only makes the signal trip *earlier* (fail-closed).
	authSprayPruneDenominator = 4
)

// AuthVelocityConfig configures the instance-wide failed-auth velocity signal.
type AuthVelocityConfig struct {
	Enabled             bool
	Window              time.Duration
	FailureThreshold    int
	IdentifierThreshold int
	Throttle            time.Duration
	// IncidentHold is how long a trip stays "in incident" for the polled alert
	// evaluator. It must be at least two evaluation intervals so a spike
	// cannot start and end between two runs unseen; it is independent of
	// Throttle, which is the (shorter) login-refusal window.
	IncidentHold time.Duration
}

// DefaultAuthVelocityConfig returns the safe defaults above: enabled, with the
// incident hold equal to the throttle (production passes a longer hold derived
// from the alert evaluation interval).
func DefaultAuthVelocityConfig() AuthVelocityConfig {
	return AuthVelocityConfig{
		Enabled:             true,
		Window:              DefaultAuthSprayWindowSeconds * time.Second,
		FailureThreshold:    DefaultAuthSprayFailureThreshold,
		IdentifierThreshold: DefaultAuthSprayIdentifierThreshold,
		Throttle:            DefaultAuthSprayThrottleSeconds * time.Second,
		IncidentHold:        DefaultAuthSprayThrottleSeconds * time.Second,
	}
}

// AuthVelocitySnapshot is an instantaneous read of the signal, consumed by the
// alert evaluator and by tests/monitoring.
type AuthVelocitySnapshot struct {
	Enabled         bool
	Failures        int
	Identifiers     int
	SourceIPs       int
	WindowSeconds   int
	Throttled       bool
	RetryAfterSecs  int
	Incident        bool
	IncidentSince   time.Time
	PeakFailures    int
	PeakIdentifiers int
	// TargetedIdentifiers names identifiers that failed from at least
	// authSprayTargetedIPThreshold distinct source IPs within the window,
	// sorted and capped. Reported for operator awareness, never auto-locked.
	TargetedIdentifiers []string
}

// authVelocityState is the signal's state. It is guarded by the owning
// AccountRateLimiter's mutex (a.mu) — the record path runs under the same lock
// as the per-pair/global counters so a single failure is accounted atomically,
// and reads take the write lock because pruning mutates the maps.
type authVelocityState struct {
	cfg AuthVelocityConfig
	now func() time.Time

	// failures is a window-ordered log of failure timestamps. head is the
	// index of the oldest still-live entry; advancing it evicts, and the dead
	// prefix is compacted amortised (see prune) so a sustained flood costs
	// O(1) per record rather than a slice copy. failureCap is the safety valve
	// that stops a flood above the threshold growing the log without bound.
	failures   []time.Time
	head       int
	failureCap int

	idLast   map[string]time.Time
	ipLast   map[string]time.Time
	idIPLast map[string]map[string]time.Time

	lastPrune time.Time

	throttleUntil   time.Time
	incidentUntil   time.Time
	incidentSince   time.Time
	peakFailures    int
	peakIdentifiers int
}

func newAuthVelocityState(cfg AuthVelocityConfig, now func() time.Time) *authVelocityState {
	d := DefaultAuthVelocityConfig()
	if cfg.Window <= 0 {
		cfg.Window = d.Window
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = d.FailureThreshold
	}
	if cfg.IdentifierThreshold <= 0 {
		cfg.IdentifierThreshold = d.IdentifierThreshold
	}
	if cfg.Throttle <= 0 {
		cfg.Throttle = d.Throttle
	}
	if cfg.IncidentHold < cfg.Throttle {
		cfg.IncidentHold = cfg.Throttle
	}
	if now == nil {
		now = time.Now
	}
	return &authVelocityState{
		cfg:        cfg,
		now:        now,
		failureCap: authSprayFailureSliceCap,
		idLast:     make(map[string]time.Time),
		ipLast:     make(map[string]time.Time),
		idIPLast:   make(map[string]map[string]time.Time),
	}
}

// failureCount is the number of live (in-window) failures.
func (v *authVelocityState) failureCount() int { return len(v.failures) - v.head }

// record accounts one failed authentication across the instance. Caller holds
// a.mu (write).
func (v *authVelocityState) record(identifier, ip string) {
	if !v.cfg.Enabled {
		return
	}
	now := v.now()
	v.prune(now)

	v.failures = append(v.failures, now)

	v.idLast[identifier] = now
	v.ipLast[ip] = now
	ips := v.idIPLast[identifier]
	if ips == nil {
		ips = make(map[string]time.Time)
		v.idIPLast[identifier] = ips
	}
	ips[ip] = now

	if v.failureCount() >= v.cfg.FailureThreshold && len(v.idLast) >= v.cfg.IdentifierThreshold {
		v.trip(now)
	}
}

// trip arms (or extends) the throttle and incident windows at now.
func (v *authVelocityState) trip(now time.Time) {
	newIncident := !now.Before(v.incidentUntil)
	v.throttleUntil = now.Add(v.cfg.Throttle)
	v.incidentUntil = now.Add(v.cfg.IncidentHold)
	if newIncident {
		v.incidentSince = now
		v.peakFailures = v.failureCount()
		v.peakIdentifiers = len(v.idLast)
		return
	}
	if v.failureCount() > v.peakFailures {
		v.peakFailures = v.failureCount()
	}
	if len(v.idLast) > v.peakIdentifiers {
		v.peakIdentifiers = len(v.idLast)
	}
}

// prune evicts failures and distinct keys older than the window. The failure
// log is evicted by advancing head, with the dead prefix compacted only once it
// is at least half the slice (so the copy is amortised O(1) per record even
// under a sustained flood); the maps are swept on the slower schedule described
// on authSprayPruneDenominator.
func (v *authVelocityState) prune(now time.Time) {
	windowStart := now.Add(-v.cfg.Window)

	for v.head < len(v.failures) && v.failures[v.head].Before(windowStart) {
		v.head++
	}
	if v.head > 0 && v.head*2 >= len(v.failures) {
		v.failures = append(v.failures[:0], v.failures[v.head:]...)
		v.head = 0
	}
	// Safety valve: never hold more than failureCap live timestamps, so a
	// flood far above the threshold cannot grow the log without bound.
	if n := v.failureCount(); n > v.failureCap {
		v.head = len(v.failures) - v.failureCap
	}

	if !v.lastPrune.IsZero() && now.Sub(v.lastPrune) < v.cfg.Window/authSprayPruneDenominator {
		return
	}
	v.lastPrune = now

	for k, t := range v.idLast {
		if t.Before(windowStart) {
			delete(v.idLast, k)
		}
	}
	for k, t := range v.ipLast {
		if t.Before(windowStart) {
			delete(v.ipLast, k)
		}
	}
	for id, ips := range v.idIPLast {
		for ip, t := range ips {
			if t.Before(windowStart) {
				delete(ips, ip)
			}
		}
		if len(ips) == 0 {
			delete(v.idIPLast, id)
		}
	}
}

func (v *authVelocityState) throttled() bool {
	return v.cfg.Enabled && v.now().Before(v.throttleUntil)
}

func (v *authVelocityState) remaining() int {
	now := v.now()
	if !v.cfg.Enabled || !now.Before(v.throttleUntil) {
		return 0
	}
	secs := int(v.throttleUntil.Sub(now).Seconds())
	if secs < 1 {
		secs = 1
	}
	return secs
}

func (v *authVelocityState) inIncident() bool {
	return v.cfg.Enabled && v.now().Before(v.incidentUntil)
}

// snapshot returns the current signal and prunes first so the numbers are
// window-accurate. Caller holds a.mu (write).
func (v *authVelocityState) snapshot() AuthVelocitySnapshot {
	now := v.now()
	v.prune(now)
	return AuthVelocitySnapshot{
		Enabled:             v.cfg.Enabled,
		Failures:            v.failureCount(),
		Identifiers:         len(v.idLast),
		SourceIPs:           len(v.ipLast),
		WindowSeconds:       int(v.cfg.Window.Seconds()),
		Throttled:           v.throttled(),
		RetryAfterSecs:      v.remaining(),
		Incident:            v.inIncident(),
		IncidentSince:       v.incidentSince,
		PeakFailures:        v.peakFailures,
		PeakIdentifiers:     v.peakIdentifiers,
		TargetedIdentifiers: v.targeted(now),
	}
}

// targeted collects identifiers that failed from many distinct source IPs
// within the window.
func (v *authVelocityState) targeted(now time.Time) []string {
	windowStart := now.Add(-v.cfg.Window)
	var out []string
	for id, ips := range v.idIPLast {
		n := 0
		for _, t := range ips {
			if !t.Before(windowStart) {
				n++
			}
		}
		if n >= authSprayTargetedIPThreshold {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	if len(out) > authSprayMaxReportedOffenders {
		out = out[:authSprayMaxReportedOffenders]
	}
	return out
}
