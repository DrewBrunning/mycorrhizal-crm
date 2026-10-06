package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 6, 6, 0, 0, 0, time.UTC)

func nowFn() time.Time { return fixedNow }

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func call(t *testing.T, e map[string]string, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := run(context.Background(), args, env(e), &buf, nowFn)
	return code, buf.String()
}

func zipOf(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	_, _ = w.Write([]byte(body))
	_ = zw.Close()
	return buf.Bytes()
}

// fakeAPI serves one workflow with 4 runs, each carrying a passed-on-retry
// signal for the same test, plus a closed-issue history and open-issue list.
func fakeAPI(t *testing.T, openTitles []string, created *[]map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/repos/o/r")
		switch {
		case p == "/actions/workflows/unit-tests.yml/runs":
			var runs []map[string]any
			for i := 1; i <= 4; i++ {
				runs = append(runs, map[string]any{"id": 100 + i, "html_url": "https://example.test/run/" + string(rune('0'+i)), "event": "push",
					"created_at": fixedNow.AddDate(0, 0, -i).Format(time.RFC3339), "conclusion": "success"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"workflow_runs": runs})
		case strings.HasSuffix(p, "/artifacts") && strings.HasPrefix(p, "/actions/runs/"):
			id := int64(0)
			for _, c := range strings.Split(p, "/")[3] {
				id = id*10 + int64(c-'0')
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": []map[string]any{{"id": id, "name": "flake-signals-go"}}})
		case strings.HasPrefix(p, "/actions/artifacts/"):
			_, _ = w.Write(zipOf(t, "flake-signals-go.json", `{"suite":"go/core","tests":[{"test":"pkg.TestFlaky","outcome":"passed_on_retry"},{"test":"pkg.TestOK","outcome":"passed"}]}`))
		case p == "/issues" && r.Method == "GET":
			if r.URL.Query().Get("state") == "closed" {
				_, _ = w.Write([]byte(`[{"number":1,"title":"Flaky test: go/core pkg.TestFlaky","closed_at":"` + fixedNow.AddDate(0, 0, -30).Format(time.RFC3339) + `"}]`))
				return
			}
			var is []map[string]any
			for _, ti := range openTitles {
				is = append(is, map[string]any{"number": 2, "title": ti})
			}
			_ = json.NewEncoder(w).Encode(is)
		case p == "/issues" && r.Method == "POST":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			*created = append(*created, b)
			w.WriteHeader(201)
			_, _ = w.Write([]byte("{}"))
		case strings.HasPrefix(p, "/labels/"):
			_, _ = w.Write([]byte("{}"))
		case p == "/milestones":
			_, _ = w.Write([]byte(`[{"number":30,"due_on":"2026-10-16T00:00:00Z"}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCollectReportIssuesPipeline(t *testing.T) {
	var created []map[string]any
	srv := fakeAPI(t, nil, &created)
	e := map[string]string{"GITHUB_REPOSITORY": "o/r", "GITHUB_TOKEN": "t", "FLAKELEDGER_API_BASE": srv.URL}
	dir := t.TempDir()
	data := filepath.Join(dir, "data")

	if code, out := call(t, e, "collect", "-data", data, "-workflows", "unit-tests.yml, ,"); code != 0 || !strings.Contains(out, "collected 4 run(s), 4 artifact(s)") {
		t.Fatalf("collect: %d %s", code, out)
	}
	md, cands := filepath.Join(dir, "ledger.md"), filepath.Join(dir, "cands.json")
	code, out := call(t, e, "report", "-data", data, "-md", md, "-candidates", cands)
	if code != 0 || !strings.Contains(out, "1 issue candidate(s)") {
		t.Fatalf("report: %d %s", code, out)
	}
	b, _ := os.ReadFile(md)
	if !strings.Contains(string(b), "| go/core | pkg.TestFlaky | 4 | 4 | 4 | 0 | 100% |") {
		t.Errorf("ledger markdown:\n%s", b)
	}
	// The closed issue (30 days ago) predates every event, so the test is still a candidate.
	if code, out := call(t, e, "issues", "-candidates", cands, "-limit", "1"); code != 0 || !strings.Contains(out, "opened: Flaky test: go/core pkg.TestFlaky") {
		t.Fatalf("issues: %d %s", code, out)
	}
	if len(created) != 1 || created[0]["milestone"] != float64(30) {
		t.Fatalf("%v", created)
	}
}

func TestIssuesSkipsExistingAndDefers(t *testing.T) {
	var created []map[string]any
	srv := fakeAPI(t, []string{"Flaky test: s a"}, &created)
	e := map[string]string{"GITHUB_REPOSITORY": "o/r", "FLAKELEDGER_API_BASE": srv.URL}
	cands := filepath.Join(t.TempDir(), "c.json")
	_ = os.WriteFile(cands, []byte(`[{"suite":"s","test":"a","title":"Flaky test: s a"},{"suite":"s","test":"b","title":"Flaky test: s b"},{"suite":"s","test":"c","title":"Flaky test: s c"}]`), 0o600)
	code, out := call(t, e, "issues", "-candidates", cands, "-limit", "1")
	if code != 0 || len(created) != 1 || !strings.Contains(out, "1 candidate(s) deferred") {
		t.Fatalf("%d %s %v", code, out, created)
	}
}

func TestReportClosedIssueSuppressesCandidate(t *testing.T) {
	// Same fixtures, but the issue was closed yesterday: only 0 events postdate it.
	var created []map[string]any
	srv := fakeAPI(t, nil, &created)
	e := map[string]string{"GITHUB_REPOSITORY": "o/r", "FLAKELEDGER_API_BASE": srv.URL}
	dir := t.TempDir()
	if code, out := call(t, e, "collect", "-data", dir, "-workflows", "unit-tests.yml"); code != 0 {
		t.Fatal(out)
	}
	closed := map[string]string{"Flaky test: go/core pkg.TestFlaky": fixedNow.AddDate(0, 0, -1).Add(time.Hour).Format(time.RFC3339), "x": "garbage"}
	b, _ := json.Marshal(closed)
	_ = os.WriteFile(filepath.Join(dir, "closed.json"), b, 0o600)
	cands := filepath.Join(dir, "c.json")
	if code, out := call(t, e, "report", "-data", dir, "-md", filepath.Join(dir, "l.md"), "-candidates", cands); code != 0 || !strings.Contains(out, "0 issue candidate(s)") {
		t.Fatalf("%d %s", code, out)
	}
	if b, _ := os.ReadFile(cands); strings.TrimSpace(string(b)) != "[]" {
		t.Errorf("candidates must be [] not null: %s", b)
	}
	// Corrupt closed.json degrades to no history.
	_ = os.WriteFile(filepath.Join(dir, "closed.json"), []byte("{"), 0o600)
	if code, out := call(t, e, "report", "-data", dir, "-md", filepath.Join(dir, "l.md"), "-candidates", cands); code != 0 || !strings.Contains(out, "1 issue candidate(s)") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestUsageAndArgErrors(t *testing.T) {
	e := map[string]string{"GITHUB_REPOSITORY": "o/r"}
	for _, args := range [][]string{
		{},
		{"nope"},
		{"collect", "-bogus"},
		{"collect"},
		{"report"},
		{"issues"},
		{"report", "-data", t.TempDir(), "-md", "x", "-candidates", "y"}, // no runs.json
		{"issues", "-candidates", filepath.Join(t.TempDir(), "missing.json")},
	} {
		if code, _ := call(t, e, args...); code != 2 {
			t.Errorf("%v: want exit 2, got %d", args, code)
		}
	}
	// Missing GITHUB_REPOSITORY.
	if code, out := call(t, nil, "collect", "-data", t.TempDir(), "-workflows", "a.yml"); code != 2 || !strings.Contains(out, "GITHUB_REPOSITORY") {
		t.Errorf("%d %s", code, out)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte("{"), 0o600)
	if code, _ := call(t, e, "issues", "-candidates", bad); code != 2 {
		t.Error("bad candidates json must exit 2")
	}
	good := filepath.Join(t.TempDir(), "good.json")
	_ = os.WriteFile(good, []byte("[]"), 0o600)
	if code, _ := call(t, nil, "issues", "-candidates", good); code != 2 {
		t.Error("missing repo must exit 2")
	}
}

func TestOutputWriteFailuresAndAPIErrors(t *testing.T) {
	var created []map[string]any
	srv := fakeAPI(t, nil, &created)
	e := map[string]string{"GITHUB_REPOSITORY": "o/r", "FLAKELEDGER_API_BASE": srv.URL}
	dir := t.TempDir()
	if code, out := call(t, e, "collect", "-data", dir, "-workflows", "unit-tests.yml"); code != 0 {
		t.Fatal(out)
	}
	noDir := filepath.Join(t.TempDir(), "no", "such")
	if code, _ := call(t, e, "report", "-data", dir, "-md", filepath.Join(noDir, "a.md"), "-candidates", filepath.Join(dir, "c.json")); code != 2 {
		t.Error("md write failure must exit 2")
	}
	if code, _ := call(t, e, "report", "-data", dir, "-md", filepath.Join(dir, "a.md"), "-candidates", filepath.Join(noDir, "c.json")); code != 2 {
		t.Error("candidates write failure must exit 2")
	}

	// collect: closed.json cannot be written when -data is a file.
	f := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(f, nil, 0o600)
	if code, _ := call(t, e, "collect", "-data", f, "-workflows", "unit-tests.yml"); code != 2 {
		t.Error("collect into a file must exit 2")
	}

	// A 500 from every endpoint: collect degrades (warnings) but still writes; issues exits 2.
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }))
	defer down.Close()
	e["FLAKELEDGER_API_BASE"] = down.URL
	d2 := t.TempDir()
	if code, out := call(t, e, "collect", "-data", d2, "-workflows", "unit-tests.yml"); code != 0 || !strings.Contains(out, "warning:") {
		t.Fatalf("%d %s", code, out)
	}
	cands := filepath.Join(d2, "c.json")
	_ = os.WriteFile(cands, []byte(`[{"suite":"s","test":"t","title":"T"}]`), 0o600)
	if code, _ := call(t, e, "issues", "-candidates", cands); code != 2 {
		t.Error("API failure while opening issues must exit 2")
	}
}
