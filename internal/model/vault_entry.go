package model

import "time"

type VaultEntry struct {
	ID            string    `json:"id"`
	TokenID       string    `json:"token_id"`
	PANCiphertext []byte    `json:"-"`
	IV            []byte    `json:"-"`
	AuthTag       []byte    `json:"-"`
	DEKEncrypted  []byte    `json:"-"`
	KMSKeyID      string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
}
