package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pci-vault/vault/internal/model"
)

type VaultRepo struct {
	pool *pgxpool.Pool
}

func NewVaultRepo(pool *pgxpool.Pool) *VaultRepo {
	return &VaultRepo{pool: pool}
}

func (r *VaultRepo) Create(ctx context.Context, ve *model.VaultEntry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO vault_entries (token_id, pan_ciphertext, iv, auth_tag, dek_encrypted, kms_key_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ve.TokenID, ve.PANCiphertext, ve.IV, ve.AuthTag, ve.DEKEncrypted, ve.KMSKeyID,
	)
	if err != nil {
		return fmt.Errorf("vault entry create: %w", err)
	}
	return nil
}

func (r *VaultRepo) FindByTokenID(ctx context.Context, tokenID string) (*model.VaultEntry, error) {
	var ve model.VaultEntry
	err := r.pool.QueryRow(ctx,
		`SELECT id, token_id, pan_ciphertext, iv, auth_tag, dek_encrypted, kms_key_id, created_at
		 FROM vault_entries WHERE token_id = $1`, tokenID,
	).Scan(&ve.ID, &ve.TokenID, &ve.PANCiphertext, &ve.IV, &ve.AuthTag,
		&ve.DEKEncrypted, &ve.KMSKeyID, &ve.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vault entry find: %w", err)
	}
	return &ve, nil
}
