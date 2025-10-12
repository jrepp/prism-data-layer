package interfaces_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeyValueInterface_ConcurrentWrites tests parallel writes to backend drivers
// Tests the concurrency patterns from RFC-025: Worker Pool and Fan-Out patterns
func TestKeyValueInterface_ConcurrentWrites(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Single Writer High Throughput", func(t *testing.T) {
				const numOps = 1000
				start := time.Now()

				for i := 0; i < numOps; i++ {
					key := fmt.Sprintf("%s:single-writer-key-%d", t.Name(), i)
					value := fmt.Sprintf("value-%d", i)
					err := driver.Set(key, []byte(value), 0)
					require.NoError(t, err)
				}

				duration := time.Since(start)
				opsPerSecond := float64(numOps) / duration.Seconds()
				t.Logf("%s: %d operations in %v (%.0f ops/sec)",
					backendSetup.Name, numOps, duration, opsPerSecond)

				// Verify all keys written
				for i := 0; i < numOps; i++ {
					key := fmt.Sprintf("%s:single-writer-key-%d", t.Name(), i)
					_, found, err := driver.Get(key)
					require.NoError(t, err)
					assert.True(t, found, "Key %s not found", key)
				}
			})

			t.Run("Multiple Writers No Conflicts", func(t *testing.T) {
				// Use helper for concurrent writes
				cfg := DefaultConcurrencyConfig()
				cfg.NumWorkers = 10
				cfg.OpsPerWorker = 100
				RunConcurrentWrites(t, driver, t.Name(), cfg)
			})

			t.Run("Concurrent Writers Same Key", func(t *testing.T) {
				// Test contention on a single key using helper
				cfg := DefaultConcurrencyConfig()
				cfg.NumWorkers = 20
				cfg.OpsPerWorker = 50
				cfg.UseSharedKey = true
				cfg.VerifyReadback = false // Can't verify exact value with contention
				RunConcurrentWrites(t, driver, t.Name(), cfg)

				// Verify key exists with some value
				sharedKey := fmt.Sprintf("%s:shared-key", t.Name())
				value, found, err := driver.Get(sharedKey)
				require.NoError(t, err)
				assert.True(t, found)
				assert.NotEmpty(t, value)
			})

			t.Run("Worker Pool Pattern", func(t *testing.T) {
				// Use helper for worker pool pattern
				RunWorkerPool(t, driver, t.Name(), 5, 50)
			})
		})
	}
}

// TestKeyValueInterface_ConcurrentReads tests parallel reads with consistent data
// Tests the Fan-Out pattern from RFC-025
func TestKeyValueInterface_ConcurrentReads(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Single Key Many Readers", func(t *testing.T) {
				// Use helper for concurrent reads
				RunConcurrentReads(t, driver, t.Name(), 50)
			})

			t.Run("Fan-Out Read Pattern", func(t *testing.T) {
				// Setup: Write multiple keys using helper
				const numKeys = 20
				SetupTestData(t, driver, t.Name(), numKeys)

				// Fan-out: Read all keys in parallel
				type result struct {
					index int
					value string
					err   error
				}

				results := make([]result, numKeys)
				var wg sync.WaitGroup

				for i := 0; i < numKeys; i++ {
					wg.Add(1)
					go func(index int) {
						defer wg.Done()

						key := fmt.Sprintf("%s:setup-key-%d", t.Name(), index)
						value, found, err := driver.Get(key)

						if err != nil {
							results[index] = result{index: index, err: err}
							return
						}
						if !found {
							results[index] = result{index: index, err: fmt.Errorf("key not found")}
							return
						}

						results[index] = result{
							index: index,
							value: string(value),
							err:   nil,
						}
					}(i)
				}

				wg.Wait()

				// Verify all reads successful
				for i, res := range results {
					require.NoError(t, res.err, "Fan-out read %d failed", i)
					expectedValue := fmt.Sprintf("setup-value-%d", i)
					assert.Equal(t, expectedValue, res.value)
				}
			})
		})
	}
}

// TestKeyValueInterface_ReadWriteRace tests concurrent read/write operations
// Tests memory consistency and race conditions
func TestKeyValueInterface_ReadWriteRace(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Concurrent Read While Writing", func(t *testing.T) {
				const numOps = 100
				const numReaders = 10

				sharedKey := fmt.Sprintf("%s:rw-race-key", t.Name())

				// Initialize key
				err := driver.Set(sharedKey, []byte("initial"), 0)
				require.NoError(t, err)

				var wg sync.WaitGroup
				errors := make(chan error, numReaders+1)

				// Writer goroutine
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < numOps; i++ {
						value := fmt.Sprintf("value-%d", i)
						if err := driver.Set(sharedKey, []byte(value), 0); err != nil {
							errors <- fmt.Errorf("writer failed: %w", err)
							return
						}
						time.Sleep(1 * time.Millisecond)
					}
				}()

				// Reader goroutines
				for r := 0; r < numReaders; r++ {
					wg.Add(1)
					go func(readerID int) {
						defer wg.Done()
						for i := 0; i < numOps; i++ {
							_, found, err := driver.Get(sharedKey)
							if err != nil {
								errors <- fmt.Errorf("reader %d failed: %w", readerID, err)
								return
							}
							if !found {
								errors <- fmt.Errorf("reader %d: key disappeared", readerID)
								return
							}
							time.Sleep(1 * time.Millisecond)
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
				require.Empty(t, errs, "Race condition detected")
			})
		})
	}
}

