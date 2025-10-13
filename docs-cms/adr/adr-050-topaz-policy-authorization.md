---
date: 2025-10-09
deciders: Core Team
doc_uuid: 5077b7fe-3350-4dc8-837b-17fa706c0798
id: adr-050
project_id: prism-data-layer
status: Accepted
tags:
- authorization
- security
- policy
- topaz
- openpolicyagent
- rbac
- abac
title: Topaz for Policy-Based Authorization
---

# ADR-050: Topaz for Policy-Based Authorization

## Status

Accepted

## Context

Prism requires fine-grained authorization beyond simple OIDC authentication. We need:

1. **Multi-tenancy isolation**: Users can only access their namespace's data
2. **Role-based access control (RBAC)**: Different permissions for developers, operators, admins
3. **Attribute-based access control (ABAC)**: Context-aware policies (time of day, IP address, data sensitivity)
4. **Resource-level policies**: Per-namespace, per-backend, per-pattern permissions
5. **Audit trail**: Who accessed what, when, and why

**Design Constraints**:
- Authorization decisions must be fast (&lt;5ms P99)
- Policies must be centrally managed and versioned
- Must integrate with existing OIDC authentication (RFC-010, RFC-011)
- Should support both synchronous (proxy) and asynchronous (admin) authorization
- Need local policy enforcement for low-latency decisions

## Decision

We will use **Topaz** by Aserto as our policy engine for authorization decisions.

**What is Topaz?**
- Open-source authorization engine based on Open Policy Agent (OPA)
- Built-in directory service for storing user/group/resource relationships
- Supports fine-grained, real-time authorization (FGA)
- Sidecar deployment model for low-latency local decisions
- Centralized policy management with decentralized enforcement

## Rationale

### Alternatives Considered

#### Alternative A: Open Policy Agent (OPA) Alone

**Pros**:
- Industry standard policy engine
- Flexible Rego policy language
- Wide adoption and ecosystem

**Cons**:
- ❌ No built-in directory service (need separate user/resource store)
- ❌ No relationship/graph modeling (need to implement ourselves)
- ❌ Limited real-time updates (bundle-based refresh only)
- ❌ No built-in audit logging

**Verdict**: Too much plumbing required. We'd essentially rebuild Topaz.

#### Alternative B: Cloud Provider IAM (AWS IAM, Google Cloud IAM)

**Pros**:
- Integrated with cloud infrastructure
- No additional infrastructure to manage

**Cons**:
- ❌ Cloud-specific (not portable)
- ❌ Coarse-grained (resource-level, not fine-grained)
- ❌ High latency (API calls to cloud control plane)
- ❌ No support for on-premises deployments

**Verdict**: Doesn't meet latency or portability requirements.

#### Alternative C: Zanzibar-based Systems (SpiceDB, Ory Keto)

**Pros**:
- Google Zanzibar-inspired relationship-based access control
- Fine-grained permissions with relationships
- High performance with caching

**Cons**:
- ⚠️ More complex to set up than Topaz
- ⚠️ SpiceDB requires separate deployment (not sidecar)
- ⚠️ Ory Keto still maturing

**Verdict**: Good alternative, but Topaz provides simpler integration.

#### Alternative D: Topaz (Selected)

**Pros**:
- ✅ Built on OPA (reuses Rego policy language)
- ✅ Includes directory service (users, groups, resources)
- ✅ Sidecar deployment for &lt;1ms authorization checks
- ✅ Relationship-based authorization (like Zanzibar)
- ✅ Real-time policy updates (no bundle delays)
- ✅ Built-in audit logging
- ✅ Open source with commercial support option (Aserto)
- ✅ gRPC and REST APIs for integration
- ✅ Works on-premises and in cloud

**Cons**:
- ⚠️ Smaller ecosystem than OPA
- ⚠️ Requires learning Aserto-specific concepts (manifests, directory API)

**Verdict**: ✅ **Best fit** - combines OPA's power with Zanzibar-style relationships and local enforcement.

## Architecture

### Deployment Model: Sidecar Pattern

