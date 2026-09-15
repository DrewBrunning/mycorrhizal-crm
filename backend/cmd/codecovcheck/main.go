// Command codecovcheck is the structural gate for codecov.yml (issue #979).
//
// codecov.yml sits at the repo root, outside every suite's own review: a PR
// that edits only codecov.yml sets every .github/filters.yaml `changes`
// output false, so the suites whose coverage it governs never run, and
// nothing in CI parsed the file at all -- a one-line target/threshold/
// ignore: edit merged on human review alone. This command closes that gap:
//
//  1. codecov.yml's coverage.status.patch.* areas (target/threshold/flags/
//     only_pulls) must match, byte-for-byte after YAML parsing, the fenced
//     example in docs/development/coverage.md between the
//     codecov-patch-status markers. A target/threshold change that isn't
//     also a doc change fails.
//  2. Every entry in codecov.yml's ignore: list must carry a justifying `#`
//     comment (docs/development/coverage.md's Override path already asks
//     for this in prose; this makes it mechanical).
//
// Exit 0: consistent. Exit 1: at least one finding. Exit 2: could not run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"mycorrhizal/internal/covgate"
)

const (
	codecovFile = "codecov.yml"
	docFile     = "docs/development/coverage.md"
)

func main() {
	os.Exit(run(os.Stdout)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(w io.Writer) int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "codecovcheck:", err) // # pragma: no cover
		return 2                                      // # pragma: no cover
	}
	return runAt(w, root)
}

// runAt is the testable core: it reads codecov.yml and the coverage doc
// under root, runs every check, and returns the process exit code.
func runAt(w io.Writer, root string) int {
	// #nosec G304 -- root is findRepoRoot's output (or a test temp dir), the leaf is a constant
	codecovBytes, err := os.ReadFile(filepath.Join(root, codecovFile))
	if err != nil {
		fmt.Fprintln(w, "cannot read "+codecovFile+": "+err.Error())
		return 2
	}
	// #nosec G304 -- root is findRepoRoot's output (or a test temp dir), the leaf is a constant
	docBytes, err := os.ReadFile(filepath.Join(root, docFile))
	if err != nil {
		fmt.Fprintln(w, "cannot read "+docFile+": "+err.Error())
		return 2
	}

	var findings []string

	codecovAreas, f := covgate.ParsePatchAreas(codecovBytes, codecovFile)
	findings = append(findings, f...)

	docBlock, f := covgate.ExtractDocPatchStatusBlock(string(docBytes))
	findings = append(findings, f...)

	if codecovAreas != nil && docBlock != "" {
		docAreas, f := covgate.ParsePatchAreas([]byte(docBlock), docFile)
		findings = append(findings, f...)
		if docAreas != nil {
			findings = append(findings, covgate.CrossCheckPatchAreas(codecovAreas, docAreas)...)
		}
	}

	findings = append(findings, covgate.CheckIgnoreEntriesJustified(codecovBytes)...)

	if len(findings) == 0 {
		fmt.Fprintf(w, "codecov.yml OK: patch-status targets match %s, every ignore: entry is justified\n", docFile)
		return 0
	}
	sort.Strings(findings)
	for _, f := range findings {
		fmt.Fprintln(w, f)
	}
	fmt.Fprintf(w, "\n%d codecov.yml finding(s). Fix %s and/or %s.\n", len(findings), codecovFile, docFile)
	return 1
}

// findRepoRoot walks up from the working directory to backend/go.mod, the
// sentinel cmd/docscheck / cmd/releasegatecheck use.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err // # pragma: no cover
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "backend", "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod)") // # pragma: no cover
		}
		dir = parent
	}
}
