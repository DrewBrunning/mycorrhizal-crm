package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary repository root with a backend/correspondence
// directory (what findRepoRoot keys on), so the command resolves a repo root
// without touching the real tree. correspondence.Render() reads the embedded
// table, so it works anywhere.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "correspondence"), 0o755))
	return root
}

// TestRunWritesCommittedMatrix proves the happy path: from the repo root, run()
// writes docs/data-01-field-compatibility-matrix.md and exits 0.
func TestRunWritesCommittedMatrix(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(root)

	require.Equal(t, 0, run())

	body, err := os.ReadFile(filepath.Join(root, matrixRelPath))
	require.NoError(t, err, "matrix must be written")
	require.Contains(t, string(body), "DATA-01")
	require.Contains(t, string(body), "Loss reports (DATA-02 input)")
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
// directory exists but the target filename is already a directory, so the write
// fails deterministically (EISDIR).
func TestRunFailsOnUnwritableMatrix(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, matrixRelPath)), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, matrixRelPath), 0o755))
	t.Chdir(root)

	require.Equal(t, 2, run())
}
