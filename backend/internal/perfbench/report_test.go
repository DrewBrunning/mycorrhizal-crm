package perfbench

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderReport_WithFindings(t *testing.T) {
	rbp := sampleResults()
	all := append(append([]Result(nil), rbp["smoke"]...), rbp["typical"]...)
	md := RenderReport(rbp, []string{"smoke", "typical"}, AnalyzeGrowth(all))

	assert.Contains(t, md, "# Core operation benchmarks (PERF-02)")
	assert.Contains(t, md, "## Measurements")
	assert.Contains(t, md, "## Cross-scale growth")
	assert.Contains(t, md, "`duplicates.find_pairs`")
	assert.Contains(t, md, "Super-linear operations (hard findings)")
}

func TestRenderReport_FlagsRegressionAndDurationConcern(t *testing.T) {
	rbp := sampleResults()
	findings := []GrowthFinding{
		{Operation: "contact_detail", SmallProfile: "smoke", LargeProfile: "typical",
			RowScaleRatio: 3, QueryRatio: 20, ResultRatio: 1, DurationRatio: 40,
			Class: GrowthSuperlinear, Expected: GrowthConstant, Regression: true, DurationConcern: true},
	}
	md := RenderReport(rbp, []string{"smoke", "typical"}, findings)
	assert.Contains(t, md, "⚠️ regression")
	assert.Contains(t, md, "⏱")
	assert.Contains(t, md, "Wall-clock grew super-linearly (advisory")
}

func TestRenderReport_NoFindings(t *testing.T) {
	rbp := map[string][]Result{"smoke": sampleResults()["smoke"]}
	md := RenderReport(rbp, []string{"smoke"}, nil)
	assert.Contains(t, md, "Not enough profiles measured")
}

func TestRenderReport_UnlistedProfileStillRendered(t *testing.T) {
	rbp := sampleResults()
	rbp["large"] = []Result{{Operation: "contact_list.plain", Profile: "large", Category: "read", Queries: 1, ResultSize: 50, RowScale: 45000, ExpectedGrowth: GrowthConstant}}
	md := RenderReport(rbp, []string{"smoke"}, nil) // only smoke listed; typical + large must still appear
	assert.Equal(t, 1, strings.Count(md, "| `contact_list.plain` | read | large |"))
	assert.Contains(t, md, "| `contact_list.plain` | read | typical |")
}
