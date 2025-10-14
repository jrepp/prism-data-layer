package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/drivers/postgres"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/common"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	postgresBackend *backends.PostgresBackend
	testCtx         context.Context
)

// TestMain sets up the PostgreSQL container once for all tests
func TestMain(m *testing.M) {
	testCtx = context.Background()

	// Start PostgreSQL container once
	postgresBackend = backends.SetupPostgres(&testing.T{}, testCtx)

	// Run all tests
	code := m.Run()

	// Cleanup after all tests
	postgresBackend.Cleanup()

	os.Exit(code)
}

func TestPostgresPattern_BasicOperations(t *testing.T) {
	// Create PostgreSQL pattern
	postgresPlugin := postgres.New()

	// Configure with shared testcontainer
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"host":     postgresBackend.Host,
			"port":     postgresBackend.Port,
			"database": postgresBackend.Database,
			"user":     postgresBackend.User,
			"password": postgresBackend.Password,
		},
	}

	// Create test harness
	harness := common.NewPatternHarness(t, postgresPlugin, config)
	defer harness.Cleanup()

	// Wait for plugin to be healthy
	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err, "Plugin did not become healthy")

	t.Run("Set and Get", func(t *testing.T) {
		key := fmt.Sprintf("%s:test-key", t.Name())
		err := postgresPlugin.Set(key, []byte("test-value"), 0)
		require.NoError(t, err)

		value, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "test-value", string(value))
	})

	t.Run("Get Non-Existent Key", func(t *testing.T) {
		key := fmt.Sprintf("%s:non-existent", t.Name())
		value, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, value)
	})

	t.Run("Delete", func(t *testing.T) {
		key := fmt.Sprintf("%s:delete-me", t.Name())
		err := postgresPlugin.Set(key, []byte("temporary"), 0)
		require.NoError(t, err)

		err = postgresPlugin.Delete(key)
		require.NoError(t, err)

		_, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("Exists", func(t *testing.T) {
		keyExists := fmt.Sprintf("%s:exists-key", t.Name())
		keyNonExistent := fmt.Sprintf("%s:non-existent", t.Name())

		err := postgresPlugin.Set(keyExists, []byte("value"), 0)
		require.NoError(t, err)

		exists, err := postgresPlugin.Exists(keyExists)
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = postgresPlugin.Exists(keyNonExistent)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("Overwrite Existing Key", func(t *testing.T) {
		key := fmt.Sprintf("%s:overwrite", t.Name())
		err := postgresPlugin.Set(key, []byte("original"), 0)
		require.NoError(t, err)

		err = postgresPlugin.Set(key, []byte("updated"), 0)
		require.NoError(t, err)

		value, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "updated", string(value))
	})

	t.Run("Binary Data", func(t *testing.T) {
		key := fmt.Sprintf("%s:binary-key", t.Name())
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
		err := postgresPlugin.Set(key, binaryData, 0)
		require.NoError(t, err)

		value, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, binaryData, value)
	})

	t.Run("Large Value", func(t *testing.T) {
		key := fmt.Sprintf("%s:large-key", t.Name())
		// Create 1MB value
		largeValue := make([]byte, 1024*1024)
		for i := range largeValue {
			largeValue[i] = byte(i % 256)
		}

		err := postgresPlugin.Set(key, largeValue, 0)
		require.NoError(t, err, "Should handle large values")

		value, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, len(largeValue), len(value))
	})
}

