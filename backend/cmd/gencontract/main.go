// Command gencontract regenerates the shared contract-test fixtures from
// backend/openapi.yaml's response examples (issue #266).
//
// The web and Android contract suites (frontend/src/api/contractFixtures.test.ts
// and android/core/network/.../ContractFixtureTest.kt) both parse the same
// checked-in files under testdata/contract-fixtures/. Those files are
// generated, not hand-captured: each pinned response's `example:` in the
// OpenAPI spec IS the fixture. When the backend response contract changes,
// the workflow is:
//
//  1. edit the example (and, if needed, the schema) in backend/openapi.yaml
//  2. run `go run ./cmd/gencontract` (or `make gen-contract-fixtures`) from backend/
//  3. review the fixture diff — the drift test
//     (TestContractFixturesMatchSpec) enforces step 2 in CI
//
// Exit status 0 means the fixtures were regenerated in place; 1 means the
// spec or a pinned example is broken; 2 means the command could not run.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/internal/contractfixtures"
)

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable.
func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencontract:", err)
		return 2
	}
	doc, err := contractfixtures.Load(filepath.Join(root, "backend", contractfixtures.SpecPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencontract:", err)
		return 1
	}
	files, err := contractfixtures.Generate(doc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencontract:", err)
		return 1
	}

	dir := filepath.Join(root, contractfixtures.FixturesDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "gencontract:", err)
		return 2
	}
	for _, pin := range contractfixtures.Pinned {
		body := files[pin.Filename]
		if err := os.WriteFile(filepath.Join(dir, pin.Filename), body, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "gencontract:", err)
			return 2
		}
		fmt.Printf("Generated %s\n", pin.Filename)
	}
	fmt.Printf("Fixtures written to %s\n", dir)
	return 0
}

// findRepoRoot walks up from the working directory until it finds the
// repository root (the directory containing backend/openapi.yaml and
// testdata/contract-fixtures/), so the command works from backend/ (the
// documented invocation) and from anywhere below the repo.
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
