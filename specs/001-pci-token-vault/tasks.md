# Tasks: PCI-Compliant Token Vault & Reveal-Forward Proxy

**Input**: Design documents from `/specs/001-pci-token-vault/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api.yaml

**Tests**: Not explicitly requested in spec — test tasks omitted.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1, US2, US3, US4)
- Exact file paths included in descriptions

---

## Phase 1: Setup

**Purpose**: Project initialization, dependencies, and local dev environment

- [x] T001 Initialize Go module with `go mod init` and add dependencies (chi v5, pgx v5, go-redis v9, aws-sdk-go-v2 KMS, testify, golang-migrate, otel) in `go.mod`
- [x] T002 Create project directory structure per plan.md: `cmd/tokenizer/`, `cmd/proxy/`, `cmd/migrate/`, `internal/` (auth, crypto, handler, kms, model, proxy, repository, redis, server, audit), `migrations/`, `config/`, `test/mock-provider/`
- [x] T003 [P] Create environment-based configuration loader in `config/config.go` (DATABASE_URL, REDIS_URL, KMS_KEY_ARN, KMS_ENDPOINT, AWS_REGION, HMAC_KEY, CVV_TTL, PORT_TOKENIZER, PORT_PROXY, LOG_LEVEL, LOG_FORMAT)
- [x] T004 [P] Create `docker-compose.yaml` with PostgreSQL 16, Redis 7, LocalStack (KMS on port 4566), and mock-provider services
- [x] T005 [P] Create mock destination Express app in `test/mock-provider/` (package.json, index.js, Dockerfile) — receives any JSON payload on POST /receive, logs the full body (including revealed PAN/CVV), returns mock success response

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T006 Define domain model structs in `internal/model/token.go` (Token with token_id, pan_blind_index, status, expiry_month, expiry_year, timestamps)
- [x] T007 [P] Define domain model structs in `internal/model/vault_entry.go` (VaultEntry with token_id, pan_ciphertext, iv, auth_tag, dek_encrypted, kms_key_id)
- [x] T008 [P] Define domain model structs in `internal/model/audit.go` (AuditLogEntry with correlation_id, operation, token_id_masked, actor, result, detail)
- [x] T009 Implement AES-256-GCM encrypt/decrypt functions in `internal/crypto/aes.go` (Encrypt(plaintext, key) → ciphertext+iv+tag; Decrypt(ciphertext, iv, tag, key) → plaintext; unique IV via crypto/rand per call)
- [x] T010 [P] Implement HMAC-SHA256 blind index function in `internal/crypto/hmac.go` (ComputeBlindIndex(pan, hmacKey) → hex string for deterministic PAN lookup)
- [x] T011 [P] Implement envelope encryption helpers in `internal/crypto/envelope.go` (GenerateDEK, EncryptWithDEK, DecryptWithDEK — wraps aes.go with DEK generation)
- [x] T012 Implement KMS client using AWS SDK v2 in `internal/kms/client.go` (WrapKey(dek) → encrypted_dek via KMS Encrypt; UnwrapKey(encrypted_dek) → dek via KMS Decrypt; GenerateDataKey for new DEKs; configurable endpoint for LocalStack in dev via KMS_ENDPOINT env var)
- [x] T013 Create SQL migration 001: tokens table in `migrations/001_create_tokens.up.sql` and `migrations/001_create_tokens.down.sql` per data-model.md (indexes on token_id, pan_blind_index, status)
- [x] T014 [P] Create SQL migration 002: vault_entries table in `migrations/002_create_vault_entries.up.sql` and `migrations/002_create_vault_entries.down.sql` (FK to tokens.token_id, unique constraint)
- [x] T015 [P] Create SQL migration 003: audit_log table in `migrations/003_create_audit_log.up.sql` and `migrations/003_create_audit_log.down.sql` (append-only, index on correlation_id)
- [x] T016 Implement migration CLI entrypoint in `cmd/migrate/main.go` using golang-migrate (up, down, version commands against DATABASE_URL)
- [x] T017 Implement token repository in `internal/repository/token_repo.go` (Create, FindByBlindIndex, FindByTokenID, UpdateStatus, UpdateLastUsed — using pgx v5 and pgxpool)
- [x] T018 [P] Implement vault entry repository in `internal/repository/vault_repo.go` (Create, FindByTokenID — pgx v5)
- [x] T019 [P] Implement audit log repository in `internal/repository/audit_repo.go` (Append — insert only, no update/delete; FindByTokenID with pagination)
- [x] T020 Implement Redis CVV store in `internal/redis/cvv_store.go` (Store(tokenID, encryptedCVV, ttl); Retrieve(tokenID) → encryptedCVV using GETDEL for atomic single-use; uses go-redis v9)
- [x] T021 Implement structured audit logger in `internal/audit/logger.go` (wraps repository + slog; masks PAN; ensures CVV never logged; adds correlation_id from context)
- [x] T022 Implement structured error response types in `internal/server/errors.go` (ErrorResponse struct with code, message, correlation_id; helper functions for 400, 401, 403, 404, 502, 503)
- [x] T023 Implement HTTP server setup and base router in `internal/server/server.go` and `internal/server/router.go` (chi router, graceful shutdown, request ID middleware, structured logging middleware, recovery middleware)
- [x] T024 [P] Implement health check handler in `internal/handler/health.go` (GET /health — checks PostgreSQL ping, Redis ping, KMS availability; returns HealthResponse per api.yaml)

**Checkpoint**: Foundation ready — all shared infrastructure in place. User story implementation can begin.

---

## Phase 3: User Story 1 — Tokenize Payment Card Data (Priority: P1) MVP

**Goal**: Accept PAN + expiry + CVV, validate, encrypt, store, return deterministic token. Same PAN returns same token with CVV/expiry update.

**Independent Test**: POST /vault/tokenize with valid PAN → receive token; POST again with same PAN → receive same token; verify PAN stored encrypted in DB; verify CVV in Redis with TTL.

### Implementation for User Story 1

- [x] T025 [US1] Implement Luhn validation helper in `internal/handler/tokenize.go` (validatePAN function — validates digit count 13-19 and Luhn checksum)
- [x] T026 [US1] Implement token generation function in `internal/handler/tokenize.go` (generateTokenID — produces `tok_` prefixed unique non-reversible identifier using crypto/rand)
- [x] T027 [US1] Implement tokenize handler in `internal/handler/tokenize.go` (POST /vault/tokenize per api.yaml: parse request → validate PAN (Luhn) → compute blind index → check if PAN exists → if exists: return existing token + update CVV/expiry → if new: generate token, encrypt PAN with envelope encryption via KMS, store Token + VaultEntry in DB transaction, store CVV in Redis with TTL → return TokenizeResponse → log audit entry)
- [x] T028 [US1] Wire tokenize route in `internal/server/router.go` (POST /vault/tokenize → tokenize handler)
- [x] T029 [US1] Create Tokenizer service entrypoint in `cmd/tokenizer/main.go` (load config, init pgxpool, init Redis client, init KMS client, build router, start HTTP server with graceful shutdown)

**Checkpoint**: User Story 1 complete — tokenization works end-to-end.

---

## Phase 4: User Story 2 — Reveal and Forward via Proxy (Priority: P2)

**Goal**: Proxy receives arbitrary JSON payload + destination URL, scans for token patterns (`tok_...`), reveals each token via Tokenizer internal API, replaces tokens with real values in payload, forwards to destination, returns response verbatim. Proxy stores NOTHING.

**Independent Test**: Tokenize a card (US1), then POST /proxy/forward with a payload containing the token and mock destination URL → mock destination receives real PAN/CVV in payload → response returned verbatim.

### Implementation for User Story 2

- [x] T030 [US2] Implement detokenize handler in `internal/handler/detokenize.go` (POST /internal/detokenize on Tokenizer service: validate token exists + active → decrypt PAN from vault entry via KMS unwrap + AES decrypt → retrieve CVV from Redis via GETDEL → return raw PAN + expiry + CVV nullable → log audit entry; mTLS-only enforcement)
- [x] T031 [US2] Wire detokenize route in `internal/server/router.go` (POST /internal/detokenize → detokenize handler, restricted to mTLS-authenticated Proxy cert)
- [x] T032 [US2] Implement token revealer in `internal/proxy/revealer.go` (ScanAndReveal(payload, detokenizeClient) — recursively walks JSON payload, identifies string values matching `tok_` pattern, calls Tokenizer /internal/detokenize for each token, replaces token values with revealed data structure {pan, expiry_month, expiry_year, cvv?}, returns modified payload)
- [x] T033 [US2] Implement HTTP forwarder in `internal/proxy/forwarder.go` (Forward(destination, method, headers, payload) — sends revealed payload to destination URL, returns raw response status + headers + body, wipes sensitive data from memory after send)
- [x] T034 [US2] Implement forward handler in `internal/handler/forward.go` (POST /proxy/forward per api.yaml: parse ForwardRequest → call revealer.ScanAndReveal → call forwarder.Forward to destination → return ForwardResponse with destination's status/headers/body verbatim → log audit entry with correlation_id, NO sensitive data in logs)
- [x] T035 [US2] Wire forward route in `internal/server/router.go` (POST /proxy/forward → forward handler)
- [x] T036 [US2] Create Proxy service entrypoint in `cmd/proxy/main.go` (load config, init mTLS HTTP client for Tokenizer, build router — NO database pool, NO Redis client — start HTTP server with graceful shutdown)

**Checkpoint**: User Stories 1 AND 2 complete — full tokenize → reveal → forward flow works end-to-end with mock destination.

---

## Phase 5: User Story 3 — Manage Token Lifecycle (Priority: P3)

**Goal**: Validate token status, deactivate tokens (soft-delete), view audit trail per token.

**Independent Test**: Tokenize a card → GET /vault/tokens/{id} returns active → DELETE /vault/tokens/{id} → GET returns inactive → POST /proxy/forward with deactivated token → rejected → GET /vault/tokens/{id}/audit → returns all operations.

### Implementation for User Story 3

- [x] T037 [US3] Implement get token status handler in `internal/handler/token_manage.go` (GET /vault/tokens/{token_id} per api.yaml: lookup by token_id → return TokenStatusResponse or 404 → log audit entry)
- [x] T038 [US3] Implement deactivate token handler in `internal/handler/token_manage.go` (DELETE /vault/tokens/{token_id} per api.yaml: set status=inactive → return updated TokenStatusResponse or 404 → log audit entry)
- [x] T039 [US3] Implement audit trail handler in `internal/handler/token_manage.go` (GET /vault/tokens/{token_id}/audit per api.yaml: query audit_repo with pagination → return AuditTrailResponse)
- [x] T040 [US3] Wire token management routes in `internal/server/router.go` (GET and DELETE /vault/tokens/{token_id}, GET /vault/tokens/{token_id}/audit)

**Checkpoint**: User Stories 1, 2, AND 3 complete — full lifecycle management available.

---

## Phase 6: User Story 4 — Authenticate and Authorize Access (Priority: P4)

**Goal**: All endpoints require authentication. RBAC restricts operations per service role. /internal/detokenize only accessible via Proxy mTLS cert.

**Independent Test**: Call any endpoint without auth → 401. Call /internal/detokenize from non-Proxy cert → 403. Call /vault/tokenize with valid auth + "tokenize" role → 200. Call /proxy/forward with valid auth + "forward" role → 200.

### Implementation for User Story 4

- [x] T041 [US4] Implement Bearer token authentication middleware in `internal/auth/middleware.go` (extract and validate Bearer token from Authorization header; inject service identity into request context; return 401 on failure)
- [x] T042 [US4] Implement RBAC authorization middleware in `internal/auth/rbac.go` (define roles: tokenize, forward, manage, detokenize; check service identity against required role per route; return 403 on insufficient permissions)
- [x] T043 [US4] Implement mTLS certificate validation for /internal/detokenize in `internal/auth/middleware.go` (extract client cert CN from TLS connection state; verify CN matches expected Proxy service identity; return 403 if non-Proxy cert)
- [x] T044 [US4] Wire auth + RBAC middleware into all routes in `internal/server/router.go` (apply Bearer auth globally; apply role-specific RBAC per route group: /vault/* requires "tokenize" or "manage" role, /proxy/* requires "forward" role, /internal/* requires mTLS cert check; /health exempt from auth)

**Checkpoint**: All user stories complete — system is fully authenticated and authorized.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Containerization, documentation, and production readiness

- [x] T045 [P] Create multi-stage Dockerfile for Tokenizer in `Dockerfile.tokenizer` (build stage: golang:1.22-alpine, CGO_ENABLED=0; runtime stage: gcr.io/distroless/static-debian12; COPY binary; expose PORT_TOKENIZER)
- [x] T046 [P] Create multi-stage Dockerfile for Proxy in `Dockerfile.proxy` (same pattern; expose PORT_PROXY; NO database env vars)
- [x] T047 [P] Add OpenTelemetry tracing middleware in `internal/server/router.go` (inject trace_id + span_id into context and logs; propagate correlation_id across Proxy→Tokenizer calls)
- [x] T048 Validate quickstart.md flow end-to-end: `docker compose up` → create KMS key in LocalStack → run migrations → tokenize → forward to mock destination → verify mock received real PAN/CVV → verify audit trail
- [x] T049 [P] Add `.env.example` with all required environment variables documented
- [x] T050 Security review: verify PAN never in logs (grep for plaintext patterns), CVV never persisted beyond Redis, memory wipe after forwarding in forward handler, no sensitive data in error responses

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

- T003, T004, T005 can run in parallel (config, docker, mock destination)
- T006–T008 can run in parallel (all model definitions)
- T009–T011 can run in parallel (crypto functions)
- T013–T015 can run in parallel (all SQL migrations)
- T017–T019 can run in parallel (repositories)
- T045–T047, T049 can run in parallel (polish tasks)

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
3. Add US2 → Full tokenize → reveal → forward flow with mock destination
4. Add US3 → Lifecycle management + audit trail
5. Add US4 → Auth + RBAC across all endpoints
6. Polish → Dockerfiles, tracing, security review

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story
- Proxy is a stateless reveal-and-forward pipe — NO database, NO Redis, NO state
- Proxy scans JSON payloads for token patterns and reveals them automatically
- Tokenizer is the single owner of PostgreSQL and Redis
- Mock destination (Express) is for testing only — production uses real 3rd party endpoints
- System is extensible beyond payments: any tokenized data (personal info, etc.) can be revealed and forwarded
- Commit after each task or logical group

## Deferred (Constitution Compliance)

The following constitution MUST requirements are deferred to post-MVP:

- **Integration tests** (Constitution Development Workflow): "Integration tests MUST cover tokenization, encryption, TTL enforcement, and access control boundaries." Deferred to a dedicated testing sprint after US1-US4 are implemented. Add via `/speckit.checklist`.
- **Key rotation policy** (Constitution Principle III): "Key rotation policies MUST be defined and enforced." Deferred to ops/infrastructure phase. Requires: re-encryption helper for existing vault entries when KEK rotates, HMAC key rotation strategy for blind indexes.
