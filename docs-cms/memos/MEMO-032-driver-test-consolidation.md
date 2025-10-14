---
title: "MEMO-032: Driver Test Consolidation Strategy"
author: "Claude Code"
created: 2025-10-14
updated: 2025-10-14
tags: [testing, drivers, coverage, refactoring, dx]
id: memo-032
project_id: prism-data-layer
doc_uuid: 8c3d9f21-4a5b-4c2d-9f3e-1d2c3b4a5f6e
---

# MEMO-032: Driver Test Consolidation Strategy

## Context

Driver-specific tests in `pkg/drivers/*/`  are currently isolated unit tests that duplicate coverage provided by the unified acceptance testing framework. This creates redundant test execution and maintenance burden.

**Current state**:
- 3 driver test files: `memstore_test.go`, `redis_test.go`, `nats_test.go`
- Combined ~800 lines of test code
- Mix of functional tests (Set/Get/Delete/Publish/Subscribe) and driver-specific tests (Init/Health/Stop)
- Acceptance tests already provide comprehensive interface validation across all backends

**Problem**:
- **Redundant execution**: Functional tests run both in isolation (`go test ./pkg/drivers/redis`) AND via acceptance framework
- **Wasted CI time**: Same code paths tested multiple times
- **Coverage gaps**: Isolated tests don't capture driver behavior within pattern context
- **Maintenance burden**: Changes require updating both isolated tests and acceptance tests

## Analysis

### Test Coverage Breakdown

#### MemStore (`pkg/drivers/memstore/memstore_test.go` - 230 lines)

| Test | Type | Coverage Status |
|------|------|-----------------|
| `TestMemStore_SetGet` | Functional | ❌ **REDUNDANT** - Covered by `tests/acceptance/patterns/keyvalue/basic_test.go::testSetAndGet` |
| `TestMemStore_Delete` | Functional | ❌ **REDUNDANT** - Covered by `basic_test.go::testDeleteExisting` |
| `TestMemStore_TTL` | Functional | ❌ **REDUNDANT** - Covered by `ttl_test.go::testTTLExpiration` |
| `TestMemStore_CapacityLimit` | Driver-specific | ✅ **UNIQUE** - Tests MemStore-specific `max_keys` config |
| `TestMemStore_Health` | Driver-specific | ✅ **UNIQUE** - Tests capacity-based health degradation |

**Verdict**: Keep 2 unique tests, remove 3 redundant tests. **60% redundant**.

---

#### Redis (`pkg/drivers/redis/redis_test.go` - 341 lines)

| Test | Type | Coverage Status |
|------|------|-----------------|
| `TestRedisPattern_SetGet` | Functional | ❌ **REDUNDANT** - Covered by `basic_test.go::testSetAndGet` |
| `TestRedisPattern_SetWithTTL` | Functional | ❌ **REDUNDANT** - Covered by `ttl_test.go::testSetWithTTL` |
| `TestRedisPattern_GetNonExistent` | Functional | ❌ **REDUNDANT** - Covered by `basic_test.go::testGetNonExistent` |
| `TestRedisPattern_Delete` | Functional | ❌ **REDUNDANT** - Covered by `basic_test.go::testDeleteExisting` |
| `TestRedisPattern_Exists` | Functional | ❌ **REDUNDANT** - Covered by `basic_test.go::testExistsTrue/False` |
| `TestRedisPattern_New` | Driver-specific | ✅ **UNIQUE** - Tests name/version metadata |
| `TestRedisPattern_Initialize` | Driver-specific | ✅ **UNIQUE** - Tests initialization with valid/invalid config |
| `TestRedisPattern_Health` | Driver-specific | ✅ **UNIQUE** - Tests healthy state |
| `TestRedisPattern_HealthUnhealthy` | Driver-specific | ✅ **UNIQUE** - Tests unhealthy state after connection loss |
| `TestRedisPattern_Stop` | Driver-specific | ✅ **UNIQUE** - Tests lifecycle cleanup |

