# CLAUDE.md

This file provides guidance to Claude Code when working with the Prism data access gateway.

---

## 🚨 CRITICAL REQUIREMENT: Documentation Validation 🚨

**MANDATORY BEFORE ANY DOCUMENTATION PUSH OR COMMIT**

```bash
# THIS IS A BLOCKING REQUIREMENT - NEVER SKIP
# MUST use "uv run" - script will FAIL without proper dependencies
uv run tooling/validate_docs.py
```

**YOU MUST**:
1. ✅ **ALWAYS use `uv run`** - Script requires pydantic and python-frontmatter dependencies
2. ✅ Run `uv run tooling/validate_docs.py` BEFORE committing documentation changes
3. ✅ Fix ALL errors reported by validation (frontmatter, links, MDX syntax)
4. ✅ Ensure validation passes with "✅ SUCCESS" message
5. ❌ **NEVER run `python3 tooling/validate_docs.py` directly** - will fail with exit code 2
6. ❌ NEVER commit/push documentation if validation fails
7. ❌ NEVER skip validation "to save time" or "fix later"

**Why "uv run" is mandatory**:
- Script uses **strict validation mode only** (no fallback)
- Requires `pydantic` for frontmatter schema validation
- Requires `python-frontmatter` for YAML parsing
- Running without `uv run` will immediately fail with clear error message
- This prevents local validation passing while CI fails

**Why this is non-negotiable**:
- MDX compilation errors break GitHub Pages builds
- Broken links create 404s for users
- Unescaped `<` and `>` characters cause build failures
- Missing frontmatter fields (author, created, updated) cause schema errors
- Pushing broken docs wastes CI/CD resources and delays deployment

**If validation fails**:
1. Fix errors immediately (frontmatter fields, MDX escaping, broken links)
2. Re-run validation with `uv run tooling/validate_docs.py` until it passes
3. Only then proceed with git commit/push

---

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
├── docs-cms/              # 📚 AUTHORITATIVE DOCUMENTATION SOURCE
│   ├── intro.md           # Documentation overview
│   ├── adr/               # Architecture Decision Records
│   ├── rfcs/              # Request for Comments
│   ├── memos/             # Design memos and diagrams
│   └── netflix/           # Netflix Data Gateway research
├── docusaurus/            # Docusaurus site configuration
│   └── docs/              # Docusaurus-specific docs (changelog, etc.)
│       └── changelog.md   # 📝 CANONICAL CHANGELOG
├── docs/                  # Built docs (GitHub Pages output)
├── admin/                 # FastAPI-based admin UI
├── prismctl/              # Go CLI for Prism (OIDC auth, namespace management)
├── prism-proxy/           # Rust high-performance gateway
├── patterns/               # Go backend plugins (containers)
│   ├── core/              # Shared plugin package
│   ├── postgres/          # PostgreSQL plugin
│   ├── kafka/             # Kafka plugin
│   ├── redis/             # Redis plugin
│   └── watcher/           # File watcher for hot reload
├── proto/                 # Protobuf definitions (source of truth)
├── tooling/               # Python utilities for repo management
├── tests/                 # Integration and load tests
└── pyproject.toml         # Python tooling dependencies (uv)
```

### 📚 Documentation Authority: `./docs-cms/` and `./docusaurus/docs/`

**IMPORTANT**: Documentation is split between two locations:

- **Source files**: Most markdown/MDX files in `docs-cms/` (version controlled)
  - `docs-cms/adr/` - Architecture Decision Records
  - `docs-cms/rfcs/` - Request for Comments (design specs)
  - `docs-cms/memos/` - Technical memos with diagrams
  - `docs-cms/netflix/` - Netflix Data Gateway research notes
  - `docs-cms/intro.md` - Documentation landing page

- **Docusaurus-specific docs**: `docusaurus/docs/` (version controlled)
  - `docusaurus/docs/changelog.md` - **📝 CANONICAL CHANGELOG** (add entries here when making changes)

- **Build output**: `docs/` directory (gitignored, generated by Docusaurus)

- **Configuration**: `docusaurus/` directory (site config)

**🚨 CRITICAL: When making documentation changes, ALWAYS update `docusaurus/docs/changelog.md` with a summary of your changes.**

**Documentation link format** (CRITICAL for Docusaurus):

Docusaurus uses **lowercase IDs from frontmatter** (e.g., `id: rfc-015`) to generate URLs.

✅ **Correct format** (absolute path + lowercase):
- `[RFC-015](/rfc/rfc-015)` - matches frontmatter `id: rfc-015`
- `[ADR-001](/adr/adr-001)` - matches frontmatter `id: adr-001`
- `[MEMO-004](/memos/memo-004)` - matches frontmatter `id: memo-004`

❌ **Wrong formats** (will break):
- `./RFC-015-plugin-acceptance-test-framework.md` - relative paths don't work
- `../rfcs/RFC-015-plugin-acceptance-test-framework.md` - cross-plugin relative paths fail
- `/rfc/RFC-015` - uppercase doesn't match frontmatter ID

**Fix broken links automatically**:
```bash
# Converts relative .md links and uppercase IDs to correct format
uv run tooling/fix_doc_links.py
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

