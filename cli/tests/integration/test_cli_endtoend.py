"""CLI end-to-end integration tests for prismctl.

Tests complete prismctl workflows (login, whoami, logout) using subprocess
calls against a live Dex identity provider.

See: MEMO-022 for comprehensive integration testing requirements.
"""

import subprocess
from pathlib import Path

import pytest

from .dex_server import DexTestServer, temp_config


class TestCLIEndToEnd:
    """End-to-end CLI integration tests."""

    @pytest.fixture(scope="class")
    def dex(self):
        """Start Dex server for all tests in this class."""
        with DexTestServer() as server:
            yield server

    def test_login_logout_cycle(self, dex):
        """Test complete login/logout cycle via CLI."""
        with temp_config(dex.issuer_url) as config_path:
            # Login with password flow
            result = subprocess.run(
                [
                    "uv",
                    "run",
                    "prismctl",
                    "logout",
                ],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,  # cli directory
            )

            # Logout first to ensure clean state (ignore errors)
            assert result.returncode in [0, 1], \
                "Logout should succeed or fail gracefully"

            # Login
            result = subprocess.run(
                [
                    "uv",
                    "run",
                    "prismctl",
                    "login",
                    "--username",
                    "test@prism.local",
                    "--password",
                    "password",
                ],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            # Check login succeeded
            assert result.returncode == 0, \
                f"Login failed: {result.stderr}"
            assert "successfully" in result.stdout.lower() or \
                   "authenticated" in result.stdout.lower(), \
                f"Login output unexpected: {result.stdout}"

            # Check whoami
            result = subprocess.run(
                ["uv", "run", "prismctl", "whoami"],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            assert result.returncode == 0, \
                f"Whoami failed: {result.stderr}"
            assert "test@prism.local" in result.stdout, \
                f"Whoami should show test@prism.local, got: {result.stdout}"

            # Logout
            result = subprocess.run(
                ["uv", "run", "prismctl", "logout"],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            assert result.returncode == 0, \
                f"Logout failed: {result.stderr}"

            # Verify logout - whoami should fail
            result = subprocess.run(
                ["uv", "run", "prismctl", "whoami"],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            assert result.returncode != 0, \
                "Whoami should fail after logout"

    def test_whoami_without_login(self, dex):
        """Test whoami fails when not logged in."""
        with temp_config(dex.issuer_url):
            # Ensure logged out
            subprocess.run(
                ["uv", "run", "prismctl", "logout"],
                capture_output=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            # Try whoami without login
            result = subprocess.run(
                ["uv", "run", "prismctl", "whoami"],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            assert result.returncode != 0, \
                "Whoami should fail when not logged in"
            assert "no token" in result.stderr.lower() or \
                   "not authenticated" in result.stderr.lower() or \
                   "not logged in" in result.stderr.lower(), \
                f"Error message should indicate not logged in: {result.stderr}"

    def test_login_with_invalid_credentials(self, dex):
        """Test login fails with invalid credentials."""
        with temp_config(dex.issuer_url):
            # Try login with wrong password
            result = subprocess.run(
                [
                    "uv",
                    "run",
                    "prismctl",
                    "login",
                    "--username",
                    "test@prism.local",
                    "--password",
                    "wrongpassword",
                ],
                capture_output=True,
                text=True,
                cwd=Path(__file__).parent.parent.parent,
            )

            assert result.returncode != 0, \
                "Login should fail with wrong password"
            assert "failed" in result.stderr.lower() or \
                   "error" in result.stderr.lower() or \
                   "invalid" in result.stderr.lower(), \
                f"Error message should indicate failure: {result.stderr}"

    def test_multiple_login_cycles(self, dex):
        """Test multiple login/logout cycles work correctly."""
        with temp_config(dex.issuer_url):
            for i in range(3):
                # Login
                result = subprocess.run(
                    [
                        "uv",
                        "run",
                        "prismctl",
                        "login",
                        "--username",
                        "test@prism.local",
                        "--password",
                        "password",
                    ],
                    capture_output=True,
                    text=True,
                    cwd=Path(__file__).parent.parent.parent,
                )

                assert result.returncode == 0, \
                    f"Login cycle {i+1} failed: {result.stderr}"

                # Verify logged in
                result = subprocess.run(
                    ["uv", "run", "prismctl", "whoami"],
                    capture_output=True,
                    text=True,
                    cwd=Path(__file__).parent.parent.parent,
                )

                assert result.returncode == 0, \
                    f"Whoami cycle {i+1} failed"

                # Logout
                result = subprocess.run(
                    ["uv", "run", "prismctl", "logout"],
                    capture_output=True,
                    cwd=Path(__file__).parent.parent.parent,
                )

                assert result.returncode == 0, \
                    f"Logout cycle {i+1} failed"

    def test_login_different_users(self, dex):
        """Test login with different users sequentially."""
        with temp_config(dex.issuer_url):
            users = [
                ("test@prism.local", "test-user-id"),
                ("admin@prism.local", "admin-user-id"),
            ]

            for email, expected_id in users:
                # Logout first
                subprocess.run(
                    ["uv", "run", "prismctl", "logout"],
                    capture_output=True,
                    cwd=Path(__file__).parent.parent.parent,
                )

                # Login as user
                result = subprocess.run(
                    [
                        "uv",
                        "run",
                        "prismctl",
                        "login",
                        "--username",
                        email,
                        "--password",
                        "password",
                    ],
                    capture_output=True,
                    text=True,
                    cwd=Path(__file__).parent.parent.parent,
                )

                assert result.returncode == 0, \
                    f"Login failed for {email}: {result.stderr}"

                # Verify correct user
                result = subprocess.run(
                    ["uv", "run", "prismctl", "whoami"],
                    capture_output=True,
                    text=True,
                    cwd=Path(__file__).parent.parent.parent,
                )

                assert result.returncode == 0, \
                    f"Whoami failed for {email}"
                assert email in result.stdout, \
                    f"Whoami should show {email}, got: {result.stdout}"
