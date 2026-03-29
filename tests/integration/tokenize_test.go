//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenize(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	t.Run("valid PAN and CVV returns 201 with both tokens", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
		})
		assert.Equal(t, 201, resp.StatusCode)
		assert.Contains(t, body["pan"].(string), "tok_")
		assert.Contains(t, body["cvv"].(string), "tok_")
		assert.NotEqual(t, body["pan"], body["cvv"], "PAN and CVV tokens must be different")
		assert.Equal(t, float64(12), body["expiry_month"])
		assert.Equal(t, float64(2027), body["expiry_year"])
	})

	t.Run("same PAN returns 200 with same PAN token (deterministic)", func(t *testing.T) {
		resp1, body1 := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4242424242424242", "expiry_month": 6, "expiry_year": 2028, "cvv": "456",
		})
		require.Equal(t, 201, resp1.StatusCode)
		panToken1 := body1["pan"].(string)
		cvvToken1 := body1["cvv"].(string)

		resp2, body2 := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4242424242424242", "expiry_month": 6, "expiry_year": 2028, "cvv": "789",
		})
		assert.Equal(t, 200, resp2.StatusCode)
		assert.Equal(t, panToken1, body2["pan"], "same PAN must return same token")
		assert.NotEqual(t, cvvToken1, body2["cvv"], "new CVV must return new CVV token")
	})

	t.Run("PAN-only tokenization works", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "5500000000000004", "expiry_month": 3, "expiry_year": 2029,
		})
		assert.Equal(t, 201, resp.StatusCode)
		assert.Contains(t, body["pan"].(string), "tok_")
		assert.Nil(t, body["cvv"], "cvv should not be in response when not sent")
	})

	t.Run("CVV-only tokenization works", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"cvv": "456",
		})
		assert.Equal(t, 200, resp.StatusCode)
		assert.Contains(t, body["cvv"].(string), "tok_")
		assert.Nil(t, body["pan"], "pan should not be in response when not sent")
	})

	t.Run("extra fields echoed unchanged", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
			"amount": 5000, "currency": "USD",
		})
		require.Equal(t, 200, resp.StatusCode) // existing PAN
		assert.Equal(t, float64(5000), body["amount"])
		assert.Equal(t, "USD", body["currency"])
	})

	t.Run("no sensitive fields returns 400", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"amount": 5000, "currency": "USD",
		})
		assert.Equal(t, 400, resp.StatusCode)
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "NO_SENSITIVE_FIELDS", errObj["code"])
	})

	t.Run("invalid PAN fails Luhn returns 400", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "1234567890123456", "expiry_month": 12, "expiry_year": 2027,
		})
		assert.Equal(t, 400, resp.StatusCode)
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "INVALID_PAN", errObj["code"])
	})

	t.Run("invalid CVV returns 400", func(t *testing.T) {
		resp, _ := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"cvv": "ab",
		})
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("PAN stored encrypted in DB", func(t *testing.T) {
		_, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "5500000000000004", "expiry_month": 3, "expiry_year": 2029,
		})
		tokenID := body["pan"].(string)

		var ciphertext []byte
		err := env.pool.QueryRow(t.Context(),
			"SELECT pan_ciphertext FROM vault_entries WHERE token_id = $1", tokenID,
		).Scan(&ciphertext)
		require.NoError(t, err)
		assert.NotEqual(t, []byte("5500000000000004"), ciphertext, "PAN must be encrypted, not plaintext")
	})
}
