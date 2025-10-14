package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
)

const (
	version = "0.1.0"
)

// subscription tracks a topic subscription with its message channel
type subscription struct {
	topic        string
	subscriberID string
	msgChan      chan *plugin.PubSubMessage
	consumer     *kafka.Consumer
	cancelFunc   context.CancelFunc
}

// KafkaPlugin implements the Prism backend plugin for Kafka
// Based on RFC-011 (SASL/SCRAM auth) and ADR-025 (Container plugin model)
type KafkaPlugin struct {
	producer *kafka.Producer
	consumer *kafka.Consumer
	config   *KafkaConfig
	subs     map[string]*subscription // key: "topic:subscriberID"
	subsMu   sync.RWMutex
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

	// Close all subscriptions
	p.subsMu.Lock()
	for key, sub := range p.subs {
		if sub.cancelFunc != nil {
			sub.cancelFunc()
		}
		if sub.consumer != nil {
			sub.consumer.Close()
		}
		close(sub.msgChan)
		delete(p.subs, key)
	}
	p.subsMu.Unlock()

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

// PubSubInterface implementation

// Publish sends a message to a Kafka topic
// Implements plugin.PubSubInterface
func (p *KafkaPlugin) Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	if p.producer == nil {
		return "", fmt.Errorf("kafka producer not initialized")
	}

	// Generate unique message ID
	messageID := uuid.New().String()

	// Create Kafka message with headers from metadata
	headers := make([]kafka.Header, 0, len(metadata)+1)
	headers = append(headers, kafka.Header{
		Key:   "message_id",
		Value: []byte(messageID),
	})
	for k, v := range metadata {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

	message := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value:   payload,
		Headers: headers,
	}

	// Async produce
	if err := p.producer.Produce(message, nil); err != nil {
		return "", fmt.Errorf("failed to produce message: %w", err)
	}

	slog.Debug("published message", "topic", topic, "message_id", messageID, "size", len(payload))
	return messageID, nil
}

// Subscribe subscribes to a Kafka topic and returns a channel for messages
// Implements plugin.PubSubInterface
func (p *KafkaPlugin) Subscribe(ctx context.Context, topic string, subscriberID string) (<-chan *plugin.PubSubMessage, error) {
	// Create consumer config for this subscription
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" && len(p.config.Brokers) > 0 {
		brokers = p.config.Brokers[0]
	}
	if brokers == "" {
		return nil, fmt.Errorf("KAFKA_BROKERS not configured")
	}

	consumerConfig := kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           fmt.Sprintf("%s-%s", p.config.ConsumerGroup, subscriberID),
		"auto.offset.reset":  p.config.AutoOffsetReset,
		"enable.auto.commit": false,
		"client.id":          fmt.Sprintf("prism-kafka-sub-%s-%s", topic, subscriberID),
	}

	// Add SASL configuration if provided
	if p.config.SASLUsername != "" && p.config.SASLPassword != "" {
		consumerConfig["security.protocol"] = "SASL_SSL"
		consumerConfig["sasl.mechanism"] = p.config.SASLMechanism
		consumerConfig["sasl.username"] = p.config.SASLUsername
		consumerConfig["sasl.password"] = p.config.SASLPassword
	}

	// Create dedicated consumer for this subscription
	consumer, err := kafka.NewConsumer(&consumerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Subscribe to topic
	if err := consumer.Subscribe(topic, nil); err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	// Create message channel
	msgChan := make(chan *plugin.PubSubMessage, 100)

	// Create cancellable context for this subscription
	subCtx, cancel := context.WithCancel(ctx)

	// Store subscription
	sub := &subscription{
		topic:        topic,
		subscriberID: subscriberID,
		msgChan:      msgChan,
		consumer:     consumer,
		cancelFunc:   cancel,
	}

	p.subsMu.Lock()
	key := fmt.Sprintf("%s:%s", topic, subscriberID)
	p.subs[key] = sub
	p.subsMu.Unlock()

	// Start consuming messages in background
	go p.consumeSubscription(subCtx, sub)

	slog.Info("subscribed to topic", "topic", topic, "subscriber_id", subscriberID)
	return msgChan, nil
}

