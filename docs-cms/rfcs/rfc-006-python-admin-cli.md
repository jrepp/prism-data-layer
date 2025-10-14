---
author: System
created: 2025-10-08
doc_uuid: 51c75a8b-c635-49f2-9c4c-a078ef110d48
id: rfc-006
project_id: prism-data-layer
sidebar_label: RFC-006 Admin CLI
status: Superseded
title: Admin CLI (prismctl)
---

# RFC-006: Admin CLI (prismctl)
**Author**: System
**Created**: 2025-10-08
**Updated**: 2025-10-09
**Superseded By**: ADR-040 (Go Binary for Admin CLI)

> **Note**: This RFC originally proposed a Python-based CLI. The implementation has shifted to **Go** for better performance, single-binary distribution, and consistency with backend plugins. See [ADR-040: Go Binary for Admin CLI](/adr/adr-040) for the accepted implementation approach. The functional specifications below remain valid regardless of implementation language.

## Abstract

This RFC proposes a comprehensive command-line interface (CLI) for administering Prism data access gateway, now implemented as **prismctl** (a Go binary). The CLI provides operational visibility, configuration management, and troubleshooting capabilities before building a full web-based admin UI. By delivering CLI-first tooling, we enable automation, scripting, and CI/CD integration while validating the admin API design.

The CLI will interact with Prism's admin gRPC services (RFC-003) to manage namespaces, monitor sessions, inspect backend health, and configure data access patterns across all supported backends (Kafka, NATS, PostgreSQL, Redis, ClickHouse).

## Motivation

### Why CLI First?

1. **Faster Time to Value**: CLI tools can be developed and iterated faster than full UIs
2. **Automation Ready**: Enable scripting and CI/CD integration from day one
3. **API Validation**: Using the CLI validates admin API design before UI investment
4. **DevOps Friendly**: Operators prefer CLI tools for troubleshooting and automation
5. **Low Barrier**: Python + Click/Typer = rapid development with excellent UX

### Real-World Use Cases

- **Namespace Management**: Create, configure, and delete namespaces
- **Health Monitoring**: Check backend connectivity and performance metrics
- **Session Inspection**: Debug active client sessions and connection pools
- **Configuration Changes**: Update backend settings, capacity limits, consistency levels
- **Traffic Analysis**: Inspect request rates, latency distributions, error rates
- **Migration Support**: Shadow traffic configuration, backend switching, rollback

## Goals

- Provide complete admin functionality via CLI before building web UI
- Support all operations defined in RFC-003 Admin gRPC API
- Enable automation and scripting for operational workflows
- Deliver excellent developer experience with rich formatting and feedback
- Support YAML-first configuration with automatic config file discovery
- Integrate with existing Python tooling ecosystem (uv, pytest)

## Non-Goals

- **Not replacing web UI**: CLI is a stepping stone and complementary tool
- **Not a data client**: Use language-specific client SDKs for data access
- **Not a full TUI**: Keep it simple; use Rich for formatting, not full-screen apps

## Architecture

### High-Level Design

```mermaid
graph TB
    subgraph "Admin CLI"
        CLI[prismctl CLI<br/>Cobra/Viper]
        Formatter[Rich Formatter<br/>Tables/Trees/JSON]
        Client[gRPC Client<br/>Admin Service]
    end

    subgraph "Prism Proxy"
        AdminAPI[Admin gRPC API<br/>RFC-003]
        ConfigMgr[Config Manager]
        Metrics[Metrics Collector]
        Health[Health Checker]
    end

    subgraph "Backends"
        Kafka[Kafka]
        Redis[Redis]
        ClickHouse[ClickHouse]
        Postgres[PostgreSQL]
        NATS[NATS]
    end

    CLI --> Formatter
    CLI --> Client
    Client -->|gRPC| AdminAPI
    AdminAPI --> ConfigMgr
    AdminAPI --> Metrics
    AdminAPI --> Health
    Health --> Kafka
    Health --> Redis
    Health --> ClickHouse
    Health --> Postgres
    Health --> NATS
```

## Authentication

The Admin CLI requires authentication to access the Prism Admin API. We use **OIDC (OpenID Connect)** with the **device code flow** for command-line SSO authentication.

### Device Code Flow (OAuth 2.0)

The device code flow is designed for CLI applications and devices without browsers. The user authenticates via a web browser while the CLI polls for completion.

```mermaid
sequenceDiagram
    participant User as Administrator
    participant CLI as prismctl
    participant OIDC as OIDC Provider<br/>(Okta/Auth0/Dex)
    participant API as Admin API

    Note over User,API: Initial Login

    User->>CLI: prismctl login
    CLI->>OIDC: POST /oauth/device/code<br/>{client_id, scope}

    OIDC-->>CLI: {<br/>  device_code,<br/>  user_code: "ABCD-1234",<br/>  verification_uri,<br/>  interval: 5<br/>}

    CLI->>User: Please visit:<br/>https://idp.example.com/activate<br/>and enter code: ABCD-1234

    User->>OIDC: Navigate to verification_uri
    OIDC->>User: Show login page
    User->>OIDC: Enter user_code: ABCD-1234
    OIDC->>User: Show consent screen<br/>(Scopes: admin:read, admin:write)
    User->>OIDC: Approve

    loop Poll every 5 seconds (max 5 minutes)
        CLI->>OIDC: POST /oauth/token<br/>{device_code, grant_type}

        alt User approved
            OIDC-->>CLI: {<br/>  access_token (JWT),<br/>  refresh_token,<br/>  expires_in: 3600<br/>}
            CLI->>CLI: Save to ~/.prism/token
            CLI-->>User: ✓ Authenticated as alice@company.com
        else Still pending
            OIDC-->>CLI: {error: "authorization_pending"}
        else User denied
            OIDC-->>CLI: {error: "access_denied"}
        end
    end

    Note over User,API: Authenticated Requests

    User->>CLI: prismctl namespace list
    CLI->>CLI: Load ~/.prism/token

    alt Token valid
        CLI->>API: gRPC: ListNamespaces()<br/>metadata: authorization: Bearer <jwt>
        API->>API: Validate JWT signature<br/>Check expiry<br/>Extract claims (email, groups, scopes)
        API-->>CLI: NamespacesResponse
        CLI-->>User: Display namespaces
    else Token expired
        CLI->>OIDC: POST /oauth/token<br/>{refresh_token, grant_type}
        OIDC-->>CLI: New access_token
        CLI->>CLI: Update ~/.prism/token
        CLI->>API: Retry with new token
    end
```

### Login Command

