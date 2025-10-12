package backends

/*
This file provides a reference implementation for adding a new backend to the acceptance test framework.

## Adding a New Backend - Quick Start

To add a new backend (e.g., "MyBackend"), follow these steps:

### 1. Create Backend File

Create `tests/acceptance/backends/mybackend.go`:

```go
package backends

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/patterns/mybackend"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/require"
)

func init() {
	// Register backend with test framework
	// This runs automatically when the package is imported
	framework.MustRegisterBackend(framework.Backend{
		Name:      "MyBackend",
		SetupFunc: setupMyBackend,

		SupportedPatterns: []framework.Pattern{
			framework.PatternKeyValueBasic,
			framework.PatternKeyValueTTL,
			// Add other supported patterns
		},

		Capabilities: framework.Capabilities{
			SupportsTTL:  true,
			SupportsScan: false,
			MaxValueSize: 10 * 1024 * 1024, // 10MB limit
			MaxKeySize:   256,                // 256 bytes
			Custom: map[string]interface{}{
				"SupportsTransactions": true,
				"IsolationLevel":       "ReadCommitted",
			},
		},
	})
}

func setupMyBackend(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// 1. Start external services if needed (e.g., testcontainer)
	// container := startMyBackendContainer(t, ctx)

	// 2. Create driver instance
	driver := mybackend.New()

	// 3. Configure driver
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "mybackend-test",
			Version: "0.1.0",
		},
		Backend: map[string]any{
			"connection_string": "localhost:1234",
			"pool_size":         10,
			// Add backend-specific config
		},
	}

	// 4. Initialize driver
	err := driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize MyBackend")

	// 5. Start driver
	err = driver.Start(ctx)
	require.NoError(t, err, "Failed to start MyBackend")

	// 6. Wait for driver to be ready
	err = waitForHealthy(driver, 10*time.Second)
	require.NoError(t, err, "MyBackend did not become healthy")

	// 7. Return driver and cleanup function
	cleanup := func() {
		driver.Stop(ctx)
		// container.Terminate(ctx)
	}

	return driver, cleanup
}

func waitForHealthy(driver core.Plugin, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for healthy status")
		case <-ticker.C:
			health, err := driver.Health(ctx)
			if err == nil && health.Status == core.HealthHealthy {
				return nil
			}
		}
	}
}
```

### 2. Import Backend Package

In your pattern test files, import the backend package:

```go
package keyvalue_test

import (
	"testing"
	_ "github.com/jrepp/prism-data-layer/tests/acceptance/backends" // Registers all backends
)

func TestKeyValuePattern(t *testing.T) {
	// Tests automatically run against all registered backends
	framework.RunPatternTests(t, framework.PatternKeyValueBasic, tests)
}
```

### 3. Run Tests

```bash
# Run all acceptance tests (your backend is automatically included)
go test ./tests/acceptance/patterns/keyvalue/...

# Run tests for specific backend only
go test ./tests/acceptance/patterns/keyvalue/... -run TestKeyValuePattern/MyBackend

# Generate compliance matrix
go run ./tests/acceptance/cmd/acceptance/ report
```

That's it! Your backend now runs through the entire test suite automatically.

## Backend Capabilities

Use the `Capabilities` struct to declare what your backend supports:

```go
Capabilities: framework.Capabilities{
	// Standard capabilities
	SupportsTTL:    true,  // Key expiration/TTL
	SupportsScan:   true,  // Iteration/scanning
	SupportsAtomic: false, // CAS, increment, etc.

	// Size limits (0 = unlimited)
	MaxValueSize: 5 * 1024 * 1024, // 5MB
	MaxKeySize:   512,              // 512 bytes

	// Custom capabilities for advanced features
	Custom: map[string]interface{}{
		"SupportsSecondaryIndexes": true,
		"SupportsFullTextSearch":   false,
		"MaxConnectionPoolSize":    100,
	},
}
```

Tests that require specific capabilities will be automatically skipped if not supported:

```go
framework.PatternTest{
	Name: "TTLExpiration",
	Func: testTTLExpiration,
	RequiresCapability: "TTL", // Skipped if SupportsTTL = false
}
```

## Testing Your Backend Implementation

1. **Unit Tests** - Test your backend logic in isolation
2. **Pattern Tests** - Automatically verify interface compliance (this framework)
3. **Integration Tests** - Test with real services
4. **Performance Tests** - Benchmark your backend

The acceptance framework handles #2 automatically. Just register your backend and run the tests!

## Common Patterns

### Backend with Testcontainer

```go
func setupWithContainer(t *testing.T, ctx context.Context) (interface{}, func()) {
	container := testcontainers.RunMyBackend(t, ctx)
	driver := mybackend.New()

	config := &core.Config{
		Backend: map[string]any{
			"address": container.Address,
		},
	}

	driver.Initialize(ctx, config)
	driver.Start(ctx)

	cleanup := func() {
		driver.Stop(ctx)
		container.Terminate(ctx)
	}

	return driver, cleanup
}
```

### Backend without External Services

```go
func setupInMemory(t *testing.T, ctx context.Context) (interface{}, func()) {
	driver := mybackend.NewInMemory()

	config := &core.Config{
		Backend: map[string]any{
			"max_entries": 10000,
		},
	}

	driver.Initialize(ctx, config)
	driver.Start(ctx)

	cleanup := func() {
		driver.Stop(ctx)
	}

	return driver, cleanup
}
```

### Shared Container Across Tests

For expensive containers (PostgreSQL, etc.), use TestMain:

```go
var sharedContainer *testcontainers.Container

func TestMain(m *testing.M) {
	ctx := context.Background()
	sharedContainer = startContainer(ctx)
	defer sharedContainer.Terminate(ctx)

	os.Exit(m.Run())
}

func setupWithSharedContainer(t *testing.T, ctx context.Context) (interface{}, func()) {
	// Use sharedContainer.Address
	// ...
}
```

## Troubleshooting

### Tests Not Running

- Ensure your backend package is imported with `_ "path/to/backends"`
- Check that `init()` function is called (add debug print)
- Verify `MustRegisterBackend` doesn't panic

### Tests Skipping

- Check `SupportedPatterns` includes the pattern being tested
- Verify required capabilities are enabled
- Look for skip messages in test output

### Tests Failing

- Review detailed error messages in compliance report
- Run single backend: `go test -run TestPattern/YourBackend`
- Add debug logging to your setup function
- Check driver health status

## Best Practices

1. **Keep setup functions focused** - Just start the service and return the driver
2. **Use testcontainers** - Provides isolation and cleanup
3. **Set realistic capabilities** - Don't claim support you can't deliver
4. **Test capability limits** - Verify MaxValueSize, etc. are accurate
5. **Document gotchas** - Add comments about backend quirks

## Examples in This Repo

See these files for real implementations:
- `memstore.go` - Simple in-memory backend
- `redis.go` - Backend with testcontainer
- `nats.go` - PubSub pattern backend
*/

// This is a documentation-only file.
// The examples in the package comment above show how to add new backends.
