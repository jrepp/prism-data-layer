package keyvalue_basic_test

import (
	"context"
	"testing"

	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/interface-suites/keyvalue_basic"
	"github.com/stretchr/testify/require"
)

// TestMemStoreKeyValueBasic runs the KeyValueBasic interface suite against MemStore driver
func TestMemStoreKeyValueBasic(t *testing.T) {
	// Create test suite with MemStore-specific setup/teardown
	suite := &keyvalue_basic.Suite{
		CreateBackend: func(t *testing.T) plugin.KeyValueBasicInterface {
			return createMemStoreBackend(t)
		},
		CleanupBackend: func(t *testing.T, backend plugin.KeyValueBasicInterface) {
			cleanupMemStoreBackend(t, backend)
		},
	}

	// Run the full suite
	suite.Run(t)
}

// createMemStoreBackend creates and initializes a MemStore driver
func createMemStoreBackend(t *testing.T) plugin.KeyValueBasicInterface {
	ctx := context.Background()

	// Create MemStore driver (in-memory, no external dependencies)
	driver := memstore.New()

	// Initialize with default config
	config := &plugin.Config{
		BackendConfig: map[string]interface{}{
			"max_keys":       10000,
			"cleanup_period": "60s",
		},
	}

	err := driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize MemStore driver")

	// MemStore doesn't need Start() for basic operations
	// but we call it for consistency with other backends
	go func() {
		_ = driver.Start(ctx)
	}()

	return driver
}

// cleanupMemStoreBackend stops the MemStore driver
func cleanupMemStoreBackend(t *testing.T, backend plugin.KeyValueBasicInterface) {
	if driver, ok := backend.(*memstore.MemStore); ok {
		_ = driver.Stop(context.Background())
	}
}
