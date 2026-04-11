package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// Cipher wraps an AES-256-GCM AEAD to encrypt and decrypt kubeconfig payloads.
// The nonce is randomly generated per encryption and prepended to the ciphertext.
// The combined bytes are base64-encoded before storage and decoded before decryption.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher initialises a Cipher from a 32-byte (AES-256) key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be exactly 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}

	return &Cipher{aead: aead}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64-encoded string
// of the form: base64(nonce || ciphertext). A fresh random nonce is used per call.
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends the encrypted data to nonce, producing: nonce || ciphertext || tag
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes a base64-encoded ciphertext produced by Encrypt and decrypts it.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt failed (wrong key or corrupted data): %w", err)
	}

	return plaintext, nil
}
