# Tasks: Independent CVV Tokens

**Input**: Design documents from `/specs/003-independent-cvv-tokens/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api.yaml

**Tests**: Integration tests included — the existing codebase uses integration tests with testcontainers-go as the primary testing strategy.

**Organization**: Tasks grouped by user story. No new SQL migrations needed (CVV tokens are Redis-only).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: No project initialization needed — this feature extends the existing codebase. Phase 1 is empty.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extend the CVV store to support independent CVV tokens with their own token IDs. These changes are required by all user stories.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T001 Add CVV token storage methods to `internal/redis/cvv_store.go` (StoreCVVToken(ctx, tenantID, cvvTokenID, encryptedCVV, ttl), RetrieveCVVToken(ctx, tenantID, cvvTokenID) using GETDEL, ExistsCVVToken(ctx, tenantID, cvvTokenID) for status checks; key format `cvvtok:{tenant_id}:{cvv_token_id}`; reuse existing encryption pattern DEK(32)+IV(12)+ciphertext)
- [x] T002 Add CVV token owner mapping methods to `internal/redis/cvv_store.go` (SetCVVTokenOwner(ctx, tenantID, panTokenID, cvvTokenID, ttl) stores mapping at `cvvtok_owner:{tenant_id}:{pan_token_id}`, GetAndDeletePreviousCVVToken(ctx, tenantID, panTokenID) retrieves previous cvvTokenID, deletes the old CVV token key, and updates the owner mapping; TTL matches CVV token TTL)

**Checkpoint**: CVV token Redis operations ready — user story implementation can begin.

---

## Phase 3: User Story 1 — Dynamic Tokenization with Echo Response (Priority: P1) 🎯 MVP

**Goal**: Transform the tokenize endpoint from fixed-schema to dynamic echo model. Accept arbitrary JSON, scan for `pan`/`cvv` at top level, tokenize each independently, echo everything else unchanged.

**Independent Test**: POST /vault/tokenize with `{"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123", "amount": 5000}` → response echoes all fields with `pan` and `cvv` replaced by `tok_` tokens.

### Implementation for User Story 1

- [x] T003 [US1] Rewrite tokenize handler in `internal/handler/tokenize.go` — replace fixed `TokenizeRequest`/`TokenizeResponse` structs with dynamic `map[string]any` processing: decode request body to `map[string]any`, validate at least one of `pan` or `cvv` is present at top level, extract and remove sensitive fields for processing, echo all remaining fields unchanged in response
- [x] T004 [US1] Implement PAN tokenization logic within the rewritten handler in `internal/handler/tokenize.go` — when `pan` key is present: validate (Luhn, 13-19 digits), extract `expiry_month`/`expiry_year` if present, compute blind index, check for existing token via `FindByBlindIndex`, if new: generate `tok_` ID + encrypt PAN + store Token + VaultEntry, if existing: update expiry; replace `pan` value in response map with the `tok_` token ID
- [x] T005 [US1] Implement CVV tokenization logic within the rewritten handler in `internal/handler/tokenize.go` — when `cvv` key is present: validate (3-4 digits), generate new `tok_` ID for CVV token, encrypt CVV using existing `crypto.GenerateDEK` + `crypto.EncryptWithDEK` pattern, store via `StoreCVVToken` from T001, if PAN was also tokenized in same request: call `GetAndDeletePreviousCVVToken` + `SetCVVTokenOwner` from T002 to invalidate previous CVV token; replace `cvv` value in response map with the CVV `tok_` token ID
- [x] T006 [US1] Wire up HTTP status codes in `internal/handler/tokenize.go` — return 201 if a new PAN token was created, 200 if existing PAN token returned or CVV-only request; add audit logging for both PAN and CVV token creation operations (reuse existing `audit.Logger`)
- [x] T007 [US1] Update integration tests in `tests/integration/tokenize_test.go` — add subtests: (1) PAN+CVV+extra fields → echo response with both tokens, (2) PAN-only → echo with PAN token only, (3) CVV-only → echo with CVV token only, (4) no sensitive fields → 400 error, (5) same PAN twice → same PAN token + different CVV tokens, (6) extra fields echoed unchanged, (7) invalid PAN → 400, (8) invalid CVV → 400, (9) expiry stored as PAN metadata

**Checkpoint**: Tokenize endpoint fully functional with dynamic echo model. PAN and CVV tokens generated independently.

---

## Phase 4: User Story 2 — Use Independent Tokens in Proxy Forward (Priority: P2)

**Goal**: The detokenize endpoint resolves both PAN and CVV tokens to plain string values. The proxy revealer replaces `tok_` patterns with the returned string.

**Independent Test**: Tokenize a card (get PAN token + CVV token), then forward a payload with both tokens in separate fields → mock destination receives raw PAN in one field and raw CVV in the other.

### Implementation for User Story 2

- [x] T008 [US2] Modify detokenize handler in `internal/handler/detokenize.go` — change response from `{pan, expiry_month, expiry_year, cvv?}` to `{value: string}`: first check PostgreSQL for PAN token (if found: decrypt PAN, return `{"value": "<raw_pan>"}`), if not found in PostgreSQL: check Redis for CVV token via `RetrieveCVVToken` from T001 (if found: decrypt CVV, return `{"value": "<raw_cvv>"}`, GETDEL ensures single-use), if neither found: return 404; update audit logging for CVV token detokenization
- [x] T009 [US2] Simplify proxy revealer in `internal/proxy/revealer.go` — update `DetokenizeResult` struct to match new `{value: string}` response; update `ScanAndReveal` to replace `tok_` matches with the plain string `value` (instead of structured object); remove `PAN`, `ExpiryMonth`, `ExpiryYear`, `CVV` fields from `DetokenizeResult`
- [x] T010 [US2] Update integration tests in `tests/integration/detokenize_test.go` — add subtests: (1) PAN token → returns `{value: "4111111111111111"}`, (2) CVV token → returns `{value: "123"}` + consumed (second call → 404), (3) expired CVV token → 404, (4) inactive PAN token → 404, (5) nonexistent token → 404
- [x] T011 [US2] Update integration tests in `tests/integration/forward_test.go` — add subtests: (1) payload with PAN token + CVV token in separate fields → destination receives raw PAN string + raw CVV string, (2) payload with only PAN token → destination receives raw PAN, (3) payload with consumed CVV token → error, (4) payload with expired CVV token → error

**Checkpoint**: Full tokenize → forward → reveal flow works with independent PAN and CVV tokens.

---

## Phase 5: User Story 3 — CVV Token Lifecycle Visibility (Priority: P3)

**Goal**: Token status endpoint returns type and TTL information for CVV tokens.

**Independent Test**: Tokenize with CVV, GET /vault/tokens/{cvv_token_id} → returns `{type: "cvv", status: "active", ttl_seconds: N}`. After consumption → `{status: "consumed"}`.

### Implementation for User Story 3

- [x] T012 [US3] Modify token status handler in `internal/handler/token_status.go` (or equivalent GET handler) — when token not found in PostgreSQL: check Redis via `ExistsCVVToken` from T001; for PAN tokens return `{token, status, type: "pan", created_at}`; for CVV tokens return `{token, status: "active", type: "cvv", ttl_seconds: N}`; for missing tokens return 404
- [x] T013 [US3] Update integration tests in `tests/integration/lifecycle_test.go` — add subtests: (1) CVV token status check → active with TTL, (2) CVV token after consumption → 404 or consumed status, (3) PAN token status → existing behavior preserved with `type: "pan"`

**Checkpoint**: All three user stories complete. Token status works for both PAN and CVV tokens.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T014 [P] Update smoke test in `test/smoke.sh` — add CVV token scenarios: tokenize with dynamic echo, forward with independent CVV token, verify CVV single-use
- [x] T015 [P] Update README.md — update usage examples to reflect new dynamic echo tokenize response format, add CVV-only tokenization example, update forward examples showing independent CVV tokens
- [x] T016 Run quickstart.md validation — execute all quickstart.md steps end-to-end against running services to verify documentation accuracy

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Empty — nothing to do
- **Foundational (Phase 2)**: T001, T002 — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on T001, T002. Can start once Phase 2 is complete.
- **User Story 2 (Phase 4)**: Depends on Phase 3 (needs tokenize to produce CVV tokens for testing)
- **User Story 3 (Phase 5)**: Depends on T001 (ExistsCVVToken). Can start after Phase 2, but best after Phase 3.
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Depends on Phase 2 only — no other story dependencies
- **User Story 2 (P2)**: Depends on US1 (needs tokenize to produce tokens for detokenize/forward)
- **User Story 3 (P3)**: Depends on Phase 2. Loosely depends on US1 (needs tokenize to create CVV tokens for status checks)

### Within Each User Story

- T003 → T004 → T005 → T006 → T007 (US1: sequential, same file)
- T008 → T009 → T010 → T011 (US2: T008/T009 can partially parallel since different files, tests after)
- T012 → T013 (US3: sequential)

### Parallel Opportunities

- T001 and T002 can run in parallel (same file but independent methods)
- T008 (detokenize.go) and T009 (revealer.go) can run in parallel (different files)
- T010 and T011 can run in parallel (different test files)
- T014 and T015 can run in parallel (different files)

---

## Parallel Example: User Story 2

```bash
# These can run in parallel (different files):
Task T008: "Modify detokenize handler in internal/handler/detokenize.go"
Task T009: "Simplify proxy revealer in internal/proxy/revealer.go"

# Then these tests in parallel after T008+T009:
Task T010: "Update integration tests in tests/integration/detokenize_test.go"
Task T011: "Update integration tests in tests/integration/forward_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 2: Foundational (T001, T002)
2. Complete Phase 3: User Story 1 (T003-T007)
3. **STOP and VALIDATE**: Test tokenize endpoint with dynamic echo
4. At this point, tokenization works with independent CVV tokens

### Incremental Delivery

1. Phase 2 → Foundation ready
2. Add User Story 1 → Tokenize with dynamic echo → Test → Deploy (MVP!)
3. Add User Story 2 → Detokenize + Forward with independent tokens → Test → Deploy
4. Add User Story 3 → Token status for CVV tokens → Test → Deploy
5. Polish → Smoke tests, docs, quickstart validation

---

## Notes

- No new SQL migrations — CVV tokens are Redis-only
- Breaking change: tokenize response format changes from fixed schema to dynamic echo
- Existing `cvv:{tenant_id}:{token_id}` Redis keys from old format should still be handled by detokenize for backward compatibility during transition
- Total: 16 tasks across 6 phases
- CVV token encryption reuses existing `crypto.GenerateDEK` + `crypto.EncryptWithDEK` pattern — no new crypto code
