# Acceptance Test Framework

This directory implements the **RFC-015 Plugin Acceptance Test Framework** using testcontainers for integration testing of Prism data access patterns.

## Overview

The acceptance test framework provides:

- **Real Backend Testing**: Uses Docker containers (testcontainers) for Redis, NATS, and other backends
- **Pattern Lifecycle Validation**: Tests the complete plugin lifecycle (Initialize → Start → Health → Stop)
- **Comprehensive Coverage**: Basic operations, concurrent operations, error handling, health checks
- **Reusable Infrastructure**: Shared backend setup utilities in `tests/testing/backends/`
- **Common Test Utilities**: PatternHarness for standardized pattern testing

## Architecture

```
tests/
├── testing/               # Cross-cutting test infrastructure
│   └── backends/          # Backend container setup (Redis, NATS)
└── acceptance/            # Integration tests using real backends
    ├── common/            # Shared test utilities (PatternHarness)
    ├── redis/             # Redis pattern acceptance tests
    └── nats/              # NATS pattern acceptance tests
```

### Key Components

#### PatternHarness (`common/harness.go`)

Provides lifecycle management and health polling for patterns:

```go
import (
    "context"
    "testing"
    "github.com/jrepp/prism-data-layer/tests/acceptance/common"
    "github.com/jrepp/prism-data-layer/patterns/redis"
    "github.com/jrepp/prism-data-layer/patterns/core"
)

func TestMyPattern(t *testing.T) {
    ctx := context.Background()

    // Create pattern instance
    redisPlugin := redis.New()

    // Configure pattern
    config := &core.Config{
        Plugin: core.PluginConfig{
            Name:    "redis-test",
            Version: "0.1.0",
        },
        Backend: map[string]any{
            "address": "localhost:6379",
        },
    }

    // Use harness for lifecycle management
    harness := common.NewPatternHarness(t, redisPlugin, config)
    defer harness.Cleanup()

    // Wait for healthy state
    err := harness.WaitForHealthy(5 * time.Second)
    require.NoError(t, err)

    // Run your tests...
}
```

**Features**:
- Automatic Initialize + Start
- Health polling with configurable timeout
- Cleanup registration
- Context management

#### Backend Utilities (`tests/testing/backends/`)

Centralized container setup for cross-cutting concerns:

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

Available backends:
- `backends.SetupRedis(t, ctx)` - Redis 7 Alpine
- `backends.SetupNATS(t, ctx)` - NATS 2.10 Alpine

## Running Tests

### Via Makefile (Recommended)

```bash
# Run all acceptance tests
make test-acceptance

# Run Redis acceptance tests only
make test-acceptance-redis

# Run NATS acceptance tests only
make test-acceptance-nats

# Run all acceptance tests in quiet mode (suppress container logs)
make test-acceptance-quiet

# Generate coverage reports
make coverage-acceptance
make coverage-acceptance-redis
make coverage-acceptance-nats
```

### Quiet Mode

Testcontainers produces verbose Docker logs by default. For cleaner output:

```bash
# Environment variable
export PRISM_TEST_QUIET=1
make test-acceptance

# Or use the dedicated target
make test-acceptance-quiet
```

**Benefits of Quiet Mode**:
- Cleaner test output (no Docker container logs)
- Easier to spot test failures
- Better for CI/CD pipelines
- Reduces log noise by ~90%

### Via Go Test Directly

```bash
# All acceptance tests
cd tests/acceptance && go test -v -timeout 10m ./...

# Redis tests only
cd tests/acceptance/redis && go test -v -timeout 10m ./...

# NATS tests only
cd tests/acceptance/nats && go test -v -timeout 10m ./...

# With coverage
cd tests/acceptance/redis && go test -coverprofile=coverage.out -timeout 10m ./...
go tool cover -html=coverage.out -o coverage.html
```

## Test Organization

### Redis Acceptance Tests (`redis/redis_integration_test.go`)

**13 tests covering**:

1. **Basic Operations** (8 subtests)
   - Set and Get
   - Get Non-Existent Key
   - Delete
   - Exists
   - TTL Expiration
   - Multiple Keys
   - Overwrite Existing Key
   - Binary Data

2. **Concurrent Operations** (2 subtests)
   - Concurrent Writes
   - Concurrent Reads

3. **Health Check** (1 subtest)
   - Healthy Status

4. **Error Handling** (2 subtests)
   - Delete Non-Existent Key
   - Large Value

**Runtime**: ~5.7 seconds

### NATS Acceptance Tests (`nats/nats_integration_test.go`)

