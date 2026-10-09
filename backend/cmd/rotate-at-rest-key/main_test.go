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

// testKEK returns a deterministic 32-byte master key derived from b.
func testKEK(b byte) []byte {
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = b + byte(i)
	}
	return kek
}

func b64(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

// newMigratedDB hands back a real migrated DB file (the CLI re-opens it by
// path) plus the open handle used to seed and to inspect the result.
func newMigratedDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "rotate.db")
	db := dbtest.NewAt(t, dbPath)
	t.Cleanup(atrest.ResetForTest)
	return db, dbPath
}

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func readWrappedDEK(t *testing.T, db *gorm.DB) []byte {
	t.Helper()
	var row struct {
		WrappedDEK []byte `gorm:"column:wrapped_dek"`
	}
	require.NoError(t, db.Table("data_encryption_keys").Select("wrapped_dek").Limit(1).Scan(&row).Error)
	return row.WrappedDEK
}

func readHowWeMet(t *testing.T, db *gorm.DB, firstname string) string {
	t.Helper()
	var stored string
	require.NoError(t, db.Table("contacts").Select("how_we_met").
		Where("firstname = ?", firstname).Scan(&stored).Error)
	return stored
}

// TestRun_RotatesKeyAndIsIdempotent seeds encrypted data under an old master
// key, rotates to a new one through run(), and proves the rotation actually
// took effect: the old key can no longer unwrap the DEK, the new key can, and
// the untouched payload still decrypts to the original plaintext. A second
// run (old resolved from env, now equal to the new key) is safe.
func TestRun_RotatesKeyAndIsIdempotent(t *testing.T) {
	oldKEK := testKEK(0x11)
	newKEK := testKEK(0x22)
	db, dbPath := newMigratedDB(t)

	// Seed a payload encrypted under the old key, exactly the on-disk state a
	// pre-rotation instance has.
	require.NoError(t, atrest.Initialize(db, oldKEK))
	const plaintext = "rotated secret payload"
	ct, err := atrest.Encrypt(plaintext)
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		"INSERT INTO contacts (user_id, firstname, how_we_met) VALUES (1, 'Alice', ?)", ct).Error)
	atrest.ResetForTest()

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath, "-new", b64(newKEK)},
		envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(oldKEK)}), &out, &errBuf)
	require.Equal(t, 0, code, "stderr: %s", errBuf.String())
	require.Contains(t, out.String(), "Master key rotated")

	// The old key can no longer unwrap the DEK — a real boot with it fails
	// closed.
	atrest.ResetForTest()
	require.Error(t, atrest.Initialize(db, oldKEK),
		"after rotation the old master key must no longer unwrap the DEK")
	atrest.ResetForTest()

	// The new key unwraps the DEK and the payload (never re-encrypted) still
	// decrypts to the original plaintext.
	require.NoError(t, atrest.Initialize(db, newKEK))
	got, err := atrest.Decrypt(readHowWeMet(t, db, "Alice"))
	require.NoError(t, err)
	require.Equal(t, plaintext, got, "rotation must not alter the payload")

	// Idempotent / safe re-run: the old key is now resolved from env (= the new
	// key) and the new key is the same, so rotate-by-same-key is a no-op.
	atrest.ResetForTest()
	out.Reset()
	errBuf.Reset()
	code = run([]string{"-db", dbPath, "-new", b64(newKEK)},
		envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(newKEK)}), &out, &errBuf)
	require.Equal(t, 0, code, "stderr: %s", errBuf.String())

	require.NoError(t, atrest.Initialize(db, newKEK))
	got, err = atrest.Decrypt(readHowWeMet(t, db, "Alice"))
	require.NoError(t, err)
	require.Equal(t, plaintext, got, "a re-run must leave the payload decryptable under the new key")
}

// TestRun_MissingNewKeyUsage pins the usage exit: no -new is a usage error
// (non-zero) with the usage line on stderr, before any database work.
func TestRun_MissingNewKeyUsage(t *testing.T) {
	db, dbPath := newMigratedDB(t)
	require.NoError(t, atrest.Initialize(db, testKEK(0x33)))
	before := readWrappedDEK(t, db)

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath}, envMap(nil), &out, &errBuf)
	require.NotZero(t, code)
	require.Contains(t, errBuf.String(), "usage: rotate-at-rest-key")
	require.Equal(t, before, readWrappedDEK(t, db), "a usage error must not touch key material")
}

// TestRun_InvalidNewKeyLeavesKeyMaterialUntouched pins that a malformed -new
// key fails before RotateMasterKey runs, so the wrapped DEK is byte-identical
// afterward.
func TestRun_InvalidNewKeyLeavesKeyMaterialUntouched(t *testing.T) {
	oldKEK := testKEK(0x44)
	db, dbPath := newMigratedDB(t)
	require.NoError(t, atrest.Initialize(db, oldKEK))
	atrest.ResetForTest()
	before := readWrappedDEK(t, db)

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath, "-new", "!!!not-base64!!!"},
		envMap(map[string]string{"DATA_ENCRYPTION_KEY": b64(oldKEK)}), &out, &errBuf)
	require.NotZero(t, code)
	require.Contains(t, errBuf.String(), "failed to decode -new key")
	require.Equal(t, before, readWrappedDEK(t, db), "an invalid new key must leave the DEK untouched")
}

// TestRun_UnresolvableOldKey pins the "no way to unwrap the current DEK"
// refusal: with none of the three env vars set and no -old, run() exits
// non-zero and does not rotate.
func TestRun_UnresolvableOldKey(t *testing.T) {
	oldKEK := testKEK(0x55)
	db, dbPath := newMigratedDB(t)
	require.NoError(t, atrest.Initialize(db, oldKEK))
	atrest.ResetForTest()
	before := readWrappedDEK(t, db)

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath, "-new", b64(testKEK(0x66))}, envMap(nil), &out, &errBuf)
	require.NotZero(t, code)
	require.Contains(t, errBuf.String(), "no current master key resolved")
	require.Equal(t, before, readWrappedDEK(t, db), "an unresolvable old key must not rotate the DEK")
}
