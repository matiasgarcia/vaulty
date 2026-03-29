# Feature Specification: Independent CVV Tokens

**Feature Branch**: `003-independent-cvv-tokens`
**Created**: 2026-03-29
**Status**: Draft
**Input**: User description: "Tokenize endpoint returns separate CVV token alongside PAN token, usable independently in proxy forward payloads with its own TTL"

## Clarifications

### Session 2026-03-29

- Q: Should the CVV token use the same prefix (`tok_`) or a different one? → A: Same `tok_` prefix. The system distinguishes PAN tokens (permanent, stored in PostgreSQL) from CVV tokens (ephemeral, stored in Redis with TTL) internally. From the client's perspective, both are opaque tokens usable in proxy forward payloads.
- Q: Should the CVV token be single-use (consumed on first detokenization) like the current CVV behavior? → A: Yes. CVV tokens retain the existing single-use semantics (GETDEL). Once revealed through a proxy forward, the CVV token is consumed and subsequent uses return null/empty for that field.
- Q: What happens when a client re-tokenizes the same PAN with a new CVV? → A: The system returns the same PAN token (deterministic) and a new CVV token. The previous CVV token (if not yet expired or consumed) is replaced.
- Q: Is the tokenize request a fixed schema or dynamic? → A: Dynamic. The client sends an arbitrary JSON body. The system scans for known sensitive fields (`pan`, `cvv`), tokenizes them, and echoes back the entire body with sensitive values replaced by tokens and everything else returned as-is. This is symmetrical to the proxy forward which detokenizes `tok_` values in arbitrary payloads.
- Q: Can the client send only PAN, only CVV, or both? → A: Any combination. PAN-only, CVV-only, or both. Each is tokenized independently. Non-sensitive fields pass through unchanged.
- Q: What about `expiry_month` and `expiry_year`? → A: The tokenize API does not care about card expiry. These fields are opaque pass-through data like any other non-sensitive field. They are NOT stored as PAN token metadata. They are simply echoed back unchanged in the response.
- Q: Should the tokenize API validate or store card expiry? → A: No. The system only recognizes `pan` and `cvv` as sensitive fields. All other fields, including `expiry_month` and `expiry_year`, are opaque pass-through data. The system does not validate, store, or interpret them.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Dynamic Tokenization with Echo Response (Priority: P1)

A merchant integration sends an arbitrary JSON body to the tokenization endpoint. The body may contain `pan`, `cvv`, both, or neither, along with any other fields the client needs. The system identifies sensitive fields, tokenizes each one independently, and returns the full body back with sensitive values replaced by their corresponding tokens — all other fields echoed unchanged.

**Why this priority**: This is the fundamental API contract change. The dynamic echo model enables clients to build their payment payloads upfront and tokenize in a single call, getting back a ready-to-store tokenized version of their data.

**Independent Test**: POST /vault/tokenize with `{"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123", "amount": 5000, "currency": "USD"}`. Verify response is `{"pan": "tok_...", "expiry_month": 12, "expiry_year": 2027, "cvv": "tok_...", "amount": 5000, "currency": "USD"}`.

**Acceptance Scenarios**:

1. **Given** a JSON body with `pan`, `cvv`, and additional fields,
   **When** the client sends a tokenization request,
   **Then** the system returns the same JSON structure with `pan` replaced by a permanent PAN token, `cvv` replaced by an ephemeral CVV token, and all other fields echoed unchanged.

2. **Given** a JSON body with only `pan` (no `cvv`),
   **When** the client sends a tokenization request,
   **Then** the system returns the body with `pan` replaced by a PAN token and all other fields unchanged. No `cvv` field appears in the response.

3. **Given** a JSON body with only `cvv` (no `pan`),
   **When** the client sends a tokenization request,
   **Then** the system returns the body with `cvv` replaced by a CVV token and all other fields unchanged.

