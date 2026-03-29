package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pci-vault/vault/internal/model"
)

type TenantRepo struct {
	pool *pgxpool.Pool
}

func NewTenantRepo(pool *pgxpool.Pool) *TenantRepo {
	return &TenantRepo{pool: pool}
}

func (r *TenantRepo) Create(ctx context.Context, t *model.Tenant) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tenants (tenant_id, name, status, kms_key_arn)
		 VALUES ($1, $2, $3, $4)`,
		t.TenantID, t.Name, t.Status, t.KMSKeyARN,
	)
	if err != nil {
		return fmt.Errorf("tenant create: %w", err)
	}
	return nil
}

func (r *TenantRepo) FindByTenantID(ctx context.Context, tenantID string) (*model.Tenant, error) {
	var t model.Tenant
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, status, kms_key_arn, created_at, updated_at
		 FROM tenants WHERE tenant_id = $1`, tenantID,
	).Scan(&t.ID, &t.TenantID, &t.Name, &t.Status, &t.KMSKeyARN, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tenant find: %w", err)
	}
	return &t, nil
}

func (r *TenantRepo) List(ctx context.Context, limit, offset int) ([]model.Tenant, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("tenant count: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, name, status, kms_key_arn, created_at, updated_at
		 FROM tenants ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("tenant list: %w", err)
	}
	defer rows.Close()

	var tenants []model.Tenant
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Status, &t.KMSKeyARN, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("tenant scan: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, total, nil
}

func (r *TenantRepo) UpdateStatus(ctx context.Context, tenantID string, status model.TenantStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tenants SET status = $1, updated_at = NOW() WHERE tenant_id = $2`,
		status, tenantID,
	)
	if err != nil {
		return fmt.Errorf("tenant update status: %w", err)
	}
	return nil
}
