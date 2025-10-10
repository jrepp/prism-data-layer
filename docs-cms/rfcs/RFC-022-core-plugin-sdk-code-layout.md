---
id: rfc-022
title: "RFC-022: Core Plugin SDK Physical Code Layout"
status: Proposed
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [plugin, sdk, go, library, architecture, code-layout]
---

# RFC-022: Core Plugin SDK Physical Code Layout

## Summary

Define the physical code layout and package structure for the Prism core plugin SDK, making it publishable as a standard Go library (`github.com/prism/plugin-sdk`). This RFC establishes the directory structure, package organization, dependency boundaries, and versioning strategy to enable backend plugin authors to build plugins with a clean, well-organized SDK.

**Goals**:
1. **Clean separation**: Authentication, authorization, storage interfaces, utilities in separate packages
2. **Go idioms**: Follow standard Go project layout conventions
3. **Minimal dependencies**: Only essential external libraries
4. **Versioning**: Semantic versioning with Go modules
5. **Discoverability**: Clear package names and godoc-friendly structure
6. **Extensibility**: Easy to add new interfaces without breaking existing plugins

## Motivation

### Problem

Current plugin implementations have scattered code with unclear boundaries:
- No standard SDK structure for plugin authors to follow
- Authorization, token validation, and audit logging are reimplemented per plugin
- gRPC interceptors, connection management, and lifecycle hooks are duplicated
- No clear versioning strategy for SDK evolution
- Plugin authors need to figure out dependencies and setup from scratch

### Goals

1. **Reusable SDK**: Backend plugin authors import `github.com/prism/plugin-sdk` and get batteries-included functionality
2. **Defense-in-depth**: Authorization layer built into SDK (RFC-019 implementation)
3. **Standard interfaces**: Backend interface contracts from protobuf definitions
4. **Lifecycle management**: Plugin startup, health checks, graceful shutdown
5. **Observability**: Structured logging, metrics, tracing built-in
6. **Testing utilities**: Helpers for plugin integration tests

## Physical Code Layout

### Repository Structure