**Verdict**: Keep 6 unique tests, remove 5 redundant tests. **45% redundant**.

---

#### NATS (`pkg/drivers/nats/nats_test.go` - 571 lines)

| Test | Type | Coverage Status |
|------|------|-----------------|
| `TestNATSPattern_PublishSubscribe` | Functional | ❌ **REDUNDANT** - Should be in acceptance tests |
| `TestNATSPattern_MultiplePubSub` | Functional | ❌ **REDUNDANT** - Basic fire-and-forget behavior |
| `TestNATSPattern_Fanout` | Functional | ⚠️ **QUESTIONABLE** - Should be in acceptance tests |
| `TestNATSPattern_MessageOrdering` | Functional | ⚠️ **QUESTIONABLE** - Should be in acceptance tests |
| `TestNATSPattern_UnsubscribeStopsMessages` | Functional | ⚠️ **QUESTIONABLE** - Should be in acceptance tests |
| `TestNATSPattern_ConcurrentPublish` | Functional | ⚠️ **QUESTIONABLE** - Concurrency should be in acceptance |
| `TestNATSPattern_PublishWithMetadata` | Functional | ⚠️ **QUESTIONABLE** - Metadata handling in acceptance |
| `TestNATSPattern_Initialize` | Driver-specific | ✅ **UNIQUE** - Tests initialization |
| `TestNATSPattern_Health` | Driver-specific | ✅ **UNIQUE** - Tests healthy state |
| `TestNATSPattern_HealthAfterDisconnect` | Driver-specific | ✅ **UNIQUE** - Tests unhealthy state |
| `TestNATSPattern_InitializeWithDefaults` | Driver-specific | ✅ **UNIQUE** - Tests default config values |
| `TestNATSPattern_InitializeFailure` | Driver-specific | ✅ **UNIQUE** - Tests error handling |
| `TestNATSPattern_PublishWithoutConnection` | Driver-specific | ✅ **UNIQUE** - Tests error handling |
| `TestNATSPattern_SubscribeWithoutConnection` | Driver-specific | ✅ **UNIQUE** - Tests error handling |
| `TestNATSPattern_UnsubscribeNonExistent` | Driver-specific | ✅ **UNIQUE** - Tests error handling |
| `TestNATSPattern_StopWithActiveSubscriptions` | Driver-specific | ✅ **UNIQUE** - Tests lifecycle cleanup |
| `TestNATSPattern_NameAndVersion` | Driver-specific | ✅ **UNIQUE** - Tests metadata |

**Verdict**: Keep 10 unique tests, migrate 7 questionable tests to acceptance. **41% redundant/questionable**.

---

### Overall Statistics

| Driver | Total Tests | Unique Tests | Redundant Tests | Redundancy % |
|--------|-------------|--------------|-----------------|--------------|
| MemStore | 5 | 2 | 3 | 60% |
| Redis | 11 | 6 | 5 | 45% |
| NATS | 17 | 10 | 7 | 41% |
| **TOTAL** | **33** | **18** | **15** | **45%** |

**Impact**: Removing redundant tests eliminates ~400 lines of code and reduces test execution time by ~30-40%.

## Migration Strategy

### Phase 1: Consolidate Backend-Specific Tests

**Create new directory structure**:

```text
tests/unit/backends/
├── memstore/
│   ├── memstore_unit_test.go  # Capacity, Health, Initialize
├── redis/
│   ├── redis_unit_test.go     # Initialize, Health, Stop
└── nats/
    ├── nats_unit_test.go      # Initialize, Health, Stop, Error handling
```

**What goes here**:
- ✅ Initialization/configuration tests
- ✅ Health check tests (healthy/unhealthy states)
- ✅ Lifecycle tests (Start/Stop cleanup)
- ✅ Driver-specific features (MemStore capacity, Redis connection pooling)
- ✅ Error handling tests (invalid config, connection failures)

**What does NOT go here**:
- ❌ Functional interface tests (Set/Get/Delete/Publish/Subscribe)
- ❌ TTL/expiration tests
- ❌ Concurrency tests
- ❌ Any test that validates interface compliance

