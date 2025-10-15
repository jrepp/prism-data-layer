# Pattern Process Launcher

A lightweight, production-ready process manager for pattern executables with bulkhead isolation, automatic recovery, and comprehensive observability.

## Features

- **🛡️ Bulkhead Isolation**: Three isolation levels (None, Namespace, Session) prevent cascading failures
- **🔄 Automatic Recovery**: Crashed processes restart automatically with circuit breaker protection
- **❤️ Health Monitoring**: HTTP health checks with configurable intervals and timeouts
- **📊 Prometheus Metrics**: Complete observability with launch latency percentiles, health checks, and lifecycle events
- **🚀 Easy to Use**: Builder pattern and quick-start helpers for simple configuration
- **💡 Actionable Errors**: Detailed error messages with suggestions for resolution

## Quick Start

### 1. Install

```bash
go get github.com/jrepp/prism/pkg/launcher
```

### 2. Create a Pattern

Create `patterns/my-pattern/main.go`:

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    healthPort := os.Getenv("HEALTH_PORT")

    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprint(w, "OK")
    })

    log.Printf("Health endpoint listening on :%s", healthPort)
    http.ListenAndServe(":" + healthPort, nil)
}
```

Create `patterns/my-pattern/manifest.yaml`:

```yaml
name: my-pattern
version: 1.0.0
executable: ./my-pattern
isolation_level: namespace

healthcheck:
  port: 9090
  path: /health
  interval: 30s
  timeout: 5s

resources:
  cpu_limit: 1.0
  memory_limit: 256Mi
```

Build it:

```bash
cd patterns/my-pattern && go build -o my-pattern main.go
```

### 3. Start the Launcher

```go
package main

import (
    "context"
    "log"
    "net"

    "github.com/jrepp/prism/pkg/launcher"
    pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
    "google.golang.org/grpc"
)

func main() {
    // Create launcher with builder pattern
    service, err := launcher.NewBuilder().
        WithPatternsDir("./patterns").
        WithNamespaceIsolation().
        Build()
    if err != nil {
        log.Fatal(err)
    }
    defer service.Shutdown(context.Background())

    // Start gRPC server
    grpcServer := grpc.NewServer()
    pb.RegisterPatternLauncherServer(grpcServer, service)

    listener, _ := net.Listen("tcp", ":8080")
    log.Println("Launcher service started on :8080")
    grpcServer.Serve(listener)
}
```

Or use the quick-start helper:

```go
service := launcher.MustQuickStart("./patterns")
defer service.Shutdown(context.Background())
```

### 4. Launch Patterns

```go
client := pb.NewPatternLauncherClient(conn)

resp, err := client.LaunchPattern(ctx, &pb.LaunchRequest{
    PatternName: "my-pattern",
    Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
    Namespace:   "tenant-a",
})

log.Printf("Pattern launched: %s", resp.ProcessId)
```

## Documentation

- **[Quick Start Guide](QUICKSTART.md)** - Get started in 5 minutes
- **[Package Documentation](doc.go)** - Comprehensive API reference
- **[Examples](examples/)** - Working code samples
  - [Basic Launch](examples/basic_launch.go) - Fundamental operations
  - [Embedded Launcher](examples/embedded_launcher.go) - Embed in your app
  - [Isolation Levels](examples/isolation_levels.go) - All three isolation levels
  - [Metrics Monitoring](examples/metrics_monitoring.go) - Observability patterns
- **[RFC-035](../../docs-cms/rfcs/RFC-035-pattern-process-launcher.md)** - Full design document

## Architecture

```
Pattern Launcher Service
├── Pattern Registry (discovery & validation)
├── Isolation Managers (per level: None, Namespace, Session)
│   ├── Process Manager (Kubernetes-inspired state machine)
│   └── Process Syncer (exec.Command, health checks)
├── Lifecycle Management
│   ├── Cleanup Manager (resource cleanup)
│   ├── Orphan Detector (find & terminate orphans)
│   └── Health Check Monitor (continuous monitoring)
└── Metrics Collector (Prometheus export)
```

## Isolation Levels

### ISOLATION_NONE (Shared Process)

All requests share one process. Lowest resource usage.

**Use case**: Stateless read-only patterns (schema registry)

```go
LaunchRequest{
    PatternName: "schema-registry",
    Isolation:   IsolationLevel_ISOLATION_NONE,
}
```

### ISOLATION_NAMESPACE (Per-Tenant)

Each namespace gets its own process.

**Use case**: Multi-tenant SaaS requiring fault isolation

```go
LaunchRequest{
    PatternName: "consumer",
    Isolation:   IsolationLevel_ISOLATION_NAMESPACE,
    Namespace:   "tenant-a",
}
```

### ISOLATION_SESSION (Per-User)

Each session gets its own process.

**Use case**: High-security, compliance (PCI-DSS, HIPAA)

```go
LaunchRequest{
    PatternName: "producer",
    Isolation:   IsolationLevel_ISOLATION_SESSION,
    Namespace:   "tenant-a",
    SessionId:   "user-123",
}
```

## Builder Pattern

The builder pattern makes it easy to configure the launcher:

```go
// Development configuration
service := launcher.NewBuilder().
    WithPatternsDir("./patterns").
    WithDevelopmentDefaults().  // Fast resync, quick retries
    Build()

// Production configuration
service := launcher.NewBuilder().
    WithPatternsDir("/opt/patterns").
    WithProductionDefaults().  // Balanced monitoring, avoid retry storms
    WithResourceLimits(2.0, "1Gi").
    Build()

