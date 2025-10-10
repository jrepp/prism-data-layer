---
title: "ADR-008: Observability Strategy"
status: Accepted
date: 2025-10-05
deciders: Core Team
tags: ['operations', 'performance', 'reliability']
---

## Context

Prism is critical infrastructure sitting in the data path. We must be able to:

1. **Debug issues quickly**: When things go wrong, understand why
2. **Monitor health**: Know if Prism is operating correctly
3. **Track performance**: Measure latency, throughput, errors
4. **Capacity planning**: Understand resource usage trends
5. **Compliance**: Audit logging for regulatory requirements

Observability has three pillars:
- **Metrics**: Numerical measurements (latency, RPS, error rate)
- **Logs**: Structured events (request details, errors)
- **Traces**: Request flow through distributed system

## Decision

Adopt **OpenTelemetry** from day one for metrics, logs, and traces. Use **Prometheus** for metrics storage, **Loki** for logs, and **Jaeger/Tempo** for traces.

## Rationale

### Why OpenTelemetry?

- **Vendor neutral**: Not locked into specific backend
- **Industry standard**: Wide adoption, good tooling
- **Unified SDK**: One library for metrics, logs, traces
- **Rust support**: Excellent `opentelemetry-rust` crate
- **Future-proof**: CNCF graduated project

### Architecture

┌─────────────┐
│ Prism Proxy │
│             │
│ OpenTelemetry SDK
│   ├─ Metrics ─────► Prometheus
│   ├─ Logs ────────► Loki
│   └─ Traces ──────► Jaeger/Tempo
└─────────────┘
```

### Metrics

**Key Metrics** (Prometheus format):

```rust
use prometheus::{
    register_histogram_vec, register_counter_vec,
    HistogramVec, CounterVec,
};

lazy_static! {
    // Request latency
    static ref REQUEST_DURATION: HistogramVec = register_histogram_vec!(
        "prism_request_duration_seconds",
        "Request latency in seconds",
        &["namespace", "operation", "backend"],
        vec![0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0]
    ).unwrap();

    // Request count
    static ref REQUEST_COUNT: CounterVec = register_counter_vec!(
        "prism_requests_total",
        "Total requests",
        &["namespace", "operation", "status"]
    ).unwrap();

    // Backend connection pool
    static ref POOL_SIZE: GaugeVec = register_gauge_vec!(
        "prism_backend_pool_size",
        "Backend connection pool size",
        &["backend"]
    ).unwrap();
}

// Usage
let timer = REQUEST_DURATION
    .with_label_values(&[namespace, "get", "postgres"])
    .start_timer();

// ... do work ...

timer.observe_duration();

REQUEST_COUNT
    .with_label_values(&[namespace, "get", "success"])
    .inc();
```

**Dashboards**:
- **Golden Signals**: Latency, Traffic, Errors, Saturation
- **Per-Namespace**: Breakdown by namespace
- **Per-Backend**: Backend-specific metrics

### Structured Logging

**Format**: JSON for machine parsing

```rust
use tracing::{info, error, instrument};
use tracing_subscriber::fmt::format::json;

#[instrument(
    skip(request),
    fields(
        request_id = %request.id,
        namespace = %request.namespace,
        operation = %request.operation,
    )
)]
async fn handle_request(request: Request) -> Result<Response> {
    info!("Processing request");

    match process(&request).await {
        Ok(response) => {
            info!(
                latency_ms = response.latency_ms,
                backend = %response.backend,
                "Request succeeded"
            );
            Ok(response)
        }
        Err(e) => {
            error!(
                error = %e,
                error_kind = ?e.kind(),
                "Request failed"
            );
            Err(e)
        }
    }
}
```

**Log Output**:
```json
{
  "timestamp": "2025-10-05T12:34:56.789Z",
  "level": "INFO",
  "message": "Request succeeded",
  "request_id": "req-abc-123",
  "namespace": "user-profiles",
  "operation": "get",
  "latency_ms": 2.3,
  "backend": "postgres",
  "span": {
    "name": "handle_request",
    "trace_id": "0af7651916cd43dd8448eb211c80319c"
  }
}
```

**Log Levels**:
- `ERROR`: Something failed, needs immediate attention
- `WARN`: Something unexpected, may need attention
- `INFO`: Important events (requests, config changes)
- `DEBUG`: Detailed events (SQL queries, cache hits)
- `TRACE`: Very verbose (every function call)

Production: `INFO` level
Development: `DEBUG` level

### Distributed Tracing

**Trace Example**:

GET /namespaces/user-profiles/items/user123
│
├─ [2.5ms] prism.proxy.handle_request
│  │
│  ├─ [0.1ms] prism.authz.authorize
│  │
│  ├─ [0.2ms] prism.router.route
│  │
│  └─ [2.1ms] prism.backend.postgres.get
│     │
│     ├─ [0.3ms] postgres.acquire_connection
│     │
│     └─ [1.7ms] postgres.execute_query
│        │
│        └─ [1.5ms] SELECT FROM user_profiles WHERE id = $1
```

**Implementation**:

```rust
use opentelemetry::trace::{Tracer, SpanKind};
use tracing_opentelemetry::OpenTelemetryLayer;

#[instrument]
async fn handle_request(request: Request) -> Result<Response> {
    let span = tracing::Span::current();

    // Add attributes
    span.record("namespace", &request.namespace);
    span.record("operation", &request.operation);

    // Child span for backend call
    let response = {
        let _guard = tracing::info_span!("backend.get",
            backend = "postgres"
        ).entered();

        backend.get(&request).await?
    };

    span.record("latency_ms", response.latency_ms);
    Ok(response)
}
```

