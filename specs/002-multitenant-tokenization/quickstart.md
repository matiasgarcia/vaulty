# Quickstart: Multitenant Tokenization

Assumes the base system (001-pci-token-vault) is already running.

## 1. Run new migrations

```bash
set -a && source .env && set +a
go run cmd/migrate/main.go up
```

This creates the `tenants` table and adds `tenant_id` to existing
tables. A `default` tenant is created for any pre-existing data.

## 2. Create a tenant

```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{
    "tenant_id": "merchant-a",
    "name": "Merchant A"
  }'
```

This provisions Merchant A with a dedicated KMS key.

## 3. Tokenize a card for Merchant A

```bash
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123"
  }'
```

## 4. Create another tenant and tokenize the same PAN

```bash
# Create Merchant B
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{
    "tenant_id": "merchant-b",
    "name": "Merchant B"
  }'

# Tokenize same PAN — different token returned
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-b" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "456"
  }'
```

## 5. Verify isolation

```bash
# Try to use Merchant A's token from Merchant B — should fail
curl -X POST http://localhost:8081/proxy/forward \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-b" \
  -d '{
    "destination": "http://localhost:9090/receive",
    "payload": {
      "card": "<MERCHANT_A_TOKEN>",
      "amount": 5000
    }
  }'
# Expected: 404 — token not found (belongs to Merchant A, not B)
```

## Key Changes from Single-Tenant

- All data endpoints require `X-Tenant-ID` header
- Same PAN + different tenant = different token
- Same PAN + same tenant = same token (deterministic)
- Each tenant has a dedicated KMS key (KEK)
- Tenant management via `/admin/tenants` (admin role required)
- Redis CVV keys prefixed with tenant: `cvv:{tenant_id}:{token_id}`
