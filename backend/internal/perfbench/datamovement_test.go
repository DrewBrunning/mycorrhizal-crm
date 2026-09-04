package perfbench

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/largedata"
	"mycorrhizal/internal/schemafixture"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smokePeakHeapCeiling is a deliberately loose absolute cap for the per-PR
// smoke run: no data-movement operation on a ~300-contact dataset has any
// business holding a quarter-gigabyte. It is the coarse per-PR tripwire for an
// accumulation bug so bad it shows up even at smoke; the per-operation
// peak-heap budgets (PERF-04, budgets.json) are the precise ceilings, enforced
// at the anchor profile in release validation.
const smokePeakHeapCeiling = 256 << 20

// TestDataMovementBenchmarks is the per-PR PERF-03 gate (issue #470): every
// bulk operation runs once against the `smoke` profile on the REAL migrated
// schema, and its DETERMINISTIC signals — rows touched, output presence, and
// "did it stall concurrent writes past the busy_timeout" — must match the
// committed baseline. The cross-scale peak-memory-growth analysis needs the
// `typical` and `large` populates and lives in TestDataMovementAtScale.
func TestDataMovementBenchmarks(t *testing.T) {
	results, err := RunDataMovementProfile(EnvOptions{
		Profile: largedata.Smoke, WorkDir: t.TempDir(), OpenMigrated: migratedOpener(t),
	})
	require.NoError(t, err)

	baseline, err := EmbeddedDataMovementBaseline()
	require.NoError(t, err)

	t.Run("EveryOperationMeasured", func(t *testing.T) {
		for _, op := range DataMovementRegistry() {
			r, ok := findDMResult(results, op.Name)
			require.Truef(t, ok, "operation %q was not measured", op.Name)
			assert.Positivef(t, r.Sample.DurationNanos, "operation %q recorded no duration — did it run?", op.Name)
			assert.GreaterOrEqualf(t, r.Sample.HeapSamples+r.Sample.DiskSamples, 1,
				"operation %q was never sampled", op.Name)
		}
	})

	t.Run("MatchesCommittedSmokeBaseline", func(t *testing.T) {
		for _, r := range results {
			ob, ok := baseline.Operations[r.Operation]
			require.Truef(t, ok, "no baseline for %q — run `make gen-perf-baseline`", r.Operation)
			pm, ok := ob.Profiles["smoke"]
			require.Truef(t, ok, "no smoke baseline for %q", r.Operation)
			assert.Equalf(t, pm.RowsTouched, r.Sample.RowsTouched,
				"%s: rows touched %d != committed smoke baseline %d — run `make gen-perf-baseline`",
				r.Operation, r.Sample.RowsTouched, pm.RowsTouched)
			assert.Equalf(t, pm.RowScale, r.RowScale, "%s: row scale drifted from the baseline", r.Operation)
			assert.Equalf(t, pm.OutputPresent, r.Sample.OutputBytes > 0,
				"%s: output presence flipped vs the baseline", r.Operation)
			assert.Equalf(t, pm.ProbeStalled, r.Sample.ProbeStalledOut,
				"%s: write-lock stall-out flipped vs the baseline", r.Operation)
		}
	})

	t.Run("NoRunawayMemoryEvenAtSmoke", func(t *testing.T) {
		for _, r := range results {
			assert.Lessf(t, r.Sample.PeakHeapBytes, int64(smokePeakHeapCeiling),
				"%s: peak heap %s at smoke — an accumulation bug", r.Operation, humanBytes(r.Sample.PeakHeapBytes))
		}
	})

	t.Run("NothingStallsWritesAtSmoke", func(t *testing.T) {
		for _, r := range results {
			if !r.Sample.WriteProbed {
				continue
			}
			assert.Falsef(t, r.Sample.ProbeStalledOut,
				"%s: stalled concurrent writes past the 5s busy_timeout on a 300-contact dataset", r.Operation)
		}
	})

	t.Run("WithinBudgets", func(t *testing.T) {
		bg, err := EmbeddedBudgets()
		require.NoError(t, err)
		// smoke only: the deterministic (rows-touched) budget. The absolute
		// peak-heap ceilings are anchored to `typical` and bite only in the
		// gated at-scale run.
		breaches := bg.CheckDataMovement(results, baseline)
		assert.Emptyf(t, breaches, "smoke data-movement budget breaches (PERF-04): %v", breaches)
	})
}

