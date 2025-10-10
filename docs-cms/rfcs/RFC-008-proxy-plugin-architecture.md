---
id: rfc-008
title: "RFC-008: Proxy Plugin Architecture"
sidebar_label: RFC-008 Plugin Architecture
status: Draft
author: System
created: 2025-10-08
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
- Zero-copy path (&gt;1MB payloads): Negligible overhead regardless of size

### When Zero-Copy Matters

**High-value use cases**:
- Object storage (S3, MinIO): Large blobs (1MB-100MB+)
- Time-series data: Bulk exports, large query results
- Graph queries: Subgraph exports, path traversals
- Batch operations: Multi-get, bulk inserts

**Low-value use cases** (protobuf Any is fine):
- KeyValue operations: Small keys/values (&lt;10KB)
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

let channel = Channel::from_static("https://".to_string() + "plugin.example.com:50100")
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

## Plugin Acceptance Test Framework

### Overview

The Plugin Acceptance Test Framework provides a comprehensive suite of verification tests that ensure plugins correctly implement the `BackendPlugin` interface and behave consistently across all backend types. This framework serves two critical purposes:

1. **Per-Backend Type Verification**: Test each plugin implementation against real backend instances to verify correct protocol implementation
2. **Cross-Plugin Consistency**: Ensure all plugins handle common concerns (authentication, connection management, error handling) identically

### Test Framework Architecture

```rust
// tests/acceptance/framework.rs

/// Acceptance test harness that runs plugins against real backends
pub struct PluginAcceptanceHarness {
    backend_type: BackendType,
    backend_instance: Box<dyn TestBackend>,
    plugin: Box<dyn BackendPlugin>,
    test_config: TestConfig,
}

impl PluginAcceptanceHarness {
    /// Create harness for specific backend type
    pub async fn new(backend_type: BackendType) -> Result<Self> {
        let backend_instance = spawn_test_backend(backend_type).await?;
        let plugin = load_plugin_for_backend(backend_type)?;

        Ok(Self {
            backend_type,
            backend_instance,
            plugin,
            test_config: TestConfig::default(),
        })
    }

    /// Run full acceptance test suite
    pub async fn run_all_tests(&mut self) -> TestResults {
        let mut results = TestResults::new();

        // Core plugin lifecycle tests
        results.add(self.test_initialize().await);
        results.add(self.test_health_check().await);
        results.add(self.test_shutdown().await);

        // Authentication tests (reusable across all plugins)
        results.add(self.run_authentication_suite().await);

        // Backend-specific operation tests
        results.add(self.run_backend_operations_suite().await);

        // Error handling tests
        results.add(self.test_error_scenarios().await);

        // Performance baseline tests
        results.add(self.test_performance_baseline().await);

        results
    }
}
```

### Reusable Authentication Test Suite

**Goal**: Verify all plugins handle credential passing, authentication failures, and credential refresh identically.

