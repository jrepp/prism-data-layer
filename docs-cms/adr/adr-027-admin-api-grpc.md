---
id: adr-027
title: "ADR-027: Admin API via gRPC"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['admin', 'grpc', 'api-design', 'operations']
---

## Context

Prism requires administrative capabilities for:
- Managing named client configurations
- Monitoring active sessions
- Viewing backend health and metrics
- Managing namespaces and permissions
- Operational tasks (drain, maintenance mode, etc.)

**Requirements:**
- Separate from data plane (different authorization)
- gRPC for consistency with data layer
- Strong typing and versioning
- Audit logging for all admin operations
- RBAC for admin operations

## Decision

Implement **AdminService via gRPC** as separate service from data plane:

1. **Separate gRPC service**: `prism.admin.v1.AdminService`
2. **Admin-only port**: Run on separate port (8981) from data plane (8980)
3. **Enhanced auth**: Require admin credentials (separate from user sessions)
4. **Comprehensive audit**: Log all admin operations with actor identity
5. **Versioned API**: Follow same versioning strategy as data plane

## Rationale

### Why Separate Admin Service

**Security isolation:**
- Different port prevents accidental data plane access
- Separate authentication/authorization
- Can be firewalled differently (internal-only)

**Operational clarity:**
- Clear separation of concerns
- Different SLAs (admin can be slower)
- Independent scaling

**Evolution independence:**
- Admin API evolves separately from data API
- Breaking changes don't affect data plane

### Admin Service Definition

```protobuf
syntax = "proto3";

package prism.admin.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";
import "prism/config/v1/client_config.proto";

service AdminService {
  // Configuration Management
  rpc ListConfigs(ListConfigsRequest) returns (ListConfigsResponse);
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
  rpc CreateConfig(CreateConfigRequest) returns (CreateConfigResponse);
  rpc UpdateConfig(UpdateConfigRequest) returns (UpdateConfigResponse);
  rpc DeleteConfig(DeleteConfigRequest) returns (DeleteConfigResponse);

  // Session Management
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc GetSession(GetSessionRequest) returns (GetSessionResponse);
  rpc TerminateSession(TerminateSessionRequest) returns (TerminateSessionResponse);

  // Namespace Management
  rpc ListNamespaces(ListNamespacesRequest) returns (ListNamespacesResponse);
  rpc CreateNamespace(CreateNamespaceRequest) returns (CreateNamespaceResponse);
  rpc UpdateNamespace(UpdateNamespaceRequest) returns (UpdateNamespaceResponse);
  rpc DeleteNamespace(DeleteNamespaceRequest) returns (DeleteNamespaceResponse);

  // Backend Health
  rpc GetBackendStatus(GetBackendStatusRequest) returns (GetBackendStatusResponse);
  rpc ListBackends(ListBackendsRequest) returns (ListBackendsResponse);

  // Operational
  rpc SetMaintenanceMode(SetMaintenanceModeRequest) returns (SetMaintenanceModeResponse);
  rpc DrainConnections(DrainConnectionsRequest) returns (DrainConnectionsResponse);
  rpc GetMetrics(GetMetricsRequest) returns (GetMetricsResponse);

  // Audit
  rpc GetAuditLog(GetAuditLogRequest) returns (stream AuditLogEntry);
}
```

### Configuration Management

```protobuf
message ListConfigsRequest {
  optional string namespace = 1;
  optional prism.config.v1.AccessPattern pattern = 2;
  int32 page_size = 3;
  optional string page_token = 4;
}

message ListConfigsResponse {
  repeated prism.config.v1.ClientConfig configs = 1;
  optional string next_page_token = 2;
  int32 total_count = 3;
}

message CreateConfigRequest {
  prism.config.v1.ClientConfig config = 1;
  bool overwrite = 2;
}

message CreateConfigResponse {
  prism.config.v1.ClientConfig config = 1;
  google.protobuf.Timestamp created_at = 2;
}

message UpdateConfigRequest {
  string name = 1;
  prism.config.v1.ClientConfig config = 2;
}

message UpdateConfigResponse {
  prism.config.v1.ClientConfig config = 1;
  google.protobuf.Timestamp updated_at = 2;
}

message DeleteConfigRequest {
  string name = 1;
}

message DeleteConfigResponse {
  bool success = 1;
}
```

### Session Management

