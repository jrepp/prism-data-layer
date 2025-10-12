---
date: 2025-10-09
deciders: Platform Team
doc_uuid: 96198d1b-0f98-49f9-8487-e22667179e24
id: adr-049
project_id: prism-data-layer
status: Accepted
tags:
- containers
- podman
- testing
- performance
- dx
- macos
title: 'ADR-049: Podman and Container Optimization for Instant Testing'
---

# ADR-049: Podman and Container Optimization for Instant Testing

## Status

**Accepted** - 2025-10-09

## Context

### Problem Statement

Developers need the **fastest possible build-test cycle** for backend plugin development. Current Docker-based workflow has several pain points:

**Performance Issues:**
- Docker Desktop on Mac requires a VM (HyperKit/Virtualization.framework)
- Container startup time: 3-30 seconds depending on backend
- Docker daemon overhead: ~2GB RAM baseline
- Layer caching misses during development
- Volume mount performance degradation (osxfs)

**Developer Experience Issues:**
- Docker Desktop licensing changes (free for individuals, paid for enterprises)
- Docker daemon must be running (background process)
- Root-level daemon security concerns
- OCI compliance questions

**Testing Workflow:**
```bash
# Current slow path (Docker)
docker-compose up -d postgres  # 5-10 seconds
go test ./...                   # Test execution
docker-compose down             # 2-3 seconds
# Total: 7-13 seconds overhead per cycle
```

**Desired Workflow:**
```bash
# Instant testing (goal)
go test ./...  # <1 second total
```

### Requirements

1. **Minimize VM overhead**: Reduce or eliminate VM layer where possible
2. **Optimize container size**: Smallest possible images (&lt;10MB for Go binaries)
3. **Instant startup**: Container/process startup &lt;100ms
4. **Mac-native development**: Optimize for macOS developers
5. **CI/CD parity**: Local testing matches CI environment
6. **Zero licensing concerns**: Open source, no enterprise restrictions

### Constraints

**Technical Reality:**
- **Linux containers on Mac REQUIRE a VM** - Mac kernel ≠ Linux kernel
- No way to run Linux binaries natively on macOS
- Any container runtime on Mac uses a hypervisor (Virtualization.framework, HyperKit, etc.)

**Options Evaluated:**
1. Docker Desktop (current)
2. Podman + podman machine
3. Colima (Lima-based)
4. MicroVMs (Firecracker, Cloud Hypervisor)
5. Native macOS binaries (no containers)

## Decision

**We will adopt a layered testing strategy:**

### Layer 1: In-Process Testing (Instant) 🔥 **PRIMARY**

For rapid iteration, use **zero-container** testing:

```go
// Instant: No containers, pure Go
func TestMemStore(t *testing.T) {
    store := memstore.NewMemStore()  // In-process
    // ... tests run in <1ms
}

func TestSQLite(t *testing.T) {
    db := sql.Open("sqlite3", ":memory:")  // In-process
    // ... tests run in <10ms
}
```

**Backends supporting instant testing:**
- ✅ **MemStore**: Pure Go, sync.Map (ADR: see MEMO-004)
- ✅ **SQLite**: Embedded, no external process
- ✅ **Embedded NATS**: `server.NewServer()` in-process
- ✅ **Mock backends**: For unit tests

**Benefits:**
- Startup time: **&lt;1ms**
- No VM overhead
- No container images needed
- Perfect for TDD workflow

### Layer 2: Podman for Integration Testing (Fast)

For backends requiring real services, use **Podman**:

**Why Podman over Docker:**
- ✅ **Daemonless**: No background daemon required
- ✅ **Rootless**: Runs without root privileges
- ✅ **Open source**: Apache 2.0 license, no enterprise restrictions
- ✅ **OCI-compliant**: Drop-in replacement for Docker
- ✅ **Docker compatible**: `alias docker=podman` works
- ✅ **Smaller footprint**: No daemon overhead

**Podman on Mac:**
```bash
# Install
brew install podman

# Initialize VM (one-time, uses Lima/QEMU)
podman machine init --cpus 4 --memory 4096 --disk-size 50

# Start VM (boots in ~5 seconds)
podman machine start

# Use like Docker
podman run -d postgres:16-alpine
podman-compose up -d
```

**Reality Check:**
- Podman on Mac **still uses a VM** (qemu + Virtualization.framework)
- No escape from VM requirement for Linux containers
- **Advantage**: Lighter than Docker Desktop, daemonless, rootless

