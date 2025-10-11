//go:build stress

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

// TestKeyValueInterface_StressTest performs high-load stress testing
// Tests stability under extreme concurrent load
// Run with: go test -tags=stress -v -run TestKeyValueInterface_StressTest
func TestKeyValueInterface_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

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

			t.Run("Mixed Workload Stress Test", func(t *testing.T) {
				const duration = 5 * time.Second
				const numWriters = 10
				const numReaders = 50

				var (
					writeCount  atomic.Int64
					readCount   atomic.Int64
					deleteCount atomic.Int64
					errorCount  atomic.Int64
				)

				start := time.Now()
				deadline := start.Add(duration)

				var wg sync.WaitGroup

				// Writers
				for i := 0; i < numWriters; i++ {
					wg.Add(1)
					go func(workerID int) {
						defer wg.Done()
						for time.Now().Before(deadline) {
							key := fmt.Sprintf("stress-key-%d-%d", workerID, time.Now().UnixNano())
							value := fmt.Sprintf("stress-value-%d", workerID)
							if err := driver.Set(key, []byte(value), 0); err != nil {
								errorCount.Add(1)
							} else {
								writeCount.Add(1)
							}
						}
					}(i)
				}

				// Readers
				for i := 0; i < numReaders; i++ {
					wg.Add(1)
					go func(workerID int) {
						defer wg.Done()
						for time.Now().Before(deadline) {
							key := fmt.Sprintf("stress-key-%d-%d", workerID%numWriters, time.Now().UnixNano())
							_, _, err := driver.Get(key)
							if err != nil {
								errorCount.Add(1)
							} else {
								readCount.Add(1)
							}
						}
					}(i)
				}

				wg.Wait()

				elapsed := time.Since(start)
				writes := writeCount.Load()
				reads := readCount.Load()
				deletes := deleteCount.Load()
				errors := errorCount.Load()

				totalOps := writes + reads + deletes
				opsPerSec := float64(totalOps) / elapsed.Seconds()
				errorRate := 0.0
				if totalOps > 0 {
					errorRate = float64(errors) / float64(totalOps) * 100
				}

				t.Logf("%s Stress Test Results:", backendSetup.Name)
				t.Logf("  Duration: %v", elapsed)
				t.Logf("  Writes: %d (%d ops/sec)", writes, int64(float64(writes)/elapsed.Seconds()))
				t.Logf("  Reads: %d (%d ops/sec)", reads, int64(float64(reads)/elapsed.Seconds()))
				t.Logf("  Deletes: %d", deletes)
				t.Logf("  Errors: %d", errors)
				t.Logf("  Total ops/sec: %.2f", opsPerSec)
				t.Logf("  Error rate: %.2f%%", errorRate)

				// Assertions for stability
				assert.Greater(t, totalOps, int64(100), "Should perform significant number of operations")
				assert.Less(t, errorRate, 1.0, "Error rate %.2f%% exceeds 1%% threshold", errorRate)
			})
		})
	}
}

// TestKeyValueInterface_TTLConcurrency tests TTL operations under concurrent access
// Run with: go test -tags=stress -v -run TestKeyValueInterface_TTLConcurrency
func TestKeyValueInterface_TTLConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

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
		if !backendSetup.SupportsTTL {
			continue
		}

		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Concurrent Sets With Different TTLs", func(t *testing.T) {
				const numWorkers = 10
				const opsPerWorker = 100

				var wg sync.WaitGroup
				wg.Add(numWorkers)

				for w := 0; w < numWorkers; w++ {
					go func(workerID int) {
						defer wg.Done()
						for i := 0; i < opsPerWorker; i++ {
							key := fmt.Sprintf("ttl-key-%d-%d", workerID, i)
							value := fmt.Sprintf("ttl-value-%d", workerID)
							ttl := int64(workerID + 1) // Different TTLs per worker
							err := driver.Set(key, []byte(value), ttl)
							require.NoError(t, err)
						}
					}(w)
				}

				wg.Wait()
			})

			t.Run("Read While TTL Expires", func(t *testing.T) {
				// Set keys with very short TTLs
				for i := 0; i < 100; i++ {
					key := fmt.Sprintf("expiring-key-%d", i)
					value := fmt.Sprintf("expiring-value-%d", i)
					err := driver.Set(key, []byte(value), 1) // 1 second TTL
					require.NoError(t, err)
				}

				// Continuously read while keys expire
				start := time.Now()
				deadline := start.Add(3 * time.Second)

				var wg sync.WaitGroup
				const numReaders = 10

				for r := 0; r < numReaders; r++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for time.Now().Before(deadline) {
							for i := 0; i < 100; i++ {
								key := fmt.Sprintf("expiring-key-%d", i)
								// Don't assert - key may or may not exist depending on TTL
								driver.Get(key)
							}
						}
					}()
				}

				wg.Wait()
			})
		})
	}
}
