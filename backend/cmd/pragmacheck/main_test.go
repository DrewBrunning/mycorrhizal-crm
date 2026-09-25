package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAgainstCommittedFiles is the CI path: the real repo tree has no
// unreasoned marker.
func TestRunAgainstCommittedFiles(t *testing.T) {
	var out bytes.Buffer
	code := run(&out)
	require.Equalf(t, 0, code, "pragmacheck failed:\n%s", out.String())
	assert.Contains(t, out.String(), "pragmacheck OK")
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)
}

// TestRunAtFailsOnBareMarker proves the checker actually detects a marker
// with no reason, rather than silently passing.
func TestRunAtFailsOnBareMarker(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "backend", "pkg", "x.go"),
		[]byte("package pkg\n\nfunc f() {\n\treturn // # pragma: no cover\n}\n"),
		0o644,
	))

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "no discoverable reason")
	assert.NotContains(t, out.String(), "pragmacheck OK")
}

// TestRunAtOKOnCleanTree is the positive control.
func TestRunAtOKOnCleanTree(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "backend", "pkg", "x.go"),
		[]byte("package pkg\n\nfunc f() {\n\treturn // # pragma: no cover — reason\n}\n"),
		0o644,
	))

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "pragmacheck OK")
}

// TestRunFailsWhenRepoRootNotFound proves run() itself fails closed when no
// backend/go.mod exists above the working directory.
func TestRunFailsWhenRepoRootNotFound(t *testing.T) {
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	assert.Equal(t, 2, run(&out))
}

// TestFindRepoRootNotFound exercises findRepoRoot's own not-found branch.
func TestFindRepoRootNotFound(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := findRepoRoot()
	assert.Error(t, err)
}

// TestMainExitsZero drives main() itself through the osExit seam.
func TestMainExitsZero(t *testing.T) {
	origExit := osExit
	origStdout := os.Stdout
	defer func() {
		osExit = origExit
		os.Stdout = origStdout
	}()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	var gotCode int
	exited := false
	osExit = func(code int) { gotCode = code; exited = true }

	main()

	require.NoError(t, w.Close())
	os.Stdout = origStdout

	require.True(t, exited, "main must call osExit")
	require.Equal(t, 0, gotCode)

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(buf), "pragmacheck OK")
}
