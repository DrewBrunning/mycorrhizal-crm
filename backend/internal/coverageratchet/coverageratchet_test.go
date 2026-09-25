package coverageratchet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleProfile = `mode: atomic
mycorrhizal/internal/foo/bar.go:10.2,12.3 2 4
mycorrhizal/internal/foo/bar.go:14.2,14.10 1 0
mycorrhizal/internal/baz/qux.go:5.2,5.20 1 1
mycorrhizal/internal/baz/qux.go:7.2,7.20 3 0
`

func TestParseCoverprofile(t *testing.T) {
	blocks, err := ParseCoverprofile(strings.NewReader(sampleProfile))
	if err != nil {
		t.Fatalf("ParseCoverprofile: %v", err)
	}
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(blocks))
	}
	want := Block{File: "mycorrhizal/internal/foo/bar.go", StartLine: 10, EndLine: 12, NumStmt: 2, Count: 4}
	if blocks[0] != want {
		t.Errorf("blocks[0] = %+v, want %+v", blocks[0], want)
	}
	if blocks[1].Count != 0 || blocks[1].StartLine != 14 {
		t.Errorf("blocks[1] = %+v", blocks[1])
	}
}

func TestParseCoverprofile_MissingModeLine(t *testing.T) {
	_, err := ParseCoverprofile(strings.NewReader("not a mode line\n"))
	if err == nil {
		t.Fatal("expected an error for a missing mode: line")
	}
}

func TestParseCoverprofile_Empty(t *testing.T) {
	_, err := ParseCoverprofile(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected an error for an empty profile")
	}
}

func TestParseCoverprofile_MalformedLine(t *testing.T) {
	_, err := ParseCoverprofile(strings.NewReader("mode: atomic\nthis is not a valid block line\n"))
	if err == nil {
		t.Fatal("expected an error for a malformed block line")
	}
}

