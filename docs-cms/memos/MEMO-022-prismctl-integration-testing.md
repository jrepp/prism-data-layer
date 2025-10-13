---
author: Claude Code
created: 2025-10-13
doc_uuid: e7a9c4f1-22b5-4d3e-8a1f-9b6c7d8e9f0a
id: memo-022
project_id: prism-data-layer
tags:
- testing
- cli
- oidc
- integration
- dx
- security
title: Prismctl OIDC Integration Testing Requirements
updated: 2025-10-13
---

# MEMO-022: Prismctl OIDC Integration Testing Requirements

## Context

Prismctl's authentication system (`cli/prismctl/auth.py`) has **40% code coverage** with unit tests. The uncovered 60% consists of OIDC integration flows that require a live identity provider:

- Device code flow (recommended for CLI)
- Password flow (local dev only)
- Token refresh flow
- Userinfo endpoint calls
- OIDC endpoint discovery

**Current State**:
```text
✅ Unit tests: 6/6 passing
✅ Token storage: Secure (600 permissions)
✅ Expiry detection: Working
✅ CLI commands: Functional
❌ OIDC flows: Untested (40% coverage)
```

**Why Integration Tests Matter**:
1. **Security**: OIDC is our primary authentication mechanism
2. **User Experience**: Login flow is first interaction with prismctl
3. **Reliability**: Token refresh must work seamlessly
4. **Compatibility**: Must work with Dex (local) and production IdPs

## Integration Testing Strategy

### Test Infrastructure

**Local Dex Server** (from [RFC-016](/rfc/rfc-016)):
```yaml
# tests/integration/docker-compose.dex.yml
services:
  dex:
    image: ghcr.io/dexidp/dex:v2.37.0
    container_name: prismctl-test-dex
    ports:
      - "5556:5556"  # HTTP
    volumes:
      - ./dex-config.yaml:/etc/dex/config.yaml:ro
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:5556/healthz"]
      interval: 2s
      timeout: 2s
      retries: 10
```

**Dex Test Configuration**:
```yaml
# tests/integration/dex-config.yaml
issuer: http://localhost:5556/dex

storage:
  type: memory  # In-memory for tests

web:
  http: 0.0.0.0:5556

staticClients:
  - id: prismctl-test
    name: "Prismctl Test Client"
    redirectURIs:
      - http://localhost:8080/callback
    secret: test-secret

connectors:
  - type: mockCallback
    id: mock
    name: Mock

enablePasswordDB: true
staticPasswords:
  - email: "test@prism.local"
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"  # "password"
    username: "test"
    userID: "test-user-id"
  - email: "admin@prism.local"
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"  # "password"
    username: "admin"
    userID: "admin-user-id"
```

### Test Scenarios

#### 1. **Device Code Flow** (Priority: HIGH)

**Test**: `test_device_code_flow_success`
```python
def test_device_code_flow_success():
    """Test successful device code authentication."""
    # Start local Dex server
    with DexTestServer() as dex:
        config = OIDCConfig(
            issuer=dex.issuer_url,
            client_id="prismctl-test",
            client_secret="test-secret",
        )

        authenticator = OIDCAuthenticator(config)

        # Mock browser interaction (auto-approve)
        with mock_device_approval(dex):
            token = authenticator.login_device_code(open_browser=False)

        # Assertions
        assert token.access_token is not None
        assert token.refresh_token is not None
        assert not token.is_expired()

        # Verify token works for userinfo
        userinfo = authenticator.get_userinfo(token)
        assert userinfo["email"] == "test@prism.local"
```

**Test**: `test_device_code_flow_timeout`
```python
def test_device_code_flow_timeout():
    """Test device code flow timeout without approval."""
    with DexTestServer() as dex:
        authenticator = OIDCAuthenticator(config)

        # Don't approve - should timeout
        with pytest.raises(TimeoutError, match="timed out"):
            authenticator.login_device_code(open_browser=False)
```

**Test**: `test_device_code_flow_denied`
```python
def test_device_code_flow_denied():
    """Test device code flow when user denies."""
    with DexTestServer() as dex:
        with mock_device_denial(dex):
            with pytest.raises(ValueError, match="denied by user"):
                authenticator.login_device_code(open_browser=False)
```

#### 2. **Password Flow** (Priority: MEDIUM)

**Test**: `test_password_flow_success`
```python
def test_password_flow_success():
    """Test successful password authentication."""
    with DexTestServer() as dex:
        authenticator = OIDCAuthenticator(config)

        token = authenticator.login_password(
            username="test@prism.local",
            password="password"
        )

        assert token.access_token is not None
        assert not token.is_expired()
```

**Test**: `test_password_flow_invalid_credentials`
```python
def test_password_flow_invalid_credentials():
    """Test password flow with wrong credentials."""
    with DexTestServer() as dex:
        authenticator = OIDCAuthenticator(config)

        with pytest.raises(requests.HTTPError):
            authenticator.login_password(
                username="test@prism.local",
                password="wrong"
            )
```

