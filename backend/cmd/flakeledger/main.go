// Command flakeledger is the longitudinal flake record (issue #1488).
//
//	flakeledger collect -data DIR -workflows a.yml,b.yml [-window 14]
//	    list recent runs, download their flake-* artifacts into DIR, record
//	    recently closed flaky-test issues in DIR/closed.json
//	flakeledger report -data DIR -md FILE -candidates FILE [-window 14] [-threshold 3]
//	    build the per-test ledger from DIR; write markdown + candidate JSON
//	flakeledger issues -candidates FILE [-window 14] [-threshold 3] [-limit 5]
//	    open a flaky-test issue for each candidate with no open issue
//
// Advisory by design: report and issues exit 0 on any data problem short of
// being unable to read/write their own files. Exit 2 = could not run.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mycorrhizal/internal/flakeledger"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, time.Now)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(ctx context.Context, args []string, getenv func(string) string, w io.Writer, now func() time.Time) int {
	if len(args) == 0 {
		fmt.Fprintln(w, "usage: flakeledger collect|report|issues [flags]")
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(w)
	data := fs.String("data", "", "ledger data directory")
	workflows := fs.String("workflows", "", "comma-separated workflow files (collect)")
	window := fs.Int("window", flakeledger.DefaultWindowDays, "window in days")
	threshold := fs.Int("threshold", flakeledger.DefaultThreshold, "passed-on-retry events that open an issue")
	md := fs.String("md", "", "markdown output (report)")
	cands := fs.String("candidates", "", "candidate JSON (report output / issues input)")
	limit := fs.Int("limit", 5, "max issues opened per run (issues)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	switch args[0] {
	case "collect":
		return collect(ctx, w, getenv, now(), *data, *workflows, *window)
	case "report":
		return report(w, now(), *data, *md, *cands, *window, *threshold)
	case "issues":
		return issues(ctx, w, getenv, *cands, *window, *threshold, *limit)
	}
	fmt.Fprintf(w, "unknown subcommand %q\n", args[0])
	return 2
}

func client(getenv func(string) string) (*flakeledger.Client, error) {
	repo := getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return nil, fmt.Errorf("GITHUB_REPOSITORY is not set")
	}
	base := getenv("FLAKELEDGER_API_BASE")
	if base == "" {
		base = flakeledger.DefaultAPIBase
	}
	return &flakeledger.Client{Base: base, Repo: repo, Token: getenv("GITHUB_TOKEN")}, nil
}

func collect(ctx context.Context, w io.Writer, getenv func(string) string, now time.Time, data, workflows string, window int) int {
	if data == "" || workflows == "" {
		fmt.Fprintln(w, "collect needs -data and -workflows")
		return 2
	}
	c, err := client(getenv)
	if err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	var wfs []string
	for _, f := range strings.Split(workflows, ",") {
		if f = strings.TrimSpace(f); f != "" {
			wfs = append(wfs, f)
		}
	}
	res, err := flakeledger.Collect(ctx, c, wfs, now.AddDate(0, 0, -window), data)
	if err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	for _, x := range res.Warnings {
		fmt.Fprintln(w, "warning:", x)
	}
	fmt.Fprintf(w, "collected %d run(s), %d artifact(s)\n", res.Runs, res.Artifacts)

	// Closed issues gate re-opening (see flakeledger.Candidates). A failure
	// here degrades to "no closed history" rather than blinding the ledger.
	closed := map[string]string{}
	if issues, err := c.FlakyIssues(ctx, "closed"); err != nil {
		fmt.Fprintln(w, "warning: closed issues:", err)
	} else {
		for t, at := range flakeledger.ClosedMap(issues) {
			closed[t] = at.Format(time.RFC3339)
		}
	}
	b, _ := json.Marshal(closed)                                                       // map[string]string cannot fail to marshal
	if err := os.WriteFile(filepath.Join(data, "closed.json"), b, 0o600); err != nil { // #nosec G703 -- operator-supplied CI data dir
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	return 0
}

func report(w io.Writer, now time.Time, data, md, cands string, window, threshold int) int {
	if data == "" || md == "" || cands == "" {
		fmt.Fprintln(w, "report needs -data, -md and -candidates")
		return 2
	}
	runs, warns, err := flakeledger.LoadRuns(data)
	if err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	for _, x := range warns {
		fmt.Fprintln(w, "warning:", x)
	}
	closed := map[string]time.Time{}
	if b, err := os.ReadFile(filepath.Join(data, "closed.json")); err == nil { // #nosec G304 G703 -- operator-supplied CI data dir
		raw := map[string]string{}
		if json.Unmarshal(b, &raw) == nil {
			for t, s := range raw {
				if ts, err := time.Parse(time.RFC3339, s); err == nil {
					closed[t] = ts
				}
			}
		}
	}
	l := flakeledger.Build(runs, now, window)
	cs := flakeledger.Candidates(l, threshold, closed)
	if cs == nil {
		cs = []flakeledger.Candidate{}
	}
	if err := os.WriteFile(md, []byte(flakeledger.Render(l, threshold)), 0o600); err != nil { // #nosec G703 -- operator-supplied output path
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	b, _ := json.MarshalIndent(cs, "", "  ")              // plain structs cannot fail to marshal
	if err := os.WriteFile(cands, b, 0o600); err != nil { // #nosec G703 -- operator-supplied output path
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	fmt.Fprintf(w, "ledger: %d run(s), %d test(s), %d flaky row(s), %d issue candidate(s)\n", l.Runs, l.Tests, len(l.Rows), len(cs))
	return 0
}

func issues(ctx context.Context, w io.Writer, getenv func(string) string, cands string, window, threshold, limit int) int {
	if cands == "" {
		fmt.Fprintln(w, "issues needs -candidates")
		return 2
	}
	b, err := os.ReadFile(cands) // #nosec G304 G703 -- operator-supplied path
	if err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	var cs []flakeledger.Candidate
	if err := json.Unmarshal(b, &cs); err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	c, err := client(getenv)
	if err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	created, deferred, err := flakeledger.SyncIssues(ctx, c, cs, window, threshold, limit)
	for _, t := range created {
		fmt.Fprintln(w, "opened:", t)
	}
	if deferred > 0 {
		fmt.Fprintf(w, "%d candidate(s) deferred to the next run (limit %d)\n", deferred, limit)
	}
	if err != nil {
		fmt.Fprintln(w, "flakeledger:", err)
		return 2
	}
	fmt.Fprintf(w, "%d candidate(s), %d issue(s) opened\n", len(cs), len(created))
	return 0
}