func TestParseCoverprofile_BlankLineSkipped(t *testing.T) {
	blocks, err := ParseCoverprofile(strings.NewReader("mode: atomic\n\nmycorrhizal/internal/foo/bar.go:10.2,12.3 2 4\n"))
	if err != nil {
		t.Fatalf("ParseCoverprofile: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
}

func TestPerFileStats(t *testing.T) {
	blocks, err := ParseCoverprofile(strings.NewReader(sampleProfile))
	if err != nil {
		t.Fatalf("ParseCoverprofile: %v", err)
	}
	stats := PerFileStats(blocks)

	bar := stats["mycorrhizal/internal/foo/bar.go"]
	if bar.Statements != 3 || bar.Covered != 2 {
		t.Errorf("bar.go stats = %+v, want {3 2}", bar)
	}
	if got := bar.Percent(); got < 66.6 || got > 66.7 {
		t.Errorf("bar.go Percent() = %v, want ~66.67", got)
	}

	qux := stats["mycorrhizal/internal/baz/qux.go"]
	if qux.Statements != 4 || qux.Covered != 1 {
		t.Errorf("qux.go stats = %+v, want {4 1}", qux)
	}
}

func TestFileStat_PercentOfZeroStatementsIs100(t *testing.T) {
	var f FileStat
	if got := f.Percent(); got != 100 {
		t.Errorf("Percent() of empty FileStat = %v, want 100", got)
	}
}

func TestToRelative(t *testing.T) {
	stats := map[string]FileStat{
		"mycorrhizal/internal/foo/bar.go": {Statements: 3, Covered: 2},
	}
	rel := ToRelative(stats, "mycorrhizal/")
	if _, ok := rel["internal/foo/bar.go"]; !ok {
		t.Fatalf("ToRelative did not strip the module prefix: %+v", rel)
	}
}

func TestFilterPragma_ExcludesMarkedBlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "foo"), 0o750); err != nil {
		t.Fatal(err)
	}
	src := `package foo

func A() { // marked below
	x := 1 // # pragma: no cover — unreachable in practice
	_ = x
}

func B() {
	y := 2
	_ = y
}
`
	if err := os.WriteFile(filepath.Join(dir, "internal", "foo", "bar.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	blocks := []Block{
		{File: "mycorrhizal/internal/foo/bar.go", StartLine: 3, EndLine: 5, NumStmt: 2, Count: 0},  // covers the marked line 4
		{File: "mycorrhizal/internal/foo/bar.go", StartLine: 8, EndLine: 10, NumStmt: 2, Count: 5}, // unmarked
	}

	kept, err := FilterPragma(blocks, dir, "mycorrhizal/")
	if err != nil {
		t.Fatalf("FilterPragma: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("got %d kept blocks, want 1 (the marked block should be dropped): %+v", len(kept), kept)
	}
	if kept[0].StartLine != 8 {
		t.Errorf("kept the wrong block: %+v", kept[0])
	}
}

func TestFilterPragma_MissingSourceFileIsNotFatal(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		{File: "mycorrhizal/internal/gone/vanished.go", StartLine: 1, EndLine: 1, NumStmt: 1, Count: 1},
	}
	kept, err := FilterPragma(blocks, dir, "mycorrhizal/")
	if err != nil {
		t.Fatalf("FilterPragma should tolerate a missing source file, got: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("a missing source file should exclude nothing, got %d kept", len(kept))
	}
}

func TestBuildBaseline_KeepsExistingTolerance(t *testing.T) {
	stats := map[string]FileStat{
		"internal/foo/bar.go": {Statements: 4, Covered: 3},
	}
	existing := &Baseline{TolerancePct: 2.5}
	b := BuildBaseline(stats, existing, 1.0)
	if b.TolerancePct != 2.5 {
		t.Errorf("TolerancePct = %v, want 2.5 (kept from existing)", b.TolerancePct)
	}
	if b.Files["internal/foo/bar.go"] != 75 {
		t.Errorf("Files[...] = %v, want 75", b.Files["internal/foo/bar.go"])
	}
}

func TestBuildBaseline_DefaultToleranceWhenNoExisting(t *testing.T) {
	b := BuildBaseline(map[string]FileStat{}, nil, 1.75)
	if b.TolerancePct != 1.75 {
		t.Errorf("TolerancePct = %v, want 1.75", b.TolerancePct)
	}
}

func TestSaveAndLoadBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	b := Baseline{
		Comment:      "test",
		TolerancePct: 1.5,
		Files:        map[string]float64{"internal/foo/bar.go": 66.67},
	}
	if err := SaveBaseline(path, b); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	got, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got.TolerancePct != 1.5 || got.Files["internal/foo/bar.go"] != 66.67 {
		t.Errorf("round-tripped baseline = %+v", got)
	}
}

func TestLoadBaseline_MissingFile(t *testing.T) {
	_, err := LoadBaseline(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected an error reading a missing baseline")
	}
}

func TestCompare_PassesWhenEverythingHoldsOrImproves(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files: map[string]float64{
			"internal/foo/bar.go": 80,
		},
	}
	current := map[string]FileStat{
		"internal/foo/bar.go": {Statements: 10, Covered: 9}, // 90%, improved
	}
	report := Compare(baseline, current)
	if !report.OK {
		t.Errorf("expected OK, got findings: %v", report.Findings)
	}
}

func TestCompare_PassesOnDropWithinTolerance(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files: map[string]float64{
			"internal/foo/bar.go": 80,
		},
	}
	current := map[string]FileStat{
		// 79.5% -- a 0.5pt drop, within the 1.0pt tolerance.
		"internal/foo/bar.go": {Statements: 1000, Covered: 795},
	}
	report := Compare(baseline, current)
	if !report.OK {
		t.Errorf("expected OK (drop within tolerance), got findings: %v", report.Findings)
	}
}

