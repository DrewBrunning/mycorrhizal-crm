// Command gencompatmatrix regenerates the DATA-01 field compatibility matrix
// (issue #441) from the locked correspondence oracle and the issue #515
// canonical-field audit.
//
// The matrix is a committed artifact at docs/data-01-field-compatibility-matrix.md —
// a projection of backend/correspondence/testdata/correspondence.tsv (ADR-0002),
// never a hand-authored second source of mapping truth. It classifies every
// canonical field per serialized format (vCard 4.0 / vCard 3.0 / JSContact /
// CardDAV-on-the-wire) into the v0.6.5 milestone's five buckets and exposes the
// unsupported/lossy set as the DATA-02 loss-report input.
//
// Normal workflow:
//
//	cd backend && go run ./cmd/gencompatmatrix
//
// The drift test backend/correspondence/matrix_test.go enforces that the
// committed copy and this generator agree, so a correspondence-table change
// that alters the matrix shows up as a reviewable diff rather than silent
// drift.
//
// Exit status 0 means the matrix was regenerated in place; 2 means the command
// could not run (no repo root found / write failure).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/correspondence"
)

// matrixRelPath is the committed artifact, relative to the repository root.
const matrixRelPath = "docs/data-01-field-compatibility-matrix.md"

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable.
func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencompatmatrix:", err)
		return 2
	}

	path := filepath.Join(root, matrixRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "gencompatmatrix:", err)
		return 2
	}
	if err := os.WriteFile(path, []byte(correspondence.Render()), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "gencompatmatrix:", err)
		return 2
	}
	fmt.Printf("Field compatibility matrix written to %s (%d canonical fields, %d loss reports)\n",
		path, len(correspondence.Build()), len(correspondence.LossReports()))
	return 0
}

// findRepoRoot walks up from the working directory until it finds the
// repository root (the directory containing backend/correspondence), so the
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
		if _, err := os.Stat(filepath.Join(dir, "backend", "correspondence")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for backend/correspondence)", dir)
		}
		dir = parent
	}
}
