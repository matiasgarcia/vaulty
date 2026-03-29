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
