---
title: "ADR-025: Container Plugin Model"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['architecture', 'deployment', 'containers', 'plugins', 'backends']
---

## Context

Prism needs a standardized way to deploy backend-specific functionality as containers:
- Kafka requires publisher and consumer containers
- NATS requires publisher and consumer containers
- Paged reader requires indexed reader consumer
- Transact write requires transaction processor and mailbox listener

**Requirements:**
- **Standard interface**: All containers follow same contract
- **Backend-specific logic**: Each backend has optimized implementation
- **Horizontal scaling**: Containers can be replicated
- **Health checking**: Containers report readiness and liveness
- **Configuration**: Containers configured via environment or config files
- **Observability**: Standard metrics and logging

## Decision

Implement **container plugin model** with standardized contracts:

1. **Plugin interface**: Standard gRPC or HTTP health/metrics endpoints
2. **Backend-specific containers**: Optimized for each backend
3. **Role-based deployment**: Publisher, Consumer, Processor, Listener roles
4. **Configuration via environment**: 12-factor app principles
5. **Docker/Kubernetes-ready**: Standard container packaging

## Rationale

### Container Architecture

┌─────────────────────────────────────────────────────────────┐
│                  Prism Core Proxy                           │
│                  (gRPC Server)                              │
└─────────────────────┬───────────────────────────────────────┘
                      │
        ┌─────────────┴─────────────┐
        │                           │
        │                           │
┌───────▼─────────┐        ┌────────▼────────┐
│ Backend Plugins │        │ Backend Plugins  │
│ (Containers)    │        │ (Containers)     │
│                 │        │                  │
│ ┌─────────────┐ │        │ ┌──────────────┐│
│ │   Kafka     │ │        │ │    NATS      ││
│ │  Publisher  │ │        │ │  Publisher   ││
│ └─────────────┘ │        │ └──────────────┘│
│                 │        │                  │
│ ┌─────────────┐ │        │ ┌──────────────┐│
│ │   Kafka     │ │        │ │    NATS      ││
│ │  Consumer   │ │        │ │  Consumer    ││
│ └─────────────┘ │        │ └──────────────┘│
└─────────────────┘        └──────────────────┘

┌───────────────────┐        ┌──────────────────┐
│ Reader Plugins    │        │ Transact Plugins │
│ (Containers)      │        │ (Containers)     │
│                   │        │                  │
│ ┌───────────────┐ │        │ ┌──────────────┐ │
│ │   Indexed     │ │        │ │  Transaction │ │
│ │    Reader     │ │        │ │  Processor   │ │
│ └───────────────┘ │        │ └──────────────┘ │
│                   │        │                  │
│                   │        │ ┌──────────────┐ │
│                   │        │ │   Mailbox    │ │
│                   │        │ │   Listener   │ │
│                   │        │ └──────────────┘ │
└───────────────────┘        └──────────────────┘
```text

### Plugin Contract

All container plugins implement standard interface:

```
// proto/prism/plugin/v1/plugin.proto
syntax = "proto3";

package prism.plugin.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/struct.proto";

// Health check service (required for all plugins)
service HealthService {
  // Liveness probe
  rpc Live(LiveRequest) returns (LiveResponse);

  // Readiness probe
  rpc Ready(ReadyRequest) returns (ReadyResponse);
}

message LiveRequest {}

message LiveResponse {
  bool alive = 1;
  google.protobuf.Timestamp timestamp = 2;
}

message ReadyRequest {}

message ReadyResponse {
  bool ready = 1;
  string message = 2;
  map<string, string> dependencies = 3;  // Dependency status
}

// Metrics service (required for all plugins)
service MetricsService {
  // Get plugin metrics (Prometheus format)
  rpc GetMetrics(MetricsRequest) returns (MetricsResponse);
}

message MetricsRequest {}

message MetricsResponse {
  string metrics = 1;  // Prometheus text format
}

// Plugin info service (required for all plugins)
service PluginInfoService {
  rpc GetInfo(InfoRequest) returns (InfoResponse);
}

message InfoRequest {}

message InfoResponse {
  string name = 1;
  string version = 2;
  string role = 3;  // "publisher", "consumer", "processor", "listener"
  string backend = 4;  // "kafka", "nats", "postgres", etc.
  map<string, string> capabilities = 5;
}
```text

### Environment Configuration

All plugins configured via environment variables:

```
# Common to all plugins
PRISM_PROXY_ENDPOINT=localhost:8980
PRISM_PLUGIN_ROLE=publisher
PRISM_BACKEND_TYPE=kafka
PRISM_NAMESPACE=production
PRISM_LOG_LEVEL=info
PRISM_LOG_FORMAT=json
PRISM_METRICS_PORT=9090

