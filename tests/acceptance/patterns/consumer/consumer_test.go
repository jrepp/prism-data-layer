package consumer_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends" // Register all backends
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsumerPattern tests the consumer pattern with various backend combinations
func TestConsumerPattern(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name:               "StatelessConsumer",
			Func:               testStatelessConsumer,
			RequiresCapability: "",
		},
		{
			Name:               "StatefulConsumer",
			Func:               testStatefulConsumer,
			RequiresCapability: "",
		},
		{
			Name:               "ConsumerWithRetry",
			Func:               testConsumerWithRetry,
			RequiresCapability: "",
		},
		{
			Name:               "ConsumerStateRecovery",
			Func:               testConsumerStateRecovery,
			RequiresCapability: "",
		},
	}

	framework.RunPatternTests(t, framework.Pattern("Consumer"), tests)
}

// testStatelessConsumer tests a consumer without state persistence
func testStatelessConsumer(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cast driver to consumer backends
	backends := driver.(*ConsumerBackends)

	// Create consumer configuration
	config := consumer.Config{
		Name:        "test-stateless-consumer",
		Description: "Test consumer without state",
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "test-group",
			Topic:         "test-topic",
			MaxRetries:    3,
			AutoCommit:    false, // No state to commit
			BatchSize:     1,
		},
	}

	// Create consumer
	c, err := consumer.New(config)
	require.NoError(t, err, "Failed to create consumer")

	// Bind slots (no state store for stateless)
	err = c.BindSlots(backends.MessageSource, nil, nil, nil)
	require.NoError(t, err, "Failed to bind slots")

	// Track processed messages
	processed := make(chan *plugin.PubSubMessage, 10)
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		processed <- msg
		return nil
	})

	// Start consumer
	err = c.Start(ctx)
	require.NoError(t, err, "Failed to start consumer")
	defer c.Stop(ctx)

	// Give consumer time to subscribe
	time.Sleep(100 * time.Millisecond)

	// Publish test messages
	pubsub := backends.MessageSource.(plugin.PubSubInterface)
	testPayload := []byte("test message 1")
	_, err = pubsub.Publish(ctx, "test-topic", testPayload, nil)
	require.NoError(t, err, "Failed to publish message")

	// Wait for message to be processed
	select {
	case msg := <-processed:
		assert.Equal(t, testPayload, msg.Payload, "Message payload mismatch")
		assert.Equal(t, "test-topic", msg.Topic, "Topic mismatch")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message processing")
	}

	// Verify consumer health
	health, err := c.Health(ctx)
	require.NoError(t, err, "Health check failed")
	assert.Equal(t, plugin.HealthHealthy, health.Status, "Consumer should be healthy")
}

// testStatefulConsumer tests a consumer with state persistence
func testStatefulConsumer(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// Require state store for this test
	if backends.StateStore == nil {
		t.Skip("State store not configured")
	}

	config := consumer.Config{
		Name:        "test-stateful-consumer",
		Description: "Test consumer with state persistence",
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "stateful-group",
			Topic:         "stateful-topic",
			MaxRetries:    3,
			AutoCommit:    true,
			BatchSize:     1,
		},
	}

	c, err := consumer.New(config)
	require.NoError(t, err, "Failed to create consumer")

	// Bind slots with state store
	err = c.BindSlots(backends.MessageSource, backends.StateStore, nil, nil)
	require.NoError(t, err, "Failed to bind slots")

	processedCount := 0
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		processedCount++
		return nil
	})

	err = c.Start(ctx)
	require.NoError(t, err, "Failed to start consumer")

	time.Sleep(100 * time.Millisecond)

	// Publish messages
	pubsub := backends.MessageSource.(plugin.PubSubInterface)
	for i := 0; i < 3; i++ {
		_, err = pubsub.Publish(ctx, "stateful-topic", []byte("message"), nil)
		require.NoError(t, err, "Failed to publish message")
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Stop consumer
	err = c.Stop(ctx)
	require.NoError(t, err, "Failed to stop consumer")

	assert.Equal(t, 3, processedCount, "Should have processed 3 messages")

	// Verify state was persisted
	stateKey := "consumer:stateful-group:stateful-topic:test-stateful-consumer"
	stateData, found, err := backends.StateStore.Get(stateKey)
	require.NoError(t, err, "Failed to get state")
	assert.True(t, found, "State should be persisted")
	assert.NotEmpty(t, stateData, "State data should not be empty")
}

