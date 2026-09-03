// Command perfbench measures the PERF-02 core-operation benchmarks (issue
// #469) and regenerates two committed artifacts:
//
//   - backend/internal/perfbench/testdata/baseline.json — the diffable
//     regression gate: query count + result-set size per operation per
//     profile, plus the growth class each is allowed.
//   - docs/development/perf-benchmarks.md — the human-facing report, which
//     additionally carries wall-clock medians (indicative only).
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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, "", measureAll)) // # pragma: no cover — os.Exit; tests drive run() directly
}

// measureAll is the production measurement path; tests inject a fake.
func measureAll(profiles []largedata.Profile) (perfbench.Suite, error) { // # pragma: no cover — thin delegate; perfbench.TestCoreOperationBenchmarks exercises RunAll
	return perfbench.RunAll(profiles, "", nil)
}

func run(args []string, stdout io.Writer, startDir string, measure func([]largedata.Profile) (perfbench.Suite, error)) int {
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

	fmt.Fprintf(stdout, "perfbench: measuring %d operations across %d profile(s)...\n", len(perfbench.Registry()), len(profiles))
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

	if *check {
		if path, stale := perfbench.CheckBaseline(root, baselineJSON); stale {
			fmt.Fprintf(stdout, "perfbench: STALE %s\n", path)
			fmt.Fprintln(stdout, "perfbench: run `make gen-perf-baseline` and commit the diff")
			return 1
		}
		fmt.Fprintln(stdout, "perfbench: committed baseline is current")
		return 0
	}

	if err := perfbench.WriteArtifacts(root, baselineJSON, reportMD); err != nil {
		fmt.Fprintln(stdout, "perfbench:", err)
		return 2
	}
	baselinePath, reportPath := perfbench.ArtifactPaths(root)
	fmt.Fprintf(stdout, "perfbench: wrote %s\nperfbench: wrote %s\n", baselinePath, reportPath)
	return 0
}
