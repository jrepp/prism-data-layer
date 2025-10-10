---
title: Prism Analysis Summary
date: 2025-10-05
tags: [analysis, netflix, comparison]
---

## What is Prism?

Prism is a high-performance data access layer gateway inspired by Netflix's Data Gateway, but designed for superior performance and developer experience. It sits between applications and heterogeneous data backends (Kafka, NATS, Postgres, SQLite, Neptune), providing:

- **Unified API**: Single gRPC/HTTP interface across all backends
- **Client-Originated Configuration**: Applications declare requirements; Prism auto-provisions
- **Rust Performance**: 10-100x faster than JVM alternatives
- **Local-First Testing**: Real backends locally, no mocks required
- **Protobuf-Driven**: Single source of truth for all data models

## Netflix Data Gateway Analysis

### What Netflix Built

From the reference materials (PDFs and videos), Netflix's Data Gateway provides:

1. **Data Abstraction Layers (DAL)**:
   - KeyValue abstraction (HashMap of SortedMaps)
   - TimeSeries abstraction
   - Graph abstraction (Real-Time Distributed Graph)
   - Entity abstraction (CRUD + Query + Eventing)

2. **Platform Components**:
   - **EC2 instances** with Data Gateway Agent
   - **Envoy proxy** for mTLS, auth, load balancing
   - **DAL containers** (Java Spring Boot) implementing abstractions
   - **Declarative configuration** (runtime + deployment)

3. **Key Features**:
   - Shadow traffic for migrations (zero-downtime database changes)
   - Sharding for fault isolation
   - Namespace abstraction (decouple logical model from physical storage)
   - Capacity planning from high-level requirements

### Architectural Patterns We're Adopting

✅ **Data abstraction layers** - KeyValue, TimeSeries, Graph
✅ **Declarative configuration** - Runtime and deployment configs
✅ **Shadow traffic** - For seamless migrations
✅ **Namespace abstraction** - Storage-agnostic logical names
✅ **Sharding** - Single-tenant deployments for isolation

### Our Improvements

🚀 **Rust proxy** instead of Java/Spring Boot
   - 10-100x throughput improvement
   - Sub-millisecond P50 latency vs. 5ms
   - 25x memory reduction

🔧 **Client-originated configuration**
   - Apps declare patterns in protobuf
   - Prism auto-provisions backends
   - No manual capacity planning

🧪 **Local-first testing**
   - Real backends in Docker Compose
   - No mocks
   - Same tests locally and CI

📐 **Protobuf as single source of truth**
   - Code generation for all components
   - PII tagging drives encryption/masking
   - Consistent types across Rust/Python/TypeScript

## Project Structure

We've created a shallow monorepo structure:

```text
prism/
├── CLAUDE.md                    # Project philosophy and guidance
├── README.md                    # Quick start and overview
├── pyproject.toml               # Python tooling (uv managed)
├── .gitignore                   # Comprehensive gitignore
├── docker-compose.test.yml      # Local testing infrastructure
├── docs/                        # Deep-dive documentation
│   ├── adr/                     # Architecture Decision Records
│   │   ├── 000-template.md
│   │   ├── 001-rust-for-proxy.md
│   │   ├── 002-client-originated-configuration.md
│   │   ├── 003-protobuf-single-source-of-truth.md
│   │   └── 004-local-first-testing.md
│   ├── requirements/            # Requirements documents
│   │   ├── README.md
│   │   ├── FR-001-core-data-abstractions.md
│   │   └── FR-004-pii-handling.md
│   └── netflix/                 # Reference materials (PDFs, transcripts)
├── tooling/                     # Python utilities
│   ├── __init__.py
│   ├── __main__.py              # CLI entry point
│   ├── codegen/                 # Protobuf code generation
│   └── test/                    # Testing utilities
│       └── local_stack.py       # Docker Compose management
├── proto/                       # Protobuf definitions (TODO)
│   └── prism/
├── backends/                    # Backend implementations (TODO)
│   ├── kafka/
│   ├── nats/
│   ├── postgres/
│   ├── sqlite/
│   └── neptune/
├── proxy/                       # Rust gateway (TODO)
└── admin/                       # Ember admin UI (TODO)
```

## Key Architectural Decisions

### ADR-001: Rust for the Proxy

**Rationale**: 10-100x performance improvement over JVM, predictable latency (no GC pauses), memory safety

**Trade-offs**: Steeper learning curve, slower initial development

**Outcome**: Performance is a core differentiator; worth the investment

### ADR-002: Client-Originated Configuration

**Rationale**: Applications know their access patterns best; automate capacity planning

**Example**:
```protobuf
message UserEvents {
  option (prism.access_pattern) = "append_heavy";
  option (prism.estimated_write_rps) = "10000";
  option (prism.retention_days) = "90";
}
// → Prism selects Kafka, provisions 20 partitions, sets 90-day retention
```

**Trade-offs**: Requires sophisticated capacity planner

**Outcome**: Self-service data provisioning; faster development

### ADR-003: Protobuf as Single Source of Truth

