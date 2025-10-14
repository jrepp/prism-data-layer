# Dockerfile for MemStore Plugin
# Based on ADR-026 (Distroless Container Images)
#
# Multi-stage build:
# - builder: Compile Go binary
# - production: Minimal distroless image (~2MB)
# - debug: Distroless with shell for debugging

# Build stage
FROM docker.io/library/golang:1.23-alpine AS builder

WORKDIR /build

# Copy core module first (dependency)
COPY core/ ./core/

# Copy memstore module
COPY memstore/ ./memstore/

# Build the binary
WORKDIR /build/memstore
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o memstore \
    ./cmd/memstore

# Production image - distroless static
FROM gcr.io/distroless/static-debian12:nonroot AS production

COPY --from=builder /build/memstore/memstore /memstore
COPY --from=builder /build/memstore/config.yaml /config.yaml

USER nonroot:nonroot
EXPOSE 9090

ENTRYPOINT ["/memstore"]
CMD ["-config", "/config.yaml"]

# Debug image - distroless with busybox shell
FROM gcr.io/distroless/base-debian12:debug-nonroot AS debug

COPY --from=builder /build/memstore/memstore /memstore
COPY --from=builder /build/memstore/config.yaml /config.yaml

USER nonroot:nonroot
EXPOSE 9090

ENTRYPOINT ["/memstore"]
CMD ["-config", "/config.yaml"]
