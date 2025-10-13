"""Tests for prismctl authentication."""

import json
from datetime import datetime, timedelta
from pathlib import Path

import pytest

from prismctl.auth import Token, TokenManager
from prismctl.config import OIDCConfig


def test_token_serialization(tmp_path):
    """Test token serialization and deserialization."""
    token = Token(
        access_token="test_access",
        refresh_token="test_refresh",
        id_token="test_id",
        expires_at=datetime(2025, 10, 13, 10, 0, 0),
    )

    # Serialize
    data = token.to_dict()
    assert data["access_token"] == "test_access"
    assert data["refresh_token"] == "test_refresh"

    # Deserialize
    token2 = Token.from_dict(data)
    assert token2.access_token == token.access_token
    assert token2.refresh_token == token.refresh_token
    assert token2.expires_at == token.expires_at


def test_token_expiry():
    """Test token expiry checks."""
    # Expired token
    expired = Token(
        access_token="test",
        refresh_token=None,
        id_token=None,
        expires_at=datetime.now() - timedelta(hours=1),
    )
    assert expired.is_expired()
    assert expired.needs_refresh()

    # Fresh token
    fresh = Token(
        access_token="test",
        refresh_token=None,
        id_token=None,
        expires_at=datetime.now() + timedelta(hours=1),
    )
    assert not fresh.is_expired()
    assert not fresh.needs_refresh()

    # Token expiring soon
    expiring = Token(
        access_token="test",
        refresh_token=None,
        id_token=None,
        expires_at=datetime.now() + timedelta(minutes=3),
    )
    assert not expiring.is_expired()
    assert expiring.needs_refresh()


def test_token_manager_save_load(tmp_path):
    """Test token save and load."""
    token_path = tmp_path / "token"
    manager = TokenManager(token_path)

    # Create and save token
    token = Token(
        access_token="test_access",
        refresh_token="test_refresh",
        id_token="test_id",
        expires_at=datetime(2025, 10, 13, 10, 0, 0),
    )
    manager.save(token)

    # Check file exists with secure permissions
    assert token_path.exists()
    assert oct(token_path.stat().st_mode)[-3:] == "600"

    # Load token
    loaded = manager.load()
    assert loaded is not None
    assert loaded.access_token == token.access_token
    assert loaded.refresh_token == token.refresh_token


def test_token_manager_load_missing(tmp_path):
    """Test loading non-existent token."""
    token_path = tmp_path / "token"
    manager = TokenManager(token_path)

    token = manager.load()
    assert token is None


def test_token_manager_delete(tmp_path):
    """Test token deletion."""
    token_path = tmp_path / "token"
    manager = TokenManager(token_path)

    # Save token
    token = Token(
        access_token="test",
        refresh_token=None,
        id_token=None,
        expires_at=datetime.now() + timedelta(hours=1),
    )
    manager.save(token)
    assert token_path.exists()

    # Delete token
    manager.delete()
    assert not token_path.exists()


def test_oidc_config():
    """Test OIDC configuration."""
    config = OIDCConfig(
        issuer="http://localhost:5556/dex",
        client_id="test-client",
    )

    assert config.issuer == "http://localhost:5556/dex"
    assert config.client_id == "test-client"
    assert "openid" in config.scopes
    assert "offline_access" in config.scopes
