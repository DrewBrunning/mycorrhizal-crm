// Command bypassedcheck reports which of the main-protection ruleset's required
// status-check contexts were not green on a given commit (issue #1661).
//
// It is the engine behind .github/workflows/bypassed-check-alert.yml: after a
// PR merges, the workflow runs it against the PR's head SHA. The decision logic
// lives in internal/bypassedcheck; this command only gathers inputs (the
// required contexts from .github/rulesets/main-protection.json, the SHA's check
// runs and commit statuses) and prints the verdict as JSON.
//
// Usage:
//
//	bypassedcheck -ruleset .github/rulesets/main-protection.json -repo OWNER/REPO -sha SHA
//	bypassedcheck -ruleset ... -input fixture.json     # offline: {"check_runs":[...],"statuses":[...]}
//
// The token is read from GITHUB_TOKEN (needs checks:read and statuses:read);
// GITHUB_API_URL overrides the API base (tests, GHES).
//
// Checks still queued or running at merge time are re-evaluated every -poll
// until none are left or -wait-max is spent (default 0: evaluate once); a
// failed or missing context ends the wait at once so it alerts immediately.
//
// Output (stdout): {"sha":..., "required":N, "findings":[{context,category,detail}],
// "merged_early":[contexts incomplete at the first look], "waited_seconds":N}.
// Exit status 0 means the verdict was produced (findings may be non-empty --
// the caller decides what to do with them); 2 means it could not be produced.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mycorrhizal/internal/bypassedcheck"
	"mycorrhizal/internal/governance"
)

var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr, http.DefaultClient, os.Getenv))
}

// Fixture is the offline input shape (-input).
type Fixture struct {
	CheckRuns []bypassedcheck.CheckRun `json:"check_runs"`
	Statuses  []bypassedcheck.Status   `json:"statuses"`
}

// Verdict is the command's JSON output.
type Verdict struct {
	SHA      string                  `json:"sha"`
	Required int                     `json:"required"`
	Findings []bypassedcheck.Finding `json:"findings"`
	// MergedEarly: contexts not finished at the first look. One absent from
	// Findings later finished green.
	MergedEarly   []string `json:"merged_early"`
	WaitedSeconds int      `json:"waited_seconds"`
}

// sleep is swapped by tests.
var sleep = time.Sleep

