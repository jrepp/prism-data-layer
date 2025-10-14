package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

// ConsumerBackends holds the backends needed for consumer pattern
type ConsumerBackends struct {
	MessageSource plugin.PubSubInterface
	StateStore    plugin.KeyValueBasicInterface
	DeadLetter    plugin.QueueInterface
	// Connection strings for process-based tests
	NATSUrl      string
	MemStoreAddr string
	// For DLQ verification
	DLQDriver interface{} // NATS driver that can be used to check DLQ
}

func init() {
	// Register Consumer + NATS (stateless) - in-process testing
	framework.MustRegisterBackend(framework.Backend{
		Name:      "Consumer-NATS-Stateless",
		SetupFunc: setupConsumerNATSStateless,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("Consumer"),
		},
		Capabilities: framework.Capabilities{
			SupportsTTL:      false,
			SupportsOrdering: false,
			Custom: map[string]interface{}{
				"Stateless": true,
			},
		},
	})

	// Register Consumer + NATS + MemStore (stateful) - in-process testing
	framework.MustRegisterBackend(framework.Backend{
		Name:      "Consumer-NATS-MemStore",
		SetupFunc: setupConsumerNATSMemStore,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("Consumer"),
		},
		Capabilities: framework.Capabilities{
			SupportsTTL:      true,
			SupportsOrdering: false,
			Custom: map[string]interface{}{
				"Stateful": true,
			},
		},
	})

	// Register for process-based testing (separate process like proxy would launch)
	framework.MustRegisterBackend(framework.Backend{
		Name:      "ConsumerProcess-NATS-Stateless",
		SetupFunc: setupConsumerProcessNATSStateless,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("ConsumerProcess"),
		},
		Capabilities: framework.Capabilities{
			Custom: map[string]interface{}{
				"ProcessBased": true,
				"Stateless":    true,
			},
		},
	})

	framework.MustRegisterBackend(framework.Backend{
		Name:      "ConsumerProcess-NATS-MemStore",
		SetupFunc: setupConsumerProcessNATSMemStore,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("ConsumerProcess"),
		},
		Capabilities: framework.Capabilities{
			Custom: map[string]interface{}{
				"ProcessBased": true,
				"Stateful":     true,
			},
		},
	})

	// Register Consumer + NATS + MemStore + NATS DLQ (all 3 slots filled)
	framework.MustRegisterBackend(framework.Backend{
		Name:      "Consumer-NATS-MemStore-DLQ",
		SetupFunc: setupConsumerNATSMemStoreDLQ,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("Consumer"),
		},
		Capabilities: framework.Capabilities{
			SupportsTTL:      true,
			SupportsOrdering: false,
			Custom: map[string]interface{}{
				"Stateful":    true,
				"HasDLQ":      true,
				"FullySlotted": true,
			},
		},
	})

	// Register for process-based DLQ testing
	framework.MustRegisterBackend(framework.Backend{
		Name:      "ConsumerProcess-NATS-MemStore-DLQ",
		SetupFunc: setupConsumerProcessNATSMemStoreDLQ,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("ConsumerProcess"),
		},
		Capabilities: framework.Capabilities{
			Custom: map[string]interface{}{
				"ProcessBased": true,
				"Stateful":     true,
				"HasDLQ":       true,
				"FullySlotted": true,
			},
		},
	})
}

// setupConsumerNATSStateless creates a stateless consumer with NATS only
func setupConsumerNATSStateless(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Start NATS testcontainer
	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err, "Failed to start NATS container")

	connStr, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get NATS connection string")

	// Create NATS driver
	natsDriver := nats.New()

	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats-consumer-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url":              connStr,
			"max_reconnects":   10,
			"reconnect_wait":   "2s",
			"timeout":          "5s",
			"enable_jetstream": false,
		},
	}

	err = natsDriver.Initialize(ctx, natsConfig)
	require.NoError(t, err, "Failed to initialize NATS driver")

	err = natsDriver.Start(ctx)
	require.NoError(t, err, "Failed to start NATS driver")

	// Wait for health
	err = waitForHealthy(natsDriver, 10*time.Second)
	require.NoError(t, err, "NATS driver did not become healthy")

	// Create consumer backends (stateless - no state store)
	// NATSPattern implements PubSubInterface directly
	consumerBackends := &ConsumerBackends{
		MessageSource: natsDriver,
		StateStore:    nil, // Stateless
		DeadLetter:    nil,
		NATSUrl:       connStr,
		MemStoreAddr:  "",
	}

	cleanup := func() {
		natsDriver.Stop(ctx)
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate NATS container: %v", err)
		}
	}

	return consumerBackends, cleanup
}

