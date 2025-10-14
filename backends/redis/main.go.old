package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/redis/go-redis/v9"
)

const (
	version = "0.1.0"
)

// RedisPlugin implements the Prism backend plugin for Redis
// Based on RFC-011 (ACL + password auth) and ADR-010 (caching layer)
type RedisPlugin struct {
	client *redis.Client
	config *RedisConfig
}

// RedisConfig holds Redis-specific configuration
// Follows RFC-011 ACL authentication pattern and ADR-025 environment config
type RedisConfig struct {
	Address  string
	Database int

	// ACL authentication (RFC-011)
	Username string
	Password string

	// Vault-managed credentials
	VaultEnabled bool
	VaultPath    string

	// Connection settings
	PoolSize        int
	MaxRetries      int
	MinIdleConns    int
	ConnMaxIdleTime time.Duration

	// Key prefix for namespacing
	KeyPrefix string
}

func (p *RedisPlugin) Name() string {
	return "redis"
}

func (p *RedisPlugin) Version() string {
	return version
}

// Initialize creates Redis client
// Based on RFC-011 ACL authentication flow
func (p *RedisPlugin) Initialize(ctx context.Context, config *core.Config) error {
	slog.Info("initializing redis plugin", "version", version)

	// Extract backend-specific config
	var redisConfig RedisConfig
	if err := config.GetBackendConfig(&redisConfig); err != nil {
		return fmt.Errorf("failed to parse redis config: %w", err)
	}
	p.config = &redisConfig

	// Get address from environment or config
	address := os.Getenv("REDIS_ADDRESS")
	if address == "" {
		address = redisConfig.Address
	}
	if address == "" {
		return fmt.Errorf("REDIS_ADDRESS not configured")
	}

	// Fetch Vault credentials if enabled (RFC-011 pattern)
	if redisConfig.VaultEnabled {
		slog.Info("vault-managed credentials enabled", "path", redisConfig.VaultPath)
		// TODO: Implement Vault credential fetching
		// creds, err := p.fetchVaultCredentials(ctx, redisConfig.VaultPath)
		// if err != nil {
		//     return fmt.Errorf("failed to fetch vault credentials: %w", err)
		// }
		// redisConfig.Username = creds.Username
		// redisConfig.Password = creds.Password
	} else {
		// Use environment variables for ACL credentials
		if user := os.Getenv("REDIS_USERNAME"); user != "" {
			redisConfig.Username = user
		}
		if pass := os.Getenv("REDIS_PASSWORD"); pass != "" {
			redisConfig.Password = pass
		}
	}

	// Create Redis client (RFC-011: ACL authentication)
	options := &redis.Options{
		Addr:     address,
		DB:       redisConfig.Database,
		Username: redisConfig.Username,
		Password: redisConfig.Password,

		// Connection pool settings
		PoolSize:        redisConfig.PoolSize,
		MaxRetries:      redisConfig.MaxRetries,
		MinIdleConns:    redisConfig.MinIdleConns,
		ConnMaxIdleTime: redisConfig.ConnMaxIdleTime,

		// Health check
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	client := redis.NewClient(options)

	// Verify connectivity
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to ping redis: %w", err)
	}

	p.client = client

	slog.Info("redis plugin initialized",
		"address", address,
		"database", redisConfig.Database,
		"pool_size", redisConfig.PoolSize,
		"acl_enabled", redisConfig.Username != "",
		"key_prefix", redisConfig.KeyPrefix)

	return nil
}

// Start begins serving requests
func (p *RedisPlugin) Start(ctx context.Context) error {
	slog.Info("redis plugin started")

	// Keep running until context is cancelled
	<-ctx.Done()

	slog.Info("redis plugin stopping")
	return nil
}

// Stop gracefully shuts down the plugin
func (p *RedisPlugin) Stop(ctx context.Context) error {
	slog.Info("stopping redis plugin")

	if p.client != nil {
		if err := p.client.Close(); err != nil {
			slog.Error("error closing redis client", "error", err)
			return err
		}
		slog.Info("closed redis client")
	}

	return nil
}