```protobuf
message ListSessionsRequest {
  optional string namespace = 1;
  optional SessionStatus status = 2;
  int32 page_size = 3;
  optional string page_token = 4;
}

message ListSessionsResponse {
  repeated SessionInfo sessions = 1;
  optional string next_page_token = 2;
  int32 total_count = 3;
}

message SessionInfo {
  string session_id = 1;
  string session_token = 2;
  string client_id = 3;
  string namespace = 4;
  SessionStatus status = 5;
  google.protobuf.Timestamp created_at = 6;
  google.protobuf.Timestamp expires_at = 7;
  google.protobuf.Timestamp last_activity = 8;
  SessionMetrics metrics = 9;
}

enum SessionStatus {
  SESSION_STATUS_UNSPECIFIED = 0;
  SESSION_STATUS_ACTIVE = 1;
  SESSION_STATUS_IDLE = 2;
  SESSION_STATUS_EXPIRING = 3;
  SESSION_STATUS_TERMINATED = 4;
}

message SessionMetrics {
  int64 requests_total = 1;
  int64 bytes_sent = 2;
  int64 bytes_received = 3;
  int32 active_streams = 4;
  google.protobuf.Timestamp last_request = 5;
}

message TerminateSessionRequest {
  string session_id = 1;
  bool force = 2;
  string reason = 3;
}

message TerminateSessionResponse {
  bool success = 1;
  int32 pending_operations = 2;
}
```

### Namespace Management

```protobuf
message ListNamespacesRequest {
  int32 page_size = 1;
  optional string page_token = 2;
}

message ListNamespacesResponse {
  repeated NamespaceInfo namespaces = 1;
  optional string next_page_token = 2;
}

message NamespaceInfo {
  string name = 1;
  string description = 2;
  google.protobuf.Timestamp created_at = 3;
  NamespaceStatus status = 4;
  NamespaceQuota quota = 5;
  NamespaceMetrics metrics = 6;
}

enum NamespaceStatus {
  NAMESPACE_STATUS_UNSPECIFIED = 0;
  NAMESPACE_STATUS_ACTIVE = 1;
  NAMESPACE_STATUS_READ_ONLY = 2;
  NAMESPACE_STATUS_SUSPENDED = 3;
}

message NamespaceQuota {
  int64 max_sessions = 1;
  int64 max_storage_bytes = 2;
  int64 max_rps = 3;
}

message NamespaceMetrics {
  int64 active_sessions = 1;
  int64 storage_bytes_used = 2;
  int64 requests_per_second = 3;
}

message CreateNamespaceRequest {
  string name = 1;
  string description = 2;
  optional NamespaceQuota quota = 3;
}

message CreateNamespaceResponse {
  NamespaceInfo namespace = 1;
}
```

### Backend Health

```protobuf
message GetBackendStatusRequest {
  string backend_type = 1;  // "postgres", "kafka", etc.
}

message GetBackendStatusResponse {
  string backend_type = 1;
  BackendHealth health = 2;
  repeated BackendInstance instances = 3;
}

message BackendHealth {
  HealthStatus status = 1;
  string message = 2;
  google.protobuf.Timestamp last_check = 3;

  enum HealthStatus {
    HEALTH_STATUS_UNSPECIFIED = 0;
    HEALTH_STATUS_HEALTHY = 1;
    HEALTH_STATUS_DEGRADED = 2;
    HEALTH_STATUS_UNHEALTHY = 3;
  }
}

message BackendInstance {
  string id = 1;
  string endpoint = 2;
  BackendHealth health = 3;
  BackendMetrics metrics = 4;
}

message BackendMetrics {
  int32 active_connections = 1;
  int32 pool_size = 2;
  int32 idle_connections = 3;
  double cpu_percent = 4;
  double memory_percent = 5;
  int64 requests_per_second = 6;
  double avg_latency_ms = 7;
}
```

### Operational Commands

```protobuf
message SetMaintenanceModeRequest {
  bool enabled = 1;
  optional string message = 2;
  optional google.protobuf.Timestamp scheduled_end = 3;
}

message SetMaintenanceModeResponse {
  bool success = 1;
  MaintenanceStatus status = 2;
}

message MaintenanceStatus {
  bool enabled = 1;
  optional string message = 2;
  optional google.protobuf.Timestamp started_at = 3;
  optional google.protobuf.Timestamp ends_at = 4;
  int32 active_sessions = 5;
}

message DrainConnectionsRequest {
  optional string namespace = 1;
  optional google.protobuf.Duration timeout = 2;
}

message DrainConnectionsResponse {
  int32 drained_count = 1;
  int32 remaining_count = 2;
  bool complete = 3;
}
```

### Audit Logging

```protobuf
message GetAuditLogRequest {
  optional string namespace = 1;
  optional string actor = 2;
  optional string operation = 3;
  optional google.protobuf.Timestamp start_time = 4;
  optional google.protobuf.Timestamp end_time = 5;
  int32 limit = 6;
}

message AuditLogEntry {
  string id = 1;
  google.protobuf.Timestamp timestamp = 2;
  string actor = 3;  // Admin user who performed action
  string operation = 4;  // "CreateConfig", "TerminateSession", etc.
  string resource = 5;  // Resource affected
  string namespace = 6;
  map<string, string> metadata = 7;
  bool success = 8;
  optional string error = 9;
}
```

