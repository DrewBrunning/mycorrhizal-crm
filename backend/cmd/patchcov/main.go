// Command patchcov is the local changed-line coverage check (issue #1964). It
// intersects the same cross-package Go coverprofile the per-file ratchet reads
// with `git diff -U0` and reports the percentage of changed executable lines
// covered.
//
// Codecov remains the authoritative patch gate. By default this command warns
// and exits 0 on a low percentage; -fail-under makes it exit 1 so the focused
// pre-PR gate can block on a local patch-coverage regression. It exits 2 when
// it cannot run (bad flags, missing profile, git failure).
//
//	cd backend && go run ./cmd/patchcov -profile coverage-cross.out -base origin/main -target 95 -fail-under
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mycorrhizal/internal/coverageratchet"
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
	failUnder := fs.Bool("fail-under", false, "exit 1 when changed-line coverage is below -target")
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
		fmt.Fprintln(out, "patchcov: no changed backend Go lines; nothing to check")
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
	belowTarget := pct < *target
	status := "ok"
	if belowTarget {
		if *failUnder {
			status = fmt.Sprintf("FAIL (below the %.0f%% target)", *target)
		} else {
			status = fmt.Sprintf("WARN (below the %.0f%% target; Codecov remains the gate)", *target)
		}
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
	if belowTarget && *failUnder {
		return 1
	}
	return 0
}

// goDiff runs `git diff -U0 <base>...HEAD -- '*.go'` from the repo root.
func goDiff(root, base string) ([]byte, error) {
	cmd := exec.Command("git", "-C", root, "diff", "-U0", "--no-renames", base+"...HEAD", "--", "*.go") // #nosec G204 -- base is a validated CLI ref and root is the resolved repo root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git diff against %s failed: %s", base, msg)
	}
	return stdout.Bytes(), nil
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
