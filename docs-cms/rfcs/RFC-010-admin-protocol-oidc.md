---
title: "RFC-010: Admin Protocol with OIDC Authentication"
status: Proposed
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [admin, oidc, authentication, grpc, protocol]
---

## Abstract

This RFC specifies the complete Admin Protocol for Prism, including OIDC-based authentication with provable identity, request/response flows, session management, and operational procedures. The Admin API enables platform teams to manage configurations, monitor sessions, check backend health, and perform operational tasks with strong authentication and audit trails.

## Motivation

Platform teams require secure, authenticated access to Prism administration with:
- **Provable Identity**: OIDC tokens with claims from identity provider
- **Role-Based Access**: Different permission levels (admin, operator, viewer)
- **Audit Trail**: Every administrative action logged with actor identity
- **Session Management**: Long-lived sessions for interactive use, short-lived for automation
- **Network Isolation**: Admin API on separate port, internal-only access

**Goals:**
- Define complete Admin gRPC protocol
- Specify OIDC authentication flow with token acquisition
- Document request/response patterns with sequence diagrams
- Establish session lifecycle and management
- Enable audit logging for compliance

**Non-Goals:**
- Data plane authentication (covered in RFC-011)
- User-facing authentication (application responsibility)
- Multi-cluster admin (single cluster scope)

## Protocol Overview

### Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                      Admin Workflow                           │
└───────────────────────────────────────────────────────────────┘

Administrator → OIDC Provider → Admin CLI/UI → Prism Admin API → Backends
     (1)            (2)              (3)              (4)

1. Request identity token
2. Receive JWT with claims
3. Present JWT in gRPC metadata
4. Authorized operations
```

### Ports and Endpoints

- **Data Plane**: Port 8980 (gRPC, public)
- **Admin API**: Port 8981 (gRPC, internal-only)
- **Metrics**: Port 9090 (Prometheus, internal-only)

### Protocol Stack

```
┌────────────────────────────────────────────────┐
│ Admin Client (CLI/UI/Automation)               │
└────────────────┬───────────────────────────────┘
                 │
                 │ gRPC/HTTP2 + TLS
                 │ Authorization: Bearer <jwt>
                 │
┌────────────────▼───────────────────────────────┐
│ Prism Admin Service (:8981)                    │
│                                                │
│  ┌──────────────────────────────────────────┐ │
│  │ Authentication Middleware                │ │
│  │ - JWT validation                         │ │
│  │ - Claims extraction                      │ │
│  │ - RBAC policy check                      │ │
│  └──────────────────────────────────────────┘ │
│                                                │
│  ┌──────────────────────────────────────────┐ │
│  │ AdminService Implementation              │ │
│  │ - Configuration management               │ │
│  │ - Session monitoring                     │ │
│  │ - Backend health                         │ │
│  │ - Operational commands                   │ │
│  └──────────────────────────────────────────┘ │
│                                                │
│  ┌──────────────────────────────────────────┐ │
│  │ Audit Logger                             │ │
│  │ - Records actor + operation + result     │ │
│  └──────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
```

## OIDC Authentication Flow

### Token Acquisition

```mermaid
sequenceDiagram
    participant Admin as Administrator
    participant CLI as Admin CLI
    participant OIDC as OIDC Provider<br/>(Okta/Auth0/Google)
    participant API as Prism Admin API

    Note over Admin,API: Initial Authentication

    Admin->>CLI: prism-admin namespace list
    CLI->>CLI: Check token cache<br/>~/.prism/token

    alt Token missing or expired
        CLI->>OIDC: Device Code Flow:<br/>POST /oauth/device/code
        OIDC-->>CLI: device_code, user_code,<br/>verification_uri

        CLI->>Admin: Open browser to:<br/>https://idp.example.com/activate<br/>Enter code: ABCD-1234

        Admin->>OIDC: Navigate to verification_uri<br/>Enter user_code
        OIDC->>Admin: Show consent screen
        Admin->>OIDC: Approve scopes:<br/>- admin:read<br/>- admin:write

        loop Poll for token (max 5 min)
            CLI->>OIDC: POST /oauth/token<br/>{device_code, grant_type}
            alt User approved
                OIDC-->>CLI: access_token (JWT),<br/>refresh_token, expires_in
            else Still pending
                OIDC-->>CLI: {error: "authorization_pending"}
            end
        end

        CLI->>CLI: Cache token to ~/.prism/token
    end

    Note over CLI,API: Authenticated Request

    CLI->>API: gRPC: ListNamespaces()<br/>metadata:<br/>  authorization: Bearer eyJhbG...
    API->>API: Validate JWT signature<br/>Check expiry<br/>Extract claims

    alt JWT valid
        API->>API: Check RBAC:<br/>user.email has admin:read?
        API-->>CLI: ListNamespacesResponse
        API->>API: Audit log: alice@company.com<br/>listed namespaces
    else JWT invalid/expired
        API-->>CLI: Unauthenticated (16)
        CLI->>OIDC: Refresh token
    end
