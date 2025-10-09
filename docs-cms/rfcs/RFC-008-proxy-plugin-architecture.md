---
id: rfc-008
title: RFC-008 Proxy Plugin Architecture
sidebar_label: RFC-008 Plugin Architecture
status: Draft
---

# RFC-008: Proxy Plugin Architecture and Responsibility Separation

**Status**: Draft
**Author**: System
**Created**: 2025-10-08
**Updated**: 2025-10-08

## Abstract

This RFC defines the architectural separation between Prism's **proxy core** (minimal, stable, generic) and **backend plugins** (specialized, extensible, data-source-specific). By reducing the proxy's surface area and offloading backend-specific logic to plugins, we achieve:

1. **Minimal Proxy Core**: Handles networking, configuration, authentication, observability
2. **Backend Plugins**: Implement data-source-specific protocols via secure channels
3. **Clear Boundaries**: Plugins receive configuration, credentials, and tunneled connections
4. **Extensibility**: Add new backends without modifying proxy core
5. **Security**: Plugins operate in isolated contexts with limited capabilities

The proxy becomes a **lightweight orchestrator** that tunnels traffic to specialized shims, rather than a monolithic component that understands every backend protocol.

## Motivation

### Current Challenges

**Monolithic Proxy Problem**:
- Proxy must understand Kafka, NATS, PostgreSQL, Redis, ClickHouse, MinIO protocols
- Each backend adds complexity to proxy codebase
- Testing matrix grows combinatorially (N backends × M features)
- Deployment coupling: Backend changes require proxy redeployment
- Security surface: Proxy vulnerabilities affect all backends

### Desired State

**Plugin-Based Architecture**:
- Proxy knows only about gRPC, HTTP/2, auth, config, metrics
- Backends implemented as **plugins** (WASM, native shared libraries, or sidecar processes)
- Proxy provides **secure channels** to plugins (mTLS, Unix sockets, gRPC streams)
- Plugins handle backend-specific logic (connection pooling, query translation, caching)
- Plugins receive configuration but don't manage it
- Plugins report metrics but don't aggregate them

## Goals

- Define clear responsibilities for proxy vs. plugins
- Establish plugin interface (gRPC-based, extensible)
- Support multiple plugin deployment models (in-process, sidecar, remote)
- Enable hot-reloading of plugins without proxy restart
- Maintain security isolation between proxy and plugins

## Non-Goals

- **Not replacing existing backends**: Existing backends can be wrapped as plugins
- **Not a full plugin ecosystem**: Focus on Prism-maintained plugins initially
- **Not supporting arbitrary code**: Plugins must conform to secure interface

## Responsibility Separation

### Proxy Core Responsibilities

| Responsibility              | Description                                             |
|-----------------------------|---------------------------------------------------------|
| **Network Termination**     | Accept gRPC/HTTP connections from clients               |
| **Authentication**          | Validate mTLS certificates, OAuth2 tokens               |
| **Authorization**           | Enforce namespace-level access control                  |
| **Configuration Management**| Load, validate, distribute namespace configs to plugins |
| **Routing**                 | Route requests to appropriate backend plugins           |
| **Observability**           | Collect metrics, traces, logs from plugins              |
| **Health Checking**         | Monitor plugin health, restart on failure               |
| **Rate Limiting**           | Apply namespace-level rate limits                       |
| **Circuit Breaking**        | Prevent cascading failures across plugins               |

### Backend Plugin Responsibilities

| Responsibility              | Description                                             |
|-----------------------------|---------------------------------------------------------|
| **Protocol Implementation** | Implement backend-specific wire protocols               |
| **Connection Pooling**      | Manage connections to backend (e.g., PostgreSQL pool)   |
| **Query Translation**       | Translate generic requests to backend-specific queries  |
| **Caching Logic**           | Implement cache strategies (see RFC-007)                |
| **Error Handling**          | Map backend errors to gRPC status codes                 |
| **Schema Management**       | Create tables, indexes, buckets as needed               |
| **Performance Optimization**| Backend-specific optimizations (batching, pipelining)   |
| **Metrics Reporting**       | Report plugin-level metrics to proxy                    |

