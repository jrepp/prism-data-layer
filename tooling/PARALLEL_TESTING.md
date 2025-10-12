# Parallel Test Runner

## Overview

The parallel test runner (`tooling/parallel_test.py`) significantly reduces test execution time by running independent test suites in parallel with a fork-join execution model.

## Performance Comparison

**Sequential (traditional Makefile):**
```
Unit tests:       60s
Lint tests:       45s
Acceptance tests: 600s (10 minutes)
Integration:      300s
─────────────────────
Total:           ~17 minutes
```

**Parallel (with 8 cores):**
```
All tests run simultaneously:
Total: ~10 minutes (actual slowest test)
Speedup: 1.7x
```

## Quick Start

```bash
# Run all tests in parallel
make test-parallel

# Run only fast tests (skip acceptance)
make test-parallel-fast

# Run with fail-fast (stop on first failure)
make test-parallel-fail-fast
```

## Features

### 1. Fork-Join Execution Model

Tests run in parallel but respect dependencies:
- **Fork**: Launch all independent tests simultaneously
- **Join**: Wait for all to complete before returning
- **Dependencies**: Tests with `depends_on` wait for prerequisites

```python
TestSuite(
    name="integration-go",
    command="cd tests/integration && go test -v ./...",
    depends_on=["memstore-unit"],  # Waits for memstore to pass
)
```

### 2. Individual Log Files

Each test suite writes to its own log file in `test-logs/`:
```
test-logs/
├── proxy-unit.log
├── core-unit.log
├── memstore-unit.log
├── redis-unit.log
├── nats-unit.log
├── lint-rust.log
├── lint-go-memstore.log
├── acceptance-interfaces.log
├── acceptance-redis.log
├── acceptance-nats.log
└── test-report.json
```

