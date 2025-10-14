package kafka_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/drivers/kafka"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/common"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	kafkaBackend *backends.KafkaBackend
	testCtx      context.Context
)

// TestMain sets up the Kafka container once for all tests
func TestMain(m *testing.M) {
	testCtx = context.Background()

	// Start Kafka container once
	kafkaBackend = backends.SetupKafka(&testing.T{}, testCtx)

	// Run all tests
	code := m.Run()

	// Cleanup after all tests
	kafkaBackend.Cleanup()

	os.Exit(code)
}

func TestKafkaPattern_PubSubBasic(t *testing.T) {
	// Create Kafka pattern
	kafkaPlugin := kafka.New()

	// Configure with shared testcontainer
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "kafka-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"brokers":           kafkaBackend.Brokers,
			"consumer_group":    "test-group",
			"auto_offset_reset": "earliest",
		},
	}

	// Create test harness
	harness := common.NewPatternHarness(t, kafkaPlugin, config)
	defer harness.Cleanup()

	// Wait for plugin to be healthy
	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err, "Plugin did not become healthy")

	t.Run("Publish and Subscribe", func(t *testing.T) {
		topic := fmt.Sprintf("test.%s.topic", t.Name())
		subscriberID := fmt.Sprintf("%s-subscriber-1", t.Name())
		payload := []byte("test message from Kafka")
		metadata := map[string]string{
			"source": "integration-test",
			"type":   "test-event",
		}

		// Subscribe
		msgChan, err := kafkaPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)

		// Wait for subscription to be established (Kafka consumer needs time to join group)
		time.Sleep(2 * time.Second)

		// Publish
		messageID, err := kafkaPlugin.Publish(harness.Context(), topic, payload, metadata)
		require.NoError(t, err)
		assert.NotEmpty(t, messageID)

		// Receive message
		select {
		case msg := <-msgChan:
			assert.Equal(t, topic, msg.Topic)
			assert.Equal(t, payload, msg.Payload)
			assert.NotEmpty(t, msg.MessageID)
			assert.NotZero(t, msg.Timestamp)
			// Check metadata
			assert.Equal(t, "integration-test", msg.Metadata["source"])
			assert.Equal(t, "test-event", msg.Metadata["type"])
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for message")
		}

		// Unsubscribe
		err = kafkaPlugin.Unsubscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)
	})

	t.Run("Multiple Messages In Order", func(t *testing.T) {
		topic := fmt.Sprintf("test.%s.multi", t.Name())
		subscriberID := fmt.Sprintf("%s-subscriber-2", t.Name())

		msgChan, err := kafkaPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)
		defer kafkaPlugin.Unsubscribe(harness.Context(), topic, subscriberID)

		time.Sleep(2 * time.Second)

		// Publish 10 messages
		const numMessages = 10
		for i := 0; i < numMessages; i++ {
			payload := []byte(fmt.Sprintf("message-%03d", i))
			metadata := map[string]string{"sequence": fmt.Sprintf("%d", i)}
			_, err := kafkaPlugin.Publish(harness.Context(), topic, payload, metadata)
			require.NoError(t, err)
		}

		// Receive all messages in order (Kafka preserves order within a partition)
		received := make([]string, 0, numMessages)
		for i := 0; i < numMessages; i++ {
			select {
			case msg := <-msgChan:
				received = append(received, string(msg.Payload))
				assert.Equal(t, fmt.Sprintf("%d", i), msg.Metadata["sequence"])
			case <-time.After(10 * time.Second):
				t.Fatalf("Timeout waiting for message %d (received %d messages)", i, len(received))
			}
		}

		// Verify order
		assert.Equal(t, numMessages, len(received))
		for i := 0; i < numMessages; i++ {
			expected := fmt.Sprintf("message-%03d", i)
			assert.Equal(t, expected, received[i], "Message out of order at position %d", i)
		}
	})

	t.Run("Unsubscribe Stops Messages", func(t *testing.T) {
		topic := fmt.Sprintf("test.%s.unsub", t.Name())
		subscriberID := fmt.Sprintf("%s-subscriber-3", t.Name())

		msgChan, err := kafkaPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)

		time.Sleep(2 * time.Second)

		// Publish first message
		_, err = kafkaPlugin.Publish(harness.Context(), topic, []byte("message-1"), nil)
		require.NoError(t, err)

		// Receive first message
		select {
		case msg := <-msgChan:
			assert.Equal(t, []byte("message-1"), msg.Payload)
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for first message")
		}

		// Unsubscribe
		err = kafkaPlugin.Unsubscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		// Publish second message after unsubscribe
		_, err = kafkaPlugin.Publish(harness.Context(), topic, []byte("message-2"), nil)
		require.NoError(t, err)

		// Should NOT receive second message (channel should be closed)
		select {
		case msg, ok := <-msgChan:
			if ok {
				t.Errorf("Should not receive message after unsubscribe, got: %s", string(msg.Payload))
			}
			// Channel closed is expected
		case <-time.After(2 * time.Second):
			// Expected: timeout means no message received
			t.Log("Correctly did not receive message after unsubscribe")
		}
	})
}

