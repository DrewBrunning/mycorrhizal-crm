package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sample = `# Changelog

## [Unreleased]

### Added

- **Next thing**: soon.

## [1.6.0] - 2026-11-06

> **Upgrade**: migrations run on start.

### Fixed

- **A fix**: done.

## [1.5.1] - 2026-10-27

### Fixed

- **Older fix**: done.

## [1.5.0] - 2026-10-24

### Added
`

func TestSection(t *testing.T) {
	got, err := section(sample, "1.6.0")
	require.NoError(t, err)
	assert.Equal(t, "> **Upgrade**: migrations run on start.\n\n### Fixed\n\n- **A fix**: done.", got)
}

func TestSectionUnreleased(t *testing.T) {
	got, err := section(sample, "Unreleased")
	require.NoError(t, err)
	assert.Equal(t, "### Added\n\n- **Next thing**: soon.", got)
}

func TestSectionMissing(t *testing.T) {
	_, err := section(sample, "9.9.9")
	assert.ErrorContains(t, err, "no section")
}

func TestSectionDoesNotMatchPrefix(t *testing.T) {
	_, err := section(sample, "1.6")
	assert.Error(t, err)
}

func TestSectionEmpty(t *testing.T) {
	// Only sub-headings and blank lines: nothing to publish.
	_, err := section(sample, "1.5.0")
	assert.ErrorContains(t, err, "empty")
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	require.NoError(t, os.WriteFile(path, []byte(sample), 0o600))

	var out, errb bytes.Buffer
	require.Equal(t, 0, run([]string{"-file", path, "-section", "1.6.0"}, &out, &errb))
	assert.Contains(t, out.String(), "**A fix**")

	assert.Equal(t, 2, run([]string{"-file", path, "-section", "9.9.9"}, &out, &errb))
	assert.Equal(t, 2, run([]string{"-file", filepath.Join(dir, "none.md"), "-section", "1.6.0"}, &out, &errb))
	assert.Equal(t, 2, run([]string{"-file", path}, &out, &errb))
	assert.Equal(t, 2, run([]string{"-bogus"}, &out, &errb))
}

// A release candidate prints [Unreleased] until the maintainer renames it,
// then the renamed section; an empty renamed section is still an error.
func TestRunFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	require.NoError(t, os.WriteFile(path, []byte(sample), 0o600))

	var out, errb bytes.Buffer
	require.Equal(t, 0, run([]string{"-file", path, "-section", "1.7.0", "-fallback", "Unreleased"}, &out, &errb))
	assert.Contains(t, out.String(), "**Next thing**")

	out.Reset()
	require.Equal(t, 0, run([]string{"-file", path, "-section", "1.6.0", "-fallback", "Unreleased"}, &out, &errb))
	assert.Contains(t, out.String(), "**A fix**")
	assert.NotContains(t, out.String(), "**Next thing**")

	assert.Equal(t, 2, run([]string{"-file", path, "-section", "1.5.0", "-fallback", "Unreleased"}, &out, &errb))
}

func TestRunLinkBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	doc := "## [1.0.0]\n\n> See [Backups](docs/deployment.md#backups), [x](/docs/x.md), [top](#100) and [#7](https://github.com/o/r/issues/7).\n"
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

	var out, errb bytes.Buffer
	require.Equal(t, 0, run([]string{"-file", path, "-section", "1.0.0", "-link-base", "https://github.com/o/r/blob/v1.0.0/"}, &out, &errb))
	assert.Equal(t, "> See [Backups](https://github.com/o/r/blob/v1.0.0/docs/deployment.md#backups), "+
		"[x](https://github.com/o/r/blob/v1.0.0/docs/x.md), "+
		"[top](https://github.com/o/r/blob/v1.0.0/CHANGELOG.md#100) and "+
		"[#7](https://github.com/o/r/issues/7).\n", out.String())
}
