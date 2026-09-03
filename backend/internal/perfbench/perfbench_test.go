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

// TestCoreOperationBenchmarks is the PERF-02 suite (issue #469). One
// smoke+typical measurement pass feeds every assertion:
//
//   - every listed operation has a query count and result size at every
//     measured profile;
//   - none exceeds the committed baseline (the N+1 regression gate);
//   - the committed baseline.json is exactly what a fresh run produces;
//   - no operation's cross-scale growth class is worse than declared, and the
//     one known super-linear operation (duplicate detection) is still recorded
//     as such.
//
// It runs against the REAL migrated schema (database.InitDB — CLAUDE.md
// backend trap #1); the smoke+typical seed is a few seconds.
func TestCoreOperationBenchmarks(t *testing.T) {
	suite, err := RunAll([]largedata.Profile{largedata.Smoke, largedata.Typical}, t.TempDir(), nil)
	require.NoError(t, err)

	baseline, err := EmbeddedBaseline()
	require.NoError(t, err)

	t.Run("EveryOperationMeasuredAtEveryProfile", func(t *testing.T) {
		for _, profile := range suite.ProfileOrder {
			for _, op := range Registry() {
				r, ok := findResult(suite.ResultsByProfile[profile], op.Name)
				require.Truef(t, ok, "operation %q not measured at profile %q", op.Name, profile)
				assert.Positivef(t, r.Queries, "operation %q at %q issued no queries — did it actually run?", op.Name, profile)
			}
		}
	})

	t.Run("NoOperationExceedsBaselineQueryCount", func(t *testing.T) {
		for _, profile := range suite.ProfileOrder {
			for _, r := range suite.ResultsByProfile[profile] {
				ob, ok := baseline.Operations[r.Operation]
				require.Truef(t, ok, "no baseline for %q — regenerate with `make gen-perf-baseline`", r.Operation)
				pm, ok := ob.Profiles[profile]
				require.Truef(t, ok, "no baseline for %q at profile %q", r.Operation, profile)
				assert.LessOrEqualf(t, r.Queries, pm.Queries,
					"%s @ %s: %d queries exceeds committed baseline %d — an N+1 regression, or regenerate if deliberate",
					r.Operation, profile, r.Queries, pm.Queries)
				assert.LessOrEqualf(t, r.ResultSize, pm.ResultSize+pm.ResultSize/10+1,
					"%s @ %s: result size %d grew past baseline %d", r.Operation, profile, r.ResultSize, pm.ResultSize)
			}
		}
	})

	t.Run("CommittedBaselineIsCurrent", func(t *testing.T) {
		fresh, err := suite.Baseline().Marshal()
		require.NoError(t, err)
		committed, err := os.ReadFile(BaselineFile)
		require.NoError(t, err)
		assert.Equal(t, string(committed), string(fresh),
			"internal/perfbench/testdata/baseline.json is stale — run `make gen-perf-baseline` and commit the diff")
	})

	t.Run("GrowthShapeHasNoRegression", func(t *testing.T) {
		require.NotEmpty(t, suite.Findings, "growth analysis needs at least two scales")
		var superlinear int
		for _, f := range suite.Findings {
			assert.Falsef(t, f.Regression,
				"%s grew %s across %s→%s (query x%.1f, result x%.1f) — expected at most %s",
				f.Operation, f.Class, f.SmallProfile, f.LargeProfile, f.QueryRatio, f.ResultRatio, f.Expected)
			if f.Class == GrowthSuperlinear {
				superlinear++
				assert.Equalf(t, GrowthSuperlinear, f.Expected,
					"%s measured super-linear but is not declared so — record it in the registry", f.Operation)
			}
		}
		assert.GreaterOrEqual(t, superlinear, 1,
			"duplicate detection is expected to still show as a super-linear finding on the block-replicated fixture")
	})

	t.Run("PathologicalRecordsAreMeasuredExplicitly", func(t *testing.T) {
		_, ok := findResult(suite.ResultsByProfile["smoke"], "contact_detail.pathological")
		assert.True(t, ok, "the pathological contact-detail measurement must be in the registry")
	})
}