func TestKafkaPattern_QueueOperations(t *testing.T) {
	kafkaPlugin := kafka.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "kafka-queue-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"brokers":           kafkaBackend.Brokers,
			"consumer_group":    "queue-test-group",
			"auto_offset_reset": "earliest",
		},
	}

	harness := common.NewPatternHarness(t, kafkaPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Enqueue and Receive", func(t *testing.T) {
		queue := fmt.Sprintf("test.%s.queue", t.Name())
		payload := []byte("queued message")
		metadata := map[string]string{"priority": "high"}

		// Start receiver first
		msgChan, err := kafkaPlugin.Receive(harness.Context(), queue)
		require.NoError(t, err)

		time.Sleep(2 * time.Second)

		// Enqueue message
		messageID, err := kafkaPlugin.Enqueue(harness.Context(), queue, payload, metadata)
		require.NoError(t, err)
		assert.NotEmpty(t, messageID)

		// Receive message
		select {
		case msg := <-msgChan:
			assert.Equal(t, queue, msg.Topic)
			assert.Equal(t, payload, msg.Payload)
			assert.Equal(t, "high", msg.Metadata["priority"])
			assert.NotEmpty(t, msg.MessageID)

			// Acknowledge message
			err = kafkaPlugin.Acknowledge(harness.Context(), queue, msg.MessageID)
			assert.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for queued message")
		}
	})

	t.Run("Load Balancing Across Consumers", func(t *testing.T) {
		queue := fmt.Sprintf("test.%s.loadbalance", t.Name())

		// Start 3 consumers (load balancing via consumer group)
		const numConsumers = 3
		consumers := make([]<-chan *plugin.PubSubMessage, numConsumers)
		messageCount := make([]int, numConsumers)
		var mu sync.Mutex

		for i := 0; i < numConsumers; i++ {
			msgChan, err := kafkaPlugin.Receive(harness.Context(), queue)
			require.NoError(t, err)
			consumers[i] = msgChan
		}

		time.Sleep(3 * time.Second) // Wait for all consumers to join group

		// Publish 30 messages
		const numMessages = 30
		for i := 0; i < numMessages; i++ {
			payload := []byte(fmt.Sprintf("loadbalance-message-%d", i))
			_, err := kafkaPlugin.Enqueue(harness.Context(), queue, payload, nil)
			require.NoError(t, err)
		}

		// Collect messages from all consumers
		var wg sync.WaitGroup
		wg.Add(numConsumers)

		for consumerID, ch := range consumers {
			go func(id int, msgChan <-chan *plugin.PubSubMessage) {
				defer wg.Done()
				timeout := time.After(15 * time.Second)
				for {
					select {
					case _, ok := <-msgChan:
						if !ok {
							return
						}
						mu.Lock()
						messageCount[id]++
						mu.Unlock()
						// Check if we've received all messages
						mu.Lock()
						total := 0
						for _, count := range messageCount {
							total += count
						}
						mu.Unlock()
						if total >= numMessages {
							return
						}
					case <-timeout:
						return
					}
				}
			}(consumerID, ch)
		}

		wg.Wait()

		// Verify total messages received
		total := 0
		for i, count := range messageCount {
			t.Logf("Consumer %d received %d messages", i, count)
			total += count
		}
		assert.Equal(t, numMessages, total, "Not all messages were received")

		// Verify load distribution (each consumer should get some messages)
		for i, count := range messageCount {
			assert.Greater(t, count, 0, "Consumer %d received no messages", i)
		}
	})
}