4. **Given** the same PAN is tokenized twice,
   **When** two separate requests contain the same PAN value,
   **Then** the same PAN token is returned both times (deterministic). If CVV is included, a new CVV token is issued each time.

5. **Given** a JSON body with `pan` and `expiry_month`/`expiry_year`,
   **When** the client sends a tokenization request,
   **Then** `expiry_month` and `expiry_year` are echoed as-is (not tokenized, not stored — opaque pass-through).

6. **Given** a JSON body with no `pan` and no `cvv`,
   **When** the client sends a tokenization request,
   **Then** the system rejects with a validation error — at least one sensitive field must be present.

---

### User Story 2 - Use Independent Tokens in Proxy Forward (Priority: P2)

A backend client constructs a JSON payload for a third-party provider where PAN and CVV are in separate fields. The client places the PAN token and CVV token independently in the payload. The proxy scans for all `tok_` patterns, reveals each one independently (PAN token resolves to the real PAN string, CVV token resolves to the real CVV string), and forwards the assembled payload.

**Why this priority**: This is the primary business value — enabling flexible payload construction where PAN and CVV can be placed in different locations within any provider's expected format.

**Independent Test**: Tokenize a card, then submit a forward request with a payload containing the PAN token in one field and the CVV token in another. Verify the mock destination receives the real PAN in one field and the real CVV in the other.

**Acceptance Scenarios**:

1. **Given** a payload containing a PAN token in one field and a CVV token in another,
   **When** the client sends a forward request,
   **Then** the proxy replaces each token with its plain string value (PAN token → raw PAN, CVV token → raw CVV), forwards the modified payload, and returns the destination's response.

2. **Given** a payload containing a CVV token that has expired,
   **When** the client sends a forward request,
   **Then** the proxy rejects the request with a structured error indicating the token has expired.

3. **Given** a payload containing a CVV token that was already consumed (single-use),
   **When** the client sends a forward request,
   **Then** the proxy rejects the request with a structured error indicating the token is no longer available.

4. **Given** a payload containing only a PAN token (no CVV token),
   **When** the client sends a forward request,
   **Then** the proxy reveals just the PAN and forwards the payload (existing behavior preserved).

---

### User Story 3 - CVV Token Lifecycle Visibility (Priority: P3)

A client or administrator needs to verify whether a CVV token is still valid (not expired, not consumed) before attempting a payment through the proxy.

**Why this priority**: Operational convenience — allows clients to check CVV token validity proactively rather than discovering expiration at payment time.

**Independent Test**: Tokenize a card with CVV, check the CVV token status (valid), wait for expiration or consume it, then check again (expired/consumed).

**Acceptance Scenarios**:

1. **Given** a valid, unexpired CVV token,
   **When** the client queries the token status endpoint,
   **Then** the system indicates the CVV token is available with its remaining TTL.

2. **Given** an expired or consumed CVV token,
   **When** the client queries the token status endpoint,
   **Then** the system indicates the CVV token is no longer available.

---

### Edge Cases

