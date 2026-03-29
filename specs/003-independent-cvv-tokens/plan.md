# Implementation Plan: Independent CVV Tokens

**Branch**: `003-independent-cvv-tokens` | **Date**: 2026-03-29 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/003-independent-cvv-tokens/spec.md`

## Summary

Transform the tokenize endpoint from a fixed-schema API into a dynamic echo model where the system scans the request body for known sensitive fields (`pan`, `cvv`), tokenizes each independently, and returns the full body with sensitive values replaced by tokens. PAN tokens remain permanent and deterministic (PostgreSQL). CVV tokens are new: ephemeral, single-use, stored in Redis with configurable TTL (default 1h). The proxy's detokenize flow must resolve both token types — PAN tokens to the real PAN string, CVV tokens to the real CVV string — enabling clients to place them in arbitrary payload positions.

## Technical Context

**Language/Version**: Go 1.22+ (existing codebase)
**Primary Dependencies**: chi v5, pgx v5, go-redis v9, AWS SDK v2 (KMS) — no new dependencies
**Storage**: PostgreSQL 16+ (existing tokens/vault_entries tables unchanged), Redis 7+ (new CVV token keys alongside existing CVV keys)
**Testing**: go test with `//go:build integration` tag, testcontainers-go (PostgreSQL, Redis, LocalStack)
**Target Platform**: Linux server (Docker/Kubernetes)
**Project Type**: Web service (PCI-compliant tokenization vault)
**Performance Goals**: Tokenization response < 2s end-to-end, 1000 concurrent requests
**Constraints**: PCI DSS compliance (Requirements 3, 4, 7, 8, 10, 11), CVV max TTL 1h, single-use CVV
**Scale/Scope**: Multi-tenant, per-tenant KMS keys

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. PCI DSS Compliance First | PASS | CVV tokens retain TTL and single-use semantics per PCI DSS Req 3.2. PAN encryption unchanged. No new plaintext storage. |
| II. Zero Trust Security | PASS | CVV tokens tenant-scoped. Detokenization still requires mTLS (proxy only). RBAC unchanged. |
| III. Encryption by Default | PASS | CVV tokens store encrypted CVV in Redis (same AES-256-GCM + local DEK pattern). No plaintext CVV in Redis. |
| IV. Ephemeral Sensitive Data | PASS | CVV tokens have configurable TTL (default 1h, max 1h). Single-use via GETDEL. PAN never in plaintext at rest. |
| V. Observability & Auditability | PASS | CVV token creation and revelation logged in audit trail. CVV values never logged. Token IDs masked in logs. |

**Gate result**: PASS — all five principles satisfied. No violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/003-independent-cvv-tokens/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── api.yaml         # Updated OpenAPI contract
└── tasks.md             # Phase 2 output (via /speckit.tasks)
```

### Source Code (repository root)

```text
cmd/
├── tokenizer/main.go       # No changes needed
├── proxy/main.go            # No changes needed
└── migrate/main.go          # No changes needed

internal/
├── handler/
│   ├── tokenize.go          # MAJOR CHANGE: dynamic echo model, CVV token generation
│   └── detokenize.go        # CHANGE: resolve CVV tokens (return plain string) + PAN tokens
├── redis/
│   └── cvv_store.go         # CHANGE: new methods for CVV token storage (keyed by cvv_token_id)
├── proxy/
│   └── revealer.go          # CHANGE: detokenize returns plain string value, simplify replacement
├── model/                   # No changes expected (CVV tokens are Redis-only)
├── crypto/                  # No changes (reuse existing AES-256-GCM + DEK pattern)
├── auth/                    # No changes
├── server/                  # No changes
├── repository/              # No changes (PAN token flow unchanged)
├── kms/                     # No changes
└── audit/                   # No changes (reuse existing audit logging)

migrations/                  # No new migrations (CVV tokens are Redis-only)

tests/
└── integration/
    ├── tokenize_test.go     # UPDATE: test dynamic echo, CVV-only, PAN-only
    ├── detokenize_test.go   # UPDATE: test CVV token resolution
    ├── forward_test.go      # UPDATE: test independent CVV token in payloads
    └── helpers_test.go      # Minor updates if needed
```

**Structure Decision**: No new packages or directories. Changes contained within existing handler, redis, and proxy packages. CVV tokens are ephemeral (Redis-only), so no new database tables or migrations.

## Complexity Tracking

No violations to justify. The implementation extends existing patterns without new architectural complexity.
