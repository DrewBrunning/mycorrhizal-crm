package reasoncode_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/lint/reasoncode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pkgDir is the package directory (the go test cwd); backendRoot is the module
// root the command runs from.
var (
	pkgDir      = mustGetwd()
	backendRoot = filepath.Join(pkgDir, "..", "..", "..")
)

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}

// run executes Run from the backend module root (the cwd cmd/reasoncodecheck
// uses), since Count makes paths relative to it.
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	require.NoError(t, os.Chdir(backendRoot))
	var out, errb bytes.Buffer
	code := reasoncode.Run(args, &out, &errb)
	t.Cleanup(func() { _ = os.Chdir(pkgDir) })
	return code, out.String(), errb.String()
}

func TestRunWriteThenCheckThenRatchet(t *testing.T) {
	list := filepath.Join(t.TempDir(), "allow.txt")

	// controllers has unmigrated sites, so a write records them.
	code, out, _ := run(t, "-allowlist", list, "-write-allowlist", "./controllers/")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "wrote")
	allow := mustParse(t, list)
	require.NotEmpty(t, allow)

	// Exactly matching passes.
	code, _, errs := run(t, "-allowlist", list, "./controllers/")
	assert.Equal(t, 0, code, errs)

	// Lowering the recorded count below reality (a new site) fails.
	for p, n := range allow {
		allow[p] = n - 1
		if n-1 == 0 {
			delete(allow, p)
		}
		break
	}
	writeAllow(t, list, allow)
	code, _, errs = run(t, "-allowlist", list, "./controllers/")
	assert.Equal(t, 1, code)
	assert.Contains(t, errs, "reason code")

	// A clean package against an allowlist that still lists sites: lower it.
	code, _, errs = run(t, "-allowlist", list, "./errors/")
	assert.Equal(t, 1, code)
	assert.Contains(t, errs, "lower it")
}

func TestRunErrors(t *testing.T) {
	code, _, _ := run(t, "-nope")
	assert.Equal(t, 2, code)

	code, _, errs := run(t, "-allowlist", filepath.Join(t.TempDir(), "missing.txt"), "./errors/")
	assert.Equal(t, 2, code)
	assert.Contains(t, errs, "reasoncodecheck:")

	bad := filepath.Join(t.TempDir(), "bad.txt")
	require.NoError(t, os.WriteFile(bad, []byte("nocolon\n"), 0o600))
	code, _, _ = run(t, "-allowlist", bad, "./errors/")
	assert.Equal(t, 2, code)

	code, _, _ = run(t, "-allowlist", bad, "./does/not/exist/...")
	assert.Equal(t, 2, code)

	// Default pattern is ./... ; just confirm an unwritable -write target errors.
	code, _, _ = run(t, "-allowlist", filepath.Join(t.TempDir(), "no", "dir.txt"), "-write-allowlist", "./errors/")
	assert.Equal(t, 2, code)
}

func mustParse(t *testing.T, path string) map[string]int {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	m, err := reasoncode.ParseAllowlist(f)
	require.NoError(t, err)
	return m
}

func writeAllow(t *testing.T, path string, m map[string]int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, reasoncode.WriteAllowlist(f, m))
}

// emptyAllowlist writes an allowlist with no entries (header only).
func emptyAllowlist(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "empty.txt")
	writeAllow(t, path, map[string]int{})
	return path
}

// TestReleaseTagRequiresEmptyAllowlist pins the milestone gate (issue #2093):
// from ZeroFromRelease onward a release may not carry any unmigrated site, so
// the ratchet must have reached zero, not merely stopped growing.
func TestReleaseTagRequiresEmptyAllowlist(t *testing.T) {
	list := filepath.Join(t.TempDir(), "allow.txt")
	code, _, _ := run(t, "-allowlist", list, "-write-allowlist", "./controllers/")
	require.Equal(t, 0, code)
	require.NotEmpty(t, mustParse(t, list))

	// Before the milestone the recorded count is accepted, as always.
	for _, tag := range []string{"v1.5.9", "v1.5.1", "v1.5.9-rc.1"} {
		code, _, errs := run(t, "-allowlist", list, "-release-tag", tag, "./controllers/")
		assert.Equal(t, 0, code, "%s: %s", tag, errs)
	}

	// At and after it, a non-empty allowlist blocks the release, naming the count.
	for _, tag := range []string{"v1.6.0", "v1.6.0-rc.1", "v1.6.1", "v1.7.0", "v2.0.0"} {
		code, _, errs := run(t, "-allowlist", list, "-release-tag", tag, "./controllers/")
		assert.Equal(t, 1, code, tag)
		assert.Contains(t, errs, "#2093", tag)
		assert.Contains(t, errs, "unmigrated", tag)
	}
}

func TestReleaseTagPassesOnceMigrated(t *testing.T) {
	// ./errors/ has no unmigrated sites, and the allowlist has no entries.
	code, _, errs := run(t, "-allowlist", emptyAllowlist(t), "-release-tag", "v1.6.0", "./errors/")
	assert.Equal(t, 0, code, errs)
}

func TestReleaseTagStillEnforcesTheRatchet(t *testing.T) {
	// The release-tag mode is additive: an allowlist that disagrees with the
	// code fails before the milestone too.
	list := filepath.Join(t.TempDir(), "allow.txt")
	code, _, _ := run(t, "-allowlist", list, "-write-allowlist", "./controllers/")
	require.Equal(t, 0, code)
	code, _, errs := run(t, "-allowlist", list, "-release-tag", "v1.5.9", "./errors/")
	assert.Equal(t, 1, code, errs)
	assert.Contains(t, errs, "lower it")
}

func TestReleaseTagRejectsMalformedTags(t *testing.T) {
	for _, tag := range []string{"1.6.0", "v1.6", "latest"} {
		code, _, errs := run(t, "-allowlist", emptyAllowlist(t), "-release-tag", tag, "./errors/")
		assert.Equal(t, 2, code, tag)
		assert.Contains(t, errs, "release-tag", tag)
	}
}
