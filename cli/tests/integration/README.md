# Prismctl Integration Tests

This directory contains integration tests for prismctl OIDC authentication flows using a live Dex identity provider.

## Overview

These tests verify the 60% of prismctl authentication code that requires a real OIDC server:

- **Password Flow**: Resource Owner Password Credentials Grant
- **Token Refresh**: Using refresh tokens to get new access tokens
- **Userinfo Endpoint**: Retrieving user information with tokens
- **CLI End-to-End**: Complete prismctl command workflows (login, whoami, logout)

Combined with unit tests (40%), these integration tests achieve **85%+ coverage** for `prismctl/auth.py`.

## Prerequisites

- **Podman**: Container runtime (ADR-049)
- **uv**: Python package manager
- **Dex**: Automatically started via docker-compose

## Running Tests

### Via Makefile (Recommended)

```bash
# Run all integration tests with automatic Dex server management
make test-prismctl-integration
```

This target:
1. Ensures Podman machine is running
2. Starts Dex server via docker-compose
3. Runs pytest with coverage reporting
4. Stops Dex server (even if tests fail)

### Manually

```bash
# 1. Start Podman machine
podman machine start

# 2. Start Dex server
cd cli/tests/integration
podman compose -f docker-compose.dex.yml up -d

# 3. Wait for Dex to be healthy
sleep 5

# 4. Run tests
cd ../..
uv run pytest tests/integration/ -v --cov=prismctl.auth

# 5. Stop Dex server
cd tests/integration
podman compose -f docker-compose.dex.yml down
```

## Test Structure

```
tests/integration/
├── README.md                    # This file
├── __init__.py                  # Package initialization
├── conftest.py                  # Pytest configuration
├── dex_server.py                # DexTestServer utility
├── docker-compose.dex.yml       # Dex container config
├── dex-config.yaml              # Dex OIDC configuration
├── test_password_flow.py        # Password flow tests (5 tests)
├── test_token_refresh.py        # Token refresh tests (6 tests)
├── test_userinfo.py             # Userinfo endpoint tests (8 tests)
└── test_cli_endtoend.py         # CLI end-to-end tests (5 tests)
```

## Test Configuration

### Dex Server

- **Issuer**: `http://localhost:5556/dex`
- **Client ID**: `prismctl-test`
- **Client Secret**: `test-secret`

### Test Users

Two static users are configured in Dex:

| Username | Password | Email | User ID |
|----------|----------|-------|---------|
| test     | password | test@prism.local | test-user-id |
| admin    | password | admin@prism.local | admin-user-id |

## Coverage Goals

- **Unit Tests**: 40% (token handling, serialization)
- **Integration Tests**: 60% (OIDC flows)
- **Combined**: **85%+** for `prismctl/auth.py`

## Implementation Status

### ✅ Completed (Phase 1-3)

- [x] Test infrastructure (Dex compose file, config)
- [x] DexTestServer utility class
- [x] Makefile target (`make test-prismctl-integration`)
- [x] Password flow tests (5 scenarios)
- [x] Token refresh tests (6 scenarios)
- [x] Userinfo endpoint tests (8 scenarios)
- [x] CLI end-to-end tests (5 scenarios)

**Total: 24 integration tests**

### ⏳ Planned (Future Phases)

- [ ] Device code flow tests (requires browser mock)
- [ ] Error handling tests (network failures, timeouts)
- [ ] CI/CD integration (GitHub Actions)

## Troubleshooting

### Dex server won't start

```bash
# Check Podman machine status
podman machine list

# Restart Podman machine
podman machine stop
podman machine start

# Check Dex logs
cd cli/tests/integration
podman compose -f docker-compose.dex.yml logs dex
```

### Tests fail with connection errors

```bash
# Verify Dex is healthy
curl http://localhost:5556/dex/.well-known/openid-configuration

# Should return JSON with OIDC configuration
```

### Port 5556 already in use

```bash
# Check what's using the port
lsof -i :5556

# Stop existing Dex server
cd cli/tests/integration
podman compose -f docker-compose.dex.yml down
```

## References

- [MEMO-022: Prismctl OIDC Integration Testing Requirements](/memos/memo-022)
- [ADR-046: Dex IdP for Local Testing](/adr/adr-046)
- [RFC-016: Local Development Infrastructure](/rfc/rfc-016)
- [Dex Documentation](https://dexidp.io/docs/)
- [OIDC Specification](https://openid.net/specs/openid-connect-core-1_0.html)
