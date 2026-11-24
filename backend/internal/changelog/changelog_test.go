package changelog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSectionLookups(t *testing.T) {
	for i, want := range Sections {
		byName, ok := SectionForName(want.Name)
		require.True(t, ok, want.Name)
		assert.Equal(t, want, byName)

		byKey, ok := SectionForKey(want.Key)
		require.True(t, ok, want.Key)
		assert.Equal(t, want, byKey)

		pos, ok := SectionPosition(want.Name)
		require.True(t, ok, want.Name)
		assert.Equal(t, i+1, pos)

		assert.Equal(t, i, SectionIndex(want))
	}

	_, ok := SectionForName("Nope")
	assert.False(t, ok)
	_, ok = SectionForKey("nope")
	assert.False(t, ok)
	_, ok = SectionPosition("Nope")
	assert.False(t, ok)
	assert.Equal(t, len(Sections), SectionIndex(Section{Name: "Nope", Key: "nope"}))

	assert.Equal(t, "Added, Changed, Deprecated, Removed, Fixed, Security, Internal", SectionNameList())
	assert.Equal(t, "added, changed, deprecated, removed, fixed, security, internal", SectionKeyList())
}

func TestValidateBullet(t *testing.T) {
	tooLong := "- **Lead**: " + strings.Repeat("word ", MaxWords)

	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "clean",
			text: "- **Lead** ([#12](https://github.com/o/r/issues/12)): it works.",
		},
		{
			name: "bare issue reference",
			text: "- **Lead** (#12): it works.",
			want: []string{"reference issues with a full URL, not a bare #number"},
		},
		{
			name: "no bold lead",
			text: "- plain text.",
			want: []string{"bullet must start with a bold lead phrase: - **Lead**: ..."},
		},
		{
			name: "too many words",
			text: tooLong,
			want: []string{"bullet has 62 words; keep it to about 40 (limit 60)"},
		},
		{
			name: "bare link text with a full URL is fine",
			text: "- **Lead** ([#12](https://x/issues/12)): it works.",
		},
		{
			name: "several findings",
			text: "- plain (#12): " + strings.Repeat("word ", MaxWords),
			want: []string{
				"bullet must start with a bold lead phrase: - **Lead**: ...",
				"reference issues with a full URL, not a bare #number",
				"bullet has 63 words; keep it to about 40 (limit 60)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidateBullet(tt.text))
		})
	}
}

func TestBulletReferencesIssue(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		issue int
		want  bool
	}{
		{"exact", "- **A** ([#12](https://github.com/o/r/issues/12)): b.", 12, true},
		{"another issue", "- **A** ([#13](https://github.com/o/r/issues/13)): b.", 12, false},
		{"prefix is not a match", "- **A** ([#12](https://github.com/o/r/issues/120)): b.", 12, false},
		{"several links", "- **A** ([#1](https://x/issues/1), [#12](https://x/issues/12)): b.", 12, true},
		{"no link", "- **A**: b.", 12, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BulletReferencesIssue(tt.text, tt.issue))
		})
	}
}

func TestParseFragmentName(t *testing.T) {
	tests := []struct {
		name    string
		want    FragmentName
		wantErr string
	}{
		{name: "fixed-2917.md", want: FragmentName{Section: Sections[4], Issue: 2917}},
		{name: "added-2513-2.md", want: FragmentName{Section: Sections[0], Issue: 2513, Suffix: 2}},
		{name: "internal-1.md", want: FragmentName{Section: Sections[6], Issue: 1}},
		{name: "fixed.md", wantErr: "must be named"},
		{name: "nope-12.md", wantErr: `unknown section "nope"`},
		{name: "Fixed-12.md", wantErr: "must be named"},
		{name: "fixed-12-x.md", wantErr: "must be named"},
		{name: "fixed-12-1-2.md", wantErr: "must be named"},
		{name: "fixed--12.md", wantErr: "must be named"},
		{name: "fixed-0.md", wantErr: "positive GitHub issue number"},
		{name: "README.md", wantErr: "must be named"},
		{name: "fixed-12.txt", wantErr: "must be named"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFragmentName(tt.name)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Not a fragment\n\n- **Read me** (#1): nope.\n")
	writeFile(t, dir, "notes.txt", "- **A** ([#1](https://x/issues/1)): b.\n")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o755))
	writeFile(t, filepath.Join(dir, "nested"), "added-1.md", "- **Nested** ([#1](https://x/issues/1)): b.\n")

	writeFile(t, dir, "fixed-2917.md", "- **Fix** ([#2917](https://github.com/o/r/issues/2917)): done.\n")
	writeFile(t, dir, "added-2513-2.md", "- **Second** ([#2513](https://github.com/o/r/issues/2513)): done.\n")
	writeFile(t, dir, "added-2513.md", "- **First** ([#2513](https://github.com/o/r/issues/2513)): done.\n")

	fragments, findings, err := LoadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, findings)
	require.Len(t, fragments, 3)
	assert.Equal(t, []string{"added-2513-2.md", "added-2513.md", "fixed-2917.md"}, []string{
		fragments[0].Name, fragments[1].Name, fragments[2].Name,
	})
	assert.Equal(t, "- **Fix** ([#2917](https://github.com/o/r/issues/2917)): done.", fragments[2].Bullet)
	assert.Equal(t, 1, fragments[2].BulletLine)
	assert.Equal(t, 2917, fragments[2].Issue)
	assert.Equal(t, Sections[4], fragments[2].Section)
}

