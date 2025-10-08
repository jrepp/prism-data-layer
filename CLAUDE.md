# CLAUDE.md

This file provides guidance to Claude Code when working with the Prism data access gateway.

## Project Purpose

Prism is a high-performance data access layer gateway that sits between applications and heterogeneous data backends. Inspired by Netflix's Data Gateway but designed for superior performance and developer experience.

**Core Mission**: Provide a unified, client-configurable interface to multiple data backends while maintaining security, observability, and operational simplicity.

## Architecture Philosophy

### Design Principles

1. **Performance First**: Rust-based proxy for maximum throughput and minimal latency
2. **Client-Originated Configuration**: Applications declare their data access patterns; Prism provisions and optimizes
3. **Local-First Testing**: Prioritize local resources (sqlite, local kafka) over mocks for realistic testing
4. **Pluggable Backends**: Clean abstraction layer allows adding backends without changing application code
5. **DRY via Code Generation**: Protobuf definitions with custom tags drive code generation across components

### Netflix Data Gateway Learnings

From Netflix's implementation, we adopt:
- **Data Abstraction Layers (DAL)**: KeyValue, TimeSeries, Graph, Entity patterns
- **Declarative Configuration**: Runtime and deployment configurations separate concerns
- **Shadow Traffic**: Enable zero-downtime migrations between backends
- **Sharding for Isolation**: Single-tenant deployments prevent noisy neighbor problems
- **Namespace Abstraction**: Decouple logical data models from physical storage

Our improvements over Netflix:
- **Rust Proxy**: 10-100x performance improvement over JVM-based proxies
- **Client Configuration**: Applications specify requirements (RPS, consistency, latency SLOs); Prism auto-configures
- **Unified Testing Framework**: Built-in load testing with local backends from day one
- **Smaller Footprint**: No Kubernetes dependency; runs on bare metal, VMs, or containers

## Monorepo Structure

Keep the tree shallow for easy navigation:

```
prism/
├── CLAUDE.md              # This file - project guidance
├── README.md              # Quick start and overview
├── docs/                  # Deep-dive documentation
│   ├── adr/               # Architecture Decision Records
│   ├── requirements/      # Requirements documents
│   └── netflix/           # Reference materials (PDFs, transcripts)
├── admin/                 # Ember-based admin UI
├── proxy/                 # Rust high-performance gateway
├── backends/              # Backend implementations
│   ├── kafka/
│   ├── nats/
│   ├── postgres/
│   ├── sqlite/
│   └── neptune/
├── proto/                 # Protobuf definitions (source of truth)
├── tooling/               # Python utilities for repo management
├── tests/                 # Integration and load tests
└── pyproject.toml         # Python tooling dependencies (uv)
```

## Core Requirements

### Security

**CRITICAL: Never commit credentials, API keys, or secrets.**

- All auth handled via mTLS or OAuth2
- PII tagging in protobuf definitions drives automatic handling
- Audit logging for all data access
- Per-namespace authorization policies

### Data Backends

Initial supported backends (pluggable architecture allows easy additions):

1. **Kafka**: Event streaming, append-only logs
2. **NATS**: Lightweight messaging, pub/sub
3. **PostgreSQL**: Relational data, strong consistency
4. **SQLite**: Local testing, embedded use cases
5. **Neptune (AWS)**: Graph data

Each backend has:
- **Producers**: Write operations (insert, update, delete, append)
- **Consumers**: Read operations (get, scan, query, subscribe)
- **Configuration**: Connection, capacity, consistency settings

### Protobuf Strategy

Protobuf is the lingua franca of Prism:

```protobuf
message UserProfile {
  string user_id = 1 [(prism.index) = "primary"];
  string email = 2 [(prism.pii) = "email"];
  string name = 3 [(prism.pii) = "name"];
  int64 created_at = 4 [(prism.index) = "secondary"];

  option (prism.backend) = "postgres";
  option (prism.cache) = "true";
  option (prism.consistency) = "strong";
}
```

Custom tags enable:
- **Code generation**: Client libraries, proxy routes, backend adapters
- **PII handling**: Automatic encryption, masking, audit
- **Index creation**: Backend-specific optimizations
- **Data attribution**: Track data lineage and transformations

## Development Workflow

### Setup

