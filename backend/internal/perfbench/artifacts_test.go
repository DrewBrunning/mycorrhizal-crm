package perfbench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo lays out just enough of a repo tree for the artifact helpers:
// backend/go.mod, the baseline testdata dir, and docs/development.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "internal", "perfbench", "testdata"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "development"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "go.mod"), []byte("module x\n"), 0o600))
	return root
}

func TestFindRepoRoot(t *testing.T) {
	root := fakeRepo(t)
	nested := filepath.Join(root, "backend", "internal", "perfbench")

	got, err := FindRepoRoot(nested)
	require.NoError(t, err)
	assert.Equal(t, root, got)

	_, err = FindRepoRoot(t.TempDir()) // a bare dir with no backend/go.mod above it
	assert.Error(t, err)
}

func TestFindRepoRoot_FromWorkingDir(t *testing.T) {
	// The test process's cwd is backend/, which is under the repo root.
	got, err := FindRepoRoot("")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(got, "backend", "go.mod"))
}

func TestWriteArtifacts_ReportWriteFailureSurfaces(t *testing.T) {
	root := fakeRepo(t)
	// Remove the docs dir so the report write fails while the baseline write
	// (its dir exists) succeeds.
	require.NoError(t, os.RemoveAll(filepath.Join(root, "docs")))
	err := WriteArtifacts(root, []byte("{}\n"), []byte("# r\n"))
	assert.Error(t, err)
}

func TestWriteArtifacts_BaselineWriteFailureSurfaces(t *testing.T) {
	root := t.TempDir() // no backend/internal/... tree
	err := WriteArtifacts(root, []byte("{}\n"), []byte("# r\n"))
	assert.Error(t, err)
}

func TestWriteAndCheckArtifacts(t *testing.T) {
	root := fakeRepo(t)
	baseline := []byte("{\"baseline\":true}\n")
	report := []byte("# report\n")

	// Nothing written yet: the baseline is stale.
	_, stale := CheckBaseline(root, baseline)
	assert.True(t, stale)

	require.NoError(t, WriteArtifacts(root, baseline, report))
	baselinePath, reportPath := ArtifactPaths(root)
	assert.FileExists(t, reportPath)
	_, stale = CheckBaseline(root, baseline)
	assert.False(t, stale)

	// A drift in the baseline is reported (the report is never gated).
	path, stale := CheckBaseline(root, []byte("changed\n"))
	assert.True(t, stale)
	assert.Equal(t, baselinePath, path)
}

func TestArtifacts_FromSuite(t *testing.T) {
	s := Suite{
		ResultsByProfile: sampleResults(),
		ProfileOrder:     []string{"smoke", "typical"},
	}
	s.Findings = AnalyzeGrowth(append(s.ResultsByProfile["smoke"], s.ResultsByProfile["typical"]...))

	baseline, report, err := s.Artifacts()
	require.NoError(t, err)
	assert.Contains(t, string(baseline), "\"contact_list.plain\"")
	assert.Contains(t, string(report), "# Core operation benchmarks (PERF-02)")
	assert.Contains(t, string(report), "duplicates.find_pairs")
}