```text
plugin-sdk/
├── go.mod                      # Module: github.com/prism/plugin-sdk
├── go.sum
├── README.md                   # SDK overview, quick start, examples
├── LICENSE                     # Apache 2.0
├── .github/
│   └── workflows/
│       ├── ci.yml              # Build, test, lint
│       └── release.yml         # Automated releases with tags
├── doc.go                      # Package documentation root
│
├── auth/                       # Package: github.com/prism/plugin-sdk/auth
│   ├── token.go                # Token validation (JWT/OIDC)
│   ├── token_test.go
│   ├── jwks.go                 # JWKS caching
│   ├── jwks_test.go
│   ├── claims.go               # Token claims extraction
│   └── doc.go                  # Package documentation
│
├── authz/                      # Package: github.com/prism/plugin-sdk/authz
│   ├── topaz.go                # Topaz client for policy checks
│   ├── topaz_test.go
│   ├── cache.go                # Decision caching (5s TTL)
│   ├── cache_test.go
│   ├── policy.go               # Policy decision types
│   └── doc.go                  # Package documentation
│
├── audit/                      # Package: github.com/prism/plugin-sdk/audit
│   ├── logger.go               # Async audit logger
│   ├── logger_test.go
│   ├── event.go                # Audit event types
│   ├── buffer.go               # Buffered event channel
│   └── doc.go                  # Package documentation
│
├── plugin/                     # Package: github.com/prism/plugin-sdk/plugin
│   ├── server.go               # gRPC server setup
│   ├── server_test.go
│   ├── lifecycle.go            # Startup, health, shutdown hooks
│   ├── lifecycle_test.go
│   ├── config.go               # Plugin configuration loading
│   ├── config_test.go
│   ├── interceptor.go          # gRPC interceptors (auth, logging)
│   ├── interceptor_test.go
│   └── doc.go                  # Package documentation
│
├── interfaces/                 # Package: github.com/prism/plugin-sdk/interfaces
│   ├── keyvalue.go             # KeyValue interface contracts
│   ├── pubsub.go               # PubSub interface contracts
│   ├── stream.go               # Stream interface contracts
│   ├── queue.go                # Queue interface contracts
│   ├── list.go                 # List interface contracts
│   ├── set.go                  # Set interface contracts
│   ├── sortedset.go            # SortedSet interface contracts
│   ├── timeseries.go           # TimeSeries interface contracts
│   ├── graph.go                # Graph interface contracts
│   ├── document.go             # Document interface contracts
│   └── doc.go                  # Package documentation
│
├── storage/                    # Package: github.com/prism/plugin-sdk/storage
│   ├── connection.go           # Connection pooling helpers
│   ├── connection_test.go
│   ├── retry.go                # Retry logic with backoff
│   ├── retry_test.go
│   ├── health.go               # Health check helpers
│   ├── health_test.go
│   └── doc.go                  # Package documentation
│
├── observability/              # Package: github.com/prism/plugin-sdk/observability
│   ├── logging.go              # Structured logging (zap wrapper)
│   ├── logging_test.go
│   ├── metrics.go              # Prometheus metrics helpers
│   ├── metrics_test.go
│   ├── tracing.go              # OpenTelemetry tracing helpers
│   ├── tracing_test.go
│   └── doc.go                  # Package documentation
│
├── testing/                    # Package: github.com/prism/plugin-sdk/testing
│   ├── mock_auth.go            # Mock token validator
│   ├── mock_authz.go           # Mock policy checker
│   ├── mock_audit.go           # Mock audit logger
│   ├── testserver.go           # Test gRPC server helper
│   ├── fixtures.go             # Test fixtures (tokens, configs)
│   └── doc.go                  # Package documentation
│
├── errors/                     # Package: github.com/prism/plugin-sdk/errors
│   ├── errors.go               # Standard error types
│   ├── grpc.go                 # gRPC status code mapping
│   └── doc.go                  # Package documentation
│
├── proto/                      # Generated protobuf code
│   ├── keyvalue/               # KeyValue interface protos
│   │   ├── keyvalue_basic.pb.go
│   │   ├── keyvalue_scan.pb.go
│   │   ├── keyvalue_ttl.pb.go
│   │   └── ...
│   ├── pubsub/                 # PubSub interface protos
│   ├── stream/                 # Stream interface protos
│   ├── queue/                  # Queue interface protos
│   └── ...                     # Other interfaces
│
├── examples/                   # Example plugins (not part of SDK)
│   ├── memstore/               # MemStore example
│   │   ├── main.go
│   │   ├── keyvalue.go
│   │   └── list.go
│   ├── redis/                  # Redis example
│   │   ├── main.go
│   │   └── client.go
│   └── postgres/               # Postgres example
│       ├── main.go
│       └── pool.go
│
└── tools/                      # Build and generation tools
    ├── proto-gen.sh            # Protobuf code generation
    └── release.sh              # Release automation
```

## Package Descriptions

### 1. `auth` - Token Validation

**Purpose**: JWT/OIDC token validation with JWKS caching

**Exported Types**:
```go
type TokenValidator interface {
    Validate(ctx context.Context, token string) (*Claims, error)
    InvalidateCache()
}

type Claims struct {
    Subject   string
    Issuer    string
    Audience  []string
    ExpiresAt time.Time
    IssuedAt  time.Time
    Custom    map[string]interface{}
}

type JWKSCache interface {
    GetKey(kid string) (*rsa.PublicKey, error)
    Refresh() error
}
```

**Configuration**:
```go
type TokenValidatorConfig struct {
    JWKSEndpoint    string
    CacheTTL        time.Duration  // Default: 1 hour
    AllowedIssuers  []string
    AllowedAudiences []string
}
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/auth"

validator, err := auth.NewTokenValidator(&auth.TokenValidatorConfig{
    JWKSEndpoint: "https://dex.local/keys",
    CacheTTL:     1 * time.Hour,
})

claims, err := validator.Validate(ctx, tokenString)
fmt.Printf("User: %s\n", claims.Subject)
```

### 2. `authz` - Policy-Based Authorization

**Purpose**: Topaz integration for policy checks with decision caching

