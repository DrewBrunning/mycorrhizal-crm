package integrations

import (
	"math/rand"
	"net/http"
	"testing"
	"time"
)

func TestRetryPolicyBackoff_GrowsAndIsBounded(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 6, BaseDelay: time.Second, MaxDelay: time.Minute, Multiplier: 2}

	var prev time.Duration
	for attempt := 1; attempt <= 8; attempt++ {
		d := p.Backoff(attempt, nil) // nil rnd -> deterministic, no jitter
		if d < 0 {
			t.Fatalf("attempt %d: negative delay %s", attempt, d)
		}
		if d > p.MaxDelay {
			t.Fatalf("attempt %d: delay %s exceeds MaxDelay %s", attempt, d, p.MaxDelay)
		}
		if attempt > 1 && d < prev {
			t.Fatalf("attempt %d: delay %s went backwards from %s (no jitter, must be monotonic up to the cap)", attempt, d, prev)
		}
		prev = d
	}
	// The first retry waits BaseDelay exactly.
	if got := p.Backoff(1, nil); got != time.Second {
		t.Errorf("Backoff(1) = %s, want 1s", got)
	}
	// 1s * 2^2 = 4s for the third retry.
	if got := p.Backoff(3, nil); got != 4*time.Second {
		t.Errorf("Backoff(3) = %s, want 4s", got)
	}
	// Deep attempts saturate at the cap.
	if got := p.Backoff(20, nil); got != time.Minute {
		t.Errorf("Backoff(20) = %s, want the 1m cap", got)
	}
}

func TestRetryPolicyBackoff_DefensiveClamps(t *testing.T) {
	// attempt < 1 is treated as 1; Multiplier < 1 is treated as 1 (no shrink).
	p := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute, Multiplier: 0.5}
	if got := p.Backoff(0, nil); got != time.Second {
		t.Errorf("Backoff(0) = %s, want the BaseDelay (attempt clamped to 1)", got)
	}
	if got := p.Backoff(-5, nil); got != time.Second {
		t.Errorf("Backoff(-5) = %s, want 1s", got)
	}
	if got := p.Backoff(4, nil); got != time.Second {
		t.Errorf("Backoff(4) with Multiplier<1 = %s, want 1s (no shrink below BaseDelay)", got)
	}
	// A pathological jitter fraction >= 1 can compute a delay that is negative
	// (clamped to 0) or well above MaxDelay (clamped down, post-jitter).
	wild := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: 2 * time.Second, Multiplier: 1, JitterFrac: 5}
	rnd := rand.New(rand.NewSource(2))
	sawCap := false
	for i := 0; i < 400; i++ {
		d := wild.Backoff(1, rnd)
		if d < 0 {
			t.Fatalf("Backoff produced a negative delay %s", d)
		}
		if d > wild.MaxDelay {
			t.Fatalf("Backoff exceeded MaxDelay: %s", d)
		}
		if d == wild.MaxDelay {
			sawCap = true
		}
	}
	if !sawCap {
		t.Error("expected the post-jitter MaxDelay clamp to fire at least once")
	}
}

func TestClassifyHTTPStatus(t *testing.T) {
	cases := []struct {
		code   int
		want   FailureMode
		mapped bool
	}{
		{401, FailureAuthExpiry, true},
		{403, FailureAuthzRevoked, true},
		{404, FailureRemoteResourceDeleted, true},
		{410, FailureRemoteResourceDeleted, true},
		{408, FailureTimeout, true},
		{504, FailureTimeout, true},
		{429, FailureRateLimited, true},
		{503, FailureRateLimited, true},
		{400, "", false}, // request-level 4xx with no named mode
		{200, "", false}, // success
		{500, "", false},
	}
	for _, c := range cases {
		got, ok := ClassifyHTTPStatus(c.code)
		if got != c.want || ok != c.mapped {
			t.Errorf("ClassifyHTTPStatus(%d) = %q, %v; want %q, %v", c.code, got, ok, c.want, c.mapped)
		}
	}
}

