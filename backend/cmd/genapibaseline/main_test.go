package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/apibaseline"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary repository root with a backend/openapi.yaml
// copied from the real one, so findRepoRoot resolves and the generator has a
// spec to derive from without touching the real tree.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	require.NoError(t, os.MkdirAll(backendDir, 0o755))
	spec, err := os.ReadFile(filepath.Join("..", "..", apibaseline.SpecPath))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(backendDir, apibaseline.SpecPath), spec, 0o644))
	return root
}

// TestRunRegeneratesBaseline proves the happy path: from the repo root, run()
// writes the baseline and leaves the committed file byte-identical (the drift
// test TestContractBaselineIsCurrent enforces that identity on every `go
// test`).
func TestRunRegeneratesBaseline(t *testing.T) {
	root := fakeRepo(t)

	before, err := os.ReadFile(filepath.Join("..", "..", "internal", "apibaseline", apibaseline.BaselineFile))
	require.NoError(t, err)

	t.Chdir(root)
	require.Equal(t, 0, run())

	after, err := os.ReadFile(filepath.Join(root, "backend", "internal", "apibaseline", apibaseline.BaselineFile))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "regenerating from an unchanged spec must not change the committed baseline")
}

// TestRunFailsOnBrokenSpec proves a spec that fails validation exits 1 — a
// broken source of truth must never silently leave a stale baseline in place.
func TestRunFailsOnBrokenSpec(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "backend", apibaseline.SpecPath),
		[]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: \"1.0\"\npaths:\n  /x:\n    get:\n      responses: {}\n"),
		0o644))
	t.Chdir(root)

	require.Equal(t, 1, run())
}

// TestRunFailsOutsideRepo proves findRepoRoot's error path exits 2 when the
// working directory is not under a repository root.
func TestRunFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableBaseline proves a WriteFile failure exits 2 — the
// baseline directory exists but the target filename is already a directory, so
// the write fails deterministically (EISDIR) regardless of the running user.
func TestRunFailsOnUnwritableBaseline(t *testing.T) {
	root := fakeRepo(t)
	// BaselineFile is "testdata/v1.json" — make the testdata path a file so
	// MkdirAll fails deterministically.
	apibaselineDir := filepath.Join(root, "backend", "internal", "apibaseline")
	require.NoError(t, os.MkdirAll(apibaselineDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(apibaselineDir, "testdata"), []byte("not a dir"), 0o644))
	t.Chdir(root)
	require.Equal(t, 2, run())
}

func TestRunFailsOnUnwritableBaselineFile(t *testing.T) {
	root := fakeRepo(t)
	targetDir := filepath.Join(root, "backend", "internal", "apibaseline", "testdata")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(targetDir, "v1.json"), 0o755))
	t.Chdir(root)

	require.Equal(t, 2, run())
}
