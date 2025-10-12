---
author: Platform Team
created: 2025-10-09
id: memo-topaz-authorizer
status: Active
tags:
- topaz
- authorization
- development
- testing
- local-infrastructure
title: 'MEMO-009: Topaz Local Authorizer Configuration for Development and Integration
  Testing'
updated: 2025-10-11
---

# MEMO-009: Topaz Local Authorizer Configuration

## Purpose

This memo documents how to configure Topaz as a local authorizer for two critical scenarios:

1. **Development Iteration**: Fast, lightweight authorization during local development
2. **Integration Testing**: Realistic authorization testing in CI/CD pipelines

Topaz is part of Prism's **local infrastructure layer** - reusable components that provide production-like services without external dependencies. This follows our [local-first testing philosophy](/adr/adr-004-local-testing-strategy).

## Overview

**Topaz by Aserto** provides local authorization enforcement with:
- Embedded directory service (users, groups, resources)
- Policy engine (OPA/Rego)
- gRPC and REST APIs
- In-memory caching for &lt;1ms decisions

**Key Insight**: Topaz runs as a **local sidecar** - no cloud dependencies, no network latency, fully reproducible.

```text
┌──────────────────────────────────────────────────────────────┐
│                  Local Development Stack                     │
│                                                              │
│  ┌────────────┐   ┌────────────┐   ┌────────────┐          │
│  │ Prism      │──▶│ Topaz      │   │ Dex        │          │
│  │ Proxy      │   │ (authz)    │   │ (authn)    │          │
│  │ :50051     │   │ :8282      │   │ :5556      │          │
│  └────────────┘   └────────────┘   └────────────┘          │
│         │                │                  │               │
│         │                │                  │               │
│         └────────────────┴──────────────────┘               │
│                 All on localhost                            │
└──────────────────────────────────────────────────────────────┘
```

## Local Infrastructure Layer

Topaz is one component of the **local infrastructure layer**:

| Component | Purpose | Port | Status |
|-----------|---------|------|--------|
| **Topaz** | Authorization (policy engine) | 8282 | This memo |
| **Dex** | Authentication (OIDC provider) | 5556 | [ADR-046](/adr/adr-046-dex-idp-local-testing) |
| **Vault** | Secret management | 8200 | [RFC-016](/rfc/rfc-016-local-development-infrastructure) |
| **Signoz** | Observability | 3301 | [ADR-048](/adr/adr-048-local-signoz-observability) |

**Design principle**: Each component can run **independently** or as part of a composed stack.

## Scenario 1: Development Iteration

### Requirements

**For local development, we need**:
- Fast startup (&lt;1 second)
- No external dependencies
- Simple user/group setup
- Policy hot-reload (no restart)
- Clear error messages

### Docker Compose Configuration

```yaml
# docker-compose.local.yml
version: '3.8'

services:
  topaz:
    image: ghcr.io/aserto-dev/topaz:0.30.14
    container_name: prism-topaz-local
    ports:
      - "8282:8282"  # gRPC API (authorization)
      - "8383:8383"  # REST API (directory management)
      - "8484:8484"  # Console UI (http://localhost:8484)
    volumes:
      - ./topaz/config.local.yaml:/config/topaz-config.yaml:ro
      - ./topaz/policies:/policies:ro
      - ./topaz/data:/data
    environment:
      - TOPAZ_DB_PATH=/data/topaz.db
      - TOPAZ_POLICY_ROOT=/policies
      - TOPAZ_LOG_LEVEL=info
    command: run -c /config/topaz-config.yaml
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8383/health"]
      interval: 5s
      timeout: 3s
      retries: 3

  # Optional: Proxy that uses Topaz
  prism-proxy:
    build: ./proxy
    container_name: prism-proxy-local
    depends_on:
      topaz:
        condition: service_healthy
    environment:
      - TOPAZ_ENDPOINT=topaz:8282
      - TOPAZ_ENABLED=true
      - TOPAZ_FAIL_OPEN=true  # Allow requests if Topaz unavailable (dev mode)
    ports:
      - "50051:50051"
```

### Configuration File

**`topaz/config.local.yaml`**:

