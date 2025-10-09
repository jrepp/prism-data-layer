---
title: "RFC-011: Data Proxy Authentication (Input/Output)"
status: Proposed
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [authentication, mtls, proxy, backends, security]
---

## Abstract

This RFC specifies the complete authentication model for Prism's data proxy, covering both **input authentication** (how clients authenticate to the proxy) and **output authentication** (how the proxy authenticates to backend data stores). The design emphasizes mTLS for service-to-service communication, certificate management, and secure backend connectivity.

## Motivation

The Prism data proxy sits between client applications and heterogeneous backends, requiring:

**Input Authentication (Client → Proxy):**
- Verify client service identity
- Prevent unauthorized access to data plane
- Support namespace-level access control
- Provide audit trail of data access

**Output Authentication (Proxy → Backend):**
- Authenticate proxy to backend services
- Manage credentials for multiple backend types
- Support credential rotation without downtime
- Isolate backend credentials from clients

**Goals:**
- Define mTLS-based client authentication
- Specify backend authentication patterns per backend type
- Document credential management and rotation
- Provide sequence diagrams for all authentication flows

**Non-Goals:**
- Admin API authentication (covered in RFC-010)
- Application user authentication (application responsibility)
- Data encryption at rest (backend responsibility)

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                      Authentication Boundaries                   │
└──────────────────────────────────────────────────────────────────┘

Client Service → [mTLS] → Prism Proxy → [Backend Auth] → Backends
  (Input Auth)              (Identity)    (Output Auth)

Input:  mTLS certificates validate client identity
Output: Backend-specific credentials (mTLS, passwords, API keys)
```

### Ports and Security Zones

```
┌─────────────────────────────────────────────────────────────────┐
│                        Security Zones                           │
└─────────────────────────────────────────────────────────────────┘

Zone 1: Client Services (Service Mesh)
  - mTLS enforced
  - Certificates issued by company CA
  - Short-lived (24 hours)

Zone 2: Prism Proxy (DMZ)
  - Accepts mTLS from clients
  - Holds backend credentials
  - Enforces namespace ACLs

Zone 3: Backend Services (Secure Network)
  - Postgres: mTLS or password
  - Kafka: SASL/SCRAM or mTLS
  - NATS: JWT or mTLS
  - Redis: ACL + password
```

## Input Authentication (Client → Proxy)

### mTLS Certificate-Based Authentication

```mermaid
sequenceDiagram
    participant Client as Client Service<br/>(user-api.prod)
    participant CA as Certificate Authority
    participant Proxy as Prism Proxy
    participant AuthZ as Authorization Service

    Note over Client,AuthZ: Initial Setup (Once per deployment)

    Client->>CA: Request certificate<br/>CSR: CN=user-api.prod.us-east-1
    CA->>CA: Validate service identity<br/>Check DNS/registry
    CA-->>Client: Issue certificate<br/>Valid: 24 hours

    Note over Client,AuthZ: Every Request

    Client->>Proxy: TLS Handshake<br/>ClientHello + Certificate
    Proxy->>Proxy: Verify certificate:<br/>- Signature valid?<br/>- Not expired?<br/>- Issued by trusted CA?

    alt Certificate valid
        Proxy->>Proxy: Extract service identity<br/>CN: user-api.prod.us-east-1

        Proxy->>AuthZ: Authorize request<br/>{service: "user-api.prod",<br/> namespace: "user-profiles",<br/> operation: "get"}

        alt Authorized
            AuthZ-->>Proxy: Allow
            Proxy->>Proxy: Process request
            Proxy-->>Client: Response
        else Not authorized
            AuthZ-->>Proxy: Deny (no permissions)
            Proxy-->>Client: PermissionDenied (7)
        end

    else Certificate invalid/expired
        Proxy-->>Client: Unauthenticated (16)<br/>"Certificate expired"
    end
```

### Certificate Structure

Client certificates must include:

```yaml
Subject:
  CN: user-api.prod.us-east-1        # Service name + env + region
  O: Company Name
  OU: Platform Services

Subject Alternative Names:
  - DNS: user-api.prod.us-east-1.internal
  - DNS: user-api.prod.svc.cluster.local
  - URI: spiffe://company.com/ns/prod/sa/user-api

Extensions:
  Key Usage: Digital Signature, Key Encipherment
  Extended Key Usage: Client Authentication
  Validity: 24 hours
```

### Rust Implementation

```rust
use rustls::{ServerConfig, ClientCertVerifier, Certificate};
use x509_parser::prelude::*;

