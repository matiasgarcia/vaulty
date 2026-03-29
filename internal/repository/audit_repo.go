package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pci-vault/vault/internal/model"
)

type AuditRepo struct {
	pool *pgxpool.Pool
}

func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{pool: pool}
}

func (r *AuditRepo) Append(ctx context.Context, entry *model.AuditLogEntry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_log (tenant_id, correlation_id, operation, token_id_masked, actor, result, detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.TenantID, entry.CorrelationID, entry.Operation, entry.TokenIDMasked,
		entry.Actor, entry.Result, entry.Detail,
	)
	if err != nil {
		return fmt.Errorf("audit append: %w", err)
	}
	return nil
}

func (r *AuditRepo) FindByTokenID(ctx context.Context, tenantID, tokenIDMasked string, limit, offset int) ([]model.AuditLogEntry, int, error) {
	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE tenant_id = $1 AND token_id_masked = $2`,
		tenantID, tokenIDMasked,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("audit count: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, correlation_id, operation, token_id_masked, actor, result, detail, created_at
		 FROM audit_log WHERE tenant_id = $1 AND token_id_masked = $2
		 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		tenantID, tokenIDMasked, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("audit query: %w", err)
	}
	defer rows.Close()

	var entries []model.AuditLogEntry
	for rows.Next() {
		var e model.AuditLogEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.CorrelationID, &e.Operation, &e.TokenIDMasked,
			&e.Actor, &e.Result, &e.Detail, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("audit scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, total, nil
}
