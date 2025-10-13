#!/usr/bin/env python3
"""Update RFC-022 with build system sections and pattern terminology.

Updates:
1. plugin-sdk → pattern-sdk
2. examples/ → patterns/
3. Add comprehensive build system sections
"""

import re
import sys
from pathlib import Path


def main():
    rfc_path = Path("/Users/jrepp/dev/data-access/docs-cms/rfcs/RFC-022-core-plugin-sdk-code-layout.md")
    content = rfc_path.read_text()

    # Update module paths: plugin-sdk → pattern-sdk
    content = re.sub(r"github\.com/prism/plugin-sdk", "github.com/prism/pattern-sdk", content)

    # Update directory: examples/ → patterns/
    content = re.sub(r"examples/", "patterns/", content)
    content = re.sub(r"\bexamples\b", "patterns", content)

    # Update "Example Plugin" → "Example Pattern"
    content = re.sub(r"Example Plugin", "Example Pattern", content)

    # Add new build system sections before "Open Questions"
    build_sections = """

## Build System and Tooling

### Comprehensive Makefile Structure

The pattern SDK uses a hierarchical Makefile system:

```makefile
# pattern-sdk/Makefile
.PHONY: all build test test-unit test-integration lint proto clean coverage validate install-tools

# Default target
all: validate test build

# Install development tools
install-tools:
\t@echo "Installing development tools..."
\tgo install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
\tgo install google.golang.org/protobuf/cmd/protoc-gen-go@latest
\tgo install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate protobuf code
proto:
\t@echo "Generating protobuf code..."
\t./tools/proto-gen.sh

# Build SDK (verify compilation)
build:
\t@echo "Building SDK..."
\t@CGO_ENABLED=0 go build ./...

# Run all tests
test: test-unit test-integration

# Unit tests (fast, no external dependencies)
test-unit:
\t@echo "Running unit tests..."
\t@go test -v -race -short -coverprofile=coverage-unit.out ./...

# Integration tests (requires testcontainers)
test-integration:
\t@echo "Running integration tests..."
\t@go test -v -race -run Integration -coverprofile=coverage-integration.out ./...

# Lint code
lint:
\t@echo "Linting..."
\t@golangci-lint run ./...

# Coverage report
coverage:
\t@echo "Generating coverage report..."
\t@go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
\t@go tool cover -html=coverage.out -o coverage.html
\t@echo "Coverage report: coverage.html"

# Compile-time validation
validate: validate-interfaces validate-slots

validate-interfaces:
\t@echo "Validating interface implementations..."
\t@./tools/validate-interfaces.sh

validate-slots:
\t@echo "Validating slot configurations..."
\t@go run tools/validate-slots/main.go

# Clean build artifacts
clean:
\t@echo "Cleaning..."
\t@rm -rf proto/*.pb.go coverage*.out coverage.html
\t@go clean -cache -testcache

# Format code
fmt:
\t@echo "Formatting code..."
\t@go fmt ./...
\t@goimports -w .

# Release (tag and push)
release:
\t@echo "Releasing..."
\t@./tools/release.sh
```

### Pattern-Specific Makefiles

Each pattern has its own Makefile:

```makefile
# patterns/multicast-registry/Makefile
PATTERN_NAME := multicast-registry
BINARY_NAME := $(PATTERN_NAME)

.PHONY: all build test lint run clean

all: test build

# Build pattern binary
build:
\t@echo "Building $(PATTERN_NAME)..."
\t@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \\
\t\t-ldflags="-s -w" \\
\t\t-o bin/$(BINARY_NAME) \\
\t\t./cmd/$(PATTERN_NAME)

# Build Docker image
docker:
\t@echo "Building Docker image..."
\t@docker build -t prism/$(PATTERN_NAME):latest .

# Run tests
test:
\t@echo "Running tests..."
\t@go test -v -race -cover ./...

# Run linter
lint:
\t@echo "Linting..."
\t@golangci-lint run ./...

# Run pattern locally
run: build
\t@echo "Running $(PATTERN_NAME)..."
\t@./bin/$(BINARY_NAME) -config config/local.yaml

# Clean build artifacts
clean:
\t@echo "Cleaning..."
\t@rm -rf bin/
\t@go clean -cache
```

### Build Targets Reference

| Target | Description | When to Use |
|--------|-------------|-------------|
| `make all` | Validate, test, build | Default CI/CD target |
| `make build` | Compile SDK packages | Verify compilation |
| `make test` | Run all tests | Before commit |
| `make test-unit` | Fast unit tests only | During development |
| `make test-integration` | Slow integration tests | Pre-push, CI/CD |
| `make lint` | Run linters | Pre-commit hook |
| `make coverage` | Generate coverage report | Coverage gates |
| `make validate` | Compile-time checks | Pre-commit, CI/CD |
| `make proto` | Regenerate protobuf | After .proto changes |
| `make clean` | Remove artifacts | Clean slate rebuild |
| `make fmt` | Format code | Auto-fix style |

## Compile-Time Validation

### Interface Implementation Checks

Use Go's compile-time type assertions to verify interface implementation:

```go
// interfaces/assertions.go
package interfaces

// Compile-time assertions for KeyValue interfaces
var (
    _ KeyValueBasic          = (*assertKeyValueBasic)(nil)
    _ KeyValueScan           = (*assertKeyValueScan)(nil)
    _ KeyValueTTL            = (*assertKeyValueTTL)(nil)
    _ KeyValueTransactional  = (*assertKeyValueTransactional)(nil)
    _ KeyValueBatch          = (*assertKeyValueBatch)(nil)
)

// Assertion types (never instantiated)
type assertKeyValueBasic struct{}
type assertKeyValueScan struct{}
type assertKeyValueTTL struct{}
type assertKeyValueTransactional struct{}
type assertKeyValueBatch struct{}

// Methods must exist or compilation fails
func (a *assertKeyValueBasic) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
    panic("assertion type")
}

func (a *assertKeyValueBasic) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
    panic("assertion type")
}

// ... other methods
```

### Pattern Interface Validation

Patterns can validate interface implementation at compile time:

```go
// patterns/multicast-registry/pattern.go
package multicast_registry

import (
    "github.com/prism/pattern-sdk/interfaces"
    "github.com/prism/pattern-sdk/lifecycle"
)

// Compile-time assertions
var (
    _ lifecycle.Pattern = (*Pattern)(nil)  // Implements Pattern interface
    _ interfaces.KeyValueScanDriver = (*registryBackend)(nil)  // Registry backend
    _ interfaces.PubSubDriver = (*messagingBackend)(nil)  // Messaging backend
)

type Pattern struct {
    // ... fields
}

// Pattern interface methods
func (p *Pattern) Name() string { return "multicast-registry" }
func (p *Pattern) Initialize(ctx context.Context, config *lifecycle.Config, backends map[string]interface{}) error { /* ... */ }
func (p *Pattern) Start(ctx context.Context) error { /* ... */ }
func (p *Pattern) Shutdown(ctx context.Context) error { /* ... */ }
func (p *Pattern) HealthCheck(ctx context.Context) error { /* ... */ }
```

### Validation Script

```bash
#!/usr/bin/env bash
# tools/validate-interfaces.sh
# Validates all interface implementations compile successfully

set -euo pipefail

echo "Validating interface implementations..."

# Compile interfaces package
if ! go build -o /dev/null ./interfaces/...; then
    echo "❌ Interface validation failed"
    exit 1
fi

# Check all patterns compile
for pattern_dir in patterns/*/; do
    pattern_name=$(basename "$pattern_dir")
    echo "  Checking pattern: $pattern_name"

    if ! (cd "$pattern_dir" && go build -o /dev/null ./...); then
        echo "  ❌ Pattern $pattern_name failed compilation"
        exit 1
    fi

    echo "  ✓ Pattern $pattern_name OK"
done

echo "✅ All interface validations passed"
```

### Slot Configuration Validation

Validate pattern slot configurations at build time:

```go
// tools/validate-slots/main.go
package main

import (
    "fmt"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

type SlotConfig struct {
    Name               string   `yaml:"name"`
    RequiredInterfaces []string `yaml:"required_interfaces"`
    Optional           bool     `yaml:"optional"`
}

type PatternConfig struct {
    Name  string       `yaml:"name"`
    Slots []SlotConfig `yaml:"slots"`
}

func main() {
    // Load all pattern configs
    matches, _ := filepath.Glob("patterns/*/pattern.yaml")

    for _, configPath := range matches {
        data, _ := os.ReadFile(configPath)

        var config PatternConfig
        if err := yaml.Unmarshal(data, &config); err != nil {
            fmt.Printf("❌ Invalid YAML: %s\n", configPath)
            os.Exit(1)
        }

        // Validate slots
        for _, slot := range config.Slots {
            if len(slot.RequiredInterfaces) == 0 && !slot.Optional {
                fmt.Printf("❌ Pattern %s: Required slot %s has no interfaces\n",
                    config.Name, slot.Name)
                os.Exit(1)
            }
        }

        fmt.Printf("✓ Pattern %s validated\n", config.Name)
    }

    fmt.Println("✅ All slot configurations valid")
}
```

## Linting Configuration

### golangci-lint Configuration

```yaml
# .golangci.yml
linters-settings:
  errcheck:
    check-type-assertions: true
    check-blank: true

  govet:
    enable-all: true

  gocyclo:
    min-complexity: 15

  goconst:
    min-len: 3
    min-occurrences: 3

  misspell:
    locale: US

  lll:
    line-length: 120

  gofmt:
    simplify: true

  goimports:
    local-prefixes: github.com/prism/pattern-sdk

linters:
  enable:
    - errcheck      # Unchecked errors
    - gosimple      # Simplify code
    - govet         # Vet examines Go source code
    - ineffassign   # Unused assignments
    - staticcheck   # Static analysis
    - typecheck     # Type checker
    - unused        # Unused constants, variables, functions
    - gofmt         # Formatting
    - goimports     # Import organization
    - misspell      # Spelling
    - goconst       # Repeated strings
    - gocyclo       # Cyclomatic complexity
    - lll           # Line length
    - dupl          # Duplicate code detection
    - gosec         # Security issues
    - revive        # Fast, configurable linter

  disable:
    - varcheck      # Deprecated
    - structcheck   # Deprecated
    - deadcode      # Deprecated

issues:
  exclude-rules:
    # Exclude some linters from test files
    - path: _test\\.go
      linters:
        - gocyclo
        - errcheck
        - dupl
        - gosec

    # Exclude generated files
    - path: \\.pb\\.go$
      linters:
        - all

  max-issues-per-linter: 0
  max-same-issues: 0

run:
  timeout: 5m
  tests: true
  skip-dirs:
    - proto
    - vendor
```

### Pre-Commit Hook

```bash
#!/usr/bin/env bash
# .githooks/pre-commit
# Runs linting and validation before commit

set -e

echo "🔍 Running pre-commit checks..."

# 1. Format check
echo "  Checking formatting..."
if ! make fmt > /dev/null 2>&1; then
    echo "  ❌ Code formatting required"
    echo "  Run: make fmt"
    exit 1
fi

# 2. Lint
echo "  Running linters..."
if ! make lint > /dev/null 2>&1; then
    echo "  ❌ Linting failed"
    echo "  Run: make lint"
    exit 1
fi

# 3. Validation
echo "  Validating interfaces..."
if ! make validate > /dev/null 2>&1; then
    echo "  ❌ Validation failed"
    echo "  Run: make validate"
    exit 1
fi

# 4. Unit tests
echo "  Running unit tests..."
if ! make test-unit > /dev/null 2>&1; then
    echo "  ❌ Tests failed"
    echo "  Run: make test-unit"
    exit 1
fi

echo "✅ Pre-commit checks passed"
```

### Installing Hooks

```bash
# Install hooks
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit

# Or copy to .git/hooks
cp .githooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Testing Infrastructure

### Test Organization

```text
pattern-sdk/
├── auth/
│   ├── token.go
│   ├── token_test.go           # Unit tests
│   └── token_integration_test.go  # Integration tests (build tag)
│
├── patterns/
│   ├── multicast-registry/
│   │   ├── pattern.go
│   │   ├── pattern_test.go     # Unit tests
│   │   └── integration_test.go # Integration tests
│   │
│   └── session-store/
│       ├── pattern.go
│       ├── pattern_test.go
│       └── integration_test.go
│
└── testing/
    ├── fixtures.go              # Test fixtures
    ├── containers.go            # Testcontainers helpers
    └── mock_*.go                # Mock implementations
```

### Test Build Tags

```go
// +build integration

package multicast_registry_test

import (
    "context"
    "testing"

    "github.com/testcontainers/testcontainers-go"
)

func TestMulticastRegistryIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Setup testcontainers...
}
```

### Coverage Requirements

```makefile
# Makefile - Coverage gates
COVERAGE_THRESHOLD := 80

test-coverage:
\t@echo "Running tests with coverage..."
\t@go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
\t@go tool cover -func=coverage.out -o coverage.txt
\t@COVERAGE=$$(grep total coverage.txt | awk '{print $$3}' | sed 's/%//'); \\
\tif [ $$(echo "$$COVERAGE < $(COVERAGE_THRESHOLD)" | bc) -eq 1 ]; then \\
\t\techo "❌ Coverage $$COVERAGE% is below threshold $(COVERAGE_THRESHOLD)%"; \\
\t\texit 1; \\
\tfi
\t@echo "✅ Coverage: $$(grep total coverage.txt | awk '{print $$3}')"
```

### Testcontainers Integration

```go
// testing/containers.go
package testing

import (
    "context"
    "time"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

// RedisContainer starts a Redis testcontainer
func RedisContainer(ctx context.Context) (testcontainers.Container, string, error) {
    req := testcontainers.ContainerRequest{
        Image:        "redis:7-alpine",
        ExposedPorts: []string{"6379/tcp"},
        WaitingFor: wait.ForLog("Ready to accept connections").
            WithStartupTimeout(30 * time.Second),
    }

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        return nil, "", err
    }

    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "6379")
    endpoint := host + ":" + port.Port()

    return container, endpoint, nil
}

// Usage in tests:
func TestWithRedis(t *testing.T) {
    ctx := context.Background()
    container, endpoint, err := testing.RedisContainer(ctx)
    require.NoError(t, err)
    defer container.Terminate(ctx)

    // Test using Redis at endpoint...
}
```

### Benchmark Tests

```go
// patterns/multicast-registry/benchmark_test.go
package multicast_registry_test

import (
    "context"
    "testing"
)

func BenchmarkPublishMulticast_10Subscribers(b *testing.B) {
    pattern := setupPattern(b, 10)
    event := createTestEvent()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        pattern.PublishMulticast(context.Background(), event)
    }
}

func BenchmarkPublishMulticast_100Subscribers(b *testing.B) {
    pattern := setupPattern(b, 100)
    event := createTestEvent()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        pattern.PublishMulticast(context.Background(), event)
    }
}

// Run benchmarks:
// go test -bench=. -benchmem ./patterns/multicast-registry/
```

### CI/CD Integration

```yaml
# .github/workflows/ci.yml (extended)
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
          cache: true

      - name: Install tools
        run: make install-tools

      - name: Validate interfaces
        run: make validate

      - name: Lint
        run: make lint

      - name: Unit tests
        run: make test-unit

      - name: Integration tests
        run: make test-integration

      - name: Coverage gate
        run: make test-coverage

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          file: ./coverage.out
          fail_ci_if_error: true

  build:
    runs-on: ubuntu-latest
    needs: test

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Build SDK
        run: make build

      - name: Build patterns
        run: |
          for pattern in patterns/*/; do
            echo "Building $(basename $pattern)..."
            (cd $pattern && make build)
          done
```

"""

    # Find insertion point (before "Open Questions")
    insertion_point = content.find("## Open Questions")
    if insertion_point == -1:
        print("Error: Could not find insertion point")
        return 1

    # Insert new content
    content = content[:insertion_point] + build_sections + "\n" + content[insertion_point:]

    # Write updated content
    rfc_path.write_text(content)
    print(f"✅ Updated {rfc_path}")
    print("   - Changed plugin-sdk → pattern-sdk")
    print("   - Changed examples/ → patterns/")
    print("   - Added build system sections")
    return 0


if __name__ == "__main__":
    sys.exit(main())
