//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudit(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Tokenize a card
	_, tokenBody := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
		"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
	})
	panToken := tokenBody["pan"].(string)

	t.Run("tokenize creates audit entry", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+panToken+"/audit")
		require.Equal(t, 200, resp.StatusCode)

		entries := body["entries"].([]interface{})
		require.GreaterOrEqual(t, len(entries), 1)

		// Find tokenize entry
		found := false
		for _, e := range entries {
			entry := e.(map[string]interface{})
			if entry["operation"] == "tokenize" {
				found = true
				assert.Equal(t, "success", entry["result"])
				assert.NotEmpty(t, entry["correlation_id"])
				break
			}
		}
		assert.True(t, found, "should have a tokenize audit entry")
	})

	t.Run("detokenize creates audit entry", func(t *testing.T) {
		// Detokenize
		doPostAs(t, env.tokenizerURL+"/internal/detokenize", "proxy", map[string]interface{}{
			"token": panToken,
		})

		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+panToken+"/audit")
		require.Equal(t, 200, resp.StatusCode)

		entries := body["entries"].([]interface{})
		found := false
		for _, e := range entries {
			entry := e.(map[string]interface{})
			if entry["operation"] == "detokenize" {
				found = true
				break
			}
		}
		assert.True(t, found, "should have a detokenize audit entry")
	})

	t.Run("PAN masked in audit entries", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+panToken+"/audit")
		require.Equal(t, 200, resp.StatusCode)

		raw, _ := json.Marshal(body)
		rawStr := string(raw)

		// Full PAN should never appear
		assert.NotContains(t, rawStr, "4111111111111111")
	})

	t.Run("CVV never appears in audit", func(t *testing.T) {
		resp, body := doGet(t, env.tokenizerURL+"/vault/tokens/"+panToken+"/audit")
		require.Equal(t, 200, resp.StatusCode)

		raw, _ := json.Marshal(body)
		rawStr := strings.ToLower(string(raw))

		// CVV value should never appear in audit
		assert.NotContains(t, rawStr, "\"123\"")
	})
}
