package nats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/nats-io/nats.go"
)

// NATSPattern implements a NATS-backed pub/sub messaging plugin
type NATSPattern struct {
	name    string
	version string
	conn    *nats.Conn
	config  *Config
	subs    map[string]*nats.Subscription
	subsMu  sync.RWMutex
}

// Config holds NATS-specific configuration
type Config struct {
	URL             string        `yaml:"url"`
	MaxReconnects   int           `yaml:"max_reconnects"`
	ReconnectWait   time.Duration `yaml:"reconnect_wait"`
	Timeout         time.Duration `yaml:"timeout"`
	PingInterval    time.Duration `yaml:"ping_interval"`
	MaxPendingMsgs  int           `yaml:"max_pending_msgs"`
	EnableJetStream bool          `yaml:"enable_jetstream"`
}

// New creates a new NATS pattern instance
func New() *NATSPattern {
	return &NATSPattern{
		name:    "nats",
		version: "0.1.0",
		subs:    make(map[string]*nats.Subscription),
	}
}

// Name returns the plugin name
func (n *NATSPattern) Name() string {
	return n.name
}

// Version returns the plugin version
func (n *NATSPattern) Version() string {
	return n.version
}

// Initialize prepares the plugin with configuration
func (n *NATSPattern) Initialize(ctx context.Context, config *core.Config) error {
	// Extract backend-specific config with defaults
	var backendConfig Config
	if err := config.GetBackendConfig(&backendConfig); err != nil {
		return fmt.Errorf("failed to parse backend config: %w", err)
	}

	// Apply sensible defaults
	if backendConfig.URL == "" {
		backendConfig.URL = nats.DefaultURL
	}
	if backendConfig.MaxReconnects == 0 {
		backendConfig.MaxReconnects = 10
	}
	if backendConfig.ReconnectWait == 0 {
		backendConfig.ReconnectWait = 2 * time.Second
	}
	if backendConfig.Timeout == 0 {
		backendConfig.Timeout = 5 * time.Second
	}
	if backendConfig.PingInterval == 0 {
		backendConfig.PingInterval = 20 * time.Second
	}
	if backendConfig.MaxPendingMsgs == 0 {
		backendConfig.MaxPendingMsgs = 65536
	}

	n.config = &backendConfig
	n.name = config.Plugin.Name
	n.version = config.Plugin.Version

	// Connect to NATS
	opts := []nats.Option{
		nats.MaxReconnects(backendConfig.MaxReconnects),
		nats.ReconnectWait(backendConfig.ReconnectWait),
		nats.Timeout(backendConfig.Timeout),
		nats.PingInterval(backendConfig.PingInterval),
		nats.MaxPingsOutstanding(3),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("Reconnected to NATS: %s\n", nc.ConnectedUrl())
		}),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			fmt.Printf("Disconnected from NATS: %v\n", err)
		}),
	}

	conn, err := nats.Connect(backendConfig.URL, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	n.conn = conn

	return nil
}

// Start begins serving the plugin
func (n *NATSPattern) Start(ctx context.Context) error {
	// NATS connection is already established in Initialize
	// Nothing additional needed for start
	return nil
}

// Stop gracefully shuts down the plugin
func (n *NATSPattern) Stop(ctx context.Context) error {
	n.subsMu.Lock()
	defer n.subsMu.Unlock()

	// Unsubscribe from all topics
	for topic, sub := range n.subs {
		if err := sub.Unsubscribe(); err != nil {
			fmt.Printf("Warning: failed to unsubscribe from %s: %v\n", topic, err)
		}
		delete(n.subs, topic)
	}

	// Drain and close connection
	if n.conn != nil {
		if err := n.conn.Drain(); err != nil {
			fmt.Printf("Warning: error draining NATS connection: %v\n", err)
		}
		n.conn.Close()
	}

	return nil
}

