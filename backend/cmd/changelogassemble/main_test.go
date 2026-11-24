package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/internal/changelog"
)

func setup(t *testing.T, doc string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "CHANGELOG.md")
	require.NoError(t, os.WriteFile(file, []byte(doc), 0o600))
	fragments := filepath.Join(dir, "changelog.d")
	require.NoError(t, os.Mkdir(fragments, 0o755))
	return file, fragments
}

func writeFragment(t *testing.T, dir, name, bullet string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(bullet+"\n"), 0o600))
	return path
}

func assemble(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestCheckPassesWhenEveryBulletIsPresent(t *testing.T) {
	file, dir := setup(t, "## [Unreleased]\n\n### Added\n\n- **A** ([#10](https://x/issues/10)): a.\n")
	writeFragment(t, dir, "added-10.md", "- **A** ([#10](https://x/issues/10)): a.")
	writeFragment(t, dir, "README.md", "# ignored")

	code, out, errb := assemble(t, "-file", file, "-dir", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, out)
	assert.Empty(t, errb)
}

func TestCheckFailsWhenBulletIsMissing(t *testing.T) {
	file, dir := setup(t, "## [Unreleased]\n\n### Added\n\n- **A** ([#10](https://x/issues/10)): a.\n")
	fragment := writeFragment(t, dir, "fixed-11.md", "- **F** ([#11](https://x/issues/11)): f.")

	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-check")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, fragment+": bullet is missing from CHANGELOG.md [Unreleased]")
	assert.Contains(t, out, "-write")
	assert.Empty(t, errb)
}

func TestWriteInsertsIntoExistingSectionAndPreservesContent(t *testing.T) {
	doc := `# Changelog

## [Unreleased]

> **Upgrade**: run the migration.

### Added

- **One** ([#1](https://x/issues/1)): one.
- **Two** ([#2](https://x/issues/2)): two.

### Fixed

- **Old fix** ([#3](https://x/issues/3)): old.

## [1.0.0] - 2026-01-01

### Added

- **Released** ([#4](https://x/issues/4)): old.
`
	file, dir := setup(t, doc)
	writeFragment(t, dir, "added-5.md", "- **New** ([#5](https://x/issues/5)): new.")
	writeFragment(t, dir, "fixed-6.md", "- **New fix** ([#6](https://x/issues/6)): new.")

	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	require.Equal(t, 0, code, errb)
	assert.Contains(t, out, "inserted 2 bullet(s)")

	want := `# Changelog

## [Unreleased]

> **Upgrade**: run the migration.

### Added

- **One** ([#1](https://x/issues/1)): one.
- **Two** ([#2](https://x/issues/2)): two.
- **New** ([#5](https://x/issues/5)): new.

### Fixed

- **Old fix** ([#3](https://x/issues/3)): old.
- **New fix** ([#6](https://x/issues/6)): new.

## [1.0.0] - 2026-01-01

### Added

- **Released** ([#4](https://x/issues/4)): old.
`
	assert.Equal(t, want, read(t, file))
}

func TestWriteCreatesSectionsInPolicyOrder(t *testing.T) {
	doc := `# Changelog

## [Unreleased]

> **Upgrade**: nothing.

## [1.0.0] - 2026-01-01
`
	file, dir := setup(t, doc)
	writeFragment(t, dir, "internal-12.md", "- **I** ([#12](https://x/issues/12)): i.")
	writeFragment(t, dir, "fixed-11.md", "- **F** ([#11](https://x/issues/11)): f.")
	writeFragment(t, dir, "added-10.md", "- **A** ([#10](https://x/issues/10)): a.")

	code, _, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	require.Equal(t, 0, code, errb)

	want := `# Changelog

## [Unreleased]

> **Upgrade**: nothing.

### Added

- **A** ([#10](https://x/issues/10)): a.

### Fixed

- **F** ([#11](https://x/issues/11)): f.

### Internal

- **I** ([#12](https://x/issues/12)): i.

## [1.0.0] - 2026-01-01
`
	assert.Equal(t, want, read(t, file))
}

func TestWriteInsertsASectionBetweenExistingOnes(t *testing.T) {
	doc := `## [Unreleased]

### Added

- **A** ([#1](https://x/issues/1)): a.

### Fixed

- **F** ([#2](https://x/issues/2)): f.

## [1.0.0]
`
	file, dir := setup(t, doc)
	writeFragment(t, dir, "changed-3.md", "- **C** ([#3](https://x/issues/3)): c.")

	code, _, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	require.Equal(t, 0, code, errb)

	got := read(t, file)
	added, changed, fixed := strings.Index(got, "### Added"), strings.Index(got, "### Changed"), strings.Index(got, "### Fixed")
	require.NotEqual(t, -1, changed)
	assert.Less(t, added, changed)
	assert.Less(t, changed, fixed)
	assert.Contains(t, got[changed:fixed], "- **C** ([#3](https://x/issues/3)): c.")
}

func TestWriteDedupesBullets(t *testing.T) {
	file, dir := setup(t, "## [Unreleased]\n\n### Added\n\n- **Same** ([#10](https://x/issues/10)): same.\n")
	writeFragment(t, dir, "added-10.md", "- **Same** ([#10](https://x/issues/10)): same.")
	writeFragment(t, dir, "added-10-2.md", "- **Same** ([#10](https://x/issues/10)): same.")
	writeFragment(t, dir, "fixed-11.md", "- **New** ([#11](https://x/issues/11)): new.")
	writeFragment(t, dir, "fixed-11-2.md", "- **New** ([#11](https://x/issues/11)): new.")

	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	require.Equal(t, 0, code, errb)
	assert.Contains(t, out, "inserted 1 bullet(s)")

	got := read(t, file)
	assert.Equal(t, 1, strings.Count(got, "- **New** ([#11](https://x/issues/11)): new."))
	assert.Equal(t, 1, strings.Count(got, "- **Same** ([#10](https://x/issues/10)): same."))
}

func TestWriteDeletesFragmentsUnlessKept(t *testing.T) {
	doc := "## [Unreleased]\n\n### Added\n\n- **Old** ([#1](https://x/issues/1)): old.\n"

	file, dir := setup(t, doc)
	fragment := writeFragment(t, dir, "added-2.md", "- **New** ([#2](https://x/issues/2)): new.")
	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	require.Equal(t, 0, code, errb)
	assert.Contains(t, out, "deleted 1 fragment(s)")
	_, err := os.Stat(fragment)
	assert.True(t, os.IsNotExist(err))

	file, dir = setup(t, doc)
	fragment = writeFragment(t, dir, "added-2.md", "- **New** ([#2](https://x/issues/2)): new.")
	code, out, errb = assemble(t, "-file", file, "-dir", dir, "-write", "-keep")
	require.Equal(t, 0, code, errb)
	assert.NotContains(t, out, "deleted")
	_, err = os.Stat(fragment)
	assert.NoError(t, err)
}

func TestWriteIsIdempotent(t *testing.T) {
	file, dir := setup(t, "## [Unreleased]\n\n### Added\n\n- **Old** ([#1](https://x/issues/1)): old.\n")
	writeFragment(t, dir, "added-2.md", "- **New** ([#2](https://x/issues/2)): new.")

	code, _, errb := assemble(t, "-file", file, "-dir", dir, "-write", "-keep")
	require.Equal(t, 0, code, errb)
	first := read(t, file)

	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-write", "-keep")
	require.Equal(t, 0, code, errb)
	assert.NotContains(t, out, "inserted")
	assert.Equal(t, first, read(t, file))
}

func TestWriteDeletesAlreadyAssembledFragments(t *testing.T) {
	file, dir := setup(t, "## [Unreleased]\n\n### Added\n\n- **A** ([#1](https://x/issues/1)): a.\n")
	fragment := writeFragment(t, dir, "added-1.md", "- **A** ([#1](https://x/issues/1)): a.")

	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	require.Equal(t, 0, code, errb)
	assert.NotContains(t, out, "inserted")
	_, err := os.Stat(fragment)
	assert.True(t, os.IsNotExist(err))
}

func TestWritePreservesCRLF(t *testing.T) {
	doc := "## [Unreleased]\r\n\r\n### Added\r\n\r\n- **Old** ([#1](https://x/issues/1)): old.\r\n"
	file, dir := setup(t, doc)
	writeFragment(t, dir, "added-2.md", "- **New** ([#2](https://x/issues/2)): new.")

	code, _, errb := assemble(t, "-file", file, "-dir", dir, "-write", "-keep")
	require.Equal(t, 0, code, errb)
	got := read(t, file)
	assert.Contains(t, got, "- **New** ([#2](https://x/issues/2)): new.\r\n")
	assert.Equal(t, strings.Count(got, "\n"), strings.Count(got, "\r\n"))

	code, _, errb = assemble(t, "-file", file, "-dir", dir, "-check")
	assert.Equal(t, 0, code, errb)
}

func TestWriteReportsMalformedFragmentsWithoutChangingChangelog(t *testing.T) {
	doc := "## [Unreleased]\n\n### Added\n\n- **Old** ([#1](https://x/issues/1)): old.\n"
	file, dir := setup(t, doc)
	writeFragment(t, dir, "added-2.md", "- **New** ([#99](https://x/issues/99)): new.")

	code, out, errb := assemble(t, "-file", file, "-dir", dir, "-write")
	assert.Equal(t, 1, code)
	assert.Empty(t, errb)
	assert.Contains(t, out, "must link issue #2")
	assert.Equal(t, doc, read(t, file))
}

func TestMissingUnreleasedIsAnError(t *testing.T) {
	file, dir := setup(t, "# Changelog\n\n## [1.0.0]\n")
	writeFragment(t, dir, "README.md", "# ignored")

	code, _, errb := assemble(t, "-file", file, "-dir", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, errb, "no ## [Unreleased] section")
}

func TestMissingFileIsAnError(t *testing.T) {
	_, dir := setup(t, "")
	code, _, errb := assemble(t, "-file", filepath.Join(t.TempDir(), "absent.md"), "-dir", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, errb, "changelogassemble:")
}

func TestUsageErrors(t *testing.T) {
	file, dir := setup(t, "## [Unreleased]\n")

	code, _, errb := assemble(t, "-file", file, "-dir", dir, "-check", "-write")
	assert.Equal(t, 2, code)
	assert.Contains(t, errb, "mutually exclusive")

	code, _, errb = assemble(t, "-file", file, "-dir", dir, "-keep")
	assert.Equal(t, 2, code)
	assert.Contains(t, errb, "-keep requires -write")

	code, _, _ = assemble(t, "-bogus")
	assert.Equal(t, 2, code)
}

func TestHelpers(t *testing.T) {
	lines, eol := splitLines("a\r\nb\r\n")
	assert.Equal(t, "\r\n", eol)
	assert.Equal(t, []string{"a", "b", ""}, lines)

	lines, eol = splitLines("a\nb")
	assert.Equal(t, "\n", eol)
	assert.Equal(t, []string{"a", "b"}, lines)

	_, _, err := unreleased([]string{"# Changelog"})
	assert.ErrorIs(t, err, changelog.ErrMissingUnreleased)

	assert.False(t, bulletPresent([]string{"# Changelog"}, "- **A**: b."))

	_, err = insertIntoSection([]string{"# Changelog"}, changelog.Sections[0], []string{"- **A**: b."})
	assert.ErrorIs(t, err, changelog.ErrMissingUnreleased)
}

func TestFoldReturnsSectionError(t *testing.T) {
	_, _, err := fold([]string{"# Changelog"}, []changelog.Fragment{
		{Section: changelog.Sections[0], Bullet: "- **A**: b."},
	})
	assert.ErrorIs(t, err, changelog.ErrMissingUnreleased)
}

func TestInsertIntoEmptySection(t *testing.T) {
	lines := []string{"## [Unreleased]", "", "### Added", "## [1.0.0]"}
	got, err := insertIntoSection(lines, changelog.Sections[0], []string{"- **A**: b."})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"## [Unreleased]", "", "### Added", "", "- **A**: b.", "", "## [1.0.0]",
	}, got)
}

