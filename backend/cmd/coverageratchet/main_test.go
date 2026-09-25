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
