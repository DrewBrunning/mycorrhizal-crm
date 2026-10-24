package mdref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write creates a file (and parents) in the fixture tree.
func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findingsText(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		b.WriteString(f.String())
		b.WriteString("\n")
	}
	return b.String()
}

func TestFindingString(t *testing.T) {
	if got := (Finding{File: "a.md", Line: 3, Message: "x"}).String(); got != "a.md:3: x" {
		t.Errorf("line finding = %q", got)
	}
	if got := (Finding{File: "a.md", Message: "x"}).String(); got != "a.md: x" {
		t.Errorf("unlined finding = %q", got)
	}
}

func TestKramdownID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Backups", "backups"},
		{"Rolling back a bad release (N+1 → N)", "rolling-back-a-bad-release-n1--n"},
		{"Scenario: a vulnerability is reported through SECURITY.md", "scenario-a-vulnerability-is-reported-through-securitymd"},
		{"The `/api/v1` promise", "the-apiv1-promise"},
		{"Scenario: `JWT_SECRET_KEY` is leaked", "scenario-jwt_secret_key-is-leaked"},
		{"Dirty schema", "dirty-schema"},
		{"Recovery objectives (RPO and RTO)", "recovery-objectives-rpo-and-rto"},
	}
	for _, c := range cases {
		if got := kramdownID(removeBackticks(c.in)); got != c.want {
			t.Errorf("kramdownID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractLinksIgnoresCodeAndComments(t *testing.T) {
	text := strings.Join([]string{
		"Some [working link](target.md) here.",
		"",
		"[backtick](`cmd/x`)",
		"",
		"`inline [not a link](nowhere.md)`",
		"",
		"```",
		"# fenced",
		"[also not a link](fence.md)",
		"```",
		"",
		"<!-- [not a link](comment.md) -->",
		"",
		"[a target](sub/page.html#section)",
		"",
		"![an image](img.png)",
	}, "\n")
	refs := ExtractLinks(text)
	var dests []string
	for _, r := range refs {
		dests = append(dests, r.Dest)
	}
	joined := strings.Join(dests, "|")
	for _, want := range []string{"target.md", "sub/page.html#section", "img.png"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ExtractLinks(%q) missing %q; got %v", text, want, dests)
		}
	}
	for _, not := range []string{"nowhere.md", "fence.md", "comment.md", "cmd/x"} {
		if strings.Contains(joined, not) {
			t.Errorf("ExtractLinks(%q) should not contain %q; got %v", text, not, dests)
		}
	}
}

func TestExtractLinksMultilineDestination(t *testing.T) {
	text := "**[migration-recovery.md → At-rest\nbackfill](operations/migration-recovery.md#at-rest-backfill-vs-the-audit-events-trigger):"
	refs := ExtractLinks(text)
	if len(refs) != 1 {
		t.Fatalf("want 1 ref, got %d: %v", len(refs), refs)
	}
	want := "operations/migration-recovery.md#at-rest-backfill-vs-the-audit-events-trigger"
	if refs[0].Dest != want {
		t.Errorf("dest = %q, want %q", refs[0].Dest, want)
	}
	if refs[0].Line != 1 {
		t.Errorf("line = %d, want 1", refs[0].Line)
	}
}

func TestExtractLinksNestedBrackets(t *testing.T) {
	refs := ExtractLinks("[a [b] c](dest.md) and [x](a(b)c)")
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %v", refs)
	}
	if refs[0].Dest != "dest.md" {
		t.Errorf("nested label dest = %q", refs[0].Dest)
	}
	if refs[1].Dest != "a(b)c" {
		t.Errorf("nested paren dest = %q", refs[1].Dest)
	}
}

func TestExtractLinksUnclosedLabel(t *testing.T) {
	if refs := ExtractLinks("no closing bracket [open and nothing else"); len(refs) != 0 {
		t.Errorf("expected no refs for unclosed label, got %v", refs)
	}
}

func TestScrubVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
		not  string
	}{
		{"multiline comment", "<!-- comment\n[hidden](x.md)\n-->\n[visible](real.md)", "x.md"},
		{"tilde fence", "~~~\n[hidden](y.md)\n~~~\n[visible](real.md)", "y.md"},
		{"unclosed inline code runs to end", "`unclosed [hidden](z.md)", "z.md"},
		{"multiline inline code", "`multi\nline [hidden](w.md)`\n[visible](real.md)", "w.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var dests []string
			for _, r := range ExtractLinks(c.in) {
				dests = append(dests, r.Dest)
			}
			joined := strings.Join(dests, "|")
			if strings.Contains(joined, c.not) {
				t.Errorf("scrub(%q) kept hidden dest %q: %v", c.in, c.not, dests)
			}
		})
	}
}

