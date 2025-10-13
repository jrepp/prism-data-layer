---
date: 2025-10-07
deciders: Core Team
doc_uuid: c36cea37-01e5-45d7-b8bf-5aaa25395be4
id: adr-019
project_id: prism-data-layer
status: Accepted
tags:
- rust
- concurrency
- async
- performance
- tokio
title: Rust Async Concurrency Patterns
---

## Context

Prism proxy must handle:
- Thousands of concurrent requests
- Async I/O to multiple backends
- Connection pooling and management
- Request timeouts and cancellation
- Graceful shutdown

Requirements:
- High throughput (10k+ RPS)
- Low latency (P99 < 10ms)
- Efficient resource utilization
- Predictable performance (no GC pauses)

## Decision

Use **Tokio async runtime** with established concurrency patterns:

1. **tokio as async runtime** (work-stealing scheduler)
2. **spawn tasks for concurrent operations**
3. **channels for communication** (mpsc, broadcast, oneshot)
4. **select! for multiplexing**
5. **timeout and cancellation** via `tokio::time` and `select!`
6. **connection pooling** with `deadpool` or `bb8`

## Rationale

### Architecture

```text
                    ┌─────────────┐
                    │  gRPC Server│
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Handler    │ (tokio::spawn per request)
                    └──────┬──────┘
                           │
              ┌────────────┴────────────┐
              │                         │
         CONCURRENT                     │
              │                         │
    ┌─────────▼─────────┐    ┌─────────▼─────────┐
    │  Backend Pool 1    │    │  Backend Pool 2    │
    │  (connection pool) │    │  (connection pool) │
    └────────┬───────────┘    └────────┬───────────┘
             │                         │
    ┌────────▼───────────┐    ┌────────▼───────────┐
    │   PostgreSQL       │    │     SQLite         │
    └────────────────────┘    └────────────────────┘
```

### Tokio Runtime Configuration

```rust
// main.rs
#[tokio::main(flavor = "multi_thread", worker_threads = 8)]
async fn main() -> Result<()> {
    // Configure runtime
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .worker_threads(num_cpus::get())
        .thread_name("prism-worker")
        .enable_all()
        .build()?;

    runtime.block_on(async {
        run_server().await
    })
}
```

### Spawning Concurrent Tasks

```rust
use tokio::task;

// Spawn independent tasks
async fn process_batch(items: Vec<Item>) -> Result<()> {
    let mut handles = Vec::new();

    for item in items {
        let handle = task::spawn(async move {
            process_item(item).await
        });
        handles.push(handle);
    }

    // Wait for all tasks
    for handle in handles {
        handle.await??;
    }

    Ok(())
}

// Or use join! for fixed number of tasks
use tokio::join;

async fn parallel_operations() -> Result<()> {
    let (result1, result2, result3) = join!(
        operation1(),
        operation2(),
        operation3(),
    );

    result1?;
    result2?;
    result3?;

    Ok(())
}
```

### Channel Patterns

```rust
use tokio::sync::{mpsc, oneshot, broadcast};

// Multi-producer, single-consumer
async fn worker_pool() {
    let (tx, mut rx) = mpsc::channel::<Task>(100);

    // Spawn workers
    for i in 0..8 {
        let mut rx_clone = rx.clone();
        task::spawn(async move {
            while let Some(task) = rx_clone.recv().await {
                process_task(task).await;
            }
        });
    }

    // Send work
    for task in tasks {
        tx.send(task).await?;
    }
}

// One-shot for request-response
async fn request_response() -> Result<Response> {
    let (tx, rx) = oneshot::channel();

    task::spawn(async move {
        let result = compute_result().await;
        tx.send(result).ok();
    });

    rx.await?
}

// Broadcast for fan-out
async fn broadcast_shutdown() {
    let (tx, _rx) = broadcast::channel(16);

    // Clone for each listener
    let mut rx1 = tx.subscribe();
    let mut rx2 = tx.subscribe();

    // Broadcast shutdown
    tx.send(()).ok();

    // Listeners receive
    rx1.recv().await.ok();
    rx2.recv().await.ok();
}
```

### Select for Multiplexing

```rust
use tokio::select;

async fn operation_with_timeout() -> Result<Response> {
    let timeout = tokio::time::sleep(Duration::from_secs(30));

    select! {
        result = backend.query() => {
            result
        }
        _ = timeout => {
            Err(anyhow!("operation timed out"))
        }
        _ = shutdown_signal.recv() => {
            Err(anyhow!("shutting down"))
        }
    }
}
```

### Connection Pooling

