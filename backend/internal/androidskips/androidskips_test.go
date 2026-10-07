package androidskips

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const suiteXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="x" tests="3" failures="0" skipped="1">
  <testcase name="passes" classname="a.B" time="1"/>
  <testcase name="skips" classname="a.B" time="0"><skipped/></testcase>
  <testcase name="fails" classname="a.C" time="1"><failure message="boom">trace</failure></testcase>
  <testcase name="errors" classname="a.C" time="1"><error message="boom"/></testcase>
</testsuite>`

func TestParseAllowlist(t *testing.T) {
	in := `# comment

a.B#skips | because arm64
  c.D#other   |   spaced reason
a.B#skips | duplicate
noreason#x
x.Y#z |
not a key | reason
`
	allow, findings := ParseAllowlist(strings.NewReader(in))
	if len(allow) != 2 || allow["a.B#skips"] != "because arm64" || allow["c.D#other"] != "spaced reason" {
		t.Fatalf("allow = %#v", allow)
	}
	joined := strings.Join(findings, "\n")
	for _, s := range []string{"duplicate entry a.B#skips", `"noreason#x" has no`, `"x.Y#z |" has no`, `"not a key" is not`} {
		if !strings.Contains(joined, s) {
			t.Errorf("missing finding %q in:\n%s", s, joined)
		}
	}
	if len(findings) != 4 {
		t.Errorf("findings = %d, want 4:\n%s", len(findings), joined)
	}
}

func TestParseResults(t *testing.T) {
	res, err := ParseResults(map[string][]byte{"one.xml": []byte(suiteXML)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 || res.Passed != 1 || res.Failed != 2 || len(res.Skipped) != 1 || !res.Skipped["a.B#skips"] {
		t.Fatalf("res = %+v", res)
	}
	if !res.Seen["a.C#fails"] || !res.Seen["a.B#passes"] {
		t.Fatalf("seen = %v", res.Seen)
	}
}

func TestParseResultsTestsuitesRootAndMerge(t *testing.T) {
	multi := `<testsuites><testsuite><testcase name="m" classname="p.Q"><skipped/></testcase></testsuite>
<testsuite><testcase name="n" classname="p.Q"/></testsuite></testsuites>`
	res, err := ParseResults(map[string][]byte{"a.xml": []byte(multi), "b.xml": []byte(suiteXML)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 || !res.Skipped["p.Q#m"] || !res.Skipped["a.B#skips"] {
		t.Fatalf("res = %+v", res)
	}
}

func TestParseResultsRejectsNonJUnit(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":      "",
		"garbage":    "not xml at all",
		"wrongroot":  "<html/>",
		"truncated":  "<testsuite><testcase",
		"truncated2": "<testsuites><testsuite>",
	} {
		if _, err := ParseResults(map[string][]byte{name + ".xml": []byte(doc)}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCheck(t *testing.T) {
	allow := Allowlist{"a.B#skips": "r"}
	res, _ := ParseResults(map[string][]byte{"x.xml": []byte(suiteXML)})

	t.Run("exact match is clean", func(t *testing.T) {
		if f := Check(allow, res); len(f) != 0 {
			t.Fatalf("findings = %v", f)
		}
	})
	t.Run("new skip fails", func(t *testing.T) {
		f := Check(Allowlist{}, res)
		if len(f) != 1 || !strings.Contains(f[0], "UNEXPECTED SKIP: a.B#skips") {
			t.Fatalf("findings = %v", f)
		}
	})
	t.Run("listed test that ran fails", func(t *testing.T) {
		f := Check(Allowlist{"a.B#skips": "r", "a.B#passes": "r"}, res)
		if len(f) != 1 || !strings.Contains(f[0], "UNEXPECTED RUN: a.B#passes") {
			t.Fatalf("findings = %v", f)
		}
	})
	t.Run("listed test that is absent fails", func(t *testing.T) {
		f := Check(Allowlist{"a.B#skips": "r", "gone.Z#nope": "r"}, res)
		if len(f) != 1 || !strings.Contains(f[0], "STALE ALLOWLIST ENTRY: gone.Z#nope") {
			t.Fatalf("findings = %v", f)
		}
	})
	t.Run("empty run fails", func(t *testing.T) {
		f := Check(allow, &Results{})
		if len(f) != 1 || !strings.Contains(f[0], "executed nothing") {
			t.Fatalf("findings = %v", f)
		}
	})
}

func TestSummary(t *testing.T) {
	res, _ := ParseResults(map[string][]byte{"x.xml": []byte(suiteXML)})
	ok := Summary(Allowlist{"a.B#skips": "arm64 | only"}, res, nil)
	for _, s := range []string{"4 test(s): 1 passed, 2 failed, 1 skipped (1 expected", "`a.B#skips` | arm64 \\| only", "match the allowlist exactly"} {
		if !strings.Contains(ok, s) {
			t.Errorf("summary missing %q:\n%s", s, ok)
		}
	}
	bad := Summary(Allowlist{}, res, []string{"UNEXPECTED SKIP: a.B#skips"})
	for _, s := range []string{"**NOT ALLOWLISTED**", "Skip gate failed", "- UNEXPECTED SKIP"} {
		if !strings.Contains(bad, s) {
			t.Errorf("summary missing %q:\n%s", s, bad)
		}
	}
	none := Summary(Allowlist{}, &Results{Skipped: map[string]bool{}}, nil)
	if strings.Contains(none, "| Skipped test |") {
		t.Errorf("no table expected when nothing skipped:\n%s", none)
	}
}

func TestCheckSources(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("kotlin/p/q/Has.kt", "class Has { @Test fun realTest() = runBlocking { } }")
	write("p/q/Flat.kt", "class Flat { fun flatTest ( ) {} }")
	allow := Allowlist{
		"p.q.Has#realTest":  "r",
		"p.q.Flat#flatTest": "r",
		"p.q.Has#renamed":   "r",
		"p.q.Missing#x":     "r",
	}
	f := CheckSources(allow, root)
	joined := strings.Join(f, "\n")
	if len(f) != 2 || !strings.Contains(joined, "p.q.Has#renamed") || !strings.Contains(joined, "p.q.Missing#x") {
		t.Fatalf("findings = %v", f)
	}
	if f := CheckSources(Allowlist{"p.q.Has#realTest": "r"}, root); len(f) != 0 {
		t.Fatalf("clean case findings = %v", f)
	}
}
