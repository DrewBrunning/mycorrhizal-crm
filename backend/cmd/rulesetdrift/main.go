// Command rulesetdrift is governance-drift.yml's ruleset-drift decision step
// (issue #916, #502 finding F4).
//
// The workflow's own `gh api` + jq steps compute, for every committed
// .github/rulesets/*.json, whether it has a live counterpart and whether
// that counterpart still matches after field-normalization, writing the
// results as a JSON array (see -results). This command turns those results,
// plus docs/security/governance-drift.ignore, into the job's actual
// pass/fail.
//
// Before #916, this step only ever printed a `::warning::` and a job
// summary -- a drifted or never-applied ruleset produced no finding and the
// job always exited 0, so governance-drift.yml's "standing alarm" for
// repository-governance drift could never actually alarm.
//
// Exit 0: every ruleset matches its committed JSON, or its gap is accepted
// in governance-drift.ignore, and the ignore file has no stale or unknown
// entries (including zero rulesets -- nothing to report is not a failure).
// Exit 1: at least one unaccepted drifted/not-applied ruleset, a stale
// ignore entry, or a malformed/unknown ignore entry. Exit 2: could not run
// (missing/unparseable -results).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/internal/governance"
)

func main() {
	os.Exit(run(os.Stdout, os.Args[1:])) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

// run is the testable core: it parses flags, reads -results and -ignore, and
// returns the process exit code.
func run(w io.Writer, args []string) int {
	fs := flag.NewFlagSet("rulesetdrift", flag.ContinueOnError)
	fs.SetOutput(w)
	resultsPath := fs.String("results", "", "path to a JSON array of {\"name\",\"applied\",\"drifted\"} ruleset-drift results (required)")
	ignorePath := fs.String("ignore", "docs/security/governance-drift.ignore", "path to the governance-drift ignore ledger")
	if err := fs.Parse(args); err != nil {
		return 2 // # pragma: no cover — flag.ContinueOnError already printed the usage error to w
	}
	if *resultsPath == "" {
		fmt.Fprintln(w, "rulesetdrift: -results is required")
		return 2
	}

	// #nosec G304 -- resultsPath and ignorePath are operator-supplied CLI
	// flags in a CI step this repo's own workflow controls, not external input.
	resultsBody, err := os.ReadFile(*resultsPath)
	if err != nil {
		fmt.Fprintln(w, "rulesetdrift:", err)
		return 2
	}
	var statuses []governance.RulesetStatus
	if err := json.Unmarshal(resultsBody, &statuses); err != nil {
		fmt.Fprintln(w, "rulesetdrift: -results does not parse as JSON:", err)
		return 2
	}

	// #nosec G304 -- see above
	ignoreBody, err := os.ReadFile(*ignorePath)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(w, "rulesetdrift:", err)
		return 2
	}
	ignore, parseFindings := governance.ParseGovernanceDriftIgnore(string(ignoreBody))

	evalFindings, accepted := governance.EvaluateGovernanceDrift(statuses, ignore)
	findings := append(parseFindings, evalFindings...)

	for _, a := range accepted {
		fmt.Fprintln(w, "accepted:", a)
	}
	if len(findings) == 0 {
		fmt.Fprintf(w, "governance drift: all %d ruleset(s) match .github/rulesets/ (or are accepted in %s).\n", len(statuses), *ignorePath)
		return 0
	}
	fmt.Fprintf(w, "\n%d governance-drift finding(s):\n", len(findings))
	for _, f := range findings {
		fmt.Fprintln(w, " -", f)
	}
	return 1
}
