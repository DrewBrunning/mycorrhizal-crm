package flakeledger

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIBase is the only host this tool talks to in CI (api.github.com,
// plus the blob-storage redirect GitHub issues for artifact downloads, which
// net/http follows without forwarding the Authorization header).
const DefaultAPIBase = "https://api.github.com"

const (
	maxZipEntries = 2000
	maxRunsPerWf  = 300
)

// maxArtifactBytes bounds one extracted artifact (the flake-* artifacts are
// KB-to-low-MB of XML/JSON; this is a zip-bomb ceiling, not a target). A var
// only so tests can lower it.
var maxArtifactBytes int64 = 256 << 20

// Client is a minimal GitHub REST client scoped to one repository.
type Client struct {
	Base  string // API base URL; DefaultAPIBase in production
	Repo  string // owner/name
	Token string
	HTTP  *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c *Client) req(ctx context.Context, method, path string, q url.Values, body any) (*http.Response, error) {
	u := c.Base + "/repos/" + c.Repo + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err // # pragma: no cover — only plain structs/maps are marshalled
		}
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		r.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return c.http().Do(r)
}

// StatusError carries a non-2xx status.
type StatusError struct {
	Method, Path string
	Code         int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("github %s %s: HTTP %d", e.Method, e.Path, e.Code)
}

func (c *Client) json(ctx context.Context, method, path string, q url.Values, body, out any) error {
	resp, err := c.req(ctx, method, path, q, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{method, path, resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ListRuns returns completed runs of one workflow file created on/after
// since, newest first, bounded at maxRunsPerWf.
func (c *Client) ListRuns(ctx context.Context, workflowFile string, since time.Time) ([]Run, error) {
	var out []Run
	for page := 1; len(out) < maxRunsPerWf; page++ {
		var resp struct {
			Runs []struct {
				ID         int64  `json:"id"`
				HTMLURL    string `json:"html_url"`
				Event      string `json:"event"`
				CreatedAt  string `json:"created_at"`
				Conclusion string `json:"conclusion"`
			} `json:"workflow_runs"`
		}
		q := url.Values{}
		q.Set("status", "completed")
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		q.Set("created", ">="+since.UTC().Format("2006-01-02"))
		if err := c.json(ctx, "GET", "/actions/workflows/"+workflowFile+"/runs", q, nil, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Runs {
			out = append(out, Run{Workflow: workflowFile, RunID: r.ID, URL: r.HTMLURL, Event: r.Event, CreatedAt: r.CreatedAt, Conclusion: r.Conclusion})
		}
		if len(resp.Runs) < 100 {
			break
		}
	}
	return out, nil
}

// Artifact is one downloadable run artifact.
type Artifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
}

// FlakeArtifacts lists a run's unexpired flake-* artifacts.
func (c *Client) FlakeArtifacts(ctx context.Context, runID int64) ([]Artifact, error) {
	var resp struct {
		Artifacts []Artifact `json:"artifacts"`
	}
	q := url.Values{"per_page": {"100"}}
	if err := c.json(ctx, "GET", "/actions/runs/"+strconv.FormatInt(runID, 10)+"/artifacts", q, nil, &resp); err != nil {
		return nil, err
	}
	var out []Artifact
	for _, a := range resp.Artifacts {
		if !a.Expired && strings.HasPrefix(a.Name, ArtifactPrefix) {
			out = append(out, a)
		}
	}
	return out, nil
}

// Download fetches artifact id and extracts it under dest.
func (c *Client) Download(ctx context.Context, id int64, dest string) error {
	resp, err := c.req(ctx, "GET", "/actions/artifacts/"+strconv.FormatInt(id, 10)+"/zip", nil, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{"GET", "/actions/artifacts/zip", resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxArtifactBytes+1))
	if err != nil {
		return err // # pragma: no cover — a mid-body transport reset; the status-and-size paths are tested
	}
	if int64(len(b)) > maxArtifactBytes {
		return errors.New("artifact larger than the extraction ceiling")
	}
	return Unzip(b, dest)
}

// Unzip extracts a zip held in memory under dest, refusing path escapes,
// oversized contents and entry floods.
func Unzip(b []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return err
	}
	if len(zr.File) > maxZipEntries {
		return errors.New("zip has too many entries")
	}
	var total int64
	for _, f := range zr.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("zip entry escapes destination: %q", f.Name)
		}
		target := filepath.Join(dest, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err // # pragma: no cover — a readable central directory with an unopenable entry is a corrupt-zip case the reader rejects earlier
		}
		out, err := os.Create(target) // #nosec G304 -- target is Clean'd and proven under dest above
		if err != nil {
			_ = rc.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(rc, maxArtifactBytes-total+1))
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return err // # pragma: no cover — a disk-write failure mid-copy
		}
		total += n
		if total > maxArtifactBytes {
			return errors.New("zip expands past the extraction ceiling")
		}
	}
	return nil
}

// CollectResult summarises a Collect call.
type CollectResult struct {
	Runs      int
	Artifacts int
	Warnings  []string
}

