---
author: Platform Team
created: 2025-10-11
doc_uuid: b9a8d03d-a0b3-4b42-ba16-b8077c02844d
id: memo-012
project_id: prism-data-layer
tags:
- dx
- developer-experience
- testing
- tooling
- workflows
title: Developer Experience and Common Workflows
updated: 2025-10-11
---

# MEMO-012: Developer Experience and Common Workflows

## Purpose

Document common commands, testing patterns, and workflows used daily in Prism development.

## Core Commands

### Documentation

```bash
# Validate docs before commit (MANDATORY)
uv run tooling/validate_docs.py

# Build and serve docs locally
cd docusaurus && npm run build && npm run serve

# Fix broken links
uv run tooling/fix_doc_links.py
```

### Pattern Development

```bash
# Build all patterns
cd patterns && make build

# Watch for changes and auto-rebuild
cd patterns && go run ./watcher --reload

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run with race detector
go test -race ./...

# Coverage enforcement
make coverage-sdk        # Core SDK (85% min)
make coverage-memstore   # MemStore (85% min)
```

### Proxy

```bash
# Run proxy locally
cd proxy && cargo run --release

# Run tests
cd proxy && cargo test --workspace
```

### Load Testing

```bash
# Start backends
docker compose up redis nats

# Run load test
cd cmd/prism-loadtest
go run . mixed -r 100 -d 60s \
  --redis-addr localhost:6379 \
  --nats-servers nats://localhost:4222
```

## Mental Models

### Three-Layer Testing

**Layer 1: Unit Tests** (Fast, no network)
- In-memory backends (MemStore, SQLite)
- Run constantly during development
- `go test ./storage/...`

**Layer 2: Integration Tests** (Medium, local network)
- Real backends in Docker (Redis, NATS)
- Run before commits
- `go test ./tests/integration/...`

**Layer 3: Load Tests** (Slow, full system)
- Multiple patterns + proxy
- Run before merges
- `prism-loadtest mixed -r 100 -d 60s`

### TDD Workflow

```bash
# 1. Write test (red)
vim storage/keyvalue_test.go
go test ./storage  # Should fail

# 2. Implement (green)
vim storage/keyvalue.go
go test ./storage  # Should pass

# 3. Check coverage
go test -cover ./storage
# coverage: 85.7% of statements

# 4. Commit with coverage
git commit -m "Implement KeyValue storage (coverage: 85.7%)"
```

## Speed Optimization Techniques

### Skip Full Validation During Iteration

```bash
# Fast: skip Docusaurus build
uv run tooling/validate_docs.py --skip-build

# Full: includes build (pre-commit)
uv run tooling/validate_docs.py
```

### Parallel Testing

```bash
# Run all pattern tests in parallel
cd patterns
go test ./memstore/... ./redis/... ./kafka/... -p 3
```

### Incremental Builds

```bash
# Watch mode rebuilds only changed files
cd patterns && go run ./watcher --reload

# In another terminal, edit files
vim redis/storage.go
# Watcher automatically rebuilds redis pattern
```

### Reuse Running Backends

```bash
# Start once, leave running
docker compose up -d redis nats postgres

# Run multiple test iterations without restart
go test ./tests/integration/... # Uses running containers
```

### Coverage Without HTML

```bash
# Quick: just print total
go test -cover ./... | grep coverage

# Detailed: per-function breakdown
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
```

## Common Shortcuts

### Alias Setup

```bash
# Add to ~/.bashrc or ~/.zshrc
alias prism="uv run --with prismctl prism"
alias validate-docs="uv run tooling/validate_docs.py"
alias build-patterns="cd ~/dev/prism/patterns && make build"
```

### Docker Compose Profiles

```bash
# Start only what you need
docker compose up redis        # Just Redis
docker compose up redis nats   # Redis + NATS
docker compose up              # Everything
```

### Go Test Shortcuts

```bash
# Test single package
go test ./storage

# Test with verbose output
go test -v ./storage

# Test single function
go test -run TestKeyValueStore_Set ./storage

# Benchmark single function
go test -bench=BenchmarkGet -benchmem ./storage
```

## Integration Test Setup

### Multicast Registry Pattern

```bash
# Terminal 1: Start backends
docker compose up redis nats

# Terminal 2: Run coordinator tests
cd patterns/multicast-registry
go test ./tests/integration/...

# Or run load test
cd cmd/prism-loadtest
go run . mixed -r 100 -d 10s
```

