# ADR-021: Rust Structured Logging with Tracing

**Status**: Accepted

**Date**: 2025-10-07

**Deciders**: Core Team

**Tags**: rust, logging, observability, tracing, debugging

## Context

Prism proxy requires production-grade logging and observability:
- Structured logging for machine parsing
- Distributed tracing for request flows
- Span context for debugging
- High performance (minimal overhead on hot path)
- Integration with OpenTelemetry

## Decision

Use **`tracing`** ecosystem for structured logging and distributed tracing:

1. **`tracing` for instrumentation** (spans, events, fields)
2. **`tracing-subscriber` for collection and formatting**
3. **`tracing-opentelemetry` for distributed tracing**
4. **Structured fields over string formatting**
5. **Span context for request correlation**

## Rationale

### Why tracing

**tracing** is the Rust standard for structured, contextual logging:

```rust
use tracing::{info, warn, error, instrument};

#[instrument(skip(backend), fields(namespace = %namespace))]
async fn handle_put(namespace: &str, items: Vec<Item>, backend: &Backend) -> Result<()> {
    info!(item_count = items.len(), "processing put request");

    match backend.put(namespace, items).await {
        Ok(_) => {
            info!("put request completed successfully");
            Ok(())
        }
        Err(e) => {
            error!(error = %e, "put request failed");
            Err(e)
        }
    }
}
```

**Benefits:**
- **Structured by default**: Key-value pairs, not string formatting
- **Span context**: Automatic request correlation
- **Zero-cost when disabled**: Compile-time filtering
- **OpenTelemetry integration**: Distributed tracing
- **Async-aware**: Tracks spans across await points
- **Rich ecosystem**: formatters, filters, subscribers

### Architecture

```
Application Code
      │
      ├─ tracing::info!()      ─┐
      ├─ tracing::error!()      │ Events
      ├─ #[instrument]          │ + Spans
      │                         │
      ▼                         │
   Tracing Subscriber           │
      │                         │
      ├─ Layer: fmt (console)  ─┘
      ├─ Layer: json (file)
      └─ Layer: opentelemetry (Jaeger/Tempo)
```

### Subscriber Configuration

```rust
use tracing_subscriber::{fmt, EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};

fn init_tracing() -> Result<()> {
    let env_filter = EnvFilter::try_from_default_env()
        .or_else(|_| EnvFilter::try_new("info"))?;

    let fmt_layer = fmt::layer()
        .with_target(true)
        .with_level(true)
        .json();  // JSON output for production

    // For development: .pretty() or .compact()

    tracing_subscriber::registry()
        .with(env_filter)
        .with(fmt_layer)
        .init();

    Ok(())
}
```

### Structured Events

```rust
use tracing::{info, warn, error, debug};

// Structured fields
info!(
    namespace = "production",
    item_count = 42,
    duration_ms = 123,
    "request completed"
);

// Error with context
error!(
    error = %err,
    error_debug = ?err,  // Debug representation
    namespace = %namespace,
    retry_count = retries,
    "backend operation failed"
);

// Debug with expensive computation (only evaluated if enabled)
debug!(
    items = ?items,  // Debug representation
    "processing items"
);
```

### Span Instrumentation

```rust
use tracing::{info_span, instrument, Instrument};

// Automatic instrumentation with #[instrument]
#[instrument(skip(backend), fields(namespace = %req.namespace))]
async fn handle_request(req: PutRequest, backend: Arc<Backend>) -> Result<PutResponse> {
    info!("handling request");

    let result = backend.put(req.items).await?;

    info!(items_written = result.count, "request completed");
    Ok(PutResponse { success: true })
}

// Manual span
async fn manual_span_example() {
    let span = info_span!("operation", operation = "migrate");
    async {
        info!("starting migration");
        // ... work ...
        info!("migration complete");
    }
    .instrument(span)
    .await;
}

// Span fields can be set dynamically
let span = info_span!("request", user_id = tracing::field::Empty);
span.record("user_id", &user_id);
```

### OpenTelemetry Integration

