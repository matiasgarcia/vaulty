# Implementation Plan: PCI-Compliant Token Vault & Payment Proxy

**Branch**: `001-pci-token-vault` | **Date**: 2026-03-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-pci-token-vault/spec.md`

## Summary

Build a PCI DSS-compliant tokenization vault and payment proxy system
in Go. The system accepts card data (PAN + CVV), encrypts and stores
PAN with deterministic token mapping (1:1 via HMAC blind index),
holds CVV ephemerally in Redis (TTL 1h, single-use), and exposes a
Payment Proxy that acts as a reveal-and-forward proxy: backend clients
send tokens, the proxy calls the Tokenizer's internal detokenize API
(mTLS-only) to reveal PAN + CVV, then forwards the real card data to
the configured 3rd party PSP. The Proxy has NO database credentials
— it only communicates with the Tokenizer service and the external
PSP. The Tokenizer is the single owner of PostgreSQL and Redis. For
testing, a mock Express app simulates a 3rd party PSP. Envelope
encryption (AES-256-GCM with DEK/KEK via external KMS) secures all
sensitive data at rest.

## Technical Context

**Language/Version**: Go 1.22+
**Primary Dependencies**: chi v5 (HTTP router), pgx v5 (PostgreSQL driver), go-redis v9, AWS SDK for Go v2 (KMS client), slog (stdlib logging), OpenTelemetry
**Storage**: PostgreSQL 16+ (vault/tokens/audit), Redis 7+ (ephemeral CVV)
**Testing**: go test + testify + testcontainers-go
**Target Platform**: Linux containers (Kubernetes), distroless base images
**Project Type**: Microservices (2 services: Tokenizer, Payment Proxy)
**Performance Goals**: 1,000 concurrent tokenization requests; <2s tokenization; <5s payment (p95)
**Constraints**: 99.999% uptime; PCI DSS Req 3,4,7,8,10,11; AES-256-GCM; mTLS between services
**Scale/Scope**: Server-to-server integrations only; single payment provider initially (expandable)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Pre-Phase 0 | Post-Phase 1 | Evidence |
|-----------|-------------|--------------|----------|
| I. PCI DSS Compliance First | PASS | PASS | FR-001–018 cover Req 3,4,7,8,10,11; AES-256-GCM encryption; audit logging |
| II. Zero Trust Security | PASS | PASS | mTLS between services; RBAC middleware; detokenization restricted to Payment Proxy |
| III. Encryption by Default | PASS | PASS | Envelope encryption (DEK+KEK via KMS); unique IV per record; TLS 1.2+ in transit; CVV encrypted in Redis |
| IV. Ephemeral Sensitive Data | PASS | PASS | Redis GETDEL for single-use CVV; configurable TTL (default 1h); PAN decrypted only in-memory within Proxy |
| V. Observability & Auditability | PASS | PASS | slog structured JSON logging; OpenTelemetry tracing; AuditLogEntry table; PAN masked in logs; CVV never logged |

**Gate status: PASSED** — no violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/001-pci-token-vault/
├── plan.md              # This file
├── research.md          # Phase 0: Technology decisions
├── data-model.md        # Phase 1: Entity definitions
├── quickstart.md        # Phase 1: Getting started guide
├── contracts/
│   └── api.yaml         # Phase 1: OpenAPI 3.1 specification
└── tasks.md             # Phase 2: Task breakdown (via /speckit.tasks)
```

### Source Code (repository root)

```text
cmd/
├── tokenizer/main.go       # Tokenization Service entrypoint
├── proxy/main.go            # Payment Proxy entrypoint
└── migrate/main.go          # Database migration CLI

internal/
├── auth/                    # mTLS + Bearer token auth, RBAC middleware
│   ├── middleware.go
│   └── rbac.go
├── crypto/                  # AES-256-GCM encrypt/decrypt, HMAC blind index
│   ├── aes.go
│   ├── hmac.go
│   └── envelope.go
├── handler/                 # HTTP handlers
│   ├── tokenize.go          # POST /vault/tokenize
│   ├── detokenize.go        # POST /internal/detokenize (mTLS-only, Proxy→Tokenizer)
│   ├── forward.go           # POST /proxy/forward (Proxy service — reveal & forward)
│   ├── token_manage.go      # GET/DELETE /vault/tokens/{id}, GET audit
│   └── health.go            # GET /health
├── kms/                     # KMS client (AWS SDK v2, LocalStack in dev)
│   └── client.go
├── model/                   # Domain entities
│   ├── token.go
│   ├── vault_entry.go
│   └── audit.go
├── proxy/                   # Reveal & forward engine
│   ├── revealer.go          # Scans JSON payload for token patterns, calls detokenize
│   └── forwarder.go         # Forwards revealed payload to destination URL
├── repository/              # PostgreSQL data access (pgx)
│   ├── token_repo.go
│   ├── vault_repo.go
│   └── audit_repo.go
├── redis/                   # Redis CVV store
│   └── cvv_store.go
├── server/                  # HTTP server, router, middleware chain
│   ├── router.go
│   └── server.go
└── audit/                   # Audit logger (structured, SIEM-ready)
    └── logger.go

migrations/                  # SQL migration files (golang-migrate)
├── 001_create_tokens.up.sql
├── 001_create_tokens.down.sql
├── 002_create_vault_entries.up.sql
├── 002_create_vault_entries.down.sql
├── 003_create_audit_log.up.sql
└── 003_create_audit_log.down.sql

config/
└── config.go                # Environment-based configuration

docker-compose.yaml          # Local dev: PostgreSQL + Redis + LocalStack (KMS)
Dockerfile.tokenizer         # Multi-stage distroless build
Dockerfile.proxy             # Multi-stage distroless build
go.mod
go.sum
```

test/
└── mock-provider/           # Express app simulating a 3rd party PSP
    ├── package.json
    ├── index.js             # Receives PAN+CVV+amount, returns mock response
    └── Dockerfile
```

**Structure Decision**: Go monorepo with two service entrypoints
(`cmd/tokenizer`, `cmd/proxy`) sharing internal packages. The Proxy
acts as a reveal-and-forward proxy: it receives tokens from backend
clients, detokenizes (reveals PAN + retrieves CVV), and forwards the
real card data to the configured 3rd party payment provider. The
provider is configurable per request — any PSP endpoint can be
targeted. For integration testing, a minimal Express mock provider
simulates a 3rd party PSP without requiring real provider credentials.

## Complexity Tracking

> No Constitution Check violations — no entries needed.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
