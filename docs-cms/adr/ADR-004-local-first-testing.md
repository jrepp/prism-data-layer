---
id: adr-004
title: "ADR-004: Local-First Testing Strategy"
status: Accepted
date: 2025-10-05
deciders: Core Team
tags: ['testing', 'dx', 'reliability']
---

## Context

Testing data infrastructure is challenging:

- **Traditional approach**: Use mocks/fakes for unit tests, real databases for integration tests
- **Problems with mocks**:
  - Don't catch integration bugs
  - Drift from real behavior
  - Give false confidence
  - Don't test performance characteristics

- **Problems with cloud-only testing**:
  - Slow feedback loop (deploy to test)
  - Expensive (running test infra 24/7)
  - Complex setup (VPNs, credentials, etc.)
  - Hard to reproduce CI failures locally

**Problem**: How do we test Prism thoroughly while maintaining fast iteration and developer happiness?

## Decision

Adopt a **local-first testing strategy**: All backends must support running locally with Docker Compose. Prioritize real local backends over mocks. Use the same test suite locally and in CI.

## Rationale

### Principles

1. **Real > Fake**: Use actual databases (sqlite, postgres, kafka) instead of mocks
2. **Local > Cloud**: Developers can run full stack on laptop
3. **Fast > Slow**: Optimize for sub-second test execution
4. **Simple > Complex**: Minimal setup; works out of the box

### Architecture

Developer Laptop
┌────────────────────────────────────────┐
│  Tests (Rust, Python)                  │
│         ↓ ↓ ↓                          │
│  Prism Proxy (Rust)                    │
│         ↓ ↓ ↓                          │
│  ┌──────────────────────────────────┐  │
│  │ Docker Compose                   │  │
│  │  • PostgreSQL (in-memory mode)   │  │
│  │  • Kafka (kraft, single broker)  │  │
│  │  • NATS (embedded mode)          │  │
│  │  • SQLite (file://local.db)      │  │
│  │  • Neptune (localstack)          │  │
│  └──────────────────────────────────┘  │
└────────────────────────────────────────┘
```text

### Local Stack Configuration

```
# docker-compose.test.yml
version: '3.9'

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: prism_test
      POSTGRES_USER: prism
      POSTGRES_PASSWORD: prism_test_password
    command:
      - postgres
      - -c
      - fsync=off              # Faster for tests
      - -c
      - full_page_writes=off
      - -c
      - synchronous_commit=off
    tmpfs:
      - /var/lib/postgresql/data  # In-memory for speed
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U prism"]
      interval: 1s
      timeout: 1s
      retries: 30

  kafka:
    image: apache/kafka:latest
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@localhost:9093
      # Fast for tests
      KAFKA_LOG_FLUSH_INTERVAL_MESSAGES: 1
      KAFKA_LOG_FLUSH_INTERVAL_MS: 10
    ports:
      - "9092:9092"
    healthcheck:
      test: ["CMD-SHELL", "kafka-broker-api-versions.sh --bootstrap-server localhost:9092"]
      interval: 1s
      timeout: 1s
      retries: 30

  nats:
    image: nats:alpine
    command: ["-js", "-m", "8222"]  # Enable JetStream and monitoring
    ports:
      - "4222:4222"  # Client port
      - "8222:8222"  # Monitoring port
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://localhost:8222/healthz"]
      interval: 1s
      timeout: 1s
      retries: 10

  # AWS Neptune compatible (for local graph testing)
  neptune:
    image: localstack/localstack:latest
    environment:
      SERVICES: neptune
      DEBUG: 1
    ports:
      - "8182:8182"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```text

### Python Tooling

```
# tooling/test/local_stack.py

import subprocess
import time
from dataclasses import dataclass

@dataclass
class Backend:
    name: str
    port: int
    healthcheck: callable

class LocalStack:
    """Manage local test infrastructure."""

    def __init__(self):
        self.backends = [
            Backend("postgres", 5432, self._check_postgres),
            Backend("kafka", 9092, self._check_kafka),
            Backend("nats", 4222, self._check_nats),
        ]

    def up(self, wait: bool = True):
        """Start all backend services."""
        subprocess.run([
            "docker", "compose",
            "-f", "docker-compose.test.yml",
            "up", "-d"
        ], check=True)

        if wait:
            self.wait_healthy()

    def down(self):
        """Stop and remove all services."""
        subprocess.run([
            "docker", "compose",
            "-f", "docker-compose.test.yml",
            "down", "-v"  # Remove volumes
        ], check=True)

    def wait_healthy(self, timeout: int = 60):
        """Wait for all services to be healthy."""
        start = time.time()
        while time.time() - start < timeout:
            if all(b.healthcheck() for b in self.backends):
                print("✓ All services healthy")
                return
            time.sleep(0.5)
        raise TimeoutError("Services failed to become healthy")

    def reset(self):
        """Reset all data (for test isolation)."""
        # Truncate all tables, delete all Kafka topics, etc.
        pass

# CLI
if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["up", "down", "reset"])
    args = parser.parse_args()

    stack = LocalStack()
    if args.command == "up":
        stack.up()
    elif args.command == "down":
        stack.down()
    elif args.command == "reset":
        stack.reset()
```text

### Test Structure

```
// proxy/tests/integration/keyvalue_test.rs

use prism_proxy::*;
use testcontainers::*; // Fallback if Docker Compose not available

