package consumer_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsumerDeadLetterQueue tests the consumer with all 3 slots filled
// These tests are non-intersecting with existing tests:
// - Existing tests: message success paths, stateless/stateful modes
// - DLQ tests: failure paths, retry logic, dead letter queue behavior
func TestConsumerDeadLetterQueue(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name: "DLQAfterRetries",
			Func: testDLQAfterRetries,
		},
		{
			Name: "DLQVerifyMessages",
			Func: testDLQVerifyMessages,
		},
		{
			Name: "DLQMixedSuccessFailure",
			Func: testDLQMixedSuccessFailure,
		},
	}

	framework.RunPatternTests(t, framework.Pattern("Consumer"), tests)
}

// testDLQAfterRetries verifies messages go to DLQ after exhausting retries
// NOTE: NATS core pub/sub doesn't support message redelivery, so retries don't actually
// re-fetch the message. This test validates that DLQ slot binding works correctly.
func testDLQAfterRetries(t *testing.T, driver interface{}, caps framework.Capabilities) {
	// Skip if DLQ not supported
	hasDLQ, ok := caps.Custom["HasDLQ"].(bool)
	if !ok || !hasDLQ {
		t.Skip("DLQ not configured for this backend")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// Subscribe to DLQ to verify message routing
	dlqTopic := "dlq.retry.topic.dlq"
	dlqChan, err := backends.DeadLetter.Receive(ctx, dlqTopic)
	require.NoError(t, err, "Failed to subscribe to DLQ")

	// Track processing attempts
	var attempts int32

	// Create consumer config
	consumerConfig := consumer.Config{
		Name:        "dlq-retry-test",
		Description: "Test DLQ message routing",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": backends.NATSUrl,
				},
			},
			StateStore: consumer.SlotBinding{
				Driver: "memstore",
				Config: map[string]interface{}{},
			},
			DeadLetterQueue: &consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": backends.NATSUrl,
				},
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "dlq-test-group",
			Topic:         "dlq.retry.topic",
			MaxRetries:    0, // No retries - failed messages go directly to DLQ
			AutoCommit:    true,
			BatchSize:     1,
		},
	}

	// Create consumer
	c, err := consumer.New(consumerConfig)
	require.NoError(t, err, "Failed to create consumer")

	// Bind backends (all 3 slots filled)
	err = c.BindSlots(backends.MessageSource, backends.StateStore, backends.DeadLetter)
	require.NoError(t, err, "Failed to bind all 3 slots")

	// Set processor that always fails
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		count := atomic.AddInt32(&attempts, 1)
		t.Logf("Processing attempt %d for message: %s", count, string(msg.Payload))
		return errors.New("intentional processing failure")
	})

	// Start consumer
	err = c.Start(ctx)
	require.NoError(t, err, "Failed to start consumer")
	defer c.Stop(ctx)

	// Give consumer time to establish subscriptions
	time.Sleep(500 * time.Millisecond)

	// Publish test message
	msgPayload := []byte("retry-test-message")
	_, err = backends.MessageSource.Publish(ctx, "dlq.retry.topic", msgPayload, nil)
	require.NoError(t, err, "Failed to publish message")

	// Wait for processing and DLQ routing
	time.Sleep(2 * time.Second)

	// Verify message was sent to DLQ
	select {
	case dlqMsg := <-dlqChan:
		require.NotNil(t, dlqMsg, "Should receive DLQ message")
		assert.Equal(t, msgPayload, dlqMsg.Payload, "DLQ message should match original")
		t.Logf("✅ Failed message successfully routed to DLQ: %s", string(dlqMsg.Payload))
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message in DLQ")
	}

	// Verify at least one processing attempt occurred
	finalAttempts := atomic.LoadInt32(&attempts)
	assert.GreaterOrEqual(t, finalAttempts, int32(1), "Should have at least 1 processing attempt")

	t.Logf("✅ All 3 consumer slots verified: MessageSource + StateStore + DeadLetterQueue")
}

