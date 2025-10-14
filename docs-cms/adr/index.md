---
id: adr-index
title: Architecture Decision Records
sidebar_position: 0
slug: /
---

# Architecture Decision Records (ADRs)

Architecture Decision Records document significant architectural choices made in Prism. Each ADR captures the problem context, decision rationale, alternatives considered, and consequences—creating a historical record of "why" behind the architecture.

## 🎯 New to Prism? Start Here

If you're exploring Prism's architecture, start with these foundational decisions:

1. **[ADR-001: Rust for Proxy](/adr/adr-001)** - Why Rust powers Prism's core
2. **[ADR-003: Protobuf Single Source of Truth](/adr/adr-003)** - Data modeling philosophy
3. **[ADR-002: Client-Originated Configuration](/adr/adr-002)** - How apps declare requirements
4. **[ADR-005: Backend Plugin Architecture](/adr/adr-005)** - Pluggable backend design

## 📚 Reading Paths by Intent

### Understanding the Core Architecture

Learn the foundational decisions that shaped Prism:

- **[ADR-001: Rust for Proxy](/adr/adr-001)** - Why Rust over Java/Go/C++
- **[ADR-003: Protobuf Single Source of Truth](/adr/adr-003)** - Using protobuf for everything
- **[ADR-005: Backend Plugin Architecture](/adr/adr-005)** - Extensible plugin system
- **[ADR-008: Observability Strategy](/adr/adr-008)** - Logging, metrics, tracing approach

### Building Backend Plugins

Decisions that affect plugin development:

- **[ADR-012: Go for Backend Plugins](/adr/adr-012)** - Why Go for plugins vs Rust
- **[ADR-015: Go Testing Strategy](/adr/adr-015)** - TDD and coverage requirements
- **[ADR-041: Pattern Acceptance Test Framework](/adr/adr-041)** - Automated cross-backend testing
- **[ADR-043: Hygienic Build System](/adr/adr-043)** - Out-of-source builds in `build/`

### Deploying and Operating Prism

Operational decisions for platform engineers:

- **[ADR-006: Namespace Multi-Tenancy](/adr/adr-006)** - Tenant isolation strategy
- **[ADR-007: Authentication & Authorization](/adr/adr-007)** - mTLS and OAuth2 security
- **[ADR-009: Shadow Traffic Migrations](/adr/adr-009)** - Zero-downtime backend changes
- **[ADR-048: Local Signoz Observability](/adr/adr-048)** - OpenTelemetry + Signoz setup

### Setting Up Local Development

Decisions that affect your dev environment:

- **[ADR-036: SQLite Config Storage](/adr/adr-036)** - Local-first configuration
- **[ADR-044: Prismctl CLI with Typer](/adr/adr-044)** - Python CLI design
- **[ADR-040: UV-Only Python Tooling](/adr/adr-040)** - No system Python packages
- **[ADR-049: Podman Container Optimization](/adr/adr-049)** - Scratch containers and MicroVMs

## 📖 ADRs by Category

### 🏛️ Foundation Decisions

Core technology choices that define Prism's architecture:

- **[ADR-001: Rust for Proxy](/adr/adr-001)** (Accepted) - 10-100x performance over JVM, memory safety, zero-cost abstractions
- **[ADR-003: Protobuf Single Source of Truth](/adr/adr-003)** (Accepted) - Type safety, code generation, backward compatibility
- **[ADR-004: Local-First Testing](/adr/adr-004)** (Accepted) - Real backends over mocks for realistic tests
- **[ADR-012: Go for Backend Plugins](/adr/adr-012)** (Accepted) - Ecosystem, testability, developer productivity

### 🏗️ Architecture Patterns

How Prism is structured and organized:

- **[ADR-002: Client-Originated Configuration](/adr/adr-002)** (Accepted) - Applications declare their requirements
- **[ADR-005: Backend Plugin Architecture](/adr/adr-005)** (Accepted) - Clean separation between proxy and backends
- **[ADR-010: Caching Layer](/adr/adr-010)** (Proposed) - Look-aside and write-through strategies
- **[ADR-011: Implementation Roadmap](/adr/adr-011)** (Accepted) - Phased delivery strategy

### 🔒 Security & Multi-Tenancy

Authentication, authorization, and isolation:

