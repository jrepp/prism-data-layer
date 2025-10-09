---
title: "MEMO-004: Backend Plugin Implementation Guide"
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [backends, plugins, implementation, testing, go]
---

# MEMO-004: Backend Plugin Implementation Guide

## Purpose

Strategic guide for implementing backend plugins in priority order, with analysis of Go SDK support, data model complexity, testing difficulty, and recommended demo configurations for the acceptance test framework (RFC-015).

## Backend Implementability Matrix

Comprehensive comparison of all backends discussed for Prism, ranked by ease of implementation.

### Comparison Table

| Rank | Backend | Go SDK Quality | Data Models | Test Difficulty | Protocol Complexity | Implementability Score |
|------|---------|---------------|-------------|-----------------|-------------------|----------------------|
| 1 | **Redis** | ⭐⭐⭐⭐⭐ Excellent (go-redis) | KeyValue, Cache | ⭐⭐⭐⭐⭐ Easy (testcontainers) | ⭐⭐⭐⭐⭐ Simple (RESP) | **95/100** |
| 2 | **PostgreSQL** | ⭐⭐⭐⭐⭐ Excellent (pgx, pq) | Relational, JSON | ⭐⭐⭐⭐⭐ Easy (testcontainers) | ⭐⭐⭐⭐ Moderate (SQL) | **93/100** |
| 3 | **SQLite** | ⭐⭐⭐⭐⭐ Excellent (mattn/go-sqlite3) | Relational | ⭐⭐⭐⭐⭐ Trivial (embedded) | ⭐⭐⭐⭐⭐ Simple (SQL) | **92/100** |
| 4 | **NATS** | ⭐⭐⭐⭐⭐ Excellent (nats.go - official) | PubSub, Queue | ⭐⭐⭐⭐⭐ Easy (lightweight) | ⭐⭐⭐⭐⭐ Simple (text protocol) | **90/100** |
| 5 | **Kafka** | ⭐⭐⭐⭐ Good (segmentio/kafka-go, confluent-kafka-go) | Event Streaming | ⭐⭐⭐⭐ Moderate (testcontainers, slow startup) | ⭐⭐⭐ Complex (wire protocol) | **78/100** |
| 6 | **S3/MinIO** | ⭐⭐⭐⭐⭐ Excellent (aws-sdk-go-v2, minio-go) | Object Storage | ⭐⭐⭐⭐ Moderate (MinIO for local) | ⭐⭐⭐⭐ Simple (REST API) | **85/100** |
| 7 | **ClickHouse** | ⭐⭐⭐ Good (clickhouse-go) | Columnar/TimeSeries | ⭐⭐⭐ Moderate (testcontainers) | ⭐⭐⭐ Moderate (custom protocol) | **70/100** |
| 8 | **Neptune** | ⭐⭐ Fair (gremlin-go, AWS SDK) | Graph (Gremlin/SPARQL) | ⭐⭐ Hard (AWS-only, no local) | ⭐⭐ Complex (Gremlin) | **50/100** |

### Scoring Criteria

**Implementability Score** = weighted average of:
- **Go SDK Quality** (30%): Maturity, documentation, community support
- **Data Models** (15%): Complexity and variety of supported models
- **Test Difficulty** (25%): Local testing, testcontainers support, startup time
- **Protocol Complexity** (20%): Wire protocol complexity, client implementation difficulty
- **Community/Ecosystem** (10%): Available examples, Stack Overflow answers, production usage

## Detailed Backend Analysis

### 1. Redis (Score: 95/100) - Highest Priority

**Why Implement First:**
- **Simplest protocol**: RESP (REdis Serialization Protocol) is text-based and trivial to implement
- **Fastest to test**: Starts in &lt;1 second, minimal memory footprint
- **Perfect for demos**: In-memory, no persistence needed for basic examples
- **Excellent Go SDK**: `go-redis/redis` is mature, well-documented, idiomatic Go

**Go SDK:**
```go
import "github.com/redis/go-redis/v9"

client := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
```

**Data Models Supported:**
- KeyValue (primary use case)
- Cache (TTL support)
- PubSub (lightweight messaging)
- Lists, Sets, Sorted Sets

**Testing Strategy:**
```go
// testcontainers integration
func NewRedisInstance(t *testing.T) *RedisInstance {
    req := testcontainers.ContainerRequest{
        Image:        "redis:7-alpine",
        ExposedPorts: []string{"6379/tcp"},
        WaitingFor:   wait.ForLog("Ready to accept connections"),
    }
    // Starts in &lt;1 second
}
```