#### 3. **Token Refresh** (Priority: HIGH)

**Test**: `test_token_refresh_success`
```python
def test_token_refresh_success():
    """Test successful token refresh."""
    with DexTestServer() as dex:
        authenticator = OIDCAuthenticator(config)

        # Get initial token
        old_token = authenticator.login_password("test@prism.local", "password")
        old_access = old_token.access_token

        # Wait for token to need refresh (or mock expiry)
        time.sleep(1)

        # Refresh token
        new_token = authenticator.refresh_token(old_token)

        assert new_token.access_token != old_access
        assert not new_token.is_expired()
```

**Test**: `test_token_refresh_without_refresh_token`
```python
def test_token_refresh_without_refresh_token():
    """Test refresh fails when no refresh_token available."""
    token = Token(
        access_token="test",
        refresh_token=None,  # No refresh token!
        id_token=None,
        expires_at=datetime.now(timezone.utc) - timedelta(hours=1)
    )

    with pytest.raises(ValueError, match="No refresh token"):
        authenticator.refresh_token(token)
```

#### 4. **Userinfo Endpoint** (Priority: MEDIUM)

**Test**: `test_get_userinfo_success`
```python
def test_get_userinfo_success():
    """Test retrieving user information."""
    with DexTestServer() as dex:
        authenticator = OIDCAuthenticator(config)
        token = authenticator.login_password("test@prism.local", "password")

        userinfo = authenticator.get_userinfo(token)

        assert userinfo["email"] == "test@prism.local"
        assert userinfo["name"] is not None
        assert userinfo["sub"] == "test-user-id"
```

**Test**: `test_get_userinfo_expired_token`
```python
def test_get_userinfo_expired_token():
    """Test userinfo fails with expired token."""
    expired_token = Token(
        access_token="invalid",
        refresh_token=None,
        id_token=None,
        expires_at=datetime.now(timezone.utc) - timedelta(hours=1)
    )

    with pytest.raises(requests.HTTPError, match="401"):
        authenticator.get_userinfo(expired_token)
```

#### 5. **CLI End-to-End** (Priority: HIGH)

**Test**: `test_cli_login_logout_cycle`
```python
def test_cli_login_logout_cycle():
    """Test full login/logout cycle via CLI."""
    with DexTestServer() as dex:
        # Configure prismctl to use test Dex
        with temp_config(dex.issuer_url):
            # Login
            result = subprocess.run(
                ["uv", "run", "prismctl", "login",
                 "--username", "test@prism.local",
                 "--password", "password"],
                capture_output=True,
                text=True
            )

            assert result.returncode == 0
            assert "Authenticated successfully" in result.stdout

            # Check whoami
            result = subprocess.run(
                ["uv", "run", "prismctl", "whoami"],
                capture_output=True,
                text=True
            )

            assert result.returncode == 0
            assert "test@prism.local" in result.stdout

            # Logout
            result = subprocess.run(
                ["uv", "run", "prismctl", "logout"],
                capture_output=True,
                text=True
            )

            assert result.returncode == 0
            assert "Token removed" in result.stdout
```

### Test Utilities

**DexTestServer Context Manager**:
```python
# tests/integration/dex_server.py
import subprocess
import time
from contextlib import contextmanager

class DexTestServer:
    """Manage Dex test server lifecycle."""

    def __init__(self):
        self.issuer_url = "http://localhost:5556/dex"
        self.container_name = "prismctl-test-dex"

    def start(self):
        """Start Dex container."""
        subprocess.run([
            "podman", "compose",
            "-f", "tests/integration/docker-compose.dex.yml",
            "up", "-d"
        ], check=True)

        # Wait for health check
        self._wait_for_health()

    def stop(self):
        """Stop Dex container."""
        subprocess.run([
            "podman", "compose",
            "-f", "tests/integration/docker-compose.dex.yml",
            "down"
        ], check=True)

    def _wait_for_health(self, timeout=30):
        """Wait for Dex to be healthy."""
        import requests

        start = time.time()
        while time.time() - start < timeout:
            try:
                resp = requests.get(f"{self.issuer_url}/.well-known/openid-configuration")
                if resp.status_code == 200:
                    return
            except requests.ConnectionError:
                pass
            time.sleep(0.5)

        raise TimeoutError("Dex server did not become healthy")

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, *args):
        self.stop()

@contextmanager
def temp_config(issuer_url):
    """Create temporary prismctl config for testing."""
    import tempfile
    from pathlib import Path

    with tempfile.TemporaryDirectory() as tmpdir:
        config_path = Path(tmpdir) / "config.yaml"
        config_path.write_text(f"""
oidc:
  issuer: {issuer_url}
  client_id: prismctl-test
  client_secret: test-secret

proxy:
  url: http://localhost:8080

token_path: {tmpdir}/token
""")

        # Set environment variable
        import os
        old_config = os.environ.get("PRISM_CONFIG")
        os.environ["PRISM_CONFIG"] = str(config_path)

        try:
            yield config_path
        finally:
            if old_config:
                os.environ["PRISM_CONFIG"] = old_config
            else:
                del os.environ["PRISM_CONFIG"]
```

