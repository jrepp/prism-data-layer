# KeyValue Pattern Acceptance Tests

This directory contains acceptance tests for the KeyValue pattern interfaces.

## Interfaces Tested

### KeyValueBasicInterface

The core KeyValue interface that all backends should implement:

```go
type KeyValueBasicInterface interface {
    Set(key string, value []byte, ttlSeconds int64) error
    Get(key string) ([]byte, bool, error)
    Delete(key string) error
    Exists(key string) (bool, error)
}
```

**Test Coverage:**
- ✅ Set and Get operations
- ✅ Binary-safe values
- ✅ Empty values
- ✅ Large values (up to backend limits)
- ✅ Non-existent key handling
- ✅ Delete operations (existing and non-existent)
- ✅ Exists check
- ✅ Overwrite behavior
- ✅ Concurrent operations

See: `basic_test.go`

### KeyValueTTLInterface (Optional)

Time-to-live support for key expiration:

```go
type KeyValueTTLInterface interface {
    Set(key string, value []byte, ttlSeconds int64) error
    // ttlSeconds: 0 = no expiration, >0 = expire after N seconds
}
```

**Requirements:**
- TTL=0 means no expiration
- TTL>0 means key expires after N seconds
- Overwriting a key resets its TTL
- Expired keys should not be returned by Get/Exists

**Test Coverage:**
- ✅ Set with TTL
- ✅ Expiration after TTL elapses
- ✅ TTL=0 means no expiration
- ✅ Overwrite resets TTL

See: `ttl_test.go`

**Note:** Backends that don't support TTL should set `SupportsTTL: false` in their capabilities. TTL tests will be skipped automatically.

### KeyValueScanInterface (Optional)

Iteration and scanning support:

```go
type KeyValueScanInterface interface {
    Scan(prefix string, limit int) ([]string, error)
    ScanWithValues(prefix string, limit int) (map[string][]byte, error)
}
```

**Test Coverage:**
- ✅ Prefix scanning
- ✅ Limit/pagination
- ✅ Empty result sets
- ✅ Large result sets

See: `scan_test.go` (TODO)

### KeyValueAtomicInterface (Optional)

Atomic operations for concurrent access:

```go
type KeyValueAtomicInterface interface {
    CompareAndSwap(key string, oldValue, newValue []byte) (bool, error)
    Increment(key string, delta int64) (int64, error)
    Decrement(key string, delta int64) (int64, error)
}
```

**Test Coverage:**
- ✅ CAS operations
- ✅ Concurrent CAS (linearizability)
- ✅ Increment/decrement
- ✅ Overflow handling

See: `atomic_test.go` (TODO)

## Running Tests

### All Backends, All Tests

```bash
go test ./tests/acceptance/patterns/keyvalue/...
```

### Specific Backend Only

```bash
go test ./tests/acceptance/patterns/keyvalue/... -run TestKeyValueBasicPattern/Redis
go test ./tests/acceptance/patterns/keyvalue/... -run TestKeyValueBasicPattern/MemStore
```

### Specific Test Only

```bash
go test ./tests/acceptance/patterns/keyvalue/... -run TestKeyValueBasicPattern/Redis/SetAndGet
```

### With Race Detector

```bash
go test -race ./tests/acceptance/patterns/keyvalue/...
```

### Generate Compliance Report

```bash
go run ./tests/acceptance/cmd/acceptance/ report
```

## Implementing a KeyValue Backend

### 1. Implement the Interface

```go
// patterns/mybackend/mybackend.go
package mybackend

import "github.com/jrepp/prism-data-layer/patterns/core"

type MyBackend struct {
    // ... your implementation
}

func (m *MyBackend) Set(key string, value []byte, ttlSeconds int64) error {
    // Your implementation
}

func (m *MyBackend) Get(key string) ([]byte, bool, error) {
    // Your implementation
    // Return (nil, false, nil) for non-existent keys
}

func (m *MyBackend) Delete(key string) error {
    // Your implementation
    // Should be idempotent (no error if key doesn't exist)
}

func (m *MyBackend) Exists(key string) (bool, error) {
    // Your implementation
}

// Ensure compile-time interface compliance
var _ core.KeyValueBasicInterface = (*MyBackend)(nil)
```

### 2. Register with Test Framework

