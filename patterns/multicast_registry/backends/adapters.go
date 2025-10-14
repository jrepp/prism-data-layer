package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// KeyValueRegistryAdapter adapts a KeyValueBasicInterface to RegistryBackend
type KeyValueRegistryAdapter struct {
	kv     plugin.KeyValueBasicInterface
	prefix string
}

// NewKeyValueRegistryBackend creates an adapter from a KeyValue backend to RegistryBackend
func NewKeyValueRegistryBackend(kv plugin.KeyValueBasicInterface) *KeyValueRegistryAdapter {
	return &KeyValueRegistryAdapter{
		kv:     kv,
		prefix: "multicast:registry:",
	}
}

// Set stores identity metadata with optional TTL
func (a *KeyValueRegistryAdapter) Set(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error {
	key := a.prefix + identity

	// Create identity record
	now := time.Now()
	record := map[string]interface{}{
		"id":            identity,
		"metadata":      metadata,
		"registered_at": now.Unix(),
	}

	if ttl > 0 {
		expiresAt := now.Add(ttl).Unix()
		record["expires_at"] = expiresAt
	}

	// Serialize to JSON
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	// Store in KeyValue backend
	ttlSecs := int64(0)
	if ttl > 0 {
		ttlSecs = int64(ttl.Seconds())
	}

	if err := a.kv.Set(key, data, ttlSecs); err != nil {
		return fmt.Errorf("kv set failed: %w", err)
	}

	return nil
}

// Get retrieves identity metadata
func (a *KeyValueRegistryAdapter) Get(ctx context.Context, identity string) (*Identity, error) {
	key := a.prefix + identity

	data, found, err := a.kv.Get(key)
	if err != nil {
		return nil, fmt.Errorf("kv get failed: %w", err)
	}

	if !found {
		return nil, fmt.Errorf("identity not found: %s", identity)
	}

	// Deserialize JSON
	var record map[string]interface{}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return a.parseIdentity(record)
}

// Scan returns all identities (no filtering)
func (a *KeyValueRegistryAdapter) Scan(ctx context.Context) ([]*Identity, error) {
	// KeyValueBasicInterface doesn't have scan capability
	// Return error to force use of in-memory identity tracking
	return nil, fmt.Errorf("scan not supported by KeyValueBasicInterface")
}

// Delete removes an identity
func (a *KeyValueRegistryAdapter) Delete(ctx context.Context, identity string) error {
	key := a.prefix + identity
	return a.kv.Delete(key)
}

// Enumerate queries identities with filter
func (a *KeyValueRegistryAdapter) Enumerate(ctx context.Context, filter *Filter) ([]*Identity, error) {
	// Not supported by KeyValueBasicInterface - trigger client-side filtering fallback
	return nil, fmt.Errorf("enumerate not supported by KeyValueBasicInterface (use client-side fallback)")
}

// Close closes backend connections
func (a *KeyValueRegistryAdapter) Close() error {
	// KeyValueBasicInterface doesn't have Close method
	// Rely on pattern lifecycle management
	return nil
}

// parseIdentity converts record map to Identity struct
func (a *KeyValueRegistryAdapter) parseIdentity(record map[string]interface{}) (*Identity, error) {
	identity := &Identity{
		ID: record["id"].(string),
	}

	// Parse metadata
	if metadataRaw, ok := record["metadata"]; ok {
		if metadata, ok := metadataRaw.(map[string]interface{}); ok {
			identity.Metadata = metadata
		}
	}

	// Parse registered_at timestamp
	if registeredAtRaw, ok := record["registered_at"]; ok {
		var registeredAt int64
		switch v := registeredAtRaw.(type) {
		case float64:
			registeredAt = int64(v)
		case int64:
			registeredAt = v
		}
		identity.RegisteredAt = time.Unix(registeredAt, 0)
	}

	// Parse expires_at timestamp (optional)
	if expiresAtRaw, ok := record["expires_at"]; ok {
		var expiresAt int64
		switch v := expiresAtRaw.(type) {
		case float64:
			expiresAt = int64(v)
		case int64:
			expiresAt = v
		}
		t := time.Unix(expiresAt, 0)
		identity.ExpiresAt = &t
	}

	return identity, nil
}

// PubSubMessagingAdapter adapts a PubSubInterface to MessagingBackend
type PubSubMessagingAdapter struct {
	pubsub plugin.PubSubInterface
}

// NewPubSubMessagingBackend creates an adapter from a PubSub backend to MessagingBackend
func NewPubSubMessagingBackend(pubsub plugin.PubSubInterface) *PubSubMessagingAdapter {
	return &PubSubMessagingAdapter{
		pubsub: pubsub,
	}
}

// Publish sends a message to a specific topic/identity
func (a *PubSubMessagingAdapter) Publish(ctx context.Context, topic string, payload []byte) error {
	messageID, err := a.pubsub.Publish(ctx, topic, payload, nil)
	if err != nil {
		return fmt.Errorf("pubsub publish failed: %w", err)
	}

	// Success - messageID can be used for tracking if needed
	_ = messageID

	return nil
}

// Subscribe creates a subscription for an identity (consumer-side, not used in POC 4)
func (a *PubSubMessagingAdapter) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	subscriberID := fmt.Sprintf("multicast-%d", time.Now().UnixNano())
	messageChan, err := a.pubsub.Subscribe(ctx, topic, subscriberID)
	if err != nil {
		return nil, fmt.Errorf("pubsub subscribe failed: %w", err)
	}

	// Convert plugin.PubSubMessage channel to []byte channel
	payloadChan := make(chan []byte, 100)

	go func() {
		defer close(payloadChan)
		for msg := range messageChan {
			payloadChan <- msg.Payload
		}
	}()

	return payloadChan, nil
}

