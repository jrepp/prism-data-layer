# PostgreSQL Backend Plugin

PostgreSQL plugin for Prism data gateway, implementing the KeyValue abstraction layer.

## Architecture

Based on:
- **RFC-011**: Data Proxy Authentication (Vault-managed credentials)
- **ADR-005**: Backend Plugin Architecture (KeyValue trait)
- **ADR-025**: Container Plugin Model (standard interfaces)
- **ADR-026**: Distroless Container Images (security)

## Features

- KeyValue operations (Put, Get, Delete)
- Connection pooling with pgx
- Vault-managed dynamic credentials
- Health check endpoints
- Prometheus metrics
- Distroless container image

## Configuration

### Environment Variables

```bash
# Required
DATABASE_URL=postgres://user:pass@localhost:5432/dbname
PRISM_PLUGIN_CONFIG=/etc/prism/plugin.yaml

# Optional
PRISM_LOG_LEVEL=info
PRISM_METRICS_PORT=9090
```

### Config File

```yaml
plugin:
  name: postgres
  version: 0.1.0

control_plane:
  port: 9090

backend:
  database_url: "postgres://prism:password@localhost:5432/prism"
  pool_size: 10
  vault_enabled: false
  vault_path: "database/creds/postgres-main"
```

## Database Schema

```sql
CREATE TABLE keyvalue (
    namespace VARCHAR(255) NOT NULL,
    id VARCHAR(255) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (namespace, id, key)
);

CREATE INDEX idx_keyvalue_namespace ON keyvalue(namespace);
CREATE INDEX idx_keyvalue_updated_at ON keyvalue(updated_at);
```

## Building

### Local Build

```bash
cd plugins/postgres
go build -o postgres-plugin main.go
./postgres-plugin
```

### Container Build (Podman)

```bash
# Production image
podman build -t prism/postgres-plugin:latest --target production -f Dockerfile .

# Debug image
podman build -t prism/postgres-plugin:debug --target debug -f Dockerfile .
```

## Running

### Local Execution

```bash
export DATABASE_URL="postgres://localhost/prism"
export PRISM_PLUGIN_CONFIG="config.yaml"
./postgres-plugin
```

### Container Execution

```bash
podman run -d \
  --name postgres-plugin \
  -e DATABASE_URL="postgres://prism:password@host.containers.internal:5432/prism" \
  -p 9090:9090 \
  prism/postgres-plugin:latest
```

### Docker Compose

```yaml
services:
  postgres-plugin:
    image: prism/postgres-plugin:latest
    environment:
      - DATABASE_URL=postgres://prism:password@postgres:5432/prism
    ports:
      - "9090:9090"
    depends_on:
      - postgres

  postgres:
    image: postgres:16
    environment:
      - POSTGRES_USER=prism
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=prism
```

## Health Checks

The plugin exposes standard health endpoints (ADR-025):

```bash
# gRPC health check (using grpcurl)
grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check

# Response:
{
  "status": "SERVING"
}
```

## API Operations

### Put (Insert/Update)

```go
err := plugin.Put(ctx, "users", "user:123", map[string][]byte{
    "profile": []byte(`{"name": "Alice"}`),
    "email":   []byte("alice@example.com"),
})
```

### Get (Retrieve)

```go
items, err := plugin.Get(ctx, "users", "user:123", []string{"profile", "email"})
// items["profile"] = `{"name": "Alice"}`
// items["email"] = "alice@example.com"
```

### Delete (Remove)

```go
err := plugin.Delete(ctx, "users", "user:123", []string{"profile"})
```

## Vault Integration

When `vault_enabled: true`:

```yaml
backend:
  vault_enabled: true
  vault_path: "database/creds/postgres-main"
```

The plugin will:
1. Fetch dynamic credentials from Vault
2. Use short-lived database credentials (RFC-011 pattern)
3. Automatically renew credentials before expiry
4. Gracefully handle credential rotation

## Observability

### Metrics

Prometheus metrics exposed on `:9090/metrics`:

- `postgres_plugin_operations_total` - Total operations by type
- `postgres_plugin_operation_duration_seconds` - Operation latency
- `postgres_plugin_connection_pool_size` - Current pool connections
- `postgres_plugin_health_status` - Health status (0=unhealthy, 1=degraded, 2=healthy)

### Logging

Structured JSON logging (slog):

```json
{
  "time": "2025-10-09T12:00:00Z",
  "level": "INFO",
  "msg": "postgres plugin initialized",
  "max_conns": 10,
  "vault_enabled": false
}
```

## Testing

```bash
# Unit tests
go test ./...

# Integration tests (requires Postgres)
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=test postgres:16
export DATABASE_URL="postgres://postgres:test@localhost:5432/postgres"
go test -tags=integration ./...
```

## Troubleshooting

### Debug Container

Run debug variant with shell access:

```bash
podman run -it --entrypoint /busybox/sh prism/postgres-plugin:debug
```

### Common Issues

1. **Connection refused**: Check `DATABASE_URL` and PostgreSQL is running
2. **Authentication failed**: Verify credentials or Vault configuration
3. **Pool exhausted**: Increase `pool_size` or check for connection leaks

## References

- [RFC-011: Data Proxy Authentication](/docs-cms/rfcs/RFC-011-data-proxy-authentication.md)
- [ADR-005: Backend Plugin Architecture](/docs-cms/adr/005-backend-plugin-architecture.md)
- [ADR-025: Container Plugin Model](/docs-cms/adr/025-container-plugin-model.md)
- [ADR-026: Distroless Container Images](/docs-cms/adr/026-distroless-container-images.md)