## Implementation Plan

### Phase 1: Infrastructure Setup (Week 1)
1. Create `tests/integration/` directory
2. Add `docker-compose.dex.yml` and `dex-config.yaml`
3. Implement `DexTestServer` utility class
4. Add Makefile target: `make test-prismctl-integration`

### Phase 2: Core Flow Tests (Week 2)
1. Implement device code flow tests (3 scenarios)
2. Implement password flow tests (2 scenarios)
3. Implement token refresh tests (2 scenarios)
4. Target: 70% coverage

### Phase 3: Edge Cases & CLI (Week 3)
1. Add userinfo endpoint tests (2 scenarios)
2. Implement CLI end-to-end test
3. Add error handling tests (network failures, timeouts)
4. Target: 85%+ coverage

### Phase 4: CI/CD Integration (Week 4)
1. Add integration tests to GitHub Actions
2. Run in parallel with acceptance tests
3. Cache Dex container image
4. Add coverage reporting

## CI/CD Integration

**GitHub Actions Workflow**:
```yaml
# .github/workflows/ci.yml (add new job)

test-prismctl-integration:
  name: Prismctl Integration Tests
  runs-on: ubuntu-latest

  steps:
    - name: checkout
      uses: actions/checkout@v4

    - name: setup-python
      uses: actions/setup-python@v5
      with:
        python-version: '3.11'

    - name: install-uv
      uses: astral-sh/setup-uv@v5

    - name: start-dex-server
      run: |
        cd cli/tests/integration
        docker compose -f docker-compose.dex.yml up -d

        # Wait for health check
        timeout 30 bash -c 'until wget -q --spider http://localhost:5556/dex/healthz; do sleep 1; done'

    - name: run-integration-tests
      run: |
        cd cli
        uv run pytest tests/integration/ -v --cov=prismctl.auth

    - name: upload-coverage
      uses: codecov/codecov-action@v5
      with:
        files: cli/coverage.xml
        flags: prismctl-integration
```

**Makefile Target**:
```makefile
# Makefile (add to testing section)

test-prismctl-integration: ## Run prismctl integration tests with Dex
	$(call print_blue,Starting Dex test server...)
	@cd cli/tests/integration && podman compose up -d
	@sleep 5  # Wait for Dex to be ready
	$(call print_blue,Running prismctl integration tests...)
	@cd cli && uv run pytest tests/integration/ -v --cov=prismctl.auth --cov-report=term-missing
	$(call print_blue,Stopping Dex test server...)
	@cd cli/tests/integration && podman compose down
	$(call print_green,Prismctl integration tests complete)
```

## Success Criteria

**Coverage Goals**:
- Unit tests: **40%** (current) → No change needed
- Integration tests: **60%** (new) → OIDC flows
- **Combined: 85%+ coverage** for `prismctl/auth.py`

**Test Metrics**:
- ✅ 15+ integration test scenarios
- ✅ All OIDC flows tested (device code, password, refresh)
- ✅ Error cases covered (timeouts, denials, invalid credentials)
- ✅ CLI end-to-end tests passing
- ✅ Tests run in CI/CD (< 2 minutes)
- ✅ Zero flaky tests

**Quality Gates**:
- All integration tests must pass before merge
- Coverage must not decrease
- Tests must be deterministic (no random failures)
- Dex server must start/stop reliably

## Security Considerations

**Test Credentials**:
- Use mock/test credentials only
- Never use production OIDC servers in tests
- Store test client secrets in test configs (not repo secrets)

**Token Handling**:
- Test tokens should be clearly marked as test data
- Use short expiry times in tests (1 minute)
- Clean up tokens after each test

**Network Isolation**:
- Dex should bind to localhost only
- No external network access required
- Tests should work offline (after image pull)

## References

- [ADR-046: Dex IdP for Local Testing](/adr/adr-046)
- [RFC-016: Local Development Infrastructure](/rfc/rfc-016)
- [MEMO-020: Parallel Testing and Build Hygiene](/memos/memo-020)
- Dex Documentation: https://dexidp.io/docs/
- OIDC Spec: https://openid.net/specs/openid-connect-core-1_0.html

## Next Steps

1. **Immediate**: Create test infrastructure (Dex compose file)
2. **Week 1**: Implement device code flow tests
3. **Week 2**: Implement remaining OIDC flow tests
4. **Week 3**: Add CLI end-to-end tests
5. **Week 4**: Integrate into CI/CD pipeline

**Target Completion**: 4 weeks from 2025-10-13 = **2025-11-10**