// Command prepr is the local pre-PR gate (issue #1964): it selects the CI
// areas a branch touches from .github/filters.yaml and runs the corresponding
// checks.
//
// The pre-commit hook is deliberately a fast subset. This command adds the two
// things it leaves out -- the full area suites with their coverage ratchets,
// and a flake-stress pass over the tests the branch added or changed -- so the
// failure classes that used to surface only after a PR opened are caught first.
//
// Usage:
//
//	scripts/pre-pr.sh --fast          # lint/build/drift checks for touched areas
//	scripts/pre-pr.sh --full          # + area suites, coverage ratchets, flake stress
//	scripts/pre-pr.sh --full --dry-run
//
// Exit status: 0 every executed step passed; 1 a step failed; 2 the gate could
// not run (bad flags, missing filters file, git failure).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mycorrhizal/internal/gitchanges"
	"mycorrhizal/internal/prepr"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("prepr", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fast := fs.Bool("fast", false, "lint/build/drift checks for the touched areas")
	full := fs.Bool("full", false, "fast checks plus area suites, coverage ratchets, and flake stress")
	base := fs.String("base", "origin/main", "git ref to diff against")
	root := fs.String("root", "", "repository root (default: locate it from the working directory)")
	dryRun := fs.Bool("dry-run", false, "print the selected areas and commands without running them")
	if err := fs.Parse(args); err != nil {
		return 2 // # pragma: no cover — flag.ContinueOnError already prints the usage error itself
	}
	if *fast && *full {
		fmt.Fprintln(errOut, "prepr: choose --fast or --full, not both")
		return 2
	}
	mode := prepr.ModeFast
	if *full {
		mode = prepr.ModeFull
	}
	if strings.HasPrefix(*base, "-") {
		fmt.Fprintln(errOut, "prepr: --base must be a git ref, not a flag")
		return 2
	}

	repoRoot := *root
	if repoRoot == "" {
		var err error
		repoRoot, err = findRepoRoot()
		if err != nil {
			fmt.Fprintln(errOut, "prepr:", err)
			return 2
		}
	}
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		fmt.Fprintln(errOut, "prepr:", err) // # pragma: no cover — Abs fails only on an unresolvable cwd
		return 2                            // # pragma: no cover
	}

	filtersFile, err := os.Open(filepath.Join(repoRoot, ".github", "filters.yaml")) // #nosec G304 -- fixed path under the resolved repo root
	if err != nil {
		fmt.Fprintln(errOut, "prepr:", err)
		return 2
	}
	defer filtersFile.Close() //nolint:errcheck // read-only
	filters, err := prepr.LoadFilters(filtersFile)
	if err != nil {
		fmt.Fprintln(errOut, "prepr:", err)
		return 2
	}

	changed, err := changedFiles(repoRoot, *base)
	if err != nil {
		fmt.Fprintln(errOut, "prepr:", err)
		return 2
	}

	plan := prepr.Build(filters, changed, mode, *base)
	fmt.Fprintf(out, "pre-pr gate (%s) against %s\n", mode, *base)
	if len(changed) == 0 {
		fmt.Fprintln(out, "no changed paths — running the unconditional docs/governance checks only")
	} else {
		fmt.Fprintf(out, "%d changed path(s); selected areas: %s\n", len(changed), strings.Join(plan.Selected, ", "))
	}

	if *dryRun {
		fmt.Fprintln(out, "\nplanned steps:")
		for i, step := range plan.Steps {
			if step.Command.Reminder != "" {
				fmt.Fprintf(out, "  %2d. [%s] reminder: %s\n", i+1, step.Area, step.Command.Label)
				continue
			}
			fmt.Fprintf(out, "  %2d. [%s] (cd %s && %s)\n", i+1, step.Area, dirForDisplay(step.Command.Dir), step.Display())
		}
		for _, skip := range plan.Skipped {
			fmt.Fprintf(out, "  -- skipped %s: %s\n", skip.Area, skip.Reason)
		}
		for _, note := range plan.Notes {
			fmt.Fprintf(out, "  -- note: %s\n", note)
		}
		if len(plan.Hints) > 0 {
			fmt.Fprintln(out, "\nbaseline update commands (printed on a real run):")
			for _, hint := range plan.Hints {
				fmt.Fprintf(out, "    %s\n", hint)
			}
		}
		return 0
	}

	return prepr.Execute(plan, out, runner(repoRoot))
}

// changedFiles selects the branch's committed and uncommitted changes relative
// to the merge base of base and HEAD. The gate runs before the commit, so it
// must see staged, unstaged, untracked, deleted, and renamed files too.
func changedFiles(root, base string) ([]string, error) {
	return gitchanges.ChangedFiles(root, base)
}

// runner executes a step in its area directory, streaming output to the
// terminal. A stress step repeats until the first failure.
func runner(root string) prepr.Runner {
	return func(step prepr.Step) error {
		dir := filepath.Join(root, filepath.FromSlash(step.Command.Dir))
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("working directory %s does not exist", step.Command.Dir)
		}

		attempts := step.Command.Repeat
		if attempts < 1 {
			attempts = 1
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			if attempts > 1 {
				fmt.Fprintf(os.Stdout, "    attempt %d/%d\n", attempt, attempts)
			}
			cmd := commandFor(step)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), step.Command.Env...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}
		return nil
	}
}

func commandFor(step prepr.Step) *exec.Cmd {
	if step.Command.Shell != "" {
		return exec.Command("bash", "-c", step.Command.Shell) // #nosec G204 -- fixed command table, not user input
	}
	argv := step.Command.Argv
	return exec.Command(argv[0], argv[1:]...) // #nosec G204 -- fixed command table plus validated changed-file paths
}

func dirForDisplay(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// findRepoRoot walks up from the working directory to backend/go.mod, the same
// sentinel cmd/docscheck and cmd/releasegatecheck use.
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
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod) from %s", dir)
		}
		dir = parent
	}
}
