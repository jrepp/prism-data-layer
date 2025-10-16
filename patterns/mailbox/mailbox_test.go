package mailbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/mailbox"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// MockMessageSource provides a mock implementation of PubSubInterface.
type MockMessageSource struct {
	messages chan *plugin.PubSubMessage
}

func NewMockMessageSource() *MockMessageSource {
	return &MockMessageSource{
		messages: make(chan *plugin.PubSubMessage, 100),
	}
}

func (m *MockMessageSource) Subscribe(ctx context.Context, topic, subscriberID string) (<-chan *plugin.PubSubMessage, error) {
	return m.messages, nil
}

func (m *MockMessageSource) Unsubscribe(ctx context.Context, topic, subscriberID string) error {
	return nil
}

func (m *MockMessageSource) Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	msg := &plugin.PubSubMessage{
		Topic:     topic,
		Payload:   payload,
		Metadata:  metadata,
		MessageID: "test-msg-1",
		Timestamp: time.Now().UnixMilli(),
	}
	m.messages <- msg
	return msg.MessageID, nil
}

// MockTableWriter provides a mock implementation of TableWriterInterface.
type MockTableWriter struct {
	events []*plugin.MailboxEvent
}

func NewMockTableWriter() *MockTableWriter {
	return &MockTableWriter{
		events: make([]*plugin.MailboxEvent, 0),
	}
}

func (m *MockTableWriter) WriteEvent(ctx context.Context, event *plugin.MailboxEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *MockTableWriter) DeleteOldEvents(ctx context.Context, olderThan int64) (int64, error) {
	return 0, nil
}

func (m *MockTableWriter) GetTableStats(ctx context.Context) (*plugin.TableStats, error) {
	return &plugin.TableStats{
		TotalEvents:    int64(len(m.events)),
		TotalSizeBytes: 0,
		OldestEvent:    time.Now().UnixMilli(),
		NewestEvent:    time.Now().UnixMilli(),
	}, nil
}

// MockTableReader provides a mock implementation of TableReaderInterface.
type MockTableReader struct {
	writer *MockTableWriter
}

func NewMockTableReader(writer *MockTableWriter) *MockTableReader {
	return &MockTableReader{writer: writer}
}

func (m *MockTableReader) QueryEvents(ctx context.Context, filter *plugin.EventFilter) ([]*plugin.MailboxEvent, error) {
	return m.writer.events, nil
}

func (m *MockTableReader) GetEvent(ctx context.Context, messageID string) (*plugin.MailboxEvent, error) {
	for _, event := range m.writer.events {
		if event.MessageID == messageID {
			return event, nil
		}
	}
	return nil, nil
}

func (m *MockTableReader) GetTableStats(ctx context.Context) (*plugin.TableStats, error) {
	return m.writer.GetTableStats(ctx)
}

func TestMailboxCreation(t *testing.T) {
	config := mailbox.Config{
		Name: "test-mailbox",
		Behavior: mailbox.BehaviorConfig{
			Topic:         "test.topic",
			ConsumerGroup: "test-group",
			AutoCommit:    true,
		},
		Storage: mailbox.StorageConfig{
			DatabasePath:  "/tmp/test.db",
			TableName:     "mailbox",
			RetentionDays: 90,
		},
	}

	mb, err := mailbox.New(config)
	if err != nil {
		t.Fatalf("failed to create mailbox: %v", err)
	}

	if mb == nil {
		t.Fatal("mailbox is nil")
	}

	if mb.Name() != "test-mailbox" {
		t.Errorf("expected name 'test-mailbox', got '%s'", mb.Name())
	}

	if mb.Version() != "0.1.0" {
		t.Errorf("expected version '0.1.0', got '%s'", mb.Version())
	}
}

func TestMailboxBindSlots(t *testing.T) {
	config := mailbox.Config{
		Name: "test-mailbox",
		Behavior: mailbox.BehaviorConfig{
			Topic:         "test.topic",
			ConsumerGroup: "test-group",
			AutoCommit:    true,
		},
		Storage: mailbox.StorageConfig{
			DatabasePath:  "/tmp/test.db",
			TableName:     "mailbox",
			RetentionDays: 90,
		},
	}

	mb, err := mailbox.New(config)
	if err != nil {
		t.Fatalf("failed to create mailbox: %v", err)
	}

	messageSource := NewMockMessageSource()
	tableWriter := NewMockTableWriter()
	tableReader := NewMockTableReader(tableWriter)

	err = mb.BindSlots(messageSource, tableWriter, tableReader)
	if err != nil {
		t.Fatalf("failed to bind slots: %v", err)
	}
}

