// Package soak is the long-run (soak) test harness (issue #1496): it drives a
// sustained mixed workload against a real server, samples process-lifetime
// signals from /metrics (RSS, heap, goroutines, open fds, WAL and database
// size, rate-limiter entries, per-route-class p95 latency) and fails on
// *growth*, not on wall-clock or absolute speed.
//
// Every other test and benchmark in the suite is short-lived; this is the one
// that notices a leak that only shows with process lifetime. The assertion
// vocabulary lives here (analyze.go + budgets.go) and is pure and
// deterministic over a recorded series, so it is unit-tested with synthetic
// data — including a leak series that MUST fail it — independent of any
// running server.
package soak

import (
	"fmt"
	"math"
	"sort"
)

// Point is one sample of one signal: seconds since the run started, and the
// value read at that instant.
type Point struct {
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// Series is the ordered samples of one signal.
type Series []Point

// TailFraction is the share of the run the slope is fitted over — the last
// 2/3, per the issue: the first third is warm-up (heap/pool/page-cache fill,
// the boot-time catch-up burst), and a leak is what is still climbing after
// it.
const TailFraction = 2.0 / 3.0

// MinTailPoints is the fewest samples a tail may hold before the slope is
// considered meaningful. Below it, Evaluate reports the series as
// insufficient rather than guessing from two points.
const MinTailPoints = 4

// Tail returns the samples of s in the last TailFraction of its time span
// (measured from the first sample to the last, so a run whose first sample
// lands late is not penalised).
func (s Series) Tail() Series {
	if len(s) == 0 {
		return nil
	}
	start := s[0].T
	end := s[len(s)-1].T
	cut := start + (end-start)*(1-TailFraction)
	for i, p := range s {
		if p.T >= cut {
			return s[i:]
		}
	}
	return s[len(s)-1:] // # pragma: no cover — the last point always satisfies T >= cut
}

// Fit is a least-squares line through a series.
type Fit struct {
	// Slope is value units per second.
	Slope float64 `json:"slope_per_s"`
	// Intercept is the fitted value at T=0.
	Intercept float64 `json:"intercept"`
	// Span is the time covered by the fitted points, in seconds.
	Span float64 `json:"span_s"`
	// N is the number of points fitted.
	N int `json:"n"`
}

// Growth is the fitted increase over the fitted span (slope x span), the
// quantity the budgets bound. A flat or falling series yields <= 0.
func (f Fit) Growth() float64 { return f.Slope * f.Span }

// LinearFit is ordinary least squares over s. Fewer than two points, or all
// points at one instant, yields a zero fit (slope 0) with N set, so a caller
// checks N, not the slope, to know whether there was enough data.
func LinearFit(s Series) Fit {
	n := len(s)
	f := Fit{N: n}
	if n < 2 {
		return f
	}
	var sumT, sumV float64
	for _, p := range s {
		sumT += p.T
		sumV += p.V
	}
	meanT := sumT / float64(n)
	meanV := sumV / float64(n)
	var num, den float64
	for _, p := range s {
		num += (p.T - meanT) * (p.V - meanV)
		den += (p.T - meanT) * (p.T - meanT)
	}
	f.Span = s[n-1].T - s[0].T
	if den == 0 {
		return f
	}
	f.Slope = num / den
	f.Intercept = meanV - f.Slope*meanT
	return f
}

// Max returns the largest value in s (0 for an empty series).
func (s Series) Max() float64 {
	var m float64
	for i, p := range s {
		if i == 0 || p.V > m {
			m = p.V
		}
	}
	return m
}

// Median returns the middle value of s (0 for an empty series).
func (s Series) Median() float64 {
	if len(s) == 0 {
		return 0
	}
	vs := make([]float64, len(s))
	for i, p := range s {
		vs[i] = p.V
	}
	sort.Float64s(vs)
	mid := len(vs) / 2
	if len(vs)%2 == 1 {
		return vs[mid]
	}
	return (vs[mid-1] + vs[mid]) / 2
}

// Kind selects how a Budget judges a series.
type Kind string

const (
	// KindGrowth bounds the fitted growth over the tail window: the slope of
	// the last 2/3 of the run times that window's length, in the signal's
	// unit. A bounded steady state fits ~0; a per-request leak fits large.
	KindGrowth Kind = "growth"
	// KindCeiling bounds the largest value seen in the tail window — for a
	// signal that should stay under a fixed level however long the run (the
	// WAL: SQLite checkpoints it back to a few MiB; one that climbs is not
	// being checkpointed).
	KindCeiling Kind = "ceiling"
	// KindDegradation bounds how much worse the tail's median is than the
	// first third's median (latency): tail <= head*Ratio + Floor.
	KindDegradation Kind = "degradation"
)

// Budget is one committed threshold. Budgets live in budgets.go, each with a
// non-empty Reason (a test enforces it), in the style of
// docs/development/perf-budgets.md: changing a number is a deliberate,
// reviewed edit that must rewrite the paired reason.
type Budget struct {
	// Signal is the sampled series this judges (see signals.go).
	Signal string `json:"signal"`
	Kind   Kind   `json:"kind"`
	// Limit is the maximum growth (KindGrowth), the maximum level
	// (KindCeiling), or the additive floor of the allowance (KindDegradation),
	// in the signal's unit.
	Limit float64 `json:"limit"`
	// Ratio is the multiplicative allowance for KindDegradation only.
	Ratio float64 `json:"ratio,omitempty"`
	// Unit labels Limit in reports.
	Unit string `json:"unit"`
	// Optional marks a signal that may legitimately have too few samples to
	// judge (a latency class that saw little traffic in a short run). Missing
	// data then neither fails nor passes the run silently: the verdict is
	// reported as "no data". Every other budget treats missing data as a
	// failure.
	Optional bool `json:"optional,omitempty"`
	// Advisory marks a budget whose breach is reported but does not fail the
	// run — for a signal whose steady state is not yet characterised (a
	// platform-dependent one). It is never the default.
	Advisory bool `json:"advisory,omitempty"`
	// Reason records why the number is what it is.
	Reason string `json:"reason"`
}

// Verdict is the outcome of judging one budget against a run.
type Verdict struct {
	Budget Budget `json:"budget"`
	// Observed is the measured quantity the budget bounds (growth, ceiling
	// level, or tail median), in Budget.Unit.
	Observed float64 `json:"observed"`
	// Head is, for KindDegradation, the first third's median.
	Head float64 `json:"head,omitempty"`
	// Fit is the tail fit, populated for every kind (informational outside
	// KindGrowth).
	Fit Fit `json:"fit"`
	// Samples is the number of tail samples judged.
	Samples int `json:"samples"`
	// Insufficient is true when the series had too few samples (or no data at
	// all) to judge. A required budget with no data fails — a signal that
	// silently stopped being exported must not read as "no growth".
	Insufficient bool `json:"insufficient,omitempty"`
	// Breached is true when the observed quantity exceeded the budget.
	Breached bool `json:"breached"`
}

// Failed reports whether this verdict fails the run.
func (v Verdict) Failed() bool {
	if v.Budget.Advisory {
		return false
	}
	return v.Breached || (v.Insufficient && !v.Budget.Optional)
}

// String is the one-line human form used in the failure message.
func (v Verdict) String() string {
	if v.Insufficient {
		return fmt.Sprintf("%s: no usable samples (%d in tail) — the signal is missing or the run was too short", v.Budget.Signal, v.Samples)
	}
	switch v.Budget.Kind {
	case KindCeiling:
		return fmt.Sprintf("%s: tail max %.4g %s vs ceiling %.4g", v.Budget.Signal, v.Observed, v.Budget.Unit, v.Budget.Limit)
	case KindDegradation:
		return fmt.Sprintf("%s: tail median %.4g %s vs first-third median %.4g (allowed %.4g x + %.4g)",
			v.Budget.Signal, v.Observed, v.Budget.Unit, v.Head, v.Budget.Ratio, v.Budget.Limit)
	default:
		return fmt.Sprintf("%s: fitted growth %.4g %s over the %.0fs tail window (slope %.4g/s) vs budget %.4g",
			v.Budget.Signal, v.Observed, v.Budget.Unit, v.Fit.Span, v.Fit.Slope, v.Budget.Limit)
	}
}

// Evaluate judges one budget against the series recorded for its signal.
func Evaluate(b Budget, s Series) Verdict {
	v := Verdict{Budget: b}
	tail := s.Tail()
	v.Samples = len(tail)
	v.Fit = LinearFit(tail)
	if len(tail) < MinTailPoints {
		v.Insufficient = true
		return v
	}
	switch b.Kind {
	case KindCeiling:
		v.Observed = tail.Max()
		v.Breached = v.Observed > b.Limit
	case KindDegradation:
		headCut := s[0].T + (s[len(s)-1].T-s[0].T)/3
		var head Series
		for _, p := range s {
			if p.T > headCut {
				break
			}
			head = append(head, p)
		}
		if len(head) < MinTailPoints {
			v.Insufficient = true
			return v
		}
		v.Head = head.Median()
		v.Observed = tail.Median()
		v.Breached = v.Observed > v.Head*b.Ratio+b.Limit
	default:
		v.Observed = v.Fit.Growth()
		v.Breached = v.Observed > b.Limit
	}
	return v
}

// EvaluateAll judges every budget against the series map (keyed by signal
// name) and returns the verdicts in budget order.
func EvaluateAll(budgets []Budget, series map[string]Series) []Verdict {
	out := make([]Verdict, 0, len(budgets))
	for _, b := range budgets {
		out = append(out, Evaluate(b, series[b.Signal]))
	}
	return out
}

// round3 rounds to three decimals for stable report output.
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