### What Plugins Do NOT Do

- **No Configuration Storage**: Proxy provides config; plugins consume it
- **No Authentication**: Proxy authenticates clients; plugins trust proxy
- **No Direct Client Access**: Clients always go through proxy
- **No Cross-Plugin Communication**: Plugins are isolated
- **No Global State**: Plugins operate per-namespace

## Plugin Interface

### gRPC-Based Plugin Protocol

```protobuf
syntax = "proto3";

package prism.plugin;

// Backend Plugin Service (implemented by plugins)
service BackendPlugin {
  // Initialize plugin with configuration
  rpc Initialize(InitializeRequest) returns (InitializeResponse);

  // Health check
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);

  // Execute operation (generic interface)
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);

  // Stream operations (for subscriptions, long-polls)
  rpc ExecuteStream(stream StreamRequest) returns (stream StreamResponse);

  // Shutdown gracefully
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}

// Plugin initialization
message InitializeRequest {
  string namespace = 1;
  string backend_type = 2;  // "postgres", "redis", "kafka", etc.

  // Backend-specific configuration (protobuf Any for type-safety, or bytes for zero-copy)
  // Using google.protobuf.Any allows strongly-typed config while maintaining extensibility
  google.protobuf.Any config = 3;

  // Credentials (encrypted in transit)
  map<string, string> credentials = 4;

  // Proxy capabilities
  ProxyCapabilities capabilities = 5;
}

message InitializeResponse {
  bool success = 1;
  string error = 2;

  // Plugin metadata
  string plugin_version = 3;
  repeated string supported_operations = 4;
}

// Generic execute request
message ExecuteRequest {
  string operation = 1;  // "get", "set", "query", "subscribe", etc.

  // Operation-specific parameters (protobuf Any for type-safety, bytes for zero-copy)
  // Using bytes enables zero-copy optimization for large payloads (e.g., object storage)
  oneof params {
    google.protobuf.Any typed_params = 2;  // Strongly-typed parameters
    bytes raw_params = 3;                   // Zero-copy binary data
  }

  // Request metadata (trace ID, user ID, etc.)
  map<string, string> metadata = 4;
}

message ExecuteResponse {
  bool success = 1;
  string error = 2;
  int32 error_code = 3;

  // Response data (protobuf Any for type-safety, bytes for zero-copy)
  oneof result {
    google.protobuf.Any typed_result = 4;  // Strongly-typed response
    bytes raw_result = 5;                   // Zero-copy binary data
  }

  // Plugin metrics
  PluginMetrics metrics = 6;
}

// Streaming for subscriptions, long-running queries
message StreamRequest {
  string operation = 1;

  oneof params {
    google.protobuf.Any typed_params = 2;
    bytes raw_params = 3;
  }
}

message StreamResponse {
  oneof result {
    google.protobuf.Any typed_result = 1;
    bytes raw_result = 2;
  }
  bool is_final = 3;
}

// Health check
message HealthCheckRequest {
  // Optional: check specific backend connection
  optional string connection_id = 1;
}

message HealthCheckResponse {
  enum Status {
    HEALTHY = 0;
    DEGRADED = 1;
    UNHEALTHY = 2;
  }
  Status status = 1;
  string message = 2;
  map<string, string> details = 3;
}

// Plugin metrics (reported to proxy)
// Cache metrics are interval-based: counters accumulate since last fetch, then reset
// This enables accurate rate calculation without client-side state tracking
message PluginMetrics {
  int64 requests_total = 1;
  int64 requests_failed = 2;
  double latency_ms = 3;
  int64 connections_active = 4;

  // Cache metrics (interval-based: reset on fetch)
  // Proxy fetches these periodically (e.g., every 10s), calculates hit rate,
  // then plugin resets counters to zero for next interval
  int64 cache_hits = 5;
  int64 cache_misses = 6;

  // Timestamp when plugin started tracking this interval (UTC nanoseconds)
  int64 interval_start_ns = 7;
}

// Proxy capabilities (what proxy can do for plugins)
message ProxyCapabilities {
  bool supports_metrics_push = 1;
  bool supports_distributed_tracing = 2;
  bool supports_hot_reload = 3;
  string proxy_version = 4;
}
```