```bash
# Interactive login (device code flow)
prismctl login

# Or specify OIDC issuer explicitly
prismctl login --issuer https://idp.example.com

# Local development with Dex (see ADR-046)
prismctl login --local
```

**Output**:
Opening browser for authentication...

Visit: https://idp.example.com/activate
Enter code: WXYZ-1234

Waiting for authentication...
```text

*Browser opens automatically to verification URL*

✓ Authenticated as alice@company.com
Token expires in 1 hour
Token cached to ~/.prism/token

You can now use prismctl commands:
  prismctl namespace list
  prismctl backend health
  prismctl session list
```

### Token Storage

Tokens are securely stored in `~/.prism/token`:

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_at": "2025-10-09T15:30:00Z",
  "issued_at": "2025-10-09T14:30:00Z",
  "issuer": "https://idp.example.com",
  "principal": "alice@company.com",
  "groups": ["platform-team", "admins"]
}
```

**Security**:
- File permissions: `0600` (owner read/write only)
- Tokens expire after 1 hour
- Refresh tokens extend session to 7 days
- Automatic refresh on expiry

### Logout Command

```bash
# Logout (delete cached token)
prismctl logout

# Verify logged out
prismctl namespace list
# → Error: Not authenticated. Run 'prismctl login' to authenticate.
```

### Authentication Modes

| Mode | Use Case | Command | Token Source |
|------|----------|---------|--------------|
| **Interactive** | Developer workstation | `prismctl login` | Device code flow |
| **Service Account** | CI/CD, automation | `PRISM_TOKEN=<jwt> prismctl ...` | Environment variable |
| **Local (Dex)** | Local development | `prismctl login --local` | Dex IDP (see ADR-046) |
| **Custom Issuer** | Enterprise SSO | `prismctl login --issuer <url>` | Custom OIDC provider |

### Service Account Authentication

For automation (CI/CD pipelines, cron jobs), use service account tokens:

```bash
# Export token from secret manager
export PRISM_TOKEN=$(vault kv get -field=token prism/ci-service)

# Use prismctl with service account token
prismctl namespace list
# CLI detects PRISM_TOKEN and uses it instead of cached token
```

**Service Account Token Claims**:
```json
{
  "iss": "https://idp.example.com",
  "sub": "service:prism-ci",
  "aud": "prismctl-api",
  "exp": 1696867200,
  "iat": 1696863600,
  "email": "ci-service@prism.local",
  "groups": ["ci-automation"],
  "scope": "admin:read admin:write"
}
```

### Authentication Flow Implementation

The CLI authentication is implemented in the Admin gRPC client wrapper:

```go
// internal/client/auth.go
package client

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "golang.org/x/oauth2"
)

type TokenCache struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    TokenType    string    `json:"token_type"`
    ExpiresAt    time.Time `json:"expires_at"`
    IssuedAt     time.Time `json:"issued_at"`
    Issuer       string    `json:"issuer"`
    Principal    string    `json:"principal"`
    Groups       []string  `json:"groups"`
}

func LoadToken() (*TokenCache, error) {
    // Check environment variable first
    if token := os.Getenv("PRISM_TOKEN"); token != "" {
        return &TokenCache{
            AccessToken: token,
            TokenType:   "Bearer",
        }, nil
    }

    // Load from cache file
    home, err := os.UserHomeDir()
    if err != nil {
        return nil, err
    }

    tokenPath := filepath.Join(home, ".prism", "token")
    data, err := os.ReadFile(tokenPath)
    if err != nil {
        return nil, fmt.Errorf("not authenticated: %w", err)
    }

    var cache TokenCache
    if err := json.Unmarshal(data, &cache); err != nil {
        return nil, err
    }

    // Check if token expired
    if time.Now().After(cache.ExpiresAt) {
        // Try to refresh
        return refreshToken(&cache)
    }

    return &cache, nil
}

func SaveToken(cache *TokenCache) error {
    home, err := os.UserHomeDir()
    if err != nil {
        return err
    }

    prismDir := filepath.Join(home, ".prism")
    if err := os.MkdirAll(prismDir, 0700); err != nil {
        return err
    }

    tokenPath := filepath.Join(prismDir, "token")
    data, err := json.MarshalIndent(cache, "", "  ")
    if err != nil {
        return err
    }

    // Write with restricted permissions (0600)
    return os.WriteFile(tokenPath, data, 0600)
}

func refreshToken(cache *TokenCache) (*TokenCache, error) {
    if cache.RefreshToken == "" {
        return nil, fmt.Errorf("no refresh token available")
    }

    // Use oauth2 library to refresh
    config := &oauth2.Config{
        ClientID: "prismctl",
        Endpoint: oauth2.Endpoint{
            TokenURL: cache.Issuer + "/oauth/token",
        },
    }

    token := &oauth2.Token{
        RefreshToken: cache.RefreshToken,
    }

    src := config.TokenSource(context.Background(), token)
    newToken, err := src.Token()
    if err != nil {
        return nil, fmt.Errorf("failed to refresh token: %w", err)
    }

    cache.AccessToken = newToken.AccessToken
    cache.ExpiresAt = newToken.Expiry
    cache.IssuedAt = time.Now()

    // Save updated token
    if err := SaveToken(cache); err != nil {
        return nil, err
    }

    return cache, nil
}
```

### Local Development with Dex

For local testing without cloud OIDC providers, use Dex (see ADR-046):

```bash
# Start Dex in Docker Compose
docker-compose up -d dex

# Login with local Dex
prismctl login --local
# Opens: http://localhost:5556/auth

# Login with test user
# Email: admin@prism.local
# Password: password

# Token cached, ready to use
prismctl namespace list
```

**References**:
- RFC-010: Admin Protocol with OIDC Authentication (complete OIDC specification)
- ADR-046: Dex IDP for Local Identity Testing (local development setup)
- [OAuth 2.0 Device Authorization Grant (RFC 8628)](https://datatracker.ietf.org/doc/html/rfc8628)

### Command Structure

prism
├── namespace
│   ├── create      # Create new namespace
│   ├── list        # List all namespaces
│   ├── describe    # Show namespace details
│   ├── update      # Update namespace config
│   └── delete      # Delete namespace
├── backend
│   ├── list        # List configured backends
│   ├── health      # Check backend health
│   ├── stats       # Show backend statistics
│   └── test        # Test backend connectivity
├── session
│   ├── list        # List active sessions
│   ├── describe    # Show session details
│   ├── kill        # Terminate session
│   └── trace       # Trace session requests
├── config
│   ├── show        # Display current config
│   ├── validate    # Validate config file
│   └── apply       # Apply config changes
├── metrics
│   ├── summary     # Overall metrics summary
│   ├── namespace   # Namespace-level metrics
│   └── export      # Export metrics (Prometheus format)
├── shadow
│   ├── enable      # Enable shadow traffic
│   ├── disable     # Disable shadow traffic
│   └── status      # Show shadow traffic status
└── plugin
    ├── list        # List installed plugins
    ├── install     # Install plugin from registry
    ├── update      # Update plugin version
    ├── enable      # Enable plugin
    ├── disable     # Disable plugin
    ├── status      # Show plugin health and metrics
    ├── reload      # Hot-reload plugin code
    └── logs        # View plugin logs
```text

