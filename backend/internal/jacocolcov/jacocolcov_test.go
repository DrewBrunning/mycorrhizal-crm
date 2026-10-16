package jacocolcov

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

const reportXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="test">
  <package name="com/kit">
    <sourcefile name="Foo.kt">
      <line nr="3" mi="0" ci="2" mb="0" cb="0"/>
      <line nr="4" mi="2" ci="0" mb="2" cb="0"/>
      <line nr="5" mi="1" ci="3" mb="1" cb="1"/>
      <line nr="6" mi="1" ci="1" mb="0" cb="0"/>
    </sourcefile>
    <sourcefile name="Empty.kt"/>
    <sourcefile name="Bar_Factory.java">
      <line nr="1" mi="0" ci="1"/>
    </sourcefile>
    <sourcefile name="Dup.kt">
      <line nr="1" mi="0" ci="1"/>
    </sourcefile>
  </package>
  <package name="com/app">
    <sourcefile name="Main.kt">
      <line nr="10" mi="4" ci="0"/>
    </sourcefile>
  </package>
</report>`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"android/core/src/main/kotlin/com/kit/Foo.kt":   {},
		"android/core/src/main/kotlin/com/kit/Empty.kt": {},
		"android/a/src/main/kotlin/com/kit/Dup.kt":      {},
		"android/b/src/main/kotlin/com/kit/Dup.kt":      {},
		"android/app/src/main/kotlin/com/app/Main.kt":   {},
		"android/app/src/main/res/values/strings.xml":   {},
		"android/app/build/generated/com/app/Main.kt":   {}, // build output: must not make Main.kt ambiguous
		"android/.gradle/cache/com/kit/Foo.kt":          {},
		"android/app/src/main/java/com/app/Legacy.java": {},
	}
}

func convert(t *testing.T) (Result, string) {
	t.Helper()
	files, err := ParseReport(strings.NewReader(reportXML))
	if err != nil {
		t.Fatal(err)
	}
	index, err := IndexSources(testFS(), "android")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res, err := Convert(files, index, &out)
	if err != nil {
		t.Fatal(err)
	}
	return res, out.String()
}

func TestConvertWritesResolvedFilesSortedByPath(t *testing.T) {
	_, got := convert(t)
	want := `SF:android/app/src/main/kotlin/com/app/Main.kt
DA:10,0
LF:1
LH:0
end_of_record
SF:android/core/src/main/kotlin/com/kit/Foo.kt
DA:3,1
DA:4,0
DA:5,1
DA:6,1
BRDA:4,0,0,-
BRDA:4,0,1,-
BRDA:5,0,0,1
BRDA:5,0,1,0
LF:4
LH:3
BRF:4
BRH:1
end_of_record
`
	if got != want {
		t.Fatalf("lcov output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestConvertReconciles(t *testing.T) {
	res, _ := convert(t)
	if want := (Totals{Files: 5, Lines: 7, CoveredLines: 5, Branches: 4, CoveredBranches: 1}); res.JaCoCo != want {
		t.Errorf("JaCoCo totals = %+v, want %+v", res.JaCoCo, want)
	}
	if want := (Totals{Files: 2, Lines: 5, CoveredLines: 3, Branches: 4, CoveredBranches: 1}); res.LCOV != want {
		t.Errorf("lcov totals = %+v, want %+v", res.LCOV, want)
	}
	if got := strings.Join(res.Unresolved, ","); got != "com/kit/Bar_Factory.java" {
		t.Errorf("Unresolved = %q", got)
	}
	if got := strings.Join(res.NoLines, ","); got != "com/kit/Empty.kt" {
		t.Errorf("NoLines = %q", got)
	}
	if got := strings.Join(res.Ambiguous, ","); got != "com/kit/Dup.kt" {
		t.Errorf("Ambiguous = %q", got)
	}
	// Line 6 (mi=1, ci=1, no branches) is the only instruction partial; line 5
	// is a branch partial and stays partial in lcov via BRDA.
	if res.InstructionPartials != 1 {
		t.Errorf("InstructionPartials = %d, want 1", res.InstructionPartials)
	}
}

func TestIndexSourcesSkipsBuildOutputAndNonSource(t *testing.T) {
	index, err := IndexSources(testFS(), "android")
	if err != nil {
		t.Fatal(err)
	}
	if got := index["com/app/Main.kt"]; len(got) != 1 || got[0] != "android/app/src/main/kotlin/com/app/Main.kt" {
		t.Errorf("com/app/Main.kt -> %v, want only the src/main copy", got)
	}
	if got := index["com/kit/Foo.kt"]; len(got) != 1 {
		t.Errorf("com/kit/Foo.kt -> %v, want the .gradle copy skipped", got)
	}
	if got := index["com/app/Legacy.java"]; len(got) != 1 {
		t.Errorf(".java files must be indexed, got %v", got)
	}
	if _, ok := index["values/strings.xml"]; ok {
		t.Error("non-source files must not be indexed")
	}
}

func TestIndexSourcesMissingDir(t *testing.T) {
	if _, err := IndexSources(testFS(), "nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestParseReportRejectsMalformedXML(t *testing.T) {
	if _, err := ParseReport(strings.NewReader("<report><package")); err == nil {
		t.Fatal("want a parse error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestConvertReportsWriteError(t *testing.T) {
	files := []SourceFile{{Key: "com/app/Main.kt", Lines: []Line{{Nr: 1, CI: 1}}}}
	index := map[string][]string{"com/app/Main.kt": {"android/app/Main.kt"}}
	if _, err := Convert(files, index, failWriter{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want the write error", err)
	}
}

func TestSummary(t *testing.T) {
	res, _ := convert(t)
	s := res.Summary()
	for _, want := range []string{
		"| JaCoCo report | 5 | 7 | 5 | 71.43% | 4 | 1 | 25.00% |",
		"| lcov output | 2 | 5 | 3 | 60.00% | 4 | 1 | 25.00% |",
		"Dropped: 1 unresolved (no repository file: generated/inlined code), 1 with no lines, 1 ambiguous.",
		"Instruction-level partial lines counted as covered in lcov: 1.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q\n%s", want, s)
		}
	}
	if empty := (Result{}).Summary(); !strings.Contains(empty, "| 0 | 0 | 0 | n/a |") {
		t.Errorf("empty summary should render n/a percentages:\n%s", empty)
	}
}
