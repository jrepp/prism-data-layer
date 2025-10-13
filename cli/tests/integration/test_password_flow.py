"""Integration tests for password flow authentication.

Tests OIDC password flow (Resource Owner Password Credentials Grant) with
a live Dex identity provider.

See: MEMO-022 for comprehensive integration testing requirements.
"""

import pytest
import requests
from prismctl.auth import OIDCAuthenticator, OIDCConfig

from .dex_server import DexTestServer


class TestPasswordFlow:
    """Integration tests for password flow authentication."""

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

    def test_password_flow_success(self, config):
        """Test successful password authentication with valid credentials."""
        authenticator = OIDCAuthenticator(config)

        # Login with valid credentials
        token = authenticator.login_password(
            username="test@prism.local", password="password"
        )

        # Assertions
        assert token is not None, "Token should not be None"
        assert token.access_token is not None, "Access token should be present"
        assert len(token.access_token) > 0, "Access token should not be empty"
        assert not token.is_expired(), "Token should not be expired"

        # Verify token can be used for userinfo
        userinfo = authenticator.get_userinfo(token)
        assert userinfo is not None, "Userinfo should not be None"
        assert userinfo["email"] == "test@prism.local", "Email should match"

    def test_password_flow_invalid_username(self, config):
        """Test password flow fails with invalid username."""
        authenticator = OIDCAuthenticator(config)

        # Try to login with non-existent user
        with pytest.raises(requests.HTTPError) as exc_info:
            authenticator.login_password(
                username="nonexistent@prism.local", password="password"
            )

        # Verify it's a 401 or 400 error
        assert exc_info.value.response.status_code in [400, 401], \
            "Should fail with 400 or 401"

    def test_password_flow_invalid_password(self, config):
        """Test password flow fails with wrong password."""
        authenticator = OIDCAuthenticator(config)

        # Try to login with wrong password
        with pytest.raises(requests.HTTPError) as exc_info:
            authenticator.login_password(
                username="test@prism.local", password="wrongpassword"
            )

        # Verify it's a 401 error
        assert exc_info.value.response.status_code in [400, 401], \
            "Should fail with 400 or 401"

    def test_password_flow_empty_credentials(self, config):
        """Test password flow fails with empty credentials."""
        authenticator = OIDCAuthenticator(config)

        # Try to login with empty username
        with pytest.raises(requests.HTTPError):
            authenticator.login_password(username="", password="password")

        # Try to login with empty password
        with pytest.raises(requests.HTTPError):
            authenticator.login_password(username="test@prism.local", password="")

    def test_password_flow_different_users(self, config):
        """Test password flow works for multiple different users."""
        authenticator = OIDCAuthenticator(config)

        # Login as test user
        token1 = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        userinfo1 = authenticator.get_userinfo(token1)
        assert userinfo1["email"] == "test@prism.local"

        # Login as admin user
        token2 = authenticator.login_password(
            username="admin@prism.local", password="password"
        )
        userinfo2 = authenticator.get_userinfo(token2)
        assert userinfo2["email"] == "admin@prism.local"

        # Verify tokens are different
        assert token1.access_token != token2.access_token, \
            "Tokens for different users should be different"
