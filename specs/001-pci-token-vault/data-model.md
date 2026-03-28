# Data Model: PCI-Compliant Token Vault & Payment Proxy

**Branch**: `001-pci-token-vault` | **Date**: 2026-03-28

## Entities

### Token

The core mapping between a non-reversible token and its associated
encrypted PAN. Deterministic 1:1 mapping — same PAN always returns
the same token.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, generated | Internal row identifier |
| token_id | string(64) | UNIQUE, NOT NULL, indexed | Public-facing token (e.g., `tok_...`) |
| pan_blind_index | string(64) | UNIQUE, NOT NULL, indexed | HMAC-SHA256 of PAN for dedup lookup |
| status | enum | NOT NULL, default=active | `active` or `inactive` (soft-delete) |
| expiry_month | int | NOT NULL | Card expiration month (1-12) |
| expiry_year | int | NOT NULL | Card expiration year (4-digit) |
| created_at | timestamp | NOT NULL, default=now | Creation timestamp (UTC) |
| updated_at | timestamp | NOT NULL, default=now | Last update timestamp (UTC) |
| last_used_at | timestamp | nullable | Last operation timestamp (UTC) |

**Indexes**: `pan_blind_index` (unique), `token_id` (unique), `status`

**State transitions**:
```
[created] → active → inactive (soft-delete, irreversible)
```

---

### Vault Entry

Encrypted PAN storage using envelope encryption (DEK + KEK).
One-to-one relationship with Token.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, generated | Internal row identifier |
| token_id | string(64) | FK → Token.token_id, UNIQUE | Associated token |
| pan_ciphertext | bytea | NOT NULL | AES-256-GCM encrypted PAN |
| iv | bytea(12) | NOT NULL | Initialization vector (unique per record) |
| auth_tag | bytea(16) | NOT NULL | GCM authentication tag |
| dek_encrypted | bytea | NOT NULL | DEK encrypted by KEK via KMS |
| kms_key_id | string(256) | NOT NULL | KMS key ARN/ID used for this DEK |
| created_at | timestamp | NOT NULL, default=now | Creation timestamp (UTC) |

**Constraints**:
- `token_id` is unique (1:1 with Token)
- `iv` MUST be unique per record (generated via crypto/rand)
- `pan_ciphertext` MUST NOT be queryable or indexable

---

### CVV Record (Ephemeral — Redis)

Stored in Redis with native TTL. NOT in PostgreSQL.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| key | string | `cvv:{token_id}` | Redis key pattern |
| value | string | encrypted | AES-256-GCM encrypted CVV |
| ttl | duration | default=1h, max=1h | Time-to-live, configurable |

**Lifecycle**:
```
[stored] → available → deleted (on first use via GETDEL or TTL expiry)
```

**Rules**:
- GETDEL ensures atomic retrieve-and-delete (single-use)
- CVV value is encrypted before storing in Redis
- CVV MUST NEVER appear in logs, metrics, or traces

---

### Audit Log Entry

Immutable append-only record of all system operations.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK, generated | Internal row identifier |
| correlation_id | string(64) | NOT NULL, indexed | Request correlation ID |
| operation | enum | NOT NULL | `tokenize`, `detokenize`, `payment`, `deactivate`, `validate` |
| token_id_masked | string(64) | nullable | Token reference (first 8 chars + masked) |
| actor | string(128) | NOT NULL | Service or user identity |
| result | enum | NOT NULL | `success`, `denied`, `error` |
| detail | jsonb | nullable | Additional context (NO sensitive data) |
| created_at | timestamp | NOT NULL, default=now | Event timestamp (UTC) |

**Rules**:
- Append-only: no UPDATE or DELETE operations permitted
- PAN MUST be masked as `411111******1111` if included in detail
- CVV MUST NEVER appear in any field
- Separate table (or external SIEM sink) for long-term retention

## Relationships

```
Token 1:1 VaultEntry     (token_id FK)
Token 1:1 CVVRecord      (token_id in Redis key)
Token 1:N AuditLogEntry  (correlation via token_id_masked)
```

## Validation Rules

| Entity | Field | Rule |
|--------|-------|------|
| Token | pan_blind_index | HMAC-SHA256 using dedicated HMAC key (NOT the encryption DEK) |
| Token | expiry_month | 1 <= value <= 12 |
| Token | expiry_year | >= current year |
| VaultEntry | iv | 12 bytes from crypto/rand, unique per record |
| VaultEntry | pan (pre-encrypt) | Passes Luhn algorithm |
| AuditLog | detail | MUST NOT contain PAN, CVV, or DEK material |
