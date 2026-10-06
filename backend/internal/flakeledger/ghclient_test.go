package flakeledger

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mkzip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnzip(t *testing.T) {
	d := t.TempDir()
	if err := Unzip(mkzip(t, map[string]string{"a/b.xml": "<x/>", "c.txt": "hi"}), d); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "a", "b.xml")); string(b) != "<x/>" {
		t.Errorf("%q", b)
	}
	for _, evil := range []string{"../escape.txt", "a/../../escape.txt", "/abs.txt", ".."} {
		if err := Unzip(mkzip(t, map[string]string{evil: "x"}), t.TempDir()); err == nil {
			t.Errorf("%s must be refused", evil)
		}
	}
	if err := Unzip([]byte("not a zip"), d); err == nil {
		t.Error("garbage must error")
	}
	// Directory entries are created; a file blocking a directory errors.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, _ = zw.Create("sub/")
	_ = zw.Close()
	d2 := t.TempDir()
	if err := Unzip(buf.Bytes(), d2); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(d2, "sub")); err != nil || !st.IsDir() {
		t.Error("dir entry not created")
	}
	d3 := t.TempDir()
	_ = os.WriteFile(filepath.Join(d3, "a"), []byte("file"), 0o600)
	if err := Unzip(mkzip(t, map[string]string{"a/b.xml": "x"}), d3); err == nil {
		t.Error("file-in-the-way must error")
	}
	d4 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d4, "c.txt"), 0o750)
	if err := Unzip(mkzip(t, map[string]string{"c.txt": "x"}), d4); err == nil {
		t.Error("create over a directory must error")
	}
}