// Health reports the plugin health status
// ADR-025: Required health check interface
func (p *RedisPlugin) Health(ctx context.Context) (*core.HealthStatus, error) {
	if p.client == nil {
		return &core.HealthStatus{
			Status:  core.HealthUnhealthy,
			Message: "redis client not initialized",
		}, nil
	}

	// Check Redis connectivity with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	start := time.Now()
	if err := p.client.Ping(pingCtx).Err(); err != nil {
		return &core.HealthStatus{
			Status:  core.HealthUnhealthy,
			Message: fmt.Sprintf("redis ping failed: %v", err),
			Details: map[string]string{
				"error": err.Error(),
			},
		}, nil
	}
	latency := time.Since(start)

	// Check pool stats
	stats := p.client.PoolStats()
	poolUsage := float64(stats.TotalConns) / float64(p.config.PoolSize)

	// Degraded if latency is high or pool is near capacity
	if latency > 100*time.Millisecond || poolUsage > 0.9 {
		return &core.HealthStatus{
			Status:  core.HealthDegraded,
			Message: "redis degraded",
			Details: map[string]string{
				"latency_ms":  fmt.Sprintf("%.2f", latency.Seconds()*1000),
				"total_conns": fmt.Sprintf("%d", stats.TotalConns),
				"idle_conns":  fmt.Sprintf("%d", stats.IdleConns),
				"pool_size":   fmt.Sprintf("%d", p.config.PoolSize),
				"pool_usage":  fmt.Sprintf("%.1f%%", poolUsage*100),
			},
		}, nil
	}

	return &core.HealthStatus{
		Status:  core.HealthHealthy,
		Message: "redis healthy",
		Details: map[string]string{
			"latency_ms":  fmt.Sprintf("%.2f", latency.Seconds()*1000),
			"total_conns": fmt.Sprintf("%d", stats.TotalConns),
			"idle_conns":  fmt.Sprintf("%d", stats.IdleConns),
		},
	}, nil
}

// Cache Operations (ADR-010: Look-aside cache pattern)
// These would be exposed via gRPC service in production

// Get retrieves a value from cache
func (p *RedisPlugin) Get(ctx context.Context, key string) ([]byte, error) {
	fullKey := p.config.KeyPrefix + key

	value, err := p.client.Get(ctx, fullKey).Bytes()
	if err == redis.Nil {
		slog.Debug("cache miss", "key", key)
		return nil, nil // Cache miss is not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get from cache: %w", err)
	}

	slog.Debug("cache hit", "key", key, "size", len(value))
	return value, nil
}

// Set stores a value in cache with TTL
func (p *RedisPlugin) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	fullKey := p.config.KeyPrefix + key

	if err := p.client.Set(ctx, fullKey, value, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	slog.Debug("cache set", "key", key, "size", len(value), "ttl", ttl)
	return nil
}

// Delete removes a value from cache
func (p *RedisPlugin) Delete(ctx context.Context, keys ...string) error {
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = p.config.KeyPrefix + key
	}

	deleted, err := p.client.Del(ctx, fullKeys...).Result()
	if err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}

	slog.Debug("cache delete", "requested", len(keys), "deleted", deleted)
	return nil
}

// MGet retrieves multiple values from cache
func (p *RedisPlugin) MGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = p.config.KeyPrefix + key
	}

	values, err := p.client.MGet(ctx, fullKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to mget from cache: %w", err)
	}

	result := make(map[string][]byte)
	for i, val := range values {
		if val != nil {
			if strVal, ok := val.(string); ok {
				result[keys[i]] = []byte(strVal)
			}
		}
	}

	slog.Debug("cache mget", "requested", len(keys), "hits", len(result))
	return result, nil
}

// Expire sets or updates TTL on a key
func (p *RedisPlugin) Expire(ctx context.Context, key string, ttl time.Duration) error {
	fullKey := p.config.KeyPrefix + key

	if err := p.client.Expire(ctx, fullKey, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set expiry: %w", err)
	}

	slog.Debug("cache expire", "key", key, "ttl", ttl)
	return nil
}

func main() {
	// Get config path from environment
	configPath := os.Getenv("PRISM_PLUGIN_CONFIG")
	if configPath == "" {
		configPath = "/etc/prism/plugin.yaml"
	}

	// Bootstrap the plugin using core package
	plugin := &RedisPlugin{}
	if err := core.Bootstrap(plugin, configPath); err != nil {
		slog.Error("plugin failed", "error", err)
		os.Exit(1)
	}
}
