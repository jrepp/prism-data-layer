package multicast_registry

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkCoordinator_Register benchmarks identity registration
func BenchmarkCoordinator_Register(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()
	metadata := map[string]interface{}{
		"status": "online",
		"region": "us-west",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identity := fmt.Sprintf("user-%d", i)
		coordinator.Register(ctx, identity, metadata, 0)
	}
}

// BenchmarkCoordinator_Register_WithTTL benchmarks registration with TTL
func BenchmarkCoordinator_Register_WithTTL(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()
	metadata := map[string]interface{}{
		"status": "online",
		"region": "us-west",
	}
	ttl := 5 * time.Minute

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identity := fmt.Sprintf("session-%d", i)
		coordinator.Register(ctx, identity, metadata, ttl)
	}
}

// BenchmarkCoordinator_Enumerate_NoFilter benchmarks enumerate without filter
func BenchmarkCoordinator_Enumerate_NoFilter(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register 1000 identities
	for i := 0; i < 1000; i++ {
		identity := fmt.Sprintf("device-%d", i)
		metadata := map[string]interface{}{"device_id": i, "status": "online"}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coordinator.Enumerate(ctx, nil)
	}
}

// BenchmarkCoordinator_Enumerate_WithFilter benchmarks enumerate with filter
func BenchmarkCoordinator_Enumerate_WithFilter(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register 1000 identities (500 online, 500 offline)
	for i := 0; i < 1000; i++ {
		identity := fmt.Sprintf("device-%d", i)
		status := "offline"
		if i%2 == 0 {
			status = "online"
		}
		metadata := map[string]interface{}{"device_id": i, "status": status}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	filter := NewFilter(map[string]interface{}{"status": "online"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coordinator.Enumerate(ctx, filter)
	}
}

// BenchmarkCoordinator_Multicast_10 benchmarks multicast to 10 identities
func BenchmarkCoordinator_Multicast_10(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register 10 identities
	for i := 0; i < 10; i++ {
		identity := fmt.Sprintf("device-%d", i)
		metadata := map[string]interface{}{"device_id": i, "status": "online"}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	payload := []byte("benchmark-message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coordinator.Multicast(ctx, nil, payload)
	}
}

// BenchmarkCoordinator_Multicast_100 benchmarks multicast to 100 identities
func BenchmarkCoordinator_Multicast_100(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register 100 identities
	for i := 0; i < 100; i++ {
		identity := fmt.Sprintf("device-%d", i)
		metadata := map[string]interface{}{"device_id": i, "status": "online"}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	payload := []byte("benchmark-message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coordinator.Multicast(ctx, nil, payload)
	}
}

// BenchmarkCoordinator_Multicast_1000 benchmarks multicast to 1000 identities
func BenchmarkCoordinator_Multicast_1000(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register 1000 identities
	for i := 0; i < 1000; i++ {
		identity := fmt.Sprintf("device-%d", i)
		metadata := map[string]interface{}{"device_id": i, "status": "online"}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	payload := []byte("benchmark-message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coordinator.Multicast(ctx, nil, payload)
	}
}

// BenchmarkCoordinator_Multicast_Filtered benchmarks multicast with filter
func BenchmarkCoordinator_Multicast_Filtered(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register 1000 identities (500 online, 500 offline)
	for i := 0; i < 1000; i++ {
		identity := fmt.Sprintf("device-%d", i)
		status := "offline"
		if i%2 == 0 {
			status = "online"
		}
		metadata := map[string]interface{}{"device_id": i, "status": status}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	filter := NewFilter(map[string]interface{}{"status": "online"})
	payload := []byte("benchmark-message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		coordinator.Multicast(ctx, filter, payload)
	}
}

// BenchmarkCoordinator_Unregister benchmarks identity removal
func BenchmarkCoordinator_Unregister(b *testing.B) {
	config := DefaultConfig()
	registry := NewMockRegistryBackend()
	messaging := NewMockMessagingBackend()
	coordinator, _ := NewCoordinator(config, registry, messaging, nil)
	defer coordinator.Close()

	ctx := context.Background()

	// Pre-register N identities
	for i := 0; i < b.N; i++ {
		identity := fmt.Sprintf("user-%d", i)
		metadata := map[string]interface{}{"user_id": i}
		coordinator.Register(ctx, identity, metadata, 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identity := fmt.Sprintf("user-%d", i)
		coordinator.Unregister(ctx, identity)
	}
}

// BenchmarkFilter_SimpleEquality benchmarks simple equality filter
func BenchmarkFilter_SimpleEquality(b *testing.B) {
	filter := NewFilter(map[string]interface{}{"status": "online"})
	metadata := map[string]interface{}{
		"status": "online",
		"region": "us-west",
		"count":  42,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Matches(metadata)
	}
}

// BenchmarkFilter_MultipleConditions benchmarks filter with multiple conditions
func BenchmarkFilter_MultipleConditions(b *testing.B) {
	filter := NewFilter(map[string]interface{}{
		"status": "online",
		"region": "us-west",
		"tier":   "premium",
	})
	metadata := map[string]interface{}{
		"status": "online",
		"region": "us-west",
		"tier":   "premium",
		"count":  42,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Matches(metadata)
	}
}
