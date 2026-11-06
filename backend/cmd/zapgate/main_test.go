package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A minimal slice of the real ZAP traditional-JSON report shape (captured from
// a 2.17.0 run), reduced to the fields zapgate reads. Kept as a raw string so
// the test exercises the actual JSON unmarshalling, not a Go struct round-trip.
const sampleReport = `{
  "site": [
    {
      "@name": "http://localhost:7300",
      "alerts": [
        {
          "pluginid": "40012",
          "alert": "Cross Site Scripting (Reflected)",
          "riskcode": "3",
          "riskdesc": "High (Medium)",
          "instances": [ { "uri": "http://localhost:7300/api/v1/contacts?q=1" } ]
        },
        {
          "pluginid": "10038",
          "alert": "Content Security Policy (CSP) Header Not Set",
          "riskcode": "2",
          "riskdesc": "Medium (High)",
          "instances": [ { "uri": "http://localhost:7300/api/v1/contacts" } ]
        },
        {
          "pluginid": "10021",
          "alert": "X-Content-Type-Options Header Missing",
          "riskcode": "1",
          "riskdesc": "Low (Medium)",
          "instances": [ { "uri": "http://localhost:7300/api/v1/contacts" } ]
        }
      ]
    },
    {
      "@name": "http://localhost:7301",
      "alerts": [
        {
          "pluginid": "40012",
          "alert": "Cross Site Scripting (Reflected)",
          "riskcode": "3",
          "riskdesc": "High (Medium)",
          "instances": [ { "uri": "http://localhost:7301/reflected?q=%3Cscript%3E" } ]
        },
        {
          "pluginid": "10038",
          "alert": "Content Security Policy (CSP) Header Not Set",
          "riskcode": "2",
          "riskdesc": "Medium (High)",
          "instances": [ { "uri": "http://localhost:7301/reflected?q=hello" } ]
        }
      ]
    }
  ]
}`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func writeConfigFiles(t *testing.T, report, ignore string) (string, string) {
	t.Helper()
	return writeTemp(t, "report.json", report), writeTemp(t, "dast.ignore", ignore)
}

func gateEnv(report, ignore string) func(string) string {
	return func(k string) string {
		switch k {
		case "ZAPGATE_REPORT":
			return report
		case "ZAPGATE_IGNORE":
			return ignore
		default:
			return ""
		}
	}
}

func TestRun_FailsOnUnignoredAppFindings(t *testing.T) {
	report, ignore := writeConfigFiles(t, sampleReport, "# no rules\n")
	err := run(gateEnv(report, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error for unignored app High/Medium findings")
	}
	for _, want := range []string{"40012", "10038", "Cross Site Scripting"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestRun_IgnoreListAcceptsMatchingRules(t *testing.T) {
	report, ignore := writeConfigFiles(t, sampleReport, `
# Reflected XSS false positive on the contacts search echo — justified.
40012 high http://localhost:7300/api/v1/contacts.*
# CSP is set by security_headers middleware; ZAP's passive rule misfires on
# application/json responses.
10038 medium .*api/v1/contacts.*
`)
	err := run(gateEnv(report, ignore))
	if err != nil {
		t.Fatalf("run() = %v, want nil (both app findings accepted)", err)
	}
}

func TestRun_IgnoreListRequiresExactPluginAndRisk(t *testing.T) {
	// A rule for the *wrong* plugin id (or risk) must NOT suppress a finding.
	report, ignore := writeConfigFiles(t, sampleReport, `
99999 high http://localhost:7300/api/v1/contacts.*
`)
	err := run(gateEnv(report, ignore))
	if err == nil {
		t.Fatal("run() = nil, want error: rule for a different plugin must not match")
	}
}

func TestRun_SelfTestFailsWhenCanaryXSSMissing(t *testing.T) {
	// The app is clean and its findings accepted, but the canary's reflected-XSS
	// (the self-test signal) is absent — the scan must still fail as blind.
	report, ignore := writeConfigFiles(t, `{
  "site": [
    { "@name": "http://localhost:7301", "alerts": [] }
  ]
}`, "# none\n")
	err := run(gateEnv(report, ignore))
	if err == nil {
		t.Fatal("run() = nil, want self-test failure")
	}
	if !strings.Contains(err.Error(), "self-test") {
		t.Errorf("error %q does not mention the self-test", err.Error())
	}
}

func TestRun_SelfTestPassesWithCleanApp(t *testing.T) {
	report, ignore := writeConfigFiles(t, `{
  "site": [
    {
      "@name": "http://localhost:7301",
      "alerts": [
        {
          "pluginid": "40012",
          "alert": "Cross Site Scripting (Reflected)",
          "riskcode": "3",
          "riskdesc": "High (Medium)",
          "instances": [ { "uri": "http://localhost:7301/reflected?q=1" } ]
        }
      ]
    }
  ]
}`, "# none\n")
	err := run(gateEnv(report, ignore))
	if err != nil {
		t.Fatalf("run() = %v, want nil (canary self-test present, app clean)", err)
	}
}

func TestRun_LowAndInfoIgnored(t *testing.T) {
	// Only Low/Info findings on the app: gate passes without any ignore rules.
	report, ignore := writeConfigFiles(t, `{
  "site": [
    {
      "@name": "http://localhost:7300",
      "alerts": [
        {
          "pluginid": "10021",
          "alert": "X-Content-Type-Options Header Missing",
          "riskcode": "1",
          "riskdesc": "Low (Medium)",
          "instances": [ { "uri": "http://localhost:7300/api/v1/contacts" } ]
        }
      ]
    },
    {
      "@name": "http://localhost:7301",
      "alerts": [
        {
          "pluginid": "40012",
          "alert": "Cross Site Scripting (Reflected)",
          "riskcode": "3",
          "riskdesc": "High (Medium)",
          "instances": [ { "uri": "http://localhost:7301/reflected?q=1" } ]
        }
      ]
    }
  ]
}`, "# none\n")
	err := run(gateEnv(report, ignore))
	if err != nil {
		t.Fatalf("run() = %v, want nil (Low findings are not gated)", err)
	}
}

func TestReadIgnoreList_Formats(t *testing.T) {
	rules, err := readIgnoreList(mustWrite(t, `
# a pure comment
40012 high http://localhost:7300/.*   # justified acceptance
* medium .*api/v1/.*
10038 medium *
`))
	if err != nil {
		t.Fatalf("readIgnoreList: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}
	if rules[0].justification != "justified acceptance" {
		t.Errorf("rule 0 justification = %q", rules[0].justification)
	}
	if rules[1].rule != "*" || rules[1].risk != "medium" {
		t.Errorf("rule 1 = %+v, want wildcard plugin + medium", rules[1])
	}
	if rules[2].urlRe != nil {
		t.Errorf("rule 2 urlRe = %v, want nil for '*'", rules[2].urlRe)
	}
}

func TestReadIgnoreList_BadRisk(t *testing.T) {
	_, err := readIgnoreList(mustWrite(t, "40012 critical http://localhost/.*\n"))
	if err == nil {
		t.Fatal("readIgnoreList() = nil, want error for unknown risk")
	}
}

func TestReadIgnoreList_BadRegex(t *testing.T) {
	_, err := readIgnoreList(mustWrite(t, "40012 high [unclosed\n"))
	if err == nil {
		t.Fatal("readIgnoreList() = nil, want error for bad regex")
	}
}

func TestMatchIgnore_AlertRefMatchesSubFinding(t *testing.T) {
	rules, err := readIgnoreList(mustWrite(t, "10055-6 medium .*\n"))
	if err != nil {
		t.Fatalf("readIgnoreList: %v", err)
	}
	alert := zapAlert{
		PluginID:  "10055",
		AlertRef:  "10055-6",
		RiskCode:  "2",
		Instances: []zapInstance{{URI: "http://localhost:7300/"}},
	}
	if _, ok := matchIgnore(rules, alert); !ok {
		t.Fatal("matchIgnore = false, want alertRef '10055-6' to match a '10055-6' rule")
	}

	// A different sub-finding under the same plugin must NOT match.
	alert.AlertRef = "10055-13"
	if _, ok := matchIgnore(rules, alert); ok {
		t.Fatal("matchIgnore = true, want '10055-13' to NOT match a '10055-6' rule")
	}
}

func TestIsGateRisk(t *testing.T) {
	cases := map[string]bool{
		"3": true, "2": true, "1": false, "0": false, "bogus": false,
	}
	for code, want := range cases {
		if got := isGateRisk(code); got != want {
			t.Errorf("isGateRisk(%q) = %v, want %v", code, got, want)
		}
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := loadConfig(func(string) string { return "" })
	if cfg != defaultConfig() {
		t.Errorf("loadConfig() with no overrides = %+v, want %+v", cfg, defaultConfig())
	}
}

func mustWrite(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dast.ignore")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// Issue #1487: a blind scan (only the canary self-test missing) is the one
// failure the release battery may retry; any real app finding must not be.
func TestRun_BlindOnlyIsRetryable(t *testing.T) {
	report, ignore := writeConfigFiles(t, `{"site":[{"@name":"http://localhost:7301","alerts":[]}]}`, "# none\n")
	err := run(gateEnv(report, ignore))
	if !errors.Is(err, errScanBlind) {
		t.Fatalf("run() = %v, want errScanBlind (sole failure is the self-test)", err)
	}
}

func TestRun_BlindPlusAppFindingIsNotRetryable(t *testing.T) {
	// The canary self-test is missing AND the app has an unaccepted finding:
	// retrying could hide the real finding behind a lucky second scan.
	report, ignore := writeConfigFiles(t, `{"site":[{"@name":"http://localhost:7300","alerts":[
	  {"pluginid":"10038","alert":"CSP","riskcode":"2","instances":[{"uri":"http://localhost:7300/x"}]}]}]}`, "# none\n")
	err := run(gateEnv(report, ignore))
	if err == nil || errors.Is(err, errScanBlind) {
		t.Fatalf("run() = %v, want a non-retryable error", err)
	}
}

func TestRun_AppFindingWithSelfTestSeenIsNotRetryable(t *testing.T) {
	report, ignore := writeConfigFiles(t, sampleReport, "# none\n")
	err := run(gateEnv(report, ignore))
	if err == nil || errors.Is(err, errScanBlind) {
		t.Fatalf("run() = %v, want a non-retryable error", err)
	}
}

func TestMain_ExitCodes(t *testing.T) {
	orig := osExit
	t.Cleanup(func() { osExit = orig })
	cases := []struct {
		name   string
		report string
		want   int // -1 = osExit not called (gate passed)
	}{
		{"blind only exits 3", `{"site":[{"@name":"http://localhost:7301","alerts":[]}]}`, exitBlind},
		{"app finding exits 1", sampleReport, 1},
		{"clean passes", `{"site":[{"@name":"http://localhost:7301","alerts":[
		  {"pluginid":"40012","alert":"XSS","riskcode":"3","instances":[]}]}]}`, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, ignore := writeConfigFiles(t, tc.report, "# none\n")
			t.Setenv("ZAPGATE_REPORT", report)
			t.Setenv("ZAPGATE_IGNORE", ignore)
			got := -1
			osExit = func(code int) { got = code }
			main()
			if got != tc.want {
				t.Fatalf("exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// --- plugin 10062 (PII Disclosure) UUID false positive, issue #2391 ---

// pii10062Report builds a report whose app site carries one 10062 high finding
// with the given evidence and response body, plus the canary self-test alert so
// only the 10062 verdict decides the run.
func pii10062Report(evidence, responseBody string) string {
	return `{"site":[
 {"@name":"http://localhost:7300","alerts":[{"pluginid":"10062","alert":"PII Disclosure","riskcode":"3","riskdesc":"High (High)",
   "instances":[{"uri":"http://localhost:7300/api/v1/users/2fa/status","evidence":"` + evidence + `","responsebody":` + jsonString(responseBody) + `}]}]},
 {"@name":"http://localhost:7301","alerts":[{"pluginid":"40012","alert":"Cross Site Scripting (Reflected)","riskcode":"3","riskdesc":"High (Medium)",
   "instances":[{"uri":"http://localhost:7301/reflected?q=x"}]}]}
]}`
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestRun_PII10062EvidenceIsUUIDLastGroupIsAccepted(t *testing.T) {
	// The exact v1.6.0 finding: Maestro BIN 584111 read out of the last group of
	// the request ID echoed in a 401 body.
	body := `{"error":"unauthorized","request_id":"a0769ac2-74a0-4ba6-8869-584111881481"}`
	report, ignore := writeConfigFiles(t, pii10062Report("584111881481", body), "# no rules\n")
	out := captureStdout(t, func() {
		if err := run(gateEnv(report, ignore)); err != nil {
			t.Fatalf("run() = %v, want nil: the evidence is only a UUID's last group", err)
		}
	})
	if !strings.Contains(out, "[ACCEPT]") || !strings.Contains(out, "UUID") {
		t.Errorf("stdout %q lacks an ACCEPT line naming the UUID reason", out)
	}
}

func TestRun_PII10062RealCardNumberIsRejected(t *testing.T) {
	body := `{"card":"4111111111111111"}`
	report, ignore := writeConfigFiles(t, pii10062Report("4111111111111111", body), "# no rules\n")
	err := run(gateEnv(report, ignore))
	if err == nil || !strings.Contains(err.Error(), "10062") {
		t.Fatalf("run() = %v, want a 10062 failure for a free-standing card number", err)
	}
}

func TestRun_PII10062EvidenceAlsoOutsideUUIDIsRejected(t *testing.T) {
	// The same digits appear once inside a UUID and once on their own: the
	// stand-alone occurrence could be a real number, so the finding stands.
	body := `{"request_id":"a0769ac2-74a0-4ba6-8869-584111881481","note":"584111881481"}`
	report, ignore := writeConfigFiles(t, pii10062Report("584111881481", body), "# no rules\n")
	if err := run(gateEnv(report, ignore)); err == nil {
		t.Fatal("run() = nil, want failure: evidence also appears outside a UUID")
	}
}

func TestRun_PII10062WithoutResponseBodyIsRejected(t *testing.T) {
	// No response text to prove the UUID context: fail closed.
	report, ignore := writeConfigFiles(t, pii10062Report("584111881481", ""), "# no rules\n")
	if err := run(gateEnv(report, ignore)); err == nil {
		t.Fatal("run() = nil, want failure: nothing proves the evidence is UUID-embedded")
	}
}

func TestRun_PII10062UUIDRuleDoesNotCoverOtherPlugins(t *testing.T) {
	body := `{"request_id":"a0769ac2-74a0-4ba6-8869-584111881481"}`
	rep := strings.Replace(pii10062Report("584111881481", body), `"pluginid":"10062"`, `"pluginid":"10099"`, 1)
	report, ignore := writeConfigFiles(t, rep, "# no rules\n")
	if err := run(gateEnv(report, ignore)); err == nil {
		t.Fatal("run() = nil, want failure: only plugin 10062 gets the UUID exemption")
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestAcceptPIIUUID_FailsClosedOnDegenerateInput(t *testing.T) {
	uuid := "a0769ac2-74a0-4ba6-8869-584111881481"
	cases := map[string]zapAlert{
		"no instances": {PluginID: "10062"},
		"empty evidence": {PluginID: "10062", Instances: []zapInstance{
			{ResponseBody: uuid}}},
		"evidence absent from response": {PluginID: "10062", Instances: []zapInstance{
			{Evidence: "999999999999", ResponseBody: uuid}}},
		"evidence matches a non-final UUID group": {PluginID: "10062", Instances: []zapInstance{
			{Evidence: "74a0", ResponseBody: uuid}}},
		"second instance not UUID-embedded": {PluginID: "10062", Instances: []zapInstance{
			{Evidence: "584111881481", ResponseBody: uuid},
			{Evidence: "584111881481", ResponseBody: "584111881481"}}},
	}
	for name, a := range cases {
		if ok, _ := acceptPIIUUID(a); ok {
			t.Errorf("%s: accepted, want rejected", name)
		}
	}
	if ok, _ := acceptPIIUUID(zapAlert{PluginID: "10062", Instances: []zapInstance{
		{Evidence: "584111881481", ResponseHeader: "X-Request-ID: " + uuid}}}); !ok {
		t.Error("UUID in the response header (X-Request-ID) must be accepted")
	}
}
