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

// ObjectStoreInterface defines operations for object/blob storage
// Used by claim check pattern to store large payloads
type ObjectStoreInterface interface {
	// Put stores an object
	Put(ctx context.Context, bucket, key string, data []byte) error

	// Get retrieves an object
	Get(ctx context.Context, bucket, key string) ([]byte, error)

	// Delete removes an object
	Delete(ctx context.Context, bucket, key string) error

	// SetTTL sets object expiration
	SetTTL(ctx context.Context, bucket, key string, ttlSeconds int) error

	// Exists checks if object exists
	Exists(ctx context.Context, bucket, key string) (bool, error)

	// GetMetadata retrieves object metadata without downloading
	GetMetadata(ctx context.Context, bucket, key string) (*ObjectMetadata, error)
}

// ObjectMetadata represents metadata about an object in storage
type ObjectMetadata struct {
	Size         int64
	ContentType  string
	LastModified int64 // Unix timestamp
	ETag         string
}

// TableWriterInterface defines operations for writing structured events to table storage
// Used by mailbox pattern storage slot to persist events with indexed headers
type TableWriterInterface interface {
	// WriteEvent stores an event with indexed headers and body
	WriteEvent(ctx context.Context, event *MailboxEvent) error

	// DeleteOldEvents removes events older than retention period
	DeleteOldEvents(ctx context.Context, olderThan int64) (int64, error)

	// GetTableStats returns storage statistics
	GetTableStats(ctx context.Context) (*TableStats, error)
}

// TableReaderInterface defines operations for reading structured events from table storage
// Used by mailbox pattern query slot to retrieve stored messages as array
type TableReaderInterface interface {
	// QueryEvents retrieves events matching filter criteria
	// Returns messages as array of MailboxEvent (header + payload)
	QueryEvents(ctx context.Context, filter *EventFilter) ([]*MailboxEvent, error)

	// GetEvent retrieves a single event by message ID
	GetEvent(ctx context.Context, messageID string) (*MailboxEvent, error)

	// GetTableStats returns storage statistics
	GetTableStats(ctx context.Context) (*TableStats, error)
}

// MailboxEvent represents a structured event for storage
type MailboxEvent struct {
	MessageID     string
	Timestamp     int64
	Topic         string
	ContentType   string
	SchemaID      string
	Encryption    string
	CorrelationID string
	Principal     string
	Namespace     string
	CustomHeaders map[string]string // x-* headers
	Body          []byte             // Opaque blob (may be encrypted)
}

// EventFilter defines query criteria for events
type EventFilter struct {
	StartTime     *int64
	EndTime       *int64
	Topics        []string
	Principals    []string
	CorrelationID *string
	Limit         int
	Offset        int
}

// TableStats provides storage metrics
type TableStats struct {
	TotalEvents    int64
	TotalSizeBytes int64
	OldestEvent    int64 // Unix timestamp
	NewestEvent    int64 // Unix timestamp
}

// NOTE: InterfaceSupport interface removed - interfaces are now declared
// at registration time via InterfaceDeclaration in the lifecycle protocol.
// See proto/prism/interfaces/lifecycle.proto for the new declaration format.
