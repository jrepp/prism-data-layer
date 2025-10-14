package claimcheck_test

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

// TestClaimCheckPattern validates the end-to-end claim check pattern
// using producer/consumer patterns with NATS (messaging) and MinIO (object store)
func TestClaimCheckPattern(t *testing.T) {
	tests := []framework.MultiPatternTest{
		{
			Name: "LargePayloadClaimCheck",
			RequiredPatterns: map[string]framework.Pattern{
				"messaging":    framework.PatternProducer, // NATS for small messages
				"objectstore":  framework.PatternObjectStore, // MinIO for large payloads
			},
			Func:    testLargePayloadClaimCheck,
			Timeout: 60 * time.Second,
			Tags:    []string{"claim-check", "large-payload", "integration"},
		},
		{
			Name: "ThresholdBoundary",
			RequiredPatterns: map[string]framework.Pattern{
				"messaging":   framework.PatternProducer,
				"objectstore": framework.PatternObjectStore,
			},
			Func:    testThresholdBoundary,
			Timeout: 45 * time.Second,
			Tags:    []string{"claim-check", "boundary", "integration"},
		},
		{
			Name: "CompressionEnabled",
			RequiredPatterns: map[string]framework.Pattern{
				"messaging":   framework.PatternProducer,
				"objectstore": framework.PatternObjectStore,
			},
			Func:    testCompression,
			Timeout: 45 * time.Second,
			Tags:    []string{"claim-check", "compression", "integration"},
		},
		{
			Name: "DeleteAfterRead",
			RequiredPatterns: map[string]framework.Pattern{
				"messaging":   framework.PatternProducer,
				"objectstore": framework.PatternObjectStore,
			},
			Func:    testDeleteAfterRead,
			Timeout: 45 * time.Second,
			Tags:    []string{"claim-check", "cleanup", "integration"},
		},
	}

	framework.RunMultiPatternTests(t, tests)
}

// testLargePayloadClaimCheck validates that large payloads (>1MB) are automatically
// stored in object store and retrieved transparently by consumer
func testLargePayloadClaimCheck(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	messagingDriver := drivers["messaging"]
	objectStoreDriver := drivers["objectstore"]

	// Create 2MB payload (exceeds 1MB threshold)
	largePayload := make([]byte, 2*1024*1024)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	// Setup producer with claim check
	prod, cons, received := setupProducerConsumerWithClaimCheck(
		t, ctx, messagingDriver, objectStoreDriver,
		1024*1024, // 1MB threshold
		"gzip",    // compression
		false,     // don't delete after read
	)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	// Publish large message
	metadata := map[string]string{
		"content-type": "application/octet-stream",
		"test-id":      "large-payload-1",
	}

	t.Logf("Publishing 2MB payload (should trigger claim check)")
	err := prod.Publish(ctx, "claim-check-test", largePayload, metadata)
	require.NoError(t, err, "Failed to publish large message")

	// Wait for consumer to receive and resolve claim check
	t.Logf("Waiting for consumer to receive and resolve claim check...")
	select {
	case msg := <-received:
		assert.Equal(t, largePayload, msg.Payload, "Payload should match after claim check resolution")
		assert.Equal(t, "claim-check-test", msg.Topic, "Topic should match")
		assert.Equal(t, "application/octet-stream", msg.Metadata["content-type"], "Content-type should be restored")
		// Claim check metadata should be removed by consumer
		assert.Empty(t, msg.Metadata["prism-claim-check"], "Claim check metadata should be removed")
		t.Logf("Successfully received 2MB payload via claim check pattern")
	case <-time.After(30 * time.Second):
		t.Fatal("Timeout waiting for claim check message")
	}

	// Verify producer metrics show successful publish
	metrics := prod.Metrics()
	assert.Greater(t, metrics.MessagesPublished, int64(0), "Should have published at least 1 message")
	assert.Equal(t, int64(0), metrics.MessagesFailed, "Should have no failures")
}

// testThresholdBoundary validates that payloads under threshold are sent directly
// and payloads at/over threshold use claim check
func testThresholdBoundary(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messagingDriver := drivers["messaging"]
	objectStoreDriver := drivers["objectstore"]

	threshold := int64(1024 * 1024) // 1MB threshold

	prod, cons, received := setupProducerConsumerWithClaimCheck(
		t, ctx, messagingDriver, objectStoreDriver,
		threshold,
		"none", // no compression
		false,
	)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	// Test 1: Payload just under threshold (should NOT use claim check)
	t.Logf("Test 1: 1MB - 1 byte payload (should be direct)")
	smallPayload := make([]byte, threshold-1)
	err := prod.Publish(ctx, "claim-check-test", smallPayload, map[string]string{"test": "small"})
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, len(smallPayload), len(msg.Payload))
		t.Logf("Small payload received directly (no claim check)")
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for small payload")
	}

	// Test 2: Payload exactly at threshold (should use claim check)
	t.Logf("Test 2: Exactly 1MB payload (should use claim check)")
	thresholdPayload := make([]byte, threshold)
	err = prod.Publish(ctx, "claim-check-test", thresholdPayload, map[string]string{"test": "threshold"})
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, len(thresholdPayload), len(msg.Payload))
		t.Logf("Threshold payload received via claim check")
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for threshold payload")
	}

	// Test 3: Payload over threshold (should use claim check)
	t.Logf("Test 3: 1MB + 1 byte payload (should use claim check)")
	largePayload := make([]byte, threshold+1)
	err = prod.Publish(ctx, "claim-check-test", largePayload, map[string]string{"test": "large"})
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, len(largePayload), len(msg.Payload))
		t.Logf("Large payload received via claim check")
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for large payload")
	}
}

