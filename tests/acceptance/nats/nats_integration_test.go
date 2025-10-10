package nats_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/patterns/nats"
	"github.com/jrepp/prism-data-layer/tests/acceptance/common"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNATSPattern_BasicPubSub(t *testing.T) {
	ctx := context.Background()

	// Start NATS container using centralized backend utility
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	// Create NATS pattern
	natsPlugin := nats.New()

	// Configure with testcontainer
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	// Create test harness
	harness := common.NewPatternHarness(t, natsPlugin, config)
	defer harness.Cleanup()

	// Wait for plugin to be healthy
	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err, "Plugin did not become healthy")

	t.Run("Publish and Subscribe", func(t *testing.T) {
		topic := "test.topic"
		subscriberID := "subscriber-1"
		payload := []byte("test message")

		// Subscribe
		msgChan, err := natsPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)

		// Wait for subscription to be established
		time.Sleep(100 * time.Millisecond)

		// Publish
		messageID, err := natsPlugin.Publish(harness.Context(), topic, payload, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, messageID)

		// Receive message
		select {
		case msg := <-msgChan:
			assert.Equal(t, topic, msg.Topic)
			assert.Equal(t, payload, msg.Payload)
			assert.NotEmpty(t, msg.MessageID)
			assert.NotZero(t, msg.Timestamp)
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for message")
		}

		// Unsubscribe
		err = natsPlugin.Unsubscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)
	})

	t.Run("Multiple Messages", func(t *testing.T) {
		topic := "test.multi"
		subscriberID := "subscriber-2"

		msgChan, err := natsPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)
		defer natsPlugin.Unsubscribe(harness.Context(), topic, subscriberID)

		time.Sleep(100 * time.Millisecond)

		// Publish 5 messages
		const numMessages = 5
		for i := 0; i < numMessages; i++ {
			payload := []byte(fmt.Sprintf("message-%d", i))
			_, err := natsPlugin.Publish(harness.Context(), topic, payload, nil)
			require.NoError(t, err)
		}

		// Receive all messages
		received := make(map[string]bool)
		for i := 0; i < numMessages; i++ {
			select {
			case msg := <-msgChan:
				received[string(msg.Payload)] = true
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for message %d", i)
			}
		}

		// Verify all messages received
		assert.Equal(t, numMessages, len(received))
		for i := 0; i < numMessages; i++ {
			expected := fmt.Sprintf("message-%d", i)
			assert.True(t, received[expected], "Missing message: %s", expected)
		}
	})

	t.Run("Unsubscribe Stops Messages", func(t *testing.T) {
		topic := "test.unsub"
		subscriberID := "subscriber-3"

		msgChan, err := natsPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Publish first message
		_, err = natsPlugin.Publish(harness.Context(), topic, []byte("message-1"), nil)
		require.NoError(t, err)

		// Receive first message
		select {
		case msg := <-msgChan:
			assert.Equal(t, []byte("message-1"), msg.Payload)
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for first message")
		}

		// Unsubscribe
		err = natsPlugin.Unsubscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Publish second message after unsubscribe
		_, err = natsPlugin.Publish(harness.Context(), topic, []byte("message-2"), nil)
		require.NoError(t, err)

		// Should NOT receive second message
		select {
		case msg := <-msgChan:
			t.Errorf("Should not receive message after unsubscribe, got: %s", string(msg.Payload))
		case <-time.After(500 * time.Millisecond):
			// Expected: timeout means no message received
			t.Log("Correctly did not receive message after unsubscribe")
		}
	})
}

func TestNATSPattern_Fanout(t *testing.T) {
	ctx := context.Background()

	// Start NATS container using centralized backend utility
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	natsPlugin := nats.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-fanout",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, natsPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err)

	t.Run("Multiple Subscribers Receive Same Message", func(t *testing.T) {
		topic := "test.fanout"
		payload := []byte("broadcast message")

		const numSubscribers = 5
		var channels []<-chan *nats.Message
		var subscriberIDs []string

		// Create subscribers
		for i := 0; i < numSubscribers; i++ {
			subscriberID := fmt.Sprintf("fanout-subscriber-%d", i)
			subscriberIDs = append(subscriberIDs, subscriberID)

			msgChan, err := natsPlugin.Subscribe(harness.Context(), topic, subscriberID)
			require.NoError(t, err)
			channels = append(channels, msgChan)
		}

		// Wait for all subscriptions
		time.Sleep(200 * time.Millisecond)

		// Publish one message
		_, err := natsPlugin.Publish(harness.Context(), topic, payload, nil)
		require.NoError(t, err)

		// All subscribers should receive it
		var wg sync.WaitGroup
		wg.Add(numSubscribers)

		for i, ch := range channels {
			go func(idx int, msgChan <-chan *nats.Message) {
				defer wg.Done()
				select {
				case msg := <-msgChan:
					assert.Equal(t, payload, msg.Payload, "Subscriber %d got wrong payload", idx)
					assert.Equal(t, topic, msg.Topic)
				case <-time.After(2 * time.Second):
					t.Errorf("Subscriber %d timeout", idx)
				}
			}(i, ch)
		}

		wg.Wait()

		// Cleanup subscribers
		for _, subID := range subscriberIDs {
			err := natsPlugin.Unsubscribe(harness.Context(), topic, subID)
			require.NoError(t, err)
		}
	})
}

