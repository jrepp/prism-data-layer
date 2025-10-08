# ADR-022: Dynamic Client Configuration System

**Status**: Accepted

**Date**: 2025-10-07

**Deciders**: Core Team

**Tags**: architecture, configuration, client-server, protobuf

## Context

Prism needs a flexible configuration system that:
- Separates client configuration from server infrastructure configuration
- Allows clients to specify their data access patterns at runtime
- Supports server-side configuration templates for common patterns
- Enables configuration discovery and reuse
- Follows Netflix Data Gateway patterns while improving on them

**Key Requirements:**
- **Server config**: Backend databases, queues, infrastructure (static, admin-controlled)
- **Client config**: Data access patterns, backend selection, consistency requirements (dynamic, client-controlled)
- **Configuration portability**: Clients can bring their config or use server-provided templates
- **Versioning**: Configuration evolves without breaking existing clients

## Decision

Implement **Dynamic Client Configuration** with protobuf descriptors:

1. **Separation**: Server manages infrastructure, clients manage access patterns
2. **Protobuf descriptors**: Client configuration expressed as protobuf messages
3. **Named configurations**: Server stores reusable configuration templates
4. **Runtime discovery**: Clients can query available configurations
5. **Override capability**: Clients can provide custom configurations inline

## Rationale

### Configuration Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Prism Server                         │
│                                                           │
│  ┌────────────────────┐        ┌───────────────────┐   │
│  │ Server Config      │        │ Client Config     │   │
│  │ (Static/Admin)     │        │ (Dynamic/Runtime) │   │
│  │                    │        │                   │   │
│  │ - Postgres pool    │        │ - Named configs   │   │
│  │ - Kafka brokers    │        │ - Access patterns │   │
│  │ - NATS cluster     │        │ - Backend routing │   │
│  │ - Auth policies    │        │ - Consistency     │   │
│  │ - Rate limits      │        │ - Cache policy    │   │
│  └────────────────────┘        └───────────────────┘   │
│                                                           │
└─────────────────────────────────────────────────────────┘
                           │
                           │
              ┌────────────┴────────────┐
              │                         │
              │                         │
    ┌─────────▼─────────┐     ┌─────────▼─────────┐
    │   Client A        │     │   Client B        │
    │                   │     │                   │
    │ Uses named config │     │ Provides custom   │
    │ "user-profiles"   │     │ inline config     │
    └───────────────────┘     └───────────────────┘
```

### Client Configuration Descriptor (Protobuf)

```protobuf
// proto/prism/config/v1/client_config.proto
syntax = "proto3";

package prism.config.v1;

import "prism/options.proto";

// Client configuration descriptor
message ClientConfig {
  // Configuration name (for named configs)
  string name = 1;

  // Version for evolution
  string version = 2;

  // Data access pattern
  AccessPattern pattern = 3;

  // Backend selection
  BackendConfig backend = 4;

  // Consistency requirements
  ConsistencyConfig consistency = 5;

  // Caching policy
  CacheConfig cache = 6;

  // Rate limiting
  RateLimitConfig rate_limit = 7;

  // Namespace for data isolation
  string namespace = 8;
}

// Access patterns supported by Prism
enum AccessPattern {
  ACCESS_PATTERN_UNSPECIFIED = 0;
  ACCESS_PATTERN_KEY_VALUE = 1;        // Simple get/put
  ACCESS_PATTERN_QUEUE = 2;            // Kafka-style queue
  ACCESS_PATTERN_PUBSUB = 3;           // NATS-style pub/sub
  ACCESS_PATTERN_PAGED_READER = 4;     // Database pagination
  ACCESS_PATTERN_TRANSACT_WRITE = 5;   // Transactional writes
}

// Backend configuration
message BackendConfig {
  // Backend type
  BackendType type = 1;

  // Backend-specific options
  map<string, string> options = 2;

  // Connection pool settings
  PoolConfig pool = 3;
}

enum BackendType {
  BACKEND_TYPE_UNSPECIFIED = 0;
  BACKEND_TYPE_POSTGRES = 1;
  BACKEND_TYPE_SQLITE = 2;
  BACKEND_TYPE_KAFKA = 3;
  BACKEND_TYPE_NATS = 4;
  BACKEND_TYPE_NEPTUNE = 5;
}

