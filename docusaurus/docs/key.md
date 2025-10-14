---
title: Foundations
sidebar_position: 2
id: key-documents
project_id: prism-data-layer
doc_uuid: 939d9f73-b542-4d3c-8cd3-43cc04222098
---

# Essential Reading Guide

Get up to speed on Prism fundamentals through this curated reading path. Each section builds on the previous, taking you from vision to implementation in ~45 minutes of focused reading.

---

## TL;DR: What is Prism?

**The Problem**: Netflix-scale organizations need a unified data access layer, but existing solutions (Netflix's Data Gateway) are too slow and hard to self-service.

**The Solution**: A Rust-based proxy that's 10-100x faster, with client-originated configuration that enables team self-service while maintaining policy guardrails.

**The Key Insight**: Separate **what** (client APIs), **how** (patterns), and **where** (backends) into three layers. Applications declare needs; Prism handles backend selection, provisioning, and reliability patterns.

**Performance**: P50 <1ms, P99 <10ms, 200k+ RPS per proxy instance.

---

## The Learning Journey

### Phase 1: The Vision (10 min)

**Read**: [Product Requirements Document (PRD)](/prds/prd-001)

**Why start here**: Understand the problem Prism solves, who it's for, and what success looks like.

**Key takeaways**:
- **50x team scaling**: Infrastructure team of 10 supports 500+ app teams
- **Zero-downtime migrations**: Swap backends without client changes
- **Self-service provisioning**: Teams declare needs; platform provisions resources
- **Success metrics**: <1ms P50 latency, 10k+ RPS, 99.99% uptime

**After reading**: You'll understand *why* Prism exists and *who* benefits.

---

### Phase 2: Core Decisions (15 min)

Read these four ADRs in order - they establish Prism's technical foundation:

#### [ADR-001: Why Rust for the Proxy](/adr/adr-001) (4 min)

**The choice**: Rust instead of JVM (Go/Java/Scala)

**The rationale**:
- 10-100x performance improvement (P50: 1ms vs 5-50ms)
- Memory safety without GC pauses
- 20MB idle memory vs 500MB+ JVM

**Key quote**: "Performance is a feature. Users perceive <100ms as instant; every millisecond counts."

---

#### [ADR-002: Client-Originated Configuration](/adr/adr-002) (4 min)

**The choice**: Applications declare requirements; Prism provisions infrastructure

**The rationale**:
- Applications know their needs best (RPS, consistency, latency)
- Platform team sets policy boundaries (approved backends, cost limits)
- Self-service scales to hundreds of teams

**Example**:
```protobuf
message UserEvents {
  option (prism.access_pattern) = "append_heavy";
  option (prism.estimated_write_rps) = "10000";
  option (prism.retention_days) = "90";
}
// Prism selects: Kafka with 20 partitions
```

---

#### [ADR-003: Protobuf as Single Source of Truth](/adr/adr-003) (4 min)

**The choice**: Protobuf definitions with custom tags drive all code generation

**The rationale**:
- DRY principle: Define once, generate everywhere
- Type safety across languages
- Custom tags drive: indexing, PII handling, backend selection

**Example**:
```protobuf
message UserProfile {
  string email = 2 [
    (prism.pii) = "email",
    (prism.encrypt_at_rest) = true,
    (prism.mask_in_logs) = true
  ];
}
// Generates: Encryption, masked logging, audit trails
```

---

#### [ADR-004: Local-First Testing Strategy](/adr/adr-004) (3 min)

**The choice**: Real local backends (SQLite, Docker Postgres) instead of mocks

**The rationale**:
- Mocks hide backend-specific behavior
- Local backends catch integration bugs early
- Testcontainers make this practical

**Key practice**: If you can't test it locally, you can't test it in CI.

---

### Phase 3: System Architecture (10 min)

**Read**: [MEMO-006: Backend Interface Decomposition & Schema Registry](/memos/memo-006)

**Why read this**: Understand the three-layer architecture that makes backend swapping possible.

**The three layers**:

```text
Layer 3: Client Protocols (Application APIs)
         ↓
Layer 2: Proxy DAL Patterns (KeyValue, Entity, TimeSeries, Graph)
         ↓
Layer 1: Backend Capabilities (45 thin interfaces)
```

**Key insight**: Patterns compose backend interfaces to provide higher-level abstractions.

**Example**: Multicast Registry pattern uses:
- `keyvalue_basic` (for registration storage)
- `pubsub_basic` (for event distribution)
- `queue_basic` (for durability)

Same pattern works with Redis+NATS+Postgres OR DynamoDB+SNS+SQS by swapping Layer 1 backends.

---

### Phase 4: Implementation Roadmap (10 min)

#### [RFC-018: POC Implementation Strategy](/rfc/rfc-018) (7 min)

**Why read this**: See how we're building Prism incrementally with Walking Skeleton approach.

**The 5 POCs**:
1. **KeyValue + MemStore** (2 weeks) - Simplest possible end-to-end
2. **KeyValue + Redis** (2 weeks) - Real backend + acceptance testing
3. **PubSub + NATS** (2 weeks) - Messaging pattern
4. **Multicast Registry** (3 weeks) - Composite pattern (KeyValue + PubSub + Queue)
5. **Authentication** (2 weeks) - Security + multi-tenancy

**Key principle**: Build thinnest possible slice end-to-end, then iterate.

---

#### [MEMO-004: Backend Plugin Implementation Guide](/memos/memo-004) (3 min)

**Why skim this**: See backend priorities and implementability rankings.

**Quick reference**:
- **Highest priority**: MemStore, Kafka, NATS, PostgreSQL (internal needs)
- **External priorities**: Redis, SQLite, S3/MinIO, ClickHouse
- **Implementability ranking**: MemStore (easiest) → Neptune (hardest)

**Use this when**: Choosing which backend to implement next.

---

## Development Practices (5 min)

### [CLAUDE.md (Repository Root)](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md)

**Why read this**: Your guide to contributing to Prism.

**Essential sections**:
1. **Documentation Validation** - Mandatory before committing docs
2. **TDD Workflow** - Red/Green/Refactor with coverage requirements
3. **Git Commit Format** - Concise messages with user prompts
4. **Monorepo Structure** - Where things live

**Coverage requirements**:
- Core Plugin SDK: 85% minimum, 90% target
- Plugins (complex): 80% minimum, 85% target
- Plugins (simple): 85% minimum, 90% target

**Key quote**: "Write tests first. If you can't test it locally, you can't test it in CI."

---

## Your Reading Path

### New to Prism? (35 min)

Follow this sequence to build complete understanding:

1. **Vision** (10 min): [PRD](/prds/prd-001)
2. **Decisions** (15 min): [ADR-001](/adr/adr-001), [ADR-002](/adr/adr-002), [ADR-003](/adr/adr-003), [ADR-004](/adr/adr-004)
3. **Architecture** (10 min): [MEMO-006](/memos/memo-006)
4. **Development** (5 min): [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md)

**After this**: You'll understand Prism's vision, technical foundation, architecture, and development practices.

---

### Implementing a Feature? (20 min)

Start with implementation context:

1. **POC Strategy** (7 min): [RFC-018](/rfc/rfc-018) - Which POC phase are we in?
2. **Backend Guide** (3 min): [MEMO-004](/memos/memo-004) - Backend-specific guidance
3. **Testing Framework** (5 min): [MEMO-015](/memos/memo-015) - How to write acceptance tests
4. **Observability** (5 min): [MEMO-016](/memos/memo-016) - Add tracing/metrics

**Then**: Follow TDD workflow from [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md)

---

### Writing an ADR/RFC? (10 min)

Understand the decision-making context:

1. **Read existing ADRs**: See [ADR Index](/adr) for past decisions
2. **Check RFC precedents**: See [RFC Index](/rfc) for design patterns
3. **Use templates**: `docs-cms/adr/ADR-000-template.md` for ADRs

**Key principle**: Every significant architectural decision gets an ADR. Every feature design gets an RFC.

---

## Quick Reference

### Most Referenced Documents

| Document | Purpose | When to Read |
|----------|---------|--------------|
| [PRD](/prds/prd-001) | Product vision | Onboarding, strategic decisions |
| [ADR-001](/adr/adr-001) - [ADR-004](/adr/adr-004) | Core decisions | Understanding technical foundation |
| [MEMO-006](/memos/memo-006) | Three-layer architecture | Designing patterns, understanding backend abstraction |
| [RFC-018](/rfc/rfc-018) | POC roadmap | Planning implementation work |
| [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md) | Development guide | Daily development, code reviews |

---

### Additional Deep Dives

Once you're comfortable with foundations, explore these for deeper understanding:

- **Testing**: [MEMO-015](/memos/memo-015) - Cross-backend acceptance testing, [MEMO-030](/memos/memo-030) - Pattern-based test migration
- **Security**: [MEMO-031](/memos/memo-031) - RFC-031 security review, [ADR-050](/adr/adr-050) - Authentication strategy
- **Performance**: [MEMO-007](/memos/memo-007) - Podman container optimization, [ADR-049](/adr/adr-049) - Container strategy
- **Observability**: [MEMO-016](/memos/memo-016) - OpenTelemetry integration, [RFC-016](/rfc/rfc-016) - Local dev infrastructure

---

## Document Evolution

This guide evolves as the project grows. When adding foundational documents:

1. **Place in narrative**: Where does it fit in the learning journey?
2. **Add time estimate**: How long to read/understand?
3. **Explain "Why read this"**: What understanding does it unlock?
4. **Update reading paths**: Does it change the recommended sequence?

**Principle**: Every document should have a clear purpose in someone's learning journey.

---

*Reading time estimates assume focused reading with note-taking. Skim faster if reviewing familiar concepts.*

*Last updated: 2025-10-14*
