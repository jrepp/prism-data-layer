package keyvalue_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends"
)

// TestKeyValueTTLPattern executes the KeyValue TTL interface test suite
// Tests are automatically skipped for backends that don't support TTL
func TestKeyValueTTLPattern(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name:               "SetWithTTL",
			Func:               testSetWithTTL,
			RequiresCapability: "TTL",
		},
		{
			Name:               "TTLExpiration",
			Func:               testTTLExpiration,
			RequiresCapability: "TTL",
		},
		{
			Name:               "ZeroTTLNoExpiration",
			Func:               testZeroTTLNoExpiration,
			RequiresCapability: "TTL",
		},
		{
			Name:               "OverwriteResetsExpiration",
			Func:               testOverwriteResetsExpiration,
			RequiresCapability: "TTL",
		},
	}

	framework.RunPatternTests(t, framework.PatternKeyValueTTL, tests)
}

// testSetWithTTL verifies that keys can be set with expiration
func testSetWithTTL(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:with-ttl", t.Name())
	value := []byte("expiring-value")
	ttlSeconds := int64(10) // 10 seconds

	// Set with TTL
	err := kv.Set(key, value, ttlSeconds)
	require.NoError(t, err, "Set with TTL should succeed")

	// Verify key exists immediately
	retrieved, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found, "Key should exist immediately after Set with TTL")
	assert.Equal(t, value, retrieved)

	exists, err := kv.Exists(key)
	require.NoError(t, err)
	assert.True(t, exists, "Key should exist before expiration")
}

// testTTLExpiration verifies that keys expire after TTL
func testTTLExpiration(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:expires", t.Name())
	value := []byte("short-lived")
	ttlSeconds := int64(2) // 2 seconds

	// Set with short TTL
	err := kv.Set(key, value, ttlSeconds)
	require.NoError(t, err)

	// Verify exists initially
	exists, err := kv.Exists(key)
	require.NoError(t, err)
	assert.True(t, exists, "Key should exist before expiration")

	// Wait for expiration (TTL + buffer)
	time.Sleep(time.Duration(ttlSeconds+1) * time.Second)

	// Verify key no longer exists
	exists, err = kv.Exists(key)
	require.NoError(t, err)
	assert.False(t, exists, "Key should not exist after TTL expiration")

	// Verify Get returns not found
	_, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.False(t, found, "Get should return not found after expiration")
}

// testZeroTTLNoExpiration verifies that TTL=0 means no expiration
func testZeroTTLNoExpiration(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:no-expiration", t.Name())
	value := []byte("permanent")

	// Set with TTL=0 (no expiration)
	err := kv.Set(key, value, 0)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(3 * time.Second)

	// Verify key still exists
	exists, err := kv.Exists(key)
	require.NoError(t, err)
	assert.True(t, exists, "Key with TTL=0 should not expire")

	retrieved, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, value, retrieved)
}

// testOverwriteResetsExpiration verifies that overwriting a key resets its TTL
func testOverwriteResetsExpiration(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:reset-ttl", t.Name())

	// Set with 5 second TTL
	err := kv.Set(key, []byte("first"), 5)
	require.NoError(t, err)

	// Wait 3 seconds (key should still exist)
	time.Sleep(3 * time.Second)

	// Overwrite with new value and reset TTL to 5 seconds
	err = kv.Set(key, []byte("second"), 5)
	require.NoError(t, err)

	// Wait another 3 seconds (total 6 from first Set, but 3 from second Set)
	time.Sleep(3 * time.Second)

	// Key should still exist because we reset the TTL
	exists, err := kv.Exists(key)
	require.NoError(t, err)
	assert.True(t, exists, "Key should exist after TTL reset")

	// Verify value was updated
	value, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("second"), value, "Value should be updated")

	// Wait for expiration from second Set
	time.Sleep(3 * time.Second)

	// Now key should be expired
	exists, err = kv.Exists(key)
	require.NoError(t, err)
	assert.False(t, exists, "Key should expire after reset TTL elapses")
}
