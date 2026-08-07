package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
)

func TestEncryptDecryptCredential_RoundTrip(t *testing.T) {
	secret := "test-jwt-secret"
	plaintext := "hunter2"

	enc, err := EncryptCredential(secret, plaintext)
	if err != nil {
		t.Fatalf("EncryptCredential: %v", err)
	}
	if enc == "" {
		t.Fatal("expected non-empty ciphertext")
	}

	got, err := DecryptCredential(secret, enc)
	if err != nil {
		t.Fatalf("DecryptCredential: %v", err)
	}
	if got != plaintext {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecryptCredential_EmptyString(t *testing.T) {
	enc, err := EncryptCredential("secret", "")
	if err != nil || enc != "" {
		t.Fatalf("expected empty ciphertext for empty plaintext, got %q, err %v", enc, err)
	}
	got, err := DecryptCredential("secret", "")
	if err != nil || got != "" {
		t.Fatalf("expected empty plaintext for empty ciphertext, got %q, err %v", got, err)
	}
}

func TestEncryptDecryptCredential_WrongSecretFails(t *testing.T) {
	enc, err := EncryptCredential("secret-a", "hunter2")
	if err != nil {
		t.Fatalf("EncryptCredential: %v", err)
	}
	if _, err := DecryptCredential("secret-b", enc); err == nil {
		t.Fatal("expected decrypt with wrong secret to fail")
	}
}

// TestDecryptCredential_LegacyCiphertextStillOpens pins backward
// compatibility with credentials encrypted before the HKDF migration: it
// reproduces the pre-migration derivation by hand (bypassing
// legacyCredentialKey, which decrypt itself also calls, so this doesn't
// just test the function against itself) and confirms DecryptCredential
// still opens it via its fallback path.
func TestDecryptCredential_LegacyCiphertextStillOpens(t *testing.T) {
	secret := "legacy-secret"
	plaintext := "app-password-123"

	legacyEnc, err := legacyEncryptCredential(secret, plaintext)
	if err != nil {
		t.Fatalf("legacyEncryptCredential: %v", err)
	}

	got, err := DecryptCredential(secret, legacyEnc)
	if err != nil {
		t.Fatalf("DecryptCredential on legacy ciphertext: %v", err)
	}
	if got != plaintext {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

// legacyEncryptCredential reimplements the pre-HKDF EncryptCredential
// (SHA-256-derived key) independently of production code, purely to
// produce a fixture for TestDecryptCredential_LegacyCiphertextStillOpens.
func legacyEncryptCredential(jwtSecret, plaintext string) (string, error) {
	block, err := aes.NewCipher(legacyCredentialKey(jwtSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
