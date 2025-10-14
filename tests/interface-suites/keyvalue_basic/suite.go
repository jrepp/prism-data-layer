package keyvalue_basic

import (
	"testing"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Suite provides reusable tests for the KeyValueBasic interface.
// Any backend that implements plugin.KeyValueBasicInterface can use this suite.
type Suite struct {
	// CreateBackend is called before each test to create a fresh backend instance
	CreateBackend func(t *testing.T) plugin.KeyValueBasicInterface
	// CleanupBackend is called after each test to cleanup
	CleanupBackend func(t *testing.T, backend plugin.KeyValueBasicInterface)
}

// Run executes all KeyValueBasic interface tests
func (s *Suite) Run(t *testing.T) {
	t.Run("SetAndGet", s.TestSetAndGet)
	t.Run("GetNonExistent", s.TestGetNonExistent)
	t.Run("Delete", s.TestDelete)
	t.Run("Exists", s.TestExists)
	t.Run("OverwriteValue", s.TestOverwriteValue)
}

// TestSetAndGet verifies Set and Get operations
func (s *Suite) TestSetAndGet(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "test-key"
	value := []byte("test-value")

	// Set a value
	err := backend.Set(key, value, 0)
	require.NoError(t, err, "Set should succeed")

	// Get the value back
	retrieved, found, err := backend.Get(key)
	require.NoError(t, err, "Get should succeed")
	assert.True(t, found, "Key should exist")
	assert.Equal(t, value, retrieved, "Retrieved value should match")
}

// TestGetNonExistent verifies Get returns not found for missing keys
func (s *Suite) TestGetNonExistent(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	_, found, err := backend.Get("nonexistent")
	require.NoError(t, err, "Get should not error on missing key")
	assert.False(t, found, "Key should not exist")
}

// TestDelete verifies Delete operation
func (s *Suite) TestDelete(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "delete-test"
	value := []byte("value")

	// Set then delete
	err := backend.Set(key, value, 0)
	require.NoError(t, err)

	err = backend.Delete(key)
	require.NoError(t, err, "Delete should succeed")

	// Verify deleted
	_, found, err := backend.Get(key)
	require.NoError(t, err)
	assert.False(t, found, "Key should not exist after delete")
}

// TestExists verifies Exists operation
func (s *Suite) TestExists(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "exists-test"

	// Check non-existent key
	exists, err := backend.Exists(key)
	require.NoError(t, err)
	assert.False(t, exists, "Key should not exist initially")

	// Set and check again
	err = backend.Set(key, []byte("value"), 0)
	require.NoError(t, err)

	exists, err = backend.Exists(key)
	require.NoError(t, err)
	assert.True(t, exists, "Key should exist after Set")
}

// TestOverwriteValue verifies overwriting existing values
func (s *Suite) TestOverwriteValue(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "overwrite-test"
	value1 := []byte("value1")
	value2 := []byte("value2")

	// Set initial value
	err := backend.Set(key, value1, 0)
	require.NoError(t, err)

	// Overwrite with new value
	err = backend.Set(key, value2, 0)
	require.NoError(t, err)

	// Verify new value
	retrieved, found, err := backend.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, value2, retrieved, "Should return overwritten value")
}
