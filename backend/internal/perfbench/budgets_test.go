package perfbench

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedBudgetsValid is the cheap per-PR structural gate: the committed
// budgets.json parses and every registered operation (PERF-02 and PERF-03) has
// a well-formed entry with a non-empty reason. No database — this is pure
// file + registry validation.
func TestEmbeddedBudgetsValid(t *testing.T) {
	bg, err := EmbeddedBudgets()
	require.NoError(t, err)

	assert.Equal(t, "typical", bg.AnchorProfile)
	problems := bg.Validate(Registry(), DataMovementRegistry())
	assert.Empty(t, problems, "budgets.json is malformed:\n%v", problems)
}

// TestBudgetsCoverEveryOperation is the issue #471 "every operation from #469
// and #470 has either a budget or a recorded decision that it does not need
// one" check, phrased per-operation so a gap names itself.
func TestBudgetsCoverEveryOperation(t *testing.T) {
	bg, err := EmbeddedBudgets()
	require.NoError(t, err)

	for _, op := range Registry() {
		cb, ok := bg.CoreOperations[op.Name]
		require.Truef(t, ok, "core operation %q has no budget entry in budgets.json", op.Name)
		assert.NotEmptyf(t, cb.Reason, "core budget %q must record why (a ceiling or an explicit waiver)", op.Name)
	}
	for _, op := range DataMovementRegistry() {
		db, ok := bg.DataMovementOperations[op.Name]
		require.Truef(t, ok, "data-movement operation %q has no budget entry in budgets.json", op.Name)
		assert.NotEmptyf(t, db.Reason, "data-movement budget %q must record why", op.Name)
		assert.Positivef(t, db.MaxPeakHeapMiB, "data-movement budget %q needs a peak-heap ceiling (memory is the OOM axis)", op.Name)
	}
}

func TestValidateCatchesProblems(t *testing.T) {
	core := []Operation{{Name: "known"}, {Name: "missing"}}
	dm := []DataMovementOperation{{Name: "dm_known"}, {Name: "dm_missing"}}
	bg := Budgets{
		AnchorProfile: "",
		CoreOperations: map[string]CoreBudget{
			"known": {MaxWallMS: 10, Reason: ""},   // wall too low + empty reason
			"stale": {MaxWallMS: 200, Reason: "x"}, // not a registered op
		},
		DataMovementOperations: map[string]DataMovementBudget{
			"dm_known": {MaxPeakHeapMiB: 99999, Reason: ""}, // heap too high + empty reason
			"dm_stale": {MaxPeakHeapMiB: 32, Reason: "x"},   // not registered
		},
	}
	problems := bg.Validate(core, dm)

	joined := ""
	for _, p := range problems {
		joined += p + "\n"
	}
	assert.Contains(t, joined, "anchor_profile is empty")
	assert.Contains(t, joined, `core operation "missing" has no budget entry`)
	assert.Contains(t, joined, `core budget "known" has an empty reason`)
	assert.Contains(t, joined, `max_wall_ms 10 outside`)
	assert.Contains(t, joined, `core budget "stale" is not a registered operation`)
	assert.Contains(t, joined, `data-movement budget "dm_known": max_peak_heap_mib 99999 outside`)
	assert.Contains(t, joined, `data-movement budget "dm_known" has an empty reason`)
	assert.Contains(t, joined, `data-movement operation "dm_missing" has no budget entry`)
	assert.Contains(t, joined, `data-movement budget "dm_stale" is not a registered operation`)
}

func TestValidateAcceptsWaivedWallBudget(t *testing.T) {
	bg := Budgets{
		AnchorProfile:          "typical",
		CoreOperations:         map[string]CoreBudget{"op": {MaxWallMS: 0, Reason: "not interactive"}},
		DataMovementOperations: map[string]DataMovementBudget{},
	}
	assert.Empty(t, bg.Validate([]Operation{{Name: "op"}}, nil))
}

func TestCheckCoreQueryBudget(t *testing.T) {
	bg := Budgets{AnchorProfile: "typical", CoreOperations: map[string]CoreBudget{"op": {MaxWallMS: 0, Reason: "x"}}}
	base := Baseline{Operations: map[string]OpBaseline{
		"op": {Profiles: map[string]ProfileMetric{"smoke": {Queries: 10}}},
	}}

	assert.Empty(t, bg.CheckCore([]Result{{Operation: "op", Profile: "smoke", Queries: 10}}, base),
		"query count exactly at the baseline is within budget")

	br := bg.CheckCore([]Result{{Operation: "op", Profile: "smoke", Queries: 11}}, base)
	require.Len(t, br, 1)
	assert.Equal(t, BreachQueryCount, br[0].Kind)
	assert.EqualValues(t, 11, br[0].Measured)
	assert.EqualValues(t, 10, br[0].Budget)

	assert.Empty(t, bg.CheckCore([]Result{{Operation: "op", Profile: "large", Queries: 999}}, base),
		"a profile with no baseline row skips the deterministic check")

	noBaseline := Budgets{AnchorProfile: "typical", CoreOperations: map[string]CoreBudget{"lonely": {MaxWallMS: 0, Reason: "x"}}}
	assert.Empty(t, noBaseline.CheckCore([]Result{{Operation: "lonely", Profile: "smoke", Queries: 999}}, base),
		"an operation absent from the baseline entirely skips the deterministic check")
}