```rust
use sqlx::postgres::PgPoolOptions;
use std::time::Duration;

async fn create_pool(database_url: &str) -> Result<PgPool> {
    PgPoolOptions::new()
        .max_connections(20)
        .min_connections(5)
        .acquire_timeout(Duration::from_secs(5))
        .idle_timeout(Duration::from_secs(600))
        .max_lifetime(Duration::from_secs(1800))
        .connect(database_url)
        .await
        .context("failed to create connection pool")
}

// Use pool
async fn query_database(pool: &PgPool, key: &str) -> Result<Item> {
    sqlx::query_as("SELECT * FROM items WHERE key = $1")
        .bind(key)
        .fetch_one(pool)  // Automatically acquires from pool
        .await
        .context("database query failed")
}
```

### Graceful Shutdown

```rust
use tokio::signal;
use tokio::sync::broadcast;

async fn run_server() -> Result<()> {
    let (shutdown_tx, _) = broadcast::channel(1);

    // Spawn server
    let server_handle = {
        let mut shutdown_rx = shutdown_tx.subscribe();
        task::spawn(async move {
            Server::builder()
                .add_service(service)
                .serve_with_shutdown(addr, async {
                    shutdown_rx.recv().await.ok();
                })
                .await
        })
    };

    // Wait for shutdown signal
    signal::ctrl_c().await?;
    tracing::info!("shutdown signal received");

    // Broadcast shutdown
    shutdown_tx.send(()).ok();

    // Wait for server to stop
    server_handle.await??;

    tracing::info!("server stopped gracefully");
    Ok(())
}
```

### Alternatives Considered

1. **async-std instead of tokio**
   - Pros: Simpler API, mirrors std library
   - Cons: Smaller ecosystem, fewer libraries
   - Rejected: tokio is industry standard

2. **Synchronous multithreading**
   - Pros: Simpler mental model
   - Cons: Thread overhead, doesn't scale to 10k+ connections
   - Rejected: async required for high concurrency

3. **Custom executor**
   - Pros: Full control
   - Cons: Complex, error-prone
   - Rejected: tokio is battle-tested

## Consequences

### Positive

- **High concurrency**: 10k+ concurrent requests
- **Low latency**: Async I/O doesn't block threads
- **Efficient**: Work-stealing scheduler maximizes CPU utilization
- **Ecosystem**: Rich library support (tonic, sqlx, etc.)
- **Predictable**: No GC pauses

### Negative

- **Complexity**: Async code harder to debug than sync
- **Colored functions**: Async/await splits the function space
- **Learning curve**: async Rust has sharp edges

### Neutral

- Runtime overhead (minimal for I/O-bound workloads)
- Must choose correct runtime flavor (multi_thread vs current_thread)

## Implementation Notes

### Cargo Dependencies

```toml
[dependencies]
tokio = { version = "1.35", features = ["full"] }
tokio-util = "0.7"
async-trait = "0.1"
futures = "0.3"
```

### Task Spawning Best Practices

```rust
// ✅ Good: spawn for CPU-bound work
task::spawn_blocking(|| {
    // CPU-intensive computation
    compute_hash(large_data)
});

// ✅ Good: spawn for independent async work
task::spawn(async move {
    background_job().await
});

// ❌ Bad: don't spawn for every tiny operation
for item in items {
    task::spawn(async move {  // Too much overhead!
        trivial_operation(item).await
    });
}

// ✅ Good: batch work
task::spawn(async move {
    for item in items {
        trivial_operation(item).await;
    }
});
```

### Error Handling in Async

```rust
// Propagate errors with ?
async fn operation() -> Result<()> {
    let result = backend.query().await?;
    process(result).await?;
    Ok(())
}

// Handle task join errors
let handle = task::spawn(async {
    do_work().await
});

match handle.await {
    Ok(Ok(result)) => { /* success */ }
    Ok(Err(e)) => { /* work failed */ }
    Err(e) => { /* task panicked or cancelled */ }
}
```

### Testing Async Code

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_concurrent_operations() {
        let result = parallel_operations().await;
        assert!(result.is_ok());
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn test_with_specific_runtime() {
        // Test with 2 workers
    }
}
```

## References

- [Tokio Documentation](https://tokio.rs)
- [Async Rust Book](https://rust-lang.github.io/async-book/)
- [Tokio Runtime Documentation](https://docs.rs/tokio/latest/tokio/runtime/)
- ADR-001: Rust for the Proxy
- ADR-018: Rust Error Handling Strategy
- ADR-014: Go Concurrency Patterns (parallel Go patterns)

## Revision History

- 2025-10-07: Initial draft and acceptance