package perfbench

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"mycorrhizal/internal/largedata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoreOperationBenchmarks is the per-PR PERF-02 gate (issue #469): it
// measures every registered operation at the `smoke` profile against the REAL
// migrated schema (internal/dbtest template copy — CLAUDE.md backend trap #1)
// and fails if any exceeds its committed query-count baseline (the N+1
// regression signal) or its recorded result size. The cross-scale growth
// analysis and the full smoke+typical baseline-current check need the 900-row
// `typical` populate, which is too slow for every PR under -race — those live
// in TestBenchmarksAtScale, gated behind MYCORRHIZAL_LARGE_TESTS=1.
func TestCoreOperationBenchmarks(t *testing.T) {
	env, results, err := RunProfileKeepEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: t.TempDir(), OpenMigrated: migratedOpener(t)})
	require.NoError(t, err)
	defer env.Close()

	baseline, err := EmbeddedBaseline()
	require.NoError(t, err)

	t.Run("EveryOperationMeasured", func(t *testing.T) {
		for _, op := range Registry() {
			r, ok := findResult(results, op.Name)
			require.Truef(t, ok, "operation %q was not measured", op.Name)
			assert.Positivef(t, r.Queries, "operation %q issued no queries — did it run?", op.Name)
		}
		_, ok := findResult(results, "contact_detail.pathological")
		assert.True(t, ok, "the pathological contact-detail record must be measured explicitly")
	})

	t.Run("MatchesCommittedSmokeBaseline", func(t *testing.T) {
		for _, r := range results {
			ob, ok := baseline.Operations[r.Operation]
			require.Truef(t, ok, "no baseline for %q — run `make gen-perf-baseline`", r.Operation)
			pm, ok := ob.Profiles["smoke"]
			require.Truef(t, ok, "no smoke baseline for %q", r.Operation)
			assert.LessOrEqualf(t, r.Queries, pm.Queries,
				"%s: %d queries exceeds committed smoke baseline %d — an N+1 regression, or regenerate if deliberate",
				r.Operation, r.Queries, pm.Queries)
			assert.Equalf(t, pm.ResultSize, r.ResultSize,
				"%s: result size %d != committed smoke baseline %d — run `make gen-perf-baseline`",
				r.Operation, r.ResultSize, pm.ResultSize)
			assert.Equalf(t, pm.RowScale, r.RowScale, "%s: row scale drifted from the baseline", r.Operation)
		}
	})

	t.Run("WithinBudgets", func(t *testing.T) {
		bg, err := EmbeddedBudgets()
		require.NoError(t, err)
		// smoke only: the deterministic (query-count) budget is the N+1
		// gate — this is the CLAUDE.md hand-verify target for #471. The
		// absolute wall-clock budgets are anchored to `typical` and so only
		// bite in the gated at-scale run, never on this noisy per-PR box.
		breaches := bg.CheckCore(results, baseline)
		assert.Emptyf(t, breaches, "smoke performance-budget breaches (PERF-04): %v", breaches)
	})

	// The rest reuse the already-populated Env for fixture-handle and
	// error-path coverage without a second populate.
	t.Run("ResolvesDistinctHandles", func(t *testing.T) {
		assert.NotZero(t, env.UserID)
		assert.NotZero(t, env.HubContact.ID)
		assert.NotEqual(t, env.HubContact.ID, env.NormalContact.ID, "hub and normal contact must differ")
		assert.NotEqual(t, env.HubContact.ID, env.SecondHubContact.ID, "the second hub must not be the chain head")
		assert.NotEqual(t, env.NormalContact.ID, env.PathologicalContact.ID, "the pathological record must be its own contact")
		assert.NotEmpty(t, env.CircleName)
		assert.Greater(t, env.ContactCount(), largedata.Smoke.Contacts, "multi-user smoke populates more than one user's worth")
	})

	t.Run("ExpectSurfacesNon2xx", func(t *testing.T) {
		_, err := env.expect(env.get("/contacts/99999999/detail"), 200)
		assert.ErrorContains(t, err, "status 404")
	})

	t.Run("MeasurePropagatesWarmUpError", func(t *testing.T) {
		_, err := Measure(env, Operation{Name: "boom", Category: "read", Run: func(*Env) (int, error) {
			return 0, assert.AnError
		}})
		assert.ErrorContains(t, err, "warm-up boom")
	})

	t.Run("MeasurePropagatesDestructiveRunError", func(t *testing.T) {
		_, err := Measure(env, Operation{Name: "boom", Category: "write", Destructive: true, Run: func(*Env) (int, error) {
			return 0, assert.AnError
		}})
		assert.ErrorContains(t, err, "run boom")
	})

	t.Run("MeasureReadOpIterates", func(t *testing.T) {
		r, err := Measure(env, Operation{Name: "noop", Category: "read", ExpectedGrowth: GrowthConstant, Run: func(*Env) (int, error) {
			return 7, nil
		}})
		require.NoError(t, err)
		assert.Equal(t, readIterations, r.Iterations)
		assert.Equal(t, 7, r.ResultSize)
	})
}