// Collect lists runs for each workflow file since the cutoff, downloads every
// flake-* artifact into <dir>/<run_id>/<artifact>/ and writes runs.json.
// Per-run failures are warnings: one missing artifact must not blind the
// ledger.
func Collect(ctx context.Context, c *Client, workflows []string, since time.Time, dir string) (CollectResult, error) {
	var res CollectResult
	var all []Run
	for _, wf := range workflows {
		runs, err := c.ListRuns(ctx, wf, since)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", wf, err))
			continue
		}
		for _, r := range runs {
			arts, err := c.FlakeArtifacts(ctx, r.RunID)
			if err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("run %d: %v", r.RunID, err))
				continue
			}
			if len(arts) == 0 {
				continue
			}
			ok := false
			for _, a := range arts {
				d := filepath.Join(dir, strconv.FormatInt(r.RunID, 10), a.Name)
				if err := c.Download(ctx, a.ID, d); err != nil {
					res.Warnings = append(res.Warnings, fmt.Sprintf("run %d artifact %s: %v", r.RunID, a.Name, err))
					continue
				}
				res.Artifacts++
				ok = true
			}
			if ok {
				all = append(all, r)
			}
		}
	}
	res.Runs = len(all)
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return res, err // # pragma: no cover — Run is a plain struct
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return res, err
	}
	return res, os.WriteFile(filepath.Join(dir, RunsFile), b, 0o600)
}

// ---- issues ----

// Label names the policy uses.
const (
	LabelFlaky    = "flaky-test"
	LabelPriority = "p1"
)

// Issue is the slice of an issue the policy needs.
type Issue struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	ClosedAt string `json:"closed_at"`
}

// FlakyIssues lists issues carrying the flaky-test label in the given state.
func (c *Client) FlakyIssues(ctx context.Context, state string) ([]Issue, error) {
	var out []Issue
	for page := 1; page <= 10; page++ {
		var batch []struct {
			Issue
			PullRequest *struct{} `json:"pull_request"`
		}
		q := url.Values{"labels": {LabelFlaky}, "state": {state}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		if err := c.json(ctx, "GET", "/issues", q, nil, &batch); err != nil {
			return nil, err
		}
		for _, i := range batch {
			if i.PullRequest == nil {
				out = append(out, i.Issue)
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}

// ClosedMap maps issue title -> latest close time.
func ClosedMap(closed []Issue) map[string]time.Time {
	m := map[string]time.Time{}
	for _, i := range closed {
		t, err := time.Parse(time.RFC3339, i.ClosedAt)
		if err != nil {
			continue
		}
		if cur, ok := m[i.Title]; !ok || t.After(cur) {
			m[i.Title] = t
		}
	}
	return m
}

// EnsureLabel creates the label if it does not exist.
func (c *Client) EnsureLabel(ctx context.Context, name, color, desc string) error {
	err := c.json(ctx, "GET", "/labels/"+url.PathEscape(name), nil, nil, nil)
	var se *StatusError
	if err == nil {
		return nil
	}
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		return err
	}
	return c.json(ctx, "POST", "/labels", nil, map[string]string{"name": name, "color": color, "description": desc}, nil)
}

// InFlightMilestone returns the open milestone with the earliest due date, or
// 0 when none has one.
func (c *Client) InFlightMilestone(ctx context.Context) (int, error) {
	var ms []struct {
		Number int    `json:"number"`
		DueOn  string `json:"due_on"`
	}
	if err := c.json(ctx, "GET", "/milestones", url.Values{"state": {"open"}, "per_page": {"100"}}, nil, &ms); err != nil {
		return 0, err
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].DueOn < ms[j].DueOn })
	for _, m := range ms {
		if m.DueOn != "" {
			return m.Number, nil
		}
	}
	return 0, nil
}

// IssueBody renders the auto-opened issue body.
func IssueBody(c Candidate, windowDays, threshold int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The flake ledger (issue #1488) recorded **%d** passed-on-retry event(s) for this test in the last %d days (%d run(s) observed, threshold %d).\n\n", c.PassedOnRetry, windowDays, c.Runs, threshold)
	fmt.Fprintf(&b, "- Suite: `%s`\n- Test: `%s`\n", c.Suite, c.Test)
	if c.Failed > 0 {
		fmt.Fprintf(&b, "- Also failed every attempt in %d run(s).\n", c.Failed)
	}
	b.WriteString("\nRuns:\n\n")
	for _, l := range c.RunLinks {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	b.WriteString("\nClosing this issue requires a **root-cause note** (what was racing or leaking, and the fix). Bumping a retry count or timeout without one is not a resolution. See `docs/development/testing.md` (Flake ledger).\n")
	return b.String()
}

// SyncIssues opens one issue per candidate that has no open issue, up to
// limit per call (a first run must not flood the tracker). Returns the titles
// created and the number deferred.
func SyncIssues(ctx context.Context, c *Client, cands []Candidate, windowDays, threshold, limit int) ([]string, int, error) {
	if len(cands) == 0 {
		return nil, 0, nil
	}
	open, err := c.FlakyIssues(ctx, "open")
	if err != nil {
		return nil, 0, err
	}
	have := map[string]bool{}
	for _, i := range open {
		have[i.Title] = true
	}
	if err := c.EnsureLabel(ctx, LabelFlaky, "d93f0b", "Test passed only on retry; see the flake ledger"); err != nil {
		return nil, 0, err
	}
	if err := c.EnsureLabel(ctx, LabelPriority, "e99695", "Priority 1"); err != nil {
		return nil, 0, err
	}
	ms, err := c.InFlightMilestone(ctx)
	if err != nil {
		return nil, 0, err
	}
	var created []string
	deferred := 0
	for _, cand := range cands {
		if have[cand.Title] {
			continue
		}
		if len(created) >= limit {
			deferred++
			continue
		}
		body := map[string]any{
			"title":  cand.Title,
			"body":   IssueBody(cand, windowDays, threshold),
			"labels": []string{LabelFlaky, LabelPriority},
		}
		if ms > 0 {
			body["milestone"] = ms
		}
		if err := c.json(ctx, "POST", "/issues", nil, body, nil); err != nil {
			return created, deferred, err
		}
		created = append(created, cand.Title)
	}
	return created, deferred, nil
}
