package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jrepp/prism-data-layer/pkg/patterns/common"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// Producer implements a message producer pattern using pluggable backend drivers.
// It depends only on backend interfaces, not concrete implementations.
type Producer struct {
	name   string
	config Config

	// Backend interfaces (slots)
	messageSink interface{} // PubSubInterface or QueueInterface
	stateStore  plugin.KeyValueBasicInterface
	objectStore plugin.ObjectStoreInterface // Optional: for claim check pattern

	// Runtime state
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Batching
	batchMu   sync.Mutex
	batch     []*Message
	batchTime time.Time

	// Metrics
	metrics ProducerMetrics
}

// Message represents a message to be published.
type Message struct {
	Topic    string
	Payload  []byte
	Metadata map[string]string
	ID       string // Optional: for deduplication

	// Internal fields
	retryCount int
	enqueued   time.Time
}

// ProducerMetrics tracks producer statistics.
type ProducerMetrics struct {
	mu                  sync.RWMutex
	MessagesPublished   int64
	MessagesFailed      int64
	MessagesDedup       int64 // Deduplicated messages
	BytesPublished      int64
	BatchesPublished    int64
	PublishLatencyP50   time.Duration
	PublishLatencyP99   time.Duration
	LastPublishTime     time.Time
	LastPublishDuration time.Duration
}

// ProducerState tracks producer state for deduplication and sequencing.
type ProducerState struct {
	MessageIDs    map[string]time.Time `json:"message_ids"`    // Deduplication cache
	SequenceNum   int64                `json:"sequence_num"`   // Monotonic sequence number
	LastPublished time.Time            `json:"last_published"` // Last successful publish
}

// ClaimCheckMessage is an alias to the shared common package implementation.
type ClaimCheckMessage = common.ClaimCheckMessage

// New creates a new Producer instance.
// Backend slots must be bound via BindSlots() before publishing.
func New(config Config) (*Producer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Producer{
		name:      config.Name,
		config:    config,
		batch:     make([]*Message, 0, config.Behavior.BatchSize),
		batchTime: time.Now(),
	}, nil
}

// BindSlots connects backend drivers to the pattern's slots.
// This is where the abstraction meets concrete implementations.
func (p *Producer) BindSlots(
	messageSink interface{},
	stateStore plugin.KeyValueBasicInterface,
	objectStore plugin.ObjectStoreInterface,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("cannot bind slots while producer is running")
	}

	// Validate message sink implements required interface
	switch messageSink.(type) {
	case plugin.PubSubInterface, plugin.QueueInterface:
		p.messageSink = messageSink
	default:
		return fmt.Errorf("message_sink must implement PubSubInterface or QueueInterface")
	}

	// State store is optional (producer will run stateless if nil)
	p.stateStore = stateStore

	// Object store is optional (for claim check pattern)
	p.objectStore = objectStore

	return nil
}

// Start begins the producer lifecycle (background batch flusher if enabled).
func (p *Producer) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("producer already running")
	}

	if p.messageSink == nil {
		return fmt.Errorf("message_sink slot must be bound before starting")
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.running = true

	// Start background batch flusher if batching is enabled
	if p.config.Behavior.BatchSize > 0 {
		go p.batchFlusher()
	}

	stateful := "stateless"
	if p.stateStore != nil {
		stateful = "stateful"
	}

	slog.Info("producer started",
		"name", p.name,
		"mode", stateful,
		"batch_size", p.config.Behavior.BatchSize)

	return nil
}

// Stop stops the producer, flushing any pending batched messages.
func (p *Producer) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	// Flush any pending batch
	if err := p.flushBatch(); err != nil {
		slog.Warn("failed to flush batch on stop", "error", err)
	}

	p.cancel()
	p.running = false

	slog.Info("producer stopped", "name", p.name,
		"published", p.metrics.MessagesPublished,
		"failed", p.metrics.MessagesFailed,
		"deduped", p.metrics.MessagesDedup)

	return nil
}

// Publish publishes a single message.
func (p *Producer) Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) error {
	return p.PublishWithID(ctx, topic, payload, metadata, "")
}

