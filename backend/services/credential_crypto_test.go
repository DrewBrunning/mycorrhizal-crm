package services

import "testing"

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
