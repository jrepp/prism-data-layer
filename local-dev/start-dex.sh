#!/bin/bash
# Quick start script for Dex identity provider

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🚀 Starting Dex identity provider..."
echo

# Start Dex
docker-compose -f "$SCRIPT_DIR/docker-compose.dex.yml" up -d

# Wait for Dex to be ready
echo "⏳ Waiting for Dex to be ready..."
max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if curl -s http://localhost:5556/dex/.well-known/openid-configuration > /dev/null 2>&1; then
        echo
        echo "✅ Dex is ready!"
        echo
        echo "📋 Dex Endpoints:"
        echo "   Issuer: http://localhost:5556/dex"
        echo "   Discovery: http://localhost:5556/dex/.well-known/openid-configuration"
        echo "   Telemetry: http://localhost:5558/healthz"
        echo
        echo "👤 Test Users (password: 'password'):"
        echo "   • dev@local.prism (Developer)"
        echo "   • admin@local.prism (Admin)"
        echo "   • alice@example.com (Test User)"
        echo "   • bob@example.com (Test User)"
        echo
        echo "🔐 Try logging in:"
        echo "   uv run --with prismctl prism login"
        echo
        echo "📊 View logs:"
        echo "   docker logs -f prism-dex"
        echo
        echo "🛑 Stop Dex:"
        echo "   docker-compose -f $SCRIPT_DIR/docker-compose.dex.yml down"
        echo
        exit 0
    fi

    attempt=$((attempt + 1))
    sleep 1
done

echo "❌ Timeout waiting for Dex to start"
echo "Check logs with: docker logs prism-dex"
exit 1
