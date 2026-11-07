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

const good = `# Changelog

## [Unreleased]

> **Upgrade**: nothing to do.

### Added

- **A thing** ([#12](https://github.com/o/r/issues/12)): it works.
  Second line.

### Internal

- **One**: x.

## [1.0.0] - 2026-01-01

### Whatever

- old entries are not checked (#99).
`

func lint(t *testing.T, doc string) []string {
	t.Helper()
	return check(doc)
}

func TestCheckClean(t *testing.T) {
	assert.Empty(t, lint(t, good))
}

func TestCheckUnknownSection(t *testing.T) {
	got := lint(t, "## [Unreleased]\n\n### Stuff\n\n- **A**: b.\n")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], `unknown section "Stuff"`)
}

func TestCheckBareIssueReference(t *testing.T) {
	got := lint(t, "## [Unreleased]\n\n### Fixed\n\n- **A** (#12): b.\n")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "full URL")
}

func TestCheckBareLinkTextOK(t *testing.T) {
	assert.Empty(t, lint(t, "## [Unreleased]\n\n### Fixed\n\n- **A** ([#12](https://x/issues/12)): b.\n"))
}

func TestCheckTooLong(t *testing.T) {
	got := lint(t, "## [Unreleased]\n\n### Fixed\n\n- **A**: "+strings.Repeat("word ", 61)+"\n")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "words")
}

func TestCheckBulletMustStartWithBoldLead(t *testing.T) {
	got := lint(t, "## [Unreleased]\n\n### Fixed\n\n- plain text.\n")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "bold lead")
}

func TestCheckInternalLimit(t *testing.T) {
	doc := "## [Unreleased]\n\n### Internal\n\n" + strings.Repeat("- **X**: y.\n", 6)
	got := lint(t, doc)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "Internal")
}

func TestCheckSectionOrder(t *testing.T) {
	got := lint(t, "## [Unreleased]\n\n### Fixed\n\n- **A**: b.\n\n### Added\n\n- **C**: d.\n\n### Fixed\n\n- **E**: f.\n")
	require.Len(t, got, 2)
	assert.Contains(t, got[0], `"Added" is out of order`)
	assert.Contains(t, got[1], `"Fixed" is out of order or repeated`)
}

func TestCheckSubBullet(t *testing.T) {
	got := lint(t, "## [Unreleased]\n\n### Fixed\n\n- **A**: b.\n  - nested.\n")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "sub-bullets")
}

func TestCheckMissingUnreleased(t *testing.T) {
	got := lint(t, "# Changelog\n")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "[Unreleased]")
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	require.NoError(t, os.WriteFile(path, []byte(good), 0o600))
	var out, errb bytes.Buffer
	assert.Equal(t, 0, run([]string{"-file", path}, &out, &errb))

	require.NoError(t, os.WriteFile(path, []byte("## [Unreleased]\n\n### Nope\n"), 0o600))
	assert.Equal(t, 1, run([]string{"-file", path}, &out, &errb))
	assert.Contains(t, out.String(), "unknown section")

	assert.Equal(t, 2, run([]string{"-file", filepath.Join(dir, "none")}, &out, &errb))
	assert.Equal(t, 2, run([]string{"-bogus"}, &out, &errb))
}