## Architecture Diagram

```mermaid
graph TB
    subgraph "Client Layer"
        Client1[Client App 1]
        Client2[Client App 2]
    end

    subgraph "Prism Proxy Core"
        GRPCServer[gRPC Server<br/>Port 50051]
        Auth[Authentication<br/>mTLS / OAuth2]
        Router[Request Router<br/>Namespace → Plugin]
        ConfigMgr[Config Manager<br/>Namespace Configs]
        Metrics[Metrics Aggregator<br/>Prometheus]
        HealthMgr[Health Manager<br/>Plugin Monitoring]
    end

    subgraph "Backend Plugins (In-Process)"
        PGPlugin[PostgreSQL Plugin]
        RedisPlugin[Redis Plugin]
    end

    subgraph "Backend Plugins (Sidecar)"
        KafkaPlugin[Kafka Plugin<br/>Sidecar Process]
        ClickHousePlugin[ClickHouse Plugin<br/>Sidecar Process]
    end

    subgraph "Data Sources"
        Postgres[(PostgreSQL)]
        Redis[(Redis)]
        Kafka[(Kafka)]
        ClickHouse[(ClickHouse)]
    end

    Client1 -->|gRPC| GRPCServer
    Client2 -->|gRPC| GRPCServer
    GRPCServer --> Auth
    Auth --> Router
    Router --> ConfigMgr

    Router -->|Secure Channel| PGPlugin
    Router -->|Secure Channel| RedisPlugin
    Router -->|Unix Socket| KafkaPlugin
    Router -->|gRPC| ClickHousePlugin

    PGPlugin --> Postgres
    RedisPlugin --> Redis
    KafkaPlugin --> Kafka
    ClickHousePlugin --> ClickHouse

    PGPlugin -.Metrics.-> Metrics
    RedisPlugin -.Metrics.-> Metrics
    KafkaPlugin -.Metrics.-> Metrics
    ClickHousePlugin -.Metrics.-> Metrics

    HealthMgr -.Health Check.-> PGPlugin
    HealthMgr -.Health Check.-> RedisPlugin
    HealthMgr -.Health Check.-> KafkaPlugin
    HealthMgr -.Health Check.-> ClickHousePlugin
```

## Zero-Copy Proxying and Performance

### Zero-Copy Data Path

The plugin architecture is designed to enable **zero-copy proxying** for large payloads:

```rust
// Zero-copy example: Object storage GET request
pub async fn handle_get(&self, req: &ExecuteRequest) -> Result<ExecuteResponse> {
    // Extract key from protobuf without copying
    let key = match &req.params {
        Some(params::RawParams(bytes)) => bytes.as_ref(),  // No allocation
        _ => return Err("Invalid params".into()),
    };

    // Fetch object from backend (e.g., MinIO, S3)
    // Returns Arc<Bytes> for reference-counted, zero-copy sharing
    let object_data: Arc<Bytes> = self.client.get_object(key).await?;

    // Return data without copying - gRPC uses the same Arc<Bytes>
    Ok(ExecuteResponse {
        success: true,
        result: Some(result::RawResult(object_data.as_ref().to_vec())),  // gRPC owns bytes
        ..Default::default()
    })
}
```

### gRPC Rust Efficiency

**Tonic** (gRPC Rust implementation) provides excellent zero-copy characteristics:

1. **Tokio Integration**: Uses `Bytes` type for efficient buffer management
2. **Streaming**: Server-streaming enables chunked transfers without buffering
3. **Arc Sharing**: Reference-counted buffers avoid copies between proxy and plugin
4. **Prost Encoding**: Efficient protobuf encoding with minimal allocations

