---
title: "Documentation Change Log"
description: "Recent changes to Prism documentation with quick links"
sidebar_position: 1
---

# Documentation Change Log

Quick access to recently updated documentation. Changes listed in reverse chronological order (newest first).

## Recent Changes

### 2025-10-09

#### MEMO-005: Client Protocol Design Philosophy - Composition vs Use-Case Specificity (NEW)
**Link**: [MEMO-005](/memos/MEMO-005-client-protocol-design-philosophy)

**Summary**: Comprehensive memo resolving the architectural tension between composable primitives (RFC-014) and use-case-specific protocols (RFC-017), covering:
- Context comparison: RFC-014 composable primitives vs RFC-017 use-case patterns
- Four design principles (push complexity down, developer comprehension, schema evolution, keep proxy small)
- Proposed layered API architecture: Layer 1 (generic primitives) + Layer 2 (use-case patterns)
- Pattern coordinators as plugins (not core proxy) for independent evolution
- Configuration examples showing per-namespace choice of primitives vs patterns
- Decision matrix comparing primitives-only, patterns-only, and layered approaches
- Implementation roadmap aligned with RFC-018 POCs
- Success metrics for developer experience, system complexity, and pattern adoption

**Key Innovation**: Applications choose per-namespace between Layer 1 (generic KeyValue, PubSub) for maximum control or Layer 2 (ergonomic Multicast Registry, Saga) for rapid development. Pattern coordinators are optional plugins that compose Layer 1 primitives, keeping core proxy small (~10k LOC) while providing self-documenting APIs for common use cases.

**Impact**: Resolves "composition vs use-case" design question with both layers, addressing developer simplicity (Layer 2), proxy size (plugins), and flexibility (Layer 1).

---

#### MEMO-003: Documentation-First Development Approach (NEW)
**Link**: [MEMO-003](/memos/MEMO-003-documentation-first-development)

**Summary**: Comprehensive memo defining the documentation-first development approach used in Prism, covering:
- Definition and core principles (Design in Documentation → Review → Implement → Validate)
- Notable improvements over code-first workflows with concrete examples
- Expected outcomes (faster reviews, better designs, reduced rework)
- Strategies for success (blocking requirements, design tool, living documentation)
- Validation and quality assurance (tooling/validate_docs.py)
- Metrics and success criteria (documentation coverage, build success rate, review velocity)
- Proposed improvements (code example validation, decision graph visualization, RFC-driven task generation)

**Impact**: Establishes documentation-first as the core development methodology, with validation tooling as a blocking requirement before commits.

---

#### RFC-011: Data Proxy Authentication - Secrets Provider Abstraction (EXPANDED)
**Link**: [RFC-011](/rfc/RFC-011-data-proxy-authentication)

**Summary**: Major expansion adding comprehensive secrets provider abstraction:
- Pluggable SecretsProvider trait supporting multiple secret management services
- Four provider implementations: HashiCorp Vault, AWS Secrets Manager, Google Secret Manager, Azure Key Vault
- Provider comparison matrix (dynamic credentials, auto-rotation, versioning, audit logging, cost)
- Multi-provider hybrid cloud deployment patterns
- Configuration examples for each provider
- Credential management with automatic caching and renewal

**Impact**: Enables secure credential management across cloud providers and on-premises deployments with consistent abstraction layer.

---

#### RFC-006: Admin CLI - OIDC Authentication (EXPANDED)
**Link**: [RFC-006](/rfc/rfc-006)

**Summary**: Added comprehensive OIDC authentication section covering:
- Device code flow (OAuth 2.0) for command-line SSO authentication
- Mermaid sequence diagram showing complete authentication flow
- Login/logout commands with token caching (~/.prism/token)
- Token storage security (file permissions 0600, automatic refresh)
- Authentication modes (interactive, service account, local Dex, custom issuer)
- Go implementation examples for token management
- Local development with Dex (references ADR-046)
- Principal column added to session list output
- Shadow traffic example updated to Postgres version upgrade (14 → 16) use case

**Impact**: Complete CLI authentication specification enabling secure admin access with OIDC integration and local testing support.

---

#### ADR-046: Dex IDP for Local Identity Testing (NEW)
**Link**: [ADR-046](/adr/ADR-046-dex-idp-local-testing)

**Summary**: New ADR proposing Dex as the local OIDC provider for development and testing:
- Self-hosted OIDC provider for local development (no cloud dependencies)
- Docker Compose integration with test user configuration
- Full OIDC spec support including device code flow
- Integration with prismctl for local authentication
- Testing workflow with realistic OIDC flows

**Impact**: Enables local development and testing of authentication features without external OIDC provider dependencies.

---

#### RFC-014: Layered Data Access Patterns - Client Pattern Catalog (EXPANDED)
**Link**: [RFC-014](/rfc/RFC-014-layered-data-access-patterns)

