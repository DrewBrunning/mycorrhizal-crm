package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/mutationscope"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary backend module root (a go.mod stub, so
// findBackendDir resolves) with a minimal fake package directory, and
// swaps internal/mutationscope.Scopes to reference it for the duration of
// the test — so the generator is exercised without touching the real
// backend/.gremlins/ tree.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module mycorrhizal\n\ngo 1.26.0\n"), 0o600))
	pkgDir := filepath.Join(root, "fakepkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "a.go"), []byte("package fakepkg\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "b.go"), []byte("package fakepkg\n"), 0o600))

	orig := mutationscope.Scopes
	t.Cleanup(func() { mutationscope.Scopes = orig })
	mutationscope.Scopes = []mutationscope.Scope{
		{
			Name:           "fakepkg-scope",
			PackageDir:     "fakepkg",
			TargetFiles:    []string{"a.go"},
			Efficacy:       50,
			MutantCoverage: 50,
			Reason:         "test",
		},
	}
	return root
}

// TestRunRegeneratesConfigs proves the happy path: from the fake backend
// root, run() writes the generated config for every scope.
func TestRunRegeneratesConfigs(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(root)

	require.Equal(t, 0, run())

	got, err := os.ReadFile(filepath.Join(root, mutationscope.ConfigDir, "fakepkg-scope.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(got), "efficacy: 50")
	require.Contains(t, string(got), `^b\.go$`)
}

// TestRunFailsOutsideRepo proves findBackendDir's error path exits 2 when
// the working directory is not under a module root (no go.mod above it).
func TestRunFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	require.Equal(t, 2, run())
}

// TestRunFailsOnGenerateError proves a scope naming a nonexistent package
// exits 1 rather than silently writing a partial/empty config set.
func TestRunFailsOnGenerateError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module mycorrhizal\n\ngo 1.26.0\n"), 0o600))
	orig := mutationscope.Scopes
	t.Cleanup(func() { mutationscope.Scopes = orig })
	mutationscope.Scopes = []mutationscope.Scope{
		{Name: "ghost", PackageDir: "does-not-exist", TargetFiles: []string{"x.go"}},
	}
	t.Chdir(root)

	require.Equal(t, 1, run())
}
