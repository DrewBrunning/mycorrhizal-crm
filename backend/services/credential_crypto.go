package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// credentialKey derives a stable AES-256 key from the JWT secret via
// HKDF-SHA256 (RFC 5869) — a real KDF, not a bare hash, giving proper
// domain separation and construction hygiene over the previous plain
// SHA-256 derivation. Brute-force resistance itself still comes from
// JWT_SECRET_KEY's enforced ≥32-character length (config.Validate), not
// from this function. Stored credentials become undecryptable if
// JWT_SECRET_KEY changes; callers must treat decryption failures as
// "credentials need to be re-entered".
func credentialKey(jwtSecret string) []byte {
	key := make([]byte, 32)
	// info provides domain separation from any other HKDF use of the same
	// secret; err is always nil here since 32 bytes is far under HKDF's
	// per-hash output limit (255 * sha256.Size).
	_, _ = io.ReadFull(hkdf.New(sha256.New, []byte(jwtSecret), nil, []byte("mycorrhizal-credential-encryption-v2")), key)
	return key
}

// legacyCredentialKey reproduces the original (pre-HKDF) derivation:
// SHA-256 of a prefixed secret. Kept only so DecryptCredential can still
// open credentials written before the HKDF switch — new encryptions never
// use this, and nothing re-encrypts old rows proactively, so there is no
// automatic signal for when this is safe to delete. Only remove it once
// you've deliberately confirmed every deployment's stored credentials were
// re-saved (encrypt is called on every update path) since the switch, or
// accepted losing access to the ones that weren't.
func legacyCredentialKey(jwtSecret string) []byte {
	sum := sha256.Sum256([]byte("mycorrhizal-credential-encryption:" + jwtSecret))
	return sum[:]
}

// EncryptCredential encrypts a secret with AES-256-GCM for storage at rest.
// An empty plaintext encrypts to an empty string.
func EncryptCredential(jwtSecret, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(credentialKey(jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptCredential reverses EncryptCredential. It tries the current
// HKDF-derived key first and, only if that fails to authenticate, falls
// back to the pre-migration SHA-256-derived key so credentials encrypted
// before the HKDF switch keep working — GCM's auth tag makes a wrong-key
// attempt fail cleanly rather than yield garbage, so this fallback cannot
// mask corruption as a false success.
func DecryptCredential(jwtSecret, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("stored credential is corrupted")
	}

	plaintext, err := openCredential(raw, credentialKey(jwtSecret))
	if err != nil {
		plaintext, err = openCredential(raw, legacyCredentialKey(jwtSecret))
	}
	if err != nil {
		return "", errors.New("failed to decrypt stored credential (was the JWT secret changed?)")
	}

	return string(plaintext), nil
}

// openCredential AES-GCM-opens raw ciphertext (nonce prefixed, as produced
// by EncryptCredential) under the given key.
func openCredential(raw, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("stored credential is corrupted")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
}
