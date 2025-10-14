# Unified Acceptance Testing Framework

This document describes the unified acceptance testing framework that dynamically discovers pattern interfaces and runs appropriate test suites.

## Overview

The unified testing framework eliminates the need for backend-specific test files (kafka_integration_test.go, postgres_integration_test.go, etc.). Instead, it:

1. **Discovers interfaces** - Connects to a running pattern executable and queries which interfaces it implements via the `Initialize()` RPC
2. **Selects tests dynamically** - Looks up test suites registered for those interfaces
3. **Runs appropriate tests** - Executes only the tests relevant to the discovered interfaces

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Unified Test Runner                      │
│                                                             │
│  1. Connect to Pattern Executable (gRPC)                   │
│  2. Call Initialize() → Get InterfaceDeclaration list      │
│  3. Lookup test suites for those interfaces                │
│  4. Run tests dynamically                                  │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ gRPC
                            ▼
┌─────────────────────────────────────────────────────────────┐
│              Pattern Executable                             │
│              (kafka, redis, multicast_registry, etc.)       │
│                                                             │
│  Implements:                                                │
│    - LifecycleInterface (required)                          │
│    - KeyValueBasicInterface (optional)                      │
│    - PubSubBasicInterface (optional)                        │
│    - QueueBasicInterface (optional)                         │
│    - etc.                                                   │
└─────────────────────────────────────────────────────────────┘
```

## Key Components

### 1. Pattern Discovery (`framework/pattern_discovery.go`)

```go
// Connect to pattern and discover interfaces
pattern, err := framework.DiscoverPatternInterfaces(ctx, "kafka", "localhost:50051", config)
if err != nil {
    log.Fatal(err)
}
defer pattern.Cleanup()

// Get supported interfaces
interfaces := pattern.GetSupportedInterfaces()
// Returns: ["KeyValueBasicInterface", "PubSubBasicInterface", "QueueBasicInterface"]
```

### 2. Test Suite Registry (`framework/test_suite_registry.go`)

Test suites register themselves at init time:

```go
func init() {
    framework.MustRegisterTestSuite(framework.TestSuite{
        InterfaceName: "KeyValueBasicInterface",
        Pattern:       framework.PatternKeyValueBasic,
        Description:   "Tests basic KeyValue operations",
        Tests: []framework.PatternTest{
            {
                Name: "SetAndGet",
                Func: testKeyValueSetGet,
            },
            {
                Name: "Delete",
                Func: testKeyValueDelete,
            },
        },
    })
}
```

### 3. Unified Test Runner (`framework/unified_runner.go`)

Main entry point for running tests:

```go
func TestMyPattern(t *testing.T) {
    config := framework.PatternTestConfig{
        Name:    "kafka",
        Address: "localhost:50051",
        Config:  map[string]interface{}{
            "brokers": []string{"localhost:9092"},
        },
        Timeout: 30 * time.Second,
    }

    // This will:
    // 1. Discover interfaces
    // 2. Lookup test suites
    // 3. Run tests dynamically
    framework.RunUnifiedPatternTests(t, config)
}
```

## Usage

### Testing a Specific Pattern

1. **Start the pattern executable**:
   ```bash
   # Example: multicast registry
   ./patterns/multicast_registry/cmd/multicast-registry-runner/multicast-registry-runner \
     --port 50051 \
     --config examples/redis-nats.yaml
   ```

2. **Run unified tests**:
   ```bash
   # Test whatever pattern is running on localhost:50051
   go test -v ./tests/acceptance -run TestUnifiedPattern
   ```

   The test will:
   - Connect to localhost:50051
   - Call `Initialize()` to get interface list
   - Discover the pattern implements KeyValueBasicInterface and PubSubBasicInterface
   - Run KeyValue and PubSub test suites automatically

### Testing Multiple Patterns

```bash
# Test Kafka
./patterns/kafka/kafka --port 50051 &
KAFKA_PID=$!
go test -v ./tests/acceptance -run TestUnifiedPattern
kill $KAFKA_PID

