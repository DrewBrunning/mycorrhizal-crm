// Command coverageratchet is the backend half of the per-file
// no-regression coverage ratchet (frontend counterpart:
// frontend/scripts/check-coverage-ratchet.mjs), closing the gap the
// diff-based codecov/patch/backend status leaves: a file that already sits
// at 0% coverage stays there forever, and a PR that deletes or guts a test
// for a file it doesn't otherwise touch trips no status at all.
//
// It reads the CROSS-PACKAGE coverprofile (issue #1477): the `backend` job in
// .github/workflows/unit-tests.yml runs `go test ./... -coverpkg=./...` once
// and writes backend/coverage-cross.out, so a file is credited for a test in
// ANY package -- the ratchet asks "is any test exercising this file", while
// Codecov's patch gate (per-package profiles) asks "does this package's own
// suite cover the line". Blocks repeated in the profile are merged (counts
// summed) while reading. It compares each file's statement coverage % against the committed baseline
// (backend/internal/coverageratchet/testdata/baseline.json).
//
// Normal workflow:
//
//	cd backend && go run ./cmd/coverageratchet                # check (default)
//	cd backend && go run ./cmd/coverageratchet -update         # regenerate the baseline
//	cd backend && go run ./cmd/coverageratchet -profile x.out  # check a different profile
//
// Generate the profile (heavy: whole suite, ~4 min), by hand:
//
//	go test ./... -coverpkg=./... -covermode=atomic -p 4 -coverprofile=coverage-cross.out
//
// Exit status 0: within tolerance (or baseline regenerated). 1: at least
// one file's coverage dropped past tolerance. 2: the command could not
// run (missing/malformed profile or baseline).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/internal/coverageratchet"
)

// modulePrefix matches this module's own name (go.mod's `module
// mycorrhizal`) plus the trailing slash coverprofile paths always have
// after it (e.g. "mycorrhizal/internal/foo/bar.go").
const modulePrefix = "mycorrhizal/"

// defaultTolerancePct is used only the first time a baseline is generated
// (no existing testdata/baseline.json to carry a tolerance forward from).
// See docs/development/coverage.md for the empirical run-to-run variance
// check this number is based on.
const defaultTolerancePct = 1.5

const baselinePath = "internal/coverageratchet/testdata/baseline.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, w interface{ Write([]byte) (int, error) }) int {
	fs := flag.NewFlagSet("coverageratchet", flag.ContinueOnError)
	profile := fs.String("profile", "coverage-cross.out", "path to the cross-package Go coverprofile (go test ./... -coverpkg=./...)")
	update := fs.Bool("update", false, "regenerate the committed baseline instead of checking it")
	root := fs.String("root", ".", "source root the coverprofile's module paths resolve against (normally backend/)")
	if err := fs.Parse(args); err != nil {
		return 2 // # pragma: no cover — flag.ContinueOnError already prints the usage error itself
	}

	f, err := os.Open(*profile) // #nosec G304 -- profile is a CLI flag pointing at this run's own coverage.out, not user input
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverageratchet: %v\n", err)
		return 2
	}
	defer f.Close() //nolint:errcheck // read-only baseline/profile handle

	blocks, err := coverageratchet.ParseMergedCoverprofile(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverageratchet: %v\n", err)
		return 2
	}

	blocks, err = coverageratchet.FilterPragma(blocks, *root, modulePrefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverageratchet: %v\n", err)
		return 2
	}

	stats := coverageratchet.ToRelative(coverageratchet.PerFileStats(blocks), modulePrefix)

	baselineFile := filepath.Join(*root, baselinePath)

	if *update {
		var existing *coverageratchet.Baseline
		if b, err := coverageratchet.LoadBaseline(baselineFile); err == nil {
			existing = &b
		}
		next := coverageratchet.BuildBaseline(stats, existing, defaultTolerancePct)
		if err := coverageratchet.SaveBaseline(baselineFile, next); err != nil {
			fmt.Fprintf(os.Stderr, "coverageratchet: %v\n", err)
			return 2
		}
		fmt.Fprintf(w, "coverageratchet: wrote new baseline to %s (%d files)\n", baselineFile, len(next.Files))
		return 0
	}

	baseline, err := coverageratchet.LoadBaseline(baselineFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverageratchet: %v -- generate it with `go run ./cmd/coverageratchet -update`\n", err)
		return 2
	}

	report := coverageratchet.Compare(baseline, stats)
	for _, file := range report.NewFiles {
		fmt.Fprintf(w, "new (not gated here): %s\n", file)
	}
	for _, file := range report.GoneFiles {
		fmt.Fprintf(w, "gone (baseline stale, run -update): %s\n", file)
	}
	for _, finding := range report.Findings {
		fmt.Fprintf(w, "DROP: %s\n", finding)
	}

	if !report.OK {
		fmt.Fprintf(os.Stderr, "\ncoverageratchet: FAIL -- %d file(s) dropped coverage past tolerance\n", len(report.DropFiles))
		return 1
	}
	fmt.Fprintln(w, "coverageratchet: ok")
	return 0
}
