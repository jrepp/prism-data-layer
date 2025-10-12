---
date: 2025-10-05
deciders: Core Team
doc_uuid: dbf661b2-ebfc-493a-97d6-071f027c9f6e
id: adr-011
project_id: prism-data-layer
status: Accepted
tags:
- architecture
- planning
title: 'ADR-011: Implementation Roadmap and Next Steps'
---

## Context

We have comprehensive architecture documentation (ADRs 001-010), protobuf data model plan, and PRD. Now we need a concrete implementation roadmap that balances:

1. **Quick wins**: Show value early
2. **Risk reduction**: Validate core assumptions
3. **Incremental delivery**: Each step produces working software
4. **Learning**: Build expertise progressively

The roadmap must deliver a working system that demonstrates Prism's core value proposition within 4 weeks.

## Decision

Implement Prism in **6 major steps**, each building on the previous, with clear deliverables and success criteria.

## Step 1: Protobuf Foundation (Week 1, Days 1-3)

### Goal
Establish protobuf as single source of truth with code generation pipeline.

### Deliverables

1. **Create `proto/` directory structure**:
proto/
├── prism/
│   ├── options.proto         # Custom Prism tags
│   └── common/
│       ├── types.proto       # Timestamps, UUIDs, etc.
│       ├── errors.proto      # Error definitions
│       └── metadata.proto    # Item metadata
├── buf.yaml                   # Buf configuration
└── buf.lock
```text

2. **Implement Prism custom options**:
   - Message-level: `namespace`, `backend`, `access_pattern`, `estimated_*_rps`, etc.
   - Field-level: `pii`, `encrypt_at_rest`, `index`, `validation`
   - Service/RPC-level: `require_auth`, `timeout_ms`, `idempotent`

3. **Set up code generation**:
```
# Install buf
brew install bufbuild/buf/buf

# Generate Rust code
buf generate --template buf.gen.rust.yaml

