package core

import "fmt"

// ConsumerConfig represents consumer pattern configuration
// Consumers bind to backends and handle subscription/consumption logic
type ConsumerConfig struct {
	BackendRef   string         `yaml:"backend_ref"`   // Reference to backend by config_path
	SubscriberID string         `yaml:"subscriber_id"` // Unique subscriber identifier
	Topic        string         `yaml:"topic"`         // Topic/queue/stream to consume from
	Concurrency  int            `yaml:"concurrency"`   // Number of concurrent consumers
	BatchSize    int            `yaml:"batch_size"`    // Messages per batch
	AckMode      string         `yaml:"ack_mode"`      // auto, manual, client
	RetryPolicy  *RetryPolicy   `yaml:"retry_policy"`  // Retry configuration
	Config       map[string]any `yaml:"config"`        // Consumer-specific configuration
}

// RetryPolicy defines retry behavior for failed messages
type RetryPolicy struct {
	MaxRetries      int     `yaml:"max_retries"`       // Maximum retry attempts
	InitialBackoff  string  `yaml:"initial_backoff"`   // Initial backoff duration
	MaxBackoff      string  `yaml:"max_backoff"`       // Maximum backoff duration
	BackoffFactor   float64 `yaml:"backoff_factor"`    // Exponential backoff multiplier
	DeadLetterTopic string  `yaml:"dead_letter_topic"` // Topic for failed messages
}

// GetConsumerBackend retrieves the backend referenced by the consumer config
func (c *Config) GetConsumerBackend() (*BackendConfig, error) {
	if c.Consumer == nil {
		return nil, fmt.Errorf("no consumer configured")
	}

	if c.Consumer.BackendRef == "" {
		return nil, fmt.Errorf("consumer has no backend_ref")
	}

	return c.GetBackend(c.Consumer.BackendRef)
}
