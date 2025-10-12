# Prism Acceptance Test Framework Design

## Vision

Create a world-class acceptance testing framework that makes writing backend drivers a joy. The framework should:

1. **Be pattern-centric** - Tests organized by interface (KeyValue, PubSub, Queue), not by backend
2. **Minimize boilerplate** - Adding a new backend requires ~50 lines of code
3. **Provide comprehensive coverage** - Happy path, edge cases, concurrency, performance
4. **Generate actionable reports** - Compliance matrix, performance comparison, detailed failures
5. **Serve as documentation** - Tests demonstrate how to implement each pattern correctly

## Architecture

### Directory Structure

```
tests/acceptance/
├── README.md                           # Backend developer guide
├── FRAMEWORK_DESIGN.md                 # This file
│
├── framework/                          # Core testing infrastructure
│   ├── registry.go                     # Backend registration system
│   ├── runner.go                       # Test execution engine
│   ├── reporter.go                     # Report generation
│   ├── benchmark.go                    # Performance benchmarking
│   └── assertions.go                   # Custom assertion helpers
│
├── patterns/                           # Pattern test suites
│   ├── keyvalue/                       # KeyValue pattern tests
│   │   ├── README.md                   # KeyValue pattern requirements
│   │   ├── basic_test.go               # CRUD operations
│   │   ├── ttl_test.go                 # TTL/expiration tests
│   │   ├── scan_test.go                # Scan/iteration tests
│   │   ├── atomic_test.go              # Atomic operations (CAS, Inc/Dec)
│   │   ├── concurrent_test.go          # Concurrency tests
│   │   ├── edge_cases_test.go          # Edge cases (large values, special chars)
│   │   ├── benchmark_test.go           # Performance benchmarks
│   │   └── compliance_test.go          # Full compliance suite
│   │
│   ├── pubsub/                         # PubSub pattern tests
│   │   ├── README.md                   # PubSub pattern requirements
│   │   ├── publish_test.go             # Publish operations
│   │   ├── subscribe_test.go           # Subscribe operations
│   │   ├── fanout_test.go              # Fan-out patterns
│   │   ├── ordering_test.go            # Message ordering guarantees
│   │   ├── concurrent_test.go          # Concurrent pub/sub
│   │   ├── edge_cases_test.go          # Large messages, etc.
│   │   ├── benchmark_test.go           # Throughput/latency benchmarks
│   │   └── compliance_test.go          # Full compliance suite
│   │
│   └── queue/                          # Queue pattern tests (future)
│       └── ...
│
├── backends/                           # Backend setup implementations
│   ├── registry.go                     # Backend registry
│   ├── example.go                      # Reference implementation
│   ├── memstore.go                     # MemStore backend
│   ├── redis.go                        # Redis backend
│   ├── nats.go                         # NATS backend
│   ├── postgres.go                     # PostgreSQL backend (TODO)
│   ├── kafka.go                        # Kafka backend (TODO)
│   └── testcontainers.go               # Shared testcontainer utilities
│
├── common/                             # Shared test utilities (existing)
│   ├── harness.go                      # Pattern test harness
│   ├── fixtures.go                     # Test data fixtures
│   └── helpers.go                      # Test helper functions
│
├── cmd/
│   └── acceptance/                     # CLI test runner
│       ├── main.go                     # Entry point
│       ├── run.go                      # Run command
│       ├── report.go                   # Report command
│       └── list.go                     # List command
│
└── reports/                            # Generated reports (gitignored)
    ├── compliance-matrix.md            # Backend x Pattern compliance
    ├── performance-comparison.md       # Performance metrics
    ├── detailed-results.json           # Machine-readable results
    └── coverage-summary.md             # Test coverage per pattern
```

## Core Components

### 1. Backend Registration System

**Goal:** Make it trivial to add a new backend to the test suite.

**Interface:**

```go
// Backend represents a testable backend implementation
type Backend struct {
    Name            string
    SetupFunc       SetupFunc
    TeardownFunc    TeardownFunc
    SupportedPatterns []Pattern
    Capabilities    Capabilities
}

// Pattern represents an interface pattern (KeyValue, PubSub, etc.)
type Pattern string

const (
    PatternKeyValue         Pattern = "KeyValue"
    PatternKeyValueTTL      Pattern = "KeyValueTTL"
    PatternKeyValueScan     Pattern = "KeyValueScan"
    PatternKeyValueAtomic   Pattern = "KeyValueAtomic"
    PatternPubSubBasic      Pattern = "PubSubBasic"
    PatternQueue            Pattern = "Queue"
)

// Capabilities defines optional features a backend may support
type Capabilities struct {
    SupportsTTL         bool
    SupportsScan        bool
    SupportsAtomic      bool
    SupportsTransactions bool
    MaxValueSize        int64 // 0 = unlimited
    MaxKeySize          int   // 0 = unlimited
}

// SetupFunc prepares a backend for testing
type SetupFunc func(t *testing.T, ctx context.Context) (driver interface{}, cleanup func())

// Register a backend with the test framework
func RegisterBackend(backend Backend) {
    registry[backend.Name] = backend
}
```