```

### JWT Structure

```json
{
  "header": {
    "alg": "RS256",
    "typ": "JWT",
    "kid": "key-2024-10"
  },
  "payload": {
    "iss": "https://idp.example.com",
    "sub": "user:alice@company.com",
    "aud": "prism-admin-api",
    "exp": 1696867200,
    "iat": 1696863600,
    "email": "alice@company.com",
    "email_verified": true,
    "groups": ["platform-team", "admins"],
    "scope": "admin:read admin:write admin:operational"
  },
  "signature": "..."
}
```

### Token Validation

```rust
use jsonwebtoken::{decode, DecodingKey, Validation};
use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct Claims {
    pub sub: String,
    pub email: String,
    pub email_verified: bool,
    pub groups: Vec<String>,
    pub scope: String,
    pub exp: u64,
    pub iat: u64,
}

pub struct JwtValidator {
    issuer: String,
    audience: String,
    jwks_client: JwksClient,
}

impl JwtValidator {
    pub async fn validate_token(&self, token: &str) -> Result<Claims> {
        // Decode header to get key ID
        let header = decode_header(token)?;
        let kid = header.kid.ok_or(Error::MissingKeyId)?;

        // Fetch public key from JWKS endpoint
        let jwk = self.jwks_client.get_key(&kid).await?;
        let decoding_key = DecodingKey::from_jwk(&jwk)?;

        // Validate signature and claims
        let mut validation = Validation::new(jsonwebtoken::Algorithm::RS256);
        validation.set_issuer(&[&self.issuer]);
        validation.set_audience(&[&self.audience]);
        validation.validate_exp = true;

        let token_data = decode::<Claims>(token, &decoding_key, &validation)?;

        // Additional validation
        if !token_data.claims.email_verified {
            return Err(Error::EmailNotVerified);
        }

        Ok(token_data.claims)
    }
}
```

## Request/Response Flows

### Namespace Creation Flow

```mermaid
sequenceDiagram
    participant CLI as Admin CLI
    participant Auth as Auth Middleware
    participant Service as AdminService
    participant Store as Config Store<br/>(Postgres)
    participant Audit as Audit Logger

    Note over CLI,Audit: Create Namespace Request

    CLI->>Auth: CreateNamespace()<br/>metadata: authorization: Bearer JWT<br/>body: {name: "analytics", ...}

    Auth->>Auth: Validate JWT
    Auth->>Auth: Extract claims:<br/>email: alice@company.com<br/>groups: ["platform-team"]

    Auth->>Auth: Check RBAC:<br/>Does alice have admin:write?

    alt Authorized
        Auth->>Service: Forward request with<br/>actor: alice@company.com

        Service->>Store: BEGIN TRANSACTION
        Service->>Store: INSERT INTO namespaces<br/>(name, description, quota, ...)

        alt Namespace created
            Store-->>Service: Success
            Service->>Store: INSERT INTO audit_log<br/>(actor, operation, namespace, ...)
            Service->>Store: COMMIT

            Service->>Audit: Log success:<br/>{actor: alice, operation: CreateNamespace,<br/>namespace: analytics, success: true}

            Service-->>CLI: CreateNamespaceResponse<br/>{namespace: {...}, created_at: ...}

        else Namespace exists
            Store-->>Service: UniqueViolation error
            Service->>Store: ROLLBACK

            Service->>Audit: Log failure:<br/>{actor: alice, operation: CreateNamespace,<br/>namespace: analytics, success: false,<br/>error: "already exists"}

            Service-->>CLI: AlreadyExists (6)
        end

    else Not authorized
        Auth->>Audit: Log denial:<br/>{actor: alice, operation: CreateNamespace,<br/>decision: deny, reason: "insufficient permissions"}

        Auth-->>CLI: PermissionDenied (7)
    end
