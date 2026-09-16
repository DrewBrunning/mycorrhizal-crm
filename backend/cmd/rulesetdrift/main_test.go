package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestRunRequiresResultsFlag(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 2, run(&out, nil))
	assert.Contains(t, out.String(), "-results is required")
}

func TestRunMissingResultsFileExits2(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 2, run(&out, []string{"-results", filepath.Join(t.TempDir(), "nope.json")}))
	assert.Contains(t, out.String(), "rulesetdrift:")
}

func TestRunMalformedResultsJSONExits2(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, "not json")
	var out bytes.Buffer
	assert.Equal(t, 2, run(&out, []string{"-results", results}))
	assert.Contains(t, out.String(), "does not parse as JSON")
}

// TestRunPassesWhenEverythingMatches covers the all-clear path, including a
// missing ignore file (which must be treated as zero entries, not an error).
func TestRunPassesWhenEverythingMatches(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[{"name":"main-protection","applied":true,"drifted":false}]`)
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", filepath.Join(dir, "no-such-ignore-file")})
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "governance drift: all 1 ruleset(s) match")
}

// TestRunFailsOnUnignoredDrift is the #916 regression test at the CLI
// boundary: a drifted ruleset with no governance-drift.ignore entry must
// produce a non-zero exit code, not the pre-fix always-0 behavior.
func TestRunFailsOnUnignoredDrift(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[{"name":"main-protection","applied":true,"drifted":true}]`)
	ignore := filepath.Join(dir, "governance-drift.ignore")
	writeFile(t, ignore, "# no entries\n")
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", ignore})
	assert.Equal(t, 1, code)
	s := out.String()
	assert.Contains(t, s, "main-protection")
	assert.Contains(t, s, "1 governance-drift finding(s)")
}

func TestRunFailsOnUnappliedRuleset(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[{"name":"tags-v","applied":false,"drifted":false}]`)
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", filepath.Join(dir, "missing")})
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "not applied to the repository")
}

func TestRunAcceptsIgnoredDrift(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[{"name":"main-protection","applied":true,"drifted":true}]`)
	ignore := filepath.Join(dir, "governance-drift.ignore")
	writeFile(t, ignore, "main-protection  # rotation in progress, see #916\n")
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", ignore})
	assert.Equal(t, 0, code)
	s := out.String()
	assert.Contains(t, s, "accepted:")
	assert.Contains(t, s, "rotation in progress")
}

func TestRunFailsOnStaleIgnoreEntry(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[{"name":"main-protection","applied":true,"drifted":false}]`)
	ignore := filepath.Join(dir, "governance-drift.ignore")
	writeFile(t, ignore, "main-protection  # stale\n")
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", ignore})
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "back in sync")
}

func TestRunFailsOnMalformedIgnoreLine(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[{"name":"main-protection","applied":true,"drifted":false}]`)
	ignore := filepath.Join(dir, "governance-drift.ignore")
	writeFile(t, ignore, "this line has no hash reason\n")
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", ignore})
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "expected `<ruleset name>  # <reason>`")
}

func TestRunNoRulesetsPasses(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results.json")
	writeFile(t, results, `[]`)
	var out bytes.Buffer
	code := run(&out, []string{"-results", results, "-ignore", filepath.Join(dir, "missing")})
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "all 0 ruleset(s) match")
}
