---
author: Platform Team
created: 2025-10-08
doc_uuid: c6dbb179-7bd3-4d60-9262-8ca117270771
id: rfc-003
project_id: prism-data-layer
status: Proposed
title: Admin Interface for Prism
updated: 2025-10-08
---

## Abstract

This RFC specifies the administrative interface for Prism, enabling operators to manage configurations, monitor sessions, view backend health, and perform operational tasks. The design separates admin concerns from the data plane while maintaining consistency with Prism's gRPC-first architecture.

## Motivation

Prism requires administrative capabilities beyond the data plane APIs. Operators need to:

1. **Manage Configuration**: Create, update, and delete client configurations
2. **Monitor Sessions**: View active sessions, metrics, and troubleshoot issues
3. **Maintain System Health**: Check backend status, drain connections, enable maintenance mode
4. **Audit Operations**: Track administrative actions for compliance and debugging
5. **Visualize System State**: Browser-accessible UI for non-CLI users

**Goals:**
- Provide complete administrative control via gRPC API
- Enable browser-based administration for broader accessibility
- Maintain security isolation from data plane
- Support audit logging for all administrative actions
- Keep deployment simple with minimal dependencies

**Non-Goals:**
- Real-time monitoring dashboards (use Prometheus/Grafana)
- Complex workflow orchestration (use external tools)
- Multi-cluster management (single cluster scope)

## Proposed Design

### Architecture Overview

┌──────────────────────────────────────────────────────────┐
│                    Admin Clients                         │
│                                                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │ CLI Tool     │  │ Web Browser  │  │ Automation   │   │
│  │ (grpcurl)    │  │ (FastAPI UI) │  │ (Go/Python)  │   │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
│         │                  │                  │            │
└─────────┼──────────────────┼──────────────────┼───────────┘
          │                  │                  │
          │ gRPC             │ HTTP/gRPC-Web    │ gRPC
          │                  │                  │
┌─────────▼──────────────────▼──────────────────▼───────────┐
│               Prism Proxy (Port 8981)                      │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ AdminService (gRPC)                                  │ │
│  │ - Config management                                  │ │
│  │ - Session monitoring                                 │ │
│  │ - Namespace operations                               │ │
│  │ - Backend health                                     │ │
│  │ - Operational commands                               │ │
│  └──────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ Audit Logger                                         │ │
│  │ - Records all admin operations                       │ │
│  │ - Actor identity tracking                            │ │
│  └──────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
          │
          │ Data access
          │
┌─────────▼───────────────────────────────────────────────────┐
│                   Backend Storage                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │ Postgres │  │ Kafka    │  │ NATS     │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
└─────────────────────────────────────────────────────────────┘
```text

### Component 1: Admin API (gRPC)

**Design Choice: Separate gRPC service on dedicated port**

**Rationale:**
- **Security Isolation**: Different port prevents accidental data plane access
- **Independent Scaling**: Admin traffic patterns differ from data plane
- **Evolution**: Admin API versions independently
- **Firewall-Friendly**: Internal-only port easily restricted

**Alternative Considered: Combined service**
- *Pros*: Simpler deployment, single port
- *Cons*: Security risk, hard to separate auth, unclear boundaries
- *Decision*: Rejected due to security requirements

**API Surface:**

```
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

  // Operational Commands
  rpc SetMaintenanceMode(SetMaintenanceModeRequest) returns (SetMaintenanceModeResponse);
  rpc DrainConnections(DrainConnectionsRequest) returns (DrainConnectionsResponse);
  rpc GetMetrics(GetMetricsRequest) returns (GetMetricsResponse);

  // Audit
  rpc GetAuditLog(GetAuditLogRequest) returns (stream AuditLogEntry);
}
```text

**Implementation:**

Run on separate port (8981) alongside data plane (8980):

```
// proxy/src/main.rs
#[tokio::main]
async fn main() -> Result<()> {
    let data_addr = "0.0.0.0:8980".parse()?;
    let admin_addr = "0.0.0.0:8981".parse()?;

    // Data plane server
    let data_server = Server::builder()
        .add_service(SessionServiceServer::new(session_svc))
        .add_service(QueueServiceServer::new(queue_svc))
        .serve(data_addr);

    // Admin plane server
    let admin_server = Server::builder()
        .add_service(AdminServiceServer::new(admin_svc))
        .serve(admin_addr);

    tokio::try_join!(data_server, admin_server)?;
    Ok(())
}
```text

**Authentication:**

Admin API requires separate credentials:

```
# Metadata in all admin requests
metadata:
  x-admin-token: "admin-abc123"
  x-admin-user: "alice@example.com"
