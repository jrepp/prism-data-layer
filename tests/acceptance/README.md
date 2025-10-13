# Prism Acceptance Test Framework

**A world-class acceptance testing framework that makes implementing Prism backend drivers a joy.**

## 🎯 Purpose

This framework transforms backend testing from a chore into a delightful experience by:

1. **Pattern-Based Testing** - Tests organized by interface (KeyValue, PubSub), not backend
2. **Minimal Boilerplate** - Adding a new backend requires ~50 lines of code
3. **Comprehensive Coverage** - Automatic testing of happy paths, edge cases, and concurrency
4. **Actionable Reports** - Compliance matrix shows exactly what works and what doesn't
5. **Self-Documenting** - Tests demonstrate correct implementation

## 🚀 Quick Start

### Adding a New Backend (3 Steps)

**Step 1:** Implement the pattern interface

```go
// patterns/mybackend/mybackend.go
package mybackend

import "github.com/jrepp/prism-data-layer/patterns/core"

type MyBackend struct {
    // your implementation
}

func (m *MyBackend) Set(key string, value []byte, ttlSeconds int64) error {
    // your implementation
}

// ... implement other interface methods

var _ core.KeyValueBasicInterface = (*MyBackend)(nil) // compile-time check
```

**Step 2:** Register with test framework

```go
// tests/acceptance/backends/mybackend.go
package backends

import (
    "github.com/jrepp/prism-data-layer/tests/acceptance/framework"
)

func init() {
    framework.MustRegisterBackend(framework.Backend{
        Name:      "MyBackend",
        SetupFunc: setupMyBackend,

        SupportedPatterns: []framework.Pattern{
            framework.PatternKeyValueBasic,
        },

        Capabilities: framework.Capabilities{
            SupportsTTL:  true,
            MaxValueSize: 10 * 1024 * 1024, // 10MB
        },
    })
}

func setupMyBackend(t *testing.T, ctx context.Context) (interface{}, func()) {
    driver := mybackend.New()
    // ... initialize driver

    cleanup := func() {
        driver.Stop(ctx)
    }

    return driver, cleanup
}
```

**Step 3:** Run tests

```bash
go test ./tests/acceptance/patterns/keyvalue/...
```

**That's it!** Your backend now runs through the entire test suite automatically.

## 📊 Example Output

```
=== RUN   TestKeyValueBasicPattern
=== RUN   TestKeyValueBasicPattern/MemStore
=== RUN   TestKeyValueBasicPattern/MemStore/SetAndGet
=== RUN   TestKeyValueBasicPattern/MemStore/GetNonExistent
=== RUN   TestKeyValueBasicPattern/MemStore/LargeValue
--- PASS: TestKeyValueBasicPattern/MemStore (0.15s)

=== RUN   TestKeyValueBasicPattern/Redis
=== RUN   TestKeyValueBasicPattern/Redis/SetAndGet
=== RUN   TestKeyValueBasicPattern/Redis/GetNonExistent
=== RUN   TestKeyValueBasicPattern/Redis/LargeValue
--- PASS: TestKeyValueBasicPattern/Redis (0.42s)

PASS
```

## 📁 Framework Architecture

```
tests/acceptance/
├── README.md                    # This file
├── FRAMEWORK_DESIGN.md          # Detailed architecture docs
│
├── framework/                   # Core testing infrastructure
│   ├── types.go                 # Pattern, Backend, Capabilities types
│   ├── registry.go              # Backend registration system
│   ├── runner.go                # Test execution engine
│   └── reporter.go              # Compliance report generation
│
├── patterns/                    # Pattern test suites
│   ├── keyvalue/                # KeyValue pattern tests
│   │   ├── README.md            # KeyValue requirements
│   │   ├── basic_test.go        # CRUD operations
│   │   ├── ttl_test.go          # TTL/expiration
│   │   └── concurrent_test.go   # Concurrency tests
│   │
│   └── pubsub/                  # PubSub pattern tests
│       └── ... (TODO)
│
├── backends/                    # Backend implementations
│   ├── example.go               # Reference implementation
│   ├── memstore.go              # MemStore backend
│   ├── redis.go                 # Redis backend
│   └── nats.go                  # NATS backend
│
└── cmd/acceptance/              # CLI tools (TODO)
    └── main.go                  # Report generator, test runner
```