## Command Specifications

### Namespace Management

#### Create Namespace

```
# Preferred: Declarative mode from config file
prismctl namespace create --config namespace.yaml

# Config file discovery (searches . and parent dirs for .config.yaml)
prismctl namespace create my-app  # Uses .config.yaml from current or parent dir

# Inline configuration (for simple cases or scripting)
prismctl namespace create my-app \
  --backend postgres \
  --pattern keyvalue \
  --consistency strong \
  --cache-ttl 300
```text

**Output (Rich table)**:
┏━━━━━━━━━━━━┳━━━━━━━━━━┳━━━━━━━━━━━━┳━━━━━━━━━━━━━━┓
┃ Namespace  ┃ Backend  ┃ Pattern    ┃ Status       ┃
┡━━━━━━━━━━━━╇━━━━━━━━━━╇━━━━━━━━━━━━╇━━━━━━━━━━━━━━┩
│ my-app     │ postgres │ keyvalue   │ ✓ Created    │
└────────────┴──────────┴────────────┴──────────────┘

Created namespace 'my-app' successfully
gRPC endpoint: localhost:50051
Admin endpoint: localhost:50052
```

#### List Namespaces

```bash
# Default table view
prismctl namespace list

# JSON output for scripting
prismctl namespace list --output json

# Filter by backend
prismctl namespace list --backend redis

# Show inactive namespaces
prismctl namespace list --include-inactive
```

**Output**:
┏━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━┳━━━━━━━━━━━━┳━━━━━━━━━━┳━━━━━━━━━━━━┓
┃ Namespace      ┃ Backend    ┃ Pattern    ┃ Sessions ┃ RPS        ┃
┡━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━╇━━━━━━━━━━━━╇━━━━━━━━━━╇━━━━━━━━━━━━┩
│ user-profiles  │ postgres   │ keyvalue   │ 24       │ 1,234      │
│ event-stream   │ kafka      │ stream     │ 8        │ 45,678     │
│ session-cache  │ redis      │ cache      │ 156      │ 89,012     │
│ metrics-olap   │ clickhouse │ timeseries │ 4        │ 12,345     │
└────────────────┴────────────┴────────────┴──────────┴────────────┘
```text

#### Describe Namespace

```
prismctl namespace describe my-app

# Include recent errors
prismctl namespace describe my-app --show-errors

# Include configuration
prismctl namespace describe my-app --show-config
```text

**Output**:
Namespace: my-app
Status: Active
Created: 2025-10-01 14:23:45 UTC
Updated: 2025-10-08 09:12:34 UTC

Backend Configuration:
  Type: PostgreSQL
  Pattern: KeyValue
  Connection: postgres://prism-pg-1:5432/prism_my_app
  Consistency: Strong
  Connection Pool: 20 max connections

Performance:
  Current RPS: 1,234
  P50 Latency: 2.3ms
  P99 Latency: 12.7ms
  Error Rate: 0.02%

Active Sessions: 24
  ├─ session-abc123: 2 connections, 45 RPS
  ├─ session-def456: 5 connections, 234 RPS
  └─ ... (22 more)

Recent Errors (last 10):
  [2025-10-08 09:05:12] Connection timeout (1 occurrence)
```

### Backend Management

#### Health Check

```bash
# Check all backends
prismctl backend health

# Check specific backend
prismctl backend health --backend postgres

# Detailed health check with diagnostics
prismctl backend health --detailed
```

**Output**:
Backend Health Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ PostgreSQL (postgres-1)
  Status: Healthy
  Latency: 1.2ms
  Connections: 45/100 used
  Last Check: 2s ago

✓ Redis (redis-cache-1)
  Status: Healthy
  Latency: 0.3ms
  Memory: 2.1GB / 8GB
  Last Check: 2s ago

✓ ClickHouse (clickhouse-1)
  Status: Healthy
  Latency: 3.4ms
  Queries: 234 active
  Last Check: 2s ago

✗ Kafka (kafka-1)
  Status: Degraded
  Error: Connection refused to broker-3
  Last Success: 5m ago
  Action: Check broker-3 connectivity
```text

#### Backend Statistics

```
# Show stats for all backends
prismctl backend stats

# Stats for specific namespace
prismctl backend stats --namespace my-app

# Export to JSON
prismctl backend stats --output json
```text

### Session Management

#### List Sessions

```
# List all active sessions across all namespaces
prismctl session list

# Scope to specific namespace (preferred for focused inspection)
prismctl session list --namespace my-app

# Scope using config file (.config.yaml must specify namespace)
prismctl session list  # Auto-scopes if .config.yaml has namespace set

# Show long-running sessions
prismctl session list --duration ">1h"
```text

**Output**:
┏━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━┳━━━━━━━━━━━━┳━━━━━━━━━━━━┓
┃ Session ID    ┃ Principal            ┃ Namespace      ┃ Duration   ┃ Requests   ┃ RPS        ┃
┡━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━╇━━━━━━━━━━━━╇━━━━━━━━━━━━┩
│ sess-abc123   │ alice@company.com    │ user-profiles  │ 2h 34m     │ 456,789    │ 45         │
│ sess-def456   │ user-api.prod (svc)  │ event-stream   │ 45m        │ 123,456    │ 234        │
│ sess-ghi789   │ bob@company.com      │ session-cache  │ 12m        │ 89,012     │ 567        │
└───────────────┴──────────────────────┴────────────────┴────────────┴────────────┴────────────┘
```

#### Trace Session

```bash
# Live trace of session requests
prismctl session trace sess-abc123

# Trace with filtering
prismctl session trace sess-abc123 --min-latency 100ms

