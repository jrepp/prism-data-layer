---
title: "Netflix Data Gateway Reference"
sidebar_label: "Overview"
sidebar_position: 0
tags: [netflix, data-gateway, reference, architecture]
---

# Netflix Data Gateway Reference

This section contains research and learnings from Netflix's Data Gateway architecture, which serves as inspiration for the Prism data access layer.

## What is Netflix's Data Gateway?

Netflix's Data Gateway is a battle-tested platform that provides **data abstraction layers** to simplify and scale data access across thousands of microservices. It decouples application logic from database implementations, enabling:

- **Unified APIs** for diverse data stores (Cassandra, EVCache, DynamoDB, etc.)
- **Operational resilience** through circuit breaking, load shedding, and failover
- **Zero-downtime migrations** via shadow traffic and dual-write patterns
- **Massive scale**: 8M+ QPS, 3,500+ use cases, petabytes of data

## Why This Matters for Prism

Prism adopts many of Netflix's proven patterns while improving on performance and operational simplicity:

| Aspect | Netflix Approach | Prism Enhancement |
|--------|-----------------|-------------------|
| **Proxy Layer** | JVM-based gateway | **Rust-based** for 10-100x performance |
| **Configuration** | Runtime deployment configs | **Client-originated** requirements (apps declare needs) |
| **Testing** | Production-validated | **Local-first** with sqlite, local kafka |
| **Deployment** | Kubernetes-native | **Flexible**: bare metal, VMs, or containers |

## Core Concepts

### Data Abstractions

Netflix built multiple abstraction layers on their Data Gateway platform:

- **[Key-Value](/prism-data-layer/netflix/abstractions)**: Primary abstraction for 400+ use cases
- **TimeSeries**: Handles 10M writes/sec for event tracking
- **Distributed Counter**: Scalable counting with tunable accuracy
- **[Write-Ahead Log (WAL)](/prism-data-layer/netflix/write-ahead-log)**: Durable, ordered mutation delivery

### Scale & Performance

Netflix's Data Gateway operates at massive scale:

- **8 million queries per second** (key-value abstraction)
- **10 million writes per second** (time-series data)
- **3,500+ use cases** across the organization
- **Petabyte-scale** storage with low-latency retrieval

**[Read more about scale metrics →](/prism-data-layer/netflix/scale)**

### Migration Patterns

Netflix's approach to zero-downtime migrations:

- **[Dual-Write Pattern](/prism-data-layer/netflix/dual-write-migration)**: Write to old and new datastores simultaneously
- **Shadow Traffic**: Validate new systems with production load
- **Phased Cutover**: Gradual migration with rollback capability
- **[Schema Evolution](/prism-data-layer/netflix/data-evolve-migration)**: Automated compatibility checking

## Key Learnings

### 1. **Abstraction Simplifies Scale**

Managing database API complexity becomes unmanageable as services scale. A robust data abstraction layer:
- Shields applications from database breaking changes
- Provides user-friendly gRPC/HTTP APIs tailored to access patterns
- Enables backend changes without application code changes

### 2. **Prioritize Reliability**

Building for redundancy and resilience:
- Circuit breaking and back-pressure prevent cascading failures
- Automated load shedding protects backends during traffic spikes
- Rigorous capacity planning prevents resource exhaustion

### 3. **Data Management is Critical**

Proactive data lifecycle management:
- **TTL and cleanup** should be designed in from day one
- **Cost monitoring**: Every byte has a cost
- **Tiering strategies**: Move cold data to cost-effective storage

### 4. **Sharding for Isolation**

Product/feature sharding prevents noisy neighbor problems:
- Dedicated proxy instances per product or SLA tier
- Independent scaling and capacity planning
- Clear ownership and blast radius containment

**[Read full lessons learned →](/prism-data-layer/netflix/summary)**

## Reference Materials

- **[Netflix Data Gateway Use Cases](/prism-data-layer/netflix/key-use-cases)**: Real-world applications
- **[Scale Metrics](/prism-data-layer/netflix/scale)**: Performance and throughput numbers
- **[Data Abstractions](/prism-data-layer/netflix/abstractions)**: Counter, WAL, and other patterns
- **[Migration Strategies](/prism-data-layer/netflix/dual-write-migration)**: Dual-write and shadow traffic
- **Video Transcripts**: Conference talks on data abstractions (see sidebar)

### PDF References

Original blog posts and articles are archived in the `references/` directory:
- Data Gateway platform overview
- Key-Value abstraction deep dive
- Time-series architecture
- Real-time data processing

## How Prism Uses These Learnings

Prism incorporates Netflix's battle-tested patterns:

1. **Data Abstractions** (ADR-026): KeyValue, TimeSeries, Graph, Entity patterns
2. **Client Configuration** (ADR-001): Apps declare requirements, Prism provisions
3. **Backend Plugins** (RFC-008): Clean abstraction for adding new backends
4. **Shadow Traffic** (ADR-031): Zero-downtime migrations like Netflix
5. **Sharding Strategy** (ADR-034): Product/feature isolation

See our [ADRs](/prism-data-layer/adr) and [RFCs](/prism-data-layer/rfc) for implementation details.

---

**Note**: This documentation is derived from Netflix's public blog posts, conference talks, and open-source contributions. All credit goes to Netflix for pioneering these patterns at scale.
