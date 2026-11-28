package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fixture is shaped like v1.7.0 against main: the release has a flat nav and
// ships some pages without front matter; main publishes them under groups
// the release does not have.
func fixture(t *testing.T) (release, current string) {
	t.Helper()
	release, current = t.TempDir(), t.TempDir()
	put(t, release, "deployment.md", "---\ntitle: Deployment\nnav_order: 8\n---\n# Deployment\n")
	put(t, release, "faq.md", "---\ntitle: \"FAQ & Troubleshooting\"\nnav_order: 25\n---\n# FAQ\n")
	put(t, release, "operations/disaster-recovery.md", "# Disaster recovery boundaries\n\nv1.7.0 text.\n")
	put(t, release, "operations/search-index.md", "# Rebuilding the search index\n")
	put(t, release, "import/README.md", "# Source imports\n")
	put(t, release, "import/monica-mapping.md", "# Monica\n")
	put(t, release, "adrs/0001-x.md", "# ADR\n") // main does not publish it either
	put(t, release, "gone.md", "# Only in the release\n")

	put(t, current, "deployment.md", "---\ntitle: Deployment\nparent: Self-hosting\nnav_order: 1\n---\n# Deployment (main text)\n")
	put(t, current, "operations/disaster-recovery.md", "---\ntitle: Disaster recovery boundaries\nparent: Self-hosting\nnav_order: 5\ndescription: What recovery guarantees exist.\n---\n\n# Disaster recovery boundaries\n\nmain text.\n")
	put(t, current, "operations/search-index.md", "---\ntitle: Rebuilding the search index\nparent: Self-hosting\nnav_order: 7\n---\n# Rebuilding\n")
	put(t, current, "import/README.md", "---\ntitle: Source imports\nparent: Reference\nhas_children: true\nnav_order: 10\n---\n# Source imports\n")
	put(t, current, "import/monica-mapping.md", "---\ntitle: Monica import mapping\nparent: Source imports\ngrand_parent: Reference\nnav_order: 1\n---\n# Monica\n")
	put(t, current, "adrs/0001-x.md", "# ADR\n")
	return release, current
}