**Usage Example:**

```go
// backends/memstore.go
func init() {
    RegisterBackend(Backend{
        Name: "MemStore",
        SetupFunc: setupMemStore,
        SupportedPatterns: []Pattern{
            PatternKeyValue,
            PatternKeyValueTTL,
        },
        Capabilities: Capabilities{
            SupportsTTL:  true,
            SupportsScan: false,
            MaxValueSize: 0, // unlimited
        },
    })
}

func setupMemStore(t *testing.T, ctx context.Context) (interface{}, func()) {
    backend := backends.SetupMemStore(t, ctx)
    return backend.Driver, backend.Cleanup
}
```

### 2. Test Runner

**Goal:** Execute pattern tests against all compatible backends automatically.

**Interface:**

```go
// RunPatternTests executes all tests for a pattern against compatible backends
func RunPatternTests(t *testing.T, pattern Pattern, tests []PatternTest) {
    backends := GetBackendsForPattern(pattern)

    for _, backend := range backends {
        t.Run(backend.Name, func(t *testing.T) {
            driver, cleanup := backend.SetupFunc(t, context.Background())
            defer cleanup()

            for _, test := range tests {
                t.Run(test.Name, func(t *testing.T) {
                    // Skip test if backend doesn't support required capability
                    if test.RequiresCapability != "" && !backend.HasCapability(test.RequiresCapability) {
                        t.Skipf("Backend %s doesn't support %s", backend.Name, test.RequiresCapability)
                    }

                    test.Func(t, driver, backend.Capabilities)
                })
            }
        })
    }
}

// PatternTest represents a single test case
type PatternTest struct {
    Name                string
    Func                TestFunc
    RequiresCapability  string // Optional: skip if not supported
}

type TestFunc func(t *testing.T, driver interface{}, caps Capabilities)
```

**Usage Example:**

```go
// patterns/keyvalue/basic_test.go
func TestKeyValuePattern(t *testing.T) {
    tests := []PatternTest{
        {
            Name: "SetAndGet",
            Func: testSetAndGet,
        },
        {
            Name: "DeleteExistingKey",
            Func: testDeleteExisting,
        },
        {
            Name: "TTLExpiration",
            Func: testTTLExpiration,
            RequiresCapability: "TTL",
        },
    }

    RunPatternTests(t, PatternKeyValue, tests)
}

func testSetAndGet(t *testing.T, driver interface{}, caps Capabilities) {
    kv := driver.(core.KeyValueBasicInterface)

    err := kv.Set("test-key", []byte("test-value"), 0)
    require.NoError(t, err)

    value, found, err := kv.Get("test-key")
    require.NoError(t, err)
    assert.True(t, found)
    assert.Equal(t, "test-value", string(value))
}
```

### 3. Compliance Reporter

**Goal:** Generate actionable reports showing which backends pass which tests.

**Output Format:**

```markdown
# Prism Backend Compliance Matrix

Generated: 2025-10-12 21:00:00

## KeyValue Pattern

| Backend   | Basic | TTL | Scan | Atomic | Score |
|-----------|-------|-----|------|--------|-------|
| MemStore  | ✅ 10/10 | ✅ 5/5 | ❌ 0/8 | ❌ N/A | 75% |
| Redis     | ✅ 10/10 | ✅ 5/5 | ✅ 8/8 | ✅ 6/6 | 100% |
| Postgres  | ✅ 10/10 | ❌ 0/5 | ✅ 8/8 | ⚠️  4/6 | 76% |

**Legend:**
- ✅ All tests passing
- ⚠️  Some tests failing
- ❌ Pattern not supported or all tests failing
- N/A - Pattern not applicable

## PubSub Pattern

| Backend   | Basic | Ordering | Fanout | Score |
|-----------|-------|----------|--------|-------|
| NATS      | ✅ 8/8 | ⚠️  6/8  | ✅ 5/5 | 90% |
| Redis     | ✅ 8/8 | ❌ 2/8  | ✅ 5/5 | 71% |
| Kafka     | ✅ 8/8 | ✅ 8/8  | ✅ 5/5 | 100% |

---

## Detailed Failures

### Postgres - KeyValueAtomic

**Test:** TestCompareAndSwap/ConcurrentCAS
**Error:** Race condition detected - CAS operation not atomic
**Recommendation:** Implement using PostgreSQL's SERIALIZABLE isolation level or row-level locking

### NATS - PubSubOrdering

**Test:** TestMessageOrdering/CrossSubscriberOrdering
**Error:** Messages received out of order across different subscribers
**Expected:** [msg1, msg2, msg3]
**Got:** [msg1, msg3, msg2]
**Note:** This is expected behavior for core NATS (at-most-once semantics). Use JetStream for ordering guarantees.
```