message PoolConfig {
  int32 min_connections = 1;
  int32 max_connections = 2;
  int32 idle_timeout_seconds = 3;
}

// Consistency configuration
message ConsistencyConfig {
  ConsistencyLevel level = 1;
  int32 timeout_ms = 2;
}

enum ConsistencyLevel {
  CONSISTENCY_LEVEL_UNSPECIFIED = 0;
  CONSISTENCY_LEVEL_EVENTUAL = 1;
  CONSISTENCY_LEVEL_STRONG = 2;
  CONSISTENCY_LEVEL_BOUNDED_STALENESS = 3;
}

// Cache configuration
message CacheConfig {
  bool enabled = 1;
  int32 ttl_seconds = 2;
  int32 max_size_mb = 3;
}

// Rate limit configuration
message RateLimitConfig {
  int32 requests_per_second = 1;
  int32 burst = 2;
}
```

### Configuration Service (gRPC)

```protobuf
// proto/prism/config/v1/config_service.proto
syntax = "proto3";

package prism.config.v1;

import "prism/config/v1/client_config.proto";

// Configuration service for managing client configs
service ConfigService {
  // List available named configurations
  rpc ListConfigs(ListConfigsRequest) returns (ListConfigsResponse);

  // Get a specific named configuration
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);

  // Register a new named configuration (admin only)
  rpc RegisterConfig(RegisterConfigRequest) returns (RegisterConfigResponse);

  // Validate a configuration before use
  rpc ValidateConfig(ValidateConfigRequest) returns (ValidateConfigResponse);
}

message ListConfigsRequest {
  // Filter by access pattern
  optional AccessPattern pattern = 1;

  // Filter by namespace
  optional string namespace = 2;
}

message ListConfigsResponse {
  repeated ClientConfig configs = 1;
}

message GetConfigRequest {
  string name = 1;
  optional string version = 2;  // Empty = latest
}

message GetConfigResponse {
  ClientConfig config = 1;
}

message RegisterConfigRequest {
  ClientConfig config = 1;
  bool overwrite = 2;  // Allow updating existing
}

message RegisterConfigResponse {
  bool success = 1;
  string message = 2;
}

message ValidateConfigRequest {
  ClientConfig config = 1;
}

message ValidateConfigResponse {
  bool valid = 1;
  repeated string errors = 2;
  repeated string warnings = 3;
}
```

### Client Connection Flow

```
Client                          Prism Server
  │                                  │
  │  1. Connect with auth           │
  ├─────────────────────────────────>│
  │                                  │
  │  2. Request config "user-profiles" │
  ├─────────────────────────────────>│
  │                                  │
  │  3. Return ClientConfig         │
  │<─────────────────────────────────┤
  │  {                               │
  │    name: "user-profiles"         │
  │    pattern: KEY_VALUE            │
  │    backend: POSTGRES             │
  │    consistency: STRONG           │
  │  }                               │
  │                                  │
  │  4. Establish session with config│
  ├─────────────────────────────────>│
  │                                  │
  │  5. Session token + metadata    │
  │<─────────────────────────────────┤
  │                                  │
  │  6. Make data requests          │
  ├─────────────────────────────────>│
  │     (using session token)        │
  │                                  │
```

### Example: Named Configuration

Server stores common configurations:

```yaml
# Server-side: config/named/user-profiles.yaml
name: user-profiles
version: "1.0"
pattern: KEY_VALUE
backend:
  type: POSTGRES
  options:
    table: user_profiles
  pool:
    min_connections: 5
    max_connections: 20
consistency:
  level: STRONG
  timeout_ms: 5000
cache:
  enabled: true
  ttl_seconds: 300
rate_limit:
  requests_per_second: 1000
  burst: 2000
namespace: production
```

Client retrieves and uses:

```go
// Client code
client := prism.NewClient(endpoint)

// Option 1: Use named config
config, err := client.GetConfig("user-profiles")
session, err := client.StartSession(config)

// Option 2: Provide inline config
config := &prism.ClientConfig{
    Pattern: prism.AccessPattern_KEY_VALUE,
    Backend: &prism.BackendConfig{
        Type: prism.BackendType_POSTGRES,
    },
    Consistency: &prism.ConsistencyConfig{
        Level: prism.ConsistencyLevel_STRONG,
    },
}
session, err := client.StartSession(config)
```

### Server Configuration (Static)

Remains infrastructure-focused:

```yaml
# Server config (admin-controlled)
server:
  host: 0.0.0.0
  port: 8980

