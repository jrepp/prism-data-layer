package keyvalue_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends"
)

// TestKeyValueConcurrency executes concurrency and thread-safety tests
func TestKeyValueConcurrency(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name: "ConcurrentWrites",
			Func: testConcurrentWrites,
		},
		{
			Name: "ConcurrentReads",
			Func: testConcurrentReads,
		},
		{
			Name: "ConcurrentReadWrite",
			Func: testConcurrentReadWrite,
		},
		{
			Name: "ConcurrentDeleteNoRace",
			Func: testConcurrentDeleteNoRace,
		},
	}

	framework.RunPatternTests(t, framework.PatternKeyValueBasic, tests)
}

// testConcurrentWrites verifies that concurrent writes to different keys don't corrupt data
func testConcurrentWrites(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(plugin.KeyValueBasicInterface)

	const numWorkers = 10
	const opsPerWorker = 10

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	// Launch workers writing different keys
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				key := fmt.Sprintf("%s:worker-%d-key-%d", t.Name(), workerID, i)
				value := []byte(fmt.Sprintf("worker-%d-value-%d", workerID, i))

				if err := kv.Set(key, value, 0); err != nil {
					errors <- fmt.Errorf("worker %d failed: %w", workerID, err)
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
	require.Empty(t, errs, "Workers should not encounter errors")

	// Verify all keys were written correctly
	for w := 0; w < numWorkers; w++ {
		for i := 0; i < opsPerWorker; i++ {
			key := fmt.Sprintf("%s:worker-%d-key-%d", t.Name(), w, i)
			expectedValue := []byte(fmt.Sprintf("worker-%d-value-%d", w, i))

			value, found, err := kv.Get(key)
			require.NoError(t, err, "Failed to get key %s", key)
			assert.True(t, found, "Key %s should exist", key)
			assert.Equal(t, expectedValue, value, "Value mismatch for key %s", key)
		}
	}
}

// testConcurrentReads verifies that concurrent reads of the same key work correctly
func testConcurrentReads(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(plugin.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:shared", t.Name())
	expectedValue := []byte("shared-value")

	// Set up test key
	err := kv.Set(key, expectedValue, 0)
	require.NoError(t, err)

	const numReaders = 20
	var wg sync.WaitGroup
	errors := make(chan error, numReaders)

	// Launch concurrent readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			value, found, err := kv.Get(key)
			if err != nil {
				errors <- fmt.Errorf("reader %d failed: %w", readerID, err)
				return
			}
			if !found {
				errors <- fmt.Errorf("reader %d: key not found", readerID)
				return
			}
			if string(value) != string(expectedValue) {
				errors <- fmt.Errorf("reader %d: got %s, want %s", readerID, string(value), string(expectedValue))
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
	require.Empty(t, errs, "Readers should not encounter errors")
}

// testConcurrentReadWrite verifies that concurrent reads and writes don't cause data corruption
func testConcurrentReadWrite(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(plugin.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:rw", t.Name())

	// Initialize key
	err := kv.Set(key, []byte("initial"), 0)
	require.NoError(t, err)

	const numReaders = 10
	const numWriters = 5
	const writesPerWorker = 10

	var readerWg sync.WaitGroup
	var writerWg sync.WaitGroup
	errors := make(chan error, numReaders+numWriters)
	stopReading := atomic.Bool{}
	stopReading.Store(false)

	// Launch readers
	for r := 0; r < numReaders; r++ {
		readerWg.Add(1)
		go func(readerID int) {
			defer readerWg.Done()

			for !stopReading.Load() {
				_, found, err := kv.Get(key)
				if err != nil {
					errors <- fmt.Errorf("reader %d failed: %w", readerID, err)
					return
				}
				// Value might change due to writers, but key should always exist
				if !found {
					errors <- fmt.Errorf("reader %d: key disappeared", readerID)
					return
				}
			}
		}(r)
	}

	// Launch writers
	for w := 0; w < numWriters; w++ {
		writerWg.Add(1)
		go func(writerID int) {
			defer writerWg.Done()

			for i := 0; i < writesPerWorker; i++ {
				value := []byte(fmt.Sprintf("writer-%d-iteration-%d", writerID, i))
				if err := kv.Set(key, value, 0); err != nil {
					errors <- fmt.Errorf("writer %d failed: %w", writerID, err)
					return
				}
			}
		}(w)
	}

	// Wait for writers to finish
	writerWg.Wait()

	// Stop readers and wait for them to finish
	stopReading.Store(true)
	readerWg.Wait()

	close(errors)

	// Check for errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "Concurrent read/write should not cause errors")

	// Verify key still exists
	_, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found, "Key should still exist after concurrent access")
}

// testConcurrentDeleteNoRace verifies that concurrent deletes don't cause race conditions
func testConcurrentDeleteNoRace(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(plugin.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:delete-race", t.Name())

	// Set up key
	err := kv.Set(key, []byte("value"), 0)
	require.NoError(t, err)

	const numDeleters = 10
	var wg sync.WaitGroup
	deleteSucceeded := atomic.Int32{}

	// Multiple goroutines try to delete the same key
	for i := 0; i < numDeleters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// All delete attempts should succeed (idempotent)
			if err := kv.Delete(key); err == nil {
				deleteSucceeded.Add(1)
			}
		}()
	}

	wg.Wait()

	// All deletes should succeed (delete is idempotent)
	assert.Equal(t, int32(numDeleters), deleteSucceeded.Load(),
		"All concurrent deletes should succeed (idempotent operation)")

	// Key should be gone
	exists, err := kv.Exists(key)
	require.NoError(t, err)
	assert.False(t, exists, "Key should be deleted")
}
