package agentcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/schemafixture"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// validTree builds the registry's files: AGENTS.md entries at the target line
// count and CLAUDE.md aliases in symlink form. It also writes the canonical
// sources the Suite and Fact checks read, so an AGENTS-only fixture is a clean
// tree.
func validTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, af := range Registry() {
		if af.AliasOf != "" {
			abs := filepath.Join(root, filepath.FromSlash(af.Path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			link, err := filepath.Rel(filepath.Dir(abs), filepath.Join(root, filepath.FromSlash(af.AliasOf)))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(link, abs); err != nil {
				t.Fatal(err)
			}
			continue
		}
		writeFile(t, root, af.Path, strings.Repeat("context\n", af.TargetLines))
	}
	writeFixturePrereqs(t, root)
	return root
}

func writeFixturePrereqs(t *testing.T, root string) {
	t.Helper()
	locales := []string{"de", "en", "es", "fr", "it"}
	for _, code := range locales {
		writeFile(t, root, "frontend/src/i18n/locales/"+code+".json", "{}\n")
	}
	writeFile(t, root, "frontend/e2e/global-setup.ts", "export const APP_ORIGIN = 'http://localhost:7300';\n")
	writeFile(t, root, "backend/Makefile", "gen-contract-fixtures:\n\t@echo fixtures\n")
	writeFile(t, root, "backend/database/AGENTS.md", fmt.Sprintf(
		"Upgrades are supported from `%s` (migration `%06d`) and later.\n",
		schemafixture.FloorTag, schemafixture.FloorVersion))
	writeFile(t, root, "frontend/AGENTS.md",
		"All five locale files (`en`, `de`, `es`, `fr`, `it`) are translated.\n"+
			"The e2e setup hardcodes `http://localhost:7300`.\n")
	writeFile(t, root, ".github/workflows/fixture.yml", strings.Join([]string{
		"name: Fixture",
		"on: push",
		"jobs:",
		"  upgrade-matrix:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: go test ./internal/schemafixture/ -count=1",
		"  schemafixture-suite:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: go test ./internal/schemafixture/ -count=1",
		"  backend-tests:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: go test ./...",
		"  backend-checks:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: go test -run '^$' -bench . -benchtime=1x ./...",
		"  frontend:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: npx tsc --noEmit",
		"  docs-citations:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: go run ./cmd/citecheck",
		"      - run: go run ./cmd/releasegatecheck",
		"      - run: go run ./cmd/depexceptions",
		"      - run: go run ./cmd/docscheck",
		"      - run: go run ./cmd/governancecheck",
		"      - run: go run ./cmd/nightlyalertcheck",
		"      - run: go run ./cmd/mainalertcheck",
		"  release:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - run: go run ./cmd/releasegatecheck",
	}, "\n")+"\n")
	writeFile(t, root, ".github/release-gates.json", `{"gates":[{"tier":"per-pr","mandatory":true,"check_context":"Backend (Go)"}]}`)
	writeFile(t, root, ".github/rulesets/main-protection.json", `{"rules":[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Backend (Go)"}]}}]}`)
	if err := os.MkdirAll(filepath.Join(root, SkillsRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.agents/skills", filepath.Join(root, ClaudeSkillsRoot)); err != nil {
		t.Fatal(err)
	}
}

func findings(root string) []string {
	var out []string
	for _, f := range Check(root) {
		out = append(out, f.String())
	}
	return out
}

func TestCleanTreePasses(t *testing.T) {
	root := validTree(t)
	if got := findings(root); len(got) != 0 {
		t.Fatalf("clean tree should have no findings, got:\n%s", strings.Join(got, "\n"))
	}
}

func TestLinesBetweenTargetAndHardMaximumPass(t *testing.T) {
	for _, lines := range []int{0, 1, 100, 150, 200} {
		root := validTree(t)
		writeFile(t, root, "backend/AGENTS.md", strings.Repeat("context\n", lines))
		if got := findings(root); len(got) != 0 {
			t.Errorf("%d lines should pass (target is guidance, only the 200-line maximum fails):\n%s",
				lines, strings.Join(got, "\n"))
		}
	}
}

func TestOverHardMaximumFailsAtFirstOverBudgetLine(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/AGENTS.md", strings.Repeat("context\n", 201))
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "backend/AGENTS.md:201: exceeds hard maximum of 200 lines (201)") {
		t.Fatalf("findings do not report the over-budget line: %s", got)
	}
}

func TestMissingRegisteredFileFails(t *testing.T) {
	for _, rel := range []string{"backend/AGENTS.md", "backend/CLAUDE.md"} {
		t.Run(rel, func(t *testing.T) {
			root := validTree(t)
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
			got := strings.Join(findings(root), "\n")
			if !strings.Contains(got, rel+":1: registered") {
				t.Fatalf("missing %s was not reported: %s", rel, got)
			}
		})
	}
}

func TestPlainAliasMustBeTheOneLineImport(t *testing.T) {
	root := validTree(t)
	rel := "backend/CLAUDE.md"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, rel, "Read backend/AGENTS.md instead.\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, rel+":1: alias must be a symlink to backend/AGENTS.md or exactly the one-line @AGENTS.md import") {
		t.Fatalf("plain alias was not rejected: %s", got)
	}
}

