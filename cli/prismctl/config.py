"""Configuration management for prismctl."""

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

import yaml


@dataclass
class OIDCConfig:
    """OIDC configuration."""

    issuer: str
    client_id: str
    client_secret: Optional[str] = None
    scopes: list[str] = None

    def __post_init__(self):
        if self.scopes is None:
            self.scopes = ["openid", "profile", "email", "offline_access"]


@dataclass
class ProxyConfig:
    """Proxy server configuration."""

    url: str
    timeout: int = 30


@dataclass
class Config:
    """Prismctl configuration."""

    oidc: OIDCConfig
    proxy: ProxyConfig
    token_path: Path

    @classmethod
    def from_file(cls, path: Path) -> "Config":
        """Load configuration from YAML file."""
        with path.open() as f:
            data = yaml.safe_load(f)

        return cls(
            oidc=OIDCConfig(**data["oidc"]),
            proxy=ProxyConfig(**data["proxy"]),
            token_path=Path(data.get("token_path", "~/.prism/token")).expanduser(),
        )

    @classmethod
    def default(cls) -> "Config":
        """Create default configuration for local development."""
        return cls(
            oidc=OIDCConfig(
                issuer="http://localhost:5556/dex",
                client_id="prismctl",
                client_secret="prismctl-secret",
            ),
            proxy=ProxyConfig(url="http://localhost:8080"),
            token_path=Path("~/.prism/token").expanduser(),
        )

    @classmethod
    def load(cls) -> "Config":
        """Load configuration from default locations."""
        # Check for config file
        config_path = Path("~/.prism/config.yaml").expanduser()
        if config_path.exists():
            return cls.from_file(config_path)

        # Check environment variable
        if env_path := os.getenv("PRISM_CONFIG"):
            return cls.from_file(Path(env_path))

        # Fall back to default (local dev)
        return cls.default()


def ensure_prism_dir() -> Path:
    """Ensure ~/.prism directory exists."""
    prism_dir = Path("~/.prism").expanduser()
    prism_dir.mkdir(parents=True, exist_ok=True)
    return prism_dir
