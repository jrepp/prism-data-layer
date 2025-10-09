---
title: "ADR-046: Dex IDP for Local Identity Testing"
status: Accepted
date: 2025-10-09
deciders: Platform Team
tags: [authentication, oidc, testing, local-development, dex]
---

## Context

Prism uses OIDC authentication for both the Admin API (RFC-010) and Data Proxy (RFC-011). During local development and testing, developers need:

1. **Local OIDC Provider**: Test authentication flows without external dependencies
2. **Multiple Identity Scenarios**: Simulate different users, groups, and permissions
3. **Fast Iteration**: No cloud setup or API keys required
4. **Realistic Testing**: Same OIDC flows as production
5. **CI/CD Integration**: Run authentication tests in GitHub Actions

**Current Problems**:
- Mocking OIDC flows doesn't test real JWT validation
- Using cloud providers (Auth0, Okta) requires API keys and network access
- Hard to test edge cases (expired tokens, invalid signatures, missing claims)
- Developers can't test authentication without cloud credentials

**Requirements**:
- Self-hosted OIDC provider for local development
- Supports standard OIDC flows (device code, authorization code, client credentials)
- Lightweight (can run in Docker Compose alongside Prism)
- Configurable users, groups, and scopes
- Compatible with Prism's JWT validation (RFC-010, RFC-011)

## Decision

We will use **Dex** as the local OIDC provider for development and testing.

**What is Dex?**
- Open-source federated OIDC provider by CoreOS (now part of CNCF)
- Lightweight (single Go binary, ~20MB Docker image)
- Supports multiple authentication connectors (static users, LDAP, SAML, GitHub, Google)
- Full OIDC 1.0 support (including device code flow for CLI testing)
- Kubernetes-native but works standalone

**Why Dex?**
1. **Self-Hosted**: No cloud dependencies, runs in Docker Compose
2. **OIDC Compliant**: Full spec support, works with standard libraries
3. **Flexible Configuration**: YAML-based config for users, groups, clients
4. **Well-Maintained**: Active CNCF project, used by Kubernetes ecosystem
5. **Fast**: Go-based, starts in &lt;1 second
6. **Documented**: Extensive docs and examples

**Alternatives Considered**:

| Provider | Pros | Cons | Verdict |
|----------|------|------|---------|
| **Dex** | Lightweight, OIDC compliant, self-hosted | Requires configuration | ✅ **Chosen** |
| **Keycloak** | Feature-rich, admin UI | Heavy (Java, 2GB RAM), slow startup | ❌ Too heavy for local dev |
| **mock-oidc** | Minimal, fast | Not full OIDC spec, less realistic | ❌ Insufficient fidelity |
| **Okta/Auth0** | Production-grade | Requires cloud account, API keys, slow | ❌ Not self-hosted |
| **Hydra (Ory)** | OAuth2 focused, cloud-native | More complex setup, overkill | ❌ Over-engineered |

## Implementation

### 1. Docker Compose Integration

Add Dex to local development stack:

```yaml
# docker-compose.yaml
services:
  dex:
    image: ghcr.io/dexidp/dex:v2.38.0
    ports:
      - "5556:5556"  # HTTP
      - "5557:5557"  # gRPC (optional)
    volumes:
      - ./local/dex/config.yaml:/etc/dex/config.yaml:ro
    command: ["serve", "/etc/dex/config.yaml"]
    networks:
      - prism-dev

  prism-proxy:
    image: prism/proxy:dev
    environment:
      PRISM_OIDC_ISSUER: http://dex:5556
      PRISM_OIDC_AUDIENCE: prismctl-api
      PRISM_OIDC_JWKS_URI: http://dex:5556/keys
    depends_on:
      - dex
    networks:
      - prism-dev
```

### 2. Dex Configuration

Create `local/dex/config.yaml`:

```yaml
issuer: http://localhost:5556

storage:
  type: memory  # Ephemeral for local dev

web:
  http: 0.0.0.0:5556

# Static users for testing
staticPasswords:
  - email: alice@prism.local
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"  # bcrypt("password")
    username: alice
    userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"

  - email: bob@prism.local
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: bob
    userID: "41331323-6f44-45e6-b3b9-2c4b2b6e2e4"

# OAuth2 clients
staticClients:
  # Prism Admin CLI
  - id: prismctl
    name: Prism Admin CLI
    secret: prismctl-secret
    redirectURIs:
      - http://localhost:8000/callback  # For web-based flows
      - http://127.0.0.1:8000/callback
    public: true  # Allow device code flow without secret

  # Prism Data Proxy
  - id: prism-proxy
    name: Prism Data Proxy
    secret: prism-proxy-secret
    redirectURIs:
      - http://localhost:8980/callback

# OIDC configuration
oauth2:
  skipApprovalScreen: true  # Auto-approve for local testing

# Enable device code flow for CLI testing
enablePasswordDB: true
```

