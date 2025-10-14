package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/redis/go-redis/v9"
)

// RedisPattern implements a Redis-backed key-value store plugin
type RedisPattern struct {
	name    string
	version string
	client  *redis.Client
	config  *Config
}

// Config holds Redis-specific configuration
type Config struct {
	Address         string        `yaml:"address"`
	Password        string        `yaml:"password"`
	DB              int           `yaml:"db"`
	MaxRetries      int           `yaml:"max_retries"`
	PoolSize        int           `yaml:"pool_size"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`
	DialTimeout     time.Duration `yaml:"dial_timeout"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
}

// New creates a new Redis plugin
func New() *RedisPattern {
	return &RedisPattern{
		name:    "redis",
		version: "0.1.0",
	}
}

// Name returns the plugin name
func (r *RedisPattern) Name() string {
	return r.name
}

// Version returns the plugin version
func (r *RedisPattern) Version() string {
	return r.version
}

// Initialize prepares the plugin with configuration
func (r *RedisPattern) Initialize(ctx context.Context, config *plugin.Config) error {
	// Extract backend-specific config
	var backendConfig Config
	if err := config.GetBackendConfig(&backendConfig); err != nil {
		return fmt.Errorf("failed to parse backend config: %w", err)
	}

	// Apply defaults
	if backendConfig.Address == "" {
		backendConfig.Address = "localhost:6379"
	}

	// Strip redis:// or rediss:// scheme prefix if present (for testcontainers compatibility)
	if len(backendConfig.Address) > 8 && backendConfig.Address[:8] == "redis://" {
		backendConfig.Address = backendConfig.Address[8:]
	} else if len(backendConfig.Address) > 9 && backendConfig.Address[:9] == "rediss://" {
		backendConfig.Address = backendConfig.Address[9:]
	}

	if backendConfig.MaxRetries == 0 {
		backendConfig.MaxRetries = 3
	}
	if backendConfig.PoolSize == 0 {
		backendConfig.PoolSize = 10
	}
	if backendConfig.ConnMaxIdleTime == 0 {
		backendConfig.ConnMaxIdleTime = 5 * time.Minute
	}
	if backendConfig.DialTimeout == 0 {
		backendConfig.DialTimeout = 5 * time.Second
	}
	if backendConfig.ReadTimeout == 0 {
		backendConfig.ReadTimeout = 3 * time.Second
	}
	if backendConfig.WriteTimeout == 0 {
		backendConfig.WriteTimeout = 3 * time.Second
	}

	r.config = &backendConfig

	// Create Redis client with connection pool
	r.client = redis.NewClient(&redis.Options{
		Addr:            backendConfig.Address,
		Password:        backendConfig.Password,
		DB:              backendConfig.DB,
		MaxRetries:      backendConfig.MaxRetries,
		PoolSize:        backendConfig.PoolSize,
		ConnMaxIdleTime: backendConfig.ConnMaxIdleTime,
		DialTimeout:     backendConfig.DialTimeout,
		ReadTimeout:     backendConfig.ReadTimeout,
		WriteTimeout:    backendConfig.WriteTimeout,
	})

	// Test connection
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return nil
}

// Start begins serving requests
func (r *RedisPattern) Start(ctx context.Context) error {
	// Redis client is ready from Initialize, nothing to start
	return nil
}

// Stop gracefully shuts down the plugin
func (r *RedisPattern) Stop(ctx context.Context) error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Health returns the plugin health status
func (r *RedisPattern) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	// Check Redis connection with PING
	if err := r.client.Ping(ctx).Err(); err != nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthUnhealthy,
			Message: fmt.Sprintf("Redis ping failed: %v", err),
			Details: map[string]string{
				"error": err.Error(),
			},
		}, nil
	}

	// Get pool stats
	stats := r.client.PoolStats()

	status := plugin.HealthHealthy
	message := fmt.Sprintf("healthy, %d connections", stats.TotalConns)

	// Check if pool is near capacity
	if stats.TotalConns >= uint32(r.config.PoolSize*90/100) {
		status = plugin.HealthDegraded
		message = fmt.Sprintf("pool near capacity: %d/%d connections", stats.TotalConns, r.config.PoolSize)
	}

	return &plugin.HealthStatus{
		Status:  status,
		Message: message,
		Details: map[string]string{
			"total_conns": fmt.Sprintf("%d", stats.TotalConns),
			"idle_conns":  fmt.Sprintf("%d", stats.IdleConns),
			"pool_size":   fmt.Sprintf("%d", r.config.PoolSize),
		},
	}, nil
}

// Set stores a value with optional TTL
func (r *RedisPattern) Set(key string, value []byte, ttlSeconds int64) error {
	ctx := context.Background()

	if ttlSeconds > 0 {
		duration := time.Duration(ttlSeconds) * time.Second
		return r.client.Set(ctx, key, value, duration).Err()
	}

	return r.client.Set(ctx, key, value, 0).Err()
}

// Get retrieves a value by key
func (r *RedisPattern) Get(key string) ([]byte, bool, error) {
	ctx := context.Background()

	value, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil // Key not found
	}
	if err != nil {
		return nil, false, err
	}

	return value, true, nil
}

// Delete removes a key
func (r *RedisPattern) Delete(key string) error {
	ctx := context.Background()
	return r.client.Del(ctx, key).Err()
}

// Exists checks if a key exists
func (r *RedisPattern) Exists(key string) (bool, error) {
	ctx := context.Background()

	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Compile-time interface compliance checks
// These ensure that RedisPattern implements the expected interfaces
var (
	_ plugin.Plugin                 = (*RedisPattern)(nil) // Core plugin interface
	_ plugin.KeyValueBasicInterface = (*RedisPattern)(nil) // KeyValue basic operations
	_ plugin.InterfaceSupport       = (*RedisPattern)(nil) // Interface introspection
)

// SupportsInterface returns true if RedisPattern implements the named interface
func (r *RedisPattern) SupportsInterface(interfaceName string) bool {
	supported := map[string]bool{
		"Plugin":                 true,
		"KeyValueBasicInterface": true,
		"InterfaceSupport":       true,
	}
	return supported[interfaceName]
}

// ListInterfaces returns all interfaces that RedisPattern implements
func (r *RedisPattern) ListInterfaces() []string {
	return []string{
		"Plugin",
		"KeyValueBasicInterface",
		"InterfaceSupport",
	}
}
