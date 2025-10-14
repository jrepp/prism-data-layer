package consumer

import "fmt"

// SlotConfig defines the backend slots required by the consumer pattern.
// Each slot specifies an interface requirement that must be filled by a backend driver.
type SlotConfig struct {
	// MessageSource provides the messages to consume.
	// Required interface: PubSubInterface or QueueInterface
	MessageSource SlotBinding `json:"message_source" yaml:"message_source"`

	// StateStore stores consumer state (offsets, checkpoints, metadata).
	// Required interface: KeyValueBasicInterface
	StateStore SlotBinding `json:"state_store" yaml:"state_store"`

	// DeadLetterQueue (optional) stores messages that fail processing.
	// Required interface: QueueInterface
	DeadLetterQueue *SlotBinding `json:"dead_letter_queue,omitempty" yaml:"dead_letter_queue,omitempty"`
}

// SlotBinding connects a pattern slot to a backend driver.
type SlotBinding struct {
	// Driver is the backend driver name (e.g., "nats", "redis", "kafka").
	Driver string `json:"driver" yaml:"driver"`

	// Config is the backend-specific configuration.
	Config map[string]interface{} `json:"config" yaml:"config"`
}

// Config is the complete consumer pattern configuration.
type Config struct {
	// Pattern metadata
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Slot bindings
	Slots SlotConfig `json:"slots" yaml:"slots"`

	// Consumer behavior
	Behavior BehaviorConfig `json:"behavior" yaml:"behavior"`
}

// BehaviorConfig controls consumer behavior.
type BehaviorConfig struct {
	// ConsumerGroup is the consumer group ID for coordinated consumption.
	ConsumerGroup string `json:"consumer_group" yaml:"consumer_group"`

	// Topic/Queue to consume from.
	Topic string `json:"topic" yaml:"topic"`

	// MaxRetries before sending to dead letter queue.
	MaxRetries int `json:"max_retries" yaml:"max_retries"`

	// BatchSize for batch processing (0 = single message).
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	// AutoCommit enables automatic offset commits.
	AutoCommit bool `json:"auto_commit" yaml:"auto_commit"`

	// CommitInterval for automatic commits (if AutoCommit=true).
	CommitInterval string `json:"commit_interval,omitempty" yaml:"commit_interval,omitempty"`
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("consumer name is required")
	}

	if c.Slots.MessageSource.Driver == "" {
		return fmt.Errorf("message_source slot requires a driver")
	}

	if c.Slots.StateStore.Driver == "" {
		return fmt.Errorf("state_store slot requires a driver")
	}

	if c.Behavior.ConsumerGroup == "" {
		return fmt.Errorf("behavior.consumer_group is required")
	}

	if c.Behavior.Topic == "" {
		return fmt.Errorf("behavior.topic is required")
	}

	if c.Behavior.MaxRetries < 0 {
		return fmt.Errorf("behavior.max_retries must be >= 0")
	}

	if c.Behavior.BatchSize < 0 {
		return fmt.Errorf("behavior.batch_size must be >= 0")
	}

	return nil
}
