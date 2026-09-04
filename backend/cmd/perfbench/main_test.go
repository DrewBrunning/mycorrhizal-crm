package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/largedata"
	"mycorrhizal/internal/perfbench"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSuite is a tiny hand-built Suite so run()'s glue can be exercised
// without the multi-second real measurement pass (that path is covered by
// perfbench.TestCoreOperationBenchmarks).
func fakeSuite(profiles []largedata.Profile) (perfbench.Suite, error) {
	s := perfbench.Suite{ResultsByProfile: map[string][]perfbench.Result{}}
	for i, p := range profiles {
		s.ProfileOrder = append(s.ProfileOrder, p.Name)
		s.ResultsByProfile[p.Name] = []perfbench.Result{{
			Operation: "contact_list.plain", Profile: p.Name, Category: "read",
			Queries: 1, ResultSize: 50, RowScale: 300 * (i + 1), ExpectedGrowth: perfbench.GrowthConstant,
		}}
	}
	return s, nil
}

// fakeDMSuite is the PERF-03 analogue of fakeSuite.
func fakeDMSuite(profiles []largedata.Profile) (perfbench.DataMovementSuite, error) {
	s := perfbench.DataMovementSuite{ResultsByProfile: map[string][]perfbench.DataMovementResult{}}
	for i, p := range profiles {
		s.ProfileOrder = append(s.ProfileOrder, p.Name)
		s.ResultsByProfile[p.Name] = []perfbench.DataMovementResult{{
			Operation: "export.bundle", Profile: p.Name, Category: "export",
			RowScale: 300 * (i + 1), ExpectedMemoryGrowth: perfbench.GrowthLinear,
			Sample: perfbench.ResourceSample{RowsTouched: 300 * (i + 1), OutputBytes: 1024},
		}}
	}
	return s, nil
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "internal", "perfbench", "testdata"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "development"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "go.mod"), []byte("module x\n"), 0o600))
	return root
}

func TestRun_WritesThenCheckIsClean(t *testing.T) {
	root := fakeRepo(t)
	var out bytes.Buffer

	require.Equal(t, 0, run(nil, &out, root, fakeSuite, fakeDMSuite))
	assert.Contains(t, out.String(), "wrote")

	baselinePath, reportPath := perfbench.ArtifactPaths(root)
	assert.FileExists(t, baselinePath)
	assert.FileExists(t, reportPath)
	dmBaselinePath, dmReportPath := perfbench.DataMovementArtifactPaths(root)
	assert.FileExists(t, dmBaselinePath)
	assert.FileExists(t, dmReportPath)
	assert.FileExists(t, perfbench.WallClockTrendPath(root))

	out.Reset()
	assert.Equal(t, 0, run([]string{"-check"}, &out, root, fakeSuite, fakeDMSuite))
	assert.Contains(t, out.String(), "current")
}

func TestRun_CheckDetectsStale(t *testing.T) {
	root := fakeRepo(t)
	baselinePath, _ := perfbench.ArtifactPaths(root)
	require.NoError(t, os.WriteFile(baselinePath, []byte("stale\n"), 0o600))

	var out bytes.Buffer
	assert.Equal(t, 1, run([]string{"-check"}, &out, root, fakeSuite, fakeDMSuite))
	assert.Contains(t, out.String(), "STALE")
}

func TestRun_CheckDetectsStaleDataMovementBaseline(t *testing.T) {
	root := fakeRepo(t)
	// The PERF-02 baseline is current (write a real pass first), the PERF-03 one is not.
	var out bytes.Buffer
	require.Equal(t, 0, run(nil, &out, root, fakeSuite, fakeDMSuite))
	dmBaselinePath, _ := perfbench.DataMovementArtifactPaths(root)
	require.NoError(t, os.WriteFile(dmBaselinePath, []byte("stale\n"), 0o600))

	out.Reset()
	assert.Equal(t, 1, run([]string{"-check"}, &out, root, fakeSuite, fakeDMSuite))
	assert.Contains(t, out.String(), "datamovement-baseline.json")
}

func TestRun_MeasurementError(t *testing.T) {
	root := fakeRepo(t)
	var out bytes.Buffer
	code := run(nil, &out, root, func([]largedata.Profile) (perfbench.Suite, error) {
		return perfbench.Suite{}, errors.New("boom")
	}, fakeDMSuite)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "boom")
}

func TestRun_DataMovementMeasurementError(t *testing.T) {
	root := fakeRepo(t)
	var out bytes.Buffer
	code := run(nil, &out, root, fakeSuite, func([]largedata.Profile) (perfbench.DataMovementSuite, error) {
		return perfbench.DataMovementSuite{}, errors.New("dm-boom")
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "dm-boom")
}

func TestRun_LargeFlagAddsProfile(t *testing.T) {
	root := fakeRepo(t)
	var seen int
	var out bytes.Buffer
	run([]string{"-large"}, &out, root, func(p []largedata.Profile) (perfbench.Suite, error) {
		seen = len(p)
		return fakeSuite(p)
	}, fakeDMSuite)
	assert.Equal(t, 3, seen, "-large adds the large profile to smoke+typical")
}

func TestRun_LargeTestsEnvForcesLargeProfile(t *testing.T) {
	t.Setenv("MYCORRHIZAL_LARGE_TESTS", "1")
	root := fakeRepo(t)
	var seen int
	var out bytes.Buffer
	run(nil, &out, root, func(p []largedata.Profile) (perfbench.Suite, error) {
		seen = len(p)
		return fakeSuite(p)
	}, fakeDMSuite)
	assert.Equal(t, 3, seen, "MYCORRHIZAL_LARGE_TESTS=1 adds the large profile even without -large")
}

func TestRun_WriteFailureIsExit2(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "docs")))
	var out bytes.Buffer
	assert.Equal(t, 2, run(nil, &out, root, fakeSuite, fakeDMSuite))
}

func TestRun_BadFlagIsUsageError(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 2, run([]string{"-nonsense"}, &out, fakeRepo(t), fakeSuite, fakeDMSuite))
}

func TestRun_RepoRootNotFound(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 2, run(nil, &out, t.TempDir(), fakeSuite, fakeDMSuite))
	assert.Contains(t, strings.ToLower(out.String()), "repo root")
}
