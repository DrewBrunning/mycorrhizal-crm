// Command releaseplan is the workflow-facing front of internal/releaseplan
// (issue #1487): the release battery's gate ledger, the `rerun_gates` plan, and
// the tag-time reuse decision. Workflows call it through `go run`, so the
// logic is unit-tested Go rather than inline shell.
//
//	releaseplan ledger -composer F -results F -sha S -tag T -run-id N [-skip a,b] [-carried JSON] [-out F]
//	    Write the per-gate ledger release-validate.yml's `results` job uploads.
//	releaseplan rerun  -composer F -rerun a,b|failed -ledger F -sha S -tag T
//	    Plan a `rerun_gates` dispatch; prints key=value lines for $GITHUB_OUTPUT.
//	releaseplan reuse  -composer F -ledger F -tag T -tag-sha S -changed-files F
//	    Decide tag-time reuse of the pre-tag battery; prints key=value lines.
//
// Exit 0 on success; `ledger` and `rerun` exit 1 on an input they cannot act
// on (a bad gate name, an empty plan). `reuse` never fails: any doubt, including
// an unreadable ledger, is `reuse=false` so the caller runs the full battery.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"mycorrhizal/internal/releaseplan"
)

// osExit is os.Exit through a seam so tests can drive main() itself.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr, os.ReadFile, os.WriteFile))
}

type (
	readFileFn  func(string) ([]byte, error)
	writeFileFn func(string, []byte, os.FileMode) error
)

func run(args []string, stdout, stderr io.Writer, read readFileFn, write writeFileFn) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: releaseplan <ledger|rerun|reuse> [flags]")
		return 2
	}
	switch args[0] {
	case "ledger":
		return cmdLedger(args[1:], stdout, stderr, read, write)
	case "rerun":
		return cmdRerun(args[1:], stdout, stderr, read)
	case "reuse":
		return cmdReuse(args[1:], stdout, stderr, read)
	default:
		fmt.Fprintf(stderr, "releaseplan: unknown subcommand %q\n", args[0])
		return 2
	}
}

func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func composedGates(read readFileFn, composer string) ([]string, error) {
	b, err := read(composer)
	if err != nil {
		return nil, err
	}
	return releaseplan.ComposedGates(string(b))
}

func compact(v any) string {
	b, _ := json.Marshal(v) // only plain string structs are marshalled; cannot fail
	return string(b)
}