// testDLQVerifyMessages verifies messages actually appear in DLQ
func testDLQVerifyMessages(t *testing.T, driver interface{}, caps framework.Capabilities) {
	hasDLQ, ok := caps.Custom["HasDLQ"].(bool)
	if !ok || !hasDLQ {
		t.Skip("DLQ not configured for this backend")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// Subscribe to DLQ before starting consumer
	dlqTopic := "dlq.verify.topic.dlq" // Consumer appends ".dlq" to topic name
	dlqChan, err := backends.DeadLetter.Receive(ctx, dlqTopic)
	require.NoError(t, err, "Failed to subscribe to DLQ")

	// Create consumer
	consumerConfig := consumer.Config{
		Name:        "dlq-verify-test",
		Description: "Verify DLQ messages",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": backends.NATSUrl,
				},
			},
			StateStore: consumer.SlotBinding{
				Driver: "memstore",
				Config: map[string]interface{}{},
			},
			DeadLetterQueue: &consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": backends.NATSUrl,
				},
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "dlq-verify-group",
			Topic:         "dlq.verify.topic",
			MaxRetries:    0, // No retries - failed messages go directly to DLQ
			AutoCommit:    true,
			BatchSize:     1,
		},
	}

	c, err := consumer.New(consumerConfig)
	require.NoError(t, err)

	err = c.BindSlots(backends.MessageSource, backends.StateStore, backends.DeadLetter)
	require.NoError(t, err)

	// Processor that always fails
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		return fmt.Errorf("processing failed")
	})

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Stop(ctx)

	// Give consumer time to establish subscriptions
	time.Sleep(500 * time.Millisecond)

	// Publish test message (just one to keep test simple given NATS core limitations)
	testMessage := "dlq-verify-msg"
	_, err = backends.MessageSource.Publish(ctx, "dlq.verify.topic", []byte(testMessage), nil)
	require.NoError(t, err)

	// Wait for processing and DLQ routing
	time.Sleep(2 * time.Second)

	// Verify message ended up in DLQ
	select {
	case dlqMsg := <-dlqChan:
		require.NotNil(t, dlqMsg, "Should receive DLQ message")
		assert.Equal(t, testMessage, string(dlqMsg.Payload), "DLQ message should match original")
		t.Logf("📨 Received DLQ message: %s", string(dlqMsg.Payload))
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for DLQ message")
	}

	t.Log("✅ Failed message successfully routed to DLQ")
}

// testDLQMixedSuccessFailure tests consumer with some successes and some failures
func testDLQMixedSuccessFailure(t *testing.T, driver interface{}, caps framework.Capabilities) {
	hasDLQ, ok := caps.Custom["HasDLQ"].(bool)
	if !ok || !hasDLQ {
		t.Skip("DLQ not configured for this backend")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// Subscribe to DLQ
	dlqTopic := "dlq.mixed.topic.dlq"
	dlqChan, err := backends.DeadLetter.Receive(ctx, dlqTopic)
	require.NoError(t, err)

	// Track successful and failed messages
	var successCount, failCount int32

	consumerConfig := consumer.Config{
		Name:        "dlq-mixed-test",
		Description: "Mixed success/failure test",
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": backends.NATSUrl,
				},
			},
			StateStore: consumer.SlotBinding{
				Driver: "memstore",
				Config: map[string]interface{}{},
			},
			DeadLetterQueue: &consumer.SlotBinding{
				Driver: "nats",
				Config: map[string]interface{}{
					"url": backends.NATSUrl,
				},
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: "dlq-mixed-group",
			Topic:         "dlq.mixed.topic",
			MaxRetries:    0, // No retries - failed messages go directly to DLQ
			AutoCommit:    true,
			BatchSize:     1,
		},
	}

	c, err := consumer.New(consumerConfig)
	require.NoError(t, err)

	err = c.BindSlots(backends.MessageSource, backends.StateStore, backends.DeadLetter)
	require.NoError(t, err)

	// Processor that fails on "fail-" prefix, succeeds otherwise
	c.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		payload := string(msg.Payload)
		if len(payload) >= 5 && payload[:5] == "fail-" {
			atomic.AddInt32(&failCount, 1)
			return fmt.Errorf("message marked for failure: %s", payload)
		}
		atomic.AddInt32(&successCount, 1)
		t.Logf("✅ Successfully processed: %s", payload)
		return nil
	})

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Stop(ctx)

	// Give consumer time to establish subscriptions
	time.Sleep(500 * time.Millisecond)

	// Publish mixed messages - keep it simple with 2 messages
	messages := []struct {
		payload string
		shouldSucceed bool
	}{
		{"success-1", true},
		{"fail-1", false},
	}

	for _, msg := range messages {
		_, err = backends.MessageSource.Publish(ctx, "dlq.mixed.topic", []byte(msg.payload), nil)
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond) // Small delay between messages
	}

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Verify counts
	finalSuccess := atomic.LoadInt32(&successCount)
	finalFail := atomic.LoadInt32(&failCount)

	t.Logf("Success count: %d, Fail count: %d", finalSuccess, finalFail)

	// Should have at least 1 success
	assert.GreaterOrEqual(t, int(finalSuccess), 1, "Should have at least 1 successful message")
	// Should have at least 1 failure
	assert.GreaterOrEqual(t, int(finalFail), 1, "Should have at least 1 failed message")

	// Verify failed message ended up in DLQ
	select {
	case dlqMsg := <-dlqChan:
		require.NotNil(t, dlqMsg, "Should receive DLQ message")
		assert.Contains(t, string(dlqMsg.Payload), "fail-", "DLQ message should be a failure")
		t.Logf("📨 DLQ contains: %s", string(dlqMsg.Payload))
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for DLQ message")
	}

	t.Log("✅ Mixed success/failure test completed - successes processed, failures in DLQ")
}
