# Dex Identity Provider for Local Development

Local OIDC identity provider for Prism development and testing.

## Quick Start

```bash
# Start Dex
docker-compose -f local-dev/docker-compose.dex.yml up -d

# Check status
docker logs prism-dex

# Stop Dex
docker-compose -f local-dev/docker-compose.dex.yml down
```

## Endpoints

- **Issuer**: http://localhost:5556/dex
- **Discovery**: http://localhost:5556/dex/.well-known/openid-configuration
- **Authorization**: http://localhost:5556/dex/auth
- **Token**: http://localhost:5556/dex/token
- **Userinfo**: http://localhost:5556/dex/userinfo
- **gRPC API**: localhost:5557
- **Telemetry**: http://localhost:5558/healthz

## Test Users

All passwords are: `password`

| Email | Username | User ID | Role |
|-------|----------|---------|------|
| dev@local.prism | dev | 08a8684b-db88-4b73-90a9-3cd1661f5466 | Developer |
| admin@local.prism | admin | 3b241101-e2bb-4255-8caf-4136c566a962 | Admin |
| alice@example.com | alice | 41331323-6f44-45e6-b3b9-2c4b60c02be5 | Test User |
| bob@example.com | bob | 7e8b6c3e-5e37-4f5e-9e0e-3e5f8f8f8f8f | Test User |

## OAuth2 Clients

### prismctl (CLI)

- **Client ID**: `prismctl`
- **Client Secret**: `prismctl-secret`
- **Grant Types**: authorization_code, refresh_token, password, device_code
- **Redirect URIs**:
  - http://localhost:8080/callback
  - http://localhost:5556/dex/callback

### prism-admin (Web UI)

- **Client ID**: `prism-admin`
- **Client Secret**: `prism-admin-secret`
- **Grant Types**: authorization_code, refresh_token
- **Redirect URIs**:
  - http://localhost:3000/callback
  - http://localhost:3000/auth/callback

## Token Configuration

- **Device Request Expiry**: 5 minutes
- **ID Token Expiry**: 24 hours
- **Refresh Token Valid If Not Used For**: 90 days
- **Refresh Token Absolute Lifetime**: 165 days

## Testing Authentication

### With prismctl

```bash
# Device code flow
uv run --with prismctl prism login

# Password flow
uv run --with prismctl prism login --username dev@local.prism --password password

# Check status
uv run --with prismctl prism whoami
```

### With curl (Password Flow)

```bash
# Get token
curl -X POST http://localhost:5556/dex/token \
  -d grant_type=password \
  -d username=dev@local.prism \
  -d password=password \
  -d client_id=prismctl \
  -d client_secret=prismctl-secret \
  -d scope="openid profile email offline_access"

# Get userinfo
curl http://localhost:5556/dex/userinfo \
  -H "Authorization: Bearer <access_token>"
```

### With curl (Device Code Flow)

```bash
# Step 1: Request device code
curl -X POST http://localhost:5556/dex/device/code \
  -d client_id=prismctl \
  -d scope="openid profile email offline_access"

# Step 2: Visit verification_uri and enter user_code

# Step 3: Poll for token
curl -X POST http://localhost:5556/dex/token \
  -d grant_type=urn:ietf:params:oauth:grant-type:device_code \
  -d device_code=<device_code> \
  -d client_id=prismctl
```

## Configuration

Configuration file: `local-dev/dex/config.yaml`

Key settings:
- **Storage**: In-memory (data lost on restart)
- **Password DB**: Enabled with static users
- **Skip Approval Screen**: Yes (for local dev)
- **Log Level**: Debug

## Security Considerations

⚠️ **WARNING**: This configuration is for local development only!

**DO NOT use in production:**
- Static passwords with weak hashing
- Skip approval screen enabled
- In-memory storage (no persistence)
- Hardcoded client secrets
- No TLS/HTTPS
- Debug logging enabled

## Troubleshooting

### Dex won't start

```bash
# Check logs
docker logs prism-dex

# Common issues:
# - Port 5556 already in use
# - Invalid config.yaml syntax
# - Missing config.yaml file
```

### Authentication fails

```bash
# Verify Dex is running
curl http://localhost:5556/dex/.well-known/openid-configuration

# Check if user exists in config.yaml
grep -A 5 "email:" local-dev/dex/config.yaml

# Verify password hash
# bcrypt hash for "password": $2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W
```

### Generate new password hash

```bash
# Install bcrypt
pip install bcrypt

# Generate hash
python -c "import bcrypt; print(bcrypt.hashpw(b'your_password', bcrypt.gensalt()).decode())"
```

## References

- [Dex Documentation](https://dexidp.io/docs/)
- [OAuth 2.0 Device Code Flow](https://oauth.net/2/device-flow/)
- [OpenID Connect Discovery](https://openid.net/specs/openid-connect-discovery-1_0.html)