# Kafka-specific
KAFKA_BROKERS=localhost:9092,localhost:9093
KAFKA_TOPIC=events
KAFKA_CONSUMER_GROUP=prism-consumer
KAFKA_AUTO_OFFSET_RESET=earliest
KAFKA_COMPRESSION=snappy

# NATS-specific
NATS_URL=nats://localhost:4222
NATS_SUBJECT=events.>
NATS_QUEUE_GROUP=prism-consumers
NATS_STREAM=EVENTS

# Database-specific
DATABASE_URL=postgres://user:pass@localhost/db
DATABASE_POOL_SIZE=10
DATABASE_TABLE=events

# Mailbox-specific
MAILBOX_TABLE=mailbox
MAILBOX_POLL_INTERVAL=1s
MAILBOX_BATCH_SIZE=100
```text

### Kafka Plugin Containers

#### Kafka Publisher

```
// containers/kafka-publisher/src/main.rs

use rdkafka::producer::{FutureProducer, FutureRecord};
use tonic::transport::Channel;
use prism_proto::queue::v1::queue_service_client::QueueServiceClient;

struct KafkaPublisher {
    producer: FutureProducer,
    topic: String,
}

impl KafkaPublisher {
    async fn run(&self) -> Result<()> {
        // Connect to Prism proxy
        let mut client = QueueServiceClient::connect(
            env::var("PRISM_PROXY_ENDPOINT")?
        ).await?;

        // Create session
        let session = client.create_session(/* ... */).await?;

        // Subscribe to internal queue for messages to publish
        let messages = self.receive_from_internal_queue().await?;

        // Publish to Kafka
        for message in messages {
            let record = FutureRecord::to(&self.topic)
                .payload(&message.payload)
                .key(&message.key);

            self.producer.send(record, Duration::from_secs(5)).await?;
        }

        Ok(())
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt::init();

    let publisher = KafkaPublisher::new()?;
    publisher.run().await
}
```text

#### Kafka Consumer

```
// containers/kafka-consumer/src/main.rs

use rdkafka::consumer::{Consumer, StreamConsumer};
use rdkafka::Message;

struct KafkaConsumer {
    consumer: StreamConsumer,
    proxy_client: QueueServiceClient<Channel>,
}

impl KafkaConsumer {
    async fn run(&self) -> Result<()> {
        // Subscribe to Kafka topic
        self.consumer.subscribe(&[&self.topic])?;

        loop {
            match self.consumer.recv().await {
                Ok(message) => {
                    // Forward to Prism proxy
                    self.proxy_client.publish(PublishRequest {
                        topic: message.topic().to_string(),
                        payload: message.payload().unwrap().to_vec(),
                        offset: Some(message.offset()),
                        partition: Some(message.partition()),
                    }).await?;

                    // Commit offset
                    self.consumer.commit_message(&message, CommitMode::Async)?;
                }
                Err(e) => {
                    tracing::error!("Kafka error: {}", e);
                }
            }
        }
    }
}
```text

### NATS Plugin Containers

#### NATS Publisher

```
// containers/nats-publisher/src/main.rs

use async_nats::Client;

struct NatsPublisher {
    client: Client,
    subject: String,
}

impl NatsPublisher {
    async fn run(&self) -> Result<()> {
        // Connect to Prism proxy for source messages
        let mut proxy_client = PubSubServiceClient::connect(/* ... */).await?;

        // Subscribe to internal stream
        let mut stream = proxy_client.subscribe(/* ... */).await?.into_inner();

        // Publish to NATS
        while let Some(event) = stream.message().await? {
            self.client.publish(&self.subject, event.payload.into()).await?;
        }

        Ok(())
    }
}
```text

#### NATS Consumer

```
// containers/nats-consumer/src/main.rs

use async_nats::{Client, jetstream};

struct NatsConsumer {
    client: Client,
    stream: String,
    consumer: String,
}

impl NatsConsumer {
    async fn run(&self) -> Result<()> {
        let jetstream = jetstream::new(self.client.clone());

        let consumer = jetstream
            .get_stream(&self.stream)
            .await?
            .get_consumer(&self.consumer)
            .await?;

        let mut messages = consumer.messages().await?;

        // Connect to Prism proxy
        let mut proxy_client = PubSubServiceClient::connect(/* ... */).await?;

        while let Some(message) = messages.next().await {
            let message = message?;

            // Forward to Prism proxy
            proxy_client.publish(PublishRequest {
                topic: message.subject.clone(),
                payload: message.payload.to_vec(),
                metadata: Default::default(),
            }).await?;

            // Ack message
            message.ack().await?;
        }

        Ok(())
    }
}
```text

### Paged Reader Plugin

```
// containers/indexed-reader/src/main.rs

use sqlx::PgPool;

struct IndexedReader {
    pool: PgPool,
    table: String,
    index_column: String,
}

impl IndexedReader {
    async fn run(&self) -> Result<()> {
        // Connect to Prism proxy
        let mut proxy_client = ReaderServiceClient::connect(/* ... */).await?;

        // Process read requests
        loop {
            // Get read request from internal queue
            let request = self.receive_read_request().await?;

            // Query database with index
            let rows = sqlx::query(&format!(
                "SELECT * FROM {} WHERE {} > $1 ORDER BY {} LIMIT $2",
                self.table, self.index_column, self.index_column
            ))
            .bind(&request.cursor)
            .bind(request.page_size)
            .fetch_all(&self.pool)
            .await?;

            // Stream results back
            for row in rows {
                proxy_client.send_page(/* ... */).await?;
            }
        }
    }
}
```text

### Transact Writer Plugins

#### Transaction Processor

```
// containers/transact-processor/src/main.rs

use sqlx::{PgPool, Transaction};

struct TransactProcessor {
    pool: PgPool,
}

impl TransactProcessor {
    async fn process_transaction(&self, req: WriteRequest) -> Result<WriteResponse> {
        let mut tx = self.pool.begin().await?;

        // Write to data table
        let data_result = self.write_data(&mut tx, req.data).await?;

        // Write to mailbox table
        let mailbox_result = self.write_mailbox(&mut tx, req.mailbox).await?;

        // Commit transaction
        tx.commit().await?;

        Ok(WriteResponse {
            transaction_id: uuid::Uuid::new_v4().to_string(),
            committed: true,
            data_result,
            mailbox_result,
        })
    }

