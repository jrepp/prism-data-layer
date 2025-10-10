---
title: "ADR-035: Database Connection Pooling vs Direct Connections"
status: Proposed
date: 2025-10-08
deciders: [System]
tags: ['performance', 'backend', 'reliability', 'architecture']
---

## Context

Prism's backend plugins need to connect to data stores (PostgreSQL, Redis, ClickHouse, etc.). Each plugin must decide: **use connection pooling or direct connections per request**?

### The Tradeoff

**Connection Pooling**:
- Pre-established connections reused across requests
- Lower latency (no TCP handshake + auth per request)
- Fixed resource usage (pool size limits)
- Complexity: pool management, health checks, stale connection handling

**Direct Connections**:
- New connection per request
- Higher latency (TCP + TLS + auth overhead: ~5-50ms)
- Unbounded resource usage (connections scale with request rate)
- Simplicity: no pool management needed

### Why This Matters at Scale

From Netflix's experience at 8M QPS:
- **Connection churn** kills performance at scale
- PostgreSQL max_connections: typically 100-200 (too low for high concurrency)
- Redis benefits from persistent connections (pipelining, reduced latency)
- But: connection pools can become bottlenecks if undersized

## Decision

**Use connection pooling by default for all backends**, with backend-specific tuning:

### Connection Pool Strategy Matrix

| Backend | Pool Type | Pool Size Formula | Rationale |
|---------|-----------|-------------------|-----------|
| **PostgreSQL** | Shared pool per namespace | `max(10, RPS / 100)` | Expensive connections, limited max_connections |
| **Redis** | Shared pool per namespace | `max(5, RPS / 1000)` | Cheap connections, benefits from pipelining |
| **Kafka** | Producer pool | `max(3, num_partitions / 10)` | Producers are heavyweight, batching preferred |
| **ClickHouse** | Shared pool per namespace | `max(5, RPS / 200)` | Query-heavy, benefits from persistent HTTP/2 |
| **NATS** | Single persistent connection | `1` per namespace | Multiplexing over single connection |
| **Object Storage (S3/MinIO)** | No pool (HTTP client reuse) | N/A | HTTP client handles pooling internally |

### Pool Configuration

```yaml
# Per-backend pool settings
backends:
  postgres:
    pool:
      min_size: 10
      max_size: 100
      idle_timeout: 300s  # Close idle connections after 5 min
      max_lifetime: 1800s  # Recycle connections after 30 min
      connection_timeout: 5s
      health_check_interval: 30s

  redis:
    pool:
      min_size: 5
      max_size: 50
      idle_timeout: 600s  # Redis connections are cheap to keep alive
      max_lifetime: 3600s
      connection_timeout: 2s
      health_check_interval: 60s
```

## Rationale

### PostgreSQL: Pool Essentials

**Why pool?**
- PostgreSQL connection cost: ~10-20ms (TCP + TLS + auth)
- At 1000 RPS → 10-20 seconds of CPU wasted per second (unsustainable)
- PostgreSQL's `max_connections` limit (often 100-200) too low for direct per-request

**Sizing**:
- Rule of thumb: `pool_size = (total_requests_per_second * avg_query_duration) / num_proxy_instances`
- Example: 5000 RPS * 0.005s avg query / 5 instances = 5 connections per instance
- Add buffer for spikes: 5 * 2 = 10 connections

**Gotchas**:
- Postgres transaction state: ensure proper `BEGIN/COMMIT` handling
- Connection reuse: always `ROLLBACK` on error to clean state
- Prepared statements: cache per connection for efficiency

### Redis: Pool for Pipelining

**Why pool?**
- Redis connection cost: ~1-2ms (cheap, but adds up)
- Pipelining benefits: batch multiple commands over single connection
- At 10K RPS → 1 connection can handle 100K RPS with pipelining

**Sizing**:
- Much smaller pools than PostgreSQL (Redis is single-threaded per instance)
- More connections don't help unless sharding across Redis instances
- Example: 50K RPS → 5-10 connections sufficient

### Kafka: Producer Pooling

**Why pool producers?**
- KafkaProducer is heavyweight (metadata fetching, batching logic)
- Creating per-request is extremely inefficient
- One producer can handle 10K+ messages/sec

**Sizing**:
- Typically 1-3 producers per partition
- Example: 10 partitions → 3 producers (round-robin)

**Key insight**: Kafka producers do internal batching, so pooling amplifies efficiency.

### NATS: Single Connection

**Why not pool?**
- NATS protocol supports multiplexing over single connection
- Creating multiple connections adds no benefit (and wastes resources)
- NATS client libraries handle this internally

**Configuration**:
```rust
// Single NATS connection per namespace
let nats_client = nats::connect(&config.connection_string).await?;
// All requests multiplex over this connection
```

### Object Storage: Client-Level Pooling

**Why not explicit pool?**
- HTTP clients (reqwest, hyper) handle connection pooling internally
- S3 API is stateless, no transaction semantics
- Client library's default pooling is usually optimal

**Configuration**:
```rust
// HTTP client with built-in connection pool
let s3_client = aws_sdk_s3::Client::from_conf(
    aws_sdk_s3::config::Builder::new()
        .http_client(
            aws_smithy_runtime::client::http::hyper_014::HyperClientBuilder::new()
                .build_https()  // Uses hyper's connection pool
        )
        .build()
);
```

