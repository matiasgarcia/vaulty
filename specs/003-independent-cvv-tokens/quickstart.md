# Quickstart: Independent CVV Tokens

**Branch**: `003-independent-cvv-tokens` | **Date**: 2026-03-29

## Prerequisites

Same as existing setup — Docker, Go 1.22+, running infrastructure (`docker compose up -d`).

## New Flow

### 1. Create a tenant (if not already done)

```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{"tenant_id": "merchant-a", "name": "Merchant A"}'
```

### 2. Tokenize a card (PAN + CVV)

```bash
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123",
    "amount": 5000,
    "currency": "USD"
  }'
```

**Response** (201 Created):
```json
{
  "pan": "tok_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
  "expiry_month": 12,
  "expiry_year": 2027,
  "cvv": "tok_z9y8x7w6v5u4t3s2r1q0p9o8n7m6l5k4j3i2h1g0",
  "amount": 5000,
  "currency": "USD"
}
```

Note: `pan` and `cvv` are replaced by tokens. Everything else is echoed as-is.

### 3. Tokenize only CVV

```bash
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{"cvv": "456"}'
```

**Response** (201 Created):
```json
{
  "cvv": "tok_f1e2d3c4b5a6z7y8x9w0v1u2t3s4r5q6p7o8n9m0"
}
```

### 4. Tokenize only PAN

```bash
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027
  }'
```

**Response** (200 OK — same PAN returns same token):
```json
{
  "pan": "tok_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
  "expiry_month": 12,
  "expiry_year": 2027
}
```

### 5. Forward with independent tokens

```bash
curl -X POST http://localhost:8081/proxy/forward \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "destination": "http://localhost:9090/receive",
    "payload": {
      "card_number": "tok_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
      "security_code": "tok_z9y8x7w6v5u4t3s2r1q0p9o8n7m6l5k4j3i2h1g0",
      "amount": 5000,
      "currency": "USD"
    }
  }'
```

The proxy resolves each token to its plain string value:
- `card_number`: `"tok_aaa..."` → `"4111111111111111"`
- `security_code`: `"tok_bbb..."` → `"123"`

The mock provider receives:
```json
{
  "card_number": "4111111111111111",
  "security_code": "123",
  "amount": 5000,
  "currency": "USD"
}
```

### 6. Check CVV token status

```bash
curl http://localhost:8080/vault/tokens/tok_z9y8x7w6v5u4t3s2r1q0p9o8n7m6l5k4j3i2h1g0 \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a"
```

**Response** (if already consumed by forward):
```json
{
  "token": "tok_z9y8x7w6v5u4t3s2r1q0p9o8n7m6l5k4j3i2h1g0",
  "status": "consumed",
  "type": "cvv"
}
```

## Key Behavioral Changes

| Aspect | Before (v1) | After (v2) |
|--------|-------------|------------|
| Tokenize request | Fixed schema: `{pan, expiry_month, expiry_year, cvv}` | Dynamic JSON: any fields, `pan`/`cvv` detected at top level |
| Tokenize response | `{token, cvv_stored, is_existing}` | Echo of input with `pan`/`cvv` replaced by tokens |
| CVV handling | CVV attached to PAN token, revealed together | CVV gets its own independent token |
| Proxy PAN reveal | `tok_` → `{pan, expiry_month, expiry_year, cvv?}` | `tok_` (PAN) → `"4111111111111111"` (plain string) |
| Proxy CVV reveal | Bundled with PAN reveal | `tok_` (CVV) → `"123"` (plain string) |
| CVV in payload | Not independently referenceable | Place CVV token anywhere in payload |

## Verification

```bash
# After running forward, check mock provider logs:
docker compose logs mock-provider

# Verify CVV token was consumed (second forward with same CVV token fails):
curl -X POST http://localhost:8081/proxy/forward \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "destination": "http://localhost:9090/receive",
    "payload": {
      "security_code": "tok_z9y8x7w6v5u4t3s2r1q0p9o8n7m6l5k4j3i2h1g0"
    }
  }'
# Expected: 404 error — CVV token already consumed
```