**Demo Plugin Operations:**
- `GET`, `SET`, `DEL` (basic operations)
- `EXPIRE`, `TTL` (cache semantics)
- `PUBLISH`, `SUBSCRIBE` (pub/sub pattern)

**RFC-015 Test Coverage:**
- Authentication: `AUTH` command with password
- Connection pooling: Verify multiple connections
- Error handling: Wrong key types, expired keys
- Concurrency: 1000s of concurrent ops

---

### 2. PostgreSQL (Score: 93/100) - Production Ready

**Why Implement Second:**
- **Industry standard**: Most developers understand SQL
- **Strong Go ecosystem**: `pgx` is the gold standard for Postgres Go clients
- **Rich testing**: testcontainers, postgres:alpine images
- **Complex data models**: Supports JSON, arrays, full-text search

**Go SDK:**
```go
import "github.com/jackc/pgx/v5"

conn, _ := pgx.Connect(context.Background(), "postgres://user:pass@localhost:5432/db")
```

**Data Models Supported:**
- Relational (tables, foreign keys, transactions)
- JSON/JSONB (document-like queries)
- Full-text search
- Time-series (with extensions like TimescaleDB)

**Testing Strategy:**
```go
func NewPostgresInstance(t *testing.T) *PostgresInstance {
    req := testcontainers.ContainerRequest{
        Image:        "postgres:16-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_PASSWORD": "testpass",
        },
        WaitingFor: wait.ForLog("database system is ready to accept connections"),
    }
    // Starts in 3-5 seconds
}
```

**Demo Plugin Operations:**
- `SELECT`, `INSERT`, `UPDATE`, `DELETE`
- `BEGIN`, `COMMIT`, `ROLLBACK` (transactions)
- `LISTEN`, `NOTIFY` (pub/sub via Postgres)
- Prepared statements for performance

**RFC-015 Test Coverage:**
- Authentication: Username/password, SSL/TLS
- Transaction isolation levels
- Constraint violations (foreign keys, unique)
- JSON operations and indexing
- Connection pool exhaustion

---

### 3. SQLite (Score: 92/100) - Perfect for Demos

**Why Implement Third:**
- **Zero configuration**: Embedded, no separate process
- **Instant startup**: No container needed
- **Perfect for CI/CD**: Fast, deterministic tests
- **Same SQL as Postgres**: Easy to understand

**Go SDK:**
```go
import "github.com/mattn/go-sqlite3"

db, _ := sql.Open("sqlite3", ":memory:") // In-memory DB
```

**Data Models Supported:**
- Relational (full SQL support)
- JSON1 extension for JSON queries
- Full-text search (FTS5)

**Testing Strategy:**
```go
func NewSQLiteInstance(t *testing.T) *SQLiteInstance {
    // No container needed!
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatal(err)
    }

    // Create schema immediately
    db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")

    return &SQLiteInstance{db: db}
}
```

**Demo Plugin Operations:**
- All standard SQL operations
- In-memory for speed, file-backed for persistence
- WAL mode for concurrent reads

**RFC-015 Test Coverage:**
- Authentication: N/A (file-based permissions)
- Concurrency: Multiple readers, single writer
- Error handling: Locked database, constraint violations

**Use Cases:**
- Local development without Docker
- CI/CD where container startup overhead matters
- Embedded demos (single binary with DB)

---

### 4. NATS (Score: 90/100) - Cloud-Native Messaging

**Why Implement Fourth:**
- **Go-native**: Written in Go, official Go client
- **Lightweight**: &lt;10MB memory, starts instantly
- **Modern patterns**: Request-reply, streams, key-value (JetStream)
- **Simple protocol**: Text-based, easy to debug

**Go SDK:**
```go
import "github.com/nats-io/nats.go"

nc, _ := nats.Connect("nats://localhost:4222")
```

**Data Models Supported:**
- PubSub (core NATS)
- Queue groups (load balancing)
- JetStream (persistent streams, like Kafka-lite)
- Key-Value store (JetStream KV)

**Testing Strategy:**
```go
func NewNATSInstance(t *testing.T) *NATSInstance {
    // Option 1: Embedded NATS server (no container!)
    s, err := server.NewServer(&server.Options{
        Port: -1, // Random port
    })
    s.Start()

    // Option 2: Container for full features
    req := testcontainers.ContainerRequest{
        Image: "nats:2-alpine",
        ExposedPorts: []string{"4222/tcp"},
    }
    // Starts in &lt;2 seconds
}
```

