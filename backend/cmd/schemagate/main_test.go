package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleReport is a minimal slice of a real `schemathesis run --report ndjson`
// report (captured from 4.25.2 against a deliberately-vulnerable stub server),
// reduced to the ScenarioFinished events schemagate reads. It exercises the two
// gated check classes (not_a_server_error -> server_error, ignored_auth ->
// auth) plus a non-gated conformance failure that must be ignored. Note the
// real report shape: `recorder.checks` is a case-ID-keyed map parallel to
// `recorder.cases`, not nested inside each case.
const sampleReport = `{"ScenarioFinished":{"status":"failure","recorder":{"label":"GET /boom","cases":{"a1":{"value":{"method":"GET","path":"/boom"}}},"checks":{"a1":[{"name":"not_a_server_error","status":"failure"},{"name":"status_code_conformance","status":"failure"},{"name":"ignored_auth","status":"success"}]}}}}
{"ScenarioFinished":{"status":"failure","recorder":{"label":"GET /protected","cases":{"b1":{"value":{"method":"GET","path":"/protected"}}},"checks":{"b1":[{"name":"not_a_server_error","status":"success"},{"name":"ignored_auth","status":"failure"},{"name":"negative_data_rejection","status":"failure"}]}}}}
`

// cleanReport has only non-gated conformance failures and passing gated checks:
// the gate must pass without any ignore rules.
const cleanReport = `{"ScenarioFinished":{"status":"failure","recorder":{"label":"GET /contacts","cases":{"c1":{"value":{"method":"GET","path":"/contacts"}}},"checks":{"c1":[{"name":"not_a_server_error","status":"success"},{"name":"ignored_auth","status":"success"},{"name":"response_schema_conformance","status":"failure"}]}}}}
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func gateEnv(reports []string, ignore string) func(string) string {
	return func(k string) string {
		switch k {
		case "SCHEMAGATE_REPORT":
			return strings.Join(reports, ",")
		case "SCHEMAGATE_IGNORE":
			return ignore
		default:
			return ""
		}
	}
}

func TestRun_FailsOnUnacceptedFindings(t *testing.T) {
	report := writeTemp(t, "report.ndjson", sampleReport)
	ignore := writeTemp(t, "ignore", "# no rules\n")
	err := run(gateEnv([]string{report}, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error for unaccepted server_error + auth findings")
	}
	for _, want := range []string{"server_error", "auth", "GET /boom", "GET /protected"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
	// The non-gated conformance failures must not appear in the failure list.
	if strings.Contains(err.Error(), "conformance") {
		t.Errorf("error %q mentions a non-gated check", err.Error())
	}
}

func TestRun_IgnoreListAcceptsMatchingRules(t *testing.T) {
	report := writeTemp(t, "report.ndjson", sampleReport)
	ignore := writeTemp(t, "ignore", `
server_error GET /boom   # the canary endpoint 500s by design in this fixture
auth GET /protected      # the fixture returns data without auth by design
`)
	err := run(gateEnv([]string{report}, ignore))
	if err != nil {
		t.Fatalf("run() = %v, want nil (both findings accepted)", err)
	}
}

func TestRun_IgnoreListRequiresExactKind(t *testing.T) {
	// A rule for the wrong kind must not suppress the finding.
	report := writeTemp(t, "report.ndjson", sampleReport)
	ignore := writeTemp(t, "ignore", "auth GET /boom\nserver_error GET /protected\n")
	err := run(gateEnv([]string{report}, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error: kind-swapped rules must not match")
	}
}

func TestRun_CleanReportPassesWithoutRules(t *testing.T) {
	report := writeTemp(t, "report.ndjson", cleanReport)
	ignore := writeTemp(t, "ignore", "# none\n")
	if err := run(gateEnv([]string{report}, ignore)); err != nil {
		t.Fatalf("run() = %v, want nil (no gated failures, conformance noise only)", err)
	}
}

func TestRun_FailsOnEmptyReport(t *testing.T) {
	report := writeTemp(t, "report.ndjson", "# not ndjson\n")
	ignore := writeTemp(t, "ignore", "# none\n")
	err := run(gateEnv([]string{report}, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error: a report with no check results is a blind scan")
	}
}

func TestRun_ReadsMultipleReports(t *testing.T) {
	// The same operation failing in two reports (e.g. anonymous + authenticated
	// runs) must be deduplicated to a single failure line.
	r1 := writeTemp(t, "anon.ndjson", sampleReport)
	r2 := writeTemp(t, "auth.ndjson", sampleReport)
	ignore := writeTemp(t, "ignore", "# none\n")
	err := run(gateEnv([]string{r1, r2}, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error")
	}
	if strings.Count(err.Error(), "GET /boom") != 1 {
		t.Errorf("error %q should mention GET /boom exactly once (dedupe), got %d",
			err.Error(), strings.Count(err.Error(), "GET /boom"))
	}
}

func TestReadIgnoreList_Formats(t *testing.T) {
	rules, err := readIgnoreList(mustWrite(t, `
