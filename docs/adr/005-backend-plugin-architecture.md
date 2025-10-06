# ADR-005: Backend Plugin Architecture

**Status**: Accepted

**Date**: 2025-10-05

**Deciders**: Core Team

**Tags**: backend, architecture, dx

## Context

Prism must support multiple backend storage engines (Postgres, Kafka, NATS, SQLite, Neptune) for different data abstractions (KeyValue, TimeSeries, Graph). Each backend has unique characteristics:

- **Postgres**: Relational, ACID transactions, SQL queries
- **Kafka**: Append-only log, high throughput, event streaming
- **NATS**: Lightweight messaging, JetStream for persistence
- **SQLite**: Embedded, file-based, perfect for local testing
- **Neptune**: Graph database, Gremlin/SPARQL queries

We need to:
1. Add new backends without changing application-facing APIs
2. Swap backends transparently (e.g., Postgres → Cassandra)
3. Reuse common functionality (connection pooling, retries, metrics)
4. Keep backend-specific code isolated

## Decision

Implement a **trait-based plugin architecture** where each data abstraction defines a trait, and backends implement the trait.

## Rationale

### Architecture

```rust
// Core abstraction trait
#[async_trait]
pub trait KeyValueBackend: Send + Sync {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()>;
    async fn get(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<Vec<Item>>;
    async fn delete(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<()>;
    async fn scan(&self, namespace: &str, id: &str) -> Result<ScanIterator>;
}

// Postgres implementation
pub struct PostgresKeyValue {
    pool: PgPool,
}

#[async_trait]
impl KeyValueBackend for PostgresKeyValue {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()> {
        // Postgres-specific implementation
        let mut tx = self.pool.begin().await?;
        for item in items {
            sqlx::query("INSERT INTO kv (namespace, id, key, value) VALUES ($1, $2, $3, $4)")
                .bind(namespace)
                .bind(id)
                .bind(&item.key)
                .bind(&item.value)
                .execute(&mut tx)
                .await?;
        }
        tx.commit().await?;
        Ok(())
    }
    // ... other methods
}

// Kafka implementation
pub struct KafkaKeyValue {
    producer: FutureProducer,
}

#[async_trait]
impl KeyValueBackend for KafkaKeyValue {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()> {
        // Kafka-specific implementation
        for item in items {
            let record = FutureRecord::to(&format!("kv-{}", namespace))
                .key(&format!("{}:{}", id, String::from_utf8_lossy(&item.key)))
                .payload(&item.value);
            self.producer.send(record, Duration::from_secs(5)).await?;
        }
        Ok(())
    }
    // ... other methods
}
```

### Backend Registry

```rust
pub struct BackendRegistry {
    keyvalue_backends: HashMap<String, Arc<dyn KeyValueBackend>>,
    timeseries_backends: HashMap<String, Arc<dyn TimeSeriesBackend>>,
    graph_backends: HashMap<String, Arc<dyn GraphBackend>>,
}

impl BackendRegistry {
    pub fn new() -> Self {
        let mut registry = Self::default();

        // Register built-in backends
        registry.register_keyvalue("postgres", Arc::new(PostgresKeyValue::new()));
        registry.register_keyvalue("kafka", Arc::new(KafkaKeyValue::new()));
        registry.register_keyvalue("sqlite", Arc::new(SqliteKeyValue::new()));

        registry
    }

    pub fn get_keyvalue(&self, backend_name: &str) -> Option<&Arc<dyn KeyValueBackend>> {
        self.keyvalue_backends.get(backend_name)
    }

    // Plugin registration (for third-party backends)
    pub fn register_keyvalue(&mut self, name: impl Into<String>, backend: Arc<dyn KeyValueBackend>) {
        self.keyvalue_backends.insert(name.into(), backend);
    }
}
```

### Namespace Configuration

```yaml
# namespace-config.yaml
namespaces:
  - name: user-profiles
    abstraction: keyvalue
    backend: postgres
    config:
      connection_string: postgres://localhost/prism
      pool_size: 20

  - name: user-events
    abstraction: timeseries
    backend: kafka
    config:
      brokers: localhost:9092
      topic_prefix: events
      partitions: 20
```

### Routing

```rust
pub struct Router {
    registry: BackendRegistry,
    namespace_configs: HashMap<String, NamespaceConfig>,
}

impl Router {
    pub async fn route_put(&self, namespace: &str, request: PutRequest) -> Result<PutResponse> {
        let config = self.namespace_configs.get(namespace)
            .ok_or_else(|| Error::NamespaceNotFound)?;

        let backend = self.registry.get_keyvalue(&config.backend)
            .ok_or_else(|| Error::BackendNotFound)?;

        backend.put(namespace, &request.id, request.items).await?;
        Ok(PutResponse { success: true })
    }
}
```

### Alternatives Considered

1. **Hard-coded backends**
   - Pros: Simple, no abstraction overhead
   - Cons: Can't add backends without changing core code
   - Rejected: Not extensible