**Performance Benchmarks**:
- In-process plugin: ~0.1ms overhead vs direct backend call
- Sidecar plugin (Unix socket): ~1-2ms overhead
- Remote plugin (gRPC/mTLS): ~5-10ms overhead
- Zero-copy path (>1MB payloads): Negligible overhead regardless of size

### When Zero-Copy Matters

**High-value use cases**:
- Object storage (S3, MinIO): Large blobs (1MB-100MB+)
- Time-series data: Bulk exports, large query results
- Graph queries: Subgraph exports, path traversals
- Batch operations: Multi-get, bulk inserts

**Low-value use cases** (protobuf Any is fine):
- KeyValue operations: Small keys/values (<10KB)
- Session management: Session tokens, metadata
- Configuration updates: Namespace settings

## Plugin Deployment Models

### Recommended Default: Out-of-Process (Sidecar)

**For most backends, use sidecar deployment as the default** to maximize:
- **Fault Isolation**: Plugin crashes don't affect proxy
- **Independent Scaling**: Scale plugins independently of proxy (e.g., compute-heavy ClickHouse aggregations)
- **Language Flexibility**: Implement plugins in Go, Python, Java without Rust FFI constraints
- **Security**: Process-level isolation limits blast radius

**In-process should be reserved for**:
- Ultra-low latency requirements (&lt;1ms P99)
- Backends with minimal dependencies (Redis, Memcached)
- Mature, battle-tested libraries with proven stability

### Model 1: In-Process Plugins (Shared Library)

**Use Case**: Low latency, high throughput backends (Redis, PostgreSQL)

```rust
// Plugin loaded as dynamic library
pub struct RedisPlugin {
    connection_pool: RedisConnectionPool,
    config: RedisConfig,
}

// Plugin implements standard interface
impl BackendPlugin for RedisPlugin {
    async fn initialize(&mut self, req: InitializeRequest) -> Result<InitializeResponse> {
        // Decode protobuf Any to strongly-typed RedisConfig
        self.config = req.config.unpack::<RedisConfig>()?;
        self.connection_pool = RedisConnectionPool::new(&self.config).await?;

        Ok(InitializeResponse {
            success: true,
            plugin_version: env!("CARGO_PKG_VERSION").to_string(),
            supported_operations: vec!["get", "set", "delete", "mget"],
            ..Default::default()
        })
    }

    async fn execute(&self, req: ExecuteRequest) -> Result<ExecuteResponse> {
        match req.operation.as_str() {
            "get" => self.handle_get(&req).await,
            "set" => self.handle_set(&req).await,
            _ => Err(format!("Unsupported operation: {}", req.operation).into()),
        }
    }
}
```

**Pros**:
- Lowest latency (no IPC overhead)
- Shared memory access
- Simplest deployment

**Cons**:
- Plugin crash can crash proxy
- Security: Plugin has proxy's memory access
- Versioning: Plugin must be compatible with proxy ABI

### Model 2: Sidecar Plugins (Separate Process)

**Use Case**: Complex backends with large dependencies (Kafka, ClickHouse)

```yaml
# docker-compose.yml
services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "50051:50051"
    volumes:
      - /var/run/plugins:/var/run/plugins

  kafka-plugin:
    image: prism/kafka-plugin:latest
    volumes:
      - /var/run/plugins:/var/run/plugins
    environment:
      PLUGIN_SOCKET: /var/run/plugins/kafka.sock

  clickhouse-plugin:
    image: prism/clickhouse-plugin:latest
    ports:
      - "50100:50100"
    environment:
      PLUGIN_GRPC_PORT: 50100
```

**Communication**: Unix socket or gRPC over localhost