### 3. Test Users and Groups

For testing RBAC scenarios:

```yaml
# local/dex/config.yaml (extended)
staticPasswords:
  # Admin user
  - email: admin@prism.local
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: admin
    userID: "admin-001"
    groups:
      - platform-team
      - admins

  # Operator user
  - email: operator@prism.local
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: operator
    userID: "operator-001"
    groups:
      - platform-team

  # Viewer user (read-only)
  - email: viewer@prism.local
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: viewer
    userID: "viewer-001"
    groups:
      - viewers
```

### 4. CLI Integration

Update `prismctl` to support Dex for local testing:

```bash
# Local development
prismctl login --issuer http://localhost:5556 --client-id prismctl

# Will open browser to:
# http://localhost:5556/auth?client_id=prismctl&...

# User logs in with:
# Email: admin@prism.local
# Password: password

# CLI receives token and caches to ~/.prism/token
```

### 5. Testing Integration

```go
// tests/integration/auth_test.go
func TestAdminAuthWithDex(t *testing.T) {
    // Start Dex in test mode
    dex := startDexServer(t)
    defer dex.Close()

    // Configure Prism to use Dex
    proxy := startProxyWithOIDC(t, ProxyConfig{
        OIDCIssuer:   dex.URL(),
        OIDCAudience: "prismctl-api",
    })
    defer proxy.Close()

    // Acquire token from Dex
    token := dex.AcquireToken(t, DexUser{
        Email:  "admin@prism.local",
        Groups: []string{"platform-team", "admins"},
    })

    // Call Admin API with token
    client := admin.NewClient(proxy.AdminURL(), token)
    namespaces, err := client.ListNamespaces(context.Background())
    require.NoError(t, err)
    assert.NotEmpty(t, namespaces)
}
```

### 6. JWT Claims Structure

Dex issues JWTs with this structure (matches RFC-010 expectations):

```json
{
  "iss": "http://localhost:5556",
  "sub": "admin-001",
  "aud": "prismctl-api",
  "exp": 1696867200,
  "iat": 1696863600,
  "email": "admin@prism.local",
  "email_verified": true,
  "groups": ["platform-team", "admins"],
  "name": "admin"
}
```

### 7. Development Workflow

```bash
# 1. Start local stack (includes Dex)
docker-compose up -d

# 2. Login with Dex
prismctl login --local  # Shorthand for --issuer http://localhost:5556

# Browser opens, login with:
# Email: admin@prism.local
# Password: password

# 3. Use Prism normally
prismctl namespace list
prismctl namespace create test-namespace

# 4. JWT validation happens locally against Dex
# No external network calls, no cloud dependencies
```

## Consequences

### Positive

1. ✅ **Zero External Dependencies**: Developers can test authentication without internet
2. ✅ **Fast Iteration**: Start Dex in &lt;1 second, test immediately
3. ✅ **Realistic Testing**: Full OIDC flows, not mocks
4. ✅ **Flexible Scenarios**: Easy to add test users with different permissions
5. ✅ **CI/CD Ready**: Dex runs in GitHub Actions, no secrets required
6. ✅ **Production Parity**: Same OIDC libraries used locally and in prod
7. ✅ **Multi-User Testing**: Simulate multiple users in integration tests
8. ✅ **Well-Documented**: Dex has extensive docs and examples

### Negative

1. ❌ **Extra Service**: One more container in Docker Compose
   - *Mitigation*: Dex is lightweight (20MB image, &lt;50MB RAM)
2. ❌ **Configuration Required**: Need to maintain `dex/config.yaml`
   - *Mitigation*: Provide sensible defaults, document patterns
3. ❌ **Learning Curve**: Developers must understand OIDC basics
   - *Mitigation*: Provide quick start guide, pre-configured users
4. ❌ **Static Users**: Local Dex uses static user database
   - *Mitigation*: Sufficient for testing, not meant for production

### Neutral

- **Not for Production**: Dex is for local/test only, production uses real IdP (Auth0/Okta/Azure AD)
- **Additional Docs**: Need to document Dex setup and test user credentials
- **Token Expiry**: Tokens expire after 1 hour (can be configured)