### Layer 3: Optimized Container Images (Smallest)

**Container Size Optimization:**

```dockerfile
# BEFORE: Alpine-based (15MB compressed, 45MB uncompressed)
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o plugin ./cmd/server

FROM alpine:latest
COPY --from=builder /app/plugin /plugin
ENTRYPOINT ["/plugin"]
# Size: ~15MB

# AFTER: Distroless (8MB compressed, 12MB uncompressed)
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o plugin ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/plugin /plugin
USER nonroot:nonroot
ENTRYPOINT ["/plugin"]
# Size: ~8MB

# BEST: Scratch (2MB compressed, 6MB uncompressed)
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w -extldflags '-static'" \
    -o plugin ./cmd/server

FROM scratch
COPY --from=builder /app/plugin /plugin
ENTRYPOINT ["/plugin"]
# Size: ~2MB (just the binary!)
```

**Size Comparison:**

| Base Image | Compressed | Uncompressed | Startup Time | Security Updates |
|------------|-----------|--------------|--------------|------------------|
| Alpine     | 15MB      | 45MB         | 500ms        | ✅ Yes (apk)     |
| Distroless | 8MB       | 12MB         | 300ms        | ✅ Yes (minimal) |
| **Scratch** | **2MB**   | **6MB**      | **100ms**    | ⚠️ Binary only   |

**Recommendation**: Use **scratch** for plugins (statically linked Go binaries).

### Layer 4: MicroVMs (Experimental)

**Firecracker/Cloud Hypervisor on Mac:**

❌ **Not practical for macOS development:**
- Firecracker requires KVM (Linux kernel module)
- Mac uses Virtualization.framework (different API)
- Would need QEMU wrapper → same VM overhead as Podman
- No significant performance gain on Mac

**Where MicroVMs help:**
- ✅ Linux CI/CD environments (GitHub Actions, AWS)
- ✅ Production Kubernetes clusters
- ❌ Mac development workflow

**Verdict**: Skip microVMs for local Mac development.

## Implementation Strategy

### Phase 1: In-Process Testing (Week 1)

**Priority: Enable instant testing for 80% of development workflow**

```go
// plugins/postgres/internal/store/store_test.go

func TestPostgresPlugin_FastPath(t *testing.T) {
    // Use in-memory SQLite as Postgres substitute
    db := sql.Open("sqlite3", ":memory:")
    defer db.Close()

    // Most SQL is compatible
    db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
    db.Exec("INSERT INTO users (id, name) VALUES (1, 'Alice')")

    // Test plugin logic without container
    // Runs in <10ms
}

func TestPostgresPlugin_RealBackend(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Use real Postgres via testcontainers
    // Only runs with: go test -v (not go test -short)
}
```

**Run modes:**
```bash
# Fast: In-process only (TDD workflow)
go test -short ./...  # <1 second

# Full: With containers (pre-commit)
go test ./...         # ~30 seconds
```

### Phase 2: Podman Migration (Week 2)

**Replace Docker with Podman:**

```bash
# Remove Docker Desktop dependency
brew uninstall --cask docker

# Install Podman
brew install podman podman-compose

# Initialize Podman machine
podman machine init prism-dev \
    --cpus 4 \
    --memory 4096 \
    --disk-size 50 \
    --rootful=false

podman machine start prism-dev
```

**Makefile updates:**
```makefile
# Old
DOCKER := docker
COMPOSE := docker-compose

# New (Podman-compatible)
CONTAINER_RUNTIME := $(shell command -v podman || command -v docker)
COMPOSE := $(shell command -v podman-compose || command -v docker-compose)

.PHONY: test-integration
test-integration:
	$(COMPOSE) -f local-dev/compose.yml up -d
	go test -v ./tests/integration/...
	$(COMPOSE) -f local-dev/compose.yml down
```

### Phase 3: Container Optimization (Week 3)

**Rebuild all plugin images with scratch base:**

```yaml
# plugins/postgres/Dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w -extldflags '-static'" \
    -o plugin-postgres ./cmd/server

FROM scratch
COPY --from=builder /app/plugin-postgres /plugin
EXPOSE 50051
ENTRYPOINT ["/plugin"]
```

**Expected improvements:**
- Image size: 45MB → 6MB (**87% reduction**)
- Pull time: 3s → 200ms
- Startup time: 500ms → 100ms
- Memory: 50MB → 10MB baseline

