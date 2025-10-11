# Multicast Registry Pattern

**Status**: ✅ POC Complete
**Version**: 1.0.0
**RFC**: [RFC-017: Multicast Registry Pattern](/rfc/rfc-017)

## Overview

The **Multicast Registry Pattern** is a composite pattern that combines identity registration, metadata-based filtering, and selective message broadcasting. It's designed for scenarios where you need to:

1. **Register** identities (users, devices, services) with metadata
2. **Enumerate** identities matching specific criteria (filters)
3. **Multicast** messages to filtered subsets of identities
4. **TTL-based expiration** for automatic cleanup of stale registrations

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│         Multicast Registry Pattern Coordinator          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Register(identity, metadata, ttl)                │ │
│  │    ├─> Store in Registry Backend (Redis)          │ │
│  │    └─> Track with TTL-based expiration            │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Enumerate(filter)                                │ │
│  │    ├─> Query Registry Backend                     │ │
│  │    ├─> Evaluate filter (native or client-side)    │ │
│  │    └─> Return matching identities                 │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Multicast(filter, payload)                       │ │
│  │    ├─> Enumerate identities matching filter       │ │
│  │    ├─> Fan-out to Messaging Backend (parallel)    │ │
│  │    └─> Aggregate delivery status                  │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │   Registry   │  │  Messaging   │  │  Durability  │ │
│  │   Slot       │  │    Slot      │  │  Slot (Opt)  │ │
│  │  (Redis)     │  │   (NATS)     │  │   (Kafka)    │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Backend Slots

The pattern uses a **3-slot architecture** allowing independent backend selection:

1. **Registry Slot**: Stores identity metadata with TTL support
   - **Redis**: High-performance key-value store with EXPIRE support
   - **PostgreSQL**: Relational storage with query capabilities
   - **SQLite**: Local testing and embedded use cases

2. **Messaging Slot**: Delivers multicast messages
   - **NATS**: Lightweight, high-performance pub/sub
   - **Kafka**: Durable, ordered message delivery
   - **Redis Pub/Sub**: Simple in-memory messaging

3. **Durability Slot** (Optional): Persistent message queue
   - **Kafka**: Guaranteed delivery with replay
   - **PostgreSQL**: Transactional message storage

## Core Operations

### Register

Register an identity with metadata and optional TTL:

```go
import "github.com/prism/patterns/multicast_registry"

// Create coordinator
config := multicast_registry.DefaultConfig()
coordinator, err := multicast_registry.NewCoordinator(
    config,
    registryBackend,   // Redis, PostgreSQL, etc.
    messagingBackend,  // NATS, Kafka, etc.
    nil,               // Optional durability backend
)

// Register with metadata and 5-minute TTL
err = coordinator.Register(ctx, "device-sensor-42", map[string]interface{}{
    "status": "online",
    "firmware_version": "1.2.3",
    "location": "warehouse-a",
}, 5*time.Minute)
```

### Enumerate

Query identities matching a filter:

```go
// Simple equality filter
filter := multicast_registry.NewFilter(map[string]interface{}{
    "status": "online",
    "location": "warehouse-a",
})

identities, err := coordinator.Enumerate(ctx, filter)
// Returns: []*Identity with metadata

// No filter = enumerate all
allIdentities, err := coordinator.Enumerate(ctx, nil)
```

### Multicast

Broadcast a message to filtered identities:

```go
// Multicast to all online devices in warehouse-a
filter := multicast_registry.NewFilter(map[string]interface{}{
    "status": "online",
    "location": "warehouse-a",
})

payload := []byte(`{"command":"restart","delay_seconds":10}`)
response, err := coordinator.Multicast(ctx, filter, payload)

// Check delivery results
fmt.Printf("Delivered: %d, Failed: %d\n",
    response.DeliveredCount,
    response.FailedCount)
```

### Unregister

Remove an identity:

```go
err = coordinator.Unregister(ctx, "device-sensor-42")
// Idempotent: succeeds even if identity doesn't exist
```

## Filter Expressions

### Simple Equality (Supported in POC)

```go
// Single condition
filter := NewFilter(map[string]interface{}{
    "status": "online",
})

// Multiple conditions (AND logic)
filter := NewFilter(map[string]interface{}{
    "status": "online",
    "tier": "premium",
    "region": "us-west",
})
```

### Advanced Filters (Future)

```go
import "github.com/prism/patterns/multicast_registry/filter"

// Complex AST-based filters
filterNode := &filter.AndNode{
    Children: []filter.FilterNode{
        &filter.EqualityNode{Field: "status", Value: "online"},
        &filter.GreaterThanNode{Field: "version", Value: "1.0.0"},
        &filter.OrNode{
            Children: []filter.FilterNode{
                &filter.EqualityNode{Field: "region", Value: "us-west"},
                &filter.EqualityNode{Field: "region", Value: "us-east"},
            },
        },
    },
}
```