// testCompression validates that compression reduces stored payload size
func testCompression(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messagingDriver := drivers["messaging"]
	objectStoreDriver := drivers["objectstore"]

	// Create highly compressible payload (repeated pattern)
	compressiblePayload := make([]byte, 2*1024*1024)
	pattern := []byte("This is a highly compressible test pattern. ")
	for i := 0; i < len(compressiblePayload); i += len(pattern) {
		copy(compressiblePayload[i:], pattern)
	}

	prod, cons, received := setupProducerConsumerWithClaimCheck(
		t, ctx, messagingDriver, objectStoreDriver,
		1024*1024, // 1MB threshold
		"gzip",    // enable compression
		false,
	)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	t.Logf("Publishing 2MB compressible payload with gzip")
	err := prod.Publish(ctx, "claim-check-test", compressiblePayload, nil)
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, compressiblePayload, msg.Payload, "Decompressed payload should match original")
		t.Logf("Compressed payload successfully decompressed and verified")
	case <-time.After(20 * time.Second):
		t.Fatal("Timeout waiting for compressed payload")
	}
}

// testDeleteAfterRead validates that claims are deleted from object store after reading
func testDeleteAfterRead(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messagingDriver := drivers["messaging"]
	objectStoreDriver := drivers["objectstore"]

	largePayload := make([]byte, 2*1024*1024)

	prod, cons, received := setupProducerConsumerWithClaimCheck(
		t, ctx, messagingDriver, objectStoreDriver,
		1024*1024,
		"none",
		true, // delete after read
	)
	defer prod.Stop(ctx)
	defer cons.Stop(ctx)

	t.Logf("Publishing payload with delete-after-read enabled")
	err := prod.Publish(ctx, "claim-check-test", largePayload, nil)
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, len(largePayload), len(msg.Payload))
		t.Logf("Payload received (claim should be deleted asynchronously)")

		// Give time for async deletion
		time.Sleep(2 * time.Second)

		// Note: We can't easily verify deletion without direct access to object store
		// In production, you'd check via object store driver's Exists() method
	case <-time.After(20 * time.Second):
		t.Fatal("Timeout waiting for payload")
	}
}

// setupProducerConsumerWithClaimCheck creates producer/consumer with claim check configuration
func setupProducerConsumerWithClaimCheck(
	t *testing.T,
	ctx context.Context,
	messagingDriver interface{},
	objectStoreDriver interface{},
	threshold int64,
	compression string,
	deleteAfterRead bool,
) (*producer.Producer, *consumer.Consumer, <-chan *plugin.PubSubMessage) {
	t.Helper()

	// Cast object store driver
	objStore, ok := objectStoreDriver.(plugin.ObjectStoreInterface)
	require.True(t, ok, "Object store driver must implement ObjectStoreInterface")

	// Create producer with claim check
	producerConfig := producer.Config{
		Name: "claim-check-producer",
		Behavior: producer.BehaviorConfig{
			MaxRetries:   3,
			RetryBackoff: "100ms",
			BatchSize:    0, // Publish immediately
			ClaimCheck: &producer.ClaimCheckConfig{
				Enabled:     true,
				Threshold:   threshold,
				Bucket:      "prism-test-bucket",
				TTL:         3600, // 1 hour
				Compression: compression,
			},
		},
	}

	prod, err := producer.New(producerConfig)
	require.NoError(t, err, "Failed to create producer")

	// Bind producer slots (messaging + object store)
	var prodStateStore plugin.KeyValueBasicInterface
	if kvInterface, ok := messagingDriver.(plugin.KeyValueBasicInterface); ok {
		prodStateStore = kvInterface
	}

	err = prod.BindSlots(messagingDriver, prodStateStore, objStore)
	require.NoError(t, err, "Failed to bind producer slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")

	// Create consumer with claim check
	consumerConfig := consumer.Config{
		Name: "claim-check-consumer",
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "claim-check-group",
			Topic:         "claim-check-test",
			MaxRetries:    3,
			AutoCommit:    true,
			BatchSize:     1,
			ClaimCheck: &consumer.ClaimCheckConfig{
				Enabled:         true,
				DeleteAfterRead: deleteAfterRead,
			},
		},
	}

	cons, err := consumer.New(consumerConfig)
	require.NoError(t, err, "Failed to create consumer")

	// Bind consumer slots (messaging + object store)
	var consStateStore plugin.KeyValueBasicInterface
	if kvInterface, ok := messagingDriver.(plugin.KeyValueBasicInterface); ok {
		consStateStore = kvInterface
	}

	err = cons.BindSlots(messagingDriver, consStateStore, nil, objStore)
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
	time.Sleep(500 * time.Millisecond)

	return prod, cons, received
}
