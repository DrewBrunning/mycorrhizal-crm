package atrest

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// testKEK returns a deterministic 32-byte master key for tests.
func testKEK(t *testing.T) []byte {
	t.Helper()
	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	return kek
}

// arm arms the engine with the test KEK as both master key and DEK (the pure
// crypto tests don't touch the data_encryption_keys table, so the DEK is just
// the KEK bytes). disarm undoes it.
func arm(t *testing.T) {
	t.Helper()
	armForTest(testKEK(t), testKEK(t))
	t.Cleanup(ResetForTest)
}

// armWith arms with an explicit master key (for wrong-key tests).
func armWith(t *testing.T, kek []byte) {
	t.Helper()
	armForTest(kek, kek)
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	arm(t)

	plain := "sensitive note body with PII"
	ct, err := Encrypt(plain)
	require.NoError(t, err)
	require.NotEqual(t, plain, ct)
	require.True(t, len(ct) > len(plain))

	got, err := Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, plain, got)
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	arm(t)

	ct, err := Encrypt("")
	require.NoError(t, err)
	require.Equal(t, "", ct)

	got, err := Decrypt("")
	require.NoError(t, err)
	require.Equal(t, "", got)
}

func TestEncrypt_NullPayloadIsNotEncrypted(t *testing.T) {
	arm(t)

	// GORM's JSON serializer stores NULL/zero structs as NULL or the string
	// "null" — neither must be encrypted, or the "null" round-trip breaks.
	ct, err := Encrypt("null")
	require.NoError(t, err)
	require.NotEqual(t, "null", ct)

	got, err := Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, "null", got)
}

func TestDecrypt_LegacyPlaintextPassesThrough(t *testing.T) {
	arm(t)

	// A value without the prefix is a pre-backfill row — pass it through.
	got, err := Decrypt("plaintext from before encryption")
	require.NoError(t, err)
	require.Equal(t, "plaintext from before encryption", got)
}

func TestEncryptDecrypt_UninitializedIsTransparent(t *testing.T) {
	ResetForTest()

	got, err := Encrypt("anything")
	require.NoError(t, err)
	require.Equal(t, "anything", got)

	got, err = Decrypt("anything")
	require.NoError(t, err)
	require.Equal(t, "anything", got)
}

func TestDecrypt_WrongKeyFailsClosed(t *testing.T) {
	arm(t)
	ct, err := Encrypt("top secret")
	require.NoError(t, err)

	// A different key must fail, never return garbage.
	other := make([]byte, keySize)
	other[0] = 0xFF
	armWith(t, other)

	_, err = Decrypt(ct)
	require.Error(t, err, "decryption with a wrong key must fail closed")
}

func TestDecrypt_CorruptedCiphertextFailsClosed(t *testing.T) {
	arm(t)
	ct, err := Encrypt("top secret")
	require.NoError(t, err)

	// Flip a byte in the middle of the base64 payload. (Flipping the LAST
	// character is nondeterministic: base64's final-character low bits are
	// ignored, so some flips decode to the same bytes.)
	buf := []byte(ct)
	mid := len(buf) / 2
	if buf[mid] == 'A' {
		buf[mid] = 'B'
	} else {
		buf[mid] = 'A'
	}
	_, err = Decrypt(string(buf))
	require.Error(t, err, "tampered ciphertext must fail authentication")
}

func TestDecrypt_MalformedValueFailsClosed(t *testing.T) {
	arm(t)

	for _, bad := range []string{"encv1:main:%%%notbase64", "encv1:other:AAAA", "encv1:main:", "encv1:"} {
		_, err := Decrypt(bad)
		require.Error(t, err, "malformed value %q must fail closed", bad)
	}
}

func TestDecrypt_EncryptedValueWhenUninitializedFailsClosed(t *testing.T) {
	arm(t)
	ct, err := Encrypt("secret")
	require.NoError(t, err)

	// Shut down the layer: a read of an encrypted value must fail closed
	// rather than return ciphertext as if it were plaintext.
	ResetForTest()
	_, err = Decrypt(ct)
	require.Error(t, err)
}

func TestEncrypt_NonceUniqueness(t *testing.T) {
	arm(t)

	a, err := Encrypt("same plaintext")
	require.NoError(t, err)
	b, err := Encrypt("same plaintext")
	require.NoError(t, err)
	require.NotEqual(t, a, b, "two encryptions of the same plaintext must differ (fresh nonce)")
}

func TestEncryptionKey_FromEnvVar(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(testKEK(t))
	t.Setenv("DATA_ENCRYPTION_KEY", key)
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
	t.Setenv("JWT_SECRET_KEY", "")

	kek, err := EncryptionKey()
	require.NoError(t, err)
	require.Equal(t, testKEK(t), kek)
}

func TestEncryptionKey_FromFile(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("JWT_SECRET_KEY", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	key := base64.StdEncoding.EncodeToString(testKEK(t))
	require.NoError(t, os.WriteFile(path, []byte(key), 0o600))
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", path)

	kek, err := EncryptionKey()
	require.NoError(t, err)
	require.Equal(t, testKEK(t), kek)
}

func TestEncryptionKey_FromJWTFallback(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
	t.Setenv("JWT_SECRET_KEY", "a-jwt-secret-that-is-long-enough-12345")

	kek, err := EncryptionKey()
	require.NoError(t, err)
	require.NotNil(t, kek)
	require.Len(t, kek, keySize)
}

func TestEncryptionKey_NoneConfigured(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
	t.Setenv("JWT_SECRET_KEY", "")

	kek, err := EncryptionKey()
	require.NoError(t, err)
	require.Nil(t, kek)
}

func TestEncryptionKey_InvalidBase64Rejected(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "not-valid-base64!!!")
	_, err := EncryptionKey()
	require.Error(t, err)
}

func TestDecodeMasterKey(t *testing.T) {
	kek, err := DecodeMasterKey(base64.StdEncoding.EncodeToString(testKEK(t)))
	require.NoError(t, err)
	require.Equal(t, testKEK(t), kek)

	_, err = DecodeMasterKey("tooshort")
	require.Error(t, err, "must reject a non-base64 / non-32-byte key")
}
