package crypto

import (
	"crypto/rand"
	"fmt"
)

const dekSize = 32 // AES-256

// GenerateDEK generates a random 256-bit Data Encryption Key.
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, dekSize)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}
	return dek, nil
}

// EncryptWithDEK encrypts plaintext using a provided DEK via AES-256-GCM.
func EncryptWithDEK(plaintext, dek []byte) (ciphertext, iv []byte, err error) {
	return Encrypt(plaintext, dek)
}

// DecryptWithDEK decrypts ciphertext using a provided DEK via AES-256-GCM.
func DecryptWithDEK(ciphertext, iv, dek []byte) ([]byte, error) {
	return Decrypt(ciphertext, iv, dek)
}
