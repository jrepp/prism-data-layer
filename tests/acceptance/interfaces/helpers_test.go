package interfaces_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetStandardBackends returns the standard list of backend drivers to test
// This eliminates duplication of backend setup across test files
func GetStandardBackends() []BackendDriverSetup {
	return []BackendDriverSetup{
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
}

// ConcurrencyTestConfig holds configuration for concurrency tests
type ConcurrencyTestConfig struct {
	NumWorkers     int
	OpsPerWorker   int
	MaxConcurrency int
	UseSharedKey   bool
	VerifyReadback bool
}

// DefaultConcurrencyConfig returns sensible defaults for concurrency tests
func DefaultConcurrencyConfig() ConcurrencyTestConfig {
	return ConcurrencyTestConfig{
		NumWorkers:     10,
		OpsPerWorker:   10,
		MaxConcurrency: 5,
		UseSharedKey:   false,
		VerifyReadback: true,
	}
}

// RunConcurrentWrites executes concurrent write operations and verifies results
// This is a helper for the common pattern of:
// 1. Launch N workers
// 2. Each worker writes M keys
// 3. Verify all keys were written correctly
func RunConcurrentWrites(t *testing.T, driver KeyValueBasicDriver, testName string, cfg ConcurrencyTestConfig) {
	t.Helper()

	var wg sync.WaitGroup
	errors := make(chan error, cfg.NumWorkers)

	// Launch workers
	for w := 0; w < cfg.NumWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < cfg.OpsPerWorker; i++ {
				var key string
				if cfg.UseSharedKey {
					key = fmt.Sprintf("%s:shared-key", testName)
				} else {
					key = fmt.Sprintf("%s:worker-%d-key-%d", testName, workerID, i)
				}
				value := fmt.Sprintf("worker-%d-value-%d", workerID, i)

				if err := driver.Set(key, []byte(value), 0); err != nil {
					errors <- fmt.Errorf("worker %d op %d failed: %w", workerID, i, err)
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "Workers encountered errors")

	// Verify all keys written (if not using shared key and readback enabled)
	if !cfg.UseSharedKey && cfg.VerifyReadback {
		VerifyKeysWritten(t, driver, testName, cfg.NumWorkers, cfg.OpsPerWorker)
	}
}

// VerifyKeysWritten checks that all expected keys exist with correct values
func VerifyKeysWritten(t *testing.T, driver KeyValueBasicDriver, testName string, numWorkers, opsPerWorker int) {
	t.Helper()

	for w := 0; w < numWorkers; w++ {
		for i := 0; i < opsPerWorker; i++ {
			key := fmt.Sprintf("%s:worker-%d-key-%d", testName, w, i)
			expectedValue := fmt.Sprintf("worker-%d-value-%d", w, i)

			value, found, err := driver.Get(key)
			require.NoError(t, err, "Failed to get key %s", key)
			assert.True(t, found, "Key %s not found", key)
			assert.Equal(t, expectedValue, string(value), "Value mismatch for key %s", key)
		}
	}
}

// RunConcurrentReads executes concurrent read operations against a known key
func RunConcurrentReads(t *testing.T, driver KeyValueBasicDriver, testName string, numReaders int) {
	t.Helper()

	// Setup: Write known value
	testKey := fmt.Sprintf("%s:shared-read-key", testName)
	testValue := []byte("shared-value")
	err := driver.Set(testKey, testValue, 0)
	require.NoError(t, err, "Failed to setup test key")

	var wg sync.WaitGroup
	errors := make(chan error, numReaders)

	// Launch concurrent readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			value, found, err := driver.Get(testKey)
			if err != nil {
				errors <- fmt.Errorf("reader %d failed: %w", readerID, err)
				return
			}
			if !found {
				errors <- fmt.Errorf("reader %d: key not found", readerID)
				return
			}
			if string(value) != string(testValue) {
				errors <- fmt.Errorf("reader %d: got %s, want %s", readerID, string(value), string(testValue))
				return
			}
		}(r)
	}

	wg.Wait()
	close(errors)

	// Verify no errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "Readers encountered errors")
}

