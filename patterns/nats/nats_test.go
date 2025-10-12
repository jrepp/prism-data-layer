package nats

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
)

// setupTestNATS creates a test NATS server and initialized pattern
func setupTestNATS(t *testing.T) (*NATSPattern, *natsserver.Server) {
	t.Helper()

	// Start embedded NATS server for testing
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // Random port
	server := natstest.RunServer(&opts)

	// Create pattern config pointing to test server
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url":              server.ClientURL(),
			"max_reconnects":   10,
			"reconnect_wait":   "1s",
			"timeout":          "2s",
			"ping_interval":    "5s",
			"max_pending_msgs": 1000,
			"enable_jetstream": false,
		},
	}

	// Initialize pattern
	plugin := New()
	ctx := context.Background()
	if err := plugin.Initialize(ctx, config); err != nil {
		server.Shutdown()
		t.Fatalf("failed to initialize plugin: %v", err)
	}

	if err := plugin.Start(ctx); err != nil {
		server.Shutdown()
		t.Fatalf("failed to start plugin: %v", err)
	}

	return plugin, server
}

func TestNATSPattern_Initialize(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	if plugin.name != "nats-test" {
		t.Errorf("Expected name 'nats-test', got '%s'", plugin.name)
	}

	if plugin.conn == nil {
		t.Error("Connection should be established")
	}
}

func TestNATSPattern_Health(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	health, err := plugin.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if health.Status != core.HealthHealthy {
		t.Errorf("Expected HEALTHY status, got %v", health.Status)
	}

	if health.Message == "" {
		t.Error("Expected non-empty health message")
	}
}