func TestNATSPattern_MessageOrdering(t *testing.T) {
	ctx := context.Background()

	// Start NATS container using centralized backend utility
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	natsPlugin := nats.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-ordering",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, natsPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err)

	t.Run("Messages Delivered In Order", func(t *testing.T) {
		topic := "test.ordering"
		subscriberID := "order-subscriber"

		msgChan, err := natsPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)
		defer natsPlugin.Unsubscribe(harness.Context(), topic, subscriberID)

		time.Sleep(100 * time.Millisecond)

		// Publish messages in order
		const numMessages = 20
		for i := 0; i < numMessages; i++ {
			payload := []byte(fmt.Sprintf("message-%03d", i))
			_, err := natsPlugin.Publish(harness.Context(), topic, payload, nil)
			require.NoError(t, err)
			// Small delay to ensure ordering
			time.Sleep(10 * time.Millisecond)
		}

		// Verify order
		for i := 0; i < numMessages; i++ {
			expected := fmt.Sprintf("message-%03d", i)
			select {
			case msg := <-msgChan:
				assert.Equal(t, expected, string(msg.Payload), "Message out of order at position %d", i)
			case <-time.After(3 * time.Second):
				t.Fatalf("Timeout waiting for message %d", i)
			}
		}
	})
}

func TestNATSPattern_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()

	// Start NATS container using centralized backend utility
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	natsPlugin := nats.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-concurrent",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, natsPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err)

	t.Run("Multiple Publishers", func(t *testing.T) {
		topic := "test.concurrent"

		const numPublishers = 10
		const messagesPerPublisher = 10

		var wg sync.WaitGroup
		wg.Add(numPublishers)

		errors := make(chan error, numPublishers*messagesPerPublisher)

		// Launch publishers
		for p := 0; p < numPublishers; p++ {
			go func(publisherID int) {
				defer wg.Done()
				for i := 0; i < messagesPerPublisher; i++ {
					payload := []byte(fmt.Sprintf("publisher-%d-message-%d", publisherID, i))
					_, err := natsPlugin.Publish(harness.Context(), topic, payload, nil)
					if err != nil {
						errors <- err
					}
				}
			}(p)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Publish error: %v", err)
		}
	})
}

func TestNATSPattern_HealthCheck(t *testing.T) {
	ctx := context.Background()

	// Start NATS container using centralized backend utility
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	natsPlugin := nats.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-health",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, natsPlugin, config)
	defer harness.Cleanup()

	t.Run("Healthy Status", func(t *testing.T) {
		status, err := natsPlugin.Health(harness.Context())
		require.NoError(t, err)
		assert.Equal(t, core.HealthHealthy, status.Status)
		assert.NotEmpty(t, status.Message)
		assert.NotEmpty(t, status.Details)

		// Check for expected details
		assert.Contains(t, status.Details, "subscriptions")
		assert.Contains(t, status.Details, "in_msgs")
		assert.Contains(t, status.Details, "out_msgs")
	})
}

func TestNATSPattern_WildcardSubscriptions(t *testing.T) {
	ctx := context.Background()

	// Start NATS container using centralized backend utility
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	natsPlugin := nats.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-wildcard",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	harness := common.NewPatternHarness(t, natsPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err)

	t.Run("Wildcard Subscription", func(t *testing.T) {
		// Subscribe to wildcard topic
		wildcard := "events.*"
		subscriberID := "wildcard-sub"

		msgChan, err := natsPlugin.Subscribe(harness.Context(), wildcard, subscriberID)
		require.NoError(t, err)
		defer natsPlugin.Unsubscribe(harness.Context(), wildcard, subscriberID)

		time.Sleep(100 * time.Millisecond)

		// Publish to multiple topics that match wildcard
		topics := []string{"events.created", "events.updated", "events.deleted"}
		for _, topic := range topics {
			payload := []byte(fmt.Sprintf("payload for %s", topic))
			_, err := natsPlugin.Publish(harness.Context(), topic, payload, nil)
			require.NoError(t, err)
		}

		// Receive all messages
		received := make(map[string]bool)
		for i := 0; i < len(topics); i++ {
			select {
			case msg := <-msgChan:
				received[msg.Topic] = true
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for message %d", i)
			}
		}

		// Verify all topics received
		for _, topic := range topics {
			assert.True(t, received[topic], "Did not receive message for topic: %s", topic)
		}
	})
}
