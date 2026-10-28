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