pub struct PrismClientVerifier {
    ca_cert: Certificate,
}

impl ClientCertVerifier for PrismClientVerifier {
    fn verify_client_cert(
        &self,
        cert_chain: &[Certificate],
        _sni: Option<&str>,
    ) -> Result<ClientCertVerified, TLSError> {
        if cert_chain.is_empty() {
            return Err(TLSError::NoCertificatesPresented);
        }

        let client_cert = &cert_chain[0];

        // Verify signature chain
        self.verify_cert_chain(client_cert, &self.ca_cert)?;

        // Check expiry
        let (_, parsed) = X509Certificate::from_der(&client_cert.0)
            .map_err(|_| TLSError::InvalidCertificateData("Failed to parse".into()))?;

        if !parsed.validity().is_valid() {
            return Err(TLSError::InvalidCertificateData("Expired".into()));
        }

        Ok(ClientCertVerified::assertion())
    }
}

pub struct ServiceIdentity {
    pub service_name: String,
    pub environment: String,
    pub region: String,
}

impl ServiceIdentity {
    pub fn from_certificate(cert: &Certificate) -> Result<Self> {
        let (_, parsed) = X509Certificate::from_der(&cert.0)?;

        // Extract CN from subject
        let cn = parsed.subject()
            .iter_common_name()
            .next()
            .and_then(|cn| cn.as_str().ok())
            .ok_or(Error::MissingCommonName)?;

        // Parse: user-api.prod.us-east-1
        let parts: Vec<&str> = cn.split('.').collect();
        if parts.len() < 2 {
            return Err(Error::InvalidCommonName);
        }

        Ok(ServiceIdentity {
            service_name: parts[0].to_string(),
            environment: parts.get(1).unwrap_or(&"unknown").to_string(),
            region: parts.get(2).unwrap_or(&"unknown").to_string(),
        })
    }
}
```

### Request Flow with mTLS

```mermaid
sequenceDiagram
    participant App as Application<br/>(user-api)
    participant Proxy as Prism Proxy
    participant Backend as Backend<br/>(Postgres)

    Note over App,Backend: Data Request with mTLS

    App->>Proxy: TLS Handshake<br/>Present certificate

    Proxy->>Proxy: Verify certificate<br/>Extract identity:<br/>service=user-api.prod

    App->>Proxy: gRPC: Get()<br/>{namespace: "user-profiles",<br/> id: "user:123", key: "profile"}

    Proxy->>Proxy: Authorize:<br/>Can user-api.prod read<br/>namespace user-profiles?

    alt Authorized
        Proxy->>Proxy: Determine backend:<br/>user-profiles → postgres-main

        Proxy->>Backend: Authenticate via<br/>backend credentials<br/>(see Output Auth)

        Backend-->>Proxy: Data

        Proxy->>Proxy: Audit log:<br/>service=user-api.prod,<br/>namespace=user-profiles,<br/>operation=get,<br/>result=success

        Proxy-->>App: GetResponse

    else Not authorized
        Proxy->>Proxy: Audit log:<br/>service=user-api.prod,<br/>namespace=user-profiles,<br/>operation=get,<br/>result=denied

        Proxy-->>App: PermissionDenied (7)
    end
```

### Certificate Rotation

```rust
use notify::{Watcher, RecursiveMode};

pub struct CertificateReloader {
    cert_path: PathBuf,
    key_path: PathBuf,
    server_config: Arc<RwLock<ServerConfig>>,
}

impl CertificateReloader {
    pub async fn watch(&self) -> Result<()> {
        let (tx, rx) = mpsc::channel();
        let mut watcher = notify::watcher(tx, Duration::from_secs(30))?;

        watcher.watch(&self.cert_path, RecursiveMode::NonRecursive)?;
        watcher.watch(&self.key_path, RecursiveMode::NonRecursive)?;

        loop {
            match rx.recv() {
                Ok(DebouncedEvent::Write(_) | DebouncedEvent::Create(_)) => {
                    tracing::info!("Certificate files changed, reloading...");

                    let new_config = self.load_server_config().await?;

                    let mut config = self.server_config.write().await;
                    *config = new_config;

                    tracing::info!("Certificate reloaded successfully");
                }
                _ => {}
            }
        }
    }
}
```

## Output Authentication (Proxy → Backend)

### Per-Backend Authentication Strategies

```
┌─────────────────────────────────────────────────────────────────┐
│                 Backend Authentication Matrix                   │
└─────────────────────────────────────────────────────────────────┘

