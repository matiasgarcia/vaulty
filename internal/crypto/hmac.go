package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// ComputeBlindIndex computes an HMAC-SHA256 of the tenant-scoped PAN
// using a dedicated HMAC key. The tenantID is prepended to ensure
// the same PAN produces different blind indexes for different tenants.
// Returns a hex-encoded string for deterministic per-tenant PAN dedup lookup.
// The HMAC key MUST be separate from the encryption DEK.
func ComputeBlindIndex(tenantID, pan string, hmacKey []byte) string {
	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(tenantID + ":" + pan))
	return hex.EncodeToString(h.Sum(nil))
}
