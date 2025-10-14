package unified_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/patterns/producer"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends" // Register all backends
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProducerConsumerIntegration demonstrates multi-pattern acceptance testing
// using the framework. The consumer pattern is used to validate the producer pattern.
func TestProducerConsumerIntegration(t *testing.T) {
	tests := []framework.MultiPatternTest{
		{
			Name: "SingleMessage",
			RequiredPatterns: map[string]framework.Pattern{
				"producer": framework.PatternProducer,
				"consumer": framework.PatternConsumer,
			},
			Func:    testSingleMessage,
			Timeout: 30 * time.Second,
			Tags:    []string{"integration", "end-to-end"},
		},
		{
			Name: "MultipleMessages",
			RequiredPatterns: map[string]framework.Pattern{
				"producer": framework.PatternProducer,
				"consumer": framework.PatternConsumer,
			},
			Func:    testMultipleMessages,
			Timeout: 30 * time.Second,
			Tags:    []string{"integration", "end-to-end"},
		},
		{
			Name: "Metrics",
			RequiredPatterns: map[string]framework.Pattern{
				"producer": framework.PatternProducer,
				"consumer": framework.PatternConsumer,
			},
			Func:    testMetrics,
			Timeout: 30 * time.Second,
			Tags:    []string{"integration", "metrics"},
		},
		{
			Name: "StatePersistence",
			RequiredPatterns: map[string]framework.Pattern{
				"producer": framework.PatternProducer,
				"consumer": framework.PatternConsumer,
			},
			Func:    testStatePersistence,
			Timeout: 30 * time.Second,
			Tags:    []string{"integration", "state"},
		},
	}

	framework.RunMultiPatternTests(t, tests)
}

