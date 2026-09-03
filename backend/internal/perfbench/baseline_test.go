package perfbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleResults() map[string][]Result {
	return map[string][]Result{
		"smoke": {
			{Operation: "contact_list.plain", Profile: "smoke", Category: "read", Queries: 1, ResultSize: 50, RowScale: 300, ExpectedGrowth: GrowthConstant},
			{Operation: "duplicates.find_pairs", Profile: "smoke", Category: "read", Queries: 5, ResultSize: 370, RowScale: 300, ExpectedGrowth: GrowthSuperlinear, ClassifyResultGrowth: true},
			{Operation: "contact_merge", Profile: "smoke", Category: "write", Queries: 100, ResultSize: 0, RowScale: 300, Iterations: 1, ExpectedGrowth: GrowthConstant},
		},
		"typical": {
			{Operation: "contact_list.plain", Profile: "typical", Category: "read", Queries: 1, ResultSize: 50, RowScale: 900, ExpectedGrowth: GrowthConstant},
			{Operation: "duplicates.find_pairs", Profile: "typical", Category: "read", Queries: 5, ResultSize: 14220, RowScale: 900, ExpectedGrowth: GrowthSuperlinear, ClassifyResultGrowth: true},
			{Operation: "contact_merge", Profile: "typical", Category: "write", Queries: 100, ResultSize: 0, RowScale: 900, Iterations: 1, ExpectedGrowth: GrowthConstant},
		},
	}
}

func TestBuildBaseline_ShapeAndOrder(t *testing.T) {
	b := BuildBaseline(sampleResults(), []string{"smoke", "typical"})

	assert.Equal(t, []string{"smoke", "typical"}, b.Profiles)
	require.Contains(t, b.Operations, "contact_list.plain")
	op := b.Operations["contact_list.plain"]
	assert.Equal(t, "read", op.Category)
	assert.Equal(t, GrowthConstant, op.ExpectedGrowth)
	assert.Equal(t, int64(1), op.Profiles["smoke"].Queries)
	assert.Equal(t, 900, op.Profiles["typical"].RowScale)

	assert.Equal(t, 14220, b.Operations["duplicates.find_pairs"].Profiles["typical"].ResultSize)
}

func TestBuildBaseline_UnlistedProfileAppendedInNameOrder(t *testing.T) {
	rs := sampleResults()
	rs["large"] = []Result{{Operation: "contact_list.plain", Profile: "large", Category: "read", Queries: 1, ResultSize: 50, RowScale: 45000, ExpectedGrowth: GrowthConstant}}
	b := BuildBaseline(rs, []string{"typical", "smoke"})
	assert.Equal(t, []string{"typical", "smoke", "large"}, b.Profiles)
}

func TestBaseline_MarshalRoundTrips(t *testing.T) {
	b := BuildBaseline(sampleResults(), []string{"smoke", "typical"})
	data, err := b.Marshal()
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), data[len(data)-1])

	var back Baseline
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, b, back)
}

func TestBaseline_MarshalIsStableAcrossRuns(t *testing.T) {
	a, err := BuildBaseline(sampleResults(), []string{"smoke", "typical"}).Marshal()
	require.NoError(t, err)
	c, err := BuildBaseline(sampleResults(), []string{"smoke", "typical"}).Marshal()
	require.NoError(t, err)
	assert.Equal(t, string(a), string(c))
}

func TestLoadBaseline_FromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	data, err := BuildBaseline(sampleResults(), []string{"smoke", "typical"}).Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	got, err := LoadBaseline(path)
	require.NoError(t, err)
	assert.Equal(t, int64(5), got.Operations["duplicates.find_pairs"].Profiles["smoke"].Queries)

	_, err = LoadBaseline(filepath.Join(dir, "missing.json"))
	assert.Error(t, err)

	garbage := filepath.Join(dir, "garbage.json")
	require.NoError(t, os.WriteFile(garbage, []byte("{not json"), 0o600))
	_, err = LoadBaseline(garbage)
	assert.ErrorContains(t, err, "parsing baseline")
}

// TestEmbeddedBaseline_ParsesAndIsPopulated guards the committed file: it must
// be valid JSON and carry the full registry (a placeholder or a truncated
// regeneration fails here).
func TestEmbeddedBaseline_ParsesAndIsPopulated(t *testing.T) {
	b, err := EmbeddedBaseline()
	require.NoError(t, err)
	assert.Contains(t, b.Profiles, "smoke")
	assert.Contains(t, b.Profiles, "typical")
	for _, op := range Registry() {
		ob, ok := b.Operations[op.Name]
		require.Truef(t, ok, "committed baseline is missing operation %q — regenerate with `make gen-perf-baseline`", op.Name)
		assert.NotEmptyf(t, ob.Profiles, "operation %q has no per-profile numbers", op.Name)
	}
}
