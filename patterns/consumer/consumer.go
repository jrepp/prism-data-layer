package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
)

// Consumer implements a scale-out consumer pattern that binds to backend patterns
// It provides subscription/consumption logic on top of existing backend providers
type Consumer struct {
	name    string
	version string
	config  *core.ConsumerConfig
	backend *core.BackendConfig

	// Subscriber interface to backend pattern
	subscriber Subscriber

	// Message handlers
	handler MessageHandler
	workers []*Worker

	mu       sync.RWMutex
	stopCh   chan struct{}
	doneCh   chan struct{}
	started  bool
}

// Subscriber defines the interface for subscribing to a backend
type Subscriber interface {
	Subscribe(ctx context.Context, topic string, subscriberID string) (<-chan *Message, error)
	Unsubscribe(ctx context.Context, topic string, subscriberID string) error
}

// Message represents a message from the backend
type Message struct {
	ID        string
	Topic     string
	Payload   []byte
	Metadata  map[string]string
	Timestamp time.Time
	
	// Acknowledgment callback
	Ack  func() error
	Nack func() error
}

// MessageHandler processes messages
type MessageHandler interface {
	Handle(ctx context.Context, msg *Message) error
}

// MessageHandlerFunc is a function adapter for MessageHandler
type MessageHandlerFunc func(ctx context.Context, msg *Message) error

func (f MessageHandlerFunc) Handle(ctx context.Context, msg *Message) error {
	return f(ctx, msg)
}

// Worker represents a concurrent consumer worker
type Worker struct {
	id        int
	consumer  *Consumer
	msgChan   <-chan *Message
	stopCh    chan struct{}
	wg        sync.WaitGroup
	ctx       context.Context
	ctxCancel context.CancelFunc
}

// New creates a new Consumer pattern
func New() *Consumer {
	return &Consumer{
		name:    "consumer",
		version: "0.1.0",
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Name returns the plugin name
func (c *Consumer) Name() string {
	return c.name
}

// Version returns the plugin version
func (c *Consumer) Version() string {
	return c.version
}

// Initialize prepares the consumer with configuration
func (c *Consumer) Initialize(ctx context.Context, config *core.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate consumer config
	if config.Consumer == nil {
		return fmt.Errorf("no consumer configuration provided")
	}

	// Get backend configuration
	backend, err := config.GetConsumerBackend()
	if err != nil {
		return fmt.Errorf("failed to get consumer backend: %w", err)
	}

	// Apply defaults
	c.config = config.Consumer
	if c.config.Concurrency == 0 {
		c.config.Concurrency = 1
	}
	if c.config.BatchSize == 0 {
		c.config.BatchSize = 10
	}
	if c.config.AckMode == "" {
		c.config.AckMode = "auto"
	}

	c.backend = backend
	c.name = config.Plugin.Name

	slog.Info("consumer initialized",
		"name", c.name,
		"backend", backend.ConfigPath,
		"topic", c.config.Topic,
		"concurrency", c.config.Concurrency)

	return nil
}

// SetSubscriber sets the backend subscriber implementation
func (c *Consumer) SetSubscriber(subscriber Subscriber) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscriber = subscriber
}

// SetHandler sets the message handler
func (c *Consumer) SetHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start begins consuming messages
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("consumer already started")
	}

	if c.subscriber == nil {
		return fmt.Errorf("no subscriber configured")
	}

	if c.handler == nil {
		return fmt.Errorf("no message handler configured")
	}

	// Subscribe to topic
	msgChan, err := c.subscriber.Subscribe(ctx, c.config.Topic, c.config.SubscriberID)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// Start workers
	c.workers = make([]*Worker, c.config.Concurrency)
	for i := 0; i < c.config.Concurrency; i++ {
		worker := &Worker{
			id:       i,
			consumer: c,
			msgChan:  msgChan,
			stopCh:   make(chan struct{}),
		}
		worker.ctx, worker.ctxCancel = context.WithCancel(ctx)
		c.workers[i] = worker

		worker.wg.Add(1)
		go worker.run()
	}

	c.started = true

	slog.Info("consumer started",
		"name", c.name,
		"workers", len(c.workers),
		"topic", c.config.Topic)

	// Wait for context cancellation
	go func() {
		<-ctx.Done()
		close(c.stopCh)
	}()

	// Wait for shutdown
	<-c.stopCh
	close(c.doneCh)

	return nil
}

