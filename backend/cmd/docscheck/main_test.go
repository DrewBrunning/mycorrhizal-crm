package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/integrations"
)

// write is a test helper that creates a file (and parents) in the fixture tree.
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

func fixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	matrix := "# Supported runtime matrix\n\n"
	page := "# Supported versions\n\n"
	for _, f := range operatorFloors {
		matrix += fmt.Sprintf("| %s | %s |\n", f.label, f.matrix)
		page += fmt.Sprintf("%s: %s\n", f.label, f.page)
	}
	// Docker Engine is pinned by its table row, not the token table above
	// (issue #941), so the fixture carries a real row on each side.
	matrix += "| **Docker Engine** | `>=23.0` | why |\n"
	page += "| **Docker Engine** | `>= 23.0` | why |\n"
	write(t, root, "docs/development/supported-runtime-matrix.md", matrix)
	write(t, root, "docs/supported-versions.md", page)

	ownership := "# Integration ownership\n\n"
	for _, e := range integrations.Registry() {
		ownership += fmt.Sprintf("### %s\n\n<a id=\"%s\"></a>\n\n", e.Name, e.ID)
	}
	write(t, root, "docs/integration-ownership.md", ownership)

	write(t, root, "docs/getting-started.md", "# Getting Started\n\n```sh\ndocker compose up -d --build\n```\n")
	write(t, root, "docs/deployment.md", "# Deployment\n\n## Upgrades\n\n```sh\ndocker compose pull\ndocker compose up -d\n```\n")
	write(t, root, ".github/workflows/deploy-smoke.yml", "name: smoke\njobs:\n  smoke:\n    runs-on: ubuntu-latest\n    steps:\n      - run: docker compose up -d --build\n")
	return root
}

func TestRunAtPassesOnFixtureTree(t *testing.T) {
	root := fixtureTree(t)
	if err := runAt(root); err != nil {
		t.Fatalf("runAt should pass on a clean fixture tree: %v", err)
	}
}

// collectMatches returns the findings (as a joined string) for assertions.
func collectMatches(t *testing.T, root string) string {
	t.Helper()
	return strings.Join(collectFindings(root), "\n")
}

// TestRunAtDetectsFindings makes each check fail in turn and asserts runAt
// reports it. Every check must be reachable by a breakage.
func TestRunAtDetectsFindings(t *testing.T) {
	cases := []struct {
		name   string
		breakF func(t *testing.T, root string)
		match  string
	}{
		{
			"missing operator page",
			func(t *testing.T, root string) { os.Remove(filepath.Join(root, "docs/supported-versions.md")) },
			"supported-versions drift",
		},
		{
			"floor drift between matrix and operator page",
			func(t *testing.T, root string) {
				p := filepath.Join(root, "docs/supported-versions.md")
				b, _ := os.ReadFile(p)
				os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "23.0", "99.0")), 0o644)
			},
			"does not state",
		},
		{
			"missing integration section",
			func(t *testing.T, root string) {
				// Drop the last registry anchor.
				all := integrations.Registry()
				last := all[len(all)-1]
				p := filepath.Join(root, "docs/integration-ownership.md")
				b, _ := os.ReadFile(p)
				os.WriteFile(p, []byte(strings.ReplaceAll(string(b), `<a id="`+last.ID+`"></a>`, "")), 0o644)
			},
			"has no operator section",
		},
		{
			"broken internal link",
			func(t *testing.T, root string) {
				write(t, root, "docs/extra.md", "[nope](missing-file.md)\n")
			},
			"link target missing",
		},
		{
			"broken anchor",
			func(t *testing.T, root string) {
				write(t, root, "docs/extra.md", "[nope](#missing-heading)\n")
			},
			"anchor not found",
		},
		{
			"broken github link",
			func(t *testing.T, root string) {
				write(t, root, "docs/extra.md", "[nope](https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/nope-missing.go)\n")
			},
			"github link target missing",
		},
		{
			"file reference missing",
			func(t *testing.T, root string) {
				write(t, root, "docs/extra.md", "see `backend/services/nope_missing.go` for details.\n")
			},
			"file reference missing",
		},
		{
			"unannotated toolchain command",
			func(t *testing.T, root string) {
				write(t, root, "docs/deployment.md", "# Deployment\n\nRun it:\n\n```sh\ngo run ./cmd/something\n```\n")
			},
			"needs a repo checkout",
		},
		{
			"documented bring-up command drifts from CI",
			func(t *testing.T, root string) {
				p := filepath.Join(root, ".github/workflows/deploy-smoke.yml")
				b, _ := os.ReadFile(p)
				os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "docker compose up -d --build", "docker compose up")), 0o644)
			},
			"no longer runs the bring-up command",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := fixtureTree(t)
			c.breakF(t, root)
			got := collectMatches(t, root)
			if !strings.Contains(got, c.match) {
				t.Fatalf("findings %q do not mention %q", got, c.match)
			}
		})
	}
}

