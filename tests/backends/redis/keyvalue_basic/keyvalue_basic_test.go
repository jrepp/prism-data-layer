package keyvalue_basic_test

import (
	"context"
	"testing"

	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/interface-suites/keyvalue_basic"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestRedisKeyValueBasic runs the KeyValueBasic interface suite against Redis driver
func TestRedisKeyValueBasic(t *testing.T) {
	// Create test suite with Redis-specific setup/teardown
	suite := &keyvalue_basic.Suite{
		CreateBackend: func(t *testing.T) plugin.KeyValueBasicInterface {
			return createRedisBackend(t)
		},
		CleanupBackend: func(t *testing.T, backend plugin.KeyValueBasicInterface) {
			cleanupRedisBackend(t, backend)
		},
	}

	// Run the full suite
	suite.Run(t)
}

// createRedisBackend starts a Redis container and returns a configured driver
func createRedisBackend(t *testing.T) plugin.KeyValueBasicInterface {
	ctx := context.Background()

	// Start Redis container
	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Redis container")

	// Get container endpoint
	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	// Create Redis driver
	driver := redis.New()

	// Initialize with container endpoint
	config := &plugin.Config{
		BackendConfig: map[string]interface{}{
			"address": endpoint,
		},
	}

	err = driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize Redis driver")

	err = driver.Start(ctx)
	require.NoError(t, err, "Failed to start Redis driver")

	// Store container for cleanup
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	return driver
}

// cleanupRedisBackend stops the Redis driver
func cleanupRedisBackend(t *testing.T, backend plugin.KeyValueBasicInterface) {
	if driver, ok := backend.(*redis.RedisPattern); ok {
		_ = driver.Stop(context.Background())
	}
}
