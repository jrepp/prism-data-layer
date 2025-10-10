---
id: adr-018
title: "ADR-018: Rust Error Handling Strategy"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['rust', 'error-handling', 'reliability', 'observability']
---

## Context

Rust proxy implementation requires consistent error handling that:
- Preserves error context through call chains
- Enables debugging without verbose logging
- Leverages Rust's type system for compile-time safety
- Integrates with async/await
- Provides structured error information for observability

## Decision

Adopt modern Rust error handling with `thiserror` and `anyhow`:

1. **Use `thiserror` for library code** (typed errors with context)
2. **Use `anyhow` for application code** (error propagation with context)
3. **Implement `From` traits** for error conversion
4. **Use `?` operator** for error propagation
5. **Add context with `.context()`** at each layer
6. **Define domain-specific error types** per module

## Rationale

### Why thiserror + anyhow

**thiserror** for library/domain errors:
```rust
use thiserror::Error;

#[derive(Error, Debug)]
pub enum BackendError {
    #[error("backend unavailable: {0}")]
    Unavailable(String),

    #[error("namespace not found: {namespace}")]
    NamespaceNotFound { namespace: String },

    #[error("invalid configuration: {0}")]
    InvalidConfig(String),

    #[error("database error: {0}")]
    Database(#[from] sqlx::Error),
}
```

**anyhow** for application/handler errors:
```rust
use anyhow::{Context, Result};

async fn handle_put_request(req: PutRequest) -> Result<PutResponse> {
    let backend = get_backend(&req.namespace)
        .await
        .context(format!("failed to get backend for namespace: {}", req.namespace))?;

    backend
        .put(req.items)
        .await
        .context("failed to put items")?;

    Ok(PutResponse { success: true })
}
```

**Benefits:**
- Compile-time error type checking (thiserror)
- Ergonomic error propagation (anyhow)
- Rich error context without manual wrapping
- Stack traces in debug builds
- Structured error information

### Error Conversion Pattern

```rust
// Domain error type
#[derive(Error, Debug)]
pub enum KeyValueError {
    #[error("item not found: {key}")]
    NotFound { key: String },

    #[error("backend error: {0}")]
    Backend(#[from] BackendError),
}

// Automatic conversion via From trait
impl From<sqlx::Error> for KeyValueError {
    fn from(e: sqlx::Error) -> Self {
        Self::Backend(BackendError::Database(e))
    }
}

// Usage
async fn get_item(key: &str) -> Result<Item, KeyValueError> {
    let row = sqlx::query_as("SELECT * FROM items WHERE key = ?")
        .bind(key)
        .fetch_one(&pool)
        .await?;  // Automatic conversion via From

    Ok(row)
}
```

### Alternatives Considered

1. **Manual error wrapping**
   - Pros: No dependencies
   - Cons: Verbose, error-prone, no stack traces
   - Rejected: Too much boilerplate

2. **`eyre` instead of `anyhow`**
   - Pros: Customizable reports, similar API
   - Cons: Smaller ecosystem, less battle-tested
   - Rejected: anyhow more widely adopted

3. **`snafu` instead of `thiserror`**
   - Pros: Context selectors, different API
   - Cons: More complex, steeper learning curve
   - Rejected: thiserror simpler and more idiomatic

## Consequences

### Positive

- Type-safe error handling with thiserror
- Ergonomic error propagation with `?` operator
- Rich error context for debugging
- Stack traces in development
- Structured errors for observability
- Compile-time guarantees

### Negative

- Two dependencies (but they work together seamlessly)
- Must decide when to use thiserror vs anyhow
- Error types require upfront design

### Neutral