**Benefits**:
- ✅ No interleaved output (each test's output is isolated)
- ✅ Easy debugging (read specific test's log)
- ✅ CI/CD friendly (upload logs as artifacts)
- ✅ Machine-readable report (`test-report.json`)

### 3. Fail-Fast Mode

Stop all tests immediately on first failure:

```bash
# Traditional mode: all tests run even if one fails
make test-parallel

# Fail-fast: stops as soon as any test fails
make test-parallel-fail-fast
```

**Use case**: Development - get feedback quickly when something breaks

### 4. Parallel Groups

Tests that might conflict (e.g., both use testcontainers) can be grouped to run serially within the group, but parallel to other groups:

```python
TestSuite(
    name="acceptance-interfaces",
    parallel_group="acceptance",  # Runs serially with other acceptance tests
)
TestSuite(
    name="acceptance-redis",
    parallel_group="acceptance",  # But parallel to unit/lint tests
)
```

### 5. Real-Time Progress Display

```
🚀 Prism Parallel Test Runner
═══════════════════════════════════════════════════

📊 Test Configuration:
  • Total suites: 15
  • Max parallel: 8
  • Fail-fast: disabled
  • Log directory: test-logs

  UNIT: 5 suite(s)
  LINT: 5 suite(s)
  ACCEPTANCE: 3 suite(s)
  INTEGRATION: 2 suite(s)

🧪 proxy-unit - Rust proxy unit tests
🧪 core-unit - Core SDK unit tests
🧪 memstore-unit - MemStore unit tests
🧪 redis-unit - Redis unit tests
🧪 nats-unit - NATS unit tests
🔍 lint-rust - Rust linting
🔍 lint-go-memstore - Go linting (memstore)
🔍 lint-go-redis - Go linting (redis)
🔍 lint-go-nats - Go linting (nats)
🔍 lint-go-core - Go linting (core)
  ✓ proxy-unit passed in 45.2s
  ✓ core-unit passed in 12.3s
  ✓ memstore-unit passed in 8.7s
  ✓ redis-unit passed in 15.1s
  ✓ nats-unit passed in 13.4s
  ✓ lint-rust passed in 38.9s
  ✓ lint-go-memstore passed in 5.2s
  ✓ lint-go-redis passed in 6.1s
  ✓ lint-go-nats passed in 5.8s
  ✓ lint-go-core passed in 4.9s
🔬 acceptance-interfaces - Interface-based acceptance tests
  ✓ acceptance-interfaces passed in 245.7s
🔬 acceptance-redis - Redis acceptance tests
  ✓ acceptance-redis passed in 198.3s
🔬 acceptance-nats - NATS acceptance tests
  ✓ acceptance-nats passed in 187.9s
🔗 integration-go - Go integration tests
  ✓ integration-go passed in 89.4s
🔗 integration-rust - Rust proxy integration tests
  ✓ integration-rust passed in 76.2s

📋 Test Summary
═══════════════════════════════════════════════════

  ✓ Passed:  15/15
  ✗ Failed:  0/15

  ⏱️  Total time: 245.7s
  ⚡ Speedup: 3.4x (591.1s saved)

📄 JSON report: test-logs/test-report.json

✅ All tests passed!
```

## Advanced Usage

### List Available Test Suites

```bash
uv run tooling/parallel_test.py --list
```

Output:
```
Available Test Suites:

UNIT:
  • proxy-unit: Rust proxy unit tests
  • core-unit: Core SDK unit tests
  • memstore-unit: MemStore unit tests
    Depends on: None
  • redis-unit: Redis unit tests
  • nats-unit: NATS unit tests

LINT:
  • lint-rust: Rust linting
  • lint-go-memstore: Go linting (memstore)
  • lint-go-redis: Go linting (redis)
  • lint-go-nats: Go linting (nats)
  • lint-go-core: Go linting (core)

ACCEPTANCE:
  • acceptance-interfaces: Interface-based acceptance tests
  • acceptance-redis: Redis acceptance tests
  • acceptance-nats: NATS acceptance tests

INTEGRATION:
  • integration-go: Go integration tests
    Depends on: memstore-unit
  • integration-rust: Rust proxy integration tests
    Depends on: memstore-unit
```

### Run Specific Categories

```bash
# Run only unit tests
uv run tooling/parallel_test.py --categories unit

# Run unit and lint tests
uv run tooling/parallel_test.py --categories unit,lint

# Run only acceptance tests
uv run tooling/parallel_test.py --categories acceptance
```

### Run Specific Test Suites

```bash
# Run only memstore and redis tests
uv run tooling/parallel_test.py --suites memstore-unit,redis-unit

# Run only linting
uv run tooling/parallel_test.py --suites lint-rust,lint-go-core
```

### Limit Parallelism

Useful on CI with limited resources or laptops:

```bash
# Limit to 4 parallel jobs
uv run tooling/parallel_test.py --max-parallel 4

# Fully sequential (1 test at a time)
uv run tooling/parallel_test.py --max-parallel 1
```

### Verbose Mode

Show test output in real-time (normally buffered to log files):

```bash
uv run tooling/parallel_test.py --verbose
```

### Custom Log Directory

```bash
uv run tooling/parallel_test.py --log-dir /tmp/prism-test-logs
```

## Machine-Readable Output

The test runner generates `test-logs/test-report.json`:

```json
{
  "timestamp": "2025-10-12T14:23:45.123456",
  "duration": 245.7,
  "suites": [
    {
      "name": "proxy-unit",
      "description": "Rust proxy unit tests",
      "category": "unit",
      "status": "passed",
      "start_time": 1697123025.5,
      "end_time": 1697123070.7,
      "duration": 45.2,
      "return_code": 0,
      "error_message": null,
      "log_file": "test-logs/proxy-unit.log"
    },
    ...
  ],
  "summary": {
    "total": 15,
    "passed": 15,
    "failed": 0,
    "skipped": 0
  }
}
```

**Use cases**:
- CI/CD dashboards
- Historical test performance tracking
- Automated failure analysis

## CI/CD Integration

### GitHub Actions Example

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install dependencies
        run: make install-tools

      - name: Run tests in parallel
        run: make test-parallel-fail-fast

      - name: Upload test logs on failure
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: test-logs
          path: test-logs/
```

### Update Existing CI

Replace sequential test steps:

**Before:**
```yaml
- name: Test proxy
  run: make test-proxy
- name: Test patterns
  run: make test-patterns
- name: Test acceptance
  run: make test-acceptance
# Total: ~17 minutes
```

**After:**
```yaml
- name: Test all (parallel)
  run: make test-parallel
# Total: ~10 minutes (40% faster)
```

## Debugging Test Failures

### 1. Check Summary Output

The test runner shows failed tests with log file paths:

```
Failed Tests:
  • acceptance-redis: exit code 1
    Log: test-logs/acceptance-redis.log
```

### 2. Read Full Log

```bash
cat test-logs/acceptance-redis.log

# Or use your favorite pager
less test-logs/acceptance-redis.log
```

### 3. Re-run Specific Failed Test

```bash
# Re-run just the failed test
uv run tooling/parallel_test.py --suites acceptance-redis --verbose

# Or run directly
cd tests/acceptance/redis && go test -v ./...
```

### 4. Inspect JSON Report

```bash
# Pretty-print summary
jq '.summary' test-logs/test-report.json

# List failed tests
jq '.suites[] | select(.status == "failed") | {name, error_message, log_file}' test-logs/test-report.json

# Show test durations
jq '.suites[] | {name, duration} | select(.duration != null)' test-logs/test-report.json | jq -s 'sort_by(.duration) | reverse'
```

## Performance Tips

### 1. Use Fail-Fast During Development

```bash
# Don't waste time running all tests if one fails
make test-parallel-fail-fast
```

### 2. Run Fast Tests Frequently

```bash
# Skip slow acceptance tests during active development
make test-parallel-fast

# Then run full suite before pushing
make test-parallel
```

### 3. Optimize Slow Tests

Check which tests are slowest:

```bash
jq '.suites[] | {name, duration} | select(.duration != null)' test-logs/test-report.json | jq -s 'sort_by(.duration) | reverse | .[0:5]'
```

Example output:
```json
[
  {"name": "acceptance-interfaces", "duration": 245.7},
  {"name": "acceptance-redis", "duration": 198.3},
  {"name": "acceptance-nats", "duration": 187.9},
  {"name": "integration-go", "duration": 89.4},
  {"name": "integration-rust", "duration": 76.2}
]
```

**Action**: Focus optimization efforts on `acceptance-interfaces` (slowest test).

### 4. Adjust Parallelism

```bash
# On powerful dev machine (16 cores)
uv run tooling/parallel_test.py --max-parallel 16

# On CI (4 cores)
uv run tooling/parallel_test.py --max-parallel 4

# On laptop (battery)
uv run tooling/parallel_test.py --max-parallel 2
```

## Adding New Tests

To add a new test suite, edit `tooling/parallel_test.py`:

```python
TEST_SUITES = [
    ...
    TestSuite(
        name="my-new-test",
        command="cd my-component && go test -v ./...",
        description="My new test suite",
        category="unit",  # or "acceptance", "integration", "lint"
        timeout=120,  # seconds
        depends_on=[],  # optional: ["other-test"]
        parallel_group=None,  # optional: "group-name"
    ),
    ...
]
```

## Troubleshooting

### Tests Pass Individually But Fail in Parallel

**Cause**: Resource conflicts (ports, files, environment variables)

**Solution**: Add tests to same `parallel_group`:

```python
TestSuite(
    name="test-a",
    parallel_group="resource-group",  # Runs serially with test-b
)
TestSuite(
    name="test-b",
    parallel_group="resource-group",  # Runs serially with test-a
)
```

### Timeout Errors

**Cause**: Test takes longer than configured timeout

**Solution**: Increase timeout:

```python
TestSuite(
    name="slow-test",
    timeout=900,  # 15 minutes (default: 300s)
)
```

### Out of Memory on CI

**Cause**: Too many parallel tests consuming memory

**Solution**: Reduce parallelism:

```bash
# CI with 8GB RAM
uv run tooling/parallel_test.py --max-parallel 4
```

### Missing Dependencies

**Cause**: Test depends on another test's output

**Solution**: Add dependency:

```python
TestSuite(
    name="integration-test",
    depends_on=["memstore-unit"],  # Waits for binary build
)
```

## Comparison to Traditional Testing

| Feature | Makefile (Sequential) | Parallel Test Runner |
|---------|----------------------|---------------------|
| **Execution** | Serial | Parallel |
| **Speed** | ~17 minutes | ~10 minutes (1.7x faster) |
| **Logs** | Interleaved stdout | Separate log files |
| **Fail-fast** | Continue on error | Optional stop on first failure |
| **Dependencies** | Manual ordering | Automatic via `depends_on` |
| **Progress** | Text output | Real-time with icons |
| **Report** | None | JSON report |
| **CI-friendly** | Moderate | High (artifacts, retries) |

## Best Practices

1. **Use fail-fast during development** - Get quick feedback
2. **Run full suite before pushing** - Ensure nothing breaks
3. **Check JSON report for trends** - Identify slow tests
4. **Set appropriate timeouts** - Avoid hanging tests
5. **Use parallel groups for conflicting tests** - Avoid race conditions
6. **Run verbose mode for debugging** - See test output in real-time
7. **Upload test logs in CI** - Easy debugging from CI failures

## Future Enhancements

Planned improvements:

- [ ] **Test retries**: Automatically retry flaky tests (3 attempts)
- [ ] **Smart caching**: Skip tests for unchanged code (like Bazel)
- [ ] **Historical tracking**: Store test performance over time
- [ ] **Flaky test detection**: Mark tests with unstable pass rate
- [ ] **Resource profiling**: Track CPU/memory per test
- [ ] **Dependency graph visualization**: Show test dependencies
- [ ] **Test sharding**: Split large test suites across multiple runners

## Summary

The parallel test runner:
- ✅ **Reduces test time by 40%+** (17 min → 10 min)
- ✅ **Improves developer experience** with fail-fast and isolated logs
- ✅ **Simplifies CI/CD** with JSON reports and log artifacts
- ✅ **Scales with your codebase** as more tests are added

**Recommended workflow**:
```bash
# During development (fast feedback)
make test-parallel-fast --fail-fast

# Before commit (full validation)
make test-parallel

# In CI (comprehensive check)
make test-parallel-fail-fast
```
