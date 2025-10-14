package backends

import (
	"context"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"
)

// NATSMessagingBackend implements MessagingBackend using NATS
type NATSMessagingBackend struct {
	conn          *nats.Conn
	subscriptions map[string]*nats.Subscription
	mu            sync.RWMutex
}

// NewNATSMessagingBackend creates a NATS-backed messaging backend
func NewNATSMessagingBackend(servers []string) (*NATSMessagingBackend, error) {
	if len(servers) == 0 {
		servers = []string{nats.DefaultURL}
	}

	// Connect to NATS
	conn, err := nats.Connect(
		servers[0], // POC 4: Single server, multi-server in production
		nats.MaxReconnects(10),
		nats.ReconnectWait(nats.DefaultReconnectWait),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connection failed: %w", err)
	}

	return &NATSMessagingBackend{
		conn:          conn,
		subscriptions: make(map[string]*nats.Subscription),
	}, nil
}

// Publish sends a message to a topic
func (n *NATSMessagingBackend) Publish(ctx context.Context, topic string, payload []byte) error {
	if err := n.conn.Publish(topic, payload); err != nil {
		return fmt.Errorf("nats publish failed: %w", err)
	}

	// Flush to ensure message is sent
	return n.conn.FlushWithContext(ctx)
}

// Subscribe creates a subscription for a topic (consumer-side)
func (n *NATSMessagingBackend) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if already subscribed
	if _, exists := n.subscriptions[topic]; exists {
		return nil, fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// Create buffered channel for messages
	msgChan := make(chan []byte, 100)

	// Subscribe with handler
	sub, err := n.conn.Subscribe(topic, func(msg *nats.Msg) {
		select {
		case msgChan <- msg.Data:
			// Message delivered
		case <-ctx.Done():
			// Context cancelled
			return
		default:
			// Channel full, drop message (at-most-once semantics)
		}
	})

	if err != nil {
		close(msgChan)
		return nil, fmt.Errorf("nats subscribe failed: %w", err)
	}

	n.subscriptions[topic] = sub

	// Start goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		n.Unsubscribe(context.Background(), topic)
		close(msgChan)
	}()

	return msgChan, nil
}

// Unsubscribe removes a subscription
func (n *NATSMessagingBackend) Unsubscribe(ctx context.Context, topic string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	sub, exists := n.subscriptions[topic]
	if !exists {
		// Idempotent: succeed even if not subscribed
		return nil
	}

	if err := sub.Unsubscribe(); err != nil {
		return fmt.Errorf("nats unsubscribe failed: %w", err)
	}

	delete(n.subscriptions, topic)
	return nil
}

// Close closes NATS connection
func (n *NATSMessagingBackend) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Unsubscribe from all topics
	for _, sub := range n.subscriptions {
		sub.Unsubscribe()
	}
	n.subscriptions = make(map[string]*nats.Subscription)

	// Close connection
	n.conn.Close()
	return nil
}

// ConnectionStatus returns the current connection state
func (n *NATSMessagingBackend) ConnectionStatus() string {
	if n.conn.IsConnected() {
		return "CONNECTED"
	}
	if n.conn.IsReconnecting() {
		return "RECONNECTING"
	}
	if n.conn.IsClosed() {
		return "CLOSED"
	}
	return "UNKNOWN"
}