```bash
# Install uv for Python dependency management
curl -LsSf https://astral.sh/uv/install.sh | sh

# Bootstrap the environment
uv sync
python -m tooling.bootstrap
```

### Testing Philosophy

**Avoid mocks. Use real local backends.**

```bash
# Start local backends (sqlite, local kafka, local postgres)
python -m tooling.test.local-stack up

# Run integration tests against local backends
cargo test --workspace

# Run load tests
python -m tooling.test.load-test --scenario high-throughput
```

### Common Commands

```bash
# Generate code from proto definitions
python -m tooling.codegen

# Run proxy locally
cd proxy && cargo run --release

# Run admin UI
cd admin && ember serve

# Deploy to staging
python -m tooling.deploy --env staging
```

## Architecture Decision Records (ADRs)

All significant architectural decisions are documented in `docs/adr/`.

Template: `docs/adr/000-template.md`

### ADR Format

All ADRs and RFCs use **YAML frontmatter** for metadata:

```markdown
---
title: "ADR-XXX: Descriptive Title"
status: Accepted
date: 2025-10-08
deciders: Core Team
tags: [architecture, backend, security]
---

## Context

[Description of the problem and requirements...]

## Decision

[What was decided...]

## Consequences

[Impact of the decision...]
```

**Frontmatter Fields:**
- `title`: Full title including ADR/RFC number
- `status`: Proposed | Accepted | Implemented | Deprecated | Superseded
- `date`: Decision date (ISO 8601)
- `deciders`: Team or individuals who made the decision
- `tags`: Array of topic tags for categorization
- `author` (RFCs): Document author
- `created`/`updated` (RFCs): Timestamp fields

**Converting to Frontmatter:**
```bash
python3 tooling/convert_to_frontmatter.py
```

Key ADRs:
- **ADR-001**: Why Rust for the proxy
- **ADR-002**: Client-originated configuration design
- **ADR-003**: Protobuf as single source of truth
- **ADR-004**: Local-first testing strategy
- **ADR-005**: Backend plugin architecture

## Requirements Process

Requirements live in `docs/requirements/`:

- **FR-001-data-model.md**: Core data abstractions (KeyValue, TimeSeries, Graph, Entity)
- **FR-002-auth.md**: Authentication and authorization
- **FR-003-logging.md**: Structured logging and audit trails
- **FR-004-pii.md**: PII handling, encryption, compliance
- **NFR-001-performance.md**: Latency, throughput targets
- **NFR-002-reliability.md**: Availability, durability, consistency

Each requirement:
1. Starts as a discussion document
2. Gets refined through code examples and prototypes
3. Results in ADRs and implementation tasks

## Key Technologies

- **Rust**: Proxy implementation (tokio, tonic for gRPC, axum for HTTP)
- **Ember.js**: Admin UI (modern Ember with TypeScript)
- **Protobuf**: Data models and service definitions
- **Python + uv**: Tooling and orchestration
- **Docker Compose**: Local testing infrastructure
- **GitHub Actions**: CI/CD

## Contributing

1. Create ADR for significant changes
2. Update requirements docs as understanding evolves
3. Generate code from proto definitions (never hand-write generated code)
4. Write tests using local backends
5. Run load tests to validate performance claims

## Git Commit Best Practices

**CRITICAL**: All commits must be concise and include the original user prompt.

**Format**: `<action> <subject>` on first line, blank line, body with user prompt

**Actions**: add, implement, update, fix, refactor, remove, document

**Required Structure**:
```
<Action> <concise subject without period>

User request: "<exact user prompt or paraphrased intent>"

<Optional: 1-2 sentence explanation of implementation approach>

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

**Examples**:

```
Add Rust proxy skeleton with gRPC server

User request: "Create the initial Rust proxy with basic gRPC setup"

Initializes Rust workspace with tokio and tonic dependencies.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

```
Implement KeyValue DAL with sqlite backend

User request: "Build the first data abstraction using SQLite for local testing"

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

**Rules**:
- **ALWAYS include "User request:" line with the original prompt** (most important!)
- Keep subject line concise (under 50 chars when possible)
- Capitalize first word
- No period at end of subject
- Body wrapped at 72 chars
- Focus on what/why, not how
- Separate logical changes into distinct commits
- User prompt provides context for why the change was made
