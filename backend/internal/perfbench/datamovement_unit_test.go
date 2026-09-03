package perfbench

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dmResult is a terse DataMovementResult builder for the growth/baseline/report
// unit tests.
func dmResult(op, profile string, rowScale, rowsTouched int, peakHeap int64, exp GrowthClass, fold bool) DataMovementResult {
	return DataMovementResult{
		Operation: op, Profile: profile, Category: "maintenance",
		RowScale: rowScale, ExpectedMemoryGrowth: exp, FoldResultGrowth: fold,
		Sample: ResourceSample{RowsTouched: rowsTouched, PeakHeapBytes: peakHeap, DurationNanos: 1},
	}
}

func TestAnalyzeMemoryGrowth_SkipsThinAndFlatScales(t *testing.T) {
	t.Run("fewer than two measurements", func(t *testing.T) {
		assert.Empty(t, AnalyzeMemoryGrowth([]DataMovementResult{
			dmResult("solo", "smoke", 300, 10, 1<<20, GrowthConstant, false),
		}))
	})
	t.Run("two measurements at the same row scale", func(t *testing.T) {
		assert.Empty(t, AnalyzeMemoryGrowth([]DataMovementResult{
			dmResult("x", "a", 300, 10, 1<<20, GrowthConstant, false),
			dmResult("x", "b", 300, 10, 9<<20, GrowthConstant, false),
		}))
	})
}

func TestClassifyMemoryGrowth_HeapVsWork(t *testing.T) {
	// Heap grew 12x while the op's own work grew 3x -> superlinear, and since
	// the op declared "constant" that is a regression.
	f := classifyMemoryGrowth(
		dmResult("acc", "smoke", 300, 300, 1<<20, GrowthConstant, false),
		dmResult("acc", "typical", 900, 900, 12<<20, GrowthConstant, false),
	)
	assert.Equal(t, GrowthSuperlinear, f.Class)
	assert.True(t, f.Regression)
	assert.InDelta(t, 3.0, f.RowsTouchedRatio, 0.01)
}

func TestClassifyMemoryGrowth_BelowSignalFloorIsConstant(t *testing.T) {
	// Both peaks are under minHeapSignalBytes: noise, reported constant even
	// though the raw ratio is large.
	f := classifyMemoryGrowth(
		dmResult("tiny", "smoke", 300, 300, 1<<10, GrowthConstant, false),
		dmResult("tiny", "typical", 900, 900, 400<<10, GrowthConstant, false),
	)
	assert.Equal(t, GrowthConstant, f.Class)
	assert.False(t, f.Regression)
}

func TestClassifyMemoryGrowth_FoldJudgesAgainstDataset(t *testing.T) {
	// Pair-style op: its work (pairs) exploded 40x for a 3x dataset, and heap
	// tracked the pairs. With the fold on, heap is judged against the DATASET
	// (3x) -> superlinear; that matches its "superlinear" declaration, so no
	// regression.
	small := dmResult("dup", "smoke", 300, 370, 2<<20, GrowthSuperlinear, true)
	large := dmResult("dup", "typical", 900, 14220, 80<<20, GrowthSuperlinear, true)
	f := classifyMemoryGrowth(small, large)
	assert.Equal(t, GrowthSuperlinear, f.Class)
	assert.False(t, f.Regression)
}

func TestClassifyMemoryRatio(t *testing.T) {
	// flat heap while work grew several-fold -> constant
	assert.Equal(t, GrowthConstant, classifyMemoryRatio(1.2, 10))
	// heap grew with the work, inside the buffer-realloc slack -> linear
	assert.Equal(t, GrowthLinear, classifyMemoryRatio(6, 3))    // 6 <= 3*2.5
	assert.Equal(t, GrowthLinear, classifyMemoryRatio(20, 100)) // way under the data growth
	// heap outgrew the work even allowing the slack -> superlinear
	assert.Equal(t, GrowthSuperlinear, classifyMemoryRatio(12, 3)) // 12 > 3*2.5
	// no work growth to compare against -> anything above flat is superlinear
	assert.Equal(t, GrowthSuperlinear, classifyMemoryRatio(4, 1))
}

func TestBuildDataMovementBaseline_ShapeOrderAndAppend(t *testing.T) {
	byProfile := map[string][]DataMovementResult{
		"smoke":   {dmResult("import.vcf", "smoke", 300, 300, 5<<20, GrowthLinear, false)},
		"typical": {dmResult("import.vcf", "typical", 900, 900, 15<<20, GrowthLinear, false)},
		"large":   {dmResult("import.vcf", "large", 45000, 19000, 300<<20, GrowthLinear, false)},
	}
	b := BuildDataMovementBaseline(byProfile, []string{"typical", "smoke"})
	assert.Equal(t, []string{"typical", "smoke", "large"}, b.Profiles, "unlisted profile appended in name order")

	op := b.Operations["import.vcf"]
	assert.Equal(t, "maintenance", op.Category)
	assert.Equal(t, GrowthLinear, op.ExpectedMemoryGrowth)
	assert.Equal(t, 300, op.Profiles["smoke"].RowsTouched)
	assert.Equal(t, 45000, op.Profiles["large"].RowScale)
}

