package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/prepr"
)

const testFilters = `backend:
  - 'backend/**'
frontend:
  - 'frontend/**'
workflows:
  - '.github/**'
`

// newRepo creates a throwaway git repository with two commits: the second
// touches a backend source file and its test.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "commit.gpgsign", "false")

	write(t, filepath.Join(dir, ".github", "filters.yaml"), testFilters)
	write(t, filepath.Join(dir, "README.md"), "test\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "initial")

	write(t, filepath.Join(dir, "backend", "go.mod"), "module mycorrhizal\n\ngo 1.26.0\n")
	write(t, filepath.Join(dir, "backend", "services", "foo.go"), "package services\n")
	write(t, filepath.Join(dir, "backend", "services", "foo_test.go"), "package services\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "backend change")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRunDryRunFast(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	code := run([]string{"--fast", "--dry-run", "--root", dir, "--base", "HEAD~1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	for _, want := range []string{
		"selected areas: backend",
		"planned steps:",
		"cd backend && go build ./...",
		"go run ./cmd/docscheck",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunDryRunFullPrintsRatchetAndStress(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	code := run([]string{"--full", "--dry-run", "--root", dir, "--base", "HEAD~1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	for _, want := range []string{
		"go run ./cmd/coverageratchet",
		"go run ./cmd/patchcov",
		"go test -race -count=10 -shuffle=on ./services",
		"make gen-coverage-baseline",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunDryRunFix(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	code := run([]string{"--focused", "--fix", "--dry-run", "--root", dir, "--base", "HEAD~1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	for _, want := range []string{
		"cd backend && gofmt -w .",
		"golangci-lint run --fix ./...",
		"fix mode: formatters may modify the working tree",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunRejectsConflictingModes(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--fast", "--full"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "choose exactly one of --fast, --focused, or --full") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunRejectsFocusedAndFull(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--focused", "--full"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "choose exactly one of") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunRejectsFlagLikeBase(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--base", "--evil"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestRunMissingFilters(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	if code := run([]string{"--root", dir}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "filters.yaml") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunBadBase(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"--root", dir, "--base", "does-not-exist"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "git diff") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunEmptyDiffRunsGovernanceOnly(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	code := run([]string{"--dry-run", "--root", dir, "--base", "HEAD"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "no changed paths") {
		t.Errorf("output = %s", out.String())
	}
	if strings.Contains(out.String(), "[backend]") {
		t.Errorf("empty diff selected an area:\n%s", out.String())
	}
}

// TestRunExecutesSteps exercises the non-dry path and the failure summary.
// The throwaway backend has no toolchain files, so the commands fail fast.
func TestRunExecutesSteps(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	code := run([]string{"--fast", "--root", dir, "--base", "HEAD~1"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("run = %d, want 1\nout: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "FAILED — reproduce with:") {
		t.Errorf("output missing failure summary:\n%s", out.String())
	}
}

func TestRunner(t *testing.T) {
	root := t.TempDir()

	if err := runner(root)(prepr.Step{Command: prepr.Command{Argv: []string{"true"}}}); err != nil {
		t.Errorf("argv runner: %v", err)
	}
	if err := runner(root)(prepr.Step{Command: prepr.Command{Shell: "true"}}); err != nil {
		t.Errorf("shell runner: %v", err)
	}
	if err := runner(root)(prepr.Step{Command: prepr.Command{Argv: []string{"true"}, Repeat: 2}}); err != nil {
		t.Errorf("repeat runner: %v", err)
	}
	if err := runner(root)(prepr.Step{Command: prepr.Command{Argv: []string{"false"}}}); err == nil {
		t.Error("runner returned nil for a failing command")
	}
	if err := runner(root)(prepr.Step{Command: prepr.Command{Dir: "does-not-exist", Argv: []string{"true"}}}); err == nil {
		t.Error("runner returned nil for a missing working directory")
	}
}

func TestCommandFor(t *testing.T) {
	if got := commandFor(prepr.Step{Command: prepr.Command{Argv: []string{"go", "build"}}}); strings.Join(got.Args, " ") != "go build" {
		t.Errorf("argv command = %v", got.Args)
	}
	if got := commandFor(prepr.Step{Command: prepr.Command{Shell: "echo hi"}}); strings.Join(got.Args, " ") != "bash -c echo hi" {
		t.Errorf("shell command = %v", got.Args)
	}
}

func TestDirForDisplay(t *testing.T) {
	if got := dirForDisplay(""); got != "." {
		t.Errorf("dirForDisplay(\"\") = %q", got)
	}
	if got := dirForDisplay("backend"); got != "backend" {
		t.Errorf("dirForDisplay(backend) = %q", got)
	}
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backend", "go.mod")); err != nil {
		t.Fatalf("root %s has no backend/go.mod: %v", root, err)
	}

	t.Chdir(t.TempDir())
	if _, err := findRepoRoot(); err == nil {
		t.Fatal("findRepoRoot succeeded outside the repository")
	}
}

func TestRunResolvesRootFromCwd(t *testing.T) {
	dir := newRepo(t)
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	if code := run([]string{"--dry-run", "--base", "HEAD~1"}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "selected areas: backend") {
		t.Errorf("output = %s", out.String())
	}
}

func TestRunMalformedFilters(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".github", "filters.yaml"), "backend: not-a-list\n")
	var out, errOut bytes.Buffer
	if code := run([]string{"--root", dir}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestRunDryRunShowsReminderSkipAndNote(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "commit.gpgsign", "false")
	write(t, filepath.Join(dir, ".github", "filters.yaml"), `backend:
  - 'backend/**'
infra:
  - 'docker-compose*.yml'
openapi:
  - 'backend/openapi.yaml'
changelog-relevant:
  - 'backend/config/config.go'
`)
	write(t, filepath.Join(dir, "README.md"), "test\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "initial")
	write(t, filepath.Join(dir, "backend", "go.mod"), "module mycorrhizal\n\ngo 1.26.0\n")
	write(t, filepath.Join(dir, "backend", "config", "config.go"), "package config\n")
	write(t, filepath.Join(dir, "backend", "openapi.yaml"), "openapi: 3.0.0\n")
	write(t, filepath.Join(dir, "docker-compose.yml"), "services: {}\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "mixed change")

	var out, errOut bytes.Buffer
	if code := run([]string{"--fast", "--dry-run", "--root", dir, "--base", "HEAD~1"}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	for _, want := range []string{"reminder:", "-- skipped infra:", "-- note: openapi implies"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestChangedFilesEmptyGitStderr(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if _, err := changedFiles(t.TempDir(), "origin/main"); err == nil {
		t.Fatal("changedFiles succeeded with a failing git")
	}
}
