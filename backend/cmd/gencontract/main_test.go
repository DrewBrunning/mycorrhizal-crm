package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/contractfixtures"

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
	spec, err := os.ReadFile(filepath.Join("..", "..", contractfixtures.SpecPath))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(backendDir, contractfixtures.SpecPath), spec, 0o644))
	return root
}

// TestRunGeneratesAllPinnedFixtures proves the happy path: from the repo
// root, run() writes every pinned fixture to testdata/contract-fixtures/.
func TestRunGeneratesAllPinnedFixtures(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(root)

	require.Equal(t, 0, run())

	for _, pin := range contractfixtures.Pinned {
		body, err := os.ReadFile(filepath.Join(root, contractfixtures.FixturesDir, pin.Filename))
		require.NoErrorf(t, err, "%s must be written", pin.Filename)
		require.NotEmpty(t, body)
	}
}

// TestRunFailsOnBrokenSpec proves a spec that fails validation exits 1 (a
// broken source of truth must never silently leave stale fixtures in place).
func TestRunFailsOnBrokenSpec(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "backend", contractfixtures.SpecPath),
		[]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: \"1.0\"\npaths:\n  /x:\n    get:\n      responses: {}\n"),
		0o644))
	t.Chdir(root)

	require.Equal(t, 1, run())
}

// TestRunFailsOnPinnedResponseWithoutAnExample proves Generate's error path
// exits 1: the spec loads fine, but a pinned response has no `example:`, so
// no fixture can be derived for it. This is the "you added a Pin but no
// example" failure a contributor hits.
func TestRunFailsOnPinnedResponseWithoutAnExample(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "backend", contractfixtures.SpecPath),
		[]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: \"1.0\"\npaths:\n"+
			"  /contacts:\n    get:\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema: {type: object}\n"+
			"  /contacts/{id}/detail:\n    get:\n      parameters:\n        - name: id\n          in: path\n          required: true\n          schema: {type: integer}\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema: {type: object}\n"+
			"  /dashboard:\n    get:\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema: {type: object}\n"),
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

// TestRunFailsOnUnwritableFixturesDir proves a MkdirAll failure exits 2 —
// here testdata/ is a file, so the fixture directory cannot be created.
func TestRunFailsOnUnwritableFixturesDir(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "testdata"), []byte("not a dir"), 0o644))
	t.Chdir(root)

	require.Equal(t, 2, run())
}

// TestRunFailsOnUnwritableFixture proves a WriteFile failure exits 2 — the
// fixture directory exists but one target filename is already a directory, so
// the write fails deterministically (EISDIR) regardless of the running user.
func TestRunFailsOnUnwritableFixture(t *testing.T) {
	root := fakeRepo(t)
	fixturesDir := filepath.Join(root, contractfixtures.FixturesDir)
	require.NoError(t, os.MkdirAll(fixturesDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(fixturesDir, contractfixtures.Pinned[0].Filename), 0o755))
	t.Chdir(root)

	require.Equal(t, 2, run())
}
