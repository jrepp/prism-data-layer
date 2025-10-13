---
title: Foundations
sidebar_position: 2
id: key-documents
project_id: prism-data-layer
doc_uuid: 939d9f73-b542-4d3c-8cd3-43cc04222098
---

# Key Philosophy & Foundational Documents

This page references the most important documents that drive the philosophy, architecture, and development practices of the Prism Data Access Layer.

---

## 🎯 Vision & Requirements

### [Product Requirements Document (PRD)](/prds/prd-001)
**Why it matters**: Defines the core value proposition, user personas, success metrics, and implementation roadmap for Prism.

**Key insights**:
- 10-100x faster than existing solutions (Netflix Data Gateway)
- Self-service data provisioning
- Zero-downtime migrations
- Sub-millisecond P50 latency target

---

## 🏛️ Architectural Foundations

### [ADR-001: Why Rust for the Proxy](/adr/adr-001)
**Why it matters**: Explains the fundamental technology choice that enables Prism's performance characteristics.

**Key decision**: Rust provides 10-100x performance improvement over JVM-based proxies with memory safety guarantees.

### [ADR-002: Client-Originated Configuration](/adr/adr-002)
**Why it matters**: Defines how applications declare their data access patterns and requirements.

**Key insight**: Applications specify requirements (RPS, consistency, latency SLOs); Prism auto-configures and provisions backends.

### [ADR-003: Protobuf as Single Source of Truth](/adr/adr-003)
**Why it matters**: Establishes the data modeling philosophy and code generation strategy.

**Key principle**: Protobuf definitions with custom tags drive code generation across all components (clients, proxy, backends).

### [ADR-004: Local-First Testing Strategy](/adr/adr-004)
**Why it matters**: Defines the development and testing philosophy that prioritizes real backends over mocks.

**Key practice**: Use real local backends (SQLite, local Kafka, PostgreSQL in Docker) for realistic testing.

---

## 🔧 Implementation Philosophy

### [MEMO-004: Backend Plugin Implementation Guide](/memos/memo-004)
**Why it matters**: Comprehensive backend comparison and implementation priorities.

**Key content**:
- 8 backends ranked by implementability
- Go SDK quality, data models, testing difficulty analysis
- Demo plugin configurations
- Implementation phases and priorities

### [MEMO-006: Backend Interface Decomposition & Schema Registry](/memos/memo-006)
**Why it matters**: Defines the three-layer schema architecture and backend interface decomposition strategy.

**Key architecture**:
- Layer 1: Backend Capabilities (KeyValue, PubSub, Timeseries, Locks)
- Layer 2: Proxy DAL Patterns (KeyValue, Entity, TimeSeries, Graph)
- Layer 3: Client Protocols (application-specific)

### [RFC-018: POC Implementation Strategy](/rfc/rfc-018)
**Why it matters**: Defines the incremental POC approach with Walking Skeleton methodology.

**Key strategy**:
- POC 1: KeyValue + MemStore (foundation)
- POC 2: KeyValue + Redis (real backend)
- POC 3: PubSub + NATS (messaging pattern)
- POC 4: Multicast Registry (composite pattern)
- POC 5: Authentication (security + multi-tenancy)

---

## 📚 Development Practices

### [CLAUDE.md (in repository root)](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md)
**Why it matters**: Comprehensive development guide for contributors and AI assistants.

**Key practices**:
- Documentation validation workflow (mandatory before commits)
- Test-Driven Development (TDD) with code coverage requirements
- Git commit best practices
- Architecture Decision Records (ADR) process
- Monorepo structure and navigation

---

## 🧪 Testing & Quality

### [MEMO-015: Cross-Backend Acceptance Test Framework](/memos/memo-015)
**Why it matters**: Establishes the table-driven, property-based testing approach for backend compliance.

**Key innovation**: Single test suite automatically validates all backends (Redis, MemStore, PostgreSQL) with randomized data.

### [MEMO-016: Observability & Lifecycle Implementation](/memos/memo-016)
**Why it matters**: Documents the observability infrastructure and lifecycle testing framework.

**Key features**:
- OpenTelemetry tracing
- Prometheus metrics endpoints
- Graceful shutdown handling
- Zero-boilerplate backend drivers

---

## 📖 How to Use This Index

**For new contributors**:
1. Start with the [PRD](/prds/prd-001) to understand the vision
2. Read [ADR-001](/adr/adr-001) through [ADR-004](/adr/adr-004) for architectural foundations
3. Review [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md) for development practices
4. Explore [MEMO-004](/memos/memo-004) for implementation guidance

**For architectural decisions**:
- Review existing ADRs to understand past decisions
- Follow the ADR template when proposing new decisions
- Ensure alignment with foundational principles

**For implementation work**:
- Follow TDD practices from [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md)
- Refer to [MEMO-004](/memos/memo-004) for backend-specific guidance
- Use [MEMO-015](/memos/memo-015) test framework for new backends
- Ensure observability from [MEMO-016](/memos/memo-016) is integrated

---

## 🔗 Document Hierarchy

```text
Vision & Requirements (WHY)
├── PRD - Product vision and success criteria
│
Architecture (WHAT)
├── ADR-001 - Technology: Rust
├── ADR-002 - Pattern: Client-originated config
├── ADR-003 - Data modeling: Protobuf
├── ADR-004 - Testing: Local-first
│
Implementation (HOW)
├── MEMO-004 - Backend implementation guide
├── MEMO-006 - Three-layer schema architecture
├── RFC-018 - POC implementation strategy
│
Development Practices (WORKFLOWS)
├── CLAUDE.md - Development guide
├── MEMO-015 - Testing framework
└── MEMO-016 - Observability implementation
```

---

## 📝 Keeping This Index Updated

When creating new foundational documents:
1. Add them to this index with a brief explanation of "Why it matters"
2. Update the document hierarchy if the structure changes
3. Ensure cross-references are maintained
4. Update the changelog with notable additions

---

*Last updated: 2025-10-12*
