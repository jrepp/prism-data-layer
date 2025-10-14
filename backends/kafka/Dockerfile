# Kafka Plugin Container
# Based on ADR-026: Distroless Container Images

# Build stage - Go compiler
FROM golang:1.23 as builder

WORKDIR /build

# Install librdkafka (required by confluent-kafka-go)
RUN apt-get update && apt-get install -y \
    librdkafka-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Copy go.mod files for caching
COPY go.mod go.sum ./
COPY core/go.mod ../core/

# Download dependencies
RUN go mod download

# Copy source code
COPY core/ ../core/
COPY kafka/ .

# Build with CGO enabled (required for librdkafka)
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -tags musl \
    -ldflags '-extldflags "-static"' \
    -o kafka-plugin \
    main.go

# Production runtime - distroless cc (for glibc and librdkafka)
# ADR-026: Use distroless for minimal attack surface
FROM gcr.io/distroless/cc-debian12:nonroot as production

# Copy binary from builder
COPY --from=builder /build/kafka-plugin /usr/local/bin/kafka-plugin

# Copy default config
COPY --from=builder /build/config.yaml /etc/prism/plugin.yaml

# Use non-root user (UID 65532)
USER nonroot:nonroot

# Control plane port (ADR-025)
EXPOSE 9091

ENTRYPOINT ["/usr/local/bin/kafka-plugin"]

# Debug variant - includes busybox for troubleshooting
FROM gcr.io/distroless/cc-debian12:debug-nonroot as debug

COPY --from=builder /build/kafka-plugin /usr/local/bin/kafka-plugin
COPY --from=builder /build/config.yaml /etc/prism/plugin.yaml

USER nonroot:nonroot

EXPOSE 9091

ENTRYPOINT ["/usr/local/bin/kafka-plugin"]
