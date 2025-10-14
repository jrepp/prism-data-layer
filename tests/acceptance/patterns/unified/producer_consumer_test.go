package unified_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/patterns/producer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// TestProducerConsumerIntegration tests end-to-end message flow from producer to consumer
func TestProducerConsumerIntegration(t *testing.T) {
	tests := []struct {
		name           string
		setupBackend   func(t *testing.T) (messageBus interface{}, stateStore plugin.KeyValueBasicInterface, cleanup func())
		skipCondition  func(t *testing.T) bool
	}{
		{
			name:         "MemStore",
			setupBackend: setupMemStoreBackend,
		},
		{
			name:         "NATS+MemStore",
			setupBackend: setupNATSBackend,
			skipCondition: func(t *testing.T) bool {
				// Skip if DOCKER_HOST not set (testcontainers requirement)
				return testcontainers.IsProviderReachable(context.Background()) != nil
			},
		},
		{
			name:         "Redis",
			setupBackend: setupRedisBackend,
			skipCondition: func(t *testing.T) bool {
				return testcontainers.IsProviderReachable(context.Background()) != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipCondition != nil && tt.skipCondition(t) {
				t.Skip("Skipping test: backend not available")
			}

			runProducerConsumerTest(t, tt.setupBackend)
		})
	}
}

func runProducerConsumerTest(
	t *testing.T,
	setupBackend func(t *testing.T) (messageBus interface{}, stateStore plugin.KeyValueBasicInterface, cleanup func()),
) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Setup backend
	messageBus, stateStore, cleanup := setupBackend(t)
	defer cleanup()

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

	err = prod.BindSlots(messageBus, stateStore)
	require.NoError(t, err, "Failed to bind producer slots")

	err = prod.Start(ctx)
	require.NoError(t, err, "Failed to start producer")
	defer prod.Stop(ctx)

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

	err = cons.BindSlots(messageBus, stateStore, nil)
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
	defer cons.Stop(ctx)

	// Give consumer time to subscribe
	time.Sleep(200 * time.Millisecond)

	// Test 1: Publish single message
	t.Run("SingleMessage", func(t *testing.T) {
		testPayload := []byte(`{"order_id":"123","amount":99.99}`)
		metadata := map[string]string{
			"content-type": "application/json",
			"version":      "v1",
		}

		err = prod.Publish(ctx, "unified-test-topic", testPayload, metadata)
		require.NoError(t, err, "Failed to publish message")

		// Wait for consumer to receive
		select {
		case msg := <-received:
			assert.Equal(t, testPayload, msg.Payload, "Payload mismatch")
			assert.Equal(t, "unified-test-topic", msg.Topic, "Topic mismatch")
			assert.Equal(t, "application/json", msg.Metadata["content-type"], "Metadata mismatch")
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for message")
		}
	})

	// Test 2: Publish multiple messages
	t.Run("MultipleMessages", func(t *testing.T) {
		messageCount := 5
		for i := 0; i < messageCount; i++ {
			payload := []byte(`{"message_number":` + string(rune(i+'0')) + `}`)
			err = prod.Publish(ctx, "unified-test-topic", payload, nil)
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
	})

	// Test 3: Verify metrics
	t.Run("Metrics", func(t *testing.T) {
		prodMetrics := prod.Metrics()
		assert.GreaterOrEqual(t, prodMetrics.MessagesPublished, int64(6), "Producer should have published at least 6 messages")
		assert.Equal(t, int64(0), prodMetrics.MessagesFailed, "Producer should have no failures")

		consHealth, err := cons.Health(ctx)
		require.NoError(t, err, "Consumer health check failed")
		assert.Equal(t, plugin.HealthHealthy, consHealth.Status, "Consumer should be healthy")

		prodHealth, err := prod.Health(ctx)
		require.NoError(t, err, "Producer health check failed")
		assert.Equal(t, plugin.HealthHealthy, prodHealth.Status, "Producer should be healthy")
	})

	// Test 4: State persistence (if state store available)
	if stateStore != nil {
		t.Run("StatePersistence", func(t *testing.T) {
			// Check consumer state was persisted
			stateKey := "consumer:unified-test-group:unified-test-topic:unified-test-consumer"
			stateData, found, err := stateStore.Get(stateKey)
			require.NoError(t, err, "Failed to get consumer state")
			assert.True(t, found, "Consumer state should be persisted")
			assert.NotEmpty(t, stateData, "State data should not be empty")
		})
	}
}

// setupMemStoreBackend creates an in-memory backend for testing
func setupMemStoreBackend(t *testing.T) (interface{}, plugin.KeyValueBasicInterface, func()) {
	driver := memstore.NewDriver()
	err := driver.Init(context.Background(), map[string]interface{}{
		"capacity": 10000,
	})
	require.NoError(t, err, "Failed to initialize MemStore")

	cleanup := func() {
		driver.Stop(context.Background())
	}

	return driver, driver, cleanup
}

// setupNATSBackend creates a NATS backend using testcontainers
func setupNATSBackend(t *testing.T) (interface{}, plugin.KeyValueBasicInterface, func()) {
	ctx := context.Background()

	// Start NATS container
	natsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:latest",
			ExposedPorts: []string{"4222/tcp"},
		},
		Started: true,
	})
	require.NoError(t, err, "Failed to start NATS container")

	host, err := natsContainer.Host(ctx)
	require.NoError(t, err, "Failed to get NATS host")

	port, err := natsContainer.MappedPort(ctx, "4222")
	require.NoError(t, err, "Failed to get NATS port")

	// Initialize NATS driver
	natsDriver := nats.NewDriver()
	err = natsDriver.Init(ctx, map[string]interface{}{
		"url": "nats://" + host + ":" + port.Port(),
	})
	require.NoError(t, err, "Failed to initialize NATS driver")

	// Use MemStore for state persistence
	stateStore := memstore.NewDriver()
	err = stateStore.Init(ctx, map[string]interface{}{"capacity": 1000})
	require.NoError(t, err, "Failed to initialize state store")

	cleanup := func() {
		natsDriver.Stop(ctx)
		stateStore.Stop(ctx)
		natsContainer.Terminate(ctx)
	}

	return natsDriver, stateStore, cleanup
}

// setupRedisBackend creates a Redis backend using testcontainers
func setupRedisBackend(t *testing.T) (interface{}, plugin.KeyValueBasicInterface, func()) {
	ctx := context.Background()

	// Start Redis container
	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:latest",
			ExposedPorts: []string{"6379/tcp"},
		},
		Started: true,
	})
	require.NoError(t, err, "Failed to start Redis container")

	host, err := redisContainer.Host(ctx)
	require.NoError(t, err, "Failed to get Redis host")

	port, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err, "Failed to get Redis port")

	// Initialize Redis driver
	redisDriver := redis.NewDriver()
	err = redisDriver.Init(ctx, map[string]interface{}{
		"address": host + ":" + port.Port(),
	})
	require.NoError(t, err, "Failed to initialize Redis driver")

	cleanup := func() {
		redisDriver.Stop(ctx)
		redisContainer.Terminate(ctx)
	}

	return redisDriver, redisDriver, cleanup
}
