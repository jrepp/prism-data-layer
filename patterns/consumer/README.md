# Consumer Pattern

A message consumer pattern implemented using **pluggable backend drivers** and **interface slots**.

## Architecture

The consumer pattern depends **only on backend interfaces**, not concrete implementations. This enables:
- ✅ **Backend flexibility**: Swap Redis for PostgreSQL without changing consumer logic
- ✅ **Composability**: Mix and match backends (NATS + Redis, Kafka + PostgreSQL, etc.)
- ✅ **Testability**: Use MemStore for testing, real backends for production
- ✅ **Clear separation**: Pattern logic vs. backend implementation

### Slot-Based Design

The consumer pattern has **3 slots** that must be filled with backend drivers:

| Slot | Required Interface | Purpose |
|------|-------------------|---------|
| **MessageSource** | `PubSubInterface` or `QueueInterface` | Provides messages to consume |
| **StateStore** | `KeyValueBasicInterface` | Stores consumer state (offsets, checkpoints) |
| **DeadLetterQueue** | `QueueInterface` (optional) | Stores failed messages |

### Interface Contracts

```go
// Pattern depends only on these interfaces
type PubSubInterface interface {
    Subscribe(ctx context.Context, topic string, subscriberID string) (<-chan *Message, error)
    Unsubscribe(ctx context.Context, topic string, subscriberID string) error
}

type QueueInterface interface {
    Enqueue(ctx context.Context, queue string, payload []byte, metadata map[string]string) (string, error)
    Receive(ctx context.Context, queue string) (<-chan *Message, error)
}

type KeyValueBasicInterface interface {
    Set(key string, value []byte, ttl int64) error
    Get(key string) ([]byte, bool, error)
    Delete(key string) error
    Exists(key string) (bool, error)
}
```

## Plugins

Plugins bind specific backend drivers to slots. Each plugin is a self-contained module.

### Available Plugins

| Plugin | MessageSource | StateStore | Use Case |
|--------|---------------|------------|----------|
| **nats-redis** | NATS (PubSub) | Redis (KV) | Lightweight messaging, fast state |
| **kafka-postgres** | Kafka (Queue) | PostgreSQL (KV) | High-throughput, durable state |

### Creating Custom Plugins

```go
// 1. Import consumer and drivers
import (
    "github.com/jrepp/prism-data-layer/patterns/consumer"
    "github.com/jrepp/prism-data-layer/pkg/drivers/nats"
    "github.com/jrepp/prism-data-layer/pkg/drivers/redis"
)

// 2. Create plugin struct
type MyPlugin struct {
    consumer *consumer.Consumer
    nats     *nats.NATSPattern
    redis    *redis.RedisPattern
}

// 3. Initialize drivers and bind to slots
func (p *MyPlugin) Initialize(ctx context.Context, config *plugin.Config) error {
    // Initialize NATS
    p.nats = nats.New()
    if err := p.nats.Initialize(ctx, natsConfig); err != nil {
        return err
    }

    // Initialize Redis
    p.redis = redis.New()
    if err := p.redis.Initialize(ctx, redisConfig); err != nil {
        return err
    }

    // Bind slots: NATS provides PubSubInterface, Redis provides KeyValueBasicInterface
    return p.consumer.BindSlots(p.nats, p.redis, nil)
}
```

## Usage

### 1. Using NATS+Redis Plugin

```go
package main

import (
    "context"
    "log"

    "github.com/jrepp/prism-data-layer/patterns/consumer"
    natsredis "github.com/jrepp/prism-data-layer/patterns/consumer/plugins/nats-redis"
    "github.com/jrepp/prism-data-layer/pkg/plugin"
)

func main() {
    // Define consumer config
    config := consumer.Config{
        Name: "order-processor",
        Slots: consumer.SlotConfig{
            MessageSource: consumer.SlotBinding{
                Driver: "nats",
                Config: map[string]interface{}{
                    "url": "nats://localhost:4222",
                },
            },
            StateStore: consumer.SlotBinding{
                Driver: "redis",
                Config: map[string]interface{}{
                    "address": "localhost:6379",
                },
            },
        },
        Behavior: consumer.BehaviorConfig{
            ConsumerGroup: "order-processor-group",
            Topic:         "orders.created",
            MaxRetries:    3,
            AutoCommit:    true,
        },
    }

    // Create plugin
    plugin, err := natsredis.New(config)
    if err != nil {
        log.Fatal(err)
    }

    // Initialize (binds backends to slots)
    ctx := context.Background()
    pluginConfig := &plugin.Config{
        Backend: map[string]interface{}{
            "slots": map[string]interface{}{
                "message_source": config.Slots.MessageSource,
                "state_store":    config.Slots.StateStore,
            },
        },
    }

    if err := plugin.Initialize(ctx, pluginConfig); err != nil {
        log.Fatal(err)
    }

    // Set message processor
    plugin.SetProcessor(func(ctx context.Context, msg *plugin.Message) error {
        log.Printf("Processing message: %s", msg.MessageID)
        // Process message logic here
        return nil
    })

    // Start consuming
    if err := plugin.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Wait for interrupt
    // ...

    // Stop gracefully
    plugin.Stop(ctx)
}
```