// TestDataMovementRegistryIsInBaseline is a cheap per-PR structural check that
// the committed baseline has not lost an operation (the byte-for-byte
// currency check is in the gated TestDataMovementAtScale).
func TestDataMovementRegistryIsInBaseline(t *testing.T) {
	baseline, err := EmbeddedDataMovementBaseline()
	require.NoError(t, err)
	for _, op := range DataMovementRegistry() {
		ob, ok := baseline.Operations[op.Name]
		require.Truef(t, ok, "committed datamovement-baseline.json is missing %q — run `make gen-perf-baseline`", op.Name)
		assert.Equal(t, op.Category, ob.Category, "%s: category drift", op.Name)
		assert.Equal(t, op.ExpectedMemoryGrowth, ob.ExpectedMemoryGrowth, "%s: expected-growth drift", op.Name)
		_, ok = ob.Profiles["smoke"]
		assert.Truef(t, ok, "%s: no smoke metric in the baseline", op.Name)
	}
}

// TestDataMovementAtScale is the full cross-scale check: smoke + typical +
// large. Gated behind MYCORRHIZAL_LARGE_TESTS=1 like the other large-dataset
// tests; the nightly / main-push migration-tests.yml large-dataset job sets
// it. It verifies the committed baseline is exactly a fresh smoke+typical run,
// that no operation's peak-memory class regresses, that duplicate detection is
// still a recorded super-linear finding, and that the write probe actually
// caught the FTS rebuild holding the write lock.
func TestDataMovementAtScale(t *testing.T) {
	if os.Getenv("MYCORRHIZAL_LARGE_TESTS") != "1" {
		t.Skip("cross-scale data-movement benchmarks run in the nightly/main-push large-dataset CI job (set MYCORRHIZAL_LARGE_TESTS=1 to run locally)")
	}

	suite, err := RunAllDataMovement(
		[]largedata.Profile{largedata.Smoke, largedata.Typical, largedata.Large},
		t.TempDir(), migratedOpener(t))
	require.NoError(t, err)

	t.Run("CommittedBaselineIsCurrent", func(t *testing.T) {
		committed := DataMovementSuite{
			ResultsByProfile: map[string][]DataMovementResult{
				"smoke":   suite.ResultsByProfile["smoke"],
				"typical": suite.ResultsByProfile["typical"],
			},
			ProfileOrder: []string{"smoke", "typical"},
		}
		fresh, err := committed.Baseline().Marshal()
		require.NoError(t, err)
		onDisk, err := os.ReadFile(DataMovementBaselineFile)
		require.NoError(t, err)
		assert.Equal(t, string(onDisk), string(fresh),
			"internal/perfbench/testdata/datamovement-baseline.json is stale — run `make gen-perf-baseline` and commit the diff")
	})

	t.Run("NoMemoryGrowthRegression", func(t *testing.T) {
		require.NotEmpty(t, suite.MemoryFindings)
		// Log every measured peak heap and every finding under -v, not just the
		// ones that fail: this classification runs on hardware this suite never
		// sees before CI (a fresh 16 GiB runner is not this dev box), so on a
		// failure the only way to see the actual numbers used to be a second
		// CI round-trip with ad hoc diagnostics patched in — worse, the
		// `out="$(go test ...)" ` / `set -e` pattern in migration-tests.yml
		// discarded a failing run's entire stdout before it was ever echoed
		// (fixed alongside this). Printing the numbers unconditionally means a
		// red run's CI log already has what a local repro needs.
		for _, prof := range suite.ProfileOrder {
			for _, r := range suite.ResultsByProfile[prof] {
				t.Logf("memstat %s @ %-8s peakHeap=%d rowsTouched=%d durNanos=%d", r.Operation, prof, r.Sample.PeakHeapBytes, r.Sample.RowsTouched, r.Sample.DurationNanos)
			}
		}
		for _, f := range suite.MemoryFindings {
			t.Logf("finding %s %s→%s class=%s heapRatio=%.2f workRatio=%.2f expected=%s regression=%v", f.Operation, f.SmallProfile, f.LargeProfile, f.Class, f.PeakHeapRatio, f.RowsTouchedRatio, f.Expected, f.Regression)
		}
		var superlinear int
		for _, f := range suite.MemoryFindings {
			assert.Falsef(t, f.Regression,
				"%s peak memory grew %s across %s→%s (heap x%.1f for work x%.1f) — expected at most %s",
				f.Operation, f.Class, f.SmallProfile, f.LargeProfile, f.PeakHeapRatio, f.RowsTouchedRatio, f.Expected)
			if f.Class == GrowthSuperlinear {
				superlinear++
			}
		}
		assert.GreaterOrEqual(t, superlinear, 1,
			"duplicate detection is expected to stay a recorded super-linear peak-memory finding")
	})

	t.Run("WriteProbeSeesTheFTSRebuildHoldTheLock", func(t *testing.T) {
		large := suite.ResultsByProfile["large"]
		fts, ok := findDMResult(large, "fts.rebuild")
		require.True(t, ok)
		backup, ok := findDMResult(large, "backup.vacuum_into")
		require.True(t, ok)
		assert.Greater(t, fts.Sample.ProbeMaxStallNanos, backup.Sample.ProbeMaxStallNanos,
			"the FTS rebuild holds the write lock longer than VACUUM INTO (which reads through the WAL)")
	})

	// PERF-04 (issue #471): the absolute peak-heap ceilings — the OOM
	// boundary — are enforced here, at the anchor profile, in release
	// validation rather than per PR.
	t.Run("WithinBudgetsAtScale", func(t *testing.T) {
		bg, err := EmbeddedBudgets()
		require.NoError(t, err)
		base, err := EmbeddedDataMovementBaseline()
		require.NoError(t, err)
		breaches := bg.CheckDataMovement(suite.ResultsByProfile[bg.AnchorProfile], base)
		assert.Emptyf(t, breaches, "typical data-movement budget breaches (PERF-04): %v", breaches)
	})

	t.Run("WallClockTrend", func(t *testing.T) {
		assertWallClockTrendAdvisory(t, Suite{}, suite)
	})
}