**Exported Types**:
```go
type PolicyChecker interface {
    Check(ctx context.Context, req *AuthzRequest) (*Decision, error)
    InvalidateCache()
}

type AuthzRequest struct {
    Subject  string
    Action   string
    Resource string
    Context  map[string]interface{}
}

type Decision struct {
    Allowed bool
    Reason  string
    CachedAt time.Time
}
```

**Configuration**:
```go
type TopazConfig struct {
    Endpoint     string
    CacheTTL     time.Duration  // Default: 5 seconds
    FailOpen     bool           // Default: false (fail-closed)
}
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/authz"

checker, err := authz.NewTopazClient(&authz.TopazConfig{
    Endpoint: "localhost:8282",
    CacheTTL: 5 * time.Second,
})

decision, err := checker.Check(ctx, &authz.AuthzRequest{
    Subject:  "user:alice",
    Action:   "read",
    Resource: "namespace:production",
})

if !decision.Allowed {
    return errors.New("access denied")
}
```

### 3. `audit` - Audit Logging

**Purpose**: Async audit logging with buffered events

**Exported Types**:
```go
type AuditLogger interface {
    LogAccess(ctx context.Context, event *AccessEvent) error
    Flush() error
    Close() error
}

type AccessEvent struct {
    Timestamp   time.Time
    Subject     string
    Action      string
    Resource    string
    Outcome     string  // "allow" | "deny"
    Latency     time.Duration
    Metadata    map[string]interface{}
}
```

**Configuration**:
```go
type AuditConfig struct {
    Destination string  // "stdout" | "file" | "syslog" | "kafka"
    BufferSize  int     // Default: 1000
    FlushInterval time.Duration  // Default: 1 second
}
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/audit"

logger, err := audit.NewAuditLogger(&audit.AuditConfig{
    Destination: "stdout",
    BufferSize:  1000,
})
defer logger.Close()

logger.LogAccess(ctx, &audit.AccessEvent{
    Subject:  "user:alice",
    Action:   "keyvalue.Set",
    Resource: "namespace:production/key:user:123",
    Outcome:  "allow",
})
```

### 4. `plugin` - Plugin Lifecycle and Server

**Purpose**: gRPC server setup, lifecycle hooks, interceptors

**Exported Types**:
```go
type Plugin interface {
    Name() string
    Version() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    HealthCheck(ctx context.Context) (*HealthStatus, error)
}

type Server struct {
    Config      *ServerConfig
    GRPCServer  *grpc.Server
    Interceptors []grpc.UnaryServerInterceptor
}

type ServerConfig struct {
    ListenAddress string
    MaxConns      int
    EnableAuth    bool
    EnableAuthz   bool
    EnableAudit   bool
}
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/plugin"

server := plugin.NewServer(&plugin.ServerConfig{
    ListenAddress: ":50051",
    EnableAuth:    true,
    EnableAuthz:   true,
    EnableAudit:   true,
})

// Register services
pb.RegisterKeyValueBasicInterfaceServer(server.GRPCServer, myPlugin)

// Start server
if err := server.Start(); err != nil {
    log.Fatal(err)
}
defer server.Stop()
```

### 5. `interfaces` - Backend Interface Contracts

**Purpose**: Go interface definitions matching protobuf services

**Exported Types**:
```go
// KeyValue interfaces
type KeyValueBasic interface {
    Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error)
    Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error)
    Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error)
    Exists(ctx context.Context, req *pb.ExistsRequest) (*pb.ExistsResponse, error)
}

type KeyValueScan interface {
    Scan(req *pb.ScanRequest, stream pb.KeyValueScanInterface_ScanServer) error
    ScanKeys(req *pb.ScanKeysRequest, stream pb.KeyValueScanInterface_ScanKeysServer) error
    Count(ctx context.Context, req *pb.CountRequest) (*pb.CountResponse, error)
}

type KeyValueTTL interface {
    Expire(ctx context.Context, req *pb.ExpireRequest) (*pb.ExpireResponse, error)
    GetTTL(ctx context.Context, req *pb.GetTTLRequest) (*pb.GetTTLResponse, error)
    Persist(ctx context.Context, req *pb.PersistRequest) (*pb.PersistResponse, error)
}

// PubSub interfaces
type PubSubBasic interface {
    Publish(ctx context.Context, req *pb.PublishRequest) (*pb.PublishResponse, error)
    Subscribe(req *pb.SubscribeRequest, stream pb.PubSubBasicInterface_SubscribeServer) error
    Unsubscribe(ctx context.Context, req *pb.UnsubscribeRequest) (*pb.UnsubscribeResponse, error)
}

// Queue interfaces
type QueueBasic interface {
    Enqueue(ctx context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error)
    Dequeue(ctx context.Context, req *pb.DequeueRequest) (*pb.DequeueResponse, error)
    Peek(ctx context.Context, req *pb.PeekRequest) (*pb.PeekResponse, error)
}

// ... other interfaces
```

