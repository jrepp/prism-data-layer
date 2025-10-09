# Prism Backend Plugins

Modular backend plugins for the Prism data gateway, implementing various data access patterns.

## Architecture

Based on:
- **RFC-011**: Data Proxy Authentication (Input/Output)
- **ADR-005**: Backend Plugin Architecture (trait-based)
- **ADR-025**: Container Plugin Model (standard interfaces)
- **ADR-026**: Distroless Container Images (security)

## Available Plugins

### 1. PostgreSQL Plugin (`postgres/`)
- **Use case**: KeyValue abstraction
- **Operations**: Put, Get, Delete, Scan
- **Auth**: mTLS or password via Vault
- **Port**: 9090

### 2. Kafka Plugin (`kafka/`)
- **Use case**: Event streaming, TimeSeries abstraction
- **Operations**: Publish, Subscribe, Tail
- **Auth**: SASL/SCRAM-SHA-512 via Vault
- **Port**: 9091

### 3. Redis Plugin (`redis/`)
- **Use case**: Caching layer
- **Operations**: Get, Set, Delete, MGet, Expire
- **Auth**: ACL + password via Vault
- **Port**: 9092

## Quick Start

### Build All Plugins

```bash
cd plugins
make build
```

### Build Specific Plugin

```bash
make postgres       # Build postgres production image
make kafka-debug    # Build kafka debug image
make redis          # Build redis production image
```

### Run Plugin Locally

```bash
# Start postgres plugin (requires PostgreSQL running)
make run-postgres

# Start kafka plugin (requires Kafka running)
make run-kafka

# Start redis plugin (requires Redis running)
make run-redis
```

## Project Structure

```
plugins/
├── Makefile            # Root build system (Podman-based)
├── README.md           # This file
├── core/               # Shared core package
│   ├── go.mod
│   ├── plugin.go       # Plugin interface and bootstrap
│   ├── controlplane.go # Control plane gRPC server
│   └── config.go       # Configuration management
├── postgres/           # PostgreSQL plugin
│   ├── go.mod
│   ├── main.go
│   ├── config.yaml
│   ├── Dockerfile      # Multi-stage build
│   └── README.md
├── kafka/              # Kafka plugin
│   ├── go.mod
│   ├── main.go
│   ├── config.yaml
│   ├── Dockerfile
│   └── README.md
├── redis/              # Redis plugin
│   ├── go.mod
│   ├── main.go
│   ├── config.yaml
│   ├── Dockerfile
│   └── README.md
└── watcher/            # File watcher for dynamic reload
    └── (to be implemented)
```

## Core Package

All plugins depend on the `core` package which provides:

- **Plugin interface**: Lifecycle methods (Initialize, Start, Stop, Health)
- **Bootstrap**: Automatic setup with config loading and signal handling
- **Control plane**: gRPC server for health checks and management
- **Configuration**: YAML-based config with environment variable overrides

## Building

### Prerequisites

- Go 1.23+
- Podman (or Docker)

### Build Commands

```bash
# Build all production images
make build

# Build all debug images
make build-debug

# Build specific plugin
make postgres kafka redis

# Build with specific version
make VERSION=v1.0.0 build

# Build for specific registry
make REGISTRY=ghcr.io/myorg build
```

### Image Tags

Production images:
- `prism/postgres-plugin:latest`
- `prism/postgres-plugin:v1.0.0`

Debug images (with busybox):
- `prism/postgres-plugin:debug`
- `prism/postgres-plugin:v1.0.0-debug`

## Running

### Docker Compose

```yaml
version: '3.8'

services:
  postgres-plugin:
    image: prism/postgres-plugin:latest
    environment:
      - DATABASE_URL=postgres://prism:password@postgres:5432/prism
    ports:
      - "9090:9090"
    depends_on:
      - postgres

  kafka-plugin:
    image: prism/kafka-plugin:latest
    environment:
      - KAFKA_BROKERS=kafka:9092
      - KAFKA_TOPIC=events
      - KAFKA_SASL_USERNAME=prism-kafka
      - KAFKA_SASL_PASSWORD=secret
    ports:
      - "9091:9091"
    depends_on:
      - kafka

  redis-plugin:
    image: prism/redis-plugin:latest
    environment:
      - REDIS_ADDRESS=redis:6379
      - REDIS_PASSWORD=secret
    ports:
      - "9092:9092"
    depends_on:
      - redis

  # Backend services
  postgres:
    image: postgres:16
    environment:
      - POSTGRES_USER=prism
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=prism

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    environment:
      - KAFKA_BROKER_ID=1
      - KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181
      - KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass secret
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prism-postgres-plugin
spec:
  replicas: 2
  selector:
    matchLabels:
      app: prism-postgres-plugin
  template:
    metadata:
      labels:
        app: prism-postgres-plugin
    spec:
      containers:
      - name: postgres-plugin
        image: prism/postgres-plugin:latest
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: prism-secrets
              key: postgres-url
        ports:
        - containerPort: 9090
          name: control-plane
        livenessProbe:
          grpc:
            port: 9090
          initialDelaySeconds: 10
        readinessProbe:
          grpc:
            port: 9090
          initialDelaySeconds: 5
```

