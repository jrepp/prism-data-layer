# Dockerfile for prism-admin (RFC-038 Raft-based HA control plane)
# Usage:
#   docker build -t prism-admin:latest -f cmd/prism-admin/Dockerfile .
#   docker run -p 8980:8980 -p 8990:8990 prism-admin:latest

FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.work go.work.sum ./
COPY cmd/prism-admin/go.mod cmd/prism-admin/go.sum cmd/prism-admin/
COPY pkg/plugin/go.mod pkg/plugin/go.sum pkg/plugin/

# Download dependencies
WORKDIR /build/cmd/prism-admin
RUN go mod download

# Copy source code
WORKDIR /build
COPY cmd/prism-admin/ cmd/prism-admin/
COPY pkg/plugin/ pkg/plugin/

# Build binary
WORKDIR /build/cmd/prism-admin
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o prism-admin .

# Runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/cmd/prism-admin/prism-admin /app/

# Create data directory for Raft persistence
RUN mkdir -p /data && chown -R nobody:nobody /data /app

USER nobody

# Expose ports
# 8980: Control Plane gRPC (external)
# 8990: Raft transport (cluster-internal)
EXPOSE 8980 8990

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/app/prism-admin", "health"]

# Default command
ENTRYPOINT ["/app/prism-admin"]
CMD ["serve"]