```

### Session Monitoring Flow

```mermaid
sequenceDiagram
    participant CLI as Admin CLI
    participant API as Admin API
    participant SessionMgr as Session Manager
    participant Metrics as Metrics Store

    CLI->>API: ListSessions()<br/>{namespace: "user-api", status: ACTIVE}

    API->>API: Authorize request

    API->>SessionMgr: Query active sessions
    SessionMgr->>SessionMgr: Filter by namespace + status

    loop For each session
        SessionMgr->>Metrics: Get session metrics<br/>(requests, bytes, last_activity)
        Metrics-->>SessionMgr: SessionMetrics
    end

    SessionMgr-->>API: List of SessionInfo<br/>[{session_id, client_id, metrics}, ...]

    API->>API: Audit log: alice listed sessions

    API-->>CLI: ListSessionsResponse<br/>{sessions: [...], total_count: 42}

    CLI->>CLI: Format as table
    CLI-->>CLI: Display to admin
```

### Backend Health Check Flow

```mermaid
sequenceDiagram
    participant CLI as Admin CLI
    participant API as Admin API
    participant HealthCheck as Health Checker
    participant PG as Postgres
    participant Kafka as Kafka
    participant NATS as NATS

    CLI->>API: GetBackendStatus()<br/>{backend_type: "all"}

    API->>API: Authorize request

    par Check Postgres
        API->>HealthCheck: Check Postgres
        HealthCheck->>PG: SELECT 1
        alt Healthy
            PG-->>HealthCheck: Success (< 100ms)
            HealthCheck-->>API: HEALTHY
        else Degraded
            PG-->>HealthCheck: Success (> 500ms)
            HealthCheck-->>API: DEGRADED
        else Unhealthy
            PG--xHealthCheck: Connection timeout
            HealthCheck-->>API: UNHEALTHY
        end
    and Check Kafka
        API->>HealthCheck: Check Kafka
        HealthCheck->>Kafka: List topics
        Kafka-->>HealthCheck: Success
        HealthCheck-->>API: HEALTHY
    and Check NATS
        API->>HealthCheck: Check NATS
        HealthCheck->>NATS: Ping
        NATS-->>HealthCheck: Pong
        HealthCheck-->>API: HEALTHY
    end

    API->>API: Aggregate results
    API->>API: Audit log: alice checked backend health

    API-->>CLI: GetBackendStatusResponse<br/>{<br/>  backends: [<br/>    {type: "postgres", status: HEALTHY, latency_ms: 2.3},<br/>    {type: "kafka", status: HEALTHY, latency_ms: 5.1},<br/>    {type: "nats", status: HEALTHY, latency_ms: 1.2}<br/>  ]<br/>}

    CLI->>CLI: Format health summary
    CLI-->>CLI: Display to admin
