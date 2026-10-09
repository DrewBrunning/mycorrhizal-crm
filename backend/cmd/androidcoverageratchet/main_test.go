package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoFileXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="test">
  <package name="com/kit">
    <sourcefile name="Foo.kt">
      <line nr="1" mi="0" ci="2"/>
      <line nr="2" mi="1" ci="0"/>
    </sourcefile>
    <sourcefile name="Bar.kt">
      <line nr="1" mi="0" ci="3"/>
    </sourcefile>
  </package>
</report>`

func updateArgs(profile, baseline string) []string {
	return []string{"-profile", profile, "-baseline", baseline, "-update"}
}

func checkArgs(profile, baseline string) []string {
	return []string{"-profile", profile, "-baseline", baseline}
}

func TestRun_UpdateThenCheckRoundTrips(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	baseline := filepath.Join(dir, "coverage-baseline.json")

	var out bytes.Buffer
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "wrote new baseline") {
		t.Errorf("update output = %q", out.String())
	}
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("baseline was not written: %v", err)
	}

	out.Reset()
	if code := run(checkArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("check exited %d after a fresh -update, want 0: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "androidcoverageratchet: ok") {
		t.Errorf("check output = %q", out.String())
	}
}

func TestRun_DetectsRegression(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	baseline := filepath.Join(dir, "coverage-baseline.json")

	var out bytes.Buffer
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}

	// A regressed report: Foo.kt's previously-covered line now reads 0.
	regressed := `<?xml version="1.0" encoding="UTF-8"?>
<report name="test">
  <package name="com/kit">
    <sourcefile name="Foo.kt">
      <line nr="1" mi="2" ci="0"/>
      <line nr="2" mi="1" ci="0"/>
    </sourcefile>
    <sourcefile name="Bar.kt">
      <line nr="1" mi="0" ci="3"/>
    </sourcefile>
  </package>
</report>`
	regressedPath := writeFile(t, filepath.Join(dir, "regressed.xml"), regressed)

	out.Reset()
	code := run(checkArgs(regressedPath, baseline), &out, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for a regressed file, got %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "com/kit/Foo.kt") {
		t.Errorf("output did not name the regressed file: %q", out.String())
	}
}

func TestRun_NewFileNotGated(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	baseline := filepath.Join(dir, "coverage-baseline.json")

	var out bytes.Buffer
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}

	withNewFile := strings.Replace(twoFileXML,
		`  </package>`,
		`    <sourcefile name="New.kt"><line nr="1" mi="4" ci="0"/></sourcefile>
  </package>`, 1)
	newPath := writeFile(t, filepath.Join(dir, "new.xml"), withNewFile)

	out.Reset()
	if code := run(checkArgs(newPath, baseline), &out, &out); code != 0 {
		t.Fatalf("a brand-new uncovered file must not fail the ratchet, got exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "new (not gated here): com/kit/New.kt") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRun_GoneFileReportedNotFailed(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	baseline := filepath.Join(dir, "coverage-baseline.json")

	var out bytes.Buffer
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}

	// A follow-up report that drops Bar.kt entirely: it's gone from current
	// stats, so run must report it (not gate on it).
	withoutBar := `<?xml version="1.0" encoding="UTF-8"?>
<report name="test">
  <package name="com/kit">
    <sourcefile name="Foo.kt">
      <line nr="1" mi="0" ci="2"/>
      <line nr="2" mi="1" ci="0"/>
    </sourcefile>
  </package>
