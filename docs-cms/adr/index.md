---
id: adr-index
title: Architecture Decision Records
sidebar_position: 0
slug: /
---

# Architecture Decision Records (ADRs)

This section documents all significant architectural decisions made in the Prism Data Access Layer project. Each ADR captures the context, decision, alternatives considered, and consequences of a specific architectural choice.

## What is an ADR?

An Architecture Decision Record (ADR) is a document that captures an important architectural decision along with its context and consequences. ADRs help teams:

- **Understand** why certain decisions were made
- **Evaluate** alternatives that were considered
- **Learn** from past decisions
- **Onboard** new team members to the project's architectural philosophy

## ADR Format

Each ADR follows a consistent structure:

1. **Context**: The problem or situation that requires a decision
2. **Decision**: The architectural choice that was made
3. **Rationale**: Why this decision was made (alternatives considered)
4. **Consequences**: Impact of the decision (positive, negative, neutral)

## Browse ADRs

Use the sidebar to browse all ADRs by category, or explore some key decisions:

### Foundation
- [ADR-001: Rust for Proxy](/adr/adr-001) - Why Rust over Java/Go/C++
- [ADR-003: Protobuf Single Source of Truth](/adr/adr-003) - Using protobuf for data models
- [ADR-004: Local-First Testing](/adr/adr-004) - Real backends over mocks

### Architecture
- [ADR-002: Client-Originated Configuration](/adr/adr-002) - Applications declare requirements
- [ADR-005: Backend Plugin Architecture](/adr/adr-005) - Pluggable backend design
- [ADR-008: Observability Strategy](/adr/adr-008) - Logging, metrics, tracing

### Security
- [ADR-007: Authentication & Authorization](/adr/adr-007) - mTLS and OAuth2
- [ADR-006: Namespace Multi-Tenancy](/adr/adr-006) - Tenant isolation

### Operations
- [ADR-009: Shadow Traffic Migrations](/adr/adr-009) - Zero-downtime backend migrations
- [ADR-010: Caching Layer](/adr/adr-010) - Performance optimization strategy

### Development Process
- [ADR-012: Go for Backend Plugins](/adr/adr-012) - Why Go for plugins vs Rust
- [ADR-015: Go Testing Strategy](/adr/adr-015) - Test-driven development approach
- [ADR-040: UV-Only Python Tooling](/adr/adr-040) - No system Python packages, uv for everything
- [ADR-041: Pattern Acceptance Test Framework](/adr/adr-041) - Cross-backend test automation
- [ADR-043: Hygienic Build System](/adr/adr-043) - Out-of-source builds in `build/` directory
- [ADR-049: Podman Container Optimization](/adr/adr-049) - Scratch containers and MicroVMs

### Local Development
- [ADR-036: SQLite Config Storage](/adr/adr-036) - Local-first configuration
- [ADR-044: Prismctl CLI with Typer](/adr/adr-044) - Python CLI design
- [ADR-048: Local Signoz Observability](/adr/adr-048) - OpenTelemetry + Signoz for local testing
- [ADR-016: Local Development Infrastructure](/adr/adr-016) - Dex, Signoz, service lifecycle

### Performance & Reliability
- [ADR-011: Implementation Roadmap](/adr/adr-011) - Phased delivery strategy
- [ADR-013: Error Handling Strategy](/adr/adr-013) - Structured errors with retryability
- [ADR-020: Parallel Testing Infrastructure](/adr/adr-020) - Fork-join test execution
- [ADR-021: Parallel Linting System](/adr/adr-021) - 54-90x faster linting

### Documentation & Governance
- [ADR-042: MDX Docusaurus Migration](/adr/adr-042) - Documentation platform choice
- [ADR-050: Documentation Validation Pipeline](/adr/adr-050) - Automated link checking, frontmatter validation

## ADR Status

ADRs can have the following statuses:

- **Proposed**: Under discussion, not yet decided
- **Accepted**: Decision made and documented
- **Implemented**: Decision implemented in code
- **Deprecated**: No longer applicable
- **Superseded**: Replaced by a newer ADR

## Contributing

When making a significant architectural decision:

1. Create a new ADR using the template: `ADR-000-template.md`
2. Number it sequentially (next available number)
3. Fill in all sections with context and rationale
4. Submit for review with the core team
5. Update status as the decision progresses

---

**Total ADRs**: 50+ decisions documented

**Latest Updates**: See the [Changelog](/docs/changelog) for recent ADRs
