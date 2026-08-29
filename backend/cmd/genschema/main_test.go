package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/schemafixture"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary repository root with a backend/database/migrations
// directory (what findRepoRoot keys on), so the command resolves a repo root
// without touching the real tree. The migration chain itself is embedded, so
// GenerateDump works anywhere.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "database", "migrations"), 0o755))
	return root
}

// TestRunGeneratesEverySupportedDump proves the happy path: from the repo
// root, run() regenerates every SupportedReleases dump into
// backend/database/testdata/schemas/ and exits 0.
func TestRunGeneratesEverySupportedDump(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(root)

	require.Equal(t, 0, run())

	schemasDir := filepath.Join(root, schemafixture.SchemaDumpsDirRel)
	for _, release := range schemafixture.SupportedReleases {
		body, err := os.ReadFile(filepath.Join(schemasDir, schemafixture.DumpFile(release)))
		require.NoErrorf(t, err, "%s must be written", schemafixture.DumpFile(release))
		require.NotEmpty(t, body)
	}
}

// TestRunFailsOutsideRepo proves findRepoRoot's error path exits 2 when the
// working directory is not under a repository root.
func TestRunFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableSchemasDir proves a MkdirAll failure exits 2 — here
// backend/database/testdata is a file, so the schemas directory cannot be
// created.
func TestRunFailsOnUnwritableSchemasDir(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "database"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "database", "testdata"), []byte("not a dir"), 0o644))
	t.Chdir(root)

	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableDump proves a WriteFile failure exits 2 — the
// schemas directory exists but one target filename is already a directory, so
// the write fails deterministically (EISDIR) regardless of the running user.
func TestRunFailsOnUnwritableDump(t *testing.T) {
	root := fakeRepo(t)
	schemasDir := filepath.Join(root, schemafixture.SchemaDumpsDirRel)
	require.NoError(t, os.MkdirAll(schemasDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(schemasDir, schemafixture.DumpFile(schemafixture.SupportedReleases[0])), 0o755))
	t.Chdir(root)

	require.Equal(t, 2, run())
}