**Summary**: New RFC defining how Prism separates client API from backend implementation through pattern composition. Covers:
- Three-layer architecture (Client API, Pattern Composition, Backend Execution)
- Publisher with Claim Check pattern implementation
- Pattern layering and compatibility matrix
- Proxy internal structure with mermaid diagrams
- Authentication and authorization flow diagrams
- Pattern routing and execution strategies

**Impact**: Provides foundation for composable reliability patterns without client code changes.

---

#### RFC-011: Data Proxy Authentication - Open Questions Expanded
**Link**: [RFC-011](/rfc/RFC-011-data-proxy-authentication)

**Summary**: Added comprehensive feedback to open questions:
- Certificate Authority: Use Vault for certificate management
- Credential Caching: 24-hour default, configurable with refresh tokens
- Connection Pooling: Per-credential pooling for multi-tenancy isolation
- Fallback Auth: Fail closed with configurable grace period
- Observability: Detailed metrics for credential events and session establishment

**Impact**: Clarifies authentication implementation decisions with practical recommendations.

---

#### RFC-010: Admin Protocol with OIDC - Multi-Provider Support
**Link**: [RFC-010](/rfc/RFC-010-admin-protocol-oidc)

**Summary**: Expanded open questions with detailed answers:
- OIDC Provider Support: AWS Cognito, Azure AD, Google, Okta, Auth0, Dex
- Token Caching: 24-hour default with JWKS caching and refresh token support
- Offline Access: JWT validation with cached JWKS, security trade-offs
- Multi-Tenancy: Four mapping options (group-based, claim-based, OPA policy, tenant-scoped)
- Service Accounts: Four approaches with comparison table and best practices

**Impact**: Production-ready guidance for OIDC integration across multiple identity providers.

---

#### RFC-009: Distributed Reliability Patterns - Change Notification Graph
**Link**: [RFC-009](/rfc/RFC-009-distributed-reliability-patterns)

**Summary**: Added change notification flow diagram to CDC pattern showing:
- Change type classification (INSERT, UPDATE, DELETE, SCHEMA)
- Notification consumers (Cache Invalidator, Search Indexer, Analytics Loader, Webhook Notifier, Audit Logger)
- Data flow from PostgreSQL WAL through Kafka to downstream systems
- Key notification patterns and use cases

**Impact**: Visual guide for implementing CDC-based change notification architectures.

---

## Older Changes

### 2025-10-08

#### RFC-009: Distributed Reliability Patterns (INITIAL)
**Link**: [RFC-009](/rfc/RFC-009-distributed-reliability-patterns)

**Summary**: Initial RFC documenting 7 distributed reliability patterns:
1. Tiered Storage - Hot/warm/cold data lifecycle
2. Write-Ahead Log - Durable, fast writes
3. Claim Check - Large payload handling in messaging
4. Event Sourcing - Immutable event log as source of truth
5. Change Data Capture - Database replication without dual writes
6. CQRS - Separate read/write models
7. Outbox - Transactional messaging

**Impact**: Establishes pattern catalog for building reliable distributed systems.

---

### 2025-10-07

#### RFC-001: Prism Architecture (INITIAL)
**Link**: [RFC-001](/rfc/RFC-001-prism-architecture)

**Summary**: Foundational architecture RFC defining:
- System components and layered interface hierarchy
- Client configuration system (server vs client config)
- Session management lifecycle
- Five interface layers (Queue, PubSub, Reader, Transact, Config)
- Container plugin model for backend-specific logic
- Performance targets (P99 &lt;10ms, 10k+ RPS)

**Impact**: Core architectural vision for Prism data access gateway.

---

#### RFC-002: Data Layer Interface Specification (INITIAL)
**Link**: [RFC-002](/rfc/RFC-002-data-layer-interface)

**Summary**: Complete gRPC interface specification covering:
- Session Service (authentication, heartbeat, lifecycle)
- Queue Service (Kafka-style operations)
- PubSub Service (NATS-style wildcards)
- Reader Service (database-style paged reading)
- Transact Service (two-table transactional writes)
- Error handling and backward compatibility

**Impact**: Stable, versioned API contracts for all client interactions.

---

## How to Use This Log

1. **Quick Navigation**: Click any link to jump directly to the updated document
2. **Impact Assessment**: Each entry includes an "Impact" section explaining significance
3. **Reverse Chronological**: Newest changes at the top for easy discovery
4. **Detailed Summaries**: Key changes summarized without needing to read full docs

## Contributing Changes

When updating documentation:

1. Add entry to "Recent Changes" section (top)
2. Include: Date, Document title, Link, Summary, Impact
3. Move entries older than 30 days to "Older Changes"
4. Keep most recent 10-15 entries in "Recent Changes"

## Change Categories

- **NEW**: Brand new documentation
- **EXPANDED**: Significant additions to existing docs
- **UPDATED**: Modifications or clarifications
- **DEPRECATED**: Marked as outdated or superseded
- **REMOVED**: Deleted or consolidated
