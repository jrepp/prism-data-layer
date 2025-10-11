# POC 4: Multicast Registry Pattern - Implementation Tracking

**Status**: 🚧 IN PROGRESS
**Started**: 2025-10-11
**Estimated Duration**: 3 weeks
**Complexity**: High (Composite pattern with multiple backend slots)

## Objective

Demonstrate **pattern composition** implementing RFC-017 Multicast Registry Pattern with:
- Register/Enumerate/Multicast operations
- Multiple backend slots (registry + messaging + optional durability)
- Filter expression language
- Complex coordination logic between backends

## Success Criteria

### Functional Requirements

| Requirement | Test | Status |
|-------------|------|--------|
| Register identity with metadata | `test_register_identity` | ⬜ |
| Enumerate with filter expression | `test_enumerate_filtered` | ⬜ |
| Multicast to all identities | `test_multicast_all` | ⬜ |
| Multicast to filtered subset | `test_multicast_filtered` | ⬜ |
| TTL expiration removes identity | `test_ttl_expiration` | ⬜ |
| Unregister removes identity | `test_unregister` | ⬜ |
| Filter evaluation (equality, comparison) | `test_filter_operators` | ⬜ |
| Multiple subscribers receive multicast | `test_fanout_delivery` | ⬜ |

### Non-Functional Requirements

| Requirement | Target | Actual | Status |
|-------------|--------|--------|--------|
| Enumerate with filter | <20ms (1000 identities) | TBD | ⬜ |
| Multicast to 100 identities | <100ms | TBD | ⬜ |
| Concurrent register/multicast | No race conditions | TBD | ⬜ |
| Test coverage | >80% | TBD | ⬜ |

### Code Coverage Requirements

| Component | Target | Actual | Status |
|-----------|--------|--------|--------|
| Registry coordinator | 85%+ | TBD | ⬜ |
| Filter evaluator | 90%+ | TBD | ⬜ |
| Backend slot handlers | 80%+ | TBD | ⬜ |
| Integration tests | All passing | TBD | ⬜ |

## Implementation Plan

### Week 1: Core Pattern Infrastructure

**Goal**: Build pattern coordinator and backend slot architecture

#### Day 1-2: Pattern Coordinator Skeleton
- [ ] Create `patterns/multicast_registry/` directory structure
- [ ] Implement `MulticastRegistryCoordinator` struct
- [ ] Define backend slot interfaces (Registry, Messaging, Durability)
- [ ] **TDD**: Write coordinator lifecycle tests first
- [ ] **Coverage Target**: 85%+

**Files**:
```
patterns/multicast_registry/
├── coordinator.go          # Main coordinator logic
├── coordinator_test.go     # TDD tests
├── slots.go               # Backend slot interfaces
└── config.go              # Configuration structs
```

#### Day 3: Filter Expression Parser
- [ ] Implement filter expression AST
- [ ] Parser for JSON-like filter syntax
- [ ] **TDD**: Write filter parsing tests (20+ test cases)
- [ ] Support operators: eq, ne, lt, lte, gt, gte, startswith, contains
- [ ] **Coverage Target**: 90%+

**Files**:
```
patterns/multicast_registry/filter/
├── ast.go                 # Filter AST nodes
├── parser.go              # JSON → AST parser
├── parser_test.go         # TDD tests with 20+ cases
└── evaluator.go           # Evaluate filter against metadata
```

#### Day 4-5: Register + Enumerate Operations
- [ ] Implement Register operation (write to registry backend)
- [ ] Implement Enumerate operation (query registry with filter)
- [ ] **TDD**: Write register/enumerate integration tests
- [ ] Use MemStore as registry backend initially
- [ ] **Coverage Target**: 85%+

**Protobuf**:
```protobuf
// proto/prism/pattern/multicast_registry.proto
service MulticastRegistryPattern {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Enumerate(EnumerateRequest) returns (EnumerateResponse);
  rpc Multicast(MulticastRequest) returns (MulticastResponse);
  rpc Unregister(UnregisterRequest) returns (UnregisterResponse);
}
```

