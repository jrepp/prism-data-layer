package backends

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/require"
)

func init() {
	// Register Redis backend with the acceptance test framework
	framework.MustRegisterBackend(framework.Backend{
		Name:      "Redis",
		SetupFunc: setupRedis,

		SupportedPatterns: []framework.Pattern{
			framework.PatternKeyValueBasic,
			framework.PatternKeyValueTTL,
			framework.PatternKeyValueScan,
			// TODO: Add PubSub patterns when Redis driver implements PubSubInterface
			// framework.PatternPubSubBasic,
			// framework.PatternPubSubFanout,
			// framework.PatternProducer,
			// framework.PatternConsumer,
		},

		Capabilities: framework.Capabilities{
			SupportsTTL:    true,
			SupportsScan:   true,  // SCAN command
			SupportsAtomic: true,  // INCR, DECR, etc.
			MaxValueSize:   512 * 1024 * 1024, // 512MB (Redis limit)
			MaxKeySize:     512 * 1024 * 1024, // 512MB (same as value)
		},
	})
}

// setupRedis creates a Redis backend for testing using testcontainers
func setupRedis(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Start Redis testcontainer
	redisBackend := backends.SetupRedis(t, ctx)

	// Create Redis driver
	driver := redis.New()

	// Configure driver with testcontainer connection string
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "redis-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"address": redisBackend.ConnectionString,
		},
	}

	// Initialize driver
	err := driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize Redis driver")

	// Start driver
	err = driver.Start(ctx)
	require.NoError(t, err, "Failed to start Redis driver")

	// Wait for driver to be healthy
	err = waitForHealthy(driver, 5*time.Second)
	require.NoError(t, err, "Redis driver did not become healthy")

	// Cleanup function stops driver and terminates container
	cleanup := func() {
		driver.Stop(ctx)
		redisBackend.Cleanup()
	}

	return driver, cleanup
}

// waitForHealthy polls the driver's health endpoint until it reports healthy
func waitForHealthy(driver plugin.Plugin, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			health, err := driver.Health(ctx)
			if err == nil && health.Status == plugin.HealthHealthy {
				return nil
			}
		}
	}
}