// setupConsumerNATSMemStore creates a stateful consumer with NATS + MemStore
func setupConsumerNATSMemStore(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Start NATS testcontainer
	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err, "Failed to start NATS container")

	connStr, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get NATS connection string")

	// Create NATS driver
	natsDriver := nats.New()

	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats-consumer-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url":              connStr,
			"max_reconnects":   10,
			"reconnect_wait":   "2s",
			"timeout":          "5s",
			"enable_jetstream": false,
		},
	}

	err = natsDriver.Initialize(ctx, natsConfig)
	require.NoError(t, err, "Failed to initialize NATS driver")

	err = natsDriver.Start(ctx)
	require.NoError(t, err, "Failed to start NATS driver")

	err = waitForHealthy(natsDriver, 10*time.Second)
	require.NoError(t, err, "NATS driver did not become healthy")

	// Create MemStore driver for state
	memstoreBackend := backends.SetupMemStore(t, ctx)

	// Create consumer backends (stateful)
	// NATSPattern implements PubSubInterface directly
	consumerBackends := &ConsumerBackends{
		MessageSource: natsDriver,
		StateStore:    memstoreBackend.Driver,
		DeadLetter:    nil,
		NATSUrl:       connStr,
		MemStoreAddr:  "", // MemStore is in-process, no address needed
	}

	cleanup := func() {
		natsDriver.Stop(ctx)
		memstoreBackend.Cleanup()
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate NATS container: %v", err)
		}
	}

	return consumerBackends, cleanup
}

// setupConsumerProcessNATSStateless sets up for process-based testing (stateless)
func setupConsumerProcessNATSStateless(t *testing.T, ctx context.Context) (interface{}, func()) {
	// Reuse the in-process setup - same backends, just different usage
	return setupConsumerNATSStateless(t, ctx)
}

// setupConsumerProcessNATSMemStore sets up for process-based testing (stateful)
func setupConsumerProcessNATSMemStore(t *testing.T, ctx context.Context) (interface{}, func()) {
	// Reuse the in-process setup
	return setupConsumerNATSMemStore(t, ctx)
}

// setupConsumerNATSMemStoreDLQ creates consumer with all 3 slots filled
func setupConsumerNATSMemStoreDLQ(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Start NATS testcontainer
	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err, "Failed to start NATS container")

	connStr, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get NATS connection string")

	// Create NATS driver for message source AND DLQ
	natsDriver := nats.New()

	natsConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats-consumer-dlq-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"url":              connStr,
			"max_reconnects":   10,
			"reconnect_wait":   "2s",
			"timeout":          "5s",
			"enable_jetstream": false,
		},
	}

	err = natsDriver.Initialize(ctx, natsConfig)
	require.NoError(t, err, "Failed to initialize NATS driver")

	err = natsDriver.Start(ctx)
	require.NoError(t, err, "Failed to start NATS driver")

	err = waitForHealthy(natsDriver, 10*time.Second)
	require.NoError(t, err, "NATS driver did not become healthy")

	// Create MemStore driver for state
	memstoreBackend := backends.SetupMemStore(t, ctx)

	// Create consumer backends with all 3 slots
	consumerBackends := &ConsumerBackends{
		MessageSource: natsDriver, // PubSubInterface
		StateStore:    memstoreBackend.Driver,
		DeadLetter:    natsDriver, // QueueInterface (same NATS instance)
		NATSUrl:       connStr,
		MemStoreAddr:  "",
		DLQDriver:     natsDriver, // For verifying DLQ messages
	}

	cleanup := func() {
		natsDriver.Stop(ctx)
		memstoreBackend.Cleanup()
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate NATS container: %v", err)
		}
	}

	return consumerBackends, cleanup
}

// setupConsumerProcessNATSMemStoreDLQ sets up for process-based DLQ testing
func setupConsumerProcessNATSMemStoreDLQ(t *testing.T, ctx context.Context) (interface{}, func()) {
	// Reuse the in-process setup
	return setupConsumerNATSMemStoreDLQ(t, ctx)
}

// waitForHealthy polls a driver until it reports healthy
func waitForHealthy(driver plugin.Plugin, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			health, err := driver.Health(ctx)
			if err == nil && health.Status == plugin.HealthHealthy {
				return nil
			}
		}
	}
}