```text
┌────────────────────────────────────────────┐
│            Prism Proxy (Rust)              │
│                                            │
│  ┌──────────────────────────────────────┐ │
│  │   gRPC Request Handler               │ │
│  └──────────┬───────────────────────────┘ │
│             │                              │
│             │ 1. Authorize(user, resource) │
│             ▼                              │
│  ┌──────────────────────────────────────┐ │
│  │   Topaz Sidecar (localhost:8282)     │ │
│  │   - Policy Engine (Rego)             │ │
│  │   - Directory Service (users/groups) │ │
│  │   - Decision Cache (local)           │ │
│  └──────────┬───────────────────────────┘ │
└─────────────┼───────────────────────────────┘
              │
              │ 2. Policy sync (background)
              ▼
   ┌──────────────────────────────┐
   │  Central Policy Repository   │
   │  (Git + Aserto Control Plane)│
   └──────────────────────────────┘
```

**Flow**:
1. Prism proxy receives gRPC request
2. Proxy calls Topaz sidecar: `Is(user, "can-read", namespace)`
3. Topaz evaluates policy locally (&lt;1ms)
4. Topaz returns `{ allowed: true, reasons: [...] }`
5. Proxy enforces decision (allow or deny request)

**Policy updates**:
- Policies stored in Git (as Rego files)
- Topaz syncs policies every 30s from central control plane
- No proxy restarts required for policy changes

### Authorization Model: Relationship-Based Access Control

**Inspired by Google Zanzibar**, Topaz models authorization as relationships between subjects (users), objects (resources), and permissions.

**Example Relationships**:

```text
# User alice is a member of team platform-engineering
alice | member | group:platform-engineering

# Group platform-engineering is an admin of namespace iot-devices
group:platform-engineering | admin | namespace:iot-devices

# Namespace iot-devices contains backend redis-001
namespace:iot-devices | contains | backend:redis-001

# Policy: Admins of a namespace can read/write its backends
allow(user, "read", backend) if
  user | admin | namespace
  namespace | contains | backend
```

**Result**: Alice can read backend `redis-001` because:
1. Alice ∈ platform-engineering (member)
2. platform-engineering → admin → iot-devices
3. iot-devices → contains → redis-001
4. Policy: admin → can read contained backends

### Integration Points

#### 1. Prism Proxy (Rust)

**Authorization Middleware**:

```rust
// src/authz/topaz.rs
use tonic::Request;
use anyhow::Result;

pub struct TopazAuthz {
    client: TopazClient,
}

impl TopazAuthz {
    pub async fn authorize(&self, req: &AuthzRequest) -> Result<bool> {
        let decision = self.client.is(IsRequest {
            subject: req.user,
            relation: req.permission,  // "read", "write", "admin"
            object: req.resource,      // "namespace:iot-devices"
        }).await?;

        if decision.is {
            info!("Authorized: {} can {} {}", req.user, req.permission, req.resource);
            Ok(true)
        } else {
            warn!("Denied: {} cannot {} {}", req.user, req.permission, req.resource);
            Ok(false)
        }
    }
}

// Middleware applied to all gRPC requests
pub async fn authz_middleware(
    req: Request<()>,
    authz: &TopazAuthz,
) -> Result<Request<()>, Status> {
    let metadata = req.metadata();
    let user = metadata.get("x-user-id")
        .ok_or_else(|| Status::unauthenticated("Missing user ID"))?;

    let resource = extract_resource_from_request(&req)?;
    let permission = infer_permission_from_method(&req)?;

    let allowed = authz.authorize(&AuthzRequest {
        user: user.to_str()?,
        permission,
        resource,
    }).await?;

    if allowed {
        Ok(req)
    } else {
        Err(Status::permission_denied("Access denied by policy"))
    }
}
```

#### 2. Admin CLI (Python)

**Authorization Check Before Commands**:

```python
# cli/prismctl/authz.py
import asyncio
from topaz_grpc import Topaz ClientStub

class TopazClient:
    def __init__(self, endpoint="localhost:8282"):
        self.client = TopazClientStub(endpoint)

    async def can_user(self, user: str, permission: str, resource: str) -> bool:
        """Check if user has permission on resource."""
        response = await self.client.is_request(
            subject=user,
            relation=permission,
            object=resource
        )
        return response.is_allowed

# Usage in CLI commands
@click.command()
@click.argument('namespace')
async def delete_namespace(namespace: str):
    """Delete a namespace (requires admin permission)."""
    user = get_current_user()

    # Check authorization before dangerous operation
    authz = TopazClient()
    if not await authz.can_user(user, "admin", f"namespace:{namespace}"):
        click.echo(f"❌ Access denied: You don't have admin permission on {namespace}")
        sys.exit(1)

    # Proceed with deletion
    click.echo(f"Deleting namespace {namespace}...")
    # ... deletion logic
```