## Alternatives Considered

### 1. Direct Connections (No Pooling)

```rust
// Create new connection per request
pub async fn execute(&self, req: ExecuteRequest) -> Result<ExecuteResponse> {
    let conn = PostgresConnection::connect(&self.config).await?;  // 10-20ms overhead!
    let result = conn.query(&req.query).await?;
    Ok(result)
}
```

- **Pros**: Simple, no pool management, no connection reuse bugs
- **Cons**: Terrible performance (10-20ms overhead per request), exhausts `max_connections`
- **Rejected because**: Unsustainable at any meaningful scale

### 2. Thread-Local Connections

- **Pros**: No contention, one connection per thread
- **Cons**: Doesn't work with async (threads != tasks), wastes connections
- **Rejected because**: Incompatible with tokio async model

### 3. Per-Request Connection with Caching

- **Pros**: Automatic pooling via LRU cache
- **Cons**: Complex TTL management, unclear ownership, health check challenges
- **Rejected because**: Reinventing connection pooling poorly

## Consequences

### Positive

- **10-100x latency improvement** vs direct connections (no TCP handshake per request)
- **Resource efficiency**: Fixed connection count prevents database overload
- **Predictable performance**: Pool size controls max concurrency
- **Backend protection**: Prevents stampeding herd from overwhelming databases

### Negative

- **Stale connections**: Need health checks and recycling
- **Pool exhaustion**: If pool too small, requests queue (but better than overwhelming DB)
- **Complex configuration**: Need to tune pool sizes per workload
- **Connection state**: Must ensure clean state between reuses (transactions, temp tables)

### Neutral

- **Warm-up time**: Pools need to fill on startup (min_size connections created)
- **Monitoring**: Need metrics on pool utilization, wait times, health check failures
- **Graceful shutdown**: Must drain pools cleanly on shutdown

## Implementation Notes

### PostgreSQL Pool Implementation (Using deadpool-postgres)

```rust
use deadpool_postgres::{Config, Pool, Runtime};

pub struct PostgresPlugin {
    pool: Pool,
}

impl PostgresPlugin {
    pub async fn new(config: PostgresConfig) -> Result<Self> {
        let mut cfg = Config::new();
        cfg.url = Some(config.connection_string);
        cfg.pool = Some(deadpool::managed::PoolConfig {
            max_size: config.pool_max_size,
            timeouts: deadpool::managed::Timeouts {
                wait: Some(Duration::from_secs(5)),
                create: Some(Duration::from_secs(5)),
                recycle: Some(Duration::from_secs(1)),
            },
        });

        let pool = cfg.create_pool(Some(Runtime::Tokio1))?;

        Ok(Self { pool })
    }

    pub async fn execute(&self, req: ExecuteRequest) -> Result<ExecuteResponse> {
        // Get connection from pool (blocks if pool exhausted)
        let conn = self.pool.get().await?;

        // Execute query
        let rows = conn.query(&req.query, &req.params).await?;

        // Connection automatically returned to pool when dropped
        Ok(ExecuteResponse::from_rows(rows))
    }
}
```

### Health Checks

```rust
// Periodic health check removes stale connections
async fn health_check_loop(pool: Pool) {
    let mut interval = tokio::time::interval(Duration::from_secs(30));

    loop {
        interval.tick().await;

        // Test a connection
        match pool.get().await {
            Ok(conn) => {
                if let Err(e) = conn.simple_query("SELECT 1").await {
                    warn!("Pool health check failed: {}", e);
                    // Pool will recreate connection on next checkout
                }
            }
            Err(e) => {
                error!("Failed to get connection for health check: {}", e);
            }
        }
    }
}
```

### Pool Metrics

```rust
// Expose pool metrics to Prometheus
pub fn record_pool_metrics(pool: &Pool, namespace: &str, backend: &str) {
    let status = pool.status();

    metrics::gauge!("prism_pool_size", status.size as f64,
        "namespace" => namespace, "backend" => backend);

    metrics::gauge!("prism_pool_available", status.available as f64,
        "namespace" => namespace, "backend" => backend);

    metrics::gauge!("prism_pool_waiting", status.waiting as f64,
        "namespace" => namespace, "backend" => backend);
}
```

### Configuration Tuning Guidance

**Start with conservative sizes**:
pool_size = max(min_size, expected_p99_rps * p99_query_latency_seconds)
```text

**Example**:
- Expected P99 RPS: 1000
- P99 query latency: 50ms = 0.05s
- Pool size: max(10, 1000 * 0.05) = 50 connections

**Monitor and adjust**:
- If `pool_waiting` metric > 0: pool too small, increase size
- If `pool_available` always ~= `pool_size`: pool too large, decrease size
- If connection errors spike: check database `max_connections` limit

## References

- [PostgreSQL Connection Pooling Best Practices](https://www.postgresql.org/docs/current/runtime-config-connection.html)
- [Redis Pipelining](https://redis.io/docs/manual/pipelining/)
- [HikariCP (Java) Connection Pool Sizing](https://github.com/brettwooldridge/HikariCP/wiki/About-Pool-Sizing)
- RFC-008: Proxy Plugin Architecture (where pools live)
- [deadpool-postgres Documentation](https://docs.rs/deadpool-postgres/)

## Revision History

- 2025-10-08: Initial draft with backend-specific pooling strategies

```