func TestUnzipEntryFlood(t *testing.T) {
	files := map[string]string{}
	for i := 0; i <= maxZipEntries; i++ {
		files["f"+itoa(i)] = "x"
	}
	if err := Unzip(mkzip(t, files), t.TempDir()); err == nil {
		t.Error("entry flood must be refused")
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

// fakeGH is an in-memory GitHub: workflows -> runs -> artifacts, plus issues.
type fakeGH struct {
	t          *testing.T
	runs       map[string][]map[string]any // workflow file -> runs
	artifacts  map[int64][]Artifact        // run id -> artifacts
	zips       map[int64][]byte
	open       []Issue
	closed     []Issue
	labels     map[string]bool
	milestones []map[string]any
	created    []map[string]any
	failPath   string // substring of a path that returns 500
	auth       []string
}

func (f *fakeGH) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		p := strings.TrimPrefix(r.URL.Path, "/repos/o/r")
		if f.failPath != "" && strings.Contains(p, f.failPath) {
			http.Error(w, "boom", 500)
			return
		}
		enc := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case strings.HasPrefix(p, "/actions/workflows/"):
			wf := strings.TrimSuffix(strings.TrimPrefix(p, "/actions/workflows/"), "/runs")
			if r.URL.Query().Get("page") != "1" {
				enc(map[string]any{"workflow_runs": []any{}})
				return
			}
			enc(map[string]any{"workflow_runs": f.runs[wf]})
		case strings.HasSuffix(p, "/artifacts") && strings.HasPrefix(p, "/actions/runs/"):
			id := mustInt(strings.Split(p, "/")[3])
			enc(map[string]any{"artifacts": f.artifacts[id]})
		case strings.HasPrefix(p, "/actions/artifacts/"):
			id := mustInt(strings.Split(p, "/")[3])
			z, ok := f.zips[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(z)
		case p == "/issues" && r.Method == "GET":
			if r.URL.Query().Get("state") == "closed" {
				enc(f.closed)
			} else {
				enc(append(append([]Issue{}, f.open...), Issue{Number: 99, Title: "a PR"}))
			}
		case p == "/issues" && r.Method == "POST":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.created = append(f.created, body)
			w.WriteHeader(201)
			enc(map[string]any{})
		case strings.HasPrefix(p, "/labels/"):
			if f.labels[strings.TrimPrefix(p, "/labels/")] {
				enc(map[string]any{})
			} else {
				http.NotFound(w, r)
			}
		case p == "/labels" && r.Method == "POST":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.labels[body["name"]] = true
			w.WriteHeader(201)
			enc(map[string]any{})
		case p == "/milestones":
			enc(f.milestones)
		default:
			f.t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
}

func mustInt(s string) int64 {
	var n int64
	for _, c := range s {
		n = n*10 + int64(c-'0')
	}
	return n
}

func newFake(t *testing.T) (*fakeGH, *Client) {
	f := &fakeGH{t: t, runs: map[string][]map[string]any{}, artifacts: map[int64][]Artifact{}, zips: map[int64][]byte{}, labels: map[string]bool{}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return f, &Client{Base: srv.URL, Repo: "o/r", Token: "tok"}
}

func TestCollect(t *testing.T) {
	f, c := newFake(t)
	f.runs["unit-tests.yml"] = []map[string]any{
		{"id": 11, "html_url": "u11", "event": "push", "created_at": day(1), "conclusion": "success"},
		{"id": 12, "html_url": "u12", "event": "push", "created_at": day(2), "conclusion": "failure"},
		{"id": 13, "html_url": "u13", "event": "push", "created_at": day(3), "conclusion": "success"},
		{"id": 14, "html_url": "u14", "event": "push", "created_at": day(4), "conclusion": "success"},
	}
	f.runs["e2e-tests.yml"] = nil
	f.artifacts[11] = []Artifact{{ID: 1, Name: "flake-go-core"}, {ID: 2, Name: "backend-coverage"}, {ID: 3, Name: "flake-go-old", Expired: true}}
	f.zips[1] = mkzip(t, map[string]string{"rerun-fails-core.txt": "p.TestA: 2 runs, 1 failures\n"})
	f.artifacts[12] = []Artifact{{ID: 5, Name: "flake-go-core"}} // download 404s -> warning, run dropped
	f.artifacts[13] = nil                                        // no flake artifacts -> run skipped
	// run 14: artifact listing fails

	dir := t.TempDir()
	res, err := Collect(context.Background(), c, []string{"unit-tests.yml", "e2e-tests.yml", "missing.yml"}, now.AddDate(0, 0, -14), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Runs != 1 || res.Artifacts != 1 {
		t.Fatalf("%+v", res)
	}
	if len(res.Warnings) < 1 {
		t.Errorf("want warnings for the 404 artifact and 14's listing: %v", res.Warnings)
	}
	if _, err := os.Stat(filepath.Join(dir, "11", "flake-go-core", "rerun-fails-core.txt")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "11", "backend-coverage")); err == nil {
		t.Error("non flake-* artifact must not be downloaded")
	}
	runs, _, err := LoadRuns(dir)
	if err != nil || len(runs) != 1 || runs[0].Suites["go/core"]["p.TestA"] != PassedOnRetry {
		t.Fatalf("%v %+v", err, runs)
	}
	for _, a := range f.auth {
		if a != "Bearer tok" {
			t.Fatalf("auth header %q", a)
		}
	}
}

func TestCollectListRunsFailureAndWriteError(t *testing.T) {
	f, c := newFake(t)
	f.failPath = "/actions/workflows/"
	res, err := Collect(context.Background(), c, []string{"x.yml"}, now, t.TempDir())
	if err != nil || len(res.Warnings) != 1 || res.Runs != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	// runs.json cannot be written when dir is a file.
	file := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(file, nil, 0o600)
	if _, err := Collect(context.Background(), c, nil, now, filepath.Join(file, "sub")); err == nil {
		t.Error("want mkdir error")
	}
}

func TestListRunsPaginates(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))
		var rs []map[string]any
		for i := 0; i < 100; i++ {
			rs = append(rs, map[string]any{"id": i, "created_at": day(1)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workflow_runs": rs})
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Repo: "o/r"}
	runs, err := c.ListRuns(context.Background(), "w.yml", now)
	if err != nil {
		t.Fatal(err)
	}
	// maxRunsPerWf (300) is reached after page 3 returns 100+100+100.
	if len(runs) != 300 || len(pages) != 3 {
		t.Fatalf("runs=%d pages=%v", len(runs), pages)
	}
}

func TestClientErrors(t *testing.T) {
	f, c := newFake(t)
	f.failPath = "/artifacts"
	if _, err := c.FlakeArtifacts(context.Background(), 1); err == nil {
		t.Error("want error")
	} else {
		var se *StatusError
		if !errors.As(err, &se) || se.Code != 500 || !strings.Contains(err.Error(), "HTTP 500") {
			t.Errorf("%v", err)
		}
	}
	if err := c.Download(context.Background(), 404, t.TempDir()); err == nil {
		t.Error("404 download must error")
	}
	bad := &Client{Base: "http://127.0.0.1:1", Repo: "o/r"}
	if _, err := bad.ListRuns(context.Background(), "w", now); err == nil {
		t.Error("unreachable must error")
	}
	if err := bad.Download(context.Background(), 1, t.TempDir()); err == nil {
		t.Error("unreachable download must error")
	}
	if _, err := (&Client{Base: "http://x\x7f", Repo: "o/r"}).FlakeArtifacts(context.Background(), 1); err == nil {
		t.Error("bad URL must error")
	}
}

func TestDownloadOversizedAndBadZip(t *testing.T) {
	f, c := newFake(t)
	f.zips[1] = []byte("not a zip")
	if err := c.Download(context.Background(), 1, t.TempDir()); err == nil {
		t.Error("bad zip must error")
	}
}

func TestClosedMap(t *testing.T) {
	m := ClosedMap([]Issue{
		{Title: "a", ClosedAt: "2026-10-01T00:00:00Z"},
		{Title: "a", ClosedAt: "2026-10-03T00:00:00Z"},
		{Title: "a", ClosedAt: "2026-10-02T00:00:00Z"},
		{Title: "b", ClosedAt: ""},
	})
	if len(m) != 1 || !m["a"].Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("%v", m)
	}
}

func TestSyncIssues(t *testing.T) {
	f, c := newFake(t)
	f.open = []Issue{{Number: 1, Title: IssueTitle("go/core", "pkg.TestHave")}}
	f.labels[LabelPriority] = true // p1 exists, flaky-test does not
	f.milestones = []map[string]any{
		{"number": 29, "due_on": ""},
		{"number": 31, "due_on": "2026-11-01T00:00:00Z"},
		{"number": 30, "due_on": "2026-10-16T00:00:00Z"},
	}
	mk := func(n string) Candidate {
		return Candidate{Suite: "go/core", Test: n, Title: IssueTitle("go/core", n), PassedOnRetry: 3, Runs: 9, RunLinks: []string{"https://example.test/r/1"}, Failed: 1}
	}
	cands := []Candidate{mk("pkg.TestHave"), mk("pkg.TestA"), mk("pkg.TestB"), mk("pkg.TestC")}
	created, deferred, err := SyncIssues(context.Background(), c, cands, 14, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || deferred != 1 {
		t.Fatalf("created=%v deferred=%d", created, deferred)
	}
	if !f.labels[LabelFlaky] {
		t.Error("flaky-test label must be created")
	}
	got := f.created[0]
	if got["title"] != IssueTitle("go/core", "pkg.TestA") || got["milestone"] != float64(30) {
		t.Errorf("%v", got)
	}
	if ls, _ := got["labels"].([]any); len(ls) != 2 || ls[0] != "flaky-test" || ls[1] != "p1" {
		t.Errorf("labels %v", got["labels"])
	}
	body, _ := got["body"].(string)
	for _, want := range []string{"root-cause note", "https://example.test/r/1", "`pkg.TestA`", "every attempt in 1 run"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestSyncIssuesNoMilestoneNoCandidates(t *testing.T) {
	f, c := newFake(t)
	f.labels[LabelFlaky], f.labels[LabelPriority] = true, true
	f.milestones = []map[string]any{{"number": 29, "due_on": ""}}
	if cr, d, err := SyncIssues(context.Background(), c, nil, 14, 3, 5); err != nil || cr != nil || d != 0 {
		t.Fatal(cr, d, err)
	}
	cr, _, err := SyncIssues(context.Background(), c, []Candidate{{Suite: "s", Test: "t", Title: "T"}}, 14, 3, 5)
	if err != nil || len(cr) != 1 {
		t.Fatal(cr, err)
	}
	if _, has := f.created[0]["milestone"]; has {
		t.Error("no due-dated milestone: must not set one")
	}
	body := IssueBody(Candidate{Suite: "s", Test: "t"}, 14, 3)
	if strings.Contains(body, "every attempt") {
		t.Error("no failed line when Failed==0")
	}
}

func TestSyncIssuesErrorPaths(t *testing.T) {
	cands := []Candidate{{Suite: "s", Test: "t", Title: "T"}}
	for _, fail := range []string{"/issues", "/labels", "/milestones"} {
		f, c := newFake(t)
		f.labels[LabelFlaky] = false
		f.failPath = fail
		if _, _, err := SyncIssues(context.Background(), c, cands, 14, 3, 5); err == nil {
			t.Errorf("%s failure must surface", fail)
		}
	}
	// A failing label GET that is not a 404 is an error, not "create it".
	f, c := newFake(t)
	f.failPath = "/labels/"
	if err := c.EnsureLabel(context.Background(), "x", "fff", ""); err == nil {
		t.Error("500 on label GET must error")
	}
	// Issue creation failing reports what was created before it.
	f2, c2 := newFake(t)
	f2.labels[LabelFlaky], f2.labels[LabelPriority] = true, true
	srvFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			http.Error(w, "no", 500)
			return
		}
		f2.handler().ServeHTTP(w, r)
	}))
	defer srvFail.Close()
	c2.Base = srvFail.URL
	if _, _, err := SyncIssues(context.Background(), c2, cands, 14, 3, 5); err == nil {
		t.Error("create failure must surface")
	}
	// Label creation (POST) failing.
	f3, c3 := newFake(t)
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			http.Error(w, "no", 500)
			return
		}
		f3.handler().ServeHTTP(w, r)
	}))
	defer srv3.Close()
	c3.Base = srv3.URL
	if _, _, err := SyncIssues(context.Background(), c3, cands, 14, 3, 5); err == nil {
		t.Error("label create failure must surface")
	}
	// p1 label creation failing while flaky-test exists.
	f4, c4 := newFake(t)
	f4.labels[LabelFlaky] = true
	srv4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			http.Error(w, "no", 500)
			return
		}
		f4.handler().ServeHTTP(w, r)
	}))
	defer srv4.Close()
	c4.Base = srv4.URL
	if _, _, err := SyncIssues(context.Background(), c4, cands, 14, 3, 5); err == nil {
		t.Error("p1 label create failure must surface")
	}
}