# Export trace to file
prismctl session trace sess-abc123 --duration 60s --output trace.json
```

**Output (live streaming)**:
Tracing session sess-abc123 (Ctrl+C to stop)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

09:15:23.456 | GET    | user_profiles:user:12345    | 2.3ms  | ✓
09:15:23.489 | SET    | user_profiles:user:12345    | 3.1ms  | ✓
09:15:23.512 | GET    | user_profiles:user:67890    | 1.8ms  | ✓
09:15:23.534 | DELETE | user_profiles:user:11111    | 145ms  | ✗ Not Found

Statistics:
  Requests: 4
  Success: 3 (75%)
  Avg Latency: 38.05ms
  P99 Latency: 145ms
```text

### Configuration Management

#### Show Configuration

```
# Show proxy-wide configuration
prismctl config show

# Show namespace-specific config (scoped view)
prismctl config show --namespace my-app

# Auto-scope using .config.yaml (if namespace specified in config)
prismctl config show  # Uses namespace from .config.yaml if present

# Export configuration
prismctl config show --output yaml > prism-config.yaml
```text

#### Validate Configuration

```
# Validate config file before applying
prismctl config validate prism-config.yaml

# Dry-run mode
prismctl config validate prism-config.yaml --dry-run
```text

**Output**:
Validating configuration: prism-config.yaml
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ YAML syntax valid
✓ Schema validation passed
✓ Backend connections verified (3/3)
✓ Namespace uniqueness verified
✓ Capacity limits within bounds
✗ Warning: Redis memory limit (16GB) exceeds available (12GB)

Validation: PASSED (1 warning)
Safe to apply: Yes (with warnings)
```

### Metrics and Monitoring

#### Metrics Summary

```bash
# Overall metrics across all namespaces
prismctl metrics summary

# Namespace-specific metrics (scoped view)
prismctl metrics summary --namespace my-app

# Auto-scope using .config.yaml
prismctl metrics summary  # Uses namespace from .config.yaml if present

# Time range filtering
prismctl metrics summary --since "1h ago" --namespace my-app
```

**Output**:
Prism Metrics Summary (Last 1 hour)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Request Volume:
  Total Requests: 12,456,789
  Success Rate: 99.98%
  Error Rate: 0.02%

Performance:
  P50 Latency: 2.3ms
  P95 Latency: 8.7ms
  P99 Latency: 23.4ms

Top Namespaces by RPS:
  1. event-stream      45,678 RPS
  2. session-cache     12,345 RPS
  3. user-profiles      1,234 RPS

Backend Health:
  ✓ PostgreSQL    (5 instances)
  ✓ Redis         (3 instances)
  ✓ ClickHouse    (2 instances)
  ✗ Kafka         (1 degraded)
```text

#### Export Metrics

```
# Prometheus format
prismctl metrics export --format prometheus > metrics.prom

# JSON format with metadata
prismctl metrics export --format json --include-metadata > metrics.json
```text

### Shadow Traffic

#### Enable Shadow Traffic

```
# Enable shadow traffic for Postgres version upgrade (14 → 16)
prismctl shadow enable user-profiles \
  --source postgres-14-primary \
  --target postgres-16-replica \
  --percentage 10

# Gradual rollout with automatic ramp-up
prismctl shadow enable user-profiles \
  --source postgres-14-primary \
  --target postgres-16-replica \
  --ramp-up "10%,25%,50%,100%" \
  --interval 1h
```text

**Output**:
Enabling shadow traffic for namespace 'user-profiles'
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Migration: Postgres 14 → Postgres 16 upgrade

Configuration:
  Source: postgres-14-primary (current production)
  Target: postgres-16-replica (upgrade candidate)
  Initial Percentage: 10%
  Ramp-up Schedule:
    • 10% at 09:15:00 (now)
    • 25% at 10:15:00 (+1h)
    • 50% at 11:15:00 (+2h)
    • 100% at 12:15:00 (+3h)

✓ Shadow traffic enabled
  Monitor: prismctl shadow status user-profiles
  Disable: prismctl shadow disable user-profiles
```

#### Shadow Status

```bash
prismctl shadow status user-profiles
```

**Output**:
Shadow Traffic Status: user-profiles
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Migration: Postgres 14 → Postgres 16 upgrade

Status: Active
Current Stage: 25% traffic to target
Next Stage: 50% at 11:15:00 (+45m)

Backends:
  Source: postgres-14-primary (Postgres 14.10)
  Target: postgres-16-replica (Postgres 16.1)

Traffic Distribution:
  ┌────────────────────────────────────────┐
  │ ████████████████████████████           │ 75% → postgres-14-primary
  │ ████████                               │ 25% → postgres-16-replica
  └────────────────────────────────────────┘

Comparison Metrics:
  ┏━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━┓
  ┃ Metric     ┃ PG 14 (Source)     ┃ PG 16 (Target)     ┃ Delta      ┃
  ┡━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━┩
  │ P50        │ 2.3ms              │ 2.1ms              │ -9%        │
  │ P99        │ 12.7ms             │ 11.8ms             │ -7%        │
  │ Error Rate │ 0.02%              │ 0.01%              │ -50%       │
  │ QPS        │ 1,234              │ 1,234              │ 0%         │
  └────────────┴────────────────────┴────────────────────┴────────────┘

Query Compatibility:
  ✓ All queries compatible with PG 16
  ✓ No deprecated features detected
  ✓ Performance parity achieved

✓ Target performing well, ready for next stage
```text

### Plugin Management

Plugin commands manage backend plugins (installation, updates, health monitoring, hot-reload).

For complete plugin development guide, see [RFC-008: Plugin Development Experience](/rfc/rfc-008).

#### List Plugins

```
# List all installed plugins
prismctl plugin list

# Filter by status
prismctl plugin list --status enabled
prismctl plugin list --status disabled

# Show plugin versions
prismctl plugin list --show-versions
```text

**Output**:
┏━━━━━━━━━━━━━━┳━━━━━━━━━┳━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━┓
┃ Plugin       ┃ Version ┃ Status   ┃ Namespaces       ┃ Health     ┃
┡━━━━━━━━━━━━━━╇━━━━━━━━━╇━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━┩
│ postgres     │ 1.2.0   │ enabled  │ 45 namespaces    │ ✓ Healthy  │
│ redis        │ 2.1.3   │ enabled  │ 123 namespaces   │ ✓ Healthy  │
│ kafka        │ 3.0.1   │ enabled  │ 18 namespaces    │ ⚠ Degraded │
│ clickhouse   │ 1.5.0   │ disabled │ 0 namespaces     │ - Disabled │
│ mongodb      │ 0.9.0   │ enabled  │ 7 namespaces     │ ✓ Healthy  │
└──────────────┴─────────┴──────────┴──────────────────┴────────────┘
```

#### Install Plugin

```bash
# Install from registry (default: latest version)
prismctl plugin install mongodb

