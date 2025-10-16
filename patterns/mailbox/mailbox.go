package mailbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// Mailbox implements a searchable event store pattern by consuming messages
// from a queue and storing them in a structured database with indexed headers.
type Mailbox struct {
	name   string
	config Config

	// Backend interfaces (slots)
	messageSource interface{}                   // PubSubInterface or QueueInterface
	tableWriter   plugin.TableWriterInterface   // Storage backend for writing events
	tableReader   plugin.TableReaderInterface   // Query interface for reading events

	// Runtime state
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	metrics MailboxMetrics
}

// MailboxMetrics tracks mailbox performance metrics.
type MailboxMetrics struct {
	mu                sync.RWMutex
	EventsReceived    int64
	EventsStored      int64
	EventsFailed      int64
	BytesStored       int64
	LastEventTime     time.Time
	ProcessingLatency time.Duration
}

// New creates a new Mailbox instance.
// Backend slots must be bound via BindSlots() before starting.
func New(config Config) (*Mailbox, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Mailbox{
		name:   config.Name,
		config: config,
	}, nil
}

// BindSlots connects backend drivers to the pattern's slots.
func (m *Mailbox) BindSlots(
	messageSource interface{},
	tableWriter plugin.TableWriterInterface,
	tableReader plugin.TableReaderInterface,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("cannot bind slots while mailbox is running")
	}

	// Validate message source implements required interface
	switch messageSource.(type) {
	case plugin.PubSubInterface, plugin.QueueInterface:
		m.messageSource = messageSource
	default:
		return fmt.Errorf("message_source must implement PubSubInterface or QueueInterface")
	}

	// Table writer is required
	if tableWriter == nil {
		return fmt.Errorf("table_writer slot is required")
	}
	m.tableWriter = tableWriter

	// Table reader is optional (mailbox can run write-only)
	m.tableReader = tableReader

	return nil
}

// Start begins consuming messages and storing them.
func (m *Mailbox) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("mailbox already running")
	}

	if m.messageSource == nil {
		return fmt.Errorf("message_source slot must be bound before starting")
	}

	if m.tableWriter == nil {
		return fmt.Errorf("table_writer slot must be bound before starting")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running = true

	// Start consumption
	go m.consume()

	slog.Info("mailbox started",
		"name", m.name,
		"topic", m.config.Behavior.Topic,
		"database", m.config.Storage.DatabasePath)

	return nil
}

// Stop stops the mailbox.
func (m *Mailbox) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.cancel()
	m.running = false

	slog.Info("mailbox stopped", "name", m.name)
	return nil
}

// consume is the main consumption loop.
func (m *Mailbox) consume() {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	// Subscribe based on source type
	var msgChan <-chan *plugin.PubSubMessage
	var subErr error

	if pubsub, ok := m.messageSource.(plugin.PubSubInterface); ok {
		msgChan, subErr = pubsub.Subscribe(m.ctx, m.config.Behavior.Topic, m.config.Behavior.ConsumerGroup)
	} else if queue, ok := m.messageSource.(plugin.QueueInterface); ok {
		msgChan, subErr = queue.Receive(m.ctx, m.config.Behavior.Topic)
	}

	if subErr != nil {
		slog.Error("failed to subscribe", "error", subErr)
		return
	}

	// Process messages
	for {
		select {
		case <-m.ctx.Done():
			return
		case msg := <-msgChan:
			if msg == nil {
				continue
			}

			start := time.Now()

			if err := m.storeMessage(msg); err != nil {
				slog.Error("failed to store message",
					"message_id", msg.MessageID,
					"error", err)

				m.metrics.mu.Lock()
				m.metrics.EventsFailed++
				m.metrics.mu.Unlock()
			} else {
				m.metrics.mu.Lock()
				m.metrics.EventsStored++
				m.metrics.BytesStored += int64(len(msg.Payload))
				m.metrics.LastEventTime = time.Now()
				m.metrics.ProcessingLatency = time.Since(start)
				m.metrics.mu.Unlock()
			}

			m.metrics.mu.Lock()
			m.metrics.EventsReceived++
			m.metrics.mu.Unlock()
		}
	}
}

