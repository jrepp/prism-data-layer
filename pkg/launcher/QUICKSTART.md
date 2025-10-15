# Pattern Launcher Quick Start Guide

Get started with the Pattern Process Launcher in 5 minutes.

## What is the Pattern Launcher?

The Pattern Launcher is a lightweight process manager that launches and manages pattern executables with:

- **Bulkhead Isolation**: Prevent cascading failures with three isolation levels
- **Automatic Recovery**: Crashed processes restart automatically with circuit breaker protection
- **Health Monitoring**: HTTP health checks ensure process readiness
- **Prometheus Metrics**: Complete observability for production deployments

## Quick Start

### Step 1: Install Prerequisites

```bash
# Ensure Go 1.21+ is installed
go version

# Install Prism dependencies
cd /Users/jrepp/dev/data-access
go mod download
```

### Step 2: Create a Pattern Executable

Create a simple pattern that responds to health checks:

```bash
mkdir -p patterns/hello-pattern
```

Create `patterns/hello-pattern/main.go`:

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    // Read configuration from environment
    patternName := os.Getenv("PATTERN_NAME")
    namespace := os.Getenv("NAMESPACE")
    healthPort := os.Getenv("HEALTH_PORT")
    grpcPort := os.Getenv("GRPC_PORT")

    log.Printf("Starting %s pattern", patternName)
    log.Printf("  Namespace: %s", namespace)
    log.Printf("  Health port: %s", healthPort)
    log.Printf("  gRPC port: %s", grpcPort)

    // Implement health check endpoint
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "OK - %s is healthy\n", patternName)
    })

    // Start HTTP server for health checks
    addr := ":" + healthPort
    log.Printf("Health endpoint listening on %s", addr)

    if err := http.ListenAndServe(addr, nil); err != nil {
        log.Fatalf("Health server failed: %v", err)
    }
}
```

Build the pattern:

```bash
cd patterns/hello-pattern
go build -o hello-pattern main.go
chmod +x hello-pattern
```

### Step 3: Create Pattern Manifest

Create `patterns/hello-pattern/manifest.yaml`:

```yaml
name: hello-pattern
version: 1.0.0
executable: ./hello-pattern
isolation_level: namespace

healthcheck:
  port: 9090
  path: /health
  interval: 30s
  timeout: 5s
  failure_threshold: 3

resources:
  cpu_limit: 1.0
  memory_limit: 256Mi

environment:
  LOG_LEVEL: info
```

### Step 4: Start the Launcher Service

Option A: Run launcher directly

```bash
cd /Users/jrepp/dev/data-access
go run cmd/pattern-launcher/main.go \
    --patterns-dir ./patterns \
    --grpc-port 8080
```

Option B: Use embedded launcher (for production)

Create `my-launcher.go`:

```go
package main

import (
    "context"
    "log"
    "net"
    "time"

    "github.com/jrepp/prism/pkg/isolation"
    "github.com/jrepp/prism/pkg/launcher"
    pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
    "google.golang.org/grpc"
)

func main() {
    // Create launcher service
    config := &launcher.Config{
        PatternsDir:      "./patterns",
        DefaultIsolation: isolation.IsolationNamespace,
        ResyncInterval:   30 * time.Second,
        BackOffPeriod:    5 * time.Second,
    }

    service, err := launcher.NewService(config)
    if err != nil {
        log.Fatalf("Failed to create launcher: %v", err)
    }
    defer service.Shutdown(context.Background())

    // Start gRPC server
    grpcServer := grpc.NewServer()
    pb.RegisterPatternLauncherServer(grpcServer, service)

    listener, err := net.Listen("tcp", ":8080")
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }

    log.Printf("Launcher service started on :8080")
    grpcServer.Serve(listener)
}
```

Run it:

```bash
go run my-launcher.go
```

### Step 5: Launch Your First Pattern

Create a client to launch patterns:

```go
package main

