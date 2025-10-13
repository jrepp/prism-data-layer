"""Integration tests for userinfo endpoint.

Tests retrieving user information using OIDC tokens with a live Dex identity provider.

See: MEMO-022 for comprehensive integration testing requirements.
"""

from datetime import datetime, timedelta, timezone

import pytest
import requests
from prismctl.auth import OIDCAuthenticator, OIDCConfig, Token

from .dex_server import DexTestServer


class TestUserinfo:
    """Integration tests for userinfo endpoint."""

    @pytest.fixture(scope="class")
    def dex(self):
        """Start Dex server for all tests in this class."""
        with DexTestServer() as server:
            yield server

    @pytest.fixture
    def config(self, dex):
        """Create OIDC config for testing."""
        return OIDCConfig(
            issuer=dex.issuer_url,
            client_id="prismctl-test",
            client_secret="test-secret",
            scopes=["openid", "profile", "email", "offline_access"],
        )

    def test_get_userinfo_success(self, config):
        """Test retrieving user information with valid token."""
        authenticator = OIDCAuthenticator(config)

        # Get token
        token = authenticator.login_password(
            username="test@prism.local", password="password"
        )

        # Get userinfo
        userinfo = authenticator.get_userinfo(token)

        # Assertions
        assert userinfo is not None, "Userinfo should not be None"
        assert "email" in userinfo, "Userinfo should contain email"
        assert userinfo["email"] == "test@prism.local"
        assert "sub" in userinfo, "Userinfo should contain subject ID"
        assert userinfo["sub"] == "test-user-id", "Subject ID should match"

    def test_get_userinfo_contains_expected_claims(self, config):
        """Test that userinfo contains all expected OIDC standard claims."""
        authenticator = OIDCAuthenticator(config)

        token = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        userinfo = authenticator.get_userinfo(token)

        # Check for standard OIDC claims
        expected_claims = ["sub", "email"]
        for claim in expected_claims:
            assert claim in userinfo, f"Userinfo should contain '{claim}' claim"

    def test_get_userinfo_different_users(self, config):
        """Test that userinfo returns correct data for different users."""
        authenticator = OIDCAuthenticator(config)

        # Get info for test user
        token1 = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        userinfo1 = authenticator.get_userinfo(token1)

        # Get info for admin user
        token2 = authenticator.login_password(
            username="admin@prism.local", password="password"
        )
        userinfo2 = authenticator.get_userinfo(token2)

        # Verify different users have different info
        assert userinfo1["email"] != userinfo2["email"]
        assert userinfo1["sub"] != userinfo2["sub"]
        assert userinfo1["email"] == "test@prism.local"
        assert userinfo2["email"] == "admin@prism.local"

    def test_get_userinfo_with_expired_token(self, config):
        """Test userinfo fails with expired token."""
        authenticator = OIDCAuthenticator(config)

        # Create expired token
        expired_token = Token(
            access_token="invalid-expired-token",
            refresh_token=None,
            id_token=None,
            expires_at=datetime.now(timezone.utc) - timedelta(hours=1),
        )

        # Attempt to get userinfo should fail
        with pytest.raises(requests.HTTPError) as exc_info:
            authenticator.get_userinfo(expired_token)

        # Should be a 401 unauthorized error
        assert exc_info.value.response.status_code == 401

    def test_get_userinfo_with_invalid_token(self, config):
        """Test userinfo fails with invalid/malformed token."""
        authenticator = OIDCAuthenticator(config)

        # Create token with invalid access token
        invalid_token = Token(
            access_token="completely-invalid-token",
            refresh_token=None,
            id_token=None,
            expires_at=datetime.now(timezone.utc) + timedelta(hours=1),
        )

        # Attempt to get userinfo should fail
        with pytest.raises(requests.HTTPError) as exc_info:
            authenticator.get_userinfo(invalid_token)

        # Should be a 401 unauthorized error
        assert exc_info.value.response.status_code == 401

    def test_get_userinfo_after_token_refresh(self, config):
        """Test that userinfo works correctly after token refresh."""
        authenticator = OIDCAuthenticator(config)

        # Get initial token
        token = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        userinfo_before = authenticator.get_userinfo(token)

        # Refresh token
        refreshed_token = authenticator.refresh_token(token)
        userinfo_after = authenticator.get_userinfo(refreshed_token)

        # Userinfo should be the same
        assert userinfo_before["email"] == userinfo_after["email"]
        assert userinfo_before["sub"] == userinfo_after["sub"]

    def test_get_userinfo_with_empty_token(self, config):
        """Test userinfo fails gracefully with empty/None token."""
        authenticator = OIDCAuthenticator(config)

        # Create token with empty access token
        empty_token = Token(
            access_token="",
            refresh_token=None,
            id_token=None,
            expires_at=datetime.now(timezone.utc) + timedelta(hours=1),
        )

        # Should fail with appropriate error
        with pytest.raises((requests.HTTPError, ValueError)):
            authenticator.get_userinfo(empty_token)
