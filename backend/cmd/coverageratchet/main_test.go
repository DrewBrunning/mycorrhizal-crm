package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProfile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoFileProfile = `mode: atomic
mycorrhizal/internal/foo/bar.go:1.1,3.2 2 5
mycorrhizal/internal/foo/bar.go:5.1,5.2 1 0
mycorrhizal/internal/baz/qux.go:1.1,2.2 1 3
`

func TestRun_UpdateThenCheckRoundTrips(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)

	var out bytes.Buffer
	code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out)
	if code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "wrote new baseline") {
		t.Errorf("update output = %q", out.String())
	}

	baselinePathAbs := filepath.Join(dir, baselinePath)
	if _, err := os.Stat(baselinePathAbs); err != nil {
		t.Fatalf("baseline was not written: %v", err)
	}

	out.Reset()
	code = run([]string{"-profile", profile, "-root", dir}, &out)
	if code != 0 {
		t.Fatalf("check exited %d after a fresh -update, want 0: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "coverageratchet: ok") {
		t.Errorf("check output = %q", out.String())
	}
}

func TestRun_DetectsRegression(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)

	var out bytes.Buffer
	if code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}

	// A regressed profile: bar.go's previously-covered block now reads 0.
	regressed := `mode: atomic
mycorrhizal/internal/foo/bar.go:1.1,3.2 2 0
mycorrhizal/internal/foo/bar.go:5.1,5.2 1 0
mycorrhizal/internal/baz/qux.go:1.1,2.2 1 3
`
	profile2 := writeProfile(t, dir, "coverage2.out", regressed)

	out.Reset()
	code := run([]string{"-profile", profile2, "-root", dir}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for a regressed file, got %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "internal/foo/bar.go") {
		t.Errorf("output did not name the regressed file: %q", out.String())
	}
}

func TestRun_NewFileNotGated(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)

	var out bytes.Buffer
	if code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}

	withNewFile := twoFileProfile + "mycorrhizal/internal/newthing/new.go:1.1,2.2 1 0\n"
	profile2 := writeProfile(t, dir, "coverage2.out", withNewFile)

	out.Reset()
	code := run([]string{"-profile", profile2, "-root", dir}, &out)
	if code != 0 {
		t.Fatalf("a brand-new uncovered file must not fail the ratchet, got exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "new (not gated here)") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRun_MissingProfileExitsTwo(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	code := run([]string{"-profile", filepath.Join(dir, "nope.out"), "-root", dir}, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 for a missing profile, got %d", code)
	}
}

func TestRun_MalformedProfileExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", "not a coverprofile\n")
	var out bytes.Buffer
	code := run([]string{"-profile", profile, "-root", dir}, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 for a malformed profile, got %d", code)
	}
}

func TestRun_CheckWithoutBaselineExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)
	var out bytes.Buffer
	code := run([]string{"-profile", profile, "-root", dir}, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 checking with no baseline present, got %d: %s", code, out.String())
	}
}

func TestRun_PragmaMarkedLineExcluded(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "foo"), 0o750); err != nil {
		t.Fatal(err)
	}
	src := "package foo\n\nfunc A() {\n\tx := 1 // # pragma: no cover\n\t_ = x\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "internal", "foo", "bar.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	profileContents := `mode: atomic
mycorrhizal/internal/foo/bar.go:3.1,5.2 2 0
`
	profile := writeProfile(t, dir, "coverage.out", profileContents)

	var out bytes.Buffer
	code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out)
	if code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}
	b, err := loadBaselineForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 0 {
		t.Errorf("a file whose only block is pragma-excluded should not appear in the baseline at all: %v", b)
	}
}

func TestRun_UpdateTwiceReusesExistingBaseline(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)

	var out bytes.Buffer
	if code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out); code != 0 {
		t.Fatalf("first -update exited %d: %s", code, out.String())
	}

	// A second -update with an existing baseline on disk must load and
	// carry it forward (main.go's `existing = &b` branch) rather than
	// treating it as absent.
	out.Reset()
	code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out)
	if code != 0 {
		t.Fatalf("second -update exited %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "wrote new baseline") {
		t.Errorf("second update output = %q", out.String())
	}
}

func TestRun_SaveBaselineFailureExitsTwo(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)

	// Put a plain file where the baseline's parent directory needs to be
	// created, so os.MkdirAll inside SaveBaseline fails.
	blockingDir := filepath.Dir(filepath.Join(dir, baselinePath))
	if err := os.MkdirAll(filepath.Dir(blockingDir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blockingDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 when SaveBaseline can't create its directory, got %d: %s", code, out.String())
	}
}

func TestRun_FilterPragmaErrorExitsTwo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "foo"), 0o750); err != nil {
		t.Fatal(err)
	}
	// A single source line far past bufio.Scanner's default 64KiB token
	// buffer makes pragmaLinesOf's scanner.Scan() fail with ErrTooLong,
	// which FilterPragma propagates as an error -- main.go's 74-76 branch.
	huge := strings.Repeat("x", 128*1024)
	src := "package foo\n\nvar _ = \"" + huge + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "internal", "foo", "bar.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	profileContents := "mode: atomic\nmycorrhizal/internal/foo/bar.go:3.1,3.2 1 0\n"
	profile := writeProfile(t, dir, "coverage.out", profileContents)

	var out bytes.Buffer
	code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out)
	if code != 2 {
		t.Fatalf("expected exit 2 for a FilterPragma scan error, got %d: %s", code, out.String())
	}
}

func TestRun_GoneFileReportedNotFailed(t *testing.T) {
	dir := t.TempDir()
	profile := writeProfile(t, dir, "coverage.out", twoFileProfile)

	var out bytes.Buffer
	if code := run([]string{"-profile", profile, "-root", dir, "-update"}, &out); code != 0 {
		t.Fatalf("-update exited %d: %s", code, out.String())
	}

	// A follow-up profile that drops baz/qux.go entirely: it's gone from
	// current stats, so main.go must report it (not gate on it).
	withoutQux := `mode: atomic
mycorrhizal/internal/foo/bar.go:1.1,3.2 2 5
mycorrhizal/internal/foo/bar.go:5.1,5.2 1 0
`
	profile2 := writeProfile(t, dir, "coverage2.out", withoutQux)

	out.Reset()
	code := run([]string{"-profile", profile2, "-root", dir}, &out)
	if code != 0 {
		t.Fatalf("a gone file must not fail the ratchet, got exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "gone (baseline stale, run -update): internal/baz/qux.go") {
		t.Errorf("output did not report the gone file: %q", out.String())
	}
}

func loadBaselineForTest(dir string) (map[string]float64, error) {
	data, err := os.ReadFile(filepath.Join(dir, baselinePath))
	if err != nil {
		return nil, err
	}
	// Minimal decode without importing encoding/json twice for a one-off
	// assertion helper -- reuse the package's own loader instead.
	var parsed struct {
		Files map[string]float64 `json:"files"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed.Files, nil
}