func TestFindRepoRoot(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "backend/go.mod", "module test\n")
	sub := filepath.Join(root, "backend", "internal", "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("findRepoRoot() = %q, want %q", got, root)
	}
}

// TestCheckFileLinksVariantTargets covers the html-mapping and absolute-link
// branches of checkFileLinks.
func TestCheckFileLinksVariantTargets(t *testing.T) {
	root := fixtureTree(t)
	// .html link mapping onto a real page plus a real anchor.
	write(t, root, "docs/some/page.md", "# Page\n\n## A section\n")
	write(t, root, "docs/from.md", "[here](some/page.html#a-section) and [abs](/CLAUDE.md) and [bad](some/missing.html)\n")
	findings := collectFindings(root)
	got := strings.Join(findings, "\n")
	if strings.Contains(got, "some/page.html#a-section") {
		t.Errorf("html-mapped anchor should resolve: %s", got)
	}
	if !strings.Contains(got, "missing.html") {
		t.Errorf("missing .html target should be reported: %s", got)
	}
}

func TestCheckFileReferencesRelaxation(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "backend/internal/fsguard/fsguard.go", "package fsguard\n")
	write(t, root, "backend/cmd/backfill-search-index/main.go", "package main\n")
	// Go-symbol qualified reference, an alias cmd/, a glob, and prose dirs.
	write(t, root, "docs/dev.md", `see backend/internal/fsguard.NetworkFilesystemWarning and cmd/backfill-search-index; globs backend/database/migrate_*_test.go are fine; also `+"`"+"`"+"docs/backend\n` and backend/frontend are not files.\n")
	// a genuinely missing file must still be caught
	write(t, root, "docs/dev2.md", "`backend/services/nope_missing.go`\n")
	findings := collectFindings(root)
	got := strings.Join(findings, "\n")
	if strings.Contains(got, "fsguard.NetworkFilesystemWarning") {
		t.Errorf("go-symbol reference should relax to package: %s", got)
	}
	if strings.Contains(got, "backfill-search-index") {
		t.Errorf("cmd/ alias should resolve: %s", got)
	}
	if !strings.Contains(got, "nope_missing.go") {
		t.Errorf("missing file reference should be reported: %s", got)
	}
}

