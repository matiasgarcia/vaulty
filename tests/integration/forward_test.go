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
	token := tokenBody["token"].(string)

	t.Run("forward reveals token and sends real PAN to destination", func(t *testing.T) {
		var mu sync.Mutex
		var received map[string]interface{}

		// In-process mock destination
		mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			received = body
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "mock": true})
		}))
		defer mockDest.Close()

		resp, body := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": mockDest.URL,
			"payload": map[string]interface{}{
				"card":   token,
				"amount": 5000,
			},
		})

		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, float64(200), body["status_code"])

		mu.Lock()
		defer mu.Unlock()
		require.NotNil(t, received, "mock destination should have received payload")

		// The token should have been replaced with revealed card data
		cardData, ok := received["card"].(map[string]interface{})
		require.True(t, ok, "card field should be an object with revealed data")
		assert.Equal(t, "4111111111111111", cardData["pan"])
		assert.Equal(t, float64(12), cardData["expiry_month"])
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

	t.Run("forward with invalid token returns error", func(t *testing.T) {
		mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("destination should not be called for invalid token")
		}))
		defer mockDest.Close()

		resp, _ := doPost(t, env.proxyURL+"/proxy/forward", map[string]interface{}{
			"destination": mockDest.URL,
			"payload": map[string]interface{}{
				"card": "tok_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		})
		assert.Equal(t, 404, resp.StatusCode)
	})
}
