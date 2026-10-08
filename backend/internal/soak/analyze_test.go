package soak

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// line builds n samples one second apart: value = base + slope*t.
func line(n int, base, slope float64) Series {
	s := make(Series, n)
	for i := range s {
		s[i] = Point{T: float64(i), V: base + slope*float64(i)}
	}
	return s
}

func TestLinearFit_RecoversSlopeAndIntercept(t *testing.T) {
	f := LinearFit(line(30, 5, 2))
	assert.InDelta(t, 2, f.Slope, 1e-9)
	assert.InDelta(t, 5, f.Intercept, 1e-9)
	assert.InDelta(t, 29, f.Span, 1e-9)
	assert.Equal(t, 30, f.N)
	assert.InDelta(t, 58, f.Growth(), 1e-6)
}

func TestLinearFit_DegenerateInputsAreFlat(t *testing.T) {
	assert.Zero(t, LinearFit(nil).Slope)
	assert.Equal(t, 1, LinearFit(Series{{T: 1, V: 9}}).N)
	// All samples at one instant: no defined slope, and no division by zero.
	f := LinearFit(Series{{T: 3, V: 1}, {T: 3, V: 100}})
	assert.Zero(t, f.Slope)
	assert.False(t, math.IsNaN(f.Intercept))
}

func TestTail_IsTheLastTwoThirdsOfTheTimeSpan(t *testing.T) {
	s := line(31, 0, 1) // t = 0..30
	tail := s.Tail()
	// cut = 30*(1/3) = 10: the first third (t<10) is warm-up and dropped.
	assert.InDelta(t, 10, tail[0].T, 1e-9)
	assert.InDelta(t, 30, tail[len(tail)-1].T, 1e-9)
	assert.Nil(t, Series(nil).Tail())
	assert.Len(t, Series{{T: 4, V: 1}}.Tail(), 1)
}

func TestSeries_MaxAndMedian(t *testing.T) {
	assert.Zero(t, Series(nil).Max())
	assert.Zero(t, Series(nil).Median())
	s := Series{{V: 3}, {V: -1}, {V: 9}, {V: 4}}
	assert.Equal(t, 9.0, s.Max())
	assert.Equal(t, 3.5, s.Median(), "even count averages the middle two")
	assert.Equal(t, 3.0, s[:3].Median(), "odd count takes the middle")
	assert.Equal(t, -2.0, Series{{V: -2}, {V: -5}}[:1].Max())
}

func growthBudget(limit float64) Budget {
	return Budget{Signal: "x", Kind: KindGrowth, Limit: limit, Unit: "u", Reason: "r"}
}

// The core claim of the harness: a flat series (with noise) passes, a series
// that keeps climbing fails — at a number well inside the committed budgets.
func TestEvaluate_Growth_FlatPassesLeakFails(t *testing.T) {
	flat := make(Series, 60)
	for i := range flat {
		flat[i] = Point{T: float64(i), V: 100 + float64(i%3)} // bounded jitter
	}
	v := Evaluate(growthBudget(5), flat)
	assert.False(t, v.Breached, v.String())
	assert.False(t, v.Failed())

	leak := line(60, 100, 1) // +1 per second
	v = Evaluate(growthBudget(5), leak)
	assert.True(t, v.Breached, v.String())
	assert.True(t, v.Failed())
	assert.InDelta(t, 39, v.Observed, 1, "growth over the 2/3 tail window = slope x span")
	assert.Contains(t, v.String(), "fitted growth")
}

func TestEvaluate_Growth_WarmUpIsIgnored(t *testing.T) {
	// Climbs steeply for the first third (warm-up), then is flat: must pass.
	s := make(Series, 61)
	for i := range s {
		v := 500.0
		if i < 20 {
			v = float64(i) * 25
		}
		s[i] = Point{T: float64(i), V: v}
	}
	v := Evaluate(growthBudget(5), s)
	assert.False(t, v.Breached, "warm-up growth must not count: %s", v.String())
}

func TestEvaluate_Ceiling(t *testing.T) {
	b := Budget{Signal: "wal", Kind: KindCeiling, Limit: 10, Unit: "MiB", Reason: "r"}
	assert.False(t, Evaluate(b, line(30, 4, 0.01)).Breached)
	v := Evaluate(b, line(30, 4, 1)) // reaches 33
	assert.True(t, v.Breached)
	assert.Contains(t, v.String(), "ceiling")
	// A spike in the warm-up third is not judged.
	s := line(30, 4, 0)
	s[1].V = 99
	assert.False(t, Evaluate(b, s).Breached)
}

func TestEvaluate_Degradation(t *testing.T) {
	b := Budget{Signal: "p95", Kind: KindDegradation, Limit: 0.25, Ratio: 4, Unit: "s", Reason: "r"}
	steady := line(30, 0.01, 0)
	assert.False(t, Evaluate(b, steady).Breached)

	// Head ~0.01s, tail ~2s: 2 > 0.01*4+0.25.
	deg := make(Series, 30)
	for i := range deg {
		v := 0.01
		if i > 15 {
			v = 2
		}
		deg[i] = Point{T: float64(i), V: v}
	}
	v := Evaluate(b, deg)
	assert.True(t, v.Breached)
	assert.InDelta(t, 0.01, v.Head, 1e-9)
	assert.Contains(t, v.String(), "first-third")

	// Too few head samples to know the baseline.
	short := Series{{T: 0, V: 1}, {T: 10, V: 1}, {T: 20, V: 1}, {T: 30, V: 1}, {T: 31, V: 1}, {T: 32, V: 1}}
	assert.True(t, Evaluate(b, short).Insufficient)
}

func TestEvaluate_InsufficientDataFailsUnlessOptionalOrAdvisory(t *testing.T) {
	b := growthBudget(5)
	v := Evaluate(b, line(3, 0, 0))
	assert.True(t, v.Insufficient)
	assert.True(t, v.Failed(), "a required signal with no data must fail, not read as no growth")
	assert.Contains(t, v.String(), "no usable samples")

	assert.True(t, Evaluate(b, nil).Failed(), "a signal that was never exported fails")

	b.Optional = true
	assert.False(t, Evaluate(b, nil).Failed())

	b = growthBudget(1)
	b.Advisory = true
	assert.False(t, Evaluate(b, line(30, 0, 5)).Failed(), "an advisory breach is reported, not failed")
	assert.True(t, Evaluate(b, line(30, 0, 5)).Breached)
}

func TestEvaluateAll_PreservesBudgetOrder(t *testing.T) {
	budgets := []Budget{
		{Signal: "a", Kind: KindGrowth, Limit: 1, Reason: "r"},
		{Signal: "b", Kind: KindCeiling, Limit: 1, Reason: "r"},
	}
	vs := EvaluateAll(budgets, map[string]Series{"a": line(30, 0, 0), "b": line(30, 9, 0)})
	require.Len(t, vs, 2)
	assert.Equal(t, "a", vs[0].Budget.Signal)
	assert.False(t, vs[0].Breached)
	assert.True(t, vs[1].Breached)
}
