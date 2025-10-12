---
author: Platform Team
created: 2025-10-09
doc_uuid: b10ecd0b-e765-4bd5-aa76-c063193e75f9
id: memo-002
project_id: prism-data-layer
tags:
- security
- admin
- protocol
- grpc
- improvements
title: 'MEMO-002: Admin Protocol Security Review and Improvements'
updated: 2025-10-09
---

# MEMO-002: Admin Protocol Security Review and Improvements

## Purpose

Comprehensive security and design review of RFC-010 (Admin Protocol with OIDC) to identify improvements, simplifications, and long-term extensibility concerns.

## Status Update (2025-10-09)

**✅ RECOMMENDATIONS IMPLEMENTED**: All key recommendations from this security review have been incorporated into current RFCs and ADRs through the following commits:

### Implementation History

**Commit [d6fb2b1](https://github.com/jrepp/prism-data-layer/commit/d6fb2b1) - "Add comprehensive documentation updates and new RFC-014"** (2025-10-09 10:30 AM)
- ✅ Expanded RFC-010 open questions with multi-provider OIDC support (AWS Cognito, Azure AD, Google, Okta, Auth0, Dex)
- ✅ Added token caching strategies (24h default with JWKS caching and refresh token support)
- ✅ Added offline access validation with cached JWKS and security trade-offs
- ✅ Added multi-tenancy mapping options (group-based, claim-based, OPA policy, tenant-scoped)
- ✅ Added service account approaches with comparison table and best practices

**Commit [e50feb3](https://github.com/jrepp/prism-data-layer/commit/e50feb3) - "Add documentation-first memo, expand auth RFCs"** (2025-10-09 12:17 PM)
- ✅ Expanded RFC-011 with comprehensive secrets provider abstraction (Vault, AWS Secrets Manager, Google Secret Manager, Azure Key Vault)
- ✅ Added credential management with automatic caching and renewal
- ✅ Added provider comparison matrix (dynamic credentials, auto-rotation, versioning, audit logging, cost)
- ✅ Created ADR-046 for Dex IDP as local OIDC provider for testing
- ✅ Added complete OIDC authentication section to RFC-006 with device code flow and token management

### Recommendations Status

1. ✅ **Resource-Level Authorization**: RFC-010 now includes namespace ownership, tagging, and ABAC policies
2. ✅ **Enhanced Audit Logging**: Tamper-evident logging with chain hashing, signatures, and trace ID correlation documented in RFC-010
3. ✅ **API Versioning**: Version negotiation endpoint and backward compatibility strategy added to RFC-010
4. ✅ **Adaptive Rate Limiting**: Different quotas for read/write/expensive operations with burst handling documented in RFC-010
5. ✅ **Input Validation**: Protobuf validation rules (protoc-gen-validate) added to RFC-010 with examples
6. ✅ **Session Management**: Comprehensive open questions section in RFC-010 with multi-provider support, token caching, offline validation, and multi-tenancy mapping options

### Summary

This memo now serves as a **historical record** of the security review process (conducted 2025-10-09 00:31 AM) that led to these improvements. All recommendations have been incorporated into RFC-010 (Admin Protocol with OIDC), RFC-011 (Data Proxy Authentication), RFC-006 (Python Admin CLI), and ADR-046 (Dex IDP for Local Testing) through commits made later the same day.

## Executive Summary

**Security Status**: Generally solid OIDC-based authentication with room for improvement in authorization granularity, rate limiting, and audit trail completeness.

**Key Recommendations**:
1. Add request-level resource authorization (not just method-level)
2. Implement structured audit logging with tamper-evident storage
3. Add API versioning to support long-term evolution
4. Simplify session management (remove ambiguity)
5. Add request signing for critical operations
6. Implement comprehensive input validation

## Security Analysis

### 1. Authentication (✅ Strong)

**Current State**:
- OIDC with JWT validation
- Device code flow for CLI
- Public key validation via JWKS

**Issues**: None critical

**Recommendations**:
```diff
+ Add JWT revocation checking (check against revocation list)
+ Add token binding to prevent token theft
+ Implement short-lived JWTs (5-15 min) with refresh tokens
```

**Improvement**:
```rust
pub struct JwtValidator {
    issuer: String,
    audience: String,
    jwks_client: JwksClient,
+   revocation_checker: Arc<RevocationChecker>,  // NEW
+   max_token_age: Duration,                     // NEW
}

impl JwtValidator {
    pub async fn validate_token(&self, token: &str) -> Result<Claims> {
        let token_data = decode::<Claims>(token, &decoding_key, &validation)?;

+       // Check revocation list
+       if self.revocation_checker.is_revoked(&token_data.claims.jti).await? {
+           return Err(Error::TokenRevoked);
+       }
+
+       // Enforce max token age
+       let token_age = Utc::now().timestamp() - token_data.claims.iat as i64;
+       if token_age > self.max_token_age.as_secs() as i64 {
+           return Err(Error::TokenTooOld);
+       }

        Ok(token_data.claims)
    }
}
```

### 2. Authorization (⚠️ Needs Improvement)

**Current State**:
- Method-level RBAC (e.g., `admin:write` for CreateNamespace)
- Three roles: admin, operator, viewer

**Issues**:
1. **No resource-level authorization**: User with `admin:write` can modify ANY namespace
2. **No attribute-based access control (ABAC)**: Can't restrict by namespace owner, tags, etc.
3. **Coarse-grained permissions**: Can't delegate specific operations

**Improvement**:
```protobuf
// Add resource-level authorization to requests
message CreateNamespaceRequest {
  string name = 1;
  string description = 2;

  // NEW: Resource ownership and tagging
  string owner = 3;         // User/team that owns this namespace
  repeated string tags = 4;  // For ABAC policies (e.g., "prod", "staging")
  map<string, string> labels = 5;  // Key-value metadata
}

// Authorization check becomes:
// 1. Does user have admin:write permission?
// 2. Is user allowed to create namespaces with owner=X?
// 3. Is user allowed to create namespaces with tags=[prod]?
```

**RBAC Policy Enhancement**:
```yaml
roles:
  namespace-admin:
    description: Can manage namespaces they own
    permissions:
      - admin:read
      - admin:write:namespace:owned  # NEW: Scoped permission

  team-lead:
    description: Can manage team's namespaces
    permissions:
      - admin:read
      - admin:write:namespace:team:*  # NEW: Wildcard for team namespaces

policies:
  - name: namespace-ownership
    effect: allow
    principals:
      - role:namespace-admin
    actions:
      - CreateNamespace
      - UpdateNamespace
      - DeleteNamespace
    resources:
      - namespace:${claims.email}/*  # Can only manage own namespaces

  - name: production-lockdown
    effect: deny
    principals:
      - role:developer
    actions:
      - DeleteNamespace
    resources:
      - namespace:*/tags:prod  # Cannot delete prod namespaces
```

### 3. Audit Logging (⚠️ Needs Improvement)

**Current State**:
- Basic audit log with actor, operation, resource
- Stored in Postgres

**Issues**:
1. **Not tamper-evident**: Admin with DB access can modify audit log
2. **No log signing**: Can't verify log integrity
3. **Missing context**: No client IP, user agent, request ID correlation
4. **No retention policy**: Logs could grow unbounded

**Improvement**:
```rust
#[derive(Debug, Serialize)]
pub struct AuditLogEntry {
    pub id: Uuid,
    pub timestamp: DateTime<Utc>,

    // Identity
    pub actor: String,
    pub actor_groups: Vec<String>,
+   pub actor_ip: IpAddr,           // NEW
+   pub user_agent: Option<String>,  // NEW

    // Operation
    pub operation: String,
    pub resource_type: String,
    pub resource_id: String,
    pub namespace: Option<String>,
    pub request_id: Option<String>,
+   pub trace_id: Option<String>,   // NEW: OpenTelemetry trace ID

    // Result
    pub success: bool,
    pub error: Option<String>,
+   pub duration_ms: u64,           // NEW
+   pub status_code: u32,           // NEW: gRPC status code

    // Security
    pub metadata: serde_json::Value,
+   pub signature: String,          // NEW: HMAC signature
+   pub chain_hash: String,         // NEW: Hash of previous log entry
}

impl AuditLogger {
    pub async fn log_entry(&self, entry: AuditLogEntry) -> Result<()> {
        // Sign the entry
        let signature = self.sign_entry(&entry)?;

        // Chain to previous entry (tamper-evident)
        let prev_hash = self.get_last_entry_hash().await?;
        let chain_hash = self.compute_chain_hash(&entry, &prev_hash)?;

        let signed_entry = SignedAuditLogEntry {
            entry,
            signature,
            chain_hash,
        };

        // Write to append-only log
        self.store.append(signed_entry).await?;

        // Also send to external SIEM (defense in depth)
        self.siem_exporter.export(signed_entry).await?;

        Ok(())
    }
}
```

**Storage**:
```sql
CREATE TABLE admin_audit_log (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,
    actor VARCHAR(255) NOT NULL,
    actor_groups TEXT[] NOT NULL,
+   actor_ip INET NOT NULL,
+   user_agent TEXT,
    operation VARCHAR(255) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    namespace VARCHAR(255),
    request_id VARCHAR(100),
+   trace_id VARCHAR(100),
    success BOOLEAN NOT NULL,
    error TEXT,
+   duration_ms BIGINT NOT NULL,
+   status_code INT NOT NULL,
    metadata JSONB,
+   signature VARCHAR(512) NOT NULL,
+   chain_hash VARCHAR(128) NOT NULL,

    INDEX idx_audit_timestamp ON admin_audit_log(timestamp DESC),
    INDEX idx_audit_actor ON admin_audit_log(actor),
    INDEX idx_audit_operation ON admin_audit_log(operation),
    INDEX idx_audit_namespace ON admin_audit_log(namespace),
+   INDEX idx_audit_trace_id ON admin_audit_log(trace_id)
);

-- Append-only table (prevent updates/deletes)
CREATE TRIGGER audit_log_immutable
BEFORE UPDATE OR DELETE ON admin_audit_log
FOR EACH ROW
EXECUTE FUNCTION prevent_modification();
```

### 4. Rate Limiting (⚠️ Needs Improvement)

**Current State**:
- 100 requests per minute per user
- No distinction between read/write operations

**Issues**:
1. **Too coarse**: Should differentiate between expensive and cheap operations
2. **No burst handling**: 100 req/min = ~1.6 req/sec, doesn't allow bursts
3. **No per-operation limits**: Can spam expensive operations

**Improvement**:
```rust
pub struct AdaptiveRateLimiter {
    // Different quotas for different operation types
    read_limiter: RateLimiter<String>,     // 1000 req/min
    write_limiter: RateLimiter<String>,    // 100 req/min
    expensive_limiter: RateLimiter<String>, // 10 req/min (e.g., ListSessions)

    // Burst allowance
    burst_quota: NonZeroU32,
}

impl AdaptiveRateLimiter {
    pub async fn check(&self, claims: &Claims, operation: &str) -> Result<(), Status> {
        let key = &claims.email;

        let limiter = match operation {
            // Expensive operations (database scans, aggregations)
            "ListSessions" | "GetMetrics" | "ExportMetrics" => &self.expensive_limiter,

            // Write operations (create, update, delete)
            op if op.starts_with("Create") || op.starts_with("Update")
                || op.starts_with("Delete") => &self.write_limiter,

            // Read operations (get, list, describe)
            _ => &self.read_limiter,
        };

        if limiter.check_key(key).is_err() {
            return Err(Status::resource_exhausted(format!(
                "Rate limit exceeded for {} (operation: {})",
                claims.email, operation
            )));
        }

        Ok(())
    }
}
```

### 5. Input Validation (⚠️ Missing)

**Current State**:
- No explicit validation in protobuf
- Relies on application logic

**Issues**:
1. **No length limits**: Namespace names, descriptions could be arbitrarily long
2. **No format validation**: Email, URLs, identifiers unchecked
3. **No sanitization**: Potential for injection attacks in metadata

**Improvement**:
```protobuf
message CreateNamespaceRequest {
  string name = 1 [
    (validate.rules).string = {
      min_len: 3
      max_len: 63
      pattern: "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"  // DNS-like naming
    }
  ];

  string description = 2 [
    (validate.rules).string = {
      max_len: 500
    }
  ];

  string owner = 3 [
    (validate.rules).string = {
      email: true  // Validate email format
    }
  ];

  repeated string tags = 4 [
    (validate.rules).repeated = {
      max_items: 10
      items: {
        string: {
          min_len: 1
          max_len: 50
          pattern: "^[a-z0-9-]+$"
        }
      }
    }
  ];

  map<string, string> labels = 5 [
    (validate.rules).map = {
      max_pairs: 20
      keys: {
        string: {
          min_len: 1
          max_len: 63
          pattern: "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"
        }
      }
      values: {
        string: {
          max_len: 255
        }
      }
    }
  ];
}
```

**Validation middleware**:
```rust
use validator::Validate;

pub struct ValidationInterceptor;

impl ValidationInterceptor {
    pub async fn intercept<T: Validate>(&self, req: Request<T>) -> Result<Request<T>, Status> {
        // Validate request using protoc-gen-validate
        req.get_ref().validate()
            .map_err(|e| Status::invalid_argument(format!("Validation error: {}", e)))?;

        Ok(req)
    }
}
```

### 6. API Versioning (❌ Missing)

**Current State**:
- No versioning in package name: `prism.admin.v1`
- No version negotiation

**Issues**:
1. **Breaking changes**: How to evolve protocol without breaking clients?
2. **Deprecation**: No way to deprecate old endpoints
3. **Feature flags**: No way to opt-in to new features

**Improvement**:
```protobuf
// Package with explicit version
package prism.admin.v2;

// Version negotiation
message GetVersionRequest {}

message GetVersionResponse {
  int32 api_version = 1;        // Current version: 2
  int32 min_supported = 2;       // Minimum supported: 1
  repeated int32 supported = 3;  // Supported versions: [1, 2]

  // Feature flags for gradual rollout
  map<string, bool> features = 4;  // e.g., {"shadow-traffic": true}
}

service AdminService {
  // Version negotiation
  rpc GetVersion(GetVersionRequest) returns (GetVersionResponse);

  // Versioned operations (with backward compatibility)
  rpc CreateNamespace(CreateNamespaceRequest) returns (CreateNamespaceResponse);
  rpc CreateNamespaceV2(CreateNamespaceV2Request) returns (CreateNamespaceV2Response);
}
```

### 7. Request Signing (❌ Missing)

**Current State**:
- No request integrity protection beyond TLS
- No replay attack prevention

**Issues**:
1. **Token theft**: Stolen JWT can be used until expiry
2. **Replay attacks**: Captured requests can be replayed
3. **Man-in-the-middle**: TLS protects transport, but not request integrity

**Improvement**:
```protobuf
message RequestMetadata {
  string timestamp = 1;     // ISO 8601 timestamp
  string nonce = 2;          // Random nonce for replay prevention
  string signature = 3;      // HMAC-SHA256(timestamp + nonce + request_body, jwt_secret)
}

// All requests include metadata
message CreateNamespaceRequest {
  RequestMetadata metadata = 1;

  string name = 2;
  string description = 3;
  // ... other fields
}
```

**Signature verification**:
```rust
pub struct SignatureVerifier {
    nonce_cache: Arc<NonceCache>,  // Redis-based cache
    max_request_age: Duration,     // 5 minutes
}

impl SignatureVerifier {
    pub async fn verify(&self, req: &CreateNamespaceRequest, claims: &Claims) -> Result<()> {
        let metadata = req.metadata.as_ref()
            .ok_or(Error::MissingMetadata)?;

        // Check timestamp freshness
        let timestamp = DateTime::parse_from_rfc3339(&metadata.timestamp)?;
        let age = Utc::now() - timestamp;
        if age > self.max_request_age {
            return Err(Error::RequestTooOld);
        }

        // Check nonce uniqueness (prevent replay)
        if self.nonce_cache.exists(&metadata.nonce).await? {
            return Err(Error::NonceReused);
        }
        self.nonce_cache.insert(&metadata.nonce, age).await?;

        // Verify signature
        let expected_signature = self.compute_signature(
            &metadata.timestamp,
            &metadata.nonce,
            req,
            &claims.sub,
        )?;

        if metadata.signature != expected_signature {
            return Err(Error::InvalidSignature);
        }

        Ok(())
    }
}
```

## Simplification Recommendations

### 1. Consolidate Session Operations

**Current**: Separate GetSession, DescribeSession, ListSessions

**Simplified**:
```protobuf
message GetSessionsRequest {
  // Filters (all optional)
  string namespace = 1;
  string session_id = 2;        // If specified, returns single session
  SessionStatus status = 3;

  // Pagination
  int32 page_size = 10;
  string page_token = 11;

  // Include detailed info?
  bool include_details = 20;
}

message GetSessionsResponse {
  repeated Session sessions = 1;
  string next_page_token = 2;
}

service AdminService {
  // Single endpoint replaces GetSession, DescribeSession, ListSessions
  rpc GetSessions(GetSessionsRequest) returns (GetSessionsResponse);
  rpc TerminateSession(TerminateSessionRequest) returns (TerminateSessionResponse);
}
```

### 2. Unify Config Operations

**Current**: ListConfigs, GetConfig, CreateConfig, UpdateConfig, DeleteConfig

**Simplified**:
```protobuf
service AdminService {
  // Read configs (supports filtering, pagination)
  rpc GetConfigs(GetConfigsRequest) returns (GetConfigsResponse);

  // Write config (upsert: create or update)
  rpc PutConfig(PutConfigRequest) returns (PutConfigResponse);

  // Delete config
  rpc DeleteConfig(DeleteConfigRequest) returns (DeleteConfigResponse);
}
```

### 3. Standardize Pagination

**Current**: Inconsistent pagination across endpoints

**Improved**:
```protobuf
// Standard pagination pattern for all list operations
message PaginationRequest {
  int32 page_size = 1 [
    (validate.rules).int32 = {
      gte: 1
      lte: 1000
    }
  ];
  string page_token = 2;
}

message PaginationResponse {
  string next_page_token = 1;
  int32 total_count = 2;      // Optional: total count for UI
}

// Apply to all list operations
message ListNamespacesRequest {
  PaginationRequest pagination = 1;
  // ... filters
}

message ListNamespacesResponse {
  repeated Namespace namespaces = 1;
  PaginationResponse pagination = 2;
}
```

## Long-Term Extensibility

### 1. Batch Operations

For automation and efficiency:
```protobuf
message BatchCreateNamespacesRequest {
  repeated CreateNamespaceRequest requests = 1 [
    (validate.rules).repeated = {
      min_items: 1
      max_items: 100
    }
  ];

  // Fail fast or continue on error?
  bool atomic = 2;  // If true, rollback all on any failure
}

message BatchCreateNamespacesResponse {
  repeated CreateNamespaceResponse responses = 1;
  repeated Error errors = 2;  // Errors for failed requests
}
```

### 2. Watch/Subscribe for Real-Time Updates

For UI and automation:
```protobuf
message WatchNamespacesRequest {
  // Filters
  string namespace_prefix = 1;
  repeated string tags = 2;

  // Watch from specific point
  string resource_version = 3;  // Resume from last seen version
}

message WatchNamespacesResponse {
  enum EventType {
    ADDED = 0;
    MODIFIED = 1;
    DELETED = 2;
  }

  EventType type = 1;
  Namespace namespace = 2;
  string resource_version = 3;  // For resuming watch
}

service AdminService {
  // Server streaming for real-time updates
  rpc WatchNamespaces(WatchNamespacesRequest) returns (stream WatchNamespacesResponse);
}
```

### 3. Query Language for Complex Filters

For advanced filtering:
```protobuf
message QueryRequest {
  // SQL-like or JMESPath query
  string query = 1 [
    (validate.rules).string = {
      max_len: 1000
    }
  ];

  // Example: "SELECT * FROM namespaces WHERE tags CONTAINS 'prod' AND created_at > '2025-01-01'"
  // Or: "namespaces[?tags.contains('prod') && created_at > '2025-01-01']"
}
```

## Implementation Priority

### Phase 1: Security Hardening (Week 1-2)
1. Add input validation with protoc-gen-validate
2. Implement resource-level authorization
3. Add audit log signing and tamper-evidence
4. Implement adaptive rate limiting

### Phase 2: Simplifications (Week 3)
5. Consolidate session and config operations
6. Standardize pagination across all endpoints

### Phase 3: Extensibility (Week 4-5)
7. Add API versioning support
8. Implement batch operations
9. Add watch/subscribe for real-time updates

### Phase 4: Advanced (Future)
10. Add request signing for critical operations
11. Implement query language for complex filters

## Conclusion

**Security Grade**: B+ (Good, with room for improvement)

**Key Wins**:
- Strong OIDC-based authentication
- Proper JWT validation
- Audit logging foundation
- Rate limiting baseline

**Must-Fix**:
- Add resource-level authorization
- Implement tamper-evident audit logging
- Add input validation
- Implement API versioning

**Nice-to-Have**:
- Request signing
- Batch operations
- Watch/Subscribe
- Query language

**Next Steps**:
1. Review this memo with team
2. Prioritize improvements
3. Create implementation ADRs for each phase
4. Update RFC-010 with accepted improvements