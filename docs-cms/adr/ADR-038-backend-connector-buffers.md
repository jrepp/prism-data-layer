---
title: "ADR-038: Backend Connector Buffer Architecture"
status: Proposed
date: 2025-10-08
deciders: [System]
tags: ['architecture', 'performance', 'backend', 'isolation', 'scalability']
---

## Context

Backend connector logic (connection pooling, query buffering, batching, retries) currently lives inside backend plugins (RFC-008). As Prism scales, we face challenges:

### Current State: Connector Logic in Plugins

```rust
// PostgreSQL plugin manages its own connection pool and buffering
pub struct PostgresPlugin {
    pool: deadpool_postgres::Pool,  // Connection pool
    query_buffer: VecDeque<Query>,  // Buffering for batch execution
    retry_queue: RetryQueue,         // Retry logic
}
```

**Problems**:
1. **Resource Contention**: Plugin connection pools compete for database connections
2. **No Centralized Control**: Can't enforce global limits (e.g., max 500 connections to Postgres cluster)
3. **Plugin Complexity**: Each plugin reimplements buffering, retries, batching
4. **Scaling Challenges**: Can't scale connector independently of plugin business logic
5. **Observability Gaps**: Connection metrics scattered across plugins

### Real-World Scenario (Netflix Scale)

From Netflix metrics:
- **8M QPS** across key-value abstraction
- **Multiple backends**: Cassandra, EVCache, PostgreSQL
- **Problem**: Without centralized connector management, connection storms during traffic spikes overwhelm databases

**Specific issues**:
- Connection pool exhaustion when one plugin has traffic spike
- No global rate limiting (each plugin rate-limits independently)
- Difficult to implement backend-wide circuit breaking

## Decision

**Extract backend connector logic into separate, scalable "connector buffer" processes**.

### Architecture

┌────────────────────────────────────────────────────────────────┐
│                        Prism Proxy (Rust)                       │
│                                                                 │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐ │
│  │ PostgreSQL   │      │    Redis     │      │    Kafka     │ │
│  │   Plugin     │      │   Plugin     │      │   Plugin     │ │
│  │  (Business   │      │  (Business   │      │  (Business   │ │
│  │   Logic)     │      │   Logic)     │      │   Logic)     │ │
│  └──────┬───────┘      └──────┬───────┘      └──────┬───────┘ │
│         │                     │                     │         │
│         │ gRPC                │ gRPC                │ gRPC    │
│         ▼                     ▼                     ▼         │
└─────────┼─────────────────────┼─────────────────────┼─────────┘
          │                     │                     │
          │                     │                     │
┌─────────▼─────────┐  ┌────────▼────────┐  ┌────────▼────────┐
│  PostgreSQL       │  │  Redis          │  │  Kafka          │
│  Connector        │  │  Connector      │  │  Connector      │
│  Buffer           │  │  Buffer         │  │  Buffer         │
│  (Go Process)     │  │  (Go Process)   │  │  (Go Process)   │
│                   │  │                 │  │                 │
│  - Conn Pool      │  │  - Conn Pool    │  │  - Producer     │
│  - Batching       │  │  - Pipelining   │  │    Pool         │
│  - Retries        │  │  - Clustering   │  │  - Batching     │
│  - Rate Limiting  │  │  - Failover     │  │  - Partitioning │
│  - Circuit Break  │  │                 │  │  - Compression  │
└─────────┬─────────┘  └────────┬────────┘  └────────┬────────┘
          │                     │                     │
          ▼                     ▼                     ▼
   ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
   │  PostgreSQL  │      │    Redis     │      │    Kafka     │
   │   Cluster    │      │   Cluster    │      │   Cluster    │
   └──────────────┘      └──────────────┘      └──────────────┘