// TestMigrationAtScaleResourceEnvelope records migration's resource envelope
// "for comparability with the rest" (issue #470) — issue #495 owns migration
// depth (the per-release matrix at 2,010 contacts). It transplants a populated
// dataset down to the v0.6.0 schema floor and measures the `→ current` upgrade
// through the same sampler as every other operation, asserting the peak heap
// stays bounded: a table-rebuild migration that loaded a whole table into
// memory would fail here.
func TestMigrationAtScaleResourceEnvelope(t *testing.T) {
	if os.Getenv("MYCORRHIZAL_LARGE_TESTS") != "1" {
		t.Skip("migration resource envelope runs in the nightly/main-push large-dataset CI job (set MYCORRHIZAL_LARGE_TESTS=1 to run locally)")
	}

	// Typical (900 contacts) keeps this to ~1 minute; #495's job covers the
	// 2,010-contact per-release matrix.
	src := dbtest.NewAt(t, filepath.Join(t.TempDir(), "mig-src.db"))
	base, err := canonicalfixture.Read()
	require.NoError(t, err)
	_, err = largedata.Populate(src, base, largedata.Typical)
	require.NoError(t, err)

	const v060 = 31 // schemafixture.SupportedReleases[0].Version
	floor := filepath.Join(t.TempDir(), "mig-v060.db")
	require.NoError(t, schemafixture.TransplantDataToVersion(src, v060, floor))

	sample, err := sampleResources([]string{floor}, nil, func() (int, int64, error) {
		if err := database.MigrateUp(floor); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	})
	require.NoError(t, err)

	t.Logf("migration v0.6.0→current (typical, 900 contacts): duration=%s peak-heap=%s peak-extra-disk=%s",
		humanDuration(sample.DurationNanos), humanBytes(sample.PeakHeapBytes), humanBytes(sample.PeakExtraDiskBytes))
	assert.Less(t, sample.PeakHeapBytes, int64(64<<20),
		"migration peak heap should not scale with the table size — a table rebuild must stream, not load")
}

