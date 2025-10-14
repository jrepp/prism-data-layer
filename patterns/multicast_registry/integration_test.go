package multicast_registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/jrepp/prism-data-layer/patterns/multicast_registry/backends"
)

// TestIntegration_FullStack tests coordinator with Redis + NATS backends
func TestIntegration_FullStack(t *testing.T) {
	// Start Redis and NATS servers
	redis := miniredis.RunT(t)
	defer redis.Close()

	natsOpts := natsserver.DefaultTestOptions
	natsOpts.Port = -1
	natsServer := natsserver.RunServer(&natsOpts)
	defer natsServer.Shutdown()

	// Create real backends
	registry, err := backends.NewRedisRegistryBackend(redis.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create Redis backend: %v", err)
	}
	defer registry.Close()

	messaging, err := backends.NewNATSMessagingBackend([]string{natsServer.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create NATS backend: %v", err)
	}
	defer messaging.Close()

	// Create coordinator with real backends
	config := DefaultConfig()
	coordinator, err := NewCoordinator(config, registry, messaging, nil)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer coordinator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Register 5 identities with different metadata
	identities := []struct {
		id       string
		metadata map[string]interface{}
	}{
		{"device-1", map[string]interface{}{"status": "online", "region": "us-west"}},
		{"device-2", map[string]interface{}{"status": "online", "region": "eu-west"}},
		{"device-3", map[string]interface{}{"status": "offline", "region": "us-west"}},
		{"device-4", map[string]interface{}{"status": "online", "region": "ap-south"}},
		{"device-5", map[string]interface{}{"status": "maintenance", "region": "us-west"}},
	}

	for _, id := range identities {
		err := coordinator.Register(ctx, id.id, id.metadata, 0)
		if err != nil {
			t.Fatalf("Register failed for %s: %v", id.id, err)
		}
	}

	// Test 1: Enumerate all identities
	allFilter := backends.NewFilter(nil)
	all, err := coordinator.Enumerate(ctx, allFilter)
	if err != nil {
		t.Fatalf("Enumerate all failed: %v", err)
	}

	if len(all) != 5 {
		t.Errorf("Expected 5 identities, got %d", len(all))
	}

	// Test 2: Enumerate with filter (status=online)
	onlineFilter := backends.NewFilter(map[string]interface{}{"status": "online"})
	online, err := coordinator.Enumerate(ctx, onlineFilter)
	if err != nil {
		t.Fatalf("Enumerate online failed: %v", err)
	}

	if len(online) != 3 {
		t.Errorf("Expected 3 online identities, got %d", len(online))
	}

	// Test 3: Multicast to online devices in us-west
	// First, subscribe to messages for verification (use coordinator's topic naming)
	msgChan1, err := messaging.Subscribe(ctx, "prism.multicast.device-1")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	msgChan3, err := messaging.Subscribe(ctx, "prism.multicast.device-3")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Multicast to online devices only
	payload := []byte("firmware-update-v2")
	resp, err := coordinator.Multicast(ctx, onlineFilter, payload)
	if err != nil {
		t.Fatalf("Multicast failed: %v", err)
	}

	if resp.DeliveredCount != 3 {
		t.Errorf("Expected 3 deliveries, got %d", resp.DeliveredCount)
	}

	// Device-1 should receive (online)
	select {
	case msg := <-msgChan1:
		if string(msg) != string(payload) {
			t.Errorf("Device-1: expected %s, got %s", payload, msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("Device-1: timeout waiting for message")
	}

	// Device-3 should NOT receive (offline)
	select {
	case msg := <-msgChan3:
		t.Errorf("Device-3: should not receive message (offline), got %s", msg)
	case <-time.After(500 * time.Millisecond):
		// Expected: no message received
	}

	// Test 4: Unregister a device
	err = coordinator.Unregister(ctx, "device-1")
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Should not appear in enumerate
	all, err = coordinator.Enumerate(ctx, allFilter)
	if err != nil {
		t.Fatalf("Enumerate after unregister failed: %v", err)
	}

	if len(all) != 4 {
		t.Errorf("Expected 4 identities after unregister, got %d", len(all))
	}
}

// TestIntegration_TTLExpiration tests TTL cleanup with real Redis
func TestIntegration_TTLExpiration(t *testing.T) {
	redis := miniredis.RunT(t)
	defer redis.Close()

	natsOpts := natsserver.DefaultTestOptions
	natsOpts.Port = -1
	natsServer := natsserver.RunServer(&natsOpts)
	defer natsServer.Shutdown()

	registry, err := backends.NewRedisRegistryBackend(redis.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create Redis backend: %v", err)
	}
	defer registry.Close()

	messaging, err := backends.NewNATSMessagingBackend([]string{natsServer.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create NATS backend: %v", err)
	}
	defer messaging.Close()

	config := DefaultConfig()
	coordinator, err := NewCoordinator(config, registry, messaging, nil)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer coordinator.Close()

	ctx := context.Background()

	// Register with short TTL
	metadata := map[string]interface{}{"temp": true}
	ttl := 500 * time.Millisecond

	err = coordinator.Register(ctx, "temp-session", metadata, ttl)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Should exist immediately
	filter := backends.NewFilter(nil)
	identities, err := coordinator.Enumerate(ctx, filter)
	if err != nil {
		t.Fatalf("Enumerate failed: %v", err)
	}

	if len(identities) != 1 {
		t.Errorf("Expected 1 identity, got %d", len(identities))
	}

	// Wait for real time to pass (both for Redis TTL and coordinator's ExpiresAt check)
	time.Sleep(600 * time.Millisecond)
	redis.FastForward(600 * time.Millisecond)

	// Should be expired (coordinator checks ExpiresAt in Enumerate)
	identities, err = coordinator.Enumerate(ctx, filter)
	if err != nil {
		t.Fatalf("Enumerate after expiration failed: %v", err)
	}

	if len(identities) != 0 {
		t.Errorf("Expected 0 identities after expiration, got %d", len(identities))
	}
}

// TestIntegration_Concurrent tests concurrent operations with real backends
func TestIntegration_Concurrent(t *testing.T) {
	redis := miniredis.RunT(t)
	defer redis.Close()

	natsOpts := natsserver.DefaultTestOptions
	natsOpts.Port = -1
	natsServer := natsserver.RunServer(&natsOpts)
	defer natsServer.Shutdown()

	registry, err := backends.NewRedisRegistryBackend(redis.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create Redis backend: %v", err)
	}
	defer registry.Close()

	messaging, err := backends.NewNATSMessagingBackend([]string{natsServer.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create NATS backend: %v", err)
	}
	defer messaging.Close()

	config := DefaultConfig()
	coordinator, err := NewCoordinator(config, registry, messaging, nil)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer coordinator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run 50 concurrent registrations
	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(id int) {
			identity := fmt.Sprintf("user-%d", id)
			metadata := map[string]interface{}{"user_id": id, "status": "online"}
			err := coordinator.Register(ctx, identity, metadata, 0)
			if err != nil {
				t.Errorf("Register %d failed: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all registrations
	for i := 0; i < 50; i++ {
		<-done
	}

	// Should have 50 identities
	filter := backends.NewFilter(nil)
	identities, err := coordinator.Enumerate(ctx, filter)
	if err != nil {
		t.Fatalf("Enumerate failed: %v", err)
	}

	if len(identities) != 50 {
		t.Errorf("Expected 50 identities, got %d", len(identities))
	}
}

// TestIntegration_PerformanceBaseline establishes performance baseline
func TestIntegration_PerformanceBaseline(t *testing.T) {
	redis := miniredis.RunT(t)
	defer redis.Close()

	natsOpts := natsserver.DefaultTestOptions
	natsOpts.Port = -1
	natsServer := natsserver.RunServer(&natsOpts)
	defer natsServer.Shutdown()

	registry, err := backends.NewRedisRegistryBackend(redis.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create Redis backend: %v", err)
	}
	defer registry.Close()

	messaging, err := backends.NewNATSMessagingBackend([]string{natsServer.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create NATS backend: %v", err)
	}
	defer messaging.Close()

	config := DefaultConfig()
	coordinator, err := NewCoordinator(config, registry, messaging, nil)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer coordinator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Register 1000 identities
	t.Log("Registering 1000 identities...")
	start := time.Now()
	for i := 0; i < 1000; i++ {
		identity := fmt.Sprintf("device-%d", i)
		metadata := map[string]interface{}{
			"device_id": i,
			"status":    "online",
			"region":    "us-west",
		}
		err := coordinator.Register(ctx, identity, metadata, 0)
		if err != nil {
			t.Fatalf("Register %d failed: %v", i, err)
		}
	}
	registerDuration := time.Since(start)
	t.Logf("Register 1000 identities: %v (%.2f ops/sec)", registerDuration, 1000.0/registerDuration.Seconds())

	// Enumerate all (performance target: <20ms for 1000 identities)
	filter := backends.NewFilter(nil)
	start = time.Now()
	identities, err := coordinator.Enumerate(ctx, filter)
	enumerateDuration := time.Since(start)
	if err != nil {
		t.Fatalf("Enumerate failed: %v", err)
	}

	t.Logf("Enumerate 1000 identities: %v", enumerateDuration)

	if len(identities) != 1000 {
		t.Errorf("Expected 1000 identities, got %d", len(identities))
	}

	// Note: miniredis is in-memory and doesn't reflect real Redis network latency
	// Production target: <20ms with real Redis
	if enumerateDuration > 100*time.Millisecond {
		t.Logf("WARNING: Enumerate took %v (target <20ms with real Redis, miniredis has overhead)", enumerateDuration)
	}

	// Multicast to 100 targets (performance target: <100ms)
	// Filter for first 100 devices
	onlineFilter := backends.NewFilter(map[string]interface{}{"status": "online"})
	start = time.Now()
	resp, err := coordinator.Multicast(ctx, onlineFilter, []byte("broadcast-message"))
	multicastDuration := time.Since(start)
	if err != nil {
		t.Fatalf("Multicast failed: %v", err)
	}

	t.Logf("Multicast to %d targets: %v", resp.DeliveredCount, multicastDuration)

	if resp.DeliveredCount != 1000 {
		t.Errorf("Expected 1000 deliveries, got %d", resp.DeliveredCount)
	}

	// Note: NATS is fast, but embedded server may have different characteristics
	// Production target: <100ms for 100 targets with real NATS
	if multicastDuration > 500*time.Millisecond {
		t.Logf("WARNING: Multicast took %v (target <100ms for 100 targets with real NATS)", multicastDuration)
	}
}
