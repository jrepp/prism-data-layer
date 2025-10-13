# Building Prism

This document describes how to build and test the Prism data access gateway.

## Prerequisites

- **Rust** 1.70+ (install via [rustup](https://rustup.rs))
- **Go** 1.21+
- **Protocol Buffers** compiler (`protoc`)
- **Python** 3.10+ with `uv` package manager
- **Node.js** 18+ (for documentation)

### Quick Install Tools

```bash
make install-tools
```

This will install all required development tools including:
- Rust formatter and linter (rustfmt, clippy)
- Go protobuf generators
- Python uv package manager
- Optional: cargo-watch for live reloading

## Quick Start

### Build Everything

```bash
make
```

This is the default target and builds:
- Rust proxy (release mode)
- All Go patterns (MemStore, etc.)
- Generated protobuf code

### Run All Tests

```bash
make test
```

Runs:
- Rust proxy unit tests (18 tests)
- Go pattern tests (MemStore, Core SDK)
- Shows coverage percentages

### Run Tests in Parallel (⚡ 40%+ Faster)

```bash
# Run all tests in parallel
make test-parallel

# Run only fast tests (skip acceptance)
make test-parallel-fast

# Run with fail-fast (stop on first failure)
make test-parallel-fail-fast
```

Parallel testing reduces total test time from 17 minutes to 10 minutes by running independent test suites concurrently. See [tooling/PARALLEL_TESTING.md](tooling/PARALLEL_TESTING.md) for details.

### Run Integration Tests

```bash
make test-integration
```

Runs full end-to-end integration tests:
- Proxy spawns MemStore process
- Complete lifecycle: Initialize → Start → HealthCheck → Stop
- Requires MemStore binary to be built first

### Run Everything (CI Mode)

```bash
make test-all
```

Runs all unit tests + integration tests.

## Common Tasks

### Development Workflow

```bash
# Build in debug mode (faster compilation)
make build-dev

# Run tests continuously on file changes
make watch-test

# Format all code
make fmt

# Lint all code (parallel - fastest!)
make lint-parallel

# Lint critical linters only (fast feedback)
make lint-parallel-critical

# Lint all code (traditional sequential)
make lint

# Auto-fix linting issues
make lint-fix

# Run pre-commit checks (format + lint + test)
make pre-commit
```

**Linting Performance:**
- `make lint-parallel`: 3-4 seconds (45+ linters in 10 categories, all 15+ Go modules)
- `make lint-parallel-critical`: 1-2 seconds (critical + security only)
- `make lint`: 45+ minutes (sequential, legacy)

See [MEMO-021](https://jrepp.github.io/prism-data-layer/memos/memo-021) for comprehensive parallel linting documentation.

### Documentation

```bash
# Validate documentation (MANDATORY before committing)
make docs-validate

# Build documentation site
make docs-build

# Serve documentation locally at http://localhost:3000
make docs-serve
```

### Code Coverage

```bash
# Generate coverage reports for all components
make coverage

# MemStore coverage only (opens HTML report)
make coverage-memstore

# Core SDK coverage only
make coverage-core
```

### Cleaning

```bash
# Clean all build artifacts
make clean

# Clean only proxy
make clean-proxy

# Clean only patterns
make clean-patterns
```

## Component-Specific Targets

### Rust Proxy

```bash
make build-proxy       # Build proxy (release mode)
make test-proxy        # Run proxy unit tests
make coverage-proxy    # Generate coverage report
make watch-proxy       # Auto-rebuild on changes
```

### Go Patterns

```bash
make build-patterns    # Build all patterns
make build-memstore    # Build MemStore only
make test-patterns     # Test all patterns
make test-memstore     # Test MemStore only
make test-core         # Test Core SDK only
```

### Protobuf Generation

```bash
make proto             # Generate all protobuf code
make proto-go          # Generate Go code only
make proto-rust        # Note: Rust uses build.rs
```

## Makefile Help

View all available targets:

```bash
make help
```

## CI/CD Integration

### GitHub Actions

```yaml
- name: Build and test
  run: make ci
```

The `make ci` target runs:
1. Lint all code
2. Run all tests (unit + integration)
3. Validate documentation

### Pre-commit Hook

Add to `.git/hooks/pre-commit`:

```bash
#!/bin/bash
make pre-commit
```

## Project Structure

```
.
├── Makefile              # Root build system
├── proxy/                # Rust proxy (cargo workspace)
│   ├── Cargo.toml
│   ├── src/
│   └── tests/
├── patterns/             # Go patterns
│   ├── core/             # Shared SDK
│   ├── memstore/         # In-memory KV pattern
│   └── ...
├── proto/                # Protobuf definitions
└── docusaurus/           # Documentation site
```

## Test Coverage Requirements

| Component      | Minimum | Target |
|----------------|---------|--------|
| Rust Proxy     | N/A     | 80%+   |
| MemStore       | 80%     | 85%+   |
| Core SDK       | 85%     | 90%+   |

Run `make coverage` to see current coverage statistics.

## Troubleshooting

### Podman machine not running (testcontainers error)

If you see `panic: rootless Docker not found`, the Podman machine isn't running:

```bash
# Start Podman machine
podman machine start

# Set DOCKER_HOST for testcontainers (add to ~/.bashrc or ~/.zshrc)
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"

# Or for current session only
export DOCKER_HOST='unix:///var/folders/.../podman-machine-default-api.sock'
```

**Why this is needed**: Per [ADR-049](/docs-cms/adr/adr-049-podman-container-optimization.md), we use Podman instead of Docker Desktop. The `testcontainers-go` library needs the `DOCKER_HOST` environment variable to find the Podman socket.

**Alternative**: Run fast tests without containers:
```bash
go test -short ./...  # Skips integration tests (instant feedback)
```

### Cargo not found

```bash
# Add to your shell profile (~/.bashrc, ~/.zshrc)
export PATH="$HOME/.cargo/bin:$PATH"
```

### Protoc not found

```bash
# macOS
brew install protobuf

# Linux
apt-get install protobuf-compiler

# Or download from: https://github.com/protocolbuffers/protobuf/releases
```

### Integration tests failing

Ensure MemStore binary is built and in the correct location:

```bash
make build-memstore
ls -la patterns/memstore/memstore
```

The proxy expects the binary at `../patterns/memstore/memstore` relative to the test executable.

### Tests timing out

Integration tests spawn real processes. If you see timeout errors:

1. Kill any leftover processes: `pkill -9 memstore`
2. Increase timeout in test (default: 2 minutes)
3. Check system resources

## Performance Tips

1. **Use `make build-dev`** for faster iteration (no optimizations)
2. **Use `make watch-test`** for TDD workflow (requires cargo-watch)
3. **Run `make test` frequently** (unit tests are fast: <1 second)
4. **Run `make test-integration` less often** (slower: ~3 seconds)

## Success Criteria

✅ `make` completes without errors
✅ `make test` shows all tests passing
✅ `make test-integration` shows full lifecycle working
✅ `make docs-validate` passes (required before pushing)

## Example Session

```bash
# Start fresh
make clean

# Build everything
make

# Run unit tests
make test

# Run integration tests
make test-integration

# Generate coverage reports
make coverage

# Validate documentation
make docs-validate

# All checks before committing
make pre-commit
```

## Getting Help

- View all targets: `make help`
- CI/CD pipeline: `make ci`
- Questions: See [CLAUDE.md](./CLAUDE.md)
