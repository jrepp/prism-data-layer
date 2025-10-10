---
title: "ADR-033: Capability API for Prism Instance Queries"
status: Proposed
date: 2025-10-08
deciders: System
tags: ['api-design', 'client-server', 'versioning', 'operations']
---

## Context

Client applications and admin tools need a way to discover what features, backends, and configurations are supported by a specific Prism instance before attempting to use them. Different Prism deployments may:

- Support different backend types (some may have Kafka, others may not)
- Have different versions of the data abstraction APIs
- Support different cache strategies or consistency levels
- Have different operational limits (max RPS, max connections, etc.)
- Enable/disable specific features (shadow traffic, protocol recording, etc.)

Without a capability discovery mechanism, clients must either:
1. **Hard-code assumptions** about what's available (brittle, breaks across environments)
2. **Try and fail** with requests to unsupported backends (poor UX, unnecessary errors)
3. **Rely on out-of-band configuration** (deployment-specific client configs, hard to maintain)

Netflix's Data Gateway learned this lesson: different clusters support different backends and configurations. Their solution was to provide runtime capability queries.

## Decision

**Implement a gRPC Capability API** that allows clients to query Prism instance capabilities at runtime.

The API will expose:
- **Supported Backends**: List of available backend types (postgres, redis, kafka, etc.)
- **API Version**: Protobuf schema version and supported operations
- **Feature Flags**: Enabled/disabled features (shadow traffic, caching, protocol recording)
- **Operational Limits**: Max RPS per namespace, max connections, rate limits
- **Backend-Specific Capabilities**: Per-backend features (e.g., Redis supports vector search)

### API Definition

```protobuf
syntax = "proto3";

package prism.admin;

service CapabilityService {
  // Get overall Prism instance capabilities
  rpc GetCapabilities(GetCapabilitiesRequest) returns (GetCapabilitiesResponse);

  // Get backend-specific capabilities
  rpc GetBackendCapabilities(GetBackendCapabilitiesRequest) returns (GetBackendCapabilitiesResponse);
}

message GetCapabilitiesRequest {
  // Optional: request capabilities as of a specific API version
  optional string api_version = 1;
}

message GetCapabilitiesResponse {
  // Prism instance metadata
  string instance_id = 1;
  string version = 2;  // e.g., "0.3.0"
  string api_version = 3;  // e.g., "v1alpha1"

  // Supported backends
  repeated string backends = 4;  // ["postgres", "redis", "kafka", "nats", "clickhouse"]

  // Feature flags
  map<string, bool> features = 5;
  // Example: {"shadow_traffic": true, "protocol_recording": false, "cache_strategies": true}

  // Operational limits
  OperationalLimits limits = 6;

  // Supported data access patterns
  repeated string patterns = 7;  // ["keyvalue", "stream", "timeseries", "graph"]
}

message OperationalLimits {
  int64 max_namespaces = 1;
  int64 max_rps_per_namespace = 2;
  int64 max_connections_per_namespace = 3;
  int64 max_payload_size_bytes = 4;
  int64 max_stream_duration_seconds = 5;
}

message GetBackendCapabilitiesRequest {
  string backend = 1;  // "redis", "postgres", etc.
}

message GetBackendCapabilitiesResponse {
  string backend = 1;
  bool available = 2;
  string version = 3;  // Backend client library version

  // Supported operations for this backend
  repeated string operations = 4;  // ["get", "set", "mget", "scan"]

  // Backend-specific features
  map<string, bool> features = 5;
  // Example for Redis: {"vector_search": true, "geo_queries": false, "streams": true}

  // Backend-specific limits
  map<string, int64> limits = 6;
  // Example: {"max_key_size": 512000, "max_value_size": 104857600}
}
```

### Usage Patterns

**Client Discovery on Startup**:
```rust
// Client queries capabilities on initialization
let caps = client.get_capabilities().await?;

if !caps.backends.contains(&"redis".to_string()) {
    return Err("Redis backend not available in this environment".into());
}

if !caps.features.get("shadow_traffic").unwrap_or(&false) {
    warn!("Shadow traffic not supported, skipping migration setup");
}
```

**Admin CLI Feature Detection**:
```bash
# CLI checks capabilities before presenting commands
prism backend list  # Only shows available backends

# Attempting unsupported operation fails gracefully
prism shadow enable my-ns
# Error: Shadow traffic not enabled in this Prism instance
```

**Version Compatibility Check**:
```python
# Python client checks API compatibility
caps = await client.get_capabilities()
if caps.api_version != "v1alpha1":
    raise ValueError(f"Client expects v1alpha1, server has {caps.api_version}")
```

