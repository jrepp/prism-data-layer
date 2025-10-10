---
title: "ADR-007: Authentication and Authorization"
status: Accepted
date: 2025-10-05
deciders: Core Team
tags: ['security', 'architecture']
---

## Context

Prism handles sensitive data and must ensure:
- **Authentication**: Verify who/what is making requests
- **Authorization**: Ensure they're allowed to access the data
- **Audit**: Track all access for compliance

Multiple access patterns:
- **Service-to-service**: Backend microservices calling Prism
- **User-facing**: APIs exposed to end users (via app backends)
- **Admin**: Platform team managing Prism itself

## Decision

Use **mTLS for service-to-service** authentication and **OAuth2/JWT for user-facing** APIs, with **namespace-level authorization** policies.

## Rationale

### Authentication Strategy

**Service-to-Service (Primary)**:
Service A --[mTLS]--> Prism Proxy --[mTLS]--> Backend
           (cert-based auth)

Certificate contains:
- Service name (CN: user-api.prod.company.com)
- Environment (prod, staging, dev)
- Expiry (auto-rotated)
```

**User-Facing APIs** (if exposed):
User --> App Backend --[OAuth2 JWT]--> Prism Proxy
                      (Bearer token)

JWT contains:
- User ID
- Scopes/permissions
- Expiry
```

### Certificate-Based Authentication (mTLS)

Every service gets a certificate signed by company CA:

```rust
use rustls::{ServerConfig, ClientConfig};

// Proxy server config
let mut server_config = ServerConfig::new(NoClientAuth::new());
server_config
    .set_single_cert(server_cert, server_key)?
    .set_client_certificate_verifier(
        AllowAnyAuthenticatedClient::new(client_ca_cert)
    );

// Verify client certificate
let tls_acceptor = TlsAcceptor::from(Arc::new(server_config));
let tls_stream = tls_acceptor.accept(tcp_stream).await?;

// Extract service identity from cert
let peer_certs = tls_stream.get_ref().1.peer_certificates();
let service_name = extract_cn_from_cert(peer_certs[0])?;

// service_name = "user-api.prod.company.com"
```

### Authorization Model

**Namespace-based RBAC**:

```yaml
namespace: user-profiles

access_control:
  # Teams that own this namespace
  owners:
    - team: user-platform
      role: admin

  # Services that can access
  consumers:
    - service: user-api.prod.*
      permissions: [read, write]

    - service: analytics-pipeline.prod.*
      permissions: [read]

    - service: admin-dashboard.prod.*
      permissions: [read]

  # Deny by default
  default_policy: deny
```

**Permission Levels**:
- `read`: Get, Scan operations
- `write`: Put, Delete operations
- `admin`: Modify namespace configuration

### Authorization Flow

```rust
pub struct AuthorizationService {
    policies: Arc<RwLock<HashMap<String, NamespacePolicy>>>,
}

impl AuthorizationService {
    pub async fn authorize(
        &self,
        service_name: &str,
        namespace: &str,
        operation: Operation,
    ) -> Result<Decision> {
        let policy = self.policies.read().await
            .get(namespace)
            .ok_or(Error::NamespaceNotFound)?;

        // Check if service is allowed
        for consumer in &policy.consumers {
            if consumer.service_pattern.matches(service_name) {
                let required_perm = match operation {
                    Operation::Get | Operation::Scan => Permission::Read,
                    Operation::Put | Operation::Delete => Permission::Write,
                };

                if consumer.permissions.contains(&required_perm) {
                    return Ok(Decision::Allow);
                }
            }
        }

        // Deny by default
        Ok(Decision::Deny(format!(
            "Service {} not authorized for {:?} on namespace {}",
            service_name, operation, namespace
        )))
    }
}
```

### Tower Middleware Integration

```rust
use tower::{Service, Layer};

pub struct AuthLayer {
    authz: Arc<AuthorizationService>,
}

impl<S> Layer<S> for AuthLayer {
    type Service = AuthMiddleware<S>;

    fn layer(&self, inner: S) -> Self::Service {
        AuthMiddleware {
            inner,
            authz: self.authz.clone(),
        }
    }
}

pub struct AuthMiddleware<S> {
    inner: S,
    authz: Arc<AuthorizationService>,
}

impl<S> Service<Request> for AuthMiddleware<S>
where
    S: Service<Request>,
{
    type Response = S::Response;
    type Error = S::Error;

    async fn call(&mut self, req: Request) -> Result<Self::Response> {
        // Extract service identity from mTLS cert
        let service_name = req.extensions()
            .get::<PeerCertificate>()
            .and_then(|cert| extract_service_name(cert))?;

        // Extract namespace and operation from request
        let namespace = req.extensions().get::<Namespace>()?;
        let operation = Operation::from_method(&req.method());

        // Authorize
        match self.authz.authorize(&service_name, namespace, operation).await? {
            Decision::Allow => {
                // Log and allow
                tracing::info!(
                    service = %service_name,
                    namespace = %namespace,
                    operation = ?operation,
                    "Request authorized"
                );
                self.inner.call(req).await
            }
            Decision::Deny(reason) => {
                // Log and reject
                tracing::warn!(
                    service = %service_name,
                    namespace = %namespace,
                    operation = ?operation,
                    reason = %reason,
                    "Request denied"
                );
                Err(Error::Forbidden(reason))
            }
        }
    }
}
```