// consumeSubscription handles message consumption for a specific subscription
func (p *KafkaPlugin) consumeSubscription(ctx context.Context, sub *subscription) {
	defer func() {
		slog.Info("subscription consumer stopped", "topic", sub.topic, "subscriber_id", sub.subscriberID)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := sub.consumer.ReadMessage(1 * time.Second)
			if err != nil {
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue // Normal timeout, keep polling
				}
				slog.Error("consumer error", "error", err, "topic", sub.topic)
				continue
			}

			// Extract metadata from headers
			metadata := make(map[string]string)
			messageID := ""
			for _, header := range msg.Headers {
				if header.Key == "message_id" {
					messageID = string(header.Value)
				} else {
					metadata[header.Key] = string(header.Value)
				}
			}

			// If no message ID in headers, generate one
			if messageID == "" {
				messageID = fmt.Sprintf("%s-%d-%d", *msg.TopicPartition.Topic, msg.TopicPartition.Partition, msg.TopicPartition.Offset)
			}

			// Send to channel
			select {
			case sub.msgChan <- &plugin.PubSubMessage{
				Topic:     *msg.TopicPartition.Topic,
				Payload:   msg.Value,
				Metadata:  metadata,
				MessageID: messageID,
				Timestamp: msg.Timestamp.Unix(),
			}:
				// Commit offset after successful delivery
				if _, err := sub.consumer.CommitMessage(msg); err != nil {
					slog.Error("failed to commit offset", "error", err)
				}
			case <-ctx.Done():
				return
			default:
				// Channel full, drop message
				slog.Warn("message dropped (channel full)", "topic", sub.topic, "subscriber_id", sub.subscriberID)
			}
		}
	}
}

// Unsubscribe unsubscribes from a Kafka topic
// Implements plugin.PubSubInterface
func (p *KafkaPlugin) Unsubscribe(ctx context.Context, topic string, subscriberID string) error {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()

	key := fmt.Sprintf("%s:%s", topic, subscriberID)
	sub, exists := p.subs[key]
	if !exists {
		return fmt.Errorf("no subscription found for topic %s with subscriber %s", topic, subscriberID)
	}

	// Cancel consumer goroutine
	if sub.cancelFunc != nil {
		sub.cancelFunc()
	}

	// Close consumer
	if sub.consumer != nil {
		sub.consumer.Close()
	}

	// Close channel
	close(sub.msgChan)

	delete(p.subs, key)

	slog.Info("unsubscribed from topic", "topic", topic, "subscriber_id", subscriberID)
	return nil
}

// QueueInterface implementation

// Enqueue sends a message to a Kafka topic (queue semantics)
// Implements plugin.QueueInterface
func (p *KafkaPlugin) Enqueue(ctx context.Context, queue string, payload []byte, metadata map[string]string) (string, error) {
	// Kafka doesn't distinguish between topics and queues - both use topics
	// Consumer groups provide queue semantics (load balancing)
	return p.Publish(ctx, queue, payload, metadata)
}

