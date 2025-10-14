package producer

import (
	"fmt"
	"time"
)

// SlotConfig defines the backend slots required by the producer pattern.
// Each slot specifies an interface requirement that must be filled by a backend driver.
type SlotConfig struct {
	// MessageSink receives published messages.
	// Required interface: PubSubInterface or QueueInterface
	MessageSink SlotBinding `json:"message_sink" yaml:"message_sink"`

	// StateStore stores producer state (deduplication, sequence numbers).
	// Required interface: KeyValueBasicInterface
	StateStore SlotBinding `json:"state_store" yaml:"state_store"`
}

// SlotBinding connects a pattern slot to a backend driver.
type SlotBinding struct {
	// Driver is the backend driver name (e.g., "nats", "redis", "kafka").
	Driver string `json:"driver" yaml:"driver"`

	// Config is the backend-specific configuration.
	Config map[string]interface{} `json:"config" yaml:"config"`
}

// Config is the complete producer pattern configuration.
type Config struct {
	// Pattern metadata
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Slot bindings
	Slots SlotConfig `json:"slots" yaml:"slots"`

	// Producer behavior
	Behavior BehaviorConfig `json:"behavior" yaml:"behavior"`
}

// BehaviorConfig controls producer behavior.
type BehaviorConfig struct {
	// MaxRetries before giving up on a message.
	MaxRetries int `json:"max_retries" yaml:"max_retries"`

	// RetryBackoff duration between retries (e.g., "100ms", "1s").
	RetryBackoff string `json:"retry_backoff" yaml:"retry_backoff"`

	// BatchSize for batching messages (0 = no batching).
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	// BatchInterval time to wait before flushing partial batch (e.g., "100ms", "1s").
	BatchInterval string `json:"batch_interval,omitempty" yaml:"batch_interval,omitempty"`

	// Deduplication enables message deduplication based on content hash or explicit ID.
	Deduplication bool `json:"deduplication" yaml:"deduplication"`

	// DeduplicationWindowDuration is the time window for duplicate detection (e.g., "5m", "1h").
	DeduplicationWindowDuration string `json:"deduplication_window,omitempty" yaml:"deduplication_window,omitempty"`

	// Compression enables payload compression (if supported by backend).
	Compression bool `json:"compression,omitempty" yaml:"compression,omitempty"`

	// CompressionAlgorithm (e.g., "gzip", "snappy", "lz4").
	CompressionAlgorithm string `json:"compression_algorithm,omitempty" yaml:"compression_algorithm,omitempty"`

	// OrderingKey field name for maintaining message order (Kafka partition key, etc.).
	OrderingKey string `json:"ordering_key,omitempty" yaml:"ordering_key,omitempty"`
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("producer name is required")
	}

	// Slots.MessageSink.Driver is optional when using direct slot binding via BindSlots()
	// It's only required when using configuration-based initialization

	if c.Behavior.MaxRetries < 0 {
		return fmt.Errorf("behavior.max_retries must be >= 0")
	}

	if c.Behavior.BatchSize < 0 {
		return fmt.Errorf("behavior.batch_size must be >= 0")
	}

	if c.Behavior.BatchSize > 0 && c.Behavior.BatchInterval == "" {
		return fmt.Errorf("behavior.batch_interval is required when batch_size > 0")
	}

	if c.Behavior.Deduplication && c.Behavior.DeduplicationWindowDuration == "" {
		c.Behavior.DeduplicationWindowDuration = "5m" // Default to 5 minutes
	}

	if c.Behavior.Compression && c.Behavior.CompressionAlgorithm == "" {
		c.Behavior.CompressionAlgorithm = "gzip" // Default to gzip
	}

	// Validate durations
	if c.Behavior.RetryBackoff != "" {
		if _, err := time.ParseDuration(c.Behavior.RetryBackoff); err != nil {
			return fmt.Errorf("invalid behavior.retry_backoff duration: %w", err)
		}
	} else {
		c.Behavior.RetryBackoff = "1s" // Default to 1 second
	}

	if c.Behavior.BatchInterval != "" {
		if _, err := time.ParseDuration(c.Behavior.BatchInterval); err != nil {
			return fmt.Errorf("invalid behavior.batch_interval duration: %w", err)
		}
	}

	if c.Behavior.DeduplicationWindowDuration != "" {
		if _, err := time.ParseDuration(c.Behavior.DeduplicationWindowDuration); err != nil {
			return fmt.Errorf("invalid behavior.deduplication_window duration: %w", err)
		}
	}

	return nil
}

// RetryBackoffDuration returns the retry backoff duration.
func (b *BehaviorConfig) RetryBackoffDuration() time.Duration {
	if b.RetryBackoff == "" {
		return 1 * time.Second
	}
	d, _ := time.ParseDuration(b.RetryBackoff)
	return d
}

// BatchIntervalDuration returns the batch interval duration.
func (b *BehaviorConfig) BatchIntervalDuration() time.Duration {
	if b.BatchInterval == "" {
		return 100 * time.Millisecond
	}
	d, _ := time.ParseDuration(b.BatchInterval)
	return d
}

// DeduplicationWindow returns the deduplication window duration.
func (b *BehaviorConfig) DeduplicationWindow() time.Duration {
	if b.DeduplicationWindowDuration == "" {
		return 5 * time.Minute
	}
	d, _ := time.ParseDuration(b.DeduplicationWindowDuration)
	return d
}