```yaml
# Topaz configuration for local development
version: 2

# Logging
logger:
  prod: false
  log_level: info

# API configuration
api:
  grpc:
    listen_address: "0.0.0.0:8282"
    connection_timeout: 5s
  rest:
    listen_address: "0.0.0.0:8383"
  gateway:
    listen_address: "0.0.0.0:8484"
    http: true
    read_timeout: 5s
    write_timeout: 5s

# Directory configuration (embedded)
directory:
  db:
    type: sqlite
    path: /data/topaz.db
  seed_metadata: true

# Policy engine configuration
policy:
  engine: opa
  policy_root: /policies

# Edge configuration (sync with remote - disabled for local)
edge:
  enabled: false  # No cloud sync in local dev

# Decision logging (for debugging)
decision_logger:
  type: self
  config:
    store_directory: /data/decisions

# Authorization configuration
authorizer:
  grpc:
    connection_timeout: 5s
  needs:
    - kind: policy
    - kind: directory
```

### Seed Data Setup

**Topaz Directory Initialization** - `topaz/seed/bootstrap.sh`:

```bash
#!/usr/bin/env bash
# Bootstrap Topaz directory with development users and permissions

set -euo pipefail

TOPAZ_REST="http://localhost:8383"

echo "🔐 Bootstrapping Topaz directory..."

# Wait for Topaz to be ready
until curl -s "$TOPAZ_REST/health" > /dev/null; do
    echo "Waiting for Topaz..."
    sleep 1
done

echo "✅ Topaz is ready"

# Create users
echo "👤 Creating users..."
curl -X POST "$TOPAZ_REST/api/v2/directory/objects" \
    -H "Content-Type: application/json" \
    -d '{
        "object": {
            "type": "user",
            "id": "dev@local.prism",
            "display_name": "Local Developer",
            "properties": {
                "email": "dev@local.prism",
                "roles": ["developer"]
            }
        }
    }'

curl -X POST "$TOPAZ_REST/api/v2/directory/objects" \
    -H "Content-Type: application/json" \
    -d '{
        "object": {
            "type": "user",
            "id": "admin@local.prism",
            "display_name": "Local Admin",
            "properties": {
                "email": "admin@local.prism",
                "roles": ["admin"]
            }
        }
    }'

# Create groups
echo "👥 Creating groups..."
curl -X POST "$TOPAZ_REST/api/v2/directory/objects" \
    -H "Content-Type: application/json" \
    -d '{
        "object": {
            "type": "group",
            "id": "developers",
            "display_name": "Developers"
        }
    }'

curl -X POST "$TOPAZ_REST/api/v2/directory/objects" \
    -H "Content-Type: application/json" \
    -d '{
        "object": {
            "type": "group",
            "id": "admins",
            "display_name": "Administrators"
        }
    }'

# Add users to groups
echo "🔗 Creating group memberships..."
curl -X POST "$TOPAZ_REST/api/v2/directory/relations" \
    -H "Content-Type: application/json" \
    -d '{
        "relation": {
            "object_type": "group",
            "object_id": "developers",
            "relation": "member",
            "subject_type": "user",
            "subject_id": "dev@local.prism"
        }
    }'

curl -X POST "$TOPAZ_REST/api/v2/directory/relations" \
    -H "Content-Type: application/json" \
    -d '{
        "relation": {
            "object_type": "group",
            "object_id": "admins",
            "relation": "member",
            "subject_type": "user",
            "subject_id": "admin@local.prism"
        }
    }'

# Create namespaces
echo "📦 Creating namespaces..."
curl -X POST "$TOPAZ_REST/api/v2/directory/objects" \
    -H "Content-Type: application/json" \
    -d '{
        "object": {
            "type": "namespace",
            "id": "dev-playground",
            "display_name": "Developer Playground",
            "properties": {
                "description": "Sandbox for local development"
            }
        }
    }'

curl -X POST "$TOPAZ_REST/api/v2/directory/objects" \
    -H "Content-Type: application/json" \
    -d '{
        "object": {
            "type": "namespace",
            "id": "test-namespace",
            "display_name": "Test Namespace",
            "properties": {
                "description": "For integration tests"
            }
        }
    }'

# Grant permissions
echo "🔑 Granting permissions..."
# Developers → dev-playground
curl -X POST "$TOPAZ_REST/api/v2/directory/relations" \
    -H "Content-Type: application/json" \
    -d '{
        "relation": {
            "object_type": "namespace",
            "object_id": "dev-playground",
            "relation": "developer",
            "subject_type": "group",
            "subject_id": "developers"
        }
    }'

# Admins → all namespaces
curl -X POST "$TOPAZ_REST/api/v2/directory/relations" \
    -H "Content-Type: application/json" \
    -d '{
        "relation": {
            "object_type": "namespace",
            "object_id": "dev-playground",
            "relation": "admin",
            "subject_type": "group",
            "subject_id": "admins"
        }
    }'

curl -X POST "$TOPAZ_REST/api/v2/directory/relations" \
    -H "Content-Type: application/json" \
    -d '{
        "relation": {
            "object_type": "namespace",
            "object_id": "test-namespace",
            "relation": "admin",
            "subject_type": "group",
            "subject_id": "admins"
        }
    }'

echo "✅ Topaz directory bootstrapped successfully!"
echo ""
echo "Test users created:"
echo "  - dev@local.prism (developer role)"
echo "  - admin@local.prism (admin role)"
echo ""
echo "Test namespaces created:"
echo "  - dev-playground (developers can access)"
echo "  - test-namespace (admins can access)"
```

