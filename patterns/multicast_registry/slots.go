package multicast_registry

import (
	"context"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/multicast_registry/backends"
)

// RegistryBackend defines the interface for identity registry storage
type RegistryBackend interface {
	// Set stores identity metadata with optional TTL
	Set(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error

	// Get retrieves identity metadata
	Get(ctx context.Context, identity string) (*backends.Identity, error)

	// Scan returns all identities (no filtering)
	Scan(ctx context.Context) ([]*backends.Identity, error)

	// Delete removes an identity
	Delete(ctx context.Context, identity string) error

	// Enumerate queries identities with filter
	// Backend-native filtering if supported, otherwise returns all for client-side filtering
	Enumerate(ctx context.Context, filter *backends.Filter) ([]*backends.Identity, error)

	// Close closes backend connections
	Close() error
}

// MessagingBackend defines the interface for multicast message delivery
type MessagingBackend interface {
	// Publish sends a message to a specific topic/identity
	Publish(ctx context.Context, topic string, payload []byte) error

	// Subscribe creates a subscription for an identity (consumer-side, not used in POC 4)
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)

	// Unsubscribe removes a subscription
	Unsubscribe(ctx context.Context, topic string) error

	// Close closes backend connections
	Close() error
}

// DurabilityBackend defines the interface for persistent message queues (optional)
type DurabilityBackend interface {
	// Enqueue adds a message to a queue for an identity
	Enqueue(ctx context.Context, queue string, payload []byte) error

	// Dequeue retrieves the next message from a queue
	Dequeue(ctx context.Context, queue string) ([]byte, error)

	// Acknowledge confirms message delivery
	Acknowledge(ctx context.Context, queue string, messageID string) error

	// Close closes backend connections
	Close() error
}

// Type aliases for convenience (avoid backends. prefix everywhere)
type Identity = backends.Identity
type Filter = backends.Filter

// NewFilter creates a simple equality filter (delegates to backends package)
func NewFilter(conditions map[string]interface{}) *Filter {
	return backends.NewFilter(conditions)
}

// DeliveryResult represents the result of a multicast delivery attempt
type DeliveryResult struct {
	Identity string
	Status   DeliveryStatus
	Error    error
	Latency  time.Duration
}

// DeliveryStatus indicates delivery state
type DeliveryStatus int

const (
	DeliveryStatusDelivered DeliveryStatus = iota
	DeliveryStatusPending
	DeliveryStatusFailed
	DeliveryStatusTimeout
)

func (s DeliveryStatus) String() string {
	switch s {
	case DeliveryStatusDelivered:
		return "DELIVERED"
	case DeliveryStatusPending:
		return "PENDING"
	case DeliveryStatusFailed:
		return "FAILED"
	case DeliveryStatusTimeout:
		return "TIMEOUT"
	default:
		return "UNKNOWN"
	}
}