### 2. Using Kafka+PostgreSQL Plugin

```go
import (
    kafkapostgres "github.com/jrepp/prism-data-layer/patterns/consumer/plugins/kafka-postgres"
)

config := consumer.Config{
    Name: "payment-processor",
    Slots: consumer.SlotConfig{
        MessageSource: consumer.SlotBinding{
            Driver: "kafka",
            Config: map[string]interface{}{
                "brokers": []string{"localhost:9092"},
            },
        },
        StateStore: consumer.SlotBinding{
            Driver: "postgres",
            Config: map[string]interface{}{
                "host":     "localhost",
                "port":     5432,
                "database": "prism",
            },
        },
    },
    Behavior: consumer.BehaviorConfig{
        ConsumerGroup: "payment-processor-group",
        Topic:         "payments.pending",
        MaxRetries:    5,
        BatchSize:     10, // Process 10 messages at a time
        AutoCommit:    false,
    },
}

plugin, _ := kafkapostgres.New(config)
// ... same as above
```

## Configuration

### YAML Configuration

```yaml
# consumer-config.yaml
name: "order-processor"
description: "Processes order events"

slots:
  message_source:
    driver: "nats"
    config:
      url: "nats://localhost:4222"
      max_reconnects: 10

  state_store:
    driver: "redis"
    config:
      address: "localhost:6379"
      pool_size: 10

  dead_letter_queue:
    driver: "nats"
    config:
      url: "nats://localhost:4222"

behavior:
  consumer_group: "order-processor-group"
  topic: "orders.created"
  max_retries: 3
  batch_size: 0
  auto_commit: true
  commit_interval: "5s"
```

## State Management

The consumer tracks state in the StateStore slot:

```json
{
  "offset": 12345,
  "last_message_id": "msg-abc-123",
  "last_updated": "2025-10-13T20:00:00Z",
  "retry_count": 0
}
```

State key format: `consumer:{group}:{topic}:{name}`

## Error Handling

1. **Transient Errors**: Retry up to `max_retries`
2. **Persistent Failures**: Send to Dead Letter Queue (if configured)
3. **State Persistence**: Auto-commit or manual commit based on config

## Testing

### With MemStore (In-Memory Testing)

```go
import (
    "github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
    "github.com/jrepp/prism-data-layer/pkg/drivers/nats"
)

// Use MemStore for state in tests
memstore := memstore.New()
nats := nats.New()

consumer.BindSlots(nats, memstore, nil)
```

### With Real Backends (Integration Testing)

Use testcontainers to spin up real backends:

```go
// Start Redis container
redisContainer := testcontainers.GenericContainer(...)
redis := redis.New()
redis.Initialize(ctx, redisConfig)

// Start NATS server
natsServer := natstest.RunServer(&natstest.DefaultTestOptions)
nats := nats.New()
nats.Initialize(ctx, natsConfig)

consumer.BindSlots(nats, redis, nil)
```

## Architecture Benefits

### ✅ Separation of Concerns

- **Pattern logic**: Consumer behavior, state management, retry logic
- **Backend logic**: Connection management, protocol implementation

### ✅ Composability

Mix and match backends without changing consumer code:

```go
// Development: NATS + MemStore (fast, local)
consumer.BindSlots(nats, memstore, nil)

// Staging: NATS + Redis (realistic)
consumer.BindSlots(nats, redis, nil)

// Production: Kafka + PostgreSQL (durable)
consumer.BindSlots(kafka, postgres, nil)
```

### ✅ Testability

```go
// Unit tests: Mock interfaces
mockSource := &MockPubSub{}
mockState := &MockKeyValue{}
consumer.BindSlots(mockSource, mockState, nil)

// Integration tests: Real backends
consumer.BindSlots(realNATS, realRedis, nil)
```

### ✅ Evolution

Add new backends without touching consumer pattern:

```go
// Future: Add S3 for state archival
consumer.BindSlots(nats, s3, nil)
```

## Related Patterns

- **[RFC-017: Multicast Registry Pattern](/rfc/rfc-017)** - Schematized backend slots
- **[MEMO-004: Backend Implementation Guide](/memos/memo-004)** - Backend comparison
- **[RFC-008: Proxy Plugin Architecture](/rfc/rfc-008)** - Plugin lifecycle

## TODOs

- [ ] Add batch processing support
- [ ] Implement message filtering
- [ ] Add metrics/observability
- [ ] Support multiple dead letter strategies
- [ ] Add consumer group rebalancing
- [ ] Implement exactly-once semantics
