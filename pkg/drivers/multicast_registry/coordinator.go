package multicast_registry

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Coordinator implements the Multicast Registry pattern
type Coordinator struct {
	config     *Config
	registry   RegistryBackend
	messaging  MessagingBackend
	durability DurabilityBackend // optional

	// Internal state for identity tracking
	identities map[string]*Identity
	mu         sync.RWMutex

	// Cleanup goroutine control
	stopCleanup chan struct{}
	wg          sync.WaitGroup
}

// NewCoordinator creates a new multicast registry coordinator
func NewCoordinator(
	config *Config,
	registry RegistryBackend,
	messaging MessagingBackend,
	durability DurabilityBackend,
) (*Coordinator, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if registry == nil {
		return nil, fmt.Errorf("registry backend is required")
	}

	if messaging == nil {
		return nil, fmt.Errorf("messaging backend is required")
	}

	c := &Coordinator{
		config:      config,
		registry:    registry,
		messaging:   messaging,
		durability:  durability,
		identities:  make(map[string]*Identity),
		stopCleanup: make(chan struct{}),
	}

	// Start TTL cleanup goroutine
	c.wg.Add(1)
	go c.cleanupExpiredIdentities()

	return c, nil
}

// Register stores an identity with metadata and optional TTL
func (c *Coordinator) Register(ctx context.Context, identity string, metadata map[string]interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if identity already exists
	if _, exists := c.identities[identity]; exists {
		return fmt.Errorf("identity %s already registered", identity)
	}

	// Check max identities limit
	if c.config.MaxIdentities > 0 && len(c.identities) >= c.config.MaxIdentities {
		return fmt.Errorf("max identities limit reached (%d)", c.config.MaxIdentities)
	}

	// Create identity record
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

	// Store in registry backend
	if err := c.registry.Set(ctx, identity, metadata, ttl); err != nil {
		return fmt.Errorf("registry backend set failed: %w", err)
	}

	// Track in local state
	c.identities[identity] = id

	return nil
}

// Enumerate returns identities matching the filter
func (c *Coordinator) Enumerate(ctx context.Context, filter *Filter) ([]*Identity, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try backend-native filtering first
	results, err := c.registry.Enumerate(ctx, filter)
	if err == nil && results != nil {
		// Backend supports native filtering
		return results, nil
	}

	// Fallback to client-side filtering
	var matches []*Identity

	for _, identity := range c.identities {
		// Skip expired identities
		if identity.ExpiresAt != nil && time.Now().After(*identity.ExpiresAt) {
			continue
		}

		// Apply filter
		if filter == nil || filter.Matches(identity.Metadata) {
			matches = append(matches, identity)
		}
	}

	return matches, nil
}

// Multicast sends a message to all identities matching the filter
func (c *Coordinator) Multicast(ctx context.Context, filter *Filter, payload []byte) (*MulticastResponse, error) {
	// 1. Enumerate target identities
	targets, err := c.Enumerate(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("enumerate failed: %w", err)
	}

	response := &MulticastResponse{
		TargetCount: len(targets),
		Results:     make([]*DeliveryResult, 0, len(targets)),
	}

	if len(targets) == 0 {
		return response, nil
	}

	// 2. Fan out to messaging backend (parallel delivery)
	var wg sync.WaitGroup
	resultsChan := make(chan *DeliveryResult, len(targets))

	for _, identity := range targets {
		wg.Add(1)
		go func(id *Identity) {
			defer wg.Done()

			start := time.Now()
			topic := c.identityTopic(id.ID)

			// Attempt delivery with retries
			var lastErr error
			for attempt := 0; attempt <= c.config.Messaging.RetryAttempts; attempt++ {
				if attempt > 0 {
					time.Sleep(c.config.Messaging.RetryDelay)
				}

				err := c.messaging.Publish(ctx, topic, payload)
				if err == nil {
					resultsChan <- &DeliveryResult{
						Identity: id.ID,
						Status:   DeliveryStatusDelivered,
						Latency:  time.Since(start),
					}
					return
				}
				lastErr = err
			}

			// All retries failed
			resultsChan <- &DeliveryResult{
				Identity: id.ID,
				Status:   DeliveryStatusFailed,
				Error:    lastErr,
				Latency:  time.Since(start),
			}
		}(identity)
	}

	// Wait for all deliveries to complete
	wg.Wait()
	close(resultsChan)

	// 3. Aggregate results
	for result := range resultsChan {
		response.Results = append(response.Results, result)
		if result.Status == DeliveryStatusDelivered {
			response.DeliveredCount++
		} else {
			response.FailedCount++
		}
	}

	return response, nil
}

// Unregister removes an identity
func (c *Coordinator) Unregister(ctx context.Context, identity string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if exists
	if _, exists := c.identities[identity]; !exists {
		// Idempotent: succeeds even if doesn't exist
		return nil
	}

	// Remove from registry backend
	if err := c.registry.Delete(ctx, identity); err != nil {
		return fmt.Errorf("registry backend delete failed: %w", err)
	}

	// Remove from local state
	delete(c.identities, identity)

	return nil
}

// Close closes all backend connections and stops background goroutines
func (c *Coordinator) Close() error {
	// Stop cleanup goroutine
	close(c.stopCleanup)
	c.wg.Wait()

	// Close backends
	var errs []error

	if err := c.registry.Close(); err != nil {
		errs = append(errs, fmt.Errorf("registry close: %w", err))
	}

	if err := c.messaging.Close(); err != nil {
		errs = append(errs, fmt.Errorf("messaging close: %w", err))
	}

	if c.durability != nil {
		if err := c.durability.Close(); err != nil {
			errs = append(errs, fmt.Errorf("durability close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// identityTopic generates the messaging topic for an identity
func (c *Coordinator) identityTopic(identity string) string {
	prefix := c.config.Messaging.TopicPrefix
	if prefix == "" {
		prefix = "prism.multicast."
	}
	return prefix + identity
}

// cleanupExpiredIdentities runs in background to remove expired identities
func (c *Coordinator) cleanupExpiredIdentities() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performCleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// performCleanup removes expired identities
func (c *Coordinator) performCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expired []string

	for id, identity := range c.identities {
		if identity.ExpiresAt != nil && now.After(*identity.ExpiresAt) {
			expired = append(expired, id)
		}
	}

	// Remove expired identities
	ctx := context.Background()
	for _, id := range expired {
		c.registry.Delete(ctx, id)
		delete(c.identities, id)
	}

	if len(expired) > 0 {
		// Log cleanup (in production, use structured logger)
		fmt.Printf("Cleaned up %d expired identities\n", len(expired))
	}
}

// MulticastResponse contains the result of a multicast operation
type MulticastResponse struct {
	TargetCount    int               `json:"target_count"`
	DeliveredCount int               `json:"delivered_count"`
	FailedCount    int               `json:"failed_count"`
	Results        []*DeliveryResult `json:"results"`
}
