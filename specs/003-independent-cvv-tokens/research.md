# Research: Independent CVV Tokens

**Branch**: `003-independent-cvv-tokens` | **Date**: 2026-03-29

## R1: Dynamic Echo Model — How to Handle Arbitrary JSON Pass-through

**Decision**: Accept `map[string]any` (via `json.RawMessage` or `json.Decoder` into `map[string]interface{}`), scan top-level keys for `"pan"` and `"cvv"`, process those, and re-serialize the full map as the response.

**Rationale**: Go's `encoding/json` natively supports `map[string]any` round-trips. Decoding to a map preserves all unknown fields and their types (strings, numbers, booleans, nested objects, arrays). Re-encoding produces the same JSON structure with sensitive values replaced. This is the simplest approach that satisfies "echo with selective tokenization."

**Alternatives considered**:
- `json.RawMessage` field-by-field: More complex, no benefit for top-level-only scanning
- Streaming JSON parser: Over-engineered for this use case
- Separate `/vault/tokenize-pan` and `/vault/tokenize-cvv` endpoints: Violates the single-call echo design

## R2: CVV Token Storage — Key Format and Data Layout in Redis

**Decision**: Store CVV tokens with key format `cvvtok:{tenant_id}:{cvv_token_id}` in Redis. The value is the same combined encrypted format as existing CVV storage: `DEK(32) + IV(12) + ciphertext_with_tag`. Additionally, store a reverse mapping `cvvtok_owner:{tenant_id}:{pan_token_id}` → `cvv_token_id` to enable invalidation of previous CVV tokens when re-tokenizing.

**Rationale**: Reuses the proven encryption pattern from existing CVV storage. Separate key prefix (`cvvtok:`) avoids collision with existing `cvv:` keys. The owner mapping enables FR-010 (invalidate previous CVV token on re-tokenize) without scanning Redis keys.

**Alternatives considered**:
- Store CVV token data inside the existing `cvv:{tenant_id}:{token_id}` keys: Breaks backward compatibility with existing detokenize flow during migration
- PostgreSQL table for CVV tokens: Over-engineered for ephemeral data with TTL
- No owner mapping (skip invalidation): Violates FR-010

## R3: Detokenize Endpoint — How to Distinguish PAN vs CVV Tokens

**Decision**: The detokenize handler first checks if the token exists in PostgreSQL (PAN token). If not found, checks Redis for a CVV token key (`cvvtok:{tenant_id}:{token_id}`). PAN tokens return `{"pan": "...", "expiry_month": N, "expiry_year": N}`. CVV tokens return `{"cvv": "..."}`. The proxy revealer uses the response shape to determine replacement behavior.

**Rationale**: Token IDs share the same `tok_` prefix (opaque to clients). The detokenize endpoint is the single resolution point. Checking PostgreSQL first (the common case for payment flows) then falling back to Redis is efficient — PAN lookups are indexed, and CVV token lookups in Redis are O(1).

**Alternatives considered**:
- Different token prefixes (`pantok_`, `cvvtok_`): Leaks internal type information to clients, violates opacity principle
- Separate detokenize endpoints: Doubles the proxy's complexity and breaks the existing `tok_` scanning pattern
- Metadata field in token ID encoding the type: Adds complexity without clear benefit

## R4: Proxy Revealer — Plain String Replacement for All Token Types

**Decision**: Both PAN and CVV tokens resolve to plain string replacements. A PAN token is replaced with the raw PAN string. A CVV token is replaced with the raw CVV string. The detokenize endpoint returns a uniform response with a `value` field containing the plain string, regardless of token type.

**Rationale**: Symmetrical design — both token types work the same way. The client already has the expiry (returned as echo from tokenize), so the proxy doesn't need to inject it. This keeps the proxy revealer simple: scan for `tok_` patterns, call detokenize, replace with the returned string value.

**Alternatives considered**:
- PAN token returns structured object `{pan, expiry_month, expiry_year}`: Over-complicated, expiry is not the proxy's concern — the client places expiry values directly in the payload
- Different response shapes per token type: Adds complexity to the revealer for no benefit

## R5: Backward Compatibility — Impact on Existing Clients

**Decision**: This is a **breaking change** to the tokenize response format. The current response `{token, cvv_stored, is_existing}` changes to a dynamic echo of the input body. The proxy forward flow and detokenize endpoint remain backward-compatible — existing `tok_` patterns in payloads still work.

**Rationale**: The tokenize response must change to support the dynamic echo model. Existing clients must update their integration to read `pan` and `cvv` fields from the echo response instead of the `token` field. However, the proxy (the actual consumer of tokens in payment flows) is unaffected.

**Alternatives considered**:
- Versioned API (`/v2/vault/tokenize`): Adds maintenance burden for a single consumer-facing change
- Feature flag to toggle old/new response: Temporary complexity that must be removed later
- Keep old response + add new fields: Conflicting semantics (`token` vs `pan` field)

## R6: CVV-Only Tokenization — PAN Token Association

**Decision**: When only `cvv` is provided (no `pan`), the CVV token is created without a PAN token association. There is no owner mapping to invalidate. The CVV token stands alone — it is ephemeral, single-use, and expires after TTL.

**Rationale**: A CVV-only tokenization is valid for cases where the PAN was previously tokenized and the client just needs a fresh CVV token. However, without a PAN in the same request, there's no way to associate them. The client is responsible for tracking which CVV token corresponds to which PAN token.

**Alternatives considered**:
- Require `pan` or `pan_token` field to associate CVV: Adds complexity, restricts flexibility
- Always require PAN + CVV together: Contradicts the spec requirement for independent tokenization
