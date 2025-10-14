# Multicast Registry Runner

The multicast registry runner is an executable that implements the [RFC-017 Multicast Registry Pattern](../../../../../../docs-cms/rfcs/RFC-017-multicast-registry-pattern.md) using the Prism data access layer.

## Overview

The multicast registry pattern combines identity registration with multicast messaging, enabling use cases like:
- Microservice discovery with selective broadcasting
- IoT device fleet management
- Presence systems with targeted notifications
- Agent pool coordination

## Architecture

The runner uses a **slot-based backend architecture** with three slots:

1. **Registry Slot** (required): KeyValue backend for identity storage
   - Stores identity metadata with optional TTL
   - Supports filtering and enumeration
   - Example: Redis, MemStore

2. **Messaging Slot** (required): PubSub backend for multicast delivery
   - Publishes messages to filtered identities
   - Low-latency fan-out
   - Example: NATS, Kafka

3. **Durability Slot** (optional): Queue backend for guaranteed delivery
   - Persists messages for offline identities
   - Retry with acknowledgment
   - Example: NATS with queue groups

## Usage

### Standalone Mode (File-Based Config)

Run with a YAML configuration file:

```bash
./multicast-registry-runner -config <path-to-config.yaml> [-v]
```

**Example**:
```bash
cd patterns/multicast_registry/cmd/multicast-registry-runner
go build -o multicast-registry-runner .
./multicast-registry-runner -config ../../examples/redis-nats.yaml
```

### Proxy Mode (Control Plane)

Connect to the Prism proxy control plane:

```bash
./multicast-registry-runner -proxy-addr <proxy-address> [-v]
```

**Example**:
```bash
./multicast-registry-runner -proxy-addr localhost:9090 -v
```

In proxy mode, the runner:
1. Connects to the proxy control plane via gRPC
2. Registers as a multicast-registry pattern
3. Waits for Initialize/Start/Stop commands
4. Receives backend configuration dynamically from proxy

## Configuration

### Configuration File Format

```yaml
namespaces:
  - name: device-fleet
    pattern: multicast-registry
    pattern_version: v1
    description: "Device fleet management with Redis + NATS"

    # Slot configuration
    slots:
      registry:
        backend: redis
        interfaces:
          - KeyValueBasicInterface
        config:
          address: localhost:6379
          password: ""
          db: 0
          prefix: "device:"

      messaging:
        backend: nats
        interfaces:
          - PubSubInterface
        config:
          servers:
            - nats://localhost:4222

      # Optional: durability slot for guaranteed delivery
      durability:
        backend: nats
        interfaces:
          - QueueInterface
        config:
          servers:
            - nats://localhost:4222
          queue_prefix: "durable."

    # Behavior configuration
    behavior:
      max_identities: 10000
      topic_prefix: "prism.multicast"
      retry_attempts: 3
      retry_delay_ms: 100
```

### Configuration Fields

#### Namespace

- `name`: Unique namespace identifier
- `pattern`: Must be "multicast-registry"
- `pattern_version`: Pattern version (e.g., "v1")
- `description`: Human-readable description

#### Slots

Each slot specifies:
- `backend`: Backend driver name (redis, nats, memstore, etc.)
- `interfaces`: Required interfaces (e.g., KeyValueBasicInterface)
- `config`: Backend-specific configuration (address, credentials, etc.)

#### Behavior

- `max_identities`: Maximum identities in registry (default: 10000)
- `topic_prefix`: Prefix for multicast topics (default: "prism.multicast")
- `retry_attempts`: Number of retry attempts for failed deliveries (default: 3)
- `retry_delay_ms`: Delay between retries in milliseconds (default: 100)

## Example Configurations

### Redis + NATS (Basic)

See `../../examples/redis-nats.yaml`

- **Registry**: Redis with local connection
- **Messaging**: NATS with local connection
- **Use case**: Development and testing

### Redis + NATS + Durability

See `../../examples/redis-nats-with-durability.yaml`