func TestCompare_FailsOnDropPastTolerance(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files: map[string]float64{
			"internal/foo/bar.go": 80,
		},
	}
	current := map[string]FileStat{
		"internal/foo/bar.go": {Statements: 10, Covered: 5}, // 50%, a 30pt drop
	}
	report := Compare(baseline, current)
	if report.OK {
		t.Fatal("expected a failure for a 30pt coverage drop")
	}
	if len(report.DropFiles) != 1 || report.DropFiles[0] != "internal/foo/bar.go" {
		t.Errorf("DropFiles = %v", report.DropFiles)
	}
	if len(report.Findings) != 1 || !strings.Contains(report.Findings[0], "internal/foo/bar.go") {
		t.Errorf("Findings = %v", report.Findings)
	}
}

func TestCompare_NewFileNotGated(t *testing.T) {
	baseline := Baseline{TolerancePct: 1.0, Files: map[string]float64{}}
	current := map[string]FileStat{
		"internal/foo/new.go": {Statements: 10, Covered: 0},
	}
	report := Compare(baseline, current)
	if !report.OK {
		t.Errorf("a brand-new file must not be gated by this ratchet, got findings: %v", report.Findings)
	}
	if len(report.NewFiles) != 1 || report.NewFiles[0] != "internal/foo/new.go" {
		t.Errorf("NewFiles = %v", report.NewFiles)
	}
}

func TestCompare_RemovedFileNotFailed(t *testing.T) {
	baseline := Baseline{
		TolerancePct: 1.0,
		Files: map[string]float64{
			"internal/foo/deleted.go": 90,
		},
	}
	report := Compare(baseline, map[string]FileStat{})
	if !report.OK {
		t.Errorf("a removed/renamed file must not fail the ratchet, got findings: %v", report.Findings)
	}
	if len(report.GoneFiles) != 1 || report.GoneFiles[0] != "internal/foo/deleted.go" {
		t.Errorf("GoneFiles = %v", report.GoneFiles)
	}
}

func TestParseCoverprofile_LineTooLongIsAnError(t *testing.T) {
	// ParseCoverprofile sets its scanner's max token size to 1MiB; a line
	// past that makes Scan() fail with ErrTooLong, which must surface via
	// scanner.Err() rather than silently truncating the profile.
	huge := "mode: atomic\n# " + strings.Repeat("x", 2*1024*1024) + "\n"
	_, err := ParseCoverprofile(strings.NewReader(huge))
	if err == nil {
		t.Fatal("expected an error for an oversized coverprofile line")
	}
}

func TestFilterPragma_PropagatesSourceScanError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "foo"), 0o750); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", 128*1024)
	src := "package foo\n\nvar _ = \"" + huge + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "internal", "foo", "bar.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	blocks := []Block{{File: "mycorrhizal/internal/foo/bar.go", StartLine: 3, EndLine: 3, NumStmt: 1, Count: 0}}
	_, err := FilterPragma(blocks, dir, "mycorrhizal/")
	if err == nil {
		t.Fatal("expected FilterPragma to propagate the underlying scan error")
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

func TestCompare_OneStatementOfASmallFileIsWithinTolerance(t *testing.T) {
	baseline := Baseline{TolerancePct: 1.5, Files: map[string]float64{"cmd/tiny/main.go": 100}}
	// 20 statements: losing one is a 5pt drop -- noise, not a lost test.
	one := map[string]FileStat{"cmd/tiny/main.go": {Statements: 20, Covered: 19}}
	if r := Compare(baseline, one); !r.OK {
		t.Fatalf("one statement of 20 should pass, got %v", r.Findings)
	}
	two := map[string]FileStat{"cmd/tiny/main.go": {Statements: 20, Covered: 18}}
	if r := Compare(baseline, two); r.OK {
		t.Fatal("two statements of 20 should still fail")
	}
}

func TestUnitTolerance(t *testing.T) {
	if got := unitTolerance(1.5, 1000); got != 1.5 {
		t.Errorf("large file: got %v, want the pt tolerance", got)
	}
	if got := unitTolerance(1.5, 20); got < 5 || got > 5.001 {
		t.Errorf("20 statements: got %v, want ~5", got)
	}
	if got := unitTolerance(1.5, 0); got != 1.5 {
		t.Errorf("no statements: got %v, want the pt tolerance", got)
	}
}
