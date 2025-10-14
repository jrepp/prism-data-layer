package backends

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisRegistry_SetGet(t *testing.T) {
	// Start miniredis server
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Set identity with metadata
	identity := "user-alice"
	metadata := map[string]interface{}{
		"user_id": "alice",
		"status":  "online",
		"room":    "engineering",
	}

	err = backend.Set(ctx, identity, metadata, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get identity
	result, err := backend.Get(ctx, identity)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.ID != identity {
		t.Errorf("Expected ID %s, got %s", identity, result.ID)
	}

	if result.Metadata["user_id"] != "alice" {
		t.Errorf("Expected user_id=alice, got %v", result.Metadata["user_id"])
	}
}

func TestRedisRegistry_SetWithTTL(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Set with TTL
	identity := "session-123"
	metadata := map[string]interface{}{"session_id": "123"}
	ttl := 2 * time.Second

	err = backend.Set(ctx, identity, metadata, ttl)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should exist immediately
	_, err = backend.Get(ctx, identity)
	if err != nil {
		t.Fatalf("Get failed immediately after set: %v", err)
	}

	// Fast-forward time in miniredis
	s.FastForward(3 * time.Second)

	// Should be expired
	_, err = backend.Get(ctx, identity)
	if err == nil {
		t.Error("Expected error for expired key, got nil")
	}
}

func TestRedisRegistry_Delete(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Set then delete
	identity := "user-bob"
	metadata := map[string]interface{}{"user": "bob"}

	backend.Set(ctx, identity, metadata, 0)

	err = backend.Delete(ctx, identity)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not exist
	_, err = backend.Get(ctx, identity)
	if err == nil {
		t.Error("Expected error after delete, got nil")
	}
}

func TestRedisRegistry_Scan(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Set multiple identities
	identities := []string{"user-1", "user-2", "user-3"}
	for _, id := range identities {
		backend.Set(ctx, id, map[string]interface{}{"id": id}, 0)
	}

	// Scan all
	results, err := backend.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 identities, got %d", len(results))
	}
}

func TestRedisRegistry_ScanSkipsExpired(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Set identity with short TTL
	backend.Set(ctx, "expires-soon", map[string]interface{}{"temp": true}, 1*time.Second)
	backend.Set(ctx, "stays", map[string]interface{}{"permanent": true}, 0)

	// Fast-forward time
	s.FastForward(2 * time.Second)

	// Scan should only return non-expired
	results, err := backend.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 non-expired identity, got %d", len(results))
	}

	if len(results) > 0 && results[0].ID != "stays" {
		t.Errorf("Expected 'stays', got %s", results[0].ID)
	}
}

func TestRedisRegistry_GetNotFound(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	_, err = backend.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent identity, got nil")
	}
}

func TestRedisRegistry_EnumerateFallback(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	backend, err := NewRedisRegistryBackend(s.Addr(), "", 0, "test:")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Enumerate should return error to trigger client-side fallback
	filter := NewFilter(map[string]interface{}{"status": "online"})
	_, err = backend.Enumerate(ctx, filter)
	if err == nil {
		t.Error("Expected error for Enumerate (not implemented), got nil")
	}
}