func TestDataMovementBaseline_MarshalRoundTripsAndIsStable(t *testing.T) {
	byProfile := map[string][]DataMovementResult{
		"smoke": {
			dmResult("import.vcf", "smoke", 300, 300, 5<<20, GrowthLinear, false),
			func() DataMovementResult {
				r := dmResult("backup.vacuum_into", "smoke", 300, 140, 1<<20, GrowthConstant, false)
				r.Sample.OutputBytes = 4096
				return r
			}(),
		},
	}
	a, err := BuildDataMovementBaseline(byProfile, []string{"smoke"}).Marshal()
	require.NoError(t, err)
	c, err := BuildDataMovementBaseline(byProfile, []string{"smoke"}).Marshal()
	require.NoError(t, err)
	assert.Equal(t, string(a), string(c))
	assert.Equal(t, byte('\n'), a[len(a)-1])

	var back DataMovementBaseline
	require.NoError(t, json.Unmarshal(a, &back))
	assert.True(t, back.Operations["backup.vacuum_into"].Profiles["smoke"].OutputPresent)
	assert.False(t, back.Operations["import.vcf"].Profiles["smoke"].OutputPresent)
}

func TestLoadAndCheckDataMovementBaseline(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "backend", "internal", "perfbench", "testdata")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "development"), 0o750))

	suite := DataMovementSuite{
		ResultsByProfile: map[string][]DataMovementResult{
			"smoke": {dmResult("import.vcf", "smoke", 300, 300, 5<<20, GrowthLinear, false)},
		},
		ProfileOrder: []string{"smoke"},
	}
	baselineJSON, reportMD, err := suite.Artifacts()
	require.NoError(t, err)
	require.NoError(t, WriteDataMovementArtifacts(root, baselineJSON, reportMD))

	bPath, rPath := DataMovementArtifactPaths(root)
	assert.FileExists(t, bPath)
	assert.FileExists(t, rPath)

	loaded, err := LoadDataMovementBaseline(bPath)
	require.NoError(t, err)
	assert.Contains(t, loaded.Operations, "import.vcf")

	_, stale := CheckDataMovementBaseline(root, baselineJSON)
	assert.False(t, stale, "just-written baseline is current")
	_, stale = CheckDataMovementBaseline(root, []byte("different"))
	assert.True(t, stale)
	_, stale = CheckDataMovementBaseline(t.TempDir(), baselineJSON)
	assert.True(t, stale, "a missing file is stale")
}

func TestParseDataMovementBaseline_RejectsGarbage(t *testing.T) {
	_, err := parseDataMovementBaseline([]byte("{not json"))
	assert.ErrorContains(t, err, "parsing data-movement baseline")
}

func TestEmbeddedDataMovementBaseline_IsPopulated(t *testing.T) {
	b, err := EmbeddedDataMovementBaseline()
	require.NoError(t, err)
	for _, op := range DataMovementRegistry() {
		_, ok := b.Operations[op.Name]
		assert.Truef(t, ok, "committed datamovement-baseline.json missing %q", op.Name)
	}
}

func TestRenderDataMovementReport_Sections(t *testing.T) {
	byProfile := map[string][]DataMovementResult{
		"smoke": {
			dmResult("import.vcf", "smoke", 300, 300, 5<<20, GrowthLinear, false),
			withProbe(dmResult("fts.rebuild", "smoke", 300, 420, 1<<20, GrowthConstant, false), 10, 0, false),
		},
		"typical": {
			dmResult("import.vcf", "typical", 900, 900, 60<<20, GrowthLinear, false),
			withProbe(dmResult("fts.rebuild", "typical", 900, 1260, 1<<20, GrowthConstant, false), 2, 5_000_000_000, true),
		},
	}
	findings := AnalyzeMemoryGrowth(flatten(byProfile))
	md := RenderDataMovementReport(byProfile, []string{"smoke", "typical"}, findings)

	for _, want := range []string{
		"# Data-movement benchmarks (PERF-03)",
		"## Measurements",
		"## Peak-memory growth (the gate)",
		"## Free-space requirements (per operation)",
		"## Write-lock hold (outage vs background task)",
		"## Migration",
		"## Method",
		"`import.vcf`",
		"`fts.rebuild`",
	} {
		assert.Containsf(t, md, want, "report missing %q", want)
	}
	assert.Contains(t, md, "**yes**", "the stalled-out FTS rebuild row is flagged")
}

