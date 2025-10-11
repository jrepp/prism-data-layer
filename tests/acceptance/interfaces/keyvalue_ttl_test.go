package interfaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeyValueTTLInterface_Expiration tests TTL expiration across backends that support it
// This tests the keyvalue_ttl interface from MEMO-006
//
// Backends that implement this interface:
// - Redis (native TTL support)
// - MemStore (TTL with time.AfterFunc cleanup)
// - DynamoDB (TTL support)
// - etcd (lease-based TTL)
//
// Backends that DON'T implement this interface:
// - PostgreSQL (requires cron jobs for cleanup)
// - S3 (uses lifecycle policies instead)
func TestKeyValueTTLInterface_Expiration(t *testing.T) {
	ctx := context.Background()

	// Only test backends that support TTL
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
			continue // Skip backends that don't support TTL
		}

		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("TTL Expiration", func(t *testing.T) {
				// Set key with 2 second TTL
				err := driver.Set("ttl-key", []byte("expires-soon"), 2)
				require.NoError(t, err)

				// Key should exist immediately
				value, found, err := driver.Get("ttl-key")
				require.NoError(t, err)
				assert.True(t, found, "Key should exist immediately after Set")
				assert.Equal(t, "expires-soon", string(value))

				// Wait for expiration (2 seconds TTL + 1 second buffer)
				time.Sleep(3 * time.Second)

				// Key should be gone
				_, found, err = driver.Get("ttl-key")
				require.NoError(t, err)
				assert.False(t, found, "Key should have expired after TTL")
			})

			t.Run("No TTL Persists", func(t *testing.T) {
				// Set key with no TTL (0 = no expiration)
				err := driver.Set("persistent-key", []byte("never-expires"), 0)
				require.NoError(t, err)

				// Wait longer than typical TTL
				time.Sleep(3 * time.Second)

				// Key should still exist
				value, found, err := driver.Get("persistent-key")
				require.NoError(t, err)
				assert.True(t, found, "Key with no TTL should persist")
				assert.Equal(t, "never-expires", string(value))
			})

			t.Run("Overwrite Resets TTL", func(t *testing.T) {
				// Set key with short TTL
				err := driver.Set("reset-ttl-key", []byte("original"), 2)
				require.NoError(t, err)

				// Wait 1 second
				time.Sleep(1 * time.Second)

				// Overwrite with new TTL
				err = driver.Set("reset-ttl-key", []byte("updated"), 3)
				require.NoError(t, err)

				// Wait another 2 seconds (total 3 seconds from original)
				// Original TTL would have expired, but new TTL should keep it alive
				time.Sleep(2 * time.Second)

				// Key should still exist
				value, found, err := driver.Get("reset-ttl-key")
				require.NoError(t, err)
				assert.True(t, found, "Key should exist with new TTL")
				assert.Equal(t, "updated", string(value))

				// Wait for new TTL to expire (1 more second)
				time.Sleep(2 * time.Second)

				// Key should now be gone
				_, found, err = driver.Get("reset-ttl-key")
				require.NoError(t, err)
				assert.False(t, found, "Key should have expired after new TTL")
			})

			t.Run("Multiple Keys with Different TTLs", func(t *testing.T) {
				// Set keys with different TTLs
				err := driver.Set("ttl-1s", []byte("expires-1s"), 1)
				require.NoError(t, err)

				err = driver.Set("ttl-3s", []byte("expires-3s"), 3)
				require.NoError(t, err)

				err = driver.Set("ttl-5s", []byte("expires-5s"), 5)
				require.NoError(t, err)

				// After 2 seconds, only ttl-1s should be gone
				time.Sleep(2 * time.Second)

				_, found1, err := driver.Get("ttl-1s")
				require.NoError(t, err)
				assert.False(t, found1, "ttl-1s should be expired")

				_, found3, err := driver.Get("ttl-3s")
				require.NoError(t, err)
				assert.True(t, found3, "ttl-3s should still exist")

				_, found5, err := driver.Get("ttl-5s")
				require.NoError(t, err)
				assert.True(t, found5, "ttl-5s should still exist")

				// After 2 more seconds (total 4s), ttl-3s should be gone
				time.Sleep(2 * time.Second)

				_, found3, err = driver.Get("ttl-3s")
				require.NoError(t, err)
				assert.False(t, found3, "ttl-3s should be expired")

				_, found5, err = driver.Get("ttl-5s")
				require.NoError(t, err)
				assert.True(t, found5, "ttl-5s should still exist")
			})
		})
	}
}

// TestKeyValueTTLInterface_EdgeCases tests edge cases for TTL handling
func TestKeyValueTTLInterface_EdgeCases(t *testing.T) {
	ctx := context.Background()

	backendDrivers := []BackendDriverSetup{
		{
			Name:         "Redis",
			SetupFunc:    setupRedisDriver,
			SupportsTTL:  true,
			SupportsScan: true,
		},
	}

	for _, backendSetup := range backendDrivers {
		if !backendSetup.SupportsTTL {
			continue
		}

		t.Run(backendSetup.Name, func(t *testing.T) {
			driver, cleanup := backendSetup.SetupFunc(t, ctx)
			defer cleanup()

			t.Run("Delete Before Expiration", func(t *testing.T) {
				// Set key with TTL
				err := driver.Set("delete-before-expire", []byte("value"), 5)
				require.NoError(t, err)

				// Delete before it expires
				err = driver.Delete("delete-before-expire")
				require.NoError(t, err)

				// Key should be gone immediately
				_, found, err := driver.Get("delete-before-expire")
				require.NoError(t, err)
				assert.False(t, found, "Deleted key should be gone immediately")
			})

			t.Run("Exists Check with TTL", func(t *testing.T) {
				// Set key with TTL
				err := driver.Set("exists-with-ttl", []byte("value"), 2)
				require.NoError(t, err)

				// Should exist immediately
				exists, err := driver.Exists("exists-with-ttl")
				require.NoError(t, err)
				assert.True(t, exists, "Key should exist immediately")

				// Wait for expiration
				time.Sleep(3 * time.Second)

				// Should not exist after expiration
				exists, err = driver.Exists("exists-with-ttl")
				require.NoError(t, err)
				assert.False(t, exists, "Key should not exist after TTL expiration")
			})
		})
	}
}