# a pure comment
server_error POST /contacts/import/upload   # multipart uploads 500 on fuzzed bodies
auth GET /contacts/.*
* POST /contacts/import/.*
`))
	if err != nil {
		t.Fatalf("readIgnoreList: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}
	if rules[0].kind != "server_error" || rules[0].justification != "multipart uploads 500 on fuzzed bodies" {
		t.Errorf("rule 0 = %+v, want kind=server_error with justification", rules[0])
	}
	if rules[1].kind != "auth" {
		t.Errorf("rule 1 kind = %q, want auth", rules[1].kind)
	}
	if rules[2].kind != "*" {
		t.Errorf("rule 2 kind = %q, want *", rules[2].kind)
	}
}

func TestReadIgnoreList_BadKind(t *testing.T) {
	_, err := readIgnoreList(mustWrite(t, "schema GET /contacts\n"))
	if err == nil {
		t.Fatal("readIgnoreList() = nil, want error for unknown kind")
	}
}

func TestReadIgnoreList_BadRegex(t *testing.T) {
	_, err := readIgnoreList(mustWrite(t, "server_error [unclosed\n"))
	if err == nil {
		t.Fatal("readIgnoreList() = nil, want error for bad regex")
	}
}

func TestMatchIgnore_WildcardKind(t *testing.T) {
	rules, err := readIgnoreList(mustWrite(t, "* GET /contacts/.*\n"))
	if err != nil {
		t.Fatalf("readIgnoreList: %v", err)
	}
	for _, kind := range []string{"server_error", "auth"} {
		f := finding{kind: kind, operation: "GET /contacts/123"}
		if _, ok := matchIgnore(rules, f); !ok {
			t.Errorf("matchIgnore(%s) = false, want wildcard kind to match", kind)
		}
	}
	f := finding{kind: "server_error", operation: "GET /circles"}
	if _, ok := matchIgnore(rules, f); ok {
		t.Error("matchIgnore = true, want non-matching operation to be rejected")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := loadConfig(func(string) string { return "" })
	if len(cfg.reportPaths) != 1 || cfg.reportPaths[0] != "schemathesis/report.ndjson" {
		t.Errorf("reportPaths = %v, want default single path", cfg.reportPaths)
	}
	if cfg.ignorePath != "schemathesis/schemathesis.ignore" {
		t.Errorf("ignorePath = %q", cfg.ignorePath)
	}
}

func TestLoadConfig_SplitsReportPaths(t *testing.T) {
	cfg := loadConfig(func(k string) string {
		if k == "SCHEMAGATE_REPORT" {
			return "a.ndjson, b.ndjson , c.ndjson"
		}
		return ""
	})
	want := []string{"a.ndjson", "b.ndjson", "c.ndjson"}
	if len(cfg.reportPaths) != len(want) {
		t.Fatalf("reportPaths = %v, want %v", cfg.reportPaths, want)
	}
	for i := range want {
		if cfg.reportPaths[i] != want[i] {
			t.Errorf("reportPaths[%d] = %q, want %q", i, cfg.reportPaths[i], want[i])
		}
	}
}

func mustWrite(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schemathesis.ignore")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}
