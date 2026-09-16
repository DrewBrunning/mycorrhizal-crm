package mutationscope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakePackage lays out a minimal fake package directory (source files,
// a test file, and a subdirectory that must be ignored) for generateOne unit
// tests that don't need the real backend tree.
func writeFakePackage(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	for _, name := range []string{"a.go", "b.go", "c.go", "c_test.go"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("package fake\n"), 0o600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "d.go"), []byte("package sub\n"), 0o600))
}

func TestExcludedFilesNarrowsToTargets(t *testing.T) {
	root := t.TempDir()
	writeFakePackage(t, filepath.Join(root, "fakepkg"))

	got, err := excludedFiles(root, Scope{PackageDir: "fakepkg", TargetFiles: []string{"b.go"}})
	require.NoError(t, err)
	// a.go and c.go are excluded; b.go (target) and c_test.go (not .go
	// source, always skipped) are not; sub/d.go is in a subdirectory and
	// never considered.
	assert.Equal(t, []string{"a.go", "c.go"}, got)
}

func TestExcludedFilesErrorsOnMissingTargetFile(t *testing.T) {
	root := t.TempDir()
	writeFakePackage(t, filepath.Join(root, "fakepkg"))

	_, err := excludedFiles(root, Scope{PackageDir: "fakepkg", TargetFiles: []string{"does-not-exist.go"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist.go")
}

func TestGenerateOneWholePackageHasNoExcludes(t *testing.T) {
	root := t.TempDir()
	writeFakePackage(t, filepath.Join(root, "fakepkg"))

	content, err := generateOne(root, Scope{Name: "fakepkg", PackageDir: "fakepkg", Efficacy: 42, MutantCoverage: 24})
	require.NoError(t, err)
	assert.Contains(t, content, "exclude-files: []")
	assert.Contains(t, content, "efficacy: 42")
	assert.Contains(t, content, "mutant-coverage: 24")
}

func TestGenerateOnePropagatesMissingPackageDir(t *testing.T) {
	root := t.TempDir()

	_, err := generateOne(root, Scope{Name: "ghost", PackageDir: "does-not-exist", TargetFiles: []string{"x.go"}})
	require.Error(t, err)
}

func TestGenerateReturnsOneEntryPerScope(t *testing.T) {
	root := t.TempDir()
	writeFakePackage(t, filepath.Join(root, "fakepkg"))
	orig := Scopes
	t.Cleanup(func() { Scopes = orig })
	Scopes = []Scope{
		{Name: "fakepkg", PackageDir: "fakepkg", Efficacy: 50, MutantCoverage: 50, Reason: "test"},
	}

	files, err := Generate(root)
	require.NoError(t, err)
	require.Contains(t, files, filepath.Join(ConfigDir, "fakepkg.yaml"))
}

func TestGeneratePropagatesGenerateOneError(t *testing.T) {
	root := t.TempDir()
	orig := Scopes
	t.Cleanup(func() { Scopes = orig })
	Scopes = []Scope{
		{Name: "ghost", PackageDir: "does-not-exist", TargetFiles: []string{"x.go"}},
	}

	_, err := Generate(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestWritePropagatesGenerateError(t *testing.T) {
	root := t.TempDir()
	orig := Scopes
	t.Cleanup(func() { Scopes = orig })
	Scopes = []Scope{
		{Name: "ghost", PackageDir: "does-not-exist", TargetFiles: []string{"x.go"}},
	}

	require.Error(t, Write(root))
}

func TestWriteFailsWhenConfigDirCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	writeFakePackage(t, filepath.Join(root, "fakepkg"))
	orig := Scopes
	t.Cleanup(func() { Scopes = orig })
	Scopes = []Scope{
		{Name: "fakepkg", PackageDir: "fakepkg", Efficacy: 50, MutantCoverage: 50, Reason: "test"},
	}

	// ConfigDir would be created under root/.gremlins; make that path
	// component a regular file so os.MkdirAll fails with ENOTDIR.
	require.NoError(t, os.WriteFile(filepath.Join(root, ConfigDir), []byte("not a directory"), 0o600))

	require.Error(t, Write(root))
}

func TestWriteCreatesConfigDirAndFiles(t *testing.T) {
	root := t.TempDir()
	writeFakePackage(t, filepath.Join(root, "fakepkg"))
	orig := Scopes
	t.Cleanup(func() { Scopes = orig })
	Scopes = []Scope{
		{Name: "fakepkg", PackageDir: "fakepkg", Efficacy: 50, MutantCoverage: 50, Reason: "test"},
	}

	require.NoError(t, Write(root))

	got, err := os.ReadFile(filepath.Join(root, ConfigDir, "fakepkg.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "efficacy: 50")
}
