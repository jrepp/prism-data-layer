"""Prism API client."""

from typing import Optional

import requests

from .auth import Token, TokenManager
from .config import ProxyConfig


class PrismClient:
    """HTTP client for Prism proxy admin APIs."""

    def __init__(self, config: ProxyConfig, token: Optional[Token] = None):
        self.config = config
        self.token = token
        self.session = requests.Session()

        if token:
            self.session.headers["Authorization"] = f"Bearer {token.access_token}"

    def health(self) -> dict:
        """Check proxy health."""
        resp = self.session.get(f"{self.config.url}/health", timeout=self.config.timeout)
        resp.raise_for_status()
        return resp.json()

    def ready(self) -> dict:
        """Check if proxy is ready."""
        resp = self.session.get(f"{self.config.url}/ready", timeout=self.config.timeout)
        resp.raise_for_status()
        return resp.json()

    def metrics(self) -> str:
        """Get Prometheus metrics."""
        resp = self.session.get(f"{self.config.url}/metrics", timeout=self.config.timeout)
        resp.raise_for_status()
        return resp.text

    def list_namespaces(self) -> list[dict]:
        """List all namespaces."""
        resp = self.session.get(
            f"{self.config.url}/api/v1/namespaces", timeout=self.config.timeout
        )
        resp.raise_for_status()
        return resp.json()

    def get_namespace(self, name: str) -> dict:
        """Get namespace details."""
        resp = self.session.get(
            f"{self.config.url}/api/v1/namespaces/{name}", timeout=self.config.timeout
        )
        resp.raise_for_status()
        return resp.json()

    def list_sessions(self, namespace: Optional[str] = None) -> list[dict]:
        """List active sessions."""
        url = f"{self.config.url}/api/v1/sessions"
        if namespace:
            url += f"?namespace={namespace}"

        resp = self.session.get(url, timeout=self.config.timeout)
        resp.raise_for_status()
        return resp.json()