## Use Cases

### 1. IoT Device Management

**Scenario**: Manage 10,000 IoT sensors, broadcast firmware updates selectively

```yaml
# examples/iot-device-management.yaml
pattern: multicast-registry
slots:
  registry: redis  # Fast metadata lookups
  messaging: nats  # Lightweight command broadcast
```

**Operations**:
- Device heartbeats → Register with 5-minute TTL
- Find outdated devices → Enumerate by firmware_version
- Broadcast update command → Multicast to filtered devices

**Benefits**:
- Automatic cleanup via TTL (dead devices disappear)
- Selective updates (only target specific firmware versions)
- Fast enumeration (<10ms for 10k devices)

### 2. User Presence & Chat

**Scenario**: Track online users in chat rooms, broadcast messages

```yaml
# examples/user-presence.yaml
pattern: multicast-registry
slots:
  registry: redis  # Short TTL (60s) for presence
  messaging: nats  # Real-time message delivery
```

**Operations**:
- User joins room → Register with 60s TTL
- Get room members → Enumerate by room
- Send message → Multicast to room members

**Benefits**:
- Automatic user timeout (missed heartbeats)
- Room-based isolation (filter by room metadata)
- Scales to 100k+ concurrent users

### 3. Microservice Discovery

**Scenario**: Service registry with health broadcasting

```yaml
# examples/service-discovery.yaml
pattern: multicast-registry
slots:
  registry: redis  # 30s TTL for health checks
  messaging: nats  # Config updates, drain commands
```

**Operations**:
- Service starts → Register with health metadata
- Find healthy services → Enumerate by service_name + health_status
- Broadcast config update → Multicast to specific service instances

**Benefits**:
- Automatic failure detection (TTL expiration)
- Load balancer integration (enumerate for endpoints)
- Graceful deployment (multicast drain commands)

## Performance

### Benchmarks

All benchmarks run on mock backends (in-memory):

| Operation | Throughput | Latency (p50) | Allocations |
|-----------|------------|---------------|-------------|
| **Register** | 1.93M ops/sec | 517 ns | 337 B/op |
| **Register with TTL** | 1.78M ops/sec | 563 ns | 384 B/op |
| **Enumerate (1000, no filter)** | - | 9.7 µs | 17.6 KB |
| **Enumerate (1000, with filter)** | - | 43.7 µs | 9.4 KB |
| **Multicast to 10** | - | 5.1 µs | 3.8 KB |
| **Multicast to 100** | - | 51.3 µs | 35.2 KB |
| **Multicast to 1000** | - | 528 µs | 297 KB |
| **Unregister** | 4.09M ops/sec | 245 ns | 23 B/op |
| **Filter Evaluation** | 33.8M ops/sec | 29.6 ns | 0 allocs |

**Key Insights**:
- Filter evaluation is **zero-allocation** (29ns per check)
- Multicast scales **sub-linearly** with goroutine fan-out
- Enumerate performance depends on filter complexity but stays well within targets
- All operations have minimal memory overhead

### Production Targets (with real backends)

| Metric | Target | Achieved (Integration Tests) |
|--------|--------|-------------------------------|
| **Enumerate 1000 identities** | <20ms | **93µs** (215x faster) |
| **Multicast to 100 identities** | <100ms | **24ms** (4x faster) |
| **Concurrent operations** | Race-free | ✅ All tests pass -race |

## Configuration

### Basic Configuration

```go
config := &multicast_registry.Config{
    PatternName:    "my-pattern",
    DefaultTTL:     5 * time.Minute,
    MaxIdentities:  10000,
    MaxFilterDepth: 5,
    MaxClauses:     20,
}
```

### Backend Slot Configuration

```go
// Redis registry backend
import "github.com/prism/patterns/multicast_registry/backends"

registryBackend, err := backends.NewRedisRegistryBackend(
    "localhost:6379", // addr
    "",               // password
    0,                // db
    "myapp:",         // key prefix
)

// NATS messaging backend
messagingBackend, err := backends.NewNATSMessagingBackend(
    []string{"nats://localhost:4222"},
)

// Create coordinator
coordinator, err := multicast_registry.NewCoordinator(
    config,
    registryBackend,
    messagingBackend,
    nil, // no durability backend
)
defer coordinator.Close()
```

## Testing

### Unit Tests

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Coverage**: 81.0% overall
- Coordinator: 81.1%
- Filter: 87.4%
- Backends: 76.3%