func TestCheckCoreWallClockBudget(t *testing.T) {
	bg := Budgets{AnchorProfile: "typical", CoreOperations: map[string]CoreBudget{
		"slow":   {MaxWallMS: 100, Reason: "x"},
		"waived": {MaxWallMS: 0, Reason: "x"},
	}}
	base := Baseline{Operations: map[string]OpBaseline{
		"slow":   {Profiles: map[string]ProfileMetric{"typical": {Queries: 5}}},
		"waived": {Profiles: map[string]ProfileMetric{"typical": {Queries: 5}}},
	}}
	over := int64(250) * int64(time.Millisecond)

	br := bg.CheckCore([]Result{{Operation: "slow", Profile: "typical", Queries: 5, MedianNanos: over}}, base)
	require.Len(t, br, 1)
	assert.Equal(t, BreachWallClock, br[0].Kind)

	assert.Empty(t, bg.CheckCore([]Result{{Operation: "slow", Profile: "smoke", Queries: 5, MedianNanos: over}}, base),
		"wall-clock budgets apply only at the anchor profile")
	assert.Empty(t, bg.CheckCore([]Result{{Operation: "waived", Profile: "typical", Queries: 5, MedianNanos: int64(9) * int64(time.Second)}}, base),
		"a waived (0) wall budget never breaches")
	assert.Empty(t, bg.CheckCore([]Result{{Operation: "slow", Profile: "typical", Queries: 5, MedianNanos: int64(50) * int64(time.Millisecond)}}, base),
		"under the ceiling is fine")
}

func TestCheckCoreUnbudgeted(t *testing.T) {
	bg := Budgets{AnchorProfile: "typical", CoreOperations: map[string]CoreBudget{}}
	br := bg.CheckCore([]Result{{Operation: "mystery", Profile: "smoke"}}, Baseline{})
	require.Len(t, br, 1)
	assert.Equal(t, BreachUnbudgeted, br[0].Kind)
	assert.Contains(t, br[0].String(), "mystery")
}

func TestCheckDataMovementBudgets(t *testing.T) {
	bg := Budgets{AnchorProfile: "typical", DataMovementOperations: map[string]DataMovementBudget{
		"imp": {MaxPeakHeapMiB: 64, Reason: "x"},
	}}
	base := DataMovementBaseline{Operations: map[string]DataMovementOpBaseline{
		"imp": {Profiles: map[string]DataMovementProfileMetric{
			"smoke":   {RowsTouched: 150},
			"typical": {RowsTouched: 900},
		}},
	}}

	// Rows touched over the baseline, at any profile.
	br := bg.CheckDataMovement([]DataMovementResult{
		{Operation: "imp", Profile: "smoke", Sample: ResourceSample{RowsTouched: 151}},
	}, base)
	require.Len(t, br, 1)
	assert.Equal(t, BreachRowsTouched, br[0].Kind)

	// Peak heap over the ceiling, only at the anchor profile.
	overHeap := ResourceSample{RowsTouched: 900, PeakHeapBytes: int64(65) * mib}
	br = bg.CheckDataMovement([]DataMovementResult{{Operation: "imp", Profile: "typical", Sample: overHeap}}, base)
	require.Len(t, br, 1)
	assert.Equal(t, BreachPeakHeap, br[0].Kind)
	assert.Contains(t, br[0].String(), "MiB")

	assert.Empty(t, bg.CheckDataMovement([]DataMovementResult{
		{Operation: "imp", Profile: "smoke", Sample: ResourceSample{RowsTouched: 150, PeakHeapBytes: int64(200) * mib}},
	}, base), "peak-heap ceiling is not evaluated off the anchor profile")

	br = bg.CheckDataMovement([]DataMovementResult{{Operation: "unknown", Profile: "smoke"}}, base)
	require.Len(t, br, 1)
	assert.Equal(t, BreachUnbudgeted, br[0].Kind)

	// An operation with a budget entry but no baseline row skips the
	// deterministic (rows-touched) check.
	lonely := Budgets{AnchorProfile: "typical", DataMovementOperations: map[string]DataMovementBudget{"lonely": {MaxPeakHeapMiB: 32, Reason: "x"}}}
	assert.Empty(t, lonely.CheckDataMovement([]DataMovementResult{
		{Operation: "lonely", Profile: "smoke", Sample: ResourceSample{RowsTouched: 999999}},
	}, base))
}

func TestBreachStringShapes(t *testing.T) {
	assert.Contains(t, Breach{Operation: "o", Profile: "typical", Kind: BreachQueryCount, Measured: 12, Budget: 8, Detail: "d"}.String(), "query_count 12 over budget 8")
	assert.Contains(t, Breach{Operation: "o", Profile: "typical", Kind: BreachWallClock, Measured: int64(time.Second), Budget: int64(300) * int64(time.Millisecond), Detail: "d"}.String(), "wall_clock")
	assert.Contains(t, Breach{Operation: "o", Profile: "typical", Kind: BreachPeakHeap, Measured: 200 * mib, Budget: 128 * mib, Detail: "d"}.String(), "MiB over budget")
}

func TestLoadBudgetsFromDisk(t *testing.T) {
	bg, err := LoadBudgets(filepath.Join("testdata", "budgets.json"))
	require.NoError(t, err)
	assert.Equal(t, "typical", bg.AnchorProfile)
	assert.NotEmpty(t, bg.CoreOperations)
}

func TestLoadBudgetsErrors(t *testing.T) {
	_, err := LoadBudgets(filepath.Join(t.TempDir(), "nope.json"))
	assert.Error(t, err)

	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{not json"), 0o600))
	_, err = LoadBudgets(bad)
	assert.ErrorContains(t, err, "parsing budgets")
}
