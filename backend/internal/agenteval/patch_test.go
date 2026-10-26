package agenteval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const gitShimScript = `#!/bin/sh
if [ -f "$SHIM_MARKER" ]; then
  for arg in "$@"; do
    if [ "$arg" = "$SHIM_TOKEN" ]; then
      case "$SHIM_MODE" in
        fail) echo "shim: forced git failure" >&2; exit 128 ;;
        huge) dd if=/dev/zero bs=1048576 count=17 2>/dev/null | tr '\0' 'x'; exit 0 ;;
        malformed-rename) printf 'R\0backend/old_test.go'; exit 0 ;;
      esac
    fi
  done
fi
exec "$SHIM_REAL_GIT" "$@"
`

// installGitShim puts a git wrapper first on PATH. The wrapper delegates to the
// real git until the marker exists, then intercepts one matching argument. Tests
// only create the marker after setup, so worktree/seed commands still run.
func installGitShim(t *testing.T, token, mode, marker string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	if err := os.WriteFile(shim, []byte(gitShimScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHIM_TOKEN", token)
	t.Setenv("SHIM_MODE", mode)
	t.Setenv("SHIM_MARKER", marker)
	t.Setenv("SHIM_REAL_GIT", realGit)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func touchMarker(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("on"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotBaseErrors(t *testing.T) {
	t.Run("not a git repository", func(t *testing.T) {
		if _, err := snapshotBase(context.Background(), t.TempDir()); err == nil {
			t.Fatal("snapshotBase should fail outside a git repository")
		}
	})

	t.Run("staged base inspection fails", func(t *testing.T) {
		root, _ := initRepo(t)
		marker := filepath.Join(t.TempDir(), "marker")
		touchMarker(t, marker)
		installGitShim(t, "--cached", "fail", marker)
		if _, err := snapshotBase(context.Background(), root); err == nil || !strings.Contains(err.Error(), "inspect staged eval base") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("base commit fails", func(t *testing.T) {
		root, _ := initRepo(t)
		writeRepoFile(t, root, "AGENTS.md", "# changed\n")
		marker := filepath.Join(t.TempDir(), "marker")
		touchMarker(t, marker)
		installGitShim(t, "commit", "fail", marker)
		if _, err := snapshotBase(context.Background(), root); err == nil || !strings.Contains(err.Error(), "snapshot eval base") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestHeadSHAAndHeadMovedErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := headSHA(context.Background(), dir); err == nil {
		t.Fatal("headSHA should fail outside a git repository")
	}
	if _, err := headMoved(context.Background(), dir, "base"); err == nil {
		t.Fatal("headMoved should fail outside a git repository")
	}
}

func TestChangedFilesUntrackedError(t *testing.T) {
	root, sha := initRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	touchMarker(t, marker)
	installGitShim(t, "ls-files", "fail", marker)
	if _, err := changedFiles(context.Background(), root, sha); err == nil || !strings.Contains(err.Error(), "git ls-files") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorktreePatchDiffError(t *testing.T) {
	root, _ := initRepo(t)
	if _, _, _, err := worktreePatch(context.Background(), root, strings.Repeat("f", 40)); err == nil || !strings.Contains(err.Error(), "integrity scan") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorktreePatchUntrackedError(t *testing.T) {
	root, sha := initRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	touchMarker(t, marker)
	installGitShim(t, "ls-files", "fail", marker)
	if _, _, _, err := worktreePatch(context.Background(), root, sha); err == nil || !strings.Contains(err.Error(), "git ls-files for integrity scan") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorktreePatchUntrackedFiles(t *testing.T) {
	root, sha := initRepo(t)
	writeRepoFile(t, root, "backend/new_test.go", "package x\n")
	writeRepoFile(t, root, "backend/blob.bin", "abc\x00def")
	if err := os.WriteFile(filepath.Join(root, "backend", "large.txt"), []byte(strings.Repeat("a", maxPatchFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("new_test.go", filepath.Join(root, "backend", "link")); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "backend", "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o600) })

	patch, warnings, truncated, err := worktreePatch(context.Background(), root, sha)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("small untracked files should not truncate the patch")
	}
	if !strings.Contains(patch, "+++ b/backend/new_test.go") || !strings.Contains(patch, "+package x") {
		t.Fatalf("patch missing untracked addition:\n%s", patch)
	}
	for _, skipped := range []string{"backend/blob.bin", "backend/large.txt", "backend/link"} {
		if strings.Contains(patch, skipped) {
			t.Errorf("patch should not include skipped file %q", skipped)
		}
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"backend/blob.bin: binary", "backend/large.txt: larger than 1 MiB"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q: %v", want, warnings)
		}
	}
	if os.Getuid() != 0 {
		if strings.Contains(patch, "backend/secret.txt") {
			t.Error("patch should not include an unreadable file")
		}
		if !strings.Contains(joined, "backend/secret.txt: unreadable") {
			t.Errorf("warnings missing unreadable file: %v", warnings)
		}
	}
}

func TestWorktreePatchTruncatesLargeDiff(t *testing.T) {
	root, sha := initRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	touchMarker(t, marker)
	installGitShim(t, "--unified=0", "huge", marker)

	patch, warnings, truncated, err := worktreePatch(context.Background(), root, sha)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("large diff should be truncated")
	}
	if len(patch) != maxPatchBytes {
		t.Fatalf("patch length = %d, want %d", len(patch), maxPatchBytes)
	}
	if !contains(warnings, "integrity scan truncated at 16 MiB") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDeletedFiles(t *testing.T) {
	root, _ := initRepo(t)
	if _, err := deletedFiles(context.Background(), root, "bad-ref"); err == nil {
		t.Fatal("deletedFiles should fail for an unknown base")
	}

	writeRepoFile(t, root, "backend/gone.go", "package x\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "add gone")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	if err := os.Remove(filepath.Join(root, "backend", "gone.go")); err != nil {
		t.Fatal(err)
	}
	got, err := deletedFiles(context.Background(), root, base)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "backend/gone.go") {
		t.Fatalf("deleted = %v", got)
	}
}

func TestRenamedFilesMalformedStatus(t *testing.T) {
	root, _ := initRepo(t)
	if _, err := renamedFiles(context.Background(), root, "bad-ref"); err == nil {
		t.Fatal("renamedFiles should fail for an unknown base")
	}

	marker := filepath.Join(t.TempDir(), "marker")
	touchMarker(t, marker)
	installGitShim(t, "--name-status", "malformed-rename", marker)
	got, err := renamedFiles(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("malformed rename output = %+v", got)
	}
}