Backend      | Primary Auth      | Fallback       | Credential Store
-------------|-------------------|----------------|------------------
Postgres     | mTLS              | Password       | Vault/K8s Secret
Kafka        | SASL/SCRAM        | mTLS           | Vault/K8s Secret
NATS         | JWT               | NKey           | Vault/K8s Secret
Redis        | ACL + Password    | None           | Vault/K8s Secret
SQLite       | File permissions  | None           | N/A (local)
S3           | IAM Role          | Access Keys    | Instance Profile
```

### Postgres Authentication

```mermaid
sequenceDiagram
    participant Proxy as Prism Proxy
    participant Vault as HashiCorp Vault
    participant PG as PostgreSQL

    Note over Proxy,PG: Initial Connection

    Proxy->>Vault: Request credentials<br/>GET /v1/database/creds/postgres-main
    Vault->>Vault: Generate dynamic credentials<br/>User: prism-prod-abc123<br/>Password: <random><br/>TTL: 1 hour
    Vault->>PG: CREATE ROLE prism-prod-abc123<br/>WITH LOGIN PASSWORD '...'<br/>VALID UNTIL '2025-10-09 15:00'
    Vault-->>Proxy: {username, password, lease_id}

    Proxy->>Proxy: Cache credentials<br/>Set lease renewal timer

    Note over Proxy,PG: Data Operations

    Proxy->>PG: Connect<br/>SSL mode: require<br/>User: prism-prod-abc123<br/>Password: <from vault>
    PG->>PG: Verify credentials
    PG-->>Proxy: Connection established

    Proxy->>PG: SELECT * FROM user_profiles<br/>WHERE id = 'user:123'
    PG-->>Proxy: Row data

    Note over Proxy,PG: Credential Renewal

    loop Every 30 minutes
        Proxy->>Vault: Renew lease<br/>PUT /v1/sys/leases/renew<br/>{lease_id}
        Vault-->>Proxy: Lease extended
    end

    Note over Proxy,PG: Credential Expiry

    alt Lease expires
        Vault->>PG: REVOKE ROLE prism-prod-abc123
        Proxy->>Vault: Request new credentials
        Vault-->>Proxy: New {username, password}
        Proxy->>Proxy: Update connection pool
    end
```

### Kafka Authentication (SASL/SCRAM)

```mermaid
sequenceDiagram
    participant Proxy as Prism Proxy
    participant Vault as HashiCorp Vault
    participant Kafka as Kafka Broker

    Note over Proxy,Kafka: Bootstrap

    Proxy->>Vault: GET /v1/kafka/creds/producer-main
    Vault->>Vault: Generate SCRAM credentials<br/>User: prism-kafka-xyz789<br/>Password: <scram-sha-512>
    Vault->>Kafka: kafka-configs --alter<br/>--entity-type users<br/>--entity-name prism-kafka-xyz789<br/>--add-config 'SCRAM-SHA-512=[...]'
    Vault-->>Proxy: {username, password, mechanism: SCRAM-SHA-512}

    Proxy->>Kafka: SASL Handshake<br/>Mechanism: SCRAM-SHA-512
    Kafka-->>Proxy: SASL Challenge

    Proxy->>Kafka: SASL Response<br/>{username, scrambled_password}
    Kafka->>Kafka: Verify SCRAM
    Kafka-->>Proxy: Authenticated

    Note over Proxy,Kafka: Produce Messages

    Proxy->>Kafka: ProduceRequest<br/>Topic: user-events
    Kafka->>Kafka: Check ACLs:<br/>Can prism-kafka-xyz789 write to topic?
    Kafka-->>Proxy: ProduceResponse
```

### NATS Authentication (JWT)

```mermaid
sequenceDiagram
    participant Proxy as Prism Proxy
    participant Vault as HashiCorp Vault
    participant NATS as NATS Server

    Proxy->>Vault: GET /v1/nats/creds/publisher
    Vault->>Vault: Generate JWT + NKey<br/>Claims: {<br/>  pub: ["events.>"],<br/>  sub: ["responses.prism.>"]<br/>}
    Vault-->>Proxy: {jwt, seed (nkey)}

    Proxy->>NATS: CONNECT {<br/>  jwt: "eyJhbG...",<br/>  sig: sign(nonce, nkey)<br/>}
    NATS->>NATS: Verify JWT signature<br/>Check expiry<br/>Validate claims
    NATS-->>Proxy: +OK

    Proxy->>NATS: PUB events.user.login 42<br/>{user_id: "123", timestamp: ...}
    NATS->>NATS: Check permissions:<br/>Can JWT publish to events.user.login?
    NATS-->>Proxy: +OK