    async fn write_data(&self, tx: &mut Transaction<'_, Postgres>, data: DataWrite) -> Result<DataWriteResult> {
        let result = sqlx::query(&data.to_sql())
            .execute(&mut **tx)
            .await?;

        Ok(DataWriteResult {
            rows_affected: result.rows_affected() as i64,
            generated_values: Default::default(),
        })
    }

    async fn write_mailbox(&self, tx: &mut Transaction<'_, Postgres>, mailbox: MailboxWrite) -> Result<MailboxWriteResult> {
        let result = sqlx::query(
            "INSERT INTO mailbox (mailbox_id, message, metadata) VALUES ($1, $2, $3) RETURNING id, sequence"
        )
        .bind(&mailbox.mailbox_id)
        .bind(&mailbox.message)
        .bind(&mailbox.metadata)
        .fetch_one(&mut **tx)
        .await?;

        Ok(MailboxWriteResult {
            message_id: result.get("id"),
            sequence: result.get("sequence"),
        })
    }
}
```text

#### Mailbox Listener

```
// containers/mailbox-listener/src/main.rs

use sqlx::PgPool;

struct MailboxListener {
    pool: PgPool,
    mailbox_id: String,
    poll_interval: Duration,
}

impl MailboxListener {
    async fn run(&self) -> Result<()> {
        let mut last_sequence = 0i64;

        loop {
            // Poll for new messages
            let messages = sqlx::query_as::<_, MailboxMessage>(
                "SELECT * FROM mailbox WHERE mailbox_id = $1 AND sequence > $2 ORDER BY sequence LIMIT $3"
            )
            .bind(&self.mailbox_id)
            .bind(last_sequence)
            .bind(100)
            .fetch_all(&self.pool)
            .await?;

            for message in messages {
                // Process message
                self.process_message(&message).await?;

                // Update last sequence
                last_sequence = message.sequence;

                // Mark as processed
                sqlx::query("UPDATE mailbox SET processed = true WHERE id = $1")
                    .bind(&message.id)
                    .execute(&self.pool)
                    .await?;
            }

            tokio::time::sleep(self.poll_interval).await;
        }
    }

