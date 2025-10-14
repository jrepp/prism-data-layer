package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

const (
	version = "0.1.0"
)

// KafkaPlugin implements the Prism backend plugin for Kafka
// Based on RFC-011 (SASL/SCRAM auth) and ADR-025 (Container plugin model)
type KafkaPlugin struct {
	producer *kafka.Producer
	consumer *kafka.Consumer
	config   *KafkaConfig
}

// KafkaConfig holds Kafka-specific configuration
// Follows RFC-011 authentication patterns and ADR-025 environment config
type KafkaConfig struct {
	Brokers       []string
	Topic         string
	ConsumerGroup string

	// SASL/SCRAM authentication (RFC-011)
	SASLMechanism string // "SCRAM-SHA-512"
	SASLUsername  string
	SASLPassword  string

	// Vault-managed credentials
	VaultEnabled bool
	VaultPath    string

	// Producer settings
	Compression string // "snappy", "gzip", "lz4", "zstd"
	Acks        string // "all", "1", "0"

	// Consumer settings
	AutoOffsetReset string // "earliest", "latest"
}

func (p *KafkaPlugin) Name() string {
	return "kafka"
}

func (p *KafkaPlugin) Version() string {
	return version
}

// Initialize creates Kafka producer and consumer
// Based on RFC-011 SASL/SCRAM authentication flow
func (p *KafkaPlugin) Initialize(ctx context.Context, config *plugin.Config) error {
	slog.Info("initializing kafka plugin", "version", version)

	// Extract backend-specific config
	var kafkaConfig KafkaConfig
	if err := config.GetBackendConfig(&kafkaConfig); err != nil {
		return fmt.Errorf("failed to parse kafka config: %w", err)
	}
	p.config = &kafkaConfig

	// Get brokers from environment or config
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" && len(kafkaConfig.Brokers) > 0 {
		brokers = kafkaConfig.Brokers[0] // Use first broker from config
	}
	if brokers == "" {
		return fmt.Errorf("KAFKA_BROKERS not configured")
	}

	// Fetch Vault credentials if enabled (RFC-011 pattern)
	if kafkaConfig.VaultEnabled {
		slog.Info("vault-managed credentials enabled", "path", kafkaConfig.VaultPath)
		// TODO: Implement Vault credential fetching
		// creds, err := p.fetchVaultCredentials(ctx, kafkaConfig.VaultPath)
		// if err != nil {
		//     return fmt.Errorf("failed to fetch vault credentials: %w", err)
		// }
		// kafkaConfig.SASLUsername = creds.Username
		// kafkaConfig.SASLPassword = creds.Password
	} else {
		// Use environment variables for SASL credentials
		if user := os.Getenv("KAFKA_SASL_USERNAME"); user != "" {
			kafkaConfig.SASLUsername = user
		}
		if pass := os.Getenv("KAFKA_SASL_PASSWORD"); pass != "" {
			kafkaConfig.SASLPassword = pass
		}
	}

	// Create producer (RFC-011: SASL/SCRAM authentication)
	producerConfig := kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"compression.type":  kafkaConfig.Compression,
		"acks":              kafkaConfig.Acks,
		"client.id":         fmt.Sprintf("prism-kafka-producer-%s", version),
	}

	// Add SASL configuration if provided (RFC-011)
	if kafkaConfig.SASLUsername != "" && kafkaConfig.SASLPassword != "" {
		producerConfig["security.protocol"] = "SASL_SSL"
		producerConfig["sasl.mechanism"] = kafkaConfig.SASLMechanism
		producerConfig["sasl.username"] = kafkaConfig.SASLUsername
		producerConfig["sasl.password"] = kafkaConfig.SASLPassword
		slog.Info("kafka SASL authentication configured",
			"mechanism", kafkaConfig.SASLMechanism,
			"username", kafkaConfig.SASLUsername)
	}

	producer, err := kafka.NewProducer(&producerConfig)
	if err != nil {
		return fmt.Errorf("failed to create producer: %w", err)
	}
	p.producer = producer

	// Create consumer (RFC-011: SASL/SCRAM authentication)
	consumerConfig := kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           kafkaConfig.ConsumerGroup,
		"auto.offset.reset":  kafkaConfig.AutoOffsetReset,
		"enable.auto.commit": false, // Manual commit for reliability
		"client.id":          fmt.Sprintf("prism-kafka-consumer-%s", version),
	}

	// Add SASL configuration for consumer
	if kafkaConfig.SASLUsername != "" && kafkaConfig.SASLPassword != "" {
		consumerConfig["security.protocol"] = "SASL_SSL"
		consumerConfig["sasl.mechanism"] = kafkaConfig.SASLMechanism
		consumerConfig["sasl.username"] = kafkaConfig.SASLUsername
		consumerConfig["sasl.password"] = kafkaConfig.SASLPassword
	}

	consumer, err := kafka.NewConsumer(&consumerConfig)
	if err != nil {
		producer.Close()
		return fmt.Errorf("failed to create consumer: %w", err)
	}
	p.consumer = consumer

	// Subscribe to topic
	if err := consumer.Subscribe(kafkaConfig.Topic, nil); err != nil {
		producer.Close()
		consumer.Close()
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	slog.Info("kafka plugin initialized",
		"brokers", brokers,
		"topic", kafkaConfig.Topic,
		"consumer_group", kafkaConfig.ConsumerGroup,
		"compression", kafkaConfig.Compression,
		"sasl_enabled", kafkaConfig.SASLUsername != "")

	return nil
}