func TestInsertIntoSectionWithoutTrailingBlank(t *testing.T) {
	lines := []string{"## [Unreleased]", "### Added", "- **Old**: x.", "### Fixed", "- **Fix**: y."}
	got, err := insertIntoSection(lines, changelog.Sections[0], []string{"- **New**: n."})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"## [Unreleased]", "### Added", "- **Old**: x.", "- **New**: n.", "", "### Fixed", "- **Fix**: y.",
	}, got)
}

func TestInsertCreatesSectionBeforeUnknownHeading(t *testing.T) {
	lines := []string{"## [Unreleased]", "### Stuff", "- x", "### Fixed", "- y"}
	got, err := insertIntoSection(lines, changelog.Sections[0], []string{"- **A**: b."})
	require.NoError(t, err)
	added, stuff := strings.Index(strings.Join(got, "\n"), "### Added"), strings.Index(strings.Join(got, "\n"), "### Stuff")
	assert.Less(t, added, stuff)
}

func TestInsertAtEndWithoutTrailingBlank(t *testing.T) {
	lines := []string{"## [Unreleased]", "> **Upgrade**: x.", "## [1.0.0]"}
	got, err := insertIntoSection(lines, changelog.Sections[0], []string{"- **A**: b."})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"## [Unreleased]", "> **Upgrade**: x.", "", "### Added", "", "- **A**: b.", "", "## [1.0.0]",
	}, got)
}

func TestMissingFragmentDirectory(t *testing.T) {
	file, _ := setup(t, "## [Unreleased]\n")
	code, _, errb := assemble(t, "-file", file, "-dir", filepath.Join(t.TempDir(), "absent"))
	assert.Equal(t, 2, code)
	assert.Contains(t, errb, "changelogassemble:")
}

func TestMain(t *testing.T) {
	oldArgs, oldExit := os.Args, osExit
	t.Cleanup(func() {
		os.Args, osExit = oldArgs, oldExit
	})
	file, dir := setup(t, "## [Unreleased]\n\n### Added\n\n- **A** ([#1](https://x/issues/1)): a.\n")
	os.Args = []string{"changelogassemble", "-file", file, "-dir", dir, "-check"}
	code := -1
	osExit = func(c int) { code = c }
	main()
	assert.Equal(t, 0, code)
}