# Bootstrap the environment (creates ~/.prism directory)
uv sync
uv run tooling/bootstrap.py

# Start Podman machine (required for testcontainers)
podman machine start

# Verify Podman is running and DOCKER_HOST is set (Makefile does this automatically)
make env

# If running testcontainers outside Makefile, set manually:
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
```

The bootstrap script creates:
```
~/.prism/
├── config.yaml          # Admin CLI configuration
├── token                # OIDC token cache
└── patterns/             # Standard plugin manifests
    ├── postgres.yaml    # PostgreSQL plugin spec
    ├── kafka.yaml       # Kafka plugin spec
    └── redis.yaml       # Redis plugin spec
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

### Parallel Testing (⚡ 40%+ Faster)

**NEW**: Parallel test runner significantly reduces test time (17min → 10min) using fork-join execution:

```bash
# Run all tests in parallel (fastest)
make test-parallel

# Run only fast tests (skip acceptance)
make test-parallel-fast

# Run with fail-fast (stop on first failure)
make test-parallel-fail-fast

# List available test suites
uv run tooling/parallel_test.py --list

# Run specific categories
uv run tooling/parallel_test.py --categories unit,lint

# See comprehensive docs
cat tooling/PARALLEL_TESTING.md
```

**Key Features**:
- ✅ **1.7x faster**: Tests run in parallel with max 8 concurrent jobs
- ✅ **Isolated logs**: Each test writes to separate log file in `test-logs/`
- ✅ **Fail-fast mode**: Stop on first failure for quick feedback
- ✅ **JSON reports**: Machine-readable test results for CI/CD
- ✅ **Smart dependencies**: Tests with `depends_on` wait for prerequisites
- ✅ **Resource groups**: Conflicting tests run serially within group

**Development Workflow**:
```bash
# During active development (fast feedback)
make test-parallel-fast --fail-fast

# Before commit (full validation)
make test-parallel

# Debug specific failure
cat test-logs/acceptance-redis.log
```

### Common Commands

```bash
# Build prismctl CLI
make build-prismctl

# Use prismctl (after building)
build/binaries/prismctl --help
build/binaries/prismctl login
build/binaries/prismctl namespace list
build/binaries/prismctl health

# Or install to $GOPATH/bin
cd prismctl && make install
prismctl --help

# Build backend plugins
cd patterns && make build

# Watch plugins for changes and auto-rebuild
cd patterns && go run ./watcher --reload

# Run proxy locally
cd prism-proxy && cargo run --release

# Run admin UI
cd admin && npm run dev

# Deploy to staging
uv run tooling/deploy.py --env staging
```

### Parallel Linting (⚡ 54-90x Faster)

**NEW**: Parallel linting runner dramatically reduces linting time (45-75 min → 3-4 sec) by running linter categories concurrently:

```bash
# Run all linters in parallel (fastest!)
make lint-parallel

# Run critical linters only (fast feedback, 1-2 sec)
make lint-parallel-critical

# Run all linters (traditional sequential, ~45+ min)
make lint

# Auto-fix issues across all languages
make lint-fix

# List all linter categories
make lint-parallel-list
```

**Key Features**:
- ✅ **54-90x faster**: 45+ linters run in 3.7s vs 45-75 min sequential
- ✅ **10 categories**: critical, security, style, quality, errors, performance, bugs, testing, maintainability, misc
- ✅ **Multi-module support**: Automatically discovers and lints all 15+ Go modules in monorepo
- ✅ **JSON output parsing**: Structured issue reporting with file, line, column details
- ✅ **Progress tracking**: Real-time status updates during execution
- ✅ **CI optimization**: Matrix strategy runs categories in parallel on GitHub Actions