### Phase 4: CI/CD Optimization (Week 4)

**GitHub Actions with layer caching:**

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # Fast: In-process tests only
      - name: Unit tests (instant)
        run: go test -short -v ./...
        timeout-minutes: 1

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # Use Podman in CI (faster than Docker)
      - name: Install Podman
        run: |
          sudo apt-get update
          sudo apt-get -y install podman

      # Full: With real backends
      - name: Integration tests
        run: |
          podman-compose -f local-dev/compose.yml up -d
          go test -v ./tests/integration/...
          podman-compose -f local-dev/compose.yml down
        timeout-minutes: 5
```

## Performance Targets

### Before (Docker Desktop)

| Test Type | Startup | Execution | Teardown | Total | Frequency |
|-----------|---------|-----------|----------|-------|-----------|
| Unit (mock) | 0ms | 100ms | 0ms | **100ms** | Every save |
| Unit (SQLite) | 50ms | 200ms | 10ms | **260ms** | Every save |
| Integration (Postgres) | 5000ms | 1000ms | 2000ms | **8000ms** | Pre-commit |
| Integration (Kafka) | 30000ms | 2000ms | 3000ms | **35000ms** | Pre-commit |

### After (Podman + Optimization)

| Test Type | Startup | Execution | Teardown | Total | Frequency |
|-----------|---------|-----------|----------|-------|-----------|
| Unit (MemStore) | 0ms | 1ms | 0ms | **1ms** ⚡ | Every save |
| Unit (SQLite) | 1ms | 50ms | 1ms | **52ms** ⚡ | Every save |
| Integration (Postgres) | 3000ms | 1000ms | 500ms | **4500ms** ✅ | Pre-commit |
| Integration (Kafka) | 20000ms | 2000ms | 1000ms | **23000ms** ✅ | Pre-commit |

**Key Improvements:**
- **Instant tests**: 100ms → **1ms** (100x faster) 🔥
- **SQLite tests**: 260ms → **52ms** (5x faster)
- **Integration tests**: 8s → **4.5s** (44% faster)
- **Kafka tests**: 35s → **23s** (34% faster)

## Technical Details

### Podman Machine Configuration

**Optimal settings for Mac development:**

```bash
# ~/.config/containers/containers.conf
[containers]
netns="host"
userns="host"
ipcns="host"
utsns="host"
cgroupns="host"
cgroups="disabled"
log_driver = "k8s-file"
pids_limit = 2048

[engine]
cgroup_manager = "cgroupfs"
events_logger = "file"
runtime = "crun"  # Faster than runc

[network]
network_backend = "netavark"  # Faster than CNI
```

**VM resource allocation:**
```bash
# For 16GB Mac (adjust proportionally)
podman machine init prism-dev \
    --cpus 4 \
    --memory 4096 \
    --disk-size 50 \
    --rootful=false \
    --now
```

### Container Build Optimization

**Multi-stage build with caching:**

```dockerfile
# Stage 1: Dependencies (cached layer)
FROM golang:1.21 AS deps
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

# Stage 2: Build
FROM deps AS builder
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o plugin ./cmd/server

# Stage 3: Runtime (scratch)
FROM scratch
COPY --from=builder /app/plugin /plugin
ENTRYPOINT ["/plugin"]
```

**Build cache usage:**
```bash
# First build: ~60 seconds (downloads deps)
podman build -t plugin-postgres:latest .

# Subsequent builds: ~5 seconds (cached deps)
# Only rebuilds if source code changes
podman build -t plugin-postgres:latest .
```

### Testing Strategy

**Three-tier testing approach:**

```go
// tests/testing.go

// Tier 1: Instant (in-process)
func NewTestStore(t *testing.T) Store {
    if testing.Short() {
        return memstore.NewMemStore()  // <1ms
    }
    return newContainerStore(t)
}

// Tier 2: Fast (embedded)
func newContainerStore(t *testing.T) Store {
    if useSQLite := os.Getenv("USE_SQLITE"); useSQLite == "true" {
        db, _ := sql.Open("sqlite3", ":memory:")
        return sqlite.NewStore(db)  // <50ms
    }
    return newRealBackend(t)
}

// Tier 3: Real (testcontainers)
func newRealBackend(t *testing.T) Store {
    container := startPostgres(t)  // 3-5 seconds
    return postgres.NewStore(container.ConnectionString())
}
```

**Environment-based control:**
```bash
# Development: Instant tests only
go test -short ./...

