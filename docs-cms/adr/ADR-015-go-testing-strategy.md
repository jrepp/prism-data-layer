---
title: "ADR-015: Go Testing Strategy"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['go', 'testing', 'quality', 'ci-cd']
---

## Context

We need a comprehensive testing strategy for Go tooling that:
- Ensures correctness at multiple levels
- Maintains 80%+ code coverage
- Supports rapid development
- Catches regressions early
- Validates integration points with Prism proxy

Testing pyramid: Unit tests (base) → Integration tests → E2E tests (top)

## Decision

Implement **three-tier testing strategy**:

1. **Unit Tests**: Package-level, test individual functions
2. **Integration Tests**: Test package interactions and proxy integration
3. **E2E Tests**: Validate full CLI workflows with real backends

### Coverage Requirements

- **Minimum**: 80% per package (CI enforced)
- **Target**: 90%+ for critical packages (`internal/config`, `internal/migrate`)
- **New code**: 100% coverage required

## Rationale

### Testing Tiers

#### Tier 1: Unit Tests

**Scope**: Individual functions and types within a package

**Location**: `*_test.go` files alongside source

**Pattern**:
```go
package config

import "testing"

func TestValidateConfig_Valid(t *testing.T) {
    cfg := &Config{
        Namespace: "test",
        Backend:   "postgres",
    }
    if err := ValidateConfig(cfg); err != nil {
        t.Fatalf("ValidateConfig() error = %v", err)
    }
}

func TestValidateConfig_MissingNamespace(t *testing.T) {
    cfg := &Config{Backend: "postgres"}
    err := ValidateConfig(cfg)
    if !errors.Is(err, ErrInvalidConfig) {
        t.Errorf("expected ErrInvalidConfig, got %v", err)
    }
}
```

**Characteristics**:
- Fast (milliseconds)
- No external dependencies
- Use table-driven tests for multiple scenarios
- Mock external interfaces

#### Tier 2: Integration Tests

**Scope**: Package interactions, integration with Prism proxy

**Location**: `*_integration_test.go`

**Pattern**:
```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/prism/tools/internal/migrate"
    "github.com/prism/tools/testutil"
)

func TestMigrate_PostgresToSqlite(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Start test Prism proxy
    proxy := testutil.StartTestProxy(t, testutil.ProxyConfig{
        Backends: []string{"postgres", "sqlite"},
    })
    defer proxy.Stop()

    // Run migration
    ctx := context.Background()
    err := migrate.Run(ctx, migrate.Config{
        Source: "postgres://localhost/test",
        Dest:   "sqlite://test.db",
    })
    if err != nil {
        t.Fatalf("migrate.Run() error = %v", err)
    }

    // Verify data migrated
    // ...
}
```

#### Tier 3: End-to-End Tests

**Scope**: Full CLI workflows with real Prism proxy

**Location**: `cmd/*/e2e_test.go`

**Pattern**:
```go
package main_test

import (
    "bytes"
    "os/exec"
    "testing"
)

func TestCLI_Get_E2E(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping e2e test in short mode")
    }

    // Run prism-cli binary
    cmd := exec.Command("./bin/prism-cli", "get", "test", "user123", "profile")
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        t.Fatalf("prism-cli failed: %v\nstderr: %s", err, stderr.String())
    }

    // Validate output
    output := stdout.String()
    if !strings.Contains(output, "value") {
        t.Errorf("expected value in output, got: %s", output)
    }
}
```

### Test Harness for Proxy Integration

```go
package testutil

import (
    "os"
    "os/exec"
    "testing"
    "time"
)

type ProxyConfig struct {
    Port     int
    Backends []string
}

type TestProxy struct {
    cmd     *exec.Cmd
    cleanup func()
}

func (p *TestProxy) Stop() { p.cleanup() }

func StartTestProxy(t *testing.T, cfg ProxyConfig) *TestProxy {
    t.Helper()

    // Start proxy in background
    cmd := exec.Command("../proxy/target/release/prism-proxy", "--port", fmt.Sprintf("%d", cfg.Port))
    if err := cmd.Start(); err != nil {
        t.Fatal(err)
    }

    // Wait for proxy to be ready
    time.Sleep(1 * time.Second)

    return &TestProxy{
        cmd: cmd,
        cleanup: func() {
            cmd.Process.Kill()
            cmd.Wait()
        },
    }
}
```

## Consequences

### Positive

- High confidence in correctness (three validation levels)
- Fast feedback loop (unit tests ~seconds)
- Integration tests catch proxy interaction bugs
- E2E tests validate production behavior

### Negative

- More code to maintain (tests often 2x source code)
- Integration tests require Prism proxy running
- E2E tests slower (seconds to minutes)

### Neutral

- 80%+ coverage requirement enforced in CI

## Implementation Notes

### Directory Structure

tools/
├── cmd/
│   ├── prism-cli/
│   │   ├── main.go
│   │   ├── main_test.go       # Unit tests
│   │   └── e2e_test.go        # E2E tests
│   └── prism-migrate/
│       ├── main.go
│       └── main_test.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   ├── config_test.go
│   │   └── config_integration_test.go
│   └── migrate/
│       ├── migrate.go
│       └── migrate_test.go
└── testutil/                   # Test harness
    ├── proxy.go
    └── fixtures.go
```

### Running Tests

```bash
# Unit tests only (fast)
go test ./... -short

# All tests including integration
go test ./...

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# E2E only
go test ./cmd/... -run E2E

# Specific package
go test ./internal/migrate -v
```

### CI Configuration

```yaml
# .github/workflows/go-test.yml
jobs:
  test:
    steps:
      - name: Unit Tests
        run: |
          cd tools
          go test ./... -short -coverprofile=coverage.out

      - name: Build Proxy (for integration tests)
        run: |
          cd proxy
          cargo build --release

      - name: Integration Tests
        run: |
          cd tools
          go test ./...

      - name: Coverage Check
        run: |
          go tool cover -func=coverage.out | grep total | \
          awk '{if ($3 < 80.0) {print "Coverage below 80%"; exit 1}}'
```

## References

- [Go Testing Documentation](https://go.dev/doc/tutorial/add-a-test)
- [Table Driven Tests in Go](https://go.dev/wiki/TableDrivenTests)
- ADR-012: Go for Tooling
- ADR-014: Go Concurrency Patterns
- org-stream-producer ADR-007: Testing Strategy

## Revision History

- 2025-10-07: Initial draft and acceptance (adapted from org-stream-producer)