// storeMessage extracts headers and stores the message in the table.
func (m *Mailbox) storeMessage(msg *plugin.PubSubMessage) error {
	// Extract headers from metadata
	event := &plugin.MailboxEvent{
		MessageID:     msg.MessageID,
		Timestamp:     msg.Timestamp,
		Topic:         msg.Topic,
		Body:          msg.Payload,
		CustomHeaders: make(map[string]string),
	}

	// Extract standard headers from metadata
	if val, ok := msg.Metadata["prism-content-type"]; ok {
		event.ContentType = val
	}
	if val, ok := msg.Metadata["prism-schema-id"]; ok {
		event.SchemaID = val
	}
	if val, ok := msg.Metadata["prism-encryption"]; ok {
		event.Encryption = val
	}
	if val, ok := msg.Metadata["prism-correlation-id"]; ok {
		event.CorrelationID = val
	}
	if val, ok := msg.Metadata["prism-principal"]; ok {
		event.Principal = val
	}
	if val, ok := msg.Metadata["prism-namespace"]; ok {
		event.Namespace = val
	}

	// Extract custom headers (x-* prefix)
	for key, val := range msg.Metadata {
		if len(key) > 2 && key[:2] == "x-" {
			event.CustomHeaders[key] = val
		}
	}

	// Write to table
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	return m.tableWriter.WriteEvent(ctx, event)
}

// QueryEvents retrieves events matching filter criteria.
// Requires table_reader slot to be bound.
func (m *Mailbox) QueryEvents(ctx context.Context, filter *plugin.EventFilter) ([]*plugin.MailboxEvent, error) {
	if m.tableReader == nil {
		return nil, fmt.Errorf("table_reader slot not bound")
	}

	return m.tableReader.QueryEvents(ctx, filter)
}

// GetEvent retrieves a single event by message ID.
// Requires table_reader slot to be bound.
func (m *Mailbox) GetEvent(ctx context.Context, messageID string) (*plugin.MailboxEvent, error) {
	if m.tableReader == nil {
		return nil, fmt.Errorf("table_reader slot not bound")
	}

	return m.tableReader.GetEvent(ctx, messageID)
}

// GetStats returns mailbox and storage statistics.
func (m *Mailbox) GetStats(ctx context.Context) (map[string]interface{}, error) {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	stats := map[string]interface{}{
		"events_received":     m.metrics.EventsReceived,
		"events_stored":       m.metrics.EventsStored,
		"events_failed":       m.metrics.EventsFailed,
		"bytes_stored":        m.metrics.BytesStored,
		"last_event_time":     m.metrics.LastEventTime,
		"processing_latency":  m.metrics.ProcessingLatency.String(),
	}

	// Add table stats if writer available
	if m.tableWriter != nil {
		tableStats, err := m.tableWriter.GetTableStats(ctx)
		if err == nil {
			stats["table_total_events"] = tableStats.TotalEvents
			stats["table_total_size_bytes"] = tableStats.TotalSizeBytes
			stats["table_oldest_event"] = tableStats.OldestEvent
			stats["table_newest_event"] = tableStats.NewestEvent
		}
	}

	return stats, nil
}

// Health returns the mailbox's health status.
func (m *Mailbox) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	status := &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "mailbox operating normally",
		Details: map[string]string{
			"name":            m.name,
			"topic":           m.config.Behavior.Topic,
			"running":         fmt.Sprintf("%t", m.running),
			"events_received": fmt.Sprintf("%d", m.metrics.EventsReceived),
			"events_stored":   fmt.Sprintf("%d", m.metrics.EventsStored),
			"events_failed":   fmt.Sprintf("%d", m.metrics.EventsFailed),
		},
	}

	if !m.running {
		status.Status = plugin.HealthDegraded
		status.Message = "mailbox not running"
	}

	// Check if failure rate is too high
	if m.metrics.EventsReceived > 0 {
		failureRate := float64(m.metrics.EventsFailed) / float64(m.metrics.EventsReceived)
		if failureRate > 0.1 { // 10% failure threshold
			status.Status = plugin.HealthDegraded
			status.Message = fmt.Sprintf("high failure rate: %.2f%%", failureRate*100)
		}
	}

	return status, nil
}

// Name returns the mailbox pattern name.
func (m *Mailbox) Name() string {
	return m.name
}

// Version returns the pattern version.
func (m *Mailbox) Version() string {
	return "0.1.0"
}