func TestMailboxMessageStorage(t *testing.T) {
	config := mailbox.Config{
		Name: "test-mailbox",
		Behavior: mailbox.BehaviorConfig{
			Topic:         "test.topic",
			ConsumerGroup: "test-group",
			AutoCommit:    true,
		},
		Storage: mailbox.StorageConfig{
			DatabasePath:  "/tmp/test.db",
			TableName:     "mailbox",
			RetentionDays: 90,
		},
	}

	mb, err := mailbox.New(config)
	if err != nil {
		t.Fatalf("failed to create mailbox: %v", err)
	}

	messageSource := NewMockMessageSource()
	tableWriter := NewMockTableWriter()
	tableReader := NewMockTableReader(tableWriter)

	if err := mb.BindSlots(messageSource, tableWriter, tableReader); err != nil {
		t.Fatalf("failed to bind slots: %v", err)
	}

	ctx := context.Background()

	// Start mailbox
	if err := mb.Start(ctx); err != nil {
		t.Fatalf("failed to start mailbox: %v", err)
	}
	defer mb.Stop(ctx)

	// Publish test message
	metadata := map[string]string{
		"prism-content-type":   "application/json",
		"prism-principal":      "test-user",
		"prism-correlation-id": "test-trace-123",
		"x-custom-header":      "custom-value",
	}

	if _, err := messageSource.Publish(ctx, "test.topic", []byte("test payload"), metadata); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify message was stored
	if len(tableWriter.events) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(tableWriter.events))
	}

	event := tableWriter.events[0]
	if event.MessageID != "test-msg-1" {
		t.Errorf("expected message_id 'test-msg-1', got '%s'", event.MessageID)
	}

	if event.ContentType != "application/json" {
		t.Errorf("expected content_type 'application/json', got '%s'", event.ContentType)
	}

	if event.Principal != "test-user" {
		t.Errorf("expected principal 'test-user', got '%s'", event.Principal)
	}

	if event.CorrelationID != "test-trace-123" {
		t.Errorf("expected correlation_id 'test-trace-123', got '%s'", event.CorrelationID)
	}

	if event.CustomHeaders["x-custom-header"] != "custom-value" {
		t.Errorf("expected custom header 'custom-value', got '%s'", event.CustomHeaders["x-custom-header"])
	}

	if string(event.Body) != "test payload" {
		t.Errorf("expected body 'test payload', got '%s'", string(event.Body))
	}
}

func TestMailboxQueryEvents(t *testing.T) {
	config := mailbox.Config{
		Name: "test-mailbox",
		Behavior: mailbox.BehaviorConfig{
			Topic:         "test.topic",
			ConsumerGroup: "test-group",
			AutoCommit:    true,
		},
		Storage: mailbox.StorageConfig{
			DatabasePath:  "/tmp/test.db",
			TableName:     "mailbox",
			RetentionDays: 90,
		},
	}

	mb, err := mailbox.New(config)
	if err != nil {
		t.Fatalf("failed to create mailbox: %v", err)
	}

	messageSource := NewMockMessageSource()
	tableWriter := NewMockTableWriter()
	tableReader := NewMockTableReader(tableWriter)

	if err := mb.BindSlots(messageSource, tableWriter, tableReader); err != nil {
		t.Fatalf("failed to bind slots: %v", err)
	}

	ctx := context.Background()

	// Add test events directly to writer
	testEvent := &plugin.MailboxEvent{
		MessageID:   "test-msg-1",
		Timestamp:   time.Now().UnixMilli(),
		Topic:       "test.topic",
		ContentType: "application/json",
		Principal:   "test-user",
		Body:        []byte("test payload"),
	}

	if err := tableWriter.WriteEvent(ctx, testEvent); err != nil {
		t.Fatalf("failed to write test event: %v", err)
	}

	// Query events
	filter := &plugin.EventFilter{
		Limit: 10,
	}

	events, err := mb.QueryEvents(ctx, filter)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].MessageID != "test-msg-1" {
		t.Errorf("expected message_id 'test-msg-1', got '%s'", events[0].MessageID)
	}
}

func TestMailboxHealth(t *testing.T) {
	config := mailbox.Config{
		Name: "test-mailbox",
		Behavior: mailbox.BehaviorConfig{
			Topic:         "test.topic",
			ConsumerGroup: "test-group",
			AutoCommit:    true,
		},
		Storage: mailbox.StorageConfig{
			DatabasePath:  "/tmp/test.db",
			TableName:     "mailbox",
			RetentionDays: 90,
		},
	}

	mb, err := mailbox.New(config)
	if err != nil {
		t.Fatalf("failed to create mailbox: %v", err)
	}

	ctx := context.Background()

	health, err := mb.Health(ctx)
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}

	if health.Status != plugin.HealthDegraded {
		t.Errorf("expected health status 'degraded' (not running), got '%s'", health.Status)
	}
}