**Demo Plugin Operations:**
- `Publish`, `Subscribe` (pub/sub)
- `Request`, `Reply` (RPC pattern)
- `QueueSubscribe` (load balancing)
- JetStream: `AddStream`, `Publish`, `Subscribe` with ack

**RFC-015 Test Coverage:**
- Authentication: Token, username/password, TLS certs
- Connection resilience: Automatic reconnect
- Consumer acknowledgments
- Exactly-once delivery (JetStream)

---

### 5. Kafka (Score: 78/100) - Production Event Streaming

**Why Implement Fifth:**
- **Industry standard**: De facto event streaming platform
- **Complex but mature**: Well-understood patterns
- **Good Go SDKs**: `segmentio/kafka-go` (pure Go) or `confluent-kafka-go` (C bindings)
- **Testable**: testcontainers support, but slow startup

**Go SDK:**
```go
// Option 1: segmentio/kafka-go (pure Go)
import "github.com/segmentio/kafka-go"

writer := &kafka.Writer{
    Addr:  kafka.TCP("localhost:9092"),
    Topic: "events",
}

// Option 2: confluent-kafka-go (faster, C deps)
import "github.com/confluentinc/confluent-kafka-go/v2/kafka"

producer, _ := kafka.NewProducer(&kafka.ConfigMap{
    "bootstrap.servers": "localhost:9092",
})
```

**Data Models Supported:**
- Event streaming (append-only log)
- Partitioned queues
- Change data capture (Kafka Connect)
- Stream processing (Kafka Streams)

**Testing Strategy:**
```go
func NewKafkaInstance(t *testing.T) *KafkaInstance {
    req := testcontainers.ContainerRequest{
        Image:        "confluentinc/cp-kafka:7.5.0",
        ExposedPorts: []string{"9092/tcp", "9093/tcp"},
        Env: map[string]string{
            "KAFKA_BROKER_ID": "1",
            "KAFKA_ZOOKEEPER_CONNECT": "zookeeper:2181",
            // ... complex configuration
        },
        WaitingFor: wait.ForLog("started (kafka.server.KafkaServer)").
            WithStartupTimeout(120 * time.Second), // Slow!
    }
    // Starts in 30-60 seconds (needs Zookeeper or KRaft mode)
}
```

**Demo Plugin Operations:**
- `Produce` with key and value
- `Consume` with consumer group
- Offset management (commit, reset)
- Partition assignment

**RFC-015 Test Coverage:**
- Authentication: SASL/SCRAM, mTLS
- Consumer groups: Rebalancing, partition assignment
- Exactly-once semantics: Idempotent producer, transactional writes
- High throughput: 10k+ messages/sec

**Challenges:**
- Startup time: 30-60 seconds vs &lt;5 seconds for Redis/Postgres
- Configuration complexity: Many knobs to tune
- Testing: Requires Zookeeper (or KRaft mode in newer versions)

---

### 6. S3/MinIO (Score: 85/100) - Object Storage

**Why Implement Sixth:**
- **Standard API**: S3-compatible API used everywhere
- **MinIO for local**: Production-grade S3 alternative
- **Essential for patterns**: Claim Check pattern requires object storage
- **Excellent SDKs**: AWS SDK v2 and MinIO Go client

**Go SDK:**
```go
// AWS S3
import "github.com/aws/aws-sdk-go-v2/service/s3"

client := s3.NewFromConfig(cfg)

// MinIO (S3-compatible)
import "github.com/minio/minio-go/v7"

minioClient, _ := minio.New("localhost:9000", &minio.Options{
    Creds: credentials.NewStaticV4("minioadmin", "minioadmin", ""),
})
```

**Data Models Supported:**
- Object storage (key → blob)
- Metadata (key-value tags per object)
- Versioning (multiple versions of same key)
- Lifecycle policies (auto-archival)

**Testing Strategy:**
```go
func NewMinIOInstance(t *testing.T) *MinIOInstance {
    req := testcontainers.ContainerRequest{
        Image:        "minio/minio:latest",
        ExposedPorts: []string{"9000/tcp", "9001/tcp"},
        Cmd:          []string{"server", "/data", "--console-address", ":9001"},
        Env: map[string]string{
            "MINIO_ROOT_USER":     "minioadmin",
            "MINIO_ROOT_PASSWORD": "minioadmin",
        },
        WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000"),
    }
    // Starts in 3-5 seconds
}
```

