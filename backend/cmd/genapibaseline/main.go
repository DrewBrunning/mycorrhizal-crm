// Command genapibaseline regenerates the frozen /api/v1 contract baseline from
// backend/openapi.yaml (MAINT-02, issue #491).
//
// The baseline is the breaking-change detector's reference surface: the drift
// test (backend/internal/apibaseline/drift_test.go) asserts the live spec is a
// SUPERSET of the committed snapshot. Regenerating the baseline is therefore
// the deliberate act of declaring a breaking change — it records what additive
// change is free to build on, and removing any of it is a flagged break.
//
// Normal workflow:
//
//	cd backend && go run ./cmd/genapibaseline
//
// Exit status 0 means the baseline was regenerated in place; 1 means the spec
// or the extraction is broken; 2 means the command could not run.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/internal/apibaseline"
)

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genapibaseline:", err)
		return 2
	}
	doc, err := apibaseline.Load(filepath.Join(root, "backend", apibaseline.SpecPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "genapibaseline:", err)
		return 1
	}
	baseline := apibaseline.Extract(doc)
	data, err := baseline.Marshal()
	if err != nil { // # pragma: no cover — MarshalIndent of this struct can never fail
		fmt.Fprintln(os.Stderr, "genapibaseline:", err)
		return 1
	}

	path := filepath.Join(root, "backend", "internal", "apibaseline", apibaseline.BaselineFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "genapibaseline:", err)
		return 2
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "genapibaseline:", err)
		return 2
	}
	fmt.Printf("Baseline written to %s (%d operations, %d schemas)\n", path, len(baseline.Operations), len(baseline.Schemas))
	return 0
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil { // # pragma: no cover — Getwd fails only when the cwd has been deleted, which no test can arrange for its own process
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", apibaseline.SpecPath)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo root not found above %s", dir)
		}
		dir = parent
	}
}
