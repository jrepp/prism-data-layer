---
title: "RFC-001: Prism Data Access Layer Architecture"
status: Draft
author: Core Team
created: 2025-10-07
updated: 2025-10-07
---

## Abstract

This RFC defines the complete architecture for Prism, a high-performance data access layer gateway that provides a unified, client-configurable interface to heterogeneous data backends. Prism is inspired by Netflix's Data Gateway but designed for superior performance, developer experience, and operational simplicity.

## 1. Introduction

### 1.1 Purpose

Prism addresses the complexity of managing multiple data backends in modern applications by providing:

1. **Unified Interface**: Single gRPC API for accessing databases, message queues, and pub/sub systems
2. **Dynamic Configuration**: Clients declare their data access patterns; Prism handles provisioning and routing
3. **Performance**: Rust-based proxy with sub-millisecond overhead and 10k+ RPS per connection
4. **Backend Abstraction**: Applications can switch backends without code changes
5. **Observability**: Built-in tracing, metrics, and audit logging

### 1.2 Goals

- **Performance**: P99 latency `<10ms`, 10k+ RPS sustained per connection
- **Flexibility**: Support multiple access patterns (KV, Queue, PubSub, Paged Reader, Transactions)
- **Scalability**: Horizontal scaling of both proxy and backend-specific containers
- **Security**: mTLS, OAuth2, PII tagging, audit logging
- **Developer Experience**: Type-safe gRPC interfaces with generated clients

### 1.3 Non-Goals

- **Not a database**: Prism is a gateway, not a storage engine
- **Not for complex queries**: Simple access patterns only; complex analytics use dedicated tools
- **Not a cache**: Caching is optional per-namespace configuration
- **Not a message broker**: Prism wraps existing brokers (Kafka, NATS), doesn't replace them

## 2. Architecture Overview

### 2.1 System Components

┌──────────────────────────────────────────────────────────┐
│                   Client Applications                    │
│              (Go, Rust, Python, JavaScript)              │
└────────────────────────┬─────────────────────────────────┘
                         │ gRPC/HTTP2
                         │
┌────────────────────────▼─────────────────────────────────┐
│                   Prism Proxy Core                       │
│                   (Rust + Tokio)                         │
│                                                           │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐    │
│  │   Config    │  │   Session   │  │    Queue     │    │
│  │   Service   │  │   Service   │  │   Service    │    │
│  └─────────────┘  └─────────────┘  └──────────────┘    │
│                                                           │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐    │
│  │   PubSub    │  │   Reader    │  │   Transact   │    │
│  │   Service   │  │   Service   │  │   Service    │    │
│  └─────────────┘  └─────────────┘  └──────────────┘    │
└────────────────────────┬─────────────────────────────────┘
                         │
        ┌────────────────┴────────────────┐
        │                                 │
┌───────▼──────────┐           ┌──────────▼────────┐
│ Container Plugins│           │ Container Plugins │
│                  │           │                   │
│ • Kafka Pub      │           │ • NATS Pub       │
│ • Kafka Con      │           │ • NATS Con       │
│ • Indexed Reader │           │ • Transact Proc  │
│ • Mailbox Listen │           │ • Custom...      │
└───────┬──────────┘           └──────────┬────────┘
        │                                 │
