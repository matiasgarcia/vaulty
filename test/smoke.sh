#!/usr/bin/env bash
set -euo pipefail

TOKENIZER_URL="${TOKENIZER_URL:-http://localhost:8080}"
PROXY_URL="${PROXY_URL:-http://localhost:8081}"
MOCK_PROVIDER_URL="${MOCK_PROVIDER_URL:-http://localhost:9090}"
AUTH="Authorization: Bearer client:secret"

echo "=== 1. Health checks ==="
echo "Tokenizer:"
curl -sf "$TOKENIZER_URL/health" | jq .
echo "Proxy:"
curl -sf "$PROXY_URL/health" | jq .

echo ""
echo "=== 2. Create tenants ==="
curl -sf -X POST "$TOKENIZER_URL/admin/tenants" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{"tenant_id": "merchant-a", "name": "Merchant A"}' | jq .

curl -sf -X POST "$TOKENIZER_URL/admin/tenants" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{"tenant_id": "merchant-b", "name": "Merchant B"}' | jq .

echo ""
echo "=== 3. Tokenize a card (Merchant A) ==="
TOKEN_RESPONSE=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123"
  }')
echo "$TOKEN_RESPONSE" | jq .
TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.token')

echo ""
echo "=== 4. Tokenize same PAN again — Merchant A (should return same token) ==="
curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "456"
  }' | jq .

echo ""
echo "=== 4b. Tokenize same PAN — Merchant B (should return DIFFERENT token) ==="
TOKEN_B_RESPONSE=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-b" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "456"
  }')
echo "$TOKEN_B_RESPONSE" | jq .
TOKEN_B=$(echo "$TOKEN_B_RESPONSE" | jq -r '.token')
echo "Merchant A token: $TOKEN"
echo "Merchant B token: $TOKEN_B"
if [ "$TOKEN" = "$TOKEN_B" ]; then
  echo "ERROR: Tokens should be different for different tenants!"
  exit 1
fi
echo "OK: Different tokens for different tenants"

echo ""
echo "=== 5. Get token status (Merchant A) ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$TOKEN" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" | jq .

echo ""
echo "=== 6. Forward to mock provider — Merchant A (reveal token) ==="
curl -sf -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {
      \"card\": \"$TOKEN\",
      \"amount\": 5000,
      \"currency\": \"USD\"
    }
  }" | jq .

echo ""
echo "=== 7. Cross-tenant isolation (Merchant B tries Merchant A's token) ==="
echo "Attempting to use Merchant A's token from Merchant B context..."
ISOLATION_RESP=$(curl -s -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-b" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {\"card\": \"$TOKEN\"}
  }")
echo "$ISOLATION_RESP" | jq .
echo "Expected: error (token belongs to Merchant A, not B)"

echo ""
echo "=== 8. Get audit trail (Merchant A) ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$TOKEN/audit" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" | jq .

echo ""
echo "=== 9. Deactivate token (Merchant A) ==="
curl -sf -X DELETE "$TOKENIZER_URL/vault/tokens/$TOKEN" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" | jq .

echo ""
echo "=== 10. Try forward with deactivated token (should fail) ==="
curl -s -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: merchant-a" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {
      \"card\": \"$TOKEN\",
      \"amount\": 1000,
      \"currency\": \"USD\"
    }
  }" | jq .

echo ""
echo "=== 11. Check mock provider logs ==="
echo "Run: docker compose logs mock-provider"
echo ""
echo "Done! Check that step 5 shows the real PAN in mock-provider logs."
