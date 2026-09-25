package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/contractfixtures"
	"mycorrhizal/internal/tsapi"

	"github.com/stretchr/testify/require"
)

// fakeRepo builds a temporary repository root with a copy of the real spec.
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

func TestRunWritesGeneratedFile(t *testing.T) {
	root := fakeRepo(t)
	t.Chdir(filepath.Join(root, "backend"))

	require.Equal(t, 0, run())

	body, err := os.ReadFile(filepath.Join(root, tsapi.OutputPath))
	require.NoError(t, err)
	require.Contains(t, string(body), "DO NOT EDIT")
}

func TestRunFailsOnBrokenSpec(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", contractfixtures.SpecPath),
		[]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: \"1.0\"\npaths:\n  /x:\n    get:\n      responses: {}\n"), 0o644))
	t.Chdir(root)
	require.Equal(t, 1, run())
}

// A valid spec with no components.schemas cannot be mapped: exit 1.
func TestRunFailsOnUngeneratableSpec(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", contractfixtures.SpecPath),
		[]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: \"1.0\"\npaths: {}\n"), 0o644))
	t.Chdir(root)
	require.Equal(t, 1, run())
}

func TestRunFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	require.Equal(t, 2, run())
}

// frontend/ is a file, so the output directory cannot be created: exit 2.
func TestRunFailsOnUnwritableDir(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "frontend"), []byte("x"), 0o644))
	t.Chdir(root)
	require.Equal(t, 2, run())
}

// The output path is a directory, so the write fails (EISDIR): exit 2.
func TestRunFailsOnUnwritableFile(t *testing.T) {
	root := fakeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, tsapi.OutputPath), 0o755))
	t.Chdir(root)
	require.Equal(t, 2, run())
}
