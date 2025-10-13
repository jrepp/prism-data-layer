# prismctl - Prism CLI

Command-line interface for managing the Prism data access gateway with OIDC authentication.

## Features

- **OIDC Authentication**: Secure authentication using OAuth2 device code flow
- **Local Development**: Built-in support for Dex identity provider
- **Token Management**: Automatic token caching and refresh
- **Admin Operations**: Manage namespaces, sessions, and proxy health
- **Zero Installation**: Run via `uv` without installing

## Quick Start

### 1. Start Dex (Local Development)

```bash
# Start Dex identity provider
docker-compose -f local-dev/docker-compose.dex.yml up -d

# Verify Dex is running
curl http://localhost:5556/dex/.well-known/openid-configuration
```

### 2. Authenticate

```bash
# Device code flow (recommended)
uv run --with prismctl prism login

# Or using password flow (testing only)
uv run --with prismctl prism login --username dev@local.prism --password password
```

The device code flow will:
1. Display a verification URL and code
2. Automatically open your browser
3. Wait for you to authenticate
4. Save the token to `~/.prism/token`

### 3. Use CLI Commands

```bash
# Check authentication status
uv run --with prismctl prism whoami

# Check proxy health
uv run --with prismctl prism health

# List namespaces
uv run --with prismctl prism namespace list

# Show namespace details
uv run --with prismctl prism namespace show my-namespace

# List active sessions
uv run --with prismctl prism session list

# Filter sessions by namespace
uv run --with prismctl prism session list --namespace my-namespace

# Logout (remove token)
uv run --with prismctl prism logout
```

## Configuration

### Default Configuration (Local Development)

By default, prismctl uses:
- **OIDC Issuer**: `http://localhost:5556/dex`
- **Client ID**: `prismctl`
- **Proxy URL**: `http://localhost:8080`
- **Token Path**: `~/.prism/token`

### Custom Configuration

Create `~/.prism/config.yaml`:

```yaml
oidc:
  issuer: http://localhost:5556/dex
  client_id: prismctl
  client_secret: prismctl-secret
  scopes:
    - openid
    - profile
    - email
    - offline_access

proxy:
  url: http://localhost:8080
  timeout: 30

token_path: ~/.prism/token
```

Or use environment variable:

```bash
export PRISM_CONFIG=/path/to/config.yaml
```

## Test Users (Local Development)

When using Dex locally, these test users are available:

| Email | Username | Password | Description |
|-------|----------|----------|-------------|
| dev@local.prism | dev | password | Developer user |
| admin@local.prism | admin | password | Admin user |
| alice@example.com | alice | password | Test user 1 |
| bob@example.com | bob | password | Test user 2 |

**⚠️ WARNING**: Never use these credentials in production!

## Installation (Optional)

For regular use, install prismctl:

```bash
# Install via uv
cd cli
uv pip install -e .

# Or via pip
pip install -e .

# Then use directly
prism login
prism health
```

## Authentication Flows

### Device Code Flow (Recommended)

Secure flow for CLI applications:

```bash
prism login
```

1. CLI requests device code from Dex
2. Displays verification URL and code
3. Opens browser automatically
4. User authenticates in browser
5. CLI polls for token completion
6. Token saved locally

### Password Flow (Testing Only)

Direct username/password authentication:

```bash
prism login --username dev@local.prism --password password
```

**⚠️ WARNING**: Only use password flow for local testing. Never use in production.

## Token Management

### Token Storage

Tokens are stored in `~/.prism/token` with secure permissions (0600):

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "refresh_token": "ChQxMjM0NTY3ODkwMTI...",
  "id_token": "eyJhbGciOiJSUzI1NiIs...",
  "expires_at": "2025-10-13T10:30:00",
  "token_type": "Bearer"
}
```

### Token Lifecycle

1. **Login**: Authenticate and save token
2. **Auto-refresh**: Token automatically refreshed if within 5 minutes of expiry
3. **Expiry**: Token expires after 24 hours (configurable in Dex)
4. **Logout**: Remove token from disk

### Manual Token Inspection

```bash
# Check current authentication
prism whoami

# View token file
cat ~/.prism/token | jq

# Remove token
prism logout
```

## Commands Reference

### Authentication

```bash
prism login [--device-code | --username USER --password PASS]
prism logout
prism whoami
```

### Proxy Health

```bash
prism health        # Check if proxy is healthy
```

### Namespaces

```bash
prism namespace list               # List all namespaces
prism namespace show NAME          # Show namespace details
```

### Sessions

```bash
prism session list                 # List all sessions
prism session list --namespace NS  # Filter by namespace
```

## Development

### Running Tests

```bash
cd cli
pytest
```

### Linting

```bash
ruff check prismctl/
ruff format prismctl/
```

### Building Package

```bash
cd cli
python -m build
```

## Troubleshooting

### "Not authenticated" Error

```bash
# Check if token exists
ls -la ~/.prism/token

# Check if token is expired
prism whoami

# Re-authenticate
prism login
```

### Dex Connection Failed

```bash
# Check if Dex is running
curl http://localhost:5556/dex/.well-known/openid-configuration

# Start Dex
docker-compose -f local-dev/docker-compose.dex.yml up -d

# Check Dex logs
docker logs prism-dex
```

### Token Permission Error

```bash
# Fix permissions
chmod 600 ~/.prism/token
```

## Production Configuration

For production use with external OIDC provider:

```yaml
# ~/.prism/config.yaml
oidc:
  issuer: https://auth.example.com
  client_id: prism-production
  client_secret: <secret-from-vault>
  scopes:
    - openid
    - profile
    - email
    - offline_access

proxy:
  url: https://prism.example.com
  timeout: 30
```

Then authenticate:

```bash
prism login  # Uses device code flow with production issuer
```

## Architecture

```
┌─────────────┐                  ┌──────────┐
│   prismctl  │─────────────────▶│   Dex    │
│   (CLI)     │  1. Device Code  │  (OIDC)  │
└─────────────┘                  └──────────┘
      │                                ▲
      │ 2. Token                       │
      │                                │
      ▼                          3. Authenticate
┌─────────────┐                        │
│ ~/.prism/   │                  ┌──────────┐
│   token     │                  │ Browser  │
└─────────────┘                  └──────────┘
      │
      │ 4. Authenticated Requests
      │
      ▼
┌─────────────┐
│   Prism     │
│   Proxy     │
└─────────────┘
```

## Related Documentation

- [RFC-006: Admin CLI with OIDC](/rfc/rfc-006)
- [ADR-046: Dex IDP for Local Testing](/adr/adr-046)
- [Dex Documentation](https://dexidp.io/docs/)
- [OAuth 2.0 Device Code Flow](https://oauth.net/2/device-flow/)