func TestCheckCommandBlocksAnnotationVariants(t *testing.T) {
	newRoot := func(doc string) string {
		root := t.TempDir()
		p := filepath.Join(root, "docs", "deployment.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}

	cases := []struct {
		name   string
		doc    string
		broken bool
	}{
		{
			"section heading names the toolchain",
			"# Deployment\n\n### Rebuild with a Go toolchain\n\n```sh\ngo run ./cmd/backfill-search-index\n```\n",
			false,
		},
		{
			"shell comment names the toolchain",
			"# Deployment\n\n```sh\n# from a checkout of the repo with a Go toolchain\nmake backup\n```\n",
			false,
		},
		{
			"non-shell fence is not checked",
			"# Deployment\n\n```nginx\nlocation / { proxy_pass http://localhost:7300; }\n```\n",
			false,
		},
		{
			"unannotated toolchain command is broken",
			"# Deployment\n\n```sh\ngo run ./cmd/backfill-search-index\n```\n",
			true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := checkCommandBlocks(newRoot(c.doc))
			if c.broken && len(findings) == 0 {
				t.Errorf("expected an unannotated-toolchain finding for %q", c.doc)
			}
			if !c.broken && len(findings) > 0 {
				t.Errorf("did not expect findings for %q: %v", c.doc, findings)
			}
		})
	}

	// A document-level declaration anywhere above the block licenses it.
	doc := "# Deployment\n\nCommands here assume a checkout of this repo with a Go toolchain.\n\n## Rebuild\n\n```sh\ngo run ./cmd/backfill-search-index\n```\n"
	if findings := checkCommandBlocks(newRoot(doc)); len(findings) > 0 {
		t.Errorf("doc-level declaration should license blocks below it: %v", findings)
	}
}

func TestDockerEngineRowFloor(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			"matrix row",
			"| **Docker Engine** | `>=23.0` | why |\n",
			"23.0",
		},
		{
			"operator row with a space",
			"| **Docker Engine** | `>= 23.0` | 23.0 is when Compose V2 landed |\n",
			"23.0",
		},
		{
			"no row",
			"| **Docker Compose** | V2 |\n",
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			floors := dockerEngineRowFloor(c.text)
			if c.want == "" {
				if len(floors) != 0 {
					t.Fatalf("want no floors, got %v", floors)
				}
				return
			}
			if len(floors) != 1 || floors[0] != c.want {
				t.Fatalf("dockerEngineRowFloor(%q) = %v, want [%s]", c.text, floors, c.want)
			}
		})
	}
}

// TestDockerEngineFloorRegression is the issue #941 regression: the operator
// page stated `>= 22.0` in its Docker Engine row while the matrix and a note
// elsewhere said 23.0, and the old token Contains check passed. The structural
// row check must catch the mismatch, a competing second floor, and a missing
// row on either side.
func TestDockerEngineFloorRegression(t *testing.T) {
	matrix := "# matrix\n\n| **Docker Engine** | `>=23.0` | why |\n"
	page := "# page\n\n| **Docker Engine** | `>= 23.0` | why |\n"

	t.Run("matching passes", func(t *testing.T) {
		if got := checkDockerEngineFloor(matrix, page); len(got) != 0 {
			t.Fatalf("matching Docker rows should not be flagged: %v", got)
		}
	})
	t.Run("wrong floor is caught even with the right number in prose", func(t *testing.T) {
		// The 23.0 note is exactly the decoy that defeated the old check.
		badPage := "# page\n\n23.0 is when Compose V2 landed.\n\n| **Docker Engine** | `>= 22.0` | why |\n"
		got := strings.Join(checkDockerEngineFloor(matrix, badPage), "\n")
		if !strings.Contains(got, "does not state the matrix's floor") {
			t.Fatalf("expected a Docker floor mismatch finding, got: %s", got)
		}
	})
	t.Run("competing floors are rejected", func(t *testing.T) {
		badPage := "# page\n\n| **Docker Engine** | `>= 22.0` and `>= 23.0` | why |\n"
		got := strings.Join(checkDockerEngineFloor(matrix, badPage), "\n")
		if !strings.Contains(got, "more than one competing floor") {
			t.Fatalf("expected a competing-floor finding, got: %s", got)
		}
	})
	t.Run("missing page row is rejected", func(t *testing.T) {
		got := strings.Join(checkDockerEngineFloor(matrix, "# page\n\nno docker row\n"), "\n")
		if !strings.Contains(got, "has no **Docker Engine** row") {
			t.Fatalf("expected a missing-row finding, got: %s", got)
		}
	})
	t.Run("missing matrix row is rejected", func(t *testing.T) {
		got := strings.Join(checkDockerEngineFloor("# matrix\n\nno docker row\n", page), "\n")
		if !strings.Contains(got, "engineering matrix has no **Docker Engine** row") {
			t.Fatalf("expected a missing-matrix-row finding, got: %s", got)
		}
	})
}

func TestSupportedVersionsDriftMatrixMissingToken(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "docs/development/supported-runtime-matrix.md", "# matrix\n")
	got := strings.Join(collectFindings(root), "\n")
	if !strings.Contains(got, "no longer states") {
		t.Errorf("expected a matrix drift finding, got: %s", got)
	}
}