```rust
// tests/acceptance/auth_suite.rs

/// Reusable authentication test suite that works across all plugin types
pub struct AuthenticationTestSuite {
    harness: Arc<PluginAcceptanceHarness>,
}

impl AuthenticationTestSuite {
    pub async fn run(&self) -> Vec<TestResult> {
        vec![
            self.test_valid_credentials().await,
            self.test_invalid_credentials().await,
            self.test_missing_credentials().await,
            self.test_expired_credentials().await,
            self.test_credential_rotation().await,
            self.test_connection_pool_auth().await,
        ]
    }

    /// Test plugin initializes successfully with valid credentials
    async fn test_valid_credentials(&self) -> TestResult {
        let init_req = InitializeRequest {
            namespace: "test-auth".to_string(),
            backend_type: self.harness.backend_type.to_string(),
            config: self.harness.test_config.clone(),
            credentials: hashmap! {
                "username" => "test_user",
                "password" => "test_pass",
            },
            ..Default::default()
        };

        let resp = self.harness.plugin.initialize(init_req).await;

        TestResult::assert_ok(resp, "Plugin should initialize with valid credentials")
    }

    /// Test plugin fails gracefully with invalid credentials
    async fn test_invalid_credentials(&self) -> TestResult {
        let init_req = InitializeRequest {
            namespace: "test-auth-invalid".to_string(),
            backend_type: self.harness.backend_type.to_string(),
            config: self.harness.test_config.clone(),
            credentials: hashmap! {
                "username" => "invalid_user",
                "password" => "wrong_pass",
            },
            ..Default::default()
        };

        let resp = self.harness.plugin.initialize(init_req).await;

        TestResult::assert_err(
            resp,
            "Plugin should reject invalid credentials",
            ErrorCode::UNAUTHENTICATED
        )
    }

    /// Test plugin handles missing credentials
    async fn test_missing_credentials(&self) -> TestResult {
        let init_req = InitializeRequest {
            namespace: "test-auth-missing".to_string(),
            backend_type: self.harness.backend_type.to_string(),
            config: self.harness.test_config.clone(),
            credentials: hashmap! {},  // Empty credentials
            ..Default::default()
        };

        let resp = self.harness.plugin.initialize(init_req).await;

        TestResult::assert_err(
            resp,
            "Plugin should detect missing credentials",
            ErrorCode::INVALID_ARGUMENT
        )
    }

    /// Test plugin handles credential expiration/rotation
    async fn test_credential_rotation(&self) -> TestResult {
        // Initialize with valid credentials
        self.harness.plugin.initialize(valid_creds()).await?;

        // Simulate credential rotation in backend
        self.harness.backend_instance.rotate_credentials().await?;

        // Execute operation - should fail with auth error
        let exec_req = ExecuteRequest {
            operation: "get".to_string(),
            params: test_params(),
            ..Default::default()
        };

        let resp = self.harness.plugin.execute(exec_req).await;

        // Plugin should detect expired credentials
        assert_eq!(resp.error_code, ErrorCode::UNAUTHENTICATED);

        // Reinitialize with new credentials
        self.harness.plugin.initialize(rotated_creds()).await?;

        // Operation should now succeed
        let resp = self.harness.plugin.execute(exec_req.clone()).await;
        TestResult::assert_ok(resp, "Plugin should work after credential rotation")
    }

    /// Test connection pool handles authentication per-connection
    async fn test_connection_pool_auth(&self) -> TestResult {
        // Initialize plugin with pool size > 1
        let init_req = InitializeRequest {
            config: ConnectionPoolConfig {
                pool_size: 5,
                ..Default::default()
            },
            credentials: valid_creds(),
            ..Default::default()
        };

        self.harness.plugin.initialize(init_req).await?;

        // Execute multiple concurrent operations
        let operations: Vec<_> = (0..10)
            .map(|i| {
                let plugin = self.harness.plugin.clone();
                tokio::spawn(async move {
                    plugin.execute(ExecuteRequest {
                        operation: "get".to_string(),
                        params: test_params_for_key(i),
                        ..Default::default()
                    }).await
                })
            })
            .collect();

        // All operations should succeed (each connection authenticated)
        let results: Vec<_> = futures::future::join_all(operations).await;

        TestResult::assert_all_ok(
            results,
            "All pooled connections should authenticate successfully"
        )
    }
}
```

### Per-Backend Verification Tests

**Goal**: Verify each plugin correctly implements backend-specific protocols using actual backend instances.

