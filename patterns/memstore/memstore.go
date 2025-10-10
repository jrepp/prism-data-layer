package memstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
)

// MemStore implements an in-memory key-value store plugin
type MemStore struct {
	name    string
	version string
	data    sync.Map
	ttl     sync.Map // key -> expiration time
	config  *Config
	stopCh  chan struct{}
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
func (m *MemStore) Initialize(ctx context.Context, config *core.Config) error {
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

	m.config = &backendConfig

	return nil
}

// Start begins serving requests
func (m *MemStore) Start(ctx context.Context) error {
	// Start TTL cleanup goroutine
	go m.cleanupExpiredKeys(ctx)

	// Block until context is cancelled
	<-ctx.Done()
	close(m.stopCh)
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
func (m *MemStore) Health(ctx context.Context) (*core.HealthStatus, error) {
	keyCount := 0
	m.data.Range(func(key, value interface{}) bool {
		keyCount++
		return true
	})

	status := core.HealthHealthy
	message := fmt.Sprintf("healthy, %d keys stored", keyCount)

	if m.config != nil && keyCount >= m.config.MaxKeys {
		status = core.HealthDegraded
		message = fmt.Sprintf("at capacity: %d/%d keys", keyCount, m.config.MaxKeys)
	}

	return &core.HealthStatus{
		Status:  status,
		Message: message,
		Details: map[string]string{
			"keys":     fmt.Sprintf("%d", keyCount),
			"max_keys": fmt.Sprintf("%d", m.config.MaxKeys),
		},
	}, nil
}

// Set stores a value with optional TTL
func (m *MemStore) Set(key string, value []byte, ttlSeconds int64) error {
	// Check capacity
	if m.config != nil {
		keyCount := 0
		m.data.Range(func(k, v interface{}) bool {
			keyCount++
			return true
		})

		if keyCount >= m.config.MaxKeys {
			_, exists := m.data.Load(key)
			if !exists {
				return fmt.Errorf("capacity limit reached: %d keys", m.config.MaxKeys)
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

// cleanupExpiredKeys periodically removes expired keys
func (m *MemStore) cleanupExpiredKeys(ctx context.Context) {
	ticker := time.NewTicker(m.config.CleanupPeriod)
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