```

## Session Management

### Session Lifecycle

Admin sessions support both interactive use (CLI) and automation (CI/CD):

**Interactive Session:**
- Acquire OIDC token via device code flow
- Token cached to `~/.prism/token`
- Token expires after 1 hour (refresh_token extends to 7 days)
- Automatic token refresh on expiry

**Automation Session:**
- Service account with client_credentials grant
- Token expires after 1 hour (no refresh token)
- Must re-authenticate for new token

### Session Establishment

```mermaid
sequenceDiagram
    participant User as Administrator
    participant CLI as Admin CLI
    participant OIDC as OIDC Provider
    participant API as Admin API

    Note over User,API: First-time Setup

    User->>CLI: prism-admin login
    CLI->>OIDC: Device code flow
    OIDC-->>CLI: device_code, verification_uri

    CLI->>User: Please visit:<br/>https://idp.example.com/activate<br/>and enter code: WXYZ-5678

    User->>OIDC: Complete authentication

    CLI->>OIDC: Poll for token
    OIDC-->>CLI: access_token, refresh_token

    CLI->>CLI: Save to ~/.prism/token:<br/>{<br/>  access_token,<br/>  refresh_token,<br/>  expires_at: <timestamp><br/>}

    CLI-->>User: ✓ Authenticated as alice@company.com<br/>Token expires in 1 hour

    Note over User,API: Subsequent Commands

    User->>CLI: prism-admin namespace list
    CLI->>CLI: Load ~/.prism/token
    CLI->>CLI: Check expiry

    alt Token valid
        CLI->>API: ListNamespaces()<br/>Authorization: Bearer <access_token>
        API-->>CLI: Response
    else Token expired
        CLI->>OIDC: POST /oauth/token<br/>{grant_type: refresh_token, ...}
        OIDC-->>CLI: New access_token
        CLI->>CLI: Update ~/.prism/token
        CLI->>API: ListNamespaces()<br/>Authorization: Bearer <new_token>
        API-->>CLI: Response
    end
```

## gRPC Protocol Specification

### Service Definition

```protobuf
syntax = "proto3";

package prism.admin.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/empty.proto";