// Start begins serving requests
func (p *KafkaPlugin) Start(ctx context.Context) error {
	slog.Info("kafka plugin started")

	// Start producer event handler
	go p.handleProducerEvents(ctx)

	// Start consumer loop
	go p.consumeMessages(ctx)

	// Keep running until context is cancelled
	<-ctx.Done()

	slog.Info("kafka plugin stopping")
	return nil
}

// Stop gracefully shuts down the plugin
func (p *KafkaPlugin) Stop(ctx context.Context) error {
	slog.Info("stopping kafka plugin")

	if p.consumer != nil {
		p.consumer.Close()
		slog.Info("closed kafka consumer")
	}

	if p.producer != nil {
		// Flush pending messages
		remaining := p.producer.Flush(5000) // 5 second timeout
		if remaining > 0 {
			slog.Warn("not all messages flushed", "remaining", remaining)
		}
		p.producer.Close()
		slog.Info("closed kafka producer")
	}

	return nil
}

// Health reports the plugin health status
func (p *KafkaPlugin) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	if p.producer == nil || p.consumer == nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthUnhealthy,
			Message: "kafka clients not initialized",
		}, nil
	}

	// Check producer metrics
	producerLen := p.producer.Len()
	if producerLen > 10000 {
		return &plugin.HealthStatus{
			Status:  plugin.HealthDegraded,
			Message: "producer queue backing up",
			Details: map[string]string{
				"queue_length": fmt.Sprintf("%d", producerLen),
			},
		}, nil
	}

	return &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "kafka healthy",
		Details: map[string]string{
			"producer_queue": fmt.Sprintf("%d", producerLen),
			"topic":          p.config.Topic,
		},
	}, nil
}

// handleProducerEvents processes delivery reports from Kafka
func (p *KafkaPlugin) handleProducerEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-p.producer.Events():
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					slog.Error("message delivery failed",
						"error", ev.TopicPartition.Error,
						"topic", *ev.TopicPartition.Topic,
						"partition", ev.TopicPartition.Partition)
				} else {
					slog.Debug("message delivered",
						"topic", *ev.TopicPartition.Topic,
						"partition", ev.TopicPartition.Partition,
						"offset", ev.TopicPartition.Offset)
				}
			}
		}
	}
}

// consumeMessages reads messages from Kafka and forwards to Prism proxy
func (p *KafkaPlugin) consumeMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := p.consumer.ReadMessage(1 * time.Second)
			if err != nil {
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue // Normal timeout, keep polling
				}
				slog.Error("consumer error", "error", err)
				continue
			}

			// Process message
			slog.Debug("consumed message",
				"topic", *msg.TopicPartition.Topic,
				"partition", msg.TopicPartition.Partition,
				"offset", msg.TopicPartition.Offset,
				"key", string(msg.Key),
				"size", len(msg.Value))

			// TODO: Forward to Prism proxy via gRPC

			// Commit offset
			if _, err := p.consumer.CommitMessage(msg); err != nil {
				slog.Error("failed to commit offset", "error", err)
			}
		}
	}
}

// Publish sends a message to Kafka
// ADR-005: Queue/TimeSeries abstraction
func (p *KafkaPlugin) Publish(ctx context.Context, topic string, key, value []byte) error {
	message := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Key:   key,
		Value: value,
	}

	// Async produce
	if err := p.producer.Produce(message, nil); err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	slog.Debug("published message", "topic", topic, "key", string(key), "size", len(value))
	return nil
}


// New creates a new Kafka driver instance
func New() *KafkaPlugin {
	return &KafkaPlugin{}
}

// Compile-time interface compliance checks
var (
	_ plugin.Plugin = (*KafkaPlugin)(nil)
)

