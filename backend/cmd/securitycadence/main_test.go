package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureRegister = beginMarker + `
| id | obligation | interval_days | last_done | action |
|---|---|---|---|---|
| ` + "`jwt_secret_key`" + ` | Rotate ` + "`JWT_SECRET_KEY`" + ` | 365 | 2026-01-01 | see runbook |
| ` + "`data_encryption_key`" + ` | Rotate ` + "`DATA_ENCRYPTION_KEY`" + ` | 365 | 2026-01-01 | see runbook |
| ` + "`release_app_key`" + ` | Rotate the release App key | 365 | 2026-01-01 | see runbook |
| ` + "`android_signing_key`" + ` | Confirm the keystore is recoverable | 365 | 2026-01-01 | see runbook |
| ` + "`access_list_review`" + ` | Review the access list | 365 | 2026-01-01 | see GOVERNANCE.md |
| ` + "`support_window_review`" + ` | Review the support window | 365 | 2026-01-01 | see SECURITY.md |
| ` + "`governance_read_token`" + ` | Rotate the governance PAT | 365 | 2026-01-01 | see docs |
| ` + "`scorecard_token`" + ` | Rotate the scorecard PAT | 365 | 2026-01-01 | see docs |
` + endMarker

// writeFixtureTree creates a minimal repo tree whose docs/security/security-cadence.md
// is body, and returns its root.
func writeFixtureTree(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "security")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "security-cadence.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture register: %v", err)
	}
	return root
}

func TestParseRegister(t *testing.T) {
	entries, err := parseRegister([]byte(fixtureRegister))
	if err != nil {
		t.Fatalf("parseRegister: %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("got %d entries, want 8", len(entries))
	}
	want := Entry{
		ID:           "jwt_secret_key",
		Obligation:   "Rotate `JWT_SECRET_KEY`",
		IntervalDays: 365,
		LastDone:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Action:       "see runbook",
	}
	if entries[0] != want {
		t.Errorf("entries[0] = %+v, want %+v", entries[0], want)
	}
}

func TestParseRegisterRejectsMalformedRows(t *testing.T) {
	cases := map[string]string{
		"missing markers":  "| id | obligation | interval_days | last_done | action |\n|---|---|---|---|---|\n",
		"wrong cell count": beginMarker + "\n| id | obligation | interval_days | last_done | action |\n|---|---|---|---|---|\n| jim | Rotate | 365 | 2026-01-01 |\n" + endMarker,
		"bad interval":     beginMarker + "\n| id | obligation | interval_days | last_done | action |\n|---|---|---|---|---|\n| jim | Rotate | soon | 2026-01-01 | x |\n" + endMarker,
		"zero interval":    beginMarker + "\n| id | obligation | interval_days | last_done | action |\n|---|---|---|---|---|\n| jim | Rotate | 0 | 2026-01-01 | x |\n" + endMarker,
		"bad date":         beginMarker + "\n| id | obligation | interval_days | last_done | action |\n|---|---|---|---|---|\n| jim | Rotate | 365 | sometime | x |\n" + endMarker,
		"empty":            beginMarker + "\n" + endMarker,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRegister([]byte(body)); err == nil {
				t.Fatalf("parseRegister(%s) succeeded, want an error", name)
			}
		})
	}
}

func TestRunCurrentExitsZero(t *testing.T) {
	root := writeFixtureTree(t, fixtureRegister)
	var out strings.Builder
	// 2026-01-01 + 365 days = 2027-01-01; before that it is current.
	code := run(&out, root, config{registerPath: registerFile, now: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)})
	if code != 0 {
		t.Fatalf("run returned %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "All 8 security-stewardship obligation(s) are current.") {
		t.Errorf("output did not carry the all-current summary:\n%s", out.String())
	}
}

func TestRunOverdueExitsOne(t *testing.T) {
	root := writeFixtureTree(t, fixtureRegister)
	var out strings.Builder
	// Just past 2027-01-01.
	code := run(&out, root, config{registerPath: registerFile, now: time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)})
	if code != 1 {
		t.Fatalf("run returned %d, want 1\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "OVERDUE  jwt_secret_key") {
		t.Errorf("overdue output did not name jwt_secret_key:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "8 of 8 security-stewardship obligation(s) are overdue.") {
		t.Errorf("output did not carry the overdue tally:\n%s", out.String())
	}
}

// TestRunRejectsMissingRequiredObligation is the guard that stops a required
// secret class from being silently dropped from the register: the command must
// fail (exit 2, not "all current") when one is absent.
func TestRunRejectsMissingRequiredObligation(t *testing.T) {
	body := beginMarker + `
| id | obligation | interval_days | last_done | action |
|---|---|---|---|---|
| ` + "`jwt_secret_key`" + ` | Rotate | 365 | 2026-01-01 | x |
| ` + "`data_encryption_key`" + ` | Rotate | 365 | 2026-01-01 | x |
| ` + "`release_app_key`" + ` | Rotate | 365 | 2026-01-01 | x |
| ` + "`android_signing_key`" + ` | Review | 365 | 2026-01-01 | x |
| ` + "`access_list_review`" + ` | Review | 365 | 2026-01-01 | x |
` + endMarker
	root := writeFixtureTree(t, body)
	var out strings.Builder
	code := run(&out, root, config{registerPath: registerFile, now: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)})
	if code != 2 {
		t.Fatalf("run returned %d, want 2\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "support_window_review") {
		t.Errorf("output did not name the missing obligation:\n%s", out.String())
	}
}

func TestRunRefusesTraversalPath(t *testing.T) {
	root := writeFixtureTree(t, fixtureRegister)
	var out strings.Builder
	if code := run(&out, root, config{registerPath: "../secret", now: time.Now()}); code != 2 {
		t.Fatalf("run returned %d, want 2 for a traversal path", code)
	}
}

// TestRealRegisterParsesAndCoversEveryObligation runs the command against the
// committed register, so a malformed edit fails in CI even before the
// scheduled workflow runs.
func TestRealRegisterParsesAndCoversEveryObligation(t *testing.T) {
	path := findRepoFile(t, registerFile)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", registerFile, err)
	}
	entries, err := parseRegister(body)
	if err != nil {
		t.Fatalf("the committed register does not parse: %v", err)
	}
	if missing := missingRequired(entries); len(missing) > 0 {
		t.Fatalf("the committed register is missing required obligation(s): %v", missing)
	}
}