```text

**Pros:**
- Strong security boundary
- Standard gRPC authentication patterns
- Supports multiple auth methods (API keys, OAuth2, mTLS)

**Cons:**
- Requires credential management
- Different auth flow than data plane

**Decision**: Use admin API keys with rotation policy

### Component 2: Admin UI (FastAPI + gRPC-Web)

**Design Choice: FastAPI serving static files + gRPC-Web proxy**

**Rationale:**
- **Browser Compatibility**: gRPC-Web enables browser access to gRPC backend
- **Simple Deployment**: Single container with Python service
- **No Framework Overhead**: Vanilla JavaScript sufficient for admin UI
- **Fast Iteration**: No build step for frontend changes

**Alternative Considered: React/Vue SPA**
- *Pros*: Rich UI, reactive, component-based
- *Cons*: Build complexity, large bundle size, overkill for admin UI
- *Decision*: Rejected in favor of simplicity

**Alternative Considered: Envoy gRPC-Web proxy**
- *Pros*: Production-grade, feature-rich
- *Cons*: Additional process, more complex configuration
- *Decision*: Deferred; FastAPI sufficient initially, can migrate if needed

**Architecture:**

Browser → HTTP/gRPC-Web → FastAPI → gRPC → Prism Admin API
```

**Implementation:**

```python
# admin-ui/main.py
from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse

app = FastAPI(title="Prism Admin UI")

# Serve static files
app.mount("/static", StaticFiles(directory="static"), name="static")

@app.get("/")
async def read_root():
    return FileResponse("static/index.html")

@app.post("/prism.admin.v1.AdminService/{method}")
async def grpc_proxy(method: str, request: bytes):
    """Proxy gRPC-Web requests to gRPC backend"""
    channel = grpc.aio.insecure_channel("prism-proxy:8981")
    # Forward request and convert response
    # ... implementation details ...
```

**Frontend Structure:**

admin-ui/static/
├── index.html          # Main page
├── css/
│   └── styles.css      # Tailwind or minimal CSS
├── js/
│   ├── admin_grpc_web_pb.js   # Generated gRPC-Web client
│   ├── config.js       # Config management
│   ├── sessions.js     # Session monitoring
│   └── health.js       # Health dashboard
└── lib/
    └── grpc-web.js     # gRPC-Web runtime
```text

**JavaScript Client:**

```
// admin-ui/static/js/config.js
import {AdminServiceClient} from './admin_grpc_web_pb.js';

const client = new AdminServiceClient('http://localhost:8000');

