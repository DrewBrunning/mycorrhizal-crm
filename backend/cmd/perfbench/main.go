// Command perfbench measures the benchmark suites and regenerates their
// committed artifacts:
//
// PERF-02 (core-operation benchmarks, issue #469):
//   - backend/internal/perfbench/testdata/baseline.json — the diffable
//     regression gate: query count + result-set size per operation per
//     profile, plus the growth class each is allowed.
//   - docs/development/perf-benchmarks.md — the human-facing report, which
//     additionally carries wall-clock medians (indicative only).
//
// PERF-03 (data-movement benchmarks, issue #470):
//   - backend/internal/perfbench/testdata/datamovement-baseline.json — the
//     gate: rows touched + memory-growth class + the >5s write-lock stall
//     flag per bulk operation per profile.
//   - docs/development/data-movement-benchmarks.md — the report, which adds
//     the indicative durations, peak heap and peak disk.
//
// Normal workflow:
//
//	cd backend && go run ./cmd/perfbench          # smoke + typical
//	cd backend && go run ./cmd/perfbench -large   # + the large profile (~2 min seed)
//	cd backend && go run ./cmd/perfbench -check   # verify the committed files are current
//
// Exit status 0: files regenerated (or, with -check, already current). 1: a
// measurement or -check comparison failed. 2: the command could not run.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/internal/largedata"
	"mycorrhizal/internal/perfbench"
)

// measureFunc / measureDMFunc are the two measurement passes; tests inject fakes.
type (
	measureFunc   func([]largedata.Profile) (perfbench.Suite, error)
	measureDMFunc func([]largedata.Profile) (perfbench.DataMovementSuite, error)
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, "", measureAll, measureDataMovement)) // # pragma: no cover — os.Exit; tests drive run() directly
}

// measureAll / measureDataMovement are the production measurement paths.
func measureAll(profiles []largedata.Profile) (perfbench.Suite, error) { // # pragma: no cover — thin delegate; perfbench.TestCoreOperationBenchmarks exercises RunAll
	return perfbench.RunAll(profiles, "", nil)
}

func measureDataMovement(profiles []largedata.Profile) (perfbench.DataMovementSuite, error) { // # pragma: no cover — thin delegate; perfbench.TestDataMovementBenchmarks exercises RunAllDataMovement
	return perfbench.RunAllDataMovement(profiles, "", nil)
}

func run(args []string, stdout io.Writer, startDir string, measure measureFunc, measureDM measureDMFunc) int {
	fs := flag.NewFlagSet("perfbench", flag.ContinueOnError)
	fs.SetOutput(stdout)
	large := fs.Bool("large", false, "also measure the large profile (~2 min seed)")
	check := fs.Bool("check", false, "do not write; exit non-zero if the committed files are stale")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if os.Getenv("MYCORRHIZAL_LARGE_TESTS") == "1" {
		*large = true
	}

	root, err := perfbench.FindRepoRoot(startDir)
	if err != nil {
		fmt.Fprintln(stdout, "perfbench:", err)
		return 2
	}

	profiles := []largedata.Profile{largedata.Smoke, largedata.Typical}
	if *large {
		profiles = append(profiles, largedata.Large)
	}

	// --- PERF-02: core operations ---
	fmt.Fprintf(stdout, "perfbench: measuring %d core operations across %d profile(s)...\n", len(perfbench.Registry()), len(profiles))
	suite, err := measure(profiles)
	if err != nil {
		fmt.Fprintln(stdout, "perfbench:", err)
		return 1
	}
	baselineJSON, reportMD, err := suite.Artifacts()
	if err != nil { // # pragma: no cover — the baseline struct always marshals
		fmt.Fprintln(stdout, "perfbench:", err)
		return 1
	}

	// --- PERF-03: data-movement operations ---
	fmt.Fprintf(stdout, "perfbench: measuring %d data-movement operations across %d profile(s)...\n", len(perfbench.DataMovementRegistry()), len(profiles))
	dmSuite, err := measureDM(profiles)
	if err != nil {
		fmt.Fprintln(stdout, "perfbench:", err)
		return 1
	}
	dmBaselineJSON, dmReportMD, err := dmSuite.Artifacts()
	if err != nil { // # pragma: no cover — the baseline struct always marshals
		fmt.Fprintln(stdout, "perfbench:", err)
		return 1
	}

	if *check {
		stale := false
		if path, s := perfbench.CheckBaseline(root, baselineJSON); s {
			fmt.Fprintf(stdout, "perfbench: STALE %s\n", path)
			stale = true
		}
		if path, s := perfbench.CheckDataMovementBaseline(root, dmBaselineJSON); s {
			fmt.Fprintf(stdout, "perfbench: STALE %s\n", path)
			stale = true
		}
		if stale {
			fmt.Fprintln(stdout, "perfbench: run `make gen-perf-baseline` and commit the diff")
			return 1
		}
		fmt.Fprintln(stdout, "perfbench: committed baselines are current")
		return 0
	}

	if err := perfbench.WriteArtifacts(root, baselineJSON, reportMD); err != nil {
		fmt.Fprintln(stdout, "perfbench:", err)
		return 2
	}
	if err := perfbench.WriteDataMovementArtifacts(root, dmBaselineJSON, dmReportMD); err != nil { // # pragma: no cover — same two dirs WriteArtifacts just wrote to; only reachable if they vanish mid-run (TestRun_WriteFailureIsExit2 covers the sibling)
		fmt.Fprintln(stdout, "perfbench:", err)
		return 2
	}
	bPath, rPath := perfbench.ArtifactPaths(root)
	dmBPath, dmRPath := perfbench.DataMovementArtifactPaths(root)
	fmt.Fprintf(stdout, "perfbench: wrote %s\nperfbench: wrote %s\nperfbench: wrote %s\nperfbench: wrote %s\n", bPath, rPath, dmBPath, dmRPath)
	return 0
}
