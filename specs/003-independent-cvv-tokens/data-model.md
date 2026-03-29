# Data Model: Independent CVV Tokens

**Branch**: `003-independent-cvv-tokens` | **Date**: 2026-03-29

## Entities

### Token (unchanged)

The core mapping between a non-reversible token and its associated
encrypted PAN. Deterministic 1:1 mapping — same PAN always returns
the same token. No schema changes required.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, generated | Internal row identifier |
| tenant_id | string(64) | FK → tenants, NOT NULL | Tenant scoping |
| token_id | string(64) | UNIQUE, NOT NULL, indexed | Public-facing token (`tok_...`) |
| pan_blind_index | string(64) | NOT NULL, indexed | HMAC-SHA256 of tenant:PAN for dedup |
| status | enum | NOT NULL, default=active | `active` or `inactive` |
| expiry_month | int | NOT NULL | Card expiration month (1-12) |
| expiry_year | int | NOT NULL | Card expiration year (4-digit) |
| created_at | timestamp | NOT NULL, default=now | Creation timestamp (UTC) |
| updated_at | timestamp | NOT NULL, default=now | Last update timestamp (UTC) |
| last_used_at | timestamp | nullable | Last operation timestamp (UTC) |

**Indexes**: `(tenant_id, pan_blind_index)` UNIQUE, `token_id` UNIQUE, `tenant_id`

---

### Vault Entry (unchanged)

Encrypted PAN storage. No schema changes required.

---

### CVV Token (NEW — Redis ephemeral)

Independent token that resolves to an encrypted CVV value. Stored in
Redis with native TTL. NOT in PostgreSQL.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| key | string | `cvvtok:{tenant_id}:{cvv_token_id}` | Redis key pattern |
| value | bytes | encrypted | Combined format: DEK(32) + IV(12) + AES-256-GCM ciphertext |
| ttl | duration | default=1h, max=1h | Time-to-live, configurable |

**Lifecycle**:
```
[stored] → available → deleted (on first use via GETDEL or TTL expiry)
```

**Rules**:
- GETDEL ensures atomic retrieve-and-delete (single-use)
- CVV value is encrypted before storing in Redis (same pattern as existing CVV storage)
- CVV MUST NEVER appear in logs, metrics, or traces
- Token ID uses `tok_` prefix (same as PAN tokens — opaque to clients)

---

### CVV Token Owner Mapping (NEW — Redis ephemeral)

Reverse mapping from PAN token to its most recent CVV token. Enables
invalidation of previous CVV tokens when a PAN is re-tokenized with
a new CVV.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| key | string | `cvvtok_owner:{tenant_id}:{pan_token_id}` | Redis key pattern |
| value | string | cvv_token_id | The current CVV token ID for this PAN token |
| ttl | duration | same as CVV token TTL | Expires when the CVV token expires |

**Rules**:
- When a new CVV token is created for a PAN, the previous CVV token
  (if any) is deleted from Redis, and the owner mapping is updated
- If CVV is tokenized without a PAN (CVV-only request), no owner
  mapping is created

---

### CVV Record (DEPRECATED — existing Redis storage)

The existing `cvv:{tenant_id}:{token_id}` keys from the original
design are superseded by the new CVV Token entity. The old keys stored
CVV associated directly with a PAN token ID. The new CVV tokens have
their own independent token IDs.

**Migration**: The old CVV storage format continues to work during the
transition. The detokenize handler checks for both old-format keys
(`cvv:`) and new-format keys (`cvvtok:`) to maintain backward
compatibility with CVVs stored before this feature.

---

### Audit Log Entry (unchanged)

No schema changes. CVV token operations use existing audit fields:
- `operation`: `tokenize` (for CVV token creation), `detokenize` (for CVV token revelation)
- `token_id_masked`: Masked CVV token ID
- CVV values never logged

## Relationships

```
Token 1:1 VaultEntry         (token_id FK, unchanged)
Token 0:1 CVVTokenOwner      (pan_token_id in Redis key, optional)
CVVTokenOwner 1:1 CVVToken   (cvv_token_id as value)
Token 1:N AuditLogEntry      (correlation via token_id_masked, unchanged)
```

## Validation Rules

| Entity | Field | Rule |
|--------|-------|------|
| CVV Token | cvv_token_id | `tok_` prefix + 40 hex chars (same format as PAN tokens) |
| CVV Token | value (pre-encrypt) | 3-4 digits |
| CVV Token | ttl | <= 1 hour (enforced) |
| Tokenize Request | body | Must contain at least one of `pan` or `cvv` at top level |
| Tokenize Request | pan (if present) | 13-19 digits, passes Luhn algorithm |
| Tokenize Request | cvv (if present) | 3-4 digits |

## Database Migrations

No new SQL migrations required. CVV tokens are stored entirely in Redis.
