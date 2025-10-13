"""OIDC authentication with device code flow for prismctl."""

import json
import time
import webbrowser
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path

import requests

from .config import OIDCConfig


@dataclass
class Token:
    """OIDC token with metadata."""

    access_token: str
    refresh_token: str | None
    id_token: str | None
    expires_at: datetime
    token_type: str = "Bearer"

    def is_expired(self) -> bool:
        """Check if access token is expired."""
        return datetime.now(timezone.utc) >= self.expires_at

    def needs_refresh(self) -> bool:
        """Check if token should be refreshed (within 5 minutes of expiry)."""
        return datetime.now(timezone.utc) >= (self.expires_at - timedelta(minutes=5))

    def to_dict(self) -> dict:
        """Serialize to dictionary."""
        return {
            "access_token": self.access_token,
            "refresh_token": self.refresh_token,
            "id_token": self.id_token,
            "expires_at": self.expires_at.isoformat(),
            "token_type": self.token_type,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "Token":
        """Deserialize from dictionary."""
        return cls(
            access_token=data["access_token"],
            refresh_token=data.get("refresh_token"),
            id_token=data.get("id_token"),
            expires_at=datetime.fromisoformat(data["expires_at"]),
            token_type=data.get("token_type", "Bearer"),
        )


class OIDCAuthenticator:
    """OIDC authentication using device code flow."""

    def __init__(self, config: OIDCConfig) -> None:
        """Initialize OIDC authenticator with configuration.

        Args:
            config: OIDC configuration including issuer and client details
        """
        self.config = config
        self.issuer = config.issuer.rstrip("/")

        # Discover endpoints
        self._discover_endpoints()

    def _discover_endpoints(self) -> None:
        """Discover OIDC endpoints from issuer."""
        discovery_url = f"{self.issuer}/.well-known/openid-configuration"
        resp = requests.get(discovery_url, timeout=10)
        resp.raise_for_status()

        discovery = resp.json()
        self.token_endpoint = discovery["token_endpoint"]
        self.device_authorization_endpoint = discovery.get("device_authorization_endpoint")
        self.authorization_endpoint = discovery["authorization_endpoint"]
        self.userinfo_endpoint = discovery.get("userinfo_endpoint")

    def login_device_code(self, open_browser: bool = True) -> Token:
        """Login using device code flow.

        This is the recommended flow for CLI applications. The user will be
        shown a verification URL and code to enter in their browser.

        Args:
            open_browser: If True, automatically open the verification URL in browser

        Returns:
            Token with access_token, refresh_token, and metadata
        """
        if not self.device_authorization_endpoint:
            raise ValueError("Device code flow not supported by OIDC provider")

        # Step 1: Request device code
        scopes = self.config.scopes or ["openid", "profile", "email"]
        device_resp = requests.post(
            self.device_authorization_endpoint,
            data={
                "client_id": self.config.client_id,
                "scope": " ".join(scopes),
            },
            timeout=10,
        )
        device_resp.raise_for_status()
        device_data = device_resp.json()

        device_code = device_data["device_code"]
        user_code = device_data["user_code"]
        verification_uri = device_data["verification_uri"]
        verification_uri_complete = device_data.get("verification_uri_complete")
        expires_in = device_data["expires_in"]
        interval = device_data.get("interval", 5)

        # Display instructions to user
        print("\n🔐 Prism Authentication")
        print(f"{'=' * 60}")
        print("\n1. Open this URL in your browser:")
        print(f"   {verification_uri}")
        print("\n2. Enter this code:")
        print(f"   {user_code}")
        print(f"\nWaiting for authentication (code expires in {expires_in}s)...\n")

        # Optionally open browser
        if open_browser and verification_uri_complete:
            webbrowser.open(verification_uri_complete)
        elif open_browser:
            webbrowser.open(verification_uri)

        # Step 2: Poll for token
        deadline = time.time() + expires_in
        while time.time() < deadline:
            time.sleep(interval)

            token_resp = requests.post(
                self.token_endpoint,
                data={
                    "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
                    "device_code": device_code,
                    "client_id": self.config.client_id,
                },
                timeout=10,
            )

            if token_resp.status_code == 200:
                # Success!
                token_data = token_resp.json()
                return self._create_token(token_data)

            # Handle polling errors
            error_data = token_resp.json()
            error = error_data.get("error")

            if error == "authorization_pending":
                # User hasn't completed auth yet, keep polling
                continue
            if error == "slow_down":
                # Increase polling interval
                interval += 5
                continue
            if error == "expired_token":
                raise ValueError("Device code expired. Please try again.")
            if error == "access_denied":
                raise ValueError("Authentication denied by user.")
            raise ValueError(f"Authentication error: {error}")

        raise TimeoutError("Authentication timed out. Please try again.")

    def login_password(self, username: str, password: str) -> Token:
        """Login using resource owner password credentials (for testing only).

        WARNING: This flow is not recommended for production use. It's only
        included for local development and testing.

        Args:
            username: User's username or email
            password: User's password

        Returns:
            Token with access_token, refresh_token, and metadata
        """
        scopes = self.config.scopes or ["openid", "profile", "email"]
        token_resp = requests.post(
            self.token_endpoint,
            data={
                "grant_type": "password",
                "username": username,
                "password": password,
                "client_id": self.config.client_id,
                "client_secret": self.config.client_secret,
                "scope": " ".join(scopes),
            },
            timeout=10,
        )
        token_resp.raise_for_status()
        token_data = token_resp.json()

        return self._create_token(token_data)

    def refresh_token(self, token: Token) -> Token:
        """Refresh an expired access token using refresh token.

        Args:
            token: Token with refresh_token

        Returns:
            New token with updated access_token and expiry
        """
        if not token.refresh_token:
            raise ValueError("No refresh token available. Please login again.")

        token_resp = requests.post(
            self.token_endpoint,
            data={
                "grant_type": "refresh_token",
                "refresh_token": token.refresh_token,
                "client_id": self.config.client_id,
                "client_secret": self.config.client_secret,
            },
            timeout=10,
        )
        token_resp.raise_for_status()
        token_data = token_resp.json()

        return self._create_token(token_data)

    def get_userinfo(self, token: Token) -> dict:
        """Get user information from OIDC provider.

        Args:
            token: Valid access token

        Returns:
            User information dictionary
        """
        if not self.userinfo_endpoint:
            raise ValueError("Userinfo endpoint not available")

        resp = requests.get(
            self.userinfo_endpoint,
            headers={"Authorization": f"Bearer {token.access_token}"},
            timeout=10,
        )
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]

    def _create_token(self, token_data: dict) -> Token:
        """Create Token object from OIDC response."""
        expires_in = token_data.get("expires_in", 3600)
        expires_at = datetime.now(timezone.utc) + timedelta(seconds=expires_in)

        return Token(
            access_token=token_data["access_token"],
            refresh_token=token_data.get("refresh_token"),
            id_token=token_data.get("id_token"),
            expires_at=expires_at,
            token_type=token_data.get("token_type", "Bearer"),
        )


class TokenManager:
    """Manage token storage and retrieval."""

    def __init__(self, token_path: Path) -> None:
        """Initialize token manager.

        Args:
            token_path: Path to token file
        """
        self.token_path = token_path

    def save(self, token: Token) -> None:
        """Save token to disk with secure permissions."""
        self.token_path.parent.mkdir(parents=True, exist_ok=True)

        with self.token_path.open("w") as f:
            json.dump(token.to_dict(), f, indent=2)

        # Set secure permissions (owner read/write only)
        self.token_path.chmod(0o600)

    def load(self) -> Token | None:
        """Load token from disk."""
        if not self.token_path.exists():
            return None

        with self.token_path.open() as f:
            data = json.load(f)

        return Token.from_dict(data)

    def delete(self) -> None:
        """Delete stored token."""
        if self.token_path.exists():
            self.token_path.unlink()