**Pros**:
- Process isolation (plugin crash doesn't affect proxy)
- Independent deployment and versioning
- Different runtime (e.g., plugin in Python, proxy in Rust)

**Cons**:
- IPC latency (~1-2ms)
- More complex deployment
- Resource overhead (separate process)

### Model 3: Remote Plugins (External Service)

**Use Case**: Proprietary backends, cloud-managed plugins

```yaml
# Namespace config pointing to remote plugin
namespaces:
  - name: custom-backend
    backend: remote
    plugin:
      type: grpc
      endpoint: "custom-plugin.example.com:50100"
      tls:
        enabled: true
        ca_cert: /path/to/ca.pem
```

**Pros**:
- Maximum isolation
- Can run in different regions/clusters
- Proprietary plugin implementations

**Cons**:
- Network latency (10-50ms)
- Requires network security (mTLS)
- Higher operational complexity

## Secure Channels

### Channel Security Requirements

1. **Encryption**: All plugin communication encrypted (TLS, Unix sockets with permissions)
2. **Authentication**: Proxy authenticates plugins (mTLS, shared secrets)
3. **Authorization**: Plugins can only access their namespace's data
4. **Isolation**: Plugins cannot communicate with each other
5. **Audit**: All plugin calls logged with namespace/user context

### Unix Socket Security (Sidecar Model)

```rust
// Proxy creates Unix socket with restricted permissions
let socket_path = "/var/run/plugins/postgres.sock";
let listener = UnixListener::bind(socket_path)?;

// Set permissions: only proxy user can access
std::fs::set_permissions(socket_path, Permissions::from_mode(0o600))?;

// Accept plugin connection
let (stream, _) = listener.accept().await?;

// Wrap in secure channel
let secure_stream = SecureChannel::new(stream, ChannelSecurity::UnixSocket);
```

### gRPC Channel Security (Remote Model)

```rust
// mTLS configuration for remote plugin
let tls = ClientTlsConfig::new()
    .ca_certificate(Certificate::from_pem(ca_cert))
    .identity(Identity::from_pem(client_cert, client_key));

let channel = Channel::from_static("https://plugin.example.com:50100")
    .tls_config(tls)?
    .connect()
    .await?;

let plugin_client = BackendPluginClient::new(channel);
```

## Configuration Flow

### Proxy → Plugin Configuration

```mermaid
sequenceDiagram
    participant Proxy as Proxy Core
    participant ConfigMgr as Config Manager
    participant Plugin as PostgreSQL Plugin
    participant Postgres as PostgreSQL DB

    Note over Proxy,Postgres: Startup Sequence

    Proxy->>ConfigMgr: Load namespace configs
    ConfigMgr-->>Proxy: Namespaces (user-profiles, etc.)

    Proxy->>Plugin: Initialize(config, credentials)
    activate Plugin
    Plugin->>Postgres: Connect with credentials
    Postgres-->>Plugin: Connection established
    Plugin-->>Proxy: InitializeResponse(success=true)
    deactivate Plugin

    Note over Proxy,Postgres: Runtime Request

    Proxy->>Plugin: Execute(operation="get", key="user:123")
    activate Plugin
    Plugin->>Postgres: SELECT * FROM users WHERE id=123
    Postgres-->>Plugin: User data
    Plugin-->>Proxy: ExecuteResponse(result=<data>)
    deactivate Plugin
```

### Configuration Example

```yaml
# Namespace configuration (managed by proxy)
namespaces:
  - name: user-profiles
    backend: postgres
    plugin:
      type: in_process
      library: libprism_postgres_plugin.so

    # Backend-specific config (passed to plugin)
    config:
      connection_string: "postgres://user:pass@localhost:5432/prism"
      pool_size: 20
      idle_timeout: 300
      statement_cache_size: 100

    # Credentials (encrypted, passed to plugin securely)
    credentials:
      username: "prism_user"
      password: "{{ secret:postgres_password }}"

  - name: event-stream
    backend: kafka
    plugin:
      type: sidecar
      socket: /var/run/plugins/kafka.sock

    config:
      brokers:
        - kafka-1:9092
        - kafka-2:9092
        - kafka-3:9092
      topic_prefix: "prism_"
      consumer_group: "prism-proxy"

    credentials:
      sasl_username: "prism"
      sasl_password: "{{ secret:kafka_password }}"
```

## Hot-Reloading Plugins

### Reload Sequence

```mermaid
sequenceDiagram
    participant Admin as Admin CLI
    participant Proxy as Proxy Core
    participant OldPlugin as Old Plugin v1.2
    participant NewPlugin as New Plugin v1.3
    participant Backend as PostgreSQL

    Admin->>Proxy: Reload plugin(namespace="user-profiles")
    Proxy->>NewPlugin: Initialize(config, credentials)
    NewPlugin->>Backend: Connect
    Backend-->>NewPlugin: Connected
    NewPlugin-->>Proxy: InitializeResponse(success=true)

    Note over Proxy: Drain old plugin (no new requests)

    OldPlugin->>Backend: Complete in-flight requests
    Backend-->>OldPlugin: Responses

    Proxy->>OldPlugin: Shutdown()
    OldPlugin->>Backend: Close connections
    OldPlugin-->>Proxy: ShutdownResponse

    Proxy->>Proxy: Swap old → new plugin

    Note over Proxy: New plugin handles all requests
```

### Reload Trigger

```bash
# Admin CLI triggers plugin reload
prism plugin reload user-profiles --version v1.3

# Or via API
curl -X POST https://proxy:50052/admin/plugin/reload \
  -d '{"namespace": "user-profiles", "version": "v1.3"}'
```

## Metrics and Observability

### Plugin-Reported Metrics

```protobuf
message PluginMetrics {
  // Request metrics
  int64 requests_total = 1;
  int64 requests_failed = 2;
  double latency_ms = 3;

  // Backend metrics
  int64 connections_active = 4;
  int64 connections_idle = 5;
  int64 queries_executed = 6;

  // Cache metrics (if applicable)
  int64 cache_hits = 7;
  int64 cache_misses = 8;

  // Custom backend-specific metrics (strongly-typed via protobuf Any)
  google.protobuf.Any custom_metrics = 9;
}
```

### Proxy Aggregation

```rust
// Proxy aggregates plugin metrics
pub struct MetricsAggregator {
    plugin_metrics: HashMap<String, PluginMetrics>,
}

impl MetricsAggregator {
    pub fn record_plugin_metrics(&mut self, namespace: &str, metrics: PluginMetrics) {
        // Store latest metrics
        self.plugin_metrics.insert(namespace.to_string(), metrics);

        // Export to Prometheus
        metrics::gauge!("plugin_requests_total", metrics.requests_total as f64,
            "namespace" => namespace);
        metrics::gauge!("plugin_connections_active", metrics.connections_active as f64,
            "namespace" => namespace);
        // ...
    }
}
```

## Testing Strategy

### Plugin Testing

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_plugin_lifecycle() {
        let mut plugin = PostgresPlugin::new();

        // Initialize with strongly-typed config
        let config = PostgresConfig {
            connection_string: "postgres://localhost".to_string(),
            ..Default::default()
        };
        let init_req = InitializeRequest {
            namespace: "test".to_string(),
            config: Some(Any::pack(&config)?),
            ..Default::default()
        };
        let init_resp = plugin.initialize(init_req).await.unwrap();
        assert!(init_resp.success);

        // Execute with typed params
        let params = GetRequest {
            key: "test:123".to_string(),
        };
        let exec_req = ExecuteRequest {
            operation: "get".to_string(),
            params: Some(params::TypedParams(Any::pack(&params)?)),
            ..Default::default()
        };
        let exec_resp = plugin.execute(exec_req).await.unwrap();
        assert!(exec_resp.success);

        // Shutdown
        let shutdown_resp = plugin.shutdown(ShutdownRequest {}).await.unwrap();
        assert!(shutdown_resp.success);
    }
}
```

### Integration Testing with Mock Proxy

```rust
// Mock proxy provides plugin interface
struct MockProxy {
    config: NamespaceConfig,
}