**Usage**: Plugin implementations satisfy these interfaces:
```go
type MyPlugin struct {
    // ... fields
}

// Implement KeyValueBasic interface
func (p *MyPlugin) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
    // Implementation
}
```

### 6. `storage` - Storage Utilities

**Purpose**: Connection pooling, retry logic, health checks

**Exported Types**:
```go
type ConnectionPool interface {
    Get(ctx context.Context) (Connection, error)
    Put(conn Connection) error
    Close() error
}

type RetryPolicy struct {
    MaxAttempts int
    InitialBackoff time.Duration
    MaxBackoff time.Duration
    Multiplier float64
}

func WithRetry(ctx context.Context, policy *RetryPolicy, fn func() error) error
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/storage"

policy := &storage.RetryPolicy{
    MaxAttempts:    3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     5 * time.Second,
    Multiplier:     2.0,
}

err := storage.WithRetry(ctx, policy, func() error {
    return db.Exec("INSERT INTO ...")
})
```

### 7. `observability` - Logging, Metrics, Tracing

**Purpose**: Structured logging, Prometheus metrics, OpenTelemetry tracing

**Exported Types**:
```go
type Logger interface {
    Info(msg string, fields ...Field)
    Error(msg string, err error, fields ...Field)
    Debug(msg string, fields ...Field)
    With(fields ...Field) Logger
}

type MetricsRegistry interface {
    Counter(name string, labels ...string) Counter
    Gauge(name string, labels ...string) Gauge
    Histogram(name string, buckets []float64, labels ...string) Histogram
}

type Tracer interface {
    StartSpan(ctx context.Context, name string) (context.Context, Span)
}
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/observability"

logger := observability.NewLogger(&observability.LogConfig{
    Level: "info",
    Format: "json",
})

logger.Info("Request received",
    observability.String("method", "Set"),
    observability.String("key", req.Key),
)

metrics := observability.NewMetrics()
requestCounter := metrics.Counter("plugin_requests_total", "method", "status")
requestCounter.Inc("Set", "success")
```

### 8. `testing` - Test Utilities

**Purpose**: Mock implementations, test fixtures, test server helpers

**Exported Types**:
```go
type MockTokenValidator struct {
    ValidateFunc func(ctx context.Context, token string) (*auth.Claims, error)
}

type MockPolicyChecker struct {
    CheckFunc func(ctx context.Context, req *authz.AuthzRequest) (*authz.Decision, error)
}

type TestServer struct {
    Server *grpc.Server
    Port   int
}

func NewTestServer(plugin interface{}) (*TestServer, error)
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/testing"

// Mock token validator for tests
mockAuth := &testing.MockTokenValidator{
    ValidateFunc: func(ctx context.Context, token string) (*auth.Claims, error) {
        return &auth.Claims{Subject: "test-user"}, nil
    },
}

// Test server
testServer, err := testing.NewTestServer(myPlugin)
defer testServer.Stop()

conn, _ := grpc.Dial(fmt.Sprintf("localhost:%d", testServer.Port), grpc.WithInsecure())
client := pb.NewKeyValueBasicInterfaceClient(conn)
```

### 9. `errors` - Standard Error Types

**Purpose**: Standard error types with gRPC status code mapping

**Exported Types**:
```go
var (
    ErrNotFound        = errors.New("not found")
    ErrAlreadyExists   = errors.New("already exists")
    ErrInvalidArgument = errors.New("invalid argument")
    ErrPermissionDenied = errors.New("permission denied")
    ErrUnauthenticated = errors.New("unauthenticated")
    ErrInternal        = errors.New("internal error")
)

func ToGRPCStatus(err error) *status.Status
func FromGRPCStatus(st *status.Status) error
```

