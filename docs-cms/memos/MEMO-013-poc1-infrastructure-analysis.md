---
title: "MEMO-013: POC 1 Infrastructure Analysis - SDK and Load Testing"
author: Platform Team
created: 2025-10-11
updated: 2025-10-11
tags: [poc1, pattern-sdk, load-testing, infrastructure, developer-experience]
---

# MEMO-013: POC 1 Infrastructure Analysis - SDK and Load Testing

## Executive Summary

Comprehensive analysis of POC 1 infrastructure reveals two critical improvement areas: **Pattern SDK shared complexity** and **load testing strategy**. This memo synthesizes findings from MEMO-014 (SDK analysis) and RFC-029 (load testing evaluation) to provide actionable recommendations for POC 1 implementation.

**Key Recommendations**:
1. **Extract shared complexity** to Pattern SDK (38% code reduction)
2. **Adopt two-tier load testing** (custom tool + ghz)
3. **Prioritize connection pooling, TTL management, and health checks** for SDK
4. **Implement in 2-week sprint** alongside POC 1 plugin development

**Impact**:
- 🎯 **Faster plugin development**: 38% less code per plugin
- 🎯 **Better testing**: Pattern-level + integration coverage
- 🎯 **Higher quality**: 85%+ test coverage in SDK
- 🎯 **Reduced maintenance**: Standardized patterns

## Context

### Trigger

RFC-021 defines three POC 1 plugins (MemStore, Redis, Kafka) with significant code duplication. Additionally, the current load testing tool (`prism-loadtest`) lacks integration-level testing through the Rust proxy.

### Analysis Scope

**MEMO-014**: Pattern SDK Shared Complexity
- Analyzed all three plugin implementations
- Identified 10 areas of duplication
- Proposed SDK enhancements
- Estimated code reduction: 38%

**RFC-029**: Load Testing Framework Evaluation
- Evaluated 5 frameworks (ghz, k6, fortio, vegeta, custom)
- Compared against Prism requirements
- Proposed two-tier testing strategy
- Recommended ghz for integration testing

### Goals

1. **Reduce code duplication** in plugin implementations
2. **Improve developer experience** with reusable SDK packages
3. **Establish comprehensive testing strategy** for POC 1+
4. **Maintain high code quality** (80%+ coverage)

## Findings

### Finding 1: Significant Code Duplication Across Plugins

#### Evidence

| Shared Feature | MemStore | Redis | Kafka | SDK Support |
|---------------|----------|-------|-------|-------------|
| TTL Management | ✅ Custom | ✅ Redis EXPIRE | ❌ N/A | ❌ None |
| Connection Pooling | ❌ N/A | ✅ Custom | ✅ Custom | ❌ None |
| Health Checks | ✅ Custom | ✅ Custom | ✅ Custom | ❌ None |
| Retry Logic | ❌ N/A | ✅ Custom | ✅ Custom | ✅ Basic |
| Config Loading | ✅ Custom | ✅ Custom | ✅ Custom | ❌ None |

**10 of 10 features** have duplication across plugins.

#### Impact

**Current State** (without SDK enhancements):
- MemStore: ~600 LOC
- Redis: ~700 LOC
- Kafka: ~800 LOC
- **Total**: 2,100 LOC

**Future State** (with SDK enhancements):
- MemStore: ~350 LOC (42% reduction)
- Redis: ~450 LOC (36% reduction)
- Kafka: ~500 LOC (38% reduction)
- **Total**: 1,300 LOC (38% reduction)

**Savings**: 800 lines of code across three plugins

#### Root Cause

Pattern SDK was designed as minimal skeleton (RFC-022):
- Auth stubs
- Basic observability
- Lifecycle hooks
- Basic retry logic

**Missing**: Higher-level abstractions for common patterns (pooling, TTL, health)

---

### Finding 2: Two Types of Load Testing Needed

#### Evidence

