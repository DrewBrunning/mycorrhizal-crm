package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLoadConfig_EnvOverrides(t *testing.T) {
	env := map[string]string{
		"ZAPGATE_REPORT": "r.json", "ZAPGATE_IGNORE": "i.ignore", "ZAPGATE_APP_HOST": "app:1",
		"ZAPGATE_CANARY_HOST": "canary:2", "ZAPGATE_SELFTEST_RULE": "999",
	}
	cfg := loadConfig(func(k string) string { return env[k] })
	want := config{reportPath: "r.json", ignorePath: "i.ignore", appHost: "app:1", canaryHost: "canary:2", selfTestRule: "999"}
	if cfg != want {
		t.Errorf("loadConfig() = %+v, want %+v", cfg, want)
	}
}

func TestRun_UnreadableInputs(t *testing.T) {
	good, ignore := writeConfigFiles(t, sampleReport, "# none\n")
	missing := filepath.Join(t.TempDir(), "nope")
	if err := run(gateEnv(missing, ignore)); err == nil || !strings.Contains(err.Error(), "read report") {
		t.Errorf("missing report: err = %v", err)
	}
	if err := run(gateEnv(good, missing)); err == nil || !strings.Contains(err.Error(), "read ignore-list") {
		t.Errorf("missing ignore list: err = %v", err)
	}
	bad := writeTemp(t, "bad.json", "{not json")
	if _, err := readReport(bad); err == nil {
		t.Error("readReport on malformed JSON = nil error")
	}
}

func TestReadIgnoreList_TooFewFields(t *testing.T) {
	if _, err := readIgnoreList(mustWrite(t, "40012 high\n")); err == nil {
		t.Error("two-field rule must be rejected")
	}
}

func TestReadIgnoreList_Missing(t *testing.T) {
	if _, err := readIgnoreList(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing ignore list must error")
	}
}

func TestMatchIgnore_URLRegexAndWildcards(t *testing.T) {
	alert := zapAlert{PluginID: "1", RiskCode: "2", Instances: []zapInstance{{URI: "http://x/a"}}}
	if _, ok := matchIgnore([]ignoreRule{{rule: "*", risk: "*"}}, alert); !ok {
		t.Error("wildcard rule with no URL regex must accept")
	}
	if _, ok := matchIgnore([]ignoreRule{{rule: "1", risk: "medium", urlRe: regexp.MustCompile("^zzz")}}, alert); ok {
		t.Error("non-matching URL regex must not accept")
	}
	if _, ok := matchIgnore([]ignoreRule{{rule: "1", risk: "high"}}, alert); ok {
		t.Error("risk mismatch must not accept")
	}
}

func TestDescribe_NoInstance(t *testing.T) {
	got := describe(zapAlert{PluginID: "1", Alert: "A", RiskCode: "3"}, "site")
	if !strings.Contains(got, "(no instance)") {
		t.Errorf("describe = %q", got)
	}
}
