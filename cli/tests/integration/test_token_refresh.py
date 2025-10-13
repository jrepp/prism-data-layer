"""Integration tests for token refresh flow.

Tests OIDC token refresh with a live Dex identity provider.

See: MEMO-022 for comprehensive integration testing requirements.
"""

from datetime import datetime, timedelta, timezone

import pytest
from prismctl.auth import OIDCAuthenticator, OIDCConfig, Token

from .dex_server import DexTestServer


class TestTokenRefresh:
    """Integration tests for token refresh functionality."""

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

    def test_token_refresh_success(self, config):
        """Test successful token refresh with valid refresh token."""
        authenticator = OIDCAuthenticator(config)

        # Get initial token via password flow
        old_token = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        old_access = old_token.access_token
        assert old_token.refresh_token is not None, \
            "Initial token should have refresh token"

        # Refresh token
        new_token = authenticator.refresh_token(old_token)

        # Assertions
        assert new_token is not None, "Refreshed token should not be None"
        assert new_token.access_token is not None, "New access token should be present"
        assert new_token.access_token != old_access, \
            "New access token should be different"
        assert not new_token.is_expired(), "New token should not be expired"

        # Verify new token works for userinfo
        userinfo = authenticator.get_userinfo(new_token)
        assert userinfo["email"] == "test@prism.local"

    def test_token_refresh_without_refresh_token(self, config):
        """Test refresh fails when no refresh_token is available."""
        authenticator = OIDCAuthenticator(config)

        # Create token without refresh token
        token = Token(
            access_token="test-access-token",
            refresh_token=None,  # No refresh token!
            id_token=None,
            expires_at=datetime.now(timezone.utc) + timedelta(hours=1),
        )

        # Attempt refresh should fail
        with pytest.raises(ValueError, match="No refresh token available"):
            authenticator.refresh_token(token)

    def test_token_refresh_with_invalid_refresh_token(self, config):
        """Test refresh fails with invalid refresh token."""
        authenticator = OIDCAuthenticator(config)

        # Create token with fake refresh token
        token = Token(
            access_token="test-access-token",
            refresh_token="invalid-refresh-token",
            id_token=None,
            expires_at=datetime.now(timezone.utc) - timedelta(hours=1),
        )

        # Attempt refresh should fail
        with pytest.raises(Exception):  # Could be HTTPError or other error
            authenticator.refresh_token(token)

    def test_refresh_token_extends_expiry(self, config):
        """Test that refreshed token has extended expiry time."""
        authenticator = OIDCAuthenticator(config)

        # Get initial token
        old_token = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        old_expiry = old_token.expires_at

        # Refresh token
        new_token = authenticator.refresh_token(old_token)
        new_expiry = new_token.expires_at

        # New token should have later expiry (or at least not earlier)
        # Note: In some OIDC implementations, expiry might be the same
        # if refresh happens very quickly, so we just check it's not earlier
        assert new_expiry >= old_expiry, \
            "Refreshed token should not expire before original"

    def test_refresh_preserves_user_identity(self, config):
        """Test that refresh preserves the user's identity."""
        authenticator = OIDCAuthenticator(config)

        # Get initial token for test user
        token1 = authenticator.login_password(
            username="test@prism.local", password="password"
        )
        userinfo1 = authenticator.get_userinfo(token1)

        # Refresh token
        token2 = authenticator.refresh_token(token1)
        userinfo2 = authenticator.get_userinfo(token2)

        # Identity should be preserved
        assert userinfo1["email"] == userinfo2["email"], \
            "User identity should be preserved after refresh"
        assert userinfo1["sub"] == userinfo2["sub"], \
            "Subject ID should be preserved after refresh"

    def test_multiple_consecutive_refreshes(self, config):
        """Test that tokens can be refreshed multiple times consecutively."""
        authenticator = OIDCAuthenticator(config)

        # Get initial token
        token = authenticator.login_password(
            username="test@prism.local", password="password"
        )

        # Perform multiple refreshes
        for i in range(3):
            new_token = authenticator.refresh_token(token)
            assert new_token.access_token != token.access_token, \
                f"Refresh {i+1} should produce different token"

            # Verify token still works
            userinfo = authenticator.get_userinfo(new_token)
            assert userinfo["email"] == "test@prism.local"

            token = new_token