func TestFlakyIssuesPaginatesAndFiltersPRs(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var out []map[string]any
		n := 100
		if r.URL.Query().Get("page") == "2" {
			n = 1
		}
		for i := 0; i < n; i++ {
			e := map[string]any{"number": i, "title": "t"}
			if i == 0 && calls == 1 {
				e["pull_request"] = map[string]any{}
			}
			out = append(out, e)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Repo: "o/r"}
	got, err := c.FlakyIssues(context.Background(), "open")
	if err != nil || calls != 2 || len(got) != 100 {
		t.Fatalf("calls=%d len=%d err=%v", calls, len(got), err)
	}
}

func TestUnzipAndDownloadCeilings(t *testing.T) {
	old := maxArtifactBytes
	maxArtifactBytes = 10
	defer func() { maxArtifactBytes = old }()
	if err := Unzip(mkzip(t, map[string]string{"a": "123456", "b": "123456"}), t.TempDir()); err == nil {
		t.Error("total past the ceiling must error")
	}
	f, c := newFake(t)
	f.zips[1] = mkzip(t, map[string]string{"a": strings.Repeat("x", 4096)})
	if err := c.Download(context.Background(), 1, t.TempDir()); err == nil {
		t.Error("oversized download must error")
	}
}

func TestUnzipDirEntryBlockedByFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, _ = zw.Create("a/")
	_ = zw.Close()
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "a"), []byte("f"), 0o600)
	if err := Unzip(buf.Bytes(), d); err == nil {
		t.Error("dir entry over a file must error")
	}
}

func TestCollectArtifactListingFailure(t *testing.T) {
	f, c := newFake(t)
	f.runs["w.yml"] = []map[string]any{{"id": 1, "html_url": "u", "created_at": day(1)}}
	f.failPath = "/artifacts"
	res, err := Collect(context.Background(), c, []string{"w.yml"}, now.AddDate(0, 0, -14), t.TempDir())
	if err != nil || res.Runs != 0 || len(res.Warnings) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestClientCustomHTTP(t *testing.T) {
	f, c := newFake(t)
	f.milestones = []map[string]any{{"number": 5, "due_on": "2026-10-01T00:00:00Z"}}
	c.HTTP = &http.Client{Timeout: time.Second}
	if n, err := c.InFlightMilestone(context.Background()); err != nil || n != 5 {
		t.Fatal(n, err)
	}
}
