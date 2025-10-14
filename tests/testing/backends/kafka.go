package backends

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

// KafkaBackend provides Kafka testcontainer setup and cleanup
type KafkaBackend struct {
	Brokers []string
	cleanup func()
}

// SetupKafka starts a Kafka container and returns connection info
func SetupKafka(t *testing.T, ctx context.Context) *KafkaBackend {
	t.Helper()

	// Start Kafka container (includes embedded Zookeeper)
	kafkaContainer, err := tckafka.Run(ctx,
		"confluentinc/confluent-local:7.5.0",
		tckafka.WithClusterID("test-cluster"),
	)
	require.NoError(t, err, "Failed to start Kafka container")

	// Get brokers
	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err, "Failed to get Kafka brokers")

	// Convert to string slice
	brokerList := make([]string, len(brokers))
	for i, broker := range brokers {
		brokerList[i] = broker
	}

	// Ensure at least one broker
	require.NotEmpty(t, brokerList, "No Kafka brokers available")

	t.Logf("Kafka broker available at: %s", brokerList[0])

	return &KafkaBackend{
		Brokers: brokerList,
		cleanup: func() {
			if err := kafkaContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate Kafka container: %v", err)
			}
		},
	}
}

// ConnectionString returns the first broker address for single-broker configurations
func (b *KafkaBackend) ConnectionString() string {
	if len(b.Brokers) > 0 {
		return b.Brokers[0]
	}
	return ""
}

// BrokerList returns all broker addresses as comma-separated string
func (b *KafkaBackend) BrokerList() string {
	result := ""
	for i, broker := range b.Brokers {
		if i > 0 {
			result += ","
		}
		result += broker
	}
	return result
}

// Cleanup terminates the Kafka container
func (b *KafkaBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}

// CreateTopic creates a topic in the Kafka cluster (for advanced testing)
func (b *KafkaBackend) CreateTopic(ctx context.Context, topic string, partitions int, replicationFactor int) error {
	// Note: With testcontainers-go/modules/kafka, topics are auto-created on first publish
	// This is a placeholder for explicit topic management if needed in the future
	return fmt.Errorf("explicit topic creation not yet implemented - topics auto-create on first use")
}
