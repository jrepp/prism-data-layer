package multicast_registry

import (
	"context"
	"testing"
	"time"
)

// TestCoordinator_Register tests identity registration
func TestCoordinator_Register(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	// Test basic registration
	identity := "user-alice-session-1"
	metadata := map[string]interface{}{
		"user_id":      "alice",
		"display_name": "Alice",
		"status":       "online",
		"room":         "engineering",
	}

	err := coordinator.Register(ctx, identity, metadata, 5*time.Minute)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify identity was stored in registry backend
	stored, err := coordinator.registry.Get(ctx, identity)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if stored.ID != identity {
		t.Errorf("Expected identity %s, got %s", identity, stored.ID)
	}

	if stored.Metadata["user_id"] != "alice" {
		t.Errorf("Expected user_id=alice, got %v", stored.Metadata["user_id"])
	}

	if stored.TTL != 5*time.Minute {
		t.Errorf("Expected TTL=5m, got %v", stored.TTL)
	}
}

// TestCoordinator_Register_Duplicate tests duplicate identity registration
func TestCoordinator_Register_Duplicate(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	identity := "user-bob-session-1"
	metadata := map[string]interface{}{"user_id": "bob"}

	// First registration should succeed
	err := coordinator.Register(ctx, identity, metadata, 0)
	if err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	// Second registration should fail (unless replace=true)
	err = coordinator.Register(ctx, identity, metadata, 0)
	if err == nil {
		t.Fatal("Expected error on duplicate registration, got nil")
	}
}

// TestCoordinator_Register_WithoutTTL tests registration without TTL
func TestCoordinator_Register_WithoutTTL(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	identity := "service-payment-1"
	metadata := map[string]interface{}{"service": "payment"}

	err := coordinator.Register(ctx, identity, metadata, 0) // TTL = 0 means no expiration
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stored, err := coordinator.registry.Get(ctx, identity)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if stored.ExpiresAt != nil {
		t.Errorf("Expected no expiration, got %v", stored.ExpiresAt)
	}
}

// TestCoordinator_Enumerate_NoFilter tests enumerate without filter (returns all)
func TestCoordinator_Enumerate_NoFilter(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	// Register 3 identities
	identities := []string{"user-1", "user-2", "user-3"}
	for _, id := range identities {
		err := coordinator.Register(ctx, id, map[string]interface{}{"user": id}, 0)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
	}

	// Enumerate without filter (should return all 3)
	results, err := coordinator.Enumerate(ctx, nil)
	if err != nil {
		t.Fatalf("Enumerate failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 identities, got %d", len(results))
	}
}

// TestCoordinator_Enumerate_WithFilter tests enumerate with simple equality filter
func TestCoordinator_Enumerate_WithFilter(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	// Register identities with different metadata
	coordinator.Register(ctx, "user-alice", map[string]interface{}{"status": "online", "room": "engineering"}, 0)
	coordinator.Register(ctx, "user-bob", map[string]interface{}{"status": "away", "room": "engineering"}, 0)
	coordinator.Register(ctx, "user-carol", map[string]interface{}{"status": "online", "room": "sales"}, 0)

	// Filter: status=online AND room=engineering (should match alice only)
	filter := NewFilter(map[string]interface{}{
		"status": "online",
		"room":   "engineering",
	})

	results, err := coordinator.Enumerate(ctx, filter)
	if err != nil {
		t.Fatalf("Enumerate failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 identity, got %d", len(results))
	}

	if len(results) > 0 && results[0].ID != "user-alice" {
		t.Errorf("Expected user-alice, got %s", results[0].ID)
	}
}

// TestCoordinator_Unregister tests identity removal
func TestCoordinator_Unregister(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	identity := "user-dave"
	coordinator.Register(ctx, identity, map[string]interface{}{"user": "dave"}, 0)

	// Verify exists
	_, err := coordinator.registry.Get(ctx, identity)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Unregister
	err = coordinator.Unregister(ctx, identity)
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Verify removed
	_, err = coordinator.registry.Get(ctx, identity)
	if err == nil {
		t.Fatal("Expected error after unregister, got nil")
	}
}

// TestCoordinator_Multicast_All tests multicast to all identities
func TestCoordinator_Multicast_All(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	// Register 5 identities
	for i := 1; i <= 5; i++ {
		identity := "device-" + string(rune('0'+i))
		coordinator.Register(ctx, identity, map[string]interface{}{"device_id": i}, 0)
	}

	// Multicast to all (no filter)
	payload := []byte("firmware update available")
	result, err := coordinator.Multicast(ctx, nil, payload)
	if err != nil {
		t.Fatalf("Multicast failed: %v", err)
	}

	if result.TargetCount != 5 {
		t.Errorf("Expected 5 targets, got %d", result.TargetCount)
	}

	// In mock implementation, all should succeed
	if result.DeliveredCount != 5 {
		t.Errorf("Expected 5 delivered, got %d", result.DeliveredCount)
	}
}

// TestCoordinator_Multicast_Filtered tests multicast to filtered subset
func TestCoordinator_Multicast_Filtered(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	// Register devices with different firmware versions
	coordinator.Register(ctx, "device-1", map[string]interface{}{"firmware": "1.0.0"}, 0)
	coordinator.Register(ctx, "device-2", map[string]interface{}{"firmware": "1.0.0"}, 0)
	coordinator.Register(ctx, "device-3", map[string]interface{}{"firmware": "2.0.0"}, 0)
	coordinator.Register(ctx, "device-4", map[string]interface{}{"firmware": "1.0.0"}, 0)

	// Multicast only to devices with firmware 1.0.0 (should match 3 devices)
	filter := NewFilter(map[string]interface{}{
		"firmware": "1.0.0",
	})

	payload := []byte("critical security update")
	result, err := coordinator.Multicast(ctx, filter, payload)
	if err != nil {
		t.Fatalf("Multicast failed: %v", err)
	}

	if result.TargetCount != 3 {
		t.Errorf("Expected 3 targets, got %d", result.TargetCount)
	}

	if result.DeliveredCount != 3 {
		t.Errorf("Expected 3 delivered, got %d", result.DeliveredCount)
	}
}

// TestCoordinator_Concurrent tests concurrent operations (race detector)
func TestCoordinator_Concurrent(t *testing.T) {
	ctx := context.Background()
	coordinator := NewCoordinatorWithMocks(t)
	defer coordinator.Close()

	// Run 50 concurrent registrations
	done := make(chan bool)
	for i := 0; i < 50; i++ {
		go func(id int) {
			identity := "user-" + string(rune('0'+id))
			coordinator.Register(ctx, identity, map[string]interface{}{"id": id}, 0)
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 50; i++ {
		<-done
	}

	// Enumerate should return all 50
	results, err := coordinator.Enumerate(ctx, nil)
	if err != nil {
		t.Fatalf("Enumerate failed: %v", err)
	}

	if len(results) != 50 {
		t.Errorf("Expected 50 identities after concurrent registration, got %d", len(results))
	}
}

// NewCoordinatorWithMocks creates a coordinator with mock backends for testing
func NewCoordinatorWithMocks(t *testing.T) *Coordinator {
	config := DefaultConfig()

	// Use in-memory mock backends
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()

	coordinator, err := NewCoordinator(config, registry, messaging, nil)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	return coordinator
}
