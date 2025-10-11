package keyvalue

import (
	"context"
	"fmt"

	"github.com/jrepp/prism-data-layer/patterns/core"
)

// KeyValueDriver defines the backend driver interface for KeyValue pattern.
// This is implemented by backend drivers (e.g., drivers/memstore, drivers/redis)
// that support the keyvalue_basic and keyvalue_ttl backend interfaces.
//
// Backend drivers that implement this interface:
// - drivers/memstore (keyvalue_basic + keyvalue_ttl interfaces)
// - drivers/redis (keyvalue_basic + keyvalue_ttl + keyvalue_scan interfaces)
// - drivers/postgres (keyvalue_basic + keyvalue_scan interfaces, no TTL)
type KeyValueDriver interface {
	core.Plugin // Lifecycle methods

	// KeyValue basic operations (keyvalue_basic interface)
	Set(key string, value []byte, ttlSeconds int64) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) error
	Exists(key string) (bool, error)

	// TODO: Add optional interfaces:
	// - keyvalue_scan (Scan, ScanKeys, Count)
	// - keyvalue_batch (BatchSet, BatchGet, BatchDelete)
	// - keyvalue_transactional (BeginTx, Commit, Rollback)
	// - keyvalue_cas (CompareAndSwap)
}

// KeyValue is a pattern implementation that wraps a backend driver.
//
// ARCHITECTURE (from MEMO-006):
// - Backend: The actual storage system (Redis, PostgreSQL, etc.)
// - Backend Driver: Go implementation that connects to backend (drivers/redis, drivers/memstore)
// - Pattern: High-level data access pattern (this file - patterns/keyvalue)
// - Interface: Thin proto service definitions (keyvalue_basic, keyvalue_ttl, etc.)
//
// The KeyValue pattern delegates to a configured backend driver.
// Configuration specifies which driver to use (memstore, redis, postgres, etc.)
//
// Example configuration:
//
//	pattern: keyvalue
//	backend_driver: memstore  # or redis, postgres, etc.
//	backend:
//	  max_keys: 10000
type KeyValue struct {
	driver KeyValueDriver
	name   string
}

// NewWithDriver creates a KeyValue pattern with a specific backend driver
func NewWithDriver(driver KeyValueDriver) *KeyValue {
	return &KeyValue{
		driver: driver,
		name:   fmt.Sprintf("keyvalue-%s", driver.Name()),
	}
}

// Name returns the pattern name
func (kv *KeyValue) Name() string {
	return kv.name
}

// Version returns the pattern version
func (kv *KeyValue) Version() string {
	return kv.driver.Version()
}

// Initialize prepares the pattern and underlying driver
func (kv *KeyValue) Initialize(ctx context.Context, config *core.Config) error {
	return kv.driver.Initialize(ctx, config)
}

// Start begins serving requests
func (kv *KeyValue) Start(ctx context.Context) error {
	return kv.driver.Start(ctx)
}

// Stop gracefully shuts down the pattern
func (kv *KeyValue) Stop(ctx context.Context) error {
	return kv.driver.Stop(ctx)
}

// Health returns the pattern health status
func (kv *KeyValue) Health(ctx context.Context) (*core.HealthStatus, error) {
	return kv.driver.Health(ctx)
}

// Set stores a key-value pair with optional TTL
func (kv *KeyValue) Set(key string, value []byte, ttlSeconds int64) error {
	return kv.driver.Set(key, value, ttlSeconds)
}

// Get retrieves a value by key
func (kv *KeyValue) Get(key string) ([]byte, bool, error) {
	return kv.driver.Get(key)
}

// Delete removes a key
func (kv *KeyValue) Delete(key string) error {
	return kv.driver.Delete(key)
}

// Exists checks if a key exists
func (kv *KeyValue) Exists(key string) (bool, error) {
	return kv.driver.Exists(key)
}
