package interfaces_test

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

// KeyValueBasicDriver defines the interface for basic KeyValue operations
// This maps to prism.interfaces.keyvalue.KeyValueBasicInterface proto service
type KeyValueBasicDriver interface {
	Set(key string, value []byte, ttlSeconds int64) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) error
	Exists(key string) (bool, error)
}

// BackendDriverSetup represents a backend driver setup function
type BackendDriverSetup struct {
	Name         string
	SetupFunc    func(t *testing.T, ctx context.Context) (KeyValueBasicDriver, func())
	SupportsTTL  bool
	SupportsScan bool
}

// setupRedisDriver creates a Redis backend driver for testing
func setupRedisDriver(t *testing.T, ctx context.Context) (KeyValueBasicDriver, func()) {
	t.Helper()

	// Start Redis backend using centralized backend utility
	backend := backends.SetupRedis(t, ctx)

	// Create Redis driver
	driver := redis.New()

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
	harness := common.NewPatternHarness(t, driver, config)

	// Wait for driver to be healthy
	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err, "Driver did not become healthy")

	cleanup := func() {
		harness.Cleanup()
		backend.Cleanup()
	}

	return driver, cleanup
}

// setupMemStoreDriver creates a MemStore backend driver for testing
func setupMemStoreDriver(t *testing.T, ctx context.Context) (KeyValueBasicDriver, func()) {
	t.Helper()

	// MemStore requires no external backend - just the driver
	backend := backends.SetupMemStore(t, ctx)

	// Driver is already initialized and started by SetupMemStore
	return backend.Driver, backend.Cleanup
}

// TestKeyValueBasicInterface_SetGet tests Set and Get operations across all backends
func TestKeyValueBasicInterface_SetGet(t *testing.T) {
	ctx := context.Background()

	// Table of backend drivers to test
	backendDrivers := []BackendDriverSetup{
		{
			Name:         "Redis",
			SetupFunc:    setupRedisDriver,
			SupportsTTL:  true,
			SupportsScan: true,
		},
		{
			Name:         "MemStore",
			SetupFunc:    setupMemStoreDriver,
			SupportsTTL:  true,
			SupportsScan: false, // MemStore doesn't implement keyvalue_scan interface
		},
		// Add more backend drivers here:
		// {
		//     Name:         "PostgreSQL",
		//     SetupFunc:    setupPostgresDriver,
		//     SupportsTTL:  false,
		//     SupportsScan: true,
		// },
	}

	for _, backendSetup := range backendDrivers {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Set and Get", func(t *testing.T) {
				err := driver.Set("test-key", []byte("test-value"), 0)
				require.NoError(t, err)

				value, found, err := driver.Get("test-key")
				require.NoError(t, err)
				assert.True(t, found, "Key should be found")
				assert.Equal(t, "test-value", string(value))
			})

			t.Run("Get Non-Existent Key", func(t *testing.T) {
				value, found, err := driver.Get("non-existent-key")
				require.NoError(t, err)
				assert.False(t, found, "Key should not be found")
				assert.Nil(t, value)
			})

			t.Run("Overwrite Existing Key", func(t *testing.T) {
				err := driver.Set("overwrite-key", []byte("original"), 0)
				require.NoError(t, err)

				err = driver.Set("overwrite-key", []byte("updated"), 0)
				require.NoError(t, err)

				value, found, err := driver.Get("overwrite-key")
				require.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, "updated", string(value))
			})

			t.Run("Binary Data", func(t *testing.T) {
				binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
				err := driver.Set("binary-key", binaryData, 0)
				require.NoError(t, err)

				value, found, err := driver.Get("binary-key")
				require.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, binaryData, value)
			})

			t.Run("Empty Value", func(t *testing.T) {
				err := driver.Set("empty-key", []byte(""), 0)
				require.NoError(t, err)

				value, found, err := driver.Get("empty-key")
				require.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, "", string(value))
			})

			t.Run("Large Value", func(t *testing.T) {
				// Create 1MB value
				largeValue := make([]byte, 1024*1024)
				for i := range largeValue {
					largeValue[i] = byte(i % 256)
				}

				err := driver.Set("large-key", largeValue, 0)
				require.NoError(t, err, "Should handle large values")

				value, found, err := driver.Get("large-key")
				require.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, len(largeValue), len(value))
			})
		})
	}
}