# Test Redis
./patterns/redis/redis --port 50052 &
REDIS_PID=$!
PATTERN_ADDR=localhost:50052 go test -v ./tests/acceptance -run TestUnifiedPattern
kill $REDIS_PID
```

### CI/CD Integration

```yaml
# .github/workflows/acceptance-tests.yml
jobs:
  test-patterns:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        pattern:
          - name: kafka
            port: 50051
            config: configs/kafka.yaml
          - name: redis
            port: 50052
            config: configs/redis.yaml
          - name: multicast-registry
            port: 50053
            config: configs/multicast.yaml

    steps:
      - name: Start pattern executable
        run: |
          ./patterns/${{ matrix.pattern.name }}/runner \
            --port ${{ matrix.pattern.port }} \
            --config ${{ matrix.pattern.config }} &
          echo $! > pattern.pid
          sleep 2  # Wait for startup

      - name: Run unified tests
        env:
          PATTERN_ADDR: localhost:${{ matrix.pattern.port }}
          PATTERN_NAME: ${{ matrix.pattern.name }}
        run: |
          go test -v ./tests/acceptance -run TestUnifiedPattern

      - name: Stop pattern
        if: always()
        run: |
          kill $(cat pattern.pid) || true
```

## Comparison: Before vs After

### Before (Backend-Specific Tests)

```
tests/acceptance/
├── kafka/kafka_integration_test.go         # 454 lines, Kafka-specific
├── postgres/postgres_integration_test.go   # 415 lines, Postgres-specific
├── redis/redis_integration_test.go         # 200 lines, Redis-specific
└── nats/nats_integration_test.go           # 300 lines, NATS-specific
```

Problems:
- Duplicated test logic across backends
- Hard to add new backends (must write full test file)
- Tests are tightly coupled to specific backends
- Backend-specific quirks leak into test code

### After (Unified Interface-Based Tests)

```
tests/acceptance/
├── unified_pattern_test.go                 # Single entry point
├── framework/
│   ├── pattern_discovery.go                # Interface discovery
│   ├── test_suite_registry.go              # Test suite management
│   └── unified_runner.go                   # Test execution
└── suites/
    ├── keyvalue_basic_suite.go             # KeyValue tests (all backends)
    ├── keyvalue_ttl_suite.go               # TTL tests (backends with TTL)
    ├── pubsub_basic_suite.go               # PubSub tests (all backends)
    └── queue_basic_suite.go                # Queue tests (all backends)
```

Benefits:
- **No duplication** - Tests written once, run on all compatible backends
- **Easy to add backends** - Just implement interfaces, tests run automatically
- **Dynamic selection** - Only relevant tests run based on discovered interfaces
- **Clear separation** - Test logic separate from backend setup

## Writing New Test Suites

To add tests for a new interface:

1. **Create test suite file** (`tests/acceptance/suites/myinterface_suite.go`):

```go
package suites

import (
    "github.com/jrepp/prism-data-layer/tests/acceptance/framework"
)

func init() {
    framework.MustRegisterTestSuite(framework.TestSuite{
        InterfaceName: "MyCustomInterface",
        Pattern:       framework.PatternCustom,  // Add to framework/types.go
        Description:   "Tests MyCustomInterface operations",
        Tests: []framework.PatternTest{
            {
                Name: "BasicOperation",
                Func: testBasicOperation,
                Timeout: 5 * time.Second,
            },
            {
                Name: "ErrorHandling",
                Func: testErrorHandling,
                RequiresCapability: "ErrorRecovery",
            },
        },
    })
}

