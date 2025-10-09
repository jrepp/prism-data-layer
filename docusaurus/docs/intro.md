---
title: Documentation Overview
sidebar_position: 1
---

# Prism Documentation

**Unify your data access. One API, any backend. Blazing fast.**

Welcome to the Prism Data Access Layer documentation. Prism is a high-performance data access gateway that provides a unified, client-configurable interface to heterogeneous data backends.

## 🆕 What's New

Stay up to date with the latest changes, new features, and improvements:

**[View Recent Changes →](https://github.com/jrepp/prism-data-layer/blob/main/docs-cms/CHANGELOG.md)**

Recent highlights:
- RFC-014: Layered Data Access Patterns
- Enhanced Admin Protocol with OIDC (RFC-010)
- Data Proxy Authentication improvements (RFC-011)
- Distributed Reliability Patterns (RFC-009)

---

## What is Prism?

Prism sits between applications and data backends (Kafka, NATS, Postgres, SQLite, Neptune), providing:

- **Unified API**: Single gRPC/HTTP interface across all backends
- **Client-Originated Configuration**: Applications declare requirements; Prism auto-provisions
- **Rust Performance**: 10-100x faster than JVM alternatives
- **Local-First Testing**: Real backends locally, no mocks required
- **Protobuf-Driven**: Single source of truth for all data models

## Documentation Types

Our documentation is organized into three main types:

### 📋 [Architecture Decision Records (ADRs)](/adr)

Track all significant architectural decisions made in the Prism project. Each ADR captures the context, decision, and consequences of a specific choice.

**When to read**: Understanding why certain technical choices were made, evaluating alternatives that were considered, onboarding to the project's architectural philosophy.

[Browse ADRs →](/adr)

---

### 📐 [Request for Comments (RFCs)](/rfc)

Detailed technical specifications for major features and components. RFCs provide comprehensive design documentation before implementation.

**When to read**: Understanding complete system designs, implementing new features, reviewing proposed changes before they're built.

[Browse RFCs →](/rfc)

---

### 📖 General Documentation

Tutorials, guides, and reference documentation for using and developing Prism.

**When to read**: Getting started with Prism, learning how to use specific features, troubleshooting issues.

---

## Key Concepts

### Data Abstractions

Prism provides three primary data abstractions:

1. **KeyValue**: HashMap of SortedMaps backed by Postgres, Cassandra, SQLite, or S3
2. **TimeSeries**: Append-only log with timestamp queries, backed by Kafka, ClickHouse, or NATS
3. **Graph**: Nodes and edges with traversal, backed by Neptune, Neo4j, or Postgres

### Client-Originated Configuration

Applications declare their data access patterns in protobuf:

```protobuf
message UserEvents {
  option (prism.access_pattern) = "append_heavy";
  option (prism.estimated_write_rps) = "10000";
  option (prism.retention_days) = "90";
}
// → Prism selects Kafka, provisions 20 partitions, sets 90-day retention
```

### PII Handling

Protobuf field annotations drive automatic PII handling:

```protobuf
message UserProfile {
  string email = 2 [
    (prism.pii) = "email",
    (prism.encrypt_at_rest) = true,
    (prism.mask_in_logs) = true
  ];
}
// → Generates encryption code, masked logging, audit trails
```

## Getting Started

1. **Understand the Architecture**: Start with [RFC-001: Prism Architecture](/rfc/RFC-001-prism-architecture)
2. **Review Key Decisions**: Browse [ADRs](/adr) to understand architectural choices
3. **Explore Interfaces**: Read [RFC-002: Data Layer Interface](/rfc/RFC-002-data-layer-interface)
4. **Set Up Locally**: Follow the development setup in the [repository](https://github.com/jrepp/prism-data-layer)

## Performance Targets

- **P50 Latency**: `<1ms`
- **P99 Latency**: `<10ms`
- **Throughput**: 10k+ RPS per connection
- **Memory**: `<500MB` per proxy instance

## Project Philosophy

Prism is built on these principles:

1. **Performance First**: Rust-based proxy for maximum throughput and minimal latency
2. **Client-Originated Configuration**: Applications know their needs best
3. **Local-First Testing**: Real backends over mocks for realistic testing
4. **Pluggable Backends**: Clean abstraction layer allows adding backends without changing application code
5. **DRY via Code Generation**: Protobuf definitions with custom tags drive code generation across components

---

For more details on the project philosophy and development practices, see [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md) in the repository.
