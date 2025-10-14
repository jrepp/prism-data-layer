package kafkapostgres

import (
	"context"
	"fmt"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/kafka"
	"github.com/jrepp/prism-data-layer/pkg/drivers/postgres"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// Plugin implements a consumer pattern using Kafka for messages and PostgreSQL for state storage.
//
// Slot Bindings:
//   - MessageSource: Kafka (QueueInterface)
//   - StateStore: PostgreSQL (KeyValueBasicInterface)
//   - DeadLetterQueue: Kafka (QueueInterface) - optional
type Plugin struct {
	consumer *consumer.Consumer
	kafka    *kafka.KafkaPlugin
	postgres *postgres.PostgresPlugin
}

// New creates a new Kafka+PostgreSQL consumer plugin.
func New(config consumer.Config) (*Plugin, error) {
	// Create consumer with config
	c, err := consumer.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &Plugin{
		consumer: c,
	}, nil
}

// Initialize initializes the backend drivers and binds them to slots.
func (p *Plugin) Initialize(ctx context.Context, config *plugin.Config) error {
	// Extract slot configurations
	slots := config.Backend["slots"].(map[string]interface{})
	messageSourceCfg := slots["message_source"].(map[string]interface{})
	stateStoreCfg := slots["state_store"].(map[string]interface{})

	// Initialize Kafka driver
	p.kafka = kafka.New()
	kafkaConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "kafka",
			Version: "0.1.0",
		},
		Backend: messageSourceCfg["config"].(map[string]interface{}),
	}

	if err := p.kafka.Initialize(ctx, kafkaConfig); err != nil {
		return fmt.Errorf("failed to initialize Kafka: %w", err)
	}

	if err := p.kafka.Start(ctx); err != nil {
		return fmt.Errorf("failed to start Kafka: %w", err)
	}

	// Initialize PostgreSQL driver
	p.postgres = postgres.New()
	postgresConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "postgres",
			Version: "0.1.0",
		},
		Backend: stateStoreCfg["config"].(map[string]interface{}),
	}

	if err := p.postgres.Initialize(ctx, postgresConfig); err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	if err := p.postgres.Start(ctx); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// Bind slots to consumer
	// Kafka provides QueueInterface
	// PostgreSQL provides KeyValueBasicInterface
	if err := p.consumer.BindSlots(p.kafka, p.postgres, nil); err != nil {
		return fmt.Errorf("failed to bind slots: %w", err)
	}

	return nil
}

// Start starts the consumer.
func (p *Plugin) Start(ctx context.Context) error {
	return p.consumer.Start(ctx)
}

// Stop stops the consumer and underlying drivers.
func (p *Plugin) Stop(ctx context.Context) error {
	// Stop consumer first
	if err := p.consumer.Stop(ctx); err != nil {
		return err
	}

	// Stop drivers
	if p.kafka != nil {
		if err := p.kafka.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop Kafka: %w", err)
		}
	}

	if p.postgres != nil {
		if err := p.postgres.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop PostgreSQL: %w", err)
		}
	}

	return nil
}

// Health returns the health status of the consumer and drivers.
func (p *Plugin) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	// Get consumer health
	consumerHealth, err := p.consumer.Health(ctx)
	if err != nil {
		return nil, err
	}

	// Get Kafka health
	kafkaHealth, err := p.kafka.Health(ctx)
	if err != nil {
		consumerHealth.Details["kafka_error"] = err.Error()
		consumerHealth.Status = plugin.HealthDegraded
	} else {
		consumerHealth.Details["kafka_status"] = kafkaHealth.Status.String()
	}

	// Get PostgreSQL health
	postgresHealth, err := p.postgres.Health(ctx)
	if err != nil {
		consumerHealth.Details["postgres_error"] = err.Error()
		consumerHealth.Status = plugin.HealthDegraded
	} else {
		consumerHealth.Details["postgres_status"] = postgresHealth.Status.String()
	}

	return consumerHealth, nil
}

// SetProcessor sets the message processing function.
func (p *Plugin) SetProcessor(processor consumer.MessageProcessor) {
	p.consumer.SetProcessor(processor)
}

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return "consumer-kafka-postgres"
}

// Version returns the plugin version.
func (p *Plugin) Version() string {
	return "0.1.0"
}
