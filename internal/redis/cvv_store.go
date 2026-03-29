package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type CVVStore struct {
	client *redis.Client
}

func NewCVVStore(client *redis.Client) *CVVStore {
	return &CVVStore{client: client}
}

func cvvKey(tenantID, tokenID string) string {
	return "cvv:" + tenantID + ":" + tokenID
}

// Store saves an encrypted CVV with the given TTL, scoped by tenant.
func (s *CVVStore) Store(ctx context.Context, tenantID, tokenID string, encryptedCVV []byte, ttl time.Duration) error {
	if err := s.client.Set(ctx, cvvKey(tenantID, tokenID), encryptedCVV, ttl).Err(); err != nil {
		return fmt.Errorf("cvv store: %w", err)
	}
	return nil
}

// Retrieve atomically gets and deletes the encrypted CVV (single-use via GETDEL).
// Returns nil, nil if the key does not exist or has expired.
func (s *CVVStore) Retrieve(ctx context.Context, tenantID, tokenID string) ([]byte, error) {
	val, err := s.client.GetDel(ctx, cvvKey(tenantID, tokenID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cvv retrieve: %w", err)
	}
	return val, nil
}

// --- CVV Token methods (independent CVV tokens with their own tok_ IDs) ---

func cvvTokenKey(tenantID, cvvTokenID string) string {
	return "cvvtok:" + tenantID + ":" + cvvTokenID
}

func cvvTokenOwnerKey(tenantID, panTokenID string) string {
	return "cvvtok_owner:" + tenantID + ":" + panTokenID
}

// StoreCVVToken saves an encrypted CVV under an independent CVV token ID with TTL.
func (s *CVVStore) StoreCVVToken(ctx context.Context, tenantID, cvvTokenID string, encryptedCVV []byte, ttl time.Duration) error {
	if err := s.client.Set(ctx, cvvTokenKey(tenantID, cvvTokenID), encryptedCVV, ttl).Err(); err != nil {
		return fmt.Errorf("cvv token store: %w", err)
	}
	return nil
}

// RetrieveCVVToken atomically gets and deletes a CVV token (single-use via GETDEL).
// Returns nil, nil if the key does not exist or has expired.
func (s *CVVStore) RetrieveCVVToken(ctx context.Context, tenantID, cvvTokenID string) ([]byte, error) {
	val, err := s.client.GetDel(ctx, cvvTokenKey(tenantID, cvvTokenID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cvv token retrieve: %w", err)
	}
	return val, nil
}

// ExistsCVVToken checks if a CVV token exists (not expired, not consumed).
// Returns TTL remaining. TTL < 0 means the key does not exist.
func (s *CVVStore) ExistsCVVToken(ctx context.Context, tenantID, cvvTokenID string) (time.Duration, bool) {
	ttl, err := s.client.TTL(ctx, cvvTokenKey(tenantID, cvvTokenID)).Result()
	if err != nil || ttl < 0 {
		return 0, false
	}
	return ttl, true
}

// SetCVVTokenOwner records which CVV token is currently associated with a PAN token.
// Used to invalidate the previous CVV token when re-tokenizing with a new CVV.
func (s *CVVStore) SetCVVTokenOwner(ctx context.Context, tenantID, panTokenID, cvvTokenID string, ttl time.Duration) error {
	if err := s.client.Set(ctx, cvvTokenOwnerKey(tenantID, panTokenID), cvvTokenID, ttl).Err(); err != nil {
		return fmt.Errorf("cvv token owner set: %w", err)
	}
	return nil
}

// InvalidatePreviousCVVToken deletes the previous CVV token for a PAN token (if any)
// and updates the owner mapping to the new CVV token ID.
func (s *CVVStore) InvalidatePreviousCVVToken(ctx context.Context, tenantID, panTokenID, newCVVTokenID string, ttl time.Duration) error {
	// Get previous CVV token ID
	prevID, err := s.client.Get(ctx, cvvTokenOwnerKey(tenantID, panTokenID)).Result()
	if err == nil && prevID != "" {
		// Delete the previous CVV token
		s.client.Del(ctx, cvvTokenKey(tenantID, prevID))
	}
	// Set new owner mapping
	return s.SetCVVTokenOwner(ctx, tenantID, panTokenID, newCVVTokenID, ttl)
}
