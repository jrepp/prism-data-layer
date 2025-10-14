package backends

import (
	"context"
	"testing"

	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
)

func init() {
	// Register MemStore backend with the acceptance test framework
	framework.MustRegisterBackend(framework.Backend{
		Name:      "MemStore",
		SetupFunc: setupMemStore,

		SupportedPatterns: []framework.Pattern{
			framework.PatternKeyValueBasic,
			framework.PatternKeyValueTTL,
			// Note: MemStore only implements KeyValue, not PubSub
		},

		Capabilities: framework.Capabilities{
			SupportsTTL:    true,
			SupportsScan:   false, // MemStore doesn't support scan operations
			SupportsAtomic: false,
			MaxValueSize:   0,    // Unlimited (memory-constrained)
			MaxKeySize:     0,    // Unlimited
		},
	})
}

// setupMemStore creates a MemStore backend for testing
// MemStore is an in-memory implementation requiring no external services
func setupMemStore(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Use existing backend setup helper
	backend := backends.SetupMemStore(t, ctx)

	// Return driver and cleanup function
	return backend.Driver, backend.Cleanup
}
