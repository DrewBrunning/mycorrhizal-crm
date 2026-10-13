package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rulesetJSON = `{"name":"main-protection","target":"branch","enforcement":"active",
"rules":[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Backend (Go)"},{"context":"codecov/patch/backend"}]}}]}`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runCmd(t *testing.T, client *http.Client, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, client, func(k string) string { return env[k] })
	return code, out.String(), errb.String()
}

func TestOfflineFixture(t *testing.T) {
	rs := writeTemp(t, "rs.json", rulesetJSON)
	fx := writeTemp(t, "fx.json", `{"check_runs":[{"name":"Backend (Go)","status":"completed","conclusion":"failure","at":"2026-10-01T00:00:00Z"}],
"statuses":[{"context":"codecov/patch/backend","state":"success","at":"2026-10-01T00:00:00Z"}]}`)
	code, out, _ := runCmd(t, nil, nil, "-ruleset", rs, "-input", fx, "-sha", "abc")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var v Verdict
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if v.SHA != "abc" || v.Required != 2 || len(v.Findings) != 1 || v.Findings[0].Context != "Backend (Go)" || v.Findings[0].Category != "failed" {
		t.Fatalf("verdict %+v", v)
	}
}

func TestAllGreenEmitsEmptyArray(t *testing.T) {
	rs := writeTemp(t, "rs.json", rulesetJSON)
	fx := writeTemp(t, "fx.json", `{"check_runs":[{"name":"Backend (Go)","status":"completed","conclusion":"success","at":"2026-10-01T00:00:00Z"}],
"statuses":[{"context":"codecov/patch/backend","state":"success","at":"2026-10-01T00:00:00Z"}]}`)
	code, out, _ := runCmd(t, nil, nil, "-ruleset", rs, "-input", fx)
	if code != 0 || !strings.Contains(out, `"findings": []`) {
		t.Fatalf("exit %d out %s", code, out)
	}
}

func TestUsageErrors(t *testing.T) {
	rs := writeTemp(t, "rs.json", rulesetJSON)
	noChecks := writeTemp(t, "nochecks.json", `{"name":"x","target":"branch","enforcement":"active","rules":[{"type":"required_linear_history"}]}`)
	badFx := writeTemp(t, "bad.json", `{`)
	tests := []struct {
		name string
		env  map[string]string
		args []string
	}{
		{"bad flag", nil, []string{"-nope"}},
		{"missing ruleset", nil, []string{"-ruleset", "/nonexistent"}},
		{"invalid ruleset", nil, []string{"-ruleset", writeTemp(t, "i.json", `{}`)}},
		{"no required checks", nil, []string{"-ruleset", noChecks}},
		{"missing fixture", nil, []string{"-ruleset", rs, "-input", "/nonexistent"}},
		{"bad fixture", nil, []string{"-ruleset", rs, "-input", badFx}},
		{"no repo/sha", nil, []string{"-ruleset", rs}},
		{"no token", nil, []string{"-ruleset", rs, "-repo", "o/r", "-sha", "s"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code, _, _ := runCmd(t, nil, tc.env, tc.args...); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
		})
	}
}

// fakeAPI serves check-runs and statuses, paginating at perPage, and records
// the Authorization header.
func fakeAPI(t *testing.T, runsTotal int, failStatus bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		page := r.URL.Query().Get("page")
		switch {
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			if r.URL.Query().Get("filter") != "all" {
				t.Errorf("filter=all not sent")
			}
			var runs []string
			if page == "1" {
				n := runsTotal
				if n > perPage {
					n = perPage
				}
				for i := 0; i < n; i++ {
					runs = append(runs, fmt.Sprintf(`{"name":"filler%d","status":"completed","conclusion":"success","started_at":"2026-10-01T00:00:00Z","completed_at":"2026-10-01T00:01:00Z"}`, i))
				}
			} else {
				runs = append(runs,
					`{"name":"Backend (Go)","status":"completed","conclusion":"failure","started_at":"2026-10-01T00:00:00Z","completed_at":"2026-10-01T00:05:00Z"}`,
					`{"name":"queuedjob","status":"queued","conclusion":null,"started_at":"2026-10-01T00:00:00Z","completed_at":null}`)
			}
			fmt.Fprintf(w, `{"total_count":%d,"check_runs":[%s]}`, runsTotal, strings.Join(runs, ","))
		case strings.HasSuffix(r.URL.Path, "/statuses"):
			if failStatus {
				fmt.Fprint(w, `not json`)
				return
			}
			fmt.Fprint(w, `[{"context":"codecov/patch/backend","state":"success","created_at":"2026-10-01T00:00:00Z"}]`)
		}
	}))
}

func TestLiveMode(t *testing.T) {
	srv := fakeAPI(t, perPage, false) // 100 on page 1 forces a second page
	defer srv.Close()
	rs := writeTemp(t, "rs.json", rulesetJSON)
	code, out, errs := runCmd(t, srv.Client(), map[string]string{"GITHUB_TOKEN": "tok", "GITHUB_API_URL": srv.URL},
		"-ruleset", rs, "-repo", "o/r", "-sha", "deadbeef")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	var v Verdict
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Findings) != 1 || v.Findings[0].Context != "Backend (Go)" || v.Findings[0].Detail != "failure" {
		t.Fatalf("verdict %+v", v)
	}
}

func TestLiveModeAPIErrors(t *testing.T) {
	srv := fakeAPI(t, 1, false)
	defer srv.Close()
	rs := writeTemp(t, "rs.json", rulesetJSON)
	// Wrong token -> HTTP 401.
	if code, _, _ := runCmd(t, srv.Client(), map[string]string{"GITHUB_TOKEN": "bad", "GITHUB_API_URL": srv.URL},
		"-ruleset", rs, "-repo", "o/r", "-sha", "s"); code != 2 {
		t.Fatalf("401: exit %d", code)
	}
	// Malformed statuses body.
	bad := fakeAPI(t, 1, true)
	defer bad.Close()
	if code, _, _ := runCmd(t, bad.Client(), map[string]string{"GITHUB_TOKEN": "tok", "GITHUB_API_URL": bad.URL},
		"-ruleset", rs, "-repo", "o/r", "-sha", "s"); code != 2 {
		t.Fatalf("bad json: exit %d", code)
	}
	// Unreachable server.
	srv.Close()
	if code, _, _ := runCmd(t, http.DefaultClient, map[string]string{"GITHUB_TOKEN": "tok", "GITHUB_API_URL": srv.URL},
		"-ruleset", rs, "-repo", "o/r", "-sha", "s"); code != 2 {
		t.Fatalf("unreachable: exit %d", code)
	}
}

func TestMainUsesOsExit(t *testing.T) {
	var got int
	osExit = func(c int) { got = c }
	defer func() { osExit = os.Exit }()
	old := os.Args
	os.Args = []string{"bypassedcheck", "-nope"}
	defer func() { os.Args = old }()
	main()
	if got != 2 {
		t.Fatalf("exit %d", got)
	}
}

func TestTruncate(t *testing.T) {
	if len(truncate(strings.Repeat("x", 500))) != 200 || truncate("ab") != "ab" {
		t.Fatal("truncate")
	}
}
