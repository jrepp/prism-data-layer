package natsredis

import (
	"context"
	"fmt"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// Plugin implements a consumer pattern using NATS for messages and Redis for state storage.
//
// Slot Bindings:
//   - MessageSource: NATS (PubSubInterface)
//   - StateStore: Redis (KeyValueBasicInterface)
//   - DeadLetterQueue: NATS (QueueInterface) - optional
type Plugin struct {
	consumer *consumer.Consumer
	nats     *nats.NATSPattern
	redis    *redis.RedisPattern
}

// New creates a new NATS+Redis consumer plugin.
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

	// Initialize NATS driver
	p.nats = nats.New()
	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats",
			Version: "0.1.0",
		},
		Backend: messageSourceCfg["config"].(map[string]interface{}),
	}

	if err := p.nats.Initialize(ctx, natsConfig); err != nil {
		return fmt.Errorf("failed to initialize NATS: %w", err)
	}

	if err := p.nats.Start(ctx); err != nil {
		return fmt.Errorf("failed to start NATS: %w", err)
	}

	// Initialize Redis driver
	p.redis = redis.New()
	redisConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "redis",
			Version: "0.1.0",
		},
		Backend: stateStoreCfg["config"].(map[string]interface{}),
	}

	if err := p.redis.Initialize(ctx, redisConfig); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	if err := p.redis.Start(ctx); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// Bind slots to consumer
	// NATS provides PubSubInterface
	// Redis provides KeyValueBasicInterface
	if err := p.consumer.BindSlots(p.nats, p.redis, nil); err != nil {
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
	if p.nats != nil {
		if err := p.nats.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop NATS: %w", err)
		}
	}

	if p.redis != nil {
		if err := p.redis.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop Redis: %w", err)
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

	// Get NATS health
	natsHealth, err := p.nats.Health(ctx)
	if err != nil {
		consumerHealth.Details["nats_error"] = err.Error()
		consumerHealth.Status = plugin.HealthDegraded
	} else {
		consumerHealth.Details["nats_status"] = natsHealth.Status.String()
	}

	// Get Redis health
	redisHealth, err := p.redis.Health(ctx)
	if err != nil {
		consumerHealth.Details["redis_error"] = err.Error()
		consumerHealth.Status = plugin.HealthDegraded
	} else {
		consumerHealth.Details["redis_status"] = redisHealth.Status.String()
	}

	return consumerHealth, nil
}

// SetProcessor sets the message processing function.
func (p *Plugin) SetProcessor(processor consumer.MessageProcessor) {
	p.consumer.SetProcessor(processor)
}

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return "consumer-nats-redis"
}

// Version returns the plugin version.
func (p *Plugin) Version() string {
	return "0.1.0"
}
