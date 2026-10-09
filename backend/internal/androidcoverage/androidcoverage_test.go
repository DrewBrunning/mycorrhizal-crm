package androidcoverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleXML mirrors the real aggregated report's shape, including the DOCTYPE
// the JaCoCo serializer emits, so parsing is exercised against what CI
// actually produces.
const sampleXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="mycorrhizal-android">
  <package name="com/mycorrhizal/crm/ui/components">
    <sourcefile name="Foo.kt">
      <line nr="1" mi="0" ci="3" mb="0" cb="0"/>
      <line nr="2" mi="0" ci="1" mb="0" cb="0"/>
      <line nr="3" mi="1" ci="0" mb="0" cb="0"/>
    </sourcefile>
    <sourcefile name="Bar.kt">
      <line nr="1" mi="0" ci="4" mb="0" cb="0"/>
    </sourcefile>
    <sourcefile name="Empty.kt"/>
  </package>
  <package name="com/mycorrhizal/crm/ui">
    <sourcefile name="Baz.kt">
      <line nr="1" mi="2" ci="2" mb="1" cb="0"/>
    </sourcefile>
  </package>
</report>`

func TestParse(t *testing.T) {
	stats, err := Parse(strings.NewReader(sampleXML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stats) != 4 {
		t.Fatalf("got %d files, want 4: %+v", len(stats), stats)
	}

	foo, ok := stats["com/mycorrhizal/crm/ui/components/Foo.kt"]
	if !ok {
		t.Fatalf("Foo.kt missing from stats: %+v", stats)
	}
	if foo.Lines != 5 || foo.Covered != 4 {
		t.Errorf("Foo.kt = %+v, want {5 4}", foo)
	}
	if got := foo.Percent(); got != 80 {
		t.Errorf("Foo.kt Percent() = %v, want 80", got)
	}

	bar := stats["com/mycorrhizal/crm/ui/components/Bar.kt"]
	if bar.Lines != 4 || bar.Covered != 4 || bar.Percent() != 100 {
		t.Errorf("Bar.kt = %+v (%.2f%%), want {4 4} 100%%", bar, bar.Percent())
	}

	baz := stats["com/mycorrhizal/crm/ui/Baz.kt"]
	if baz.Lines != 4 || baz.Covered != 2 || baz.Percent() != 50 {
		t.Errorf("Baz.kt = %+v (%.2f%%), want {4 2} 50%%", baz, baz.Percent())
	}
}

func TestParse_SourceFileWithNoLinesIs100(t *testing.T) {
	stats, err := Parse(strings.NewReader(sampleXML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	empty := stats["com/mycorrhizal/crm/ui/components/Empty.kt"]
	if empty.Lines != 0 || empty.Covered != 0 {
		t.Errorf("Empty.kt = %+v, want {0 0}", empty)
	}
	if got := empty.Percent(); got != 100 {
		t.Errorf("Empty.kt Percent() = %v, want 100 (nothing to fail on)", got)
	}
}

func TestParse_EmptyReportIsNotAnError(t *testing.T) {
	stats, err := Parse(strings.NewReader(`<report name="empty"></report>`))
	if err != nil {
		t.Fatalf("Parse of an empty report: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("got %d files, want 0", len(stats))
	}
}

func TestParse_MalformedXML(t *testing.T) {
	if _, err := Parse(strings.NewReader("not xml at all")); err == nil {
		t.Fatal("expected an error for malformed XML")
	}
}

func TestFileStat_PercentOfZeroUnitsIs100(t *testing.T) {
	var f FileStat
	if got := f.Percent(); got != 100 {
		t.Errorf("Percent() of empty FileStat = %v, want 100", got)
	}
}

func TestRound2_HalfUp(t *testing.T) {
	cases := map[float64]float64{
		66.665: 66.67, // exactly half -> up
		66.664: 66.66,
		0.005:  0.01,
		0.004:  0,
		100:    100,
	}
	for in, want := range cases {
		if got := round2(in); got != want {
			t.Errorf("round2(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestBuildBaseline_KeepsExistingTolerance(t *testing.T) {
	stats := map[string]FileStat{
		"com/mycorrhizal/crm/ui/components/Foo.kt": {Lines: 4, Covered: 3},
	}
	existing := &Baseline{TolerancePct: 2.5}
	b := BuildBaseline(stats, existing, 1.0)
	if b.TolerancePct != 2.5 {
		t.Errorf("TolerancePct = %v, want 2.5 (kept from existing)", b.TolerancePct)
	}
	if b.Files["com/mycorrhizal/crm/ui/components/Foo.kt"] != 75 {
		t.Errorf("Files[...] = %v, want 75", b.Files["com/mycorrhizal/crm/ui/components/Foo.kt"])
	}
	if b.Comment != baselineComment {
		t.Errorf("Comment not set to the generated-artifact comment")
	}
}

func TestBuildBaseline_DefaultToleranceWhenNoExisting(t *testing.T) {
	b := BuildBaseline(map[string]FileStat{}, nil, 1.75)
	if b.TolerancePct != 1.75 {
		t.Errorf("TolerancePct = %v, want 1.75", b.TolerancePct)
	}
}

func TestSaveAndLoadBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage-baseline.json")
	b := Baseline{
		Comment:      "test",
		TolerancePct: 1.5,
		Files:        map[string]float64{"com/mycorrhizal/crm/ui/components/Foo.kt": 66.67},
	}
	if err := SaveBaseline(path, b); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	got, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got.TolerancePct != 1.5 || got.Files["com/mycorrhizal/crm/ui/components/Foo.kt"] != 66.67 {
		t.Errorf("round-tripped baseline = %+v", got)
	}
}

func TestLoadBaseline_MissingFile(t *testing.T) {
	_, err := LoadBaseline(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected an error reading a missing baseline")
	}
}

func TestLoadBaseline_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBaseline(path)
	if err == nil {
		t.Fatal("expected an error loading a malformed baseline")
	}
}

func TestSaveBaseline_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()
	// Put a plain file where the baseline's parent directory needs to be
	// created, so os.MkdirAll fails.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SaveBaseline(filepath.Join(blocker, "sub", "baseline.json"), Baseline{Files: map[string]float64{}})
	if err == nil {
		t.Fatal("expected an error when SaveBaseline's directory can't be created")
	}
}

func TestSaveBaseline_WriteFileFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory at the exact path SaveBaseline wants to write to makes
	// os.WriteFile fail (it's a directory, not a writable file).
	target := filepath.Join(dir, "baseline.json")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatal(err)
	}
	err := SaveBaseline(target, Baseline{Files: map[string]float64{}})
	if err == nil {
		t.Fatal("expected an error when SaveBaseline's target path is a directory")
	}
}

func TestCompare_PassesWhenEverythingHoldsOrImproves(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files:        map[string]float64{"com/kit/Foo.kt": 80},
	}
	current := map[string]FileStat{
		"com/kit/Foo.kt": {Lines: 10, Covered: 9}, // 90%, improved
	}
	report := Compare(baseline, current)
	if !report.OK {
		t.Errorf("expected OK, got findings: %v", report.Findings)
	}
}

func TestCompare_PassesOnDropWithinTolerance(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files:        map[string]float64{"com/kit/Foo.kt": 80},
	}
	current := map[string]FileStat{
		// 79.5% -- a 0.5pt drop, within the 1.0pt tolerance.
		"com/kit/Foo.kt": {Lines: 1000, Covered: 795},
	}
	report := Compare(baseline, current)
	if !report.OK {
		t.Errorf("expected OK (drop within tolerance), got findings: %v", report.Findings)
	}
}

func TestCompare_FailsOnDropPastTolerance(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files:        map[string]float64{"com/kit/Foo.kt": 80},
	}
	current := map[string]FileStat{
		"com/kit/Foo.kt": {Lines: 10, Covered: 5}, // 50%, a 30pt drop
	}
	report := Compare(baseline, current)
	if report.OK {
		t.Fatal("expected a failure for a 30pt coverage drop")
	}
	if len(report.DropFiles) != 1 || report.DropFiles[0] != "com/kit/Foo.kt" {
		t.Errorf("DropFiles = %v", report.DropFiles)
	}
	if len(report.Findings) != 1 || !strings.Contains(report.Findings[0], "com/kit/Foo.kt") {
		t.Errorf("Findings = %v", report.Findings)
	}
}

func TestCompare_NewFileNotGated(t *testing.T) {
	baseline := Baseline{TolerancePct: 1.0, Files: map[string]float64{}}
	current := map[string]FileStat{
		"com/kit/New.kt": {Lines: 10, Covered: 0},
	}
	report := Compare(baseline, current)
	if !report.OK {
		t.Errorf("a brand-new file must not be gated by this ratchet, got findings: %v", report.Findings)
	}
	if len(report.NewFiles) != 1 || report.NewFiles[0] != "com/kit/New.kt" {
		t.Errorf("NewFiles = %v", report.NewFiles)
	}
}

func TestCompare_RemovedFileNotFailed(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files:        map[string]float64{"com/kit/Deleted.kt": 90},
	}
	report := Compare(baseline, map[string]FileStat{})
	if !report.OK {
		t.Errorf("a removed/renamed file must not fail the ratchet, got findings: %v", report.Findings)
	}
	if len(report.GoneFiles) != 1 || report.GoneFiles[0] != "com/kit/Deleted.kt" {
		t.Errorf("GoneFiles = %v", report.GoneFiles)
	}
}

func TestCompare_OneUnitOfASmallFileIsWithinTolerance(t *testing.T) {
	baseline := Baseline{TolerancePct: 1.5, Files: map[string]float64{"com/kit/tiny.kt": 100}}
	// 20 units: losing one is a 5pt drop -- noise, not a lost test.
	one := map[string]FileStat{"com/kit/tiny.kt": {Lines: 20, Covered: 19}}
	if r := Compare(baseline, one); !r.OK {
		t.Fatalf("one unit of 20 should pass, got %v", r.Findings)
	}
	two := map[string]FileStat{"com/kit/tiny.kt": {Lines: 20, Covered: 18}}
	if r := Compare(baseline, two); r.OK {
		t.Fatal("two units of 20 should still fail")
	}
}

func TestUnitTolerance(t *testing.T) {
	if got := unitTolerance(1.5, 1000); got != 1.5 {
		t.Errorf("large file: got %v, want the pt tolerance", got)
	}
	if got := unitTolerance(1.5, 20); got < 5 || got > 5.001 {
		t.Errorf("20 units: got %v, want ~5", got)
	}
	if got := unitTolerance(1.5, 0); got != 1.5 {
		t.Errorf("no units: got %v, want the pt tolerance", got)
	}
}
