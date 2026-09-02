// Command genintegrationmatrix regenerates the INT-01 integration
// classification matrix (issue #464) from backend/integrations.
//
// The matrix is a committed artifact at docs/int-01-integration-classification-matrix.md —
// a projection of integrations.Registry() + integrations.Dispositions(), never
// a hand-authored second source. The drift test
// backend/integrations/matrix_test.go enforces that the committed copy and this
// generator agree, so a classification change is always a reviewable diff.
//
// Normal workflow:
//
//	cd backend && go run ./cmd/genintegrationmatrix
//
// Exit status 0 means the matrix was regenerated in place; 2 means the command
// could not run (no repo root found / write failure).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/integrations"
)

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable.
func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genintegrationmatrix:", err)
		return 2
	}

	path := filepath.Join(root, integrations.MatrixDocRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "genintegrationmatrix:", err)
		return 2
	}
	if err := os.WriteFile(path, []byte(integrations.Render()), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "genintegrationmatrix:", err)
		return 2
	}
	fmt.Printf("Integration classification matrix written to %s (%d integrations)\n",
		path, len(integrations.Registry()))
	return 0
}

// findRepoRoot walks up from the working directory until it finds the
// repository root (the directory containing backend/integrations), so the
// command works from backend/ (the documented invocation) and from anywhere
// below the repo.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "integrations")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for backend/integrations)", dir)
		}
		dir = parent
	}
}
