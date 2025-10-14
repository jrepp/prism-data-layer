package producer_test

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

// ProducerBackends holds the backend instances required for producer pattern tests.
type ProducerBackends struct {
	// MessageSink is where messages are published (PubSubInterface or QueueInterface)
	MessageSink interface{}

	// StateStore is for producer state (deduplication, sequencing)
	// Optional: can be nil for stateless producers
	StateStore plugin.KeyValueBasicInterface
}

func init() {
	// Register Producer + NATS (stateless) - in-process testing
	framework.MustRegisterBackend(framework.Backend{
		Name:      "Producer-NATS-Stateless",
		SetupFunc: setupProducerNATSStateless,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("Producer"),
		},
		Capabilities: framework.Capabilities{
			SupportsTTL:      false,
			SupportsOrdering: false,
			Custom: map[string]interface{}{
				"Stateless": true,
			},
		},
	})

	// Register Producer + NATS + MemStore (stateful for deduplication)
	framework.MustRegisterBackend(framework.Backend{
		Name:      "Producer-NATS-MemStore",
		SetupFunc: setupProducerNATSMemStore,
		SupportedPatterns: []framework.Pattern{
			framework.Pattern("Producer"),
		},
		Capabilities: framework.Capabilities{
			SupportsTTL:      true,
			SupportsOrdering: false,
			Custom: map[string]interface{}{
				"Stateful": true,
			},
		},
	})
}

// setupProducerNATSStateless creates a stateless producer with NATS only
func setupProducerNATSStateless(t *testing.T, ctx context.Context) (interface{}, func()) {
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
			Name:    "nats-producer-test",
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

	// Create producer backends (stateless - no state store)
	producerBackends := &ProducerBackends{
		MessageSink: natsDriver,
		StateStore:  nil, // Stateless
	}

	cleanup := func() {
		natsDriver.Stop(ctx)
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate NATS container: %v", err)
		}
	}

	return producerBackends, cleanup
}

// setupProducerNATSMemStore creates a stateful producer with NATS + MemStore
func setupProducerNATSMemStore(t *testing.T, ctx context.Context) (interface{}, func()) {
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
			Name:    "nats-producer-test",
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

	// Create producer backends (stateful for deduplication)
	producerBackends := &ProducerBackends{
		MessageSink: natsDriver,
		StateStore:  memstoreBackend.Driver,
	}

	cleanup := func() {
		natsDriver.Stop(ctx)
		memstoreBackend.Cleanup()
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate NATS container: %v", err)
		}
	}

	return producerBackends, cleanup
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
