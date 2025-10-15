# Pattern Launcher Examples

This directory contains comprehensive examples demonstrating the Pattern Process Launcher functionality.

## Prerequisites

Before running these examples, ensure you have:

1. **Built the test pattern binary**:
   ```bash
   cd /Users/jrepp/dev/data-access
   go build -o patterns/test-pattern/test-pattern patterns/test-pattern/main.go
   ```

2. **Started the launcher service**:
   ```bash
   cd cmd/pattern-launcher
   go run . --patterns-dir ../../patterns --grpc-port 8080
   ```

## Examples

### 1. Basic Launch (`basic_launch.go`)

Demonstrates the fundamental launcher operations:
- Connecting to the launcher gRPC service
- Launching patterns with namespace isolation
- Listing running patterns
- Checking launcher health
- Gracefully terminating patterns

**Run**:
```bash
go run basic_launch.go
```

**What it does**:
- Launches consumer pattern for tenant-a
- Launches consumer pattern for tenant-b (separate process)
- Lists all running patterns
- Checks launcher health
- Terminates both patterns gracefully

**Expected output**:
```
Launching consumer pattern for tenant-a...
✓ Pattern launched successfully!
  Process ID: ns:tenant-a:consumer
  Address: localhost:50051
  State: STATE_RUNNING
  Healthy: true

Launching consumer pattern for tenant-b...
✓ Pattern launched successfully!
  Process ID: ns:tenant-b:consumer

Listing all running patterns...
✓ Found 2 running patterns:
  - consumer (ns:tenant-a:consumer): STATE_RUNNING [namespace=tenant-a, uptime=5s]
  - consumer (ns:tenant-b:consumer): STATE_RUNNING [namespace=tenant-b, uptime=3s]

Checking launcher health...
✓ Launcher is healthy!
  Total processes: 2
  Running: 2
  Failed: 0
  Isolation distribution: map[Namespace:2]
```

### 2. Embedded Launcher (`embedded_launcher.go`)

Shows how to embed the launcher service in your own application:
- Creating and configuring a launcher service programmatically
- Starting a gRPC server
- Handling graceful shutdown on SIGINT/SIGTERM
- Exporting metrics periodically

**Run**:
```bash
go run embedded_launcher.go
```

**What it does**:
- Creates launcher service with custom configuration
- Starts gRPC server on :8080
- Exports Prometheus metrics every 30 seconds
- Gracefully shuts down on Ctrl+C

**Expected output**:
```
Creating pattern launcher service...
✓ Pattern launcher service started
  Patterns directory: ./patterns
  Default isolation: Namespace
  gRPC address: [::]:8080

Initial metrics:
  Total processes: 0
  Running processes: 0
  Uptime: 0s

Serving gRPC requests on :8080...
Press Ctrl+C to shutdown gracefully

=== Metrics Export ===
# HELP pattern_launcher_processes_total Total number of processes
# TYPE pattern_launcher_processes_total gauge
pattern_launcher_processes_total{state="running"} 0
...
```

### 3. Isolation Levels (`isolation_levels.go`)

Comprehensive demonstration of all three isolation levels:
- **ISOLATION_NONE**: All requests share one process
- **ISOLATION_NAMESPACE**: Each tenant gets its own process
- **ISOLATION_SESSION**: Each user gets its own process

**Run**:
```bash
go run isolation_levels.go
```

**What it does**:
- Launches schema-registry with NONE isolation (shared process)
- Launches consumer with NAMESPACE isolation (per-tenant)
- Launches producer with SESSION isolation (per-user)
- Verifies process reuse and separation

**Expected output**:
```
=== Isolation Level Demonstration ===

1. ISOLATION_NONE: Shared process
   Use case: Stateless patterns, read-only lookups

   Request 1 → Process ID: shared:schema-registry
   Request 2 → Process ID: shared:schema-registry
   ✓ Both requests share the same process!

2. ISOLATION_NAMESPACE: Per-tenant processes
   Use case: Multi-tenant SaaS, fault isolation

   tenant-a → Process ID: ns:tenant-a:consumer
   tenant-b → Process ID: ns:tenant-b:consumer
   tenant-a (again) → Process ID: ns:tenant-a:consumer
   ✓ tenant-a requests share the same process!
   ✓ tenant-a and tenant-b have separate processes!

3. ISOLATION_SESSION: Per-user processes
   Use case: High-security, compliance (PCI-DSS, HIPAA)

   tenant-a:user-1 → Process ID: session:tenant-a:user-1:producer
   tenant-a:user-2 → Process ID: session:tenant-a:user-2:producer
   tenant-b:user-1 → Process ID: session:tenant-b:user-1:producer
   tenant-a:user-1 (again) → Process ID: session:tenant-a:user-1:producer
   ✓ Same user's requests share the same process!
   ✓ Different users have separate processes!
   ✓ Same user in different tenants have separate processes!

=== Summary ===
Total running processes: 7
Isolation distribution:
  None: 1 processes
  Namespace: 2 processes
  Session: 4 processes
```

