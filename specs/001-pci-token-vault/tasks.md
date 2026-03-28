# Tasks: PCI-Compliant Token Vault & Payment Proxy

**Input**: Design documents from `/specs/001-pci-token-vault/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api.yaml

**Tests**: Not explicitly requested in spec — test tasks omitted. Integration tests can be added via `/speckit.checklist`.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1, US2, US3, US4)
- Exact file paths included in descriptions

---

## Phase 1: Setup

**Purpose**: Project initialization, dependencies, and local dev environment

- [ ] T001 Initialize Go module with `go mod init` and add dependencies (chi v5, pgx v5, go-redis v9, testify, golang-migrate, slog, otel) in `go.mod`
- [ ] T002 Create project directory structure per plan.md: `cmd/tokenizer/`, `cmd/proxy/`, `cmd/migrate/`, `internal/` (auth, crypto, handler, kms, model, provider, redis, repository, server, audit), `migrations/`, `config/`, `test/mock-provider/`
- [ ] T003 [P] Create environment-based configuration loader in `config/config.go` (DATABASE_URL, REDIS_URL, KMS_KEY_ARN, HMAC_KEY, CVV_TTL, PORT_TOKENIZER, PORT_PROXY, PROVIDER_BASE_URL, LOG_LEVEL, LOG_FORMAT)
- [ ] T004 [P] Create `docker-compose.yaml` with PostgreSQL 16, Redis 7, LocalStack (KMS on port 4566), and mock-provider services
- [ ] T005 [P] Create mock PSP Express app in `test/mock-provider/` (package.json, index.js, Dockerfile) — receives PAN+CVV+amount on POST /charge, returns mock success/failure response

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T006 Define domain model structs in `internal/model/token.go` (Token with token_id, pan_blind_index, status, expiry_month, expiry_year, timestamps)
- [ ] T007 [P] Define domain model structs in `internal/model/vault_entry.go` (VaultEntry with token_id, pan_ciphertext, iv, auth_tag, dek_encrypted, kms_key_id)
- [ ] T008 [P] Define domain model structs in `internal/model/payment.go` (PaymentTransaction with token_id, idempotency_key, amount, currency, status, provider fields, cvv_used)
- [ ] T009 [P] Define domain model structs in `internal/model/audit.go` (AuditLogEntry with correlation_id, operation, token_id_masked, actor, result, detail)
- [ ] T010 Implement AES-256-GCM encrypt/decrypt functions in `internal/crypto/aes.go` (Encrypt(plaintext, key) → ciphertext+iv+tag; Decrypt(ciphertext, iv, tag, key) → plaintext; unique IV via crypto/rand per call)
- [ ] T011 [P] Implement HMAC-SHA256 blind index function in `internal/crypto/hmac.go` (ComputeBlindIndex(pan, hmacKey) → hex string for deterministic PAN lookup)
- [ ] T012 [P] Implement envelope encryption helpers in `internal/crypto/envelope.go` (GenerateDEK, EncryptWithDEK, DecryptWithDEK — wraps aes.go with DEK generation)
- [ ] T013 Implement KMS client using AWS SDK v2 in `internal/kms/client.go` (WrapKey(dek) → encrypted_dek via KMS Encrypt; UnwrapKey(encrypted_dek) → dek via KMS Decrypt; GenerateDataKey for new DEKs; configurable endpoint for LocalStack in dev via KMS_ENDPOINT env var)
- [ ] T014 Create SQL migration 001: tokens table in `migrations/001_create_tokens.up.sql` and `migrations/001_create_tokens.down.sql` per data-model.md (indexes on token_id, pan_blind_index, status)
- [ ] T015 [P] Create SQL migration 002: vault_entries table in `migrations/002_create_vault_entries.up.sql` and `migrations/002_create_vault_entries.down.sql` (FK to tokens.token_id, unique constraint)
- [ ] T016 [P] Create SQL migration 003: payment_transactions table in `migrations/003_create_payment_transactions.up.sql` and `migrations/003_create_payment_transactions.down.sql` (unique idempotency_key index, FK to tokens.token_id)
- [ ] T017 [P] Create SQL migration 004: audit_log table in `migrations/004_create_audit_log.up.sql` and `migrations/004_create_audit_log.down.sql` (append-only, index on correlation_id)
- [ ] T018 Implement migration CLI entrypoint in `cmd/migrate/main.go` using golang-migrate (up, down, version commands against DATABASE_URL)
- [ ] T019 Implement token repository in `internal/repository/token_repo.go` (Create, FindByBlindIndex, FindByTokenID, UpdateStatus, UpdateLastUsed — using pgx v5 and pgxpool)
- [ ] T020 [P] Implement vault entry repository in `internal/repository/vault_repo.go` (Create, FindByTokenID — pgx v5)
- [ ] T021 [P] Implement audit log repository in `internal/repository/audit_repo.go` (Append — insert only, no update/delete; FindByTokenID with pagination)
- [ ] T022 Implement Redis CVV store in `internal/redis/cvv_store.go` (Store(tokenID, encryptedCVV, ttl); Retrieve(tokenID) → encryptedCVV using GETDEL for atomic single-use; uses go-redis v9)
- [ ] T023 Implement structured audit logger in `internal/audit/logger.go` (wraps repository + slog; masks PAN; ensures CVV never logged; adds correlation_id from context)
- [ ] T024 Implement structured error response types in `internal/server/errors.go` (ErrorResponse struct with code, message, correlation_id; helper functions for 400, 401, 403, 404, 502, 503)
- [ ] T025 Implement HTTP server setup and base router in `internal/server/server.go` and `internal/server/router.go` (chi router, graceful shutdown, request ID middleware, structured logging middleware, recovery middleware)
- [ ] T026 [P] Implement health check handler in `internal/handler/health.go` (GET /health — checks PostgreSQL ping, Redis ping, KMS availability; returns HealthResponse per api.yaml)

**Checkpoint**: Foundation ready — all shared infrastructure in place. User story implementation can begin.

---

## Phase 3: User Story 1 — Tokenize Payment Card Data (Priority: P1) MVP

**Goal**: Accept PAN + expiry + CVV, validate, encrypt, store, return deterministic token. Same PAN returns same token with CVV/expiry update.

**Independent Test**: POST /vault/tokenize with valid PAN → receive token; POST again with same PAN → receive same token; verify PAN stored encrypted in DB; verify CVV in Redis with TTL.

### Implementation for User Story 1

- [ ] T027 [US1] Implement Luhn validation helper in `internal/handler/tokenize.go` (validatePAN function — validates digit count 13-19 and Luhn checksum)
- [ ] T028 [US1] Implement token generation function in `internal/handler/tokenize.go` (generateTokenID — produces `tok_` prefixed unique non-reversible identifier using crypto/rand)
- [ ] T029 [US1] Implement tokenize handler in `internal/handler/tokenize.go` (POST /vault/tokenize per api.yaml: parse request → validate PAN (Luhn) → compute blind index → check if PAN exists → if exists: return existing token + update CVV/expiry → if new: generate token, encrypt PAN with envelope encryption via KMS, store Token + VaultEntry in DB transaction, store CVV in Redis with TTL → return TokenizeResponse → log audit entry)
- [ ] T030 [US1] Wire tokenize route in `internal/server/router.go` (POST /vault/tokenize → tokenize handler)
- [ ] T031 [US1] Create Tokenizer service entrypoint in `cmd/tokenizer/main.go` (load config, init pgxpool, init Redis client, init KMS client, build router, start HTTP server with graceful shutdown)

**Checkpoint**: User Story 1 complete — tokenization works end-to-end. Cards can be securely stored and tokens returned.

---

## Phase 4: User Story 2 — Process Payment Using Token (Priority: P2)

**Goal**: Proxy receives token + amount + currency, calls Tokenizer's internal detokenize API to reveal PAN + CVV, forwards to 3rd party PSP, returns result. Supports idempotency.

**Independent Test**: Tokenize a card (US1), then POST /proxy/charge with token → Proxy calls /internal/detokenize on Tokenizer → receives PAN+CVV → forwards to mock PSP → returns result. Retry same idempotency key → returns cached result.

### Implementation for User Story 2

- [ ] T032 [US2] Implement payment transaction repository in `internal/repository/payment_repo.go` (Create, FindByIdempotencyKey, UpdateStatus — pgx v5)
- [ ] T033 [US2] Implement detokenize handler in `internal/handler/detokenize.go` (POST /internal/detokenize on Tokenizer service: validate token exists + active → decrypt PAN from vault entry via KMS unwrap + AES decrypt → retrieve CVV from Redis via GETDEL → return raw PAN + expiry + CVV nullable → log audit entry; mTLS-only enforcement)
- [ ] T034 [US2] Implement payment record handler in `internal/handler/payment_record.go` (POST /internal/payments on Tokenizer service: check idempotency_key → if exists: return cached ChargeResponse → if new: create PaymentTransaction record → return 201 with ChargeResponse → log audit entry; mTLS-only)
- [ ] T035 [US2] Wire internal routes in `internal/server/router.go` (POST /internal/detokenize and POST /internal/payments → respective handlers, restricted to mTLS-authenticated Proxy cert)
- [ ] T035 [US2] Implement provider adapter interface in `internal/provider/adapter.go` (PaymentProvider interface: Charge(pan, cvv, expiry, amount, currency) → ProviderResponse)
- [ ] T036 [US2] Implement generic HTTP forwarder in `internal/provider/http_forwarder.go` (implements PaymentProvider interface; POSTs raw card data + amount to configurable PROVIDER_BASE_URL; parses response; wipes PAN/CVV from memory after send)
- [ ] T037 [US2] Implement provider configuration in `internal/provider/config.go` (load provider endpoint + credentials from config; support selecting provider per request or default)
- [ ] T038 [US2] Implement charge handler in `internal/handler/charge.go` (POST /proxy/charge per api.yaml: parse request → check idempotency key → if exists: return cached result → call Tokenizer /internal/detokenize via mTLS HTTP client → receive PAN+CVV → forward to PSP via provider adapter → store PaymentTransaction → wipe card data from memory → return ChargeResponse → log audit entry)
- [ ] T039 [US2] Wire charge route in `internal/server/router.go` (POST /proxy/charge → charge handler)
- [ ] T040 [US2] Create Proxy service entrypoint in `cmd/proxy/main.go` (load config, init mTLS HTTP client for Tokenizer, init provider adapter, build router — NO database pool, NO Redis client — start HTTP server with graceful shutdown)

**Checkpoint**: User Stories 1 AND 2 complete — full tokenize → pay flow works end-to-end with mock PSP.

---

## Phase 5: User Story 3 — Manage Token Lifecycle (Priority: P3)

**Goal**: Validate token status, deactivate tokens (soft-delete), view audit trail per token.

**Independent Test**: Tokenize a card → GET /vault/tokens/{id} returns active → DELETE /vault/tokens/{id} → GET returns inactive → POST /proxy/charge with deactivated token → rejected → GET /vault/tokens/{id}/audit → returns all operations.

### Implementation for User Story 3

- [ ] T041 [US3] Implement get token status handler in `internal/handler/token_manage.go` (GET /vault/tokens/{token_id} per api.yaml: lookup by token_id → return TokenStatusResponse or 404 → log audit entry)
- [ ] T042 [US3] Implement deactivate token handler in `internal/handler/token_manage.go` (DELETE /vault/tokens/{token_id} per api.yaml: set status=inactive → return updated TokenStatusResponse or 404 → log audit entry)
- [ ] T043 [US3] Implement audit trail handler in `internal/handler/token_manage.go` (GET /vault/tokens/{token_id}/audit per api.yaml: query audit_repo with pagination → return AuditTrailResponse)
- [ ] T044 [US3] Wire token management routes in `internal/server/router.go` (GET and DELETE /vault/tokens/{token_id}, GET /vault/tokens/{token_id}/audit)

**Checkpoint**: User Stories 1, 2, AND 3 complete — full lifecycle management available.

---

## Phase 6: User Story 4 — Authenticate and Authorize Access (Priority: P4)

**Goal**: All endpoints require authentication. RBAC restricts operations per service role. /internal/detokenize only accessible via Proxy mTLS cert.

**Independent Test**: Call any endpoint without auth → 401. Call /internal/detokenize from non-Proxy cert → 403. Call /vault/tokenize with valid auth + "tokenize" role → 200. Call /proxy/charge with valid auth + "payment" role → 200.

### Implementation for User Story 4

- [ ] T045 [US4] Implement Bearer token authentication middleware in `internal/auth/middleware.go` (extract and validate Bearer token from Authorization header; inject service identity into request context; return 401 on failure)
- [ ] T046 [US4] Implement RBAC authorization middleware in `internal/auth/rbac.go` (define roles: tokenize, payment, manage, detokenize; check service identity against required role per route; return 403 on insufficient permissions)
- [ ] T047 [US4] Implement mTLS certificate validation for /internal/detokenize in `internal/auth/middleware.go` (extract client cert CN from TLS connection state; verify CN matches expected Proxy service identity; return 403 if non-Proxy cert)
- [ ] T048 [US4] Wire auth + RBAC middleware into all routes in `internal/server/router.go` (apply Bearer auth globally; apply role-specific RBAC per route group: /vault/* requires "tokenize" or "manage" role, /proxy/* requires "payment" role, /internal/detokenize requires "detokenize" role + mTLS cert check; /health exempt from auth)

**Checkpoint**: All user stories complete — system is fully authenticated and authorized.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Containerization, documentation, and production readiness

- [ ] T049 [P] Create multi-stage Dockerfile for Tokenizer in `Dockerfile.tokenizer` (build stage: golang:1.22-alpine, CGO_ENABLED=0; runtime stage: gcr.io/distroless/static-debian12; COPY binary; expose PORT_TOKENIZER)
- [ ] T050 [P] Create multi-stage Dockerfile for Proxy in `Dockerfile.proxy` (same pattern; expose PORT_PROXY; NO database env vars)
- [ ] T051 [P] Add OpenTelemetry tracing middleware in `internal/server/router.go` (inject trace_id + span_id into context and logs; propagate correlation_id across Proxy→Tokenizer calls)
- [ ] T052 Validate quickstart.md flow end-to-end: `docker compose up` → run migrations → tokenize → charge → verify mock PSP received PAN+CVV → verify audit trail
- [ ] T053 [P] Add `.env.example` with all required environment variables documented
- [ ] T054 Security review: verify PAN never in logs (grep for plaintext patterns), CVV never persisted beyond Redis, memory wipe after forwarding in charge handler, no sensitive data in error responses

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational — MVP milestone
- **US2 (Phase 4)**: Depends on US1 (needs tokenize to create tokens) + Foundational
- **US3 (Phase 5)**: Depends on Foundational (can start after Phase 2, parallel with US1/US2)
- **US4 (Phase 6)**: Depends on all other stories being routed (applies middleware)
- **Polish (Phase 7)**: Depends on all user stories complete

### Within Each User Story

- Models before repositories (Phase 2)
- Repositories before handlers
- Handlers before route wiring
- Route wiring before service entrypoint
- Entrypoint validates full story works

### Parallel Opportunities

- T003, T004, T005 can run in parallel (config, docker, mock PSP)
- T006–T009 can run in parallel (all model definitions)
- T010–T012 can run in parallel (crypto functions)
- T014–T017 can run in parallel (all SQL migrations)
- T019–T021 can run in parallel (repositories)
- T049–T051, T053 can run in parallel (polish tasks)

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 — Tokenize
4. **STOP and VALIDATE**: Tokenize a card, verify encrypted storage
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Tokenize works → **MVP!**
3. Add US2 → Full tokenize-pay flow with mock PSP
4. Add US3 → Lifecycle management + audit trail
5. Add US4 → Auth + RBAC across all endpoints
6. Polish → Dockerfiles, tracing, security review

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story
- Proxy has NO database access — communicates only with Tokenizer via mTLS and PSP via HTTPS
- Tokenizer is the single owner of PostgreSQL and Redis
- Mock PSP (Express) is for integration testing only — production uses real PSP endpoints
- Commit after each task or logical group