// TestSmokeEnv covers the fixture-handle resolution and the Measure/expect
// error paths in one Env build (the Env is the expensive part).
func TestSmokeEnv(t *testing.T) {
	e, err := NewEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: t.TempDir()})
	require.NoError(t, err)
	defer e.Close()

	t.Run("ResolvesDistinctHandles", func(t *testing.T) {
		assert.NotZero(t, e.UserID)
		assert.NotZero(t, e.HubContact.ID)
		assert.NotZero(t, e.NormalContact.ID)
		assert.NotEqual(t, e.HubContact.ID, e.NormalContact.ID, "the hub and the normal contact must differ")
		assert.NotEqual(t, e.HubContact.ID, e.SecondHubContact.ID, "the second hub must not be the chain head")
		assert.NotEqual(t, e.NormalContact.ID, e.PathologicalContact.ID, "the pathological record must be its own contact")
		assert.NotEmpty(t, e.CircleName)
		assert.Greater(t, e.ContactCount(), largedata.Smoke.Contacts, "multi-user smoke populates more than one user's worth")
	})

	t.Run("ExpectSurfacesNon2xx", func(t *testing.T) {
		_, err := e.expect(e.get("/contacts/99999999/detail"), 200)
		assert.ErrorContains(t, err, "status 404")
	})

	t.Run("MeasurePropagatesWarmUpError", func(t *testing.T) {
		_, err := Measure(e, Operation{Name: "boom", Category: "read", Run: func(*Env) (int, error) {
			return 0, assert.AnError
		}})
		assert.ErrorContains(t, err, "warm-up boom")
	})

	t.Run("MeasurePropagatesDestructiveRunError", func(t *testing.T) {
		_, err := Measure(e, Operation{Name: "boom", Category: "write", Destructive: true, Run: func(*Env) (int, error) {
			return 0, assert.AnError
		}})
		assert.ErrorContains(t, err, "run boom")
	})

	t.Run("MeasureReadOpProducesMedianOverIterations", func(t *testing.T) {
		r, err := Measure(e, Operation{Name: "noop", Category: "read", ExpectedGrowth: GrowthConstant, Run: func(*Env) (int, error) {
			return 7, nil
		}})
		require.NoError(t, err)
		assert.Equal(t, readIterations, r.Iterations)
		assert.Equal(t, 7, r.ResultSize)
	})
}

// TestConcurrentWriteShape is the in-process analogue of cmd/loadsmoke: fire
// overlapping writes through the perfbench router and assert none hits
// SQLITE_BUSY / "database is locked" (CLAUDE.md backend trap #9 — the
// _txlock=immediate protection) and every write still succeeds.
func TestConcurrentWriteShape(t *testing.T) {
	e, err := NewEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: t.TempDir()})
	require.NoError(t, err)
	defer e.Close()
	e.Router() // build once before the goroutines race on it

	const workers, perWorker = 6, 15
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

// TestProfileBenchmarksAtScale runs the suite at the `large` profile. Gated
// behind MYCORRHIZAL_LARGE_TESTS=1 like the other large-dataset tests (the
// nightly / main-push migration-tests.yml job sets it); the ~2-minute seed is
// not worth blocking every PR on. It re-asserts the query-count ceiling and
// that no operation's growth class regresses between typical and large.
func TestProfileBenchmarksAtScale(t *testing.T) {
	if os.Getenv("MYCORRHIZAL_LARGE_TESTS") != "1" {
		t.Skip("large-profile benchmarks run in the nightly/main-push large-dataset CI job (set MYCORRHIZAL_LARGE_TESTS=1 to run locally)")
	}

	suite, err := RunAll([]largedata.Profile{largedata.Typical, largedata.Large}, t.TempDir(), nil)
	require.NoError(t, err)

	baseline, err := EmbeddedBaseline()
	require.NoError(t, err)

	for _, r := range suite.ResultsByProfile["large"] {
		ob := baseline.Operations[r.Operation]
		// Every core operation is constant in query count; large must not
		// balloon past a small multiple of the typical baseline.
		if tp, ok := ob.Profiles["typical"]; ok && ob.ExpectedGrowth == GrowthConstant {
			assert.LessOrEqualf(t, r.Queries, tp.Queries*2+5,
				"%s @ large: %d queries vs typical baseline %d — query count is not staying constant at scale",
				r.Operation, r.Queries, tp.Queries)
		}
	}

	for _, f := range suite.Findings {
		assert.Falsef(t, f.Regression,
			"%s regressed to %s between %s and %s (expected at most %s)",
			f.Operation, f.Class, f.SmallProfile, f.LargeProfile, f.Expected)
		if f.DurationConcern {
			t.Logf("advisory: %s wall-clock grew x%.1f for x%.1f rows (%s→%s)",
				f.Operation, f.DurationRatio, f.RowScaleRatio, f.SmallProfile, f.LargeProfile)
		}
	}
}