#### 3. Admin UI (FastAPI)

**Protect API Endpoints**:

```python
# admin/app/authz.py
from fastapi import Depends, HTTPException, status
from topaz import TopazClient

authz = TopazClient(endpoint="localhost:8282")

async def require_permission(
    permission: str,
    resource_type: str
):
    """FastAPI dependency for authorization."""
    async def check_permission(
        resource_id: str,
        current_user: str = Depends(get_current_user)
    ):
        resource = f"{resource_type}:{resource_id}"

        allowed = await authz.can_user(
            user=current_user,
            permission=permission,
            resource=resource
        )

        if not allowed:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail=f"You don't have '{permission}' permission on {resource}"
            )

    return check_permission

# Usage
@app.delete("/api/namespaces/{namespace_id}")
async def delete_namespace(
    namespace_id: str,
    _: None = Depends(require_permission("admin", "namespace"))
):
    """Delete namespace (requires admin permission)."""
    # ... deletion logic
```

### Policy Examples

#### Policy 1: Namespace Isolation (Multi-Tenancy)

```rego
# policies/namespace_isolation.rego
package prism.authz

# Default deny
default allow = false

# Users can only access namespaces they have explicit permission for
allow {
    input.permission == "read"
    input.resource_type == "namespace"
    has_namespace_access(input.user, input.resource_id)
}

allow {
    input.permission == "write"
    input.resource_type == "namespace"
    has_namespace_write_access(input.user, input.resource_id)
}

has_namespace_access(user, namespace) {
    # Check if user is member of group with access
    user_groups := data.directory.user_groups[user]
    group := user_groups[_]
    data.directory.group_namespaces[group][namespace]
}

has_namespace_write_access(user, namespace) {
    # Only admins can write
    is_namespace_admin(user, namespace)
}

is_namespace_admin(user, namespace) {
    user_groups := data.directory.user_groups[user]
    group := user_groups[_]
    data.directory.group_roles[group][namespace] == "admin"
}
```

#### Policy 2: Time-Based Access (Maintenance Windows)

```rego
# policies/maintenance_windows.rego
package prism.authz

# Allow writes only during maintenance window
allow {
    input.permission == "write"
    input.resource_type == "backend"
    is_maintenance_window()
}

is_maintenance_window() {
    # Maintenance: Sundays 02:00-06:00 UTC
    now := time.now_ns()
    day_of_week := time.weekday(now)
    hour := time.clock(now)[0]

    day_of_week == "Sunday"
    hour >= 2
    hour < 6
}
```

#### Policy 3: Data Sensitivity (PII Protection)

```rego
# policies/pii_protection.rego
package prism.authz

# PII data can only be accessed by users with pii-access role
allow {
    input.permission == "read"
    contains_pii(input.resource_id)
    user_has_pii_access(input.user)
}

contains_pii(resource) {
    # Check if resource is marked as containing PII
    data.directory.resource_attributes[resource].sensitivity == "pii"
}

user_has_pii_access(user) {
    user_roles := data.directory.user_roles[user]
    user_roles[_] == "pii-access"
}
```

## Directory Schema

**Topaz Directory Models Users, Groups, Resources, and Relationships**:

```yaml
# topaz/directory/schema.yaml
model:
  version: 3

  types:
    # Subjects
    user:
      relations:
        member: group

    group:
      relations:
        admin: namespace
        developer: namespace
        viewer: namespace

    # Resources
    namespace:
      relations:
        contains: backend
        contains: pattern

    backend:
      relations:
        exposed_by: namespace

    pattern:
      relations:
        used_by: namespace

  permissions:
    # Namespace permissions
    namespace:
      read:
        - viewer
        - developer
        - admin
      write:
        - developer
        - admin
      admin:
        - admin

    # Backend permissions
    backend:
      read:
        - admin@namespace[exposed_by]
        - developer@namespace[exposed_by]
      write:
        - admin@namespace[exposed_by]
      admin:
        - admin@namespace[exposed_by]
```

**Populating the Directory**:

```bash
# Add users
topaz directory set user alice@example.com

# Add groups
topaz directory set group platform-engineering

# Add user to group
topaz directory set relation alice@example.com member group:platform-engineering

# Add namespace
topaz directory set namespace iot-devices

# Grant group admin access to namespace
topaz directory set relation group:platform-engineering admin namespace:iot-devices

# Add backend
topaz directory set backend redis-001

# Link backend to namespace
topaz directory set relation namespace:iot-devices contains backend:redis-001
```