**Usage Example**:
```go
import "github.com/prism/plugin-sdk/errors"

if key == "" {
    return nil, errors.ErrInvalidArgument
}

if !found {
    return nil, errors.ErrNotFound
}
```

## Dependency Management

### External Dependencies (Minimal Set)

```go
// go.mod
module github.com/prism/plugin-sdk

go 1.21

require (
    // gRPC and protobuf
    google.golang.org/grpc v1.58.0
    google.golang.org/protobuf v1.31.0

    // Auth (JWT validation)
    github.com/golang-jwt/jwt/v5 v5.0.0
    github.com/lestrrat-go/jwx/v2 v2.0.11

    // Authz (Topaz client)
    github.com/aserto-dev/go-authorizer v0.20.0
    github.com/aserto-dev/go-grpc-authz v0.8.0

    // Observability
    go.uber.org/zap v1.25.0
    github.com/prometheus/client_golang v1.16.0
    go.opentelemetry.io/otel v1.16.0
    go.opentelemetry.io/otel/trace v1.16.0
)
```

**Rationale**:
- **gRPC/Protobuf**: Core communication protocol
- **JWT libraries**: Token validation (JWKS, claims parsing)
- **Topaz SDK**: Aserto's official Go client for policy checks
- **Zap**: High-performance structured logging
- **Prometheus**: Standard metrics library
- **OpenTelemetry**: Distributed tracing standard

### Dependency Boundaries

```text
┌─────────────────────────────────────────────────────────────┐
│                      Plugin Implementation                   │
│                  (Backend-specific code)                     │
└────────────────────────────┬────────────────────────────────┘
                             │
                             │ imports
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    plugin-sdk/plugin                         │
│              (Server, Lifecycle, Interceptors)               │
└──┬──────────────────────┬──────────────────┬────────────────┘
   │                      │                  │
   │ imports              │ imports          │ imports
   ▼                      ▼                  ▼
┌──────────┐      ┌──────────┐      ┌──────────┐
│   auth   │      │   authz  │      │  audit   │
└──────────┘      └──────────┘      └──────────┘
   │                      │                  │
   │                      │                  │
   ▼                      ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│                   External Dependencies                      │
│       (gRPC, JWT, Topaz, Zap, Prometheus, OTel)             │
└─────────────────────────────────────────────────────────────┘
```

**Rules**:
1. **No circular dependencies**: Packages must have clear import hierarchy
2. **Minimal external deps**: Only add dependencies that provide significant value
3. **Interface boundaries**: Packages export interfaces, not concrete types where possible
4. **Testing isolation**: `testing` package has no dependencies on `auth`, `authz`, `audit`

## Versioning Strategy

### Semantic Versioning

```text
v0.1.0 - Initial release (POC 1)
v0.2.0 - Add PubSub interfaces (POC 2)
v0.3.0 - Add Stream interfaces (POC 3)
v1.0.0 - Stable API (all core interfaces)
v1.1.0 - Add new optional interface (backward compatible)
v2.0.0 - Breaking change (e.g., change interface signature)
```

**Go Modules**:
```bash
# Install specific version
go get github.com/prism/plugin-sdk@v0.1.0

# Install latest
go get github.com/prism/plugin-sdk@latest

# Install pre-release
go get github.com/prism/plugin-sdk@v0.2.0-beta.1
```

### Version Compatibility

**Backward Compatibility Rules**:
1. **Adding interfaces**: Non-breaking (plugins can ignore new interfaces)
2. **Adding methods to interfaces**: Breaking (requires major version bump)
3. **Adding optional fields to configs**: Non-breaking (use pointers for optionality)
4. **Changing function signatures**: Breaking (requires major version bump)

**Deprecation Policy**:
```go
// Deprecated: Use NewTokenValidator instead
func NewValidator(cfg *Config) (*Validator, error) {
    // Old implementation kept for 2 minor versions
}
```

## Example Plugin Using SDK

### Complete MemStore Plugin

