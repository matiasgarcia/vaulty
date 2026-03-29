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
	token := tokenBody["token"].(string)

	t.Run("get status returns active", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+token)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "active", body["status"])
		assert.Equal(t, token, body["token"])
	})

	t.Run("deactivate sets inactive", func(t *testing.T) {
		resp, body := doDelete(t, env.tokenizerURL+"/vault/tokens/"+token)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "inactive", body["status"])
	})

	t.Run("get status after deactivation returns inactive", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+token)
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "inactive", body["status"])
	})

	t.Run("forward with deactivated token fails", func(t *testing.T) {
		resp, _ := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": "http://localhost:9999/should-not-reach",
			"payload":     map[string]interface{}{"card": token},
		})
		assert.NotEqual(t, 200, resp.StatusCode)
	})
}