# Install specific version
prismctl plugin install mongodb --version 1.0.0

# Install from local path (development)
prismctl plugin install --local /path/to/mongodb-plugin.so

# Install with custom config
prismctl plugin install mongodb --config plugin-config.yaml
```

**Output**:
Installing plugin: mongodb
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ Downloaded mongodb-plugin v1.0.0 (5.2 MB)
✓ Verified plugin signature
✓ Loaded plugin library
✓ Initialized plugin
✓ Health check passed

Plugin 'mongodb' installed successfully
Supported operations: get, set, query, aggregate
Ready to create namespaces with backend: mongodb
```text

#### Update Plugin

```
# Update to latest version
prismctl plugin update mongodb

# Update to specific version
prismctl plugin update mongodb --version 1.1.0

# Dry-run mode (check compatibility without applying)
prismctl plugin update mongodb --dry-run
```text

**Output (with warnings)**:
Updating plugin: mongodb (1.0.0 → 1.1.0)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

⚠ Warning: 7 namespaces using this plugin will be restarted
  - mongodb-cache
  - user-sessions
  - product-catalog
  - (4 more...)

✓ Downloaded mongodb-plugin v1.1.0 (5.3 MB)
✓ Verified plugin signature

Proceed with update? [y/N]: y

✓ Stopped old plugin instances
✓ Loaded new plugin version
✓ Migrated plugin state
✓ Restarted namespaces (7/7 healthy)

Plugin 'mongodb' updated successfully (1.0.0 → 1.1.0)
```

#### Enable/Disable Plugin

```bash
# Disable plugin (prevent new namespaces, keep existing running)
prismctl plugin disable kafka

# Enable previously disabled plugin
prismctl plugin enable kafka

# Force disable (stop all namespaces using this plugin)
prismctl plugin disable kafka --force
```

**Output**:
Disabling plugin: kafka
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

⚠ 18 namespaces currently using this plugin

Actions:
  ✓ Prevent new namespaces from using kafka backend
  ✓ Existing namespaces will continue running
  ⚠ Use --force to stop all kafka namespaces

Plugin 'kafka' disabled successfully
```text

#### Plugin Status

```
# View plugin health and detailed metrics
prismctl plugin status mongodb

# Include recent errors
prismctl plugin status mongodb --show-errors

# Live monitoring mode
prismctl plugin status mongodb --watch
```text

**Output**:
Plugin Status: mongodb (v1.0.0)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Health: ✓ Healthy
Status: Enabled
Deployment: Sidecar (gRPC: localhost:50105)

Namespaces Using Plugin: 7
  ├─ mongodb-cache (active, 1234 RPS)
  ├─ user-sessions (active, 567 RPS)
  └─ ... (5 more)

Performance Metrics (Last 5 minutes):
  Requests: 456,789 total (98.5% success rate)
  Latency:
    P50: 2.1ms
    P99: 8.3ms
    P999: 24.7ms
  Connections: 45 active, 12 idle

Resource Usage:
  CPU: 1.2 cores (30% of limit)
  Memory: 1.8 GB (22% of limit)
  Network: 245 MB/s in, 312 MB/s out

Recent Errors (last 10):
  [09:15:23] Connection timeout to mongodb-1 (1 occurrence)
  [09:12:45] Query timeout for aggregate operation (3 occurrences)
```

#### Hot-Reload Plugin

```bash
# Reload plugin code without restarting namespaces
prismctl plugin reload mongodb

# Reload with validation
prismctl plugin reload mongodb --validate

# Reload and tail logs
prismctl plugin reload mongodb --tail
```

**Output**:
Reloading plugin: mongodb
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ Built new plugin binary
✓ Verified plugin signature
✓ Validated plugin compatibility
✓ Loaded new plugin instance
✓ Migrated active connections (45 connections)
✓ Switched traffic to new instance
✓ Drained old instance

Plugin 'mongodb' reloaded successfully
Namespaces affected: 7 (all healthy)
Reload time: 2.3s (zero downtime)
```text

#### View Plugin Logs

```
# View recent logs
prismctl plugin logs mongodb

# Follow logs (live tail)
prismctl plugin logs mongodb --follow

# Filter by log level
prismctl plugin logs mongodb --level error

# Show logs from specific time range
prismctl plugin logs mongodb --since "1h ago"
```text

**Output**:
Tailing logs: mongodb (Ctrl+C to stop)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

09:15:23.456 INFO  Initialized connection pool (size: 20)
09:15:23.789 INFO  Health check passed
09:15:24.123 DEBUG Executing query: {"op": "find", "collection": "users"}
09:15:24.156 DEBUG Query completed in 33ms
09:15:25.234 WARN  Connection to mongodb-1 timeout, retrying...
09:15:25.567 INFO  Connection re-established
```

### Plugin Development Workflow

For developers creating new plugins, the CLI provides scaffolding and testing tools:

```bash
# Create new plugin from template
prism-plugin-init --name mybackend --language rust

# Test plugin locally (without Prism proxy)
cd mybackend-plugin
prismctl plugin test --config test-config.yaml

# Build and package plugin
prismctl plugin build

