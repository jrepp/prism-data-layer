#!/usr/bin/env bash
# Compare Docker image sizes for different optimization strategies

set -e

REGISTRY="prism"
VERSION="size-test"
BUILDER="${BUILDER:-docker}"  # Use docker by default, can override with BUILDER=podman

echo "Building MemStore with different optimization strategies..."
echo "Using builder: ${BUILDER}"
echo "=============================================================="
echo ""

cd "$(dirname "$0")/.."

# Build all variants
echo "1. Building MINIMAL variant (0-byte base image)..."
${BUILDER} build \
    --target minimal \
    -t ${REGISTRY}/memstore-plugin:${VERSION}-minimal \
    -f memstore/Dockerfile.optimized \
    .

echo ""
echo "2. Building UPX variant (compressed binary on scratch)..."
${BUILDER} build \
    --target upx-compressed \
    -t ${REGISTRY}/memstore-plugin:${VERSION}-upx \
    -f memstore/Dockerfile.optimized \
    .

echo ""
echo "3. Building DISTROLESS variant (current production)..."
${BUILDER} build \
    --target production \
    -t ${REGISTRY}/memstore-plugin:${VERSION}-distroless \
    -f memstore/Dockerfile.optimized \
    .

echo ""
echo "4. Building DEBUG variant (with shell)..."
${BUILDER} build \
    --target debug \
    -t ${REGISTRY}/memstore-plugin:${VERSION}-debug \
    -f memstore/Dockerfile.optimized \
    .

echo ""
echo "=============================================================="
echo "Image Size Comparison:"
echo "=============================================================="

${BUILDER} images --format "table {{.Repository}}:{{.Tag}}\t{{.Size}}" | \
    grep "${REGISTRY}/memstore-plugin:${VERSION}" | \
    sort -k2 -h

echo ""
echo "Detailed breakdown:"
echo "-------------------"

for variant in minimal upx distroless debug; do
    IMAGE="${REGISTRY}/memstore-plugin:${VERSION}-${variant}"
    SIZE=$(${BUILDER} images --format "{{.Size}}" "${IMAGE}")
    echo "${variant}: ${SIZE}"

    # Show layer breakdown
    echo "  Layers:"
    ${BUILDER} history --human --no-trunc "${IMAGE}" | head -n 10
    echo ""
done

echo ""
echo "Recommendations:"
echo "================"
echo ""
echo "✓ MINIMAL (0-byte base):"
echo "  - Smallest possible image"
echo "  - No shell, no debugging tools"
echo "  - Perfect for production when you don't need introspection"
echo "  - Size: ~7-8 MB (just the binary + config)"
echo ""
echo "✓ UPX (compressed on scratch):"
echo "  - 60-70% smaller than uncompressed"
echo "  - Slightly slower startup (~10-50ms decompression)"
echo "  - Best for size-constrained environments"
echo "  - Size: ~3-4 MB"
echo ""
echo "✓ DISTROLESS (current):"
echo "  - Small base (~2MB) with essential runtime files"
echo "  - Better for compliance/security scanning"
echo "  - No shell, minimal attack surface"
echo "  - Size: ~9-10 MB"
echo ""
echo "✓ DEBUG:"
echo "  - Includes busybox shell for debugging"
echo "  - Useful for troubleshooting in production"
echo "  - Larger but still minimal"
echo "  - Size: ~12-15 MB"
