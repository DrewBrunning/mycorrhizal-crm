package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary repository root with a backend/integrations
// directory (what findRepoRoot keys on), so the command resolves a repo root
// without touching the real tree. integrations.Render() has no filesystem
// dependency, so it works anywhere.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "integrations"), 0o755))
	return root
}

// TestRunWritesCommittedMatrix proves the happy path: from the repo root, run()
// writes docs/int-01-integration-classification-matrix.md and exits 0.
func TestRunWritesCommittedMatrix(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(root)

	require.Equal(t, 0, run())

	body, err := os.ReadFile(filepath.Join(root, "docs", "int-01-integration-classification-matrix.md"))
	require.NoError(t, err, "matrix must be written")
	require.Contains(t, string(body), "INT-01")
	require.Contains(t, string(body), "Transient vs permanent")
}

// TestRunFailsOutsideRepo proves findRepoRoot's error path exits 2 when the
// working directory is not under a repository root.
func TestRunFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableDocs proves a MkdirAll failure exits 2 — here the
// repo's docs/ path is a file, so the matrix directory cannot be created.
func TestRunFailsOnUnwritableDocs(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs"), []byte("not a dir"), 0o644))
	t.Chdir(root)

	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableMatrix proves a WriteFile failure exits 2 — the docs
// directory exists but the target filename is already a directory (EISDIR).
func TestRunFailsOnUnwritableMatrix(t *testing.T) {
	root := fakeRepo(t)
	target := filepath.Join(root, "docs", "int-01-integration-classification-matrix.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.Mkdir(target, 0o755))
	t.Chdir(root)

	require.Equal(t, 2, run())
}
