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

	t.Run("valid PAN returns 201 with token", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
		})
		assert.Equal(t, 201, resp.StatusCode)
		assert.Contains(t, body["token"], "tok_")
		assert.Equal(t, true, body["cvv_stored"])
		assert.Equal(t, false, body["is_existing"])
	})

	t.Run("same PAN returns 200 with same token (deterministic)", func(t *testing.T) {
		resp1, body1 := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4242424242424242", "expiry_month": 6, "expiry_year": 2028, "cvv": "456",
		})
		require.Equal(t, 201, resp1.StatusCode)
		token1 := body1["token"].(string)

		resp2, body2 := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4242424242424242", "expiry_month": 6, "expiry_year": 2028, "cvv": "789",
		})
		assert.Equal(t, 200, resp2.StatusCode)
		assert.Equal(t, token1, body2["token"])
		assert.Equal(t, true, body2["is_existing"])
	})

	t.Run("invalid PAN fails Luhn returns 400", func(t *testing.T) {
		resp, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "1234567890123456", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
		})
		assert.Equal(t, 400, resp.StatusCode)
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "INVALID_PAN", errObj["code"])
	})

	t.Run("missing CVV returns 400", func(t *testing.T) {
		resp, _ := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "",
		})
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("PAN stored encrypted in DB", func(t *testing.T) {
		_, body := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "5500000000000004", "expiry_month": 3, "expiry_year": 2029, "cvv": "321",
		})
		tokenID := body["token"].(string)

		var ciphertext []byte
		err := env.pool.QueryRow(t.Context(),
			"SELECT pan_ciphertext FROM vault_entries WHERE token_id = $1", tokenID,
		).Scan(&ciphertext)
		require.NoError(t, err)
		assert.NotEqual(t, []byte("5500000000000004"), ciphertext, "PAN must be encrypted, not plaintext")
	})
}
