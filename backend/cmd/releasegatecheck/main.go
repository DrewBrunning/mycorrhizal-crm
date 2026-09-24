// Command releasegatecheck is the structural gate for the release-gate registry
// (REL-03, issue #447).
//
// docs/development/release-gates.md is the canonical list of what must pass
// before a tag publishes; .github/release-gates.json is its machine-readable
// twin, read by the `release-gate` job in docker-publish.yml. An unenforced
// list is a wish, so this command re-verifies on every run:
//
//  1. The JSON parses and every gate obeys the registry's structural rules
//     (known tier / check_kind, unique name, non-empty criterion, and the
//     release_gate:true invariants).
//  2. Every gate names a workflow file that actually exists under
//     .github/workflows/.
//  3. The human-readable table in docs/development/release-gates.md has exactly
//     one row per registry gate, with a matching tier and mandatory flag — so a
//     gate cannot be added to one without the other, and a listed gate can
//     never lack a real job.
//  4. Every workflow the release composer must call (each release_gate:true
//     gate and each release-tier suite) declares a top-level `workflow_call`
//     trigger, so ADR 0021's composition is possible and a new mandatory gate
//     cannot silently reintroduce the dispatch-and-poll path; and the composer
//     (`release-validate.yml`) calls exactly that set — no omission, no extra.
//
// Exit 0: everything lines up. Exit 1: at least one finding. Exit 2: the check
// itself could not run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"mycorrhizal/internal/releasegates"
	"mycorrhizal/internal/releaseworkflow"
)

const (
	registryFile = ".github/release-gates.json"
	docFile      = "docs/development/release-gates.md"
	workflowsDir = ".github/workflows"
	composerFile = ".github/workflows/release-validate.yml"
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
		fmt.Fprintln(os.Stderr, "releasegatecheck:", err)
		return 2
	}
	return runAt(w, root)
}

// runAt is the testable core: it runs every check rooted at root (the real
// repository root in production, a constructed temp directory in tests) and
// returns the process exit code.
func runAt(w io.Writer, root string) int {
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	regBytes, err := os.ReadFile(filepath.Join(root, registryFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasegatecheck: read", registryFile, err)
		return 2
	}
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	docBytes, err := os.ReadFile(filepath.Join(root, docFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasegatecheck: read", docFile, err)
		return 2
	}

	reg, findings := releasegates.Parse(regBytes)
	findings = append(findings, releasegates.CheckWorkflows(reg, func(name string) bool {
		_, statErr := os.Stat(filepath.Join(root, workflowsDir, name))
		return statErr == nil
	})...)
	findings = append(findings, releasegates.CheckCallable(reg, func(name string) (string, bool) {
		// #nosec G304 -- name is a gate's declared workflow filename, and the
		// read is confined to the .github/workflows directory.
		b, readErr := os.ReadFile(filepath.Join(root, workflowsDir, name))
		return string(b), readErr == nil
	})...)
	// #nosec G304 -- constant leaf under the repository root
	composerBytes, err := os.ReadFile(filepath.Join(root, composerFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasegatecheck: read", composerFile, err)
		return 2
	}
	findings = append(findings, releasegates.CheckComposer(reg, string(composerBytes))...)
	// #nosec G304 -- constant leaf under the repository root
	releaseBytes, err := os.ReadFile(filepath.Join(root, workflowsDir, "release.yml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasegatecheck: read release.yml", err)
		return 2
	}
	findings = append(findings, releaseworkflow.CheckRelease(string(releaseBytes))...)
	// #nosec G304 -- constant leaf under the repository root
	promoteBytes, err := os.ReadFile(filepath.Join(root, workflowsDir, "promote-rc.yml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasegatecheck: read promote-rc.yml", err)
		return 2
	}
	findings = append(findings, releaseworkflow.CheckPromote(string(promoteBytes))...)
	findings = append(findings, releasegates.CrossCheckDoc(reg, string(docBytes))...)

	if len(findings) == 0 {
		fmt.Fprintf(w, "release gates OK: %d gates, all workflows exist and are composable, the composer covers them exactly, doc table matches (%s)\n",
			len(reg.Gates), registryFile)
		return 0
	}
	sort.Strings(findings)
	for _, f := range findings {
		fmt.Fprintln(w, f)
	}
	fmt.Fprintf(w, "\n%d release-gate finding(s). Fix %s and/or %s.\n", len(findings), registryFile, docFile)
	return 1
}

// findRepoRoot walks up from the working directory until it finds backend/go.mod
// — the sentinel cmd/docscheck and cmd/deprecations use.
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
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod) from working directory")
		}
		dir = parent
	}
}