// TestKeyValueInterface_BulkheadPattern tests resource isolation
// Tests the Bulkhead pattern from RFC-025
func TestKeyValueInterface_BulkheadPattern(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Bounded Concurrency with Semaphore", func(t *testing.T) {
				// Use helper for bulkhead pattern
				maxActive := RunBoundedConcurrency(t, driver, t.Name(), 5, 50)

				// Verify bulkhead respected (allow +1 for race)
				assert.LessOrEqual(t, int(maxActive), 6,
					"Max active operations (%d) exceeded bulkhead capacity (5)",
					maxActive)
			})
		})
	}
}

// TestKeyValueInterface_AtomicOperations tests atomic operations under concurrent access
// Tests consistency guarantees
func TestKeyValueInterface_AtomicOperations(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Delete Is Atomic", func(t *testing.T) {
				// Use helper for atomic delete test
				RunAtomicDeleteTest(t, driver, t.Name(), 100)
			})

			t.Run("Exists Is Consistent", func(t *testing.T) {
				const numChecks = 100

				key := fmt.Sprintf("%s:exists-consistency-key", t.Name())
				err := driver.Set(key, []byte("value"), 0)
				require.NoError(t, err)

				var wg sync.WaitGroup
				trueCount := atomic.Int32{}
				falseCount := atomic.Int32{}

				// Many goroutines check same key existence
				for i := 0; i < numChecks; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						exists, err := driver.Exists(key)
						if err != nil {
							return
						}

						if exists {
							trueCount.Add(1)
						} else {
							falseCount.Add(1)
						}
					}()
				}

				wg.Wait()

				// All checks should return true (key exists)
				assert.Equal(t, int32(numChecks), trueCount.Load(),
					"All Exists checks should return true")
				assert.Equal(t, int32(0), falseCount.Load(),
					"No Exists checks should return false")
			})
		})
	}
}

// TestKeyValueInterface_PipelinePattern tests sequential processing stages
// Tests the Pipeline pattern from RFC-025
func TestKeyValueInterface_PipelinePattern(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Three Stage Pipeline", func(t *testing.T) {
				const numItems = 20

				// Stage 1: Generate keys
				stage1 := make(chan string, numItems)
				go func() {
					defer close(stage1)
					for i := 0; i < numItems; i++ {
						stage1 <- fmt.Sprintf("%s:pipeline-key-%d", t.Name(), i)
					}
				}()

				// Stage 2: Set values
				stage2 := make(chan string, numItems)
				go func() {
					defer close(stage2)
					for key := range stage1 {
						value := []byte(key + "-value")
						if err := driver.Set(key, value, 0); err != nil {
							t.Errorf("Stage 2 failed: %v", err)
							continue
						}
						stage2 <- key
					}
				}()

				// Stage 3: Verify values
				var verified int32
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					for key := range stage2 {
						value, found, err := driver.Get(key)
						if err != nil {
							t.Errorf("Stage 3 failed: %v", err)
							continue
						}
						if !found {
							t.Errorf("Stage 3: key %s not found", key)
							continue
						}

						expectedValue := key + "-value"
						if string(value) != expectedValue {
							t.Errorf("Stage 3: got %s, want %s", string(value), expectedValue)
							continue
						}

						atomic.AddInt32(&verified, 1)
					}
				}()

				wg.Wait()

				// All items should be processed through pipeline
				assert.Equal(t, int32(numItems), atomic.LoadInt32(&verified),
					"All items should pass through pipeline")
			})
		})
	}
}

// TestKeyValueInterface_GracefulDegradation tests behavior under failure scenarios
// Tests resilience patterns from RFC-025
func TestKeyValueInterface_GracefulDegradation(t *testing.T) {
	ctx := context.Background()

	for _, backendSetup := range GetStandardBackends() {
		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Continue Operation After Errors", func(t *testing.T) {
				const numOps = 50

				var successCount atomic.Int32
				var errorCount atomic.Int32

				var wg sync.WaitGroup

				for i := 0; i < numOps; i++ {
					wg.Add(1)
					go func(opID int) {
						defer wg.Done()

						key := fmt.Sprintf("%s:degraded-key-%d", t.Name(), opID)
						value := fmt.Sprintf("value-%d", opID)

						// Try operation
						err := driver.Set(key, []byte(value), 0)
						if err != nil {
							errorCount.Add(1)
						} else {
							successCount.Add(1)
						}
					}(i)
				}

				wg.Wait()

				t.Logf("Success: %d, Errors: %d",
					successCount.Load(), errorCount.Load())

				// Most operations should succeed (>95%)
				successRate := float64(successCount.Load()) / float64(numOps) * 100
				assert.Greater(t, successRate, 95.0,
					"Success rate %.1f%% should be >95%%", successRate)
			})
		})
	}
}