// PublishWithID publishes a message with an explicit ID for deduplication.
func (p *Producer) PublishWithID(ctx context.Context, topic string, payload []byte, metadata map[string]string, id string) error {
	p.mu.RLock()
	running := p.running
	p.mu.RUnlock()

	if !running {
		return fmt.Errorf("producer not running")
	}

	// Generate ID if not provided
	if id == "" && p.config.Behavior.Deduplication {
		id = p.generateMessageID(topic, payload)
	}

	// Check for duplicates
	if id != "" && p.config.Behavior.Deduplication {
		isDup, err := p.isDuplicate(id)
		if err != nil {
			return fmt.Errorf("deduplication check failed: %w", err)
		}
		if isDup {
			p.metrics.mu.Lock()
			p.metrics.MessagesDedup++
			p.metrics.mu.Unlock()
			slog.Debug("duplicate message skipped", "message_id", id)
			return nil // Silently skip duplicate
		}
	}

	msg := &Message{
		Topic:    topic,
		Payload:  payload,
		Metadata: metadata,
		ID:       id,
		enqueued: time.Now(),
	}

	// Batch or publish immediately
	if p.config.Behavior.BatchSize > 0 {
		return p.addToBatch(msg)
	}

	return p.publishMessage(ctx, msg)
}

// PublishBatch publishes multiple messages as a batch.
func (p *Producer) PublishBatch(ctx context.Context, messages []*Message) error {
	p.mu.RLock()
	running := p.running
	p.mu.RUnlock()

	if !running {
		return fmt.Errorf("producer not running")
	}

	if len(messages) == 0 {
		return nil
	}

	// Deduplicate messages in batch
	if p.config.Behavior.Deduplication {
		filtered := make([]*Message, 0, len(messages))
		for _, msg := range messages {
			if msg.ID == "" {
				msg.ID = p.generateMessageID(msg.Topic, msg.Payload)
			}

			isDup, err := p.isDuplicate(msg.ID)
			if err != nil {
				return fmt.Errorf("deduplication check failed: %w", err)
			}

			if !isDup {
				filtered = append(filtered, msg)
			} else {
				p.metrics.mu.Lock()
				p.metrics.MessagesDedup++
				p.metrics.mu.Unlock()
			}
		}
		messages = filtered
	}

	if len(messages) == 0 {
		return nil // All messages were duplicates
	}

	startTime := time.Now()

	// Publish based on sink type
	if pubsub, ok := p.messageSink.(plugin.PubSubInterface); ok {
		for _, msg := range messages {
			if _, err := pubsub.Publish(ctx, msg.Topic, msg.Payload, msg.Metadata); err != nil {
				slog.Error("failed to publish message in batch",
					"message_id", msg.ID,
					"topic", msg.Topic,
					"error", err)
				p.metrics.mu.Lock()
				p.metrics.MessagesFailed++
				p.metrics.mu.Unlock()
				return err
			}
		}
	} else if queue, ok := p.messageSink.(plugin.QueueInterface); ok {
		for _, msg := range messages {
			if _, err := queue.Enqueue(ctx, msg.Topic, msg.Payload, msg.Metadata); err != nil {
				slog.Error("failed to enqueue message in batch",
					"message_id", msg.ID,
					"topic", msg.Topic,
					"error", err)
				p.metrics.mu.Lock()
				p.metrics.MessagesFailed++
				p.metrics.mu.Unlock()
				return err
			}
		}
	}

	duration := time.Since(startTime)

	// Update metrics
	p.metrics.mu.Lock()
	p.metrics.MessagesPublished += int64(len(messages))
	p.metrics.BatchesPublished++
	p.metrics.LastPublishTime = time.Now()
	p.metrics.LastPublishDuration = duration
	for _, msg := range messages {
		p.metrics.BytesPublished += int64(len(msg.Payload))
	}
	p.metrics.mu.Unlock()

	// Record message IDs for deduplication
	if p.config.Behavior.Deduplication {
		for _, msg := range messages {
			if err := p.recordMessageID(msg.ID); err != nil {
				slog.Warn("failed to record message ID", "message_id", msg.ID, "error", err)
			}
		}
	}

	slog.Debug("batch published",
		"count", len(messages),
		"duration", duration,
		"bytes", p.metrics.BytesPublished)

	return nil
}

// Flush flushes any pending batched messages immediately.
func (p *Producer) Flush(ctx context.Context) error {
	return p.flushBatch()
}

