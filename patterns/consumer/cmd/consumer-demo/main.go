package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	natstest "github.com/nats-io/nats-server/v2/test"
)

func main() {
	var (
		mode = flag.String("mode", "stateless", "Consumer mode: stateless, stateful, or full-durability")
	)
	flag.Parse()

	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting consumer demo", "mode", *mode)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start embedded NATS server for demo
	opts := natstest.DefaultTestOptions
	opts.Port = 4222
	server := natstest.RunServer(&opts)
	defer server.Shutdown()

	slog.Info("Started embedded NATS server", "url", server.ClientURL())

	// Create consumer based on mode
	var c *consumer.Consumer
	var natsDriver *nats.NATSPattern
	var memstoreDriver *memstore.MemStore
	var err error

	switch *mode {
	case "stateless":
		c, natsDriver, err = createStatelessConsumer(ctx, server.ClientURL())
	case "stateful":
		c, natsDriver, memstoreDriver, err = createStatefulConsumer(ctx, server.ClientURL())
	default:
		log.Fatalf("Unsupported mode: %s", *mode)
	}

	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}

	// Set message processor
	messageCount := 0
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		messageCount++
		slog.Info("Processed message",
			"count", messageCount,
			"message_id", msg.MessageID,
			"topic", msg.Topic,
			"payload_size", len(msg.Payload))
		return nil
	})

	// Start consumer
	if err := c.Start(ctx); err != nil {
		log.Fatalf("Failed to start consumer: %v", err)
	}

	slog.Info("Consumer started successfully")

	// Publish test messages
	go publishTestMessages(ctx, natsDriver)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down...")

	// Stop consumer
	if err := c.Stop(ctx); err != nil {
		slog.Error("Failed to stop consumer", "error", err)
	}

	// Stop drivers
	if natsDriver != nil {
		natsDriver.Stop(ctx)
	}
	if memstoreDriver != nil {
		memstoreDriver.Stop(ctx)
	}

	slog.Info("Consumer demo stopped", "messages_processed", messageCount)
}

func createStatelessConsumer(ctx context.Context, natsURL string) (*consumer.Consumer, *nats.NATSPattern, error) {
	// Configure consumer (stateless - no state store)
	config := consumer.Config{
		Name: "demo-consumer-stateless",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": natsURL,
				},
			},
			// No StateStore - runs stateless
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "demo-group",
			Topic:         "demo.topic",
			MaxRetries:    1,
			AutoCommit:    false,
		},
	}

	c, err := consumer.New(config)
	if err != nil {
		return nil, nil, err
	}

	// Initialize NATS driver
	natsDriver := nats.New()
	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"url": natsURL,
		},
	}

	if err := natsDriver.Initialize(ctx, natsConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize NATS: %w", err)
	}

	if err := natsDriver.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to start NATS: %w", err)
	}

	// Bind slots (no state store for stateless mode)
	if err := c.BindSlots(natsDriver, nil, nil); err != nil {
		return nil, nil, fmt.Errorf("failed to bind slots: %w", err)
	}

	return c, natsDriver, nil
}

func createStatefulConsumer(ctx context.Context, natsURL string) (*consumer.Consumer, *nats.NATSPattern, *memstore.MemStore, error) {
	// Configure consumer (stateful - with state store)
	config := consumer.Config{
		Name: "demo-consumer-stateful",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
			},
			StateStore: consumer.SlotBinding{
				Driver: "memstore",
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "demo-group",
			Topic:         "demo.topic",
			MaxRetries:    3,
			AutoCommit:    true,
		},
	}

	c, err := consumer.New(config)
	if err != nil {
		return nil, nil, nil, err
	}

	// Initialize NATS driver
	natsDriver := nats.New()
	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"url": natsURL,
		},
	}

	if err := natsDriver.Initialize(ctx, natsConfig); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize NATS: %w", err)
	}

	if err := natsDriver.Start(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start NATS: %w", err)
	}

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
		return nil, nil, nil, fmt.Errorf("failed to initialize MemStore: %w", err)
	}

	// Bind slots (with state store for stateful mode)
	if err := c.BindSlots(natsDriver, memstoreDriver, nil); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to bind slots: %w", err)
	}

	return c, natsDriver, memstoreDriver, nil
}

func publishTestMessages(ctx context.Context, natsDriver *nats.NATSPattern) {
	// Wait for consumer to subscribe
	time.Sleep(200 * time.Millisecond)

	slog.Info("Publishing test messages...")

	for i := 0; i < 10; i++ {
		payload := []byte(fmt.Sprintf("test-message-%d", i))
		if _, err := natsDriver.Publish(ctx, "demo.topic", payload, nil); err != nil {
			slog.Error("Failed to publish message", "error", err, "index", i)
		}

		time.Sleep(500 * time.Millisecond)
	}

	slog.Info("Finished publishing test messages")
}