func TestKafkaPattern_ConcurrentPublish(t *testing.T) {
	kafkaPlugin := kafka.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "kafka-concurrent",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"brokers":           kafkaBackend.Brokers,
			"consumer_group":    "concurrent-test-group",
			"auto_offset_reset": "earliest",
		},
	}

	harness := common.NewPatternHarness(t, kafkaPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Multiple Publishers", func(t *testing.T) {
		topic := fmt.Sprintf("test.%s.concurrent", t.Name())

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
					metadata := map[string]string{
						"publisher": fmt.Sprintf("%d", publisherID),
						"sequence":  fmt.Sprintf("%d", i),
					}
					_, err := kafkaPlugin.Publish(harness.Context(), topic, payload, metadata)
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

func TestKafkaPattern_HealthCheck(t *testing.T) {
	kafkaPlugin := kafka.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "kafka-health",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"brokers":           kafkaBackend.Brokers,
			"consumer_group":    "health-test-group",
			"auto_offset_reset": "earliest",
		},
	}

	harness := common.NewPatternHarness(t, kafkaPlugin, config)
	defer harness.Cleanup()

	t.Run("Healthy Status", func(t *testing.T) {
		status, err := kafkaPlugin.Health(harness.Context())
		require.NoError(t, err)
		assert.Equal(t, plugin.HealthHealthy, status.Status)
		assert.NotEmpty(t, status.Message)
		assert.NotEmpty(t, status.Details)

		// Check for expected details
		assert.Contains(t, status.Details, "producer_queue")
		assert.Contains(t, status.Details, "topic")
	})
}

func TestKafkaPattern_BinaryData(t *testing.T) {
	kafkaPlugin := kafka.New()
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "kafka-binary",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"brokers":           kafkaBackend.Brokers,
			"consumer_group":    "binary-test-group",
			"auto_offset_reset": "earliest",
		},
	}

	harness := common.NewPatternHarness(t, kafkaPlugin, config)
	defer harness.Cleanup()

	err := harness.WaitForHealthy(10 * time.Second)
	require.NoError(t, err)

	t.Run("Binary Payload", func(t *testing.T) {
		topic := fmt.Sprintf("test.%s.binary", t.Name())
		subscriberID := fmt.Sprintf("%s-binary-subscriber", t.Name())

		// Binary data with all byte values
		binaryData := make([]byte, 256)
		for i := range binaryData {
			binaryData[i] = byte(i)
		}

		msgChan, err := kafkaPlugin.Subscribe(harness.Context(), topic, subscriberID)
		require.NoError(t, err)
		defer kafkaPlugin.Unsubscribe(harness.Context(), topic, subscriberID)

		time.Sleep(2 * time.Second)

		// Publish binary data
		_, err = kafkaPlugin.Publish(harness.Context(), topic, binaryData, nil)
		require.NoError(t, err)

		// Receive and verify
		select {
		case msg := <-msgChan:
			assert.Equal(t, binaryData, msg.Payload, "Binary data corrupted")
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for binary message")
		}
	})
}
