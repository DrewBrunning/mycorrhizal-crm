package omitemptyguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanFile_FindsUnallowlistedSliceOmitempty(t *testing.T) {
	src := `package x
type Resp struct {
	Notes []string ` + "`json:\"notes,omitempty\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 1 || findings[0].StructKey != "Resp.Notes" {
		t.Fatalf("findings = %+v, want one finding for Resp.Notes", findings)
	}
}

func TestScanFile_FindsUnallowlistedMapOmitempty(t *testing.T) {
	src := `package x
type Resp struct {
	Meta map[string]string ` + "`json:\"meta,omitempty\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 1 || findings[0].StructKey != "Resp.Meta" {
		t.Fatalf("findings = %+v, want one finding for Resp.Meta", findings)
	}
}

func TestScanFile_AllowlistedFieldWithReasonIsNotAFinding(t *testing.T) {
	src := `package x
type Resp struct {
	Notes []string ` + "`json:\"notes,omitempty\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{"Resp.Notes": "request-only DTO"})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (allowlisted)", findings)
	}
}

// TestScanFile_AllowlistEntryWithEmptyReasonStillFindingIsAFinding proves an
// allowlist entry with a blank reason does not silently exempt a field —
// the whole point of recording a reason is that it must actually say
// something.
func TestScanFile_AllowlistEntryWithBlankReasonIsStillAFinding(t *testing.T) {
	src := `package x
type Resp struct {
	Notes []string ` + "`json:\"notes,omitempty\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{"Resp.Notes": "   "})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one finding (blank reason doesn't count)", findings)
	}
}

func TestScanFile_NonOmitemptyFieldIsNotAFinding(t *testing.T) {
	src := `package x
type Resp struct {
	Notes []string ` + "`json:\"notes\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (no omitempty)", findings)
	}
}

func TestScanFile_ScalarOmitemptyFieldIsNotAFinding(t *testing.T) {
	src := `package x
type Resp struct {
	Name string ` + "`json:\"name,omitempty\"`" + `
	Ptr  *string ` + "`json:\"ptr,omitempty\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (not a slice/map)", findings)
	}
}

func TestScanFile_JSONDashFieldIsNotAFinding(t *testing.T) {
	src := `package x
type Resp struct {
	Internal []string ` + "`json:\"-\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (json:\"-\")", findings)
	}
}

func TestScanFile_FixedSizeArrayIsNotAFinding(t *testing.T) {
	src := `package x
type Resp struct {
	Fixed [4]string ` + "`json:\"fixed,omitempty\"`" + `
}
`
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (fixed-size array, not a slice)", findings)
	}
}

func TestScanFile_UnterminatedTagQuoteIsNotAFinding(t *testing.T) {
	// extractTagValue's closing-quote search fails on a malformed tag with
	// no closing `"` -- jsonOmitempty must treat that as "no json tag"
	// rather than panicking on a bad slice index.
	src := "package x\ntype Resp struct {\n\tNotes []string `json:\"notes,omitempty`\n}\n"
	findings, err := ScanFile("x.go", src, map[string]string{})
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none for an unterminated tag", findings)
	}
}

func TestScanFile_UnparseableFileErrors(t *testing.T) {
	if _, err := ScanFile("x.go", "not valid go {{{", map[string]string{}); err == nil {
		t.Fatal("want a parse error")
	}
}

func TestScanDir_IgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte(`package x
type Resp struct {
	Notes []string `+"`json:\"notes,omitempty\"`"+`
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := ScanDir(dir, map[string]string{})
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none (test file excluded)", findings)
	}
}

func TestScanDir_MissingDirErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := ScanDir(dir, map[string]string{}); err == nil {
		t.Fatal("want an error for a missing directory")
	}
}

func TestScanDir_UnreadableFileErrors(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o644) //nolint:errcheck // best-effort cleanup so t.TempDir can remove it

	if _, err := ScanDir(dir, map[string]string{}); err == nil {
		t.Fatal("want an error reading an unreadable file")
	}
}

func TestScanDir_UnparseableFilePropagatesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("not valid go {{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ScanDir(dir, map[string]string{}); err == nil {
		t.Fatal("want ScanFile's parse error to propagate from ScanDir")
	}
}

func TestScanDirObserved_PropagatesScanDirError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := ScanDirObserved(dir); err == nil {
		t.Fatal("want ScanDir's error to propagate from ScanDirObserved")
	}
}

func TestScanDirObserved_CollectsAllowlistedAndNot(t *testing.T) {
	dir := t.TempDir()
	src := `package x
type Resp struct {
	Notes []string ` + "`json:\"notes,omitempty\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	observed, err := ScanDirObserved(dir)
	if err != nil {
		t.Fatalf("ScanDirObserved: %v", err)
	}
	if !observed["Resp.Notes"] {
		t.Fatalf("observed = %+v, want Resp.Notes present", observed)
	}
}

func TestFormatFinding_RendersPathLineAndTag(t *testing.T) {
	f := Finding{Path: "models/x.go", Line: 12, StructKey: "Resp.Notes", JSONTag: "notes,omitempty"}
	got := FormatFinding(f)
	for _, want := range []string{"models/x.go:12:", "Resp.Notes", `"notes,omitempty"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatFinding() = %q, missing %q", got, want)
		}
	}
}

func TestUnusedAllowlistEntries_FindsStaleEntry(t *testing.T) {
	allowlist := map[string]string{"Gone.Field": "reason", "Still.Here": "reason"}
	observed := map[string]bool{"Still.Here": true}
	unused := UnusedAllowlistEntries(allowlist, observed)
	if len(unused) != 1 || unused[0] != "Gone.Field" {
		t.Fatalf("unused = %v, want [Gone.Field]", unused)
	}
}

// TestRealModelsAndControllersPackages is the actual gate: every slice/map
// omitempty struct field in backend/models and backend/controllers must be
// allowlisted with a real reason. See allowlist.go for the reasons.
//
// A test proving this actually catches a broken input lives in
// omitemptyguard_integration_test.go (it can't corrupt the real repo
// files, so it constructs its own fixture instead — the same pattern
// cmd/releasegatecheck's tests use for the checkers in this repo).
func TestRealModelsAndControllersPackages(t *testing.T) {
	for _, dir := range []string{"../../models", "../../controllers"} {
		findings, err := ScanDir(dir, Allowlist)
		if err != nil {
			t.Fatalf("ScanDir(%s): %v", dir, err)
		}
		for _, f := range findings {
			t.Error(FormatFinding(f))
		}
	}
}

// TestAllowlistHasNoStaleEntries fails if an allowlist entry no longer
// matches any real field in models/controllers — the field was fixed,
// renamed, or removed, and the entry should have been deleted with it
// (mirroring docs/security/citation-drift.ignore's own drift check).
func TestAllowlistHasNoStaleEntries(t *testing.T) {
	observed := map[string]bool{}
	for _, dir := range []string{"../../models", "../../controllers"} {
		o, err := ScanDirObserved(dir)
		if err != nil {
			t.Fatalf("ScanDirObserved(%s): %v", dir, err)
		}
		for k := range o {
			observed[k] = true
		}
	}
	for _, key := range UnusedAllowlistEntries(Allowlist, observed) {
		t.Errorf("internal/omitemptyguard.Allowlist[%q] no longer matches any field in models/controllers; remove the stale entry", key)
	}
}