// TestScheduledWorkflowRunsTheCadenceCheck is the drift guard: the register and
// command are worthless if no scheduled workflow ever invokes them. It fails if
// the workflow stops referencing `go run ./cmd/securitycadence` or loses its
// schedule trigger, mirroring the #946 pentest-harness guard.
func TestScheduledWorkflowRunsTheCadenceCheck(t *testing.T) {
	body, err := os.ReadFile(findRepoFile(t, ".github/workflows/security-cadence.yml"))
	if err != nil {
		t.Fatalf("reading the cadence workflow: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "go run ./cmd/securitycadence") {
		t.Errorf(".github/workflows/security-cadence.yml no longer runs `go run ./cmd/securitycadence`")
	}
	if !strings.Contains(text, "schedule:") || !strings.Contains(text, "cron:") {
		t.Errorf(".github/workflows/security-cadence.yml no longer has a schedule trigger — the reminder would never fire")
	}
}

// findRepoFile walks up from the working directory until it finds rel, so the
// test works from backend/cmd/securitycadence (the default `go test` cwd) and
// from the repo root.
func findRepoFile(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("%s not found from the working directory upward", rel)
		}
		dir = parent
	}
}

func TestDueSoonWindow(t *testing.T) {
	e := Entry{IntervalDays: 90, LastDone: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)} // due 2027-02-09
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"well before the window", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), statusOK},
		{"one day before the window opens", time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC), statusOK},
		{"first day of the 7-day window", time.Date(2027, 2, 2, 0, 0, 0, 0, time.UTC), statusDueSoon},
		{"on the due date", time.Date(2027, 2, 9, 0, 0, 0, 0, time.UTC), statusDueSoon},
		{"past the due date", time.Date(2027, 2, 10, 0, 0, 0, 0, time.UTC), statusOverdue},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := e.Status(c.now, 7); got != c.want {
				t.Errorf("Status = %q, want %q", got, c.want)
			}
		})
	}
	if got := e.Status(time.Date(2027, 2, 5, 0, 0, 0, 0, time.UTC), 0); got != statusOK {
		t.Errorf("lead 0 must never report due-soon, got %q", got)
	}
}

// Due soon is a heads-up, not a failure: the exit code stays 0 so the job only
// goes red when something is actually overdue.
func TestRunDueSoonStillExitsZeroButIsLabelled(t *testing.T) {
	root := writeFixtureTree(t, fixtureRegister)
	var out strings.Builder
	// fixtureRegister entries fall due 2027-01-01.
	code := run(&out, root, config{registerPath: registerFile, now: time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC), leadDays: 7})
	if code != 0 {
		t.Fatalf("run returned %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "DUE SOON jwt_secret_key") {
		t.Errorf("output did not flag jwt_secret_key as due soon:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "8 security-stewardship obligation(s) are due within 7 days") {
		t.Errorf("output did not carry the due-soon tally:\n%s", out.String())
	}
}

func TestRunJSONListsStatusPerEntry(t *testing.T) {
	root := writeFixtureTree(t, fixtureRegister)
	var out strings.Builder
	code := run(&out, root, config{registerPath: registerFile, now: time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), leadDays: 7, jsonOut: true})
	if code != 1 {
		t.Fatalf("run returned %d, want 1 (overdue) even in JSON mode\n%s", code, out.String())
	}
	var got []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Due    string `json:"due"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("JSON mode output is not a bare JSON array: %v\n%s", err, out.String())
	}
	if len(got) != 8 || got[0].ID != "jwt_secret_key" || got[0].Status != statusOverdue || got[0].Due != "2027-01-01" || got[0].Action != "see runbook" {
		t.Errorf("unexpected JSON entries: %+v", got)
	}
}

// The two fine-grained PATs expire on GitHub's clock, not ours, so they are
// required register entries on a 90-day interval.
func TestRealRegisterCarriesTokenEntriesOn90Days(t *testing.T) {
	body, err := os.ReadFile(findRepoFile(t, registerFile))
	if err != nil {
		t.Fatalf("reading %s: %v", registerFile, err)
	}
	entries, err := parseRegister(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, id := range []string{"governance_read_token", "scorecard_token"} {
		found := false
		for _, e := range entries {
			if e.ID == id {
				found = true
				if e.IntervalDays != 90 {
					t.Errorf("%s interval = %d, want 90", id, e.IntervalDays)
				}
			}
		}
		if !found {
			t.Errorf("register is missing %s", id)
		}
	}
}

func TestRotationIssuesScriptIsWired(t *testing.T) {
	if _, err := os.Stat(findRepoFile(t, ".github/scripts/rotation-issues.sh")); err != nil {
		t.Fatalf("rotation-issues.sh missing: %v", err)
	}
	body, err := os.ReadFile(findRepoFile(t, ".github/workflows/security-cadence.yml"))
	if err != nil {
		t.Fatalf("reading workflow: %v", err)
	}
	if !strings.Contains(string(body), "rotation-issues.sh") || !strings.Contains(string(body), "issues: write") {
		t.Errorf("security-cadence.yml must run rotation-issues.sh with issues: write")
	}
}
