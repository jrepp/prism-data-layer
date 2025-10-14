package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/patterns/common"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// Consumer implements a message consumer pattern using pluggable backend drivers.
// It depends only on backend interfaces, not concrete implementations.
type Consumer struct {
	name   string
	config Config

	// Backend interfaces (slots)
	messageSource interface{} // PubSubInterface or QueueInterface
	stateStore    plugin.KeyValueBasicInterface
	deadLetter    interface{} // QueueInterface (optional)
	objectStore   plugin.ObjectStoreInterface // Optional: for claim check pattern

	// Runtime state
	mu        sync.RWMutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	processor MessageProcessor
}

// MessageProcessor is a user-provided function that processes consumed messages.
type MessageProcessor func(ctx context.Context, msg *plugin.PubSubMessage) error

// ConsumerState tracks the consumer's position in the message stream.
type ConsumerState struct {
	Offset        int64     `json:"offset"`
	LastMessageID string    `json:"last_message_id"`
	LastUpdated   time.Time `json:"last_updated"`
	RetryCount    int       `json:"retry_count"`
}

// ClaimCheckMessage is an alias to the shared common package implementation.
type ClaimCheckMessage = common.ClaimCheckMessage

// New creates a new Consumer instance.
// Backend slots must be bound via BindSlots() before starting.
func New(config Config) (*Consumer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Consumer{
		name:   config.Name,
		config: config,
	}, nil
}

// BindSlots connects backend drivers to the pattern's slots.
// This is where the abstraction meets concrete implementations.
func (c *Consumer) BindSlots(
	messageSource interface{},
	stateStore plugin.KeyValueBasicInterface,
	deadLetter interface{},
	objectStore plugin.ObjectStoreInterface,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("cannot bind slots while consumer is running")
	}

	// Validate message source implements required interface
	switch messageSource.(type) {
	case plugin.PubSubInterface, plugin.QueueInterface:
		c.messageSource = messageSource
	default:
		return fmt.Errorf("message_source must implement PubSubInterface or QueueInterface")
	}

	// State store is optional (consumer will run stateless if nil)
	c.stateStore = stateStore

	// Dead letter queue is optional
	if deadLetter != nil {
		if _, ok := deadLetter.(plugin.QueueInterface); !ok {
			return fmt.Errorf("dead_letter must implement QueueInterface")
		}
		c.deadLetter = deadLetter
	}

	// Object store is optional (for claim check pattern)
	c.objectStore = objectStore

	return nil
}

// SetProcessor sets the message processing function.
func (c *Consumer) SetProcessor(processor MessageProcessor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processor = processor
}

// Start begins consuming messages.
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("consumer already running")
	}

	if c.messageSource == nil {
		return fmt.Errorf("message_source slot must be bound before starting")
	}

	if c.processor == nil {
		return fmt.Errorf("message processor must be set before starting")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running = true

	// Start consumption based on message source type
	go c.consume()

	stateful := "stateless"
	if c.stateStore != nil {
		stateful = "stateful"
	}

	slog.Info("consumer started",
		"name", c.name,
		"mode", stateful,
		"group", c.config.Behavior.ConsumerGroup,
		"topic", c.config.Behavior.Topic)

	return nil
}

// Stop stops the consumer.
func (c *Consumer) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.cancel()
	c.running = false

	slog.Info("consumer stopped", "name", c.name)
	return nil
}

// consume is the main consumption loop.
func (c *Consumer) consume() {
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	// Load consumer state
	state, err := c.loadState()
	if err != nil {
		slog.Error("failed to load consumer state", "error", err)
		return
	}

	// Subscribe based on source type
	var msgChan <-chan *plugin.PubSubMessage
	var subErr error

	if pubsub, ok := c.messageSource.(plugin.PubSubInterface); ok {
		msgChan, subErr = pubsub.Subscribe(c.ctx, c.config.Behavior.Topic, c.config.Behavior.ConsumerGroup)
	} else if queue, ok := c.messageSource.(plugin.QueueInterface); ok {
		msgChan, subErr = queue.Receive(c.ctx, c.config.Behavior.Topic)
	}

	if subErr != nil {
		slog.Error("failed to subscribe", "error", subErr)
		return
	}

	// Process messages
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-msgChan:
			if msg == nil {
				continue
			}

			if err := c.processMessage(msg, state); err != nil {
				slog.Error("failed to process message",
					"message_id", msg.MessageID,
					"error", err)

				// Handle retry logic
				if c.shouldRetry(state) {
					state.RetryCount++
					if err := c.saveState(state); err != nil {
						slog.Error("failed to save retry state", "error", err)
					}
				} else {
					// Send to dead letter queue
					if err := c.sendToDeadLetter(msg); err != nil {
						slog.Error("failed to send to dead letter queue", "error", err)
					}
					state.RetryCount = 0
				}
			} else {
				// Success - update state
				state.Offset++
				state.LastMessageID = msg.MessageID
				state.LastUpdated = time.Now()
				state.RetryCount = 0

				if c.config.Behavior.AutoCommit {
					if err := c.saveState(state); err != nil {
						slog.Error("failed to save state", "error", err)
					}
				}
			}
		}
	}
}

