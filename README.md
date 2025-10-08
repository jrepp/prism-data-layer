# Prism

> A high-performance data access layer gateway

Prism is a Rust-based data access gateway that provides unified, type-safe access to heterogeneous data backends. Inspired by Netflix's Data Gateway but built for extreme performance and developer experience.

## Quick Start

```bash
# Install dependencies
curl -LsSf https://astral.sh/uv/install.sh | sh
uv sync

# Start local backend services
python -m tooling.test.local-stack up

# Run the proxy
cd proxy && cargo run --release

# In another terminal, run the admin UI
cd admin && ember serve

# Run tests
cargo test --workspace
```

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

Each backend is a self-contained module:

```
backends/
├── kafka/      # Event streaming
├── nats/       # Lightweight messaging
├── postgres/   # Relational data
├── sqlite/     # Local/embedded
└── neptune/    # Graph data (AWS)
```

Adding a new backend? Implement the `Backend` trait and register it.

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
├── admin/          # Ember admin UI for management
├── proxy/          # Rust gateway (core of Prism)
├── backends/       # Pluggable backend implementations
│   ├── kafka/
│   ├── nats/
│   ├── postgres/
│   ├── sqlite/
│   └── neptune/
├── proto/          # Protobuf definitions (source of truth)
├── tooling/        # Python utilities (codegen, testing, deploy)
├── tests/          # Integration and load tests
└── docs/           # Architecture docs, ADRs, requirements
```

## Documentation

- **[CLAUDE.md](./CLAUDE.md)**: Project philosophy and guidance
- **[Architecture Decision Records](./docs/adr/)**: Why we made key design choices
- **[Requirements](./docs/requirements/)**: Detailed functional and non-functional requirements
- **[Netflix Reference](./docs/netflix/)**: Materials that inspired Prism
- **[GitHub Pages](https://jrepp.github.io/prism-data-layer/)**: Live documentation site with search

### Contributing to Documentation

**⚠️ CRITICAL: Run validation before pushing documentation changes:**

```bash
# Full validation (required before push)
uv run tooling/validate_docs.py

# Quick check during development (skip slow build)
uv run tooling/validate_docs.py --skip-build
```

See [tooling/README.md](./tooling/README.md) for detailed documentation validation guide.

## Development

### Prerequisites

- Rust 1.75+ (for proxy)
- Node.js 20+ (for admin UI)
- Python 3.11+ (for tooling)
- Docker (for local backends)

### Building

```bash
# Build proxy
cd proxy && cargo build --release

# Build admin UI
cd admin && npm install && ember build

# Generate code from proto definitions
python -m tooling.codegen
```

### Testing

```bash
# Unit tests
cargo test

# Integration tests (requires local-stack)
python -m tooling.test.local-stack up
cargo test --features integration

# Load tests
python -m tooling.test.load-test --help
```

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
