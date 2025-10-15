# CLAUDE.md

Guidance for working with the Prism data access gateway.

---

## 🚨 CRITICAL: Documentation Validation

**MANDATORY before any documentation commit/push**

```bash
uv run tooling/validate_docs.py  # MUST use "uv run", NOT python3
```

**Requirements**:
- ✅ ALWAYS use `uv run` (script needs pydantic + python-frontmatter)
- ✅ Fix ALL errors before committing
- ✅ Verify "✅ SUCCESS" message
- ❌ NEVER commit if validation fails
- ❌ NEVER skip validation

**Why mandatory**: MDX compilation errors break GitHub Pages builds, broken links create 404s, missing frontmatter breaks schema validation.

For detailed documentation guide, use: `/doc-guide`

---

## Project Overview

**Prism**: High-performance data access gateway between applications and heterogeneous backends. Inspired by Netflix's Data Gateway with superior performance and developer experience.

**Core Mission**: Unified, client-configurable interface to multiple backends with security, observability, and operational simplicity.

### Design Principles

1. **Performance First**: Rust-based proxy for maximum throughput
2. **Client-Originated Configuration**: Apps declare patterns; Prism provisions
3. **Local-First Testing**: Real local backends, not mocks
4. **Pluggable Backends**: Clean abstraction, no app code changes
5. **DRY via Code Generation**: Protobuf drives everything

### Architecture

**Data Flow**: Pattern Commands → Prism Proxy (Rust) → Backend Drivers (Go) → Backends (Redis, NATS, etc.)

**Key Points**:
- Drivers are libraries only (no main() functions)
- Proxy loads drivers dynamically
- Pattern commands connect to proxy
- No standalone backend servers

---

## Monorepo Structure

```
prism/
├── CLAUDE.md                  # This file
├── docs-cms/                  # ADRs, RFCs, MEMOs (source)
│   └── {adr,rfcs,memos}/     # Versioned documentation
├── docusaurus/docs/           # Changelog only
│   └── changelog.md           # CANONICAL CHANGELOG - update for all changes
├── cmd/                       # CLI tools (prismctl, prism-admin, loadtest)
├── pkg/                       # Go libraries (plugin SDK, drivers)
├── patterns/                  # Pattern implementations
├── proto/                     # Protobuf (source of truth)
└── tooling/                   # Python utilities
```

**Documentation Authority**:
- `docs-cms/` - ADRs, RFCs, MEMOs (frontmatter-based)
- `docusaurus/docs/changelog.md` - **CANONICAL CHANGELOG** (always update!)
- Use absolute lowercase links: `[RFC-015](/rfc/rfc-015)`

---

## Development Workflow

### Setup

```bash
# Install uv
curl -LsSf https://astral.sh/uv/install.sh | sh

# Bootstrap environment
uv sync && uv run tooling/bootstrap.py

# Start Podman (required for testcontainers)
podman machine start
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
```

### Common Commands

```bash
# Build & run
make build-prismctl
cd prism-proxy && cargo run --release

# Test & lint (use parallel for speed)
make test-parallel              # 1.7x faster than sequential
make lint-parallel              # 54-90x faster (3.7s vs 45+ min)

# Documentation (MANDATORY before commit)
uv run tooling/validate_docs.py
```

For detailed testing/linting guide, use: `/test-lint`

### Testing Philosophy

**Avoid mocks. Use real local backends** (SQLite, local Kafka, Postgres).

```bash
make test-parallel-fast         # During development
make test-parallel              # Before commit
```

---

## Critical Workflows

### TDD (Test-Driven Development)

All Go code MUST use TDD with mandatory coverage tracking.

**Coverage Requirements**: Core SDK 85%, Plugins 80-85%, Utilities 90%

For detailed TDD guide, use: `/tdd-guide`

### Git Commits

**CRITICAL**: All commits must include the original user prompt.

**Format**: `<action> <subject>` + blank line + `User request: "<prompt>"` + Co-Authored-By

For detailed commit format, use: `/commit-format`

### Documentation Changes

**🚨 ALWAYS update `docusaurus/docs/changelog.md`** when making documentation changes.

**Workflow**:
1. Create/modify docs with proper frontmatter
2. Label ALL code blocks (use ```text for plain output)
3. Escape `<` and `>` in prose (\<5 or &lt;5)
4. Run `uv run tooling/validate_docs.py`
5. Fix errors until ✅ SUCCESS
6. Update changelog
7. Commit

---

## Key Technologies

- **Rust**: Proxy (tokio, tonic, axum)
- **Go**: Plugins, CLI tools
- **Protobuf**: Data models
- **Python + uv**: Tooling
- **Podman**: Containers (ADR-049)
- **GitHub Actions**: CI/CD

---

## Core Requirements

### Security

**CRITICAL: Never commit credentials, API keys, or secrets.**

- All auth via mTLS or OAuth2
- PII tagging in protobuf
- Audit logging for all access
- Per-namespace authorization

### Data Backends

Kafka, NATS, PostgreSQL, SQLite, Neptune (AWS)

Each has: Producers (writes), Consumers (reads), Configuration (connection, capacity)

---

## Automation with uv

**IMPORTANT**: Use `uv run` for all Python scripts (handles dependencies automatically).

```bash
# Documentation validation (MANDATORY)
uv run tooling/validate_docs.py

# Other tools
uv run tooling/fix_doc_links.py
uv run tooling/migrate_docs_format.py [--dry-run]
```

**Why uv?**: Zero venv management, fast cold starts, portable, CI-friendly

---

## Architecture Decision Records

All significant decisions in `docs-cms/adr/`.

**Format**: YAML frontmatter + Context + Decision + Consequences

**Key ADRs**:
- ADR-001: Rust for proxy
- ADR-002: Client-originated configuration
- ADR-003: Protobuf as single source of truth
- ADR-004: Local-first testing
- ADR-005: Backend plugin architecture

---

## Contributing

1. Create ADR for significant changes
2. Update requirements docs as understanding evolves
3. Generate code from proto (never hand-write generated code)
4. Write tests using local backends
5. Run load tests to validate performance

---

## Slash Commands Reference

- `/doc-guide` - Documentation best practices (frontmatter, validation, common errors)
- `/tdd-guide` - TDD workflow and coverage requirements
- `/test-lint` - Testing and linting commands
- `/commit-format` - Git commit format with examples

Use these commands for detailed guidance without cluttering this file.
