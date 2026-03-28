package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with a random IV.
// Returns ciphertext, IV (12 bytes), and auth tag (16 bytes appended to ciphertext by GCM).
func Encrypt(plaintext, key []byte) (ciphertext, iv []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("aes new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("aes new gcm: %w", err)
	}

	iv = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, nil, fmt.Errorf("generate iv: %w", err)
	}

	ciphertext = gcm.Seal(nil, iv, plaintext, nil)
	return ciphertext, iv, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM.
// The ciphertext includes the appended auth tag (as produced by GCM Seal).
func Decrypt(ciphertext, iv, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes new gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("aes gcm open: %w", err)
	}

	return plaintext, nil
}
