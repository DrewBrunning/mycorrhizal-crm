package prepr

import (
	"fmt"
	"io"
	"strings"
)

// Runner executes one planned command. Command.Repeat, if > 1, is the
// runner's responsibility (it is a stress loop, not a shell detail).
type Runner func(Step) error

// Execute runs every step in order, reporting pass/fail without stopping at
// the first failure (so one run surfaces everything wrong, like CI's job
// list). It prints the plan's skips, notes, and baseline hints, then returns
// 0 when everything passed and 1 when any executed step failed.
func Execute(plan *Plan, out io.Writer, run Runner) int {
	failures := make([]Step, 0)
	warnings := make([]Step, 0)
	executed := 0

	for i, step := range plan.Steps {
		if step.Command.Reminder != "" {
			continue
		}
		executed++
		writef(out, "\n==> [%d/%d] %s\n", i+1, len(plan.Steps), step.Command.Label)
		writef(out, "    %s\n", step.Display())
		if err := run(step); err != nil {
			if step.Command.Warn {
				warnings = append(warnings, step)
				writef(out, "    WARN (does not fail the gate): %s\n", step.Command.Label)
				continue
			}
			failures = append(failures, step)
			writef(out, "    FAILED: %s\n", step.Command.Label)
			continue
		}
		writeln(out, "    ok")
	}

	for _, step := range plan.Steps {
		if step.Command.Reminder != "" {
			writef(out, "\n!! %s\n%s\n", step.Command.Label, step.Command.Reminder)
		}
	}

	for _, skip := range plan.Skipped {
		writef(out, "\n-- skipped %s: %s\n", skip.Area, skip.Reason)
	}
	for _, note := range plan.Notes {
		writef(out, "-- note: %s\n", note)
	}
	if len(plan.Unmatched) > 0 {
		writef(out, "-- no changed paths: %s\n", strings.Join(plan.Unmatched, ", "))
	}
	if len(plan.Hints) > 0 {
		writeln(out, "\nCoverage baseline update (review and commit the diff):")
		for _, hint := range plan.Hints {
			writef(out, "    %s\n", hint)
		}
	}

	writef(out, "\npre-pr (%s): %d command(s) run, %d area(s) selected, %d skipped\n",
		plan.Mode, executed, len(plan.Selected), len(plan.Skipped))
	for _, step := range warnings {
		writef(out, "warning (non-blocking): %s\n", step.Command.Label)
	}
	if len(failures) > 0 {
		writef(out, "\nFAILED — reproduce with:\n")
		for _, step := range failures {
			writef(out, "    (cd %s && %s)\n", dirForDisplay(step.Command.Dir), step.Display())
		}
		return 1
	}
	writeln(out, "all pre-PR checks passed")
	return 0
}

func writef(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format, args...)
}

func writeln(out io.Writer, args ...any) {
	_, _ = fmt.Fprintln(out, args...)
}

func dirForDisplay(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}