impl MockProxy {
    async fn test_plugin(plugin: &dyn BackendPlugin) {
        // Initialize plugin
        plugin.initialize(...).await.unwrap();

        // Run test scenarios
        // ...

        // Shutdown
        plugin.shutdown(...).await.unwrap();
    }
}
```

## Migration Path

### Phase 1: Plugin Interface Definition (Week 1-2)

1. **Protobuf Service**: Define BackendPlugin gRPC service
2. **Rust Trait**: Define `BackendPlugin` trait for in-process plugins
3. **Plugin Manager**: Proxy component to load/manage plugins
4. **Documentation**: Plugin development guide

**Deliverable**: Plugin interface specification

### Phase 2: First Plugin (PostgreSQL) (Week 3-4)

1. **Wrap Existing Backend**: Convert PostgreSQL backend to plugin
2. **In-Process Loading**: Dynamic library loading in proxy
3. **Testing**: Integration tests with plugin model
4. **Metrics**: Plugin metrics reporting

**Deliverable**: PostgreSQL plugin (backward compatible)

### Phase 3: Sidecar Model (Week 5-6)

1. **Unix Socket Channel**: Proxy ↔ Plugin communication
2. **Kafka Plugin**: Implement as sidecar
3. **Docker Compose**: Multi-container deployment
4. **Health Checks**: Plugin health monitoring

**Deliverable**: Sidecar plugin deployment

### Phase 4: Hot-Reload and Remote Plugins (Week 7-8)

1. **Hot-Reload**: Swap plugins without proxy restart
2. **gRPC Remote Plugins**: Support external plugin services
3. **Security Hardening**: mTLS, credential encryption
4. **Admin CLI**: Plugin management commands

**Deliverable**: Production-ready plugin system

## Security Considerations

### Plugin Isolation

- **Process Isolation**: Sidecar plugins run in separate processes
- **Resource Limits**: cgroups for CPU/memory limits per plugin
- **Network Isolation**: Plugins can only access their backend
- **Credential Encryption**: Credentials encrypted in transit to plugins
- **Audit Logging**: All plugin operations logged with namespace context

### Plugin Verification

```rust
// Verify plugin before loading
pub fn verify_plugin(plugin_path: &Path) -> Result<()> {
    // 1. Check file permissions (must be owned by prism user)
    let metadata = std::fs::metadata(plugin_path)?;
    let permissions = metadata.permissions();
    if permissions.mode() & 0o002 != 0 {
        return Err("Plugin is world-writable".into());
    }

    // 2. Verify signature (if applicable)
    let signature = std::fs::read(format!("{}.sig", plugin_path.display()))?;
    verify_signature(plugin_path, &signature)?;

    // 3. Load and check version compatibility
    let plugin = load_plugin(plugin_path)?;
    if !is_compatible_version(&plugin.version()) {
        return Err("Plugin version incompatible".into());
    }

    Ok(())
}
```

## Netflix Architecture Comparison

### Netflix Data Gateway Architecture

Netflix's Data Gateway provides valuable insights for plugin architecture design:

**Netflix Approach**:
- **Monolithic Gateway**: Single JVM process with all backend clients embedded
- **Library-Based Backends**: Each backend (Cassandra, EVCache, etc.) as JVM library
- **Shared Resource Pool**: Thread pools, connection pools shared across backends
- **Tight Coupling**: Backend updates require gateway redeployment

**Netflix Strengths** (we adopt):
- **Unified Interface**: Single API for all data access ✓
- **Namespace Abstraction**: Logical separation of tenants ✓
- **Shadow Traffic**: Enable zero-downtime migrations ✓
- **Client-Driven Config**: Applications declare requirements ✓

**Netflix Limitations** (we improve):
- **JVM Performance**: 10-100x slower than Rust for proxying
- **Deployment Coupling**: Backend changes require full gateway redeploy
- **Language Lock-In**: All backends must be JVM-compatible
- **Fault Isolation**: One backend crash can affect entire gateway
- **Scaling Granularity**: Can't scale individual backends independently

### Prism Improvements

| Aspect | Netflix | Prism |
|--------|---------|-------|
| **Runtime** | JVM (high latency, GC pauses) | Rust (microsecond latency, no GC) |
| **Backend Coupling** | Tight (library-based) | Loose (plugin-based) |
| **Fault Isolation** | Shared process | Separate processes (sidecar) |
| **Language Flexibility** | JVM only | Any language (gRPC interface) |
| **Deployment** | Monolithic | Independent plugin deployment |
| **Scaling** | Gateway-level only | Per-plugin scaling |
| **Performance** | ~5-10ms overhead | &lt;1ms (in-process), ~1-2ms (sidecar) |

### Lessons from Other DAL Implementations

**Vitess (YouTube)**: MySQL proxy with query rewriting
- ✅ **Plugin model**: VTGate routes to VTTablet plugins
- ✅ **gRPC-based**: Same approach as Prism
- ❌ **MySQL-specific**: Limited to one backend type

**Envoy Proxy**: L7 proxy with filter chains
- ✅ **WASM plugins**: Sandboxed extension model
- ✅ **Zero-copy**: Efficient buffer management
- ❌ **HTTP-focused**: Not designed for data access patterns

**Linkerd Service Mesh**: Rust-based proxy
- ✅ **Rust performance**: Similar performance characteristics
- ✅ **Process isolation**: Sidecar model
- ❌ **L4/L7 only**: Not data-access aware

**Prism's Unique Position**:
- Combines Netflix's data access abstraction
- With Envoy's performance and extensibility
- Purpose-built for heterogeneous data backends
- Rust performance + plugin flexibility

## Related RFCs and ADRs

- RFC-003: Admin gRPC API (proxy management)
- RFC-004: Redis Integration (example backend → plugin)
- RFC-007: Cache Strategies (plugin-level caching)
- ADR-010: Redis Integration (backend implementation)
- See `docs-cms/netflix/` for Netflix Data Gateway analysis

## References

- [gRPC Plugin System Design](https://grpc.io/docs/what-is-grpc/introduction/)
- [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin)
- [WebAssembly Component Model](https://github.com/WebAssembly/component-model)
- [Linux Capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)

## Appendix: Plugin Development Guide

### Creating a New Plugin

1. **Implement BackendPlugin Trait**:

```rust
use prism_plugin::{BackendPlugin, InitializeRequest, ExecuteRequest};