**Linter Categories**:
```bash
# Critical (6 linters, must pass): errcheck, govet, ineffassign, staticcheck, unused
# Security (2 linters): gosec, copyloopvar
# Style (6 linters): gofmt, gofumpt, goimports, gci, whitespace, wsl
# Quality (8 linters): goconst, gocritic, gocyclo, gocognit, cyclop, dupl, revive, stylecheck
# Errors (3 linters): errorlint, err113, wrapcheck
# Performance (3 linters): prealloc, bodyclose, noctx
# Bugs (8 linters): asciicheck, bidichk, durationcheck, makezero, nilerr, nilnil, rowserrcheck, sqlclosecheck
# Testing (3 linters): testpackage, paralleltest, testifylint
# Maintainability (4 linters): funlen, maintidx, nestif, lll
# Misc (7 linters): misspell, nakedret, predeclared, tagliatelle, unconvert, unparam, wastedassign
```

**Development Workflow**:
```bash
# During active development (fast feedback, 1-2 sec)
make lint-parallel-critical

# Before commit (full validation, 3-4 sec)
make lint-parallel

# Auto-fix issues where possible
make lint-fix
```

**Configuration Files**:
- `.golangci.yml`: golangci-lint v2.5.0 configuration (45+ linters)
- `ruff.toml`: Python linting/formatting (30+ rule sets)
- `tooling/parallel_lint.py`: AsyncIO-based parallel runner