### 4. Performance Benchmarking

**Goal:** Compare backend performance using identical workloads.

**Interface:**

```go
// BenchmarkConfig defines a performance test
type BenchmarkConfig struct {
    Name            string
    Pattern         Pattern
    WorkloadFunc    WorkloadFunc
    Duration        time.Duration
    Concurrency     int
}

type WorkloadFunc func(driver interface{}, ctx context.Context) error

// RunBenchmarks executes performance tests and generates comparison report
func RunBenchmarks(benchmarks []BenchmarkConfig) *PerformanceReport {
    report := &PerformanceReport{
        Timestamp: time.Now(),
        Results:   make(map[string]map[string]BenchmarkResult),
    }

    for _, bench := range benchmarks {
        backends := GetBackendsForPattern(bench.Pattern)

        for _, backend := range backends {
            result := executeBenchmark(backend, bench)
            report.Results[bench.Name][backend.Name] = result
        }
    }

    return report
}

type BenchmarkResult struct {
    OpsPerSec       float64
    P50Latency      time.Duration
    P95Latency      time.Duration
    P99Latency      time.Duration
    ErrorRate       float64
}
```

**Output Example:**

```markdown
# Performance Comparison: KeyValue Set Operation

| Backend   | Ops/Sec | P50   | P95   | P99   | Errors |
|-----------|---------|-------|-------|-------|--------|
| MemStore  | 450,000 | 0.02ms| 0.05ms| 0.1ms | 0%     |
| Redis     | 85,000  | 0.15ms| 0.8ms | 1.5ms | 0%     |
| Postgres  | 12,000  | 1.2ms | 3.5ms | 8ms   | 0.01%  |

**Workload:** 1000 concurrent clients, 10 seconds duration, 64-byte values
```

## Pattern Test Requirements

### KeyValue Pattern

**Basic Interface:**
- Set/Get/Delete/Exists
- Binary-safe values
- Empty values
- Large values (up to backend limit)
- Special characters in keys
- Concurrent operations
- Non-existent key handling

**TTL Interface (if supported):**
- Set with TTL
- Expiration verification
- TTL update
- TTL query
- Expired key behavior

**Scan Interface (if supported):**
- Prefix scanning
- Limit/pagination
- Empty result sets
- Large result sets

**Atomic Interface (if supported):**
- Compare-and-swap
- Increment/decrement
- Concurrent CAS (linearizability)

### PubSub Pattern

**Basic Interface:**
- Publish message
- Subscribe to topic
- Unsubscribe
- Multiple subscribers
- Message delivery
- Channel cleanup

**Ordering (if guaranteed):**
- Single publisher ordering
- Multi-publisher ordering
- Cross-subscriber consistency

**Fanout:**
- 1-to-many delivery
- Independent subscriber progress
- Slow subscriber handling

## Implementation Plan

### Phase 1: Framework Foundation (Week 1)
1. Create `framework/registry.go` - Backend registration
2. Create `framework/runner.go` - Test execution engine
3. Create `framework/reporter.go` - Report generation
4. Create `backends/registry.go` - Backend registry
5. Write `tests/acceptance/README.md` - Developer guide

### Phase 2: Backend Migration (Week 1)
1. Refactor `backends/memstore.go` - Use new registration
2. Refactor `backends/redis.go` - Use new registration
3. Refactor `backends/nats.go` - Use new registration
4. Create `backends/example.go` - Reference implementation

### Phase 3: KeyValue Pattern Tests (Week 2)
1. Reorganize `patterns/keyvalue/` directory
2. Migrate existing tests from `interfaces/`
3. Add missing test coverage (atomic, scan)
4. Add concurrency tests
5. Add edge case tests
6. Add benchmarks

