package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRegistryBackend implements RegistryBackend using Redis
type RedisRegistryBackend struct {
	client *redis.Client
	prefix string // Key prefix for namespace isolation
}

// NewRedisRegistryBackend creates a Redis-backed registry
func NewRedisRegistryBackend(addr string, password string, db int, prefix string) (*RedisRegistryBackend, error) {
	if prefix == "" {
		prefix = "multicast:registry:"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisRegistryBackend{
		client: client,
		prefix: prefix,
	}, nil
}

// Set stores identity metadata with optional TTL
func (r *RedisRegistryBackend) Set(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error {
	key := r.prefix + identity

	// Serialize metadata to JSON
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Create identity record with timestamps
	now := time.Now()
	record := map[string]interface{}{
		"id":           identity,
		"metadata":     string(data),
		"registered_at": now.Unix(),
	}

	if ttl > 0 {
		expiresAt := now.Add(ttl).Unix()
		record["expires_at"] = expiresAt
	}

	// Store as Redis hash
	if err := r.client.HSet(ctx, key, record).Err(); err != nil {
		return fmt.Errorf("redis hset failed: %w", err)
	}

	// Set TTL if specified
	if ttl > 0 {
		if err := r.client.Expire(ctx, key, ttl).Err(); err != nil {
			return fmt.Errorf("redis expire failed: %w", err)
		}
	}

	return nil
}

// Get retrieves identity metadata
func (r *RedisRegistryBackend) Get(ctx context.Context, identity string) (*Identity, error) {
	key := r.prefix + identity

	// Retrieve hash
	result, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis hgetall failed: %w", err)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("identity not found: %s", identity)
	}

	// Parse identity record
	return r.parseIdentity(result)
}

// Scan returns all identities (no filtering)
func (r *RedisRegistryBackend) Scan(ctx context.Context) ([]*Identity, error) {
	// Scan for all keys matching prefix
	var cursor uint64
	var identities []*Identity

	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, r.prefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("redis scan failed: %w", err)
		}

		// Fetch each identity
		for _, key := range keys {
			result, err := r.client.HGetAll(ctx, key).Result()
			if err != nil {
				continue // Skip errors
			}

			if len(result) == 0 {
				continue // Key deleted between scan and fetch
			}

			identity, err := r.parseIdentity(result)
			if err != nil {
				continue // Skip parsing errors
			}

			// Skip expired
			if identity.ExpiresAt != nil && time.Now().After(*identity.ExpiresAt) {
				continue
			}

			identities = append(identities, identity)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return identities, nil
}

// Delete removes an identity
func (r *RedisRegistryBackend) Delete(ctx context.Context, identity string) error {
	key := r.prefix + identity
	return r.client.Del(ctx, key).Err()
}

// Enumerate queries identities with filter (backend-native if supported)
func (r *RedisRegistryBackend) Enumerate(ctx context.Context, filter *Filter) ([]*Identity, error) {
	// For POC 4, Redis doesn't support native filtering
	// Return nil to trigger client-side filtering fallback
	// In Week 3, we'll add Lua script-based filtering
	return nil, fmt.Errorf("redis: native filtering not implemented yet (use client-side fallback)")
}

// Close closes Redis connection
func (r *RedisRegistryBackend) Close() error {
	return r.client.Close()
}

// parseIdentity converts Redis hash to Identity struct
func (r *RedisRegistryBackend) parseIdentity(data map[string]string) (*Identity, error) {
	identity := &Identity{
		ID: data["id"],
	}

	// Parse metadata JSON
	if metadataStr, ok := data["metadata"]; ok {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		identity.Metadata = metadata
	}

	// Parse registered_at timestamp
	if registeredAtStr, ok := data["registered_at"]; ok {
		var registeredAt int64
		fmt.Sscanf(registeredAtStr, "%d", &registeredAt)
		identity.RegisteredAt = time.Unix(registeredAt, 0)
	}

	// Parse expires_at timestamp (optional)
	if expiresAtStr, ok := data["expires_at"]; ok {
		var expiresAt int64
		fmt.Sscanf(expiresAtStr, "%d", &expiresAt)
		t := time.Unix(expiresAt, 0)
		identity.ExpiresAt = &t
	}

	return identity, nil
}