func TestPostgresPattern_ScanOperations(t *testing.T) {
	postgresPlugin := postgres.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres-scan",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"host":     postgresBackend.Host,
			"port":     postgresBackend.Port,
			"database": postgresBackend.Database,
			"user":     postgresBackend.User,
			"password": postgresBackend.Password,
		},
	}

	harness := common.NewPatternHarness(t, postgresPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Scan Keys", func(t *testing.T) {
		prefix := fmt.Sprintf("%s:scan:", t.Name())

		// Set up test data
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("%skey-%03d", prefix, i)
			value := fmt.Sprintf("value-%03d", i)
			err := postgresPlugin.Set(key, []byte(value), 0)
			require.NoError(t, err)
		}

		// Scan keys with prefix
		keys, err := postgresPlugin.Scan(prefix, 100)
		require.NoError(t, err)
		assert.Equal(t, 10, len(keys), "Should find all 10 keys")

		// Verify keys start with prefix
		for _, key := range keys {
			assert.Contains(t, key, prefix)
		}
	})

	t.Run("Scan With Limit", func(t *testing.T) {
		prefix := fmt.Sprintf("%s:limited:", t.Name())

		// Set up test data
		for i := 0; i < 20; i++ {
			key := fmt.Sprintf("%skey-%03d", prefix, i)
			value := fmt.Sprintf("value-%03d", i)
			err := postgresPlugin.Set(key, []byte(value), 0)
			require.NoError(t, err)
		}

		// Scan with limit of 5
		keys, err := postgresPlugin.Scan(prefix, 5)
		require.NoError(t, err)
		assert.Equal(t, 5, len(keys), "Should respect limit")
	})

	t.Run("Scan With Values", func(t *testing.T) {
		prefix := fmt.Sprintf("%s:values:", t.Name())

		// Set up test data
		expected := make(map[string]string)
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("%skey-%03d", prefix, i)
			value := fmt.Sprintf("value-%03d", i)
			expected[key] = value
			err := postgresPlugin.Set(key, []byte(value), 0)
			require.NoError(t, err)
		}

		// Scan with values
		results, err := postgresPlugin.ScanWithValues(prefix, 100)
		require.NoError(t, err)
		assert.Equal(t, 5, len(results))

		// Verify all key-value pairs
		for key, value := range results {
			expectedValue, exists := expected[key]
			assert.True(t, exists, "Unexpected key: %s", key)
			assert.Equal(t, expectedValue, string(value))
		}
	})

	t.Run("Scan Empty Prefix", func(t *testing.T) {
		prefix := fmt.Sprintf("%s:empty:", t.Name())

		// Scan non-existent prefix
		keys, err := postgresPlugin.Scan(prefix, 100)
		require.NoError(t, err)
		assert.Empty(t, keys, "Should return empty list for non-existent prefix")
	})
}