func run(args []string, stdout, stderr io.Writer, client *http.Client, getenv func(string) string) int {
	fs := flag.NewFlagSet("bypassedcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rulesetPath := fs.String("ruleset", ".github/rulesets/main-protection.json", "ruleset JSON whose required contexts are checked")
	repo := fs.String("repo", "", "OWNER/REPO (live mode)")
	sha := fs.String("sha", "", "commit SHA to evaluate (live mode; echoed in the output)")
	input := fs.String("input", "", "offline fixture JSON instead of calling the API")
	poll := fs.Duration("poll", 2*time.Minute, "re-evaluation interval while required checks are unfinished")
	waitMax := fs.Duration("wait-max", 0, "longest to wait for unfinished required checks (0: evaluate once)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fail := func(err error) int {
		fmt.Fprintln(stderr, "bypassedcheck:", err)
		return 2
	}

	data, err := os.ReadFile(*rulesetPath)
	if err != nil {
		return fail(err)
	}
	rs, problems := governance.ParseRuleset(*rulesetPath, data)
	if len(problems) > 0 {
		return fail(errors.New(problems[0]))
	}
	required := governance.RequiredContexts(rs)
	if len(required) == 0 {
		return fail(errors.New("ruleset has no required status checks"))
	}

	var fetch bypassedcheck.Fetch
	if *input != "" {
		raw, err := os.ReadFile(*input)
		if err != nil {
			return fail(err)
		}
		var fx Fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			return fail(fmt.Errorf("parse %s: %w", *input, err))
		}
		fetch = func() ([]bypassedcheck.CheckRun, []bypassedcheck.Status, error) {
			return fx.CheckRuns, fx.Statuses, nil
		}
	} else {
		if *repo == "" || *sha == "" {
			return fail(errors.New("-repo and -sha are required (or -input)"))
		}
		token := getenv("GITHUB_TOKEN")
		if token == "" {
			return fail(errors.New("GITHUB_TOKEN is required"))
		}
		base := getenv("GITHUB_API_URL")
		if base == "" {
			base = "https://api.github.com"
		}
		g := &ghClient{client: client, base: base, token: token}
		fetch = func() ([]bypassedcheck.CheckRun, []bypassedcheck.Status, error) {
			runs, err := g.checkRuns(*repo, *sha)
			if err != nil {
				return nil, nil, err
			}
			sts, err := g.statuses(*repo, *sha)
			return runs, sts, err
		}
	}

	set, err := bypassedcheck.Settle(required, fetch, *poll, *waitMax, sleep)
	if err != nil {
		return fail(err)
	}
	findings := set.Findings
	if findings == nil {
		findings = []bypassedcheck.Finding{}
	}
	early := set.MergedEarly
	if early == nil {
		early = []string{}
	}
	out, err := json.MarshalIndent(Verdict{SHA: *sha, Required: len(required), Findings: findings, MergedEarly: early, WaitedSeconds: int(set.Waited / time.Second)}, "", "  ")
	if err != nil {
		return fail(err) // # pragma: no cover — plain structs always marshal.
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

type ghClient struct {
	client *http.Client
	base   string
	token  string
}

const perPage = 100

// pages walks a page-numbered list endpoint, handing each page's raw body to
// decode, which returns how many items it held (a short page ends the walk).
func (g *ghClient) pages(path string, decode func([]byte) (int, error)) error {
	for page := 1; ; page++ {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s%sper_page=%d&page=%d", g.base, path, sep, perPage, page), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+g.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := g.client.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body)))
		}
		n, err := decode(body)
		if err != nil {
			return fmt.Errorf("GET %s: %w", path, err)
		}
		if n < perPage {
			return nil
		}
	}
}

func truncate(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func (g *ghClient) checkRuns(repo, sha string) ([]bypassedcheck.CheckRun, error) {
	var out []bypassedcheck.CheckRun
	// filter=all so superseded re-runs are returned too; Evaluate picks the latest.
	err := g.pages("/repos/"+repo+"/commits/"+sha+"/check-runs?filter=all", func(b []byte) (int, error) {
		var p struct {
			CheckRuns []struct {
				Name        string     `json:"name"`
				Status      string     `json:"status"`
				Conclusion  *string    `json:"conclusion"`
				StartedAt   *time.Time `json:"started_at"`
				CompletedAt *time.Time `json:"completed_at"`
			} `json:"check_runs"`
		}
		if err := json.Unmarshal(b, &p); err != nil {
			return 0, err
		}
		for _, r := range p.CheckRuns {
			cr := bypassedcheck.CheckRun{Name: r.Name, Status: r.Status}
			if r.Conclusion != nil {
				cr.Conclusion = *r.Conclusion
			}
			switch {
			case r.CompletedAt != nil:
				cr.At = *r.CompletedAt
			case r.StartedAt != nil:
				cr.At = *r.StartedAt
			}
			out = append(out, cr)
		}
		return len(p.CheckRuns), nil
	})
	return out, err
}

func (g *ghClient) statuses(repo, sha string) ([]bypassedcheck.Status, error) {
	var out []bypassedcheck.Status
	err := g.pages("/repos/"+repo+"/commits/"+sha+"/statuses", func(b []byte) (int, error) {
		var p []bypassedcheck.Status
		var raw []struct {
			Context   string    `json:"context"`
			State     string    `json:"state"`
			CreatedAt time.Time `json:"created_at"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return 0, err
		}
		for _, s := range raw {
			p = append(p, bypassedcheck.Status{Context: s.Context, State: s.State, At: s.CreatedAt})
		}
		out = append(out, p...)
		return len(raw), nil
	})
	return out, err
}
