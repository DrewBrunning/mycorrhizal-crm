package flakeledger

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/ledger.golden.md")

var now = time.Date(2026, 10, 6, 6, 0, 0, 0, time.UTC)

func day(n int) string { return now.AddDate(0, 0, -n).Format(time.RFC3339) }

// synthetic returns 6 runs: TestFlaky passes on retry in 4 of them, TestRare in
// 1, TestDead fails every attempt once, and one run is outside the window.
func synthetic() []RunData {
	var runs []RunData
	for i := 0; i < 6; i++ {
		obs := Observations{"pkg.TestStable": Passed, "pkg.TestFlaky": Passed, "pkg.TestRare": Passed}
		if i < 4 {
			obs["pkg.TestFlaky"] = PassedOnRetry
		}
		if i == 0 {
			obs["pkg.TestRare"] = PassedOnRetry
		}
		if i == 5 {
			obs["pkg.TestDead"] = Failed
		}
		runs = append(runs, RunData{
			Run:    Run{Workflow: "unit-tests.yml", RunID: int64(100 + i), URL: fmt.Sprintf("https://example.test/runs/%d", 100+i), CreatedAt: day(i)},
			Suites: map[string]Observations{"go/core": obs},
		})
	}
	runs = append(runs, RunData{
		Run:    Run{RunID: 1, URL: "https://example.test/runs/1", CreatedAt: day(30)},
		Suites: map[string]Observations{"go/core": {"pkg.TestFlaky": PassedOnRetry}},
	}, RunData{
		Run:    Run{RunID: 2, URL: "https://example.test/runs/2", CreatedAt: "garbage"},
		Suites: map[string]Observations{"go/core": {"pkg.TestFlaky": PassedOnRetry}},
	}, RunData{
		Run:    Run{RunID: 3, URL: "https://example.test/runs/3", CreatedAt: day(-3)}, // future
		Suites: map[string]Observations{"go/core": {"pkg.TestFlaky": PassedOnRetry}},
	}, RunData{
		Run: Run{RunID: 4, CreatedAt: day(1)}, // in window, no artifacts
	})
	return runs
}

func TestBuildCountsAndOrder(t *testing.T) {
	l := Build(synthetic(), now, 14)
	if l.Runs != 6 || l.Tests != 4 {
		t.Fatalf("runs=%d tests=%d", l.Runs, l.Tests)
	}
	if len(l.Rows) != 3 {
		t.Fatalf("rows: %+v", l.Rows)
	}
	// TestDead failed in its only run (100%); TestFlaky 4/6 = 67%; TestRare 1/6 = 17%.
	if l.Rows[0].Test != "pkg.TestDead" || l.Rows[0].Failed != 1 || l.Rows[0].FailedFirstAttempt() != 1 {
		t.Errorf("top row: %+v", l.Rows[0])
	}
	if l.Rows[1].Test != "pkg.TestFlaky" || l.Rows[1].PassedOnRetry != 4 || l.Rows[1].Runs != 6 || l.Rows[2].Test != "pkg.TestRare" {
		t.Errorf("order: %+v", l.Rows)
	}
	// Equal rates: more passed-on-retry first, then suite/test name.
	tied := []Stats{
		{Suite: "b", Test: "t", Runs: 2, PassedOnRetry: 1},
		{Suite: "a", Test: "t", Runs: 2, PassedOnRetry: 1},
		{Suite: "c", Test: "t", Runs: 4, PassedOnRetry: 2},
		{Suite: "a", Test: "s", Runs: 2, Failed: 1},
		{Suite: "a", Test: "a", Runs: 2, PassedOnRetry: 1},
	}
	sortStats(tied)
	if tied[0].Suite != "c" || tied[1].Test != "a" || tied[2].Test != "t" || tied[2].Suite != "a" || tied[3].Suite != "b" || tied[4].Test != "s" {
		t.Errorf("tie-break order: %+v", tied)
	}
}

func TestFlakeRateZeroRuns(t *testing.T) {
	if (Stats{}).FlakeRate() != 0 {
		t.Fatal("zero runs must not divide")
	}
}

