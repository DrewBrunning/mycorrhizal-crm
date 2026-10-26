package agenteval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func summary(success bool, passed, total int) RunSummary {
	return RunSummary{Status: "passed", Success: success, CommandsPassed: passed, CommandsTotal: total}
}

func TestBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	b, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Profiles) != 0 {
		t.Fatalf("missing file baseline = %+v", b)
	}
	RecordBaseline(&b, "pinned", "database-migration", summary(true, 3, 3))
	RecordBaseline(&b, "pinned", "localization-change", summary(true, 2, 2))
	RecordBaseline(&b, "dry-run", "database-migration", summary(false, 1, 3))
	if err := WriteBaseline(path, b); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Profiles["pinned"]["database-migration"].Success {
		t.Fatalf("baseline = %+v", got)
	}
	if len(got.Profiles) != 2 || len(got.Profiles["pinned"]) != 2 {
		t.Fatalf("baseline = %+v", got)
	}
}

func TestLoadBaselineRejectsMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(path); err == nil || !strings.Contains(err.Error(), "parse baseline") {
		t.Fatalf("err = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(path); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompareBaseline(t *testing.T) {
	b := Baseline{Version: 1, Profiles: map[string]map[string]RunSummary{}}
	RecordBaseline(&b, "pinned", "demo", summary(true, 2, 3))

	if got := CompareBaseline(b, "pinned", "missing", summary(true, 0, 0)); got.Present || got.Regressed() {
		t.Fatalf("missing task = %+v", got)
	}
	if got := CompareBaseline(b, "other", "demo", summary(true, 0, 0)); got.Present || got.Regressed() {
		t.Fatalf("missing profile = %+v", got)
	}

	better := CompareBaseline(b, "pinned", "demo", summary(true, 3, 3))
	if better.Regressed() || !better.Present {
		t.Fatalf("better = %+v", better)
	}

	worse := CompareBaseline(b, "pinned", "demo", RunSummary{Status: "passed", Success: false, CommandsPassed: 1, CommandsTotal: 3, ScopeViolations: 1, HandEditedGenerated: 1})
	if !worse.Regressed() || len(worse.Regressions) != 4 {
		t.Fatalf("worse = %+v", worse)
	}
}

func TestSummarizeRun(t *testing.T) {
	run := &RunResult{
		Success:             true,
		SuiteCommands:       []CommandResult{{Passed: true}, {Passed: false}},
		ScopeViolations:     []string{"x"},
		HandEditedGenerated: []string{"y"},
	}
	got := SummarizeRun("passed", run, "hash")
	if got.Status != "passed" || !got.Success || got.CommandsPassed != 1 || got.CommandsTotal != 2 || got.ScopeViolations != 1 || got.HandEditedGenerated != 1 || got.ConfigHash != "hash" {
		t.Fatalf("summary = %+v", got)
	}
	if got := SummarizeRun("skipped", nil, "hash"); got.Status != "skipped" || got.Success {
		t.Fatalf("nil summary = %+v", got)
	}
}

func TestCompareBaselineDetectsConfigChangeAndFailures(t *testing.T) {
	b := Baseline{Version: 1, Profiles: map[string]map[string]RunSummary{}}
	base := RunSummary{Status: "passed", Success: true, CommandsPassed: 3, CommandsTotal: 3, ConfigHash: "before"}
	RecordBaseline(&b, "pinned", "demo", base)

	changed := RunSummary{Status: "passed", Success: true, CommandsPassed: 3, CommandsTotal: 3, ConfigHash: "after"}
	if got := CompareBaseline(b, "pinned", "demo", changed); !got.Regressed() || !strings.Contains(strings.Join(got.Regressions, "\n"), "configuration changed") {
		t.Fatalf("config change = %+v", got)
	}

	failedCommand := RunSummary{Status: "passed", Success: true, CommandsPassed: 3, CommandsTotal: 4, ConfigHash: "before"}
	if got := CompareBaseline(b, "pinned", "demo", failedCommand); !got.Regressed() || !strings.Contains(strings.Join(got.Regressions, "\n"), "suite command failures") {
		t.Fatalf("failure count = %+v", got)
	}
}

func TestCommittedBaselineLoads(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(filepath.Join(root, "testdata", "agent-eval", "baseline.json")); err != nil {
		t.Fatal(err)
	}
}

func TestConfigFingerprintChanges(t *testing.T) {
	profile := Profile{Command: "agent {prompt}", Name: "agent", Model: "m", Version: "1"}
	spec := Spec{BaseRef: "abc", Prompt: "do it", Suite: "demo"}
	suites := []Suite{{Name: "demo", Commands: []string{"true"}}}
	first := ConfigFingerprint(profile, spec, suites)
	if first == "" {
		t.Fatal("empty fingerprint")
	}
	spec.Prompt = "do something else"
	if second := ConfigFingerprint(profile, spec, suites); second == first {
		t.Fatal("fingerprint ignored the prompt")
	}
}
