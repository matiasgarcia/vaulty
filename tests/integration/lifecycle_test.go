//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenLifecycle(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Tokenize
	_, tokenBody := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
		"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
	})
	panToken := tokenBody["pan"].(string)
	cvvToken := tokenBody["cvv"].(string)

	t.Run("PAN token status returns active with type pan", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+panToken)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "active", body["status"])
		assert.Equal(t, "pan", body["type"])
		assert.Equal(t, panToken, body["token"])
	})

	t.Run("CVV token status returns active with type cvv and TTL", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+cvvToken)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "active", body["status"])
		assert.Equal(t, "cvv", body["type"])
		ttl := body["ttl_seconds"].(float64)
		assert.Greater(t, ttl, float64(0))
	})

	t.Run("deactivate PAN token sets inactive", func(t *testing.T) {
		resp, body := doDelete(t, env.tokenizerURL+"/vault/tokens/"+panToken)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "inactive", body["status"])
	})

	t.Run("get status after deactivation returns inactive", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+panToken)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "inactive", body["status"])
	})

	t.Run("forward with deactivated PAN token fails", func(t *testing.T) {
		resp, _ := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": "http://localhost:9999/should-not-reach",
			"payload":     map[string]interface{}{"card": panToken},
		})
		assert.NotEqual(t, 200, resp.StatusCode)
	})
}