**6 test functions covering**:

1. **Basic Pub/Sub**
   - Simple publish and subscribe
   - Message delivery
   - Unsubscribe

2. **Fanout Pattern**
   - Multiple subscribers
   - Broadcast messages

3. **Message Ordering**
   - Sequential delivery

4. **Concurrent Operations**
   - Multiple publishers
   - Multiple subscribers

5. **Health Check**
   - Healthy status
   - Connection validation

6. **Wildcard Subscriptions**
   - Pattern matching (e.g., `events.*`)
   - Multiple topic delivery

**Runtime**: ~5.9 seconds

## Writing New Acceptance Tests

### 1. Create Test Package

```bash
mkdir -p tests/acceptance/mypattern
cd tests/acceptance/mypattern
```

### 2. Create Test File

```go
package mypattern_test

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/jrepp/prism-data-layer/tests/acceptance/common"
    "github.com/jrepp/prism-data-layer/tests/testing/backends"
    "github.com/jrepp/prism-data-layer/patterns/mypattern"
    "github.com/jrepp/prism-data-layer/patterns/core"
)

func TestMyPattern_BasicOperations(t *testing.T) {
    ctx := context.Background()

    // Start backend container
    backend := backends.SetupMyBackend(t, ctx)
    defer backend.Cleanup()

    // Create pattern instance
    plugin := mypattern.New()

    // Configure pattern
    config := &core.Config{
        Plugin: core.PluginConfig{
            Name:    "mypattern-test",
            Version: "0.1.0",
        },
        Backend: map[string]any{
            "url": backend.ConnectionString,
        },
    }

    // Use harness for lifecycle
    harness := common.NewPatternHarness(t, plugin, config)
    defer harness.Cleanup()

    // Wait for healthy
    err := harness.WaitForHealthy(5 * time.Second)
    require.NoError(t, err, "Plugin did not become healthy")

    // Run subtests
    t.Run("Operation 1", func(t *testing.T) {
        // Test operation 1
    })

    t.Run("Operation 2", func(t *testing.T) {
        // Test operation 2
    })
}
```

### 3. Add Backend Setup (if needed)

If your pattern requires a new backend, add it to `tests/testing/backends/`:

```go
// tests/testing/backends/mybackend.go
package backends

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    tcmybackend "github.com/testcontainers/testcontainers-go/modules/mybackend"
)

type MyBackendBackend struct {
    ConnectionString string
    cleanup          func()
}

func SetupMyBackend(t *testing.T, ctx context.Context) *MyBackendBackend {
    t.Helper()

    // Start container
    container, err := tcmybackend.Run(ctx, "mybackend:latest")
    require.NoError(t, err, "Failed to start MyBackend container")

    // Get connection string
    connStr, err := container.ConnectionString(ctx)
    require.NoError(t, err, "Failed to get connection string")

    return &MyBackendBackend{
        ConnectionString: connStr,
        cleanup: func() {
            if err := container.Terminate(ctx); err != nil {
                t.Logf("Failed to terminate container: %v", err)
            }
        },
    }
}

func (b *MyBackendBackend) Cleanup() {
    if b.cleanup != nil {
        b.cleanup()
    }
}
```

### 4. Add Makefile Targets

Add targets to `/Makefile`:

```makefile
test-acceptance-mypattern: ## Run MyPattern acceptance tests
	$(call print_blue,Running MyPattern acceptance tests...)
	@cd tests/acceptance/mypattern && go test -v -timeout 10m ./...
	$(call print_green,MyPattern acceptance tests passed)

coverage-acceptance-mypattern: ## Generate MyPattern acceptance test coverage
	$(call print_blue,Generating MyPattern acceptance test coverage...)
	@cd tests/acceptance/mypattern && go test -coverprofile=coverage.out -timeout 10m ./...
	@cd tests/acceptance/mypattern && go tool cover -func=coverage.out | grep total
	@cd tests/acceptance/mypattern && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,MyPattern acceptance coverage: tests/acceptance/mypattern/coverage.html)
```

Update aggregate targets:

```makefile
test-acceptance: test-acceptance-redis test-acceptance-nats test-acceptance-mypattern
coverage-acceptance: coverage-acceptance-redis coverage-acceptance-nats coverage-acceptance-mypattern
```

## CI Integration

### GitHub Actions

```yaml
name: Acceptance Tests

on: [push, pull_request]

jobs:
  acceptance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run acceptance tests
        run: make test-acceptance

      - name: Generate coverage
        run: make coverage-acceptance

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: |
            tests/acceptance/redis/coverage.out
            tests/acceptance/nats/coverage.out
```