import (
    "context"
    "fmt"
    "log"

    pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    // Connect to launcher
    conn, err := grpc.Dial("localhost:8080",
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()

    client := pb.NewPatternLauncherClient(conn)
    ctx := context.Background()

    // Launch pattern
    resp, err := client.LaunchPattern(ctx, &pb.LaunchRequest{
        PatternName: "hello-pattern",
        Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
        Namespace:   "my-app",
    })
    if err != nil {
        log.Fatalf("Failed to launch: %v", err)
    }

    fmt.Printf("✓ Pattern launched successfully!\n")
    fmt.Printf("  Process ID: %s\n", resp.ProcessId)
    fmt.Printf("  Address: %s\n", resp.Address)
    fmt.Printf("  Healthy: %v\n", resp.Healthy)
}
```

Run the client:

```bash
go run client.go
```

**Expected output**:

```
✓ Pattern launched successfully!
  Process ID: ns:my-app:hello-pattern
  Address: localhost:50051
  Healthy: true
```

### Step 6: Verify Pattern is Running

Check launcher health:

```bash
grpcurl -plaintext localhost:8080 prism.launcher.PatternLauncher/Health
```

Response:

```json
{
  "healthy": true,
  "totalProcesses": 1,
  "runningProcesses": 1,
  "isolationDistribution": {
    "Namespace": 1
  },
  "uptimeSeconds": "120"
}
```

List running patterns:

```bash
grpcurl -plaintext localhost:8080 prism.launcher.PatternLauncher/ListPatterns
```

Test pattern health directly:

```bash
curl http://localhost:9090/health
# OK - hello-pattern is healthy
```

## Understanding Isolation Levels

The launcher supports three isolation levels:

### ISOLATION_NONE (Shared)

All requests share a single process. Lowest resource usage.

**Use case**: Stateless read-only patterns (schema registry, config lookup)

```go
client.LaunchPattern(ctx, &pb.LaunchRequest{
    PatternName: "schema-registry",
    Isolation:   pb.IsolationLevel_ISOLATION_NONE,
})
```

### ISOLATION_NAMESPACE (Per-Tenant)

Each namespace (tenant) gets its own dedicated process.

**Use case**: Multi-tenant SaaS applications requiring fault isolation

```go
client.LaunchPattern(ctx, &pb.LaunchRequest{
    PatternName: "consumer",
    Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
    Namespace:   "tenant-a",
})
```

### ISOLATION_SESSION (Per-User)

Each session (user) gets its own dedicated process.

**Use case**: High-security environments, compliance requirements (PCI-DSS, HIPAA)

```go
client.LaunchPattern(ctx, &pb.LaunchRequest{
    PatternName: "producer",
    Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
    Namespace:   "tenant-a",
    SessionId:   "user-123",
})
```

## Monitoring and Metrics

### Export Prometheus Metrics

```go
// In your launcher service
prometheusMetrics := service.ExportPrometheusMetrics()
fmt.Println(prometheusMetrics)
```

Output:

```
# HELP pattern_launcher_processes_total Total number of processes
# TYPE pattern_launcher_processes_total gauge
pattern_launcher_processes_total{state="running"} 3

# HELP pattern_launcher_process_starts_total Total process starts
# TYPE pattern_launcher_process_starts_total counter
pattern_launcher_process_starts_total{pattern_isolation="hello-pattern:Namespace"} 5

# HELP pattern_launcher_launch_duration_seconds Launch duration percentiles
# TYPE pattern_launcher_launch_duration_seconds summary
pattern_launcher_launch_duration_seconds{quantile="0.5"} 1.234
pattern_launcher_launch_duration_seconds{quantile="0.95"} 2.456
pattern_launcher_launch_duration_seconds{quantile="0.99"} 3.789
```

### JSON Metrics Export

```go
jsonMetrics := service.ExportJSONMetrics()
fmt.Println(jsonMetrics)
```

Output:

```json
{
  "total_processes": 3,
  "running_processes": 3,
  "process_starts_total": {
    "hello-pattern:Namespace": 5
  },
  "launch_duration_p50_seconds": 1.234,
  "launch_duration_p95_seconds": 2.456,
  "launch_duration_p99_seconds": 3.789,
  "uptime_seconds": 300
}
```

## Error Handling and Recovery

The launcher automatically handles common failure scenarios:

### Automatic Restart

If a pattern process crashes, it's automatically restarted:

```bash
# Kill pattern process
kill -9 <pattern-pid>

# Launcher detects crash and restarts (watch logs)
# 2025-10-15 14:32:10 Process ns:my-app:hello-pattern is dead, will restart
# 2025-10-15 14:32:11 Launched process: pattern=hello-pattern, pid=98765
```

### Circuit Breaker

After 5 consecutive failures, the process is marked terminal to prevent infinite restart loops:

```bash
# Process repeatedly fails health checks
# 2025-10-15 14:32:15 Process ns:my-app:hello-pattern is unhealthy (errors: 1)
# 2025-10-15 14:32:20 Process ns:my-app:hello-pattern is unhealthy (errors: 2)
# ...
# 2025-10-15 14:32:40 Process ns:my-app:hello-pattern has exceeded max errors (5), marking as terminal
```

### Orphan Detection

Every 60 seconds, the launcher finds and cleans up orphaned pattern processes:

```bash
# 2025-10-15 14:33:00 Found 2 orphaned pattern processes, cleaning up...
# 2025-10-15 14:33:01 Terminated orphaned process: pid=12345
```

## Graceful Shutdown

Terminate patterns with grace period:

```go
client.TerminatePattern(ctx, &pb.TerminateRequest{
    ProcessId:       "ns:my-app:hello-pattern",
    GracePeriodSecs: 10,  // SIGTERM → wait → SIGKILL
})
```

## Common Issues and Solutions

### "Pattern not found"

**Problem**: Launcher can't find your pattern manifest

**Solution**: Verify manifest.yaml exists and patterns-dir is correct

```bash
ls -la patterns/hello-pattern/manifest.yaml
```

### "Health check failed"

**Problem**: Pattern doesn't implement /health endpoint or wrong port

**Solution**: Ensure pattern listens on HEALTH_PORT from environment

```go
healthPort := os.Getenv("HEALTH_PORT")  // Launcher provides this
http.ListenAndServe(":" + healthPort, nil)
```

### "Process keeps restarting"

**Problem**: Pattern crashes on startup

**Solution**: Check pattern logs for errors

```bash
# Pattern stdout/stderr goes to launcher logs
# Look for "Launched process" and subsequent error messages
```

### "Port already in use"

**Problem**: Pattern health port conflicts

**Solution**: Launcher allocates ports automatically - don't hardcode ports

```go
// ❌ Wrong: hardcoded port
http.ListenAndServe(":9090", nil)

// ✅ Correct: use HEALTH_PORT from environment
healthPort := os.Getenv("HEALTH_PORT")
http.ListenAndServe(":" + healthPort, nil)
```

## Next Steps

1. **Try examples**: Explore `pkg/launcher/examples/` for more complex use cases
2. **Read RFC-035**: Comprehensive architecture documentation
3. **Integration tests**: See `pkg/launcher/integration_test.go` for advanced patterns
4. **Production deployment**: Add authentication, TLS, resource limits

## Resources

- [Package Documentation](doc.go) - Comprehensive API reference
- [Examples Directory](examples/) - Working code samples
- [RFC-035](../../docs-cms/rfcs/RFC-035-pattern-process-launcher.md) - Full design document
- [Integration Tests](integration_test.go) - Test suite examples

## Getting Help

If you encounter issues:

1. Check the [Troubleshooting section](doc.go) in package documentation
2. Review [examples](examples/) for working code
3. Run integration tests: `go test -v ./pkg/launcher`
4. File an issue with logs and configuration

---

**You're ready!** Start building with the Pattern Launcher. 🚀
