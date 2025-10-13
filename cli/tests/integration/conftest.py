"""Pytest configuration for prismctl integration tests.

This module configures pytest for integration tests that require a live
Dex OIDC identity provider.

See: MEMO-022 for comprehensive integration testing requirements.
"""

import pytest


def pytest_configure(config):
    """Configure pytest with custom markers."""
    config.addinivalue_line(
        "markers",
        "integration: marks tests as integration tests (requiring Dex server)",
    )


@pytest.fixture(scope="session", autouse=True)
def setup_integration_tests():
    """Setup hook that runs once before all integration tests."""
    print("\n" + "=" * 70)
    print("Starting prismctl OIDC integration tests with Dex")
    print("=" * 70)
    yield
    print("\n" + "=" * 70)
    print("Completed prismctl OIDC integration tests")
    print("=" * 70)