## Performance Characteristics

### Latency

**Local sidecar authorization checks**:
- P50: &lt;0.5ms
- P95: &lt;2ms
- P99: &lt;5ms

**Why so fast?**
- Topaz sidecar runs locally (no network round-trip to remote authz service)
- Decisions cached in-memory
- Policy compiled ahead of time (not interpreted)

**Comparison**:
- Remote authz service (e.g., AWS IAM): 50-200ms
- Database lookup: 10-50ms
- Topaz local: &lt;5ms

### Throughput

**Topaz sidecar can handle**:
- 10,000+ authorization checks per second (local)
- Limited only by proxy throughput (not authz)

## Deployment

### Local Development

**Docker Compose**:

```yaml
# docker-compose.yml
services:
  topaz:
    image: ghcr.io/aserto-dev/topaz:latest
    ports:
      - "8282:8282"  # gRPC API
      - "8383:8383"  # REST API
      - "8484:8484"  # Console UI
    volumes:
      - ./topaz/config:/config
      - ./topaz/policies:/policies
    environment:
      - TOPAZ_DB_PATH=/data/topaz.db
      - TOPAZ_POLICY_ROOT=/policies
    command: run -c /config/topaz-config.yaml

  prism-proxy:
    build: ./proxy
    depends_on:
      - topaz
    environment:
      - TOPAZ_ENDPOINT=topaz:8282
    ports:
      - "50051:50051"
```

**Configuration** (`topaz/config/topaz-config.yaml`):

```yaml
# Topaz configuration
version: 2

api:
  grpc:
    listen_address: "0.0.0.0:8282"
  rest:
    listen_address: "0.0.0.0:8383"

directory:
  db:
    type: sqlite
    path: /data/topaz.db

policy:
  engine: opa
  policy_root: /policies

edge:
  enabled: true
  sync_interval: 30s
  remote: https://topaz.aserto.com  # Central policy repo
```

### Production Deployment

**Kubernetes Sidecar**:

```yaml
# k8s/prism-proxy-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prism-proxy
spec:
  template:
    spec:
      containers:
        # Main proxy container
        - name: proxy
          image: prism-proxy:latest
          env:
            - name: TOPAZ_ENDPOINT
              value: "localhost:8282"

        # Topaz sidecar
        - name: topaz
          image: ghcr.io/aserto-dev/topaz:latest
          ports:
            - containerPort: 8282
              name: grpc
          volumeMounts:
            - name: topaz-config
              mountPath: /config
            - name: topaz-policies
              mountPath: /policies

      volumes:
        - name: topaz-config
          configMap:
            name: topaz-config
        - name: topaz-policies
          configMap:
            name: topaz-policies
```

## Security Considerations

### 1. Policy Isolation

**Risk**: Malicious user modifies policies to grant themselves unauthorized access.

**Mitigation**:
- Policies stored in Git with branch protection
- Only CI/CD can push policy changes to Topaz
- Audit all policy changes in Git history

### 2. Directory Integrity

**Risk**: Unauthorized modification of user/group/resource relationships.

**Mitigation**:
- Directory API requires authentication (admin token)
- All directory changes logged to audit trail
- Periodic snapshots for disaster recovery

### 3. Sidecar Compromise

**Risk**: Attacker gains access to Topaz sidecar and bypasses authorization.

**Mitigation**:
- Topaz sidecar bound to localhost only (not exposed externally)
- Proxy and sidecar run in same pod/VM (network isolation)
- mTLS between proxy and sidecar (optional, for paranoid mode)

### 4. Denial of Service

**Risk**: Flood of authorization checks overwhelms Topaz sidecar.

**Mitigation**:
- Rate limiting in proxy before authz checks
- Circuit breaker pattern (fail open/closed configurable)
- Horizontal scaling of proxy+sidecar pairs

## Migration Path

### Phase 1: Basic RBAC (Week 1)

1. Deploy Topaz sidecar alongside Prism proxy
2. Implement simple RBAC policies (admin/developer/viewer roles)
3. Integrate with existing OIDC authentication
4. Test authorization checks in local development

### Phase 2: Namespace Isolation (Week 2)

1. Model namespaces in Topaz directory
2. Implement namespace isolation policies
3. Migrate existing namespace ACLs to Topaz
4. Validate multi-tenancy enforcement

### Phase 3: Fine-Grained Permissions (Week 3)