2. **Dynamic library plugins (`.so`/`.dll`)**
   - Pros: True runtime plugins
   - Cons: ABI compatibility nightmares, unsafe, complex
   - Rejected: Over-engineered for our needs

3. **Separate microservices per backend**
   - Pros: Complete isolation
   - Cons: Network overhead, operational complexity
   - Rejected: Too much overhead for data path

4. **Enum dispatch**
   - Pros: Zero-cost abstraction
   - Cons: Still need to modify core code to add backends
   - Rejected: Not extensible enough

## Consequences

### Positive

- **Pluggable**: Add new backends by implementing trait
- **Swappable**: Change backend without changing client code
- **Testable**: Mock backends for unit tests
- **Type-safe**: Compiler enforces contract
- **Performance**: Trait objects have minimal overhead (~1 vtable indirection)

### Negative

- **Trait object complexity**: Must use `Arc<dyn Trait>` and `async_trait`
  - *Mitigation*: Well-documented patterns, helper macros
- **Common denominator**: Traits must work for all backends
  - *Mitigation*: Backend-specific features exposed via extension traits

### Neutral

- **Registration boilerplate**: Each backend needs registration code
- **Configuration variety**: Each backend has different config needs

## Implementation Notes

### Backend Interface Per Abstraction

**KeyValue**:
```rust
#[async_trait]
pub trait KeyValueBackend: Send + Sync {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()>;
    async fn get(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<Vec<Item>>;
    async fn delete(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<()>;
    async fn scan(&self, namespace: &str, id: &str, cursor: Option<Cursor>) -> Result<ScanResult>;
    async fn compare_and_swap(&self, namespace: &str, id: &str, key: &[u8], old: Option<&[u8]>, new: &[u8]) -> Result<bool>;
}
```

**TimeSeries**:
```rust
#[async_trait]
pub trait TimeSeriesBackend: Send + Sync {
    async fn append(&self, stream: &str, events: Vec<Event>) -> Result<()>;
    async fn query(&self, stream: &str, range: TimeRange, filter: Filter) -> Result<EventStream>;
    async fn tail(&self, stream: &str, from: Timestamp) -> Result<EventStream>;
    async fn create_stream(&self, stream: &str, config: StreamConfig) -> Result<()>;
}
```

**Graph**:
```rust
#[async_trait]
pub trait GraphBackend: Send + Sync {
    async fn create_node(&self, node: Node) -> Result<()>;
    async fn get_node(&self, id: &str) -> Result<Option<Node>>;
    async fn create_edge(&self, edge: Edge) -> Result<()>;
    async fn get_edges(&self, node_id: &str, direction: Direction, filters: EdgeFilters) -> Result<Vec<Edge>>;
    async fn traverse(&self, start: &str, query: TraversalQuery) -> Result<TraversalResult>;
}
```

### Backend Capabilities

Backends can declare capabilities:

```rust
pub struct BackendCapabilities {
    pub supports_transactions: bool,
    pub supports_compare_and_swap: bool,
    pub supports_range_scans: bool,
    pub max_item_size: usize,
    pub max_batch_size: usize,
}

pub trait Backend {
    fn capabilities(&self) -> BackendCapabilities;
}
```

### Extension Traits for Backend-Specific Features

```rust
// Postgres-specific features
#[async_trait]
pub trait PostgresBackendExt {
    async fn execute_sql(&self, query: &str) -> Result<QueryResult>;
}

impl PostgresBackendExt for PostgresKeyValue {
    async fn execute_sql(&self, query: &str) -> Result<QueryResult> {
        // Direct SQL access for advanced use cases
    }
}

// Usage
if let Some(pg) = backend.downcast_ref::<PostgresKeyValue>() {
    pg.execute_sql("SELECT * FROM kv WHERE ...").await?;
}
```

### Testing

```rust
// Mock backend for unit tests
pub struct MockKeyValue {
    data: Arc<Mutex<HashMap<(String, String, Vec<u8>), Vec<u8>>>>,
}

#[async_trait]
impl KeyValueBackend for MockKeyValue {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()> {
        let mut data = self.data.lock().unwrap();
        for item in items {
            data.insert((namespace.to_string(), id.to_string(), item.key.clone()), item.value);
        }
        Ok(())
    }
    // ... in-memory implementation
}

#[tokio::test]
async fn test_router() {
    let mut registry = BackendRegistry::new();
    registry.register_keyvalue("mock", Arc::new(MockKeyValue::new()));

    // Test without real databases
}
```

## References

- [Rust Async Trait](https://docs.rs/async-trait/)
- [Trait Objects in Rust](https://doc.rust-lang.org/book/ch17-02-trait-objects.html)
- ADR-001: Rust for the Proxy
- ADR-003: Protobuf as Single Source of Truth

## Revision History

- 2025-10-05: Initial draft and acceptance