Current `prism-loadtest` tool:
- ✅ Tests pattern logic directly
- ✅ Custom metrics (multicast delivery stats)
- ✅ Production-ready (validated by MEMO-010)
- ❌ Doesn't test through Rust proxy
- ❌ No gRPC integration testing

**Gap**: Cannot validate end-to-end production path (client → proxy → plugin → backend)

#### Analysis

Prism requires **two distinct testing levels**:

**Pattern-Level Testing** (Unit Load Testing):
```text
prism-loadtest → Coordinator (direct) → Redis/NATS
```
- **Purpose**: Test pattern logic in isolation
- **Speed**: Fastest (no gRPC overhead)
- **Metrics**: Custom (multicast delivery)
- **Use Case**: Development, optimization

**Integration-Level Testing** (End-to-End):
```text
ghz → Rust Proxy (gRPC) → Pattern (gRPC) → Redis/NATS
```
- **Purpose**: Test production path
- **Speed**: Realistic (includes gRPC)
- **Metrics**: Standard (gRPC)
- **Use Case**: QA, production validation

#### Comparison

| Aspect | Pattern-Level | Integration-Level | Both? |
|--------|--------------|-------------------|-------|
| **Speed** | Fastest (&lt;1ms) | Realistic (+3-5ms gRPC) | ✅ Different use cases |
| **Metrics** | Custom ✅ | Standard only | ✅ Complementary |
| **Debugging** | Easiest | Harder | ✅ Pattern-level for dev |
| **Production Accuracy** | No proxy | Full stack ✅ | ✅ Integration for QA |
| **Coverage** | Pattern logic | Proxy + Pattern | ✅ Both needed |

**Conclusion**: Both types are necessary and complementary.

---

### Finding 3: ghz Best Tool for Integration Testing

#### Framework Evaluation Results

| Framework | gRPC | Custom Metrics | Learning Curve | Maintenance | Total Score |
|-----------|------|----------------|---------------|-------------|-------------|
| **ghz** | 5/5 | 2/5 | 4/5 | 5/5 | **24/30** ✅ |
| k6 | 3/5 | 4/5 | 2/5 | 5/5 | 20/30 |
| fortio | 5/5 | 2/5 | 4/5 | 4/5 | 22/30 |
| vegeta | 0/5 | - | - | - | Disqualified (no gRPC) |
| Custom | 0/5 | 5/5 | 5/5 | 2/5 | 22/30 |

**ghz wins** for integration testing (24/30):
- Native gRPC support
- Minimal learning curve
- Zero code maintenance
- Standard output formats (JSON, CSV, HTML)

**Custom tool wins** for pattern-level testing (22/30):
- Direct integration
- Custom metrics
- Fastest iteration

**Decision**: Use both (two-tier strategy)

---

## Recommendations

### Recommendation 1: Extract Shared Complexity to SDK

**Priority**: High (POC 1 blocker)

#### Phase 1: Foundation (3 days)

Implement three critical SDK packages:

**1. Connection Pool Manager** (`plugins/core/pool/`)
- Generic connection pooling with health checking
- ~300 LOC + ~200 LOC tests
- Coverage target: 90%+
- **Impact**: Reduces Redis and Kafka by ~150 LOC each

**2. TTL Manager** (`plugins/core/ttl/`)
- Heap-based key expiration (O(log n) vs O(1) per-key timers)
- ~250 LOC + ~150 LOC tests
- Coverage target: 95%+
- **Impact**: Reduces MemStore by ~80 LOC, 10x better scalability

**3. Health Check Framework** (`plugins/core/health/`)
- Standardized health checking with composite status
- ~200 LOC + ~100 LOC tests
- Coverage target: 90%+
- **Impact**: Reduces all plugins by ~50 LOC each

**Total Effort**: 3 days (one Go expert)

#### Phase 2: Convenience (2 days)

Implement supporting packages:

**4. gRPC Middleware** (`plugins/core/server/middleware.go`)
- Logging interceptor
- Error standardization interceptor
- ~150 LOC + ~80 LOC tests
- **Impact**: Reduces all plugins by ~30 LOC each

**5. Config Loader** (`plugins/core/config/`)
- Type-safe environment variable loading
- ~100 LOC + ~60 LOC tests
- **Impact**: Reduces all plugins by ~20 LOC each

**6. Circuit Breaker** (`plugins/core/storage/errors.go`)
- Error classification
- Circuit breaker for fault tolerance
- ~200 LOC + ~100 LOC tests
- **Impact**: Improves reliability (no LOC reduction)

**Total Effort**: 2 days

#### Phase 3: Refactor Plugins (2 days)

Refactor existing plugins to use new SDK packages:
- MemStore: Use `ttl.Manager` instead of per-key timers
- Redis: Use `pool.Pool` for connection management
- Kafka: Use `pool.Pool` for connection management
- All: Use `health.Checker`, `config.Loader`, middleware

**Total Effort**: 2 days

**Total Timeline**: **7 days** (1.5 weeks)

---

### Recommendation 2: Adopt Two-Tier Load Testing Strategy

**Priority**: High (POC 1 quality gate)

#### Keep Custom Tool (prism-loadtest)

**Enhancements**:
1. Add ramp-up load profile
2. Add spike load profile
3. Add JSON output format
4. Document usage patterns

**Effort**: 2 days

**Use Cases**:
- Pattern development and debugging
- Algorithm optimization (TTL, fan-out)
- Backend benchmarking (Redis vs SQLite)

#### Add ghz for Integration Testing

**Setup**:
1. Install ghz (`go install github.com/bojand/ghz/cmd/ghz@latest`)
2. Create test suite (`tests/load/ghz/`)
3. Add to CI/CD pipeline
4. Document baseline overhead (gRPC adds ~3-5ms)

**Effort**: 3 days

**Use Cases**:
- End-to-end testing through proxy
- Production validation
- CI/CD regression testing

**Total Timeline**: **5 days** (1 week)

---

### Recommendation 3: Implementation Order

**Week 1: SDK Foundation + Tool Enhancements**
- Days 1-3: Implement SDK packages (pool, TTL, health)
- Days 4-5: Enhance prism-loadtest (profiles, JSON output)

**Week 2: Integration + Validation**
- Days 1-2: Refactor plugins to use SDK
- Days 3-5: Add ghz integration testing + CI/CD

**Parallel Work Opportunities**:
- SDK development (Go expert)
- Load testing setup (DevOps/QA)
- Can run concurrently

---

## Benefits

### Developer Experience

**Before** (current state):
```go
// Redis plugin: ~700 LOC
// - 150 LOC connection pooling
// - 50 LOC health checks
// - 40 LOC config loading
// - 460 LOC actual Redis logic
```

**After** (with SDK):
```go
// Redis plugin: ~450 LOC
// - 10 LOC pool setup (use sdk.Pool)
// - 5 LOC health setup (use sdk.Health)
// - 5 LOC config setup (use sdk.Config)
// - 430 LOC actual Redis logic (cleaned up)
```

**Result**: **36% less boilerplate**, focus on business logic

### Code Quality

**SDK Packages**:
- 85-95% test coverage (enforced in CI)
- Comprehensive unit tests
- Race detector clean
- Benchmarked performance

**Plugins**:
- Inherit SDK quality
- Focus on integration tests
- Reduced surface area for bugs

### Maintenance

**Before**: 3 custom implementations × 3 features = 9 code paths to maintain

**After**: 1 SDK implementation × 3 features = 3 code paths to maintain

**Reduction**: 67% fewer code paths

### Testing

**Pattern-Level** (prism-loadtest):
- Fastest iteration (&lt;1ms latency)
- Custom metrics (multicast delivery rate)
- Isolated debugging