func TestIntegrationOwnershipMissingPage(t *testing.T) {
	root := fixtureTree(t)
	os.Remove(filepath.Join(root, "docs", "integration-ownership.md"))
	got := strings.Join(collectFindings(root), "\n")
	if !strings.Contains(got, "missing") {
		t.Errorf("expected a missing-page finding, got: %s", got)
	}
}

func TestCheckCICommandBindingDrift(t *testing.T) {
	root := fixtureTree(t)
	// Drop the upgrade pair from deployment.md.
	p := filepath.Join(root, "docs", "deployment.md")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "docker compose pull", "docker compose build")), 0o644)
	got := strings.Join(collectFindings(root), "\n")
	if !strings.Contains(got, "no longer documents the upgrade command pair") {
		t.Errorf("expected an upgrade-pair finding, got: %s", got)
	}

	// Drop the bring-up command from getting-started.md.
	root2 := fixtureTree(t)
	p2 := filepath.Join(root2, "docs", "getting-started.md")
	b2, _ := os.ReadFile(p2)
	os.WriteFile(p2, []byte(strings.ReplaceAll(string(b2), "docker compose up -d --build", "docker compose up")), 0o644)
	got2 := strings.Join(collectFindings(root2), "\n")
	if !strings.Contains(got2, "getting-started.md no longer contains") {
		t.Errorf("expected a getting-started finding, got: %s", got2)
	}
}

func TestIsShellAndFirstToolchainCommand(t *testing.T) {
	if !isShell("") || !isShell("sh") || !isShell("bash") || !isShell("console") {
		t.Error("expected shell info strings to be recognized")
	}
	if isShell("nginx") || isShell("json") {
		t.Error("expected non-shell info strings to be rejected")
	}
	if got := firstToolchainCommand("# comment\ncd backend && make backup"); got != "cd backend" {
		t.Errorf("firstToolchainCommand = %q, want the first command", got)
	}
}

func TestCheckFileReferencesMissingDirNotFlagged(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "docs/plain.md", "the backend/frontend and docs/x boundaries are prose\n")
	got := strings.Join(collectFindings(root), "\n")
	for _, not := range []string{"plain.md", "backend/frontend"} {
		if strings.Contains(got, not) {
			t.Errorf("prose area enumeration should not be flagged: %s", got)
		}
	}
}

func TestFindRepoRootNoRoot(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	// An empty temp dir has no backend/go.mod at or above it.
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := findRepoRoot(); err == nil {
		t.Fatal("expected an error when no repo root is above the cwd")
	}
}

func TestFirstToolchainCommandNoMatch(t *testing.T) {
	if got := firstToolchainCommand("# just a comment\nrsync -a src/ dst/"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestSupportedVersionsDriftMatrixMissing(t *testing.T) {
	root := fixtureTree(t)
	os.Remove(filepath.Join(root, "docs", "development", "supported-runtime-matrix.md"))
	got := strings.Join(collectFindings(root), "\n")
	if !strings.Contains(got, "cannot read") {
		t.Errorf("expected a read-error finding, got: %s", got)
	}
}

func TestCheckFileLinksNonLocalDestinations(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "docs/extra.md", "[mail](mailto:someone@example.com) [ext](https://example.com/x) [hash](#) [self](#existing)\n# existing\n")
	findings := collectFindings(root)
	for _, f := range findings {
		if strings.Contains(f, "extra.md") {
			t.Errorf("external/mailto/# links should not be flagged: %v", findings)
		}
	}
}

func TestCheckFileLinksCrossFileAnchorMissingAndEmptyDest(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "docs/some.md", "# Page\n")
	// Cross-file .html anchor that does not exist, plus an empty destination
	// (skipped), plus a reference-style label that is not a link.
	write(t, root, "docs/from.md", "[bad](some.html#nope) [empty]() [ref][some] more text\n")
	findings := collectFindings(root)
	got := strings.Join(findings, "\n")
	if !strings.Contains(got, "some.html#nope") {
		t.Errorf("missing cross-file anchor should be reported: %s", got)
	}
}