### Sampling

Full traces are expensive. Sample based on:

```rust
use opentelemetry::sdk::trace::{Sampler, SamplingDecision};

pub struct AdaptiveSampler {
    // Always sample errors
    // Always sample slow requests (> 100ms)
    // Sample 1% of normal requests
}

impl Sampler for AdaptiveSampler {
    fn should_sample(&self, context: &SamplingContext) -> SamplingDecision {
        // Error? Always sample
        if context.has_error {
            return SamplingDecision::RecordAndSample;
        }

        // Slow? Always sample
        if context.duration > Duration::from_millis(100) {
            return SamplingDecision::RecordAndSample;
        }

        // Otherwise, 1% sample rate
        if rand::random::<f64>() < 0.01 {
            return SamplingDecision::RecordAndSample;
        }

        SamplingDecision::Drop
    }
}
```

### Alerts

**Critical Alerts** (page on-call):

```yaml
- alert: PrismHighErrorRate
  expr: |
    (
      sum(rate(prism_requests_total{status="error"}[5m]))
      /
      sum(rate(prism_requests_total[5m]))
    ) > 0.01
  for: 2m
  annotations:
    summary: "Prism error rate > 1%"

- alert: PrismHighLatency
  expr: |
    histogram_quantile(0.99,
      rate(prism_request_duration_seconds_bucket[5m])
    ) > 0.1
  for: 5m
  annotations:
    summary: "Prism P99 latency > 100ms"

- alert: PrismDown
  expr: up{job="prism"} == 0
  for: 1m
  annotations:
    summary: "Prism instance is down"
```

**Warning Alerts** (Slack notification):

```yaml
- alert: PrismElevatedLatency
  expr: |
    histogram_quantile(0.99,
      rate(prism_request_duration_seconds_bucket[5m])
    ) > 0.05
  for: 10m
  annotations:
    summary: "Prism P99 latency > 50ms"

- alert: PrismHighCacheEvictionRate
  expr: rate(prism_cache_evictions_total[5m]) > 100
  for: 5m
  annotations:
    summary: "High cache eviction rate"
```

### Alternatives Considered

1. **Roll Our Own Metrics**
   - Pros: Full control
   - Cons: Reinventing the wheel, no ecosystem
   - Rejected: Not worth the effort

2. **Datadog/New Relic (Commercial)**
   - Pros: Turnkey solution, great UI
   - Cons: Expensive, vendor lock-in
   - Rejected: Prefer open source

3. **Jaeger Only** (no Prometheus)
   - Pros: Simpler stack
   - Cons: Traces alone insufficient for monitoring
   - Rejected: Need metrics for alerts

4. **No Structured Logging**
   - Pros: Simpler
   - Cons: Hard to query, no context
   - Rejected: Structured logs essential

## Consequences

### Positive

- **Comprehensive Visibility**: Metrics, logs, traces cover all aspects
- **Vendor Neutral**: Can switch backends (Tempo, Grafana Cloud, etc.)
- **Industry Standard**: OpenTelemetry is well-supported
- **Debugging Power**: Distributed traces show exact request flow

### Negative

- **Resource Overhead**: Metrics/traces use CPU/memory
  - *Mitigation*: Sampling, async export
- **Operational Complexity**: More services to run (Prometheus, Loki, Jaeger)
  - *Mitigation*: Use managed services or existing infrastructure

### Neutral

- **Learning Curve**: Team must learn OpenTelemetry
  - Good investment; transferable skill

## Implementation Notes

### OpenTelemetry Setup

```rust
use opentelemetry::sdk::trace::{self, Tracer};
use opentelemetry::global;
use tracing_subscriber::{layer::SubscriberExt, Registry};
use tracing_opentelemetry::OpenTelemetryLayer;

fn init_telemetry() -> Result<()> {
    // Tracer
    let tracer = opentelemetry_jaeger::new_pipeline()
        .with_service_name("prism-proxy")
        .with_agent_endpoint("jaeger:6831")
        .install_batch(opentelemetry::runtime::Tokio)?;

    // Logging + tracing layer
    let telemetry_layer = OpenTelemetryLayer::new(tracer);

    let subscriber = Registry::default()
        .with(tracing_subscriber::fmt::layer().json())
        .with(telemetry_layer);

    tracing::subscriber::set_global_default(subscriber)?;

    // Metrics
    let prometheus_exporter = opentelemetry_prometheus::exporter()
        .with_registry(prometheus::default_registry().clone())
        .init();

    global::set_meter_provider(prometheus_exporter);

    Ok(())
}
```

### Context Propagation

```rust
// Extract trace context from gRPC metadata
use tonic::metadata::MetadataMap;
use opentelemetry::propagation::TextMapPropagator;

fn extract_trace_context(metadata: &MetadataMap) -> Context {
    let propagator = TraceContextPropagator::new();
    let extractor = MetadataExtractor(metadata);
    propagator.extract(&extractor)
}

// Inject trace context when calling backend
fn inject_trace_context(context: &Context) -> MetadataMap {
    let propagator = TraceContextPropagator::new();
    let mut metadata = MetadataMap::new();
    let mut injector = MetadataInjector(&mut metadata);
    propagator.inject_context(context, &mut injector);
    metadata
}
```

## References

- [OpenTelemetry Specification](https://opentelemetry.io/docs/specs/otel/)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [Google SRE - Monitoring Distributed Systems](https://sre.google/sre-book/monitoring-distributed-systems/)
- [Tracing in Rust](https://tokio.rs/tokio/topics/tracing)

## Revision History

- 2025-10-05: Initial draft and acceptance