**Demo Plugin Operations:**
- `PutObject`, `GetObject`, `DeleteObject`
- `ListObjects` with prefix
- Multipart upload (large files)
- Presigned URLs (temporary access)

**RFC-015 Test Coverage:**
- Authentication: Access key + secret key
- Large objects: Multipart upload, streaming
- Versioning: Multiple versions of same key
- Lifecycle: Expiration policies

**Use Cases:**
- Claim Check pattern (store large payloads)
- Tiered storage (archive cold data)
- Backup and recovery

---

### 7. ClickHouse (Score: 70/100) - Analytical Queries

**Why Implement Seventh:**
- **Specialized**: Columnar database for analytics
- **Fast for aggregations**: OLAP queries
- **Decent Go SDK**: `clickhouse-go` is maintained
- **Testable**: testcontainers support

**Go SDK:**
```go
import "github.com/ClickHouse/clickhouse-go/v2"

conn, _ := clickhouse.Open(&clickhouse.Options{
    Addr: []string{"localhost:9000"},
    Auth: clickhouse.Auth{
        Database: "default",
        Username: "default",
        Password: "",
    },
})
```

**Data Models Supported:**
- TimeSeries (high-cardinality metrics)
- Columnar (fast aggregations)
- Event logs (append-only)

**Testing Strategy:**
```go
func NewClickHouseInstance(t *testing.T) *ClickHouseInstance {
    req := testcontainers.ContainerRequest{
        Image:        "clickhouse/clickhouse-server:latest",
        ExposedPorts: []string{"9000/tcp", "8123/tcp"},
        WaitingFor: wait.ForLog("Ready for connections"),
    }
    // Starts in 5-10 seconds
}
```

**Demo Plugin Operations:**
- `INSERT` (batch inserts for performance)
- `SELECT` with aggregations (`SUM`, `AVG`, `percentile`)
- Time-based queries (`toStartOfHour`, `toDate`)

**RFC-015 Test Coverage:**
- Authentication: Username/password
- Batch inserts: 10k+ rows/sec
- Complex queries: Joins, aggregations
- Compression: Verify data compression

**Use Cases:**
- Metrics and observability
- Log aggregation
- Business intelligence

---

### 8. Neptune (Score: 50/100) - Graph Database (AWS)

**Why Implement Last:**
- **AWS-only**: No local testing without AWS account
- **Complex protocol**: Gremlin (graph traversal language)
- **Limited Go support**: `gremlin-go` is less mature
- **Expensive to test**: AWS charges, no free tier for Neptune

**Go SDK:**
```go
import "github.com/apache/tinkerpop/gremlin-go/v3/driver"

remote, _ := gremlingo.NewDriverRemoteConnection("ws://localhost:8182/gremlin")
g := gremlingo.Traversal_().WithRemote(remote)
```

**Data Models Supported:**
- Graph (vertices, edges, properties)
- Property graph model (Gremlin)
- RDF triples (SPARQL)

**Testing Strategy:**
```go
// Problem: No good local testing option
// Option 1: Mock Gremlin responses (not ideal)
// Option 2: Use TinkerPop Gremlin Server (complex setup)
// Option 3: Fake AWS Neptune with localstack (limited support)

func NewNeptuneInstance(t *testing.T) *NeptuneInstance {
    // Best option: Use Gremlin Server (JVM-based)
    req := testcontainers.ContainerRequest{
        Image:        "tinkerpop/gremlin-server:latest",
        ExposedPorts: []string{"8182/tcp"},
        WaitingFor: wait.ForLog("Channel started at port 8182"),
    }
    // Starts in 10-15 seconds (JVM startup)
}
```

**Demo Plugin Operations:**
- `AddVertex`, `AddEdge` (create graph elements)
- Graph traversals: `g.V().has('name', 'Alice').out('knows')`
- Path queries: Shortest path, neighbors

**RFC-015 Test Coverage:**
- Authentication: IAM-based (AWS SigV4)
- Graph traversals: Verify Gremlin queries
- Transactions: Graph mutations

**Challenges:**
- No free local testing
- Gremlin learning curve
- Limited Go ecosystem
- Difficult to seed test data

**Recommendation**: Defer until other backends are stable.

---

## Recommended Implementation Order