```

### Redis Authentication (ACL)

```mermaid
sequenceDiagram
    participant Proxy as Prism Proxy
    participant Vault as HashiCorp Vault
    participant Redis as Redis Server

    Proxy->>Vault: GET /v1/redis/creds/cache-rw
    Vault->>Vault: Generate Redis ACL<br/>User: prism-cache-abc<br/>Password: <random><br/>ACL: ~cache:* +get +set +del
    Vault->>Redis: ACL SETUSER prism-cache-abc<br/>on >password ~cache:* +get +set +del
    Vault-->>Proxy: {username, password}

    Proxy->>Redis: AUTH prism-cache-abc <password>
    Redis->>Redis: Verify password<br/>Load ACL rules
    Redis-->>Proxy: OK

    Proxy->>Redis: GET cache:user:123:profile
    Redis->>Redis: Check ACL:<br/>Pattern match: cache:*<br/>Command allowed: GET
    Redis-->>Proxy: "{"name":"Alice",...}"

    Proxy->>Redis: SET cache:user:123:session <data>
    Redis-->>Proxy: OK
```

### Credential Management

```rust
use vaultrs::client::VaultClient;
use vaultrs::kv2;

pub struct BackendCredentials {
    pub backend_type: String,
    pub username: String,
    pub password: String,
    pub lease_id: Option<String>,
    pub expires_at: DateTime<Utc>,
}

pub struct CredentialManager {
    vault_client: VaultClient,
    credentials: Arc<RwLock<HashMap<String, BackendCredentials>>>,
}

impl CredentialManager {
    pub async fn get_credentials(&self, backend_id: &str) -> Result<BackendCredentials> {
        // Check cache
        {
            let creds = self.credentials.read().await;
            if let Some(cached) = creds.get(backend_id) {
                if cached.expires_at > Utc::now() + Duration::minutes(5) {
                    return Ok(cached.clone());
                }
            }
        }

        // Fetch from Vault
        let path = format!("database/creds/{}", backend_id);
        let creds: VaultCredentials = self.vault_client
            .read(&path)
            .await?;

        let backend_creds = BackendCredentials {
            backend_type: creds.backend_type,
            username: creds.username,
            password: creds.password,
            lease_id: Some(creds.lease_id),
            expires_at: Utc::now() + Duration::hours(1),
        };

        // Update cache
        {
            let mut cache = self.credentials.write().await;
            cache.insert(backend_id.to_string(), backend_creds.clone());
        }

        // Schedule renewal
        self.schedule_renewal(backend_id, &creds.lease_id).await;

        Ok(backend_creds)
    }

    async fn schedule_renewal(&self, backend_id: &str, lease_id: &str) {
        let vault_client = self.vault_client.clone();
        let lease_id = lease_id.to_string();

        tokio::spawn(async move {
            loop {
                tokio::time::sleep(Duration::minutes(30)).await;

                match vault_client.renew_lease(&lease_id).await {
                    Ok(_) => {
                        tracing::info!(
                            backend_id = %backend_id,
                            lease_id = %lease_id,
                            "Renewed backend credentials"
                        );
                    }
                    Err(e) => {
                        tracing::error!(
                            backend_id = %backend_id,
                            error = %e,
                            "Failed to renew credentials, will fetch new ones"
                        );
                        break;
                    }
                }
            }
        });
    }
}
```

## End-to-End Authentication Flow

```mermaid
sequenceDiagram
    participant App as Application
    participant Proxy as Prism Proxy
    participant Vault as Vault
    participant PG as Postgres
    participant Audit as Audit Log

    Note over App,Audit: Complete Request Flow

    App->>Proxy: mTLS Handshake<br/>Present client cert
    Proxy->>Proxy: Verify client cert<br/>Extract identity: user-api.prod

    App->>Proxy: Get(namespace="users", id="123", key="profile")

    Proxy->>Proxy: Authorize:<br/>user-api.prod → users namespace?

    alt Authorized
        Proxy->>Proxy: Lookup backend:<br/>users → postgres-main

        Proxy->>Proxy: Get credentials from cache

        alt Credentials expired/missing
            Proxy->>Vault: GET /database/creds/postgres-main
            Vault-->>Proxy: {username, password, lease_id}
            Proxy->>Proxy: Cache credentials
        end

        Proxy->>PG: Connect with credentials
        PG-->>Proxy: Connection OK

        Proxy->>PG: SELECT value FROM users<br/>WHERE id='123' AND key='profile'
        PG-->>Proxy: Row data

        Proxy->>Audit: Log:<br/>{service: user-api.prod,<br/> namespace: users,<br/> operation: get,<br/> backend: postgres-main,<br/> latency_ms: 2.3,<br/> result: success}

        Proxy-->>App: GetResponse{value}

    else Not authorized
        Proxy->>Audit: Log:<br/>{service: user-api.prod,<br/> namespace: users,<br/> operation: get,<br/> result: denied,<br/> reason: "no permissions"}

        Proxy-->>App: PermissionDenied (7)
    end
