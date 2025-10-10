---
title: "ADR-020: Rust Testing Strategy"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['rust', 'testing', 'quality', 'ci-cd']
---

## Context

Prism proxy requires comprehensive testing that:
- Ensures correctness at multiple levels
- Maintains 80%+ code coverage
- Supports rapid development
- Catches regressions early
- Validates async code and concurrency

Testing pyramid: Unit tests (base) → Integration tests → E2E tests (top)

## Decision

Implement **three-tier testing strategy** with Rust best practices:

1. **Unit Tests**: Module-level, test individual functions and types
2. **Integration Tests**: Test crate interactions with real backends
3. **E2E Tests**: Validate full gRPC API with test clients

### Coverage Requirements

- **Minimum**: 80% per crate (CI enforced)
- **Target**: 90%+ for critical crates (`proxy-core`, `backend`, `keyvalue`)
- **New code**: 100% coverage required

## Rationale

### Testing Tiers

#### Tier 1: Unit Tests

**Scope**: Individual functions, types, and modules

**Location**: `#[cfg(test)] mod tests` in same file or `tests/` subdirectory

**Pattern**:
```rust
// src/backend/postgres.rs
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_config_valid() {
        let config = Config {
            host: "localhost".to_string(),
            port: 5432,
            database: "test".to_string(),
        };
        assert!(validate_config(&config).is_ok());
    }

    #[test]
    fn test_validate_config_invalid_port() {
        let config = Config {
            host: "localhost".to_string(),
            port: 0,
            database: "test".to_string(),
        };
        assert!(validate_config(&config).is_err());
    }

    #[tokio::test]
    async fn test_connect_to_backend() {
        let pool = create_test_pool().await;
        let result = connect(&pool).await;
        assert!(result.is_ok());
    }
}
```

**Characteristics**:
- Fast (milliseconds)
- No external dependencies (use mocks)
- Test edge cases and error conditions
- Use `#[tokio::test]` for async tests

#### Tier 2: Integration Tests

**Scope**: Crate interactions, real backend integration

**Location**: `tests/` directory (separate from source)

**Pattern**:
```rust
// tests/integration_test.rs
use prism_proxy::{Backend, KeyValueBackend, SqliteBackend};
use sqlx::SqlitePool;

#[tokio::test]
async fn test_sqlite_backend_put_get() {
    // Create in-memory SQLite database
    let pool = SqlitePool::connect(":memory:").await.unwrap();

    // Run migrations
    sqlx::migrate!("./migrations")
        .run(&pool)
        .await
        .unwrap();

    // Create backend
    let backend = SqliteBackend::new(pool);

    // Test put
    let items = vec![Item {
        key: b"test-key".to_vec(),
        value: b"test-value".to_vec(),
        metadata: None,
    }];

    backend
        .put("test-namespace", "test-id", items)
        .await
        .unwrap();

    // Test get
    let result = backend
        .get("test-namespace", "test-id", vec![b"test-key"])
        .await
        .unwrap();

    assert_eq!(result.len(), 1);
    assert_eq!(result[0].key, b"test-key");
    assert_eq!(result[0].value, b"test-value");
}

#[tokio::test]
async fn test_postgres_backend() {
    // Use testcontainers for real Postgres
    let docker = clients::Cli::default();
    let postgres = docker.run(images::postgres::Postgres::default());
    let port = postgres.get_host_port_ipv4(5432);

    let database_url = format!("postgres://postgres:postgres@localhost:{}/test", port);
    let pool = PgPool::connect(&database_url).await.unwrap();

    // Run tests against real Postgres...
}
```

#### Tier 3: End-to-End Tests

**Scope**: Full gRPC API with real server

**Location**: `tests/e2e/`

**Pattern**:
```rust
// tests/e2e/keyvalue_test.rs
use prism_proto::keyvalue::v1::{
    key_value_service_client::KeyValueServiceClient,
    GetRequest, PutRequest, Item,
};
use tonic::transport::Channel;

#[tokio::test]
async fn test_keyvalue_put_get_e2e() {
    // Start test server
    let addr = start_test_server().await;

    // Connect client
    let mut client = KeyValueServiceClient::connect(format!("http://{}", addr))
        .await
        .unwrap();

    // Put request
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

    let response = client.put(put_req).await.unwrap();
    assert!(response.into_inner().success);

    // Get request
    let get_req = GetRequest {
        namespace: "test".to_string(),
        id: "user123".to_string(),
        predicate: None,
    };

    let response = client.get(get_req).await.unwrap();
    let items = response.into_inner().items;

    assert_eq!(items.len(), 1);
    assert_eq!(items[0].key, b"profile");
    assert_eq!(items[0].value, b"Alice");
}

async fn start_test_server() -> std::net::SocketAddr {
    // Start server in background task
    // Return address when ready
}
```

### Test Utilities and Fixtures