func TestAtImportFallbackPasses(t *testing.T) {
	for _, body := range []string{"@AGENTS.md", "@AGENTS.md\n", "@AGENTS.md\r\n"} {
		root := validTree(t)
		rel := "frontend/CLAUDE.md"
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, rel, body)
		if got := findings(root); len(got) != 0 {
			t.Errorf("%q should be accepted as the fallback import:\n%s", body, strings.Join(got, "\n"))
		}
	}
}

func TestAliasMustResolveToColocatedAgentsFile(t *testing.T) {
	root := validTree(t)
	rel := "frontend/CLAUDE.md"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../backend/AGENTS.md", filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, rel+":1: alias resolves to ../backend/AGENTS.md, want frontend/AGENTS.md") {
		t.Fatalf("wrong alias target was not reported: %s", got)
	}
}

func TestAbsoluteSymlinkAliasPasses(t *testing.T) {
	root := validTree(t)
	rel := "docs/CLAUDE.md"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "docs/AGENTS.md"), filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if got := findings(root); len(got) != 0 {
		t.Fatalf("absolute symlink should resolve: %s", strings.Join(got, "\n"))
	}
}

func TestBrokenAliasSymlinkFails(t *testing.T) {
	root := validTree(t)
	rel := "docs/CLAUDE.md"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing/AGENTS.md", filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, rel+":1: alias target missing: missing/AGENTS.md") {
		t.Fatalf("broken alias symlink was not reported: %s", got)
	}
}

func TestAliasDirectoryIsUnreadable(t *testing.T) {
	root := validTree(t)
	rel := "android/CLAUDE.md"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, rel+":1: unreadable:") {
		t.Fatalf("directory alias was not reported as unreadable: %s", got)
	}
}

func TestAgentFileThatIsADirectoryIsUnreadable(t *testing.T) {
	root := validTree(t)
	rel := "docs/AGENTS.md"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, rel+":1: unreadable:") {
		t.Fatalf("directory AGENTS.md was not reported as unreadable: %s", got)
	}
}

func TestUnregisteredAgentsFileFails(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "extra/AGENTS.md", "# Extra\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "extra/AGENTS.md:1: unregistered AGENTS.md") {
		t.Fatalf("unregistered AGENTS.md was not reported: %s", got)
	}
}

func TestParityWalkSkipsBuildAndDependencyDirs(t *testing.T) {
	root := validTree(t)
	for _, dir := range []string{".git", "node_modules/pkg", "build", "dist", "out", "target", "vendor/dep", ".gradle"} {
		writeFile(t, root, filepath.ToSlash(filepath.Join(dir, "AGENTS.md")), "# Ignored\n")
	}
	if got := findings(root); len(got) != 0 {
		t.Fatalf("excluded directories should not be walked: %s", strings.Join(got, "\n"))
	}
}

