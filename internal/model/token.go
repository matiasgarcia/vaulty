package model

import "time"

type TokenStatus string

const (
	TokenStatusActive   TokenStatus = "active"
	TokenStatusInactive TokenStatus = "inactive"
)

type Token struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant_id"`
	TokenID       string      `json:"token_id"`
	PANBlindIndex string      `json:"pan_blind_index"`
	Status        TokenStatus `json:"status"`
	ExpiryMonth   int         `json:"expiry_month"`
	ExpiryYear    int         `json:"expiry_year"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	LastUsedAt    *time.Time  `json:"last_used_at,omitempty"`
}
