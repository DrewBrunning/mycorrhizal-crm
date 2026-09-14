package atrest

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeriveBackupSigningKey covers the snapshot-signing key derivation
// (issue #943): it must be deterministic, key-size, and domain-separated from
// the master key it is derived from.
func TestDeriveBackupSigningKey(t *testing.T) {
	master := bytes.Repeat([]byte{0x42}, keySize)

	got := DeriveBackupSigningKey(master)
	require.Len(t, got, keySize)
	assert.Equal(t, got, DeriveBackupSigningKey(master), "derivation must be deterministic")
	assert.NotEqual(t, master, got, "the signing key must not be the master key itself")

	other := bytes.Repeat([]byte{0x43}, keySize)
	assert.NotEqual(t, got, DeriveBackupSigningKey(other), "different master keys must yield different signing keys")

	// The signing key must differ from the KEK derived from the same seed via
	// the JWT path: HKDF info strings are what keep the uses separate.
	fromJWT := deriveKEKFromJWT(string(master))
	assert.NotEqual(t, fromJWT, DeriveBackupSigningKey(fromJWT),
		"signing key must be domain-separated from the KEK it is derived from")
}

func TestDeriveBackupSigningKeyNilMaster(t *testing.T) {
	assert.Nil(t, DeriveBackupSigningKey(nil))
}

// TestBackupSigningKeyResolvesFromEnv pins the env precedence and the
// zero-config JWT fallback, mirroring EncryptionKey.
func TestBackupSigningKeyResolvesFromEnv(t *testing.T) {
	rawKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, keySize))
	decoded, err := base64.StdEncoding.DecodeString(rawKey)
	require.NoError(t, err)

	t.Run("DATA_ENCRYPTION_KEY wins", func(t *testing.T) {
		t.Setenv("DATA_ENCRYPTION_KEY", rawKey)
		t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
		t.Setenv("JWT_SECRET_KEY", "some-jwt-secret-that-should-be-ignored")
		got, err := BackupSigningKey()
		require.NoError(t, err)
		assert.Equal(t, DeriveBackupSigningKey(decoded), got)
	})

	t.Run("JWT_SECRET_KEY fallback derives the same key EncryptionKey would", func(t *testing.T) {
		t.Setenv("DATA_ENCRYPTION_KEY", "")
		t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
		t.Setenv("JWT_SECRET_KEY", "fallback-secret")
		got, err := BackupSigningKey()
		require.NoError(t, err)
		assert.Equal(t, DeriveBackupSigningKey(deriveKEKFromJWT("fallback-secret")), got)
	})

	t.Run("no key configured yields nil", func(t *testing.T) {
		t.Setenv("DATA_ENCRYPTION_KEY", "")
		t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
		t.Setenv("JWT_SECRET_KEY", "")
		got, err := BackupSigningKey()
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("a malformed DATA_ENCRYPTION_KEY is an error, not a silent nil", func(t *testing.T) {
		t.Setenv("DATA_ENCRYPTION_KEY", "not-base64!!")
		got, err := BackupSigningKey()
		require.Error(t, err)
		assert.Nil(t, got)
	})
}