### 4. Metrics Monitoring (`metrics_monitoring.go`)

Shows how to monitor launcher metrics and process health:
- Getting health status
- Launching multiple patterns
- Monitoring individual process status
- Tracking metrics over time
- Graceful cleanup

**Run**:
```bash
go run metrics_monitoring.go
```

**What it does**:
- Gets initial health status
- Launches 5 patterns with different isolation levels
- Monitors individual process status
- Polls health every 2 seconds for 10 seconds
- Terminates all processes gracefully

**Expected output**:
```
=== Metrics and Monitoring Demo ===

1. Initial Health Status
   Healthy: true
   Total processes: 0
   Running: 0
   Uptime: 45s

2. Launching patterns...
   Launching consumer (isolation=ISOLATION_NAMESPACE, namespace=tenant-a, session=)...
   ✓ Launched in 1.234s (process_id=ns:tenant-a:consumer)
   Launching consumer (isolation=ISOLATION_NAMESPACE, namespace=tenant-b, session=)...
   ✓ Launched in 876ms (process_id=ns:tenant-b:consumer)
   ...

4. Health Status After Launch
   Total processes: 5
   Running: 5
   Failed: 0

   Running processes:
     - consumer (ns:tenant-a:consumer): STATE_RUNNING [ns=tenant-a, uptime=3s]
     - consumer (ns:tenant-b:consumer): STATE_RUNNING [ns=tenant-b, uptime=3s]
     - producer (session:tenant-a:user-1:producer): STATE_RUNNING [ns=tenant-a, uptime=3s]
     - producer (session:tenant-a:user-2:producer): STATE_RUNNING [ns=tenant-a, uptime=3s]
     - schema-registry (shared:schema-registry): STATE_RUNNING [ns=, uptime=3s]

6. Monitoring for 10 seconds...
   [14:32:10] Running: 5, Failed: 0, Uptime: 50s
   [14:32:12] Running: 5, Failed: 0, Uptime: 52s
   [14:32:14] Running: 5, Failed: 0, Uptime: 54s
   ...
```

## Pattern Manifest Requirements

All patterns must have a `manifest.yaml` file. Example:

```yaml
name: test-pattern
version: 1.0.0
executable: ./test-pattern
isolation_level: namespace

healthcheck:
  port: 9090
  path: /health
  interval: 30s
  timeout: 5s
  failure_threshold: 3

resources:
  cpu_limit: 1.0
  memory_limit: 512Mi

environment:
  LOG_LEVEL: info
```

## Troubleshooting

### "Pattern not found"
Ensure patterns directory exists and contains valid manifest.yaml files:
```bash
ls -la patterns/*/manifest.yaml
```

### "Failed to connect to launcher"
Verify launcher service is running on :8080:
```bash
lsof -i :8080
```

### "Health check failed"
Check that pattern executable implements /health endpoint:
```bash
curl http://localhost:9090/health
```

### "Permission denied" when running pattern
Make pattern executable:
```bash
chmod +x patterns/test-pattern/test-pattern
```

## Advanced Usage

### Custom gRPC Port

If launcher is running on a different port:
```go
conn, err := grpc.Dial("localhost:9999",
    grpc.WithTransportCredentials(insecure.NewCredentials()))
```

### Custom Configuration

When embedding the launcher:
```go
config := &launcher.Config{
    PatternsDir:      "/opt/patterns",
    DefaultIsolation: isolation.IsolationSession,
    ResyncInterval:   60 * time.Second,
    BackOffPeriod:    10 * time.Second,
}
```

### Metrics Export

To export metrics programmatically:
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

## See Also

- [RFC-035: Pattern Process Launcher](../../../docs-cms/rfcs/RFC-035-pattern-process-launcher.md)
- [Package Documentation](../doc.go)
- [Integration Tests](../integration_test.go)