- What happens when the CVV store (Redis) is unavailable during tokenization? The system MUST still tokenize the PAN and return the PAN token, but the `cvv` field MUST remain as-is (not tokenized) with an error indicator.
- What happens when a CVV token is used in a forward request but Redis is unavailable at reveal time? The system MUST reject the request with a structured error rather than silently omitting the CVV.
- What happens when the same PAN is tokenized concurrently with different CVVs? The last write wins — both requests get the same PAN token, but only the most recent CVV token survives in Redis.
- How does the system handle a CVV token whose associated PAN token has been deactivated? The CVV token MUST be treated as invalid if its associated PAN token is inactive.
- What if the body contains nested objects? Only top-level `pan` and `cvv` fields are recognized as sensitive. Nested fields are echoed unchanged.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The tokenization endpoint MUST accept an arbitrary JSON object as the request body.
- **FR-002**: The system MUST scan the top-level fields of the request body for known sensitive field names (`pan`, `cvv`).
- **FR-003**: For each recognized sensitive field, the system MUST replace its value with a corresponding token in the response.
- **FR-004**: All non-sensitive fields MUST be echoed back unchanged in the response (pass-through).
- **FR-005**: PAN tokens MUST be permanent (stored in the database) and deterministic — the same PAN always produces the same token.
- **FR-006**: CVV tokens MUST be ephemeral with a configurable TTL (default: 1 hour) and auto-expire when the TTL elapses.
- **FR-007**: CVV tokens MUST be single-use — once revealed through the proxy, the token is consumed and cannot be used again.
- **FR-008**: Both PAN and CVV tokens MUST use the same `tok_` prefix format.
- **FR-009**: The system MUST NOT validate, store, or interpret any fields other than `pan` and `cvv`. All other fields (including `expiry_month`, `expiry_year`) are opaque pass-through data echoed unchanged.
- **FR-010**: When a PAN is re-tokenized with a new CVV, the system MUST issue a new CVV token and invalidate any previous CVV token for that PAN token.
- **FR-011**: The request MUST contain at least one sensitive field (`pan` or `cvv`). Requests with no sensitive fields MUST be rejected with a validation error.
- **FR-012**: The proxy MUST resolve both PAN and CVV tokens during revelation — PAN tokens resolve to the real PAN string, CVV tokens resolve to the real CVV string. Both are simple string replacements in the payload.
- **FR-013**: All token operations (creation, revelation, expiration) MUST be logged in the audit trail.
- **FR-014**: Sensitive values (PAN, CVV) MUST never appear in logs, metrics, or traces — only masked token identifiers may be logged.
- **FR-015**: All tokens MUST be tenant-scoped, consistent with the existing multitenant architecture.
- **FR-016**: PAN validation (Luhn algorithm, 13-19 digits) MUST still be enforced when a `pan` field is present.
- **FR-017**: CVV validation (3-4 digits) MUST still be enforced when a `cvv` field is present.

### Key Entities

- **PAN Token**: Permanent token mapping to an encrypted PAN. Deterministic 1:1 mapping. Stored in PostgreSQL with associated expiry metadata.
- **CVV Token**: Ephemeral token mapping to an encrypted CVV value. Attributes: token ID (`tok_` prefixed), associated PAN token reference (if PAN was tokenized in the same request), tenant ID, TTL, creation timestamp. Stored in Redis with native TTL expiration.
- **Tokenize Response**: Echo of the input body with `pan` and `cvv` values replaced by their respective tokens. All other fields passed through unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Tokenization returns the input body with sensitive fields replaced by tokens within the existing latency budget.
- **SC-002**: PAN and CVV tokens can be placed in any field of a proxy forward payload and are each resolved independently to their plain string values.
- **SC-003**: 100% of CVV tokens are invalidated within 5 seconds after TTL expiration or first use.
- **SC-004**: Sensitive values are never exposed in logs, audit trails, or error messages — only masked token identifiers appear.
- **SC-005**: Existing integration flows continue to work with the updated response format (backward compatible at the proxy level).
- **SC-006**: Non-sensitive fields in the tokenization request are returned bit-for-bit identical in the response.

## Assumptions

- The existing `tok_` prefix namespace has sufficient entropy to accommodate both PAN and CVV tokens without collision.
- Redis is the appropriate store for CVV tokens given the ephemeral nature and existing CVV storage infrastructure.
- Only top-level fields named exactly `pan` and `cvv` are treated as sensitive. No deep/recursive scanning of the request body.
- The proxy's existing token scanning mechanism (`tok_` pattern matching in JSON string values) already covers CVV tokens without modification to the scanning logic.
- The detokenization endpoint needs to be enhanced to resolve CVV tokens in addition to PAN tokens. Both resolve to plain strings (PAN → card number, CVV → CVV digits).
- The tokenize response is a breaking change from the current fixed-schema response. Existing clients will need to update their integration.