```go
// plugins/memstore/main.go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/prism/plugin-sdk/plugin"
    "github.com/prism/plugin-sdk/observability"
    pb "github.com/prism/plugin-sdk/proto/keyvalue"
)

func main() {
    // Initialize logger
    logger := observability.NewLogger(&observability.LogConfig{
        Level:  "info",
        Format: "json",
    })

    // Create plugin instance
    memstore := NewMemStorePlugin(logger)

    // Configure server
    server := plugin.NewServer(&plugin.ServerConfig{
        ListenAddress: ":50051",
        EnableAuth:    true,
        EnableAuthz:   true,
        EnableAudit:   true,
    })

    // Register services
    pb.RegisterKeyValueBasicInterfaceServer(server.GRPCServer, memstore)
    pb.RegisterKeyValueTTLInterfaceServer(server.GRPCServer, memstore)

    // Start server
    if err := server.Start(); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }

    logger.Info("MemStore plugin started", observability.Int("port", 50051))

    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    logger.Info("Shutting down...")
    server.Stop()
}
```

```go
// plugins/memstore/plugin.go
package main

import (
    "context"
    "sync"
    "time"

    "github.com/prism/plugin-sdk/interfaces"
    "github.com/prism/plugin-sdk/observability"
    "github.com/prism/plugin-sdk/errors"
    pb "github.com/prism/plugin-sdk/proto/keyvalue"
)

type MemStorePlugin struct {
    pb.UnimplementedKeyValueBasicInterfaceServer
    pb.UnimplementedKeyValueTTLInterfaceServer

    data   sync.Map  // map[string][]byte
    ttls   sync.Map  // map[string]*time.Timer
    logger observability.Logger
}

func NewMemStorePlugin(logger observability.Logger) *MemStorePlugin {
    return &MemStorePlugin{
        logger: logger,
    }
}

// Implement KeyValueBasic interface
func (m *MemStorePlugin) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
    m.logger.Info("Set operation",
        observability.String("key", req.Key),
        observability.Int("value_size", len(req.Value)),
    )

    m.data.Store(req.Key, req.Value)

    if req.TtlSeconds > 0 {
        m.setTTL(req.Key, time.Duration(req.TtlSeconds)*time.Second)
    }

    return &pb.SetResponse{Success: true}, nil
}

func (m *MemStorePlugin) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
    value, ok := m.data.Load(req.Key)
    if !ok {
        return nil, errors.ErrNotFound
    }

    return &pb.GetResponse{Value: value.([]byte)}, nil
}

func (m *MemStorePlugin) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
    _, found := m.data.LoadAndDelete(req.Key)

    if timer, ok := m.ttls.LoadAndDelete(req.Key); ok {
        timer.(*time.Timer).Stop()
    }

    return &pb.DeleteResponse{Found: found}, nil
}

func (m *MemStorePlugin) Exists(ctx context.Context, req *pb.ExistsRequest) (*pb.ExistsResponse, error) {
    _, ok := m.data.Load(req.Key)
    return &pb.ExistsResponse{Exists: ok}, nil
}

// Implement KeyValueTTL interface
func (m *MemStorePlugin) Expire(ctx context.Context, req *pb.ExpireRequest) (*pb.ExpireResponse, error) {
    m.setTTL(req.Key, time.Duration(req.Seconds)*time.Second)
    return &pb.ExpireResponse{Success: true}, nil
}

func (m *MemStorePlugin) GetTTL(ctx context.Context, req *pb.GetTTLRequest) (*pb.GetTTLResponse, error) {
    // Simplified: not tracking remaining TTL
    return &pb.GetTTLResponse{Seconds: -1}, nil
}

func (m *MemStorePlugin) Persist(ctx context.Context, req *pb.PersistRequest) (*pb.PersistResponse, error) {
    if timer, ok := m.ttls.LoadAndDelete(req.Key); ok {
        timer.(*time.Timer).Stop()
        return &pb.PersistResponse{Success: true}, nil
    }
    return &pb.PersistResponse{Success: false}, nil
}

// Helper methods
func (m *MemStorePlugin) setTTL(key string, duration time.Duration) {
    timer := time.AfterFunc(duration, func() {
        m.data.Delete(key)
        m.ttls.Delete(key)
    })
    m.ttls.Store(key, timer)
}
```

