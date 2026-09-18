package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/internal/residualrisk"
)

// fixtureRepo creates a minimal tree holding every source Summarize reads.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".trivyignore":                               "# comment\nDS-0002\n",
		".grype.yml":                                 "ignore: []\n",
		"zap/dast.ignore":                            "# comment\n",
		"schemathesis/schemathesis.ignore":           "# comment\n",
		"docker/cis-hardening.ignore":                "# comment\n",
		"android/.mobsf":                             "ignore-rules:\n  - one\n",
		"docs/security/citation-drift.ignore":        "# comment\n",
		"docs/security/crypto-surface.ignore":        "# comment\n",
		"docs/security/governance-drift.ignore":      "# comment\n",
		"docs/security/dependency-exceptions.ignore": "# comment\n",
		residualrisk.ReportFile:                      "> **ASVS Level 2, with 23 documented exceptions.** **MASVS-L1, with 1 documented exception**\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

func TestRunWritesJSONToStdout(t *testing.T) {
	root := fixtureRepo(t)
	var buf bytes.Buffer

	code := run(&buf, root, "", mustTime(t, "2026-09-18"))
	require.Equal(t, 0, code)

	var got residualrisk.Summary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, 2, got.AcceptItems.Total)
	assert.Equal(t, 23, got.Exceptions.ASVS)
}

func TestRunWritesJSONToFile(t *testing.T) {
	root := fixtureRepo(t)
	out := filepath.Join(t.TempDir(), "residual.json")
	var buf bytes.Buffer

	code := run(&buf, root, out, mustTime(t, "2026-09-18"))
	require.Equal(t, 0, code)

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(written), "\n"))
	var got residualrisk.Summary
	require.NoError(t, json.Unmarshal(written, &got))
	assert.Equal(t, 23, got.Exceptions.ASVS)
}

func TestRunBadFileFails(t *testing.T) {
	var buf bytes.Buffer
	code := run(&buf, t.TempDir(), "", mustTime(t, "2026-09-18"))
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "residualrisk:")
}

func TestRunUnwritableOutputFails(t *testing.T) {
	root := fixtureRepo(t)
	var buf bytes.Buffer
	code := run(&buf, root, filepath.Join(root, "no-such-dir", "out.json"), mustTime(t, "2026-09-18"))
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "residualrisk: write:")
}

func TestMainExitRejectsBadNow(t *testing.T) {
	var buf bytes.Buffer
	code := mainExit([]string{"-now", "not-a-date"}, &buf, mustTime(t, "2026-09-18"))
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "-now must be YYYY-MM-DD")
}

func TestMainExitEndToEnd(t *testing.T) {
	// findRepoRoot walks up from the test's cwd (backend/cmd/residualrisk),
	// which is the real repo, and summarizes the real sources.
	var buf bytes.Buffer
	code := mainExit([]string{"-now", "2026-09-18"}, &buf, mustTime(t, "2026-09-18"))
	require.Equal(t, 0, code)
	var got residualrisk.Summary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Greater(t, got.Exceptions.ASVS, 0)
}

func TestMainExitNoRepoRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	var buf bytes.Buffer
	code := mainExit(nil, &buf, mustTime(t, "2026-09-18"))
	require.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "no repository root")
}

func TestMainExitRejectsUnknownFlag(t *testing.T) {
	var buf bytes.Buffer
	require.Equal(t, 2, mainExit([]string{"-nope"}, &buf, mustTime(t, "2026-09-18")))
}

func TestMustGetwd(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, wd, mustGetwd())
}

func TestFindRepoRoot(t *testing.T) {
	root := fixtureRepo(t)
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

func mustTime(t *testing.T, date string) (out time.Time) {
	t.Helper()
	d, err := time.Parse("2006-01-02", date)
	require.NoError(t, err)
	return d
}
