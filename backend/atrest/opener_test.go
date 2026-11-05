package atrest

import (
	"context"
	"database/sql"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Opener reads a snapshot's encrypted columns for the offline backup
// verifier (issue #2367) without arming the engine or writing to the database.

func openerDB(t *testing.T) (*sql.DB, func(kek []byte) string) {
	t.Helper()
	db := dbtest.New(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// seal initializes the DEK row under kek and returns a value encrypted
	// with it, then disarms the engine again.
	return sqlDB, func(kek []byte) string {
		require.NoError(t, Initialize(db, kek))
		defer ResetForTest()
		enc, err := Encrypt("secret payload")
		require.NoError(t, err)
		return enc
	}
}

func TestOpener_DecryptsWithTheSnapshotsOwnDEK(t *testing.T) {
	sqlDB, seal := openerDB(t)
	enc := seal(testKEK(t))

	o, err := NewOpener(context.Background(), sqlDB, testKEK(t))
	require.NoError(t, err)
	got, err := o.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "secret payload", got)
	assert.False(t, engine.on, "the Opener must not arm the process-wide engine")

	plain, err := o.Decrypt("legacy plaintext")
	require.NoError(t, err)
	assert.Equal(t, "legacy plaintext", plain)
}

func TestOpener_NoKeyPassesPlaintextAndFailsClosedOnCiphertext(t *testing.T) {
	sqlDB, seal := openerDB(t)
	enc := seal(testKEK(t))

	o, err := NewOpener(context.Background(), sqlDB, nil)
	require.NoError(t, err)
	plain, err := o.Decrypt("legacy plaintext")
	require.NoError(t, err)
	assert.Equal(t, "legacy plaintext", plain)
	_, err = o.Decrypt(enc)
	require.ErrorIs(t, err, ErrNotInitialized)
}

func TestOpener_NoDEKRowFailsClosedOnCiphertext(t *testing.T) {
	sqlDB, _ := openerDB(t)

	o, err := NewOpener(context.Background(), sqlDB, testKEK(t))
	require.NoError(t, err)
	_, err = o.Decrypt(ciphertextPrefix + keyID + ":AAAA")
	require.ErrorIs(t, err, ErrNotInitialized)
}

func TestOpener_WrongKeyFailsClosed(t *testing.T) {
	sqlDB, seal := openerDB(t)
	seal(testKEK(t))

	wrong := make([]byte, keySize)
	wrong[0] = 0xFF
	_, err := NewOpener(context.Background(), sqlDB, wrong)
	require.ErrorIs(t, err, ErrAuthFailed)
	assert.Contains(t, err.Error(), "does not unwrap")
}

func TestOpener_MalformedCiphertextFailsClosed(t *testing.T) {
	sqlDB, seal := openerDB(t)
	seal(testKEK(t))

	o, err := NewOpener(context.Background(), sqlDB, testKEK(t))
	require.NoError(t, err)
	_, err = o.Decrypt(ciphertextPrefix + "other:AAAA")
	require.ErrorIs(t, err, ErrMalformed)
}

func TestOpener_UnreadableKeyTableErrors(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "data_encryption_keys")
	sqlDB, err := db.DB()
	require.NoError(t, err)

	_, err = NewOpener(context.Background(), sqlDB, testKEK(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read data_encryption_keys")
}