```rust
// tests/acceptance/backend_verification.rs

/// Backend-specific verification tests
pub trait BackendVerificationSuite {
    async fn test_basic_operations(&self) -> Vec<TestResult>;
    async fn test_error_handling(&self) -> Vec<TestResult>;
    async fn test_concurrency(&self) -> Vec<TestResult>;
    async fn test_backend_specific_features(&self) -> Vec<TestResult>;
}

/// PostgreSQL plugin verification
pub struct PostgresVerificationSuite {
    harness: Arc<PluginAcceptanceHarness>,
    postgres: PostgresTestInstance,
}

impl BackendVerificationSuite for PostgresVerificationSuite {
    async fn test_basic_operations(&self) -> Vec<TestResult> {
        vec![
            self.test_insert().await,
            self.test_select().await,
            self.test_update().await,
            self.test_delete().await,
            self.test_transaction().await,
        ]
    }

    async fn test_backend_specific_features(&self) -> Vec<TestResult> {
        vec![
            self.test_prepared_statements().await,
            self.test_json_types().await,
            self.test_array_types().await,
            self.test_listen_notify().await,
        ]
    }
}

impl PostgresVerificationSuite {
    async fn test_insert(&self) -> TestResult {
        // Insert data via plugin
        let exec_req = ExecuteRequest {
            operation: "insert".to_string(),
            params: InsertParams {
                table: "users".to_string(),
                data: json!({"id": 1, "name": "Alice"}),
            },
            ..Default::default()
        };

        let resp = self.harness.plugin.execute(exec_req).await?;

        // Verify data exists in actual PostgreSQL instance
        let row = self.postgres.query_one("SELECT * FROM users WHERE id = 1").await?;
        assert_eq!(row.get::<_, String>("name"), "Alice");

        TestResult::pass("PostgreSQL plugin correctly inserts data")
    }

    async fn test_prepared_statements(&self) -> TestResult {
        // Execute same query multiple times
        for i in 0..100 {
            let exec_req = ExecuteRequest {
                operation: "query".to_string(),
                params: QueryParams {
                    sql: "SELECT * FROM users WHERE id = $1".to_string(),
                    params: vec![i.into()],
                },
                ..Default::default()
            };

            self.harness.plugin.execute(exec_req).await?;
        }

        // Verify plugin uses prepared statements (check metrics)
        let metrics = self.harness.plugin.get_metrics().await?;
        assert!(
            metrics.prepared_statements_cached > 0,
            "Plugin should cache prepared statements"
        );

        TestResult::pass("PostgreSQL plugin uses prepared statements")
    }
}

/// Kafka plugin verification
pub struct KafkaVerificationSuite {
    harness: Arc<PluginAcceptanceHarness>,
    kafka: KafkaTestCluster,
}

impl BackendVerificationSuite for KafkaVerificationSuite {
    async fn test_basic_operations(&self) -> Vec<TestResult> {
        vec![
            self.test_produce().await,
            self.test_consume().await,
            self.test_subscribe().await,
        ]
    }

    async fn test_backend_specific_features(&self) -> Vec<TestResult> {
        vec![
            self.test_partitioning().await,
            self.test_consumer_groups().await,
            self.test_exactly_once_semantics().await,
            self.test_transactions().await,
        ]
    }
}

impl KafkaVerificationSuite {
    async fn test_produce(&self) -> TestResult {
        // Produce message via plugin
        let exec_req = ExecuteRequest {
            operation: "produce".to_string(),
            params: ProduceParams {
                topic: "test-topic".to_string(),
                key: "key1".to_string(),
                value: b"test message".to_vec(),
            },
            ..Default::default()
        };

        let resp = self.harness.plugin.execute(exec_req).await?;

        // Verify message in actual Kafka cluster
        let consumer = self.kafka.create_consumer("test-group").await?;
        consumer.subscribe(&["test-topic"]).await?;

        let message = consumer.recv().await?;
        assert_eq!(message.payload(), b"test message");

        TestResult::pass("Kafka plugin correctly produces messages")
    }

    async fn test_consumer_groups(&self) -> TestResult {
        // Create multiple consumers in same group
        let consumers = vec![
            self.create_plugin_consumer("group1").await?,
            self.create_plugin_consumer("group1").await?,
            self.create_plugin_consumer("group1").await?,
        ];

        // Produce 100 messages
        for i in 0..100 {
            self.produce_via_plugin(&format!("msg-{}", i)).await?;
        }

        // Each consumer should receive ~33 messages (partitioned)
        let counts: Vec<_> = consumers.iter()
            .map(|c| c.message_count())
            .collect();

        // Verify messages distributed across consumers
        assert!(counts.iter().all(|&c| c > 20 && c < 50));

        TestResult::pass("Kafka plugin correctly partitions messages across consumer group")
    }
}

/// Redis plugin verification
pub struct RedisVerificationSuite {
    harness: Arc<PluginAcceptanceHarness>,
    redis: RedisTestInstance,
}

impl BackendVerificationSuite for RedisVerificationSuite {
    async fn test_basic_operations(&self) -> Vec<TestResult> {
        vec![
            self.test_get_set().await,
            self.test_delete().await,
            self.test_exists().await,
            self.test_expire().await,
        ]
    }

    async fn test_backend_specific_features(&self) -> Vec<TestResult> {
        vec![
            self.test_pub_sub().await,
            self.test_lua_scripts().await,
            self.test_pipelining().await,
            self.test_transactions().await,
        ]
    }
}
```