## SDK Documentation

### README.md

```text
# Prism Plugin SDK

Go SDK for building Prism backend plugins with batteries-included authorization, audit logging, and observability.

## Installation

    go get github.com/prism/plugin-sdk@latest

## Quick Start

    import (
        "github.com/prism/plugin-sdk/plugin"
        pb "github.com/prism/plugin-sdk/proto/keyvalue"
    )

    func main() {
        server := plugin.NewServer(&plugin.ServerConfig{
            ListenAddress: ":50051",
            EnableAuth:    true,
        })

        pb.RegisterKeyValueBasicInterfaceServer(server.GRPCServer, myPlugin)

        server.Start()
    }

## Features

- ✅ **Authentication**: JWT/OIDC token validation with JWKS caching
- ✅ **Authorization**: Topaz policy checks with decision caching
- ✅ **Audit Logging**: Async audit logging with buffered events
- ✅ **Observability**: Structured logging, Prometheus metrics, OpenTelemetry tracing
- ✅ **Testing**: Mock implementations and test utilities
- ✅ **Lifecycle**: Health checks, graceful shutdown
- ✅ **Storage**: Connection pooling, retry logic

## Documentation

- [API Reference](https://pkg.go.dev/github.com/prism/plugin-sdk)
- [Examples](./examples/)
- [RFC-022: SDK Code Layout](https://jrepp.github.io/prism-data-layer/rfc/RFC-022-core-plugin-sdk-code-layout)

## Examples

See [examples/](./examples/) directory for:
- MemStore plugin (in-memory KeyValue + List)
- Redis plugin (KeyValue + PubSub + Stream)
- Postgres plugin (KeyValue + Queue + TimeSeries)

## Versioning

This project uses [Semantic Versioning](https://semver.org/):
- v0.x.x - Pre-1.0 releases (API may change)
- v1.x.x - Stable API (backward compatible)
- v2.x.x - Breaking changes

## License

Apache 2.0 - See [LICENSE](./LICENSE)
```

### godoc Documentation

```go
// Package plugin provides core functionality for building Prism backend plugins.
//
// The plugin-sdk enables backend plugin authors to build production-ready
// plugins with authentication, authorization, audit logging, and observability
// built-in.
//
// Quick Start
//
// Import the SDK and create a plugin server:
//
//     import "github.com/prism/plugin-sdk/plugin"
//
//     server := plugin.NewServer(&plugin.ServerConfig{
//         ListenAddress: ":50051",
//         EnableAuth:    true,
//         EnableAuthz:   true,
//     })
//
// Implement backend interfaces:
//
//     type MyPlugin struct {
//         // fields
//     }
//
//     func (p *MyPlugin) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
//         // implementation
//     }
//
// Register and start:
//
//     pb.RegisterKeyValueBasicInterfaceServer(server.GRPCServer, myPlugin)
//     server.Start()
//
// Authorization
//
// Enable defense-in-depth authorization with Topaz policy checks:
//
//     import "github.com/prism/plugin-sdk/authz"
//
//     checker, err := authz.NewTopazClient(&authz.TopazConfig{
//         Endpoint: "localhost:8282",
//     })
//
// Observability
//
// Structured logging and metrics:
//
//     import "github.com/prism/plugin-sdk/observability"
//
//     logger := observability.NewLogger(&observability.LogConfig{Level: "info"})
//     logger.Info("Request received", observability.String("method", "Set"))
//
// Testing
//
// Use mock implementations for unit tests:
//
//     import "github.com/prism/plugin-sdk/testing"
//
//     mockAuth := &testing.MockTokenValidator{
//         ValidateFunc: func(ctx, token) (*Claims, error) {
//             return &Claims{Subject: "test"}, nil
//         },
//     }
//
package plugin
```

## Build and Release Automation

### Makefile

```makefile
# Makefile
.PHONY: all build test lint proto clean release

all: proto test build

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	./tools/proto-gen.sh

# Build SDK (no binary, just verify compilation)
build:
	@echo "Building SDK..."
	go build ./...

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Lint code
lint:
	@echo "Linting..."
	golangci-lint run ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf proto/*.pb.go

# Release (tag and push)
release:
	@echo "Releasing..."
	./tools/release.sh
```