    async fn process_message(&self, message: &MailboxMessage) -> Result<()> {
        // Forward to downstream system, trigger workflow, etc.
        tracing::info!("Processing mailbox message: {:?}", message);
        Ok(())
    }
}
```text

### Docker Deployment

Each plugin is a separate Docker image:

```
# Dockerfile.kafka-publisher
FROM rust:1.75 as builder
WORKDIR /app
COPY . .
RUN cargo build --release --bin kafka-publisher

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/kafka-publisher /usr/local/bin/
ENTRYPOINT ["kafka-publisher"]
```text

### Docker Compose Example

```
# docker-compose.plugins.yml
version: '3.8'

services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "8980:8980"
      - "9090:9090"
    environment:
      - RUST_LOG=info

  kafka-publisher:
    image: prism/kafka-publisher:latest
    depends_on:
      - prism-proxy
      - kafka
    environment:
      - PRISM_PROXY_ENDPOINT=prism-proxy:8980
      - PRISM_PLUGIN_ROLE=publisher
      - KAFKA_BROKERS=kafka:9092
      - KAFKA_TOPIC=events
    deploy:
      replicas: 2

  kafka-consumer:
    image: prism/kafka-consumer:latest
    depends_on:
      - prism-proxy
      - kafka
    environment:
      - PRISM_PROXY_ENDPOINT=prism-proxy:8980
      - PRISM_PLUGIN_ROLE=consumer
      - KAFKA_BROKERS=kafka:9092
      - KAFKA_TOPIC=events
      - KAFKA_CONSUMER_GROUP=prism-consumers
    deploy:
      replicas: 3

  nats-publisher:
    image: prism/nats-publisher:latest
    depends_on:
      - prism-proxy
      - nats
    environment:
      - PRISM_PROXY_ENDPOINT=prism-proxy:8980
      - NATS_URL=nats://nats:4222
      - NATS_SUBJECT=events.>

  mailbox-listener:
    image: prism/mailbox-listener:latest
    depends_on:
      - prism-proxy
      - postgres
    environment:
      - PRISM_PROXY_ENDPOINT=prism-proxy:8980
      - DATABASE_URL=postgres://prism:password@postgres/prism
      - MAILBOX_ID=system
      - MAILBOX_POLL_INTERVAL=1s
```text

### Kubernetes Deployment

```
# k8s/kafka-consumer-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prism-kafka-consumer
spec:
  replicas: 3
  selector:
    matchLabels:
      app: prism-kafka-consumer
  template:
    metadata:
      labels:
        app: prism-kafka-consumer
    spec:
      containers:
      - name: kafka-consumer
        image: prism/kafka-consumer:latest
        env:
        - name: PRISM_PROXY_ENDPOINT
          value: "prism-proxy:8980"
        - name: KAFKA_BROKERS
          value: "kafka-0.kafka:9092,kafka-1.kafka:9092"
        - name: KAFKA_TOPIC
          value: "events"
        ports:
        - containerPort: 9090
          name: metrics
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8081
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```text

### Alternatives Considered

1. **Monolithic proxy with all backend logic**
   - Pros: Simpler deployment
   - Cons: Tight coupling, hard to scale independently
   - Rejected: Doesn't support horizontal scaling per backend

2. **Sidecar pattern**
   - Pros: Co-located with proxy
   - Cons: Resource overhead, complex orchestration
   - Rejected: Separate containers more flexible

3. **Embedded plugins (dynamic libraries)**
   - Pros: No network overhead
   - Cons: Language lock-in, version conflicts, crash propagation
   - Rejected: Containers provide better isolation

## Consequences

### Positive

- **Horizontal scaling**: Scale each plugin independently
- **Backend optimization**: Plugin optimized for specific backend
- **Isolation**: Plugin failures don't crash proxy
- **Standard deployment**: Docker/Kubernetes patterns
- **Observability**: Standard metrics/health endpoints
- **Language flexibility**: Plugins can be written in any language

### Negative

- **More containers**: Increased deployment complexity
- **Network overhead**: gRPC calls between proxy and plugins
- **Resource usage**: Each container has overhead

### Neutral

- **Configuration**: Environment variables (12-factor)
- **Monitoring**: Standard Prometheus metrics

## References

- [12-Factor App](https://12factor.net/)
- [Kubernetes Deployment Patterns](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
- ADR-008: Observability Strategy
- ADR-024: Layered Interface Hierarchy

## Revision History

- 2025-10-07: Initial draft and acceptance

```