---
id: memo-005
title: "MEMO-005: Client Protocol Design Philosophy - Composition vs Use-Case Specificity"
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [api-design, patterns, developer-experience, architecture, evolution]
---

# MEMO-005: Client Protocol Design Philosophy

## Purpose

Resolve the architectural tension between:
- **Composable primitives** (RFC-014: KeyValue, PubSub, Queue) - generic, reusable, small API surface
- **Use-case-specific protocols** (RFC-017: Multicast Registry) - ergonomic, self-documenting, purpose-built

**Core Question**: Should Prism offer one protobuf service per use case (IoT, presence, service discovery) or force applications to compose generic primitives?

## Context

### RFC-014: Layered Data Access Patterns (Composable Approach)

**Defines 6 generic patterns**:
```protobuf
service KeyValueService {
  rpc Set(SetRequest) returns (SetResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Scan(ScanRequest) returns (stream ScanResponse);
}

service PubSubService {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (stream Message);
}
```

**Application must compose**:
```python
# IoT device management - application composes primitives
await client.keyvalue.set(f"device:{id}", metadata)  # Registry
devices = await client.keyvalue.scan("device:*")     # Enumerate
await client.pubsub.publish("commands", message)     # Broadcast
```

**Benefits**:
- ✅ Small API surface (6 services)
- ✅ Reusable across use cases
- ✅ Easy to implement in proxy
- ✅ Schema evolution is localized

**Drawbacks**:
- ❌ Application must understand composition
- ❌ Boilerplate code for common patterns
- ❌ No semantic guarantees (e.g., registry consistency)
- ❌ Steep learning curve

### RFC-017: Multicast Registry (Use-Case-Specific Approach)

**Defines purpose-built API**:
```protobuf
service MulticastRegistryService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Enumerate(EnumerateRequest) returns (EnumerateResponse);
  rpc Multicast(MulticastRequest) returns (MulticastResponse);
}
```

**Application uses clear semantics**:
```python
# IoT device management - clear intent
await client.registry.register(
    identity="device-123",
    metadata={"type": "sensor", "location": "building-a"}
)

devices = await client.registry.enumerate(
    filter={"location": "building-a"}
)

result = await client.registry.multicast(
    filter={"type": "sensor"},
    message={"command": "read_temperature"}
)
```

**Benefits**:
- ✅ Self-documenting (clear purpose)
- ✅ Less boilerplate
- ✅ Semantic guarantees (coordinated registry + messaging)
- ✅ Easier for application developers

**Drawbacks**:
- ❌ API proliferation (one service per use case?)
- ❌ More code in proxy (20+ services?)
- ❌ Schema evolution harder (changes affect specific use cases)
- ❌ Duplication across similar patterns

## Design Principles

### 1. Push Complexity Down from Application Developers ⭐ **PRIMARY**

**Goal**: Developers shouldn't need to understand distributed systems internals.

**Implications**:
- ✅ Favor use-case-specific APIs (e.g., Multicast Registry)
- ✅ Hide coordination complexity (e.g., keeping registry + pub/sub consistent)
- ✅ Provide semantic guarantees (e.g., "multicast delivers to all registered")
- ❌ Avoid forcing developers to compose primitives manually

**Example**:
```python
# BAD: Application must coordinate registry + pub/sub
devices = await client.keyvalue.scan("device:*")
for device in devices:
    await client.pubsub.publish(f"device:{device}", message)
# Problem: Race condition if device registers between scan and publish

# GOOD: Prism coordinates atomically
await client.registry.multicast(filter={}, message=message)
# Prism guarantees atomicity: enumerate + fan-out
```

### 2. Developer Comprehension and Usability ⭐ **PRIMARY**

**Goal**: APIs should be immediately understandable without deep documentation.

**Implications**:
- ✅ Favor self-documenting method names (`register`, `enumerate`, `multicast`)
- ✅ Provide rich error messages
- ✅ Include use-case examples in docs
- ❌ Avoid generic terms requiring mental mapping (e.g., "put into keyvalue to register")

