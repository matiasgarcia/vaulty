# Data Model: Multitenant Tokenization

**Branch**: `002-multitenant-tokenization` | **Date**: 2026-03-28

## New Entity: Tenant

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, generated | Internal row identifier |
| tenant_id | string(64) | UNIQUE, NOT NULL, indexed | Public-facing tenant identifier |
| name | string(256) | NOT NULL | Tenant display name |
| status | enum | NOT NULL, default=active | `active` or `inactive` |
| kms_key_arn | string(256) | NOT NULL | Dedicated KMS key ARN for this tenant |
| created_at | timestamp | NOT NULL, default=now | Creation timestamp (UTC) |
| updated_at | timestamp | NOT NULL, default=now | Last update timestamp (UTC) |

**State transitions**:
```
[created] → active → inactive (reversible)
                    → active (re-activation)
```

---

## Modified Entity: Token

Added `tenant_id` column. Uniqueness constraint on `pan_blind_index`
changes from global to per-tenant.

| Field | Change | Description |
|-------|--------|-------------|
| tenant_id | **NEW** VARCHAR(64) NOT NULL, FK → tenants.tenant_id | Owning tenant |

**Index changes**:
- DROP: `idx_tokens_pan_blind_index` (global unique)
- ADD: `idx_tokens_tenant_blind_index` UNIQUE (tenant_id, pan_blind_index)
- ADD: `idx_tokens_tenant_id` (tenant_id) for filtered queries

---

## Modified Entity: Vault Entry

Added `tenant_id` for query filtering (denormalized from Token for
direct access without join).

| Field | Change | Description |
|-------|--------|-------------|
| tenant_id | **NEW** VARCHAR(64) NOT NULL | Owning tenant (denormalized) |

---

## Modified Entity: Audit Log Entry

Added `tenant_id` for tenant-scoped audit queries.

| Field | Change | Description |
|-------|--------|-------------|
| tenant_id | **NEW** VARCHAR(64) NOT NULL, indexed | Tenant that performed the operation |

---

## CVV Record (Redis) — Key Format Change

Old format: `cvv:{token_id}`
New format: `cvv:{tenant_id}:{token_id}`

No schema change — just key prefix update for namespace isolation.

---

## Blind Index Change

Old: `HMAC(PAN, hmac_key)`
New: `HMAC(tenant_id + ":" + PAN, hmac_key)`

This ensures the same PAN produces different blind indexes (and
therefore different tokens) for different tenants.

---

## Relationships

```
Tenant 1:N Token         (tenant_id FK)
Tenant 1:N VaultEntry    (tenant_id denormalized)
Tenant 1:N AuditLogEntry (tenant_id)
Token  1:1 VaultEntry    (token_id FK)
Token  1:1 CVVRecord     (tenant_id:token_id in Redis key)
```

## Migration Strategy

1. Create `tenants` table (migration 004)
2. Add `tenant_id` to `tokens` with DEFAULT for existing rows (migration 005)
3. Add `tenant_id` to `vault_entries` (migration 006)
4. Add `tenant_id` to `audit_log` (migration 007)
5. Existing data gets assigned to a `default` tenant created in migration 004
6. After migration, remove DEFAULT — tenant_id becomes required