### Phase 1: Foundation (Weeks 1-4)

**Priority:** Redis → SQLite → PostgreSQL

**Rationale:**
- **Redis**: Simplest protocol, fastest tests, builds confidence
- **SQLite**: Zero dependencies, perfect for demos
- **PostgreSQL**: Production-ready relational backend

**Deliverables:**
- 3 fully tested plugins with RFC-015 test suites
- Reusable authentication suite validated
- testcontainers harness proven
- Demo showing pattern composition

### Phase 2: Messaging (Weeks 5-8)

**Priority:** NATS → Kafka

**Rationale:**
- **NATS**: Modern, lightweight, Go-native
- **Kafka**: Industry standard, validates complex protocols

**Deliverables:**
- PubSub and queue patterns working
- Consumer group testing
- Exactly-once semantics verified

### Phase 3: Storage (Weeks 9-10)

**Priority:** S3/MinIO

**Rationale:**
- Essential for Claim Check pattern
- Large payload handling

**Deliverables:**
- Object storage backend
- Claim Check pattern implementation
- Multipart upload support

### Phase 4: Analytics (Weeks 11-12)

**Priority:** ClickHouse

**Rationale:**
- Differentiation for observability use cases
- TimeSeries data model

**Deliverables:**
- Columnar storage backend
- Metrics aggregation

### Phase 5: Advanced (Future)

**Priority:** Neptune (deferred)

**Rationale:**
- Specialized use case
- Complex testing requirements
- Lower immediate value

---

## Demo Plugin Configurations

### Demo 1: Redis KeyValue Store

**Purpose**: Show simplest possible plugin

```yaml
# config/demo-redis.yaml
namespaces:
  - name: cache
    pattern: keyvalue

    needs:
      latency: &lt;1ms
      ttl: true

backend:
  type: redis
  host: localhost
  port: 6379
```

**Client Code:**
```go
// Demo: GET/SET operations
client.Set("user:123", userData, 300*time.Second) // 5 min TTL
value := client.Get("user:123")
```

**Test Focus:**
- Authentication (password)
- TTL expiration
- Connection pooling

---

### Demo 2: PostgreSQL with Transactions

**Purpose**: Show transactional reliability

```yaml
namespaces:
  - name: orders
    pattern: transactional-queue

    needs:
      consistency: strong
      durability: fsync

backend:
  type: postgres
  database: orders
```

**Client Code:**
```go
// Demo: Outbox pattern
tx := client.BeginTx()
tx.Execute("INSERT INTO orders (...) VALUES (...)")
tx.Publish("order-events", orderEvent)
tx.Commit() // Atomic
```

**Test Focus:**
- ACID transactions
- Outbox pattern verification
- Rollback behavior

---

### Demo 3: Kafka Event Streaming

**Purpose**: Show high-throughput messaging

```yaml
namespaces:
  - name: events
    pattern: event-stream

    needs:
      throughput: 10k rps
      retention: 7days
      ordered: true

backend:
  type: kafka
  brokers: [localhost:9092]
  partitions: 10
```

**Client Code:**
```go
// Demo: Producer
for i := 0; i < 10000; i++ {
    client.Publish("events", event)
}

// Demo: Consumer with consumer group
for event := range client.Subscribe("events", "group1") {
    process(event)
    event.Ack()
}
```

**Test Focus:**
- Consumer groups
- Partitioning
- Offset management

---

### Demo 4: S3 Large Payload (Claim Check)

**Purpose**: Show pattern composition

```yaml
namespaces:
  - name: videos
    pattern: pubsub

    needs:
      max_message_size: 5GB
      storage_backend: s3

backend:
  type: kafka  # For metadata
  storage:
    type: s3
    bucket: prism-claim-checks
```

**Client Code:**
```go
// Demo: Transparent large payload handling
video := loadVideo("movie.mp4") // 2GB
client.Publish("videos", video)  // Prism stores in S3 automatically

// Consumer gets full video
event := client.Subscribe("videos")
video := event.Payload  // Prism fetches from S3 transparently
```

**Test Focus:**
- Claim Check pattern
- S3 upload/download
- Cleanup after consumption

---

### Demo 5: Multi-Backend Composition

**Purpose**: Show layered architecture power

```yaml
namespaces:
  - name: ml-models
    pattern: pubsub

    needs:
      consistency: strong       # → Outbox (Postgres)
      max_message_size: 5GB     # → Claim Check (S3)
      durability: strong        # → WAL
      retention: 30days         # → Tiered Storage

backends:
  transactional: postgres
  storage: s3
  queue: kafka
```

