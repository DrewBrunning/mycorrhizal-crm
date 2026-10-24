package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/agentcontext"
	"mycorrhizal/internal/schemafixture"
)

func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, root, ".git", "gitdir: /tmp/worktree\n")
	for _, af := range agentcontext.Registry() {
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
		writeFixtureFile(t, root, af.Path, strings.Repeat("context\n", af.TargetLines))
	}
	writeFixturePrereqs(t, root)
	return root
}

func writeFixturePrereqs(t *testing.T, root string) {
	t.Helper()
	for _, code := range []string{"de", "en", "es", "fr", "it"} {
		writeFixtureFile(t, root, "frontend/src/i18n/locales/"+code+".json", "{}\n")
	}
	writeFixtureFile(t, root, "frontend/e2e/global-setup.ts", "export const APP_ORIGIN = 'http://localhost:7300';\n")
	writeFixtureFile(t, root, "backend/database/AGENTS.md", fmt.Sprintf(
		"Upgrades are supported from `%s` (migration `%06d`) and later.\n",
		schemafixture.FloorTag, schemafixture.FloorVersion))
	writeFixtureFile(t, root, "frontend/AGENTS.md",
		"All five locale files (`en`, `de`, `es`, `fr`, `it`) are translated.\n"+
			"The e2e setup hardcodes `http://localhost:7300`.\n")
	writeFixtureFile(t, root, ".github/workflows/fixture.yml", strings.Join([]string{
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
	writeFixtureFile(t, root, ".github/release-gates.json", `{"gates":[{"tier":"per-pr","mandatory":true,"check_context":"Backend (Go)"}]}`)
	writeFixtureFile(t, root, ".github/rulesets/main-protection.json", `{"rules":[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Backend (Go)"}]}}]}`)
	if err := os.MkdirAll(filepath.Join(root, agentcontext.SkillsRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.agents/skills", filepath.Join(root, agentcontext.ClaudeSkillsRoot)); err != nil {
		t.Fatal(err)
	}
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunCleanTreeExitsZero(t *testing.T) {
	root := fixtureRoot(t)
	code, stdout, stderr := runCLI(t, "-root", root)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("run() = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRunReportsFindingsAndExitsOne(t *testing.T) {
	root := fixtureRoot(t)
	if err := os.Remove(filepath.Join(root, "backend/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCLI(t, "-root", root)
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stdout, "backend/AGENTS.md:1: registered AGENTS.md is missing from disk") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunListPrintsFilesAndBudgets(t *testing.T) {
	root := fixtureRoot(t)
	code, stdout, _ := runCLI(t, "-root", root, "-list")
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	for _, want := range []string{
		"AGENTS.md\ttarget=100\tmax=200\n",
		"CLAUDE.md\talias=AGENTS.md\n",
		"backend/database/CLAUDE.md\talias=backend/database/AGENTS.md\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunExplainRepoRelative(t *testing.T) {
	root := fixtureRoot(t)
	writeFixtureFile(t, root, "backend/controllers/contact_controller.go", "package controllers\n")
	code, stdout, stderr := runCLI(t, "-root", root, "-explain", "backend/controllers/contact_controller.go")
	if code != 0 {
		t.Fatalf("run() = %d, stderr %q", code, stderr)
	}
	if stdout != "AGENTS.md\nbackend/AGENTS.md\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunExplainFromASubdirectory(t *testing.T) {
	root := fixtureRoot(t)
	writeFixtureFile(t, root, "backend/controllers/contact_controller.go", "package controllers\n")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(filepath.Join(root, "backend")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "-root", root, "-explain", "controllers/contact_controller.go")
	if code != 0 {
		t.Fatalf("run() = %d, stderr %q", code, stderr)
	}
	if stdout != "AGENTS.md\nbackend/AGENTS.md\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunExplainRejectsOutsidePath(t *testing.T) {
	root := fixtureRoot(t)
	outside := filepath.Join(t.TempDir(), "x.go")
	code, _, stderr := runCLI(t, "-root", root, "-explain", outside)
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr, "outside the repository root") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunFindsRootFromASubdirectory(t *testing.T) {
	root := fixtureRoot(t)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(filepath.Join(root, "backend")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t)
	if code != 0 {
		t.Fatalf("run() = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRunMissingRootReportsEveryFinding(t *testing.T) {
	code, stdout, _ := runCLI(t, "-root", filepath.Join(t.TempDir(), "missing"))
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stdout, "could not walk the repository") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunFlagErrors(t *testing.T) {
	if code, _, _ := runCLI(t, "-nope"); code != 2 {
		t.Errorf("unknown flag: run() = %d, want 2", code)
	}
	if code, _, _ := runCLI(t, "-h"); code != 0 {
		t.Errorf("-h: run() = %d, want 0", code)
	}
}

func TestResolveUserPath(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "backend/AGENTS.md", "# Backend\n")
	writeFixtureFile(t, root, "backend/controllers/x.go", "package controllers\n")
	if got := resolveUserPath(root, "backend/controllers/x.go"); got != filepath.Join(root, "backend/controllers/x.go") {
		t.Errorf("repo-relative path = %q", got)
	}
	abs := filepath.Join(root, "elsewhere.go")
	if got := resolveUserPath(root, abs); got != abs {
		t.Errorf("absolute path = %q", got)
	}
	if got := resolveUserPath(root, "not-there.go"); got != filepath.Join(root, "not-there.go") {
		t.Errorf("missing path should fall back to repo-relative, got %q", got)
	}
}

func TestRunReportsWhenRootDiscoveryFails(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t)
	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr, "could not locate repository root") {
		t.Fatalf("stderr = %q", stderr)
	}
}
