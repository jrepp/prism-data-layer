package producer_test

import (
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// ProducerBackends holds the backend instances required for producer pattern tests.
type ProducerBackends struct {
	// MessageSink is where messages are published (PubSubInterface or QueueInterface)
	MessageSink interface{}

	// StateStore is for producer state (deduplication, sequencing)
	// Optional: can be nil for stateless producers
	StateStore plugin.KeyValueBasicInterface
}