service AdminService {
  // Configuration Management
  rpc ListConfigs(ListConfigsRequest) returns (ListConfigsResponse);
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
  rpc CreateConfig(CreateConfigRequest) returns (CreateConfigResponse);
  rpc UpdateConfig(UpdateConfigRequest) returns (UpdateConfigResponse);
  rpc DeleteConfig(DeleteConfigRequest) returns (DeleteConfigResponse);

  // Namespace Management
  rpc ListNamespaces(ListNamespacesRequest) returns (ListNamespacesResponse);
  rpc CreateNamespace(CreateNamespaceRequest) returns (CreateNamespaceResponse);
  rpc UpdateNamespace(UpdateNamespaceRequest) returns (UpdateNamespaceResponse);
  rpc DeleteNamespace(DeleteNamespaceRequest) returns (DeleteNamespaceResponse);
  rpc GetNamespace(GetNamespaceRequest) returns (GetNamespaceResponse);

  // Session Management
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc GetSession(GetSessionRequest) returns (GetSessionResponse);
  rpc TerminateSession(TerminateSessionRequest) returns (TerminateSessionResponse);

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
```

### Metadata Requirements

All requests must include:

```
authorization: Bearer <jwt_token>
request-id: <uuid>  // Optional but recommended
```

### Error Codes

Standard gRPC status codes:

- `OK (0)`: Success
- `INVALID_ARGUMENT (3)`: Invalid request parameters
- `NOT_FOUND (5)`: Resource not found
- `ALREADY_EXISTS (6)`: Resource already exists
- `PERMISSION_DENIED (7)`: Insufficient permissions
- `UNAUTHENTICATED (16)`: Missing or invalid JWT
- `INTERNAL (13)`: Server error

## RBAC Policy

### Roles

```yaml
roles:
  admin:
    description: Full administrative access
    permissions:
      - admin:read
      - admin:write
      - admin:operational
      - admin:audit

  operator:
    description: Operational tasks, read-only config
    permissions:
      - admin:read
      - admin:operational

  viewer:
    description: Read-only access
    permissions:
      - admin:read
```

### Permission Mapping

| Operation | Required Permission |
|-----------|-------------------|
| ListNamespaces | `admin:read` |
| CreateNamespace | `admin:write` |
| UpdateNamespace | `admin:write` |
| DeleteNamespace | `admin:write` |
| ListSessions | `admin:read` |
| TerminateSession | `admin:operational` |
| GetBackendStatus | `admin:read` |
| SetMaintenanceMode | `admin:operational` |
| DrainConnections | `admin:operational` |
| GetAuditLog | `admin:audit` |

### Authorization Middleware

```rust
use tonic::{Request, Status};
use tonic::metadata::MetadataMap;

pub struct AuthInterceptor {
    jwt_validator: Arc<JwtValidator>,
    rbac: Arc<RbacService>,
}

impl AuthInterceptor {
    pub async fn intercept(&self, mut req: Request<()>) -> Result<Request<()>, Status> {
        // Extract JWT from metadata
        let token = req.metadata()
            .get("authorization")
            .and_then(|v| v.to_str().ok())
            .and_then(|s| s.strip_prefix("Bearer "))
            .ok_or_else(|| Status::unauthenticated("Missing authorization header"))?;

        // Validate JWT
        let claims = self.jwt_validator.validate_token(token).await
            .map_err(|e| Status::unauthenticated(format!("Invalid token: {}", e)))?;

        // Extract required permission from method
        let method = req.uri().path();
        let required_permission = self.method_to_permission(method);

        // Check RBAC
        if !self.rbac.has_permission(&claims, &required_permission).await {
            return Err(Status::permission_denied(format!(
                "User {} lacks permission {}",
                claims.email, required_permission
            )));
        }

        // Inject claims into request extensions
        req.extensions_mut().insert(claims);

        Ok(req)
    }

    fn method_to_permission(&self, method: &str) -> String {
        match method {
            "/prism.admin.v1.AdminService/CreateNamespace" => "admin:write",
            "/prism.admin.v1.AdminService/ListNamespaces" => "admin:read",
            "/prism.admin.v1.AdminService/SetMaintenanceMode" => "admin:operational",
            "/prism.admin.v1.AdminService/GetAuditLog" => "admin:audit",
            _ => "admin:read",
        }.to_string()
    }
}
```

## Audit Logging

### Audit Entry Structure

```rust
#[derive(Debug, Serialize)]
pub struct AuditLogEntry {
    pub id: Uuid,
    pub timestamp: DateTime<Utc>,
    pub actor: String,              // Claims.email
    pub actor_groups: Vec<String>,  // Claims.groups
    pub operation: String,           // "CreateNamespace"
    pub resource_type: String,       // "namespace"
    pub resource_id: String,         // "analytics"
    pub namespace: Option<String>,
    pub request_id: Option<String>,
    pub success: bool,
    pub error: Option<String>,
    pub metadata: serde_json::Value,
}
```

### Storage

```sql
CREATE TABLE admin_audit_log (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,
    actor VARCHAR(255) NOT NULL,
    actor_groups TEXT[] NOT NULL,
    operation VARCHAR(255) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    namespace VARCHAR(255),
    request_id VARCHAR(100),
    success BOOLEAN NOT NULL,
    error TEXT,
    metadata JSONB,

    INDEX idx_audit_timestamp ON admin_audit_log(timestamp DESC),
    INDEX idx_audit_actor ON admin_audit_log(actor),
    INDEX idx_audit_operation ON admin_audit_log(operation),
    INDEX idx_audit_namespace ON admin_audit_log(namespace)
);
```

## Security Considerations

### Network Isolation

```yaml
# Kubernetes NetworkPolicy
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
          role: admin  # Only admin pods
    ports:
    - protocol: TCP
      port: 8981
```

### Rate Limiting

```rust
use governor::{Quota, RateLimiter};

pub struct RateLimitInterceptor {
    limiter: Arc<RateLimiter<String>>,  // Key: actor email
}

impl RateLimitInterceptor {
    pub fn new() -> Self {
        // 100 requests per minute per user
        let quota = Quota::per_minute(NonZeroU32::new(100).unwrap());
        Self {
            limiter: Arc::new(RateLimiter::keyed(quota)),
        }
    }

    pub async fn check(&self, claims: &Claims) -> Result<(), Status> {
        if self.limiter.check_key(&claims.email).is_err() {
            return Err(Status::resource_exhausted(format!(
                "Rate limit exceeded for {}",
                claims.email
            )));
        }
        Ok(())
    }
}
```

### TLS Configuration

```rust
use tonic::transport::{Server, ServerTlsConfig};

let tls_config = ServerTlsConfig::new()
    .identity(Identity::from_pem(cert_pem, key_pem))
    .client_ca_root(Certificate::from_pem(ca_pem));

Server::builder()
    .tls_config(tls_config)?
    .add_service(AdminServiceServer::new(admin_service))
    .serve("[::]:8981".parse()?)
    .await?;
```

## Deployment

### Docker Compose

```yaml
services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "8980:8980"  # Data plane
      - "8981:8981"  # Admin API (bind to internal network only)
    environment:
      PRISM_ADMIN_PORT: 8981
      PRISM_OIDC_ISSUER: https://idp.example.com
      PRISM_OIDC_AUDIENCE: prism-admin-api
      PRISM_OIDC_JWKS_URI: https://idp.example.com/.well-known/jwks.json
    networks:
      - internal  # Admin API not exposed publicly

networks:
  internal:
    internal: true
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prism-proxy
spec:
  template:
    spec:
      containers:
      - name: proxy
        image: prism/proxy:latest
        ports:
        - containerPort: 8980
          name: data
        - containerPort: 8981
          name: admin
        env:
        - name: PRISM_OIDC_ISSUER
          valueFrom:
            secretKeyRef:
              name: prism-oidc
              key: issuer
        - name: PRISM_OIDC_AUDIENCE
          valueFrom:
            secretKeyRef:
              name: prism-oidc
              key: audience
---
apiVersion: v1
kind: Service
metadata:
  name: prism-admin
spec:
  type: ClusterIP  # Internal only
  selector:
    app: prism-proxy
  ports:
  - port: 8981
    targetPort: 8981
    name: admin
```

## Testing

### Integration Tests

```go
func TestAdminProtocol(t *testing.T) {
    // Start mock OIDC server
    oidcServer := mockoidc.NewServer(t)
    defer oidcServer.Close()

    // Start Prism Admin API
    adminAPI := startAdminAPI(t, oidcServer.URL)
    defer adminAPI.Close()

    // Acquire token
    token, err := oidcServer.AcquireToken(
        "alice@example.com",
        []string{"admin:read", "admin:write"},
    )
    require.NoError(t, err)

    // Create namespace
    conn, err := grpc.Dial(adminAPI.Address(),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithPerRPCCredentials(BearerToken{token}),
    )
    require.NoError(t, err)
    defer conn.Close()

    client := admin.NewAdminServiceClient(conn)
    resp, err := client.CreateNamespace(context.Background(), &admin.CreateNamespaceRequest{
        Name: "test-namespace",
        Description: "Test namespace",
    })
    require.NoError(t, err)
    assert.Equal(t, "test-namespace", resp.Namespace.Name)
}
```

## Open Questions

1. **OIDC Provider Choice**: Support multiple providers (Okta, Auth0, Google, Azure AD)?
2. **Token Caching**: How long to cache validated JWTs before re-validating?
3. **Offline Access**: Support for offline token validation (signed JWTs)?
4. **Multi-Tenancy**: How to map OIDC tenants to Prism namespaces?
5. **Service Accounts**: Best practices for automation tokens?

## References

- [OAuth 2.0 Device Authorization Grant](https://datatracker.ietf.org/doc/html/rfc8628)
- [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)
- [JSON Web Token (JWT)](https://datatracker.ietf.org/doc/html/rfc7519)
- [gRPC Authentication](https://grpc.io/docs/guides/auth/)
- ADR-007: Authentication and Authorization
- ADR-027: Admin API via gRPC
- RFC-003: Admin Interface for Prism

## Revision History

- 2025-10-09: Initial draft with OIDC flows and sequence diagrams