// Health returns the plugin health status
func (n *NATSPattern) Health(ctx context.Context) (*core.HealthStatus, error) {
	if n.conn == nil {
		return &core.HealthStatus{
			Status:  core.HealthUnhealthy,
			Message: "NATS connection not established",
		}, nil
	}

	status := n.conn.Status()

	switch status {
	case nats.CONNECTED:
		n.subsMu.RLock()
		subCount := len(n.subs)
		n.subsMu.RUnlock()

		stats := n.conn.Stats()

		return &core.HealthStatus{
			Status:  core.HealthHealthy,
			Message: fmt.Sprintf("Connected to %s", n.conn.ConnectedUrl()),
			Details: map[string]string{
				"subscriptions": fmt.Sprintf("%d", subCount),
				"in_msgs":       fmt.Sprintf("%d", stats.InMsgs),
				"out_msgs":      fmt.Sprintf("%d", stats.OutMsgs),
				"in_bytes":      fmt.Sprintf("%d", stats.InBytes),
				"out_bytes":     fmt.Sprintf("%d", stats.OutBytes),
			},
		}, nil
	case nats.RECONNECTING:
		return &core.HealthStatus{
			Status:  core.HealthDegraded,
			Message: "Reconnecting to NATS server",
		}, nil
	default:
		return &core.HealthStatus{
			Status:  core.HealthUnhealthy,
			Message: fmt.Sprintf("NATS connection status: %v", status),
		}, nil
	}
}

// Publish publishes a message to a topic
func (n *NATSPattern) Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	if n.conn == nil {
		return "", fmt.Errorf("NATS connection not established")
	}

	// Publish message
	if err := n.conn.Publish(topic, payload); err != nil {
		return "", fmt.Errorf("failed to publish to topic %s: %w", topic, err)
	}

	// Flush to ensure message is sent
	if err := n.conn.FlushTimeout(n.config.Timeout); err != nil {
		return "", fmt.Errorf("failed to flush after publish: %w", err)
	}

	// NATS core doesn't return message IDs, so we generate one
	messageID := fmt.Sprintf("%s-%d", topic, time.Now().UnixNano())

	return messageID, nil
}

// Subscribe subscribes to a topic and returns a channel for messages
func (n *NATSPattern) Subscribe(ctx context.Context, topic string, subscriberID string) (<-chan *Message, error) {
	if n.conn == nil {
		return nil, fmt.Errorf("NATS connection not established")
	}

	// Create message channel
	msgChan := make(chan *Message, n.config.MaxPendingMsgs)

	// Create NATS subscription
	sub, err := n.conn.Subscribe(topic, func(msg *nats.Msg) {
		select {
		case msgChan <- &Message{
			Topic:     msg.Subject, // Use actual message subject, not subscription pattern
			Payload:   msg.Data,
			MessageID: fmt.Sprintf("%s-%d", msg.Subject, time.Now().UnixNano()),
			Timestamp: time.Now().Unix(),
		}:
		case <-ctx.Done():
			return
		default:
			// Channel full, drop message (at-most-once semantics for core NATS)
			fmt.Printf("Warning: message dropped for topic %s (channel full)\n", topic)
		}
	})

	if err != nil {
		close(msgChan)
		return nil, fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	// Store subscription
	n.subsMu.Lock()
	key := fmt.Sprintf("%s:%s", topic, subscriberID)
	n.subs[key] = sub
	n.subsMu.Unlock()

	return msgChan, nil
}

// Unsubscribe unsubscribes from a topic
func (n *NATSPattern) Unsubscribe(ctx context.Context, topic string, subscriberID string) error {
	n.subsMu.Lock()
	defer n.subsMu.Unlock()

	key := fmt.Sprintf("%s:%s", topic, subscriberID)
	sub, exists := n.subs[key]
	if !exists {
		return fmt.Errorf("no subscription found for topic %s with subscriber %s", topic, subscriberID)
	}

	if err := sub.Unsubscribe(); err != nil {
		return fmt.Errorf("failed to unsubscribe from topic %s: %w", topic, err)
	}

	delete(n.subs, key)
	return nil
}

// Message represents a pub/sub message
type Message struct {
	Topic     string
	Payload   []byte
	Metadata  map[string]string
	MessageID string
	Timestamp int64
}