```go
// tests/acceptance/backends/mybackend.go
package backends

import (
    "github.com/jrepp/prism-data-layer/tests/acceptance/framework"
    // ... other imports
)

func init() {
    framework.MustRegisterBackend(framework.Backend{
        Name:      "MyBackend",
        SetupFunc: setupMyBackend,

        SupportedPatterns: []framework.Pattern{
            framework.PatternKeyValueBasic,
            framework.PatternKeyValueTTL, // if supported
        },

        Capabilities: framework.Capabilities{
            SupportsTTL:  true, // or false
            SupportsScan: false,
            MaxValueSize: 10 * 1024 * 1024, // 10MB
        },
    })
}

func setupMyBackend(t *testing.T, ctx context.Context) (interface{}, func()) {
    // 1. Start any external services (containers, etc.)
    // 2. Create and initialize your driver
    // 3. Return driver and cleanup function

    driver := mybackend.New()
    // ... initialize driver

    cleanup := func() {
        driver.Stop(ctx)
    }

    return driver, cleanup
}
```

### 3. Run Tests

```bash
# Tests automatically discover and run against your backend
go test ./tests/acceptance/patterns/keyvalue/...
```

Your backend now runs through the entire KeyValue test suite automatically!

## Common Implementation Gotchas

### Error Handling

- **Get on non-existent key**: Return `(nil, false, nil)` - NOT an error
- **Delete on non-existent key**: Return `nil` - delete is idempotent
- **Exists on non-existent key**: Return `(false, nil)` - NOT an error

### Concurrency

- All operations must be thread-safe
- Multiple goroutines may call Set/Get/Delete simultaneously
- Use proper locking or atomic operations
- Run with `-race` flag to detect data races

### TTL Behavior

- TTL=0 means no expiration (permanent key)
- TTL>0 means expire after N seconds
- Expired keys should disappear (Get returns not found)
- Overwriting a key should reset its TTL (if TTL is provided)

### Binary Safety

- Values must be treated as opaque byte slices
- Don't assume UTF-8 encoding
- Don't trim, normalize, or modify values
- Test with `[]byte{0x00, 0xFF, ...}` to verify

### Large Values

- Support values up to your declared `MaxValueSize`
- Return clear errors if value exceeds limit
- Don't silently truncate

## Test Structure

Each test file focuses on a specific aspect:

- `basic_test.go` - CRUD operations, core functionality
- `ttl_test.go` - Expiration and TTL behavior
- `concurrent_test.go` - Thread safety and race conditions
- `edge_cases_test.go` - Special characters, large values, etc. (TODO)
- `benchmark_test.go` - Performance benchmarks (TODO)

Tests use the framework runner which:
1. Discovers all backends that support KeyValue pattern
2. Runs each test against each backend
3. Skips tests that require unsupported capabilities
4. Generates detailed compliance reports

## Debugging Failed Tests

### See Detailed Error Messages

```bash
go test -v ./tests/acceptance/patterns/keyvalue/...
```

### Run Single Test

```bash
go test ./tests/acceptance/patterns/keyvalue/... \
  -run TestKeyValueBasicPattern/YourBackend/SpecificTest -v
```

### Add Debug Logging

```go
func setupMyBackend(t *testing.T, ctx context.Context) (interface{}, func()) {
    t.Logf("Setting up MyBackend...")

    driver := mybackend.New()
    t.Logf("Driver created: %+v", driver)

    // ... rest of setup
}
```

### Check Health Status

If setup fails, check your driver's Health() method:

```go
health, err := driver.Health(ctx)
t.Logf("Health: %+v, Error: %v", health, err)
```

## Performance Expectations

Backends are benchmarked on standard operations:

- **Set**: Target <1ms P95
- **Get**: Target <1ms P95
- **Delete**: Target <1ms P95
- **Exists**: Target <1ms P95

These are guidelines, not requirements. Real-world performance depends on:
- Backend technology (in-memory vs. disk vs. network)
- Data size
- Concurrency level
- Hardware

See `benchmark_test.go` for actual benchmark code.

## Contributing

To add new tests to this suite:

1. **Add test case to existing file** (if it fits thematically)
2. **Create new test file** (if it's a new category like `scan_test.go`)
3. **Update this README** with new test coverage
4. **Ensure all existing backends pass** (or update backend capabilities)

### Test Naming Convention

- File: `{category}_test.go` (e.g., `basic_test.go`, `ttl_test.go`)
- Test function: `TestKeyValue{Category}Pattern` (e.g., `TestKeyValueBasicPattern`)
- Test case: `test{Operation}` (e.g., `testSetAndGet`, `testTTLExpiration`)

### Test Requirements

- Use `testing.T.Name()` in key names for isolation
- Clean up after yourself (tests should not interfere with each other)
- Test both success and error paths
- Document expected behavior in comments
- Keep tests fast (<100ms per test if possible)

---

**Questions?** See the main acceptance test README at `tests/acceptance/README.md`
