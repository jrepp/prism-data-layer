package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// setupTestRedis creates a test Redis pattern with miniredis
func setupTestRedis(t *testing.T) (*RedisPattern, *miniredis.Miniredis) {
	t.Helper()

	// Create miniredis server
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	// Create config pointing to miniredis
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "redis",
			Version: "0.1.0",
		},
		ControlPlane: plugin.ControlPlaneConfig{
			Port: 9091,
		},
		Backend: map[string]any{
			"address": mr.Addr(),
		},
	}

	// Initialize plugin
	p := New()
	ctx := context.Background()
	if err := p.Initialize(ctx, config); err != nil {
		mr.Close()
		t.Fatalf("failed to initialize plugin: %v", err)
	}

	return p, mr
}

func TestRedisPattern_New(t *testing.T) {
	p := New()

	if p.Name() != "redis" {
		t.Errorf("expected name 'redis', got '%s'", p.Name())
	}
	if p.Version() != "0.1.0" {
		t.Errorf("expected version '0.1.0', got '%s'", p.Version())
	}
}

func TestRedisPattern_Initialize(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	tests := []struct {
		name    string
		config  *plugin.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &plugin.Config{
				Plugin:       plugin.PluginConfig{Name: "redis", Version: "0.1.0"},
				ControlPlane: plugin.ControlPlaneConfig{Port: 9091},
				Backend: map[string]any{
					"address": mr.Addr(),
				},
			},
			wantErr: false,
		},
		// Removed "defaults applied" test case - flaky because it depends on whether
		// Redis is actually running on localhost:6379. Use invalid address test instead.
		{
			name: "invalid address",
			config: &plugin.Config{
				Plugin:       plugin.PluginConfig{Name: "redis", Version: "0.1.0"},
				ControlPlane: plugin.ControlPlaneConfig{Port: 9091},
				Backend: map[string]any{
					"address": "localhost:9999", // Invalid port that's unlikely to have Redis
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			ctx := context.Background()
			err := p.Initialize(ctx, tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil && p.client == nil {
				t.Error("Initialize() succeeded but client is nil")
			}
		})
	}
}

func TestRedisPattern_SetGet(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()
	defer p.Stop(context.Background())

	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{"simple", "key1", []byte("value1")},
		{"empty value", "key2", []byte("")},
		{"binary data", "key3", []byte{0x00, 0x01, 0x02, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set value
			if err := p.Set(tt.key, tt.value, 0); err != nil {
				t.Fatalf("Set() error = %v", err)
			}

			// Get value
			value, found, err := p.Get(tt.key)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if !found {
				t.Fatal("Get() key not found")
			}
			if string(value) != string(tt.value) {
				t.Errorf("Get() = %v, want %v", value, tt.value)
			}
		})
	}
}

func TestRedisPattern_SetWithTTL(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()
	defer p.Stop(context.Background())

	key := "ttl-key"
	value := []byte("ttl-value")
	ttl := int64(1) // 1 second

	// Set value with TTL
	if err := p.Set(key, value, ttl); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Key should exist immediately
	val, found, err := p.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found {
		t.Fatal("Get() key not found immediately after Set")
	}
	if string(val) != string(value) {
		t.Errorf("Get() = %v, want %v", val, value)
	}

	// Fast-forward miniredis time
	mr.FastForward(2 * time.Second)

	// Key should be expired
	_, found, err = p.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() key should be expired but was found")
	}
}

func TestRedisPattern_GetNonExistent(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()
	defer p.Stop(context.Background())

	_, found, err := p.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() found nonexistent key")
	}
}

func TestRedisPattern_Delete(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()
	defer p.Stop(context.Background())

	key := "delete-me"
	value := []byte("data")

	// Set value
	if err := p.Set(key, value, 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify it exists
	_, found, err := p.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found {
		t.Fatal("Get() key not found after Set")
	}

	// Delete key
	if err := p.Delete(key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, found, err = p.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() found key after Delete")
	}
}

func TestRedisPattern_Exists(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()
	defer p.Stop(context.Background())

	key := "exists-key"
	value := []byte("data")

	// Check nonexistent key
	exists, err := p.Exists(key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() returned true for nonexistent key")
	}

	// Set value
	if err := p.Set(key, value, 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Check existing key
	exists, err = p.Exists(key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing key")
	}

	// Delete key
	if err := p.Delete(key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Check deleted key
	exists, err = p.Exists(key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() returned true for deleted key")
	}
}

func TestRedisPattern_Health(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()
	defer p.Stop(context.Background())

	ctx := context.Background()
	health, err := p.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if health.Status != plugin.HealthHealthy {
		t.Errorf("Health() status = %v, want %v", health.Status, plugin.HealthHealthy)
	}

	if health.Message == "" {
		t.Error("Health() message is empty")
	}

	if len(health.Details) == 0 {
		t.Error("Health() details is empty")
	}
}

func TestRedisPattern_HealthUnhealthy(t *testing.T) {
	p, mr := setupTestRedis(t)
	// Close Redis to simulate connection failure
	mr.Close()

	ctx := context.Background()
	health, err := p.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if health.Status != plugin.HealthUnhealthy {
		t.Errorf("Health() status = %v, want %v", health.Status, plugin.HealthUnhealthy)
	}
}

func TestRedisPattern_Stop(t *testing.T) {
	p, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify client is closed by attempting an operation
	err := p.Set("key", []byte("value"), 0)
	if err == nil {
		t.Error("Set() succeeded after Stop(), expected error")
	}
}