// TestKeyValueBasicInterface_Delete tests Delete operations across all backends
func TestKeyValueBasicInterface_Delete(t *testing.T) {
	ctx := context.Background()

	backendDrivers := []BackendDriverSetup{
		{
			Name:         "Redis",
			SetupFunc:    setupRedisDriver,
			SupportsTTL:  true,
			SupportsScan: true,
		},
		{
			Name:         "MemStore",
			SetupFunc:    setupMemStoreDriver,
			SupportsTTL:  true,
			SupportsScan: false,
		},
	}

	for _, backendSetup := range backendDrivers {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Delete Existing Key", func(t *testing.T) {
				err := driver.Set("delete-me", []byte("temporary"), 0)
				require.NoError(t, err)

				err = driver.Delete("delete-me")
				require.NoError(t, err)

				_, found, err := driver.Get("delete-me")
				require.NoError(t, err)
				assert.False(t, found, "Key should be deleted")
			})

			t.Run("Delete Non-Existent Key", func(t *testing.T) {
				// Should not error when deleting non-existent key
				err := driver.Delete("non-existent-key")
				assert.NoError(t, err, "Deleting non-existent key should not error")
			})
		})
	}
}

// TestKeyValueBasicInterface_Exists tests Exists operations across all backends
func TestKeyValueBasicInterface_Exists(t *testing.T) {
	ctx := context.Background()

	backendDrivers := []BackendDriverSetup{
		{
			Name:         "Redis",
			SetupFunc:    setupRedisDriver,
			SupportsTTL:  true,
			SupportsScan: true,
		},
		{
			Name:         "MemStore",
			SetupFunc:    setupMemStoreDriver,
			SupportsTTL:  true,
			SupportsScan: false,
		},
	}

	for _, backendSetup := range backendDrivers {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Exists Returns True for Existing Key", func(t *testing.T) {
				err := driver.Set("exists-key", []byte("value"), 0)
				require.NoError(t, err)

				exists, err := driver.Exists("exists-key")
				require.NoError(t, err)
				assert.True(t, exists, "Key should exist")
			})

			t.Run("Exists Returns False for Non-Existent Key", func(t *testing.T) {
				exists, err := driver.Exists("non-existent-key")
				require.NoError(t, err)
				assert.False(t, exists, "Key should not exist")
			})
		})
	}
}

// TestKeyValueBasicInterface_ConcurrentOperations tests concurrent access patterns
func TestKeyValueBasicInterface_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	backendDrivers := []BackendDriverSetup{
		{
			Name:         "Redis",
			SetupFunc:    setupRedisDriver,
			SupportsTTL:  true,
			SupportsScan: true,
		},
		{
			Name:         "MemStore",
			SetupFunc:    setupMemStoreDriver,
			SupportsTTL:  true,
			SupportsScan: false,
		},
	}

	for _, backendSetup := range backendDrivers {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

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
							if err := driver.Set(key, []byte(value), 0); err != nil {
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
						value, found, err := driver.Get(key)
						require.NoError(t, err)
						assert.True(t, found, "Key %s not found", key)
						assert.Equal(t, expectedValue, string(value))
					}
				}
			})

			t.Run("Concurrent Reads", func(t *testing.T) {
				// Set up test data
				err := driver.Set("shared-key", []byte("shared-value"), 0)
				require.NoError(t, err)

				const numReaders = 20
				done := make(chan error, numReaders)

				// Launch readers
				for r := 0; r < numReaders; r++ {
					go func() {
						value, found, err := driver.Get("shared-key")
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
		})
	}
}