## Rationale

### Why This Approach?

1. **Runtime Discovery**: Clients adapt to environment without redeployment
2. **Graceful Degradation**: Missing features can be handled gracefully
3. **Operational Visibility**: Admins can query what's available before creating namespaces
4. **Version Negotiation**: Clients can detect API mismatches early
5. **Multi-Tenancy Support**: Different Prism instances can have different capabilities

### Netflix's Experience

Netflix's Data Gateway exposes similar capability information:
- Which data stores are available in a cluster
- What consistency levels are supported
- Regional deployment configurations

This enabled them to:
- Run different configurations per region (US, EU, APAC)
- Gradually roll out new backends without breaking clients
- Provide clear error messages when unsupported features are used

### Alternatives Considered

1. **Static Configuration Files**
   - Pros: Simple, no API needed
   - Cons: Out-of-band, must be distributed separately, version skew issues
   - Rejected because: Doesn't scale across environments, prone to drift

2. **Try-and-Fail Discovery**
   - Pros: No extra API needed
   - Cons: Poor UX, generates unnecessary errors, harder to debug
   - Rejected because: Creates operational noise, confusing error messages

3. **OpenAPI/Swagger-Style Schema Introspection**
   - Pros: Standard approach for REST APIs
   - Cons: gRPC already has reflection, but it's schema-level not capability-level
   - Rejected because: Need runtime instance state, not just schema definition

## Consequences

### Positive

- Clients can adapt to environment capabilities at runtime
- Better error messages ("feature not available" vs "unknown error")
- Enables gradual rollout of new features across environments
- Admin tools can show only relevant commands
- Easier to maintain multi-region deployments with different capabilities

### Negative

- Additional API surface to maintain
- Capability response must stay backward compatible
- Clients need to handle varying capabilities (more complex logic)

### Neutral

- Feature flags become first-class API concept (good governance needed)
- Need to document which capabilities are stable vs experimental
- Capability API itself needs versioning strategy

## Implementation Notes

### Caching Capabilities

Clients should cache capability responses:

```rust
pub struct PrismClient {
    capabilities: Arc<RwLock<Option<Capabilities>>>,
    capabilities_ttl: Duration,
}

impl PrismClient {
    pub async fn get_capabilities(&self) -> Result<Capabilities> {
        // Check cache first
        if let Some(caps) = self.capabilities.read().await.as_ref() {
            if caps.fetched_at.elapsed() < self.capabilities_ttl {
                return Ok(caps.clone());
            }
        }

        // Fetch from server
        let caps = self.stub.get_capabilities(GetCapabilitiesRequest {}).await?;

        // Update cache
        *self.capabilities.write().await = Some(caps.clone());

        Ok(caps)
    }
}
```

**Cache TTL**: 5 minutes (capabilities rarely change at runtime)

### Feature Flag Management

Feature flags should be documented:

```yaml
# config/features.yaml
features:
  shadow_traffic:
    enabled: true
    description: "Enable shadow traffic for zero-downtime migrations"
    stability: stable
    since: "0.2.0"

  protocol_recording:
    enabled: false
    description: "Record request/response for debugging (PII concerns)"
    stability: experimental
    since: "0.3.0"

  cache_strategies:
    enabled: true
    description: "Support RFC-007 cache strategies"
    stability: stable
    since: "0.2.0"
```

### Backend Capability Discovery

Backend plugins report their capabilities during initialization:

```rust
impl BackendPlugin for RedisPlugin {
    async fn initialize(&mut self, req: InitializeRequest) -> Result<InitializeResponse> {
        // ... initialization ...

        Ok(InitializeResponse {
            success: true,
            plugin_version: "0.1.0".to_string(),
            supported_operations: vec!["get", "set", "mget", "scan"],
            features: hashmap! {
                "vector_search" => true,
                "geo_queries" => false,
                "streams" => true,
            },
            limits: hashmap! {
                "max_key_size" => 512_000,
                "max_value_size" => 100 * 1024 * 1024,  // 100MB
            },
            ..Default::default()
        })
    }
}
```

Proxy aggregates plugin capabilities and exposes via Capability API.

## References

- RFC-003: Admin gRPC API (where Capability API lives)
- RFC-008: Proxy Plugin Architecture (backend capability reporting)
- [Netflix Data Gateway Multi-Region](/prism-data-layer/netflix/scale)
- [gRPC Server Reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md)
- [Kubernetes API Discovery](https://kubernetes.io/docs/reference/using-api/api-concepts/#discovery)

## Revision History

- 2025-10-08: Initial draft proposing capability API based on Netflix lessons
