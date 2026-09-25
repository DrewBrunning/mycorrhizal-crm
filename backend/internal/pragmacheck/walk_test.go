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

// TestCheckTree_MissingBackendDirIsNotAnError is the backend-leg mirror of
// the frontend case above -- checkGoTree's own IsNotExist branch.
func TestCheckTree_MissingBackendDirIsNotAnError(t *testing.T) {
	root := t.TempDir()
	findings, err := CheckTree(filepath.Join(root, "does-not-exist"), "")
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestCheckTree_SkipsUnsupportedFrontendExtensions proves checkTSTree's
// extension filter actually skips a non-.ts/.tsx/.js/.jsx file rather than
// scanning it (e.g. a .css or .json file sitting alongside real sources).
func TestCheckTree_SkipsUnsupportedFrontendExtensions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "frontend", "src", "styles.css"), "/* v8 ignore next */\nbody { color: red; }\n")

	findings, err := CheckTree("", filepath.Join(root, "frontend", "src"))
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

// TestCheckTree_SortsSamePathByLine exercises the comparator's second
// branch (findings[i].Line < findings[j].Line) -- TestCheckTree_SortsByPathThenLine
// above only ever compares findings from two different files.
func TestCheckTree_SortsSamePathByLine(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "x.go"),
		"package x\n\nfunc f() {\n\treturn // # pragma: no cover\n}\n\nfunc g() {\n\treturn // # pragma: no cover\n}\n")

	findings, err := CheckTree(filepath.Join(root, "backend"), "")
	require.NoError(t, err)
	require.Len(t, findings, 2)
	assert.Equal(t, findings[0].Path, findings[1].Path)
	assert.Less(t, findings[0].Line, findings[1].Line)
}

// TestCheckTree_BackendUnreadableDirPropagatesError and its frontend/ReadFile
// siblings below exercise checkGoTree/checkTSTree's non-IsNotExist Walk
// error branches -- a permission error from a subdirectory or file the
// walker cannot skip the way it skips a merely-missing path.
func TestCheckTree_BackendUnreadableDirPropagatesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "backend", "blocked")
	writeFile(t, filepath.Join(blocked, "x.go"), "package blocked\n")
	require.NoError(t, os.Chmod(blocked, 0o000))
	defer os.Chmod(blocked, 0o755) //nolint:errcheck // best-effort cleanup so t.TempDir can remove it

	_, err := CheckTree(filepath.Join(root, "backend"), "")
	assert.Error(t, err)
}

func TestCheckTree_BackendUnreadableFilePropagatesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	root := t.TempDir()
	path := filepath.Join(root, "backend", "x.go")
	writeFile(t, path, "package x\n")
	require.NoError(t, os.Chmod(path, 0o000))
	defer os.Chmod(path, 0o644) //nolint:errcheck // best-effort cleanup

	_, err := CheckTree(filepath.Join(root, "backend"), "")
	assert.Error(t, err)
}

func TestCheckTree_FrontendUnreadableDirPropagatesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "frontend", "src", "blocked")
	writeFile(t, filepath.Join(blocked, "x.ts"), "export const x = 1;\n")
	require.NoError(t, os.Chmod(blocked, 0o000))
	defer os.Chmod(blocked, 0o755) //nolint:errcheck // best-effort cleanup

	_, err := CheckTree("", filepath.Join(root, "frontend", "src"))
	assert.Error(t, err)
}

func TestCheckTree_FrontendUnreadableFilePropagatesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	root := t.TempDir()
	path := filepath.Join(root, "frontend", "src", "x.ts")
	writeFile(t, path, "export const x = 1;\n")
	require.NoError(t, os.Chmod(path, 0o000))
	defer os.Chmod(path, 0o644) //nolint:errcheck // best-effort cleanup

	_, err := CheckTree("", filepath.Join(root, "frontend", "src"))
	assert.Error(t, err)
}