async function loadConfigs() {
    const request = new ListConfigsRequest();

    client.listConfigs(request, {'x-admin-token': getAdminToken()}, (err, response) => {
        if (err) {
            console.error('Error:', err);
            return;
        }
        renderConfigs(response.getConfigsList());
    });
}
```text

**Pros:**
- No build step required
- Fast development iteration
- Small deployment footprint
- Easy debugging (view source in browser)

**Cons:**
- Manual DOM manipulation
- No reactive framework
- Limited UI component library

**Decision**: Use vanilla JavaScript initially, migrate to framework if UI complexity grows

### Component 3: Audit Logging

**Design Choice: Structured audit log with queryable storage**

**Rationale:**
- **Compliance**: Track all administrative actions
- **Debugging**: Reconstruct sequence of operations
- **Security**: Detect unauthorized access attempts

**Implementation:**

```
impl AdminService {
    async fn create_config(&self, req: CreateConfigRequest) -> Result<CreateConfigResponse> {
        let actor = self.get_admin_user_from_metadata()?;

        // Perform operation
        let result = self.config_store.create(req.config).await;

        // Audit log
        self.audit_logger.log(AuditLogEntry {
            timestamp: Utc::now(),
            actor: actor.email,
            operation: "CreateConfig".to_string(),
            resource: format!("config:{}", req.config.name),
            namespace: req.config.namespace,
            success: result.is_ok(),
            error: result.as_ref().err().map(|e| e.to_string()),
            metadata: extract_metadata(&req),
        }).await;

        result
    }
}
```text

**Storage:**

```
CREATE TABLE audit_log (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,
    actor TEXT NOT NULL,
    operation TEXT NOT NULL,
    resource TEXT NOT NULL,
    namespace TEXT,
    success BOOLEAN NOT NULL,
    error TEXT,
    metadata JSONB,

    INDEX idx_audit_timestamp ON audit_log(timestamp DESC),
    INDEX idx_audit_actor ON audit_log(actor),
    INDEX idx_audit_operation ON audit_log(operation)
);
```text

**Pros:**
- Comprehensive audit trail
- Queryable via SQL
- Supports compliance requirements

**Cons:**
- Storage overhead
- Performance impact (async mitigates)

**Decision**: Always log admin operations; use async writes to minimize latency

## Deployment

**Docker Compose:**

```
services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "8980:8980"  # Data plane
      - "8981:8981"  # Admin API
    environment:
      PRISM_DATA_PORT: 8980
      PRISM_ADMIN_PORT: 8981

  admin-ui:
    image: prism/admin-ui:latest
    ports:
      - "8000:8000"
    environment:
      PRISM_ADMIN_ENDPOINT: prism-proxy:8981
      ADMIN_TOKEN_SECRET: ${ADMIN_TOKEN_SECRET}
    depends_on:
      - prism-proxy
```text

**Network Policy:**

```
# Kubernetes NetworkPolicy example
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: prism-admin-policy
spec:
  podSelector:
    matchLabels:
      app: prism-proxy
  ingress:
  - from:
    - podSelector:
        matchLabels:
          role: admin
    ports:
    - protocol: TCP
      port: 8981  # Admin API - internal only
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8980  # Data plane - all pods
```text

## Security Considerations

### Authentication

**Admin API Keys:**
- Long-lived, rotatable tokens
- Scoped to specific operations (RBAC)
- Stored in secret management system

```
#[derive(Debug)]
pub struct AdminApiKey {
    pub key_id: String,
    pub key_hash: String,
    pub created_at: DateTime<Utc>,
    pub expires_at: Option<DateTime<Utc>>,
    pub permissions: Vec<Permission>,
}

pub enum Permission {
    ConfigRead,
    ConfigWrite,
    SessionRead,
    SessionTerminate,
    NamespaceAdmin,
    OperationalAdmin,
}
```text

**Alternative: OAuth2**
- *Pros*: Standard protocol, integrates with IdP
- *Cons*: More complex, external dependency
- *Decision*: Support both; OAuth2 for enterprise deployments

### Authorization

**Role-Based Access Control (RBAC):**

```
roles:
  admin:
    - config:*
    - session:*
    - namespace:*
    - operational:*

  operator:
    - config:read
    - session:read
    - session:terminate
    - operational:maintenance

  viewer:
    - config:read
    - session:read
    - backend:read
```text

### Network Isolation

**Production Setup:**
- Admin API on internal network only
- Admin UI behind VPN or internal load balancer
- mTLS for admin API connections

```
# Example firewall rules
- port: 8980
  protocol: TCP
  allow: [public]

- port: 8981
  protocol: TCP
  allow: [internal, 10.0.0.0/8]
```text

## Performance Considerations

### Caching

Admin UI caches configuration list:

```
// Cache configs for 30 seconds
const configCache = new Map();
const CACHE_TTL = 30000;

async function getConfigs() {
    const now = Date.now();
    const cached = configCache.get('list');

    if (cached && (now - cached.timestamp) < CACHE_TTL) {
        return cached.data;
    }

    const configs = await fetchConfigs();
    configCache.set('list', { data: configs, timestamp: now });
    return configs;
}
```text

### Pagination

All list operations support pagination:

```
message ListSessionsRequest {
  int32 page_size = 1;
  optional string page_token = 2;
}

message ListSessionsResponse {
  repeated SessionInfo sessions = 1;
  optional string next_page_token = 2;
  int32 total_count = 3;
}
```text

### Async Operations

Long-running operations return operation handle:

```
message DrainConnectionsRequest {
  optional string namespace = 1;
  optional google.protobuf.Duration timeout = 2;
}

