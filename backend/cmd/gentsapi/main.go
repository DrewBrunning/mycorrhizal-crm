// Command gentsapi regenerates frontend/src/generated/openapi.ts — the
// TypeScript mirror of backend/openapi.yaml's components.schemas — so the
// web client's hand-written API types can be checked against the spec at
// `tsc --noEmit` time (frontend/src/api/contractConformance.ts).
//
// Workflow when a response/request schema changes:
//
//  1. edit backend/openapi.yaml
//  2. run `go run ./cmd/gentsapi` (or `make gen-ts-api`) from backend/
//  3. fix whatever `npx tsc --noEmit` now reports in frontend/src/api/
//
// The drift test (TestGeneratedTSTypesMatchSpec) enforces step 2 in CI.
//
// Exit status 0 means the file was regenerated in place; 1 means the spec
// is broken or uses a construct the generator cannot map; 2 means the
// command could not run.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/internal/contractfixtures"
	"mycorrhizal/internal/tsapi"
)

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable.
func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gentsapi:", err)
		return 2
	}
	doc, err := contractfixtures.Load(filepath.Join(root, "backend", contractfixtures.SpecPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gentsapi:", err)
		return 1
	}
	body, err := tsapi.Generate(doc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gentsapi:", err)
		return 1
	}
	out := filepath.Join(root, tsapi.OutputPath)
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "gentsapi:", err)
		return 2
	}
	if err := os.WriteFile(out, body, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "gentsapi:", err)
		return 2
	}
	fmt.Printf("Generated %s\n", out)
	return 0
}

// findRepoRoot walks up from the working directory until it finds the
// directory containing backend/openapi.yaml, so the command works from
// backend/ (the documented invocation) and from anywhere below the repo.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", contractfixtures.SpecPath)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for backend/%s)", dir, contractfixtures.SpecPath)
		}
		dir = parent
	}
}
