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
	panToken := tokenBody["pan"].(string)
	cvvToken := tokenBody["cvv"].(string)

	t.Run("PAN token returns real PAN as value", func(t *testing.T) {
		resp, body := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": panToken,
		})
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "4111111111111111", body["value"])
	})

	t.Run("CVV token returns real CVV as value and is single-use", func(t *testing.T) {
		// First detokenize — CVV should be returned
		resp1, body1 := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": cvvToken,
		})
		require.Equal(t, 200, resp1.StatusCode)
		assert.Equal(t, "123", body1["value"])

		// Second detokenize — CVV token consumed, should return 404
		resp2, _ := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": cvvToken,
		})
		assert.Equal(t, 404, resp2.StatusCode)
	})

	t.Run("CVV token from re-tokenize invalidates previous", func(t *testing.T) {
		// Tokenize with new CVV for the same PAN
		_, body2 := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "999",
		})
		newCVVToken := body2["cvv"].(string)

		// New CVV token works
		resp, body := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": newCVVToken,
		})
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "999", body["value"])
	})

	t.Run("invalid token returns 404", func(t *testing.T) {
		resp, _ := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": "tok_nonexistent0000000000000000000000000000",
		})
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("deactivated PAN token returns 404", func(t *testing.T) {
		// Tokenize
		_, tb := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
			"pan": "378282246310005", "expiry_month": 6, "expiry_year": 2029,
		})
		tk := tb["pan"].(string)

		// Deactivate
		doDelete(t, env.tokenizerURL+"/vault/tokens/"+tk)

		// Try detokenize
		resp, _ := doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": tk,
		})
		assert.Equal(t, 404, resp.StatusCode)
	})
}