```text

### Connector Buffer Responsibilities

| Responsibility | Description | Example |
|----------------|-------------|---------|
| **Connection Pooling** | Manage connections to backend, enforce limits | 500 connections max to Postgres |
| **Request Batching** | Combine multiple requests into batches | MGET in Redis, batch INSERT in Postgres |
| **Buffering** | Queue requests during transient failures | Buffer 10K requests during 5s outage |
| **Retries** | Retry failed requests with backoff | Exponential backoff: 100ms, 200ms, 400ms |
| **Rate Limiting** | Enforce backend-wide rate limits | Max 10K QPS to ClickHouse |
| **Circuit Breaking** | Stop requests to unhealthy backend | Open circuit after 10 consecutive failures |
| **Load Balancing** | Distribute requests across backend instances | Round-robin across 5 Postgres replicas |
| **Health Checks** | Monitor backend health | TCP ping every 10s |

### Plugin Simplification

**Before** (plugin does everything):
```
impl PostgresPlugin {
    async fn execute(&self, req: ExecuteRequest) -> Result<ExecuteResponse> {
        // Plugin manages:
        // - Connection from pool
        // - Batching logic
        // - Retry on failure
        // - Metrics recording
        let conn = self.pool.get().await?;
        let result = self.execute_with_retry(conn, req).await?;
        self.record_metrics(&result);
        Ok(result)
    }
}
```text

**After** (plugin delegates to connector):
```
impl PostgresPlugin {
    async fn execute(&self, req: ExecuteRequest) -> Result<ExecuteResponse> {
        // Plugin just translates request → connector format
        let connector_req = self.to_connector_request(req);

        // Connector handles pooling, batching, retries
        let response = self.connector_client.execute(connector_req).await?;

        Ok(self.to_plugin_response(response))
    }
}
```text

## Rationale

### Why Separate Connectors?

**1. Independent Scaling**

Compute-heavy backends (ClickHouse aggregations, graph queries) need more CPU than connection management:

```
# Scale plugin for compute (Rust)
apiVersion: v1
kind: Deployment
metadata:
  name: clickhouse-plugin
spec:
  replicas: 10  # Many instances for parallel aggregation
  resources:
    cpu: "4"    # CPU-heavy

---
# Scale connector for connections (Go)
apiVersion: v1
kind: Deployment
metadata:
  name: clickhouse-connector
spec:
  replicas: 2   # Few instances, each holds many connections
  resources:
    cpu: "1"
    connections: 500  # Many connections per instance
```text

**2. Language Choice**

- **Plugin (Rust)**: Business logic, high performance, type safety
- **Connector (Go)**: Excellent database client libraries, mature connection pooling

**Go advantages for connectors**:
- `database/sql`: Standard connection pooling
- Mature libraries: pgx (PostgreSQL), go-redis, sarama (Kafka)
- Built-in connection management, health checks

**3. Failure Isolation**

Plugin crash doesn't lose connections:

Plugin crashes → Connector keeps connections alive → New plugin instance reconnects to connector
```

Without separation:
Plugin crashes → All connections lost → Reconnect storm to database
```text

**4. Global Resource Management**

```
// Connector enforces global limits across all plugin instances
type PostgresConnector struct {
    globalPool *pgxpool.Pool  // Max 500 connections globally

    // All plugin instances share this pool
    rateLimiter rate.Limiter  // Max 10K QPS globally
}
```text

### Why Go for Connectors?

| Aspect | Rust | Go |
|--------|------|-----|
| **DB Libraries** | Emerging (tokio-postgres, redis-rs) | Mature (pgx, go-redis, mongo-driver) |
| **Connection Pooling** | Manual (deadpool, r2d2) | Built-in (database/sql) |
| **Goroutines** | Async tasks (tokio) | Native green threads |
| **Ecosystem** | Growing | Established for databases |
| **Development Speed** | Slower (lifetimes, trait complexity) | Faster (simple concurrency) |

**Decision**: **Go for connectors, Rust for plugins**

## Alternatives Considered

### 1. Keep Connectors in Plugins (Current State)

- **Pros**: Simpler architecture, no extra processes
- **Cons**: Can't scale independently, resource contention, plugin complexity
- **Rejected because**: Doesn't scale operationally at Netflix-level traffic

### 2. Shared Rust Library for Connection Logic

- **Pros**: Type safety, no IPC overhead
- **Cons**: Still tightly coupled to plugin lifecycle, no independent scaling
- **Rejected because**: Doesn't solve scaling and isolation problems

### 3. Connector as Proxy Responsibility

- **Pros**: Centralized in proxy
- **Cons**: Proxy becomes bloated, can't scale connectors independently, language mismatch
- **Rejected because**: Violates separation of concerns (RFC-008)

## Consequences

### Positive

- **Independent Scaling**: Scale connectors for connection management, plugins for business logic
- **Simplified Plugins**: Plugins focus on business logic (query translation, caching strategies)
- **Global Resource Control**: Enforce limits across all instances (connections, rate limits)
- **Better Isolation**: Connector failure doesn't affect plugin, plugin failure doesn't lose connections
- **Language Optimization**: Use Go for connector (best DB libraries), Rust for plugin (best performance)

### Negative

- **Additional Processes**: More operational complexity (monitor, deploy, scale connectors)
- **IPC Latency**: gRPC call from plugin → connector adds ~0.5-1ms
- **State Synchronization**: Connector and plugin must agree on connection state

### Neutral

- **Deployment Complexity**: Must deploy connector alongside plugin (sidecar or separate pod)
- **Observability**: Need metrics from both plugin and connector
- **Configuration**: Connector needs separate config (pool size, timeouts, etc.)

## Implementation Notes

### Connector gRPC API

```
syntax = "proto3";