## 🧪 Supported Patterns

### KeyValue Pattern

Backends: MemStore ✅, Redis ✅, PostgreSQL ⏳

**Interfaces:**
- `KeyValueBasicInterface` - Set, Get, Delete, Exists
- `KeyValueTTLInterface` - TTL/expiration support
- `KeyValueScanInterface` - Iteration/scanning (TODO)
- `KeyValueAtomicInterface` - CAS, increment, decrement (TODO)

**Test Coverage:**
- ✅ Basic CRUD operations
- ✅ Binary-safe values
- ✅ TTL/expiration
- ✅ Concurrent operations
- ✅ Edge cases (large values, empty values)

See: [patterns/keyvalue/README.md](patterns/keyvalue/README.md)

### PubSub Pattern

Backends: NATS ✅, Redis ⏳, Kafka ⏳

**Interfaces:**
- `PubSubBasicInterface` - Publish, Subscribe, Unsubscribe
- `PubSubOrderingInterface` - Message ordering guarantees (TODO)
- `PubSubFanoutInterface` - Fan-out patterns (TODO)

See: [patterns/pubsub/README.md](patterns/pubsub/README.md) (TODO)

## 🎯 Running Tests

### All Patterns, All Backends

```bash
# Run entire acceptance test suite
go test ./tests/acceptance/...

# Run specific pattern
go test ./tests/acceptance/patterns/keyvalue/...
```

### Specific Backend

```bash
# Test single backend
go test ./tests/acceptance/patterns/keyvalue/... -run TestKeyValueBasicPattern/Redis

# Test single backend + single test
go test ./tests/acceptance/patterns/keyvalue/... -run TestKeyValueBasicPattern/Redis/SetAndGet
```

### With Race Detector

```bash
go test -race ./tests/acceptance/patterns/keyvalue/...
```

### Generate Compliance Report

```bash
# TODO: CLI for reports
go run ./tests/acceptance/cmd/acceptance/ report
```

## 🔧 Backend Capabilities

Declare what your backend supports via the `Capabilities` struct:

```go
Capabilities: framework.Capabilities{
    // Standard capabilities
    SupportsTTL:         true,  // Key expiration
    SupportsScan:        true,  // Iteration
    SupportsAtomic:      false, // CAS operations
    SupportsTransactions: false, // ACID transactions
    SupportsOrdering:    false, // Message ordering

    // Size limits (0 = unlimited)
    MaxValueSize: 5 * 1024 * 1024, // 5MB
    MaxKeySize:   512,               // 512 bytes

    // Custom capabilities
    Custom: map[string]interface{}{
        "SupportsSecondaryIndexes": true,
        "IsolationLevel":           "ReadCommitted",
    },
}
```

**Tests requiring unsupported capabilities are automatically skipped with clear messages.**

## 📝 Writing Pattern Tests

Pattern tests run against all compatible backends automatically:

```go
package keyvalue_test

import (
    "github.com/jrepp/prism-data-layer/tests/acceptance/framework"
    _ "github.com/jrepp/prism-data-layer/tests/acceptance/backends" // Register all
)

func TestKeyValueBasicPattern(t *testing.T) {
    tests := []framework.PatternTest{
        {
            Name: "SetAndGet",
            Func: testSetAndGet,
        },
        {
            Name: "TTLExpiration",
            Func: testTTLExpiration,
            RequiresCapability: "TTL", // Skipped if not supported
        },
    }

    framework.RunPatternTests(t, framework.PatternKeyValueBasic, tests)
}

func testSetAndGet(t *testing.T, driver interface{}, caps framework.Capabilities) {
    kv := driver.(core.KeyValueBasicInterface)

    err := kv.Set("key", []byte("value"), 0)
    require.NoError(t, err)

    value, found, err := kv.Get("key")
    require.NoError(t, err)
    assert.True(t, found)
    assert.Equal(t, []byte("value"), value)
}
```