// RunWorkerPool executes operations using worker pool pattern
func RunWorkerPool(t *testing.T, driver KeyValueBasicDriver, testName string, poolSize, numTasks int) {
	t.Helper()

	var wg sync.WaitGroup
	taskQueue := make(chan int, poolSize*2)
	errors := make(chan error, poolSize)

	// Worker function
	worker := func() {
		defer wg.Done()
		for taskID := range taskQueue {
			key := fmt.Sprintf("%s:task-%d", testName, taskID)
			value := fmt.Sprintf("result-%d", taskID)
			if err := driver.Set(key, []byte(value), 0); err != nil {
				errors <- err
			}
		}
	}

	// Start workers
	for i := 0; i < poolSize; i++ {
		wg.Add(1)
		go worker()
	}

	// Submit tasks
	for i := 0; i < numTasks; i++ {
		taskQueue <- i
	}
	close(taskQueue)

	// Wait for completion
	wg.Wait()
	close(errors)

	// Check errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "Worker pool encountered errors")

	// Verify all tasks completed
	for i := 0; i < numTasks; i++ {
		key := fmt.Sprintf("%s:task-%d", testName, i)
		_, found, err := driver.Get(key)
		require.NoError(t, err, "Failed to verify task %d", i)
		assert.True(t, found, "Task %d result not found", i)
	}
}

// RunBoundedConcurrency executes operations with a semaphore-based limit
// Returns the maximum observed concurrency level
func RunBoundedConcurrency(t *testing.T, driver KeyValueBasicDriver, testName string, maxConcurrency, numOps int) int32 {
	t.Helper()

	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var activeCount atomic.Int32
	var maxActive int32

	errors := make(chan error, numOps)

	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(opID int) {
			defer wg.Done()

			// Acquire semaphore slot
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Track active operations
			active := activeCount.Add(1)
			if active > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, active)
			}
			defer activeCount.Add(-1)

			// Perform operation
			key := fmt.Sprintf("%s:bulkhead-key-%d", testName, opID)
			value := fmt.Sprintf("bulkhead-value-%d", opID)
			if err := driver.Set(key, []byte(value), 0); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Verify no errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "Bounded concurrency test encountered errors")

	return atomic.LoadInt32(&maxActive)
}

// RunAtomicDeleteTest tests that delete operations are atomic under concurrent access
func RunAtomicDeleteTest(t *testing.T, driver KeyValueBasicDriver, testName string, numAttempts int) {
	t.Helper()

	key := fmt.Sprintf("%s:atomic-delete-key", testName)
	err := driver.Set(key, []byte("value"), 0)
	require.NoError(t, err, "Failed to setup key for atomic delete test")

	var wg sync.WaitGroup
	deleteSucceeded := atomic.Int32{}
	notFoundCount := atomic.Int32{}

	// Multiple goroutines try to delete same key
	for i := 0; i < numAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check if exists before delete
			exists, err := driver.Exists(key)
			if err != nil {
				return
			}

			if !exists {
				notFoundCount.Add(1)
				return
			}

			// Try to delete
			if err := driver.Delete(key); err == nil {
				deleteSucceeded.Add(1)
			}
		}()
	}

	wg.Wait()

	// Key should be gone
	exists, err := driver.Exists(key)
	require.NoError(t, err, "Failed to check key existence after delete")
	assert.False(t, exists, "Key should be deleted")

	t.Logf("Delete succeeded: %d, Not found: %d", deleteSucceeded.Load(), notFoundCount.Load())
}

// SetupTestData writes a set of test keys with known values
// Returns the number of keys written
func SetupTestData(t *testing.T, driver KeyValueBasicDriver, testName string, numKeys int) int {
	t.Helper()

	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("%s:setup-key-%d", testName, i)
		value := fmt.Sprintf("setup-value-%d", i)
		err := driver.Set(key, []byte(value), 0)
		require.NoError(t, err, "Failed to setup test key %d", i)
	}

	return numKeys
}

// MeasureOperationRate executes a batch of operations and returns ops/sec
func MeasureOperationRate(t *testing.T, driver KeyValueBasicDriver, testName string, numOps int) float64 {
	t.Helper()

	// start := time.Now()

	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s:perf-key-%d", testName, i)
		value := fmt.Sprintf("value-%d", i)
		err := driver.Set(key, []byte(value), 0)
		require.NoError(t, err, "Operation %d failed", i)
	}

	// TODO: Uncomment when we want actual performance measurements
	// duration := time.Since(start)
	// return float64(numOps) / duration.Seconds()

	return 0 // Placeholder for now
}