func TestRetryPolicyBackoff_JitterWithinFraction(t *testing.T) {
	// MaxDelay is far above the computed base so the cap never clips the jitter
	// window one-sidedly.
	p := RetryPolicy{MaxAttempts: 5, BaseDelay: 10 * time.Second, MaxDelay: time.Hour, Multiplier: 2, JitterFrac: 0.2}
	rnd := rand.New(rand.NewSource(1))

	base := 20 * time.Second // Backoff(2) without jitter
	lo := time.Duration(float64(base) * 0.8)
	hi := time.Duration(float64(base) * 1.2)
	sawBelow, sawAbove := false, false
	for i := 0; i < 500; i++ {
		d := p.Backoff(2, rnd)
		if d < lo || d > hi {
			t.Fatalf("jittered delay %s outside [%s, %s]", d, lo, hi)
		}
		if d < base {
			sawBelow = true
		}
		if d > base {
			sawAbove = true
		}
	}
	if !sawBelow || !sawAbove {
		t.Errorf("jitter never varied both directions (below=%v above=%v)", sawBelow, sawAbove)
	}
}

func TestRetryPolicyNextDelay_HonorsRetryAfter(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 4, BaseDelay: time.Second, MaxDelay: time.Hour, Multiplier: 2}

	// Retry-After longer than the computed backoff wins.
	if got := p.NextDelay(1, 90*time.Second, nil); got != 90*time.Second {
		t.Errorf("NextDelay with long Retry-After = %s, want 90s", got)
	}
	// A shorter (or zero) Retry-After does not shrink the backoff.
	if got := p.NextDelay(3, 0, nil); got != 4*time.Second {
		t.Errorf("NextDelay(3) with no hint = %s, want the 4s backoff", got)
	}
	if got := p.NextDelay(3, time.Second, nil); got != 4*time.Second {
		t.Errorf("NextDelay(3) with a 1s hint = %s, want the 4s backoff", got)
	}
}

func TestRetryPolicyHasAttemptsLeft(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 3}
	if !p.HasAttemptsLeft(1) || !p.HasAttemptsLeft(2) {
		t.Errorf("attempts 1 and 2 should have budget left")
	}
	if p.HasAttemptsLeft(3) || p.HasAttemptsLeft(4) {
		t.Errorf("attempt 3+ is out of a 3-attempt budget")
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d, ok := ParseRetryAfter("120"); !ok || d != 120*time.Second {
		t.Errorf("ParseRetryAfter(120) = %s, %v", d, ok)
	}
	if d, ok := ParseRetryAfter("  30  "); !ok || d != 30*time.Second {
		t.Errorf("ParseRetryAfter with surrounding space = %s, %v", d, ok)
	}
	if _, ok := ParseRetryAfter(""); ok {
		t.Errorf("empty Retry-After should not parse")
	}
	if _, ok := ParseRetryAfter("-5"); ok {
		t.Errorf("negative seconds should not parse")
	}
	if _, ok := ParseRetryAfter("not-a-header"); ok {
		t.Errorf("garbage should not parse")
	}

	// HTTP-date in the future (RFC 7231 IMF-fixdate, the format servers send).
	future := time.Now().UTC().Add(2 * time.Minute).Format(http.TimeFormat)
	d, ok := ParseRetryAfter(future)
	if !ok {
		t.Fatalf("future HTTP-date did not parse: %q", future)
	}
	if d <= 0 || d > 2*time.Minute+time.Second {
		t.Errorf("future HTTP-date delay = %s, want ~2m", d)
	}

	// HTTP-date in the past -> the hint stands, delay is 0.
	past := time.Now().UTC().Add(-time.Hour).Format(http.TimeFormat)
	if d, ok := ParseRetryAfter(past); !ok || d != 0 {
		t.Errorf("past HTTP-date = %s, %v; want 0, true", d, ok)
	}
}
