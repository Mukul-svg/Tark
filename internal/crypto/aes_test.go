package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	c, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	plaintext := []byte("apiVersion: v1\nkind: Config\nclusters: []")

	encrypted, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if encrypted == string(plaintext) {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, _ := NewCipher(key)

	plaintext := []byte("same input")
	a, _ := c.Encrypt(plaintext)
	b, _ := c.Encrypt(plaintext)

	if a == b {
		t.Error("two encryptions of the same plaintext must produce different ciphertexts (unique nonces)")
	}
}

func TestDecryptFailsWithWrongKey(t *testing.T) {
	keyA := make([]byte, 32)
	keyB := make([]byte, 32)
	_, _ = rand.Read(keyA)
	_, _ = rand.Read(keyB)

	cA, _ := NewCipher(keyA)
	cB, _ := NewCipher(keyB)

	encrypted, _ := cA.Encrypt([]byte("secret kubeconfig"))

	if _, err := cB.Decrypt(encrypted); err == nil {
		t.Error("expected decryption with wrong key to fail")
	}
}

func TestNewCipherRejectsShortKey(t *testing.T) {
	_, err := NewCipher([]byte("too-short"))
	if err == nil {
		t.Error("expected error for key shorter than 32 bytes")
	}
}
