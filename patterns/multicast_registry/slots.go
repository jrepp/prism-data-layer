package multicast_registry

import (
	"context"
	"time"
)

// RegistryBackend defines the interface for identity registry storage
type RegistryBackend interface {
	// Set stores identity metadata with optional TTL
	Set(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error

	// Get retrieves identity metadata
	Get(ctx context.Context, identity string) (*Identity, error)

	// Scan returns all identities (no filtering)
	Scan(ctx context.Context) ([]*Identity, error)

	// Delete removes an identity
	Delete(ctx context.Context, identity string) error

	// Enumerate queries identities with filter
	// Backend-native filtering if supported, otherwise returns all for client-side filtering
	Enumerate(ctx context.Context, filter *Filter) ([]*Identity, error)

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

// Identity represents a registered identity with metadata
type Identity struct {
	ID           string                 `json:"identity"`
	Metadata     map[string]interface{} `json:"metadata"`
	RegisteredAt time.Time              `json:"registered_at"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	TTL          time.Duration          `json:"ttl,omitempty"`
}

// Filter represents a metadata filter expression
type Filter struct {
	// Simple equality map for POC 4 (Week 1)
	// Example: {"status": "online", "room": "engineering"}
	Conditions map[string]interface{}

	// Advanced filter AST (Week 3)
	// Will be implemented in filter package
	AST interface{} `json:"ast,omitempty"`
}

// NewFilter creates a simple equality filter
func NewFilter(conditions map[string]interface{}) *Filter {
	return &Filter{
		Conditions: conditions,
	}
}

// Matches evaluates the filter against identity metadata (client-side)
func (f *Filter) Matches(metadata map[string]interface{}) bool {
	if f == nil || len(f.Conditions) == 0 {
		return true // No filter = match all
	}

	// Simple equality matching for POC 4
	for key, expectedValue := range f.Conditions {
		actualValue, exists := metadata[key]
		if !exists {
			return false
		}
		// TODO: Type-aware comparison (Week 3)
		if actualValue != expectedValue {
			return false
		}
	}

	return true
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