## Usage Examples

### Example 1: Admin API Testing

```bash
# Start stack with Dex
docker-compose up -d

# Login as admin
prismctl login --local
# Email: admin@prism.local
# Password: password

# Admin operations work
prismctl namespace create prod-analytics
# ✓ Success (admin has admin:write permission)
```

### Example 2: RBAC Testing

```bash
# Login as viewer
prismctl login --local
# Email: viewer@prism.local
# Password: password

# Viewer can list but not create
prismctl namespace list
# ✓ Success (viewer has admin:read permission)

prismctl namespace create test
# ✗ PermissionDenied: viewer lacks admin:write permission
```

### Example 3: Integration Tests

```go
func TestNamespaceRBAC(t *testing.T) {
    dex := startDexServer(t)
    proxy := startProxyWithOIDC(t, dex.URL())

    // Test admin can create
    adminToken := dex.AcquireToken(t, "admin@prism.local")
    adminClient := admin.NewClient(proxy.AdminURL(), adminToken)

    _, err := adminClient.CreateNamespace(ctx, "test")
    require.NoError(t, err)

    // Test viewer cannot create
    viewerToken := dex.AcquireToken(t, "viewer@prism.local")
    viewerClient := admin.NewClient(proxy.AdminURL(), viewerToken)

    _, err = viewerClient.CreateNamespace(ctx, "test2")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "PermissionDenied")
}
```

### Example 4: Data Proxy mTLS + OIDC

```bash
# Data proxy uses mTLS for clients, but services might use OIDC internally
# Example: Prism service-to-service authentication

# Service A gets token from Dex
export TOKEN=$(curl -X POST http://localhost:5556/token \
  -d grant_type=client_credentials \
  -d client_id=prism-proxy \
  -d client_secret=prism-proxy-secret)

# Service A calls Prism proxy with token
grpcurl -H "Authorization: Bearer $TOKEN" \
  localhost:8980 prism.data.v1.DataService/Get
```

## Migration Path

### Phase 1: Local Development (Immediate)
1. Add Dex to `docker-compose.yaml`
2. Create `local/dex/config.yaml` with test users
3. Update `prismctl` to support `--local` flag
4. Document quick start guide

### Phase 2: Integration Tests (1-2 weeks)
1. Create Dex test helper library
2. Update integration tests to use Dex
3. Remove mock OIDC code
4. Add CI/CD Dex setup

### Phase 3: Documentation (Ongoing)
1. Add "Local Authentication" section to docs
2. Document test users and their permissions
3. Provide troubleshooting guide
4. Create video walkthrough

## Documentation Requirements

1. **Quick Start Guide**: `docs/local-development/authentication.md`
   - How to start Dex
   - Default test users
   - Login flow walkthrough

2. **Test User Reference**: `docs/local-development/test-users.md`
   - User credentials
   - Group memberships
   - Permission matrix

3. **Integration Test Examples**: `tests/integration/README.md`
   - Using Dex in tests
   - Custom test users
   - Token acquisition patterns

4. **Troubleshooting**: `docs/troubleshooting/dex.md`
   - Common Dex errors
   - Token validation issues
   - Browser not opening

## References

- [Dex Official Documentation](https://dexidp.io/docs/)
- [Dex GitHub Repository](https://github.com/dexidp/dex)
- [OIDC Device Code Flow](https://datatracker.ietf.org/doc/html/rfc8628)
- RFC-010: Admin Protocol with OIDC Authentication
- RFC-011: Data Proxy Authentication
- ADR-007: Authentication and Authorization

## Open Questions

1. **Production Connector**: Should Dex connect to production IdP (Azure AD/Okta) for staging environment?
   - **Leaning**: No, use real IdP for staging. Dex only for local/test.

2. **Connector Support**: Should we configure Dex to support GitHub/Google login for convenience?
   - **Leaning**: Not initially. Static users sufficient for testing.

3. **Token Caching**: How long should Dex tokens be cached in `~/.prism/token`?
   - **Leaning**: Match RFC-010 recommendation (24 hours, with refresh token support)

4. **Multi-Tenancy**: Should Dex support multiple tenants for testing namespace isolation?
   - **Leaning**: Use groups for RBAC testing, not multi-tenant Dex setup

5. **Performance Testing**: Can Dex handle high-volume token issuance for load tests?
   - **Leaning**: For load testing, use production-grade IdP or mock. Dex for functional tests only.

## Revision History

- 2025-10-09: Initial ADR proposing Dex for local OIDC testing