**Example**:
```protobuf
// CLEAR: Purpose obvious from name
rpc Register(RegisterRequest) returns (RegisterResponse);

// UNCLEAR: What am I setting? Why?
rpc Set(SetRequest) returns (SetResponse);
```

### 3. Schema and Service Evolution

**Goal**: Add features without breaking existing clients.

**Implications**:
- ✅ Fewer services = fewer breaking changes (favor composition)
- ✅ Backward-compatible field additions
- ⚠️ Use-case-specific services are easier to version independently
- ❌ Changing generic primitive affects many use cases

**Example**:
```protobuf
// Adding feature to MulticastRegistry: localized impact
message RegisterRequest {
  string identity = 1;
  map<string, Value> metadata = 2;
  optional int64 ttl_seconds = 3;  // NEW: backward compatible
}

// Adding feature to KeyValue: affects ALL use cases
message SetRequest {
  string key = 1;
  bytes value = 2;
  optional int64 ttl_seconds = 3;  // NEW: breaks IoT, presence, etc.
}
```

### 4. Keep Proxy Small and Tight

**Goal**: Minimize proxy complexity and resource footprint.

**Implications**:
- ✅ Favor generic primitives (fewer service implementations)
- ✅ Pattern coordinators can be plugins (not in core proxy)
- ⚠️ Use-case services increase code size
- ❌ Too many services = maintenance burden

**Example**:
Proxy with 6 generic services: ~10k LOC, 50MB binary
Proxy with 20 use-case services: ~40k LOC, 150MB binary
```text

## Proposed Solution: Layered API Architecture

**Insight**: We don't have to choose! Provide **both layers** with clear separation.

### Layer 1: Primitives (Generic, Always Available)

**Six core primitives** (RFC-014):
```
service KeyValueService { ... }
service PubSubService { ... }
service QueueService { ... }
service TimeSeriesService { ... }
service GraphService { ... }
service TransactionalService { ... }
```text

**Characteristics**:
- ✅ Always available (core proxy functionality)
- ✅ Stable API (rarely changes)
- ✅ Generic (works for any use case)
- ❌ Requires composition knowledge

**Target Users**:
- Advanced developers building custom patterns
- Performance-critical applications (direct control)
- Unusual use cases not covered by Layer 2

### Layer 2: Patterns (Use-Case-Specific, Opt-In)

**Purpose-built patterns** (RFC-017, plus more):
```
service MulticastRegistryService { ... }  // IoT, presence, service discovery
service SagaService { ... }               // Distributed transactions
service EventSourcingService { ... }      // Audit trails, event log
service WorkQueueService { ... }          // Background jobs
service CacheAsideService { ... }         // Read-through cache
```text

**Characteristics**:
- ✅ Self-documenting (clear purpose)
- ✅ Semantic guarantees (coordinated operations)
- ✅ Less boilerplate (ergonomic APIs)
- ⚠️ Implemented as **pattern coordinators** (plugins, not core)

**Target Users**:
- Most application developers (80% of use cases)
- Teams prioritizing velocity over control
- Developers new to distributed systems

### Implementation Strategy

#### Pattern Coordinators Live in Plugins, Not Core Proxy

┌─────────────────────────────────────────────────────────┐
│                   Prism Proxy (Core)                    │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Layer 1: Primitives (Always Available)          │   │
│  │  - KeyValueService                               │   │
│  │  - PubSubService                                 │   │
│  │  - QueueService                                  │   │
│  │  - (3 more...)                                   │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                         │
                         │ gRPC
                         │
┌────────────────────────▼─────────────────────────────────┐
│           Pattern Coordinator Plugins (Opt-In)           │
│  ┌────────────────────┐  ┌────────────────────────┐     │
│  │ Multicast Registry │  │  Saga Coordinator      │     │
│  │ (RFC-017)          │  │  (Distributed Txn)     │     │
│  └────────────────────┘  └────────────────────────┘     │
│  ┌────────────────────┐  ┌────────────────────────┐     │
│  │ Event Sourcing     │  │  Cache Aside           │     │
│  │ (Append Log)       │  │  (Read-Through)        │     │
│  └────────────────────┘  └────────────────────────┘     │
└──────────────────────────────────────────────────────────┘
                         │
                         │ Uses Layer 1 APIs
                         ▼
                 (Composes primitives)
```