```rust
use opentelemetry::global;
use tracing_opentelemetry::OpenTelemetryLayer;
use opentelemetry_jaeger::JaegerPipeline;

async fn init_tracing_with_otel() -> Result<()> {
    // Configure OpenTelemetry exporter
    global::set_text_map_propagator(opentelemetry_jaeger::Propagator::new());

    let tracer = opentelemetry_jaeger::new_pipeline()
        .with_service_name("prism-proxy")
        .install_batch(opentelemetry::runtime::Tokio)?;

    let otel_layer = OpenTelemetryLayer::new(tracer);

    let fmt_layer = fmt::layer().json();

    tracing_subscriber::registry()
        .with(EnvFilter::from_default_env())
        .with(fmt_layer)
        .with(otel_layer)
        .init();

    Ok(())
}
```

### Log Levels and Filtering

```rust
// Set via environment variable
// RUST_LOG=prism_proxy=debug,sqlx=warn

// Or programmatically
let filter = EnvFilter::new("prism_proxy=debug")
    .add_directive("sqlx=warn".parse()?)
    .add_directive("tonic=info".parse()?);
```

### Performance: Conditional Logging

```rust
use tracing::Level;

// Only evaluate expensive computation if debug enabled
if tracing::level_enabled!(Level::DEBUG) {
    debug!(expensive_data = ?compute_expensive_debug_info(), "debug info");
}

// Or use span guards for hot paths
let _span = info_span!("hot_path").entered();
// Span only recorded if info level enabled
```

### Alternatives Considered

1. **`log` crate**
   - Pros: Simpler API, widely used
   - Cons: No span context, no async support, less structured
   - Rejected: tracing is superior for async and structured logging

2. **`slog`**
   - Pros: Mature, fast, structured
   - Cons: More complex, less async integration
   - Rejected: tracing is now the ecosystem standard

3. **Custom logging**
   - Pros: Full control
   - Cons: Complex, reinventing the wheel
   - Rejected: tracing ecosystem is battle-tested

## Consequences

### Positive

- **Structured by default**: All logs are machine-parsable
- **Span context**: Automatic request correlation
- **Zero-cost abstraction**: No overhead when disabled
- **OpenTelemetry integration**: Distributed tracing
- **Async-aware**: Proper async span tracking
- **Rich ecosystem**: Many formatters and exporters

### Negative

- **Learning curve**: More complex than simple logging
- **Verbosity**: `#[instrument]` adds code
- **Compile times**: Heavy macro usage can slow compilation

### Neutral

- Must configure subscriber at startup
- Requires thoughtful span design

## Implementation Notes

### Dependencies

```toml
[dependencies]
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["json", "env-filter"] }
tracing-opentelemetry = "0.22"
opentelemetry = { version = "0.21", features = ["trace"] }
opentelemetry-jaeger = { version = "0.20", features = ["rt-tokio"] }
```

### Standard Fields

Always include:
- `service.name`: "prism-proxy"
- `service.version`: from Cargo.toml
- `environment`: "production", "staging", "development"

```rust
fn init_tracing() -> Result<()> {
    tracing_subscriber::registry()
        .with(EnvFilter::from_default_env())
        .with(
            fmt::layer()
                .json()
                .with_current_span(true)
                .with_span_list(true)
        )
        .init();

    Ok(())
}
```

### Logging Guidelines

**DO:**
- Use `#[instrument]` on handler functions
- Add structured fields, not string interpolation
- Use appropriate log levels
- Include error context with `error = %e`
- Measure duration with spans

**DON'T:**
- Log in tight loops (use sample or aggregate)
- Log sensitive data (PII, credentials)
- Use string formatting (`format!()`) for fields
- Over-instrument (every function doesn't need a span)

### Testing with Tracing

```rust
#[cfg(test)]
mod tests {
    use tracing_subscriber::layer::SubscriberExt;
    use tracing_subscriber::util::SubscriberInitExt;

    #[tokio::test]
    async fn test_with_tracing() {
        // Initialize test subscriber
        let subscriber = tracing_subscriber::registry()
            .with(tracing_subscriber::fmt::layer().pretty());

        tracing::subscriber::with_default(subscriber, || {
            // Test code with tracing
        });
    }
}
```

## References

- [tracing documentation](https://docs.rs/tracing)
- [tracing-subscriber documentation](https://docs.rs/tracing-subscriber)
- [tracing-opentelemetry](https://docs.rs/tracing-opentelemetry)
- [Tokio Tracing Guide](https://tokio.rs/tokio/topics/tracing)
- ADR-001: Rust for the Proxy
- ADR-008: Observability Strategy
- ADR-017: Go Structured Logging (parallel Go patterns)

## Revision History

- 2025-10-07: Initial draft and acceptance
