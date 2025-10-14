package plugin

import "context"

// KeyValueBasicInterface defines the basic KeyValue operations
// Maps to prism.interfaces.keyvalue.KeyValueBasicInterface proto service
type KeyValueBasicInterface interface {
	Set(key string, value []byte, ttlSeconds int64) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) error
	Exists(key string) (bool, error)
}

// KeyValueTTLInterface defines TTL-specific operations
// Maps to prism.interfaces.keyvalue.KeyValueTTLInterface proto service
type KeyValueTTLInterface interface {
	SetWithTTL(key string, value []byte, ttlSeconds int64) error
	GetTTL(key string) (int64, error)
	UpdateTTL(key string, ttlSeconds int64) error
}

// KeyValueScanInterface defines scan/iteration operations
// Maps to prism.interfaces.keyvalue.KeyValueScanInterface proto service
type KeyValueScanInterface interface {
	Scan(prefix string, limit int) ([]string, error)
	ScanWithValues(prefix string, limit int) (map[string][]byte, error)
}

// KeyValueAtomicInterface defines atomic/CAS operations
// Maps to prism.interfaces.keyvalue.KeyValueAtomicInterface proto service
type KeyValueAtomicInterface interface {
	CompareAndSwap(key string, oldValue, newValue []byte) (bool, error)
	Increment(key string, delta int64) (int64, error)
	Decrement(key string, delta int64) (int64, error)
}

// PubSubBasicInterface defines basic pub/sub operations
// Maps to prism.interfaces.pubsub.PubSubBasicInterface proto service
type PubSubBasicInterface interface {
	Publish(topic string, payload []byte, metadata map[string]string) (string, error)
	Subscribe(topic string, subscriberID string) (<-chan *PubSubMessage, error)
	Unsubscribe(topic string, subscriberID string) error
}

// PubSubInterface defines full pub/sub operations with context support
// Extends PubSubBasicInterface with cancellation and timeout support
type PubSubInterface interface {
	Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error)
	Subscribe(ctx context.Context, topic string, subscriberID string) (<-chan *PubSubMessage, error)
	Unsubscribe(ctx context.Context, topic string, subscriberID string) error
}

// QueueInterface defines message queue operations
// For durable queues with explicit acknowledgment
type QueueInterface interface {
	Enqueue(ctx context.Context, queue string, payload []byte, metadata map[string]string) (string, error)
	Receive(ctx context.Context, queue string) (<-chan *PubSubMessage, error)
	Acknowledge(ctx context.Context, queue string, messageID string) error
	Reject(ctx context.Context, queue string, messageID string, requeue bool) error
}

// PubSubMessage represents a pub/sub or queue message
type PubSubMessage struct {
	Topic     string
	Payload   []byte
	Metadata  map[string]string
	MessageID string
	Timestamp int64
}

// NOTE: InterfaceSupport interface removed - interfaces are now declared
// at registration time via InterfaceDeclaration in the lifecycle protocol.
// See proto/prism/interfaces/lifecycle.proto for the new declaration format.