```rust
// tests/common/mod.rs
use sqlx::SqlitePool;

pub async fn create_test_db() -> SqlitePool {
    let pool = SqlitePool::connect(":memory:").await.unwrap();
    sqlx::migrate!("./migrations").run(&pool).await.unwrap();
    pool
}

pub fn sample_item() -> Item {
    Item {
        key: b"test".to_vec(),
        value: b"value".to_vec(),
        metadata: None,
    }
}

pub struct TestBackend {
    pool: SqlitePool,
}

impl TestBackend {
    pub async fn new() -> Self {
        Self {
            pool: create_test_db().await,
        }
    }

    pub async fn insert_item(&self, namespace: &str, id: &str, item: Item) {
        // Helper for test setup
    }
}
```

### Property-Based Testing

```rust
use proptest::prelude::*;

proptest! {
    #[test]
    fn test_key_roundtrip(key in "\\PC*", value in "\\PC*") {
        // Property: put then get should return same value
        let rt = tokio::runtime::Runtime::new().unwrap();
        rt.block_on(async {
            let backend = TestBackend::new().await;
            let item = Item {
                key: key.as_bytes().to_vec(),
                value: value.as_bytes().to_vec(),
                metadata: None,
            };

            backend.put("test", "id", vec![item.clone()]).await.unwrap();
            let result = backend.get("test", "id", vec![&item.key]).await.unwrap();

            prop_assert_eq!(result[0].value, item.value);
            Ok(())
        }).unwrap();
    }
}
```

### Benchmarking

```rust
// benches/keyvalue_bench.rs
use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn bench_put(c: &mut Criterion) {
    let rt = tokio::runtime::Runtime::new().unwrap();
    let backend = rt.block_on(async { TestBackend::new().await });

    c.bench_function("put single item", |b| {
        b.to_async(&rt).iter(|| async {
            let item = sample_item();
            backend.put("test", "id", vec![item]).await.unwrap()
        });
    });
}

criterion_group!(benches, bench_put);
criterion_main!(benches);
```

### Alternatives Considered

1. **Only unit tests**
   - Pros: Fast, simple
   - Cons: Miss integration bugs
   - Rejected: Insufficient for complex system

2. **Mock all dependencies**
   - Pros: Tests run fast, no external dependencies
   - Cons: Tests don't validate real integrations
   - Rejected: Integration tests must use real backends

3. **Fuzzing instead of property tests**
   - Pros: Finds deep bugs
   - Cons: Slow, complex setup
   - Deferred: Add cargo-fuzz later for critical parsers

## Consequences

### Positive

- High confidence in correctness
- Fast unit tests (seconds)
- Integration tests catch real issues
- E2E tests validate gRPC API
- Property tests catch edge cases
- Benchmarks prevent regressions

### Negative

- More code to maintain
- Integration tests require database setup
- E2E tests slower (seconds)
- Async tests more complex

### Neutral

- 80%+ coverage enforced in CI
- Test utilities shared across tests

## Implementation Notes

### Directory Structure

proxy/
├── src/
│   ├── lib.rs
│   ├── main.rs
│   ├── backend/
│   │   ├── mod.rs          # Contains #[cfg(test)] mod tests
│   │   ├── sqlite.rs
│   │   └── postgres.rs
│   └── keyvalue/
│       ├── mod.rs
│       └── service.rs      # Contains #[cfg(test)] mod tests
├── tests/
│   ├── common/
│   │   └── mod.rs          # Shared test utilities
│   ├── integration_test.rs
│   └── e2e/
│       └── keyvalue_test.rs
└── benches/
    └── keyvalue_bench.rs
```text

### Running Tests

```
# Unit tests only (fast)
cargo test --lib

# Integration tests
cargo test --test integration_test

# E2E tests
cargo test --test keyvalue_test

# All tests
cargo test

# With coverage
cargo tarpaulin --out Html --output-dir coverage

# Benchmarks
cargo bench

# Property tests (more iterations)
PROPTEST_CASES=10000 cargo test
```text

### CI Configuration

```
# .github/workflows/rust-test.yml
jobs:
  test:
    steps:
      - name: Unit Tests
        run: cargo test --lib

      - name: Integration Tests
        run: |
          # Start test databases
          docker-compose -f docker-compose.test.yml up -d
          cargo test --tests
          docker-compose -f docker-compose.test.yml down

      - name: Coverage
        run: |
          cargo tarpaulin --out Xml
          if [ $(grep -oP 'line-rate="\K[0-9.]+' coverage.xml | head -1 | awk '{print ($1 < 0.8)}') -eq 1 ]; then
            echo "Coverage below 80%"
            exit 1
          fi

      - name: Benchmarks (ensure no regression)
        run: cargo bench --no-fail-fast
```text

### Dependencies

```
[dev-dependencies]
tokio-test = "0.4"
proptest = "1.4"
criterion = { version = "0.5", features = ["async_tokio"] }
testcontainers = "0.15"
```text

## References

- [Rust Book: Testing](https://doc.rust-lang.org/book/ch11-00-testing.html)
- [tokio::test documentation](https://docs.rs/tokio/latest/tokio/attr.test.html)
- [proptest documentation](https://docs.rs/proptest)
- [criterion documentation](https://docs.rs/criterion)
- ADR-001: Rust for the Proxy
- ADR-019: Rust Async Concurrency Patterns
- ADR-015: Go Testing Strategy (parallel Go patterns)

## Revision History

- 2025-10-07: Initial draft and acceptance

```