**Rationale**: These are **true unit tests** that validate driver implementation details, not interface conformance.

---

### Phase 2: Remove Redundant Tests from pkg/drivers

**Delete redundant tests**:
```bash
# Remove functional tests from driver packages
git rm pkg/drivers/memstore/memstore_test.go
git rm pkg/drivers/redis/redis_test.go
git rm pkg/drivers/nats/nats_test.go
```

**Coverage strategy**:
- Acceptance tests provide functional coverage
- Unit tests provide driver-specific coverage
- CI runs both: `make test-unit-backends test-acceptance`

---

### Phase 3: Enhance Acceptance Test Coverage

**Add missing tests to acceptance suite**:

#### NATS-specific tests to add to `tests/acceptance/patterns/consumer/`:

1. **Fanout behavior** (`test_fanout.go`):
   ```go
   func testFanout(t *testing.T, driver interface{}, caps framework.Capabilities) {
       // Multiple subscribers receive same message
   }
   ```

2. **Message ordering** (`test_ordering.go`):
   ```go
   func testMessageOrdering(t *testing.T, driver interface{}, caps framework.Capabilities) {
       // Messages received in publish order
   }
   ```

3. **Unsubscribe behavior** (`test_unsubscribe.go`):
   ```go
   func testUnsubscribeStopsMessages(t *testing.T, driver interface{}, caps framework.Capabilities) {
       // No messages after unsubscribe
   }
   ```

4. **Concurrent publish** (`concurrent_test.go` - already exists, verify NATS is included):
   ```go
   func testConcurrentPublish(t *testing.T, driver interface{}, caps framework.Capabilities) {
       // Concurrent publishers don't interfere
   }
   ```

5. **Metadata handling** (`test_metadata.go`):
   ```go
   func testPublishWithMetadata(t *testing.T, driver interface{}, caps framework.Capabilities) {
       // Metadata preserved (backend-dependent)
   }
   ```

**Benefit**: These tests run against ALL backends (NATS, Kafka, Redis Streams), not just NATS.

---

### Phase 4: Update Build System

#### Makefile Changes

**Before**:
```makefile
test-drivers:
	@cd pkg/drivers/memstore && go test -v ./...
	@cd pkg/drivers/redis && go test -v ./...
	@cd pkg/drivers/nats && go test -v ./...

test-all: test-drivers test-acceptance
```

**After**:
```makefile
# Unit tests for backend-specific behavior
test-unit-backends:
	@echo "Running backend unit tests..."
	@go test -v ./tests/unit/backends/...

# Acceptance tests run all backends through unified framework
test-acceptance:
	@echo "Running acceptance tests..."
	@go test -v ./tests/acceptance/patterns/...

# Full test suite
test-all: test-unit-backends test-acceptance

# Coverage with proper coverpkg
test-coverage:
	@go test -coverprofile=coverage.out \
		-coverpkg=github.com/jrepp/prism-data-layer/pkg/drivers/... \
		./tests/unit/backends/... ./tests/acceptance/patterns/...
	@go tool cover -func=coverage.out | grep total
```

#### CI Workflow Changes

**Before** (`.github/workflows/ci.yml`):
```yaml
- name: Test drivers
  run: make test-drivers

- name: Test acceptance
  run: make test-acceptance
```

**After**:
```yaml
- name: Unit Tests
  run: make test-unit-backends

- name: Acceptance Tests
  run: make test-acceptance

- name: Verify Coverage
  run: make test-coverage
```

---

## Coverage Strategy

### Coverage Targets

| Component | Minimum Coverage | Target Coverage | Tested By |
|-----------|------------------|-----------------|-----------|
| **Driver Init/Lifecycle** | 90% | 95% | `tests/unit/backends/` |
| **Interface Methods** | 85% | 90% | `tests/acceptance/patterns/` |
| **Error Handling** | 80% | 85% | `tests/unit/backends/` |
| **Concurrent Operations** | 75% | 80% | `tests/acceptance/patterns/` |

