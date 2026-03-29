//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantProvisioning(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	t.Run("create tenant provisions KMS key", func(t *testing.T) {
		resp, body := adminPost(t, env.tokenizerURL+"/admin/tenants", map[string]interface{}{
			"tenant_id": "merchant-x",
			"name":      "Merchant X",
		})
		require.Equal(t, 201, resp.StatusCode)
		assert.Equal(t, "merchant-x", body["tenant_id"])
		assert.Equal(t, "active", body["status"])
		assert.Contains(t, body["kms_key_arn"].(string), "arn:aws:kms")
	})

	t.Run("duplicate tenant returns 409", func(t *testing.T) {
		resp, _ := adminPost(t, env.tokenizerURL+"/admin/tenants", map[string]interface{}{
			"tenant_id": "merchant-x",
			"name":      "Merchant X Again",
		})
		assert.Equal(t, 409, resp.StatusCode)
	})

	t.Run("get tenant returns details", func(t *testing.T) {
		resp, body := adminGet(t, env.tokenizerURL+"/admin/tenants/merchant-x")
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "merchant-x", body["tenant_id"])
		assert.Equal(t, "Merchant X", body["name"])
	})

	t.Run("list tenants includes created", func(t *testing.T) {
		resp, body := adminGet(t, env.tokenizerURL+"/admin/tenants")
		require.Equal(t, 200, resp.StatusCode)
		total := body["total"].(float64)
		assert.GreaterOrEqual(t, int(total), 2) // test-tenant + merchant-x
	})

	t.Run("deactivate tenant blocks operations", func(t *testing.T) {
		// Create a tenant to deactivate
		adminPost(t, env.tokenizerURL+"/admin/tenants", map[string]interface{}{
			"tenant_id": "doomed-tenant",
			"name":      "Doomed",
		})

		// Deactivate
		resp, body := adminDelete(t, env.tokenizerURL+"/admin/tenants/doomed-tenant")
		require.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, "inactive", body["status"])

		// Try to tokenize under inactive tenant
		b, _ := json.Marshal(map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
		})
		req, _ := http.NewRequest("POST", env.tokenizerURL+"/vault/tokenize", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer client:secret")
		req.Header.Set("X-Tenant-ID", "doomed-tenant")
		tokenizeResp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		tokenizeResp.Body.Close()
		assert.Equal(t, 403, tokenizeResp.StatusCode, "inactive tenant should be blocked")
	})

	t.Run("multitenant isolation — same PAN different tokens", func(t *testing.T) {
		// Create tenant-a and tenant-b
		adminPost(t, env.tokenizerURL+"/admin/tenants", map[string]interface{}{
			"tenant_id": "tenant-a", "name": "Tenant A",
		})
		adminPost(t, env.tokenizerURL+"/admin/tenants", map[string]interface{}{
			"tenant_id": "tenant-b", "name": "Tenant B",
		})

		// Tokenize same PAN from tenant-a
		respA, bodyA := doPostWithTenant(t, env.tokenizerURL+"/vault/tokenize", "tenant-a", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "123",
		})
		require.Equal(t, 201, respA.StatusCode)
		tokenA := bodyA["token"].(string)

		// Tokenize same PAN from tenant-b
		respB, bodyB := doPostWithTenant(t, env.tokenizerURL+"/vault/tokenize", "tenant-b", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "456",
		})
		require.Equal(t, 201, respB.StatusCode)
		tokenB := bodyB["token"].(string)

		// Different tokens
		assert.NotEqual(t, tokenA, tokenB, "same PAN different tenants must produce different tokens")

		// Same PAN same tenant → same token
		respA2, bodyA2 := doPostWithTenant(t, env.tokenizerURL+"/vault/tokenize", "tenant-a", map[string]interface{}{
			"pan": "4111111111111111", "expiry_month": 12, "expiry_year": 2027, "cvv": "789",
		})
		require.Equal(t, 200, respA2.StatusCode)
		assert.Equal(t, tokenA, bodyA2["token"], "same PAN same tenant must return same token")
	})
}

// Admin HTTP helpers (no X-Tenant-ID needed)

func adminPost(t *testing.T, url string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin:secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin POST %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func adminGet(t *testing.T, url string) (*http.Response, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer admin:secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin GET %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func adminDelete(t *testing.T, url string) (*http.Response, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer admin:secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin DELETE %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func doPostWithTenant(t *testing.T, url, tenantID string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client:secret")
	req.Header.Set("X-Tenant-ID", tenantID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}