func cmdLedger(args []string, stdout, stderr io.Writer, read readFileFn, write writeFileFn) int {
	fs := newFlags("ledger", stderr)
	composer := fs.String("composer", ".github/workflows/release-validate.yml", "composer workflow")
	results := fs.String("results", "", "release-gate-results.json (needs.<job>.result by job id)")
	sha := fs.String("sha", "", "commit the battery ran against")
	tag := fs.String("tag", "", "release tag")
	runID := fs.String("run-id", "", "current workflow run id")
	skip := fs.String("skip", "", "comma list of gates the composer did not run (skip_gates)")
	carried := fs.String("carried", "", "JSON ledger holding the carried successes for -skip")
	out := fs.String("out", "", "output file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	gates, err := composedGates(read, *composer)
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	raw, err := read(*results)
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	var res map[string]string
	if err := json.Unmarshal(raw, &res); err != nil {
		fmt.Fprintln(stderr, "releaseplan: results do not parse:", err)
		return 1
	}
	prior, err := releaseplan.ParseLedger([]byte(*carried))
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	l, err := releaseplan.BuildLedger(releaseplan.BuildInput{
		Gates: gates, Results: res, SHA: *sha, Tag: *tag, RunID: *runID,
		Skip: releaseplan.SplitList(*skip), Carried: prior,
	})
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	body, _ := json.MarshalIndent(l, "", "  ")
	body = append(body, '\n')
	if *out == "" {
		_, _ = stdout.Write(body)
		return 0
	}
	if err := write(*out, body, 0o644); err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	return 0
}

func cmdRerun(args []string, stdout, stderr io.Writer, read readFileFn) int {
	fs := newFlags("rerun", stderr)
	composer := fs.String("composer", ".github/workflows/release-validate.yml", "composer workflow")
	rerun := fs.String("rerun", "", "gates to re-run, comma separated, or 'failed'")
	ledger := fs.String("ledger", "", "prior release-gate-ledger.json")
	sha := fs.String("sha", "", "release commit")
	tag := fs.String("tag", "", "release tag")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	gates, err := composedGates(read, *composer)
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	raw, err := read(*ledger)
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan: prior ledger:", err)
		return 1
	}
	prior, err := releaseplan.ParseLedger(raw)
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	plan, err := releaseplan.PlanRerun(releaseplan.RerunInput{
		Gates: gates, Requested: releaseplan.SplitList(*rerun), Prior: prior, SHA: *sha, Tag: *tag,
	})
	if err != nil {
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	if len(plan.Added) > 0 {
		fmt.Fprintf(stderr, "releaseplan: also re-running %s: no recorded success for %s at %s\n", strings.Join(plan.Added, ", "), *tag, *sha)
	}
	fmt.Fprintf(stderr, "releaseplan: re-running %s; carrying forward %d gate(s)\n", strings.Join(plan.Rerun, ", "), len(plan.Skip))
	fmt.Fprintf(stdout, "rerun_gates=%s\n", strings.Join(plan.Rerun, ","))
	fmt.Fprintf(stdout, "skip_gates=%s\n", strings.Join(plan.Skip, ","))
	fmt.Fprintf(stdout, "carried=%s\n", compact(plan.Carried))
	return 0
}

func cmdReuse(args []string, stdout, stderr io.Writer, read readFileFn) int {
	fs := newFlags("reuse", stderr)
	composer := fs.String("composer", ".github/workflows/release-validate.yml", "composer workflow")
	ledger := fs.String("ledger", "", "pre-tag release-gate-ledger.json (may be missing)")
	tag := fs.String("tag", "", "release tag")
	tagSHA := fs.String("tag-sha", "", "commit the tag points at")
	changed := fs.String("changed-files", "", "file listing `git diff --name-only <validated>..<tag>` (may be missing)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	gates, err := composedGates(read, *composer)
	if err != nil {
		// Not "reuse=false": a composer that cannot be read is a broken
		// checkout, which the caller should see rather than paper over.
		fmt.Fprintln(stderr, "releaseplan:", err)
		return 1
	}
	var prior releaseplan.Ledger
	var note string
	if raw, err := read(*ledger); err != nil {
		note = "the pre-tag ledger could not be read (" + err.Error() + ")"
	} else if l, err := releaseplan.ParseLedger(raw); err != nil {
		note = err.Error()
	} else {
		prior = l
	}
	var files []string
	if raw, err := read(*changed); err == nil {
		for _, f := range strings.Split(string(raw), "\n") {
			if f = strings.TrimSpace(f); f != "" {
				files = append(files, f)
			}
		}
	} else if len(prior) > 0 {
		note = "the changed-file list could not be read (" + err.Error() + ")"
		prior = nil
	}
	d := releaseplan.DecideReuse(releaseplan.ReuseInput{Gates: gates, Prior: prior, Tag: *tag, TagSHA: *tagSHA, ChangedFiles: files})
	if note != "" {
		d.Reuse = false
		d.Reason = note + ": running the full battery"
		d.Skip, d.Retest, d.Carried = []string{}, []string{}, releaseplan.Ledger{}
	}
	fmt.Fprintf(stderr, "releaseplan: reuse=%t: %s\n", d.Reuse, d.Reason)
	fmt.Fprintf(stdout, "reuse=%t\n", d.Reuse)
	fmt.Fprintf(stdout, "reason=%s\n", strings.ReplaceAll(d.Reason, "\n", " "))
	fmt.Fprintf(stdout, "skip_gates=%s\n", strings.Join(d.Skip, ","))
	fmt.Fprintf(stdout, "carried=%s\n", compact(d.Carried))
	fmt.Fprintf(stdout, "decision=%s\n", compact(d))
	return 0
}