### Local Development

Enable pre-commit checks in `.githooks/pre-commit`:

```bash
#!/bin/bash
set -e

echo "Running acceptance tests..."
make test-acceptance

echo "Checking coverage..."
make coverage-acceptance
```

Install hooks:

```bash
git config core.hooksPath .githooks
```

## Test Coverage Goals

| Component | Minimum | Target | Current |
|-----------|---------|--------|---------|
| Redis Tests | 80% | 85% | ~85% |
| NATS Tests | 80% | 85% | ~85% |
| PatternHarness | 90% | 95% | N/A |

## Best Practices

### 1. Use testcontainers for Real Backends

❌ **Don't**: Use mocks or in-memory fakes
```go
mockRedis := &MockRedis{}
```

✅ **Do**: Use real backend containers
```go
backend := backends.SetupRedis(t, ctx)
defer backend.Cleanup()
```

### 2. Always Use PatternHarness

❌ **Don't**: Manually manage lifecycle
```go
err := plugin.Initialize(ctx, config)
err = plugin.Start(ctx)
// ... test code ...
plugin.Stop(ctx)
```

✅ **Do**: Use PatternHarness for consistency
```go
harness := common.NewPatternHarness(t, plugin, config)
defer harness.Cleanup()
err := harness.WaitForHealthy(5 * time.Second)
```

### 3. Use Subtests for Organization

❌ **Don't**: Create one test per operation
```go
func TestSet(t *testing.T) { ... }
func TestGet(t *testing.T) { ... }
func TestDelete(t *testing.T) { ... }
```

✅ **Do**: Group related operations in subtests
```go
func TestBasicOperations(t *testing.T) {
    // ... setup ...

    t.Run("Set", func(t *testing.T) { ... })
    t.Run("Get", func(t *testing.T) { ... })
    t.Run("Delete", func(t *testing.T) { ... })
}
```

### 4. Set Appropriate Timeouts

- **Unit tests**: Default (10s)
- **Acceptance tests**: 10 minutes (`-timeout 10m`)
- **Health checks**: 5 seconds
- **Message delivery**: 2-3 seconds

### 5. Clean Up Resources

Always use `defer` for cleanup:

```go
backend := backends.SetupRedis(t, ctx)
defer backend.Cleanup()  // Always cleanup

harness := common.NewPatternHarness(t, plugin, config)
defer harness.Cleanup()  // Always cleanup
```

## Troubleshooting

### Tests Time Out

**Problem**: Tests hang or timeout after 10 minutes

**Solutions**:
1. Check if Docker daemon is running: `docker ps`
2. Check container logs: `docker logs <container-id>`
3. Increase timeout: `go test -timeout 20m ./...`
4. Check if pattern's `Start()` method is blocking (should return immediately)

### Container Fails to Start

**Problem**: `Failed to start Redis container` error

**Solutions**:
1. Pull image manually: `docker pull redis:7-alpine`
2. Check Docker disk space: `docker system df`
3. Clean up old containers: `docker system prune`

### Health Check Never Succeeds

**Problem**: `Plugin did not become healthy` error

**Solutions**:
1. Verify pattern implements `Health()` correctly
2. Check if `Initialize()` completes successfully
3. Check if `Start()` returns immediately (shouldn't block)
4. Increase health check timeout from 5s to 10s

### Connection Refused Errors

**Problem**: `dial tcp 127.0.0.1:6379: connect: connection refused`

**Solutions**:
1. Verify backend setup returns correct connection string
2. Check if connection string needs prefix stripping (Redis: `redis://` → ``)
3. Wait longer for container to be ready (testcontainers should handle this)

## Related Documentation

- [RFC-015: Plugin Acceptance Test Framework](/rfc/rfc-015)
- [RFC-018: POC Implementation Strategy](/rfc/rfc-018)
- [Testing Infrastructure README](../testing/README.md)
- [testcontainers-go Documentation](https://golang.testcontainers.org/)
- [Prism CLAUDE.md](../../CLAUDE.md) - TDD workflow and coverage requirements

## Future Enhancements

### Planned

- [ ] PostgreSQL acceptance tests
- [ ] Kafka acceptance tests
- [ ] Load testing integration
- [ ] Performance benchmarks
- [ ] Test data fixtures

### Under Consideration

- [ ] Parallel test execution
- [ ] Test result reporting dashboard
- [ ] Chaos engineering tests (container failures)
- [ ] Multi-backend patterns (composite tests)
- [ ] Shadow traffic testing
