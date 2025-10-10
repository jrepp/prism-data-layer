package backends

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// RedisBackend provides Redis testcontainer setup and cleanup
type RedisBackend struct {
	ConnectionString string
	cleanup          func()
}

// SetupRedis starts a Redis container and returns connection info
func SetupRedis(t *testing.T, ctx context.Context) *RedisBackend {
	t.Helper()

	// Start Redis container
	redisContainer, err := tcredis.Run(ctx,
		"redis:7-alpine",
		tcredis.WithSnapshotting(10, 1),
		tcredis.WithLogLevel(tcredis.LogLevelVerbose),
	)
	require.NoError(t, err, "Failed to start Redis container")

	// Get connection string
	connStr, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get Redis connection string")

	// Strip redis:// prefix for go-redis compatibility
	if len(connStr) > 8 && connStr[:8] == "redis://" {
		connStr = connStr[8:]
	}

	return &RedisBackend{
		ConnectionString: connStr,
		cleanup: func() {
			if err := redisContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate Redis container: %v", err)
			}
		},
	}
}

// Cleanup terminates the Redis container
func (b *RedisBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}