1. Model backends and patterns in directory
2. Implement resource-level policies
3. Add attribute-based policies (time, IP, data sensitivity)
4. Enable audit logging

### Phase 4: Production Rollout (Week 4)

1. Deploy to staging environment
2. Load test authorization performance
3. Gradual rollout to production (canary deployment)
4. Monitor authorization latency and error rates

## Monitoring and Observability

### Metrics

**Authorization Decision Metrics**:
- `prism_authz_decisions_total{decision="allowed|denied"}` - Total authorization checks
- `prism_authz_latency_seconds` - Authorization check latency histogram
- `prism_authz_errors_total` - Failed authorization checks
- `prism_authz_cache_hit_ratio` - Decision cache hit rate

**Policy Evaluation Metrics**:
- `topaz_policy_evaluations_total` - Policy evaluation count
- `topaz_policy_errors_total` - Policy evaluation errors
- `topaz_directory_queries_total` - Directory lookups

### Logging

**Authorization Audit Trail**:

```json
{
  "timestamp": "2025-10-09T14:32:15Z",
  "event": "authorization_decision",
  "user": "alice@example.com",
  "permission": "read",
  "resource": "namespace:iot-devices",
  "decision": "allowed",
  "reasons": [
    "user is member of group platform-engineering",
    "platform-engineering has developer role on iot-devices"
  ],
  "latency_ms": 1.2
}
```

### Alerts

**Authorization Failures**:
- Alert if authorization error rate &gt; 1%
- Alert if authorization latency P99 &gt; 10ms
- Alert if policy sync fails

**Unusual Access Patterns**:
- Alert if user accesses namespace they've never accessed before
- Alert if admin actions outside maintenance window
- Alert if PII data accessed by non-authorized user

## Open Questions

### 1. Fail-Open vs Fail-Closed?

**Question**: If Topaz sidecar is unavailable, should proxy allow or deny requests?

**Options**:
- **Fail-closed** (deny all): More secure, but impacts availability
- **Fail-open** (allow all): Better availability, but security risk
- **Degraded mode** (allow read-only): Compromise between security and availability

**Recommendation**: **Fail-closed by default**, with opt-in fail-open per namespace.

### 2. How to Handle Policy Conflicts?

**Question**: What happens if multiple policies conflict (one allows, one denies)?

**Options**:
- **Deny wins**: Conservative approach (deny if any policy denies)
- **Allow wins**: Permissive approach (allow if any policy allows)
- **Explicit priority**: Policies have precedence order

**Recommendation**: **Deny wins** (secure by default).

### 3. Should We Cache Authorization Decisions?

**Question**: Can we cache authorization decisions to reduce Topaz load?

**Pros**:
- Reduces latency for repeated checks
- Reduces load on Topaz sidecar

**Cons**:
- Stale decisions if policies/relationships change
- Cache invalidation complexity

**Recommendation**: **Yes, with short TTL** (5 seconds). Trade-off between performance and freshness.

## Related Documents

- [RFC-010: Admin Protocol with OIDC](/rfc/rfc-010) - OIDC authentication
- [RFC-011: Data Proxy Authentication](/rfc/rfc-011) - Secrets provider abstraction
- [ADR-046: Dex IDP for Local Testing](/adr/adr-046) - Local OIDC provider

## Consequences

### Positive

- ✅ **Fine-grained authorization**: Per-resource, per-user, attribute-based policies
- ✅ **Low latency**: &lt;5ms P99 authorization checks (local sidecar)
- ✅ **Centralized policy management**: Git-based policy versioning and deployment
- ✅ **Audit trail**: Complete history of authorization decisions
- ✅ **Relationship-based**: Natural modeling of user/group/resource relationships
- ✅ **Open source**: Can self-host, no vendor lock-in

### Negative

- ❌ **Additional component**: Topaz sidecar must run alongside proxy
- ❌ **Learning curve**: Team must learn Rego policy language and Topaz concepts
- ❌ **Operational complexity**: Policies and directory must be kept in sync
- ⚠️ **Single point of failure**: If sidecar fails, authorization fails (mitigate with fail-open)

### Neutral

- ⚠️ **Policy language**: Rego is powerful but unfamiliar to most developers
- ⚠️ **Directory management**: Need process for onboarding users/groups/resources
- ⚠️ **Testing policies**: Requires OPA testing framework for policy unit tests

## Revision History

- 2025-10-09: Initial ADR proposing Topaz for policy-based authorization