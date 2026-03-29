# Implementation Plan: Multitenant Tokenization

**Branch**: `002-multitenant-tokenization` | **Date**: 2026-03-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-multitenant-tokenization/spec.md`

## Summary

Add multitenancy to the existing PCI token vault. Every operation is
scoped by a mandatory tenant identifier. The same PAN tokenized by
different tenants produces different tokens (tenant-scoped blind index
via `HMAC(tenant_id + PAN)`). Each tenant gets a dedicated KMS key
(KEK) created at provisioning time, ensuring cryptographic isolation.
A new tenant management API (`/admin/tenants`) handles provisioning
and lifecycle. All existing tables gain a `tenant_id` column and
per-tenant unique constraints.

## Technical Context

**Language/Version**: Go 1.22+ (existing codebase)
**Primary Dependencies**: chi v5, pgx v5, go-redis v9, AWS SDK v2 (KMS) — no new dependencies
**Storage**: PostgreSQL 16+ (add tenant_id to existing tables, new tenants table), Redis 7+ (CVV keys prefixed with tenant)
**Testing**: go test + testify + testcontainers-go
**Target Platform**: Linux containers (Kubernetes), distroless base images
**Project Type**: Microservices (Tokenizer + Proxy) — evolution of 001-pci-token-vault
**Performance Goals**: 10,000+ tenants without degradation; same latency targets as baseline
**Constraints**: PCI DSS compliance; per-tenant cryptographic isolation; zero cross-tenant data leakage
**Scale/Scope**: Modifies existing codebase — migrations, models, handlers, repositories, auth middleware

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Pre-Phase 0 | Post-Phase 1 | Evidence |
|-----------|-------------|--------------|----------|
| I. PCI DSS Compliance First | PASS | PASS | Per-tenant KEK reinforces Req 7; data isolation per tenant |
| II. Zero Trust Security | PASS | PASS | Tenant ID required on every request; cross-tenant access rejected; per-tenant RBAC |
| III. Encryption by Default | PASS | PASS | Dedicated KMS key per tenant; envelope encryption unchanged |
| IV. Ephemeral Sensitive Data | PASS | PASS | CVV store keys prefixed with tenant_id — isolation preserved |
| V. Observability & Auditability | PASS | PASS | FR-009: tenant_id in every audit log entry |

**Gate status: PASSED**

## Project Structure

### Documentation (this feature)

```text
specs/002-multitenant-tokenization/
├── plan.md              # This file
├── data-model.md        # Phase 1: Entity changes
├── quickstart.md        # Phase 1: Updated getting started
├── contracts/
│   └── api.yaml         # Phase 1: New/modified endpoints
└── tasks.md             # Phase 2: Task breakdown (via /speckit.tasks)
```

### Source Code (modifications to existing codebase)

```text
# New files
internal/model/tenant.go             # Tenant entity
internal/repository/tenant_repo.go   # Tenant CRUD
internal/handler/tenant.go           # POST/GET/DELETE /admin/tenants
internal/auth/tenant.go              # Tenant extraction middleware

# Modified files
internal/model/token.go              # Add TenantID field
internal/model/audit.go              # Add TenantID field
internal/crypto/hmac.go              # Tenant-scoped blind index
internal/repository/token_repo.go    # Add tenant_id to all queries
internal/repository/vault_repo.go    # Add tenant_id to queries
internal/repository/audit_repo.go    # Add tenant_id to queries
internal/redis/cvv_store.go          # Tenant-prefixed Redis keys
internal/handler/tokenize.go         # Extract tenant, pass to blind index + KMS
internal/handler/detokenize.go       # Validate token belongs to tenant
internal/handler/token_manage.go     # Scope all queries by tenant
internal/handler/forward.go          # Pass tenant to revealer
internal/proxy/revealer.go           # Include tenant header in detokenize calls
internal/auth/middleware.go          # Extract tenant from request
internal/auth/rbac.go                # Add admin role for tenant management
cmd/tokenizer/main.go               # Wire tenant routes + middleware

# New migrations
migrations/004_create_tenants.up.sql
migrations/004_create_tenants.down.sql
migrations/005_add_tenant_id_to_tokens.up.sql
migrations/005_add_tenant_id_to_tokens.down.sql
migrations/006_add_tenant_id_to_vault_entries.up.sql
migrations/006_add_tenant_id_to_vault_entries.down.sql
migrations/007_add_tenant_id_to_audit_log.up.sql
migrations/007_add_tenant_id_to_audit_log.down.sql
```

**Structure Decision**: No new services or packages. Multitenancy is
added as a cross-cutting concern via middleware (tenant extraction) +
model/repository changes (tenant_id column) + new tenant management
handler. The existing monorepo structure is preserved.

## Complexity Tracking

> No Constitution Check violations — no entries needed.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
