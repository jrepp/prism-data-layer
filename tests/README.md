# Prism Testing Architecture

This directory contains the comprehensive testing framework for Prism data access patterns and backends.

## Architecture Overview

The testing architecture is designed around **interface-driven testing**:

1. **Interface Test Suites** - Reusable test libraries for data access pattern interfaces
2. **Backend-Specific Tests** - Per-backend tests that import and run interface suites
3. **Pattern Runner** - Command wrapper that discovers interfaces and runs appropriate suites

```
tests/
├── interface-suites/        # ✅ Reusable test suite libraries
│   ├── keyvalue_basic/      # Tests for KeyValueBasic interface
│   ├── keyvalue_ttl/        # Tests for KeyValueTTL with expiration
│   ├── pubsub/              # Tests for PubSub interface
│   └── queue/               # Tests for Queue interface
│
├── backends/                # ✅ Backend-specific tests
│   ├── redis/               # Redis driver tests
│   │   ├── keyvalue_basic/  # KeyValueBasic suite for Redis
│   │   │   ├── go.mod
│   │   │   └── keyvalue_basic_test.go
│   │   └── keyvalue_ttl/    # KeyValueTTL suite for Redis
│   │       ├── go.mod
│   │       └── keyvalue_ttl_test.go
│   │
│   ├── memstore/            # MemStore driver tests
│   │   ├── keyvalue_basic/
│   │   │   ├── go.mod
│   │   │   └── keyvalue_basic_test.go
│   │   └── keyvalue_ttl/
│   │       ├── go.mod
│   │       └── keyvalue_ttl_test.go
│   │
│   └── nats/                # NATS driver tests
│       ├── pubsub/
│       └── queue/
│
└── acceptance/
    └── pattern-runner/      # ✅ Command wrapper for pattern testing
        ├── go.mod
        └── main.go
```

## Key Concepts

### 1. Interface Test Suites (Reusable)

Interface suites are **backend-agnostic** test libraries. Each suite tests a single data access pattern interface:

```go
// tests/interface-suites/keyvalue_basic/suite.go
package keyvalue_basic

type Suite struct {
    CreateBackend  func(t *testing.T) plugin.KeyValueBasicInterface
    CleanupBackend func(t *testing.T, backend plugin.KeyValueBasicInterface)
}

func (s *Suite) Run(t *testing.T) {
    t.Run("SetAndGet", s.TestSetAndGet)
    t.Run("Delete", s.TestDelete)
    t.Run("Exists", s.TestExists)
    // ...
}
```

**Benefits:**
- Write tests once, run against any backend
- Ensures all backends have consistent behavior
- Easy to add new backends - just import the suite

### 2. Backend-Specific Tests (Concrete)

Backend tests import interface suites and provide backend-specific setup/teardown:

```go
// tests/backends/redis/keyvalue_basic/keyvalue_basic_test.go
func TestRedisKeyValueBasic(t *testing.T) {
    suite := &keyvalue_basic.Suite{
        CreateBackend: func(t *testing.T) plugin.KeyValueBasicInterface {
            // Start Redis container
            // Initialize Redis driver
            return redisDriver
        },
        CleanupBackend: func(t *testing.T, backend plugin.KeyValueBasicInterface) {
            // Stop driver, cleanup container
        },
    }
    suite.Run(t)  // Runs all KeyValueBasic tests against Redis
}
```

**Benefits:**
- Each backend module is self-contained
- Can test interface isolation (e.g., only KeyValueBasic)
- No dependencies between backend tests

### 3. Pattern Runner (Acceptance)

The pattern-runner is a command-line tool that:

1. Takes a compiled data access pattern executable
2. Starts the pattern
3. Discovers supported interfaces via `InterfaceSupport.ListInterfaces()`
4. Runs appropriate interface suites

```bash
# Build a pattern executable
cd patterns/multicast_registry && go build -o pattern-exe

# Run acceptance tests
pattern-runner -pattern pattern-exe
```

**Output:**
```
Pattern Runner: Testing pattern-exe
Discovered 3 interfaces: [KeyValueBasic, PubSub, Registry]

🧪 Running test suite for KeyValueBasic
✅ KeyValueBasic tests passed (5/5)

🧪 Running test suite for PubSub
✅ PubSub tests passed (8/8)

🧪 Running test suite for Registry
✅ Registry tests passed (12/12)

✅ All test suites passed
```