func TestMatchBracketNoClose(t *testing.T) {
	if _, ok := matchBracket([]rune("[abc"), 0, '[', ']'); ok {
		t.Error("expected no match for an unterminated bracket")
	}
	if _, ok := matchBracket([]rune("([)]"), 1, '(', ')'); ok {
		t.Error("expected no match for an unterminated paren")
	}
	if got, ok := matchBracket([]rune("(a(b)c)"), 0, '(', ')'); !ok || got != 6 {
		t.Errorf("nested bracket match = %d,%v; want 6,true", got, ok)
	}
}

func TestGitHubBlobRepoPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/docker-compose.yml", "docker-compose.yml"},
		{"https://github.com/DrewBrunning/mycorrhizal-crm/tree/main/docs/development", "docs/development"},
		{"https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/.github/workflows/docs.yml", ".github/workflows/docs.yml"},
		{"https://example.com/other", ""},
		{"https://github.com/other/repo/blob/main/x", ""},
	}
	for _, c := range cases {
		if got := GitHubBlobRepoPath(c.in); got != c.want {
			t.Errorf("GitHubBlobRepoPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitFragment(t *testing.T) {
	cases := []struct {
		in, path, frag string
	}{
		{"a/b.md#c", "a/b.md", "c"},
		{"a/b.md", "a/b.md", ""},
		{"#only", "", "only"},
	}
	for _, c := range cases {
		path, frag := SplitFragment(c.in)
		if path != c.path || frag != c.frag {
			t.Errorf("SplitFragment(%q) = %q,%q; want %q,%q", c.in, path, frag, c.path, c.frag)
		}
	}
}

func TestHeadingIDs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(f, []byte("# Top\n\n## Backups\n\n## Backups\n\n### Scenario: `JWT_SECRET_KEY` is leaked\n\n<a id=\"custom-anchor\"></a>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids := HeadingIDs(f)
	for _, want := range []string{"top", "backups", "backups-1", "scenario-jwt_secret_key-is-leaked", "custom-anchor"} {
		if !ids[want] {
			t.Errorf("HeadingIDs(%s) missing %q: %v", f, want, ids)
		}
	}
}

func TestHeadingIDsMissingFile(t *testing.T) {
	if ids := HeadingIDs(filepath.Join(t.TempDir(), "does-not-exist.md")); ids != nil {
		t.Errorf("HeadingIDs on a missing file should return nil, got %v", ids)
	}
}

func TestHeadingIDsSectionFallback(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(f, []byte("# !!!\n\n[there](#section)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ids := HeadingIDs(f); !ids["section"] {
		t.Errorf("punctuation-only heading should fall back to id %q: %v", "section", ids)
	}
}

func TestPathExists(t *testing.T) {
	root := t.TempDir()
	if PathExists(root, "") {
		t.Error("empty path should not exist")
	}
	if PathExists("", "/nonexistent/definitely-not-here") {
		t.Error("absolute missing path should not exist")
	}
	if !PathExists(root, root) {
		t.Error("existing dir should exist")
	}
}

func TestFileReferenceResolvesDirect(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/internal/fsguard/fsguard.go", "package fsguard\n")
	if !FileReferenceResolves(root, "backend/internal/fsguard.NetworkFilesystemWarning") {
		t.Error("dot-qualified package symbol should relax to the package dir")
	}
	if FileReferenceResolves(root, "fsguard.NetworkFilesystemWarning") {
		t.Error("a path with no directory must not resolve")
	}
}

func TestTrimLineColSuffix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"backend/database/migrate.go:123", "backend/database/migrate.go"},
		{"backend/services/x.go:44-49", "backend/services/x.go"},
		{"docs/development/testing.md:517", "docs/development/testing.md"},
		{"docs/upgrade-compatibility.md", "docs/upgrade-compatibility.md"},
		{"backend/x.go:", "backend/x.go:"},
		{"backend/x.go:note", "backend/x.go:note"},
	}
	for _, c := range cases {
		if got := trimLineColSuffix(c.in); got != c.want {
			t.Errorf("trimLineColSuffix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolverLinksVariantTargets(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/some/page.md", "# Page\n\n## A section\n")
	write(t, root, "CLAUDE.md", "# root\n")
	write(t, root, "docs/from.md", "[here](some/page.html#a-section) and [abs](/CLAUDE.md) and [bad](some/missing.html)\n")
	f := File{Path: filepath.Join(root, "docs/from.md"), Display: "from.md", Text: "[here](some/page.html#a-section) and [abs](/CLAUDE.md) and [bad](some/missing.html)\n"}
	got := findingsText((Resolver{Root: root}).Links(f))
	if strings.Contains(got, "some/page.html#a-section") {
		t.Errorf("html-mapped anchor should resolve: %s", got)
	}
	if strings.Contains(got, "abs") {
		t.Errorf("absolute link should resolve: %s", got)
	}
	if !strings.Contains(got, "missing.html") {
		t.Errorf("missing .html target should be reported: %s", got)
	}
}

func TestResolverLinksNonLocalDestinations(t *testing.T) {
	root := t.TempDir()
	text := "[mail](mailto:someone@example.com) [ext](https://example.com/x) [hash](#) [self](#existing)\n# existing\n"
	write(t, root, "docs/extra.md", text)
	f := File{Path: filepath.Join(root, "docs/extra.md"), Display: "extra.md", Text: text}
	if got := findingsText((Resolver{Root: root}).Links(f)); got != "" {
		t.Errorf("external/mailto/# links should not be flagged: %s", got)
	}
}

func TestResolverLinksCrossFileAnchorMissing(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/some.md", "# Page\n")
	text := "[bad](some.html#nope) [empty]() [ref][some] more text\n"
	write(t, root, "docs/from.md", text)
	f := File{Path: filepath.Join(root, "docs/from.md"), Display: "from.md", Text: text}
	got := findingsText((Resolver{Root: root}).Links(f))
	if !strings.Contains(got, "some.html#nope") {
		t.Errorf("missing cross-file anchor should be reported: %s", got)
	}
}

func TestResolverLinksSelfAnchorMissing(t *testing.T) {
	root := t.TempDir()
	text := "[bad](#nope)\n"
	write(t, root, "docs/from.md", text)
	f := File{Path: filepath.Join(root, "docs/from.md"), Display: "from.md", Text: text}
	got := findingsText((Resolver{Root: root}).Links(f))
	if !strings.Contains(got, "anchor not found in this file: #nope") {
		t.Errorf("missing self anchor should be reported: %s", got)
	}
}

func TestResolverRepoReferencesRelaxation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/internal/fsguard/fsguard.go", "package fsguard\n")
	write(t, root, "backend/cmd/backfill-search-index/main.go", "package main\n")
	text := "see backend/internal/fsguard.NetworkFilesystemWarning and cmd/backfill-search-index; globs backend/database/migrate_*_test.go are fine; also `docs/backend` and backend/frontend are not files; missing `backend/services/nope_missing.go`.\n"
	write(t, root, "docs/dev.md", text)
	f := File{Path: filepath.Join(root, "docs/dev.md"), Display: "dev.md", Text: text}
	got := findingsText((Resolver{Root: root}).RepoReferences(f))
	if strings.Contains(got, "fsguard.NetworkFilesystemWarning") {
		t.Errorf("go-symbol reference should relax to package: %s", got)
	}
	if strings.Contains(got, "backfill-search-index") {
		t.Errorf("cmd/ alias should resolve: %s", got)
	}
	if strings.Contains(got, "backend/frontend") {
		t.Errorf("prose area enumeration should not be flagged: %s", got)
	}
	if !strings.Contains(got, "nope_missing.go") {
		t.Errorf("missing file reference should be reported: %s", got)
	}
}

func TestResolverRepoReferencesSkipsNonPaths(t *testing.T) {
	root := t.TempDir()
	text := "the `user_id` scope, `status: suggested`, `normal|private|secret`, and `GET /api/v1/contacts` are not paths\n"
	f := File{Path: filepath.Join(root, "docs/plain.md"), Display: "plain.md", Text: text}
	if got := findingsText((Resolver{Root: root}).RepoReferences(f)); got != "" {
		t.Errorf("non-path tokens should not be flagged: %s", got)
	}
}

func TestResolverRepoReferencesTestdataSkip(t *testing.T) {
	root := t.TempDir()
	text := "see `testdata/budgets.json`\n"
	f := File{Path: filepath.Join(root, "docs/plain.md"), Display: "plain.md", Text: text}
	if got := findingsText((Resolver{Root: root}).RepoReferences(f)); got != "" {
		t.Errorf("ambiguous testdata/ references should be skipped: %s", got)
	}
}

func TestResolverRepoReferencesDedup(t *testing.T) {
	root := t.TempDir()
	text := "missing `backend/services/nope_missing.go` twice `backend/services/nope_missing.go`.\n"
	f := File{Path: filepath.Join(root, "docs/plain.md"), Display: "plain.md", Text: text}
	got := findingsText((Resolver{Root: root}).RepoReferences(f))
	if strings.Count(got, "nope_missing.go") != 1 {
		t.Errorf("duplicate missing reference should be reported once: %s", got)
	}
}

func TestResolverRepoReferencesStripsFences(t *testing.T) {
	root := t.TempDir()
	text := "```\nbackend/services/nope_in_fence.go\n```\n`backend/services/nope_real.go`\n"
	f := File{Path: filepath.Join(root, "docs/plain.md"), Display: "plain.md", Text: text}
	got := findingsText((Resolver{Root: root}).RepoReferences(f))
	if strings.Contains(got, "nope_in_fence.go") {
		t.Errorf("fenced reference should not be flagged: %s", got)
	}
	if !strings.Contains(got, "nope_real.go") {
		t.Errorf("inline reference should be flagged: %s", got)
	}
}
