package memstore

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

func TestMemStore_SetGet(t *testing.T) {
	m := New()

	// Initialize with test config
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 9090,
		},
		Backend: map[string]any{
			"max_keys":       100,
			"cleanup_period": "1m",
		},
	}

	ctx := context.Background()
	if err := m.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Test Set and Get
	key := "test-key"
	value := []byte("test-value")

	if err := m.Set(key, value, 0); err != nil {
		t.Fatalf("Failed to set key: %v", err)
	}

	gotValue, found, err := m.Get(key)
	if err != nil {
		t.Fatalf("Failed to get key: %v", err)
	}
	if !found {
		t.Fatal("Key not found")
	}
	if string(gotValue) != string(value) {
		t.Errorf("Expected value %s, got %s", value, gotValue)
	}
}

func TestMemStore_Delete(t *testing.T) {
	m := New()

	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 9090,
		},
		Backend: map[string]any{
			"max_keys": 100,
		},
	}

	ctx := context.Background()
	if err := m.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Set a key
	key := "delete-key"
	value := []byte("delete-value")
	if err := m.Set(key, value, 0); err != nil {
		t.Fatalf("Failed to set key: %v", err)
	}

	// Verify it exists
	_, found, _ := m.Get(key)
	if !found {
		t.Fatal("Key should exist before delete")
	}

	// Delete the key
	if err := m.Delete(key); err != nil {
		t.Fatalf("Failed to delete key: %v", err)
	}

	// Verify it's gone
	_, found, _ = m.Get(key)
	if found {
		t.Fatal("Key should not exist after delete")
	}
}

func TestMemStore_TTL(t *testing.T) {
	m := New()

	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 9090,
		},
		Backend: map[string]any{
			"max_keys": 100,
		},
	}

	ctx := context.Background()
	if err := m.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Set a key with 1 second TTL
	key := "ttl-key"
	value := []byte("ttl-value")
	if err := m.Set(key, value, 1); err != nil {
		t.Fatalf("Failed to set key: %v", err)
	}

	// Verify it exists
	_, found, _ := m.Get(key)
	if !found {
		t.Fatal("Key should exist immediately after set")
	}

	// Wait for TTL to expire
	time.Sleep(1100 * time.Millisecond)

	// Verify it's expired
	_, found, _ = m.Get(key)
	if found {
		t.Fatal("Key should be expired after TTL")
	}
}

func TestMemStore_CapacityLimit(t *testing.T) {
	m := New()

	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 9090,
		},
		Backend: map[string]any{
			"max_keys": 5, // Small capacity for testing
		},
	}

	ctx := context.Background()
	if err := m.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Fill to capacity
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		if err := m.Set(key, []byte("value"), 0); err != nil {
			t.Fatalf("Failed to set key %s: %v", key, err)
		}
	}

	// Try to exceed capacity
	err := m.Set("overflow", []byte("value"), 0)
	if err == nil {
		t.Fatal("Expected error when exceeding capacity")
	}

	// Verify we can update existing keys
	if err := m.Set("a", []byte("new-value"), 0); err != nil {
		t.Fatalf("Should be able to update existing key: %v", err)
	}
}

func TestMemStore_Health(t *testing.T) {
	m := New()

	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 9090,
		},
		Backend: map[string]any{
			"max_keys": 10,
		},
	}

	ctx := context.Background()
	if err := m.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Check health when empty
	health, err := m.Health(ctx)
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	if health.Status != core.HealthHealthy {
		t.Errorf("Expected healthy status, got %v", health.Status)
	}

	// Add keys to near capacity
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		m.Set(key, []byte("value"), 0)
	}

	// Check health at capacity
	health, err = m.Health(ctx)
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	if health.Status != core.HealthDegraded {
		t.Errorf("Expected degraded status at capacity, got %v", health.Status)
	}
}