- **[ADR-006: Namespace Multi-Tenancy](/adr/adr-006)** (Accepted) - Logical isolation for multiple tenants
- **[ADR-007: Authentication & Authorization](/adr/adr-007)** (Accepted) - mTLS for service-to-service, OAuth2 for users
- **[ADR-050: Topaz for Policy-Based Authorization](/adr/adr-050)** (Accepted) - Relationship-based access control (ReBAC)

### 🔧 Operations & Reliability

How Prism runs in production:

- **[ADR-008: Observability Strategy](/adr/adr-008)** (Accepted) - Structured logging, metrics, distributed tracing
- **[ADR-009: Shadow Traffic Migrations](/adr/adr-009)** (Accepted) - Zero-downtime backend migrations
- **[ADR-013: Error Handling Strategy](/adr/adr-013)** (Accepted) - Structured errors with retryability signals
- **[ADR-048: Local Signoz Observability](/adr/adr-048)** (Accepted) - OpenTelemetry + Signoz for local testing

### 🧪 Testing & Quality Assurance

How we ensure code quality and correctness:

- **[ADR-015: Go Testing Strategy](/adr/adr-015)** (Accepted) - TDD with 80%+ coverage requirements
- **[ADR-041: Pattern Acceptance Test Framework](/adr/adr-041)** (Accepted) - Cross-backend test automation
- **[ADR-020: Parallel Testing Infrastructure](/adr/adr-020)** (Accepted) - Fork-join execution (40% faster)
- **[ADR-021: Parallel Linting System](/adr/adr-021)** (Accepted) - 54-90x faster linting (45min → 3-4sec)

### 🛠️ Developer Tooling

Tools and workflows for local development:

- **[ADR-036: SQLite Config Storage](/adr/adr-036)** (Accepted) - Local-first configuration database
- **[ADR-040: UV-Only Python Tooling](/adr/adr-040)** (Accepted) - Modern Python dependency management
- **[ADR-043: Hygienic Build System](/adr/adr-043)** (Accepted) - Out-of-source builds in `build/`
- **[ADR-044: Prismctl CLI with Typer](/adr/adr-044)** (Accepted) - Python CLI for namespace management
- **[ADR-049: Podman Container Optimization](/adr/adr-049)** (Accepted) - Minimal scratch containers (87% reduction)

### 📚 Documentation & Process

How we document and share knowledge:

- **[ADR-042: MDX Docusaurus Migration](/adr/adr-042)** (Accepted) - Static site generation with React components
- **[ADR-050: Documentation Validation Pipeline](/adr/adr-050)** (Accepted) - Automated link checking and frontmatter validation
- **[ADR-016: Local Development Infrastructure](/adr/adr-016)** (Proposed) - Dex, Signoz, service lifecycle management

## 🔄 ADR Status Meanings

ADRs progress through these states:

- **Proposed** → Under discussion, not yet decided
- **Accepted** → Decision made and documented
- **Implemented** → Decision implemented in code
- **Deprecated** → No longer applicable
- **Superseded** → Replaced by a newer ADR (with reference)

## 💡 Why ADRs Matter

ADRs help teams:

- **Understand** why certain decisions were made (prevents revisiting settled debates)
- **Evaluate** alternatives that were considered (shows due diligence)
- **Learn** from past decisions (builds institutional knowledge)
- **Onboard** new team members to architectural philosophy (accelerates ramp-up)

## 📝 Contributing ADRs

When making a significant architectural decision:

1. **Create** a new ADR using the template: `docs-cms/adr/000-template.md`
2. **Number** it sequentially (next available ADR-XXX number)
3. **Structure** it with: Context, Decision, Rationale, Consequences
4. **Submit** for review with the core team
5. **Update** status as the decision progresses

### What Deserves an ADR?

Write an ADR when:

- Choosing between technology alternatives (e.g., Rust vs Go)
- Defining system-wide patterns (e.g., error handling)
- Making security or compliance decisions
- Establishing development workflows (e.g., testing strategy)
- Selecting third-party tools/services

Don't write an ADR for:

- Implementation details (use code comments)
- Temporary workarounds (use TODOs)
- Personal preferences (use code reviews)

## 🔗 Related Documentation

- **[RFCs](/rfc/)** - Detailed technical specifications for features
- **[MEMOs](/memos/)** - Implementation diagrams and visual documentation
- **[Changelog](/docs/changelog)** - Recent documentation updates

---

**Total ADRs**: 50+ architectural decisions documented

**Latest Updates**: See the [Changelog](/docs/changelog) for recent ADRs