**Integration-Level** (ghz):
- Production accuracy (includes gRPC)
- Standard reporting (JSON, CSV, HTML)
- CI/CD integration

**Result**: Comprehensive coverage at two levels

---

## Risks and Mitigations

### Risk 1: SDK Complexity

**Risk**: SDK becomes too complex and hard to understand.

**Mitigation**:
- Keep packages focused (single responsibility)
- Comprehensive documentation with examples
- Code reviews for all SDK changes
- Target: 85%+ test coverage

**Probability**: Low (packages are well-scoped)

### Risk 2: Schedule Impact

**Risk**: SDK work delays POC 1 plugin implementation.

**Mitigation**:
- Parallel work streams (SDK + load testing)
- Incremental adoption (plugins can start without SDK)
- Total: 2 weeks (fits within POC 1 timeline)

**Probability**: Low (work is additive, not blocking)

### Risk 3: Tool Proliferation

**Risk**: Team confused about which load testing tool to use.

**Mitigation**:
- Clear decision matrix (pattern-level vs integration)
- Documentation in RFC-029
- Training/examples for both tools

**Probability**: Low (two tools with clear separation)

### Risk 4: Performance Regression

**Risk**: Generic SDK code slower than custom implementations.

**Mitigation**:
- Benchmark all SDK packages
- Compare against custom implementations
- Target: No regression (&lt;5% acceptable)
- Example: TTL manager 10x faster (heap vs per-key timers)

**Probability**: Very Low (SDK uses better algorithms)

---

## Success Metrics

### Code Quality Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| SDK Test Coverage | 85%+ | `make coverage-sdk` |
| Plugin Code Reduction | 35%+ | LOC comparison |
| Bug Reduction | 40%+ | Bugs in connection/TTL logic |
| Build Time | &lt;30s | CI/CD pipeline |

### Performance Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Pattern-Level P95 | &lt;5ms | prism-loadtest results |
| Integration P95 | &lt;10ms | ghz results |
| gRPC Overhead | 3-5ms | Diff between tools |
| TTL Performance | 10x improvement | Benchmark: 10K keys |

### Developer Experience Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| New Plugin Time | -30% | Time to add plugin |
| SDK Adoption | 100% | All plugins use SDK |
| Tool Clarity | 100% | Team knows which tool to use |

---

## Implementation Plan

### Week 1: Foundation

**Days 1-3: SDK Packages** (Go Expert)
- [ ] Implement `pool.Pool` with tests (90%+ coverage)
- [ ] Implement `ttl.Manager` with tests (95%+ coverage)
- [ ] Implement `health.Checker` with tests (90%+ coverage)
- [ ] Run `make coverage-sdk` to verify

**Days 4-5: Load Testing** (DevOps/QA)
- [ ] Enhance prism-loadtest with profiles
- [ ] Add JSON output format
- [ ] Document usage patterns

### Week 2: Integration

**Days 1-2: Plugin Refactoring** (Go Expert)
- [ ] Refactor MemStore to use `ttl.Manager`
- [ ] Refactor Redis to use `pool.Pool` and `health.Checker`
- [ ] Refactor Kafka to use `pool.Pool` and `health.Checker`
- [ ] Verify all tests pass

**Days 3-5: ghz Integration** (DevOps/QA)
- [ ] Install ghz
- [ ] Create ghz test suite for each pattern
- [ ] Add to CI/CD pipeline
- [ ] Document baseline performance

**Day 5: Validation**
- [ ] Run full test suite (pattern-level + integration)
- [ ] Compare results (expected: gRPC adds 3-5ms)
- [ ] Generate final report

---

## Decision Matrix

### When to Use Pattern-Level Testing (prism-loadtest)

| Scenario | Rationale |
|----------|-----------|
| **Pattern Development** | Fast iteration, no proxy needed |
| **Algorithm Optimization** | Isolated testing (TTL, fan-out) |
| **Backend Benchmarking** | Compare Redis vs SQLite |
| **Debugging** | Easiest to debug pattern logic |
| **Custom Metrics** | Multicast delivery rate |

