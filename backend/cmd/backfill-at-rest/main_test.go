package main

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"testing"

	"mycorrhizal/atrest"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func testKEK() []byte {
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = 0x30 + byte(i)
	}
	return kek
}

func b64(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

func newMigratedDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "backfill.db")
	db := dbtest.NewAt(t, dbPath)
	t.Cleanup(atrest.ResetForTest)
	return db, dbPath
}

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func readHowWeMet(t *testing.T, db *gorm.DB, firstname string) string {
	t.Helper()
	var stored string
	require.NoError(t, db.Table("contacts").Select("how_we_met").
		Where("firstname = ?", firstname).Scan(&stored).Error)
	return stored
}

// seedPlaintext writes a row directly (bypassing the at-rest serializer) — the
// exact pre-backfill state the command exists to repair.
func seedPlaintext(t *testing.T, db *gorm.DB, value string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO contacts (user_id, firstname, how_we_met) VALUES (1, 'Bob', ?)", value).Error)
}

// TestRun_BackfillsThenIsIdempotent is the happy path plus the idempotence
// guarantee: a plaintext row becomes ciphertext that decrypts to the original,
// and a second run does not double-encrypt it.
func TestRun_BackfillsThenIsIdempotent(t *testing.T) {
	key := testKEK()
	db, dbPath := newMigratedDB(t)
	const plaintext = "met at the market"
	seedPlaintext(t, db, plaintext)

	env := envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(key)})
	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath}, env, &out, &errBuf)
	require.Equal(t, 0, code, "stderr: %s", errBuf.String())
	require.Contains(t, out.String(), "At-rest encryption backfill complete")

	first := readHowWeMet(t, db, "Bob")
	require.Contains(t, first, "encv1:", "the plaintext row must be encrypted after the backfill")

	atrest.ResetForTest()
	require.NoError(t, atrest.Initialize(db, key))
	got, err := atrest.Decrypt(first)
	require.NoError(t, err)
	require.Equal(t, plaintext, got, "the backfilled ciphertext must decrypt to the original")

	// Second run: the row already carries the prefix, so its bytes are unchanged.
	out.Reset()
	errBuf.Reset()
	code = run([]string{"-db", dbPath}, env, &out, &errBuf)
	require.Equal(t, 0, code, "stderr: %s", errBuf.String())
	require.Equal(t, first, readHowWeMet(t, db, "Bob"), "a re-run must not double-encrypt")
}

// TestRun_InvalidKeyLeavesRowsUntouched pins the fail-before-write behavior for
// a malformed key: non-zero exit, a message, and the plaintext row unchanged.
func TestRun_InvalidKeyLeavesRowsUntouched(t *testing.T) {
	db, dbPath := newMigratedDB(t)
	seedPlaintext(t, db, "still plaintext")

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath},
		envMap(map[string]string{"DATA_ENCRYPTION_KEY": "!!!not-base64!!!"}), &out, &errBuf)
	require.NotZero(t, code)
	require.Contains(t, errBuf.String(), "failed to resolve at-rest encryption master key")
	require.Equal(t, "still plaintext", readHowWeMet(t, db, "Bob"), "an invalid key must not touch rows")
}

// TestRun_MissingKeyLeavesRowsUntouched pins the fail-closed behavior when no
// key is configured at all: refusing beats a silent no-op that reports success
// while leaving data unencrypted.
func TestRun_MissingKeyLeavesRowsUntouched(t *testing.T) {
	db, dbPath := newMigratedDB(t)
	seedPlaintext(t, db, "still plaintext")

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath}, envMap(nil), &out, &errBuf)
	require.NotZero(t, code)
	require.Contains(t, errBuf.String(), "failed to resolve at-rest encryption master key")
	require.Equal(t, "still plaintext", readHowWeMet(t, db, "Bob"), "an unconfigured key must not touch rows")
}

// TestRun_BadFlag pins the usage exit: an unknown flag is exit code 2 (distinct
// from the runtime-failure code 1), with the flag package's complaint on stderr
// and nothing on stdout.
func TestRun_BadFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-no-such-flag"}, envMap(nil), &out, &errBuf)
	require.Equal(t, 2, code)
	require.Contains(t, errBuf.String(), "flag provided but not defined")
	require.Empty(t, out.String())
}

// TestRun_UnopenableDatabase pins the first runtime failure: a database path
// that cannot be opened exits 1 before any key is resolved.
func TestRun_UnopenableDatabase(t *testing.T) {
	var out, errBuf bytes.Buffer
	missing := filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")
	code := run([]string{"-db", missing}, envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(testKEK())}), &out, &errBuf)
	require.Equal(t, 1, code)
	require.Contains(t, errBuf.String(), "failed to open database")
	require.Empty(t, out.String())
}

// TestRun_WrongKeyFailsToInitialize pins the boot-equivalent refusal: the
// database already holds a DEK wrapped under one master key, the operator
// supplies a different (well-formed) one, so the DEK cannot be unwrapped. The
// command must exit 1 and leave plaintext rows exactly as they were rather than
// encrypting under a key the server could never read back.
func TestRun_WrongKeyFailsToInitialize(t *testing.T) {
	db, dbPath := newMigratedDB(t)
	require.NoError(t, atrest.Initialize(db, testKEK()))
	atrest.ResetForTest()
	seedPlaintext(t, db, "still plaintext")

	wrongKEK := bytes.Repeat([]byte{0x7a}, 32)
	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath},
		envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(wrongKEK)}), &out, &errBuf)
	require.Equal(t, 1, code)
	require.Contains(t, errBuf.String(), "failed to initialize at-rest encryption")
	require.Empty(t, out.String(), "a failed run must not print the success line")
	require.Equal(t, "still plaintext", readHowWeMet(t, db, "Bob"), "a wrong key must not touch rows")
}

// TestRun_BackfillFailureRollsBack pins the last runtime failure: the key
// resolves and initializes, but the column UPDATE itself fails. A trigger that
// aborts any UPDATE of how_we_met stands in for a mid-backfill database error
// (disk full, lock, constraint). Backfill runs one transaction per column, so
// the plaintext row must be unchanged and the command must exit 1 without the
// success line.
func TestRun_BackfillFailureRollsBack(t *testing.T) {
	db, dbPath := newMigratedDB(t)
	seedPlaintext(t, db, "still plaintext")
	require.NoError(t, db.Exec(
		`CREATE TRIGGER block_how_we_met_update BEFORE UPDATE OF how_we_met ON contacts `+
			`BEGIN SELECT RAISE(ABORT, 'how_we_met update blocked by test'); END`).Error)

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath},
		envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(testKEK())}), &out, &errBuf)
	require.Equal(t, 1, code, "stderr: %s", errBuf.String())
	require.Contains(t, errBuf.String(), "failed to backfill at-rest encryption")
	require.Contains(t, errBuf.String(), "how_we_met update blocked by test")
	require.Empty(t, out.String(), "a failed backfill must not print the success line")
	require.Equal(t, "still plaintext", readHowWeMet(t, db, "Bob"), "a failed column backfill must roll back")
}