```

## Configuration

### Proxy Configuration

```yaml
# prism-proxy.yaml
data_port: 8980
admin_port: 8981

# Input Authentication
input_auth:
  type: mtls
  ca_cert: /etc/prism/certs/ca.crt
  server_cert: /etc/prism/certs/server.crt
  server_key: /etc/prism/certs/server.key
  client_cert_required: true
  verify_depth: 3

# Output Authentication
output_auth:
  credential_provider: vault
  vault:
    address: https://vault.internal:8200
    token_path: /var/run/secrets/vault-token
    namespace: prism-prod

backends:
  - name: postgres-main
    type: postgres
    auth:
      type: vault-dynamic
      path: database/creds/postgres-main
    connection:
      host: postgres.internal
      port: 5432
      database: users
      ssl_mode: require

  - name: kafka-events
    type: kafka
    auth:
      type: vault-dynamic
      path: kafka/creds/producer-main
    connection:
      brokers: [kafka-1:9092, kafka-2:9092, kafka-3:9092]
      security_protocol: SASL_SSL
      sasl_mechanism: SCRAM-SHA-512

  - name: nats-messages
    type: nats
    auth:
      type: vault-jwt
      path: nats/creds/publisher
    connection:
      servers: [nats://nats-1:4222, nats://nats-2:4222]
      tls_required: true
```

## Security Considerations

### Credential Isolation

```
Principle: Clients never see backend credentials

✓ Client presents certificate → Proxy validates
✓ Proxy fetches backend credentials → From Vault
✓ Proxy connects to backend → Using fetched credentials
✗ Client NEVER gets backend credentials
```

### Credential Rotation

```
Automatic rotation schedule:

1. Vault generates new credentials (TTL: 1 hour)
2. Proxy caches and renews every 30 minutes
3. On renewal failure, fetch new credentials
4. Gracefully drain old connections
5. Old credentials revoked by Vault after TTL
```

### Audit Requirements

Every data access must log:
- Client service identity (from mTLS cert)
- Namespace accessed
- Operation performed (get, put, delete, scan)
- Backend used
- Success/failure
- Latency

## Testing

### Integration Tests

```rust
#[tokio::test]
async fn test_mtls_authentication() {
    let ca = generate_test_ca();
    let client_cert = ca.issue_cert("test-service.prod.us-east-1");

    let proxy = ProxyServer::new_test()
        .with_ca(ca.cert())
        .start()
        .await;

    let client = Client::new()
        .with_mtls(client_cert, client_key)
        .connect(proxy.address())
        .await
        .unwrap();

    // Should succeed with valid cert
    let resp = client.get("test-namespace", "key:123", "field").await;
    assert!(resp.is_ok());

    // Should fail with expired cert
    let expired_cert = ca.issue_cert_with_ttl("expired", Duration::seconds(-1));
    let bad_client = Client::new()
        .with_mtls(expired_cert, key)
        .connect(proxy.address())
        .await;

    assert!(bad_client.is_err());
}
```

## Open Questions

1. **Certificate Authority**: Use company CA or service mesh (Linkerd/Istio)?
2. **Credential Caching**: How long to cache backend credentials?
3. **Connection Pooling**: Pool connections per backend or per credential?
4. **Fallback Auth**: What to do when Vault is unavailable?
5. **Observability**: How to monitor credential rotation and health?

## References

- [mTLS in Microservices](https://www.cloudflare.com/learning/access-management/what-is-mutual-tls/)
- [HashiCorp Vault Dynamic Secrets](https://www.vaultproject.io/docs/secrets/databases)
- [Kafka SASL/SCRAM](https://kafka.apache.org/documentation/#security_sasl_scram)
- [NATS JWT Authentication](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)
- ADR-007: Authentication and Authorization
- RFC-010: Admin Protocol with OIDC

## Revision History

- 2025-10-09: Initial draft with mTLS and backend authentication flows