### Integration Tests

```bash
# Run integration tests (requires Redis + NATS)
go test -v -run TestIntegration

# Test with real backends (Docker Compose)
docker-compose up -d redis nats
go test -v -run TestIntegration
docker-compose down
```

### Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem

# Run specific benchmark
go test -bench=BenchmarkCoordinator_Multicast_1000 -benchmem -benchtime=10s
```

## Examples

See [examples/](./examples/) directory for complete use-case configurations:

- **[iot-device-management.yaml](./examples/iot-device-management.yaml)**: IoT sensors with firmware updates
- **[user-presence.yaml](./examples/user-presence.yaml)**: Chat rooms with user presence
- **[service-discovery.yaml](./examples/service-discovery.yaml)**: Microservice registry with health checks

## Deployment

### Docker Compose (Local Testing)

```yaml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  nats:
    image: nats:2-alpine
    ports:
      - "4222:4222"

  multicast-registry:
    build: .
    environment:
      - REGISTRY_BACKEND=redis
      - REGISTRY_ADDR=redis:6379
      - MESSAGING_BACKEND=nats
      - MESSAGING_SERVERS=nats://nats:4222
    depends_on:
      - redis
      - nats
```

### Kubernetes (Production)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: multicast-registry-config
data:
  config.yaml: |
    pattern: multicast-registry
    slots:
      registry:
        backend: redis
        config:
          addr: "redis-cluster:6379"
      messaging:
        backend: nats
        config:
          servers: ["nats://nats-cluster:4222"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: multicast-registry
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: coordinator
        image: prism/multicast-registry:1.0.0
        volumeMounts:
        - name: config
          mountPath: /etc/prism
```

## Monitoring

### Metrics

Export via Prometheus:

```go
// Register Prometheus metrics
prometheus.MustRegister(
    registeredIdentitiesGauge,
    multicastDeliveredHistogram,
    enumerateLatencyHistogram,
    ttlExpirationCounter,
)

// Expose /metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

**Key Metrics**:
- `multicast_registry_registered_identities` (gauge): Current count
- `multicast_registry_multicast_delivered_total` (histogram): Delivery count per operation
- `multicast_registry_enumerate_latency_seconds` (histogram): Query latency
- `multicast_registry_ttl_expiration_total` (counter): Expired identity rate

### Alerts

```yaml
alerts:
  - alert: HighTTLExpirationRate
    expr: rate(multicast_registry_ttl_expiration_total[5m]) > 100
    annotations:
      description: "High rate of TTL expirations (dead devices/services)"

  - alert: SlowEnumerateLatency
    expr: histogram_quantile(0.99, multicast_registry_enumerate_latency_seconds) > 0.02
    annotations:
      description: "Enumerate p99 latency >20ms (target threshold)"
```

## Troubleshooting

### High Memory Usage

**Symptom**: Coordinator memory grows over time
**Cause**: Identities not expiring (TTL too long or missing heartbeats)
**Fix**:
- Set appropriate TTL based on heartbeat frequency
- Monitor `multicast_registry_registered_identities` gauge
- Enable cleanup logging to see expired identity removal

### Slow Enumerate

**Symptom**: Enumerate takes >100ms for 1000 identities
**Cause**: Client-side filtering (backend doesn't support native filtering)
**Fix**:
- Use backend-native filtering (Redis Lua scripts) if available
- Reduce filter complexity (fewer conditions)
- Add indexes to backend (Redis secondary indexes, PostgreSQL columns)

### Multicast Delivery Failures

**Symptom**: `FailedCount > 0` in multicast response
**Cause**: Messaging backend timeouts or connection failures
**Fix**:
- Increase `retry_attempts` in messaging config
- Check messaging backend health (NATS, Kafka connectivity)
- Monitor `multicast_registry_delivery_failures_total` metric

## Contributing

See [POC-004-MULTICAST-REGISTRY.md](/pocs/poc-004-multicast-registry) for implementation tracking.

**Development Workflow**:
1. Write tests first (TDD approach)
2. Ensure race detector passes (`go test -race`)
3. Maintain >80% coverage
4. Run benchmarks to verify performance
5. Update examples and documentation

## Related Documentation

- [RFC-017: Multicast Registry Pattern](/rfc/rfc-017) - Pattern specification
- [RFC-018: POC Implementation Strategy](/rfc/rfc-018) - Implementation roadmap
- [MEMO-008: Message Schema Configuration](/memos/memo-008) - Schema management
- [RFC-022: prism-probe CLI Client](/rfc/rfc-022) - Testing tool

## License

See [LICENSE](../../LICENSE) for details.
