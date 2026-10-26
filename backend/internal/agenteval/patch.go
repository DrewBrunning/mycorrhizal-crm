package agenteval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// snapshotBase commits the post-seed worktree state so later diffs describe
// only the agent's changes. Without this, a seed patch that changes a file is
// attributed to the agent, and a refusal task would fail on its own fixture.
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