### Policy Files

**`topaz/policies/prism.rego`** - Main authorization policy:

```rego
package prism.authz

import future.keywords.contains
import future.keywords.if
import future.keywords.in

# Default deny
default allow = false

# Allow if user has permission via direct relationship
allow if {
    input.permission in ["read", "write", "admin"]
    has_permission(input.user, input.permission, input.resource)
}

# Check if user has permission on resource
has_permission(user, permission, resource) if {
    # Parse resource (format: "namespace:dev-playground")
    [resource_type, resource_id] := split(resource, ":")

    # Query directory for user's permissions
    user_permissions := directory_check(user, resource_type, resource_id)

    # Check if permission is granted
    permission in user_permissions
}

# Helper: Query Topaz directory for user permissions
directory_check(user, resource_type, resource_id) = permissions if {
    # Get user's groups
    user_groups := data.directory.user_groups[user]

    # Collect all permissions from groups
    permissions := {p |
        some group in user_groups
        relation := data.directory.relations[group][resource_type][resource_id]
        p := permission_from_relation(relation)
    }
}

# Map relationship to permission
permission_from_relation("viewer") = "read"
permission_from_relation("developer") = "read"
permission_from_relation("developer") = "write"
permission_from_relation("admin") = "read"
permission_from_relation("admin") = "write"
permission_from_relation("admin") = "admin"

# Development mode: Allow all if explicitly enabled
allow if {
    input.mode == "development"
    input.allow_all == true
}
```

**`topaz/policies/namespace_isolation.rego`** - Multi-tenancy enforcement:

```rego
package prism.authz.namespace

import future.keywords.if

# Namespace isolation: Users can only access namespaces they have explicit access to
violation[msg] if {
    input.resource_type == "namespace"
    not has_namespace_access(input.user, input.resource_id)
    msg := sprintf("User %v does not have access to namespace %v", [input.user, input.resource_id])
}

# Check if user has access to namespace (via group membership)
has_namespace_access(user, namespace_id) if {
    user_groups := data.directory.user_groups[user]
    some group in user_groups
    group_namespaces := data.directory.group_namespaces[group]
    namespace_id in group_namespaces
}
```

### Developer Workflow

**Starting Topaz locally**:

```bash
# Start Topaz
docker compose -f docker-compose.local.yml up -d topaz

# Wait for startup
docker compose -f docker-compose.local.yml logs -f topaz

# Bootstrap directory
bash topaz/seed/bootstrap.sh

# Verify setup
curl http://localhost:8383/api/v2/directory/objects?object_type=user | jq .

# Open console UI
open http://localhost:8484
```

**Testing authorization from command line**:

```bash
# Check if dev@local.prism can read dev-playground
curl -X POST http://localhost:8282/api/v2/authz/is \
    -H "Content-Type: application/json" \
    -d '{
        "identity_context": {
            "type": "IDENTITY_TYPE_SUB",
            "identity": "dev@local.prism"
        },
        "resource_context": {
            "object_type": "namespace",
            "object_id": "dev-playground"
        },
        "policy_context": {
            "path": "prism.authz",
            "decisions": ["allowed"]
        }
    }' | jq .

# Expected output:
# {
#   "decisions": {
#     "allowed": true
#   }
# }
```

