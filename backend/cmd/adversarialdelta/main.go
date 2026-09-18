// Command adversarialdelta is the per-release adversarial-delta gate (issue
// #953).
//
// It reads the list of paths changed since the previous release tag (a
// `git diff --name-only <prev>..HEAD` fed on stdin), classifies any
// security-relevant surface they touch, and requires the release to have a row
// in docs/security/adversarial-deltas.md naming each class it touched. A
// release that changed no recognised surface class passes with nothing to
// record.
//
// It is wired into .github/workflows/release.yml as a final-release gate beside
// the ASVS re-verification row check, and replaces no existing check: the
// per-class mechanical gates still own their classes, and this is the
// per-release backstop for a new class none of them anticipated.
//
// Exit status 0: the delta is recorded (or no surface class was touched). 1:
// a touched surface class has no recorded delta. 2: the check itself could not
// run (missing -release, unreadable ledger).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mycorrhizal/internal/adversarialdelta"
)

func main() {
	os.Exit(mainExit(os.Args[1:], os.Stdin, os.Stdout)) // # pragma: no cover — os.Exit terminates the process; tests exercise run()
}

// mainExit parses flags, reads the changed paths from in, and delegates to run.
func mainExit(args []string, in io.Reader, w io.Writer) int {
	fs := flag.NewFlagSet("adversarialdelta", flag.ContinueOnError)
	fs.SetOutput(w)
	release := fs.String("release", "", "the release version being cut, e.g. v0.8.8 (required)")
	ledger := fs.String("ledger", adversarialdelta.LedgerFile, "per-release delta ledger, repo-root-relative")
	ack := fs.String("ack", "", "reason to proceed without a recorded delta (recorded, not silent)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*release) == "" {
		fmt.Fprintln(w, "adversarialdelta: -release is required")
		return 2
	}
	changed, err := readPaths(in)
	if err != nil {
		fmt.Fprintln(w, "adversarialdelta: read changed paths:", err)
		return 2
	}
	root, err := findRepoRoot(mustGetwd())
	if err != nil {
		fmt.Fprintln(w, "adversarialdelta:", err)
		return 2
	}
	return run(w, root, *ledger, *release, *ack, changed)
}

// readPaths returns the non-empty, trimmed lines from r.
func readPaths(r io.Reader) ([]string, error) {
	var paths []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if p := strings.TrimSpace(scanner.Text()); p != "" {
			paths = append(paths, p)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}

// run evaluates the release against the ledger at root and returns the process
// exit code. Tests drive it directly.
func run(w io.Writer, root, ledgerPath, release, ack string, changed []string) int {
	ledger, err := readRepoFile(root, ledgerPath)
	if err != nil {
		fmt.Fprintf(w, "adversarialdelta: %v\n", err)
		return 2
	}
	verdict := adversarialdelta.Evaluate(ledger, changed, release)

	if verdict.OK() {
		if len(verdict.Surfaces) == 0 {
			fmt.Fprintf(w, "adversarial delta: %s touched no recognised security-relevant surface class.\n", release)
		} else {
			fmt.Fprintf(w, "adversarial delta: %s records the new surface (%s).\n", release, strings.Join(verdict.Surfaces, ", "))
		}
		return 0
	}

	detail := adversarialdelta.FormatMissing(verdict, release)
	if ack != "" {
		fmt.Fprintf(w, "::warning::%s -- dispatcher acknowledged proceeding: %s\n", detail, ack)
		return 0
	}
	fmt.Fprintf(w, "::error::%s. Record the delta in %s, or re-dispatch with an acknowledgement reason.\n", detail, ledgerPath)
	return 1
}

// findRepoRoot walks up from start until it finds the ledger's home doc tree,
// so the command works from backend/ (go run) and backend/cmd/adversarialdelta/
// (go test) alike.
func findRepoRoot(start string) (string, error) {
	const sentinel = "docs/security/threat-model.md"
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, sentinel)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for %s)", start, sentinel)
		}
		dir = parent
	}
}

// mustGetwd returns the working directory, falling back to "." so a resolution
// failure surfaces as the normal "no repository root" error.
func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted.
		return "."
	}
	return wd
}

// readRepoFile reads a repository-relative file, refusing anything that would
// escape root.
func readRepoFile(root, rel string) ([]byte, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("refusing to read %q — outside the repository root", rel)
	}
	// #nosec G304 -- clean is repo-relative and traversal-checked immediately
	// above; root comes from findRepoRoot walking up from the working
	// directory, never from request input. Same posture as cmd/asvsstale's
	// readRepoFile.
	return os.ReadFile(filepath.Join(root, clean))
}
