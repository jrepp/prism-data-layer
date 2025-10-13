---
title: Overview
sidebar_position: 1
project_id: prism-data-layer
doc_uuid: 029df111-6f17-42db-8782-facb1eef6d94
---

# Prism Documentation

**Unify your data access. One API, any backend. Blazing fast.**

Prism is a high-performance data access gateway providing a unified interface to heterogeneous backends (Kafka, Postgres, Redis, NATS). Applications declare requirements; Prism handles provisioning, optimization, and reliability patterns.

## 🆕 What's New

**[View Recent Changes →](/docs/changelog)**

Recent highlights:
- **Architecture Guide**: Comprehensive technical overview with system diagrams
- **Three-Layer Design**: Separates client API, patterns, and backends
- **Authorization Boundaries**: Policy-driven configuration for team self-service
- **45 Thin Interfaces**: Type-safe backend composition across 10 data models

---

## Core Idea

**Three layers separate what, how, and where**:

```
┌────────────────────────────────────┐
│   Client API (What)                │  Applications use stable APIs
│   KeyValue | PubSub | Queue        │
└────────────────────────────────────┘
               ↓
┌────────────────────────────────────┐
│   Patterns (How)                   │  Prism applies reliability patterns
│   Outbox | CDC | Claim Check       │
└────────────────────────────────────┘
               ↓
┌────────────────────────────────────┐
│   Backends (Where)                 │  Data stored in optimal backend
│   Kafka | Postgres | Redis | NATS  │
└────────────────────────────────────┘
```

**Benefits**:
- **Backend Migration**: Swap Redis → DynamoDB without client changes
- **Pattern Evolution**: Add CDC without API breakage
- **Configuration-Driven**: Declare needs; Prism selects patterns
- **Organizational Scale**: Teams self-service with policy guardrails

---

## Why Prism?

### Unified Interface
Single gRPC/HTTP API across all backends. Write once, run anywhere.

### Self-Service Configuration
Applications declare requirements in protobuf:

```protobuf
message UserEvents {
  option (prism.access_pattern) = "append_heavy";
  option (prism.estimated_write_rps) = "10000";
  option (prism.retention_days) = "90";
}
// → Prism selects Kafka, provisions 20 partitions
```

**Authorization boundaries** prevent misconfigurations:
- **Guided**: Pre-approved backends for all teams (Postgres, Kafka, Redis)
- **Advanced**: Backend-specific tuning with approval
- **Expert**: Platform team unrestricted access

**Result**: Infrastructure team of 10 supports 500+ application teams (50x improvement over manual provisioning).

### Rust Performance
10-100x faster than JVM alternatives:
- **P50**: `<1ms` (vs ~5ms JVM)
- **P99**: `<10ms` (vs ~50ms JVM)
- **Throughput**: 200k+ RPS (vs ~20k JVM)
- **Memory**: 20MB idle (vs ~500MB JVM)

### Interface-Based Capabilities
Backends implement **thin interfaces** (not capability flags):

```
Redis implements:
  - keyvalue_basic   (Set, Get, Delete)
  - keyvalue_scan    (Scan, Count)
  - keyvalue_ttl     (Expire, GetTTL)
  - pubsub_basic     (Publish, Subscribe)
  - stream_basic     (Append, Read)
  → 16 interfaces total
```

**Type-safe**: Compiler enforces contracts (no runtime surprises).

---

## Docs

### Decisions
Architecture Decision Records (ADRs) capture why technical choices were made.

**When to read**: Understanding project philosophy, evaluating alternatives, onboarding.

**Start with**: [Why Rust?](/adr/adr-001-rust-for-proxy) | [Client Configuration](/adr/adr-002)

---

### Designs
Request for Comments (RFCs) provide detailed specifications before implementation.

**When to read**: Understanding system designs, implementing features, reviewing proposals.

**Start with**: [Architecture](/rfc/rfc-001-prism-architecture) | [Layered Patterns](/rfc/rfc-014-layered-data-access-patterns)

---

### Guides
Tutorials, references, and troubleshooting for using and developing Prism.

**When to read**: Getting started, learning features, debugging issues.

**Start with**: [Architecture Guide](/docs/architecture)

---

## Core Concepts

### Patterns vs Pattern Providers

- **Pattern**: Abstract concept (KeyValue, Outbox, Multicast Registry)
- **Pattern Provider**: Runtime process implementing pattern
- **Backend Driver**: Connection code for specific backends (Kafka, Redis, Postgres)

**Pattern Providers** use **Backend Drivers** configured via **slots**. Backends are configured separately, and slots bind backend interfaces to pattern requirements:

```yaml
# Backend configuration (connection details)
backends:
  redis-cache:
    type: redis
    connection: "redis://localhost:6379/0"
  nats-messaging:
    type: nats
    connection: "nats://localhost:4222"
  postgres-queue:
    type: postgres
    connection: "postgresql://localhost:5432/prism"

# Pattern configuration (slot bindings)
pattern: multicast-registry
slots:
  registry:
    backend: redis-cache      # References backend config
    interface: keyvalue_basic # Required interface
  messaging:
    backend: nats-messaging
    interface: pubsub_basic
  durability:
    backend: postgres-queue
    interface: queue_basic
```

Same application code works with different backend combinations (Redis+NATS+Postgres or DynamoDB+SNS+SQS) by changing backend configuration.

### Data Models

Prism provides 10 data models with 45 interfaces:

| Model | Interfaces | Backends |
|-------|-----------|----------|
| **KeyValue** | 6 (basic, scan, ttl, transactional, batch, cas) | Redis, Postgres, DynamoDB, MemStore |
| **PubSub** | 5 (basic, wildcards, persistent, filtering, ordering) | NATS, Redis, Kafka |
| **Stream** | 5 (basic, consumer_groups, replay, retention, partitioning) | Kafka, Redis, NATS |
| **Queue** | 5 (basic, visibility, dead_letter, priority, delayed) | Postgres, SQS, RabbitMQ |
| **TimeSeries** | 4 (basic, aggregation, retention, interpolation) | ClickHouse, TimescaleDB, InfluxDB |

### PII Handling

Protobuf annotations drive automatic PII handling:

```protobuf
message UserProfile {
  string email = 2 [
    (prism.pii) = "email",
    (prism.encrypt_at_rest) = true,
    (prism.mask_in_logs) = true
  ];
}
// → Generates encryption, masked logging, audit trails
```

---

## Start Here

1. **Architecture**: Read [Architecture Guide](/docs/architecture) for system overview
2. **Decisions**: Browse ADRs to understand technical choices
3. **Designs**: Review key RFCs ([Architecture](/rfc/rfc-001-prism-architecture), [Layered Patterns](/rfc/rfc-014-layered-data-access-patterns))
4. **Setup**: Follow [repository instructions](https://github.com/jrepp/prism-data-layer)

---

## Performance

- **P50 Latency**: `<1ms`
- **P99 Latency**: `<10ms`
- **Throughput**: 10k+ RPS per connection
- **Memory**: `<500MB` per proxy instance

---

## Philosophy

1. **Performance First**: Rust proxy for maximum throughput, minimal latency
2. **Client Configuration**: Applications know their needs best
3. **Local Testing**: Real backends over mocks for realistic testing
4. **Pluggable Backends**: Clean abstraction allows adding backends without client changes
5. **Code Generation**: Protobuf definitions drive all code generation

---

For development practices and project guidance, see [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md).