**Policy hot-reload** (no restart required):

```bash
# Edit policy file
vi topaz/policies/prism.rego

# Policies are automatically reloaded by Topaz
# No restart needed!

# Verify policy change
curl http://localhost:8383/api/v2/policies | jq .
```

## Scenario 2: Integration Testing

### Requirements

**For integration tests, we need**:
- Reproducible setup (same users/permissions every test run)
- Fast teardown/reset (clean state between tests)
- Parallel test execution (isolated Topaz instances)
- CI/CD integration (GitHub Actions)

### Test Container Setup

**Using testcontainers for Go tests**:

```go
// tests/integration/topaz_test.go
package integration_test

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

func TestAuthorizationWithTopaz(t *testing.T) {
    ctx := context.Background()

    // Start Topaz container
    topazContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "ghcr.io/aserto-dev/topaz:0.30.14",
            ExposedPorts: []string{"8282/tcp", "8383/tcp"},
            WaitingFor: wait.ForHTTP("/health").
                WithPort("8383/tcp").
                WithStartupTimeout(30 * time.Second),
            Env: map[string]string{
                "TOPAZ_DB_PATH":     "/tmp/topaz.db",
                "TOPAZ_POLICY_ROOT": "/policies",
            },
            Files: []testcontainers.ContainerFile{
                {
                    HostFilePath:      "../../topaz/config.test.yaml",
                    ContainerFilePath: "/config/topaz-config.yaml",
                    FileMode:          0644,
                },
                {
                    HostFilePath:      "../../topaz/policies",
                    ContainerFilePath: "/policies",
                    FileMode:          0755,
                },
            },
            Cmd: []string{"run", "-c", "/config/topaz-config.yaml"},
        },
        Started: true,
    })
    assert.NoError(t, err)
    defer topazContainer.Terminate(ctx)

    // Get Topaz endpoint
    host, _ := topazContainer.Host(ctx)
    port, _ := topazContainer.MappedPort(ctx, "8282")
    topazEndpoint := fmt.Sprintf("%s:%s", host, port.Port())

    // Bootstrap test data
    restPort, _ := topazContainer.MappedPort(ctx, "8383")
    bootstrapTopaz(t, host, restPort.Port())

    // Run authorization tests
    t.Run("DeveloperCanReadNamespace", func(t *testing.T) {
        allowed := checkAuthorization(t, topazEndpoint, AuthzRequest{
            User:       "dev@local.prism",
            Permission: "read",
            Resource:   "namespace:dev-playground",
        })
        assert.True(t, allowed, "Developer should be able to read dev-playground")
    })

    t.Run("DeveloperCannotAdminNamespace", func(t *testing.T) {
        allowed := checkAuthorization(t, topazEndpoint, AuthzRequest{
            User:       "dev@local.prism",
            Permission: "admin",
            Resource:   "namespace:dev-playground",
        })
        assert.False(t, allowed, "Developer should NOT be able to admin dev-playground")
    })

    t.Run("AdminCanAccessAllNamespaces", func(t *testing.T) {
        allowed := checkAuthorization(t, topazEndpoint, AuthzRequest{
            User:       "admin@local.prism",
            Permission: "admin",
            Resource:   "namespace:test-namespace",
        })
        assert.True(t, allowed, "Admin should have access to all namespaces")
    })
}

func bootstrapTopaz(t *testing.T, host, port string) {
    // Execute bootstrap script against container
    restURL := fmt.Sprintf("http://%s:%s", host, port)

    // Create test users
    createUser(t, restURL, "dev@local.prism", "Local Developer")
    createUser(t, restURL, "admin@local.prism", "Local Admin")

    // Create groups
    createGroup(t, restURL, "developers")
    createGroup(t, restURL, "admins")

    // Create relationships
    addUserToGroup(t, restURL, "dev@local.prism", "developers")
    addUserToGroup(t, restURL, "admin@local.prism", "admins")

    // Create namespaces and permissions
    createNamespace(t, restURL, "dev-playground")
    grantPermission(t, restURL, "developers", "developer", "dev-playground")
    grantPermission(t, restURL, "admins", "admin", "dev-playground")
}
```