┌───────▼──────────┐           ┌──────────▼────────┐
│     Backends     │           │     Backends      │
│                  │           │                   │
│ • Postgres       │           │ • Kafka          │
│ • SQLite         │           │ • NATS           │
│ • Neptune        │           │ • Redis          │
└──────────────────┘           └───────────────────┘
```text

### 2.2 Key Design Principles

1. **gRPC-First**: All communication uses gRPC for performance and type safety
2. **Session-Based**: All operations require an authenticated session
3. **Layered Interfaces**: From basic (sessions) to use-case-specific (queue, pubsub, reader, transact)
4. **Backend Polymorphism**: Each interface layer supports multiple backend implementations
5. **Container Plugins**: Backend-specific logic deployed as independent, scalable containers
6. **Protobuf-Driven**: All APIs, configurations, and data models defined in protobuf

## 3. Client Configuration System

### 3.1 Overview

Prism separates **server configuration** (infrastructure) from **client configuration** (access patterns).

**Server Configuration** (static, admin-controlled):
- Backend connection strings
- Resource pools
- Auth policies
- Rate limits

**Client Configuration** (dynamic, runtime):
- Access pattern (Queue, PubSub, Reader, Transact)
- Backend selection
- Consistency requirements
- Cache policy

### 3.2 Configuration Descriptor

Clients provide configuration as protobuf messages:

```
message ClientConfig {
  string name = 1;                  // Named config or custom
  string version = 2;               // For evolution
  AccessPattern pattern = 3;        // QUEUE, PUBSUB, READER, TRANSACT
  BackendConfig backend = 4;        // Backend type + options
  ConsistencyConfig consistency = 5; // EVENTUAL, STRONG, BOUNDED_STALENESS
  CacheConfig cache = 6;            // TTL, size, enabled
  RateLimitConfig rate_limit = 7;   // RPS, burst
  string namespace = 8;             // Multi-tenancy isolation
}
```text

### 3.3 Configuration Sources

**Named Configurations** (server-provided templates):
```
# Client requests pre-configured pattern
config, err := client.GetConfig("user-profiles")
session, err := client.StartSession(config)
```text

**Inline Configurations** (client-provided):
```
config := &ClientConfig{
    Pattern: ACCESS_PATTERN_QUEUE,
    Backend: &BackendConfig{Type: BACKEND_TYPE_KAFKA},
    Consistency: &ConsistencyConfig{Level: CONSISTENCY_LEVEL_EVENTUAL},
}
session, err := client.StartSession(config)
```text

### 3.4 Configuration Validation

Server validates all configurations:
- Backend compatibility with access pattern
- Namespace existence
- Rate limit sanity checks
- Resource availability

## 4. Session Management

### 4.1 Session Lifecycle

1. **Create**: Client authenticates and provides configuration
2. **Active**: Session token used for all operations
3. **Heartbeat**: Periodic keepalives extend session lifetime
4. **Close**: Clean shutdown releases resources

### 4.2 Session Service API

```
service SessionService {
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
  rpc CloseSession(CloseSessionRequest) returns (CloseSessionResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc GetSession(GetSessionRequest) returns (GetSessionResponse);
}
```text

### 4.3 Session State

Server tracks:
- Session ID and token
- User/client identity
- Active configuration
- Backend connections
- Creation and expiration timestamps
- Activity for idle timeout

### 4.4 Session Security

- mTLS for service-to-service
- OAuth2/JWT for user-facing APIs
- API keys for machine clients
- Session tokens opaque to clients
- Audit log for all session events

## 5. Interface Layers

### 5.1 Layer Hierarchy

┌────────────────────────────────────────────┐
│  Layer 5: Use-Case Specific Interfaces    │
│                                            │
│  ┌────────┐ ┌────────┐ ┌────────┐        │
│  │ Queue  │ │ PubSub │ │ Reader │        │
│  └────────┘ └────────┘ └────────┘        │
│  ┌────────┐                               │
│  │Transact│                               │
│  └────────┘                               │
└────────────────┬───────────────────────────┘
                 │
┌────────────────▼───────────────────────────┐
│  Layer 1: Session Service (Foundation)     │
│  - Authentication                          │
│  - Authorization                           │
│  - Auditing                                │
│  - Connection State                        │
└────────────────────────────────────────────┘
```

### 5.2 Queue Service

**Purpose**: Kafka-style message queue operations

**Operations**:
- `Publish`: Send message to topic
- `Subscribe`: Stream messages from topic (server-streaming)
- `Acknowledge`: Confirm message processing
- `Commit`: Commit offset
- `Seek`: Jump to specific offset

**Backend Mapping**:
- Kafka → Topics, partitions, offsets
- NATS JetStream → Streams, consumers, sequences
- Postgres → Table-based queue with SKIP LOCKED

### 5.3 PubSub Service

**Purpose**: NATS-style publish-subscribe with topic wildcards

**Operations**:
- `Publish`: Publish event to topic
- `Subscribe`: Subscribe to topic pattern (server-streaming)
- `Unsubscribe`: Cancel subscription

**Topic Patterns**:
- Exact: `events.user.created`
- Wildcard: `events.user.*`
- Multi-level: `events.>`

**Backend Mapping**:
- NATS → Native subject routing
- Kafka → Topic prefix matching
- Redis Pub/Sub → Channel patterns

### 5.4 Reader Service

**Purpose**: Database-style paged reading and queries

**Operations**:
- `Read`: Stream pages of data (cursor-based pagination)
- `Query`: Stream filtered/sorted results
- `Count`: Count matching records

**Pagination**:
- Cursor-based (opaque continuation tokens)
- Server streams pages as client ready
- No client-side buffering of full result set

**Backend Mapping**:
- Postgres/SQLite → SQL with LIMIT/OFFSET
- DynamoDB → Query with pagination tokens
- Neptune → Gremlin with pagination

### 5.5 Transact Service

**Purpose**: Transactional writes across two tables (inbox/outbox pattern)

**Operations**:
- `Write`: Single transactional write (data + mailbox)
- `Transaction`: Streaming transaction (begin, writes, commit/rollback)

**Two-Table Pattern**:
1. **Data Table**: Business data (users, orders, etc.)
2. **Mailbox Table**: Outbox for downstream processing

**Use Cases**:
- Transactional outbox pattern
- Event sourcing with guaranteed writes
- Saga coordination

**Backend Mapping**:
- Postgres/SQLite → Native transactions
- DynamoDB → TransactWriteItems

## 6. Container Plugin Model

### 6.1 Plugin Architecture

Backend-specific functionality deployed as independent containers:

**Plugin Roles**:
- **Publisher**: Produces messages to backend (Kafka Publisher, NATS Publisher)
- **Consumer**: Consumes messages from backend (Kafka Consumer, NATS Consumer)
- **Processor**: Processes operations (Transaction Processor)
- **Listener**: Listens for events (Mailbox Listener)

### 6.2 Plugin Contract

All plugins implement standard interfaces:

```protobuf
service HealthService {
  rpc Live(LiveRequest) returns (LiveResponse);    // Liveness probe
  rpc Ready(ReadyRequest) returns (ReadyResponse);  // Readiness probe
}

service MetricsService {
  rpc GetMetrics(MetricsRequest) returns (MetricsResponse);  // Prometheus metrics
}

service PluginInfoService {
  rpc GetInfo(InfoRequest) returns (InfoResponse);  // Name, version, role, backend
}
```

### 6.3 Configuration

Plugins configured via environment variables (12-factor):

```bash
# Common
PRISM_PROXY_ENDPOINT=localhost:8980
PRISM_PLUGIN_ROLE=publisher
PRISM_BACKEND_TYPE=kafka
PRISM_NAMESPACE=production

# Backend-specific
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=events
NATS_URL=nats://localhost:4222
DATABASE_URL=postgres://...
```

### 6.4 Deployment

**Docker Compose**:
```yaml
services:
  prism-proxy:
    image: prism/proxy:latest
    ports: ["8980:8980"]

  kafka-publisher:
    image: prism/kafka-publisher:latest
    environment:
      PRISM_PROXY_ENDPOINT: prism-proxy:8980
      KAFKA_BROKERS: kafka:9092
    deploy:
      replicas: 2
```

**Kubernetes**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prism-kafka-consumer
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: kafka-consumer
        image: prism/kafka-consumer:latest
        env:
        - name: PRISM_PROXY_ENDPOINT
          value: "prism-proxy:8980"
```

### 6.5 Scaling

- **Horizontal**: Deploy multiple replicas per plugin
- **Independent**: Scale each plugin independently based on load
- **Stateless**: Plugins are stateless (state in backend or proxy)

## 7. Data Flow Examples

### 7.1 Queue: Kafka Publisher Flow

1. Client calls `Publish(topic, message)`
2. Proxy validates session
3. Proxy enqueues message to internal queue
4. Kafka Publisher container polls internal queue
5. Publisher sends to Kafka broker
6. Publisher acknowledges to proxy
7. Proxy returns `PublishResponse` to client

### 7.2 Transactional Write Flow

1. Client calls `Write(data, mailbox)`
2. Proxy routes to Transaction Processor container
3. Processor begins database transaction
4. Processor inserts into data table
5. Processor inserts into mailbox table
6. Processor commits transaction
7. Processor returns success to proxy
8. Proxy returns `WriteResponse` to client
9. Mailbox Listener container polls mailbox table
10. Listener processes new messages
11. Listener marks messages as processed

### 7.3 Paged Reader Flow

1. Client calls `Read(collection, page_size=100)`
2. Proxy starts server-streaming response
3. Indexed Reader container queries database (LIMIT 100)
4. Reader streams page 1 to proxy
5. Proxy streams page 1 to client
6. Reader queries next page (OFFSET 100)
7. Reader streams page 2 to proxy
8. Proxy streams page 2 to client
9. Repeat until no more results
10. Reader closes stream

## 8. Observability

### 8.1 Distributed Tracing

- OpenTelemetry from day one
- Spans for all gRPC operations
- Trace propagation across services
- Export to Jaeger/Tempo

### 8.2 Metrics

**Proxy Metrics**:
- Request rate (per service)
- Request latency (P50, P95, P99)
- Active sessions
- Backend connection pool utilization
- Cache hit rate

**Plugin Metrics**:
- Messages published/consumed
- Processing latency
- Error rate
- Queue depth

**Export**: Prometheus `/metrics` endpoint

### 8.3 Logging

**Rust (Proxy)**:
- `tracing` crate for structured logging
- JSON output in production
- Span context for correlation

**Go (Tooling/Clients)**:
- `slog` for structured logging
- Context propagation
- JSON output

### 8.4 Audit Logging

All operations logged with:
- Session ID
- User identity
- Operation type
- Resource accessed
- Timestamp
- Success/failure

## 9. Security

### 9.1 Authentication

**Service-to-Service**:
- mTLS with mutual certificate validation
- Certificate rotation

**User-Facing**:
- OAuth2 with JWT tokens
- API keys for machine clients

### 9.2 Authorization

- Namespace-level policies
- Role-based access control (RBAC)
- Operation-level permissions

### 9.3 Data Protection

**PII Tagging**:
```protobuf
message UserProfile {
  string user_id = 1;
  string email = 2 [(prism.pii) = "email"];
  string name = 3 [(prism.pii) = "name"];
}
```

**Automatic Handling**:
- Encryption at rest (per-field)
- Audit logging for PII access
- Masking in logs

## 10. Performance Targets

### 10.1 Latency

- **P50**: `<2ms overhead`
- **P95**: `<5ms overhead`
- **P99**: `<10ms overhead`

(Overhead measured from gRPC request receipt to backend call)

### 10.2 Throughput

- **Per connection**: 10k+ RPS sustained
- **Per proxy instance**: 100k+ RPS (10k connections × 10 RPS)
- **Horizontally scalable**: Add more proxy instances

### 10.3 Resource Utilization

- **Proxy**: `<500MB RAM` per instance
- **Plugins**: `<100MB RAM` per container
- **CPU**: `<10% overhead` for routing logic

## 11. Implementation Roadmap

### Phase 1: Foundation (Weeks 1-4)

**Week 1**:
- ✅ Protobuf foundation (ADR-011, Step 1 complete)
- Rust proxy skeleton (Step 2)

**Week 2**:
- KeyValue protobuf + stubs (Step 3)
- SQLite backend implementation (Step 4)

**Week 3-4**:
- Integration tests + CI (Step 5)
- Postgres backend + docs (Step 6)

### Phase 2: Sessions + Config (Weeks 5-6)

- Dynamic client configuration system (ADR-022)
- Session service implementation
- Named configuration storage
- Auth integration (mTLS, OAuth2)

### Phase 3: Queue Layer (Weeks 7-8)

- Queue service protobuf + implementation
- Kafka publisher container
- Kafka consumer container
- Integration tests with local Kafka

### Phase 4: PubSub Layer (Weeks 9-10)

- PubSub service protobuf + implementation
- NATS publisher container
- NATS consumer container
- Topic pattern matching

### Phase 5: Reader Layer (Weeks 11-12)

- Reader service protobuf + implementation
- Indexed reader container
- Cursor-based pagination
- Query filtering

### Phase 6: Transact Layer (Weeks 13-14)

- Transact service protobuf + implementation
- Transaction processor container
- Mailbox listener container
- Two-table transaction tests

### Phase 7: Production Readiness (Weeks 15-16)

- OpenTelemetry integration
- Prometheus metrics
- Performance benchmarking
- Load testing
- Documentation
- Deployment guides

## 12. Success Criteria

✅ **Functional**:
- All 5 interface layers implemented
- All backend plugins working
- End-to-end tests passing

✅ **Performance**:
- P99 `<10ms latency`
- 10k+ RPS sustained

✅ **Operational**:
- Deployed to production
- Monitoring dashboards
- Runbooks complete

✅ **Developer Experience**:
- Client libraries for Go, Rust, Python
- Complete API documentation
- Example applications

## 13. Open Questions

1. **Shadow traffic**: When to implement for backend migrations?
2. **Multi-region**: Active-active or active-passive?
3. **Cache layer**: Implement now or defer?
4. **Admin UI**: Build Ember UI or defer to CLI tools?

## 14. References

### ADRs
- ADR-001: Rust for the Proxy
- ADR-003: Protobuf as Single Source of Truth
- ADR-011: Implementation Roadmap
- ADR-022: Dynamic Client Configuration
- ADR-023: gRPC-First Interface Design
- ADR-024: Layered Interface Hierarchy
- ADR-025: Container Plugin Model

### External
- [Netflix Data Gateway](https://netflixtechblog.com/data-gateway-a-platform-for-growing-and-protecting-the-data-tier-f1-2019-3fd1a829503)
- [gRPC Documentation](https://grpc.io)
- [Inbox/Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)

## 15. Revision History

- 2025-10-07: Initial draft
