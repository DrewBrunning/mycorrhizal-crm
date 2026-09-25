// Command nightlyalertcheck is the drift gate between .github/workflows/*.yml's
// `schedule:` triggers and nightly-failure-alert.yml's
// `on.workflow_run.workflows` registration list.
//
// Nightly is the only zero-retry CI tier here (a PR/push flake gets retried;
// a scheduled run does not), and it is where mutation testing, large-dataset
// benchmarks, chaos injection, and the real-server CardDAV suite all live.
// nightly-failure-alert.yml is the standing alarm for that tier, and its
// `workflows:` list must exactly equal the set of workflow `name:`s that have
// a `schedule:` trigger -- this command fails the build the moment those two
// sets diverge, in either direction.
//
// Exit 0: the two sets match. Exit 1: at least one finding. Exit 2: could not
// run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/nightlyalert"
)

const workflowsDir = ".github/workflows"

func main() {
	os.Exit(run(os.Stdout)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(w io.Writer) int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nightlyalertcheck:", err) // # pragma: no cover — only reachable when run outside the repository
		return 2                                           // # pragma: no cover
	}
	return runAt(w, root)
}

// runAt is the testable core: it reads every workflow file under
// <root>/.github/workflows, runs the drift check, and returns the process
// exit code.
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

	alertBytes, ok := files[nightlyalert.AlertWorkflowFile]
	if !ok {
		fmt.Fprintln(w, workflowsDir+"/"+nightlyalert.AlertWorkflowFile+" does not exist")
		return 2
	}

	scheduled, findings := nightlyalert.ScheduledWorkflowNames(files)
	registered, err := nightlyalert.RegisteredWorkflowNames(alertBytes)
	if err != nil {
		fmt.Fprintln(w, err.Error())
		return 2
	}
	findings = append(findings, nightlyalert.CheckDrift(scheduled, registered)...)

	if len(findings) == 0 {
		fmt.Fprintf(w, "nightlyalertcheck OK: %d scheduled workflow(s), all registered in %s\n",
			len(scheduled), nightlyalert.AlertWorkflowFile)
		return 0
	}
	for _, f := range findings {
		fmt.Fprintln(w, f)
	}
	fmt.Fprintf(w, "\n%d nightly-alert-drift finding(s).\n", len(findings))
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