### CI/CD Configuration (GitHub Actions)

**`.github/workflows/integration-tests.yml`**:

```yaml
name: Integration Tests with Topaz

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  integration-test:
    runs-on: ubuntu-latest

    services:
      # Topaz service container
      topaz:
        image: ghcr.io/aserto-dev/topaz:0.30.14
        ports:
          - 8282:8282
          - 8383:8383
        options: >-
          --health-cmd "curl -f http://localhost:8383/health"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        volumes:
          - ${{ github.workspace }}/topaz/config.test.yaml:/config/topaz-config.yaml:ro
          - ${{ github.workspace }}/topaz/policies:/policies:ro

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Bootstrap Topaz directory
        run: |
          bash topaz/seed/bootstrap.sh
        env:
          TOPAZ_REST: http://localhost:8383

      - name: Run integration tests
        run: |
          go test -v ./tests/integration/... -tags=integration
        env:
          TOPAZ_ENDPOINT: localhost:8282

      - name: Dump Topaz logs on failure
        if: failure()
        run: |
          docker logs ${{ job.services.topaz.id }}
```

### Test Configuration

**`topaz/config.test.yaml`** (optimized for testing):

```yaml
version: 2

logger:
  prod: true
  log_level: warn  # Less verbose for tests

api:
  grpc:
    listen_address: "0.0.0.0:8282"
    connection_timeout: 2s
  rest:
    listen_address: "0.0.0.0:8383"

directory:
  db:
    type: sqlite
    path: ":memory:"  # In-memory database for fast tests
  seed_metadata: false

policy:
  engine: opa
  policy_root: /policies

edge:
  enabled: false  # No remote sync in tests

authorizer:
  grpc:
    connection_timeout: 2s
  needs:
    - kind: policy
    - kind: directory
```

## Performance Characteristics

### Local Development

**Startup Time**:
- Topaz container: ~2 seconds
- Policy load: ~100ms
- Directory bootstrap: ~500ms
- **Total: &lt;3 seconds**

**Authorization Latency**:
- First check (cold): ~5ms
- Subsequent checks (cached): &lt;1ms
- P99: &lt;2ms

**Resource Usage**:
- Memory: ~50 MB (idle), ~100 MB (active)
- CPU: &lt;1% (idle), ~5% (under load)

### Integration Testing

**Test Suite Performance** (50 authorization tests):
- Sequential execution: ~2 seconds
- Parallel execution: ~500ms
- Per-test overhead: &lt;10ms

**Container Lifecycle**:
- Startup: ~2 seconds
- Teardown: &lt;1 second
- Total test time: &lt;5 seconds (including container lifecycle)

## Troubleshooting

### Issue 1: Topaz Container Won't Start

**Symptom**: `docker compose up` fails with connection refused

**Diagnosis**:
```bash
# Check Topaz logs
docker compose logs topaz

# Common errors:
# - Port 8282 already in use
# - Config file not found
# - Policy files have syntax errors
```

**Solution**:
```bash
# Check port availability
lsof -i :8282

# Validate config file
docker run --rm -v $(pwd)/topaz:/config ghcr.io/aserto-dev/topaz:0.30.14 \
    validate -c /config/config.local.yaml

# Validate policies
docker run --rm -v $(pwd)/topaz/policies:/policies \
    openpolicyagent/opa:latest test /policies
```

### Issue 2: Bootstrap Script Fails

**Symptom**: `bootstrap.sh` exits with "Topaz not ready"

**Diagnosis**:
```bash
# Check if Topaz is listening
curl -v http://localhost:8383/health

# Check Topaz startup logs
docker compose logs topaz | grep -i error
```

**Solution**:
```bash
# Increase wait time in bootstrap script
until curl -s "$TOPAZ_REST/health" > /dev/null; do
    echo "Waiting for Topaz..."
    sleep 2  # Increase from 1 to 2 seconds
done

# Or check specific endpoint
curl -f http://localhost:8383/api/v2/directory/objects || exit 1
```

### Issue 3: Authorization Always Denied

**Symptom**: All authorization checks return `allowed: false`

