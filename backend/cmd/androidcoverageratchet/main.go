// Command androidcoverageratchet is the Android half of the per-file
// no-regression coverage ratchet (backend counterpart:
// cmd/coverageratchet; frontend counterpart:
// frontend/scripts/check-coverage-ratchet.mjs; issue #1627), closing the gap
// the diff-based codecov/patch/android status leaves: a Kotlin file that
// already sits at 0% coverage stays there forever, and a PR that deletes or
// guts a test for a file it doesn't otherwise touch trips no status at all.
//
// It reads the aggregated JaCoCo XML the `test` job in
// .github/workflows/android-tests.yml already produces via
// ./gradlew jacocoTestReportAggregated, computes each file's line coverage
// percentage (keyed "<package>/<sourcefile>"), and compares it against the
// committed android/coverage-baseline.json.
//
// Normal workflow:
//
//	cd android && ./gradlew jacocoTestReportAggregated   # produce the report
//	cd backend && go run ./cmd/androidcoverageratchet     # check (default)
//	cd backend && go run ./cmd/androidcoverageratchet -update  # regenerate the baseline
//
// Exit status 0: within tolerance (or baseline regenerated). 1: at least one
// file's coverage dropped past tolerance. 2: the command could not run
// (missing/malformed profile or baseline).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/androidcoverage"
)

// defaultProfileRel is the aggregated report's path relative to the repo root
// (produced by ./gradlew jacocoTestReportAggregated in android/, issue #342).
const defaultProfileRel = "android/build/reports/jacoco/jacocoTestReportAggregated/jacocoTestReportAggregated.xml"

// defaultBaselineRel is the committed baseline's path relative to the repo
// root; the baseline is Android-side data, so it lives under android/.
const defaultBaselineRel = "android/coverage-baseline.json"

// defaultTolerancePct is used only the first time a baseline is generated (no
// existing android/coverage-baseline.json to carry a tolerance forward from).
// It matches the backend/frontend ratchets' committed default; see
// docs/development/coverage.md.
const defaultTolerancePct = 1.5

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("androidcoverageratchet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profile := fs.String("profile", "", "path to the aggregated JaCoCo XML (default: <root>/"+defaultProfileRel+")")
	baseline := fs.String("baseline", "", "path to the baseline JSON (default: <root>/"+defaultBaselineRel+")")
	root := fs.String("root", "", "repository root (default: found by walking up to backend/go.mod)")
	update := fs.Bool("update", false, "regenerate the committed baseline instead of checking it")
	if err := fs.Parse(args); err != nil {
		return 2 // flag.ContinueOnError already printed the usage error itself
	}

	// Resolve the repo root lazily: an explicit -profile and -baseline pair is
	// usable from anywhere, so only walk when a defaulted path needs it.
	repoRoot := ""
	resolveRoot := func() (string, error) {
		if *root != "" {
			return *root, nil
		}
		if repoRoot != "" {
			return repoRoot, nil
		}
		r, err := findRepoRoot()
		if err != nil {
			return "", err
		}
		repoRoot = r
		return r, nil
	}

	profilePath := *profile
	if profilePath == "" {
		r, err := resolveRoot()
		if err != nil {
			fmt.Fprintln(stderr, "androidcoverageratchet:", err) // # pragma: no cover — only reachable outside the repository
			return 2                                             // # pragma: no cover
		}
		profilePath = filepath.Join(r, defaultProfileRel)
	}
	baselinePath := *baseline
	if baselinePath == "" {
		r, err := resolveRoot()
		if err != nil {
			fmt.Fprintln(stderr, "androidcoverageratchet:", err) // # pragma: no cover — only reachable outside the repository
			return 2                                             // # pragma: no cover
		}
		baselinePath = filepath.Join(r, defaultBaselineRel)
	}

	f, err := os.Open(profilePath) // #nosec G304 -- profile is a CLI flag pointing at this run's own report, not user input
	if err != nil {
		fmt.Fprintf(stderr, "androidcoverageratchet: %v\n", err)
		return 2
	}
	defer f.Close() //nolint:errcheck // read-only report handle

	stats, err := androidcoverage.Parse(f)
	if err != nil {
		fmt.Fprintf(stderr, "androidcoverageratchet: %v\n", err)
		return 2
	}

	if *update {
		var existing *androidcoverage.Baseline
		if b, err := androidcoverage.LoadBaseline(baselinePath); err == nil {
			existing = &b
		}
		next := androidcoverage.BuildBaseline(stats, existing, defaultTolerancePct)
		if err := androidcoverage.SaveBaseline(baselinePath, next); err != nil {
			fmt.Fprintf(stderr, "androidcoverageratchet: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "androidcoverageratchet: wrote new baseline to %s (%d files)\n", baselinePath, len(next.Files))
		return 0
	}

	baselineData, err := androidcoverage.LoadBaseline(baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "androidcoverageratchet: %v -- generate it with `go run ./cmd/androidcoverageratchet -update`\n", err)
		return 2
	}

	report := androidcoverage.Compare(baselineData, stats)
	for _, file := range report.NewFiles {
		fmt.Fprintf(stdout, "new (not gated here): %s\n", file)
	}
	for _, file := range report.GoneFiles {
		fmt.Fprintf(stdout, "gone (baseline stale, run -update): %s\n", file)
	}
	for _, finding := range report.Findings {
		fmt.Fprintf(stdout, "DROP: %s\n", finding)
	}

	if !report.OK {
		fmt.Fprintf(stderr, "\nandroidcoverageratchet: FAIL -- %d file(s) dropped coverage past tolerance\n", len(report.DropFiles))
		return 1
	}
	fmt.Fprintln(stdout, "androidcoverageratchet: ok")
	return 0
}

// findRepoRoot walks up from the working directory to backend/go.mod, the
// sentinel the other cmd/*check tools use.
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
			return "", fmt.Errorf("backend/go.mod not found above the working directory")
		}
		dir = parent
	}
}
