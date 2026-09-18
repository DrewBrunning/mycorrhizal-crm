package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/internal/adversarialdelta"
)

const ledgerBody = `# deltas

| Release | Date | Surface | Coverage |
|---|---|---|---|
| v0.8.9 | 2026-09-20 | route | authorization matrix. |
`

// fixtureRoot lays down a repo root with the sentinel and the ledger.
func fixtureRoot(t *testing.T, ledger string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"docs/security/threat-model.md": ledger,
		adversarialdelta.LedgerFile:     ledger,
	} {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

func TestRunSurfaceCovered(t *testing.T) {
	root := fixtureRoot(t, ledgerBody)
	var buf bytes.Buffer

	code := run(&buf, root, adversarialdelta.LedgerFile, "v0.8.9", "", []string{"backend/routes/routes.go"})
	require.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "records the new surface (route)")
}

func TestRunNoSurface(t *testing.T) {
	root := fixtureRoot(t, ledgerBody)
	var buf bytes.Buffer

	code := run(&buf, root, adversarialdelta.LedgerFile, "v0.9.0", "", []string{"docs/readme.md"})
	require.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "no recognised security-relevant surface")
}

func TestRunMissingDeltaFails(t *testing.T) {
	root := fixtureRoot(t, ledgerBody)
	var buf bytes.Buffer

	code := run(&buf, root, adversarialdelta.LedgerFile, "v0.9.0", "", []string{"backend/routes/routes.go"})
	require.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "::error::")
}

func TestRunAckProceeds(t *testing.T) {
	root := fixtureRoot(t, ledgerBody)
	var buf bytes.Buffer

	code := run(&buf, root, adversarialdelta.LedgerFile, "v0.9.0", "reviewed by hand in #123", []string{"backend/routes/routes.go"})
	require.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "::warning::")
	assert.Contains(t, buf.String(), "reviewed by hand in #123")
}

func TestRunBadLedgerFails(t *testing.T) {
	root := fixtureRoot(t, ledgerBody)
	var buf bytes.Buffer

	code := run(&buf, root, "../escape", "v0.8.9", "", nil)
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "outside the repository root")
}

func TestMainExitRejectsUnknownFlag(t *testing.T) {
	var buf bytes.Buffer
	require.Equal(t, 2, mainExit([]string{"-nope"}, strings.NewReader(""), &buf))
}

func TestMainExitReadPathsError(t *testing.T) {
	var buf bytes.Buffer
	code := mainExit([]string{"-release", "v0.9.0"}, errReader{}, &buf)
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "read changed paths")
}

func TestMainExitNoRepoRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	var buf bytes.Buffer
	code := mainExit([]string{"-release", "v0.9.0"}, strings.NewReader("docs/readme.md\n"), &buf)
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "no repository root")
}

func TestReadPaths(t *testing.T) {
	got, err := readPaths(strings.NewReader("a.go\n\n  b.go  \n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"a.go", "b.go"}, got)
}

// errReader always fails, to exercise readPaths' scanner-error branch.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestReadPathsError(t *testing.T) {
	_, err := readPaths(errReader{})
	require.Error(t, err)
}

func TestMainExitRequiresRelease(t *testing.T) {
	var buf bytes.Buffer
	code := mainExit(nil, strings.NewReader(""), &buf)
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "-release is required")
}

func TestMainExitEndToEnd(t *testing.T) {
	// findRepoRoot walks up from the test's cwd (backend/cmd/adversarialdelta),
	// which is the real repo, and reads the real ledger. No recognised surface
	// class is touched, so the gate passes without a row.
	code := mainExit([]string{"-release", "v0.9.0", "-ledger", adversarialdelta.LedgerFile},
		strings.NewReader("docs/readme.md\n"), &bytes.Buffer{})
	require.Equal(t, 0, code, "real repo root should be found; ledger path %q", adversarialdelta.LedgerFile)
}

func TestFindRepoRoot(t *testing.T) {
	root := fixtureRoot(t, ledgerBody)
	sub := filepath.Join(root, "backend", "cmd")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	got, err := findRepoRoot(sub)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestFindRepoRootNone(t *testing.T) {
	_, err := findRepoRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no repository root")
}

func TestMustGetwd(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, wd, mustGetwd())
}