// Unsubscribe removes a subscription
func (a *PubSubMessagingAdapter) Unsubscribe(ctx context.Context, topic string) error {
	// PubSubInterface unsubscribe requires subscriberID
	// For now, return success (actual cleanup handled by channel close)
	return nil
}

// Close closes backend connections
func (a *PubSubMessagingAdapter) Close() error {
	// PubSubInterface doesn't have Close method
	// Rely on pattern lifecycle management
	return nil
}

// QueueDurabilityAdapter adapts a QueueInterface to DurabilityBackend (optional)
type QueueDurabilityAdapter struct {
	queue plugin.QueueInterface
}

// NewQueueDurabilityBackend creates an adapter from a Queue backend to DurabilityBackend
func NewQueueDurabilityBackend(queue plugin.QueueInterface) *QueueDurabilityAdapter {
	return &QueueDurabilityAdapter{
		queue: queue,
	}
}

// Enqueue adds a message to a queue for an identity
func (a *QueueDurabilityAdapter) Enqueue(ctx context.Context, queue string, payload []byte) error {
	messageID, err := a.queue.Enqueue(ctx, queue, payload, nil)
	if err != nil {
		return fmt.Errorf("queue enqueue failed: %w", err)
	}

	// Success - messageID can be used for tracking if needed
	_ = messageID

	return nil
}

// Dequeue retrieves the next message from a queue
func (a *QueueDurabilityAdapter) Dequeue(ctx context.Context, queue string) ([]byte, error) {
	messageChan, err := a.queue.Receive(ctx, queue)
	if err != nil {
		return nil, fmt.Errorf("queue receive failed: %w", err)
	}

	// Wait for first message (with timeout)
	select {
	case msg, ok := <-messageChan:
		if !ok {
			// Channel closed
			return nil, nil
		}
		return msg.Payload, nil
	case <-time.After(100 * time.Millisecond):
		// No messages available within timeout
		return nil, nil
	}
}

// Acknowledge confirms message delivery
func (a *QueueDurabilityAdapter) Acknowledge(ctx context.Context, queue string, messageID string) error {
	// QueueInterface doesn't have explicit ack method
	// Assume message is removed on dequeue
	return nil
}

// Close closes backend connections
func (a *QueueDurabilityAdapter) Close() error {
	// QueueInterface doesn't have Close method
	// Rely on pattern lifecycle management
	return nil
}
