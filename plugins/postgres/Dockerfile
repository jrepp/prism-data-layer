# PostgreSQL Plugin Container
# Based on ADR-026: Distroless Container Images

# Build stage - Go compiler
FROM golang:1.23 as builder

WORKDIR /build

# Copy go.mod files for caching
COPY go.mod go.sum ./
COPY core/go.mod ../core/

# Download dependencies
RUN go mod download

# Copy source code
COPY core/ ../core/
COPY postgres/ .

# Build static binary with CGO enabled (required for pgx)
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -ldflags '-extldflags "-static"' \
    -o postgres-plugin \
    main.go

# Production runtime - distroless cc (for glibc)
# ADR-026: Use distroless for minimal attack surface
FROM gcr.io/distroless/cc-debian12:nonroot as production

# Copy binary from builder
COPY --from=builder /build/postgres-plugin /usr/local/bin/postgres-plugin

# Copy default config
COPY --from=builder /build/config.yaml /etc/prism/plugin.yaml

# Use non-root user (UID 65532)
USER nonroot:nonroot

# Control plane port (ADR-025)
EXPOSE 9090

ENTRYPOINT ["/usr/local/bin/postgres-plugin"]

# Debug variant - includes busybox for troubleshooting
FROM gcr.io/distroless/cc-debian12:debug-nonroot as debug

COPY --from=builder /build/postgres-plugin /usr/local/bin/postgres-plugin
COPY --from=builder /build/config.yaml /etc/prism/plugin.yaml

USER nonroot:nonroot

EXPOSE 9090

ENTRYPOINT ["/usr/local/bin/postgres-plugin"]