func TestCandidatesThreshold(t *testing.T) {
	l := Build(synthetic(), now, 14)
	cs := Candidates(l, 3, nil)
	if len(cs) != 1 || cs[0].Test != "pkg.TestFlaky" || cs[0].PassedOnRetry != 4 {
		t.Fatalf("%+v", cs)
	}
	if cs[0].Title != "Flaky test: go/core pkg.TestFlaky" || len(cs[0].RunLinks) != 4 {
		t.Errorf("%+v", cs[0])
	}
	// Exactly-at-threshold counts; one above does not.
	if got := Candidates(l, 4, nil); len(got) != 1 {
		t.Errorf("threshold 4: %v", got)
	}
	if got := Candidates(l, 5, nil); len(got) != 0 {
		t.Errorf("threshold 5: %v", got)
	}
}

func TestCandidatesRespectClosedIssue(t *testing.T) {
	l := Build(synthetic(), now, 14)
	title := IssueTitle("go/core", "pkg.TestFlaky")
	// Runs 0..3 are 0..3 days old. Closing 1.5 days ago leaves only runs 0 and 1
	// (newer than the close) = 2 events < 3: no re-open.
	closedAt := now.AddDate(0, 0, -2).Add(12 * time.Hour)
	if got := Candidates(l, 3, map[string]time.Time{title: closedAt}); len(got) != 0 {
		t.Fatalf("closed issue must suppress: %+v", got)
	}
	// An event exactly at the close time predates the close: not counted
	// (runs 0..3 sit at now, now-1d, now-2d, now-3d; close at now-2d leaves 2).
	if got := Candidates(l, 3, map[string]time.Time{title: now.AddDate(0, 0, -2)}); len(got) != 0 {
		t.Fatalf("event at the close instant must not count: %+v", got)
	}
	// Closed long ago: everything counts again.
	if got := Candidates(l, 3, map[string]time.Time{title: now.AddDate(0, 0, -20)}); len(got) != 1 {
		t.Fatalf("old close must not suppress: %+v", got)
	}
	// A close with an unparseable hit time is conservative: that hit is not counted.
	for i := range l.Rows {
		if l.Rows[i].Test == "pkg.TestFlaky" {
			l.Rows[i].Hits[0].Time = "garbage"
		}
	}
	if got := Candidates(l, 4, map[string]time.Time{title: now.AddDate(0, 0, -20)}); len(got) != 0 {
		t.Fatalf("unparseable hit time under a close must not count: %+v", got)
	}
}

func TestCandidatesLinkCapAndFailedCount(t *testing.T) {
	var hits []Hit
	for i := 0; i < MaxHitLinks+5; i++ {
		hits = append(hits, Hit{URL: fmt.Sprintf("u%d", i), Time: day(1), Outcome: PassedOnRetry})
	}
	hits = append(hits, Hit{URL: "f", Time: day(1), Outcome: Failed})
	l := Ledger{Rows: []Stats{{Suite: "s", Test: "t", Runs: 30, PassedOnRetry: MaxHitLinks + 5, Failed: 1, Hits: hits}}}
	cs := Candidates(l, 3, nil)
	if len(cs) != 1 || len(cs[0].RunLinks) != MaxHitLinks || cs[0].Failed != 1 {
		t.Fatalf("%+v", cs)
	}
}

func TestIssueTitleTruncates(t *testing.T) {
	if got := IssueTitle("s", strings.Repeat("x", 500)); len(got) != 200 {
		t.Fatalf("len %d", len(got))
	}
}

func TestRenderGolden(t *testing.T) {
	got := Render(Build(synthetic(), now, 14), 3)
	path := filepath.Join("testdata", "ledger.golden.md")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("rendered ledger drifted from %s (run go test ./internal/flakeledger -update):\n%s", path, got)
	}
}

func TestRenderEmptyAndTruncated(t *testing.T) {
	if out := Render(Ledger{Now: now, WindowDays: 14}, 3); !strings.Contains(out, "No test failed a first attempt") {
		t.Errorf("%s", out)
	}
	var rows []Stats
	for i := 0; i < MaxRenderedRows+7; i++ {
		rows = append(rows, Stats{Suite: "s|x", Test: fmt.Sprintf("t%d", i), Runs: 1, PassedOnRetry: 1})
	}
	out := Render(Ledger{Now: now, WindowDays: 14, Rows: rows}, 3)
	if !strings.Contains(out, "7 more row(s) omitted") || !strings.Contains(out, `s\|x`) {
		t.Errorf("truncation/escape missing")
	}
}

func TestLoadRunsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runs := []Run{{Workflow: "unit-tests.yml", RunID: 7, URL: "u7", CreatedAt: day(1)}, {RunID: 8, URL: "u8", CreatedAt: day(2)}}
	b, _ := json.Marshal(runs)
	write("runs.json", string(b))
	// Go leg: junit says the test failed (attempt log), rerun report says it
	// passed on retry; the rerun report must win.
	write("7/flake-go-core/junit-core.xml", `<testsuites><testcase classname="p" name="TestA"><failure/></testcase><testcase classname="p" name="TestB"/></testsuites>`)
	write("7/flake-go-core/rerun-fails-core.txt", "p.TestA: 2 runs, 1 failures\n")
	// Playwright.
	write("7/flake-playwright-e2e/results.json", pwFixture)
	// Android: attempt 1 failed T1, final run passes it.
	write("7/flake-android-api35/connected/final.xml", `<testsuite><testcase classname="a" name="T1"/><testcase classname="a" name="T2"/></testsuite>`)
	write("7/flake-android-api35/flaky-attempt-1/first.xml", `<testsuite><testcase classname="a" name="T1"><failure/></testcase></testsuite>`)
	// Neutral signals feed a suite of their own.
	write("7/flake-signals-floor/flake-signals-floor.json", `{"suite":"go/floor","tests":[{"test":"q.TestS","outcome":"passed_on_retry"}]}`)
	// Noise: non-flake artifact, corrupt files (warnings), run with no dir.
	write("7/other-artifact/junit.xml", `<testcase name="ignored"><failure/></testcase>`)
	write("7/flake-go-bad/junit-bad.xml", `<testsuites><testcase name=`)
	write("7/flake-go-bad/rerun-fails-bad.txt", "")
	write("7/flake-playwright-bad/results.json", `{`)
	write("7/flake-signals-bad/flake-signals-x.json", `{`)
	write("7/flake-go-empty/readme.md", "nothing parseable")
	write("7/flake-go-unreadable/rerun-fails-x.txt", "p.T: 2 runs, 1 failures\n")
	write("7/flake-signals-unreadable/flake-signals-u.json", "{}")
	for _, rel := range []string{"7/flake-go-unreadable/rerun-fails-x.txt", "7/flake-signals-unreadable/flake-signals-u.json"} {
		if err := os.Chmod(filepath.Join(dir, rel), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "7", "flake-go-core", "dir.xml"), []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}

	got, warns, err := LoadRuns(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[1].Suites) != 0 {
		t.Fatalf("runs: %+v", got)
	}
	s := got[0].Suites
	if s["go/core"]["p.TestA"] != PassedOnRetry || s["go/core"]["p.TestB"] != Passed {
		t.Errorf("go/core: %v", s["go/core"])
	}
	if s["playwright/e2e"]["credentialStorage.spec.ts > flaky one [chromium]"] != PassedOnRetry {
		t.Errorf("playwright: %v", s["playwright/e2e"])
	}
	if s["android/api35"]["a.T1"] != PassedOnRetry || s["android/api35"]["a.T2"] != Passed {
		t.Errorf("android: %v", s["android/api35"])
	}
	if s["go/floor"]["q.TestS"] != PassedOnRetry {
		t.Errorf("signals: %v", s["go/floor"])
	}
	if _, ok := s["other/artifact"]; ok {
		t.Error("non flake-* artifact must be ignored")
	}
	if len(warns) < 4 {
		t.Errorf("want >=4 warnings for the corrupt files, got %v", warns)
	}
}

func TestLoadRunsErrors(t *testing.T) {
	if _, _, err := LoadRuns(t.TempDir()); err == nil {
		t.Error("missing runs.json must error")
	}
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "runs.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadRuns(d); err == nil {
		t.Error("bad runs.json must error")
	}
}

func TestSuiteOf(t *testing.T) {
	for in, want := range map[string]string{
		"flake-go-core":           "go/core",
		"flake-playwright-prod-d": "playwright/prod-d",
		"flake-lonely":            "lonely",
	} {
		if got := suiteOf(in); got != want {
			t.Errorf("%s: %s want %s", in, got, want)
		}
	}
}
