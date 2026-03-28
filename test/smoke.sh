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
echo "=== 2. Tokenize a card ==="
TOKEN_RESPONSE=$(curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123"
  }')
echo "$TOKEN_RESPONSE" | jq .
TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.token')

echo ""
echo "=== 3. Tokenize same PAN again (should return same token) ==="
curl -sf -X POST "$TOKENIZER_URL/vault/tokenize" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "456"
  }' | jq .

echo ""
echo "=== 4. Get token status ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$TOKEN" \
  -H "$AUTH" | jq .

echo ""
echo "=== 5. Forward to mock provider (reveal token) ==="
curl -sf -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {
      \"card\": \"$TOKEN\",
      \"amount\": 5000,
      \"currency\": \"USD\"
    }
  }" | jq .

echo ""
echo "=== 6. Get audit trail ==="
curl -sf "$TOKENIZER_URL/vault/tokens/$TOKEN/audit" \
  -H "$AUTH" | jq .

echo ""
echo "=== 7. Deactivate token ==="
curl -sf -X DELETE "$TOKENIZER_URL/vault/tokens/$TOKEN" \
  -H "$AUTH" | jq .

echo ""
echo "=== 8. Try forward with deactivated token (should fail) ==="
curl -s -X POST "$PROXY_URL/proxy/forward" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d "{
    \"destination\": \"$MOCK_PROVIDER_URL/receive\",
    \"payload\": {
      \"card\": \"$TOKEN\",
      \"amount\": 1000,
      \"currency\": \"USD\"
    }
  }" | jq .

echo ""
echo "=== 9. Check mock provider logs ==="
echo "Run: docker compose logs mock-provider"
echo ""
echo "Done! Check that step 5 shows the real PAN in mock-provider logs."
