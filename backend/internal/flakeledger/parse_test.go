package flakeledger

import (
	"strings"
	"testing"
)

func TestParseRerunReport(t *testing.T) {
	in := "mycorrhizal/controllers.TestFlaky: 2 runs, 1 failures\n" +
		"mycorrhizal/services.TestBroken: 2 runs, 2 failures\n" +
		"\n" +
		"no separator here\n" +
		"pkg.TestZero: 1 runs, 0 failures\n" +
		"pkg.TestGarbage: many runs, some failures\n"
	obs, err := ParseRerunReport(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("want 2 entries, got %v", obs)
	}
	if obs["mycorrhizal/controllers.TestFlaky"] != PassedOnRetry {
		t.Errorf("flaky: %v", obs)
	}
	if obs["mycorrhizal/services.TestBroken"] != Failed {
		t.Errorf("broken: %v", obs)
	}
}

func TestParseRerunReportScannerError(t *testing.T) {
	// A single line beyond the 1 MiB scanner ceiling surfaces as an error.
	if _, err := ParseRerunReport(strings.NewReader(strings.Repeat("x", 2<<20))); err == nil {
		t.Fatal("want error for oversized line")
	}
}

const junitFixture = `<?xml version="1.0"?>
<testsuites>
  <testsuite name="pkg">
    <testcase classname="pkg" name="TestPass" time="0.1"/>
    <testcase classname="pkg" name="TestFlaky"><failure message="boom"/></testcase>
    <testcase classname="pkg" name="TestFlaky"/>
    <testcase classname="pkg" name="TestDead"><failure/></testcase>
    <testcase classname="pkg" name="TestErr"><error/></testcase>
    <testcase classname="pkg" name="TestSkip"><skipped/></testcase>
    <testcase name="NoClass"/>
  </testsuite>
</testsuites>`

func TestParseJUnit(t *testing.T) {
	obs, err := ParseJUnit(strings.NewReader(junitFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := Observations{
		"pkg.TestPass":  Passed,
		"pkg.TestFlaky": PassedOnRetry,
		"pkg.TestDead":  Failed,
		"pkg.TestErr":   Failed,
		"NoClass":       Passed,
	}
	if len(obs) != len(want) {
		t.Fatalf("got %v want %v", obs, want)
	}
	for k, v := range want {
		if obs[k] != v {
			t.Errorf("%s: got %v want %v", k, obs[k], v)
		}
	}
}

func TestParseJUnitMalformed(t *testing.T) {
	if _, err := ParseJUnit(strings.NewReader("<testsuites><testcase name=")); err == nil {
		t.Fatal("want error")
	}
	if _, err := ParseJUnit(strings.NewReader(`<testcase name="a"><failure></testcase>`)); err == nil {
		t.Fatal("want decode error")
	}
}

func TestMergeAttempts(t *testing.T) {
	first := Observations{"A": Failed, "B": Failed, "C": Passed, "D": Failed}
	final := Observations{"A": Passed, "B": Failed, "C": Passed, "E": Passed}
	got := MergeAttempts(first, final)
	want := Observations{"A": PassedOnRetry, "B": Failed, "C": Passed, "D": Failed, "E": Passed}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %v want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
}

func TestParseSignals(t *testing.T) {
	suite, obs, err := ParseSignals(strings.NewReader(
		`{"suite":"go/floor","tests":[{"test":"pkg.TestX","outcome":"passed_on_retry"},{"test":"pkg.TestY","outcome":"failed"},{"test":"pkg.TestZ","outcome":"passed"}]}`))
	if err != nil || suite != "go/floor" {
		t.Fatal(suite, err)
	}
	if obs["pkg.TestX"] != PassedOnRetry || obs["pkg.TestY"] != Failed || obs["pkg.TestZ"] != Passed {
		t.Errorf("%v", obs)
	}
	for _, bad := range []string{
		`not json`,
		`{"tests":[]}`,
		`{"suite":"s","tests":[{"test":"t","outcome":"wat"}]}`,
		`{"suite":"s","tests":[{"test":"","outcome":"failed"}]}`,
	} {
		if _, _, err := ParseSignals(strings.NewReader(bad)); err == nil {
			t.Errorf("want error for %s", bad)
		}
	}
}

func TestOutcomeStringRoundTrip(t *testing.T) {
	for _, o := range []Outcome{Passed, PassedOnRetry, Failed} {
		got, ok := ParseOutcome(o.String())
		if !ok || got != o {
			t.Errorf("%v", o)
		}
	}
	if _, ok := ParseOutcome("x"); ok {
		t.Error("x should not parse")
	}
}

const pwFixture = `{"suites":[{"specs":[],"suites":[{"specs":[
 {"title":"ok","file":"a.spec.ts","tests":[{"projectName":"chromium","status":"expected"}]},
 {"title":"flaky one","file":"credentialStorage.spec.ts","tests":[{"projectName":"chromium","status":"flaky"}]},
 {"title":"dead","file":"b.spec.ts","tests":[{"status":"unexpected"}]},
 {"title":"skip","file":"c.spec.ts","tests":[{"status":"skipped"}]}
]}]}]}`

func TestParsePlaywright(t *testing.T) {
	obs, err := ParsePlaywright(strings.NewReader(pwFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := Observations{
		"a.spec.ts > ok [chromium]":                        Passed,
		"credentialStorage.spec.ts > flaky one [chromium]": PassedOnRetry,
		"b.spec.ts > dead":                                 Failed,
	}
	if len(obs) != len(want) {
		t.Fatalf("got %v", obs)
	}
	for k, v := range want {
		if obs[k] != v {
			t.Errorf("%s: %v", k, obs[k])
		}
	}
	if _, err := ParsePlaywright(strings.NewReader("{")); err == nil {
		t.Error("want error")
	}
}

func TestObservationsMergeKeepsWorst(t *testing.T) {
	a := Observations{"x": Failed, "y": Passed}
	a.Merge(Observations{"x": Passed, "y": PassedOnRetry, "z": Passed})
	if a["x"] != Failed || a["y"] != PassedOnRetry || a["z"] != Passed {
		t.Errorf("%v", a)
	}
}