### Week 2: Multicast and Backend Integration

**Goal**: Implement multicast fan-out and integrate real backends

#### Day 6-7: Multicast Operation
- [ ] Implement Multicast operation
- [ ] Enumerate → Filter → Fan-out algorithm
- [ ] Parallel message delivery to messaging backend
- [ ] **TDD**: Write multicast fan-out tests
- [ ] Test with 10, 100, 1000 identities
- [ ] **Coverage Target**: 85%+

**Algorithm**:
```go
func (c *Coordinator) Multicast(req *MulticastRequest) (*MulticastResponse, error) {
    // 1. Evaluate filter to find target identities
    targets := c.registry.Enumerate(req.Filter)

    // 2. Fan out to messaging backend
    var wg sync.WaitGroup
    results := make(chan DeliveryResult, len(targets))

    for _, identity := range targets {
        wg.Add(1)
        go func(id string) {
            defer wg.Done()
            err := c.messaging.Publish(identityTopic(id), req.Payload)
            results <- DeliveryResult{Identity: id, Error: err}
        }(identity.ID)
    }

    wg.Wait()
    close(results)

    // 3. Aggregate delivery status
    return aggregateResults(results)
}
```

#### Day 8-9: Backend Slot Integration
- [ ] Extend Redis pattern to support registry slot
- [ ] Use NATS pattern for messaging slot
- [ ] **TDD**: Integration tests with Redis+NATS backends
- [ ] Test TTL expiration with Redis EXPIRE
- [ ] **Coverage Target**: 80%+

**Configuration**:
```yaml
namespaces:
  - name: presence
    pattern: multicast-registry

    backend_slots:
      registry:
        type: redis
        host: localhost:6379
        ttl_default: 300

      messaging:
        type: nats
        servers: ["nats://localhost:4222"]
        delivery: at-most-once
```

#### Day 10: TTL and Cleanup
- [ ] Implement TTL expiration via backend (Redis EXPIRE)
- [ ] Background cleanup goroutine for expired identities
- [ ] Unregister operation
- [ ] **TDD**: TTL expiration tests with time mocking
- [ ] **Coverage Target**: 85%+

### Week 3: Advanced Features and Polish

**Goal**: Production-ready features and comprehensive testing

#### Day 11-12: Filter Expression Evaluation
- [ ] Implement backend-native filtering (Redis Lua scripts)
- [ ] Fallback to client-side filtering for backends without native support
- [ ] **TDD**: Performance tests comparing native vs client-side
- [ ] Test with 1000 identities, complex filters
- [ ] **Coverage Target**: 90%+

**Strategies**:
```go
// Backend-native filtering (Redis Lua)
func (r *RedisRegistry) EnumerateNative(filter *Filter) ([]Identity, error) {
    luaScript := translateFilterToLua(filter)
    return r.client.Eval(ctx, luaScript).Result()
}

// Client-side filtering (generic)
func (r *GenericRegistry) EnumerateFallback(filter *Filter) ([]Identity, error) {
    all := r.GetAll()
    return filterInMemory(all, filter)
}
```

#### Day 13-14: Delivery Status Tracking
- [ ] Track delivery success/failure per identity
- [ ] Retry logic for failed deliveries
- [ ] Timeout handling
- [ ] **TDD**: Delivery failure and retry tests
- [ ] **Coverage Target**: 85%+

**Response**:
```go
type MulticastResponse struct {
    TargetCount     int
    DeliveredCount  int
    FailedCount     int
    Statuses        []DeliveryStatus
}

type DeliveryStatus struct {
    Identity string
    Status   StatusEnum  // DELIVERED, PENDING, FAILED, TIMEOUT
    Error    string
}
```

#### Day 15: Integration Tests and Performance Validation
- [ ] End-to-end tests: Proxy → Pattern → Backends
- [ ] Load test with 100 concurrent operations
- [ ] Performance benchmarks (enumerate, multicast)
- [ ] **TDD**: All acceptance tests passing
- [ ] **Coverage Target**: Overall 85%+