- Error handling is explicit (Rust's philosophy)
- Need to define error types per module

## Implementation Notes

### Module Structure

proxy/src/
├── error.rs          # Top-level error types
├── backend/
│   ├── mod.rs
│   └── error.rs      # Backend-specific errors
├── keyvalue/
│   ├── mod.rs
│   └── error.rs      # KeyValue-specific errors
└── main.rs
```text

### Top-Level Error Types

```
// proxy/src/error.rs
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ProxyError {
    #[error("configuration error: {0}")]
    Config(String),

    #[error("backend error: {0}")]
    Backend(#[from] crate::backend::BackendError),

    #[error("keyvalue error: {0}")]
    KeyValue(#[from] crate::keyvalue::KeyValueError),

    #[error("gRPC error: {0}")]
    Grpc(#[from] tonic::Status),
}

// Convert to gRPC Status for responses
impl From<ProxyError> for tonic::Status {
    fn from(e: ProxyError) -> Self {
        match e {
            ProxyError::Backend(BackendError::NamespaceNotFound { .. }) =>
                tonic::Status::not_found(e.to_string()),
            ProxyError::Config(_) =>
                tonic::Status::invalid_argument(e.to_string()),
            _ =>
                tonic::Status::internal(e.to_string()),
        }
    }
}
```text

### Handler Pattern

```
use anyhow::{Context, Result};
use tonic::{Request, Response, Status};

#[tonic::async_trait]
impl KeyValueService for KeyValueHandler {
    async fn put(
        &self,
        request: Request<PutRequest>,
    ) -> Result<Response<PutResponse>, Status> {
        let req = request.into_inner();

        // Use anyhow::Result internally
        let result: Result<PutResponse> = async {
            let backend = self
                .get_backend(&req.namespace)
                .await
                .context(format!("namespace: {}", req.namespace))?;

            backend
                .put(&req.id, req.items)
                .await
                .context("backend put operation")?;

            Ok(PutResponse { success: true })
        }
        .await;

        // Convert to gRPC Status
        match result {
            Ok(resp) => Ok(Response::new(resp)),
            Err(e) => {
                tracing::error!("put request failed: {:?}", e);
                Err(Status::internal(e.to_string()))
            }
        }
    }
}
```text

### Backend Error Definition

```
// proxy/src/backend/error.rs
use thiserror::Error;

#[derive(Error, Debug)]
pub enum BackendError {
    #[error("connection failed: {endpoint}")]
    ConnectionFailed { endpoint: String },

    #[error("timeout after {timeout_ms}ms")]
    Timeout { timeout_ms: u64 },

    #[error("namespace not found: {namespace}")]
    NamespaceNotFound { namespace: String },

    #[error("sqlx error: {0}")]
    Sqlx(#[from] sqlx::Error),

    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
}
```text

### Testing Error Conditions

```
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_get_nonexistent_namespace() {
        let handler = KeyValueHandler::new();
        let req = Request::new(GetRequest {
            namespace: "nonexistent".to_string(),
            id: "123".to_string(),
            predicate: None,
        });

        let result = handler.get(req).await;
        assert!(result.is_err());
        assert_eq!(result.unwrap_err().code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_backend_error_conversion() {
        let backend_err = BackendError::NamespaceNotFound {
            namespace: "test".to_string(),
        };
        let proxy_err: ProxyError = backend_err.into();
        let status: tonic::Status = proxy_err.into();

        assert_eq!(status.code(), tonic::Code::NotFound);
    }
}
```text

### Error Logging

Integrate with structured logging:

```
use tracing::error;

match do_operation().await {
    Ok(result) => result,
    Err(e) => {
        error!(
            error = %e,
            error_debug = ?e,  // Full debug representation
            namespace = %namespace,
            "operation failed"
        );
        return Err(e);
    }
}
```text

## References

- [thiserror documentation](https://docs.rs/thiserror)
- [anyhow documentation](https://docs.rs/anyhow)
- [Rust Error Handling](https://doc.rust-lang.org/book/ch09-00-error-handling.html)
- ADR-001: Rust for the Proxy
- ADR-013: Go Error Handling Strategy (parallel Go patterns)

## Revision History

- 2025-10-07: Initial draft and acceptance

```