package prism.connector;

service BackendConnector {
  // Execute single operation
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);

  // Execute batch (connector handles batching)
  rpc ExecuteBatch(stream ExecuteRequest) returns (stream ExecuteResponse);

  // Health check
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);

  // Connection pool stats
  rpc GetStats(GetStatsRequest) returns (ConnectorStats);
}

message ExecuteRequest {
  string operation = 1;  // "query", "insert", "get", etc.
  bytes params = 2;      // Backend-specific parameters
  map<string, string> metadata = 3;
}

message ExecuteResponse {
  bool success = 1;
  bytes result = 2;
  string error = 3;
  ConnectorMetrics metrics = 4;
}

message ConnectorStats {
  int64 active_connections = 1;
  int64 idle_connections = 2;
  int64 total_requests = 3;
  int64 queued_requests = 4;
  double avg_latency_ms = 5;
}
```text

### PostgreSQL Connector Example (Go)

```
package main

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
    pb "prism/proto/connector"
)

type PostgresConnector struct {
    pool *pgxpool.Pool
    rateLimiter *rate.Limiter
    circuitBreaker *CircuitBreaker
}

func (c *PostgresConnector) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
    // 1. Rate limiting
    if err := c.rateLimiter.Wait(ctx); err != nil {
        return nil, err
    }

    // 2. Circuit breaker check
    if !c.circuitBreaker.Allow() {
        return nil, ErrCircuitOpen
    }

    // 3. Get connection from pool
    conn, err := c.pool.Acquire(ctx)
    if err != nil {
        return nil, err
    }
    defer conn.Release()

    // 4. Execute query
    result, err := conn.Query(ctx, req.Query, req.Params...)
    if err != nil {
        c.circuitBreaker.RecordFailure()
        return nil, err
    }

    c.circuitBreaker.RecordSuccess()
    return &pb.ExecuteResponse{Success: true, Result: result}, nil
}
```text

### Deployment Topology

**Option 1: Sidecar Pattern** (Recommended for Kubernetes):
```
apiVersion: v1
kind: Pod
metadata:
  name: prism-playback-0
spec:
  containers:
  - name: prism-proxy
    image: prism/proxy:latest

  - name: postgres-connector
    image: prism/postgres-connector:latest
    env:
    - name: POSTGRES_URL
      value: "postgres://db:5432"
    - name: POOL_SIZE
      value: "100"

  - name: postgres-plugin
    image: prism/postgres-plugin:latest
    env:
    - name: CONNECTOR_ENDPOINT
      value: "localhost:50200"  # Talk to sidecar connector
```text

**Option 2: Shared Connector Pool** (For bare metal):
┌─────────────────────┐
│  Prism Instance 1   │──┐
└─────────────────────┘  │
                          ├─► Shared Postgres Connector (500 connections)
┌─────────────────────┐  │       ↓
│  Prism Instance 2   │──┘   PostgreSQL Cluster
└─────────────────────┘
```

## References

- RFC-008: Proxy Plugin Architecture (plugins delegate to connectors)
- ADR-035: Connection Pooling (connector implements pooling)
- [pgx Connection Pool](https://github.com/jackc/pgx)
- [go-redis Clustering](https://github.com/redis/go-redis)
- [Sarama Kafka Client](https://github.com/IBM/sarama)
- [Netflix Hystrix](https://github.com/Netflix/Hystrix) (circuit breaker pattern)

## Revision History

- 2025-10-08: Initial draft proposing Go-based connector buffers