**Tests**:
```go
// tests/acceptance/multicast_registry_test.go
func TestMulticastRegistry_EndToEnd(t *testing.T) {
    // Start Redis + NATS backends
    redis := testcontainers.RunRedis(t)
    nats := testcontainers.RunNATS(t)

    // Start proxy with multicast-registry pattern
    proxy := startProxy(t, multicastRegistryConfig(redis, nats))

    // Register 10 identities
    for i := 0; i < 10; i++ {
        proxy.Register(ctx, &RegisterRequest{
            Identity: fmt.Sprintf("user-%d", i),
            Metadata: map[string]interface{}{
                "status": "online",
                "room":   "engineering",
            },
        })
    }

    // Enumerate all online users in engineering room
    identities := proxy.Enumerate(ctx, &EnumerateRequest{
        Filter: &Filter{
            "status": "online",
            "room":   "engineering",
        },
    })
    assert.Equal(t, 10, len(identities.Identities))

    // Multicast to filtered subset (5 users)
    result := proxy.Multicast(ctx, &MulticastRequest{
        Filter: &Filter{"status": "online"},
        Payload: []byte("Hello!"),
    })
    assert.Equal(t, 10, result.TargetCount)
    assert.Equal(t, 10, result.DeliveredCount)
}
```

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│         Multicast Registry Pattern Coordinator          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Register(identity, metadata, ttl)                │ │
│  │    ├─> Store in Registry Backend (Redis)          │ │
│  │    └─> Create subscriber mapping                  │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Enumerate(filter)                                │ │
│  │    ├─> Query Registry Backend                     │ │
│  │    ├─> Evaluate filter (native or client-side)    │ │
│  │    └─> Return matching identities                 │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Multicast(filter, payload)                       │ │
│  │    ├─> Enumerate identities matching filter       │ │
│  │    ├─> Fan-out to Messaging Backend (NATS)        │ │
│  │    └─> Aggregate delivery status                  │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │   Registry   │  │  Messaging   │  │  Durability  │ │
│  │   Slot       │  │    Slot      │  │  Slot (Opt)  │ │
│  │  (Redis)     │  │   (NATS)     │  │              │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────┘
           │                  │                  │
           ▼                  ▼                  ▼
     ┌──────────┐      ┌──────────┐      ┌──────────┐
     │  Redis   │      │   NATS   │      │  Kafka   │
     │  (local) │      │  (local) │      │ (future) │
     └──────────┘      └──────────┘      └──────────┘
