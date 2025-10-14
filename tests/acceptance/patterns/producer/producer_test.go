package producer_test

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/producer"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends" // Register all backends
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProducerPattern tests the producer pattern with various backend combinations
func TestProducerPattern(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name:               "BasicPublish",
			Func:               testBasicPublish,
			RequiresCapability: "",
		},
		{
			Name:               "BatchedPublish",
			Func:               testBatchedPublish,
			RequiresCapability: "",
		},
		{
			Name:               "PublishWithRetry",
			Func:               testPublishWithRetry,
			RequiresCapability: "",
		},
		{
			Name:               "Deduplication",
			Func:               testDeduplication,
			RequiresCapability: "",
		},
		{
			Name:               "ProducerMetrics",
			Func:               testProducerMetrics,
			RequiresCapability: "",
		},
	}

	framework.RunPatternTests(t, framework.Pattern("Producer"), tests)
}

// testBasicPublish tests basic message publishing
func testBasicPublish(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ProducerBackends)

	config := producer.Config{
		Name:        "test-basic-producer",
		Description: "Test basic publishing",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "100ms",
			BatchSize:    0, // No batching
		},
	}

	prod, err := producer.New(config)
	require.NoError(t, err, "Failed to create producer")

	err = prod.BindSlots(backends.MessageSink, nil)
	require.NoError(t, err, "Failed to bind slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")
	defer prod.Stop(ctx)

	// Publish test message
	testPayload := []byte("test message")
	err = prod.Publish(ctx, "test-topic", testPayload, map[string]string{"key": "value"})
	require.NoError(t, err, "Failed to publish message")

	// Verify metrics
	metrics := prod.Metrics()
	assert.Equal(t, int64(1), metrics.MessagesPublished, "Should have published 1 message")
	assert.Equal(t, int64(0), metrics.MessagesFailed, "Should have 0 failed messages")

	// Verify health
	health, err := prod.Health(ctx)
	require.NoError(t, err, "Health check failed")
	assert.Equal(t, plugin.HealthHealthy, health.Status, "Producer should be healthy")
}

// testBatchedPublish tests batched message publishing
func testBatchedPublish(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ProducerBackends)

	config := producer.Config{
		Name: "test-batch-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:    3,
			RetryBackoff:  "50ms",
			BatchSize:     5,
			BatchInterval: "1s",
		},
	}

	prod, err := producer.New(config)
	require.NoError(t, err, "Failed to create producer")

	err = prod.BindSlots(backends.MessageSink, nil)
	require.NoError(t, err, "Failed to bind slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")
	defer prod.Stop(ctx)

	// Publish 12 messages (should create 2 full batches + 2 in partial batch)
	for i := 0; i < 12; i++ {
		err = prod.Publish(ctx, "batch-topic", []byte("batch message"), nil)
		require.NoError(t, err, "Failed to publish message %d", i)
	}

	// Wait for batch interval to flush remaining messages
	time.Sleep(1500 * time.Millisecond)

	// Verify all messages were published
	metrics := prod.Metrics()
	assert.Equal(t, int64(12), metrics.MessagesPublished, "Should have published 12 messages")
	assert.GreaterOrEqual(t, metrics.BatchesPublished, int64(3), "Should have published at least 3 batches")
}

// testPublishWithRetry tests producer retry behavior
func testPublishWithRetry(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ProducerBackends)

	config := producer.Config{
		Name: "test-retry-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "10ms",
			BatchSize:    0,
		},
	}

	prod, err := producer.New(config)
	require.NoError(t, err, "Failed to create producer")

	err = prod.BindSlots(backends.MessageSink, nil)
	require.NoError(t, err, "Failed to bind slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")
	defer prod.Stop(ctx)

	// Publish message - should succeed even with retries configured
	err = prod.Publish(ctx, "retry-topic", []byte("test"), nil)
	require.NoError(t, err, "Publish should succeed")

	metrics := prod.Metrics()
	assert.Equal(t, int64(1), metrics.MessagesPublished, "Should have published 1 message")
}

// testDeduplication tests message deduplication
func testDeduplication(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ProducerBackends)

	if backends.StateStore == nil {
		t.Skip("State store required for deduplication test")
	}

	config := producer.Config{
		Name: "test-dedup-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:                  3,
			RetryBackoff:                "50ms",
			BatchSize:                   0,
			Deduplication:               true,
			DeduplicationWindowDuration: "5m",
		},
	}

	prod, err := producer.New(config)
	require.NoError(t, err, "Failed to create producer")

	err = prod.BindSlots(backends.MessageSink, backends.StateStore)
	require.NoError(t, err, "Failed to bind slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")
	defer prod.Stop(ctx)

	// Publish same message 5 times
	testPayload := []byte("duplicate test")
	for i := 0; i < 5; i++ {
		err = prod.Publish(ctx, "dedup-topic", testPayload, nil)
		require.NoError(t, err, "Publish should not fail")
	}

	// Only first message should be published, rest deduplicated
	metrics := prod.Metrics()
	assert.Equal(t, int64(1), metrics.MessagesPublished, "Only 1 message should be published")
	assert.Equal(t, int64(4), metrics.MessagesDedup, "4 messages should be deduplicated")
}

// testProducerMetrics tests producer metrics tracking
func testProducerMetrics(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ProducerBackends)

	config := producer.Config{
		Name: "test-metrics-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "50ms",
			BatchSize:    0,
		},
	}

	prod, err := producer.New(config)
	require.NoError(t, err, "Failed to create producer")

	err = prod.BindSlots(backends.MessageSink, nil)
	require.NoError(t, err, "Failed to bind slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")
	defer prod.Stop(ctx)

	// Publish multiple messages of varying sizes
	messages := [][]byte{
		[]byte("small"),
		[]byte("medium sized message here"),
		[]byte("a very long message with lots of content to make it bigger"),
	}

	totalBytes := 0
	for _, msg := range messages {
		err = prod.Publish(ctx, "metrics-topic", msg, nil)
		require.NoError(t, err, "Failed to publish message")
		totalBytes += len(msg)
	}

	// Verify metrics
	metrics := prod.Metrics()
	assert.Equal(t, int64(len(messages)), metrics.MessagesPublished, "Published message count mismatch")
	assert.Equal(t, int64(totalBytes), metrics.BytesPublished, "Published bytes count mismatch")
	assert.Equal(t, int64(0), metrics.MessagesFailed, "Should have no failed messages")
	assert.NotZero(t, metrics.LastPublishTime, "Last publish time should be set")
}