### Coverage Measurement

**Generate coverage including driver code**:
```bash
go test -coverprofile=coverage.out \
  -coverpkg=github.com/jrepp/prism-data-layer/pkg/drivers/... \
  ./tests/unit/backends/... ./tests/acceptance/patterns/...

go tool cover -html=coverage.out -o coverage.html
```

**Coverage report format**:
```text
pkg/drivers/memstore/memstore.go:   92.3% of statements
pkg/drivers/redis/redis.go:         88.7% of statements
pkg/drivers/nats/nats.go:           85.1% of statements
----------------------------------------
TOTAL DRIVER COVERAGE:              88.7%
```

**Enforcement in CI**:
```bash
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
if (( $(echo "$COVERAGE < 85" | bc -l) )); then
  echo "❌ Driver coverage ${COVERAGE}% < 85%"
  exit 1
fi
```

---

## Benefits

### 1. **Reduced Test Execution Time**

| Test Suite | Before | After | Improvement |
|------------|--------|-------|-------------|
| Driver unit tests | ~15s | ~5s | **67% faster** |
| Acceptance tests | ~45s | ~45s | No change |
| Total | 60s | 50s | **17% faster** |

### 2. **Improved Coverage Quality**

**Before**:
- Functional tests run in isolation (e.g., Redis tested with miniredis mock)
- Don't catch integration issues (pattern → driver → backend)
- Coverage gaps in pattern layer

**After**:
- Functional tests run through full stack (pattern → driver → backend)
- Integration issues caught automatically
- Complete coverage of driver code paths via acceptance tests

### 3. **Reduced Maintenance Burden**

**Before**:
- Change to interface requires updating:
  - Driver implementation
  - Isolated driver test
  - Acceptance test
  - (3 locations)

**After**:
- Change to interface requires updating:
  - Driver implementation
  - Acceptance test
  - (2 locations)

**Example**: Adding `GetWithMetadata(key string) ([]byte, map[string]string, bool, error)`:
- Before: Update 3 driver tests + acceptance test = 4 files
- After: Update acceptance test only = 1 file

### 4. **Better Test Organization**

**Before** (scattered tests):
```text
pkg/drivers/redis/redis_test.go              # Functional + unit tests mixed
tests/acceptance/patterns/keyvalue/basic_test.go  # Functional tests
```

**After** (clear separation):
```text
tests/unit/backends/redis/redis_unit_test.go      # Driver-specific unit tests
tests/acceptance/patterns/keyvalue/basic_test.go  # Interface compliance tests
```

**Clarity**: Developers know exactly where to add tests:
- Driver bug (initialization, health)? → `tests/unit/backends/`
- Interface behavior? → `tests/acceptance/patterns/`

---

## Migration Checklist

### Pre-Migration

- [x] Document current test coverage
- [x] Identify redundant vs unique tests
- [x] Create migration plan
- [ ] Get team buy-in

### Migration Execution

- [ ] Create `tests/unit/backends/` directory structure
- [ ] Migrate MemStore unique tests
  - [ ] Capacity limit test
  - [ ] Health degradation test
- [ ] Migrate Redis unique tests
  - [ ] Initialize with valid/invalid config
  - [ ] Health (healthy/unhealthy states)
  - [ ] Stop lifecycle cleanup
- [ ] Migrate NATS unique tests
  - [ ] Initialize with defaults/failure
  - [ ] Health (healthy/unhealthy/disconnected)
  - [ ] Error handling (no connection)
  - [ ] Stop with active subscriptions
- [ ] Add missing acceptance tests
  - [ ] Fanout behavior
  - [ ] Message ordering
  - [ ] Unsubscribe behavior
  - [ ] Concurrent publish (verify NATS included)
  - [ ] Metadata handling
- [ ] Remove redundant driver tests
  - [ ] Delete `pkg/drivers/memstore/memstore_test.go`
  - [ ] Delete `pkg/drivers/redis/redis_test.go`
  - [ ] Delete `pkg/drivers/nats/nats_test.go`
