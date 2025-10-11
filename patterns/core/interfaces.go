package core

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

// PubSubMessage represents a pub/sub message
type PubSubMessage struct {
	Topic     string
	Payload   []byte
	Metadata  map[string]string
	MessageID string
	Timestamp int64
}

// InterfaceSupport provides a way to query which interfaces a backend supports
type InterfaceSupport interface {
	// SupportsInterface returns true if the backend implements the named interface
	SupportsInterface(interfaceName string) bool

	// ListInterfaces returns all interface names this backend implements
	ListInterfaces() []string
}
