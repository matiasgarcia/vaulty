package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pci-vault/vault/internal/model"
)

type TokenRepo struct {
	pool *pgxpool.Pool
}

func NewTokenRepo(pool *pgxpool.Pool) *TokenRepo {
	return &TokenRepo{pool: pool}
}

func (r *TokenRepo) Create(ctx context.Context, t *model.Token) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tokens (token_id, pan_blind_index, status, expiry_month, expiry_year)
		 VALUES ($1, $2, $3, $4, $5)`,
		t.TokenID, t.PANBlindIndex, t.Status, t.ExpiryMonth, t.ExpiryYear,
	)
	if err != nil {
		return fmt.Errorf("token create: %w", err)
	}
	return nil
}

func (r *TokenRepo) FindByBlindIndex(ctx context.Context, blindIndex string) (*model.Token, error) {
	return r.scanOne(ctx,
		`SELECT id, token_id, pan_blind_index, status, expiry_month, expiry_year,
		        created_at, updated_at, last_used_at
		 FROM tokens WHERE pan_blind_index = $1`, blindIndex)
}

func (r *TokenRepo) FindByTokenID(ctx context.Context, tokenID string) (*model.Token, error) {
	return r.scanOne(ctx,
		`SELECT id, token_id, pan_blind_index, status, expiry_month, expiry_year,
		        created_at, updated_at, last_used_at
		 FROM tokens WHERE token_id = $1`, tokenID)
}

func (r *TokenRepo) UpdateStatus(ctx context.Context, tokenID string, status model.TokenStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tokens SET status = $1, updated_at = NOW() WHERE token_id = $2`,
		status, tokenID,
	)
	if err != nil {
		return fmt.Errorf("token update status: %w", err)
	}
	return nil
}

func (r *TokenRepo) UpdateExpiry(ctx context.Context, tokenID string, month, year int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tokens SET expiry_month = $1, expiry_year = $2, updated_at = NOW()
		 WHERE token_id = $3`,
		month, year, tokenID,
	)
	if err != nil {
		return fmt.Errorf("token update expiry: %w", err)
	}
	return nil
}

func (r *TokenRepo) UpdateLastUsed(ctx context.Context, tokenID string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`UPDATE tokens SET last_used_at = $1, updated_at = $1 WHERE token_id = $2`,
		now, tokenID,
	)
	if err != nil {
		return fmt.Errorf("token update last used: %w", err)
	}
	return nil
}

func (r *TokenRepo) scanOne(ctx context.Context, query string, args ...interface{}) (*model.Token, error) {
	var t model.Token
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.TokenID, &t.PANBlindIndex, &t.Status,
		&t.ExpiryMonth, &t.ExpiryYear,
		&t.CreatedAt, &t.UpdatedAt, &t.LastUsedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("token scan: %w", err)
	}
	return &t, nil
}