### Authentication

Admin API requires separate authentication:

```protobuf
// Metadata in all admin requests
metadata {
  "x-admin-token": "admin-abc123",
  "x-admin-user": "alice@example.com"
}
```

**Authentication methods:**
- Admin API keys (long-lived, rotatable)
- OAuth2 with admin scope
- mTLS with admin certificate

### Authorization

Role-based access control:

```yaml
roles:
  admin:
    - config:*
    - session:*
    - namespace:*
    - backend:read
    - operational:*
    - audit:read

  operator:
    - config:read
    - session:read
    - session:terminate
    - backend:read
    - operational:maintenance
    - audit:read

  viewer:
    - config:read
    - session:read
    - backend:read
    - audit:read
```

### Deployment

Admin API runs on separate port:

```yaml
# docker-compose.yml
services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "8980:8980"  # Data plane
      - "8981:8981"  # Admin API
      - "9090:9090"  # Metrics
    environment:
      PRISM_DATA_PORT: 8980
      PRISM_ADMIN_PORT: 8981
```

**Firewall rules:**
- 8980: Public (data plane)
- 8981: Internal only (admin API)
- 9090: Metrics (internal/monitoring)

### Alternatives Considered

1. **REST API for admin**
   - Pros: Simpler, HTTP-friendly, easier debugging
   - Cons: Inconsistent with data plane, no streaming, manual typing
   - Rejected: Want consistency with gRPC data layer

2. **Combined admin/data service**
   - Pros: Simpler deployment, single port
   - Cons: Security risk, hard to separate, version skew
   - Rejected: Security isolation critical

3. **Admin commands in data plane**
   - Pros: No separate service
   - Cons: Auth complexity, unclear boundaries
   - Rejected: Violates separation of concerns

## Consequences

### Positive

- **Security**: Separate port and auth for admin operations
- **Type safety**: gRPC/protobuf for admin operations
- **Audit trail**: All admin actions logged
- **Consistency**: Same patterns as data plane
- **Evolution**: Admin API versions independently

### Negative

- **Complexity**: Another service to manage
- **Port management**: Two ports to configure/firewall
- **Client tooling**: Need admin client libraries

### Neutral

- **Learning curve**: Admins must use gRPC tools
- **Firewall rules**: Must configure internal-only access

## Implementation Notes

### Server Setup

```rust
// proxy/src/main.rs
#[tokio::main]
async fn main() -> Result<()> {
    // Data plane
    let data_addr = "0.0.0.0:8980".parse()?;
    let data_server = Server::builder()
        .add_service(SessionServiceServer::new(session_svc))
        .add_service(QueueServiceServer::new(queue_svc))
        .serve(data_addr);

    // Admin plane
    let admin_addr = "0.0.0.0:8981".parse()?;
    let admin_server = Server::builder()
        .add_service(AdminServiceServer::new(admin_svc))
        .serve(admin_addr);

    // Run both servers
    tokio::try_join!(data_server, admin_server)?;

    Ok(())
}
```

### Admin Client

```go
// tools/cmd/prism-admin/main.go
conn, err := grpc.Dial(
    "localhost:8981",
    grpc.WithTransportCredentials(creds),
)

client := admin.NewAdminServiceClient(conn)

// List sessions
resp, err := client.ListSessions(ctx, &admin.ListSessionsRequest{
    Namespace: "production",
    Status: admin.SessionStatus_SESSION_STATUS_ACTIVE,
})

for _, session := range resp.Sessions {
    fmt.Printf("Session: %s Client: %s\n", session.SessionId, session.ClientId)
}
```

### Audit Logging

```rust
impl AdminService {
    async fn create_config(&self, req: CreateConfigRequest) -> Result<CreateConfigResponse> {
        let actor = self.get_admin_user_from_metadata()?;

        // Perform operation
        let result = self.config_store.create(req.config).await;

        // Audit log
        self.audit_logger.log(AuditLogEntry {
            actor: actor.email,
            operation: "CreateConfig".to_string(),
            resource: format!("config:{}", req.config.name),
            namespace: req.config.namespace,
            success: result.is_ok(),
            error: result.as_ref().err().map(|e| e.to_string()),
            ..Default::default()
        }).await;

        result
    }
}
```

## References

- ADR-023: gRPC-First Interface Design
- ADR-024: Layered Interface Hierarchy
- RFC-002: Data Layer Interface Specification
- [gRPC Authentication](https://grpc.io/docs/guides/auth/)

## Revision History

- 2025-10-07: Initial draft and acceptance
