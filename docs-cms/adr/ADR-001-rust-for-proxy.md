---
id: adr-001
title: "ADR-001: Rust for the Proxy Implementation"
status: Accepted
date: 2025-10-05
deciders: Core Team
tags: ['proxy', 'performance', 'languages']
---

## Context

The Prism proxy is the performance-critical component that sits between all client applications and backend datastores. It must handle:

- 100,000+ requests per second per instance
- Sub-millisecond P50 latency
- P99 latency under 10ms
- Minimal resource footprint (CPU, memory)
- High reliability (handle errors gracefully, no crashes)

Netflix's Data Gateway uses Java/Spring Boot for their DAL containers. While functional, JVM-based solutions have inherent limitations:

- Garbage collection pauses impact tail latency
- Higher memory overhead
- Slower cold start times
- Less predictable performance under load

## Decision

Implement the Prism proxy in **Rust**.

## Rationale

### Why Rust?

1. **Performance**: Rust provides C/C++ level performance with zero-cost abstractions
2. **Memory Safety**: No null pointers, data races, or memory leaks without `unsafe`
3. **Predictable Latency**: No GC pauses; deterministic performance characteristics
4. **Excellent Async**: Tokio runtime provides best-in-class async I/O
5. **Strong Ecosystem**:
   - `tonic` for gRPC
   - `axum` for HTTP
   - `tower` for middleware/service composition
   - Excellent database drivers (postgres, kafka clients, etc.)

6. **Type Safety**: Protobuf integration with `prost` provides compile-time guarantees
7. **Resource Efficiency**: Lower memory and CPU usage means lower cloud costs

### Performance Comparison

Based on industry benchmarks and our prototypes:

| Metric | Java/Spring Boot | Rust/Tokio | Improvement |
|--------|------------------|------------|-------------|
| P50 Latency | ~5ms | ~0.3ms | **16x** |
| P99 Latency | ~50ms | ~2ms | **25x** |
| Throughput (RPS) | ~20k | ~200k | **10x** |
| Memory (idle) | ~500MB | ~20MB | **25x** |
| Cold Start | ~10s | ~100ms | **100x** |

### Alternatives Considered

1. **Java/Spring Boot** (Netflix's choice)
   - Pros:
     - Mature ecosystem
     - Large talent pool
     - Netflix has proven it at scale
   - Cons:
     - GC pauses hurt tail latency
     - Higher resource costs
     - Less predictable performance
   - Rejected because: Performance is a core differentiator for Prism

2. **Go**
   - Pros:
     - Good performance
     - Simple language
     - Fast compilation
     - Good concurrency primitives
   - Cons:
     - GC pauses (better than Java, but still present)
     - Less memory safety than Rust
     - Weaker type system
   - Rejected because: GC pauses are unacceptable for our latency SLOs

3. **C++**
   - Pros:
     - Maximum performance
     - Full control over memory
     - Mature ecosystem
   - Cons:
     - Memory safety issues require extreme discipline
     - Slower development velocity
     - Harder to maintain
   - Rejected because: Rust provides similar performance with better safety

4. **Zig**
   - Pros:
     - C-level performance
     - Simple language
     - Good interop
   - Cons:
     - Immature ecosystem
     - Fewer libraries
     - Smaller talent pool
   - Rejected because: Too risky for production system; ecosystem not mature enough

## Consequences

### Positive

- **Extreme Performance**: 10-100x improvement over JVM solutions
- **Predictable Latency**: No GC pauses, consistent P99/P999
- **Lower Costs**: Reduced cloud infrastructure spend
- **Memory Safety**: Entire classes of bugs eliminated at compile time
- **Excellent Async**: Tokio provides world-class async runtime
- **Strong Typing**: Protobuf + Rust type system catches errors early

### Negative

- **Learning Curve**: Rust is harder to learn than Java/Go
  - *Mitigation*: Invest in team training; create internal patterns/libraries
- **Slower Initial Development**: Borrow checker and type system require more upfront thought
  - *Mitigation*: Speed increases dramatically after learning curve; fewer runtime bugs compensate
- **Smaller Talent Pool**: Fewer Rust engineers than Java engineers
  - *Mitigation*: Rust community is growing rapidly; quality over quantity

### Neutral

- **Compilation Times**: Slower than Go, faster than C++
- **Ecosystem Maturity**: Rapidly improving; most needs met but some gaps exist

## Implementation Notes

### Key Crates

```toml
[dependencies]
# Async runtime
tokio = { version = "1.35", features = ["full"] }

# gRPC server/client
tonic = "0.10"
prost = "0.12"  # Protobuf

# HTTP server
axum = "0.7"

# Service composition
tower = "0.4"

# Database clients
sqlx = { version = "0.7", features = ["postgres", "sqlite", "runtime-tokio"] }
rdkafka = "0.35"  # Kafka
async-nats = "0.33"  # NATS

# Observability
tracing = "0.1"
tracing-subscriber = "0.3"
opentelemetry = "0.21"

# Serialization
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"
```

### Architecture Pattern

Use the **Tower** service pattern for composability:

```rust
use tower::{Service, ServiceBuilder, Layer};

// Build middleware stack
let service = ServiceBuilder::new()
    .layer(AuthLayer::new())           // mTLS auth
    .layer(RateLimitLayer::new(10000)) // Rate limiting
    .layer(LoggingLayer::new())        // Structured logging
    .layer(MetricsLayer::new())        // Prometheus metrics
    .service(ProxyService::new());     // Core proxy logic
```

### Performance Tips

1. **Use `tokio::spawn` judiciously**: Each task has overhead
2. **Pool connections**: Reuse connections to backends
3. **Avoid cloning large data**: Use `Arc` for shared read-only data
4. **Profile regularly**: Use `cargo flamegraph` to find hotspots
5. **Benchmark changes**: Use `criterion` for micro-benchmarks

## References

- [Rust Async Book](https://rust-lang.github.io/async-book/)
- [Tokio Tutorial](https://tokio.rs/tokio/tutorial)
- [Tonic gRPC](https://github.com/hyperium/tonic)
- [Tower Services](https://github.com/tower-rs/tower)
- Netflix Data Gateway (docs/netflix/)

## Revision History

- 2025-10-05: Initial draft and acceptance
