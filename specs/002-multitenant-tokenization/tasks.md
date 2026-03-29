# Tasks: Multitenant Tokenization

**Input**: Design documents from `/specs/002-multitenant-tokenization/`
**Prerequisites**: plan.md, spec.md, data-model.md, contracts/api.yaml

**Tests**: Integration tests included as Phase 0 to protect against regressions during multitenancy refactoring. Tests use testcontainers-go (real PostgreSQL + Redis + LocalStack in Docker).

**Organization**: Tasks grouped by user story. This feature modifies existing codebase from 001-pci-token-vault.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1, US2, US3)
- Exact file paths included in descriptions

---

## Phase 0: Regression Tests (Baseline Protection)

**Purpose**: Write integration tests for ALL existing functionality BEFORE modifying any code. These tests serve as a safety net during multitenancy refactoring. Run with `go test -tags=integration ./...`

**CRITICAL**: These tests MUST pass before AND after every subsequent phase.

- [x] T001 Create test helpers in `tests/integration/helpers_test.go` — setup/teardown functions for testcontainers: start PostgreSQL 16 + Redis 7 + LocalStack (KMS) containers, run migrations, create KMS key, return config with connection URLs. Build tag `//go:build integration`.
- [x] T002 Create tokenization integration test in `tests/integration/tokenize_test.go` — test against running Tokenizer HTTP server: (1) POST /vault/tokenize with valid PAN → 201 + token returned, (2) POST same PAN again → 200 + same token (deterministic), (3) POST with invalid PAN (bad Luhn) → 400, (4) POST with missing CVV → 400, (5) verify PAN is encrypted in DB (query vault_entries, assert ciphertext != plaintext PAN), (6) verify CVV in Redis with TTL
- [x] T003 Create detokenize integration test in `tests/integration/detokenize_test.go` — (1) tokenize a card, (2) POST /internal/detokenize with token → 200 + real PAN returned, (3) verify CVV returned on first call (single-use), (4) call detokenize again → CVV is null (already consumed), (5) POST with invalid token → 404, (6) deactivate token then detokenize → 404
- [x] T004 Create forward integration test in `tests/integration/forward_test.go` — start mock destination HTTP server in-process, (1) tokenize a card, (2) POST /proxy/forward with token in payload + destination → 200, (3) verify mock destination received real PAN in payload, (4) verify mock destination received CVV (if available), (5) forward with invalid token → error, (6) forward with no tokens in payload → payload forwarded unchanged
- [x] T005 [P] Create token lifecycle integration test in `tests/integration/lifecycle_test.go` — (1) tokenize → GET /vault/tokens/{id} → active, (2) DELETE /vault/tokens/{id} → inactive, (3) GET again → inactive, (4) POST /proxy/forward with deactivated token → rejected
- [x] T006 [P] Create auth integration test in `tests/integration/auth_test.go` — (1) call /vault/tokenize without Authorization header → 401, (2) call with valid Bearer → 200, (3) call /internal/detokenize from non-proxy identity → 403
- [x] T007 [P] Create audit integration test in `tests/integration/audit_test.go` — (1) tokenize a card, (2) GET /vault/tokens/{id}/audit → at least 1 entry with operation=tokenize, (3) detokenize, (4) audit → entry with operation=detokenize, (5) verify PAN is masked in audit entries, (6) verify CVV never appears in any audit entry
- [x] T008 Run all integration tests, verify all pass — this is the baseline. `go test -tags=integration -v ./tests/integration/...`

**Checkpoint**: Baseline tests green. Safe to refactor.

---

## Phase 1: Setup (Migrations & New Entity)

**Purpose**: Database schema changes and new Tenant entity. Wipe existing local data first.