// processMessage processes a single message.
func (c *Consumer) processMessage(msg *plugin.PubSubMessage, state *ConsumerState) error {
	// Check if message is a claim check
	if msg.Metadata["prism-claim-check"] == "true" {
		if err := c.resolveClaimCheck(msg); err != nil {
			return fmt.Errorf("failed to resolve claim check: %w", err)
		}
	}

	processingCtx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	return c.processor(processingCtx, msg)
}

// resolveClaimCheck retrieves the actual payload from object store.
func (c *Consumer) resolveClaimCheck(msg *plugin.PubSubMessage) error {
	if c.objectStore == nil {
		return fmt.Errorf("object store not configured for claim check")
	}

	// Deserialize claim check
	var claim ClaimCheckMessage
	if err := json.Unmarshal(msg.Payload, &claim); err != nil {
		return fmt.Errorf("invalid claim check message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Download from object store
	data, err := c.objectStore.Get(ctx, claim.Bucket, claim.ObjectKey)
	if err != nil {
		return fmt.Errorf("failed to retrieve object: %w", err)
	}

	// Decompress if needed
	if claim.Compression != "none" && claim.Compression != "" {
		data, err = c.decompressPayload(data, claim.Compression)
		if err != nil {
			return fmt.Errorf("failed to decompress payload: %w", err)
		}
	}

	// Verify checksum
	checksum := common.CalculateSHA256(data)
	if checksum != claim.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", claim.Checksum, checksum)
	}

	// Replace payload with actual data
	msg.Payload = data
	if claim.ContentType != "" {
		msg.Metadata["content-type"] = claim.ContentType
	}
	delete(msg.Metadata, "prism-claim-check")

	slog.Info("resolved claim check",
		"claim_id", claim.ClaimID,
		"original_size", claim.OriginalSize,
		"retrieved_size", len(data),
		"compression", claim.Compression)

	// Delete claim after successful retrieval if configured
	if c.config.Behavior.ClaimCheck != nil && c.config.Behavior.ClaimCheck.DeleteAfterRead {
		go func() {
			deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer deleteCancel()

			if err := c.objectStore.Delete(deleteCtx, claim.Bucket, claim.ObjectKey); err != nil {
				slog.Warn("failed to delete claim after read",
					"claim_id", claim.ClaimID,
					"error", err)
			} else {
				slog.Debug("deleted claim after read", "claim_id", claim.ClaimID)
			}
		}()
	}

	return nil
}

// decompressPayload decompresses the payload using the specified algorithm.
func (c *Consumer) decompressPayload(data []byte, algorithm string) ([]byte, error) {
	switch algorithm {
	case "gzip":
		return common.DecompressGzip(data)
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", algorithm)
	}
}

// shouldRetry determines if a message should be retried.
func (c *Consumer) shouldRetry(state *ConsumerState) bool {
	return state.RetryCount < c.config.Behavior.MaxRetries
}

// sendToDeadLetter sends a message to the dead letter queue.
func (c *Consumer) sendToDeadLetter(msg *plugin.PubSubMessage) error {
	if c.deadLetter == nil {
		return nil // No dead letter queue configured
	}

	queue := c.deadLetter.(plugin.QueueInterface)
	_, err := queue.Enqueue(c.ctx, c.config.Behavior.Topic+".dlq", msg.Payload, msg.Metadata)
	return err
}

// loadState loads the consumer state from the state store.
// If stateStore is nil, returns a new empty state (stateless mode).
func (c *Consumer) loadState() (*ConsumerState, error) {
	// Stateless mode: return new state each time
	if c.stateStore == nil {
		return &ConsumerState{
			Offset:      0,
			LastUpdated: time.Now(),
			RetryCount:  0,
		}, nil
	}

	stateKey := c.stateKey()

	data, found, err := c.stateStore.Get(stateKey)
	if err != nil {
		return nil, err
	}

	if !found {
		// Initialize new state
		return &ConsumerState{
			Offset:      0,
			LastUpdated: time.Now(),
			RetryCount:  0,
		}, nil
	}

	var state ConsumerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal consumer state: %w", err)
	}

	return &state, nil
}

// saveState saves the consumer state to the state store.
// If stateStore is nil, this is a no-op (stateless mode).
func (c *Consumer) saveState(state *ConsumerState) error {
	// Stateless mode: skip persistence
	if c.stateStore == nil {
		return nil
	}

	stateKey := c.stateKey()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal consumer state: %w", err)
	}

	return c.stateStore.Set(stateKey, data, 0)
}

// stateKey generates the state storage key.
func (c *Consumer) stateKey() string {
	return fmt.Sprintf("consumer:%s:%s:%s",
		c.config.Behavior.ConsumerGroup,
		c.config.Behavior.Topic,
		c.name)
}

// Health returns the consumer's health status.
func (c *Consumer) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "consumer operating normally",
		Details: map[string]string{
			"name":    c.name,
			"group":   c.config.Behavior.ConsumerGroup,
			"topic":   c.config.Behavior.Topic,
			"running": fmt.Sprintf("%t", c.running),
		},
	}

	if !c.running {
		status.Status = plugin.HealthDegraded
		status.Message = "consumer not running"
	}

	return status, nil
}

// Name returns the consumer pattern name.
func (c *Consumer) Name() string {
	return c.name
}

// Version returns the pattern version.
func (c *Consumer) Version() string {
	return "0.1.0"
}