func TestRenderDataMovementReport_ListsDeclaredSuperlinearAndRegressions(t *testing.T) {
	byProfile := map[string][]DataMovementResult{
		// dup: declared + measured superlinear (a recorded finding, no regression)
		"smoke": {
			dmResult("duplicates.find_pairs", "smoke", 300, 370, 2<<20, GrowthSuperlinear, true),
			dmResult("fts.rebuild", "smoke", 300, 420, 1<<20, GrowthConstant, false),
		},
		// fts: declared constant but heap exploded -> regression row
		"large": {
			dmResult("duplicates.find_pairs", "large", 45000, 999999, 900<<20, GrowthSuperlinear, true),
			dmResult("fts.rebuild", "large", 45000, 20000, 200<<20, GrowthConstant, false),
		},
	}
	findings := AnalyzeMemoryGrowth(flatten(byProfile))
	md := RenderDataMovementReport(byProfile, []string{"smoke", "large"}, findings)
	assert.Contains(t, md, "**Super-linear peak memory (recorded findings):** `duplicates.find_pairs`")
	assert.Contains(t, md, "**Regressions (hard failures):** `fts.rebuild`")
}

func TestRenderDataMovementReport_NoFindingsWhenSingleProfile(t *testing.T) {
	byProfile := map[string][]DataMovementResult{
		"smoke": {dmResult("import.vcf", "smoke", 300, 300, 5<<20, GrowthLinear, false)},
	}
	md := RenderDataMovementReport(byProfile, []string{"smoke"}, nil)
	assert.Contains(t, md, "Not enough profiles measured")
}

func TestRenderDataMovementReport_OrdersUnlistedProfilesByName(t *testing.T) {
	byProfile := map[string][]DataMovementResult{
		"typical": {dmResult("import.vcf", "typical", 900, 900, 9<<20, GrowthLinear, false)},
		"smoke":   {dmResult("import.vcf", "smoke", 300, 300, 5<<20, GrowthLinear, false)},
		"large":   {dmResult("import.vcf", "large", 45000, 2500, 25<<20, GrowthLinear, false)},
	}
	// order slice names only one profile; the other two are appended in name order.
	md := RenderDataMovementReport(byProfile, []string{"typical"}, nil)
	iTyp := indexOf(md, "| `import.vcf` | import | typical |")
	iLarge := indexOf(md, "| `import.vcf` | import | large |")
	iSmoke := indexOf(md, "| `import.vcf` | import | smoke |")
	assert.Positive(t, iTyp)
	assert.Less(t, iTyp, iLarge, "listed profile comes first")
	assert.Less(t, iLarge, iSmoke, "then the rest in name order: large before smoke")
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestBuildImportVCF_ParsesBackThroughTheImporter(t *testing.T) {
	vcf := buildImportVCF(5)
	assert.Equal(t, 5, bytes.Count(vcf, []byte("BEGIN:VCARD")))

	db := dbtest.New(t)
	parsed, _, _, err := services.ParseVCF(bytes.NewReader(vcf), db, 1)
	require.NoError(t, err)
	assert.Len(t, parsed, 5)
	assert.Equal(t, "Perf000000", parsed[0].Contact.Firstname)
}

func TestCapImportSize_CapsAtServiceCeiling(t *testing.T) {
	assert.Equal(t, 0, capImportSize(0))
	assert.Equal(t, 300, capImportSize(300))
	assert.Equal(t, maxImportContacts, capImportSize(maxImportContacts))
	assert.Equal(t, maxImportContacts, capImportSize(50000))
}

func TestDiscardResponseWriter_CountsAndDiscards(t *testing.T) {
	w := &discardResponseWriter{}
	w.WriteHeader(http.StatusOK)
	n, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	m, err := w.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, m)
	assert.Equal(t, int64(11), w.n)
	assert.NotNil(t, w.Header())
	assert.Equal(t, http.StatusOK, w.status)
}

func TestFileSize(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(p, make([]byte, 321), 0o600))
	assert.Equal(t, int64(321), fileSize(p))
	assert.Equal(t, int64(0), fileSize(filepath.Join(t.TempDir(), "missing")))
}

// --- tiny local helpers ------------------------------------------------

func withProbe(r DataMovementResult, writes int, maxStallNanos int64, stalled bool) DataMovementResult {
	r.Sample.WriteProbed = true
	r.Sample.ProbeWrites = writes
	r.Sample.ProbeMaxStallNanos = maxStallNanos
	r.Sample.ProbeStalledOut = stalled
	return r
}

func flatten(byProfile map[string][]DataMovementResult) []DataMovementResult {
	var out []DataMovementResult
	for _, rs := range byProfile {
		out = append(out, rs...)
	}
	return out
}