func TestLoadDirReportsFindingsButKeepsParsing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fixed.md", "- **A** ([#1](https://x/issues/1)): b.\n")
	writeFile(t, dir, "fixed-999.md", "- **A** ([#1](https://x/issues/1)): b.\n")
	writeFile(t, dir, "security-42.md", "\n\n")
	writeFile(t, dir, "changed-7.md", "### Changed\n\n- **A** ([#7](https://x/issues/7)): b.\n")
	writeFile(t, dir, "added-8.md", "- **A** ([#8](https://x/issues/8)): b.\n\nsecond line\n")

	fragments, findings, err := LoadDir(dir)
	require.NoError(t, err)
	assert.Len(t, fragments, 4) // fixed-999, security-42, changed-7 and added-8; fixed.md was rejected
	var messages []string
	for _, f := range findings {
		messages = append(messages, f.String())
	}
	joined := strings.Join(messages, "\n")
	assert.Contains(t, joined, "fixed.md:1: fragment \"fixed.md\" must be named")
	assert.Contains(t, joined, "fixed-999.md:1: bullet must link issue #999")
	assert.Contains(t, joined, "security-42.md:1: fragment is empty")
	assert.Contains(t, joined, "changed-7.md:1: fragment must not contain a section header")
	assert.Contains(t, joined, "changed-7.md:1: bullet must start with a bold lead phrase")
	assert.Contains(t, joined, "added-8.md:3: fragment must contain exactly one bullet line")
}

func TestLoadDirMissingDirectory(t *testing.T) {
	_, _, err := LoadDir(filepath.Join(t.TempDir(), "absent"))
	require.Error(t, err)
}

func TestLoadDirUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// A directory named like a fragment is skipped, so make ReadFile fail with
	// a path that ReadDir lists as a file but cannot be opened.
	writeFile(t, dir, "fixed-1.md", "- **A** ([#1](https://x/issues/1)): b.\n")
	require.NoError(t, os.Chmod(filepath.Join(dir, "fixed-1.md"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "fixed-1.md"), 0o600) })
	if _, err := os.ReadFile(filepath.Join(dir, "fixed-1.md")); err == nil {
		t.Skip("running with privileges that can read mode-000 files")
	}
	_, _, err := LoadDir(dir)
	require.Error(t, err)
}

func TestFindingString(t *testing.T) {
	assert.Equal(t, "changelog.d/fixed-1.md:2: bad", Finding{Path: "changelog.d/fixed-1.md", Line: 2, Message: "bad"}.String())
}

func TestSortFragments(t *testing.T) {
	fragments := []Fragment{
		{Name: "internal-1.md", Section: Sections[6]},
		{Name: "fixed-2.md", Section: Sections[4]},
		{Name: "added-9.md", Section: Sections[0]},
		{Name: "added-1.md", Section: Sections[0]},
	}
	SortFragments(fragments)
	assert.Equal(t, []string{"added-1.md", "added-9.md", "fixed-2.md", "internal-1.md"}, []string{
		fragments[0].Name, fragments[1].Name, fragments[2].Name, fragments[3].Name,
	})
}