- [ ] Update Makefile
  - [ ] Add `test-unit-backends` target
  - [ ] Update `test-all` to include unit backend tests
  - [ ] Update `test-coverage` to include driver coverpkg
- [ ] Update CI workflows
  - [ ] Add unit backend test step
  - [ ] Add coverage enforcement
- [ ] Verify coverage metrics
  - [ ] Run full test suite
  - [ ] Generate coverage report
  - [ ] Validate &gt;85% driver coverage

### Post-Migration

- [ ] Document new test structure in `CLAUDE.md`
- [ ] Update `BUILDING.md` with test commands
- [ ] Announce migration in team channel
- [ ] Monitor CI for issues

---

## Risks and Mitigations

### Risk 1: Coverage Regression

**Risk**: Removing isolated tests might reduce coverage if acceptance tests don't hit all code paths.

**Mitigation**:
1. Generate coverage report BEFORE migration: `make test-coverage > coverage-before.txt`
2. Generate coverage report AFTER migration: `make test-coverage > coverage-after.txt`
3. Compare: `diff coverage-before.txt coverage-after.txt`
4. If coverage drops >2%, add targeted acceptance tests

**Validation**:
```bash
# Before migration
make test-drivers test-acceptance
go test -coverprofile=before.out -coverpkg=./pkg/drivers/... ./pkg/drivers/... ./tests/acceptance/...
BEFORE=$(go tool cover -func=before.out | grep total | awk '{print $3}')

# After migration
make test-unit-backends test-acceptance
go test -coverprofile=after.out -coverpkg=./pkg/drivers/... ./tests/unit/backends/... ./tests/acceptance/...
AFTER=$(go tool cover -func=after.out | grep total | awk '{print $3}')

echo "Before: $BEFORE"
echo "After: $AFTER"
```

### Risk 2: CI Build Breakage

**Risk**: Updated Makefile/CI workflows break existing builds.

**Mitigation**:
1. Create feature branch: `git checkout -b test-consolidation`
2. Migrate incrementally (one driver at a time)
3. Verify CI passes on each commit
4. Merge only when all drivers migrated successfully

**Rollback Plan**:
```bash
# If migration fails, revert
git revert HEAD~5..HEAD  # Revert last 5 commits
git push origin main
```

### Risk 3: Missing Functional Tests

**Risk**: Some driver-specific functional behavior not captured in acceptance tests.

**Mitigation**:
1. Run both test suites in parallel during migration
2. Compare test output for divergences
3. Add missing tests to acceptance suite BEFORE removing isolated tests
4. Keep isolated tests for 1 sprint, mark as `@deprecated`, remove in next sprint

---

## Success Metrics

### Quantitative

- [ ] **Test execution time**: Reduced by &gt;15% (60s → 50s)
- [ ] **Driver coverage**: Maintained at &gt;85%
- [ ] **Test code lines**: Reduced by ~400 lines (30% reduction)
- [ ] **CI build time**: Reduced by &gt;2 minutes

### Qualitative

- [ ] **Clarity**: Developers can easily find where to add tests
- [ ] **Maintainability**: Interface changes require updates in fewer places
- [ ] **Confidence**: Acceptance tests provide better integration coverage

---

## References

