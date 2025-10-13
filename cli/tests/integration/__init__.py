"""Prismctl integration tests with real OIDC server.

This package contains integration tests for prismctl authentication flows
using a local Dex identity provider.

See: MEMO-022 for comprehensive integration testing requirements.
"""

from .dex_server import DexTestServer, temp_config

__all__ = ["DexTestServer", "temp_config"]
