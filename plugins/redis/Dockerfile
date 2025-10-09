# Redis Plugin Container
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
COPY redis/ .

# Build static binary (Redis client is pure Go, no CGO)
RUN CGO_ENABLED=0 GOOS=linux go build -a \
    -ldflags '-w -s -extldflags "-static"' \
    -o redis-plugin \
    main.go

# Production runtime - distroless static (no glibc needed)
# ADR-026: Use distroless for minimal attack surface
FROM gcr.io/distroless/static-debian12:nonroot as production

# Copy binary from builder
COPY --from=builder /build/redis-plugin /usr/local/bin/redis-plugin

# Copy default config
COPY --from=builder /build/config.yaml /etc/prism/plugin.yaml

# Use non-root user (UID 65532)
USER nonroot:nonroot

# Control plane port (ADR-025)
EXPOSE 9092

ENTRYPOINT ["/usr/local/bin/redis-plugin"]

# Debug variant - includes busybox for troubleshooting
FROM gcr.io/distroless/static-debian12:debug-nonroot as debug

COPY --from=builder /build/redis-plugin /usr/local/bin/redis-plugin
COPY --from=builder /build/config.yaml /etc/prism/plugin.yaml

USER nonroot:nonroot

EXPOSE 9092

ENTRYPOINT ["/usr/local/bin/redis-plugin"]
