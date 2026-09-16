// Command genconfigreference regenerates the operator-facing configuration
// register (issue #933, #501 action 1-2/7) from the `cfgreg` struct tags on
// config.Config.
//
// The register is a committed artifact at docs/configuration-reference.md —
// generated, never hand-authored, so it cannot silently drift from
// config.Config the way a hand-written reference could. The completeness
// test config.TestConfigRegisterCoversEveryField fails CI if a Config field
// is added without a cfgreg tag, and the drift test
// config.TestConfigurationReferenceDocUpToDate fails until this command is
// re-run after a tag changes.
//
// Normal workflow:
//
//	cd backend && go run ./cmd/genconfigreference
//
// Exit status 0 means the reference was regenerated in place; 2 means the
// command could not run (no repo root found / write failure).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/config"
)

// referenceRelPath is the committed artifact, relative to the repository root.
const referenceRelPath = "docs/configuration-reference.md"

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable.
func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genconfigreference:", err)
		return 2
	}

	path := filepath.Join(root, referenceRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "genconfigreference:", err)
		return 2
	}
	if err := os.WriteFile(path, []byte(config.RenderRegister()), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "genconfigreference:", err)
		return 2
	}
	fmt.Printf("Configuration reference written to %s (%d Config fields, %d out-of-Config variables)\n",
		path, len(config.Register()), len(config.OutOfConfigVars))
	return 0
}

// findRepoRoot walks up from the working directory until it finds the
// repository root (the directory containing backend/config), so the command
// works from backend/ (the documented invocation) and from anywhere below
// the repo.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "config")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for backend/config)", dir)
		}
		dir = parent
	}
}