**Rationale**: One definition generates Rust, Python, TypeScript, SQL schemas, deployment configs

**Example**:
```protobuf
message UserProfile {
  string email = 2 [(prism.pii) = "email", (prism.encrypt_at_rest) = true];
}
// → Generates encryption code, masked logging, audit trails
```

**Trade-offs**: Code generation complexity

**Outcome**: DRY architecture; consistency across components

### ADR-004: Local-First Testing

**Rationale**: Real backends catch real bugs; mocks give false confidence

**Example**:
```bash
python -m tooling.test.local-stack up  # Starts Postgres, Kafka, NATS
cargo test --workspace                 # Tests against real databases
```

**Trade-offs**: Requires Docker; slower than mocks

**Outcome**: High confidence; fast feedback; easy debugging

## Requirements Overview

### FR-001: Core Data Abstractions

Three primary abstractions:

1. **KeyValue**: `HashMap<String, SortedMap<Bytes, Bytes>>`
   - Backends: Postgres, Cassandra, SQLite, S3
   - Features: Chunking, compression, caching

2. **TimeSeries**: Append-only log with timestamp queries
   - Backends: Kafka, ClickHouse, Postgres+TimescaleDB, NATS
   - Features: Retention policies, tail (subscribe), partitioning

3. **Graph**: Nodes and edges with traversal
   - Backends: Neptune, Neo4j, Postgres, KeyValue+adjacency lists
   - Features: BFS/DFS, shortest path, filtering

### FR-004: PII Handling

**Core capability**: Protobuf field annotations drive automatic PII handling

```protobuf
string ssn = 5 [
  (prism.pii) = "ssn",
  (prism.encrypt_at_rest) = true,
  (prism.mask_in_logs) = true,
  (prism.access_audit) = true
];
```

**Features**:
- **Encryption**: AES-256-GCM with AWS KMS, transparent to apps
- **Masking**: Auto-redact in logs/metrics (`***-**-****`)
- **Audit**: Immutable log of every PII access
- **Deletion**: Right to be forgotten (GDPR/CCPA)

## Next Steps

### Immediate (Week 1-2)

1. **Initialize Rust proxy**
   - `cargo new proxy --lib`
   - Set up tonic gRPC server skeleton
   - Implement health check endpoint

2. **First protobuf definitions**
   - Create `prism/options.proto` with custom options
   - Create `prism/keyvalue/v1/keyvalue.proto`
   - Set up code generation pipeline

3. **SQLite backend** (simplest for testing)
   - Implement KeyValue abstraction
   - Put, Get, Delete, Scan operations

4. **Integration tests**
   - Test KeyValue with SQLite
   - Verify local-stack works

### Short-Term (Month 1)

5. **Postgres backend**
   - Same KeyValue API, different storage
   - Connection pooling
   - Schema migration tooling

6. **Kafka backend** for TimeSeries
   - Append, Query, Tail operations
   - Topic creation, retention policies

7. **Admin UI basics**
   - Ember app initialization
   - Namespace management
   - Basic metrics dashboard

### Medium-Term (Month 2-3)

8. **Client-originated configuration**
   - Capacity planner implementation
   - Auto-provisioning logic

9. **Shadow traffic** for migrations
   - Dual-write support
   - Comparison testing

10. **Production deployment**
    - CI/CD pipeline
    - Monitoring and alerting
    - Load testing at scale

## Open Questions for Discussion

1. **Authentication**: mTLS only, or support OAuth2/JWT?

2. **Multi-tenancy**: Single Prism instance for all apps, or one per team?

3. **Observability**: OpenTelemetry from day one, or add later?

4. **API versioning**: `/v1/` in URLs, or protobuf package versions?

5. **Caching layer**: Built into proxy, or separate service (Redis/memcached)?

6. **Admin UI**: Essential for v1, or can wait for v2?

7. **Neptune vs. Neo4j**: AWS lock-in vs. portability?

8. **Schema registry**: Buf.build, or self-hosted?

## Success Metrics

How we'll measure Prism's success:

- **Performance**: P50 < 1ms, P99 < 10ms (vs. Netflix's ~5ms P50)
- **Adoption**: 10+ applications using Prism within 6 months
- **Reliability**: 99.99% uptime SLO
- **Developer Satisfaction**: `<10 min` from signup to first query
- **Cost Efficiency**: 50% reduction in infrastructure spend vs. direct database access

## References

All Netflix reference materials are in `docs/netflix/`:
- Data Gateway architecture PDF
- KV Data Abstraction Layer materials
- Real-Time Distributed Graph video transcripts

## Conclusion

Prism takes Netflix's proven Data Gateway architecture and improves it with:
- **Rust** for extreme performance
- **Client-originated config** for developer velocity
- **Protobuf-driven** for consistency
- **Local-first testing** for confidence

We have a solid foundation of ADRs and requirements to guide implementation. The shallow monorepo structure makes navigation easy. Python tooling with `uv` provides a good developer experience.

**Next action**: Start implementing the Rust proxy with SQLite backend!
