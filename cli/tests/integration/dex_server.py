"""Dex test server management for prismctl integration tests.

This module provides utilities for managing a local Dex OIDC identity provider
during integration tests.

Usage:
    with DexTestServer() as dex:
        # Dex is now running at dex.issuer_url
        config = OIDCConfig(
            issuer=dex.issuer_url,
            client_id="prismctl-test",
            client_secret="test-secret",
        )
        # Run tests...
    # Dex is automatically stopped

See: MEMO-022 for comprehensive integration testing requirements.
"""

import os
import subprocess
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Generator


class DexTestServer:
    """Manage Dex test server lifecycle for integration tests."""

    def __init__(self, compose_file: str | None = None):
        """Initialize Dex test server manager.

        Args:
            compose_file: Path to docker-compose file (default: tests/integration/docker-compose.dex.yml)
        """
        if compose_file is None:
            # Default to docker-compose.dex.yml in same directory as this file
            compose_file = str(Path(__file__).parent / "docker-compose.dex.yml")

        self.compose_file = compose_file
        self.issuer_url = "http://localhost:5556/dex"
        self.container_name = "prismctl-test-dex"
        self._compose_dir = Path(compose_file).parent

    def start(self) -> None:
        """Start Dex container and wait for health check."""
        print(f"Starting Dex test server from {self.compose_file}...")

        # Start Dex using podman compose
        subprocess.run(
            ["podman", "compose", "-f", self.compose_file, "up", "-d"],
            check=True,
            cwd=self._compose_dir,
            capture_output=True,
        )

        # Wait for health check
        print("Waiting for Dex to be healthy...")
        self._wait_for_health()
        print(f"Dex test server ready at {self.issuer_url}")

    def stop(self) -> None:
        """Stop and remove Dex container."""
        print("Stopping Dex test server...")
        subprocess.run(
            ["podman", "compose", "-f", self.compose_file, "down"],
            check=True,
            cwd=self._compose_dir,
            capture_output=True,
        )
        print("Dex test server stopped")

    def _wait_for_health(self, timeout: int = 30) -> None:
        """Wait for Dex to be healthy.

        Args:
            timeout: Maximum time to wait in seconds

        Raises:
            TimeoutError: If Dex does not become healthy within timeout
        """
        import requests

        start = time.time()
        last_error = None

        while time.time() - start < timeout:
            try:
                # Check OIDC discovery endpoint
                resp = requests.get(
                    f"{self.issuer_url}/.well-known/openid-configuration",
                    timeout=2,
                )
                if resp.status_code == 200:
                    return
                last_error = f"HTTP {resp.status_code}"
            except requests.ConnectionError as e:
                last_error = str(e)
            except requests.Timeout as e:
                last_error = f"Timeout: {e}"

            time.sleep(0.5)

        raise TimeoutError(
            f"Dex server did not become healthy within {timeout}s. Last error: {last_error}"
        )

    def __enter__(self) -> "DexTestServer":
        """Start Dex server on context entry."""
        self.start()
        return self

    def __exit__(self, *args) -> None:
        """Stop Dex server on context exit."""
        self.stop()


@contextmanager
def temp_config(issuer_url: str) -> Generator[Path, None, None]:
    """Create temporary prismctl config for testing.

    This context manager creates a temporary directory with a prismctl config
    file and sets the PRISM_CONFIG environment variable to point to it.

    Args:
        issuer_url: OIDC issuer URL (typically from DexTestServer)

    Yields:
        Path to the temporary config file

    Example:
        with DexTestServer() as dex:
            with temp_config(dex.issuer_url) as config_path:
                # PRISM_CONFIG environment variable is set
                result = subprocess.run(["uv", "run", "prismctl", "login", ...])
    """
    import tempfile

    with tempfile.TemporaryDirectory() as tmpdir:
        config_path = Path(tmpdir) / "config.yaml"
        token_path = Path(tmpdir) / "token"

        # Create test configuration
        config_content = f"""oidc:
  issuer: {issuer_url}
  client_id: prismctl-test
  client_secret: test-secret
  scopes:
    - openid
    - profile
    - email
    - offline_access

proxy:
  url: http://localhost:8080
  timeout: 30

token_path: {token_path}
"""
        config_path.write_text(config_content)

        # Set environment variable
        old_config = os.environ.get("PRISM_CONFIG")
        os.environ["PRISM_CONFIG"] = str(config_path)

        try:
            yield config_path
        finally:
            # Restore original environment
            if old_config:
                os.environ["PRISM_CONFIG"] = old_config
            else:
                del os.environ["PRISM_CONFIG"]