**Benefits**:
- ✅ Core proxy stays small (~10k LOC)
- ✅ Pattern plugins are optional (install only what you use)
- ✅ Independent evolution (update registry plugin without touching core)
- ✅ Community can contribute patterns (not just core team)

**Configuration**:
```yaml
namespaces:
  - name: iot-devices
    # Option A: Use primitive (advanced)
    pattern: keyvalue
    backend:
      type: redis

  - name: iot-commands
    # Option B: Use pattern coordinator (ergonomic)
    pattern: multicast-registry
    coordinator_plugin: prism-multicast-registry:v1.2.0
    backend_slots:
      registry:
        type: redis
      messaging:
        type: nats
```

## Decision Matrix

| Concern | Primitives (Layer 1) | Patterns (Layer 2) | Layered Approach |
|---------|---------------------|-------------------|------------------|
| **Developer Complexity** | ❌ High (must compose) | ✅ Low (ergonomic) | ✅ Choice per use case |
| **API Clarity** | ⚠️ Generic terms | ✅ Self-documenting | ✅ Clear at both layers |
| **Proxy Size** | ✅ Small (6 services) | ❌ Large (20+ services) | ✅ Core small, plugins opt-in |
| **Schema Evolution** | ⚠️ Affects all use cases | ✅ Localized impact | ✅ Primitives stable, patterns evolve independently |
| **Flexibility** | ✅ Unlimited composition | ⚠️ Fixed patterns | ✅ Both available |
| **Performance** | ✅ Direct control | ⚠️ Coordinator overhead | ✅ Choose based on need |
| **Learning Curve** | ❌ Steep (distributed systems knowledge) | ✅ Gentle (use-case driven) | ✅ Start simple, grow into advanced |

## Comparison to Alternatives

### Alternative A: Primitives Only (No Layer 2)

**Example**: AWS DynamoDB, Redis, etcd

**Pros**:
- Simple implementation
- Small API surface
- Maximum flexibility

**Cons**:
- ❌ High application complexity
- ❌ Every team reimplements common patterns
- ❌ No semantic guarantees

**Verdict**: Too low-level for Prism's goal of "push complexity down"

### Alternative B: Use-Case APIs Only (No Layer 1)

**Example**: Twilio (SendMessage), Stripe (CreateCharge), Firebase (specific SDKs)

**Pros**:
- Ergonomic
- Self-documenting
- Fast onboarding