func TestWalkAgentFilesReturnsSortedRepoRelativePaths(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/database/deeper/AGENTS.md", "# Deep\n")
	got, err := WalkAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".github/AGENTS.md",
		"AGENTS.md",
		"android/AGENTS.md",
		"backend/AGENTS.md",
		"backend/database/AGENTS.md",
		"backend/database/deeper/AGENTS.md",
		"docs/AGENTS.md",
		"frontend/AGENTS.md",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("WalkAgentFiles() = %v, want %v", got, want)
	}
}

func TestWalkAgentFilesReportsWalkError(t *testing.T) {
	_, err := WalkAgentFiles(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("walking a missing root should return an error")
	}
}

func TestExplainShowsTheExpectedLoadPath(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/controllers/contact_controller.go", "package controllers\n")
	writeFile(t, root, "backend/database/migrations/001.up.sql", "select 1;\n")

	cases := []struct {
		target string
		want   []string
	}{
		{"backend/controllers/contact_controller.go", []string{"AGENTS.md", "backend/AGENTS.md"}},
		{"backend/database/migrations/001.up.sql", []string{"AGENTS.md", "backend/AGENTS.md", "backend/database/AGENTS.md"}},
		{"README.md", []string{"AGENTS.md"}},
	}
	for _, c := range cases {
		got, err := Explain(root, c.target)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("Explain(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}

func TestExplainAcceptsAnAbsolutePath(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "frontend/src/App.tsx", "export {}\n")
	got, err := Explain(root, filepath.Join(root, "frontend/src/App.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "AGENTS.md,frontend/AGENTS.md" {
		t.Fatalf("Explain() = %v", got)
	}
}

func TestExplainRejectsAPathOutsideTheRepository(t *testing.T) {
	root := validTree(t)
	if _, err := Explain(root, filepath.Join(t.TempDir(), "outside.go")); err == nil {
		t.Fatal("a path outside the root should be rejected")
	}
}

func TestFindRepoRootUsesGitAndAgentsMarker(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".git", "gitdir: /tmp/worktree\n")
	writeFile(t, root, "AGENTS.md", "# Root\n")
	sub := filepath.Join(root, "backend", "internal", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRepoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("FindRepoRoot() = %q, want %q", got, root)
	}
}

func TestFindRepoRootIgnoresGoMod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "backend/go.mod", "module test\n")
	writeFile(t, root, "AGENTS.md", "# Root without .git\n")
	if _, err := FindRepoRoot(root); err == nil {
		t.Fatal("go.mod alone must not identify the repository root")
	}
}

func TestRepositoryIsClean(t *testing.T) {
	root, err := FindRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if got := findings(root); len(got) != 0 {
		t.Fatalf("the real repository has context findings:\n%s", strings.Join(got, "\n"))
	}
}

func TestFileWithoutTrailingNewlineAtTheLimitPasses(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/AGENTS.md", strings.Repeat("context\n", 199)+"context")
	if got := findings(root); len(got) != 0 {
		t.Fatalf("200 lines without a trailing newline should pass: %s", strings.Join(got, "\n"))
	}
}

func TestAliasWithANonDirectoryParentIsUnreadable(t *testing.T) {
	root := validTree(t)
	if err := os.RemoveAll(filepath.Join(root, "frontend")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "frontend", "not a directory\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "frontend/CLAUDE.md:1: unreadable:") {
		t.Fatalf("an alias whose parent is a file should be reported unreadable: %s", got)
	}
}

func TestCheckReportsAWalkError(t *testing.T) {
	got := strings.Join(findings(filepath.Join(t.TempDir(), "missing")), "\n")
	if !strings.Contains(got, "could not walk the repository") {
		t.Fatalf("a missing root should surface the walk error: %s", got)
	}
}