### Quick Smoke Test

```bash
# Verify all components build
make -C patterns build && \
cargo build --manifest-path proxy/Cargo.toml && \
echo "✅ All components build successfully"
```

## Documentation Workflow

### Creating New Docs

```bash
# 1. Create file with frontmatter
vim docs-cms/memos/MEMO-XXX-my-topic.md

# 2. Validate locally
uv run tooling/validate_docs.py --skip-build

# 3. Fix any errors
uv run tooling/fix_doc_links.py  # If link errors

# 4. Full validation before commit
uv run tooling/validate_docs.py

# 5. Commit
git add docs-cms/memos/MEMO-XXX-my-topic.md
git commit -m "Add MEMO-XXX documenting <topic>"
```

### Frontmatter Templates

**ADR**:
```yaml
---
title: "ADR-XXX: Title"
status: Proposed | Accepted | Implemented
date: 2025-10-11
deciders: Core Team
tags: [architecture, backend]
id: adr-xxx
---
```

**RFC**:
```yaml
---
title: "RFC-XXX: Title"
status: Proposed | Accepted | Implemented
author: Name
created: 2025-10-11
updated: 2025-10-11
tags: [design, api]
id: rfc-xxx
---
```

**MEMO**:
```yaml
---
title: "MEMO-XXX: Title"
author: Platform Team
created: 2025-10-11
updated: 2025-10-11
tags: [implementation, testing]
id: memo-xxx
---
```

## Performance Testing

### Benchmark Comparison

```bash
# Baseline
go test -bench=. -benchmem ./... > old.txt

# After changes
go test -bench=. -benchmem ./... > new.txt

# Compare
benchcmp old.txt new.txt
```

### Load Test Profiles

```bash
# Quick validation (10s)
prism-loadtest mixed -r 100 -d 10s

# Standard test (60s)
prism-loadtest mixed -r 100 -d 60s

# Stress test (5m)
prism-loadtest mixed -r 500 -d 5m
```

## Debugging

### gRPC Tracing

```bash
# Enable gRPC logging
export GRPC_GO_LOG_VERBOSITY_LEVEL=99
export GRPC_GO_LOG_SEVERITY_LEVEL=info
go test ./tests/integration/...
```

### Race Detector

```bash
# Always run before commit
go test -race ./...

# In CI (mandatory)
make test-race
```

### Container Logs

```bash
# Follow specific service
docker compose logs -f redis

# All services
docker compose logs -f

# Last 100 lines
docker compose logs --tail=100
```

## CI/CD

### Pre-Commit Checklist

```bash
# 1. Tests pass
go test ./...

# 2. Race detector clean
go test -race ./...

# 3. Coverage meets threshold
make coverage-all

# 4. Documentation valid
uv run tooling/validate_docs.py

# 5. All builds succeed
make -C patterns build
cargo build --manifest-path proxy/Cargo.toml
```

### Fast Iteration Loop

```bash
# Option 1: Watch + Test
cd patterns && go run ./watcher --reload &
watch -n 2 'go test ./memstore/...'

# Option 2: Single command
cd patterns/memstore && \
  while true; do \
    inotifywait -e modify *.go && \
    go test ./...; \
  done
```

## Related Documentation

- [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md) - Complete project guidance
- [RFC-018: POC Implementation Strategy](/rfc/rfc-018) - Development phases
- [MEMO-010: Load Test Results](/memos/memo-010) - Performance baselines
- [ADR-004: Local-First Testing](/adr/adr-004) - Testing philosophy

## Summary

**Most Common Commands**:
1. `uv run tooling/validate_docs.py` - Before every commit
2. `go test -race ./...` - Before every commit
3. `make coverage-<component>` - Verify thresholds
4. `docker compose up redis nats` - Start backends once, reuse
5. `go run ./watcher --reload` - Watch mode for rapid iteration

**Speed Tips**:
- Skip full validation during iteration (`--skip-build`)
- Reuse running Docker containers
- Test single packages instead of `./...`
- Use watch mode for auto-rebuild

**Mental Model**:
- Unit tests (fast) → Integration tests (medium) → Load tests (slow)
- TDD: red → green → refactor (with coverage in commit message)
- Documentation: write → validate → fix → validate → commit