// Receive receives messages from a Kafka topic with queue semantics
// Uses consumer groups for load balancing - only one consumer in group gets each message
// Implements plugin.QueueInterface
func (p *KafkaPlugin) Receive(ctx context.Context, queue string) (<-chan *plugin.PubSubMessage, error) {
	// Create consumer config with dedicated queue group
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" && len(p.config.Brokers) > 0 {
		brokers = p.config.Brokers[0]
	}
	if brokers == "" {
		return nil, fmt.Errorf("KAFKA_BROKERS not configured")
	}

	queueGroup := fmt.Sprintf("%s-queue", queue)
	consumerConfig := kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           queueGroup,
		"auto.offset.reset":  p.config.AutoOffsetReset,
		"enable.auto.commit": false, // Manual commit for explicit ack
		"client.id":          fmt.Sprintf("prism-kafka-queue-%s", queue),
	}

	// Add SASL configuration if provided
	if p.config.SASLUsername != "" && p.config.SASLPassword != "" {
		consumerConfig["security.protocol"] = "SASL_SSL"
		consumerConfig["sasl.mechanism"] = p.config.SASLMechanism
		consumerConfig["sasl.username"] = p.config.SASLUsername
		consumerConfig["sasl.password"] = p.config.SASLPassword
	}

	// Create dedicated consumer for this queue
	consumer, err := kafka.NewConsumer(&consumerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Subscribe to queue topic
	if err := consumer.Subscribe(queue, nil); err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to subscribe to queue: %w", err)
	}

	// Create message channel
	msgChan := make(chan *plugin.PubSubMessage, 100)

	// Create cancellable context for this queue receiver
	queueCtx, cancel := context.WithCancel(ctx)

	// Store subscription
	sub := &subscription{
		topic:        queue,
		subscriberID: "queue",
		msgChan:      msgChan,
		consumer:     consumer,
		cancelFunc:   cancel,
	}

	p.subsMu.Lock()
	key := fmt.Sprintf("queue:%s", queue)
	p.subs[key] = sub
	p.subsMu.Unlock()

	// Start consuming messages in background
	go p.consumeSubscription(queueCtx, sub)

	slog.Info("receiving from queue", "queue", queue, "group", queueGroup)
	return msgChan, nil
}

// Acknowledge acknowledges a message (commits offset)
// Implements plugin.QueueInterface
func (p *KafkaPlugin) Acknowledge(ctx context.Context, queue string, messageID string) error {
	// In Kafka, acknowledgment is handled via offset commits
	// We commit offsets immediately after delivering messages in consumeSubscription
	// So this is a no-op (already committed)
	slog.Debug("acknowledged message", "queue", queue, "message_id", messageID)
	return nil
}

// Reject rejects a message (seek back to requeue)
// Implements plugin.QueueInterface
func (p *KafkaPlugin) Reject(ctx context.Context, queue string, messageID string, requeue bool) error {
	p.subsMu.RLock()
	key := fmt.Sprintf("queue:%s", queue)
	_, exists := p.subs[key]
	p.subsMu.RUnlock()

	if !exists {
		return fmt.Errorf("no queue receiver found for %s", queue)
	}

	if requeue {
		// To requeue in Kafka, we need to seek back to the last committed offset
		// This is complex and requires tracking message offsets
		// For now, we'll just log a warning
		slog.Warn("message reject with requeue not fully implemented", "queue", queue, "message_id", messageID)
		// TODO: Track message offsets and seek back to requeue
	} else {
		// If not requeuing, we can just commit the offset to skip this message
		// But we already committed in consumeSubscription, so this is a no-op
		slog.Debug("rejected message (no requeue)", "queue", queue, "message_id", messageID)
	}

	return nil
}


// New creates a new Kafka driver instance
func New() *KafkaPlugin {
	return &KafkaPlugin{
		subs: make(map[string]*subscription),
	}
}

// GetInterfaceDeclarations returns the interfaces this driver implements
// This is used during registration with the proxy (replacing runtime introspection)
func (p *KafkaPlugin) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	return []*pb.InterfaceDeclaration{
		{
			Name:      "PubSubInterface",
			ProtoFile: "prism/interfaces/pubsub/pubsub.proto",
			Version:   "v1",
		},
		{
			Name:      "QueueInterface",
			ProtoFile: "prism/interfaces/queue/queue.proto",
			Version:   "v1",
		},
	}
}

// Compile-time interface compliance checks
// These ensure that KafkaPlugin implements the expected interfaces
var (
	_ plugin.Plugin          = (*KafkaPlugin)(nil) // Core plugin interface
	_ plugin.PubSubInterface = (*KafkaPlugin)(nil) // PubSub interface
	_ plugin.QueueInterface  = (*KafkaPlugin)(nil) // Queue interface
)