// publishMessage publishes a single message immediately.
func (p *Producer) publishMessage(ctx context.Context, msg *Message) error {
	startTime := time.Now()

	// Check if claim check should be used
	payload := msg.Payload
	metadata := msg.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}

	if p.shouldUseClaimCheck(msg.Payload) {
		claimPayload, claimMetadata, err := p.createClaimCheck(ctx, msg)
		if err != nil {
			return fmt.Errorf("failed to create claim check: %w", err)
		}
		payload = claimPayload
		metadata = claimMetadata
	}

	// Retry logic
	var err error
	for attempt := 0; attempt <= p.config.Behavior.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * p.config.Behavior.RetryBackoffDuration()
			slog.Debug("retrying publish", "attempt", attempt, "backoff", backoff)
			time.Sleep(backoff)
		}

		// Publish based on sink type
		if pubsub, ok := p.messageSink.(plugin.PubSubInterface); ok {
			_, err = pubsub.Publish(ctx, msg.Topic, payload, metadata)
		} else if queue, ok := p.messageSink.(plugin.QueueInterface); ok {
			_, err = queue.Enqueue(ctx, msg.Topic, payload, metadata)
		}

		if err == nil {
			break // Success
		}

		slog.Warn("publish attempt failed",
			"attempt", attempt,
			"message_id", msg.ID,
			"topic", msg.Topic,
			"error", err)
	}

	duration := time.Since(startTime)

	if err != nil {
		p.metrics.mu.Lock()
		p.metrics.MessagesFailed++
		p.metrics.mu.Unlock()
		return fmt.Errorf("publish failed after %d retries: %w", p.config.Behavior.MaxRetries, err)
	}

	// Update metrics
	p.metrics.mu.Lock()
	p.metrics.MessagesPublished++
	p.metrics.BytesPublished += int64(len(msg.Payload))
	p.metrics.LastPublishTime = time.Now()
	p.metrics.LastPublishDuration = duration
	p.metrics.mu.Unlock()

	// Record message ID for deduplication
	if msg.ID != "" && p.config.Behavior.Deduplication {
		if err := p.recordMessageID(msg.ID); err != nil {
			slog.Warn("failed to record message ID", "message_id", msg.ID, "error", err)
		}
	}

	slog.Debug("message published",
		"message_id", msg.ID,
		"topic", msg.Topic,
		"duration", duration,
		"bytes", len(msg.Payload))

	return nil
}

// shouldUseClaimCheck determines if claim check should be used for this payload.
func (p *Producer) shouldUseClaimCheck(payload []byte) bool {
	if p.config.Behavior.ClaimCheck == nil || !p.config.Behavior.ClaimCheck.Enabled {
		return false
	}

	if p.objectStore == nil {
		return false
	}

	return int64(len(payload)) > p.config.Behavior.ClaimCheck.Threshold
}

// createClaimCheck uploads payload to object store and returns claim check message.
func (p *Producer) createClaimCheck(ctx context.Context, msg *Message) ([]byte, map[string]string, error) {
	if p.objectStore == nil {
		return nil, nil, fmt.Errorf("object store not configured")
	}

	cfg := p.config.Behavior.ClaimCheck

	// Compress if configured
	data := msg.Payload
	compression := "none"
	if cfg.Compression != "" && cfg.Compression != "none" {
		compressed, err := p.compressPayload(msg.Payload, cfg.Compression)
		if err != nil {
			slog.Warn("compression failed, using uncompressed payload", "error", err)
		} else {
			data = compressed
			compression = cfg.Compression
		}
	}

	// Generate claim ID and object key
	claimID := uuid.New().String()
	objectKey := fmt.Sprintf("%s/%s/%s", p.name, msg.Topic, claimID)

	// Upload to object store
	if err := p.objectStore.Put(ctx, cfg.Bucket, objectKey, data); err != nil {
		return nil, nil, fmt.Errorf("failed to upload to object store: %w", err)
	}

	// Set TTL if configured
	if cfg.TTL > 0 {
		if err := p.objectStore.SetTTL(ctx, cfg.Bucket, objectKey, cfg.TTL); err != nil {
			slog.Warn("failed to set object TTL", "error", err)
		}
	}

	// Calculate checksum
	checksum := common.CalculateSHA256(msg.Payload)

	// Create claim check message
	claim := ClaimCheckMessage{
		ClaimID:      claimID,
		Bucket:       cfg.Bucket,
		ObjectKey:    objectKey,
		OriginalSize: len(msg.Payload),
		Compression:  compression,
		ContentType:  msg.Metadata["content-type"],
		Checksum:     checksum,
	}

	if cfg.TTL > 0 {
		claim.ExpiresAt = time.Now().Add(time.Duration(cfg.TTL) * time.Second).Unix()
	}

	claimPayload, err := json.Marshal(claim)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal claim check: %w", err)
	}

	// Add claim check metadata
	metadata := make(map[string]string)
	for k, v := range msg.Metadata {
		metadata[k] = v
	}
	metadata["prism-claim-check"] = "true"

	slog.Info("created claim check",
		"claim_id", claimID,
		"original_size", len(msg.Payload),
		"compressed_size", len(data),
		"compression", compression,
		"bucket", cfg.Bucket,
		"object_key", objectKey)

	return claimPayload, metadata, nil
}