### Phase 4: PubSub Pattern Tests (Week 2)
1. Create `patterns/pubsub/` directory
2. Implement basic pub/sub tests
3. Implement ordering tests
4. Implement fanout tests
5. Add concurrency tests
6. Add benchmarks

### Phase 5: Reporting & CI (Week 3)
1. Implement compliance matrix generator
2. Implement performance comparison reporter
3. Update CI pipeline for pattern-based testing
4. Add GitHub Actions comment with compliance matrix
5. Create visualizations (charts, graphs)

## Success Criteria

### For Backend Developers
- Adding a new backend requires ≤ 50 lines of boilerplate code
- Test failures provide clear error messages with reproduction steps
- Compliance matrix clearly shows what's implemented vs. missing
- Performance comparison helps identify optimization opportunities

### For Project Maintainers
- Pattern compliance visible at a glance
- CI reports show exactly which backends broke on which patterns
- Easy to add new pattern interfaces
- Test suite runs in < 5 minutes

### For Users
- Documentation (tests) demonstrates correct backend usage
- Compliance matrix helps choose appropriate backend
- Performance data guides capacity planning

## Examples

### Adding a New Backend (PostgreSQL)

```go
// backends/postgres.go
package backends

import (
    "context"
    "testing"
    "github.com/jrepp/prism-data-layer/patterns/postgres"
    "github.com/jrepp/prism-data-layer/patterns/core"
)

func init() {
    RegisterBackend(Backend{
        Name: "Postgres",
        SetupFunc: setupPostgres,
        SupportedPatterns: []Pattern{
            PatternKeyValue,
            PatternKeyValueScan,
        },
        Capabilities: Capabilities{
            SupportsTTL:         false, // No native TTL
            SupportsScan:        true,  // LIKE queries
            SupportsAtomic:      true,  // SERIALIZABLE transactions
            SupportsTransactions: true,
            MaxValueSize:        1024 * 1024, // 1MB (practical limit)
        },
    })
}

func setupPostgres(t *testing.T, ctx context.Context) (interface{}, func()) {
    // Start testcontainer
    container := testcontainers.RunPostgres(t, ctx)

    // Create driver
    driver := postgres.New()

    // Initialize
    config := &core.Config{
        Plugin: core.PluginConfig{
            Name: "postgres-test",
            Version: "0.1.0",
        },
        Backend: map[string]any{
            "connection_string": container.ConnectionString,
        },
    }

    err := driver.Initialize(ctx, config)
    require.NoError(t, err)

    err = driver.Start(ctx)
    require.NoError(t, err)

    cleanup := func() {
        driver.Stop(ctx)
        container.Terminate(ctx)
    }

    return driver, cleanup
}
```

**That's it!** PostgreSQL now runs through the entire KeyValue test suite automatically.

### Adding a New Pattern Test

```go
// patterns/keyvalue/atomic_test.go
package keyvalue_test

import (
    "testing"
    "github.com/jrepp/prism-data-layer/tests/acceptance/framework"
)

func TestKeyValueAtomic(t *testing.T) {
    tests := []framework.PatternTest{
        {
            Name: "CompareAndSwap",
            Func: testCompareAndSwap,
            RequiresCapability: "Atomic",
        },
        {
            Name: "Increment",
            Func: testIncrement,
            RequiresCapability: "Atomic",
        },
    }

    framework.RunPatternTests(t, framework.PatternKeyValueAtomic, tests)
}

func testCompareAndSwap(t *testing.T, driver interface{}, caps framework.Capabilities) {
    kv := driver.(core.KeyValueAtomicInterface)

    // Initial set
    kv.Set("counter", []byte("10"), 0)

    // Successful CAS
    ok, err := kv.CompareAndSwap("counter", []byte("10"), []byte("20"))
    require.NoError(t, err)
    assert.True(t, ok, "CAS should succeed with correct old value")

    // Failed CAS
    ok, err = kv.CompareAndSwap("counter", []byte("10"), []byte("30"))
    require.NoError(t, err)
    assert.False(t, ok, "CAS should fail with incorrect old value")

    // Verify final value
    value, found, err := kv.Get("counter")
    require.NoError(t, err)
    assert.True(t, found)
    assert.Equal(t, "20", string(value))
}
```

## Next Steps

1. Review and approve this design
2. Begin Phase 1 implementation
3. Iterate based on feedback from first backend migration
4. Expand to cover all patterns
5. Integrate with CI/CD pipeline

---

**Document Status:** Draft for Review
**Last Updated:** 2025-10-12
**Authors:** Claude Code
