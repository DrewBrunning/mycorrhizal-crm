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

func TestAnalyzeMemoryGrowth_BaselineStepsUpFromANoiseAmplifiedSmall(t *testing.T) {
	// A buffered exporter: linear, but `smoke` peak (~4.5 MiB) is >64x smaller
	// than `large` (~950 MiB), so smoke/large is a noise-amplified ~210x that
	// straddles the linear/super-linear line. The baseline must step up to
	// `typical` (~27 MiB, within 64x of large) where the ratio is a stable
	// trend.
	t.Run("exporter steps up to typical", func(t *testing.T) {
		f := AnalyzeMemoryGrowth([]DataMovementResult{
			dmResult("export.bundle", "smoke", 300, 140, 4_500_000, GrowthLinear, false),
			dmResult("export.bundle", "typical", 900, 840, 27_700_000, GrowthLinear, false),
			dmResult("export.bundle", "large", 15000, 14000, 944_000_000, GrowthLinear, false),
		})
		require.Len(t, f, 1)
		assert.Equal(t, "typical", f[0].SmallProfile, "baseline must skip the noise-amplified smoke peak")
		assert.Equal(t, GrowthLinear, f[0].Class)
		assert.False(t, f[0].Regression)
	})

	// Genuine O(n^2): <1 MiB at smoke -> 3+ GiB at large. `smoke` (920 KiB) is
	// itself below minHeapSignalBytes now (4 MiB — see that const's doc), so
	// the baseline steps up to `typical`; nothing clears the 64x
	// stability-ratio bar either, so `typical` is used via the noise-floor
	// fallback, not the stable-ratio path. Either way the full O(n^2) growth
	// still reads super-linear — that invariant is what this case pins, not
	// which specific profile ends up as the baseline.
	t.Run("pairwise op stays super-linear however its baseline lands", func(t *testing.T) {
		f := AnalyzeMemoryGrowth([]DataMovementResult{
			dmResult("duplicates.find_pairs", "smoke", 300, 370, 920_000, GrowthSuperlinear, true),
			dmResult("duplicates.find_pairs", "typical", 900, 14220, 12_700_000, GrowthSuperlinear, true),
			dmResult("duplicates.find_pairs", "large", 15000, 3_997_000, 3_295_000_000, GrowthSuperlinear, true),
		})
		require.Len(t, f, 1)
		assert.Equal(t, "typical", f[0].SmallProfile, "920 KiB at smoke no longer clears minHeapSignalBytes")
		assert.Equal(t, GrowthSuperlinear, f[0].Class)
		assert.False(t, f[0].Regression)
	})

	// When every sub-large peak is noise, the baseline falls all the way back
	// to sorted[0] and classifyMemoryGrowth's both-below-floor guard reports
	// "constant" — no meaningless ratio leaks into the class.
	t.Run("all-noise sub-large peaks classify constant", func(t *testing.T) {
		f := AnalyzeMemoryGrowth([]DataMovementResult{
			dmResult("backup.vacuum_into", "smoke", 300, 140, 50_000, GrowthConstant, false),
			dmResult("backup.vacuum_into", "typical", 900, 840, 54_000, GrowthConstant, false),
			dmResult("backup.vacuum_into", "large", 15000, 14000, 60_000, GrowthConstant, false),
		})
		require.Len(t, f, 1)
		assert.Equal(t, "smoke", f[0].SmallProfile)
		assert.Equal(t, GrowthConstant, f[0].Class)
		assert.False(t, f[0].Regression)
	})

	// The real incident: a "Go holds nothing" op (VACUUM INTO happens entirely
	// inside SQLite) measured ~69 KiB at smoke and ~607 KiB at large on a real
	// CI run. 607 KiB clears the OLD 512 KiB floor while smoke doesn't, so the
	// both-below-floor guard no longer fires and the 9x ratio reads "linear"
	// against a "constant" declaration — a false regression on an absolute
	// 607 KiB, negligible next to this op's own 32 MiB OOM-boundary budget.
	// 4 MiB keeps both ends below the floor.
	t.Run("a small absolute peak that crept past the old floor stays constant", func(t *testing.T) {
		f := AnalyzeMemoryGrowth([]DataMovementResult{
			dmResult("backup.vacuum_into", "smoke", 300, 140, 68_744, GrowthConstant, false),
			dmResult("backup.vacuum_into", "typical", 900, 840, 71_640, GrowthConstant, false),
			dmResult("backup.vacuum_into", "large", 15000, 14000, 621_136, GrowthConstant, false),
		})
		require.Len(t, f, 1)
		assert.Equal(t, GrowthConstant, f[0].Class)
		assert.False(t, f[0].Regression)
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
	assert.Equal(t, GrowthLinear, classifyMemoryRatio(6, 3))    // 6 <= 3*3.5
	assert.Equal(t, GrowthLinear, classifyMemoryRatio(20, 100)) // way under the data growth
	// the memLinearSlack boundary is workRatio * 3.5
	assert.Equal(t, GrowthLinear, classifyMemoryRatio(10.5, 3))      // exactly on the line
	assert.Equal(t, GrowthSuperlinear, classifyMemoryRatio(10.6, 3)) // just over
	// heap outgrew the work by well over the slack -> superlinear
	assert.Equal(t, GrowthSuperlinear, classifyMemoryRatio(12, 3))
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