func TestNATSPattern_PublishSubscribe(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.topic"
	subscriberID := "subscriber-1"
	payload := []byte("test message")

	// Subscribe first
	msgChan, err := plugin.Subscribe(ctx, topic, subscriberID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Give subscription time to be established
	time.Sleep(100 * time.Millisecond)

	// Publish message
	messageID, err := plugin.Publish(ctx, topic, payload, nil)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if messageID == "" {
		t.Error("Expected non-empty message ID")
	}

	// Wait for message
	select {
	case msg := <-msgChan:
		if msg.Topic != topic {
			t.Errorf("Expected topic '%s', got '%s'", topic, msg.Topic)
		}
		if string(msg.Payload) != string(payload) {
			t.Errorf("Expected payload '%s', got '%s'", payload, msg.Payload)
		}
		if msg.MessageID == "" {
			t.Error("Expected non-empty message ID")
		}
		if msg.Timestamp == 0 {
			t.Error("Expected non-zero timestamp")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// Unsubscribe
	if err := plugin.Unsubscribe(ctx, topic, subscriberID); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
}

func TestNATSPattern_MultiplePubSub(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.multi"

	// Publish multiple messages without subscribers (fire and forget)
	for i := 0; i < 5; i++ {
		payload := []byte("message-" + string(rune('A'+i)))
		if _, err := plugin.Publish(ctx, topic, payload, nil); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}

	// No errors should occur
	t.Log("Successfully published 5 messages to topic with no subscribers")
}

func TestNATSPattern_Fanout(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.fanout"
	numSubscribers := 3
	payload := []byte("fanout message")

	// Create multiple subscribers
	var channels []<-chan *Message
	for i := 0; i < numSubscribers; i++ {
		subscriberID := "subscriber-" + string(rune('1'+i))
		msgChan, err := plugin.Subscribe(ctx, topic, subscriberID)
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
		channels = append(channels, msgChan)
	}

	// Give subscriptions time to be established
	time.Sleep(100 * time.Millisecond)

	// Publish one message
	if _, err := plugin.Publish(ctx, topic, payload, nil); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// All subscribers should receive the message
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	for i, ch := range channels {
		go func(idx int, msgChan <-chan *Message) {
			defer wg.Done()
			select {
			case msg := <-msgChan:
				if string(msg.Payload) != string(payload) {
					t.Errorf("Subscriber %d: Expected payload '%s', got '%s'", idx, payload, msg.Payload)
				}
			case <-time.After(2 * time.Second):
				t.Errorf("Subscriber %d: Timeout waiting for message", idx)
			}
		}(i, ch)
	}

	wg.Wait()

	// Cleanup - unsubscribe all
	for i := 0; i < numSubscribers; i++ {
		subscriberID := "subscriber-" + string(rune('1'+i))
		if err := plugin.Unsubscribe(ctx, topic, subscriberID); err != nil {
			t.Fatalf("Unsubscribe() error = %v", err)
		}
	}
}

func TestNATSPattern_MessageOrdering(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.ordering"
	subscriberID := "subscriber-1"
	numMessages := 10

	// Subscribe
	msgChan, err := plugin.Subscribe(ctx, topic, subscriberID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish messages in order
	for i := 0; i < numMessages; i++ {
		payload := []byte("message-" + string(rune('0'+i)))
		if _, err := plugin.Publish(ctx, topic, payload, nil); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
		// Small delay to ensure ordering
		time.Sleep(10 * time.Millisecond)
	}

	// Verify messages received in order
	for i := 0; i < numMessages; i++ {
		select {
		case msg := <-msgChan:
			expected := "message-" + string(rune('0'+i))
			if string(msg.Payload) != expected {
				t.Errorf("Message %d: Expected '%s', got '%s'", i, expected, string(msg.Payload))
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for message %d", i)
		}
	}

	// Unsubscribe
	if err := plugin.Unsubscribe(ctx, topic, subscriberID); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
}

func TestNATSPattern_UnsubscribeStopsMessages(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.unsubscribe"
	subscriberID := "subscriber-1"

	// Subscribe
	msgChan, err := plugin.Subscribe(ctx, topic, subscriberID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish first message
	if _, err := plugin.Publish(ctx, topic, []byte("message-1"), nil); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// Receive first message
	select {
	case msg := <-msgChan:
		if string(msg.Payload) != "message-1" {
			t.Errorf("Expected 'message-1', got '%s'", string(msg.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first message")
	}

	// Unsubscribe
	if err := plugin.Unsubscribe(ctx, topic, subscriberID); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish second message after unsubscribe
	if _, err := plugin.Publish(ctx, topic, []byte("message-2"), nil); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// Should NOT receive second message
	select {
	case msg := <-msgChan:
		t.Errorf("Should not receive message after unsubscribe, got: %s", string(msg.Payload))
	case <-time.After(500 * time.Millisecond):
		// Expected: timeout means no message received
		t.Log("Correctly did not receive message after unsubscribe")
	}
}

func TestNATSPattern_ConcurrentPublish(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.concurrent"
	numGoroutines := 10
	messagesPerGoroutine := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch concurrent publishers
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				payload := []byte("goroutine-" + string(rune('0'+goroutineID)) + "-msg-" + string(rune('0'+j)))
				if _, err := plugin.Publish(ctx, topic, payload, nil); err != nil {
					t.Errorf("Publish() error = %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Successfully published %d messages concurrently", numGoroutines*messagesPerGoroutine)
}

func TestNATSPattern_PublishWithMetadata(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()
	topic := "test.metadata"
	subscriberID := "subscriber-1"
	payload := []byte("test message")
	metadata := map[string]string{
		"user":      "test-user",
		"timestamp": "2025-10-10",
		"priority":  "high",
	}

	// Subscribe
	msgChan, err := plugin.Subscribe(ctx, topic, subscriberID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish with metadata (note: core NATS doesn't preserve headers, but shouldn't error)
	messageID, err := plugin.Publish(ctx, topic, payload, metadata)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if messageID == "" {
		t.Error("Expected non-empty message ID")
	}

	// Message should still be received
	select {
	case msg := <-msgChan:
		if string(msg.Payload) != string(payload) {
			t.Errorf("Expected payload '%s', got '%s'", payload, msg.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// Cleanup
	if err := plugin.Unsubscribe(ctx, topic, subscriberID); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
}

func TestNATSPattern_HealthAfterDisconnect(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()

	ctx := context.Background()

	// Initial health check should be healthy
	health1, err := plugin.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health1.Status != core.HealthHealthy {
		t.Errorf("Expected HEALTHY status, got %v", health1.Status)
	}

	// Stop the pattern (closes connection)
	if err := plugin.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Health check after stop should be unhealthy
	health2, err := plugin.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health2.Status != core.HealthUnhealthy {
		t.Errorf("Expected UNHEALTHY status after stop, got %v", health2.Status)
	}
}

func TestNATSPattern_UnsubscribeNonExistent(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()
	defer plugin.Stop(context.Background())

	ctx := context.Background()

	// Try to unsubscribe from non-existent subscription
	err := plugin.Unsubscribe(ctx, "nonexistent.topic", "nonexistent-subscriber")
	if err == nil {
		t.Error("Expected error when unsubscribing from non-existent subscription")
	}
}

func TestNATSPattern_PublishWithoutConnection(t *testing.T) {
	plugin := New()
	plugin.name = "test"
	plugin.version = "0.1.0"
	// Don't initialize (no connection)

	ctx := context.Background()
	_, err := plugin.Publish(ctx, "test.topic", []byte("payload"), nil)
	if err == nil {
		t.Error("Expected error when publishing without connection")
	}
}

func TestNATSPattern_SubscribeWithoutConnection(t *testing.T) {
	plugin := New()
	plugin.name = "test"
	plugin.version = "0.1.0"
	// Don't initialize (no connection)

	ctx := context.Background()
	_, err := plugin.Subscribe(ctx, "test.topic", "subscriber-1")
	if err == nil {
		t.Error("Expected error when subscribing without connection")
	}
}

func TestNATSPattern_NameAndVersion(t *testing.T) {
	plugin := New()

	if plugin.Name() != "nats" {
		t.Errorf("Expected name 'nats', got '%s'", plugin.Name())
	}

	if plugin.Version() != "0.1.0" {
		t.Errorf("Expected version '0.1.0', got '%s'", plugin.Version())
	}
}

func TestNATSPattern_InitializeWithDefaults(t *testing.T) {
	// Start embedded NATS server
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	server := natstest.RunServer(&opts)
	defer server.Shutdown()

	// Create pattern with minimal config (tests default values)
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-minimal",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": server.ClientURL(),
		},
	}

	plugin := New()
	ctx := context.Background()

	if err := plugin.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() with defaults error = %v", err)
	}
	defer plugin.Stop(ctx)

	// Verify defaults were applied
	if plugin.config.MaxReconnects == 0 {
		t.Error("Expected non-zero MaxReconnects default")
	}
	if plugin.config.Timeout == 0 {
		t.Error("Expected non-zero Timeout default")
	}
}

func TestNATSPattern_InitializeFailure(t *testing.T) {
	// Create pattern with invalid NATS URL
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-fail",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url":     "nats://invalid-host-that-does-not-exist:9999",
			"timeout": "100ms",
		},
	}

	plugin := New()
	ctx := context.Background()

	err := plugin.Initialize(ctx, config)
	if err == nil {
		t.Error("Expected error when connecting to invalid NATS URL")
	}
}

func TestNATSPattern_StopWithActiveSubscriptions(t *testing.T) {
	plugin, server := setupTestNATS(t)
	defer server.Shutdown()

	ctx := context.Background()

	// Create multiple subscriptions
	for i := 0; i < 3; i++ {
		topic := "test.cleanup." + string(rune('A'+i))
		subscriberID := "subscriber-" + string(rune('1'+i))
		_, err := plugin.Subscribe(ctx, topic, subscriberID)
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}

	// Stop should cleanup all subscriptions
	if err := plugin.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify all subscriptions were cleaned up
	plugin.subsMu.RLock()
	subCount := len(plugin.subs)
	plugin.subsMu.RUnlock()

	if subCount != 0 {
		t.Errorf("Expected 0 subscriptions after stop, got %d", subCount)
	}
}
