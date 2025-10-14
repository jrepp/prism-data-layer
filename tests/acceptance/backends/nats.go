package backends

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

func init() {
	// Register NATS backend with the acceptance test framework
	framework.MustRegisterBackend(framework.Backend{
		Name:      "NATS",
		SetupFunc: setupNATS,

		SupportedPatterns: []framework.Pattern{
			framework.PatternPubSubBasic,
			framework.PatternPubSubFanout,
			// Note: Core NATS doesn't guarantee ordering. Use JetStream for that.
		},

		Capabilities: framework.Capabilities{
			SupportsTTL:      false, // No native TTL
			SupportsOrdering: false, // Core NATS is at-most-once, no ordering
			MaxValueSize:     1024 * 1024, // 1MB default max message size
			Custom: map[string]interface{}{
				"AtMostOnce": true,  // Core NATS delivery semantics
				"JetStream":  false, // Not using JetStream in basic tests
			},
		},
	})
}

// setupNATS creates a NATS backend for testing using testcontainers
func setupNATS(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Start NATS testcontainer
	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err, "Failed to start NATS container")

	// Get connection string
	connStr, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get NATS connection string")

	// Create NATS driver
	driver := nats.New()

	// Configure driver with testcontainer connection
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "nats-test",
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

	// Initialize driver
	err = driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize NATS driver")

	// Start driver
	err = driver.Start(ctx)
	require.NoError(t, err, "Failed to start NATS driver")

	// Wait for driver to be healthy
	err = waitForNATSHealthy(driver, 10*time.Second)
	require.NoError(t, err, "NATS driver did not become healthy")

	// Cleanup function stops driver and terminates container
	cleanup := func() {
		driver.Stop(ctx)
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate NATS container: %v", err)
		}
	}

	return driver, cleanup
}

// waitForNATSHealthy polls the NATS driver's health endpoint
func waitForNATSHealthy(driver core.Plugin, timeout time.Duration) error {
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
			if err == nil && health.Status == core.HealthHealthy {
				return nil
			}
		}
	}
}
