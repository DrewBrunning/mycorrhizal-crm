package main

import (
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
