---
title: "MEMO-008: Pattern Launcher Quick Start for Developers"
author: Claude Code
created: 2025-10-15
updated: 2025-10-15
tags: [patterns, launcher, quickstart, developer-guide]
id: memo-008
---

# MEMO-008: Pattern Launcher Quick Start for Developers

**TL;DR**: Get a pattern running in 3 commands. Test all isolation levels in 5 minutes.

## What You're Building

A process manager that launches pattern executables with fault isolation. Think "systemd for patterns" but with tenant-level isolation.

## Prerequisites

```bash
# You need Go 1.21+
go version
```

## Step 1: Create a Test Pattern (2 minutes)

Create the simplest possible pattern:

```bash
# Create pattern directory
mkdir -p patterns/hello-pattern

# Write the pattern
cat > patterns/hello-pattern/main.go << 'EOF'
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "time"
)

func main() {
    name := os.Getenv("PATTERN_NAME")
    namespace := os.Getenv("NAMESPACE")
    healthPort := os.Getenv("HEALTH_PORT")

    log.Printf("Starting %s (namespace=%s, health_port=%s)", name, namespace, healthPort)

    // Health endpoint (required)
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "OK - %s is healthy\n", name)
    })

    // Optional: Do some work
    go func() {
        for {
            log.Printf("[%s:%s] Processing...", namespace, name)
            time.Sleep(5 * time.Second)
        }
    }()

    log.Printf("Health server starting on :%s", healthPort)
    if err := http.ListenAndServe(":"+healthPort, nil); err != nil {
        log.Fatal(err)
    }
}
EOF

# Build it
cd patterns/hello-pattern && go build -o hello-pattern main.go && cd ../..

# Make it executable
chmod +x patterns/hello-pattern/hello-pattern

# Test it works
PATTERN_NAME=hello HEALTH_PORT=9999 ./patterns/hello-pattern/hello-pattern &
curl http://localhost:9999/health
kill %1
```

**Expected**: "OK - hello is healthy"

## Step 2: Create Pattern Manifest (30 seconds)

```bash
cat > patterns/hello-pattern/manifest.yaml << 'EOF'
name: hello-pattern
version: 1.0.0
executable: ./hello-pattern
isolation_level: namespace

healthcheck:
  port: 9090
  path: /health
  interval: 30s
  timeout: 5s

resources:
  cpu_limit: 1.0
  memory_limit: 256Mi
EOF
```

**That's it**. Pattern is ready.

## Step 3: Start the Launcher (1 command)

```bash
# From project root
go run cmd/pattern-launcher/main.go \
    --patterns-dir ./patterns \
    --grpc-port 8080
```

**Expected output**:
```
Discovering patterns in directory: ./patterns
Discovered pattern: hello-pattern (version: 1.0.0, isolation: namespace)
Pattern launcher service created with 1 patterns
Serving gRPC on :8080
```

Keep this running in terminal 1.

## Step 4: Launch Your First Pattern (grpcurl)

In a new terminal:

```bash
# Install grpcurl if needed
brew install grpcurl  # or: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Launch pattern for tenant-a
grpcurl -plaintext \
    -d '{
        "pattern_name": "hello-pattern",
        "isolation": "ISOLATION_NAMESPACE",
        "namespace": "tenant-a"
    }' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern
```

**Expected**:
```json
{
  "processId": "ns:tenant-a:hello-pattern",
  "state": "STATE_RUNNING",
  "address": "localhost:50051",
  "healthy": true
}
```

**Verify it's running**:
```bash
curl http://localhost:9090/health
# OK - hello-pattern is healthy
```

## Step 5: Test Isolation Levels (3 minutes)

### Test 1: Namespace Isolation (Different Tenants = Different Processes)

