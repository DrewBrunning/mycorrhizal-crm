package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/realrelease"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runCLI(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestUsageAndUnknownSubcommand(t *testing.T) {
	code, _, errs := run()
	assert.Equal(t, 2, code)
	assert.Contains(t, errs, "usage")

	code, _, errs = run("bogus")
	assert.Equal(t, 2, code)
	assert.Contains(t, errs, "unknown subcommand")
}

func TestPrintSecret(t *testing.T) {
	code, out, _ := run("print-secret")
	assert.Equal(t, 0, code)
	assert.Equal(t, realrelease.SeedJWTSecret+"\n", out)
}

func TestMissingFlagsAreOperationalErrors(t *testing.T) {
	for _, args := range [][]string{
		{"seed"}, {"capture"}, {"verify"}, {"compare"},
		{"seed", "--nope"}, {"capture", "--nope"}, {"verify", "--nope"}, {"compare", "--nope"},
	} {
		code, _, _ := run(args...)
		assert.Equalf(t, 2, code, "%v", args)
	}
}

func TestCompareSubcommand(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json"), filepath.Join(dir, "c.json")
	require.NoError(t, os.WriteFile(a, []byte(`{"k":"v"}`), 0o600))
	require.NoError(t, os.WriteFile(b, []byte(`{"k":"v","new":1}`), 0o600))
	require.NoError(t, os.WriteFile(c, []byte(`{"k":"changed"}`), 0o600))

	code, out, _ := run("compare", "--pre", a, "--post", b)
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "OK")

	code, _, errs := run("compare", "--pre", a, "--post", c)
	assert.Equal(t, 1, code)
	assert.Contains(t, errs, "FAIL: 1 finding")

	code, _, _ = run("compare", "--pre", a, "--post", filepath.Join(dir, "missing.json"))
	assert.Equal(t, 2, code)
	code, _, _ = run("compare", "--pre", filepath.Join(dir, "missing.json"), "--post", a)
	assert.Equal(t, 2, code)
}

func TestSeedCaptureVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "install", "mycorrhizal.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o750))

	cur, err := realrelease.StartCurrent(context.Background(), dbPath, "", "")
	require.NoError(t, err)

	out := filepath.Join(dir, "out")
	code, stdout, errs := run("seed", "--base-url", cur.BaseURL(), "--out", out)
	require.Equal(t, 0, code, errs)
	assert.Contains(t, stdout, "contacts read back")

	cur.Stop()

	// capture burns a TOTP step and verify needs the same one, so capture runs
	// against a snapshot copy (the rollback leg likewise reads a restored copy).
	copyPath := filepath.Join(dir, "copy", "mycorrhizal.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(copyPath), 0o750))
	require.NoError(t, database.BackupSnapshot(dbPath, copyPath))
	cur, err = realrelease.StartCurrent(context.Background(), copyPath, filepath.Join(filepath.Dir(dbPath), "photos"), filepath.Join(filepath.Dir(dbPath), "attachments"))
	require.NoError(t, err)
	rollback := filepath.Join(dir, "rollback.json")
	code, stdout, errs = run("capture", "--base-url", cur.BaseURL(), "--creds", filepath.Join(out, "credentials.json"), "--out", rollback)
	require.Equal(t, 0, code, errs)
	assert.Contains(t, stdout, "captured")
	cur.Stop()

	// A pristine copy for the failing-verify case below (verify consumes a
	// recovery code, so the verified database cannot be verified twice).
	badDB := filepath.Join(dir, "bad", "mycorrhizal.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(badDB), 0o750))
	require.NoError(t, database.BackupSnapshot(dbPath, badDB))
	require.NoError(t, os.CopyFS(filepath.Join(dir, "bad", "attachments"), os.DirFS(filepath.Join(filepath.Dir(dbPath), "attachments"))))

	post := filepath.Join(dir, "post.json")
	code, stdout, errs = run("verify", "--db", dbPath,
		"--creds", filepath.Join(out, "credentials.json"), "--pre", filepath.Join(out, "pre-snapshot.json"), "--out", post)
	require.Equal(t, 0, code, errs)
	assert.Contains(t, stdout, "OK: upgrade preserved")
	assert.FileExists(t, post)

	// Corrupt the snapshot the old release "reported": verify must exit 1.
	bad := filepath.Join(dir, "bad-pre.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"contacts":[{"id":9999}]}`), 0o600))
	code, _, errs = run("verify", "--db", badDB,
		"--creds", filepath.Join(out, "credentials.json"), "--pre", bad)
	assert.Equal(t, 1, code)
	assert.Contains(t, errs, "FAIL")

	// Unreadable inputs and a failing seed are operational errors.
	code, _, _ = run("verify", "--db", dbPath, "--creds", filepath.Join(dir, "nope"), "--pre", bad)
	assert.Equal(t, 2, code)
	code, _, _ = run("verify", "--db", dbPath, "--creds", filepath.Join(out, "credentials.json"), "--pre", filepath.Join(dir, "nope"))
	assert.Equal(t, 2, code)
	code, _, _ = run("capture", "--base-url", "http://127.0.0.1:1", "--creds", filepath.Join(out, "credentials.json"), "--out", rollback)
	assert.Equal(t, 2, code)
	code, _, _ = run("capture", "--base-url", "http://127.0.0.1:1", "--creds", filepath.Join(dir, "nope"), "--out", rollback)
	assert.Equal(t, 2, code)
	code, _, _ = run("seed", "--base-url", "http://127.0.0.1:1", "--out", filepath.Join(dir, "out2"))
	assert.Equal(t, 2, code)
}