## Health Checks

All plugins implement standard gRPC health checks (ADR-025):

```bash
# Check postgres plugin health
grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check

# Check kafka plugin health
grpcurl -plaintext localhost:9091 grpc.health.v1.Health/Check

# Check redis plugin health
grpcurl -plaintext localhost:9092 grpc.health.v1.Health/Check
```

## Configuration

All plugins support:

1. **Config files** (YAML): `/etc/prism/plugin.yaml`
2. **Environment variables**: Override config file values
3. **Vault integration**: Dynamic credential fetching

### Environment Variables

Common to all plugins:
- `PRISM_PLUGIN_CONFIG` - Config file path (default: `/etc/prism/plugin.yaml`)
- `PRISM_LOG_LEVEL` - Log level (debug, info, warn, error)

Plugin-specific:
- **Postgres**: `DATABASE_URL`, `POSTGRES_POOL_SIZE`
- **Kafka**: `KAFKA_BROKERS`, `KAFKA_TOPIC`, `KAFKA_SASL_USERNAME`, `KAFKA_SASL_PASSWORD`
- **Redis**: `REDIS_ADDRESS`, `REDIS_USERNAME`, `REDIS_PASSWORD`

## Security

Based on ADR-026 (Distroless Images):

- **Minimal base**: Distroless images (no shell, no package manager)
- **Non-root**: Runs as UID 65532 (nonroot user)
- **Read-only filesystem**: Kubernetes security context supported
- **Debug variants**: Separate images with busybox for troubleshooting

### Debug Access

To debug a running plugin:

```bash
# Override entrypoint to get shell
podman run -it --entrypoint /busybox/sh prism/postgres-plugin:debug

# Or in Kubernetes
kubectl exec -it postgres-plugin-pod -- /busybox/sh
```

## Testing

```bash
# Run all tests
make test

# Test specific plugin
cd postgres && go test -v ./...
cd kafka && go test -v ./...
cd redis && go test -v ./...
```

## Observability

All plugins expose:

1. **Structured logging**: JSON format with slog
2. **Health endpoints**: gRPC health checks
3. **Metrics**: Prometheus format (TODO)

## Development

### Adding a New Plugin

1. Create new directory: `plugins/newplugin/`
2. Implement `core.Plugin` interface
3. Create `Dockerfile` following ADR-026 pattern
4. Add to `Makefile`
5. Write README with specific configuration

### Plugin Lifecycle

```
Initialize() → Start() → [Running] → Stop()
              ↓
         Health() (periodic)
```

## Troubleshooting

### Common Issues

1. **Import errors**: Run `go mod download` in each plugin directory
2. **Build failures**: Ensure core package is built first (`make .build-core-timestamp`)
3. **Connection refused**: Check backend services are running
4. **Authentication failed**: Verify credentials and Vault configuration

### Image Sizes

Check plugin image sizes:

```bash
make sizes
```

Expected sizes (ADR-026 targets):
- Postgres plugin: ~30-40MB (cc-debian12 + binary)
- Kafka plugin: ~40-50MB (cc-debian12 + librdkafka + binary)
- Redis plugin: ~10-20MB (static-debian12 + binary)

## References

- [RFC-011: Data Proxy Authentication](/docs-cms/rfcs/RFC-011-data-proxy-authentication.md)
- [ADR-005: Backend Plugin Architecture](/docs-cms/adr/005-backend-plugin-architecture.md)
- [ADR-025: Container Plugin Model](/docs-cms/adr/025-container-plugin-model.md)
- [ADR-026: Distroless Container Images](/docs-cms/adr/026-distroless-container-images.md)

## License

See LICENSE file in repository root.