**Client Code:**
```go
// Demo: All patterns composed automatically
with client.transaction() as tx:
    tx.execute("INSERT INTO model_registry ...")
    tx.publish("model-releases", model_weights)  // 2GB
    tx.commit()

// Prism automatically:
// 1. Writes to WAL (durability)
// 2. Stores model in S3 (claim check)
// 3. Inserts to Postgres outbox (transactional)
// 4. Publishes to Kafka (queue)
```

**Test Focus:**
- Pattern composition
- End-to-end flow
- Failure recovery

---

## Testing Infrastructure Requirements

### Docker Compose for Local Testing

```yaml
# docker-compose.test.yml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: testpass
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD", "pg_isready"]

  nats:
    image: nats:2-alpine
    ports:
      - "4222:4222"
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8222/healthz"]

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092
    ports:
      - "9092:9092"
    healthcheck:
      test: ["CMD", "kafka-broker-api-versions", "--bootstrap-server", "localhost:9092"]

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports:
      - "9000:9000"
      - "9001:9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]

  clickhouse:
    image: clickhouse/clickhouse-server:latest
    ports:
      - "9000:9000"
      - "8123:8123"
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8123/ping"]
```

**Usage:**
```bash
# Start all backends
docker-compose -f docker-compose.test.yml up -d

# Run acceptance tests
go test ./tests/acceptance/... -v

# Stop all backends
docker-compose -f docker-compose.test.yml down
```

---

## Appendix: Go SDK Comparison

### Package Recommendations

| Backend | Primary SDK | Alternative | Notes |
|---------|------------|------------|-------|
| Redis | `github.com/redis/go-redis/v9` | `github.com/gomodule/redigo` | go-redis is modern, v9+ uses context |
| PostgreSQL | `github.com/jackc/pgx/v5` | `github.com/lib/pq` | pgx is faster, better error handling |
| SQLite | `github.com/mattn/go-sqlite3` | `modernc.org/sqlite` (pure Go) | mattn requires CGO but faster |
| NATS | `github.com/nats-io/nats.go` (official) | - | Official client, well-maintained |
| Kafka | `github.com/segmentio/kafka-go` | `github.com/confluentinc/confluent-kafka-go/v2` | segmentio pure Go, confluent has C deps but faster |
| S3 | `github.com/aws/aws-sdk-go-v2` | `github.com/minio/minio-go/v7` | AWS SDK for production, MinIO for dev |
| ClickHouse | `github.com/ClickHouse/clickhouse-go/v2` | - | Official client |
| Neptune | `github.com/apache/tinkerpop/gremlin-go` | - | Gremlin traversal language |

### Installation

```bash
# Redis
go get github.com/redis/go-redis/v9

# PostgreSQL
go get github.com/jackc/pgx/v5

# SQLite
go get github.com/mattn/go-sqlite3

# NATS
go get github.com/nats-io/nats.go

# Kafka
go get github.com/segmentio/kafka-go

# S3
go get github.com/aws/aws-sdk-go-v2/service/s3
go get github.com/minio/minio-go/v7

# ClickHouse
go get github.com/ClickHouse/clickhouse-go/v2

# testcontainers
go get github.com/testcontainers/testcontainers-go
```

---

## Summary

**Implementation Priority:**
1. ✅ **Redis** (Score: 95) - Start here, builds confidence
2. ✅ **PostgreSQL** (Score: 93) - Production-ready relational
3. ✅ **SQLite** (Score: 92) - Perfect for demos and CI
4. ⏭️ **NATS** (Score: 90) - Modern messaging
5. ⏭️ **Kafka** (Score: 78) - Event streaming standard
6. ⏭️ **S3/MinIO** (Score: 85) - Essential for Claim Check
7. ⏭️ **ClickHouse** (Score: 70) - Analytics use case
8. 🔮 **Neptune** (Score: 50) - Defer until others stable

**Key Takeaways:**
- Focus on Redis, PostgreSQL, SQLite first (highest implementability)
- Use testcontainers for all backends except SQLite (embedded)
- Build acceptance tests alongside plugin implementation
- Create demos showing pattern composition
- Validate RFC-015 framework with simple backends before complex ones

---

## Revision History

- 2025-10-09: Initial draft with backend comparison matrix, implementability scoring, and demo plugin configurations