// testSingleMessage validates single message flow from producer to consumer
func testSingleMessage(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Setup producer
	driver := drivers["producer"]
	prod, cons, received := setupProducerConsumer(t, ctx, driver, driver)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	// Publish single message
	testPayload := []byte(`{"order_id":"123","amount":99.99}`)
	metadata := map[string]string{
		"content-type": "application/json",
		"version":      "v1",
	}

	err := prod.Publish(ctx, "unified-test-topic", testPayload, metadata)
	require.NoError(t, err, "Failed to publish message")

	// Wait for consumer to receive
	select {
	case msg := <-received:
		assert.Equal(t, testPayload, msg.Payload, "Payload mismatch")
		assert.Equal(t, "unified-test-topic", msg.Topic, "Topic mismatch")
		// Note: Core NATS doesn't support message headers/metadata
		// JetStream is required for metadata support
		if caps.Custom != nil {
			if jetstream, ok := caps.Custom["JetStream"].(bool); ok && jetstream {
				assert.Equal(t, "application/json", msg.Metadata["content-type"], "Metadata mismatch")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// testMultipleMessages validates multiple messages flow
func testMultipleMessages(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Setup producer and consumer
	driver := drivers["producer"]
	prod, cons, received := setupProducerConsumer(t, ctx, driver, driver)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	// Publish multiple messages
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		payload := []byte(`{"message_number":` + string(rune(i+'0')) + `}`)
		err := prod.Publish(ctx, "unified-test-topic", payload, nil)
		require.NoError(t, err, "Failed to publish message %d", i)
	}

	// Wait for all messages
	receivedCount := 0
	timeout := time.After(10 * time.Second)
receiveLoop:
	for receivedCount < messageCount {
		select {
		case <-received:
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d messages", receivedCount, messageCount)
			break receiveLoop
		}
	}

	assert.Equal(t, messageCount, receivedCount, "Should receive all messages")
}

// testMetrics validates producer and consumer metrics
func testMetrics(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Setup producer and consumer
	driver := drivers["producer"]
	prod, cons, received := setupProducerConsumer(t, ctx, driver, driver)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	// Publish test messages
	messageCount := 3
	for i := 0; i < messageCount; i++ {
		payload := []byte(`{"test":"message"}`)
		err := prod.Publish(ctx, "unified-test-topic", payload, nil)
		require.NoError(t, err, "Failed to publish message")
	}

	// Drain received channel
	for i := 0; i < messageCount; i++ {
		select {
		case <-received:
			// Message received
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for messages")
		}
	}

	// Verify producer metrics
	prodMetrics := prod.Metrics()
	assert.GreaterOrEqual(t, prodMetrics.MessagesPublished, int64(messageCount), "Producer should have published at least %d messages", messageCount)
	assert.Equal(t, int64(0), prodMetrics.MessagesFailed, "Producer should have no failures")

	// Verify consumer health
	consHealth, err := cons.Health(ctx)
	require.NoError(t, err, "Consumer health check failed")
	assert.Equal(t, plugin.HealthHealthy, consHealth.Status, "Consumer should be healthy")

	// Verify producer health
	prodHealth, err := prod.Health(ctx)
	require.NoError(t, err, "Producer health check failed")
	assert.Equal(t, plugin.HealthHealthy, prodHealth.Status, "Producer should be healthy")
}

// testStatePersistence validates state persistence (when state store available)
func testStatePersistence(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Setup producer and consumer
	driver := drivers["producer"]

	// Check if driver implements KeyValueBasicInterface (has state store)
	if _, ok := driver.(plugin.KeyValueBasicInterface); !ok {
		t.Skip("State store not available for this backend")
		return
	}

	prod, cons, received := setupProducerConsumer(t, ctx, driver, driver)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	// Publish a message
	testPayload := []byte(`{"test":"persistence"}`)
	err := prod.Publish(ctx, "unified-test-topic", testPayload, nil)
	require.NoError(t, err, "Failed to publish message")

	// Wait for message to be received
	select {
	case <-received:
		// Message received
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// Verify consumer state was persisted
	stateStore := driver.(plugin.KeyValueBasicInterface)
	stateKey := "consumer:unified-test-group:unified-test-topic:unified-test-consumer"
	stateData, found, err := stateStore.Get(stateKey)
	require.NoError(t, err, "Failed to get consumer state")
	assert.True(t, found, "Consumer state should be persisted")
	assert.NotEmpty(t, stateData, "State data should not be empty")
}

// setupProducerConsumer is a helper that creates and starts a producer and consumer
// Returns producer, consumer, and a channel for received messages
func setupProducerConsumer(
	t *testing.T,
	ctx context.Context,
	producerDriver interface{},
	consumerDriver interface{},
) (*producer.Producer, *consumer.Consumer, <-chan *plugin.PubSubMessage) {
	// Create producer
	producerConfig := producer.Config{
		Name: "unified-test-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "100ms",
			BatchSize:    0, // Publish immediately
		},
	}

	prod, err := producer.New(producerConfig)
	require.NoError(t, err, "Failed to create producer")

	// Determine state store for producer
	var prodStateStore plugin.KeyValueBasicInterface
	if kvInterface, ok := producerDriver.(plugin.KeyValueBasicInterface); ok {
		prodStateStore = kvInterface
	}

	err = prod.BindSlots(producerDriver, prodStateStore, nil) // nil = no object store for basic tests
	require.NoError(t, err, "Failed to bind producer slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")

	// Create consumer
	consumerConfig := consumer.Config{
		Name: "unified-test-consumer",
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "unified-test-group",
			Topic:         "unified-test-topic",
			MaxRetries:    3,
			AutoCommit:    true,
			BatchSize:     1,
		},
	}

	cons, err := consumer.New(consumerConfig)
	require.NoError(t, err, "Failed to create consumer")

	// Determine state store for consumer
	var consStateStore plugin.KeyValueBasicInterface
	if kvInterface, ok := consumerDriver.(plugin.KeyValueBasicInterface); ok {
		consStateStore = kvInterface
	}

	err = cons.BindSlots(consumerDriver, consStateStore, nil, nil) // nil, nil = no DLQ, no object store for basic tests
	require.NoError(t, err, "Failed to bind consumer slots")

	// Track received messages
	received := make(chan *plugin.PubSubMessage, 10)
	var receivedMu sync.Mutex
	receivedMessages := make([]*plugin.PubSubMessage, 0)

	cons.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		receivedMu.Lock()
		receivedMessages = append(receivedMessages, msg)
		receivedMu.Unlock()
		received <- msg
		return nil
	})

	err = cons.Start(ctx)
	require.NoError(t, err, "Failed to start consumer")

	// Give consumer time to subscribe
	time.Sleep(200 * time.Millisecond)

	return prod, cons, received
}
