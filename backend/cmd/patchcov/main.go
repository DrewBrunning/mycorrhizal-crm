// Command patchcov is the local, non-blocking changed-line coverage warning
// (issue #1964). It intersects the same cross-package Go coverprofile the
// per-file ratchet reads with `git diff -U0` and reports the percentage of
// changed executable lines covered.
//
// Codecov remains the authoritative patch gate; this command warns and always
// exits 0 on a low percentage. It exits 2 when it cannot run (bad flags,
// missing profile, git failure).
//
//	cd backend && go run ./cmd/patchcov -profile coverage-cross.out -base origin/main -target 95
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mycorrhizal/internal/coverageratchet"
	"mycorrhizal/internal/gitchanges"
	"mycorrhizal/internal/patchcov"
)

const modulePrefix = "mycorrhizal/"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("patchcov", flag.ContinueOnError)
	fs.SetOutput(errOut)
	profile := fs.String("profile", "coverage-cross.out", "Go coverprofile to read")
	base := fs.String("base", "origin/main", "git ref to diff against")
	root := fs.String("root", "", "repository root (default: locate it from the working directory)")
	target := fs.Float64("target", 95, "area patch-coverage target to warn below (backend: 95, frontend: 90, android: 80)")
	if err := fs.Parse(args); err != nil {
		return 2 // # pragma: no cover — flag.ContinueOnError already prints the usage error itself
	}
	if strings.HasPrefix(*base, "-") {
		fmt.Fprintln(errOut, "patchcov: --base must be a git ref, not a flag")
		return 2
	}

	repoRoot := *root
	if repoRoot == "" {
		var err error
		repoRoot, err = findRepoRoot()
		if err != nil {
			fmt.Fprintln(errOut, "patchcov:", err)
			return 2
		}
	}
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		fmt.Fprintln(errOut, "patchcov:", err) // # pragma: no cover — Abs fails only on an unresolvable cwd
		return 2                               // # pragma: no cover
	}

	diff, err := goDiff(repoRoot, *base)
	if err != nil {
		fmt.Fprintln(errOut, "patchcov:", err)
		return 2
	}
	changed, err := patchcov.ParseUnifiedDiff(bytes.NewReader(diff))
	if err != nil {
		fmt.Fprintln(errOut, "patchcov:", err)
		return 2
	}
	changedGo := map[string][]int{}
	for file, lines := range changed {
		if strings.HasPrefix(file, "backend/") && strings.HasSuffix(file, ".go") && !strings.HasSuffix(file, "_test.go") {
			changedGo[file] = lines
		}
	}
	if len(changedGo) == 0 {
		fmt.Fprintln(out, "patchcov: no changed backend Go lines; nothing to warn about")
		return 0
	}

	profilePath := *profile
	if !filepath.IsAbs(profilePath) {
		profilePath = filepath.Join(repoRoot, "backend", profilePath)
	}
	file, err := os.Open(profilePath) // #nosec G304 -- profile is a CLI flag pointing at this run's own coverage output
	if err != nil {
		fmt.Fprintln(errOut, "patchcov:", err)
		return 2
	}
	defer file.Close() //nolint:errcheck // read-only

	blocks, err := coverageratchet.ParseMergedCoverprofile(file)
	if err != nil {
		fmt.Fprintln(errOut, "patchcov:", err)
		return 2
	}
	results := patchcov.ChangedLineCoverage(blocks, changedGo, modulePrefix, "backend")
	if len(results) == 0 {
		fmt.Fprintln(out, "patchcov: no changed executable Go lines in the profile")
		return 0
	}

	total, covered := patchcov.Totals(results)
	pct := patchcov.Percent(results)
	status := "ok"
	if pct < *target {
		status = fmt.Sprintf("WARN (below the %.0f%% target; Codecov remains the gate)", *target)
	}
	fmt.Fprintf(out, "patchcov: changed-line coverage %.1f%% (%d/%d lines), target %.0f%% — %s\n",
		pct, covered, total, *target, status)
	for _, result := range results {
		filePct := 100.0
		if result.Executable > 0 {
			filePct = float64(result.Covered) / float64(result.Executable) * 100
		}
		fmt.Fprintf(out, "  %s: %.1f%% (%d/%d)\n", result.File, filePct, result.Covered, result.Executable)
	}
	return 0
}

// goDiff runs the Go-only changed-line diff against the merge base of base and
// HEAD, including staged, unstaged, and untracked files. The gate runs before
// the commit, so a committed-only diff would miss the lines being reviewed.
func goDiff(root, base string) ([]byte, error) {
	return gitchanges.UnifiedDiff(root, base, "*.go")
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err // # pragma: no cover — Getwd fails only when the cwd has been deleted
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "backend", "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod) from %s", dir)
		}
		dir = parent
	}
}