message DrainConnectionsResponse {
  string operation_id = 1;  // Track progress
  int32 initial_count = 2;
}

// Poll for status
message GetOperationRequest {
  string operation_id = 1;
}

message GetOperationResponse {
  bool complete = 1;
  int32 progress_percent = 2;
  optional string error = 3;
}
```text

## Migration and Rollout

### Phase 1: Admin API (Week 1-2)
- Implement AdminService in Rust
- Add authentication/authorization
- Deploy on port 8981
- CLI tooling for early adopters

### Phase 2: Audit Logging (Week 2-3)
- Implement audit logger
- Database schema for audit log
- Query interface via GetAuditLog RPC

### Phase 3: Admin UI (Week 3-4)
- FastAPI service setup
- gRPC-Web proxy implementation
- Basic UI (config management only)
- Deploy to staging

### Phase 4: Full UI Features (Week 4-6)
- Session monitoring page
- Backend health dashboard
- Namespace management
- Operational commands

### Phase 5: Productionization (Week 6-8)
- OAuth2 integration
- RBAC implementation
- Production deployment
- Documentation and training

## Testing Strategy

### Unit Tests
```
#[cfg(test)]
mod tests {
    #[tokio::test]
    async fn test_create_config() {
        let service = AdminService::new_test();
        let req = CreateConfigRequest { /* ... */ };
        let res = service.create_config(req).await.unwrap();
        assert!(res.success);
    }

    #[tokio::test]
    async fn test_unauthorized_access() {
        let service = AdminService::new_test();
        let req = /* request without admin token */;
        let err = service.create_config(req).await.unwrap_err();
        assert_eq!(err.code(), Code::Unauthenticated);
    }
}
```text

### Integration Tests
```
# Test admin API
grpcurl -H "x-admin-token: test-token" \
  localhost:8981 \
  prism.admin.v1.AdminService/ListConfigs

# Test audit logging
psql -c "SELECT * FROM audit_log WHERE operation = 'CreateConfig'"

# Test UI
curl http://localhost:8000
```text

### E2E Tests
```
# tests/e2e/test_admin_workflow.py
def test_full_admin_workflow():
    # Create config via UI
    ui_client.create_config(config_data)

    # Verify config exists
    config = api_client.get_config(config_name)
    assert config.name == config_name

    # Verify audit log
    audit = api_client.get_audit_log(operation="CreateConfig")
    assert len(audit) == 1
```text

## Monitoring and Observability

### Metrics

```
// Prometheus metrics for admin API
lazy_static! {
    static ref ADMIN_REQUESTS: IntCounterVec = register_int_counter_vec!(
        "prism_admin_requests_total",
        "Total admin API requests",
        &["operation", "status"]
    ).unwrap();

    static ref ADMIN_LATENCY: HistogramVec = register_histogram_vec!(
        "prism_admin_request_duration_seconds",
        "Admin API request latency",
        &["operation"]
    ).unwrap();
}
```text

### Alerts

```
# Prometheus alerting rules
groups:
- name: prism_admin
  rules:
  - alert: AdminAPIHighErrorRate
    expr: |
      rate(prism_admin_requests_total{status="error"}[5m]) > 0.1
    annotations:
      summary: "High error rate on admin API"

  - alert: UnauthorizedAdminAccess
    expr: |
      rate(prism_admin_requests_total{status="unauthenticated"}[5m]) > 0
    annotations:
      summary: "Unauthorized access attempts detected"
```text

## Open Questions

1. **OAuth2 Integration**: Which IdP to support first (Okta, Auth0, Google)?
2. **UI Framework Migration**: At what complexity threshold migrate to React/Vue?
3. **Multi-cluster Support**: How to extend admin UI for multi-cluster management?
4. **Backup/Restore**: Should admin UI include backup/restore capabilities?

## References

- ADR-027: Admin API via gRPC
- ADR-028: Admin UI with FastAPI and gRPC-Web
- ADR-029: Protocol Recording with Protobuf Tagging
- ADR-030: Schema Recording with Protobuf Tagging
- RFC-001: Prism Architecture
- RFC-002: Data Layer Interface Specification
- [gRPC-Web Specification](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md)
- [FastAPI Documentation](https://fastapi.tiangolo.com/)

## Revision History

- 2025-10-08: Initial draft

```