### When to Use Integration Testing (ghz)

| Scenario | Rationale |
|----------|-----------|
| **End-to-End Validation** | Tests full production path |
| **Proxy Performance** | Includes gRPC overhead |
| **CI/CD Regression** | Standard tool for automation |
| **Production Load Simulation** | Realistic conditions |
| **QA Acceptance** | Standard reporting format |

---

## Cost-Benefit Analysis

### Investment

| Component | Effort | LOC | Test Coverage |
|-----------|--------|-----|---------------|
| SDK Packages (3) | 3 days | ~750 | 90%+ |
| SDK Packages (3 more) | 2 days | ~450 | 85%+ |
| Plugin Refactoring | 2 days | -800 (net) | 80%+ |
| Load Testing | 5 days | ~100 | N/A |
| **Total** | **12 days** | **~400 net** | **85%+** |

### Return

| Benefit | Value | Measurement |
|---------|-------|-------------|
| **Code Reduction** | 800 LOC | 38% less plugin code |
| **Quality Improvement** | 85%+ coverage | SDK packages |
| **Developer Productivity** | 30% faster | New plugin development |
| **Maintenance Reduction** | 67% fewer paths | SDK centralization |
| **Testing Coverage** | 2 levels | Pattern + Integration |
| **Bug Reduction** | 40% fewer bugs | Shared, tested code |

**ROI**: 12 days investment → 30% faster development + 38% less code + 67% less maintenance

**Payback Period**: ~1 month (after 3-4 new plugins)

---

## Alternatives Considered

### Alternative 1: No SDK Enhancements (Status Quo)

**Pros**:
- No additional work
- Plugins work as-is

**Cons**:
- ❌ 800 LOC duplication
- ❌ Inconsistent implementations
- ❌ Higher maintenance burden
- ❌ More bugs

**Decision**: Rejected - duplication too costly

### Alternative 2: Third-Party Libraries for Pooling/TTL

**Pros**:
- Battle-tested
- No implementation work

**Cons**:
- ❌ External dependencies
- ❌ May not fit use cases
- ❌ Less control

**Decision**: Rejected - need Prism-specific features (health integration, custom metrics)

### Alternative 3: Single Tool (ghz Only or Custom Only)

**ghz Only**:
- ❌ Can't test patterns directly
- ❌ No custom metrics
- ❌ Slower iteration

**Custom Only**:
- ❌ No integration testing
- ❌ Can't validate proxy
- ❌ Missing production path

**Decision**: Rejected - both tools needed for different use cases

---

## Next Steps

### Immediate (This Week)

1. **Review this memo** with team
2. **Approve SDK packages** (pool, TTL, health)
3. **Assign owners**:
   - SDK implementation: Go expert
   - Load testing: DevOps/QA
4. **Create tracking issues** in GitHub

### Short-Term (Next 2 Weeks)

5. **Implement SDK packages** (Week 1, Days 1-3)
6. **Enhance prism-loadtest** (Week 1, Days 4-5)
7. **Refactor plugins** (Week 2, Days 1-2)
8. **Add ghz integration** (Week 2, Days 3-5)
9. **Validate and measure** (Week 2, Day 5)

### Long-Term (POC 2+)

10. **Evaluate SDK adoption** (after POC 1)
11. **Measure developer productivity** (time to add new plugin)
12. **Consider k6** (if distributed load testing needed)

---

## Related Documents

- **[MEMO-014: Pattern SDK Shared Complexity](/memos/memo-014)** - Detailed SDK analysis
- **[RFC-029: Load Testing Framework Evaluation](/rfc/rfc-029)** - Framework comparison
- **[RFC-021: POC 1 Three Plugins Implementation](/rfc/rfc-021)** - Plugin design
- **[RFC-022: Core Pattern SDK Code Layout](/rfc/rfc-022)** - SDK structure
- **[MEMO-010: Load Test Results](/memos/memo-010)** - Custom tool validation
- **[RFC-018: POC Implementation Strategy](/rfc/rfc-018)** - Overall POC roadmap