### Test Backend Lifecycle Management

**Goal**: Automatically spin up/tear down real backend instances for acceptance tests.

```rust
// tests/acceptance/test_backends.rs

/// Trait for managing test backend instances
#[async_trait]
pub trait TestBackend: Send + Sync {
    async fn start(&mut self) -> Result<()>;
    async fn stop(&mut self) -> Result<()>;
    async fn reset(&mut self) -> Result<()>;
    fn connection_config(&self) -> ConnectionConfig;
    fn credentials(&self) -> Credentials;
}

/// Spawn test backend using Docker/Testcontainers
pub async fn spawn_test_backend(backend_type: BackendType) -> Result<Box<dyn TestBackend>> {
    match backend_type {
        BackendType::Postgres => Ok(Box::new(PostgresTestInstance::new().await?)),
        BackendType::Redis => Ok(Box::new(RedisTestInstance::new().await?)),
        BackendType::Kafka => Ok(Box::new(KafkaTestCluster::new().await?)),
        BackendType::ClickHouse => Ok(Box::new(ClickHouseTestInstance::new().await?)),
    }
}

/// PostgreSQL test instance using testcontainers
pub struct PostgresTestInstance {
    container: Container<Postgres>,
    connection_pool: PgPool,
}

impl PostgresTestInstance {
    pub async fn new() -> Result<Self> {
        let container = Postgres::default()
            .with_tag("16")
            .with_env("POSTGRES_PASSWORD", "test")
            .start()
            .await?;

        let connection_string = format!(
            "postgres://postgres:test@localhost:{}",
            container.get_host_port(5432).await?
        );

        let pool = PgPool::connect(&connection_string).await?;

        // Initialize test schema
        sqlx::query(
            "CREATE TABLE IF NOT EXISTS users (
                id SERIAL PRIMARY KEY,
                name TEXT NOT NULL,
                email TEXT UNIQUE
            )"
        )
        .execute(&pool)
        .await?;

        Ok(Self {
            container,
            connection_pool: pool,
        })
    }

    pub async fn query_one(&self, sql: &str) -> Result<PgRow> {
        sqlx::query(sql).fetch_one(&self.connection_pool).await
    }
}

#[async_trait]
impl TestBackend for PostgresTestInstance {
    async fn start(&mut self) -> Result<()> {
        // Container already started in new()
        Ok(())
    }

    async fn stop(&mut self) -> Result<()> {
        self.container.stop().await
    }

    async fn reset(&mut self) -> Result<()> {
        // Truncate all tables
        sqlx::query("TRUNCATE TABLE users RESTART IDENTITY CASCADE")
            .execute(&self.connection_pool)
            .await?;
        Ok(())
    }

    fn connection_config(&self) -> ConnectionConfig {
        ConnectionConfig {
            host: "localhost".to_string(),
            port: self.container.get_host_port_blocking(5432),
            database: "postgres".to_string(),
        }
    }

    fn credentials(&self) -> Credentials {
        Credentials {
            username: "postgres".to_string(),
            password: "test".to_string(),
        }
    }
}
```

