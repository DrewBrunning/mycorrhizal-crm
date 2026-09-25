// Command pragmacheck is the structural gate for coverage-override markers
// (docs/development/coverage.md's "Override path"): every
// `// # pragma: no cover` on non-test Go under backend/, and every
// `/* v8 ignore ... */` under frontend/src, must carry a discoverable
// reason. See internal/pragmacheck for the exact rule.
//
// Exit 0: every marker has a reason. Exit 1: at least one finding. Exit 2:
// the check itself could not run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/pragmacheck"
)

// osExit is os.Exit through a seam so tests can drive main() itself without
// killing the test process.
var osExit = os.Exit

func main() {
	osExit(run(os.Stdout))
}

func run(w io.Writer) int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pragmacheck:", err)
		return 2
	}
	return runAt(w, root)
}

// runAt is the testable core: it scans backend/ and frontend/src under root
// and returns the process exit code.
func runAt(w io.Writer, root string) int {
	findings, err := pragmacheck.CheckTree(filepath.Join(root, "backend"), filepath.Join(root, "frontend", "src"))
	if err != nil {
		fmt.Fprintln(w, "pragmacheck: scanning:", err)
		return 2
	}

	if len(findings) == 0 {
		fmt.Fprintln(w, "pragmacheck OK: every no-cover/v8-ignore marker carries a reason")
		return 0
	}
	for _, f := range findings {
		fmt.Fprintf(w, "%s:%d: no-cover marker has no discoverable reason: %s\n", f.Path, f.Line, f.Text)
	}
	fmt.Fprintf(w, "\n%d marker(s) with no reason. Add one on the marker's own line, or on the line directly above (docs/development/coverage.md).\n", len(findings))
	return 1
}

// findRepoRoot walks up from the working directory until it finds
// backend/go.mod -- the sentinel cmd/docscheck and friends use.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err // # pragma: no cover — os.Getwd fails only when the cwd has been deleted out from under the process
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "backend", "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod)")
		}
		dir = parent
	}
}