// Stop gracefully shuts down the consumer
func (c *Consumer) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	slog.Info("stopping consumer", "name", c.name)

	// Stop all workers
	for _, worker := range c.workers {
		close(worker.stopCh)
		worker.ctxCancel()
	}

	// Wait for workers to finish (with timeout)
	done := make(chan struct{})
	go func() {
		for _, worker := range c.workers {
			worker.wg.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		slog.Info("consumer stopped gracefully", "name", c.name)
	case <-time.After(30 * time.Second):
		slog.Warn("consumer stop timeout", "name", c.name)
	}

	// Unsubscribe
	if c.subscriber != nil {
		if err := c.subscriber.Unsubscribe(ctx, c.config.Topic, c.config.SubscriberID); err != nil {
			slog.Warn("failed to unsubscribe", "error", err)
		}
	}

	c.started = false
	return nil
}

// Health returns the consumer health status
func (c *Consumer) Health(ctx context.Context) (*core.HealthStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := core.HealthHealthy
	message := "consumer healthy"

	if !c.started {
		status = core.HealthDegraded
		message = "consumer not started"
	}

	activeWorkers := 0
	for _, worker := range c.workers {
		select {
		case <-worker.stopCh:
			// Worker stopped
		default:
			activeWorkers++
		}
	}

	return &core.HealthStatus{
		Status:  status,
		Message: message,
		Details: map[string]string{
			"backend":        c.backend.ConfigPath,
			"topic":          c.config.Topic,
			"subscriber_id":  c.config.SubscriberID,
			"concurrency":    fmt.Sprintf("%d", c.config.Concurrency),
			"active_workers": fmt.Sprintf("%d", activeWorkers),
		},
	}, nil
}

// run executes the worker loop
func (w *Worker) run() {
	defer w.wg.Done()

	slog.Info("worker started", "id", w.id, "consumer", w.consumer.name)

	for {
		select {
		case <-w.stopCh:
			slog.Info("worker stopped", "id", w.id)
			return
		case <-w.ctx.Done():
			slog.Info("worker context cancelled", "id", w.id)
			return
		case msg, ok := <-w.msgChan:
			if !ok {
				slog.Info("message channel closed", "id", w.id)
				return
			}

			// Process message
			if err := w.consumer.handler.Handle(w.ctx, msg); err != nil {
				slog.Error("failed to handle message",
					"worker", w.id,
					"message_id", msg.ID,
					"error", err)

				// Handle nack
				if msg.Nack != nil {
					if nackErr := msg.Nack(); nackErr != nil {
						slog.Error("failed to nack message",
							"worker", w.id,
							"message_id", msg.ID,
							"error", nackErr)
					}
				}
				continue
			}

			// Handle ack
			if w.consumer.config.AckMode == "auto" && msg.Ack != nil {
				if err := msg.Ack(); err != nil {
					slog.Error("failed to ack message",
						"worker", w.id,
						"message_id", msg.ID,
						"error", err)
				}
			}
		}
	}
}

// Compile-time interface compliance checks
var (
	_ core.Plugin           = (*Consumer)(nil)
	_ core.InterfaceSupport = (*Consumer)(nil)
)

// SupportsInterface returns true if Consumer implements the named interface
func (c *Consumer) SupportsInterface(interfaceName string) bool {
	supported := map[string]bool{
		"Plugin":           true,
		"Consumer":         true,
		"InterfaceSupport": true,
	}
	return supported[interfaceName]
}

// ListInterfaces returns all interfaces that Consumer implements
func (c *Consumer) ListInterfaces() []string {
	return []string{
		"Plugin",
		"Consumer",
		"InterfaceSupport",
	}
}