# Generate Python code
buf generate --template buf.gen.python.yaml
```text

4. **Create `tooling/codegen`** module:
```
python -m tooling.codegen generate
# → Generates Rust, Python, TypeScript from proto
```text

### Success Criteria
- ✅ `prism/options.proto` compiles without errors
- ✅ Rust code generates successfully with `prost`
- ✅ Can import generated Rust code in a test program
- ✅ Buf lint passes with zero warnings

### Files to Create
- `proto/prism/options.proto` (~200 lines)
- `proto/prism/common/*.proto` (~150 lines total)
- `proto/buf.yaml` (~30 lines)
- `tooling/codegen/generator.py` (~100 lines)

---

## Step 2: Rust Proxy Skeleton (Week 1, Days 4-5)

### Goal
Create minimal gRPC server in Rust that can accept requests and return dummy responses.

### Deliverables

1. **Initialize Rust workspace**:
```
cargo new --lib proxy
cd proxy
```text

2. **Add dependencies** (`Cargo.toml`):
```
[dependencies]
tokio = { version = "1.35", features = ["full"] }
tonic = "0.10"
prost = "0.12"
tower = "0.4"
tracing = "0.1"
tracing-subscriber = "0.3"
```text

3. **Implement health check service**:
```
// proxy/src/health.rs
pub struct HealthService;

#[tonic::async_trait]
impl HealthCheck for HealthService {
    async fn check(&self, _req: Request<()>) -> Result<Response<HealthCheckResponse>> {
        Ok(Response::new(HealthCheckResponse { status: "healthy" }))
    }
}
```text

4. **Create main server**:
```
// proxy/src/main.rs
#[tokio::main]
async fn main() -> Result<()> {
    let addr = "0.0.0.0:8980".parse()?;
    let health_svc = HealthService::default();

    Server::builder()
        .add_service(HealthServer::new(health_svc))
        .serve(addr)
        .await?;

    Ok(())
}
```text

5. **Add basic logging**:
```
tracing_subscriber::fmt()
    .with_target(false)
    .compact()
    .init();
```text

### Success Criteria
- ✅ `cargo build` succeeds
- ✅ Server starts on port 8980
- ✅ Health check responds: `grpcurl localhost:8980 Health/Check`
- ✅ Logs appear in JSON format

### Files to Create
- `proxy/Cargo.toml` (~40 lines)
- `proxy/src/main.rs` (~80 lines)
- `proxy/src/health.rs` (~30 lines)

---

## Step 3: KeyValue Protobuf + Service Stub (Week 2, Days 1-2)

### Goal
Define complete KeyValue protobuf API and generate server stubs.

### Deliverables

1. **Create KeyValue proto**:
```
// proto/prism/keyvalue/v1/keyvalue.proto
service KeyValueService {
  rpc Put(PutRequest) returns (PutResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Scan(ScanRequest) returns (stream ScanResponse);
}
```text

2. **Create KeyValue types**:
```
// proto/prism/keyvalue/v1/types.proto
message Item {
  bytes key = 1;
  bytes value = 2;
  prism.common.ItemMetadata metadata = 3;
}

message PutRequest {
  string namespace = 1;
  string id = 2;
  repeated Item items = 3;
}
// ... etc
```text

3. **Regenerate Rust code**:
```
buf generate
```text

4. **Implement stub service** (returns errors):
```
// proxy/src/keyvalue/service.rs
pub struct KeyValueService;

#[tonic::async_trait]
impl KeyValue for KeyValueService {
    async fn put(&self, req: Request<PutRequest>) -> Result<Response<PutResponse>> {
        Err(Status::unimplemented("put not yet implemented"))
    }
    // ... etc
}
```text

5. **Wire into server**:
```
Server::builder()
    .add_service(HealthServer::new(health_svc))
    .add_service(KeyValueServer::new(kv_svc))  // ← New!
    .serve(addr)
    .await?;
```text

### Success Criteria
- ✅ Protobuf compiles cleanly
- ✅ Rust code generates without errors
- ✅ Server starts with KeyValue service
- ✅ `grpcurl` can call `KeyValue/Put` (gets unimplemented error)

### Files to Create/Update
- `proto/prism/keyvalue/v1/keyvalue.proto` (~80 lines)
- `proto/prism/keyvalue/v1/types.proto` (~120 lines)
- `proxy/src/keyvalue/service.rs` (~100 lines)
- `proxy/src/main.rs` (update: +5 lines)

---

## Step 4: SQLite Backend Implementation (Week 2, Days 3-5)

### Goal
Implement working KeyValue backend using SQLite for local testing.

### Deliverables

1. **Define backend trait**:
```
// proxy/src/backend/mod.rs
#[async_trait]
pub trait KeyValueBackend: Send + Sync {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()>;
    async fn get(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<Vec<Item>>;
    async fn delete(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<()>;
    async fn scan(&self, namespace: &str, id: &str) -> Result<Vec<Item>>;
}
```text

2. **Implement SQLite backend**:
```
// proxy/src/backend/sqlite.rs
pub struct SqliteBackend {
    pool: SqlitePool,
}

#[async_trait]
impl KeyValueBackend for SqliteBackend {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()> {
        let mut tx = self.pool.begin().await?;

        for item in items {
            sqlx::query(
                "INSERT OR REPLACE INTO kv (namespace, id, key, value) VALUES (?, ?, ?, ?)"
            )
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
    // ... etc
}
```text

3. **Create schema migration**:
```
-- proxy/migrations/001_create_kv_table.sql
CREATE TABLE IF NOT EXISTS kv (
    namespace TEXT NOT NULL,
    id TEXT NOT NULL,
    key BLOB NOT NULL,
    value BLOB NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (namespace, id, key)
);

CREATE INDEX idx_kv_namespace ON kv(namespace);
```text

4. **Wire backend into service**:
```
// proxy/src/keyvalue/service.rs
pub struct KeyValueService {
    backend: Arc<dyn KeyValueBackend>,
}

#[tonic::async_trait]
impl KeyValue for KeyValueService {
    async fn put(&self, req: Request<PutRequest>) -> Result<Response<PutResponse>> {
        let req = req.into_inner();
        self.backend.put(&req.namespace, &req.id, req.items).await?;
        Ok(Response::new(PutResponse { success: true }))
    }
    // ... etc
}
```text

5. **Add configuration**:
```
# proxy/config.yaml
database:
  type: sqlite
  path: ./prism.db

logging:
  level: debug
  format: json
```text

### Success Criteria
- ✅ Can put data: `grpcurl -d '{"namespace":"test","id":"1","items":[{"key":"aGVsbG8=","value":"d29ybGQ="}]}' localhost:8980 prism.keyvalue.v1.KeyValueService/Put`
- ✅ Can get data back with same value
- ✅ Data persists across server restarts
- ✅ All CRUD operations work (Put, Get, Delete, Scan)

### Files to Create
- `proxy/src/backend/mod.rs` (~50 lines)
- `proxy/src/backend/sqlite.rs` (~250 lines)
- `proxy/migrations/001_create_kv_table.sql` (~15 lines)
- `proxy/config.yaml` (~20 lines)
- `proxy/Cargo.toml` (update: add `sqlx`, `serde_yaml`)

---

## Step 5: Integration Tests + Local Stack (Week 3, Days 1-3)

### Goal
Validate end-to-end functionality with automated tests using real local backends.

### Deliverables

1. **Create integration test**:
```
// proxy/tests/integration_test.rs
#[tokio::test]
async fn test_put_get_roundtrip() {
    let client = KeyValueClient::connect("http://".to_string() + "localhost:8980").await.unwrap();

    // Put
    let put_req = PutRequest {
        namespace: "test".to_string(),
        id: "user123".to_string(),
        items: vec![Item {
            key: b"profile".to_vec(),
            value: b"Alice".to_vec(),
            metadata: None,
        }],
        item_priority_token: 0,
    };
    client.put(put_req).await.unwrap();

    // Get
    let get_req = GetRequest {
        namespace: "test".to_string(),
        id: "user123".to_string(),
        predicate: Some(KeyPredicate {
            predicate: Some(key_predicate::Predicate::MatchAll(MatchAll {})),
        }),
    };
    let response = client.get(get_req).await.unwrap().into_inner();

    assert_eq!(response.items.len(), 1);
    assert_eq!(response.items[0].key, b"profile");
    assert_eq!(response.items[0].value, b"Alice");
}
```text

2. **Enhance `docker-compose.test.yml`**:
```
services:
  prism-proxy:
    build: ./proxy
    ports:
      - "8980:8980"
    depends_on:
      - postgres
    environment:
      DATABASE_URL: postgres://prism:prism_test_password@postgres/prism_test

  postgres:
    # ... existing config ...
```text

3. **Create test helper**:
```
// proxy/tests/common/mod.rs
pub struct TestFixture {
    pub client: KeyValueClient<Channel>,
}

impl TestFixture {
    pub async fn new() -> Self {
        // Wait for server to be ready
        tokio::time::sleep(Duration::from_secs(1)).await;

        let client = KeyValueClient::connect("http://localhost:8980")
            .await
            .expect("Failed to connect");

        Self { client }
    }
}
```text

4. **Add CI workflow**:
```
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Start local stack
        run: python -m tooling.test.local-stack up

      - name: Run unit tests
        run: cargo test --lib

      - name: Run integration tests
        run: cargo test --test integration_test
```text

### Success Criteria
- ✅ All integration tests pass locally
- ✅ Tests pass in CI
- ✅ Can run full test suite in < 60 seconds
- ✅ Tests clean up after themselves (no state leakage)

### Files to Create
- `proxy/tests/integration_test.rs` (~200 lines)
- `proxy/tests/common/mod.rs` (~50 lines)
- `.github/workflows/test.yml` (~40 lines)
- `docker-compose.test.yml` (update: add prism-proxy service)

---

## Step 6: Postgres Backend + Documentation (Week 3-4, Days 4-7)

### Goal
Production-ready Postgres backend with complete documentation.

### Deliverables

1. **Implement Postgres backend**:
```
// proxy/src/backend/postgres.rs
pub struct PostgresBackend {
    pool: PgPool,
}

#[async_trait]
impl KeyValueBackend for PostgresBackend {
    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()> {
        let mut tx = self.pool.begin().await?;

        for item in items {
            sqlx::query(
                "INSERT INTO kv (namespace, id, key, value, updated_at)
                 VALUES ($1, $2, $3, $4, NOW())
                 ON CONFLICT (namespace, id, key)
                 DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()"
            )
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
    // ... etc (similar to SQLite but with Postgres-specific SQL)
}
```text

2. **Add connection pooling**:
```
let pool = PgPoolOptions::new()
    .max_connections(20)
    .connect(&database_url)
    .await?;
```text

3. **Create Postgres migrations**:
```
-- proxy/migrations/postgres/001_create_kv_table.sql
CREATE TABLE IF NOT EXISTS kv (
    namespace VARCHAR(255) NOT NULL,
    id VARCHAR(255) NOT NULL,
    key BYTEA NOT NULL,
    value BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (namespace, id, key)
);

CREATE INDEX idx_kv_namespace ON kv(namespace);
CREATE INDEX idx_kv_id ON kv(namespace, id);
```text

4. **Add integration tests for Postgres**:
```
#[tokio::test]
async fn test_postgres_backend() {
    let pool = PgPool::connect("postgres://prism:prism_test_password@localhost/prism_test")
        .await
        .unwrap();

    let backend = PostgresBackend::new(pool);

    // Run same tests as SQLite
    // ... test put, get, delete, scan
}
```text

5. **Write documentation**:
   - `docs/getting-started.md`: Quickstart guide
   - `docs/api-reference.md`: gRPC API documentation
   - `docs/deployment.md`: How to deploy Prism
   - Update `README.md` with real examples

### Success Criteria
- ✅ Postgres backend passes all integration tests
- ✅ Performance: 10k RPS sustained on laptop
- ✅ Connection pooling works correctly
- ✅ Documentation covers all key use cases
- ✅ Can deploy Prism with Postgres in production

### Files to Create
- `proxy/src/backend/postgres.rs` (~300 lines)
- `proxy/migrations/postgres/001_create_kv_table.sql` (~20 lines)
- `proxy/tests/postgres_test.rs` (~150 lines)
- `docs/getting-started.md` (~200 lines)
- `docs/api-reference.md` (~300 lines)
- `docs/deployment.md` (~150 lines)

---

## Summary Timeline

| Week | Days | Step | Deliverable | Status |
|------|------|------|-------------|--------|
| 1 | 1-3 | Step 1 | Protobuf foundation | 📋 Planned |
| 1 | 4-5 | Step 2 | Rust proxy skeleton | 📋 Planned |
| 2 | 1-2 | Step 3 | KeyValue protobuf + stubs | 📋 Planned |
| 2 | 3-5 | Step 4 | SQLite backend | 📋 Planned |
| 3 | 1-3 | Step 5 | Integration tests + CI | 📋 Planned |
| 3-4 | 4-7 | Step 6 | Postgres + docs | 📋 Planned |

**Total**: ~4 weeks to production-ready KeyValue abstraction

## Success Metrics

After completing all 6 steps, we should have:

- ✅ **Working system**: KeyValue abstraction with SQLite + Postgres
- ✅ **Performance**: P99 < 10ms, 10k RPS sustained
- ✅ **Testing**: 90%+ code coverage, all tests green
- ✅ **Documentation**: Complete getting-started guide
- ✅ **Deployable**: Can deploy to production
- ✅ **Validated**: Core architecture proven with real code

## Next Steps After Step 6

Once the foundation is solid, subsequent phases:

**Phase 2** (Weeks 5-8):
- TimeSeries abstraction + Kafka backend
- OpenTelemetry observability
- Shadow traffic support
- Production deployment

**Phase 3** (Weeks 9-12):
- Graph abstraction + Neptune backend
- Client-originated configuration
- Admin UI basics
- Auto-provisioning

## Alternatives Considered

### Big Bang Approach
- Implement all abstractions (KeyValue, TimeSeries, Graph) at once
- **Rejected**: Too risky, can't validate assumptions early

### Vertical Slice
- Implement one end-to-end use case (e.g., user profiles)
- **Rejected**: Doesn't validate platform generality

### Backend-First
- Implement all backends for KeyValue before moving to TimeSeries
- **Rejected**: Diminishing returns; SQLite + Postgres sufficient to validate

## References

- ADR-001 through ADR-010 (all previous architectural decisions)
- [Protobuf Data Model Plan](https://github.com/jrepp/prism-data-layer/blob/main/docs-cms/protobuf-data-model-plan.md)
- [PRD](https://github.com/jrepp/prism-data-layer/blob/main/PRD.md)

## Revision History

- 2025-10-05: Initial roadmap and acceptance

```