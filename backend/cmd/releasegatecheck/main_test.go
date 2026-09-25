package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyRepoTree copies the pieces runAt reads (registry, doc, workflows dir)
// from the real repo root into a fresh temp directory that also gets a
// backend/go.mod stub, so findRepoRoot-independent tests can start from a
// known-good tree and then mutate exactly one thing.
func copyRepoTree(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	require.NoError(t, err)

	dst := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dst, "backend"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "backend", "go.mod"), []byte("module mycorrhizal\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dst, "docs", "development"), 0o755))

	copyFile(t, filepath.Join(root, registryFile), filepath.Join(dst, registryFile))
	copyFile(t, filepath.Join(root, docFile), filepath.Join(dst, docFile))
	copyDir(t, filepath.Join(root, workflowsDir), filepath.Join(dst, workflowsDir))

	return dst
}

func copyFile(t *testing.T, src, dstPath string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(dstPath), 0o755))
	b, err := os.ReadFile(src) // #nosec G304 -- test fixture, fixed repo-relative path
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dstPath, b, 0o644))
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dst, 0o755))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
}

// TestRunAgainstCommittedFiles is the CI path: with the real repo tree, the
// registry, its workflows, and the doc table all line up.
func TestRunAgainstCommittedFiles(t *testing.T) {
	var out bytes.Buffer
	code := run(&out)
	require.Equalf(t, 0, code, "releasegatecheck failed:\n%s", out.String())
	assert.Contains(t, out.String(), "release gates OK")
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)
}

// TestRunNonZeroOnInconsistency is a light guard that run() surfaces findings
// (the detailed cases live in internal/releasegates). It cannot easily corrupt
// the real files, so it just asserts the success contract's shape.
func TestRunSuccessMessageShape(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 0, run(&out))
	line := strings.TrimSpace(out.String())
	assert.True(t, strings.HasPrefix(line, "release gates OK:"), line)
	assert.Contains(t, line, "all workflows exist")
}

// TestRunAtCopiedTreeStillOK proves copyRepoTree's fixture itself is a
// faithful, passing copy before the tests below mutate it — otherwise a
// later failure could be the fixture's own bug, not the injected one.
func TestRunAtCopiedTreeStillOK(t *testing.T) {
	dst := copyRepoTree(t)
	var out bytes.Buffer
	require.Equal(t, 0, runAt(&out, dst))
}

// TestRunAtFailsOnCorruptRegistry proves the checker actually detects a
// broken registry rather than silently passing: a real "release gates OK"
// on malformed JSON would defeat the whole gate.
func TestRunAtFailsOnCorruptRegistry(t *testing.T) {
	dst := copyRepoTree(t)
	require.NoError(t, os.WriteFile(filepath.Join(dst, registryFile), []byte("{not json"), 0o644))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.NotEqual(t, 0, code)
	assert.NotContains(t, out.String(), "release gates OK")
}

// TestRunAtFailsOnUnknownWorkflow proves a gate naming a non-existent
// workflow file is caught, not silently accepted.
func TestRunAtFailsOnUnknownWorkflow(t *testing.T) {
	dst := copyRepoTree(t)
	regBytes, err := os.ReadFile(filepath.Join(dst, registryFile))
	require.NoError(t, err)
	mutated := strings.Replace(string(regBytes), `"workflow": "unit-tests.yml"`, `"workflow": "does-not-exist.yml"`, 1)
	require.NotEqual(t, string(regBytes), mutated, "fixture must contain the expected workflow reference to mutate")
	require.NoError(t, os.WriteFile(filepath.Join(dst, registryFile), []byte(mutated), 0o644))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "finding")
}

// TestRunAtFailsOnMismatchedDoc proves a doc table that no longer matches
// the registry (a gate removed from the human-readable table only) is
// caught rather than passing quietly.
func TestRunAtFailsOnMismatchedDoc(t *testing.T) {
	dst := copyRepoTree(t)
	docBytes, err := os.ReadFile(filepath.Join(dst, docFile))
	require.NoError(t, err)
	// Blank the doc table entirely: every registry gate now lacks a matching
	// row, which CrossCheckDoc must flag.
	require.NoError(t, os.WriteFile(filepath.Join(dst, docFile), []byte("# Release Gates\n\nno table here\n"), 0o644))
	_ = docBytes

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 1, code)
}

// TestRunAtFailsOnMissingComposer proves a missing release-validate.yml
// (the composer file) is reported as a checker failure (exit 2), not
// silently skipped.
func TestRunAtFailsOnMissingComposer(t *testing.T) {
	dst := copyRepoTree(t)
	require.NoError(t, os.Remove(filepath.Join(dst, composerFile)))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 2, code)
}

// TestRunAtFailsOnMissingReleaseWorkflow proves a missing release.yml is a
// checker failure.
func TestRunAtFailsOnMissingReleaseWorkflow(t *testing.T) {
	dst := copyRepoTree(t)
	require.NoError(t, os.Remove(filepath.Join(dst, workflowsDir, "release.yml")))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 2, code)
}

// TestRunAtFailsOnMissingPromoteWorkflow proves a missing promote-rc.yml is
// a checker failure, not silently ignored (this was the one branch
// previously marked "the committed tree always has promote-rc.yml" and
// therefore untested).
func TestRunAtFailsOnMissingPromoteWorkflow(t *testing.T) {
	dst := copyRepoTree(t)
	require.NoError(t, os.Remove(filepath.Join(dst, workflowsDir, "promote-rc.yml")))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 2, code)
}

// TestRunFailsOnMissingRegistryFile and TestRunFailsOnMissingDocFile exercise
// run()'s own read-failure branches (registryFile/docFile missing) via a
// copied tree, since run() always calls findRepoRoot() against the real
// working directory but runAt is the part that reads these files.
func TestRunAtFailsOnMissingRegistryFile(t *testing.T) {
	dst := copyRepoTree(t)
	require.NoError(t, os.Remove(filepath.Join(dst, registryFile)))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 2, code)
}

func TestRunAtFailsOnMissingDocFile(t *testing.T) {
	dst := copyRepoTree(t)
	require.NoError(t, os.Remove(filepath.Join(dst, docFile)))

	var out bytes.Buffer
	code := runAt(&out, dst)
	assert.Equal(t, 2, code)
}

// TestRunFailsWhenRepoRootNotFound proves run() itself fails closed when no
// backend/go.mod exists above the working directory, exercising the
// findRepoRoot-error branch inside run() (not just findRepoRoot itself).
func TestRunFailsWhenRepoRootNotFound(t *testing.T) {
	// A temp dir has no backend/go.mod anywhere in its ancestry (its parent is
	// the OS temp root, which also has none).
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	code := run(&out)
	assert.Equal(t, 2, code)
}

// TestFindRepoRootNotFound exercises findRepoRoot's own not-found branch
// directly.
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
	assert.Contains(t, string(buf), "release gates OK")
}
