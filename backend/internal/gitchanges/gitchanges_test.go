package gitchanges

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
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

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "commit.gpgsign", "false")
	write(t, filepath.Join(dir, "backend", "a.go"), "package backend\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

// TestUnifiedDiffIncludesUntrackedWithoutTouchingIndex pins both halves of the
// untracked-file contract: the diff has the new file, and the contributor's
// real index is untouched (intent-to-add lives in a temporary index).
func TestUnifiedDiffIncludesUntrackedWithoutTouchingIndex(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, "backend", "b.go"), "package backend\n\nvar B = 1\n")
	write(t, filepath.Join(dir, "README.md"), "not Go\n")

	before := git(t, dir, "diff", "--cached", "--name-only", "-z")

	diff, err := UnifiedDiff(dir, "HEAD", "*.go")
	if err != nil {
		t.Fatalf("UnifiedDiff: %v", err)
	}
	for _, want := range []string{"+++ b/backend/b.go", "+var B = 1"} {
		if !strings.Contains(string(diff), want) {
			t.Errorf("diff missing %q:\n%s", want, diff)
		}
	}
	if strings.Contains(string(diff), "README.md") {
		t.Errorf("diff ignored the *.go pathspec:\n%s", diff)
	}

	after := git(t, dir, "diff", "--cached", "--name-only", "-z")
	if before != after {
		t.Errorf("real index changed during UnifiedDiff: before %q, after %q", before, after)
	}
	if staged := git(t, dir, "ls-files", "--stage", "backend/b.go"); staged != "" {
		t.Errorf("untracked file was added to the real index: %q", staged)
	}
}

// TestUnifiedDiffTreatsUntrackedPathsLiterally pins that the names from
// `git ls-files -z` are passed to `git add -N` as literal filenames, not
// pathspecs. A leading ':' is git pathspec magic; without literal pathspecs the
// add fails and patchcov silently sees no diff for that file.
func TestUnifiedDiffTreatsUntrackedPathsLiterally(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, ":colon.go"), "package backend\n\nvar Colon = 1\n")

	diff, err := UnifiedDiff(dir, "HEAD", "*.go")
	if err != nil {
		t.Fatalf("UnifiedDiff: %v", err)
	}
	if !strings.Contains(string(diff), "+++ b/:colon.go") {
		t.Errorf("diff missing the literal untracked path:\n%s", diff)
	}
}

func TestChangedFilesCombinesTrackedAndUntracked(t *testing.T) {
	dir := newRepo(t)
	write(t, filepath.Join(dir, "backend", "a.go"), "package backend\n\nvar A = 1\n")
	write(t, filepath.Join(dir, "backend", "b.go"), "package backend\n")

	got, err := ChangedFiles(dir, "HEAD")
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	want := []string{"backend/a.go", "backend/b.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ChangedFiles = %v, want %v", got, want)
	}
}

func TestChangedFilesBadBaseKeepsGitDiffWording(t *testing.T) {
	dir := newRepo(t)
	if _, err := ChangedFiles(dir, "does-not-exist"); err == nil || !strings.Contains(err.Error(), "git diff") {
		t.Fatalf("ChangedFiles error = %v, want a git diff error", err)
	}
}

func TestSplitNULPreservesWhitespace(t *testing.T) {
	got := splitNUL([]byte("a b\x00new\nline\x00"))
	if len(got) != 2 || got[0] != "a b" || got[1] != "new\nline" {
		t.Fatalf("splitNUL = %q", got)
	}
}

func TestMergeEnvOverridesEarlier(t *testing.T) {
	got := mergeEnv([]string{"A=1", "GIT_INDEX_FILE=old", "B=2"}, []string{"GIT_INDEX_FILE=new"})
	want := []string{"A=1", "B=2", "GIT_INDEX_FILE=new"}
	if !slices.Equal(got, want) {
		t.Fatalf("mergeEnv = %q, want %q", got, want)
	}
}
