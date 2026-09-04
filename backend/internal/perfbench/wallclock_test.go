package perfbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteWallClockTrend(t *testing.T) {
	root := fakeRepo(t) // from artifacts_test.go — lays out backend/internal/perfbench/testdata
	path := WallClockTrendPath(root)
	assert.Equal(t, filepath.Join(root, "backend", "internal", "perfbench", WallClockTrendFile), path)

	data := []byte("{\"note\":\"x\"}\n")
	require.NoError(t, WriteWallClockTrend(root, data))
	got, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	require.NoError(t, err)
	assert.Equal(t, data, got)

	// A missing parent dir surfaces the error rather than panicking.
	assert.Error(t, WriteWallClockTrend(filepath.Join(root, "nonexistent"), data))
}

// assertWallClockTrendAdvisory compares a fresh measured run against the
// committed wallclock-trend.json and surfaces any >3x move for human review.
// It NEVER fails the test (issue #471 action 3: wall-clock is a trend, not a
// gate) — it logs and emits a GitHub `::warning::` annotation.
func assertWallClockTrendAdvisory(t *testing.T, core Suite, dm DataMovementSuite) {
	t.Helper()
	prev, err := EmbeddedWallClockTrend()
	require.NoError(t, err)
	curr := BuildWallClockTrend(core, dm)

	moves, comparable := CompareWallClock(prev, curr, wallClockTrendReviewRatio)
	if !comparable {
		t.Logf("wall-clock trend: committed baseline is %q, this run is %q — cross-hardware comparison skipped",
			prev.GeneratedOn, curr.GeneratedOn)
		return
	}
	if len(moves) == 0 {
		t.Logf("wall-clock trend: no operation moved more than %.0fx since the committed baseline", wallClockTrendReviewRatio)
		return
	}
	for _, m := range moves {
		line := fmt.Sprintf("%s/%s @ %s moved x%.2f (%s -> %s) since the committed wall-clock trend — review whether it is real",
			m.Scope, m.Operation, m.Profile, m.Ratio, humanDuration(m.PrevNanos), humanDuration(m.CurrNanos))
		t.Logf("advisory: %s", line)
		fmt.Printf("::warning title=perf wall-clock trend::%s\n", line)
	}
}

func TestEmbeddedWallClockTrendParses(t *testing.T) {
	tr, err := EmbeddedWallClockTrend()
	require.NoError(t, err)
	assert.Contains(t, tr.Note, "trend")
	// The bootstrap file ships empty maps; a regenerated one fills them. Both
	// are valid.
	assert.NotNil(t, tr.Core)
	assert.NotNil(t, tr.DataMovement)
}

func TestBuildWallClockTrend(t *testing.T) {
	core := Suite{ResultsByProfile: map[string][]Result{
		"smoke":   {{Operation: "contact_detail", Profile: "smoke", MedianNanos: 3_000_000}},
		"typical": {{Operation: "contact_detail", Profile: "typical", MedianNanos: 3_400_000}},
	}}
	dm := DataMovementSuite{ResultsByProfile: map[string][]DataMovementResult{
		"typical": {{Operation: "import.vcf", Profile: "typical", Sample: ResourceSample{DurationNanos: 11_000_000_000}}},
	}}

	tr := BuildWallClockTrend(core, dm)
	assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, tr.GeneratedOn)
	assert.EqualValues(t, 3_400_000, tr.Core["contact_detail"]["typical"])
	assert.EqualValues(t, 3_000_000, tr.Core["contact_detail"]["smoke"])
	assert.EqualValues(t, 11_000_000_000, tr.DataMovement["import.vcf"]["typical"])

	// Marshals to stable, indented JSON with a trailing newline.
	b, err := tr.Marshal()
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), b[len(b)-1])
	var round WallClockTrend
	require.NoError(t, json.Unmarshal(b, &round))
	assert.Equal(t, tr, round)
}

func TestCompareWallClock(t *testing.T) {
	prev := WallClockTrend{
		GeneratedOn: "linux/amd64",
		Core: map[string]map[string]int64{
			"steady":  {"typical": 1_000_000},
			"slower":  {"typical": 1_000_000, "large": 2_000_000},
			"faster":  {"typical": 9_000_000},
			"missing": {"typical": 1_000_000},
		},
		DataMovement: map[string]map[string]int64{
			"imp": {"typical": 10_000_000_000},
		},
	}
	curr := WallClockTrend{
		GeneratedOn: "linux/amd64",
		Core: map[string]map[string]int64{
			"steady": {"typical": 1_200_000},                      // 1.2x — under threshold
			"slower": {"typical": 5_000_000, "large": 20_000_000}, // 5x / 10x — both flagged
			"faster": {"typical": 1_000_000},                      // 9x faster — flagged
			"fresh":  {"typical": 2_000_000},                      // no prev — skipped
		},
		DataMovement: map[string]map[string]int64{
			"imp": {"typical": 40_000_000_000}, // 4x — flagged
		},
	}

	moves, comparable := CompareWallClock(prev, curr, 3.0)
	require.True(t, comparable)
	require.Len(t, moves, 4)
	// Sorted: core before data_movement, then by operation, then by profile.
	assert.Equal(t, "core", moves[0].Scope)
	assert.Equal(t, "faster", moves[0].Operation)
	assert.InDelta(t, 9.0, moves[0].Ratio, 0.01)
	assert.Equal(t, "slower", moves[1].Operation)
	assert.Equal(t, "large", moves[1].Profile)
	assert.InDelta(t, 10.0, moves[1].Ratio, 0.01)
	assert.Equal(t, "slower", moves[2].Operation)
	assert.Equal(t, "typical", moves[2].Profile)
	assert.InDelta(t, 5.0, moves[2].Ratio, 0.01)
	assert.Equal(t, "data_movement", moves[3].Scope)
	assert.Equal(t, "imp", moves[3].Operation)
}

func TestCompareWallClockNotComparableAcrossArch(t *testing.T) {
	prev := WallClockTrend{GeneratedOn: "darwin/arm64", Core: map[string]map[string]int64{"x": {"typical": 1}}}
	curr := WallClockTrend{GeneratedOn: "linux/amd64", Core: map[string]map[string]int64{"x": {"typical": 100}}}
	moves, comparable := CompareWallClock(prev, curr, 2.0)
	assert.False(t, comparable)
	assert.Nil(t, moves)

	// The bootstrap trend (empty GeneratedOn) is never comparable.
	_, comparable = CompareWallClock(WallClockTrend{}, curr, 2.0)
	assert.False(t, comparable)
}

func TestCompareWallClockIgnoresZeroAndNegative(t *testing.T) {
	prev := WallClockTrend{GeneratedOn: "linux/amd64", Core: map[string]map[string]int64{"x": {"typical": 0}}}
	curr := WallClockTrend{GeneratedOn: "linux/amd64", Core: map[string]map[string]int64{"x": {"typical": 9_000_000}}}
	moves, comparable := CompareWallClock(prev, curr, 2.0)
	assert.True(t, comparable)
	assert.Empty(t, moves, "a zero prior is not a usable ratio")
}