### Audit Logging

Every request generates an audit entry:

```json
{
  "timestamp": "2025-10-05T12:34:56Z",
  "request_id": "req-abc-123",
  "service": "user-api.prod.us-east-1",
  "user_id": "user:12345",  // If JWT auth
  "namespace": "user-profiles",
  "operation": "get",
  "keys": ["user:12345"],  // Redacted if PII
  "decision": "allow",
  "latency_ms": 2.3,
  "backend": "postgres"
}
```

### Alternatives Considered

1. **API Keys**
   - Pros: Simple
   - Cons: Hard to rotate, often leaked
   - Rejected: Not secure enough

2. **OAuth2 for Everything**
   - Pros: Industry standard, good for users
   - Cons: Overkill for service-to-service, token endpoint becomes SPOF
   - Rejected: mTLS better for internal services

3. **No Authentication** (rely on network security)
   - Pros: Zero overhead
   - Cons: Defense in depth, compliance requirements
   - Rejected: Unacceptable for production

4. **Row-Level Security** (database-native)
   - Pros: Enforced at DB layer
   - Cons: Backend-specific, can't work with Kafka
   - Rejected: Doesn't cover all backends

## Consequences

### Positive

- **Strong Authentication**: mTLS is industry best practice
- **Fine-Grained AuthZ**: Namespace-level policies are flexible
- **Audit Trail**: Every request logged for compliance
- **Defense in Depth**: Multiple layers of security

### Negative

- **Certificate Management**: Need PKI infrastructure
  - *Mitigation*: Use existing company CA or service mesh (Linkerd, Istio)
- **Policy Complexity**: Many namespaces = many policies
  - *Mitigation*: Template-based policies, inheritance

### Neutral

- **Performance**: mTLS handshake adds ~1-2ms
  - Acceptable for our latency budget
- **OAuth2 Complexity**: Token validation adds overhead
  - Cache validated tokens

## Implementation Notes

### Certificate Rotation

```rust
// Watch for certificate updates
pub struct CertificateWatcher {
    cert_path: PathBuf,
    key_path: PathBuf,
}

impl CertificateWatcher {
    pub async fn watch(&self) -> Result<()> {
        let mut watcher = notify::watcher(Duration::from_secs(60))?;
        watcher.watch(&self.cert_path, RecursiveMode::NonRecursive)?;

        loop {
            match watcher.recv().await {
                Ok(Event::Modify(_)) => {
                    tracing::info!("Certificate updated, reloading...");
                    self.reload_certificates().await?;
                }
                _ => {}
            }
        }
    }
}
```

### Policy Storage

Policies stored in control plane database:

```sql
CREATE TABLE namespace_policies (
    namespace VARCHAR(255) PRIMARY KEY,
    policy JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    updated_by VARCHAR(255) NOT NULL
);

CREATE INDEX idx_policies_updated ON namespace_policies(updated_at);
```

### Policy Distribution

```rust
// Shards pull policies from control plane
pub struct PolicySync {
    control_plane_url: String,
    local_cache: Arc<RwLock<HashMap<String, NamespacePolicy>>>,
}

impl PolicySync {
    pub async fn sync_loop(&self) -> Result<()> {
        let mut last_sync = Timestamp::now();

        loop {
            // Long-poll for updates
            let updates: Vec<PolicyUpdate> = self.http_client
                .get(&format!("{}/policies?since={}", self.control_plane_url, last_sync))
                .timeout(Duration::from_secs(30))
                .send()
                .await?
                .json()
                .await?;

            // Apply updates
            let mut cache = self.local_cache.write().await;
            for update in updates {
                cache.insert(update.namespace, update.policy);
                last_sync = update.timestamp;
            }

            tokio::time::sleep(Duration::from_secs(10)).await;
        }
    }
}
```

## References

- [mTLS Best Practices](https://www.cloudflare.com/learning/access-management/what-is-mutual-tls/)
- [OAuth 2.0 RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749)
- [RBAC in Kubernetes](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- ADR-006: Namespace and Multi-Tenancy
- FR-004: PII Handling

## Revision History

- 2025-10-05: Initial draft and acceptance