```

## Deliverables

### Code Deliverables
- [ ] `patterns/multicast_registry/` - Pattern coordinator implementation
- [ ] `patterns/multicast_registry/filter/` - Filter expression engine
- [ ] `proto/prism/pattern/multicast_registry.proto` - gRPC service definition
- [ ] `patterns/redis/` - Extended with registry slot support
- [ ] `patterns/nats/` - Used as messaging slot
- [ ] `tests/acceptance/multicast_registry_test.go` - Integration tests
- [ ] `Makefile` - Build targets for multicast-registry pattern

### Documentation Deliverables
- [ ] `docs-cms/pocs/POC-004-MULTICAST-REGISTRY.md` - This tracking document
- [ ] `docs-cms/rfcs/rfc-017-multicast-registry-pattern.md` - Pattern specification (already exists)
- [ ] Code comments and inline documentation

### Demo Deliverables
- [ ] Demo script showing register → enumerate → multicast workflow
- [ ] Performance benchmark results
- [ ] Coverage reports showing >80% across all components

## Test Strategy

### Unit Tests (TDD Approach)

**Write tests FIRST, then implement:**

1. **Filter Expression Tests** (90% coverage target)
   - Equality operators (eq, ne)
   - Comparison operators (lt, lte, gt, gte)
   - String operators (startswith, endswith, contains)
   - Logical operators (and, or, not)
   - Complex nested filters
   - Edge cases (null, empty, type mismatches)

2. **Coordinator Tests** (85% coverage target)
   - Register operation with various metadata
   - Enumerate with different filters
   - Multicast fan-out algorithm
   - TTL expiration handling
   - Concurrent operations (race detector clean)

3. **Backend Slot Tests** (80% coverage target)
   - Registry slot operations (set, get, scan, delete)
   - Messaging slot operations (publish, subscribe)
   - Error handling (connection failures, timeouts)

### Integration Tests

**Full end-to-end scenarios:**

1. **Basic Workflow**
   - Register 10 identities
   - Enumerate all (no filter)
   - Multicast to all

2. **Filtered Workflow**
   - Register identities with varied metadata
   - Enumerate with filter (should return subset)
   - Multicast to filtered subset

3. **TTL and Expiration**
   - Register with TTL
   - Wait for expiration
   - Verify identity removed from enumeration

4. **Concurrent Operations**
   - 100 goroutines register simultaneously
   - 50 goroutines enumerate simultaneously
   - 10 multicast operations in parallel
   - Verify consistency and no data races

5. **Performance**
   - Register 1000 identities
   - Enumerate with filter (<20ms target)
   - Multicast to 100 identities (<100ms target)

### Acceptance Tests

**POC 4 acceptance criteria (from RFC-018):**

- [ ] Register/enumerate/multicast operations work end-to-end
- [ ] Filter expressions correctly select identities
- [ ] Multicast delivers to all matching identities
- [ ] TTL expiration removes stale identities
- [ ] Enumerate with filter <20ms for 1000 identities
- [ ] Multicast to 100 identities <100ms
- [ ] Handle concurrent register/multicast operations (race detector clean)

## Risk Register

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Filter complexity explosion** | Medium | High | Limit filter depth (5 levels) and clause count (20 max) |
| **Backend inconsistency** (registry vs messaging) | Medium | High | Transactional coordination, retry logic |
| **Performance degradation with large registries** | Low | Medium | Backend-native filtering, client-side fallback |
| **Race conditions in concurrent operations** | Medium | High | TDD with race detector, proper locking |
| **TTL cleanup latency** | Low | Low | Background cleanup goroutine, Redis EXPIRE |

## Open Questions

1. **Should enumerate support pagination?**
   - **Proposal**: Yes, add cursor-based pagination for >100 results
   - **Timeline**: Week 2

2. **How to handle partial multicast failures?**
   - **Proposal**: Return aggregate status with per-identity delivery reports
   - **Timeline**: Week 3

3. **Should we support watch/subscribe for registry changes?**
   - **Proposal**: Defer to post-POC (adds significant complexity)
   - **Reasoning**: Not in success criteria

4. **Backend slot selection strategy?**
   - **Proposal**: Static configuration initially, dynamic selection post-POC
   - **Timeline**: Week 1 design decision

## Progress Tracking

### Daily Standup Format

**What did I complete yesterday?**
- List completed tasks with ✅ checkboxes updated

**What am I working on today?**
- Current task from implementation plan

**Blockers?**
- Any impediments or decisions needed

**Coverage update:**
- Current coverage percentage per component

### Weekly Review

**End of Week 1:**
- [ ] Coordinator skeleton complete with 85%+ coverage
- [ ] Filter parser complete with 90%+ coverage
- [ ] Register/Enumerate operations working

**End of Week 2:**
- [ ] Multicast operation complete with fan-out
- [ ] Redis+NATS backend integration working
- [ ] TTL expiration implemented

**End of Week 3:**
- [ ] All acceptance tests passing
- [ ] Performance benchmarks meet targets
- [ ] Overall coverage >85%
- [ ] POC 4 COMPLETE ✅

## Related Documents

- [RFC-017: Multicast Registry Pattern](/rfc/rfc-017) - Pattern specification
- [RFC-018: POC Implementation Strategy](/rfc/rfc-018) - Overall POC roadmap (POC 4 section)
- [RFC-014: Layered Data Access Patterns](/rfc/rfc-014) - Base pattern definitions
- [RFC-008: Proxy Plugin Architecture](/rfc/rfc-008) - Backend slot architecture

## Revision History

- **2025-10-11**: POC 4 kicked off - Created tracking document with 3-week implementation plan