func testBasicOperation(t *testing.T, driver interface{}, caps framework.Capabilities) {
    // Cast driver to gRPC connection
    conn, ok := driver.(*grpc.ClientConn)
    require.True(t, ok)

    // Create interface-specific client
    client := pb_custom.NewMyCustomInterfaceClient(conn)

    // Run test
    resp, err := client.DoSomething(context.Background(), &pb_custom.Request{...})
    require.NoError(t, err)
    assert.True(t, resp.Success)
}
```

2. **Pattern executable declares interface** (in your pattern's main.go):

```go
func (p *MyPattern) Initialize(ctx context.Context, req *pb.InitializeRequest) (*pb.InitializeResponse, error) {
    return &pb.InitializeResponse{
        Success: true,
        Metadata: &pb.PatternMetadata{
            Name:    "my-pattern",
            Version: "1.0.0",
            Interfaces: []*pb.InterfaceDeclaration{
                {
                    Name:      "LifecycleInterface",  // Required
                    ProtoFile: "prism/interfaces/lifecycle.proto",
                },
                {
                    Name:      "MyCustomInterface",  // Your interface
                    ProtoFile: "prism/interfaces/custom/my_interface.proto",
                    Version:   "v1",
                },
            },
        },
    }, nil
}
```

3. **Tests run automatically**:
   ```bash
   # Start your pattern
   ./my-pattern --port 50051

   # Tests auto-discover MyCustomInterface and run the suite
   go test -v ./tests/acceptance -run TestUnifiedPattern
   ```

## Benefits

### 1. **Eliminates Duplication**

Same test logic runs on all backends that implement an interface:
- Write KeyValue tests once → runs on Redis, Postgres, Kafka, MemStore, etc.
- Write PubSub tests once → runs on NATS, Kafka, Redis Streams, etc.

### 2. **Easy Backend Addition**

Adding a new backend requires zero test code:
1. Implement the interfaces
2. Declare them in Initialize()
3. Tests run automatically

### 3. **Clear Interface Contracts**

Tests document the expected behavior of each interface:
- What operations must be supported
- How errors should be handled
- Edge cases that must work

### 4. **Flexible Test Selection**

```go
// Run all tests
framework.RunUnifiedPatternTests(t, config)

// Run only KeyValue tests
opts := framework.UnifiedTestOptions{
    FilterInterfaces: []string{"KeyValueBasicInterface"},
}
framework.RunUnifiedPatternTestsWithOptions(t, opts)

// Fail fast on first error
opts.FailFast = true
framework.RunUnifiedPatternTestsWithOptions(t, opts)
```

### 5. **Better CI/CD**

Matrix testing across patterns is trivial:
- Same test code
- Different pattern executables
- Automatic interface discovery

## Migration Path

To migrate existing backend-specific tests:

1. **Extract common test logic** from backend-specific files
2. **Create interface-based test suites** in `tests/acceptance/suites/`
3. **Register suites** with framework registry
4. **Update CI/CD** to use unified test runner
5. **Remove old backend-specific test files** (optional - can keep for now)

Example migration:

```bash
# 1. Extract KeyValue tests from kafka_integration_test.go
#    → tests/acceptance/suites/keyvalue_basic_suite.go

# 2. Register suite
#    framework.MustRegisterTestSuite(...)

# 3. Run with unified test runner
go test -v ./tests/acceptance -run TestUnifiedPattern

# 4. Once validated, remove old test file
rm tests/acceptance/kafka/kafka_integration_test.go
```

## Debugging

### Discover what interfaces a pattern supports:

```bash
go test -v ./tests/acceptance -run TestDiscoverInterfaces
```

Output:
```
Pattern: multicast-registry
Version: 0.1.0
Interfaces: 3

1. LifecycleInterface
   Proto: prism/interfaces/lifecycle.proto

2. KeyValueBasicInterface
   Proto: prism/interfaces/keyvalue/keyvalue_basic.proto
   Version: v1

3. PubSubBasicInterface
   Proto: prism/interfaces/pubsub/pubsub_basic.proto
   Version: v1
```

### List registered test suites:

```go
suites := framework.GetAllTestSuites()
for _, suite := range suites {
    fmt.Printf("Suite: %s (%d tests)\n", suite.InterfaceName, len(suite.Tests))
    for _, test := range suite.Tests {
        fmt.Printf("  - %s\n", test.Name)
    }
}
```

## Future Enhancements

1. **Parallel interface testing** - Run test suites for different interfaces concurrently
2. **Performance benchmarks** - Add benchmark suite registry
3. **Compliance scoring** - Track which backends pass which interface tests
4. **Chaos testing** - Inject failures and test error handling
5. **Load testing** - Stress test each interface independently

## Conclusion

The unified testing framework provides:
- ✅ **Zero duplication** - Tests written once, run everywhere
- ✅ **Dynamic discovery** - No hardcoded backend lists
- ✅ **Clear contracts** - Interfaces define testable behavior
- ✅ **Easy extension** - Adding backends or tests is trivial
- ✅ **Better CI/CD** - Matrix testing with single test codebase
