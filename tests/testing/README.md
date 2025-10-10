# Testing Infrastructure

This directory provides shared testing utilities for the Prism data access layer.

## Purpose

The `testing/` directory centralizes cross-cutting test concerns that are used across multiple test suites:

- **Backend Setup**: Testcontainer-based backend infrastructure (Redis, NATS, etc.)
- **Test Fixtures**: Common test data and configuration
- **Test Utilities**: Shared helper functions

## Structure

```
tests/testing/
├── backends/        # Backend testcontainer setup
│   ├── redis.go     # Redis container management
│   ├── nats.go      # NATS container management
│   └── doc.go       # Package documentation
├── go.mod           # Module dependencies
└── README.md        # This file
```

## Quiet Mode

By default, testcontainers produces verbose Docker logs that can clutter test output. You can suppress these logs:

**Environment Variable**:
```bash
export PRISM_TEST_QUIET=1
go test ./...
```

**Programmatic**:
```go
import "github.com/jrepp/prism-data-layer/tests/testing/backends"

func TestMain(m *testing.M) {
	backends.SetQuietMode(true)
	os.Exit(m.Run())
}
```

**Makefile Target**:
```bash
make test-acceptance-quiet  # Runs all acceptance tests in quiet mode
```

## Usage

### Redis Backend

```go
import (
	"context"
	"testing"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
)

func TestWithRedis(t *testing.T) {
	ctx := context.Background()

	// Start Redis container
	backend := backends.SetupRedis(t, ctx)
	defer backend.Cleanup()

	// Use connection string (already stripped of redis:// prefix)
	config := &core.Config{
		Backend: map[string]any{
			"address": backend.ConnectionString,
		},
	}

	// Run your tests...
}
```

### NATS Backend

```go
import (
	"context"
	"testing"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
)

func TestWithNATS(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	backend := backends.SetupNATS(t, ctx)
	defer backend.Cleanup()

	// Use connection string
	config := &core.Config{
		Backend: map[string]any{
			"url": backend.ConnectionString,
		},
	}

	// Run your tests...
}
```

## Benefits

1. **DRY Principle**: Backend setup code is written once, used everywhere
2. **Consistency**: All tests use the same backend configuration
3. **Maintainability**: Changes to backend setup only need to be made in one place
4. **Discoverability**: Clear separation between test infrastructure and test logic
5. **Reusability**: Easy to add new backends following the same pattern

## Adding New Backends

To add a new backend (e.g., PostgreSQL):

1. Create `backends/postgres.go`:
```go
package backends

import (
	"context"
	"testing"
	// ... imports
)

type PostgresBackend struct {
	ConnectionString string
	cleanup          func()
}

func SetupPostgres(t *testing.T, ctx context.Context) *PostgresBackend {
	// Start container
	// Get connection string
	// Return backend with cleanup
}

func (b *PostgresBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}
```

2. Add testcontainers module to `go.mod`:
```bash
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

3. Update this README with usage example

## Related

- [Acceptance Tests](../acceptance/) - Integration tests using these backends
- [RFC-015: Plugin Acceptance Test Framework](/rfc/rfc-015) - Test strategy
- [testcontainers-go](https://golang.testcontainers.org/) - Container management library