// TestRunAllSmokeWiring exercises the multi-profile RunAll → RunProfile →
// Suite path (which cmd/perfbench drives) with a single cheap profile.
func TestRunAllSmokeWiring(t *testing.T) {
	suite, err := RunAll([]largedata.Profile{largedata.Smoke}, t.TempDir(), migratedOpener(t))
	require.NoError(t, err)

	require.Contains(t, suite.ResultsByProfile, "smoke")
	assert.Equal(t, []string{"smoke"}, suite.ProfileOrder)
	assert.Len(t, suite.ResultsByProfile["smoke"], len(Registry()))
	assert.Empty(t, suite.Findings, "a single profile yields no cross-scale findings")

	data, err := suite.Baseline().Marshal()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"contact_list.plain"`)
	assert.Contains(t, suite.Report(), "# Core operation benchmarks (PERF-02)")
}

// TestConcurrentWriteShape is the in-process analogue of cmd/loadsmoke: fire
// overlapping writes through the perfbench router and assert none hits
// SQLITE_BUSY / "database is locked" (CLAUDE.md backend trap #9 — the
// _txlock=immediate protection) and every write still succeeds.
func TestConcurrentWriteShape(t *testing.T) {
	e, err := NewEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: t.TempDir(), OpenMigrated: migratedOpener(t)})
	require.NoError(t, err)
	defer e.Close()
	e.Router() // build once before the goroutines race on it

	const workers, perWorker = 5, 8
	var wg sync.WaitGroup
	errCh := make(chan string, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				body := contactRecordInputJSON(fmt.Sprintf("Conc%d-%d", w, i), "Test")
				if r := e.post("/contacts", body); r.status != 201 {
					errCh <- fmt.Sprintf("create w%d i%d: status %d: %s", w, i, r.status, truncate(r.body, 200))
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)

	var failures []string
	for msg := range errCh {
		failures = append(failures, msg)
	}
	assert.Emptyf(t, failures, "concurrent writes must not fail (no SQLITE_BUSY under _txlock=immediate):\n%v", failures)
}

// TestBenchmarksAtScale is the full cross-scale check: smoke + typical + large.
// Gated behind MYCORRHIZAL_LARGE_TESTS=1 like the other large-dataset tests —
// the nightly / main-push migration-tests.yml large-dataset job sets it. It
// verifies the committed baseline is exactly a fresh smoke+typical run, that
// no operation's growth class regresses across any scale step, that the one
// known super-linear operation is still recorded, and that query counts stay
// bounded out to the large profile.
func TestBenchmarksAtScale(t *testing.T) {
	if os.Getenv("MYCORRHIZAL_LARGE_TESTS") != "1" {
		t.Skip("cross-scale benchmarks run in the nightly/main-push large-dataset CI job (set MYCORRHIZAL_LARGE_TESTS=1 to run locally)")
	}

	suite, err := RunAll([]largedata.Profile{largedata.Smoke, largedata.Typical, largedata.Large}, t.TempDir(), migratedOpener(t))
	require.NoError(t, err)

	baseline, err := EmbeddedBaseline()
	require.NoError(t, err)

	t.Run("CommittedBaselineIsCurrent", func(t *testing.T) {
		// The committed file records smoke + typical only.
		committedSuite := Suite{
			ResultsByProfile: map[string][]Result{
				"smoke":   suite.ResultsByProfile["smoke"],
				"typical": suite.ResultsByProfile["typical"],
			},
			ProfileOrder: []string{"smoke", "typical"},
		}
		fresh, err := committedSuite.Baseline().Marshal()
		require.NoError(t, err)
		committed, err := os.ReadFile(BaselineFile)
		require.NoError(t, err)
		assert.Equal(t, string(committed), string(fresh),
			"internal/perfbench/testdata/baseline.json is stale — run `make gen-perf-baseline` and commit the diff")
	})

	t.Run("NoGrowthRegression", func(t *testing.T) {
		require.NotEmpty(t, suite.Findings)
		var superlinear int
		for _, f := range suite.Findings {
			assert.Falsef(t, f.Regression,
				"%s grew %s across %s→%s (query x%.1f, result x%.1f) — expected at most %s",
				f.Operation, f.Class, f.SmallProfile, f.LargeProfile, f.QueryRatio, f.ResultRatio, f.Expected)
			if f.Class == GrowthSuperlinear {
				superlinear++
				assert.Equalf(t, GrowthSuperlinear, f.Expected,
					"%s measured super-linear but is not declared so", f.Operation)
			}
			if f.DurationConcern {
				t.Logf("advisory: %s wall-clock grew x%.1f for x%.1f rows (%s→%s)",
					f.Operation, f.DurationRatio, f.RowScaleRatio, f.SmallProfile, f.LargeProfile)
			}
		}
		assert.GreaterOrEqual(t, superlinear, 1,
			"duplicate detection is expected to stay a recorded super-linear finding on the block-replicated fixture")
	})

	t.Run("QueryCountsStayBoundedAtLarge", func(t *testing.T) {
		for _, r := range suite.ResultsByProfile["large"] {
			ob := baseline.Operations[r.Operation]
			if tp, ok := ob.Profiles["typical"]; ok && ob.ExpectedGrowth == GrowthConstant {
				assert.LessOrEqualf(t, r.Queries, tp.Queries*2+5,
					"%s @ large: %d queries vs typical baseline %d — not staying constant at scale",
					r.Operation, r.Queries, tp.Queries)
			}
		}
	})

	// PERF-04 (issue #471): the absolute interactive wall-clock budgets are
	// evaluated here, at the anchor profile — this is the release-validation
	// gate the milestone's "where practical" hedge points at, not something
	// every PR pays for on a noisy runner.
	t.Run("WithinBudgetsAtScale", func(t *testing.T) {
		bg, err := EmbeddedBudgets()
		require.NoError(t, err)
		breaches := bg.CheckCore(suite.ResultsByProfile[bg.AnchorProfile], baseline)
		assert.Emptyf(t, breaches, "typical performance-budget breaches (PERF-04): %v", breaches)
	})

	t.Run("WallClockTrend", func(t *testing.T) {
		assertWallClockTrendAdvisory(t, suite, DataMovementSuite{})
	})
}