---

## Appendix A: Code Examples

### Before SDK (Redis Plugin)

```go
// plugins/redis/client/pool.go (~150 LOC)
type ConnectionPool struct {
    mu       sync.Mutex
    conns    []*redis.Client
    maxConns int
    // ... custom pooling logic
}

// plugins/redis/client/health.go (~50 LOC)
type HealthChecker struct {
    client *redis.Client
    // ... custom health check logic
}

// plugins/redis/config.go (~40 LOC)
func loadConfig() RedisConfig {
    addr := os.Getenv("REDIS_ADDR")
    if addr == "" {
        addr = "localhost:6379"
    }
    // ... custom config loading
}
```

**Total**: ~240 LOC of boilerplate

### After SDK (Redis Plugin)

```go
// plugins/redis/main.go (~30 LOC)
import (
    "github.com/prism/plugins/core/pool"
    "github.com/prism/plugins/core/health"
    "github.com/prism/plugins/core/config"
)

func main() {
    // Config loading (5 LOC)
    cfg := config.NewLoader("REDIS")
    addr := cfg.Required("ADDR")

    // Connection pool (10 LOC)
    pool := pool.NewPool(redisFactory(addr), pool.Config{
        MinIdle: 5,
        MaxOpen: 50,
    })

    // Health checks (5 LOC)
    health := health.NewChecker(health.Config{
        Interval: 30 * time.Second,
    })
    health.Register("redis", func(ctx context.Context) error {
        conn, _ := pool.Acquire(ctx)
        defer pool.Release(conn)
        return conn.(*RedisConnection).Health(ctx)
    })

    // ... actual Redis logic
}
```

**Total**: ~30 LOC (88% reduction in boilerplate)

---

## Appendix B: Load Testing Example

### Pattern-Level Test

```bash
# Test pattern logic directly (fastest)
./prism-loadtest register -r 100 -d 60s --redis-addr localhost:6379

# Output:
# Register Operations:
#   Total Requests:   3053
#   Success Rate:     100.00%
#   Latency P95:      5ms     ← No gRPC overhead
#   Latency P99:      50ms
```

### Integration-Level Test

```bash
# Test through Rust proxy (realistic)
ghz --proto proto/interfaces/keyvalue_basic.proto \
    --call prism.KeyValueBasicInterface.Set \
    --insecure \
    --rps 100 \
    --duration 60s \
    --data '{"namespace":"default","key":"test-{{.RequestNumber}}","value":"dGVzdA=="}' \
    localhost:8980

# Output:
# Summary:
#   Count:        6000
#   Requests/sec: 100.00
#   Average:      8.2ms    ← gRPC adds ~3ms
#   95th %ile:    10ms
#   99th %ile:    15ms
```

**Observation**: Integration test adds ~3-5ms latency (expected gRPC overhead)

---

## Conclusion

This memo synthesizes findings from MEMO-014 (SDK analysis) and RFC-029 (load testing evaluation) to propose a comprehensive infrastructure strategy for POC 1:

1. **Extract shared complexity to Pattern SDK** (38% code reduction)
2. **Adopt two-tier load testing strategy** (pattern + integration)
3. **Implement in 2-week sprint** (12 days effort)

**Expected Outcomes**:
- ✅ Faster plugin development (30% time savings)
- ✅ Higher code quality (85%+ SDK coverage)
- ✅ Better testing (two-level coverage)
- ✅ Reduced maintenance (67% fewer code paths)

**Recommendation**: Proceed with implementation alongside POC 1 plugin development.

---

## Revision History

- 2025-10-11: Initial synthesis of SDK analysis and load testing evaluation
