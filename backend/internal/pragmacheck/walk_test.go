package pragmacheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// TestCheckTree_FindsBareMarkerInBackendTree proves the walker actually
// surfaces a broken input from a real directory tree, not just from the
// in-memory CheckGoSource unit tests above.
func TestCheckTree_FindsBareMarkerInBackendTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "pkg", "x.go"), "package pkg\n\nfunc f() {\n\treturn // # pragma: no cover\n}\n")

	findings, err := CheckTree(filepath.Join(root, "backend"), "")
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, 4, findings[0].Line)
}

// TestCheckTree_IgnoresTestFiles proves _test.go files are out of scope, so
// a bare marker used deliberately in test fixtures/helpers does not fail
// the gate.
func TestCheckTree_IgnoresTestFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "pkg", "x_test.go"), "package pkg\n\nfunc f() {\n\treturn // # pragma: no cover\n}\n")

	findings, err := CheckTree(filepath.Join(root, "backend"), "")
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestCheckTree_ScansFrontendSrc proves the frontend leg of the walker is
// wired up and reports a bare v8-ignore marker.
func TestCheckTree_ScansFrontendSrc(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "frontend", "src", "x.ts"), "export function f() {\n  /* v8 ignore next */\n  return 1;\n}\n")

	findings, err := CheckTree("", filepath.Join(root, "frontend", "src"))
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, 2, findings[0].Line)
}

// TestCheckTree_MissingFrontendSrcIsNotAnError proves a repo checkout with
// no frontend/src (or one not yet scanned) does not fail the check.
func TestCheckTree_MissingFrontendSrcIsNotAnError(t *testing.T) {
	root := t.TempDir()
	findings, err := CheckTree("", filepath.Join(root, "does-not-exist"))
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestCheckTree_SortsByPathThenLine pins the deterministic ordering the CLI
// output relies on.
func TestCheckTree_SortsByPathThenLine(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "b.go"), "package b\n\nfunc g() {\n\treturn // # pragma: no cover\n}\n")
	writeFile(t, filepath.Join(root, "backend", "a.go"), "package a\n\nfunc f() {\n\treturn // # pragma: no cover\n}\n")

	findings, err := CheckTree(filepath.Join(root, "backend"), "")
	require.NoError(t, err)
	require.Len(t, findings, 2)
	assert.Contains(t, findings[0].Path, "a.go")
	assert.Contains(t, findings[1].Path, "b.go")
}

// TestCheckTree_NoDirsIsClean proves calling with both roots empty is a
// no-op, not an error.
func TestCheckTree_NoDirsIsClean(t *testing.T) {
	findings, err := CheckTree("", "")
	require.NoError(t, err)
	assert.Empty(t, findings)
}
