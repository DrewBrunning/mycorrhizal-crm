package agenteval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// maxPatchBytes bounds the diff text the integrity scan reads. A task that
	// exceeds it is already outside the scope the eval is designed to score.
	maxPatchBytes = 16 << 20
	// maxPatchFileBytes bounds one untracked file. Larger files are skipped
	// rather than read into memory.
	maxPatchFileBytes = 1 << 20
)

// RenamedFile is one rename reported by git.
type RenamedFile struct {
	Old string
	New string
}

// snapshotBase commits the post-seed, post-strip worktree state so later
// diffs describe only the agent's changes. Without this, a seed patch that
// adds a skip or suppression is attributed to the agent.
func snapshotBase(ctx context.Context, wt string) (string, error) {
	if err := gitRun(ctx, wt, "add", "-A"); err != nil {
		return "", err
	}
	quiet := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--cached", "--quiet") // #nosec G204 -- wt is our temp worktree
	if err := quiet.Run(); err == nil {
		return headSHA(ctx, wt)
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return "", fmt.Errorf("inspect staged eval base: %w", err)
		}
	}
	commit := exec.CommandContext(ctx, "git", "-C", wt,
		"-c", "user.name=agenteval",
		"-c", "user.email=agenteval@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--no-verify", "-q", "-m", "agenteval eval base") // #nosec G204 -- fixed args in our temp worktree
	if out, err := commit.CombinedOutput(); err != nil {
		return "", fmt.Errorf("snapshot eval base: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return headSHA(ctx, wt)
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // #nosec G204 -- callers pass fixed git verbs
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func headSHA(ctx context.Context, wt string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", wt, "rev-parse", "HEAD").Output() // #nosec G204 -- wt is our temp worktree
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func headMoved(ctx context.Context, wt, base string) (bool, error) {
	head, err := headSHA(ctx, wt)
	if err != nil {
		return false, err
	}
	return head != base, nil
}

// changedFiles returns tracked and untracked files relative to base. Diffing
// against the eval base, not HEAD, means an agent commit cannot hide changes.
func changedFiles(ctx context.Context, wt, base string) ([]string, error) {
	tracked := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--name-only", "-z", base) // #nosec G204 -- base is a validated commit SHA
	tbody, err := tracked.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	untracked := exec.CommandContext(ctx, "git", "-C", wt, "ls-files", "--others", "--exclude-standard", "-z") // #nosec G204 -- wt is our temp worktree
	ubody, err := untracked.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var out []string
	for _, body := range [][]byte{tbody, ubody} {
		for _, part := range bytes.Split(body, []byte{0}) {
			if len(part) > 0 {
				out = append(out, filepath.ToSlash(string(part)))
			}
		}
	}
	return SortStrings(unique(out)), nil
}

// worktreePatch returns the agent's diff against base, including untracked
// files as synthetic additions. It also returns scan warnings and whether the
// patch was truncated (in which case the scan cannot be trusted).
func worktreePatch(ctx context.Context, wt, base string) (string, []string, bool, error) {
	tracked, err := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--unified=0", base).Output() // #nosec G204 -- base is a validated commit SHA
	if err != nil {
		return "", nil, false, fmt.Errorf("git diff for integrity scan: %w", err)
	}
	var warnings []string
	truncated := false
	if len(tracked) > maxPatchBytes {
		tracked = tracked[:maxPatchBytes]
		truncated = true
	}

	untracked, err := exec.CommandContext(ctx, "git", "-C", wt, "ls-files", "--others", "--exclude-standard", "-z").Output() // #nosec G204 -- wt is our temp worktree
	if err != nil {
		return "", nil, false, fmt.Errorf("git ls-files for integrity scan: %w", err)
	}

	var b strings.Builder
	b.Write(tracked)
	for _, part := range bytes.Split(untracked, []byte{0}) {
		if b.Len() >= maxPatchBytes {
			truncated = true
			break
		}
		path := filepath.ToSlash(string(part))
		if path == "" {
			continue
		}
		abs := filepath.Join(wt, filepath.FromSlash(path))
		info, err := os.Lstat(abs) // #nosec G304 -- path is from our worktree's git index
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxPatchFileBytes {
			warnings = append(warnings, "integrity scan skipped "+path+": larger than 1 MiB")
			continue
		}
		body, err := os.ReadFile(abs) // #nosec G304 -- path is from our worktree's git index
		if err != nil {
			warnings = append(warnings, "integrity scan skipped "+path+": unreadable")
			continue
		}
		if bytes.IndexByte(body, 0) >= 0 {
			warnings = append(warnings, "integrity scan skipped "+path+": binary")
			continue
		}
		b.WriteString("diff --git a/" + path + " b/" + path + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + path + "\n")
		for _, line := range strings.Split(string(body), "\n") {
			b.WriteString("+" + line + "\n")
		}
	}
	if truncated {
		warnings = append(warnings, "integrity scan truncated at 16 MiB")
	}
	return b.String(), warnings, truncated, nil
}

// deletedFiles returns paths deleted relative to base.
func deletedFiles(ctx context.Context, wt, base string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--name-only", "--diff-filter=D", "-z", base).Output() // #nosec G204 -- base is a validated commit SHA
	if err != nil {
		return nil, fmt.Errorf("git diff --diff-filter=D: %w", err)
	}
	var paths []string
	for _, part := range bytes.Split(out, []byte{0}) {
		if p := filepath.ToSlash(string(part)); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// renamedFiles returns rename pairs relative to base.
func renamedFiles(ctx context.Context, wt, base string) ([]RenamedFile, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--name-status", "-z", "--find-renames", base).Output() // #nosec G204 -- base is a validated commit SHA
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w", err)
	}
	parts := strings.Split(string(out), "\x00")
	var renames []RenamedFile
	for i := 0; i < len(parts); {
		status := parts[i]
		if status == "" {
			i++
			continue
		}
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+2 >= len(parts) {
				break
			}
			if strings.HasPrefix(status, "R") {
				renames = append(renames, RenamedFile{Old: filepath.ToSlash(parts[i+1]), New: filepath.ToSlash(parts[i+2])})
			}
			i += 3
			continue
		}
		i += 2 // status + path
	}
	return renames, nil
}