// TestPeakMemoryClassifierCatchesAccumulation is the CLAUDE.md hand-verify for
// this ticket: an operation whose peak heap scales with its input must be
// caught by the growth classifier, and one whose peak heap stays flat must
// not be. It drives the REAL memory sampler (sampleResources) against real
// allocations — an accumulating "operation" that holds a slice proportional to
// its work, and a streaming one that holds a fixed buffer — then runs both
// through AnalyzeMemoryGrowth exactly as a benchmark run would.
func TestPeakMemoryClassifierCatchesAccumulation(t *testing.T) {
	// sink defeats the optimiser so the allocation is really live during the
	// sampling window.
	var sink []byte
	measureAlloc := func(bytesToHold int, rowsTouched int) ResourceSample {
		s, err := sampleResources(nil, nil, func() (int, int64, error) {
			sink = make([]byte, bytesToHold)
			for i := 0; i < len(sink); i += 4096 {
				sink[i] = byte(i)
			}
			runtime.KeepAlive(sink)
			return rowsTouched, 0, nil
		})
		require.NoError(t, err)
		return s
	}

	t.Run("accumulating is flagged", func(t *testing.T) {
		small := DataMovementResult{
			Operation: "accumulate", Profile: "smoke", RowScale: 300,
			ExpectedMemoryGrowth: GrowthConstant,
			Sample:               withRows(measureAlloc(8<<20, 300), 300),
		}
		large := DataMovementResult{
			Operation: "accumulate", Profile: "large", RowScale: 45000,
			ExpectedMemoryGrowth: GrowthConstant,
			Sample:               withRows(measureAlloc(96<<20, 45000), 45000),
		}
		findings := AnalyzeMemoryGrowth([]DataMovementResult{small, large})
		require.Len(t, findings, 1)
		assert.NotEqual(t, GrowthConstant, findings[0].Class,
			"an op holding 8MB→96MB as its work grew 150x must not read as constant")
		assert.True(t, findings[0].Regression, "and it must be a regression against a constant expectation")
	})

	t.Run("streaming is not flagged", func(t *testing.T) {
		fixed := 8 << 20
		small := DataMovementResult{
			Operation: "stream", Profile: "smoke", RowScale: 300,
			ExpectedMemoryGrowth: GrowthConstant,
			Sample:               withRows(measureAlloc(fixed, 300), 300),
		}
		large := DataMovementResult{
			Operation: "stream", Profile: "large", RowScale: 45000,
			ExpectedMemoryGrowth: GrowthConstant,
			Sample:               withRows(measureAlloc(fixed, 45000), 45000),
		}
		findings := AnalyzeMemoryGrowth([]DataMovementResult{small, large})
		require.Len(t, findings, 1)
		assert.Equal(t, GrowthConstant, findings[0].Class, "a fixed-size buffer must read as constant")
		assert.False(t, findings[0].Regression)
	})
}

func withRows(s ResourceSample, rows int) ResourceSample {
	s.RowsTouched = rows
	return s
}