func TestAnnotatedProseNoteAndWindow(t *testing.T) {
	// A toolchain note in the prose between heading and block licenses it.
	doc := "# Deployment\n\nThis section needs a checkout of this repo.\n\n```sh\nmake backup\n```\n"
	root := t.TempDir()
	p := filepath.Join(root, "docs", "deployment.md")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(doc), 0o644)
	if findings := checkCommandBlocks(root); len(findings) > 0 {
		t.Errorf("prose note should license the block: %v", findings)
	}
}

func TestCommandBlockAnnotationWindowClipped(t *testing.T) {
	// The toolchain note sits more than 60 lines above the block: the scan
	// window is clipped and the block must be flagged anyway.
	var b strings.Builder
	b.WriteString("# Deployment\n\nThis whole document needs a checkout of this repo.\n")
	for i := 0; i < 70; i++ {
		b.WriteString("filler paragraph line.\n")
	}
	b.WriteString("```sh\ngo run ./cmd/backfill-search-index\n```\n")
	root := t.TempDir()
	p := filepath.Join(root, "docs", "deployment.md")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(b.String()), 0o644)
	if findings := checkCommandBlocks(root); len(findings) == 0 {
		t.Error("expected a finding when the annotation is beyond the scan window")
	}
}

func TestFileReferencesDuplicateAndTestdataSkip(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "docs/plain.md", "missing `backend/services/nope_missing.go` twice `backend/services/nope_missing.go`, plus `testdata/budgets.json`.\n")
	got := strings.Join(collectFindings(root), "\n")
	if !strings.Contains(got, "nope_missing.go") {
		t.Errorf("missing reference should be reported once: %s", got)
	}
	if strings.Contains(got, "budgets.json") {
		t.Errorf("ambiguous testdata/ references should be skipped: %s", got)
	}
}

func TestSupportedVersionsDriftPageMissingFloor(t *testing.T) {
	root := t.TempDir()
	matrix := "# Supported runtime matrix\n\n"
	page := "# Supported versions\n\n"
	for i, f := range operatorFloors {
		matrix += fmt.Sprintf("| %s | %s |\n", f.label, f.matrix)
		if i == 0 {
			continue
		}
		page += fmt.Sprintf("%s: %s\n", f.label, f.page)
	}
	write(t, root, "docs/development/supported-runtime-matrix.md", matrix)
	write(t, root, "docs/supported-versions.md", page)
	got := strings.Join(checkSupportedVersionsDrift(root), "\n")
	if !strings.Contains(got, "does not state") || !strings.Contains(got, operatorFloors[0].label) {
		t.Errorf("expected page-missing finding for %q, got: %s", operatorFloors[0].label, got)
	}
}

func TestDockerEngineFloorMissingPageRow(t *testing.T) {
	matrix := "# Supported runtime matrix\n\n| **Docker Engine** | `>=23.0` | why |\n"
	page := "# Supported versions\n\n"
	got := strings.Join(checkDockerEngineFloor(matrix, page), "\n")
	if !strings.Contains(got, "has no **Docker Engine** row") {
		t.Errorf("expected missing-row finding, got: %s", got)
	}
}

func TestRunAtPrintsFindingsAndReturnsError(t *testing.T) {
	root := fixtureTree(t)
	write(t, root, "docs/bad.md", "[bad](missing.md)\n")
	if err := runAt(root); err == nil {
		t.Fatal("expected runAt to return an error when there are findings")
	}
}

func TestDockerEngineFloorMultipleMatrixRows(t *testing.T) {
	matrix := "# Supported runtime matrix\n\n| **Docker Engine** | `>=23.0` (was `>=24.0`) | why |\n"
	page := "# Supported versions\n\n| **Docker Engine** | `>= 23.0` | why |\n"
	got := strings.Join(checkDockerEngineFloor(matrix, page), "\n")
	if !strings.Contains(got, "engineering matrix Docker Engine row states more than one competing floor") {
		t.Errorf("expected multiple-matrix-floor finding, got: %s", got)
	}
}