// testConsumerWithRetry tests consumer retry logic
func testConsumerWithRetry(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	config := consumer.Config{
		Name:        "test-retry-consumer",
		Description: "Test consumer retry behavior",
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "retry-group",
			Topic:         "retry-topic",
			MaxRetries:    2,
			AutoCommit:    false,
			BatchSize:     1,
		},
	}

	c, err := consumer.New(config)
	require.NoError(t, err, "Failed to create consumer")

	err = c.BindSlots(backends.MessageSource, nil, nil, nil)
	require.NoError(t, err, "Failed to bind slots")

	var attemptCount atomic.Int32
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		count := attemptCount.Add(1)
		if count <= 2 {
			return assert.AnError // Fail first 2 attempts
		}
		return nil // Succeed on 3rd attempt
	})

	err = c.Start(ctx)
	require.NoError(t, err, "Failed to start consumer")
	defer c.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Publish message
	pubsub := backends.MessageSource.(plugin.PubSubInterface)
	_, err = pubsub.Publish(ctx, "retry-topic", []byte("retry-test"), nil)
	require.NoError(t, err, "Failed to publish message")

	// Wait for retries
	time.Sleep(1 * time.Second)

	// Should have attempted 3 times (initial + 2 retries)
	assert.GreaterOrEqual(t, int(attemptCount.Load()), 3, "Should have retried at least 2 times")
}

// testConsumerStateRecovery tests consumer state recovery after restart
func testConsumerStateRecovery(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	if backends.StateStore == nil {
		t.Skip("State store not configured")
	}

	config := consumer.Config{
		Name:        "test-recovery-consumer",
		Description: "Test consumer state recovery",
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "recovery-group",
			Topic:         "recovery-topic",
			MaxRetries:    3,
			AutoCommit:    true,
			BatchSize:     1,
		},
	}

	// First consumer session
	c1, err := consumer.New(config)
	require.NoError(t, err, "Failed to create first consumer")

	err = c1.BindSlots(backends.MessageSource, backends.StateStore, nil, nil)
	require.NoError(t, err, "Failed to bind slots")

	firstProcessed := 0
	c1.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		firstProcessed++
		return nil
	})

	err = c1.Start(ctx)
	require.NoError(t, err, "Failed to start first consumer")

	time.Sleep(100 * time.Millisecond)

	// Publish messages to first consumer
	pubsub := backends.MessageSource.(plugin.PubSubInterface)
	for i := 0; i < 5; i++ {
		_, err = pubsub.Publish(ctx, "recovery-topic", []byte("message"), nil)
		require.NoError(t, err, "Failed to publish message")
	}

	time.Sleep(500 * time.Millisecond)

	// Stop first consumer
	err = c1.Stop(ctx)
	require.NoError(t, err, "Failed to stop first consumer")

	assert.Equal(t, 5, firstProcessed, "First consumer should process 5 messages")

	// Create second consumer with same config
	c2, err := consumer.New(config)
	require.NoError(t, err, "Failed to create second consumer")

	err = c2.BindSlots(backends.MessageSource, backends.StateStore, nil, nil)
	require.NoError(t, err, "Failed to bind slots for second consumer")

	secondProcessed := 0
	c2.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		secondProcessed++
		return nil
	})

	err = c2.Start(ctx)
	require.NoError(t, err, "Failed to start second consumer")
	defer c2.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Second consumer should resume from where first left off
	// Verify it has the state from the first consumer
	stateKey := "consumer:recovery-group:recovery-topic:test-recovery-consumer"
	stateData, found, err := backends.StateStore.Get(stateKey)
	require.NoError(t, err, "Failed to get state")
	assert.True(t, found, "State should exist from first consumer")
	assert.NotEmpty(t, stateData, "State should have data")
}
