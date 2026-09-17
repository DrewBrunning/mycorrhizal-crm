package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary repository root with a backend/config
// directory (what findRepoRoot keys on), so the command resolves a repo root
// without touching the real tree. config.RenderRegister() reads the real
// config.Config struct tags, so it works anywhere.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "config"), 0o755))
	return root
}

// TestRunWritesConfigurationReference proves the happy path: from the repo
// root, run() writes docs/configuration-reference.md and exits 0.
func TestRunWritesConfigurationReference(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(root)

	require.Equal(t, 0, run())

	body, err := os.ReadFile(filepath.Join(root, referenceRelPath))
	require.NoError(t, err, "configuration reference must be written")
	require.Contains(t, string(body), "# Configuration reference")
	require.Contains(t, string(body), "## config.Config surface")
	require.Contains(t, string(body), "## Variables outside config.Config")
}

// TestRunFailsOutsideRepo proves findRepoRoot's error path exits 2 when the
// working directory is not under a repository root.
func TestRunFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableDocs proves a MkdirAll failure exits 2 — here the
// repo's docs/ path is a file, so the reference's directory cannot be created.
func TestRunFailsOnUnwritableDocs(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs"), []byte("not a dir"), 0o644))
	t.Chdir(root)

	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableReference proves a WriteFile failure exits 2 — the
// docs directory exists but the target filename is already a directory, so
// the write fails deterministically (EISDIR).
func TestRunFailsOnUnwritableReference(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, referenceRelPath)), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, referenceRelPath), 0o755))
	t.Chdir(root)

	require.Equal(t, 2, run())
}