</report>`
	gonePath := writeFile(t, filepath.Join(dir, "gone.xml"), withoutBar)

	out.Reset()
	if code := run(checkArgs(gonePath, baseline), &out, &out); code != 0 {
		t.Fatalf("a gone file must not fail the ratchet, got exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "gone (baseline stale, run -update): com/kit/Bar.kt") {
		t.Errorf("output did not report the gone file: %q", out.String())
	}
}

func TestRun_UpdateTwiceReusesExistingBaseline(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	baseline := filepath.Join(dir, "coverage-baseline.json")

	var out bytes.Buffer
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("first -update exited %d: %s", code, out.String())
	}

	// Hand-edited tolerance must survive a second -update.
	setTolerance(t, baseline, 3.3)

	out.Reset()
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("second -update exited %d: %s", code, out.String())
	}
	if got := toleranceOf(t, baseline); got != 3.3 {
		t.Errorf("tolerance after second -update = %v, want 3.3", got)
	}
}

func TestRun_MissingProfileExitsTwo(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	code := run(checkArgs(filepath.Join(dir, "nope.xml"), filepath.Join(dir, "b.json")), &out, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 for a missing profile, got %d", code)
	}
}

func TestRun_MalformedProfileExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), "not xml\n")
	var out bytes.Buffer
	code := run(checkArgs(profile, filepath.Join(dir, "b.json")), &out, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 for a malformed profile, got %d", code)
	}
}

func TestRun_CheckWithoutBaselineExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	var out bytes.Buffer
	code := run(checkArgs(profile, filepath.Join(dir, "b.json")), &out, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 checking with no baseline present, got %d: %s", code, out.String())
	}
}

func TestRun_SaveBaselineFailureExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)

	// Put a plain file where the baseline's parent directory needs to be
	// created, so os.MkdirAll inside SaveBaseline fails.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run(updateArgs(profile, filepath.Join(blocker, "sub", "b.json")), &out, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 when SaveBaseline can't create its directory, got %d: %s", code, out.String())
	}
}

// The default paths are repo-relative constants; -root stands in for the
// working directory so the test never touches the real android/ report.
func TestRun_DefaultPathsResolveUnderRoot(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, defaultProfileRel)
	writeFile(t, profile, twoFileXML)

	var out bytes.Buffer
	if code := run([]string{"-root", root, "-update"}, &out, &out); code != 0 {
		t.Fatalf("-update with default paths exited %d: %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, defaultBaselineRel)); err != nil {
		t.Fatalf("default baseline not written under root: %v", err)
	}
}

// Without -root the tool walks up to backend/go.mod, matching the other
// cmd/*check tools.
func TestRun_FindsRepoRootWhenRootOmitted(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "go.mod"), "module mycorrhizal\n")
	writeFile(t, filepath.Join(root, defaultProfileRel), twoFileXML)
	t.Chdir(filepath.Join(root, "backend"))

	var out bytes.Buffer
	if code := run([]string{"-update"}, &out, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, defaultBaselineRel)); err != nil {
		t.Fatalf("baseline not written at the repo root: %v", err)
	}
}

// An explicit -profile/-baseline pair works with no repo root discoverable, so
// the lazy walk never runs.
func TestRun_ExplicitPathsNeedNoRepoRoot(t *testing.T) {
	dir := t.TempDir() // no backend/go.mod anywhere above it
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	baseline := filepath.Join(dir, "coverage-baseline.json")
	t.Chdir(dir)

	var out bytes.Buffer
	if code := run(updateArgs(profile, baseline), &out, &out); code != 0 {
		t.Fatalf("-update with explicit paths exited %d: %s", code, out.String())
	}
}

func TestRun_UnknownFlagExitsTwo(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-not-a-flag"}, &out, &out); code != 2 {
		t.Fatalf("expected exit 2 for an unknown flag, got %d", code)
	}
}

// One explicit path plus a defaulted one still needs the repo root; with none
// discoverable (no backend/go.mod above the cwd) the tool must exit 2 rather
// than silently use an empty root.
func TestRun_DefaultedProfileWithoutRepoRootExitsTwo(t *testing.T) {
	dir := t.TempDir()
	baseline := writeFile(t, filepath.Join(dir, "b.json"), `{"tolerancePercentPoints":1.5,"files":{}}`)
	t.Chdir(dir)

	var out bytes.Buffer
	if code := run([]string{"-baseline", baseline}, &out, &out); code != 2 {
		t.Fatalf("expected exit 2 for an undiscoverable repo root, got %d: %s", code, out.String())
	}
}

func TestRun_DefaultedBaselineWithoutRepoRootExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeFile(t, filepath.Join(dir, "report.xml"), twoFileXML)
	t.Chdir(dir)

	var out bytes.Buffer
	if code := run([]string{"-profile", profile}, &out, &out); code != 2 {
		t.Fatalf("expected exit 2 for an undiscoverable repo root, got %d: %s", code, out.String())
	}
}

func TestFindRepoRoot_NoRepoAbove(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := findRepoRoot(); err == nil {
		t.Fatal("expected findRepoRoot to error with no backend/go.mod above the working directory")
	}
}

func toleranceOf(t *testing.T, path string) float64 {
	t.Helper()
	var b struct {
		Tolerance float64 `json:"tolerancePercentPoints"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	return b.Tolerance
}

func setTolerance(t *testing.T, path string, tol float64) {
	t.Helper()
	var b map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	b["tolerancePercentPoints"] = tol
	data, err = json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