```bash
# Launch for tenant-a (already done above)

# Launch for tenant-b
grpcurl -plaintext \
    -d '{
        "pattern_name": "hello-pattern",
        "isolation": "ISOLATION_NAMESPACE",
        "namespace": "tenant-b"
    }' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern

# List running patterns
grpcurl -plaintext \
    localhost:8080 prism.launcher.PatternLauncher/ListPatterns
```

**Expected**: 2 processes running (one per namespace)

```json
{
  "patterns": [
    {
      "patternName": "hello-pattern",
      "processId": "ns:tenant-a:hello-pattern",
      "state": "STATE_RUNNING",
      "namespace": "tenant-a"
    },
    {
      "patternName": "hello-pattern",
      "processId": "ns:tenant-b:hello-pattern",
      "state": "STATE_RUNNING",
      "namespace": "tenant-b"
    }
  ],
  "totalCount": 2
}
```

### Test 2: Session Isolation (Different Users = Different Processes)

```bash
# Launch for user-1
grpcurl -plaintext \
    -d '{
        "pattern_name": "hello-pattern",
        "isolation": "ISOLATION_SESSION",
        "namespace": "tenant-a",
        "session_id": "user-1"
    }' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern

# Launch for user-2
grpcurl -plaintext \
    -d '{
        "pattern_name": "hello-pattern",
        "isolation": "ISOLATION_SESSION",
        "namespace": "tenant-a",
        "session_id": "user-2"
    }' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern

# List again
grpcurl -plaintext \
    localhost:8080 prism.launcher.PatternLauncher/ListPatterns
```

**Expected**: 4 processes now (2 namespace + 2 session)

### Test 3: None Isolation (Shared Process)

Create a read-only pattern:

```bash
mkdir -p patterns/config-lookup

cat > patterns/config-lookup/main.go << 'EOF'
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

    log.Printf("Config lookup service on :%s", healthPort)
    http.ListenAndServe(":"+healthPort, nil)
}
EOF

cd patterns/config-lookup && go build -o config-lookup main.go && cd ../..
chmod +x patterns/config-lookup/config-lookup

cat > patterns/config-lookup/manifest.yaml << 'EOF'
name: config-lookup
version: 1.0.0
executable: ./config-lookup
isolation_level: none

healthcheck:
  port: 9090
  path: /health
  interval: 30s
  timeout: 5s
EOF

# Restart launcher to pick up new pattern (Ctrl+C and re-run)
```

Launch with NONE isolation:

```bash
# Request 1
grpcurl -plaintext \
    -d '{"pattern_name": "config-lookup", "isolation": "ISOLATION_NONE"}' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern

# Request 2 (should reuse same process)
grpcurl -plaintext \
    -d '{"pattern_name": "config-lookup", "isolation": "ISOLATION_NONE"}' \
    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern
```

**Expected**: Same processId for both requests ("shared:config-lookup")

## Step 6: Test Crash Recovery (2 minutes)

```bash
# Get process ID
grpcurl -plaintext \
    localhost:8080 prism.launcher.PatternLauncher/ListPatterns | grep processId

# Kill the process
ps aux | grep hello-pattern
kill -9 <PID>

# Wait 5 seconds, then check status
sleep 5
grpcurl -plaintext \
    localhost:8080 prism.launcher.PatternLauncher/ListPatterns
```

**Expected**: Process automatically restarted with new PID

## Step 7: Check Metrics (1 minute)

```bash
# Get launcher health
grpcurl -plaintext \
    -d '{"include_processes": true}' \
    localhost:8080 prism.launcher.PatternLauncher/Health
```

**Expected**:
```json
{
  "healthy": true,
  "totalProcesses": 5,
  "runningProcesses": 5,
  "isolationDistribution": {
    "Namespace": 2,
    "Session": 2,
    "None": 1
  }
}
```

## Step 8: Terminate Patterns (30 seconds)

```bash
# Graceful shutdown with 10 second grace period
grpcurl -plaintext \
    -d '{
        "process_id": "ns:tenant-a:hello-pattern",
        "grace_period_secs": 10
    }' \
    localhost:8080 prism.launcher.PatternLauncher/TerminatePattern
```