#[tokio::test]
async fn test_keyvalue_postgres() {
    // Uses real Postgres from docker-compose.test.yml
    let mut proxy = TestProxy::new(Backend::Postgres).await;

    // Write data
    proxy.put("user:123", b"Alice").await.unwrap();

    // Read it back
    let value = proxy.get("user:123").await.unwrap();
    assert_eq!(value, b"Alice");

    // Verify it's actually in Postgres
    let row: (String,) = sqlx::query_as("SELECT value FROM kv WHERE key = $1")
        .bind("user:123")
        .fetch_one(&proxy.postgres_pool())
        .await
        .unwrap();
    assert_eq!(row.0, "Alice");
}

#[tokio::test]
async fn test_keyvalue_kafka() {
    let mut proxy = TestProxy::new(Backend::Kafka).await;

    // Same API, different backend
    proxy.put("event:456", b"Login").await.unwrap();

    let value = proxy.get("event:456").await.unwrap();
    assert_eq!(value, b"Login");

    // Verify it's actually in Kafka
    // ... Kafka consumer check ...
}

// Load test
#[tokio::test]
#[ignore]  // Run explicitly with: cargo test --ignored
async fn load_test_keyvalue_writes() {
    let proxy = TestProxy::new(Backend::Postgres).await;

    let start = std::time::Instant::now();
    let tasks: Vec<_> = (0..1000)
        .map(|i| {
            let mut proxy = proxy.clone();
            tokio::spawn(async move {
                proxy.put(&format!("key:{}", i), b"value").await.unwrap();
            })
        })
        .collect();

    futures::future::join_all(tasks).await;

    let elapsed = start.elapsed();
    let throughput = 1000.0 / elapsed.as_secs_f64();

    println!("Throughput: {:.0} writes/sec", throughput);
    println!("Latency: {:.2}ms per write", elapsed.as_secs_f64() / 1000.0 * 1000.0);

    assert!(throughput > 500.0, "Throughput too low: {}", throughput);
}
```text

### Alternatives Considered

1. **Mock All The Things**
   - Pros:
     - Fast tests
     - No external dependencies
   - Cons:
     - Doesn't catch integration bugs
     - Mocks drift from reality
     - More code to maintain (mocks)
   - Rejected because: Low confidence in test results

2. **Cloud-Only Testing**
   - Pros:
     - Tests production environment
     - No local setup
   - Cons:
     - Slow feedback (minutes, not seconds)
     - Expensive
     - Can't debug locally
   - Rejected because: Poor developer experience

3. **In-Memory Fakes**
   - Pros:
     - Faster than real databases
     - No Docker required
   - Cons:
     - Subtle behavior differences
     - Don't test performance
     - Still not the real thing
   - Rejected because: Real backends with optimization are fast enough

4. **Testcontainers Only** (no Docker Compose)
   - Pros:
     - Programmatic container management
     - Good for isolated tests
   - Cons:
     - Slower startup per test
     - Harder to reuse containers
     - No standard docker-compose.yml for docs
   - Rejected because: Docker Compose is simpler; can use testcontainers as fallback

## Consequences

### Positive

- **High Confidence**: Tests use real backends, catch real bugs
- **Fast Feedback**: Full test suite runs in `<1 minute` locally
- **Easy Debugging**: Reproduce any test failure on laptop
- **Performance Testing**: Load tests use same local infrastructure
- **Documentation**: docker-compose.yml shows how to run Prism

### Negative

- **Requires Docker**: Developers must install Docker
  - *Mitigation*: Docker is ubiquitous; provide install instructions
- **Slower Than Mocks**: Real databases have overhead
  - *Mitigation*: Optimize with in-memory modes, tmpfs
- **More Complex Setup**: docker-compose.yml to maintain
  - *Mitigation*: Tooling abstracts complexity; `python -m tooling.test.local-stack up`

### Neutral

- **Not Production-Identical**: Local postgres ≠ AWS RDS
  - Use same software version; accept minor differences
  - Run subset of tests against staging/prod
- **Resource Usage**: Running backends uses CPU/memory
  - Modern laptops handle it fine
  - CI runners sized appropriately

## Implementation Notes

### Optimizations for Speed

1. **In-Memory Postgres**: Use `tmpfs` for data directory
2. **Kafka**: Single broker, minimal replication
3. **Connection Pooling**: Reuse connections between tests
4. **Parallel Tests**: Use `cargo test --jobs 4`
5. **Test Isolation**: Each test uses unique namespace (no truncation needed)

### CI Configuration

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

      - name: Run tests
        run: cargo test --workspace

      - name: Run load tests
        run: cargo test --workspace --ignored

      - name: Stop local stack
        if: always()
        run: python -m tooling.test.local-stack down
```text

### Developer Workflow

```
# One-time setup
curl -LsSf https://astral.sh/uv/install.sh | sh
uv sync

# Start backends (leave running)
python -m tooling.test.local-stack up

# Run tests (as many times as you want)
cargo test
cargo test --ignored  # Load tests

# Stop when done
python -m tooling.test.local-stack down
```text

## References

- [Testcontainers](https://www.testcontainers.org/)
- [Docker Compose](https://docs.docker.com/compose/)
- [Martin Fowler - Integration Testing](https://martinfowler.com/bliki/IntegrationTest.html)
- [Google Testing Blog - Test Sizes](https://testing.googleblog.com/2010/12/test-sizes.html)

## Revision History

- 2025-10-05: Initial draft and acceptance

```