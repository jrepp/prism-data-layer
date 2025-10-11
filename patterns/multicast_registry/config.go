package multicast_registry

import (
	"time"
)

// Config defines the multicast registry pattern configuration
type Config struct {
	// Pattern-level settings
	PatternName    string        `yaml:"pattern_name"`
	DefaultTTL     time.Duration `yaml:"default_ttl"`      // Default TTL for registrations (0 = no expiration)
	MaxIdentities  int           `yaml:"max_identities"`   // Maximum identities per namespace (0 = unlimited)
	MaxFilterDepth int           `yaml:"max_filter_depth"` // Maximum filter nesting depth (default: 5)
	MaxClauses     int           `yaml:"max_clauses"`      // Maximum filter clauses (default: 20)

	// Backend slot configurations
	Registry   RegistrySlotConfig   `yaml:"registry"`
	Messaging  MessagingSlotConfig  `yaml:"messaging"`
	Durability DurabilitySlotConfig `yaml:"durability,omitempty"` // Optional
}

// RegistrySlotConfig configures the registry backend slot
type RegistrySlotConfig struct {
	Type     string                 `yaml:"type"` // redis, postgres, dynamodb, etcd
	Host     string                 `yaml:"host,omitempty"`
	Port     int                    `yaml:"port,omitempty"`
	Options  map[string]interface{} `yaml:"options,omitempty"`
	TTLField string                 `yaml:"ttl_field,omitempty"` // Field name for TTL (backend-specific)
}

// MessagingSlotConfig configures the messaging backend slot
type MessagingSlotConfig struct {
	Type         string                 `yaml:"type"` // nats, kafka, redis-pubsub, rabbitmq
	Servers      []string               `yaml:"servers,omitempty"`
	TopicPrefix  string                 `yaml:"topic_prefix,omitempty"`
	Delivery     DeliverySemantics      `yaml:"delivery"` // at-most-once, at-least-once, exactly-once
	Options      map[string]interface{} `yaml:"options,omitempty"`
	RetryAttempts int                   `yaml:"retry_attempts,omitempty"` // Number of delivery retries
	RetryDelay    time.Duration         `yaml:"retry_delay,omitempty"`    // Delay between retries
}

// DurabilitySlotConfig configures the optional durability backend slot
type DurabilitySlotConfig struct {
	Enabled       bool                   `yaml:"enabled"`
	Type          string                 `yaml:"type"` // kafka, postgres, redis-stream, sqs
	Options       map[string]interface{} `yaml:"options,omitempty"`
	UseMessaging  bool                   `yaml:"use_messaging"` // If true, use messaging backend for durability
	RetentionDays int                    `yaml:"retention_days,omitempty"`
}

// DeliverySemantics defines message delivery guarantees
type DeliverySemantics string

const (
	DeliveryAtMostOnce  DeliverySemantics = "at-most-once"
	DeliveryAtLeastOnce DeliverySemantics = "at-least-once"
	DeliveryExactlyOnce DeliverySemantics = "exactly-once"
)

// DefaultConfig returns a default configuration for testing
func DefaultConfig() *Config {
	return &Config{
		PatternName:    "multicast-registry",
		DefaultTTL:     5 * time.Minute,
		MaxIdentities:  0, // unlimited
		MaxFilterDepth: 5,
		MaxClauses:     20,
		Registry: RegistrySlotConfig{
			Type: "memstore",
		},
		Messaging: MessagingSlotConfig{
			Type:          "nats",
			Delivery:      DeliveryAtMostOnce,
			RetryAttempts: 3,
			RetryDelay:    100 * time.Millisecond,
		},
		Durability: DurabilitySlotConfig{
			Enabled: false,
		},
	}
}