**Expected**: Process receives SIGTERM, shuts down gracefully

## Quick Reference

### Launch Pattern

```bash
grpcurl -plaintext -d '{
    "pattern_name": "PATTERN_NAME",
    "isolation": "ISOLATION_LEVEL",
    "namespace": "TENANT_ID",
    "session_id": "USER_ID"
}' localhost:8080 prism.launcher.PatternLauncher/LaunchPattern
```

**Isolation levels**:
- `ISOLATION_NONE`: Shared process (stateless lookups)
- `ISOLATION_NAMESPACE`: One per tenant (multi-tenant SaaS)
- `ISOLATION_SESSION`: One per user (high security)

### List Patterns

```bash
grpcurl -plaintext localhost:8080 prism.launcher.PatternLauncher/ListPatterns
```

### Check Health

```bash
grpcurl -plaintext localhost:8080 prism.launcher.PatternLauncher/Health
```

### Terminate Pattern

```bash
grpcurl -plaintext -d '{
    "process_id": "PROCESS_ID",
    "grace_period_secs": 10
}' localhost:8080 prism.launcher.PatternLauncher/TerminatePattern
```

## Common Issues

### "Pattern not found"

```bash
# Check manifest exists
ls -la patterns/*/manifest.yaml

# Check launcher logs for discovery errors
```

### "Health check failed"

```bash
# Test health endpoint directly
curl http://localhost:9090/health

# Check pattern is using HEALTH_PORT from environment
```

### "Process keeps restarting"

```bash
# Check pattern logs
# Pattern stdout/stderr goes to launcher logs

# Test pattern standalone
PATTERN_NAME=test HEALTH_PORT=9999 ./patterns/hello-pattern/hello-pattern
```

## What You Just Did

1. ✅ Created a minimal pattern with health endpoint
2. ✅ Started the launcher service
3. ✅ Launched patterns with all three isolation levels
4. ✅ Verified process isolation (separate PIDs)
5. ✅ Tested automatic crash recovery
6. ✅ Checked metrics and health status
7. ✅ Gracefully terminated patterns

**Time**: ~10 minutes total

## Next Steps

### Use the Go Client

```go
package main

import (
    "context"
    "log"

    pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    conn, _ := grpc.Dial("localhost:8080",
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    defer conn.Close()

    client := pb.NewPatternLauncherClient(conn)

    resp, err := client.LaunchPattern(context.Background(), &pb.LaunchRequest{
        PatternName: "hello-pattern",
        Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
        Namespace:   "my-app",
    })

    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Pattern launched: %s at %s", resp.ProcessId, resp.Address)
}
```

### Use the Builder (Production)

```go
service, err := launcher.NewBuilder().
    WithPatternsDir("/opt/patterns").
    WithProductionDefaults().
    Build()
```

### Add Authentication

```go
// In production, add mTLS or OIDC token validation
grpcServer := grpc.NewServer(
    grpc.Creds(credentials.NewTLS(tlsConfig)),
)
```

## Key Concepts

**Isolation Levels**:
- `NONE`: 1 process total (all tenants share)
- `NAMESPACE`: N processes (one per tenant)
- `SESSION`: M×N processes (one per user per tenant)

**Pattern Requirements**:
- HTTP `/health` endpoint on `HEALTH_PORT`
- Read config from environment variables
- Exit cleanly on SIGTERM

**Automatic Features**:
- Crash detection and restart
- Health monitoring (30s intervals)
- Circuit breaker (5 consecutive errors = terminal)
- Orphan process cleanup (60s intervals)

## Documentation

- Full docs: `pkg/launcher/README.md`
- API reference: `pkg/launcher/doc.go`
- Examples: `pkg/launcher/examples/`
- RFC: `docs-cms/rfcs/RFC-035-pattern-process-launcher.md`

---

**You're ready!** Start building patterns with fault isolation. 🚀