### Running Acceptance Tests

**Test organization**:

```bash
tests/
├── acceptance/
│   ├── framework.rs              # Test harness
│   ├── auth_suite.rs             # Reusable auth tests
│   ├── backend_verification.rs   # Per-backend test traits
│   ├── test_backends.rs          # Docker backend management
│   │
│   ├── postgres_test.rs          # PostgreSQL acceptance tests
│   ├── redis_test.rs             # Redis acceptance tests
│   ├── kafka_test.rs             # Kafka acceptance tests
│   └── clickhouse_test.rs        # ClickHouse acceptance tests
│
└── fixtures/
    ├── test_data.sql              # Seed data for PostgreSQL
    ├── test_messages.json         # Seed data for Kafka
    └── test_keys.txt              # Seed data for Redis
```

**Running tests**:

```bash
# Run all acceptance tests
cargo test --test acceptance

# Run only PostgreSQL plugin acceptance tests
cargo test --test acceptance postgres

# Run only authentication suite across all plugins
cargo test --test acceptance auth_suite

# Run with real backend instances (requires Docker)
PRISM_TEST_MODE=integration cargo test --test acceptance

# Run against specific backend version
POSTGRES_VERSION=15 cargo test --test acceptance postgres
```

**Test output**:

running 45 tests
test acceptance::postgres::auth_suite ... ok (2.3s)
test acceptance::postgres::basic_operations ... ok (1.8s)
test acceptance::postgres::prepared_statements ... ok (3.2s)
test acceptance::redis::auth_suite ... ok (0.9s)
test acceptance::redis::pub_sub ... ok (1.2s)
test acceptance::kafka::auth_suite ... ok (4.5s)
test acceptance::kafka::consumer_groups ... ok (8.1s)

Authentication Suite Results:
  PostgreSQL: ✓ 6/6 tests passed
  Redis:      ✓ 6/6 tests passed
  Kafka:      ✓ 6/6 tests passed
  ClickHouse: ✓ 6/6 tests passed

Backend-Specific Results:
  PostgreSQL: ✓ 15/15 tests passed
  Redis:      ✓ 12/12 tests passed
  Kafka:      ✓ 18/18 tests passed
  ClickHouse: ✓ 10/10 tests passed

test result: ok. 45 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out
```text

### CI/CD Integration

**GitHub Actions workflow**:

```
# .github/workflows/plugin-acceptance.yml
name: Plugin Acceptance Tests

on: [push, pull_request]

jobs:
  acceptance:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        backend: [postgres, redis, kafka, clickhouse]
        backend_version:
          - postgres: ["14", "15", "16"]
          - redis: ["7.0", "7.2"]
          - kafka: ["3.5", "3.6"]
          - clickhouse: ["23.8", "24.1"]

    steps:
      - uses: actions/checkout@v3

      - name: Setup Rust
        uses: actions-rs/toolchain@v1
        with:
          toolchain: stable

      - name: Start Docker
        run: docker compose up -d

      - name: Run acceptance tests
        run: |
          BACKEND_TYPE=${{ matrix.backend }} \
          BACKEND_VERSION=${{ matrix.backend_version }} \
          cargo test --test acceptance ${{ matrix.backend }}

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: acceptance-test-results-${{ matrix.backend }}
          path: target/test-results/
