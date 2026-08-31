package differential

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewPyRefReportsUnavailable pins the availability probe: when python3
// is absent, the reference is reported unavailable with a reason (the legs
// then skip). It points PATH at an empty dir to simulate the absence.
func TestNewPyRefReportsUnavailable(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	ref, reason := NewPyRef()
	require.Empty(t, ref.python)
	require.Contains(t, reason, "unavailable")
}

// TestNewPyRefReportsMissingVobject pins the other half of the probe: a
// python3 whose vobject import fails is reported unavailable (the pinned
// reference is installed by CI, not assumed present).
func TestNewPyRefReportsMissingVobject(t *testing.T) {
	python, err := execLookPathForTest(t)
	if err != nil {
		t.Skip("python3 not present; cannot test the missing-module probe")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "python3")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+pythonDir(python))
	ref, reason := NewPyRef()
	require.Empty(t, ref.python)
	require.Contains(t, reason, "vobject")
}

func execLookPathForTest(t *testing.T) (string, error) {
	t.Helper()
	return exec.LookPath("python3")
}

func pythonDir(python string) string {
	return filepath.Dir(python)
}

// TestNewCalcardRefEnvOverride pins the env-var resolution: the command line
// is taken verbatim (split on spaces) so CI can point the harness at a
// docker-run invocation or a built binary.
func TestNewCalcardRefEnvOverride(t *testing.T) {
	t.Setenv(calcardEnvVar, "docker run --rm -i example/calcard-ref")
	ref, reason := NewCalcardRef()
	require.Empty(t, reason)
	require.Equal(t, []string{"docker", "run", "--rm", "-i", "example/calcard-ref"}, ref.argv)
}

// TestNewCalcardRefPrebuiltBinary pins the binary fallback: a prebuilt
// calcard-ref in the expected location resolves without an env var.
func TestNewCalcardRefPrebuiltBinary(t *testing.T) {
	t.Setenv(calcardEnvVar, "")
	release := filepath.Join("reference", "calcard", "target", "release", "calcard-ref")
	debug := filepath.Join("reference", "calcard", "target", "debug", "calcard-ref")
	had := false
	for _, p := range []string{release, debug} {
		if _, err := os.Stat(p); err == nil {
			had = true
		}
	}
	if had {
		ref, reason := NewCalcardRef()
		require.Empty(t, reason)
		require.NotEmpty(t, ref.argv)
		return
	}
	// Neither binary is present on this machine: the resolver must report
	// unavailable (the scheduled CI job builds it).
	ref, reason := NewCalcardRef()
	require.Empty(t, ref.argv)
	require.Contains(t, reason, calcardEnvVar)
}

// TestNewCalcardRefMissingBinary forces the unavailable path deterministically
// (regardless of whether a prebuilt binary exists on this machine) by running
// from an empty directory, where the relative reference paths cannot resolve.
func TestNewCalcardRefMissingBinary(t *testing.T) {
	t.Setenv(calcardEnvVar, "")
	t.Chdir(t.TempDir())
	ref, reason := NewCalcardRef()
	require.Empty(t, ref.argv)
	require.Contains(t, reason, calcardEnvVar)
}

// TestPyRefRunError covers the reference-failure path: a python3 that exits
// non-zero produces a descriptive error (the differential legs surface this
// instead of emitting partial output).
func TestPyRefRunError(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "python3")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755))
	t.Setenv("PATH", dir)
	ref, reason := NewPyRef()
	require.Empty(t, ref.python, "a failing python3 must not be accepted as the reference: %s", reason)

	// The run path itself (not just the probe) surfaces the failure: a
	// python that fails mid-conversion produces a descriptive error.
	ref2 := pyRef{python: fake}
	_, err := ref2.toNeutral([]byte("BEGIN:VCARD\nEND:VCARD"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "vobject reference")
}

// TestCalcardRefRunError covers the reference-failure path for the JSContact
// leg: a command that exits non-zero surfaces a descriptive error naming the
// subcommand.
func TestCalcardRefRunError(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "calcard-ref")
	require.NoError(t, os.WriteFile(bad, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755))
	ref := calcardRef{argv: []string{bad}}
	_, err := ref.run("jscontact-reemit", []byte(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "jscontact-reemit")
}

// TestCalcardRefRunSuccess covers the success path with a fake reference
// command that echoes a fixed payload.
func TestCalcardRefRunSuccess(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "calcard-ref")
	require.NoError(t, os.WriteFile(ok, []byte("#!/bin/sh\ncat\necho '{\"@type\":\"Card\",\"version\":\"1.0\"}'\n"), 0o755))
	ref := calcardRef{argv: []string{ok}}
	out, err := ref.run("vcard-to-jscontact", []byte("BEGIN:VCARD\nEND:VCARD"))
	require.NoError(t, err)
	require.Contains(t, string(out), "@type")
}