# Publish to registry (for distribution)
prismctl plugin publish --registry https://plugins.prism.io
```

See [RFC-008: Plugin Development Experience](/rfc/rfc-008) for complete development guide.

## Protobuf Integration

The CLI communicates with Prism via the Admin gRPC API defined in RFC-003:

```protobuf
// CLI uses these services from RFC-003
service AdminService {
  // Namespace operations
  rpc CreateNamespace(CreateNamespaceRequest) returns (Namespace);
  rpc ListNamespaces(ListNamespacesRequest) returns (ListNamespacesResponse);
  rpc DescribeNamespace(DescribeNamespaceRequest) returns (Namespace);
  rpc UpdateNamespace(UpdateNamespaceRequest) returns (Namespace);
  rpc DeleteNamespace(DeleteNamespaceRequest) returns (DeleteNamespaceResponse);

  // Backend operations
  rpc ListBackends(ListBackendsRequest) returns (ListBackendsResponse);
  rpc CheckBackendHealth(HealthCheckRequest) returns (HealthCheckResponse);
  rpc GetBackendStats(BackendStatsRequest) returns (BackendStatsResponse);

  // Session operations
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc DescribeSession(DescribeSessionRequest) returns (Session);
  rpc KillSession(KillSessionRequest) returns (KillSessionResponse);
  rpc TraceSession(TraceSessionRequest) returns (stream TraceEvent);

  // Configuration operations
  rpc GetConfig(GetConfigRequest) returns (Config);
  rpc ValidateConfig(ValidateConfigRequest) returns (ValidationResult);
  rpc ApplyConfig(ApplyConfigRequest) returns (ApplyConfigResponse);

  // Metrics operations
  rpc GetMetrics(MetricsRequest) returns (MetricsResponse);
  rpc ExportMetrics(ExportMetricsRequest) returns (ExportMetricsResponse);

  // Shadow traffic operations
  rpc EnableShadowTraffic(ShadowTrafficRequest) returns (ShadowTrafficResponse);
  rpc DisableShadowTraffic(DisableShadowTrafficRequest) returns (ShadowTrafficResponse);
  rpc GetShadowStatus(ShadowStatusRequest) returns (ShadowStatus);
}
```

## Implementation

> **Note**: The implementation details below were from the original Python proposal. The actual implementation uses **Go** with Cobra/Viper framework. See [ADR-040](/adr/adr-040) for Go-specific implementation details.

### Technology Stack (Go Implementation - see ADR-040)

- **CLI Framework**: Cobra (command structure) + Viper (configuration)
- **gRPC Client**: google.golang.org/grpc + protobuf-generated stubs
- **Formatting**: Custom table rendering (or external library like pterm)
- **Configuration**: YAML via gopkg.in/yaml.v3
- **Testing**: testscript for acceptance tests (see ADR-039)
- **Distribution**: Single binary via GitHub releases

### Project Structure (Go Implementation)

tools/
├── cmd/
│   └── prismctl/
│       ├── main.go          # CLI entry point
│       ├── root.go          # Root command + config
│       ├── namespace.go     # Namespace commands
│       ├── backend.go       # Backend commands
│       ├── session.go       # Session commands
│       ├── config.go        # Config commands
│       ├── metrics.go       # Metrics commands
│       ├── shadow.go        # Shadow traffic commands
│       └── plugin.go        # Plugin management commands
├── internal/
│   ├── client/
│   │   ├── admin.go         # Admin gRPC client wrapper
│   │   └── auth.go          # Authentication helpers
│   ├── formatters/
│   │   ├── table.go         # Table formatters
│   │   ├── tree.go          # Tree formatters
│   │   └── json.go          # JSON output
│   └── proto/               # Generated protobuf stubs
│       └── admin.pb.go
├── testdata/
│   └── script/              # testscript acceptance tests
│       ├── namespace_create.txtar
│       ├── session_list.txtar
│       └── ...
├── acceptance_test.go       # testscript runner
├── go.mod
└── go.sum
```text

### Example Implementation (Namespace Commands)

```
# src/prism_cli/commands/namespace.py
import typer
from rich.console import Console
from rich.table import Table
from typing import Optional
from ..client.admin import AdminClient
from ..formatters.table import format_namespace_table

app = typer.Typer(help="Namespace management commands")
console = Console()

@app.command()
def create(
    name: str = typer.Argument(..., help="Namespace name"),
    backend: str = typer.Option(None, help="Backend type (postgres, redis, etc.)"),
    pattern: str = typer.Option(None, help="Data access pattern"),
    consistency: str = typer.Option("eventual", help="Consistency level"),
    cache_ttl: Optional[int] = typer.Option(None, help="Cache TTL in seconds"),
    config: Optional[str] = typer.Option(None, help="Path to config file (or .config.yaml)"),
):
    """Create a new namespace.

    Prefers YAML configuration. If --config not specified, searches for .config.yaml
    in current directory and parent directories.
    """
    client = AdminClient()

    # Load config from file (explicit or discovered)
    if config:
        config_data = load_yaml_config(config)
    else:
        config_data = discover_config()  # Search for .config.yaml

    # Override with CLI args if provided
    config_data.update({
        k: v for k, v in {
            'backend': backend,
            'pattern': pattern,
            'consistency': consistency,
            'cache_ttl': cache_ttl,
        }.items() if v is not None
    })

    try:
        namespace = client.create_namespace(name=name, **config_data)

        # Display result
        table = Table(title="Namespace Created")
        table.add_column("Namespace", style="cyan")
        table.add_column("Backend", style="green")
        table.add_column("Pattern", style="yellow")
        table.add_column("Status", style="green")

        table.add_row(
            namespace.name,
            namespace.backend,
            namespace.pattern,
            "✓ Created"
        )

        console.print(table)
        console.print(f"\nCreated namespace '{name}' successfully")
        console.print(f"gRPC endpoint: {namespace.grpc_endpoint}")
        console.print(f"Admin endpoint: {namespace.admin_endpoint}")

    except Exception as e:
        console.print(f"[red]Error creating namespace: {e}[/red]")
        raise typer.Exit(1)

@app.command()
def list(
    output: str = typer.Option("table", help="Output format (table, json)"),
    backend: Optional[str] = typer.Option(None, help="Filter by backend"),
    include_inactive: bool = typer.Option(False, help="Include inactive namespaces"),
):
    """List all namespaces."""
    client = AdminClient()

    try:
        namespaces = client.list_namespaces(
            backend=backend,
            include_inactive=include_inactive,
        )

        if output == "json":
            console.print_json([ns.to_dict() for ns in namespaces])
        else:
            table = format_namespace_table(namespaces)
            console.print(table)

    except Exception as e:
        console.print(f"[red]Error listing namespaces: {e}[/red]")
        raise typer.Exit(1)

@app.command()
def describe(
    name: str = typer.Argument(..., help="Namespace name"),
    show_errors: bool = typer.Option(False, help="Show recent errors"),
    show_config: bool = typer.Option(False, help="Show configuration"),
):
    """Describe a namespace in detail."""
    client = AdminClient()

    try:
        namespace = client.describe_namespace(
            name=name,
            include_errors=show_errors,
            include_config=show_config,
        )

        # Rich formatted output
        console.print(f"\n[bold cyan]Namespace: {namespace.name}[/bold cyan]")
        console.print(f"Status: [green]{namespace.status}[/green]")
        console.print(f"Created: {namespace.created_at}")
        console.print(f"Updated: {namespace.updated_at}")

        console.print("\n[bold]Backend Configuration:[/bold]")
        console.print(f"  Type: {namespace.backend}")
        console.print(f"  Pattern: {namespace.pattern}")
        console.print(f"  Connection: {namespace.connection_string}")
        console.print(f"  Consistency: {namespace.consistency}")

        console.print("\n[bold]Performance:[/bold]")
        console.print(f"  Current RPS: {namespace.current_rps:,}")
        console.print(f"  P50 Latency: {namespace.p50_latency}ms")
        console.print(f"  P99 Latency: {namespace.p99_latency}ms")
        console.print(f"  Error Rate: {namespace.error_rate:.2%}")

        if namespace.sessions:
            console.print(f"\n[bold]Active Sessions: {len(namespace.sessions)}[/bold]")
            for session in namespace.sessions[:3]:
                console.print(f"  ├─ {session.id}: {session.connections} connections, {session.rps} RPS")
            if len(namespace.sessions) > 3:
                console.print(f"  └─ ... ({len(namespace.sessions) - 3} more)")

        if show_errors and namespace.errors:
            console.print("\n[bold]Recent Errors (last 10):[/bold]")
            for error in namespace.errors[:10]:
                console.print(f"  [{error.timestamp}] {error.message} ({error.count} occurrence(s))")

    except Exception as e:
        console.print(f"[red]Error describing namespace: {e}[/red]")
        raise typer.Exit(1)
```text

