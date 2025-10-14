package keyvalue_test

import (
	"fmt"
	"testing"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Import backends to register them with the framework
	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends"
)

// TestKeyValueBasicPattern executes the KeyValue Basic interface test suite
// against all registered backends that support the pattern
func TestKeyValueBasicPattern(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name: "SetAndGet",
			Func: testSetAndGet,
		},
		{
			Name: "GetNonExistent",
			Func: testGetNonExistent,
		},
		{
			Name: "Overwrite",
			Func: testOverwrite,
		},
		{
			Name: "BinaryData",
			Func: testBinaryData,
		},
		{
			Name: "EmptyValue",
			Func: testEmptyValue,
		},
		{
			Name: "LargeValue",
			Func: testLargeValue,
		},
		{
			Name: "DeleteExisting",
			Func: testDeleteExisting,
		},
		{
			Name: "DeleteNonExistent",
			Func: testDeleteNonExistent,
		},
		{
			Name: "ExistsTrue",
			Func: testExistsTrue,
		},
		{
			Name: "ExistsFalse",
			Func: testExistsFalse,
		},
	}

	framework.RunPatternTests(t, framework.PatternKeyValueBasic, tests)
}

// testSetAndGet verifies basic Set and Get operations
func testSetAndGet(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:test-key", t.Name())
	value := []byte("test-value")

	// Set value
	err := kv.Set(key, value, 0)
	require.NoError(t, err, "Set should succeed")

	// Get value
	retrieved, found, err := kv.Get(key)
	require.NoError(t, err, "Get should not error")
	assert.True(t, found, "Key should be found")
	assert.Equal(t, value, retrieved, "Retrieved value should match")
}

// testGetNonExistent verifies Get behavior for non-existent keys
func testGetNonExistent(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:non-existent", t.Name())

	value, found, err := kv.Get(key)
	require.NoError(t, err, "Get should not error for non-existent key")
	assert.False(t, found, "Non-existent key should not be found")
	assert.Nil(t, value, "Value should be nil for non-existent key")
}

// testOverwrite verifies that Set can overwrite existing keys
func testOverwrite(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:overwrite", t.Name())

	// Set initial value
	err := kv.Set(key, []byte("original"), 0)
	require.NoError(t, err)

	// Overwrite with new value
	err = kv.Set(key, []byte("updated"), 0)
	require.NoError(t, err)

	// Verify new value
	value, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("updated"), value)
}

// testBinaryData verifies that binary data is stored correctly
func testBinaryData(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:binary", t.Name())
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}

	err := kv.Set(key, binaryData, 0)
	require.NoError(t, err)

	value, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, binaryData, value, "Binary data should be preserved")
}

// testEmptyValue verifies that empty values can be stored
func testEmptyValue(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:empty", t.Name())
	emptyValue := []byte{}

	err := kv.Set(key, emptyValue, 0)
	require.NoError(t, err)

	value, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found, "Empty value key should exist")
	assert.Equal(t, emptyValue, value, "Empty value should be preserved")
}

// testLargeValue verifies that large values can be stored
func testLargeValue(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:large", t.Name())

	// Create 1MB value (or backend max, whichever is smaller)
	size := 1024 * 1024 // 1MB default
	if caps.MaxValueSize > 0 && caps.MaxValueSize < int64(size) {
		size = int(caps.MaxValueSize)
	}

	largeValue := make([]byte, size)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err := kv.Set(key, largeValue, 0)
	require.NoError(t, err, "Should handle large values up to MaxValueSize")

	value, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, len(largeValue), len(value), "Large value length should match")
	assert.Equal(t, largeValue, value, "Large value content should match")
}

// testDeleteExisting verifies that existing keys can be deleted
func testDeleteExisting(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:to-delete", t.Name())

	// Create key
	err := kv.Set(key, []byte("temporary"), 0)
	require.NoError(t, err)

	// Delete key
	err = kv.Delete(key)
	require.NoError(t, err, "Delete should succeed")

	// Verify deleted
	_, found, err := kv.Get(key)
	require.NoError(t, err)
	assert.False(t, found, "Deleted key should not be found")
}

// testDeleteNonExistent verifies that deleting non-existent keys doesn't error
func testDeleteNonExistent(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:never-existed", t.Name())

	// Should not error when deleting non-existent key
	err := kv.Delete(key)
	assert.NoError(t, err, "Deleting non-existent key should not error")
}

// testExistsTrue verifies that Exists returns true for existing keys
func testExistsTrue(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:exists", t.Name())

	// Create key
	err := kv.Set(key, []byte("value"), 0)
	require.NoError(t, err)

	// Check existence
	exists, err := kv.Exists(key)
	require.NoError(t, err, "Exists should not error")
	assert.True(t, exists, "Exists should return true for existing key")
}

// testExistsFalse verifies that Exists returns false for non-existent keys
func testExistsFalse(t *testing.T, driver interface{}, caps framework.Capabilities) {
	kv := driver.(core.KeyValueBasicInterface)

	key := fmt.Sprintf("%s:does-not-exist", t.Name())

	exists, err := kv.Exists(key)
	require.NoError(t, err, "Exists should not error")
	assert.False(t, exists, "Exists should return false for non-existent key")
}
