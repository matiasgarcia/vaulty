//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuth(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	t.Run("request without auth header returns 401", func(t *testing.T) {
		resp := doRaw(t, "POST", env.tokenizerURL+"/vault/tokenize", map[string]string{
			"Content-Type": "application/json",
		})
		resp.Body.Close()
		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("request with valid auth succeeds", func(t *testing.T) {
		resp := doRaw(t, "POST", env.tokenizerURL+"/vault/tokenize", map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer client:secret",
		})
		resp.Body.Close()
		// Will be 400 (no body) but not 401 — auth passed
		assert.NotEqual(t, 401, resp.StatusCode)
	})

	t.Run("detokenize from non-proxy identity returns 403", func(t *testing.T) {
		resp := doRaw(t, "POST", env.tokenizerURL+"/internal/detokenize", map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer client:secret",
			"X-Tenant-ID":   defaultTestTenant,
		})
		resp.Body.Close()
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("detokenize from proxy identity succeeds auth", func(t *testing.T) {
		resp := doRaw(t, "POST", env.tokenizerURL+"/internal/detokenize", map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer proxy:secret",
			"X-Tenant-ID":   defaultTestTenant,
		})
		resp.Body.Close()
		// Will be 400 (no valid body) but not 401/403
		assert.NotEqual(t, 401, resp.StatusCode)
		assert.NotEqual(t, 403, resp.StatusCode)
	})
}