**Key principles:**
- Use `t.Name()` in keys for test isolation
- Check capabilities via `RequiresCapability`
- Test both success and failure paths
- Keep tests fast (<100ms per test)

## 🏗️ How It Works

### 1. Backend Registration

Backends register via `init()` functions:

```go
func init() {
    framework.MustRegisterBackend(framework.Backend{
        Name:              "MyBackend",
        SetupFunc:         setupMyBackend,
        SupportedPatterns: []framework.Pattern{framework.PatternKeyValueBasic},
        Capabilities:      framework.Capabilities{...},
    })
}
```

### 2. Test Execution Flow

1. **Discovery**: Framework finds all backends for a pattern
2. **Setup**: For each backend, call `SetupFunc` to create driver
3. **Execution**: Run all tests against the driver in parallel
4. **Capability Check**: Skip tests requiring unsupported capabilities
5. **Cleanup**: Call cleanup function
6. **Reporting**: Generate compliance matrix (TODO)

### 3. Parallel Execution

Tests run in parallel across backends for speed:

```
=== RUN   TestKeyValueBasicPattern
=== RUN   TestKeyValueBasicPattern/MemStore   # All run in parallel
=== RUN   TestKeyValueBasicPattern/Redis
=== RUN   TestKeyValueBasicPattern/Postgres
```

## 🐛 Debugging

### Verbose Output

```bash
go test -v ./tests/acceptance/patterns/keyvalue/...
```

### Single Backend, Single Test

```bash
go test ./tests/acceptance/patterns/keyvalue/... \
  -run TestKeyValueBasicPattern/MyBackend/SetAndGet -v
```

### Add Debug Logging

```go
func setupMyBackend(t *testing.T, ctx context.Context) (interface{}, func()) {
    t.Logf("Setting up MyBackend...")
    driver := mybackend.New()
    t.Logf("Driver created: %+v", driver)
    // ...
}
```

### Common Issues

**Tests not running?**
- Ensure backend package is imported: `_ "path/to/backends"`
- Check that `init()` registers backend without panicking
- Verify `SupportedPatterns` includes the pattern being tested

**Tests skipping?**
- Check `Capabilities` match test requirements
- Look for "skipped" messages in verbose output

**Tests failing?**
- Read error messages carefully - they're designed to be actionable
- Run single test with `-v` flag for details
- Check driver Health() status
- Verify interface compliance

## 📚 Resources

- **Framework Design**: [FRAMEWORK_DESIGN.md](FRAMEWORK_DESIGN.md) - Comprehensive architecture guide
- **KeyValue Pattern**: [patterns/keyvalue/README.md](patterns/keyvalue/README.md) - KeyValue interface requirements
- **Example Backend**: [backends/example.go](backends/example.go) - Fully documented reference implementation
- **Core Interfaces**: [patterns/core/interfaces.go](../../patterns/core/interfaces.go) - Interface definitions

## 🎓 Philosophy

> "The best documentation is code that shows how things should work."

This framework treats tests as:
1. **Specifications** - Define correct behavior
2. **Documentation** - Show how to implement patterns
3. **Quality Gates** - Verify compliance before deployment
4. **Regression Prevention** - Catch breakage immediately

By running the same tests against all backends, we ensure:
- **Consistency** - All backends behave identically
- **Completeness** - Nothing is missed
- **Confidence** - Changes don't break existing functionality

## ⚡ Parallel Test Execution

**NEW**: Run acceptance tests in parallel with a comprehensive matrix report showing Pattern × Backend compliance:

```bash
# Run all acceptance tests in parallel (fastest!)
make test-acceptance-parallel

# Generate and save reports (Markdown + JSON)
make test-acceptance-parallel-report

# Test specific backends
make test-acceptance-parallel-backends BACKENDS=MemStore,Redis

# Test specific patterns
make test-acceptance-parallel-patterns PATTERNS=KeyValueBasic,KeyValueTTL

# Direct usage with uv
uv run tooling/parallel_acceptance_test.py
uv run tooling/parallel_acceptance_test.py --format markdown --output report.md
uv run tooling/parallel_acceptance_test.py --fail-fast
```

**Example Matrix Report:**

```
🎯 Pattern × Backend Compliance Matrix:

  Pattern          │  MemStore   │   Redis     │   NATS      │ Score
  ─────────────────┼─────────────┼─────────────┼─────────────┼───────
  KeyValue         │  ✅ PASS    │  ✅ PASS    │  ───────    │ 100.0%
  KeyValueTTL      │  ✅ PASS    │  ✅ PASS    │  ───────    │ 100.0%
  KeyValueScan     │  ───────    │  ✅ PASS    │  ───────    │ 100.0%
  PubSubBasic      │  ───────    │  ───────    │  ✅ PASS    │ 100.0%
  ─────────────────┼─────────────┼─────────────┼─────────────┼───────
  Score            │  100.0%     │  100.0%     │  100.0%     │ 100.0%
```

See [example report](reports/example-acceptance-matrix.md) for full matrix with capabilities and performance metrics.

**Key Features:**
- ✅ **40-60% faster**: Tests run in parallel across patterns
- ✅ **Matrix report**: Visual Pattern × Backend compliance grid
- ✅ **Multiple formats**: Terminal (colored), Markdown, JSON
- ✅ **Smart filtering**: Test specific backends or patterns
- ✅ **Fail-fast mode**: Stop on first failure for quick feedback
- ✅ **Detailed failures**: Shows exactly which tests failed with context

## 📊 Current Status

### Implemented Patterns

- ✅ KeyValue Basic
- ✅ KeyValue TTL
- ✅ KeyValue Concurrency
- ⏳ KeyValue Scan (TODO)
- ⏳ KeyValue Atomic (TODO)
- ⏳ PubSub Basic (TODO)
- ⏳ PubSub Ordering (TODO)
- ⏳ Queue Basic (TODO)

### Implemented Backends

- ✅ MemStore (KeyValue)
- ✅ Redis (KeyValue)
- ✅ NATS (PubSub)
- ⏳ PostgreSQL (KeyValue) - TODO
- ⏳ Kafka (PubSub/Queue) - TODO

## 🤝 Contributing

### Adding Tests

1. Create test file in appropriate pattern directory
2. Follow naming convention: `{category}_test.go`
3. Use framework runner: `framework.RunPatternTests(...)`
4. Update pattern README with new coverage
5. Ensure all existing backends pass (or update capabilities)

### Adding Patterns

1. Define interface in `patterns/core/interfaces.go`
2. Add pattern constant in `framework/types.go`
3. Create pattern directory in `patterns/`
4. Write comprehensive test suite
5. Document requirements in pattern README
6. Update this README

## 🚀 Next Steps

1. ✅ Framework foundation complete
2. ✅ KeyValue Basic tests migrated
3. ✅ Backend registration system working
4. ✅ Build parallel test runner with compliance report
5. ⏳ Create PubSub pattern tests
6. ⏳ Add performance benchmarking
7. ⏳ Implement remaining KeyValue interfaces (Scan, Atomic)
8. ⏳ Integrate matrix report into CI/CD (GitHub Actions comment)

---

**Happy testing!** 🧪

For questions or issues, see [FRAMEWORK_DESIGN.md](FRAMEWORK_DESIGN.md) for comprehensive architecture details or open an issue on GitHub.
