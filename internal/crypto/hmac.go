package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// ComputeBlindIndex computes an HMAC-SHA256 of the PAN using a dedicated HMAC key.
// Returns a hex-encoded string for deterministic PAN dedup lookup.
// The HMAC key MUST be separate from the encryption DEK.
func ComputeBlindIndex(pan string, hmacKey []byte) string {
	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(pan))
	return hex.EncodeToString(h.Sum(nil))
}