**Cons**:
- ❌ API explosion (100+ services?)
- ❌ Inflexible (can't compose novel patterns)
- ❌ Large proxy binary

**Verdict**: Too rigid for Prism's diverse use cases

### Alternative C: Hybrid (Like Kubernetes)

**Example**: Kubernetes (core API + CRDs + Operators)

**Pros**:
- ✅ Core stays stable
- ✅ Extensible (community patterns)
- ✅ Balances simplicity and power

**Cons**:
- ⚠️ Two-tier documentation complexity
- ⚠️ Requires clear guidance on when to use each layer

**Verdict**: ✅ **Best fit** - matches Prism's architecture

## Implementation Roadmap

### Phase 1: POC Validation (Weeks 1-6, RFC-018)

**Implement Layer 1 only** (KeyValue, PubSub):
- POC 1: KeyValue with MemStore
- POC 2: KeyValue with Redis
- POC 3: PubSub with NATS

**Goal**: Prove primitives are sufficient for basic use cases.

### Phase 2: Pattern Coordinator Prototype (Weeks 7-9, RFC-018)

**Implement one Layer 2 pattern** (Multicast Registry):
- POC 4: Multicast Registry coordinator plugin
- Validate plugin architecture
- Measure coordination overhead

**Goal**: Prove pattern coordinators add value without excessive complexity.

### Phase 3: Expand Pattern Library (Post-POC, Weeks 12+)

**Add 3-5 common patterns**:
- Saga coordinator (distributed transactions)
- Event sourcing (append-only log + replay)
- Work queue (background jobs)
- Cache aside (read-through cache)
- Rate limiter (token bucket)

**Goal**: Cover 80% of use cases with Layer 2 patterns.

### Phase 4: Community Patterns (Months 3-6)

**Enable third-party pattern plugins**:
- Pattern plugin SDK
- Plugin marketplace
- Certification program

**Goal**: Ecosystem of community-contributed patterns.

## Naming Conventions

### Layer 1: Primitives Use Abstract Nouns

```protobuf
service KeyValueService { ... }    // Generic storage
service PubSubService { ... }      // Generic messaging
service QueueService { ... }       // Generic queue
```

**Rationale**: Abstract names signal "building block" nature.

### Layer 2: Patterns Use Domain-Specific Verbs

```protobuf
service MulticastRegistryService { ... }  // Identity management + broadcast
service SagaService { ... }               // Multi-step transactions
service EventSourcingService { ... }      // Audit-logged mutations
```

**Rationale**: Specific names signal purpose and use case.

## Open Questions

### 1. How do we prevent Layer 2 explosion?

**Proposal**: Curated pattern library with strict acceptance criteria:
- Must solve a common problem (&gt;10% of use cases)
- Must provide semantic guarantees over Layer 1 composition
- Must have clear ownership and maintenance plan

**Example rejection**: `BlogPostService` (too specific, just use KeyValue)

### 2. Can Layer 2 patterns compose with each other?

**Example**: Saga + Multicast Registry?

**Proposal**: Yes, but patterns should compose via Layer 1 APIs (not directly call each other).

SagaService (Layer 2)
  ↓ uses
KeyValueService (Layer 1)
  ↑ used by
MulticastRegistryService (Layer 2)
```text

**Rationale**: Keeps patterns loosely coupled, evolution independent.

### 3. How do we version Layer 2 patterns independently?

**Proposal**: Pattern coordinators are plugins with semantic versioning:

```
coordinator_plugin: prism-multicast-registry:v1.2.0
```text

**Migration path**:
- v1.x: Breaking changes → new major version
- v2.x: Runs side-by-side with v1.x
- Namespaces pin to specific version

### 4. Should Layer 1 APIs be Sufficient for All Use Cases?

**Proposal**: Yes - Layer 1 is Turing-complete (can implement any pattern).

**Rationale**: If a pattern can't be built on Layer 1, we have a gap in primitives (not just missing sugar).

**Litmus test**: If Multicast Registry can't be implemented using KeyValue + PubSub, we need to add primitives.

## Success Metrics

### Developer Experience

- ✅ **Onboarding time**: New developers productive in &lt;1 day (using Layer 2)
- ✅ **Code reduction**: Layer 2 reduces boilerplate by &gt;50% vs Layer 1 composition
- ✅ **Error clarity**: 90% of errors are self-explanatory without docs

### System Complexity

- ✅ **Core proxy size**: Remains &lt;15k LOC (only Layer 1)
- ✅ **Binary size**: Core &lt;75MB, each pattern plugin &lt;10MB
- ✅ **Dependency count**: Core has &lt;20 dependencies

### Pattern Adoption

- ✅ **Coverage**: Layer 2 patterns cover &gt;80% of use cases
- ✅ **Usage split**: 80% of applications use at least one Layer 2 pattern
- ✅ **Community**: 5+ community-contributed patterns within 6 months

## Related Documents

- [RFC-014: Layered Data Access Patterns](/rfc/rfc-014-layered-data-access-patterns) - Layer 1 primitives
- [RFC-017: Multicast Registry Pattern](/rfc/rfc-017-multicast-registry-pattern) - First Layer 2 pattern
- [RFC-018: POC Implementation Strategy](/rfc/rfc-018-poc-implementation-strategy) - Phased rollout plan
- [RFC-008: Proxy Plugin Architecture](/rfc/rfc-008-proxy-plugin-architecture) - Plugin system

## Revision History

- 2025-10-09: Initial draft proposing layered API architecture (primitives + patterns)


```