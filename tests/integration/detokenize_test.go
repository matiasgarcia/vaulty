//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetokenize(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Tokenize a card first
	_, tokenBody := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
		"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
	})
	token := tokenBody["token"].(string)

	t.Run("detokenize returns real PAN", func(t *testing.T) {
		resp, body := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": token,
		})
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "4111111111111111", body["pan"])
		assert.Equal(t, float64(12), body["expiry_month"])
		assert.Equal(t, float64(2027), body["expiry_year"])
	})

	t.Run("CVV returned on first call then consumed", func(t *testing.T) {
		// Tokenize a new card so CVV is fresh
		_, newBody := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4000056655665556", "expiry_month": 1, "expiry_year": 2028, "cvv": "999",
		})
		newToken := newBody["token"].(string)

		// First detokenize — CVV should be present
		resp1, body1 := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": newToken,
		})
		require.Equal(t, 200, resp1.StatusCode)
		assert.NotNil(t, body1["cvv"], "CVV should be returned on first call")

		// Second detokenize — CVV should be nil (consumed)
		resp2, body2 := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": newToken,
		})
		require.Equal(t, 200, resp2.StatusCode)
		assert.Nil(t, body2["cvv"], "CVV should be nil after first use (single-use)")
	})

	t.Run("invalid token returns 404", func(t *testing.T) {
		resp, _ := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": "tok_nonexistent",
		})
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("deactivated token returns 404", func(t *testing.T) {
		// Tokenize
		_, tb := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "378282246310005", "expiry_month": 6, "expiry_year": 2029, "cvv": "1234",
		})
		tk := tb["token"].(string)

		// Deactivate
		doDelete(t, env.tokenizerURL+"/vault/tokens/"+tk)

		// Try detokenize
		resp, _ := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": tk,
		})
		assert.Equal(t, 404, resp.StatusCode)
	})
}