### GitHub Actions CI

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install dependencies
        run: go mod download

      - name: Run tests
        run: go test -v -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./coverage.out

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: golangci/golangci-lint-action@v3
        with:
          version: latest
```

### GitHub Actions Release

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          generate_release_notes: true
```

## Migration Path

### Phase 1: Extract Core SDK (Week 1)

1. Create `github.com/prism/plugin-sdk` repository
2. Extract `auth`, `authz`, `audit` packages from RFC-019 examples
3. Add `plugin` package with server and lifecycle
4. Add `interfaces` package with Go interface definitions
5. Add `observability` package with logging/metrics/tracing
6. Write tests achieving >80% coverage

### Phase 2: Migrate Existing Plugins (Week 2)

1. Update MemStore plugin to use SDK
2. Update Redis plugin to use SDK (if exists)
3. Update Postgres plugin to use SDK (if exists)
4. Verify all plugins work with new SDK

### Phase 3: Documentation and Examples (Week 3)

1. Write comprehensive README.md
2. Add godoc comments to all exported types
3. Create 3 example plugins (MemStore, Redis, Postgres)
4. Publish SDK to pkg.go.dev

### Phase 4: Stability and v1.0.0 (Week 4)

1. Gather feedback from plugin authors
2. Fix bugs and improve ergonomics
3. Freeze API for v1.0.0 release
4. Tag v1.0.0 and announce

## Benefits

### For Plugin Authors

1. **Faster development**: Import SDK and focus on backend-specific logic
2. **Consistent patterns**: All plugins use same auth/authz/audit patterns
3. **Production-ready**: Observability, health checks, graceful shutdown built-in
4. **Easy testing**: Mock implementations for unit tests
5. **Clear documentation**: godoc and examples for all packages

### For Prism Platform

1. **Defense-in-depth**: All plugins enforce authorization automatically
2. **Audit trail**: Consistent audit logging across all backends
3. **Observability**: Standard metrics and logs from all plugins
4. **Maintainability**: SDK changes propagate to all plugins via version bump
5. **Security**: Centralized security logic reduces attack surface

### For Prism Users

1. **Consistent behavior**: All backends behave similarly (auth, authz, audit)
2. **Reliable plugins**: SDK-based plugins follow best practices
3. **Faster bug fixes**: SDK bugs fixed once, all plugins benefit
4. **Feature parity**: New SDK features available to all backends

## Open Questions

1. **Should SDK include connection pool implementations for common backends (Redis, Postgres, Kafka)?**
   - **Proposal**: Yes, add `storage/redis`, `storage/postgres`, `storage/kafka` sub-packages
   - **Trade-off**: More dependencies vs easier plugin authoring

2. **Should SDK enforce interface implementation at compile time?**
   - **Proposal**: Yes, use interface assertions in `interfaces` package
   - **Example**: `var _ interfaces.KeyValueBasic = (*MyPlugin)(nil)`

3. **Should SDK provide default implementations for optional interfaces?**
   - **Proposal**: Yes, provide "no-op" implementations that return `ErrNotImplemented`
   - **Benefit**: Plugins can embed defaults and override only what they support

4. **How to handle SDK version mismatches between proxy and plugins?**
   - **Proposal**: Include SDK version in plugin metadata, proxy checks compatibility
   - **Enforcement**: Proxy refuses to load plugins with incompatible SDK versions

## Related Documents

- [RFC-019: Plugin SDK Authorization Layer](/rfc/RFC-019-plugin-sdk-authorization-layer) - Authorization implementation
- [RFC-008: Proxy Plugin Architecture](/rfc/RFC-008-proxy-plugin-architecture) - Plugin system overview
- [MEMO-006: Backend Interface Decomposition](/memos/MEMO-006-backend-interface-decomposition-schema-registry) - Interface design principles
- [RFC-021: POC 1 Implementation Plan](/rfc/RFC-021-poc1-keyvalue-memstore-implementation) - MemStore plugin example

## Revision History

- 2025-10-09: Initial RFC defining SDK physical code layout and package structure