// Custom configuration
service := launcher.NewBuilder().
    WithPatternsDir("./patterns").
    WithSessionIsolation().
    WithResyncInterval(15 * time.Second).
    WithBackOffPeriod(3 * time.Second).
    Build()
```

## Error Handling

The launcher provides actionable error messages with suggestions:

```go
err := launcher.ErrPatternNotFound("my-pattern", "./patterns")
fmt.Println(err)
// Output:
// [PATTERN_NOT_FOUND] Pattern 'my-pattern' not found;
// Context: pattern_name=my-pattern, patterns_dir=./patterns;
// Suggestion: Verify pattern exists: ls -la ./patterns/my-pattern/manifest.yaml
```

All errors include:
- **Error code**: Categorized error type
- **Context**: Relevant details (pattern name, paths, PIDs)
- **Cause**: Underlying error (if any)
- **Suggestion**: Actionable steps to resolve

## Metrics

Export metrics in Prometheus or JSON format:

```go
// Prometheus format
prometheusMetrics := service.ExportPrometheusMetrics()
fmt.Println(prometheusMetrics)

// JSON format
jsonMetrics := service.ExportJSONMetrics()
fmt.Println(jsonMetrics)

// Structured snapshot
metrics := service.GetMetrics()
fmt.Printf("Launch p95: %v\n", metrics.LaunchDurationP95)
```

Available metrics:
- `pattern_launcher_processes_total` - Process counts by state
- `pattern_launcher_process_starts_total` - Lifecycle counters
- `pattern_launcher_health_checks_total` - Health check results
- `pattern_launcher_launch_duration_seconds` - Launch latency percentiles (p50, p95, p99)
- `pattern_launcher_uptime_seconds` - Launcher availability

## Development

### Prerequisites

- Go 1.21+
- golangci-lint (optional, for linting)

### Build and Test

```bash
# Build test pattern
make build

# Run unit tests (fast)
make test-short

# Run all tests (including integration)
make test

# Generate coverage report
make test-coverage

# Build examples
make examples

# Format and lint
make fmt lint

# Full dev cycle (format + test + build)
make dev
```

### Testing Your Pattern

```bash
# 1. Start launcher in one terminal
go run ../../cmd/pattern-launcher/main.go \
    --patterns-dir ./patterns \
    --grpc-port 8080

# 2. Launch pattern with grpcurl
grpcurl -plaintext \
    -d '{"pattern_name":"my-pattern","isolation":"ISOLATION_NAMESPACE","namespace":"test"}' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern

# 3. Check health
curl http://localhost:9090/health
```

## Troubleshooting

### Pattern won't start

**Symptom**: `PROCESS_START_FAILED` error

**Check**:
1. Executable exists: `ls -la patterns/my-pattern/my-pattern`
2. Executable is runnable: `chmod +x patterns/my-pattern/my-pattern`
3. Test standalone: `./patterns/my-pattern/my-pattern`

### Health check fails

**Symptom**: Process keeps restarting

**Check**:
1. Health endpoint responds: `curl http://localhost:9090/health`
2. Pattern uses HEALTH_PORT from environment
3. Check pattern logs for startup errors

### Circuit breaker activated

**Symptom**: `MAX_ERRORS_EXCEEDED` error after 5 failures

**Fix**:
1. Investigate why pattern keeps failing (check logs)
2. Fix underlying issue
3. Terminate pattern: `prismctl pattern terminate <process_id>`
4. Relaunch: `prismctl pattern launch <pattern_name>`

### Port conflicts

**Symptom**: "address already in use" errors

**Fix**:
- Don't hardcode ports in pattern code
- Use `HEALTH_PORT` and `GRPC_PORT` from environment
- Launcher allocates ports automatically

## Performance

- **Cold start latency**: ~500ms-2s (first launch per isolation key)
- **Process reuse**: <10ms (subsequent requests to same namespace)
- **Memory overhead**: ~50MB per process baseline
- **CPU overhead**: <1% for process management goroutines

Optimize with:
- **Warm pools**: Pre-launch processes for popular namespaces
- **Resource limits**: Set appropriate CPU/memory limits
- **Health check tuning**: Balance responsiveness vs overhead

## Production Deployment

### Systemd Integration

```ini
[Unit]
Description=Prism Pattern Launcher
After=network.target

[Service]
Type=simple
User=prism
ExecStart=/usr/local/bin/pattern-launcher \
    --patterns-dir /opt/prism/patterns \
    --grpc-port 8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Docker

```dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN cd cmd/pattern-launcher && go build -o /pattern-launcher

FROM debian:bookworm-slim
COPY --from=builder /pattern-launcher /usr/local/bin/
COPY patterns/ /opt/patterns/
EXPOSE 8080
CMD ["pattern-launcher", "--patterns-dir", "/opt/patterns", "--grpc-port", "8080"]
```

### Kubernetes

For Kubernetes, patterns should be deployed as Deployments/StatefulSets rather than using the launcher. The launcher is designed for:
- Local development
- Bare metal / VM deployments
- Environments without Kubernetes

## Contributing

1. **Add tests**: All new features require tests
2. **Format code**: Run `make fmt` before committing
3. **Lint**: Run `make lint` to catch issues
4. **Coverage**: Maintain >85% test coverage
5. **Document**: Update relevant documentation

## License

See project root for license information.

## See Also

- [RFC-035: Pattern Process Launcher](../../docs-cms/rfcs/RFC-035-pattern-process-launcher.md)
- [pkg/isolation](../isolation/) - Bulkhead isolation implementation
- [pkg/procmgr](../procmgr/) - Process manager implementation
- [Bulkhead Pattern (Release It!)](https://www.oreilly.com/library/view/release-it-2nd/9781680504552/)
