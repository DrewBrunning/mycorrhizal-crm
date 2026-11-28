package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const blob = "https://github.com/DrewBrunning/mycorrhizal-crm/blob/v1.7.0/"

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fixture is a built site and the docs/ source it was built from, shaped like
// a real release: rendered pages, an unpublished record Jekyll copied raw, an
// unpublished record it did not copy, and a static asset.
func fixture(t *testing.T) (site, src string) {
	t.Helper()
	site, src = t.TempDir(), t.TempDir()
	for _, rel := range []string{"settings", "deployment", "operations/disaster-recovery", "development/testing"} {
		put(t, src, rel+".md", "---\ntitle: x\n---\n")
		put(t, site, rel+".html", "<html></html>")
	}
	put(t, src, "adrs/0039-x.md", "# ADR\n")
	put(t, site, "adrs/0039-x.md", "# ADR\n") // copied verbatim: it has no front matter
	put(t, src, "security/baseline.md", "# Baseline\n")
	put(t, site, "assets/shot.png", "png")
	put(t, src, "assets/shot.png", "png")
	return site, src
}

func TestRewriteLinks(t *testing.T) {
	site, src := fixture(t)
	cases := []struct {
		page, href, want string
	}{
		// A rendered page linked by its .md name (a release built before the
		// docscheck .html rule) gets its rendered address.
		{"settings.html", "deployment.md#production-environment", "deployment.html#production-environment"},
		{"operations/disaster-recovery.html", "../deployment.md", "../deployment.html"},
		// An unpublished record goes to GitHub, whether Jekyll copied it raw
		// (a .md 200 that shows unrendered Markdown) or not at all.
		{"settings.html", "adrs/0039-x.md#amendment", blob + "docs/adrs/0039-x.md#amendment"},
		{"deployment.html", "security/baseline.md", blob + "docs/security/baseline.md"},
		// gensitechangelog writes every docs/X.md as X.html; one that was not
		// rendered is sent to its source.
		{"settings.html", "adrs/0039-x.html#amendment", blob + "docs/adrs/0039-x.md#amendment"},
		// Repository files outside docs/ go to GitHub at the same ref.
		{"development/testing.html", "../../docker-compose.yml", blob + "docker-compose.yml"},
		{"settings.html", "../SECURITY.md", blob + "SECURITY.md"},
		{"settings.html", "../.github/workflows/grype.yml", blob + ".github/workflows/grype.yml"},
		// Links that already resolve on the site are kept as written.
		{"settings.html", "deployment.html#x", "deployment.html#x"},
		{"settings.html", "assets/shot.png", "assets/shot.png"},
		{"settings.html", "#two-factor-authentication", "#two-factor-authentication"},
		{"settings.html", "https://example.com/a.md", "https://example.com/a.md"},
		{"settings.html", "mailto:a@example.com", "mailto:a@example.com"},
		{"settings.html", "?q=1", "?q=1"},
		{"settings.html", "/mycorrhizal-crm/next/settings.html", "/mycorrhizal-crm/next/settings.html"},
	}
	for _, tc := range cases {
		t.Run(tc.page+" "+tc.href, func(t *testing.T) {
			r := rewriter{site: site, src: src, blobBase: blob}
			got, _ := r.rewrite(`<a href="`+tc.href+`">x</a>`, tc.page)
			if want := `<a href="` + tc.want + `">x</a>`; got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestRewriteCountsOnlyChangedLinks(t *testing.T) {
	site, src := fixture(t)
	r := rewriter{site: site, src: src, blobBase: blob}
	_, n := r.rewrite(`<a href="deployment.md">a</a> <a href="deployment.html">b</a> <a href="#x">c</a>`, "settings.html")
	if n != 1 {
		t.Fatalf("changed %d links, want 1", n)
	}
}

func TestRunRewritesEveryPageInPlace(t *testing.T) {
	site, src := fixture(t)
	put(t, site, "settings.html", `<main><a href="deployment.md">d</a> <a href="../SECURITY.md">s</a></main>`)
	put(t, site, "operations/disaster-recovery.html", `<a href="../deployment.md">d</a>`)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-site", site, "-src", src, "-ref", "v1.7.0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(site, "settings.html"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<main><a href="deployment.html">d</a> <a href="` + blob + `SECURITY.md">s</a></main>`; string(got) != want {
		t.Fatalf("settings.html = %s\nwant %s", got, want)
	}
	nested, err := os.ReadFile(filepath.Join(site, "operations", "disaster-recovery.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nested), `href="../deployment.html"`) {
		t.Fatalf("nested page not rewritten: %s", nested)
	}
	if !strings.Contains(stdout.String(), "3 link(s)") {
		t.Fatalf("summary missing the count: %q", stdout.String())
	}
}

func TestRunLeavesCleanPagesUntouched(t *testing.T) {
	site, src := fixture(t)
	clean := `<a href="deployment.html">d</a>`
	put(t, site, "settings.html", clean)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-site", site, "-src", src, "-ref", "main"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if got, _ := os.ReadFile(filepath.Join(site, "settings.html")); string(got) != clean {
		t.Fatalf("clean page changed: %s", got)
	}
	if !strings.Contains(stdout.String(), "rewrote 0 link(s)") {
		t.Fatalf("summary: %q", stdout.String())
	}
}

// A page that cannot be read fails the build rather than shipping its dead
// links unnoticed.
func TestRunFailsOnAnUnreadablePage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 file") // # pragma: no cover — CI and local runs are unprivileged
	}
	site, src := fixture(t)
	p := filepath.Join(site, "settings.html")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-site", site, "-src", src, "-ref", "main"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "settings.html") {
		t.Fatalf("stderr does not name the page: %q", stderr.String())
	}
}

func TestRunRejectsBadUsage(t *testing.T) {
	site, src := fixture(t)
	for _, args := range [][]string{
		{"-src", src, "-ref", "main"},                          // no -site
		{"-site", site, "-ref", "main"},                        // no -src
		{"-site", site, "-src", src, "-ref", "../../x"},        // not a ref
		{"-site", site, "-src", src, "-ref", "main", "-bogus"}, // unknown flag
		{"-site", filepath.Join(site, "missing"), "-src", src, "-ref", "main"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}