### Admin gRPC Client Wrapper

```
# src/prism_cli/client/admin.py
import grpc
from typing import List, Optional
from ..proto import admin_pb2, admin_pb2_grpc
from .auth import get_credentials

class AdminClient:
    """Wrapper around Admin gRPC client for CLI operations."""

    def __init__(self, endpoint: str = "localhost:50052"):
        self.endpoint = endpoint
        self.credentials = get_credentials()
        self.channel = grpc.secure_channel(
            endpoint,
            self.credentials,
        )
        self.stub = admin_pb2_grpc.AdminServiceStub(self.channel)

    def create_namespace(
        self,
        name: str,
        backend: str,
        pattern: str,
        consistency: str = "eventual",
        cache_ttl: Optional[int] = None,
    ) -> Namespace:
        """Create a new namespace."""
        request = admin_pb2.CreateNamespaceRequest(
            name=name,
            backend=backend,
            pattern=pattern,
            consistency=consistency,
            cache_ttl=cache_ttl,
        )
        response = self.stub.CreateNamespace(request)
        return Namespace.from_proto(response)

    def list_namespaces(
        self,
        backend: Optional[str] = None,
        include_inactive: bool = False,
    ) -> List[Namespace]:
        """List all namespaces."""
        request = admin_pb2.ListNamespacesRequest(
            backend=backend,
            include_inactive=include_inactive,
        )
        response = self.stub.ListNamespaces(request)
        return [Namespace.from_proto(ns) for ns in response.namespaces]

    def describe_namespace(
        self,
        name: str,
        include_errors: bool = False,
        include_config: bool = False,
    ) -> Namespace:
        """Get detailed namespace information."""
        request = admin_pb2.DescribeNamespaceRequest(
            name=name,
            include_errors=include_errors,
            include_config=include_config,
        )
        response = self.stub.DescribeNamespace(request)
        return Namespace.from_proto(response)

    def check_backend_health(
        self,
        backend: Optional[str] = None,
    ) -> List[BackendHealth]:
        """Check health of backends."""
        request = admin_pb2.HealthCheckRequest(backend=backend)
        response = self.stub.CheckBackendHealth(request)
        return [BackendHealth.from_proto(h) for h in response.backends]

    def trace_session(
        self,
        session_id: str,
        min_latency_ms: Optional[int] = None,
    ):
        """Stream trace events for a session."""
        request = admin_pb2.TraceSessionRequest(
            session_id=session_id,
            min_latency_ms=min_latency_ms,
        )
        for event in self.stub.TraceSession(request):
            yield TraceEvent.from_proto(event)

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.channel.close()
```text

## Use-Case Recommendations

### ✅ When to Use CLI

- **Operational Tasks**: Health checks, session management, troubleshooting
- **Automation**: CI/CD pipelines, infrastructure-as-code, scripting
- **Quick Checks**: Rapid inspection without opening web UI
- **SSH Sessions**: Remote administration without GUI requirements
- **Development**: Local testing and debugging during development

### ❌ When CLI is Less Suitable

- **Complex Visualizations**: Graphs, charts, time-series plots (use web UI)
- **Interactive Exploration**: Clicking through related entities (web UI better)
- **Long-Running Monitoring**: Real-time dashboards (use web UI or Grafana)
- **Non-Technical Users**: Prefer graphical interfaces

### Migration to Web UI

The CLI validates the admin API design and provides immediate value. Web UI development can proceed in parallel:

1. **Phase 1**: CLI delivers all admin functionality
2. **Phase 2**: Web UI built using same Admin gRPC API
3. **Phase 3**: Both CLI and Web UI coexist (CLI for automation, UI for exploration)

## Configuration

### Configuration File Discovery

The CLI follows a hierarchical configuration search strategy:

1. **Explicit `--config` flag**: Highest priority, direct path to config file
2. **`.config.yaml` in current directory**: Checked first for project-specific config
3. **`.config.yaml` in parent directories**: Walks up the tree to find inherited config
4. **`~/.prism/config.yaml`**: User-level global configuration
5. **Command-line arguments**: Override any config file settings

**Example `.config.yaml` (project-level)**:
```
# .config.yaml - Project configuration for my-app namespace
namespace: my-app  # Default namespace for scoped commands
endpoint: localhost:50052

backend:
  type: postgres
  pattern: keyvalue
  consistency: strong
  cache_ttl: 300

# Sessions, config, metrics will auto-scope to this namespace unless --namespace specified
```text

**Example `~/.prism/config.yaml` (user-level)**:
```
# ~/.prism/config.yaml - Global CLI configuration
default_endpoint: localhost:50052
auth:
  method: mtls
  cert_path: ~/.prism/client.crt
  key_path: ~/.prism/client.key
  ca_path: ~/.prism/ca.crt

output:
  format: table  # table, json, yaml
  color: auto    # auto, always, never

timeouts:
  connect: 5s
  request: 30s

logging:
  level: info
  file: ~/.prism/cli.log
```text

**Usage pattern**:
```
# In project directory with .config.yaml (namespace: my-app):
cd ~/projects/my-app
prismctl session list          # Auto-scopes to my-app namespace
prismctl metrics summary       # Shows metrics for my-app
prismctl config show           # Shows my-app configuration

# Override with --namespace flag:
prismctl session list --namespace other-app

# Parent directory search:
cd ~/projects/my-app/src/handlers
prismctl session list          # Finds .config.yaml in ~/projects/my-app/
```text

### Environment Variables

```
# Override config file settings
export PRISM_ENDPOINT="prism.example.com:50052"
export PRISM_AUTH_METHOD="oauth2"
export PRISM_OUTPUT_FORMAT="json"
```text

## Performance and UX

### Performance Targets