func TestBackfillRendersTheReleasePagesMainPublishes(t *testing.T) {
	release, current := fixture(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-src", release, "-current", current}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}

	// The release's own text is kept; only front matter is added. Its parent
	// (Self-hosting) is not a page in the release, so the page joins the flat
	// nav after the release's own entries (max nav_order 25; import/README
	// sorts first, see TestBackfillAppendsInMainsOrder).
	got := read(t, release, "operations/disaster-recovery.md")
	want := "---\ntitle: Disaster recovery boundaries\nnav_order: 27\ndescription: What recovery guarantees exist.\n---\n\n# Disaster recovery boundaries\n\nv1.7.0 text.\n"
	if got != want {
		t.Fatalf("disaster-recovery.md =\n%s\nwant\n%s", got, want)
	}
	// A parent that the backfill itself publishes (Source imports) is kept;
	// the grand_parent the release lacks (Reference) is dropped.
	monica := read(t, release, "import/monica-mapping.md")
	if !strings.Contains(monica, "parent: Source imports\n") || strings.Contains(monica, "grand_parent") {
		t.Fatalf("monica-mapping.md front matter wrong:\n%s", monica)
	}
	if !strings.Contains(monica, "nav_order: 1\n") || !strings.HasSuffix(monica, "---\n# Monica\n") {
		t.Fatalf("monica-mapping.md kept main's order or lost the release body:\n%s", monica)
	}

	// Pages that already render, pages main does not publish, and pages main
	// no longer has are untouched.
	for rel, want := range map[string]string{
		"deployment.md":  "---\ntitle: Deployment\nnav_order: 8\n---\n# Deployment\n",
		"adrs/0001-x.md": "# ADR\n",
		"gone.md":        "# Only in the release\n",
	} {
		if got := read(t, release, rel); got != want {
			t.Errorf("%s changed:\n%s", rel, got)
		}
	}
	if !strings.Contains(stdout.String(), "4 page(s)") {
		t.Fatalf("summary: %q", stdout.String())
	}
}

func TestBackfillAppendsInMainsOrder(t *testing.T) {
	release, current := fixture(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-src", release, "-current", current}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	// Orphaned pages sort by main's (parent, nav_order, path):
	// Reference/10 import/README, Self-hosting/5 disaster-recovery,
	// Self-hosting/7 search-index.
	for rel, order := range map[string]string{
		"import/README.md":                "nav_order: 26\n",
		"operations/disaster-recovery.md": "nav_order: 27\n",
		"operations/search-index.md":      "nav_order: 28\n",
	} {
		if got := read(t, release, rel); !strings.Contains(got, order) || strings.Contains(got, "parent:") {
			t.Errorf("%s: want top-level %q, got\n%s", rel, order, got)
		}
	}
}

// A page that is top-level on main and two pages tied on main's
// (parent, nav_order) still get distinct, deterministic positions.
func TestBackfillOrdersTopLevelPagesAndTies(t *testing.T) {
	release, current := fixture(t)
	put(t, release, "relationship-dates.md", "# Relationship dates\n")
	put(t, current, "relationship-dates.md", "---\ntitle: Relationship dates\nnav_order: 3\n---\n# x\n")
	put(t, release, "operations/b.md", "# B\n")
	put(t, current, "operations/b.md", "---\ntitle: B\nparent: Self-hosting\nnav_order: 7\n---\n# x\n")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-src", release, "-current", current}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	// No parent sorts first (""), then Reference/10, then Self-hosting 5, 7 (b), 7 (search-index).
	for rel, order := range map[string]string{
		"relationship-dates.md":           "nav_order: 26\n",
		"import/README.md":                "nav_order: 27\n",
		"operations/disaster-recovery.md": "nav_order: 28\n",
		"operations/b.md":                 "nav_order: 29\n",
		"operations/search-index.md":      "nav_order: 30\n",
	} {
		if got := read(t, release, rel); !strings.Contains(got, order) {
			t.Errorf("%s: want %q, got\n%s", rel, order, got)
		}
	}
}

// An unreadable release page fails the build instead of shipping it unrendered.
func TestBackfillFailsOnAnUnreadablePage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 file") // # pragma: no cover — CI and local runs are unprivileged
	}
	release, current := fixture(t)
	p := filepath.Join(release, "operations", "disaster-recovery.md")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-src", release, "-current", current}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "disaster-recovery.md") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

// A main page that cannot be read, or a release page that cannot be
// rewritten, fails the build instead of leaving the page unrendered.
func TestBackfillFailsOnFileErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes") // # pragma: no cover — CI and local runs are unprivileged
	}
	for name, setup := range map[string]func(release, current string) string{
		"unreadable on main": func(_, current string) string { return filepath.Join(current, "operations", "disaster-recovery.md") },
		"read-only release":  func(release, _ string) string { return filepath.Join(release, "operations", "disaster-recovery.md") },
	} {
		t.Run(name, func(t *testing.T) {
			release, current := fixture(t)
			p := setup(release, current)
			mode := os.FileMode(0o000)
			if strings.HasPrefix(p, release) {
				mode = 0o444
			}
			if err := os.Chmod(p, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
			var stdout, stderr bytes.Buffer
			if code := run([]string{"-src", release, "-current", current}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "disaster-recovery.md") {
				t.Fatalf("exit %d, stderr %q", code, stderr.String())
			}
		})
	}
}

func TestBackfillIsIdempotent(t *testing.T) {
	release, current := fixture(t)
	var out, errb bytes.Buffer
	if code := run([]string{"-src", release, "-current", current}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	first := read(t, release, "operations/disaster-recovery.md")
	out.Reset()
	if code := run([]string{"-src", release, "-current", current}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if read(t, release, "operations/disaster-recovery.md") != first || !strings.Contains(out.String(), "0 page(s)") {
		t.Fatalf("second run changed something: %q", out.String())
	}
}

func TestRunRejectsBadUsage(t *testing.T) {
	release, current := fixture(t)
	put(t, current, "broken.md", "---\ntitle: [unclosed\n---\n")
	put(t, release, "broken.md", "# Broken\n")
	for _, args := range [][]string{
		{"-current", current},
		{"-src", release},
		{"-src", release, "-current", current, "-bogus"},
		{"-src", filepath.Join(release, "missing"), "-current", current},
		{"-src", release, "-current", current}, // main's front matter does not parse
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%v: exit %d, want 2 (stderr %q)", args, code, stderr.String())
		}
	}
}