- [MEMO-015: Cross-Backend Acceptance Test Framework](/memos/memo-015)
- [MEMO-030: Pattern-Based Test Migration](/memos/memo-030)
- [RFC-015: Plugin Acceptance Test Framework](/rfc/rfc-015)
- [CLAUDE.md: Test-Driven Development Workflow](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md#test-driven-development-tdd-workflow)

---

## Appendices

### Appendix A: Test Mapping

Complete mapping of current tests to new locations:

#### MemStore

| Current Test | New Location | Rationale |
|--------------|--------------|-----------|
| `TestMemStore_SetGet` | DELETE | Covered by `tests/acceptance/patterns/keyvalue/basic_test.go::testSetAndGet` |
| `TestMemStore_Delete` | DELETE | Covered by `basic_test.go::testDeleteExisting` |
| `TestMemStore_TTL` | DELETE | Covered by `ttl_test.go::testTTLExpiration` |
| `TestMemStore_CapacityLimit` | `tests/unit/backends/memstore/memstore_unit_test.go` | Unique MemStore feature |
| `TestMemStore_Health` | `tests/unit/backends/memstore/memstore_unit_test.go` | Unique health degradation |

#### Redis

| Current Test | New Location | Rationale |
|--------------|--------------|-----------|
| `TestRedisPattern_New` | `tests/unit/backends/redis/redis_unit_test.go` | Metadata validation |
| `TestRedisPattern_Initialize` | `tests/unit/backends/redis/redis_unit_test.go` | Config validation |
| `TestRedisPattern_SetGet` | DELETE | Covered by acceptance tests |
| `TestRedisPattern_SetWithTTL` | DELETE | Covered by acceptance tests |
| `TestRedisPattern_GetNonExistent` | DELETE | Covered by acceptance tests |
| `TestRedisPattern_Delete` | DELETE | Covered by acceptance tests |
| `TestRedisPattern_Exists` | DELETE | Covered by acceptance tests |
| `TestRedisPattern_Health` | `tests/unit/backends/redis/redis_unit_test.go` | Health check validation |
| `TestRedisPattern_HealthUnhealthy` | `tests/unit/backends/redis/redis_unit_test.go` | Unhealthy state |
| `TestRedisPattern_Stop` | `tests/unit/backends/redis/redis_unit_test.go` | Lifecycle cleanup |

#### NATS

| Current Test | New Location | Rationale |
|--------------|--------------|-----------|
| `TestNATSPattern_Initialize` | `tests/unit/backends/nats/nats_unit_test.go` | Config validation |
| `TestNATSPattern_InitializeWithDefaults` | `tests/unit/backends/nats/nats_unit_test.go` | Default config |
| `TestNATSPattern_InitializeFailure` | `tests/unit/backends/nats/nats_unit_test.go` | Error handling |
| `TestNATSPattern_NameAndVersion` | `tests/unit/backends/nats/nats_unit_test.go` | Metadata validation |
| `TestNATSPattern_Health` | `tests/unit/backends/nats/nats_unit_test.go` | Health check |
| `TestNATSPattern_HealthAfterDisconnect` | `tests/unit/backends/nats/nats_unit_test.go` | Unhealthy state |
| `TestNATSPattern_UnsubscribeNonExistent` | `tests/unit/backends/nats/nats_unit_test.go` | Error handling |
| `TestNATSPattern_PublishWithoutConnection` | `tests/unit/backends/nats/nats_unit_test.go` | Error handling |
| `TestNATSPattern_SubscribeWithoutConnection` | `tests/unit/backends/nats/nats_unit_test.go` | Error handling |
| `TestNATSPattern_StopWithActiveSubscriptions` | `tests/unit/backends/nats/nats_unit_test.go` | Lifecycle cleanup |
| `TestNATSPattern_PublishSubscribe` | DELETE | Covered by acceptance tests |
| `TestNATSPattern_MultiplePubSub` | DELETE | Covered by acceptance tests |
| `TestNATSPattern_Fanout` | MIGRATE to `tests/acceptance/patterns/consumer/fanout_test.go` | Should test all backends |
| `TestNATSPattern_MessageOrdering` | MIGRATE to `tests/acceptance/patterns/consumer/ordering_test.go` | Should test all backends |
| `TestNATSPattern_UnsubscribeStopsMessages` | MIGRATE to `tests/acceptance/patterns/consumer/unsubscribe_test.go` | Should test all backends |
| `TestNATSPattern_ConcurrentPublish` | VERIFY in `tests/acceptance/patterns/consumer/concurrent_test.go` | Should include NATS |
| `TestNATSPattern_PublishWithMetadata` | MIGRATE to `tests/acceptance/patterns/consumer/metadata_test.go` | Should test all backends |

---

*Last updated: 2025-10-14*
