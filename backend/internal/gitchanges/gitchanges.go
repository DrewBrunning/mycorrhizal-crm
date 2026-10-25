// Package gitchanges computes a branch's changes relative to a base ref as a
// contributor sees them before committing: committed branch changes plus the
// staged, unstaged, and untracked working-tree changes. The local pre-PR gate
// runs before the commit, so a committed-only diff is a false green.
package gitchanges

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ChangedFiles returns the repo-relative paths that differ from the merge base
// of base and HEAD, including staged, unstaged, untracked, deleted, and
// renamed files. With renames disabled, a rename contributes both the deleted
// old path and the added new path, which is what area selection needs.
//
// Paths are NUL-delimited end to end, so a filename containing whitespace or a
// newline is preserved as one path.
func ChangedFiles(root, base string) ([]string, error) {
	mergeBase, err := mergeBase(root, base)
	if err != nil {
		return nil, err
	}

	// The working-tree diff covers unstaged changes and the final content of
	// staged files; the cached diff is still needed for a staged change that
	// was reverted in the working tree, which the eventual commit would carry.
	tracked, stderr, err := runGit(root, nil, nil, "diff", "--name-only", "-z", "--no-renames", mergeBase, "--")
	if err != nil {
		return nil, diffFailed(base, stderr, err)
	}
	staged, stderr, err := runGit(root, nil, nil, "diff", "--cached", "--name-only", "-z", "--no-renames", mergeBase, "--")
	if err != nil {
		return nil, diffFailed(base, stderr, err)
	}
	untracked, stderr, err := runGit(root, nil, nil, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return nil, diffFailed(base, stderr, err)
	}

	trackedPaths := splitNUL(tracked)
	stagedPaths := splitNUL(staged)
	untrackedPaths := splitNUL(untracked)
	seen := map[string]bool{}
	out := make([]string, 0, len(trackedPaths)+len(stagedPaths)+len(untrackedPaths))
	for _, paths := range [][]string{trackedPaths, stagedPaths, untrackedPaths} {
		for _, path := range paths {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out, nil
}

// UnifiedDiff returns a `git diff -U0 --no-renames <merge-base>` stream that
// also includes untracked files matching pathspecs. It never touches the
// user's index: the untracked paths are marked intent-to-add in a temporary
// copy of the index.
func UnifiedDiff(root, base string, pathspecs ...string) ([]byte, error) {
	mergeBase, err := mergeBase(root, base)
	if err != nil {
		return nil, err
	}
	indexPath, cleanup, err := tempIndex(root, mergeBase)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	env := []string{"GIT_INDEX_FILE=" + indexPath}
	lsArgs := append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, pathspecs...)
	untracked, stderr, err := runGit(root, env, nil, lsArgs...)
	if err != nil {
		return nil, diffFailed(base, stderr, err)
	}
	if paths := splitNUL(untracked); len(paths) > 0 {
		input := []byte(strings.Join(paths, "\x00") + "\x00")
		if _, stderr, err := runGit(root, env, input, "add", "-N", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return nil, diffFailed(base, stderr, err)
		}
	}

	diffArgs := append([]string{"diff", "-U0", "--no-renames", mergeBase, "--"}, pathspecs...)
	diff, stderr, err := runGit(root, env, nil, diffArgs...)
	if err != nil {
		return nil, diffFailed(base, stderr, err)
	}
	return diff, nil
}

// mergeBase resolves the common ancestor of base and HEAD. `base...HEAD`
// alone would miss the working tree, so callers diff against this commit.
func mergeBase(root, base string) (string, error) {
	stdout, stderr, err := runGit(root, nil, nil, "merge-base", base, "HEAD")
	if err != nil {
		return "", diffFailed(base, stderr, err)
	}
	mb := strings.TrimSpace(string(stdout))
	if mb == "" {
		return "", fmt.Errorf("git diff against %s failed: merge-base returned no commit", base)
	}
	return mb, nil
}

// tempIndex copies the repository's index to a temporary file so intent-to-add
// entries for untracked files do not mutate the contributor's real index. An
// empty temporary index is initialised from the merge base.
func tempIndex(root, mergeBase string) (path string, cleanup func(), err error) {
	stdout, stderr, err := runGit(root, nil, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return "", nil, fmt.Errorf("git rev-parse index failed: %s", commandMessage(stderr, err))
	}
	indexPath := strings.TrimSpace(string(stdout))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(root, indexPath)
	}

	data, readErr := os.ReadFile(indexPath) // #nosec G304 -- path comes from git, not user input
	if readErr != nil && !os.IsNotExist(readErr) {
		return "", nil, fmt.Errorf("read git index %s: %w", indexPath, readErr)
	}

	file, err := os.CreateTemp("", "gitchanges-index-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary git index: %w", err)
	}
	tmp := file.Name()
	cleanup = func() { _ = os.Remove(tmp) } //nolint:errcheck // best-effort temp cleanup

	if readErr == nil {
		if _, err := file.Write(data); err != nil {
			_ = file.Close() //nolint:errcheck // already failing
			cleanup()
			return "", nil, fmt.Errorf("write temporary git index: %w", err)
		}
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close temporary git index: %w", err)
	}
	if readErr == nil {
		return tmp, cleanup, nil
	}

	// No index yet (for example, a freshly initialised repository). Populate
	// the temporary index from the merge base so `git diff <commit>` can still
	// describe the working tree.
	if err := os.Remove(tmp); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("reset temporary git index: %w", err)
	}
	env := []string{"GIT_INDEX_FILE=" + tmp}
	if _, stderr, err := runGit(root, env, nil, "read-tree", mergeBase); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("initialise temporary git index: %s", commandMessage(stderr, err))
	}
	return tmp, cleanup, nil
}

func runGit(root string, env []string, stdin []byte, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- fixed binary, resolved root, validated ref/pathspecs
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err = cmd.Run()
	return out.Bytes(), errOut.Bytes(), err
}

// mergeEnv applies overrides after the inherited environment, dropping any
// earlier entry with the same key so a caller-supplied GIT_INDEX_FILE cannot
// lose to the ambient one on systems where the first duplicate wins.
func mergeEnv(base, overrides []string) []string {
	kept := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		shadowed := false
		for _, override := range overrides {
			if strings.HasPrefix(override, key+"=") {
				shadowed = true
				break
			}
		}
		if !shadowed {
			kept = append(kept, entry)
		}
	}
	return append(kept, overrides...)
}

func diffFailed(base string, stderr []byte, err error) error {
	return fmt.Errorf("git diff against %s failed: %s", base, commandMessage(stderr, err))
}

func commandMessage(stderr []byte, err error) string {
	if msg := strings.TrimSpace(string(stderr)); msg != "" {
		return msg
	}
	return err.Error()
}

// splitNUL parses git's -z output without ever splitting on whitespace.
func splitNUL(raw []byte) []string {
	raw = bytes.TrimSuffix(raw, []byte{0})
	if len(raw) == 0 {
		return nil
	}
	parts := bytes.Split(raw, []byte{0})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, string(part))
	}
	return out
}
