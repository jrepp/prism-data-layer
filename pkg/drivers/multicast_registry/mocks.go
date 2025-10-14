package multicast_registry

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockRegistryBackend is an in-memory registry backend for testing
type MockRegistryBackend struct {
	data map[string]*Identity
	mu   sync.RWMutex
}

// NewMockRegistryBackend creates a new mock registry backend
func NewMockRegistryBackend() *MockRegistryBackend {
	return &MockRegistryBackend{
		data: make(map[string]*Identity),
	}
}

func (m *MockRegistryBackend) Set(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	id := &Identity{
		ID:           identity,
		Metadata:     metadata,
		RegisteredAt: now,
		TTL:          ttl,
	}

	if ttl > 0 {
		expiresAt := now.Add(ttl)
		id.ExpiresAt = &expiresAt
	}

	m.data[identity] = id
	return nil
}

func (m *MockRegistryBackend) Get(ctx context.Context, identity string) (*Identity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, exists := m.data[identity]
	if !exists {
		return nil, fmt.Errorf("identity not found: %s", identity)
	}

	// Check expiration
	if id.ExpiresAt != nil && time.Now().After(*id.ExpiresAt) {
		return nil, fmt.Errorf("identity expired: %s", identity)
	}

	return id, nil
}

func (m *MockRegistryBackend) Scan(ctx context.Context) ([]*Identity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*Identity
	now := time.Now()

	for _, id := range m.data {
		// Skip expired
		if id.ExpiresAt != nil && now.After(*id.ExpiresAt) {
			continue
		}
		results = append(results, id)
	}

	return results, nil
}

func (m *MockRegistryBackend) Delete(ctx context.Context, identity string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, identity)
	return nil
}

func (m *MockRegistryBackend) Enumerate(ctx context.Context, filter *Filter) ([]*Identity, error) {
	// Mock backend doesn't support native filtering
	// Return nil to trigger client-side filtering fallback
	return nil, fmt.Errorf("mock backend: native filtering not supported")
}

func (m *MockRegistryBackend) Close() error {
	return nil
}

// MockMessagingBackend is an in-memory messaging backend for testing
type MockMessagingBackend struct {
	published map[string][][]byte // topic -> messages
	mu        sync.RWMutex
}

// NewMockMessagingBackend creates a new mock messaging backend
func NewMockMessagingBackend() *MockMessagingBackend {
	return &MockMessagingBackend{
		published: make(map[string][][]byte),
	}
}

func (m *MockMessagingBackend) Publish(ctx context.Context, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.published[topic] = append(m.published[topic], payload)
	return nil
}

func (m *MockMessagingBackend) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	// Not implemented for POC 4 (consumer-side only)
	return nil, fmt.Errorf("subscribe not implemented in mock")
}

func (m *MockMessagingBackend) Unsubscribe(ctx context.Context, topic string) error {
	return nil
}

func (m *MockMessagingBackend) Close() error {
	return nil
}

// GetPublished returns all messages published to a topic (test helper)
func (m *MockMessagingBackend) GetPublished(topic string) [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.published[topic]
}

// GetPublishedCount returns the number of messages published to a topic (test helper)
func (m *MockMessagingBackend) GetPublishedCount(topic string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.published[topic])
}

// MockRegistryBackendWithCloseError fails on Close
type MockRegistryBackendWithCloseError struct {
	*MockRegistryBackend
}

func NewMockRegistryBackendWithCloseError() *MockRegistryBackendWithCloseError {
	return &MockRegistryBackendWithCloseError{
		MockRegistryBackend: NewMockRegistryBackend(),
	}
}

func (m *MockRegistryBackendWithCloseError) Close() error {
	return fmt.Errorf("mock: Close failed intentionally")
}

// MockRegistryBackendWithSetError fails on Set
type MockRegistryBackendWithSetError struct {
	*MockRegistryBackend
}

func NewMockRegistryBackendWithSetError() *MockRegistryBackendWithSetError {
	return &MockRegistryBackendWithSetError{
		MockRegistryBackend: NewMockRegistryBackend(),
	}
}

func (m *MockRegistryBackendWithSetError) Set(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error {
	return fmt.Errorf("mock: Set failed intentionally")
}

// MockMessagingBackendWithFailures fails first N publish attempts
type MockMessagingBackendWithFailures struct {
	*MockMessagingBackend
	failuresRemaining int
	mu                sync.Mutex
}

func NewMockMessagingBackendWithFailures(failures int) *MockMessagingBackendWithFailures {
	return &MockMessagingBackendWithFailures{
		MockMessagingBackend: NewMockMessagingBackend(),
		failuresRemaining:    failures,
	}
}

func (m *MockMessagingBackendWithFailures) Publish(ctx context.Context, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failuresRemaining > 0 {
		m.failuresRemaining--
		return fmt.Errorf("mock: Publish failed (remaining failures: %d)", m.failuresRemaining)
	}

	// After failures exhausted, delegate to parent
	return m.MockMessagingBackend.Publish(ctx, topic, payload)
}
