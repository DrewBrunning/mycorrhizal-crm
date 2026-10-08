// Command mainalertcheck is the drift gate between .github/release-gates.json's
// mandatory per-pr gates and main-failure-alert.yml's
// `on.workflow_run.workflows` registration list (issue #1568).
//
// A PR merged before its required checks finish -- or via an admin bypass --
// leaves the post-merge `push` run on `main` as the only verification the
// change ever gets. main-failure-alert.yml is the alarm for a red `main` push
// run; this command fails the build the moment a workflow carrying a
// mandatory per-pr gate is missing from that alarm's registration list, so a
// gate can never silently go unwatched on `main`.
//
// Exit 0: every mandatory per-pr workflow is registered. Exit 1: at least one
// finding. Exit 2: could not run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/mainalert"
	"mycorrhizal/internal/releasegates"
)

const workflowsDir = ".github/workflows"

func main() {
	os.Exit(run(os.Stdout)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(w io.Writer) int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "mainalertcheck:", err) // # pragma: no cover — only reachable when run outside the repository
		return 2                                        // # pragma: no cover
	}
	return runAt(w, root)
}

// runAt is the testable core: it reads every workflow file and the
// release-gate registry under <root>, runs the drift check, and returns the
// process exit code.
func runAt(w io.Writer, root string) int {
	dir := filepath.Join(root, workflowsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(w, "cannot read "+workflowsDir+": "+err.Error())
		return 2
	}

	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".yml" && filepath.Ext(name) != ".yaml" {
			continue
		}
		// #nosec G304 -- dir is workflowsDir under the repo root, name is a
		// directory entry we just listed
		b, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			fmt.Fprintln(w, "cannot read "+workflowsDir+"/"+name+": "+rerr.Error()) // # pragma: no cover — a just-listed workflow file becoming unreadable is a filesystem race, not a testable input
			return 2                                                                // # pragma: no cover
		}
		files[name] = b
	}

	alertBytes, ok := files[mainalert.AlertWorkflowFile]
	if !ok {
		fmt.Fprintln(w, workflowsDir+"/"+mainalert.AlertWorkflowFile+" does not exist")
		return 2
	}

	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	regBytes, err := os.ReadFile(filepath.Join(root, mainalert.RegistryFile))
	if err != nil {
		fmt.Fprintln(w, "cannot read "+mainalert.RegistryFile+": "+err.Error())
		return 2
	}
	reg, regFindings := releasegates.Parse(regBytes)
	if len(regFindings) > 0 {
		for _, f := range regFindings {
			fmt.Fprintln(w, f)
		}
		fmt.Fprintf(w, "\n%d release-gate registry finding(s).\n", len(regFindings))
		return 1
	}

	expected, findings := mainalert.ExpectedWorkflowNames(reg, files)
	registered, err := mainalert.RegisteredWorkflowNames(alertBytes)
	if err != nil {
		fmt.Fprintln(w, err.Error())
		return 2
	}
	findings = append(findings, mainalert.CheckDrift(expected, registered)...)

	if len(findings) == 0 {
		fmt.Fprintf(w, "mainalertcheck OK: %d mandatory per-pr workflow(s), all registered in %s\n",
			len(expected), mainalert.AlertWorkflowFile)
		return 0
	}
	for _, f := range findings {
		fmt.Fprintln(w, f)
	}
	fmt.Fprintf(w, "\n%d main-alert-drift finding(s).\n", len(findings))
	return 1
}

// findRepoRoot walks up from the working directory to backend/go.mod, the
// sentinel cmd/docscheck / cmd/releasegatecheck use.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err // # pragma: no cover — Getwd fails only when the cwd has been deleted
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "backend", "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod)") // # pragma: no cover — only reachable when run outside the repository
		}
		dir = parent
	}
}