backends:
  postgres:
    - name: primary
      connection_string: postgres://...
      max_connections: 100
    - name: replica
      connection_string: postgres://...
      max_connections: 50

  kafka:
    brokers:
      - localhost:9092
      - localhost:9093

  nats:
    urls:
      - nats://localhost:4222

auth:
  mtls:
    enabled: true
    ca_cert: /path/to/ca.pem

observability:
  tracing:
    exporter: jaeger
    endpoint: localhost:14268
  metrics:
    exporter: prometheus
    port: 9090
```

### Alternatives Considered

1. **Static client configuration files**
   - Pros: Simple, familiar pattern
   - Cons: No runtime discovery, hard to evolve, deployment coupling
   - Rejected: Doesn't support dynamic use cases

2. **REST-based configuration API**
   - Pros: Simple HTTP, easy debugging
   - Cons: No type safety, manual serialization, version skew
   - Rejected: Protobuf provides better type safety and evolution

3. **Environment variables for client config**
   - Pros: 12-factor compliant
   - Cons: Limited structure, hard to compose, no discovery
   - Rejected: Too limited for complex configurations

4. **Configuration in application code**
   - Pros: Type-safe, compile-time validation
   - Cons: Requires deployment to change, no runtime flexibility
   - Rejected: Conflicts with dynamic configuration goal

## Consequences

### Positive

- **Clean separation**: Server infrastructure vs. client access patterns
- **Runtime flexibility**: Clients can adapt configuration without redeployment
- **Discovery**: Clients can browse available configurations
- **Reusability**: Named configs shared across clients
- **Evolution**: Protobuf versioning supports backward compatibility
- **Type safety**: Protobuf ensures correct configuration structure
- **Netflix-inspired**: Follows proven patterns from Data Gateway

### Negative

- **Additional complexity**: Two configuration systems to manage
- **Discovery overhead**: Clients make extra RPC to fetch config
- **Storage required**: Server must persist named configurations
- **Validation needed**: Server must validate client-provided configs

### Neutral

- **Learning curve**: Teams must understand dual configuration model
- **Migration path**: Existing systems need gradual migration

## Implementation Notes

### Configuration Storage

Server stores named configurations:

```
config/
├── named/
│   ├── user-profiles.yaml
│   ├── session-cache.yaml
│   ├── event-queue.yaml
│   └── analytics-stream.yaml
└── templates/
    ├── key-value.yaml
    ├── queue.yaml
    └── pubsub.yaml
```

### Configuration Validation

Server validates all configurations:

```rust
impl ConfigValidator {
    fn validate(&self, config: &ClientConfig) -> Result<(), Vec<ValidationError>> {
        let mut errors = Vec::new();

        // Check backend compatibility with pattern
        if config.pattern == AccessPattern::Queue
           && config.backend.type != BackendType::Kafka {
            errors.push(ValidationError::IncompatibleBackend);
        }

        // Check namespace exists
        if !self.namespace_exists(&config.namespace) {
            errors.push(ValidationError::UnknownNamespace);
        }

        // Check rate limits are reasonable
        if config.rate_limit.requests_per_second > MAX_RPS {
            errors.push(ValidationError::RateLimitTooHigh);
        }

        if errors.is_empty() { Ok(()) } else { Err(errors) }
    }
}
```

### Configuration Caching

Client caches configurations locally:

```go
type ConfigCache struct {
    cache map[string]*ClientConfig
    ttl   time.Duration
}

func (c *ConfigCache) Get(name string) (*ClientConfig, error) {
    if config, ok := c.cache[name]; ok {
        return config, nil
    }

    // Fetch from server
    config, err := c.client.GetConfig(name)
    if err != nil {
        return nil, err
    }

    c.cache[name] = config
    return config, nil
}
```

## References

- [Netflix Data Gateway Architecture](https://netflixtechblog.com/data-gateway-a-platform-for-growing-and-protecting-the-data-tier-f1-2019-3fd1a829503)
- ADR-002: Client-Originated Configuration
- ADR-003: Protobuf as Single Source of Truth
- ADR-006: Namespace and Multi-Tenancy

## Revision History

- 2025-10-07: Initial draft and acceptance