- **Command Startup**: &lt;100ms cold start, &lt;50ms warm start
- **gRPC Calls**: &lt;10ms for simple queries, &lt;100ms for complex operations
- **Streaming**: Live trace output with &lt;10ms latency
- **Large Lists**: Pagination and streaming for 1000+ items

### UX Enhancements

- **Rich Formatting**: Colors, tables, trees, progress bars
- **Config File Discovery**: Automatic `.config.yaml` lookup in current and parent directories
- **Smart Defaults**: Sensible defaults for all optional parameters
- **Helpful Errors**: Clear error messages with suggested fixes
- **Autocomplete**: Shell completion for commands and options
- **Aliases**: Common shortcuts (e.g., `ns` for `namespace`, `be` for `backend`)

## Testing Strategy

### Unit Tests

```
# tests/test_namespace.py
from typer.testing import CliRunner
from prism_cli.main import app
from .fixtures.mock_grpc import MockAdminService

runner = CliRunner()

def test_namespace_create():
    with MockAdminService() as mock:
        mock.set_response("CreateNamespace", Namespace(
            name="test-ns",
            backend="postgres",
            pattern="keyvalue",
        ))

        result = runner.invoke(app, [
            "namespace", "create", "test-ns",
            "--backend", "postgres",
            "--pattern", "keyvalue",
        ])

        assert result.exit_code == 0
        assert "Created namespace 'test-ns'" in result.stdout

def test_namespace_list_json():
    with MockAdminService() as mock:
        mock.set_response("ListNamespaces", ListNamespacesResponse(
            namespaces=[
                Namespace(name="ns1", backend="postgres"),
                Namespace(name="ns2", backend="redis"),
            ]
        ))

        result = runner.invoke(app, ["namespace", "list", "--output", "json"])

        assert result.exit_code == 0
        data = json.loads(result.stdout)
        assert len(data) == 2
        assert data[0]["name"] == "ns1"
```text

### Integration Tests

```
# tests/integration/test_admin_client.py
import pytest
from prism_cli.client.admin import AdminClient

@pytest.fixture
def admin_client():
    """Connect to local test proxy."""
    return AdminClient(endpoint="localhost:50052")

def test_create_and_list_namespace(admin_client):
    # Create namespace
    ns = admin_client.create_namespace(
        name="test-integration",
        backend="sqlite",
        pattern="keyvalue",
    )
    assert ns.name == "test-integration"

    # List and verify
    namespaces = admin_client.list_namespaces()
    names = [ns.name for ns in namespaces]
    assert "test-integration" in names

    # Cleanup
    admin_client.delete_namespace("test-integration")
```text

## Deployment

### Installation

```
# Install via uv (development)
cd prism-cli
uv pip install -e .

# Install from package (production)
uv pip install prism-cli

# Verify installation
prismctl --version
prismctl --help
```text

### Shell Completion

```
# Bash
prismctl --install-completion bash

# Zsh
prismctl --install-completion zsh

# Fish
prismctl --install-completion fish
```text

## Migration Path

### Phase 1: Core CLI (Week 1-2)

- Namespace CRUD operations
- Backend health checks
- Session listing
- Basic metrics

**Deliverable**: Functional CLI covering 80% of admin use cases

### Phase 2: Advanced Features (Week 3-4)

- Session tracing (streaming)
- Shadow traffic management
- Configuration validation
- Metrics export

**Deliverable**: Complete feature parity with Admin gRPC API

### Phase 3: Polish and Documentation (Week 5-6)

- Comprehensive tests (unit + integration)
- Shell completion
- Man pages and documentation
- CI/CD integration examples

**Deliverable**: Production-ready CLI with excellent docs

### Phase 4: Web UI Development (Parallel)

- RFC-007: Web Admin UI specification
- Ember.js application using same Admin gRPC API
- CLI and web UI coexist and complement each other

## Monitoring and Observability

### CLI Usage Metrics

Track CLI adoption and usage patterns:

- **Command Usage**: Which commands are most popular
- **Error Rates**: Which commands fail most often
- **Latency**: gRPC call latency from CLI
- **Authentication**: Success/failure rates for auth

### Logging

CLI logs structured events:

```
{
  "timestamp": "2025-10-08T09:15:23.456Z",
  "level": "info",
  "command": "namespace create",
  "args": {"name": "my-app", "backend": "postgres"},
  "duration_ms": 234,
  "status": "success"
}
```text

## Security Considerations

- **mTLS by Default**: All gRPC connections use mutual TLS
- **Credential Storage**: Secure storage for certificates and tokens
- **Audit Logging**: All admin operations logged server-side
- **Least Privilege**: Role-based access control (RBAC) enforced by proxy
- **No Secrets in Logs**: Sanitize sensitive data from CLI logs

## Open Questions

1. **OAuth2 Integration**: Should CLI support OAuth2 device flow for cloud deployments?
2. **Plugin System**: Allow third-party commands to extend CLI?
3. **TUI Mode**: Add full-screen TUI for real-time monitoring?
4. **Multi-Proxy**: Manage multiple Prism proxies from single CLI?

## References

- RFC-003: Admin gRPC API specification
- Typer Documentation: https://typer.tiangolo.com/
- Rich Documentation: https://rich.readthedocs.io/
- Click Documentation: https://click.palletsprojects.com/

## Appendix: Command Reference

### All Commands

```
prismctl namespace create <name> [options]
prismctl namespace list [options]
prismctl namespace describe <name> [options]
prismctl namespace update <name> [options]
prismctl namespace delete <name> [options]

prismctl backend list [options]
prismctl backend health [options]
prismctl backend stats [options]
prismctl backend test <backend> [options]

prismctl session list [options]
prismctl session describe <session-id>
prismctl session kill <session-id>
prismctl session trace <session-id> [options]

prismctl config show [options]
prismctl config validate <file> [options]
prismctl config apply <file> [options]

prismctl metrics summary [options]
prismctl metrics namespace <name> [options]
prismctl metrics export [options]

prismctl shadow enable <namespace> [options]
prismctl shadow disable <namespace>
prismctl shadow status <namespace>

prismctl version
prismctl help [command]
```text

### Global Options

```
--endpoint <url>        # Proxy endpoint (default: localhost:50052)
--output <format>       # Output format: table, json, yaml
--no-color              # Disable colored output
--verbose, -v           # Verbose logging
--quiet, -q             # Suppress non-error output
--config <file>         # CLI config file
--help, -h              # Show help
```text

---

**Status**: Ready for Implementation
**Next Steps**:
1. Implement core CLI structure with Typer
2. Add namespace commands as proof-of-concept
3. Test against local Prism proxy
4. Iterate based on user feedback

```