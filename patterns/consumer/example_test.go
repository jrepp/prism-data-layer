package consumer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	natstest "github.com/nats-io/nats-server/v2/test"
)

// Example demonstrates using the consumer pattern with NATS and MemStore.
func Example() {
	// Start embedded NATS server for demo
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	server := natstest.RunServer(&opts)
	defer server.Shutdown()

	// Configure consumer
	config := consumer.Config{
		Name: "example-consumer",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": server.ClientURL(),
				},
			},
			StateStore: consumer.SlotBinding{
				Driver: "memstore",
				Config: map[string]interface{}{
					"max_keys": 1000,
				},
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "example-group",
			Topic:         "example.topic",
			MaxRetries:    3,
			AutoCommit:    true,
		},
	}

	// Create consumer
	c, err := consumer.New(config)
	if err != nil {
		fmt.Printf("Failed to create consumer: %v\n", err)
		return
	}

	// Initialize NATS driver
	natsDriver := nats.New()
	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"url": server.ClientURL(),
		},
	}

	ctx := context.Background()
	if err := natsDriver.Initialize(ctx, natsConfig); err != nil {
		fmt.Printf("Failed to initialize NATS: %v\n", err)
		return
	}
	if err := natsDriver.Start(ctx); err != nil {
		fmt.Printf("Failed to start NATS: %v\n", err)
		return
	}
	defer natsDriver.Stop(ctx)

	// Initialize MemStore driver
	memstoreDriver := memstore.New()
	memstoreConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"max_keys": 1000,
		},
	}
	if err := memstoreDriver.Initialize(ctx, memstoreConfig); err != nil {
		fmt.Printf("Failed to initialize MemStore: %v\n", err)
		return
	}

	// Bind slots to consumer
	if err := c.BindSlots(natsDriver, memstoreDriver, nil); err != nil {
		fmt.Printf("Failed to bind slots: %v\n", err)
		return
	}

	// Set message processor
	messageCount := 0
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		messageCount++
		fmt.Printf("Processed message %d: %s\n", messageCount, msg.MessageID)
		return nil
	})

	// Start consumer
	if err := c.Start(ctx); err != nil {
		fmt.Printf("Failed to start consumer: %v\n", err)
		return
	}

	// Give consumer time to subscribe
	time.Sleep(100 * time.Millisecond)

	// Publish test messages
	for i := 0; i < 3; i++ {
		payload := []byte(fmt.Sprintf("message-%d", i))
		if _, err := natsDriver.Publish(ctx, "example.topic", payload, nil); err != nil {
			fmt.Printf("Failed to publish: %v\n", err)
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Check health
	health, err := c.Health(ctx)
	if err != nil {
		fmt.Printf("Failed to get health: %v\n", err)
		return
	}
	fmt.Printf("Consumer health: %s\n", health.Status)

	// Stop consumer
	if err := c.Stop(ctx); err != nil {
		fmt.Printf("Failed to stop consumer: %v\n", err)
		return
	}

	// Output:
	// Processed message 1: <message-id>
	// Processed message 2: <message-id>
	// Processed message 3: <message-id>
	// Consumer health: HEALTHY
}

// TestConsumerWithMemStore demonstrates testing the consumer with MemStore.
func TestConsumerWithMemStore(t *testing.T) {
	// Start NATS server
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	server := natstest.RunServer(&opts)
	defer server.Shutdown()

	// Configure consumer
	config := consumer.Config{
		Name: "test-consumer",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
			},
			StateStore: consumer.SlotBinding{
				Driver: "memstore",
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "test-group",
			Topic:         "test.topic",
			MaxRetries:    1,
			AutoCommit:    true,
		},
	}

	c, err := consumer.New(config)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	// Initialize drivers
	natsDriver := nats.New()
	natsConfig := &plugin.Config{
		Plugin:  plugin.PluginConfig{Name: "nats", Version: "0.1.0"},
		Backend: map[string]interface{}{"url": server.ClientURL()},
	}
	ctx := context.Background()
	if err := natsDriver.Initialize(ctx, natsConfig); err != nil {
		t.Fatalf("Failed to initialize NATS: %v", err)
	}
	if err := natsDriver.Start(ctx); err != nil {
		t.Fatalf("Failed to start NATS: %v", err)
	}
	defer natsDriver.Stop(ctx)

	memstoreDriver := memstore.New()
	memstoreConfig := &plugin.Config{
		Plugin:  plugin.PluginConfig{Name: "memstore", Version: "0.1.0"},
		Backend: map[string]interface{}{"max_keys": 100},
	}
	if err := memstoreDriver.Initialize(ctx, memstoreConfig); err != nil {
		t.Fatalf("Failed to initialize MemStore: %v", err)
	}

	// Bind and start
	if err := c.BindSlots(natsDriver, memstoreDriver, nil); err != nil {
		t.Fatalf("Failed to bind slots: %v", err)
	}

	processed := make(chan string, 10)
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		processed <- msg.MessageID
		return nil
	})

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Failed to start consumer: %v", err)
	}
	defer c.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Publish messages
	for i := 0; i < 3; i++ {
		payload := []byte(fmt.Sprintf("test-%d", i))
		if _, err := natsDriver.Publish(ctx, "test.topic", payload, nil); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Wait for processing
	count := 0
	timeout := time.After(2 * time.Second)
	for count < 3 {
		select {
		case msgID := <-processed:
			t.Logf("Processed: %s", msgID)
			count++
		case <-timeout:
			t.Fatalf("Timeout waiting for messages, got %d/3", count)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 messages, got %d", count)
	}
}