- **Registry**: Redis
- **Messaging**: NATS pub/sub
- **Durability**: NATS queues for guaranteed delivery
- **Use case**: Production with delivery guarantees

## Building

```bash
cd patterns/multicast_registry/cmd/multicast-registry-runner
go build -o multicast-registry-runner .
```

## Testing Locally

### Prerequisites

Start local Redis and NATS:

```bash
# Redis
docker run -d -p 6379:6379 redis:7

# NATS
docker run -d -p 4222:4222 nats:latest
```

Or use Podman:

```bash
# Redis
podman run -d -p 6379:6379 redis:7

# NATS
podman run -d -p 4222:4222 nats:latest
```

### Run the Pattern

```bash
./multicast-registry-runner -config ../../examples/redis-nats.yaml -v
```

Expected output:
```
2025/10/14 00:47:27 Loading configuration from ../../examples/redis-nats.yaml
2025/10/14 00:47:27 Running multicast registry pattern for namespace: device-fleet
2025/10/14 00:47:27 Pattern: multicast-registry (v1)
2025/10/14 00:47:27 [MULTICAST-REGISTRY] Initializing registry backend: redis
2025/10/14 00:47:27 [MULTICAST-REGISTRY] ✅ redis backend initialized and started
2025/10/14 00:47:27 [MULTICAST-REGISTRY] Initializing messaging backend: nats
2025/10/14 00:47:27 [MULTICAST-REGISTRY] ✅ nats backend initialized and started
2025/10/14 00:47:27 [MULTICAST-REGISTRY] ✅ Coordinator initialized
2025/10/14 00:47:27 [MULTICAST-REGISTRY] Coordinator ready for operations
2025/10/14 00:47:27 ✅ Multicast registry is running. Press Ctrl+C to stop.
```

## Integration with Proxy

When running in proxy mode, the pattern:

1. **Connects**: Establishes bidirectional gRPC stream to proxy control plane
2. **Registers**: Sends pattern metadata and interface declarations
3. **Initializes**: Receives backend configuration and slot assignments
4. **Starts**: Begins serving multicast registry operations
5. **Health Checks**: Responds to periodic health check requests
6. **Stops**: Gracefully shuts down on proxy shutdown command

The proxy orchestrates multiple pattern instances, routes client requests, and manages backend lifecycle.

## Development

### Project Structure

```
patterns/multicast_registry/
├── cmd/
│   └── multicast-registry-runner/
│       ├── main.go              # Runner executable
│       ├── go.mod               # Go module with local replace directives
│       └── README.md            # This file
├── backends/
│   ├── adapters.go              # Backend adapters (KV→Registry, PubSub→Messaging)
│   ├── redis_registry.go        # Redis registry backend (DEPRECATED: use adapters)
│   ├── nats_messaging.go        # NATS messaging backend (DEPRECATED: use adapters)
│   └── types.go                 # Common types
├── examples/
│   ├── redis-nats.yaml          # Basic Redis + NATS config
│   └── redis-nats-with-durability.yaml  # With durability slot
├── coordinator.go               # Core coordinator logic
├── slots.go                     # Slot interface definitions
├── config.go                    # Configuration structures
└── integration_test.go          # Integration tests
```

### Adding New Backends

To add support for a new backend (e.g., PostgreSQL for registry):

1. **Implement Plugin Interface**: Create driver in `pkg/drivers/<backend>/`
2. **Add to Runner**: Update `initializeBackend()` in `main.go`
3. **Test**: Create example config and integration test
4. **Document**: Update this README and create ADR if needed

No changes needed to coordinator or adapters - the slot architecture handles any backend that implements the required interfaces.

## See Also

- [RFC-017: Multicast Registry Pattern](../../../../../../docs-cms/rfcs/RFC-017-multicast-registry-pattern.md)
- [RFC-018: POC Implementation Strategy](../../../../../../docs-cms/rfcs/RFC-018-poc-implementation-strategy.md)
- [Pattern Control Plane Protocol](../../../../../../pkg/plugin/proxy_control_plane_server.go)