```text

### Benefits of Acceptance Test Framework

1. **Consistency**: All plugins tested with same authentication suite
2. **Real Backends**: Tests use actual backend instances (not mocks)
3. **Confidence**: Comprehensive verification before production deployment
4. **CI/CD Ready**: Automated testing on every commit
5. **Version Matrix**: Test against multiple backend versions
6. **Reusability**: Authentication tests reused across all plugins
7. **Documentation**: Tests serve as examples for plugin developers

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

```
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
```text

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

## Plugin Development Experience

### Design Goals for Plugin Authors

Making plugins easy to create is critical for Prism's success. We prioritize:

1. **Minimal Boilerplate**: Plugin shell generates 80% of code
2. **Strong Typing**: Protobuf schemas prevent API mismatches
3. **Local Testing**: Test plugins without full Prism deployment
4. **Fast Iteration**: Hot-reload during development
5. **Clear Documentation**: Examples for common patterns

### Plugin Shell: `prism-plugin-init`

**Quick Start** - Create a new plugin in 30 seconds:

```
# Create new plugin from template
prism-plugin-init --name mongodb --language rust

# Generated structure:
mongodb-plugin/
├── Cargo.toml
├── src/
│   ├── lib.rs          # Plugin entry point (implements BackendPlugin trait)
│   ├── config.rs       # Backend configuration (auto-generated from proto)
│   ├── client.rs       # MongoDB client wrapper
│   └── operations/     # Operation handlers
│       ├── get.rs
│       ├── set.rs
│       └── query.rs
├── proto/
│   └── mongodb_config.proto  # Configuration schema
├── tests/
│   ├── integration_test.rs
│   └── fixtures/
└── README.md
```text

**Template provides**:
- gRPC service implementation skeleton
- Configuration parsing (protobuf → struct)
- Connection pool setup
- Health check implementation
- Metrics reporting
- Error handling patterns
- Integration test scaffolding

### Plugin SDK (`prism-plugin-sdk`)

Rust SDK provides helpers for common patterns:

```
use prism_plugin_sdk::prelude::*;

#[plugin]
pub struct MongoDbPlugin {
    #[config]
    config: MongoDbConfig,  // Auto-parsed from InitializeRequest

    #[client]
    client: MongoClient,  // Auto-initialized from config

    #[pool(size = 20)]
    pool: ConnectionPool,  // Connection pooling helper
}

#[async_trait]
impl BackendPlugin for MongoDbPlugin {
    // SDK provides default implementations for health_check, metrics

    #[operation("get")]
    async fn handle_get(&self, params: GetParams) -> PluginResult<GetResponse> {
        // SDK handles:
        // - Protobuf deserialization (params)
        // - Error mapping (PluginResult → gRPC status codes)
        // - Metrics recording (latency, errors)
        // - Tracing context propagation

        let doc = self.client.find_one(params.key).await?;
        Ok(GetResponse { value: doc })
    }
}
```text

**SDK Features**:
- **Macros**: `#[plugin]`, `#[operation]` reduce boilerplate
- **Connection Pooling**: Automatic pool management
- **Metrics**: Auto-record latency, errors, throughput
- **Health Checks**: Default implementation with customization hooks
- **Testing Utilities**: Mock proxy, request builders

### Development Workflow

**Local Testing Without Prism**:

```
# 1. Start plugin in standalone mode
cargo run --bin mongodb-plugin-server

# 2. Test with grpcurl
grpcurl -plaintext -d '{"namespace": "test", "config": {...}}' \
  localhost:50100 prism.plugin.BackendPlugin/Initialize

# 3. Send operations
grpcurl -plaintext -d '{"operation": "get", "params": {"key": "foo"}}' \
  localhost:50100 prism.plugin.BackendPlugin/Execute
```text

**Integration Testing**:

```
#[tokio::test]
async fn test_get_operation() {
    // SDK provides test harness
    let mut plugin = MongoDbPlugin::test_instance().await;

    // Initialize with test config
    plugin.initialize(test_config()).await.unwrap();

    // Execute operation
    let response = plugin.get("test-key").await.unwrap();

    assert_eq!(response.value, expected_value);
}
```text