pub struct MyBackendPlugin {
    config: MyConfig,
    client: MyBackendClient,
}

#[async_trait]
impl BackendPlugin for MyBackendPlugin {
    async fn initialize(&mut self, req: InitializeRequest) -> Result<InitializeResponse> {
        // Decode protobuf Any to strongly-typed config
        self.config = req.config.unpack::<MyConfig>()?;
        self.client = MyBackendClient::connect(&self.config).await?;

        Ok(InitializeResponse {
            success: true,
            plugin_version: "0.1.0".to_string(),
            supported_operations: vec!["get", "set"],
            ..Default::default()
        })
    }

    async fn execute(&self, req: ExecuteRequest) -> Result<ExecuteResponse> {
        match req.operation.as_str() {
            "get" => self.handle_get(&req).await,
            "set" => self.handle_set(&req).await,
            _ => Err(format!("Unsupported: {}", req.operation).into()),
        }
    }
}
```

2. **Build as Shared Library**:

```toml
[lib]
crate-type = ["cdylib"]  # Dynamic library

[dependencies]
prism-plugin-sdk = "0.1"
```

3. **Register Plugin**:

```yaml
# Add to proxy configuration
plugins:
  - name: my-backend
    library: /path/to/libmy_backend_plugin.so
    type: in_process
```

---

**Status**: Draft
**Next Steps**:
1. Define BackendPlugin gRPC service in protobuf
2. Implement plugin trait in Rust
3. Convert PostgreSQL backend to plugin architecture
4. Document plugin development process
5. Implement sidecar plugin support with Unix sockets
