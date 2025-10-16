package memstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
)

// MemPubSub implements an in-memory pub/sub plugin
// This is useful for local testing without external dependencies
type MemPubSub struct {
	name         string
	version      string
	subscribers  sync.Map // topic -> map[subscriberID]chan *plugin.PubSubMessage
	subscriberMu sync.RWMutex
	config       *PubSubConfig
	configLock   sync.RWMutex
}

// PubSubConfig holds pub/sub-specific configuration
type PubSubConfig struct {
	BufferSize int `yaml:"buffer_size"` // Channel buffer size
}

// NewPubSub creates a new in-memory pub/sub plugin
func NewPubSub() *MemPubSub {
	return &MemPubSub{
		name:    "mempubsub",
		version: "0.1.0",
	}
}

// Name returns the plugin name
func (m *MemPubSub) Name() string {
	return m.name
}

// Version returns the plugin version
func (m *MemPubSub) Version() string {
	return m.version
}

// Initialize prepares the plugin with configuration
func (m *MemPubSub) Initialize(ctx context.Context, config *plugin.Config) error {
	// Extract backend-specific config
	var backendConfig PubSubConfig
	if err := config.GetBackendConfig(&backendConfig); err != nil {
		return fmt.Errorf("failed to parse backend config: %w", err)
	}

	// Apply defaults
	if backendConfig.BufferSize == 0 {
		backendConfig.BufferSize = 100 // Default: 100 message buffer
	}

	m.configLock.Lock()
	m.config = &backendConfig
	m.configLock.Unlock()

	return nil
}

// Start begins serving requests
func (m *MemPubSub) Start(ctx context.Context) error {
	// Nothing to start for in-memory pub/sub
	return nil
}

// Drain prepares the plugin for shutdown
func (m *MemPubSub) Drain(ctx context.Context, timeoutSeconds int32, reason string) (*plugin.DrainMetrics, error) {
	// In-memory pub/sub has no persistent connections or pending operations
	// All operations are synchronous and complete immediately
	return &plugin.DrainMetrics{
		DrainedOperations: 0,
		AbortedOperations: 0,
	}, nil
}

// Stop gracefully shuts down the plugin
func (m *MemPubSub) Stop(ctx context.Context) error {
	// Close all subscriber channels
	m.subscribers.Range(func(topic, subs interface{}) bool {
		if subMap, ok := subs.(*sync.Map); ok {
			subMap.Range(func(subID, ch interface{}) bool {
				if channel, ok := ch.(chan *plugin.PubSubMessage); ok {
					close(channel)
				}
				return true
			})
		}
		return true
	})

	return nil
}

// Health returns the plugin health status
func (m *MemPubSub) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	topicCount := 0
	subscriberCount := 0

	m.subscribers.Range(func(topic, subs interface{}) bool {
		topicCount++
		if subMap, ok := subs.(*sync.Map); ok {
			subMap.Range(func(subID, ch interface{}) bool {
				subscriberCount++
				return true
			})
		}
		return true
	})

	return &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: fmt.Sprintf("healthy, %d topics, %d subscribers", topicCount, subscriberCount),
		Details: map[string]string{
			"topics":      fmt.Sprintf("%d", topicCount),
			"subscribers": fmt.Sprintf("%d", subscriberCount),
		},
	}, nil
}

// Publish sends a message to all subscribers of a topic
func (m *MemPubSub) Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	messageID := uuid.New().String()

	msg := &plugin.PubSubMessage{
		MessageID: messageID,
		Topic:     topic,
		Payload:   payload,
		Metadata:  metadata,
		Timestamp: time.Now().UnixMilli(),
	}

	// Get subscribers for this topic
	if subs, ok := m.subscribers.Load(topic); ok {
		if subMap, ok := subs.(*sync.Map); ok {
			// Send to all subscribers
			subMap.Range(func(subID, ch interface{}) bool {
				if channel, ok := ch.(chan *plugin.PubSubMessage); ok {
					// Non-blocking send to avoid deadlocks
					select {
					case channel <- msg:
					default:
						// Channel full, skip this subscriber
						// In production, this would be logged
					}
				}
				return true
			})
		}
	}

	return messageID, nil
}

// Subscribe creates a subscription to a topic
func (m *MemPubSub) Subscribe(ctx context.Context, topic string, subscriberID string) (<-chan *plugin.PubSubMessage, error) {
	m.configLock.RLock()
	bufferSize := 100 // Default
	if m.config != nil {
		bufferSize = m.config.BufferSize
	}
	m.configLock.RUnlock()

	// Create channel for this subscriber
	ch := make(chan *plugin.PubSubMessage, bufferSize)

	// Get or create subscriber map for this topic
	subMap, _ := m.subscribers.LoadOrStore(topic, &sync.Map{})

	// Add subscriber
	if sm, ok := subMap.(*sync.Map); ok {
		sm.Store(subscriberID, ch)
	}

	return ch, nil
}

// Unsubscribe removes a subscription
func (m *MemPubSub) Unsubscribe(ctx context.Context, topic string, subscriberID string) error {
	if subs, ok := m.subscribers.Load(topic); ok {
		if subMap, ok := subs.(*sync.Map); ok {
			if ch, ok := subMap.Load(subscriberID); ok {
				if channel, ok := ch.(chan *plugin.PubSubMessage); ok {
					close(channel)
				}
				subMap.Delete(subscriberID)
			}
		}
	}

	return nil
}

// Compile-time interface compliance checks
var (
	_ plugin.Plugin          = (*MemPubSub)(nil) // Core plugin interface
	_ plugin.PubSubInterface = (*MemPubSub)(nil) // PubSub operations
)

// GetInterfaceDeclarations returns the interfaces this driver implements
func (m *MemPubSub) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	return []*pb.InterfaceDeclaration{
		{
			Name:      "PubSubInterface",
			ProtoFile: "prism/interfaces/pubsub/pubsub.proto",
			Version:   "v1",
		},
	}
}