**Hot-Reload During Development**:

```
# Watch for changes and reload plugin
cargo watch -x 'build --release' -s 'prism plugin reload mongodb'
```text

### Language Support Strategy

**Priority 1: Rust** (Best Performance + Isolation)
- In-process: Shared library (cdylib)
- Out-of-process: Standalone binary with gRPC server
- **Best for**: High-performance backends (Redis, PostgreSQL)

**Priority 2: Go** (Good Performance + Ecosystem)
- Out-of-process only: gRPC server
- **Best for**: Kafka, ClickHouse (existing Go SDKs)

**Priority 3: Python** (Rapid Development)
- Out-of-process only: gRPC server
- **Best for**: Prototyping, ML backends, custom integrations

**Template Availability**:
```
prism-plugin-init --name mybackend --language [rust|go|python]
```text

Each template includes:
- gRPC service implementation
- Connection pool patterns
- Configuration management
- Testing framework
- Dockerfile for containerization

### Plugin Isolation Strategy

**Security Isolation** (Prevents malicious plugins):

| Mechanism | In-Process | Sidecar | Remote |
|-----------|-----------|---------|--------|
| **Process Boundary** | ❌ Shared | ✅ Separate | ✅ Separate |
| **Memory Isolation** | ❌ Shared | ✅ Isolated | ✅ Isolated |
| **Resource Limits** | ❌ Shared | ✅ cgroups | ✅ K8s limits |
| **Credential Access** | ⚠️ Restricted | ✅ Isolated | ✅ Isolated |

**Recommendation**: Use sidecar/remote for untrusted plugins.

**Performance Isolation** (Prevents noisy neighbor):

```
# Kubernetes resource limits per plugin
apiVersion: v1
kind: Pod
metadata:
  name: clickhouse-plugin
spec:
  containers:
  - name: plugin
    image: prism/clickhouse-plugin
    resources:
      requests:
        cpu: "1"
        memory: "2Gi"
      limits:
        cpu: "4"        # Prevent CPU starvation of other plugins
        memory: "8Gi"   # Prevent OOM affecting proxy
```text

**Network Isolation** (Prevents cross-plugin communication):

```
# NetworkPolicy: Plugin can only talk to proxy and backend
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: clickhouse-plugin-netpol
spec:
  podSelector:
    matchLabels:
      app: clickhouse-plugin
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: prism-proxy  # Only proxy can call plugin
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: clickhouse  # Plugin can only call ClickHouse
```text

### Plugin Administration

Plugin management commands are provided via the Prism Admin CLI. See [RFC-006 Plugin Administration](#) for full CLI specification.

**Quick examples**:

```
# List installed plugins
prism plugin list

# Install plugin from registry
prism plugin install mongodb --version 1.2.0

# Update plugin
prism plugin update mongodb --version 1.3.0

# Enable/disable plugin
prism plugin disable kafka
prism plugin enable kafka

# View plugin health and metrics
prism plugin status mongodb

# Hot-reload plugin code
prism plugin reload mongodb
```text

For detailed plugin admin commands, see [RFC-006 Section: Plugin Management](./RFC-006-python-admin-cli.md#plugin-management).

## Appendix: Plugin Development Guide

### Creating a New Plugin

1. **Implement BackendPlugin Trait**:

```
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
```text

2. **Build as Shared Library**:

```
[lib]
crate-type = ["cdylib"]  # Dynamic library

[dependencies]
prism-plugin-sdk = "0.1"
```text

3. **Register Plugin**:

```
# Add to proxy configuration
plugins:
  - name: my-backend
    library: /path/to/libmy_backend_plugin.so
    type: in_process
```text

---

**Status**: Draft
**Next Steps**:
1. Define BackendPlugin gRPC service in protobuf
2. Implement plugin trait in Rust
3. Convert PostgreSQL backend to plugin architecture
4. Document plugin development process
5. Implement sidecar plugin support with Unix sockets

```