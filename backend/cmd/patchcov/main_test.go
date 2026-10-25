package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

// newRepo creates a throwaway repo whose second commit changes three lines of
// backend/services/foo.go.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "commit.gpgsign", "false")
	write(t, filepath.Join(dir, "README.md"), "test\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "initial")

	write(t, filepath.Join(dir, "backend", "go.mod"), "module mycorrhizal\n\ngo 1.26.0\n")
	write(t, filepath.Join(dir, "backend", "services", "foo.go"), "package services\n\nfunc Foo() {\n}\n\nfunc Bar() {\n}\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "backend change")
	return dir
}

const coveredProfile = `mode: atomic
mycorrhizal/services/foo.go:3.15,4.2 1 1
mycorrhizal/services/foo.go:6.15,7.2 1 0
`

func TestRunReportsChangedLineCoverage(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, "backend", "coverage.out"), coveredProfile)
	var out, errOut bytes.Buffer
	code := run([]string{"--profile", "coverage.out", "--base", "HEAD~1", "--root", dir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	for _, want := range []string{"patchcov: changed-line coverage", "backend/services/foo.go", "WARN"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunUsesAbsoluteProfilePath(t *testing.T) {
	dir := newRepo(t)
	profile := filepath.Join(dir, "prof.out")
	write(t, profile, coveredProfile)
	var out, errOut bytes.Buffer
	if code := run([]string{"--profile", profile, "--base", "HEAD~1", "--root", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
}

func TestRunNoChangedGoLines(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"--base", "HEAD", "--root", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "no changed backend Go lines") {
		t.Errorf("output = %s", out.String())
	}
}

func TestRunTestOnlyDiffIsIgnored(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, "backend", "services", "foo_test.go"), "package services\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "test only")
	var out, errOut bytes.Buffer
	if code := run([]string{"--base", "HEAD~1", "--root", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "no changed backend Go lines") {
		t.Errorf("output = %s", out.String())
	}
}

// TestRunIncludesUncommittedAndUntrackedGoLines is the regression test for the
// pre-commit false green: the changed-line diff must include staged, unstaged,
// and untracked Go files, not only commits.
func TestRunIncludesUncommittedAndUntrackedGoLines(t *testing.T) {
	dir := newRepo(t)

	write(t, filepath.Join(dir, "backend", "services", "foo.go"), "package services\n\nfunc Foo() {\n\tprintln(\"foo\")\n}\n\nfunc Bar() {\n}\n")
	write(t, filepath.Join(dir, "backend", "services", "baz.go"), "package services\n\nfunc Baz() {\n}\n")
	git(t, dir, "add", "backend/services/baz.go")
	write(t, filepath.Join(dir, "backend", "services", "bar.go"), "package services\n\nfunc Bar() {\n}\n")

	write(t, filepath.Join(dir, "backend", "coverage.out"), `mode: atomic
mycorrhizal/services/foo.go:4.2,4.15 1 1
mycorrhizal/services/baz.go:3.15,4.2 1 1
mycorrhizal/services/bar.go:3.15,4.2 1 1
`)
	var out, errOut bytes.Buffer
	code := run([]string{"--profile", "coverage.out", "--base", "HEAD", "--root", dir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "no changed backend Go lines") {
		t.Fatalf("uncommitted changes were not seen:\n%s", out.String())
	}
	for _, want := range []string{
		"backend/services/foo.go",
		"backend/services/baz.go",
		"backend/services/bar.go",
		"changed-line coverage 100.0%",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunMissingProfile(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"--profile", "missing.out", "--base", "HEAD~1", "--root", dir}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestRunMalformedProfile(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, "backend", "coverage.out"), "not a coverprofile\n")
	var out, errOut bytes.Buffer
	if code := run([]string{"--profile", "coverage.out", "--base", "HEAD~1", "--root", dir}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestRunNoExecutableChangedLines(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, "backend", "coverage.out"), "mode: atomic\nmycorrhizal/services/foo.go:100.1,101.2 1 1\n")
	var out, errOut bytes.Buffer
	if code := run([]string{"--profile", "coverage.out", "--base", "HEAD~1", "--root", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "no changed executable Go lines") {
		t.Errorf("output = %s", out.String())
	}
}

func TestRunBadBase(t *testing.T) {
	dir := newRepo(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"--base", "does-not-exist", "--root", dir}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestRunRejectsFlagLikeBase(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--base", "--evil"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--nope"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
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
	write(t, filepath.Join(dir, "backend", "coverage.out"), coveredProfile)
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	if code := run([]string{"--profile", "coverage.out", "--base", "HEAD~1"}, &out, &errOut); code != 0 {
		t.Fatalf("run = %d, want 0\nstderr: %s", code, errOut.String())
	}
}

func TestRunMalformedGitDiff(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '+++ b/x.go' '@@ nope @@'\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var out, errOut bytes.Buffer
	if code := run([]string{"--root", t.TempDir(), "--base", "origin/main"}, &out, &errOut); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func TestGoDiffEmptyStderr(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if _, err := goDiff(t.TempDir(), "origin/main"); err == nil {
		t.Fatal("goDiff succeeded with a failing git")
	}
}