func TestPostgresPattern_ConcurrentOperations(t *testing.T) {
	postgresPlugin := postgres.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres-concurrent",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"host":       postgresBackend.Host,
			"port":       postgresBackend.Port,
			"database":   postgresBackend.Database,
			"user":       postgresBackend.User,
			"password":   postgresBackend.Password,
			"max_conns":  20, // Larger pool for concurrent operations
			"min_conns":  5,
		},
	}

	harness := common.NewPatternHarness(t, postgresPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Concurrent Writes", func(t *testing.T) {
		const numWorkers = 10
		const opsPerWorker = 10

		done := make(chan error, numWorkers)

		// Launch workers
		for w := 0; w < numWorkers; w++ {
			go func(workerID int) {
				for i := 0; i < opsPerWorker; i++ {
					key := fmt.Sprintf("%s:worker-%d-key-%d", t.Name(), workerID, i)
					value := fmt.Sprintf("worker-%d-value-%d", workerID, i)
					if err := postgresPlugin.Set(key, []byte(value), 0); err != nil {
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
				key := fmt.Sprintf("%s:worker-%d-key-%d", t.Name(), w, i)
				expectedValue := fmt.Sprintf("worker-%d-value-%d", w, i)
				value, found, err := postgresPlugin.Get(key)
				require.NoError(t, err)
				assert.True(t, found, "Key %s not found", key)
				assert.Equal(t, expectedValue, string(value))
			}
		}
	})

	t.Run("Concurrent Reads", func(t *testing.T) {
		// Set up test data with unique key
		key := fmt.Sprintf("%s:shared-key", t.Name())
		err := postgresPlugin.Set(key, []byte("shared-value"), 0)
		require.NoError(t, err)

		const numReaders = 20
		done := make(chan error, numReaders)

		// Launch readers
		for r := 0; r < numReaders; r++ {
			go func() {
				value, found, err := postgresPlugin.Get(key)
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

	t.Run("Concurrent Mixed Operations", func(t *testing.T) {
		const numWorkers = 10
		done := make(chan error, numWorkers)

		// Launch workers doing mixed operations
		for w := 0; w < numWorkers; w++ {
			go func(workerID int) {
				keyPrefix := fmt.Sprintf("%s:mixed-worker-%d", t.Name(), workerID)

				// Write
				key := fmt.Sprintf("%s-key", keyPrefix)
				err := postgresPlugin.Set(key, []byte("value"), 0)
				if err != nil {
					done <- err
					return
				}

				// Read
				_, found, err := postgresPlugin.Get(key)
				if err != nil {
					done <- err
					return
				}
				if !found {
					done <- fmt.Errorf("key not found after set")
					return
				}

				// Exists
				exists, err := postgresPlugin.Exists(key)
				if err != nil {
					done <- err
					return
				}
				if !exists {
					done <- fmt.Errorf("exists returned false after set")
					return
				}

				// Delete
				err = postgresPlugin.Delete(key)
				if err != nil {
					done <- err
					return
				}

				done <- nil
			}(w)
		}

		// Wait for all workers
		for w := 0; w < numWorkers; w++ {
			err := <-done
			require.NoError(t, err, "Mixed operations worker failed")
		}
	})
}

func TestPostgresPattern_HealthCheck(t *testing.T) {
	postgresPlugin := postgres.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres-health",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"host":     postgresBackend.Host,
			"port":     postgresBackend.Port,
			"database": postgresBackend.Database,
			"user":     postgresBackend.User,
			"password": postgresBackend.Password,
		},
	}

	harness := common.NewPatternHarness(t, postgresPlugin, config)
	defer harness.Cleanup()

	t.Run("Healthy Status", func(t *testing.T) {
		status, err := postgresPlugin.Health(harness.Context())
		require.NoError(t, err)
		assert.Equal(t, plugin.HealthHealthy, status.Status)
		assert.NotEmpty(t, status.Message)
		assert.NotEmpty(t, status.Details)

		// Check for expected details
		assert.Contains(t, status.Details, "max_conns")
		assert.Contains(t, status.Details, "total_conns")
		assert.Contains(t, status.Details, "idle_conns")
	})
}

func TestPostgresPattern_TransactionIsolation(t *testing.T) {
	postgresPlugin := postgres.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres-isolation",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"host":     postgresBackend.Host,
			"port":     postgresBackend.Port,
			"database": postgresBackend.Database,
			"user":     postgresBackend.User,
			"password": postgresBackend.Password,
		},
	}

	harness := common.NewPatternHarness(t, postgresPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Isolated Writes", func(t *testing.T) {
		key := fmt.Sprintf("%s:isolation-key", t.Name())

		// Writer 1 writes "value1"
		err := postgresPlugin.Set(key, []byte("value1"), 0)
		require.NoError(t, err)

		// Writer 2 overwrites with "value2"
		err = postgresPlugin.Set(key, []byte("value2"), 0)
		require.NoError(t, err)

		// Read should get the latest value
		value, found, err := postgresPlugin.Get(key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "value2", string(value))
	})
}

func TestPostgresPattern_ErrorHandling(t *testing.T) {
	postgresPlugin := postgres.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres-errors",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"host":     postgresBackend.Host,
			"port":     postgresBackend.Port,
			"database": postgresBackend.Database,
			"user":     postgresBackend.User,
			"password": postgresBackend.Password,
		},
	}

	harness := common.NewPatternHarness(t, postgresPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Delete Non-Existent Key", func(t *testing.T) {
		// Should not error
		key := fmt.Sprintf("%s:non-existent-key", t.Name())
		err := postgresPlugin.Delete(key)
		assert.NoError(t, err, "Deleting non-existent key should not error")
	})

	t.Run("Empty Key", func(t *testing.T) {
		// Empty key should be rejected or handled gracefully
		err := postgresPlugin.Set("", []byte("value"), 0)
		// Implementation may allow empty keys or reject them
		// Just verify it doesn't panic
		t.Logf("Empty key result: %v", err)
	})
}
