package gitchanges

import (
	"fmt"
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

func fakeGit(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", bin)
}

// TestChangedFilesReportsGitFailures covers each of the three listings: a
// failure in any one must abort instead of silently returning a partial set.
func TestChangedFilesReportsGitFailures(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name string
		fail string
	}{
		{"tracked diff", `*" diff --name-only "*`},
		{"staged diff", `*" diff --cached "*`},
		{"untracked listing", `*" ls-files "*`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeGit(t, fmt.Sprintf(`case "$*" in
  *" merge-base "*) echo %s ;;
  %s) echo boom >&2; exit 1 ;;
esac
`, commit, tt.fail))
			if _, err := ChangedFiles(t.TempDir(), "HEAD"); err == nil || !strings.Contains(err.Error(), "git diff against HEAD failed: boom") {
				t.Fatalf("ChangedFiles error = %v, want the failing git command", err)
			}
		})
	}
}

// TestMergeBaseEmptyOutputIsAnError is the defensive branch for a git
// invocation that succeeds without naming a commit.
func TestMergeBaseEmptyOutputIsAnError(t *testing.T) {
	fakeGit(t, `case "$*" in
  *" merge-base "*) : ;;
esac
`)
	if _, err := ChangedFiles(t.TempDir(), "HEAD"); err == nil || !strings.Contains(err.Error(), "merge-base returned no commit") {
		t.Fatalf("ChangedFiles error = %v, want an empty merge-base error", err)
	}
}

// TestUnifiedDiffReportsGitFailures covers the setup, listing, intent-to-add,
// and diff error exits. Each subtest supplies a readable index so the failure
// surfaces at the intended command rather than during index setup.
func TestUnifiedDiffReportsGitFailures(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{
			name: "rev-parse index",
			script: `case "$*" in
  *" merge-base "*) echo ` + commit + ` ;;
  *" rev-parse "*) echo "no index" >&2; exit 1 ;;
esac
`,
			wantErr: "git rev-parse index failed: no index",
		},
		{
			name: "read index",
			script: `case "$*" in
  *" merge-base "*) echo ` + commit + ` ;;
  *" rev-parse "*) echo . ;;
esac
`,
			wantErr: "read git index",
		},
		{
			name: "list untracked",
			script: `case "$*" in
  *" merge-base "*) echo ` + commit + ` ;;
  *" rev-parse "*) echo idx ;;
  *" ls-files "*) echo boom >&2; exit 1 ;;
esac
`,
			wantErr: "git diff against HEAD failed: boom",
		},
		{
			name: "intent-to-add",
			script: `case "$*" in
  *" merge-base "*) echo ` + commit + ` ;;
  *" rev-parse "*) echo idx ;;
  *" ls-files "*) printf 'foo.go\0' ;;
  *" add "*) echo boom >&2; exit 1 ;;
esac
`,
			wantErr: "git diff against HEAD failed: boom",
		},
		{
			name: "diff",
			script: `case "$*" in
  *" merge-base "*) echo ` + commit + ` ;;
  *" rev-parse "*) echo idx ;;
  *" diff "*) echo boom >&2; exit 1 ;;
esac
`,
			wantErr: "git diff against HEAD failed: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "idx"), "index")
			fakeGit(t, tt.script)
			if _, err := UnifiedDiff(root, "HEAD", "*.go"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("UnifiedDiff error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestTempIndexCreateFailure(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	root := t.TempDir()
	write(t, filepath.Join(root, "idx"), "index")
	fakeGit(t, `case "$*" in
  *" merge-base "*) echo `+commit+` ;;
  *" rev-parse "*) echo idx ;;
esac
`)
	t.Setenv("TMPDIR", filepath.Join(root, "missing"))
	if _, err := UnifiedDiff(root, "HEAD", "*.go"); err == nil || !strings.Contains(err.Error(), "create temporary git index") {
		t.Fatalf("UnifiedDiff error = %v, want the temporary-index creation failure", err)
	}
}

// TestUnifiedDiffInitialisesMissingIndex exercises the freshly-initialised
// repository path: no real index exists, so the temporary one is populated from
// the merge base and the untracked file still appears in the diff.
func TestUnifiedDiffInitialisesMissingIndex(t *testing.T) {
	dir := newRepo(t)
	if err := os.Remove(filepath.Join(dir, ".git", "index")); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	write(t, filepath.Join(dir, "backend", "c.go"), "package backend\n\nvar C = 1\n")

	diff, err := UnifiedDiff(dir, "HEAD", "*.go")
	if err != nil {
		t.Fatalf("UnifiedDiff: %v", err)
	}
	if !strings.Contains(string(diff), "+var C = 1") {
		t.Errorf("diff missing the untracked file:\n%s", diff)
	}
}

// TestTempIndexInitialiseFailure covers the no-index setup path when reading
// the merge-base tree into the temporary index fails.
func TestTempIndexInitialiseFailure(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	fakeGit(t, `case "$*" in
  *" merge-base "*) echo `+commit+` ;;
  *" rev-parse "*) echo missing-index ;;
  *" read-tree "*) echo boom >&2; exit 1 ;;
esac
`)
	if _, err := UnifiedDiff(t.TempDir(), "HEAD", "*.go"); err == nil || !strings.Contains(err.Error(), "initialise temporary git index: boom") {
		t.Fatalf("UnifiedDiff error = %v, want the read-tree failure", err)
	}
}