**Diagnosis**:
```bash
# Check directory state
curl http://localhost:8383/api/v2/directory/objects | jq .

# Check relations
curl http://localhost:8383/api/v2/directory/relations | jq .

# Check policy evaluation
curl -X POST http://localhost:8282/api/v2/authz/is \
    -H "Content-Type: application/json" \
    -d '{...}' | jq .
```

**Solution**:
```bash
# Re-run bootstrap
bash topaz/seed/bootstrap.sh

# Verify user exists
curl http://localhost:8383/api/v2/directory/objects?object_type=user | \
    jq '.results[] | select(.id=="dev@local.prism")'

# Verify relationships
curl http://localhost:8383/api/v2/directory/relations | \
    jq '.results[] | select(.subject_id=="dev@local.prism")'

# Check policy syntax
docker run --rm -v $(pwd)/topaz/policies:/policies \
    openpolicyagent/opa:latest test /policies -v
```

### Issue 4: Policy Changes Not Applied

**Symptom**: Modified policies don't take effect

**Solution**:
```bash
# Topaz should auto-reload, but force reload:
docker compose restart topaz

# Or use policy API to reload
curl -X POST http://localhost:8383/api/v2/policies/reload

# Verify policy version
curl http://localhost:8383/api/v2/policies | jq '.policies[].version'
```

## Integration with Pattern SDK

Patterns (formerly plugins) integrate with local Topaz using the authorization layer from RFC-019:

**Pattern configuration** (`patterns/redis/config.local.yaml`):

```yaml
authz:
  token:
    enabled: false  # Token validation disabled for local dev

  topaz:
    enabled: true
    endpoint: "localhost:8282"
    timeout: 2s
    cache_ttl: 5s
    tls:
      enabled: false

  audit:
    enabled: true
    destination: "stdout"

  enforce: false  # Log violations but don't block in dev mode
```

**Pattern usage**:

```go
// patterns/redis/main.go
import "github.com/prism/pattern-sdk/authz"

func main() {
    // Initialize authorizer with local Topaz
    authzConfig := authz.Config{
        Topaz: authz.TopazConfig{
            Enabled:  true,
            Endpoint: "localhost:8282",
        },
        Enforce: false,  // Dev mode: log but don't block
    }

    authorizer, _ := authz.NewAuthorizer(authzConfig)

    // Use in pattern
    pattern := &RedisPattern{
        authz: authorizer,
    }

    // Authorization automatically enforced via gRPC interceptor
    server := grpc.NewServer(
        grpc.UnaryInterceptor(authz.UnaryServerInterceptor(authorizer)),
    )
}
```

## Comparison: Development vs Integration Testing vs Production

| Aspect | Development | Integration Testing | Production |
|--------|-------------|---------------------|------------|
| **Startup** | Docker Compose | testcontainers | Kubernetes sidecar |
| **Database** | SQLite file | SQLite in-memory | PostgreSQL |
| **Policy sync** | Disabled (local files) | Disabled (local files) | Enabled (Git + Aserto) |
| **Enforcement** | Warn only (enforce: false) | Strict (enforce: true) | Strict (enforce: true) |
| **Fail mode** | Fail-open (allow if down) | Fail-closed (deny if down) | Fail-closed (deny if down) |
| **Audit logs** | Stdout | Stdout | Centralized (gRPC) |
| **Users** | Static seed data | Static test data | Dynamic (synced from OIDC) |

## Related Documents

- [ADR-050: Topaz for Policy Authorization](/adr/adr-050-topaz-policy-authorization) - Why Topaz was selected
- [RFC-019: Pattern SDK Authorization Layer](/rfc/rfc-019) - Pattern SDK integration
- [RFC-016: Local Development Infrastructure](/rfc/rfc-016-local-development-infrastructure) - Complete local stack
- [ADR-046: Dex IDP for Local Testing](/adr/adr-046-dex-idp-local-testing) - OIDC authentication
- [MEMO-008: Vault Token Exchange Flow](/memos/memo-008-vault-token-exchange-flow) - Credential management

## Revision History

- 2025-10-11: Updated terminology from "Plugin SDK" to "Pattern SDK" for consistency with RFC-022
- 2025-10-09: Initial memo documenting Topaz as local authorizer for development and integration testing