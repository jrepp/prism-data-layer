package redis_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/patterns/redis"
	"github.com/jrepp/prism-data-layer/tests/acceptance/common"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisPattern_BasicOperations(t *testing.T) {
	ctx := context.Background()

	// Start Redis container using centralized backend utility
	backend := backends.SetupRedis(t, ctx)
	defer backend.Cleanup()

	// Create Redis pattern
	redisPlugin := redis.New()

	// Configure with testcontainer
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "redis-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"address": backend.ConnectionString,
		},
	}

	// Create test harness
	harness := common.NewPatternHarness(t, redisPlugin, config)
	defer harness.Cleanup()

	// Wait for plugin to be healthy
	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err, "Plugin did not become healthy")

	t.Run("Set and Get", func(t *testing.T) {
		err := redisPlugin.Set("test-key", []byte("test-value"), 0)
		require.NoError(t, err)

		value, found, err := redisPlugin.Get("test-key")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "test-value", string(value))
	})

	t.Run("Get Non-Existent Key", func(t *testing.T) {
		value, found, err := redisPlugin.Get("non-existent")
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, value)
	})

	t.Run("Delete", func(t *testing.T) {
		err := redisPlugin.Set("delete-me", []byte("temporary"), 0)
		require.NoError(t, err)

		err = redisPlugin.Delete("delete-me")
		require.NoError(t, err)

		_, found, err := redisPlugin.Get("delete-me")
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("Exists", func(t *testing.T) {
		err := redisPlugin.Set("exists-key", []byte("value"), 0)
		require.NoError(t, err)

		exists, err := redisPlugin.Exists("exists-key")
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = redisPlugin.Exists("non-existent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("TTL Expiration", func(t *testing.T) {
		// Set key with 2 second TTL
		err := redisPlugin.Set("ttl-key", []byte("expires-soon"), 2)
		require.NoError(t, err)

		// Key should exist immediately
		value, found, err := redisPlugin.Get("ttl-key")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "expires-soon", string(value))

		// Wait for expiration
		time.Sleep(3 * time.Second)

		// Key should be gone
		_, found, err = redisPlugin.Get("ttl-key")
		require.NoError(t, err)
		assert.False(t, found, "Key should have expired")
	})

	t.Run("Multiple Keys", func(t *testing.T) {
		// Set multiple keys
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("multi-key-%d", i)
			value := fmt.Sprintf("multi-value-%d", i)
			err := redisPlugin.Set(key, []byte(value), 0)
			require.NoError(t, err)
		}

		// Verify all keys
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("multi-key-%d", i)
			expectedValue := fmt.Sprintf("multi-value-%d", i)
			value, found, err := redisPlugin.Get(key)
			require.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, expectedValue, string(value))
		}
	})

	t.Run("Overwrite Existing Key", func(t *testing.T) {
		err := redisPlugin.Set("overwrite", []byte("original"), 0)
		require.NoError(t, err)

		err = redisPlugin.Set("overwrite", []byte("updated"), 0)
		require.NoError(t, err)

		value, found, err := redisPlugin.Get("overwrite")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "updated", string(value))
	})

	t.Run("Binary Data", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
		err := redisPlugin.Set("binary-key", binaryData, 0)
		require.NoError(t, err)

		value, found, err := redisPlugin.Get("binary-key")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, binaryData, value)
	})
}

func TestRedisPattern_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	// Start Redis container using centralized backend utility
	backend := backends.SetupRedis(t, ctx)
	defer backend.Cleanup()

	redisPlugin := redis.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "redis-concurrent",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"address":   backend.ConnectionString,
			"pool_size": 20, // Larger pool for concurrent operations
		},
	}

	harness := common.NewPatternHarness(t, redisPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err)

	t.Run("Concurrent Writes", func(t *testing.T) {
		const numWorkers = 10
		const opsPerWorker = 10

		done := make(chan error, numWorkers)

		// Launch workers
		for w := 0; w < numWorkers; w++ {
			go func(workerID int) {
				for i := 0; i < opsPerWorker; i++ {
					key := fmt.Sprintf("worker-%d-key-%d", workerID, i)
					value := fmt.Sprintf("worker-%d-value-%d", workerID, i)
					if err := redisPlugin.Set(key, []byte(value), 0); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}(w)
		}

		// Wait for all workers
		for w := 0; w < numWorkers; w++ {
			err := <-done
			require.NoError(t, err, "Worker failed")
		}

		// Verify all keys were written
		for w := 0; w < numWorkers; w++ {
			for i := 0; i < opsPerWorker; i++ {
				key := fmt.Sprintf("worker-%d-key-%d", w, i)
				expectedValue := fmt.Sprintf("worker-%d-value-%d", w, i)
				value, found, err := redisPlugin.Get(key)
				require.NoError(t, err)
				assert.True(t, found, "Key %s not found", key)
				assert.Equal(t, expectedValue, string(value))
			}
		}
	})

	t.Run("Concurrent Reads", func(t *testing.T) {
		// Set up test data
		err := redisPlugin.Set("shared-key", []byte("shared-value"), 0)
		require.NoError(t, err)

		const numReaders = 20
		done := make(chan error, numReaders)

		// Launch readers
		for r := 0; r < numReaders; r++ {
			go func() {
				value, found, err := redisPlugin.Get("shared-key")
				if err != nil {
					done <- err
					return
				}
				if !found {
					done <- fmt.Errorf("key not found")
					return
				}
				if string(value) != "shared-value" {
					done <- fmt.Errorf("unexpected value: %s", string(value))
					return
				}
				done <- nil
			}()
		}

		// Wait for all readers
		for r := 0; r < numReaders; r++ {
			err := <-done
			require.NoError(t, err, "Reader failed")
		}
	})
}

func TestRedisPattern_HealthCheck(t *testing.T) {
	ctx := context.Background()

	// Start Redis container using centralized backend utility
	backend := backends.SetupRedis(t, ctx)
	defer backend.Cleanup()

	plugin := redis.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "redis-health",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"address": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, plugin, config)
	defer harness.Cleanup()

	t.Run("Healthy Status", func(t *testing.T) {
		status, err := plugin.Health(harness.Context())
		require.NoError(t, err)
		assert.Equal(t, core.HealthHealthy, status.Status)
		assert.NotEmpty(t, status.Message)
		assert.NotEmpty(t, status.Details)

		// Check for expected details
		assert.Contains(t, status.Details, "total_conns")
		assert.Contains(t, status.Details, "idle_conns")
	})
}

func TestRedisPattern_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Start Redis container using centralized backend utility
	backend := backends.SetupRedis(t, ctx)
	defer backend.Cleanup()

	redisPlugin := redis.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "redis-errors",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"address": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, redisPlugin, config)
	defer harness.Cleanup()

	t.Run("Delete Non-Existent Key", func(t *testing.T) {
		// Should not error
		err := redisPlugin.Delete("non-existent-key")
		assert.NoError(t, err, "Deleting non-existent key should not error")
	})

	t.Run("Large Value", func(t *testing.T) {
		// Create 1MB value
		largeValue := make([]byte, 1024*1024)
		for i := range largeValue {
			largeValue[i] = byte(i % 256)
		}

		err := redisPlugin.Set("large-key", largeValue, 0)
		require.NoError(t, err, "Should handle large values")

		value, found, err := redisPlugin.Get("large-key")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, len(largeValue), len(value))
	})
}
