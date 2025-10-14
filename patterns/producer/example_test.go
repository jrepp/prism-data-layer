package producer_test

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/producer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
)

// ExampleProducer_simple demonstrates basic producer usage.
func ExampleProducer_simple() {
	// Create configuration
	config := producer.Config{
		Name: "example-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "100ms",
			BatchSize:    0, // No batching
		},
	}

	// Create producer
	prod, err := producer.New(config)
	if err != nil {
		panic(err)
	}

	// Initialize NATS driver
	natsDriver := nats.NewDriver()
	cfg := map[string]interface{}{
		"url": "nats://localhost:4222",
	}
	if err := natsDriver.Init(context.Background(), cfg); err != nil {
		panic(err)
	}

	// Bind message sink
	if err := prod.BindSlots(natsDriver, nil); err != nil {
		panic(err)
	}

	// Start producer
	ctx := context.Background()
	if err := prod.Start(ctx); err != nil {
		panic(err)
	}
	defer prod.Stop(ctx)

	// Publish messages
	if err := prod.Publish(ctx, "orders.created", []byte(`{"order_id":"123"}`), nil); err != nil {
		panic(err)
	}

	// Output:
	// (producer publishes message)
}

// ExampleProducer_batching demonstrates batched publishing.
func ExampleProducer_batching() {
	config := producer.Config{
		Name: "batching-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:    3,
			RetryBackoff:  "100ms",
			BatchSize:     10,
			BatchInterval: "1s",
		},
	}

	prod, _ := producer.New(config)

	// Initialize backend
	memDriver := memstore.NewDriver()
	memDriver.Init(context.Background(), map[string]interface{}{"capacity": 1000})

	prod.BindSlots(memDriver, nil)

	ctx := context.Background()
	prod.Start(ctx)
	defer prod.Stop(ctx)

	// Publish multiple messages - will be batched
	for i := 0; i < 25; i++ {
		prod.Publish(ctx, "events", []byte(`{"event":"test"}`), nil)
	}

	// Flush remaining messages
	prod.Flush(ctx)

	// Output:
	// (producer publishes batches of 10, then remaining 5)
}

// ExampleProducer_deduplication demonstrates message deduplication.
func ExampleProducer_deduplication() {
	config := producer.Config{
		Name: "dedup-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:                  3,
			RetryBackoff:                "100ms",
			BatchSize:                   0,
			Deduplication:               true,
			DeduplicationWindowDuration: "5m",
		},
	}

	prod, _ := producer.New(config)

	// Need state store for deduplication
	memDriver := memstore.NewDriver()
	memDriver.Init(context.Background(), map[string]interface{}{"capacity": 1000})

	prod.BindSlots(memDriver, memDriver) // Use same memstore for both message sink and state

	ctx := context.Background()
	prod.Start(ctx)
	defer prod.Stop(ctx)

	payload := []byte(`{"order_id":"456"}`)

	// First publish - succeeds
	prod.Publish(ctx, "orders", payload, nil)

	// Duplicate publish - silently skipped
	prod.Publish(ctx, "orders", payload, nil)

	metrics := prod.Metrics()
	_ = metrics.MessagesDedup // Should be 1

	// Output:
	// (only one message published, second deduplicated)
}

// TestProducer_lifecycle tests the producer lifecycle.
func TestProducer_lifecycle(t *testing.T) {
	config := producer.Config{
		Name: "test-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "10ms",
			BatchSize:    0,
		},
	}

	prod, err := producer.New(config)
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}

	// Initialize backend
	memDriver := memstore.NewDriver()
	if err := memDriver.Init(context.Background(), map[string]interface{}{"capacity": 100}); err != nil {
		t.Fatalf("Failed to init memstore: %v", err)
	}

	// Bind slots
	if err := prod.BindSlots(memDriver, nil); err != nil {
		t.Fatalf("Failed to bind slots: %v", err)
	}

	ctx := context.Background()

	// Start producer
	if err := prod.Start(ctx); err != nil {
		t.Fatalf("Failed to start producer: %v", err)
	}

	// Publish test message
	if err := prod.Publish(ctx, "test.topic", []byte("test payload"), nil); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Check metrics
	metrics := prod.Metrics()
	if metrics.MessagesPublished != 1 {
		t.Errorf("Expected 1 message published, got %d", metrics.MessagesPublished)
	}

	// Check health
	health, err := prod.Health(ctx)
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	if health.Status != "healthy" {
		t.Errorf("Expected healthy status, got %s", health.Status)
	}

	// Stop producer
	if err := prod.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop producer: %v", err)
	}
}

// TestProducer_batching tests batching behavior.
func TestProducer_batching(t *testing.T) {
	config := producer.Config{
		Name: "batch-test",
		Behavior: producer.BehaviorConfig{
			MaxRetries:    3,
			RetryBackoff:  "10ms",
			BatchSize:     5,
			BatchInterval: "100ms",
		},
	}

	prod, err := producer.New(config)
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}

	memDriver := memstore.NewDriver()
	memDriver.Init(context.Background(), map[string]interface{}{"capacity": 100})
	prod.BindSlots(memDriver, nil)

	ctx := context.Background()
	prod.Start(ctx)
	defer prod.Stop(ctx)

	// Publish 12 messages (should create 2 full batches + 2 in partial batch)
	for i := 0; i < 12; i++ {
		if err := prod.Publish(ctx, "test", []byte("msg"), nil); err != nil {
			t.Fatalf("Failed to publish message %d: %v", i, err)
		}
	}

	// Wait for batch interval
	time.Sleep(200 * time.Millisecond)

	metrics := prod.Metrics()
	if metrics.MessagesPublished != 12 {
		t.Errorf("Expected 12 messages published, got %d", metrics.MessagesPublished)
	}

	if metrics.BatchesPublished < 3 {
		t.Errorf("Expected at least 3 batches, got %d", metrics.BatchesPublished)
	}
}

// TestProducer_deduplication tests deduplication logic.
func TestProducer_deduplication(t *testing.T) {
	config := producer.Config{
		Name: "dedup-test",
		Behavior: producer.BehaviorConfig{
			MaxRetries:                  3,
			RetryBackoff:                "10ms",
			BatchSize:                   0,
			Deduplication:               true,
			DeduplicationWindowDuration: "1m",
		},
	}

	prod, _ := producer.New(config)

	memDriver := memstore.NewDriver()
	memDriver.Init(context.Background(), map[string]interface{}{"capacity": 100})
	prod.BindSlots(memDriver, memDriver) // Same driver for both slots

	ctx := context.Background()
	prod.Start(ctx)
	defer prod.Stop(ctx)

	payload := []byte("duplicate test")

	// Publish same payload 5 times
	for i := 0; i < 5; i++ {
		prod.Publish(ctx, "test", payload, nil)
	}

	metrics := prod.Metrics()

	// Should only publish once, dedupe the rest
	if metrics.MessagesPublished != 1 {
		t.Errorf("Expected 1 message published, got %d", metrics.MessagesPublished)
	}

	if metrics.MessagesDedup != 4 {
		t.Errorf("Expected 4 deduplicated messages, got %d", metrics.MessagesDedup)
	}
}