## Running Tests

### Backend-Specific Tests

```bash
# Test Redis driver KeyValueBasic interface
cd tests/backends/redis/keyvalue_basic && go test -v

# Test MemStore driver KeyValueBasic interface
cd tests/backends/memstore/keyvalue_basic && go test -v

# Test all Redis interfaces
cd tests/backends/redis && go test -v ./...

# Test all backends
cd tests/backends && go test -v ./...
```

### Acceptance Tests (Pattern Runner)

```bash
# Build pattern-runner
cd tests/acceptance/pattern-runner && go build -o pattern-runner

# Test a pattern
./pattern-runner -pattern /path/to/pattern-executable -v
```

### Full Test Suite

```bash
# From repository root
make test-backends        # All backend tests
make test-acceptance      # All acceptance tests
make test                 # Everything
```

## Adding New Interfaces

### 1. Create Interface Suite

```bash
mkdir -p tests/interface-suites/new_interface
```

```go
// tests/interface-suites/new_interface/suite.go
package new_interface

import (
    "testing"
    "github.com/jrepp/prism-data-layer/pkg/plugin"
)

type Suite struct {
    CreateBackend  func(t *testing.T) plugin.NewInterface
    CleanupBackend func(t *testing.T, backend plugin.NewInterface)
}

func (s *Suite) Run(t *testing.T) {
    t.Run("TestFeature1", s.TestFeature1)
    t.Run("TestFeature2", s.TestFeature2)
}

func (s *Suite) TestFeature1(t *testing.T) {
    backend := s.CreateBackend(t)
    defer s.CleanupBackend(t, backend)
    // Test feature 1
}
```

### 2. Add Backend Tests

```bash
mkdir -p tests/backends/redis/new_interface
```

```go
// tests/backends/redis/new_interface/new_interface_test.go
package new_interface_test

import (
    "testing"
    "github.com/jrepp/prism-data-layer/tests/interface-suites/new_interface"
)

func TestRedisNewInterface(t *testing.T) {
    suite := &new_interface.Suite{
        CreateBackend:  createRedisBackend,
        CleanupBackend: cleanupRedisBackend,
    }
    suite.Run(t)
}
```

### 3. Update Pattern Runner

Add interface to `runSuiteForInterface()` in `tests/acceptance/pattern-runner/main.go`.

## Benefits of This Architecture

1. **DRY** - Write interface tests once, run everywhere
2. **Isolation** - Test each interface independently
3. **Consistency** - All backends guaranteed to behave the same
4. **Modularity** - Each backend/interface combination is a separate module
5. **Scalability** - Easy to add new backends and interfaces
6. **CI-Friendly** - Can test specific backend/interface combos in parallel

## Example: Testing a New Backend

```bash
# Add new backend (e.g., PostgreSQL)
mkdir -p tests/backends/postgres/keyvalue_basic

# Import existing suite
cat > tests/backends/postgres/keyvalue_basic/keyvalue_basic_test.go <<EOF
package keyvalue_basic_test

import (
    "testing"
    "github.com/jrepp/prism-data-layer/tests/interface-suites/keyvalue_basic"
)

func TestPostgresKeyValueBasic(t *testing.T) {
    suite := &keyvalue_basic.Suite{
        CreateBackend:  createPostgresBackend,
        CleanupBackend: cleanupPostgresBackend,
    }
    suite.Run(t)
}

func createPostgresBackend(t *testing.T) plugin.KeyValueBasicInterface {
    // Start Postgres container
    // Initialize Postgres driver
    return postgresDriver
}

func cleanupPostgresBackend(t *testing.T, backend plugin.KeyValueBasicInterface) {
    // Cleanup
}
EOF

# Run tests
go test -v
```

✅ **Zero new test code** - just setup/teardown!

## Migration Path

Old acceptance tests in `tests/acceptance/{interfaces,redis,nats}` will be gradually migrated to this new structure:

1. Extract interface tests into `tests/interface-suites/`
2. Create backend-specific modules in `tests/backends/`
3. Remove old acceptance directories once migration is complete

## See Also

- [CLAUDE.md](../CLAUDE.md) - Project development guide
- [RFC-015: Plugin Acceptance Test Framework](../docs-cms/rfcs/RFC-015-plugin-acceptance-test-framework.md)
- [ADR-026: Distroless Container Images](../docs-cms/adr/ADR-026-distroless-container-images.md)
