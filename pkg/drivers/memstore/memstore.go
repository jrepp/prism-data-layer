package memstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
)

// MemStore implements an in-memory key-value store plugin
type MemStore struct {
	name       string
	version    string
	data       sync.Map
	ttl        sync.Map // key -> expiration time
	config     *Config
	configLock sync.RWMutex // Protects config field
	stopCh     chan struct{}
}

// Config holds memstore-specific configuration
type Config struct {
	MaxKeys       int           `yaml:"max_keys"`
	CleanupPeriod time.Duration `yaml:"cleanup_period"`
}

// New creates a new MemStore plugin
func New() *MemStore {
	return &MemStore{
		name:    "memstore",
		version: "0.1.0",
		stopCh:  make(chan struct{}),
	}
}

// Name returns the plugin name
func (m *MemStore) Name() string {
	return m.name
}

// Version returns the plugin version
func (m *MemStore) Version() string {
	return m.version
}

// Initialize prepares the plugin with configuration
func (m *MemStore) Initialize(ctx context.Context, config *plugin.Config) error {
	// Extract backend-specific config
	var backendConfig Config
	if err := config.GetBackendConfig(&backendConfig); err != nil {
		return fmt.Errorf("failed to parse backend config: %w", err)
	}

	// Apply defaults
	if backendConfig.MaxKeys == 0 {
		backendConfig.MaxKeys = 10000 // Default: 10k keys
	}
	if backendConfig.CleanupPeriod == 0 {
		backendConfig.CleanupPeriod = 60 * time.Second // Default: 1 minute
	}

	m.configLock.Lock()
	m.config = &backendConfig
	m.configLock.Unlock()

	return nil
}

// Start begins serving requests
func (m *MemStore) Start(ctx context.Context) error {
	// Start TTL cleanup goroutine
	go m.cleanupExpiredKeys(ctx)

	// Start returns immediately - the cleanup goroutine runs in background
	return nil
}

// Stop gracefully shuts down the plugin
func (m *MemStore) Stop(ctx context.Context) error {
	// Clear all data
	m.data.Range(func(key, value interface{}) bool {
		m.data.Delete(key)
		return true
	})
	m.ttl.Range(func(key, value interface{}) bool {
		m.ttl.Delete(key)
		return true
	})
	return nil
}

// Health returns the plugin health status
func (m *MemStore) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	keyCount := 0
	m.data.Range(func(key, value interface{}) bool {
		keyCount++
		return true
	})

	m.configLock.RLock()
	config := m.config
	m.configLock.RUnlock()

	status := plugin.HealthHealthy
	message := fmt.Sprintf("healthy, %d keys stored", keyCount)

	if config != nil && keyCount >= config.MaxKeys {
		status = plugin.HealthDegraded
		message = fmt.Sprintf("at capacity: %d/%d keys", keyCount, config.MaxKeys)
	}

	details := map[string]string{
		"keys": fmt.Sprintf("%d", keyCount),
	}

	if config != nil {
		details["max_keys"] = fmt.Sprintf("%d", config.MaxKeys)
	}

	return &plugin.HealthStatus{
		Status:  status,
		Message: message,
		Details: details,
	}, nil
}

// Set stores a value with optional TTL
func (m *MemStore) Set(key string, value []byte, ttlSeconds int64) error {
	// Check capacity
	m.configLock.RLock()
	config := m.config
	m.configLock.RUnlock()

	if config != nil {
		keyCount := 0
		m.data.Range(func(k, v interface{}) bool {
			keyCount++
			return true
		})

		if keyCount >= config.MaxKeys {
			_, exists := m.data.Load(key)
			if !exists {
				return fmt.Errorf("capacity limit reached: %d keys", config.MaxKeys)
			}
		}
	}

	m.data.Store(key, value)

	if ttlSeconds > 0 {
		expiration := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
		m.ttl.Store(key, expiration)
	} else {
		m.ttl.Delete(key) // No TTL
	}

	return nil
}

// Get retrieves a value by key
func (m *MemStore) Get(key string) ([]byte, bool, error) {
	// Check if key is expired
	if exp, ok := m.ttl.Load(key); ok {
		if time.Now().After(exp.(time.Time)) {
			m.data.Delete(key)
			m.ttl.Delete(key)
			return nil, false, nil
		}
	}

	value, ok := m.data.Load(key)
	if !ok {
		return nil, false, nil
	}

	return value.([]byte), true, nil
}

// Delete removes a key
func (m *MemStore) Delete(key string) error {
	m.data.Delete(key)
	m.ttl.Delete(key)
	return nil
}

// Exists checks if a key exists (and is not expired)
func (m *MemStore) Exists(key string) (bool, error) {
	// Check if key is expired
	if exp, ok := m.ttl.Load(key); ok {
		if time.Now().After(exp.(time.Time)) {
			m.data.Delete(key)
			m.ttl.Delete(key)
			return false, nil
		}
	}

	_, ok := m.data.Load(key)
	return ok, nil
}

// cleanupExpiredKeys periodically removes expired keys
func (m *MemStore) cleanupExpiredKeys(ctx context.Context) {
	// Get cleanup period with lock, use default if not initialized
	m.configLock.RLock()
	cleanupPeriod := 60 * time.Second // Default
	if m.config != nil {
		cleanupPeriod = m.config.CleanupPeriod
	}
	m.configLock.RUnlock()

	ticker := time.NewTicker(cleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			m.ttl.Range(func(key, value interface{}) bool {
				expiration := value.(time.Time)
				if now.After(expiration) {
					m.data.Delete(key)
					m.ttl.Delete(key)
				}
				return true
			})
		}
	}
}

// Compile-time interface compliance checks
// These ensure that MemStore implements the expected interfaces
var (
	_ plugin.Plugin                 = (*MemStore)(nil) // Core plugin interface
	_ plugin.KeyValueBasicInterface = (*MemStore)(nil) // KeyValue basic operations
)

// GetInterfaceDeclarations returns the interfaces this driver implements
// This is used during registration with the proxy (replacing runtime introspection)
func (m *MemStore) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	return []*pb.InterfaceDeclaration{
		{
			Name:      "KeyValueBasicInterface",
			ProtoFile: "prism/interfaces/keyvalue/keyvalue_basic.proto",
			Version:   "v1",
		},
	}
}