# Pre-commit: Include embedded backends
USE_SQLITE=true go test ./...

# CI: Full integration with real backends
go test -v ./...
```

## Consequences

### Positive

1. **Instant feedback loop**: TDD with &lt;1ms test cycles
2. **No Docker Desktop dependency**: Avoid licensing concerns
3. **Smaller images**: 87% reduction in size (45MB → 6MB)
4. **Faster CI**: Parallel unit tests complete in &lt;1 second
5. **Rootless containers**: Better security posture
6. **Lower resource usage**: No Docker daemon overhead

### Negative

1. **VM still required on Mac**: Cannot eliminate VM for Linux containers
2. **Learning curve**: Developers must understand podman machine
3. **Two testing modes**: Must maintain both instant and integration paths
4. **Scratch images**: No shell for debugging (must use distroless for debug builds)
5. **Podman maturity**: Less ecosystem tooling than Docker

### Mitigations

1. **Document Podman setup**: Add to onboarding guide
2. **Makefile abstractions**: Hide container runtime details
3. **CI parity**: Use same Podman version locally and in CI
4. **Debug builds**: Provide distroless variant with shell for debugging
5. **Gradual migration**: Start with new plugins, migrate existing over time

## Alternatives Considered

### Alternative 1: Keep Docker Desktop

**Pros:**
- Familiar to all developers
- Mature ecosystem
- Good documentation

**Cons:**
- Licensing restrictions (enterprise)
- Daemon overhead (~2GB RAM)
- Slower than Podman
- Root-level daemon

**Verdict**: ❌ Rejected due to licensing and performance concerns.

### Alternative 2: Colima (Lima-based)

**Pros:**
- Docker-compatible
- Free and open source
- Good Mac integration

**Cons:**
- Another VM layer (Lima)
- Less mature than Podman
- Still requires Docker CLI

**Verdict**: ❌ Rejected - Podman is more standard.

### Alternative 3: Native macOS Binaries

**Pros:**
- No VM required
- True instant startup
- Native performance

**Cons:**
- Requires cross-compilation
- Different from production (Linux)
- Not all backends available (no Kafka for Mac ARM)
- CI/CD parity issues

**Verdict**: ✅ **Use for Tier 1 testing** (MemStore, SQLite), but not for all backends.

### Alternative 4: Remote Development (Linux VM)

**Pros:**
- Native Linux environment
- No Mac-specific issues
- True production parity

**Cons:**
- Network latency
- Requires cloud resources
- Complexity for developers

**Verdict**: ❌ Rejected - Hurts developer experience.

## Implementation Checklist

- [ ] Install Podman on all developer machines
- [ ] Configure podman machine with optimal settings
- [ ] Update all Dockerfiles to use scratch/distroless
- [ ] Create in-process test variants (MemStore, SQLite)
- [ ] Update Makefile to support both Docker and Podman
- [ ] Document testing tiers in CONTRIBUTING.md
- [ ] Update CI/CD to use Podman
- [ ] Measure and document performance improvements
- [ ] Create onboarding guide for Podman setup
- [ ] Migrate existing plugins incrementally

## Related Decisions

- [ADR-004: Local-First Testing](/adr/adr-004) - Testing philosophy
- [ADR-026: Distroless Container Images](/adr/adr-026) - Container security
- [MEMO-004: Backend Plugin Implementation Guide](/memos/memo-004-backend-plugin-implementation-guide) - MemStore for instant testing

## References

### Podman Documentation
- [Podman on macOS](https://podman.io/getting-started/installation#macos)
- [Podman Machine](https://docs.podman.io/en/latest/markdown/podman-machine.1.html)
- [Rootless Containers](https://github.com/containers/podman/blob/main/docs/tutorials/rootless_tutorial.md)

### Container Optimization
- [Distroless Images](https://github.com/GoogleContainerTools/distroless)
- [Multi-stage Builds](https://docs.docker.com/build/building/multi-stage/)
- [Go Binary Size Reduction](https://blog.filippo.io/shrinking-go-binaries/)

### MicroVMs
- [Firecracker](https://firecracker-microvm.github.io/) (Linux only)
- [Lima](https://github.com/lima-vm/lima) (Mac VM manager)

## Revision History

- 2025-10-09: Initial decision for Podman adoption and container optimization strategy