// compressPayload compresses the payload using the specified algorithm.
func (p *Producer) compressPayload(payload []byte, algorithm string) ([]byte, error) {
	switch algorithm {
	case "gzip":
		return common.CompressGzip(payload)
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", algorithm)
	}
}

// addToBatch adds a message to the current batch.
func (p *Producer) addToBatch(msg *Message) error {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()

	p.batch = append(p.batch, msg)

	// Flush batch if size threshold reached
	if len(p.batch) >= p.config.Behavior.BatchSize {
		return p.flushBatchLocked()
	}

	return nil
}

// batchFlusher periodically flushes batched messages.
func (p *Producer) batchFlusher() {
	ticker := time.NewTicker(p.config.Behavior.BatchIntervalDuration())
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if err := p.flushBatch(); err != nil {
				slog.Error("batch flush failed", "error", err)
			}
		}
	}
}

// flushBatch flushes the current batch.
func (p *Producer) flushBatch() error {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()

	return p.flushBatchLocked()
}

// flushBatchLocked flushes the batch (caller must hold batchMu).
func (p *Producer) flushBatchLocked() error {
	if len(p.batch) == 0 {
		return nil
	}

	messages := p.batch
	p.batch = make([]*Message, 0, p.config.Behavior.BatchSize)
	p.batchTime = time.Now()

	ctx := p.ctx
	if ctx.Err() != nil {
		ctx = context.Background() // Use background context if producer context is done
	}

	return p.PublishBatch(ctx, messages)
}

// generateMessageID generates a deterministic message ID based on topic and payload.
func (p *Producer) generateMessageID(topic string, payload []byte) string {
	return common.CalculateSHA256(append([]byte(topic), payload...))
}

// isDuplicate checks if a message ID has been seen recently.
func (p *Producer) isDuplicate(messageID string) (bool, error) {
	if p.stateStore == nil {
		return false, nil // Stateless mode: no deduplication
	}

	dedupKey := fmt.Sprintf("producer:%s:dedup:%s", p.name, messageID)

	data, found, err := p.stateStore.Get(dedupKey)
	if err != nil {
		return false, err
	}

	if found {
		// Check expiration
		var ts time.Time
		if err := json.Unmarshal(data, &ts); err != nil {
			return false, err
		}

		if time.Since(ts) < p.config.Behavior.DeduplicationWindow() {
			return true, nil // Duplicate within window
		}
	}

	return false, nil
}

// recordMessageID records a message ID for deduplication.
func (p *Producer) recordMessageID(messageID string) error {
	if p.stateStore == nil {
		return nil // Stateless mode: no recording
	}

	dedupKey := fmt.Sprintf("producer:%s:dedup:%s", p.name, messageID)
	now := time.Now()

	data, err := json.Marshal(now)
	if err != nil {
		return err
	}

	// Store with TTL = deduplication window
	ttl := int64(p.config.Behavior.DeduplicationWindow().Seconds())
	return p.stateStore.Set(dedupKey, data, ttl)
}

// Metrics returns the producer's metrics.
func (p *Producer) Metrics() ProducerMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	return p.metrics
}

// Health returns the producer's health status.
func (p *Producer) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "producer operating normally",
		Details: map[string]string{
			"name":      p.name,
			"running":   fmt.Sprintf("%t", p.running),
			"published": fmt.Sprintf("%d", p.metrics.MessagesPublished),
			"failed":    fmt.Sprintf("%d", p.metrics.MessagesFailed),
			"deduped":   fmt.Sprintf("%d", p.metrics.MessagesDedup),
		},
	}

	if !p.running {
		status.Status = plugin.HealthDegraded
		status.Message = "producer not running"
	}

	if p.metrics.MessagesFailed > p.metrics.MessagesPublished/10 {
		status.Status = plugin.HealthDegraded
		status.Message = fmt.Sprintf("high failure rate: %d failures out of %d published",
			p.metrics.MessagesFailed, p.metrics.MessagesPublished)
	}

	return status, nil
}

// Name returns the producer pattern name.
func (p *Producer) Name() string {
	return p.name
}

// Version returns the pattern version.
func (p *Producer) Version() string {
	return "0.1.0"
}
