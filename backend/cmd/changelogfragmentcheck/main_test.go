package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFragment(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestRunCleanDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFragment(t, dir, "README.md", "# How to add a fragment\n")
	writeFragment(t, dir, "fixed-2917.md", "- **A fix** ([#2917](https://github.com/o/r/issues/2917)): it works.\n")
	writeFragment(t, dir, "added-12-2.md", "- **A thing** ([#12](https://github.com/o/r/issues/12)): it works.\n")
	writeFragment(t, dir, "notes.txt", "not a fragment\n")

	var out, errb bytes.Buffer
	assert.Equal(t, 0, run([]string{"-dir", dir}, &out, &errb))
	assert.Empty(t, out.String())
	assert.Empty(t, errb.String())
}

func TestRunReportsMalformedFragment(t *testing.T) {
	dir := t.TempDir()
	path := writeFragment(t, dir, "fixed-12.md", "- **A fix** (#12): it works.\n")

	var out, errb bytes.Buffer
	assert.Equal(t, 1, run([]string{"-dir", dir}, &out, &errb))
	assert.Contains(t, out.String(), path+":1:")
	assert.Contains(t, out.String(), "full URL")
	assert.Empty(t, errb.String())
}

func TestRunReportsBadFilename(t *testing.T) {
	dir := t.TempDir()
	writeFragment(t, dir, "fixed.md", "- **A fix** ([#12](https://x/issues/12)): it works.\n")

	var out, errb bytes.Buffer
	assert.Equal(t, 1, run([]string{"-dir", dir}, &out, &errb))
	assert.Contains(t, out.String(), "must be named <section>-<issue>[-<n>].md")
}

func TestRunReportsWrongIssueLink(t *testing.T) {
	dir := t.TempDir()
	writeFragment(t, dir, "fixed-12.md", "- **A fix** ([#13](https://x/issues/13)): it works.\n")

	var out, errb bytes.Buffer
	assert.Equal(t, 1, run([]string{"-dir", dir}, &out, &errb))
	assert.Contains(t, out.String(), "must link issue #12")
}

func TestRunReportsMultipleFindings(t *testing.T) {
	dir := t.TempDir()
	writeFragment(t, dir, "fixed-12.md", "not a bullet\n")
	writeFragment(t, dir, "security-9.md", "- **A fix** ([#9](https://x/issues/9)): "+strings.Repeat("word ", 70)+"\n")

	var out, errb bytes.Buffer
	assert.Equal(t, 1, run([]string{"-dir", dir}, &out, &errb))
	assert.Equal(t, 3, strings.Count(out.String(), "\n"))
	assert.Contains(t, out.String(), "bold lead")
	assert.Contains(t, out.String(), "words")
}

func TestRunMissingDirectory(t *testing.T) {
	var out, errb bytes.Buffer
	assert.Equal(t, 2, run([]string{"-dir", filepath.Join(t.TempDir(), "absent")}, &out, &errb))
	assert.Contains(t, errb.String(), "changelogfragmentcheck:")
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	assert.Equal(t, 2, run([]string{"-bogus"}, &out, &errb))
}

func TestMain(t *testing.T) {
	oldArgs, oldExit := os.Args, osExit
	t.Cleanup(func() {
		os.Args, osExit = oldArgs, oldExit
	})
	os.Args = []string{"changelogfragmentcheck", "-dir", t.TempDir()}
	code := -1
	osExit = func(c int) { code = c }
	main()
	assert.Equal(t, 0, code)
}
