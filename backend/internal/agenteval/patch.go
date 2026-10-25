package agenteval

import (
	"bytes"
	"context"
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

// worktreePatch returns the agent's working-tree diff as unified text,
// including untracked files as synthetic additions. The integrity scan only
// needs the +++/--- headers and added/removed lines.
func worktreePatch(ctx context.Context, wt string) (string, error) {
	tracked, err := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--unified=0", "HEAD").Output() // #nosec G204 -- wt is our temp worktree
	if err != nil {
		return "", fmt.Errorf("git diff for integrity scan: %w", err)
	}
	if len(tracked) > maxPatchBytes {
		tracked = tracked[:maxPatchBytes]
	}

	untracked, err := exec.CommandContext(ctx, "git", "-C", wt, "ls-files", "--others", "--exclude-standard", "-z").Output() // #nosec G204 -- wt is our temp worktree
	if err != nil {
		return "", fmt.Errorf("git ls-files for integrity scan: %w", err)
	}

	var b strings.Builder
	b.Write(tracked)
	for _, part := range bytes.Split(untracked, []byte{0}) {
		if b.Len() >= maxPatchBytes {
			break
		}
		path := filepath.ToSlash(string(part))
		if path == "" {
			continue
		}
		abs := filepath.Join(wt, filepath.FromSlash(path))
		info, err := os.Lstat(abs) // #nosec G304 -- path is from our worktree's git index
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxPatchFileBytes {
			continue
		}
		body, err := os.ReadFile(abs) // #nosec G304 -- path is from our worktree's git index
		if err != nil {
			continue
		}
		if bytes.IndexByte(body, 0) >= 0 {
			continue
		}
		b.WriteString("diff --git a/" + path + " b/" + path + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + path + "\n")
		for _, line := range strings.Split(string(body), "\n") {
			b.WriteString("+" + line + "\n")
		}
	}
	return b.String(), nil
}

// deletedFiles returns paths deleted relative to HEAD.
func deletedFiles(ctx context.Context, wt string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", wt, "diff", "--name-only", "--diff-filter=D", "-z", "HEAD").Output() // #nosec G204 -- wt is our temp worktree
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
