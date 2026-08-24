package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleReport is a minimal JUnit report in the shape `schemathesis run
// --checks not_a_server_error,ignored_auth --report junit` emits (captured
// from 4.25.2). It exercises both gated classes — a "Server error" failure
// (not_a_server_error) and an "API accepts requests without authentication"
// failure (ignored_auth) — plus a network <error> element that must be
// ignored, and a passing testcase.
const sampleReport = `<?xml version="1.0" encoding="utf-8"?>
<testsuites errors="1" failures="2" skipped="0" tests="3" time="1.0">
  <testsuite name="schemathesis" errors="1" failures="2" skipped="0" tests="3" time="1.0">
    <testcase name="GET /boom" time="0.13">
      <failure type="failure">1. Test Case ID: a1

- Server error

[500] Internal Server Error:

    ` + "`{\"error\": \"boom\"}`" + `

Reproduce with:

    curl -X GET http://localhost:7300/api/v1/boom</failure>
    </testcase>
    <testcase name="GET /protected" time="0.18">
      <failure type="failure">1. Test Case ID: b1

- API accepts requests without authentication

    Expected 401, got ` + "`200 OK`" + ` for ` + "`GET /protected`" + `</failure>
    </testcase>
    <testcase name="POST /field-definitions" time="2.29">
      <error type="error">Network Error

Connection failed

    Failed to establish a new connection</error>
    </testcase>
  </testsuite>
</testsuites>`

// cleanReport has only network errors and passing cases: the gate must pass
// without any ignore rules.
const cleanReport = `<?xml version="1.0" encoding="utf-8"?>
<testsuites errors="1" failures="0" skipped="0" tests="2" time="1.0">
  <testsuite name="schemathesis" errors="1" failures="0" skipped="0" tests="2" time="1.0">
    <testcase name="GET /contacts" time="0.04" />
    <testcase name="GET /contacts/{id}" time="0.04">
      <error type="error">Network Error

Connection failed</error>
    </testcase>
  </testsuite>
</testsuites>`

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
	report := writeTemp(t, "report.xml", sampleReport)
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
	// The network-error testcase must not appear in the failure list.
	if strings.Contains(err.Error(), "field-definitions") {
		t.Errorf("error %q mentions a network-error testcase", err.Error())
	}
}

func TestRun_IgnoreListAcceptsMatchingRules(t *testing.T) {
	report := writeTemp(t, "report.xml", sampleReport)
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
	report := writeTemp(t, "report.xml", sampleReport)
	ignore := writeTemp(t, "ignore", "auth GET /boom\nserver_error GET /protected\n")
	err := run(gateEnv([]string{report}, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error: kind-swapped rules must not match")
	}
}

func TestRun_CleanReportPassesWithoutRules(t *testing.T) {
	report := writeTemp(t, "report.xml", cleanReport)
	ignore := writeTemp(t, "ignore", "# none\n")
	if err := run(gateEnv([]string{report}, ignore)); err != nil {
		t.Fatalf("run() = %v, want nil (no gated failures, network errors only)", err)
	}
}

func TestRun_FailsOnEmptyReport(t *testing.T) {
	report := writeTemp(t, "report.xml", `<testsuites tests="0"></testsuites>`)
	ignore := writeTemp(t, "ignore", "# none\n")
	err := run(gateEnv([]string{report}, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error: a report with zero testcases is a blind scan")
	}
}

func TestRun_ReadsMultipleReports(t *testing.T) {
	r1 := writeTemp(t, "anon.xml", sampleReport)
	r2 := writeTemp(t, "auth.xml", sampleReport)
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

func TestClassifyFailure(t *testing.T) {
	cases := []struct {
		text     string
		wantKind string
		wantOK   bool
	}{
		{"- Server error\n\n[500] Internal Server Error", "server_error", true},
		{"- API accepts requests without authentication\n\nExpected 401, got 200", "auth", true},
		{"- API accepts invalid authentication\n\nExpected 401, got 200", "auth", true},
		{"- Undocumented HTTP status code\n\nReceived: 500", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		kind, ok := classifyFailure(c.text)
		if ok != c.wantOK || kind != c.wantKind {
			t.Errorf("classifyFailure(%q) = (%q, %v), want (%q, %v)", c.text, kind, ok, c.wantKind, c.wantOK)
		}
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
	if len(cfg.reportPaths) != 1 || cfg.reportPaths[0] != "schemathesis/report.xml" {
		t.Errorf("reportPaths = %v, want default single path", cfg.reportPaths)
	}
	if cfg.ignorePath != "schemathesis/schemathesis.ignore" {
		t.Errorf("ignorePath = %q", cfg.ignorePath)
	}
}

func TestLoadConfig_SplitsReportPaths(t *testing.T) {
	cfg := loadConfig(func(k string) string {
		if k == "SCHEMAGATE_REPORT" {
			return "a.xml, b.xml , c.xml"
		}
		return ""
	})
	want := []string{"a.xml", "b.xml", "c.xml"}
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
