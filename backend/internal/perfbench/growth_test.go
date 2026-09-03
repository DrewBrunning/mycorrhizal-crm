package perfbench

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyRatio(t *testing.T) {
	// data grew 3x
	const rr = 3.0
	assert.Equal(t, GrowthConstant, classifyRatio(1.0, rr))
	assert.Equal(t, GrowthConstant, classifyRatio(1.4, rr))
	assert.Equal(t, GrowthLinear, classifyRatio(3.0, rr))
	assert.Equal(t, GrowthLinear, classifyRatio(4.0, rr)) // within the 3 * 1.4 slack
	assert.Equal(t, GrowthSuperlinear, classifyRatio(4.3, rr))
	assert.Equal(t, GrowthSuperlinear, classifyRatio(9.0, rr))
	assert.Equal(t, GrowthSuperlinear, classifyRatio(38.0, rr))
}

func TestClassify_ConstantQueryCount(t *testing.T) {
	small := Result{Operation: "x", Profile: "smoke", Queries: 5, ResultSize: 10, RowScale: 300, MedianNanos: 1000, ExpectedGrowth: GrowthConstant}
	large := Result{Operation: "x", Profile: "typical", Queries: 5, ResultSize: 12, RowScale: 900, MedianNanos: 1500, ExpectedGrowth: GrowthConstant}

	f := Classify(small, large)
	assert.Equal(t, GrowthConstant, f.Class)
	assert.False(t, f.Regression)
	assert.False(t, f.DurationConcern)
	assert.InDelta(t, 3.0, f.RowScaleRatio, 0.01)
	assert.InDelta(t, 1.0, f.QueryRatio, 0.01)
}

func TestClassify_PairwiseResultBlowupIsSuperlinearOnlyWhenOptedIn(t *testing.T) {
	small := Result{Operation: "dup", Profile: "smoke", Queries: 5, ResultSize: 370, RowScale: 300, MedianNanos: 1000}
	large := Result{Operation: "dup", Profile: "typical", Queries: 5, ResultSize: 14220, RowScale: 900, MedianNanos: 16000, ExpectedGrowth: GrowthSuperlinear}

	// Without opt-in, the flat query count wins: classified constant.
	assert.Equal(t, GrowthConstant, Classify(small, large).Class)

	// With opt-in, the O(n^2) result growth is folded in.
	small.ClassifyResultGrowth, large.ClassifyResultGrowth = true, true
	f := Classify(small, large)
	assert.Equal(t, GrowthSuperlinear, f.Class)
	assert.False(t, f.Regression, "superlinear matches the expected class, so it is a recorded finding, not a regression")
}

func TestClassify_RegressionWhenClassExceedsExpected(t *testing.T) {
	small := Result{Operation: "list", Profile: "smoke", Queries: 1, RowScale: 300, MedianNanos: 1000, ExpectedGrowth: GrowthConstant}
	large := Result{Operation: "list", Profile: "typical", Queries: 60, RowScale: 900, MedianNanos: 1000, ExpectedGrowth: GrowthConstant}

	f := Classify(small, large)
	assert.Equal(t, GrowthSuperlinear, f.Class, "60x queries for 3x rows is an N+1")
	assert.True(t, f.Regression)
}

func TestClassify_DurationConcernIsAdvisoryOnly(t *testing.T) {
	small := Result{Operation: "trav", Profile: "smoke", Queries: 2, ResultSize: 49, RowScale: 300, MedianNanos: int64(30 * time.Millisecond), ExpectedGrowth: GrowthConstant}
	large := Result{Operation: "trav", Profile: "typical", Queries: 2, ResultSize: 299, RowScale: 900, MedianNanos: int64(2 * time.Second), ExpectedGrowth: GrowthConstant}

	f := Classify(small, large)
	assert.Equal(t, GrowthConstant, f.Class, "query count is the gate; it stayed flat")
	assert.False(t, f.Regression)
	assert.True(t, f.DurationConcern, "68x wall-clock for 3x rows is flagged, but does not fail")
}

func TestAnalyzeGrowth_SkipsSingleScaleOps(t *testing.T) {
	results := []Result{
		{Operation: "a", Profile: "smoke", Queries: 1, RowScale: 300, ExpectedGrowth: GrowthConstant},
		{Operation: "a", Profile: "typical", Queries: 1, RowScale: 900, ExpectedGrowth: GrowthConstant},
		{Operation: "b", Profile: "smoke", Queries: 1, RowScale: 300, ExpectedGrowth: GrowthConstant}, // only one scale
	}
	findings := AnalyzeGrowth(results)
	require.Len(t, findings, 1)
	assert.Equal(t, "a", findings[0].Operation)
}

func TestAnalyzeGrowth_SkipsWhenBothScalesEqual(t *testing.T) {
	results := []Result{
		{Operation: "a", Profile: "x", Queries: 1, RowScale: 300, ExpectedGrowth: GrowthConstant},
		{Operation: "a", Profile: "y", Queries: 1, RowScale: 300, ExpectedGrowth: GrowthConstant},
	}
	assert.Empty(t, AnalyzeGrowth(results), "no growth signal when the row scale did not change")
}

func TestMedianNanos(t *testing.T) {
	assert.Equal(t, int64(0), medianNanos(nil))
	assert.Equal(t, int64(3), medianNanos([]time.Duration{5, 1, 3}))
	assert.Equal(t, int64(3), medianNanos([]time.Duration{1, 5}))
}

func TestGrowthClassRank(t *testing.T) {
	assert.Less(t, GrowthConstant.rank(), GrowthLinear.rank())
	assert.Less(t, GrowthLinear.rank(), GrowthSuperlinear.rank())
	assert.Equal(t, -1, GrowthClass("bogus").rank())
}