- [x] T009 Wipe local development data: `docker compose down -v` and `docker compose up -d` to start fresh, then re-create KMS key in LocalStack and re-run migrations
- [x] T010 Create SQL migration 004: tenants table in `migrations/004_create_tenants.up.sql` and `migrations/004_create_tenants.down.sql` (columns: id UUID PK, tenant_id VARCHAR(64) UNIQUE NOT NULL, name VARCHAR(256) NOT NULL, status VARCHAR(16) DEFAULT 'active', kms_key_arn VARCHAR(256) NOT NULL, created_at, updated_at)
- [x] T011 [P] Create SQL migration 005: add tenant_id to tokens in `migrations/005_add_tenant_id_to_tokens.up.sql` and `migrations/005_add_tenant_id_to_tokens.down.sql` (add tenant_id VARCHAR(64) NOT NULL, drop old unique index on pan_blind_index, create new unique index on (tenant_id, pan_blind_index), add index on tenant_id, add FK to tenants.tenant_id)
- [x] T012 [P] Create SQL migration 006: add tenant_id to vault_entries in `migrations/006_add_tenant_id_to_vault_entries.up.sql` and `migrations/006_add_tenant_id_to_vault_entries.down.sql` (add tenant_id VARCHAR(64) NOT NULL, add index)
- [x] T013 [P] Create SQL migration 007: add tenant_id to audit_log in `migrations/007_add_tenant_id_to_audit_log.up.sql` and `migrations/007_add_tenant_id_to_audit_log.down.sql` (add tenant_id VARCHAR(64) NOT NULL, add index)
- [x] T014 Define Tenant model in `internal/model/tenant.go` (Tenant struct with ID, TenantID, Name, Status, KMSKeyARN, CreatedAt, UpdatedAt; TenantStatus enum: active/inactive)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Tenant repository, middleware, and cross-cutting changes that all user stories depend on

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T015 Implement tenant repository in `internal/repository/tenant_repo.go` (Create, FindByTenantID, List with pagination, UpdateStatus — using pgx v5)
- [x] T016 Implement tenant extraction middleware in `internal/auth/tenant.go` (extract X-Tenant-ID header from request, validate format, lookup tenant in repo, reject if missing/invalid/inactive, inject tenant into request context; skip for /health and /admin/* routes)
- [x] T017 Modify `internal/crypto/hmac.go` — update `ComputeBlindIndex` to accept tenantID parameter: `ComputeBlindIndex(tenantID, pan, hmacKey)` computing `HMAC(tenantID + ":" + PAN, hmacKey)` for per-tenant scoping
- [x] T018 Modify `internal/model/token.go` — add `TenantID string` field to Token struct
- [x] T019 [P] Modify `internal/model/audit.go` — add `TenantID string` field to AuditLogEntry struct
- [x] T020 Modify `internal/repository/token_repo.go` — add tenant_id to all queries: Create (insert tenant_id), FindByBlindIndex (WHERE tenant_id = $X AND pan_blind_index = $Y), FindByTokenID (WHERE tenant_id = $X AND token_id = $Y), UpdateStatus (WHERE tenant_id = $X), UpdateExpiry (WHERE tenant_id = $X), UpdateLastUsed (WHERE tenant_id = $X)
- [x] T021 [P] Modify `internal/repository/vault_repo.go` — add tenant_id to Create (insert tenant_id) and FindByTokenID (WHERE tenant_id = $X AND token_id = $Y)
- [x] T022 [P] Modify `internal/repository/audit_repo.go` — add tenant_id to Append (insert tenant_id) and FindByTokenID (WHERE tenant_id = $X AND token_id_masked = $Y)
- [x] T023 Modify `internal/redis/cvv_store.go` — update key format from `cvv:{token_id}` to `cvv:{tenant_id}:{token_id}`. Update Store and Retrieve functions to accept tenantID parameter
- [x] T024 Modify `internal/audit/logger.go` — update Log function to accept tenantID and set it on AuditLogEntry before persisting

**Checkpoint**: Foundation ready — all cross-cutting tenant support in place.

---

## Phase 3: User Story 1 — Tenant-Scoped Tokenization (Priority: P1) MVP

**Goal**: Same PAN + different tenant = different token. Same PAN + same tenant = same token. Tokens are cryptographically isolated per tenant (per-tenant KEK).

**Independent Test**: Tokenize PAN `4111111111111111` from tenant-a → get tok_aaa. Tokenize same PAN from tenant-b → get tok_bbb (different). Tokenize same PAN from tenant-a again → get tok_aaa (same).

### Implementation for User Story 1

- [x] T025 [US1] Modify `internal/handler/tokenize.go` — extract tenant from context (via middleware), pass tenantID to ComputeBlindIndex, use tenant's KMS key ARN (from tenant record) instead of global config for envelope encryption, pass tenantID to cvvStore.Store, pass tenantID to token/vault repo Create, pass tenantID to audit logger
- [x] T026 [US1] Modify `internal/kms/client.go` — update GenerateDataKey, WrapKey, UnwrapKey to accept an optional keyARN parameter that overrides the client's default. If empty, use the client's default. This allows per-tenant KEK usage without changing the interface for callers that don't need it.
- [x] T027 [US1] Modify `cmd/tokenizer/main.go` — wire tenant middleware into authenticated routes (after BearerAuth, before handlers); pass tenant repo to middleware; add tenant repo to handlers that need it
- [x] T028 [US1] Update integration tests in `tests/integration/tokenize_test.go` — add multitenant test cases: (1) create two tenants with different KMS keys, (2) tokenize same PAN from each → different tokens, (3) tokenize same PAN from tenant-a again → same token, (4) verify each token's vault entry uses the correct tenant's KMS key ARN
- [x] T029 [US1] Run all integration tests — Phase 0 baseline tests MUST still pass (with tenant context added), new multitenant tests MUST pass

**Checkpoint**: Tenant-scoped tokenization works. Same PAN → different tokens per tenant.

---

## Phase 4: User Story 2 — Tenant Scoping on All Operations (Priority: P2)

**Goal**: Detokenize, forward, token management, and audit all scoped by tenant. No cross-tenant data leakage.

**Independent Test**: Tokenize PAN from tenant-a and tenant-b. Try to detokenize tenant-a's token using tenant-b's context → rejected. Forward with tenant-a's token under tenant-b → rejected. Audit trail only shows own tenant's ops.

### Implementation for User Story 2

- [x] T030 [US2] Modify `internal/handler/detokenize.go` — extract tenant from context, validate token belongs to requesting tenant (token_repo.FindByTokenID with tenant filter), use tenant's KMS key for decryption, pass tenantID to cvvStore.Retrieve, pass tenantID to audit logger
- [x] T031 [US2] Modify `internal/handler/token_manage.go` — extract tenant from context in GetStatus, Deactivate, GetAuditTrail; scope all repo queries by tenant; pass tenantID to audit logger
- [x] T032 [US2] Modify `internal/handler/forward.go` — extract tenant from context, pass X-Tenant-ID header to revealer for detokenize calls
- [x] T033 [US2] Modify `internal/proxy/revealer.go` — accept tenantID, include X-Tenant-ID header in POST /internal/detokenize requests to Tokenizer
- [x] T034 [US2] Modify `cmd/proxy/main.go` — wire tenant middleware into authenticated routes (Proxy extracts X-Tenant-ID but doesn't validate against DB — Tokenizer validates when detokenize is called)
- [x] T035 [US2] Add cross-tenant isolation integration tests in `tests/integration/isolation_test.go` — (1) tenant-a tokenizes PAN, (2) tenant-b tries to detokenize tenant-a's token → 404, (3) tenant-b tries to get status of tenant-a's token → 404, (4) tenant-b forwards payload with tenant-a's token → rejected, (5) tenant-a audit trail shows no tenant-b operations, (6) tenant-b audit trail shows no tenant-a operations
- [x] T036 [US2] Run all integration tests — all baseline + multitenant + isolation tests MUST pass

**Checkpoint**: Complete tenant isolation across all operations.

---

## Phase 5: User Story 3 — Tenant Provisioning (Priority: P3)

**Goal**: API to create, list, get, and deactivate tenants. Creating a tenant provisions a dedicated KMS key.

**Independent Test**: POST /admin/tenants with tenant_id "new-merchant" → 201 with kms_key_arn. GET /admin/tenants/new-merchant → returns details. DELETE → sets inactive. Tokenize under inactive tenant → rejected.

### Implementation for User Story 3

- [x] T037 [US3] Implement tenant handler in `internal/handler/tenant.go` — CreateTenant (POST /admin/tenants: validate input, create KMS key via kmsClient, store tenant with key ARN, return TenantResponse), GetTenant (GET /admin/tenants/{tenant_id}), ListTenants (GET /admin/tenants with pagination), DeactivateTenant (DELETE /admin/tenants/{tenant_id}: set inactive)
- [x] T038 [US3] Modify `internal/kms/client.go` — add CreateKey(ctx, description) method that calls KMS CreateKey API and returns the new key ARN
- [x] T039 [US3] Modify `internal/auth/rbac.go` — add `RoleAdmin Role = "admin"` and grant admin role access to /admin/* routes
- [x] T040 [US3] Modify `cmd/tokenizer/main.go` — wire tenant admin routes: POST/GET /admin/tenants, GET/DELETE /admin/tenants/{tenant_id}; require admin role; exempt /admin/* from tenant middleware (admin operates cross-tenant)
- [x] T041 [US3] Add tenant provisioning integration tests in `tests/integration/tenant_test.go` — (1) POST /admin/tenants → 201 + KMS key created, (2) POST duplicate tenant_id → 409, (3) GET /admin/tenants/{id} → 200, (4) GET /admin/tenants → list, (5) DELETE → inactive, (6) tokenize under inactive tenant → rejected, (7) verify KMS key ARN stored is valid
- [x] T042 [US3] Run all integration tests — complete test suite MUST pass

**Checkpoint**: Full tenant lifecycle — provision, use, deactivate.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Smoke test update, documentation

- [x] T043 [P] Update `test/smoke.sh` — add multitenant scenarios: create two tenants, tokenize same PAN from each, verify different tokens, verify isolation on forward, verify audit scoping
- [x] T044 [P] Update `README.md` — add multitenancy section: X-Tenant-ID header, tenant provisioning API, per-tenant KMS keys, isolation guarantees
- [x] T045 Validate quickstart.md flow end-to-end: create tenants → tokenize from each → forward → verify isolation → audit trail per tenant
- [x] T046 Final integration test run: `go test -tags=integration -v -count=1 ./tests/integration/...` — all tests green

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 0 (Tests)**: No dependencies — start immediately. MUST pass before any code changes.
- **Phase 1 (Setup)**: Depends on Phase 0 passing.
- **Phase 2 (Foundational)**: Depends on Phase 1 migrations — BLOCKS all user stories.
- **US1 (Phase 3)**: Depends on Foundational — MVP milestone.
- **US2 (Phase 4)**: Depends on US1 (needs tenant-scoped tokens to test isolation).
- **US3 (Phase 5)**: Depends on Foundational (can start parallel with US1/US2 but KMS CreateKey used by US1).
- **Polish (Phase 6)**: Depends on all user stories complete.

### Test Checkpoints

- **After Phase 0**: All baseline tests green (T008)
- **After Phase 3 (US1)**: Baseline + multitenant tokenization tests green (T029)
- **After Phase 4 (US2)**: + isolation tests green (T036)
- **After Phase 5 (US3)**: + provisioning tests green (T042)
- **After Phase 6**: Full suite green (T046)

### Parallel Opportunities

- T011, T012, T013 can run in parallel (independent migration files)
- T018, T019 can run in parallel (model changes in different files)
- T020, T021, T022 can run in parallel (repo changes in different files)
- T043, T044 can run in parallel (smoke test + README)

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 0: Write baseline regression tests → all green
2. Complete Phase 1: Migrations + Tenant model
3. Complete Phase 2: Foundational (tenant middleware, repo changes, crypto)
4. Complete Phase 3: Tenant-scoped tokenization + multitenant tests
5. **STOP and VALIDATE**: Same PAN, two tenants → different tokens; baseline tests still green

### Incremental Delivery

1. Phase 0 → Regression safety net
2. Setup + Foundational → Schema ready, middleware wired
3. Add US1 → Tenant-scoped tokenization → **MVP!**
4. Add US2 → Full isolation + isolation tests
5. Add US3 → Tenant provisioning + provisioning tests
6. Polish → Updated smoke tests, README, final test run

---

## Notes

- This feature modifies the existing 001-pci-token-vault codebase — no new services
- Local data wiped before migrations (not in prod, fresh start)
- Blind index changes from `HMAC(PAN)` to `HMAC(tenant_id + ":" + PAN)`
- Per-tenant KMS key created via KMS CreateKey API at tenant provisioning
- Proxy does NOT validate tenant against DB — it passes X-Tenant-ID to Tokenizer which validates
- Integration tests use testcontainers-go: real PostgreSQL, Redis, and LocalStack in Docker — no mocks
- Every phase has a test checkpoint: all tests MUST be green before proceeding
