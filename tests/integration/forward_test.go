//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForward(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Tokenize a card
	_, tokenBody := doPost(t, env.tokenizerURL+"/vault/tokenize", map[string]interface{}{
		"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
	})
	panToken := tokenBody["pan"].(string)
	cvvToken := tokenBody["cvv"].(string)

	t.Run("forward reveals PAN and CVV tokens independently", func(t *testing.T) {
		var mu sync.Mutex
		var received map[string]interface{}

		mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			received = body
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
		}))
		defer mockDest.Close()

		resp, body := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": mockDest.URL,
			"payload": map[string]interface{}{
				"card_number":   panToken,
				"security_code": cvvToken,
				"amount":        5000,
			},
		})

		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, float64(200), body["status_code"])

		mu.Lock()
		defer mu.Unlock()
		require.NotNil(t, received, "mock destination should have received payload")

		// PAN token replaced with plain PAN string
		assert.Equal(t, "4111111111111111", received["card_number"])
		// CVV token replaced with plain CVV string
		assert.Equal(t, "123", received["security_code"])
		// Non-token fields unchanged
		assert.Equal(t, float64(5000), received["amount"])
	})

	t.Run("forward with no tokens passes payload unchanged", func(t *testing.T) {
		var received map[string]interface{}
		mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&received)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}))
		defer mockDest.Close()

		resp, _ := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": mockDest.URL,
			"payload": map[string]interface{}{
				"name":  "John Doe",
				"email": "john@example.com",
			},
		})
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "John Doe", received["name"])
	})

	t.Run("forward with consumed CVV token returns error", func(t *testing.T) {
		// CVV token was already consumed in the first subtest
		mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("destination should not be called for consumed token")
		}))
		defer mockDest.Close()

		resp, _ := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": mockDest.URL,
			"payload": map[string]interface{}{
				"security_code": cvvToken,
			},
		})
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("forward with invalid token returns error", func(t *testing.T) {
		mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("destination should not be called for invalid token")
		}))
		defer mockDest.Close()

		resp, _ := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": mockDest.URL,
			"payload": map[string]interface{}{
				"card": "tok_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		})
		assert.Equal(t, 404, resp.StatusCode)
	})
}
