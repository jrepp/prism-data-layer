package backends

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/stretchr/testify/require"
)

// MemStoreBackend provides MemStore backend setup (no container needed)
type MemStoreBackend struct {
	Driver  *memstore.MemStore
	cleanup func()
}

// SetupMemStore creates a MemStore backend driver for testing
// Unlike Redis/PostgreSQL, MemStore requires no external services
func SetupMemStore(t *testing.T, ctx context.Context) *MemStoreBackend {
	t.Helper()

	// Create MemStore driver
	driver := memstore.New()

	// Configure with defaults (no external connection needed)
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "memstore-test",
			Version: "0.1.0",
		},
		ControlPlane: plugin.ControlPlaneConfig{
			Port: 9091, // Test port
		},
		Backend: map[string]any{
			"max_keys":       1000000, // High limit for stress tests
			"cleanup_period": "10s",
		},
	}

	// Initialize driver
	err := driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize MemStore driver")

	// Start driver
	go func() {
		if err := driver.Start(ctx); err != nil {
			t.Logf("MemStore driver Start() returned: %v", err)
		}
	}()

	// Wait a moment for driver to be ready
	time.Sleep(100 * time.Millisecond)

	return &MemStoreBackend{
		Driver: driver,
		cleanup: func() {
			if err := driver.Stop(ctx); err != nil {
				t.Logf("Failed to stop MemStore driver: %v", err)
			}
		},
	}
}

// Cleanup shuts down the MemStore driver
func (b *MemStoreBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}
