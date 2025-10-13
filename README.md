# Prism

> A high-performance data access layer gateway

[![CI](https://github.com/jrepp/prism-data-layer/actions/workflows/ci.yml/badge.svg)](https://github.com/jrepp/prism-data-layer/actions/workflows/ci.yml)
[![Linting](https://github.com/jrepp/prism-data-layer/actions/workflows/pre-commit.yml/badge.svg)](https://github.com/jrepp/prism-data-layer/actions/workflows/pre-commit.yml)
[![Docs](https://github.com/jrepp/prism-data-layer/actions/workflows/docs.yml/badge.svg)](https://github.com/jrepp/prism-data-layer/actions/workflows/docs.yml)
[![GitHub Pages](https://img.shields.io/badge/docs-GitHub%20Pages-blue)](https://jrepp.github.io/prism-data-layer/)

**[📖 Documentation](https://jrepp.github.io/prism-data-layer/)** | [Architecture](https://jrepp.github.io/prism-data-layer/docs/intro) | [ADRs](https://jrepp.github.io/prism-data-layer/adr) | [RFCs](https://jrepp.github.io/prism-data-layer/rfc)

Prism is a Rust-based data access gateway that provides unified, type-safe access to heterogeneous data backends. Inspired by Netflix's Data Gateway but built for extreme performance and developer experience.

## Quick Start

```bash
# Install development tools
make install-tools

# Build everything
make

# Run all tests
make test

# Run integration tests
make test-integration
```

See **[BUILDING.md](./BUILDING.md)** for complete build and test instructions.

## What is Prism?

Prism sits between your applications and data backends, providing:

- **Unified API**: Single gRPC/HTTP interface to multiple backends (Kafka, NATS, Postgres, SQLite, Neptune)
- **Client Configuration**: Declare your data access patterns; Prism handles provisioning and optimization
- **Zero-Downtime Migrations**: Shadow traffic and declarative configuration enable seamless backend changes
- **Built-in Observability**: Structured logging, metrics, and distributed tracing out of the box
- **Type Safety**: Protobuf-first design with code generation for all components

## Architecture

```
┌─────────────┐
│  Your App   │
└──────┬──────┘
       │ gRPC/HTTP
       ▼
┌─────────────┐     ┌──────────────┐
│ Prism Proxy │────▶│   Kafka      │
│   (Rust)    │     ├──────────────┤
│             │────▶│   NATS       │
│  - Auth     │     ├──────────────┤
│  - Routing  │────▶│  Postgres    │
│  - Caching  │     ├──────────────┤
│  - Logging  │────▶│   SQLite     │
│             │     ├──────────────┤
└─────────────┘     │  Neptune     │
                    └──────────────┘
```

## Key Features

### 🚀 High Performance

- **Rust-based proxy**: 10-100x faster than JVM alternatives
- **Zero-copy when possible**: Minimize allocations and copies
- **Async throughout**: Built on tokio for efficient concurrency

### 🔌 Pluggable Backends

Each backend pattern is a self-contained Go module:

```
patterns/
├── core/       # Shared pattern SDK
├── memstore/   # In-memory key-value (testing)
├── redis/      # Redis backend
├── nats/       # Lightweight messaging
├── kafka/      # Event streaming
├── postgres/   # Relational data
└── ...         # More backends coming
```

Adding a new backend? Implement the pattern interfaces and register with the SDK.

### 🎯 Client-Originated Configuration

Instead of manual provisioning:

```protobuf
message UserEvents {
  option (prism.access_pattern) = "append_heavy";
  option (prism.estimated_rps) = "10000";
  option (prism.retention_days) = "90";
}
```

Prism automatically:
- Selects optimal backend (Kafka for append-heavy)
- Provisions capacity for 10k RPS
- Configures retention policies

### 🧪 Local-First Testing

No mocks required. Run real backends locally:

```bash
# Starts postgres, kafka, nats in Docker
python -m tooling.test.local-stack up

# Run load tests against local stack
python -m tooling.test.load-test \
  --scenario writes-heavy \
  --duration 60s \
  --target-rps 5000
```

### 🔒 Security Built-In

- **mTLS by default**: All inter-service communication encrypted
- **PII tagging**: Automatic handling of sensitive data
- **Audit logging**: Track all data access
- **Fine-grained AuthZ**: Per-namespace policies

```protobuf
message User {
  string id = 1;
  string email = 2 [(prism.pii) = "email"];  // Auto-encrypted
  string name = 3 [(prism.pii) = "name"];    // Auto-masked in logs
}
```

## Project Structure

```
prism/
├── Makefile        # Build system (run 'make help')
├── BUILDING.md     # Build and test documentation
├── CLAUDE.md       # Project philosophy and guidelines
├── proxy/          # Rust gateway (core of Prism)
├── patterns/       # Go backend patterns (pluggable)
│   ├── core/       # Shared pattern SDK
│   ├── memstore/   # In-memory key-value pattern
│   └── ...
├── proto/          # Protobuf definitions (source of truth)
├── tooling/        # Python utilities (validation, deployment)
├── docs-cms/       # Documentation source (ADRs, RFCs, memos)
├── docusaurus/     # Documentation site configuration
└── docs/           # Built documentation (GitHub Pages)
```

## Documentation

- **[BUILDING.md](./BUILDING.md)**: Build, test, and development workflow
- **[CLAUDE.md](./CLAUDE.md)**: Project philosophy and guidelines
- **[Architecture Decision Records](https://jrepp.github.io/prism-data-layer/adr)**: Design decisions
- **[RFCs](https://jrepp.github.io/prism-data-layer/rfc)**: Technical proposals
- **[GitHub Pages](https://jrepp.github.io/prism-data-layer/)**: Live documentation site

### Contributing to Documentation

**⚠️ CRITICAL: Run validation before pushing documentation changes:**

```bash
# Using Makefile (recommended)
make docs-validate

# Or directly with uv
uv run tooling/validate_docs.py
```

This validates frontmatter, links, and MDX syntax. See [CLAUDE.md](./CLAUDE.md) for details.

## Development

### Prerequisites

- Rust 1.70+ (for proxy)
- Go 1.21+ (for patterns)
- Python 3.10+ with uv (for tooling)
- Protocol Buffers compiler (protoc)
- Node.js 18+ (for documentation)

Install all tools: `make install-tools`

### Building

```bash
# Build everything (default target)
make

# Build in debug mode (faster)
make build-dev

# Build specific components
make build-proxy
make build-patterns
```

### Testing

```bash
# Run all unit tests
make test

# Run tests in parallel (40%+ faster!)
make test-parallel

# Run integration tests
make test-integration

# Run everything (CI mode)
make test-all

# Generate coverage reports
make coverage
```

### Linting

Prism uses comprehensive parallel linting for maximum speed and code quality:

```bash
# Run all linters in parallel (fastest!)
make lint-parallel

# Run critical linters only (fast feedback)
make lint-parallel-critical

# Run all linters (traditional sequential)
make lint

# Auto-fix issues
make lint-fix

# List all linter categories
make lint-parallel-list
```

**45+ Go linters** across 10 categories run in parallel (3-4s vs 45+ min sequential). See [MEMO-021](https://jrepp.github.io/prism-data-layer/memos/memo-021) for details.

See **[BUILDING.md](./BUILDING.md)** for complete documentation on building, testing, and development workflow.

## Roadmap

### Phase 1: Foundation (Current)

- [ ] Rust proxy skeleton with gRPC server
- [ ] SQLite backend (simplest for testing)
- [ ] Basic KeyValue abstraction
- [ ] Protobuf codegen pipeline
- [ ] Local testing framework

### Phase 2: Core Backends

- [ ] Kafka backend with producer/consumer
- [ ] NATS backend
- [ ] PostgreSQL backend
- [ ] Shadow traffic for migrations
- [ ] Admin UI basics

### Phase 3: Production Ready

- [ ] Neptune (AWS) graph backend
- [ ] Client-originated configuration
- [ ] Auto-scaling and capacity planning
- [ ] Comprehensive observability
- [ ] Production deployment tooling

## Inspired By

- [Netflix Data Gateway](https://netflixtechblog.medium.com/data-gateway-a-platform-for-growing-and-protecting-the-data-tier-f1ed8db8f5c6)
- [Netflix KV Data Abstraction Layer](https://netflixtechblog.com/tagged/kvdal)
- Envoy Proxy
- Linkerd service mesh

## License

[To be determined]

## Contributing

See [CLAUDE.md](./CLAUDE.md) for contribution guidelines and architectural principles.