See [MEMO-021](https://jrepp.github.io/prism-data-layer/memos/memo-021) for comprehensive parallel linting documentation.

## Automation with uv

**IMPORTANT**: We automate common tasks using Python scripts invoked via `uv run`. This provides:
- Zero-setup execution (uv handles dependencies)
- Consistent tooling across environments
- Fast iteration with Python's flexibility

### Documentation Tools

**⚠️ CRITICAL WORKFLOW: Documentation Validation is MANDATORY**

See the [CRITICAL REQUIREMENT section at the top of this file](#-critical-requirement-documentation-validation-) for full details.

```bash
# 🚨 BLOCKING REQUIREMENT - Run before committing documentation
# ⚠️  MUST use "uv run" - script will FAIL without it
uv run tooling/validate_docs.py

# Development iteration (faster, skip build)
uv run tooling/validate_docs.py --skip-build

# Verbose debugging
uv run tooling/validate_docs.py --verbose
```

**What validation checks**:
- ✓ **Frontmatter schema** (ADR: title, status, date, deciders, tags, id)
- ✓ **Frontmatter schema** (RFC: title, status, author, created, updated, tags, id)
- ✓ **Frontmatter schema** (MEMO: title, author, created, updated, tags, id)
- ✓ Internal/external links (no 404s)
- ✓ MDX syntax compatibility (catches `<` and `>` issues that break builds)
- ✓ Code block language labels (prevents MDX parsing errors)
- ✓ Cross-plugin link problems (relative paths across plugins don't work)
- ✓ TypeScript compilation (docusaurus config)
- ✓ Full Docusaurus build (ensures GitHub Pages will succeed)

**REMEMBER**:
- ❌ **NEVER use `python3 tooling/validate_docs.py` directly** - will fail immediately
- ❌ NEVER commit documentation without running validation first
- ❌ NEVER push if validation fails
- ✅ **ALWAYS use `uv run`** for proper dependency loading
- ✅ ALWAYS fix errors before proceeding
- ✅ ALWAYS verify "✅ SUCCESS" message before git commit

**Other documentation tools**:
```bash
# Build documentation site locally
cd docusaurus && npm run build

# Serve documentation locally
cd docusaurus && npm run serve

# Convert documents to frontmatter format (if needed)
uv run tooling/convert_to_frontmatter.py

# Fix broken doc links (relative paths, wrong case)
uv run tooling/fix_doc_links.py

# Comprehensive migration (frontmatter + links + consistency)
uv run tooling/migrate_docs_format.py [--dry-run] [--verbose]
```

**Migration script** (`migrate_docs_format.py`):
- Ensures frontmatter has correct lowercase IDs (adr-001, rfc-015, memo-004)
- Fixes titles to include proper ID prefix (ADR-001:, RFC-015:, MEMO-004:)
- Converts all links to absolute lowercase paths
- Can run in `--dry-run` mode to preview changes
- Use `--verbose` to see detailed changes

**When to use migration script**:
- After adding new documentation files
- When importing external docs
- Before major releases to ensure consistency
- If validation reports frontmatter errors

### Git Hooks

Enable automatic validation on commit:

```bash
# Install git hooks
git config core.hooksPath .githooks

# Hooks will run automatically on:
# - pre-commit: Validates markdown files
```

### Creating New Automation Scripts

When adding new tooling:

1. **Use uv run pattern**:
   ```python
   #!/usr/bin/env python3
   """
   Script description.

   Usage:
       uv run tooling/my_script.py [args]
   """
   import argparse
   from pathlib import Path
   ```

2. **Add to CLAUDE.md** under this section

3. **Make executable**: `chmod +x tooling/my_script.py`

4. **Test directly**: `uv run tooling/my_script.py`

### Why uv for Automation?

- **No venv management**: uv handles dependencies automatically
- **Fast**: Sub-second cold starts
- **Portable**: Works on any system with uv installed
- **CI-friendly**: Easy GitHub Actions integration

## Test-Driven Development (TDD) Workflow

**CRITICAL**: All Go code MUST be developed using TDD with mandatory code coverage tracking.

### Core Principles

1. **Write Tests First** (Red Phase)
   - Define test case for new feature BEFORE implementation
   - Run test (should fail - no implementation yet)
   - Commit: "Add failing test for <feature>"

2. **Implement Minimal Code** (Green Phase)
   - Write simplest code to make test pass
   - Run test (should pass)
   - Commit: "Implement <feature> to pass tests (coverage: XX%)"

3. **Refactor** (Refactor Phase)
   - Improve code quality
   - Run tests (should still pass)
   - Commit: "Refactor <feature> for clarity (coverage: XX%)"

### Code Coverage Requirements

**MANDATORY**: All Go components must meet coverage thresholds before merge.

| Component Type | Minimum Coverage | Target Coverage |
|----------------|------------------|-----------------|
| Core Plugin SDK | 85% | 90%+ |
| Plugins (complex) | 80% | 85%+ |
| Plugins (simple) | 85% | 90%+ |
| Utilities | 90% | 95%+ |

**Enforcement**: CI builds FAIL if coverage drops below minimum.

### Coverage Commands

```bash
# Generate coverage report for a package
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
go tool cover -func=coverage.out | grep total

# Makefile targets (use these in CI)
make coverage-sdk        # Core Plugin SDK
make coverage-memstore   # MemStore plugin
make coverage-redis      # Redis plugin
make coverage-kafka      # Kafka plugin
make coverage-all        # All components

# CI enforcement example
make coverage-sdk
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
if (( $(echo "$COVERAGE < 85" | bc -l) )); then
  echo "SDK coverage ${COVERAGE}% < 85%"
  exit 1
fi
```

### TDD Development Cycle Example

```bash
# 1. Write test first (Red)
cat > storage/keyvalue_test.go <<EOF
func TestKeyValueStore_SetGet(t *testing.T) {
    store := NewKeyValueStore()
    err := store.Set("key1", []byte("value1"), 0)
    if err != nil {
        t.Fatalf("Set failed: %v", err)
    }
    value, found := store.Get("key1")
    if !found {
        t.Fatal("Key not found")
    }
    if string(value) != "value1" {
        t.Errorf("Expected value1, got %s", value)
    }
}
EOF

# 2. Run test (should fail - no implementation)
go test ./storage
# FAIL: undefined: NewKeyValueStore

# 3. Implement minimal code (Green)
cat > storage/keyvalue.go <<EOF
package storage

import "sync"

type KeyValueStore struct {
    data sync.Map
}

func NewKeyValueStore() *KeyValueStore {
    return &KeyValueStore{}
}

func (kv *KeyValueStore) Set(key string, value []byte, ttl int64) error {
    kv.data.Store(key, value)
    return nil
}

func (kv *KeyValueStore) Get(key string) ([]byte, bool) {
    value, ok := kv.data.Load(key)
    if !ok {
        return nil, false
    }
    return value.([]byte), true
}
EOF

# 4. Run test (should pass)
go test ./storage
# PASS

# 5. Check coverage
go test -cover ./storage
# coverage: 85.7% of statements

# 6. Commit with coverage in message
git add storage/
git commit -m "Implement KeyValue storage (coverage: 85.7%)"
```

### Coverage in Pull Requests

Every PR MUST include coverage report in description:

```markdown
## Coverage Report

| Component | Coverage | Change | Status |
|-----------|----------|--------|--------|
| Core SDK | 87.3% | +2.1% | ✅ Pass (>85%) |
| MemStore | 89.1% | +3.4% | ✅ Pass (>85%) |
| Redis | 82.5% | +1.8% | ✅ Pass (>80%) |
| Kafka | 78.2% | -1.2% | ❌ Fail (<80%) |

**Action Required**: Kafka coverage dropped below 80%. Need to add tests for error handling paths.
```

### CI/CD Coverage Enforcement

```yaml
# .github/workflows/ci.yml
name: CI with Coverage

on: [push, pull_request]

jobs:
  test-and-coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run tests with coverage
        run: make coverage-all

      - name: Enforce coverage thresholds
        run: |
          # Core SDK: 85% minimum
          cd patterns/core
          COVERAGE=$(go test -coverprofile=coverage.out ./... && \
                     go tool cover -func=coverage.out | grep total | \
                     awk '{print $3}' | sed 's/%//')
          if (( $(echo "$COVERAGE < 85" | bc -l) )); then
            echo "❌ Core SDK coverage ${COVERAGE}% < 85%"
            exit 1
          fi
          echo "✅ Core SDK coverage ${COVERAGE}% >= 85%"

          # MemStore: 85% minimum
          cd ../memstore
          COVERAGE=$(go test -coverprofile=coverage.out ./... && \
                     go tool cover -func=coverage.out | grep total | \
                     awk '{print $3}' | sed 's/%//')
          if (( $(echo "$COVERAGE < 85" | bc -l) )); then
            echo "❌ MemStore coverage ${COVERAGE}% < 85%"
            exit 1
          fi
          echo "✅ MemStore coverage ${COVERAGE}% >= 85%"

          # Redis/Kafka: 80% minimum (more complex, lower threshold)
          # ... similar checks

      - name: Upload coverage to Codecov
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
          fail_ci_if_error: true
```

### When to Write Tests

**ALWAYS write tests for**:
- Public API functions and methods
- Business logic and algorithms
- Error handling paths
- Edge cases (nil, empty, boundary values)
- Concurrent operations (use `-race` detector)

**Optional tests for**:
- Simple getters/setters (but consider coverage impact)
- Trivial type conversions
- Generated code (protobuf)

**NEVER skip tests for**:
- Storage operations (Set, Get, Delete, etc.)
- Network operations (connection pools, retries)
- Lifecycle hooks (startup, shutdown)
- Authorization checks
- Data serialization/deserialization

### Coverage Gap Analysis

Use coverage reports to identify untested code:

```bash
# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Open in browser and look for RED lines (untested code)
open coverage.html

# Example output:
#   storage/keyvalue.go:42: untested error path
#   storage/keyvalue.go:67: untested TTL cleanup logic
```

Add tests to cover red lines until coverage meets threshold.

### Race Detection

**MANDATORY**: Run all tests with race detector in CI.

```bash
# Local development
go test -race ./...

# CI enforcement
- name: Run tests with race detector
  run: go test -race -v ./...
```

Race detector MUST be clean (no data races) before merge.

### Benchmarking (Optional but Recommended)

Write benchmarks for performance-critical paths:

```go
func BenchmarkKeyValueStore_Set(b *testing.B) {
    store := NewKeyValueStore()
    for i := 0; i < b.N; i++ {
        store.Set(fmt.Sprintf("key%d", i), []byte("value"), 0)
    }
}

func BenchmarkKeyValueStore_Get(b *testing.B) {
    store := NewKeyValueStore()
    store.Set("key", []byte("value"), 0)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Get("key")
    }
}
```

Run benchmarks to validate performance claims:

```bash
go test -bench=. -benchmem ./...

# Example output:
# BenchmarkKeyValueStore_Set-8     5000000    230 ns/op    48 B/op   2 allocs/op
# BenchmarkKeyValueStore_Get-8    10000000    156 ns/op     0 B/op   0 allocs/op
```

### Summary: TDD Checklist

Before committing code:

- [ ] All tests written BEFORE implementation
- [ ] All tests passing (`go test ./...`)
- [ ] Coverage meets minimum threshold (`make coverage-<component>`)
- [ ] Race detector clean (`go test -race ./...`)
- [ ] Coverage percentage in commit message
- [ ] PR includes coverage report

## Architecture Decision Records (ADRs)

All significant architectural decisions are documented in `docs-cms/adr/`.

**Documentation structure:**
- `docs-cms/` - Source markdown files (version controlled)
  - `adr/` - Architecture Decision Records
  - `rfcs/` - Request for Comments
  - Other documentation
- `docs/` - Built Docusaurus site (generated, Git Pages)
- `docusaurus/` - Docusaurus configuration

Template: `docs-cms/adr/000-template.md`

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
uv run tooling/convert_to_frontmatter.py
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
