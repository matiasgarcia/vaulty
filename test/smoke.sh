#!/usr/bin/env bash
set -euo pipefail

TOKENIZER_URL="${TOKENIZER_URL:-http://localhost:8080}"
PROXY_URL="${PROXY_URL:-http://localhost:8081}"
MOCK_PROVIDER_URL="${MOCK_PROVIDER_URL:-http://localhost:9090}"
AUTH="Authorization: Bearer client:secret"
RUN_ID=$(date +%s)
TENANT_A="smoke-a-${RUN_ID}"
TENANT_B="smoke-b-${RUN_ID}"

echo "=== 1. Health checks ==="
echo "Tokenizer:"
curl -sf "$TOKENIZER_URL/health" | jq .
echo "Proxy:"
curl -sf "$PROXY_URL/health" | jq .

echo ""
echo "=== 2. Create tenants (run=$RUN_ID) ==="
curl -sf -X POST "$TOKENIZER_URL/admin/tenants" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d "{\"tenant_id\": \"$TENANT_A\", \"name\": \"Smoke A\"}" | jq .

curl -sf -X POST "$TOKENIZER_URL/admin/tenants" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d "{\"tenant_id\": \"$TENANT_B\", \"name\": \"Smoke B\"}" | jq .

echo ""
echo "=== 3. Tokenize a card (Merchant A) — dynamic echo ==="
TOKEN_RESPONSE=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123",
    "amount": 5000,
    "currency": "USD"
  }')
echo "$TOKEN_RESPONSE" | jq .
PAN_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.pan')
CVV_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.cvv')
echo "PAN token: $PAN_TOKEN"
echo "CVV token: $CVV_TOKEN"

echo ""
echo "=== 4. Tokenize same PAN again (should return same PAN token, new CVV token) ==="
TOKEN_RESPONSE_2=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "456"
  }')
echo "$TOKEN_RESPONSE_2" | jq .
PAN_TOKEN_2=$(echo "$TOKEN_RESPONSE_2" | jq -r '.pan')
CVV_TOKEN_2=$(echo "$TOKEN_RESPONSE_2" | jq -r '.cvv')
if [ "$PAN_TOKEN" != "$PAN_TOKEN_2" ]; then
  echo "ERROR: Same PAN should return same PAN token!"
  exit 1
fi
echo "OK: Same PAN token (deterministic)"
if [ "$CVV_TOKEN" = "$CVV_TOKEN_2" ]; then
  echo "ERROR: New CVV should return new CVV token!"
  exit 1
fi
echo "OK: New CVV token"

echo ""
echo "=== 4b. Tokenize same PAN — Merchant B (different PAN token) ==="
TOKEN_B_RESPONSE=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_B" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "456"
  }')
echo "$TOKEN_B_RESPONSE" | jq .
PAN_TOKEN_B=$(echo "$TOKEN_B_RESPONSE" | jq -r '.pan')
if [ "$PAN_TOKEN" = "$PAN_TOKEN_B" ]; then
  echo "ERROR: Tokens should be different for different tenants!"
  exit 1
fi
echo "OK: Different PAN tokens for different tenants"

echo ""
echo "=== 4c. CVV-only tokenization ==="
CVV_ONLY_RESPONSE=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" \
  -d '{"cvv": "789"}')
echo "$CVV_ONLY_RESPONSE" | jq .
CVV_ONLY_TOKEN=$(echo "$CVV_ONLY_RESPONSE" | jq -r '.cvv')
echo "CVV-only token: $CVV_ONLY_TOKEN"

echo ""
echo "=== 5. Get PAN token status ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$PAN_TOKEN" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" | jq .

echo ""
echo "=== 5b. Get CVV token status ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$CVV_TOKEN_2" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" | jq .

echo ""
echo "=== 6. Forward with independent PAN + CVV tokens ==="
curl -sf -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {
      \"card_number\": \"$PAN_TOKEN\",
      \"security_code\": \"$CVV_TOKEN_2\",
      \"amount\": 5000,
      \"currency\": \"USD\"
    }
  }" | jq .

echo ""
echo "=== 6b. Forward with consumed CVV token (should fail) ==="
echo "Attempting to reuse the consumed CVV token..."
CONSUMED_RESP=$(curl -s -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {\"security_code\": \"$CVV_TOKEN_2\"}
  }")
echo "$CONSUMED_RESP" | jq .
echo "Expected: error (CVV token already consumed)"

echo ""
echo "=== 7. Cross-tenant isolation (Merchant B tries Merchant A's PAN token) ==="
ISOLATION_RESP=$(curl -s -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_B" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {\"card\": \"$PAN_TOKEN\"}
  }")
echo "$ISOLATION_RESP" | jq .
echo "Expected: error (token belongs to Merchant A, not B)"

echo ""
echo "=== 8. Get audit trail ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$PAN_TOKEN/audit" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" | jq .

echo ""
echo "=== 9. Deactivate PAN token ==="
curl -sf -X DELETE "$TOKENIZER_URL/vault/tokens/$PAN_TOKEN" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" | jq .

echo ""
echo "=== 10. Forward with deactivated PAN token (should fail) ==="
curl -s -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -H "X-Tenant-ID: $TENANT_A" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {\"card\": \"$PAN_TOKEN\"}
  }" | jq .

echo ""
echo "=== 11. Check mock provider logs ==="
echo "Run: docker compose logs mock-provider"
echo ""
echo "Done! Verify mock-provider received real PAN and CVV in separate fields."
