package keyvalue_ttl

import (
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Suite provides reusable tests for KeyValue with TTL support.
// Any backend that implements plugin.KeyValueBasicInterface with TTL can use this suite.
type Suite struct {
	CreateBackend  func(t *testing.T) plugin.KeyValueBasicInterface
	CleanupBackend func(t *testing.T, backend plugin.KeyValueBasicInterface)
}

// Run executes all KeyValue TTL tests
func (s *Suite) Run(t *testing.T) {
	t.Run("SetWithTTL", s.TestSetWithTTL)
	t.Run("TTLExpiration", s.TestTTLExpiration)
	t.Run("OverwriteTTL", s.TestOverwriteTTL)
	t.Run("ZeroTTL", s.TestZeroTTL)
}

// TestSetWithTTL verifies setting values with TTL
func (s *Suite) TestSetWithTTL(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "ttl-test"
	value := []byte("value")
	ttl := int64(60) // 60 seconds

	err := backend.Set(key, value, ttl)
	require.NoError(t, err)

	// Value should exist immediately
	retrieved, found, err := backend.Get(key)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, value, retrieved)
}

// TestTTLExpiration verifies values expire after TTL
func (s *Suite) TestTTLExpiration(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "expire-test"
	value := []byte("value")
	ttl := int64(2) // 2 seconds

	err := backend.Set(key, value, ttl)
	require.NoError(t, err)

	// Value exists initially
	_, found, err := backend.Get(key)
	require.NoError(t, err)
	assert.True(t, found, "Value should exist before expiration")

	// Wait for expiration
	time.Sleep(3 * time.Second)

	// Value should be expired
	_, found, err = backend.Get(key)
	require.NoError(t, err)
	assert.False(t, found, "Value should not exist after expiration")
}

// TestOverwriteTTL verifies updating TTL on existing key
func (s *Suite) TestOverwriteTTL(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "overwrite-ttl"
	value1 := []byte("value1")
	value2 := []byte("value2")

	// Set with short TTL
	err := backend.Set(key, value1, 2)
	require.NoError(t, err)

	// Overwrite with longer TTL
	err = backend.Set(key, value2, 60)
	require.NoError(t, err)

	// Wait past original TTL
	time.Sleep(3 * time.Second)

	// Value should still exist (new TTL)
	retrieved, found, err := backend.Get(key)
	require.NoError(t, err)
	assert.True(t, found, "Value should exist with new TTL")
	assert.Equal(t, value2, retrieved)
}

// TestZeroTTL verifies zero TTL means no expiration
func (s *Suite) TestZeroTTL(t *testing.T) {
	backend := s.CreateBackend(t)
	defer s.CleanupBackend(t, backend)

	key := "no-expiry"
	value := []byte("value")

	err := backend.Set(key, value, 0)
	require.NoError(t, err)

	// Value should persist
	time.Sleep(2 * time.Second)

	retrieved, found, err := backend.Get(key)
	require.NoError(t, err)
	assert.True(t, found, "Value with zero TTL should not expire")
	assert.Equal(t, value, retrieved)
}
