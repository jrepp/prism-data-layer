---
date: 2025-10-09
deciders: Core Team
doc_uuid: 2cbe9b6e-ecf7-4c00-9c7c-dae8aebaa5cf
id: adr-047
project_id: prism-data-layer
status: Accepted
tags:
- observability
- tracing
- opentelemetry
- go
- rust
- plugin
title: OpenTelemetry Tracing Integration
---

## Context

Prism's request flow spans multiple components:
- **Client** (application)
- **Proxy** (Rust) - gRPC server, authentication, routing
- **Plugin** (Go/Rust) - Backend-specific protocol implementation
- **Backend** (PostgreSQL, Kafka, Redis, etc.) - Data storage

Debugging issues requires understanding the full request path. Without distributed tracing:
- **No visibility** into plugin-level operations
- **Can't correlate** proxy logs with plugin logs
- **Hard to identify** bottlenecks (is it proxy routing? plugin processing? backend query?)
- **No end-to-end latency** breakdown

ADR-008 established OpenTelemetry as our observability strategy. This ADR details the implementation of distributed tracing across Rust proxy and Go plugins, with trace context propagation at every hop.

## Decision

Implement **end-to-end OpenTelemetry tracing** across all Prism components:

1. **Rust Proxy**: Use `tracing` + `tracing-opentelemetry` crates
2. **Go Plugins**: Use `go.opentelemetry.io/otel` in plugin core library
3. **Trace Propagation**: W3C Trace Context via gRPC metadata
4. **Plugin Core Integration**: Automatic tracing middleware in `plugins/core` package
5. **Sampling Strategy**: Adaptive sampling (100% errors, 100% slow requests, 1% normal)

## Rationale

### Why End-to-End Tracing?

**Problem**: Request takes 150ms. Where is the time spent?

**With tracing**:
prism.handle_request           [150ms total]
├─ prism.auth.verify            [5ms]
├─ prism.routing.select_plugin  [2ms]
├─ plugin.postgres.execute      [140ms]  ← Bottleneck found!
│  ├─ plugin.pool.acquire       [3ms]
│  └─ postgres.query            [137ms]
│     └─ SQL: SELECT * FROM... [135ms]
└─ prism.response.serialize     [3ms]
```text

### Architecture

```
sequenceDiagram
    participant Client
    participant Proxy as Rust Proxy
    participant Plugin as Go Plugin
    participant Backend as Backend (PostgreSQL)

    Client->>Proxy: gRPC Request<br/>(traceparent header)
    Note over Proxy: Extract trace context<br/>Create root span<br/>trace_id: abc123

    Proxy->>Proxy: Auth span<br/>(parent: abc123)
    Proxy->>Proxy: Routing span<br/>(parent: abc123)

    Proxy->>Plugin: gRPC Plugin Call<br/>(inject traceparent)
    Note over Plugin: Extract trace context<br/>Create child span<br/>same trace_id: abc123

    Plugin->>Backend: PostgreSQL Query<br/>(with trace context)
    Backend-->>Plugin: Result

    Plugin-->>Proxy: gRPC Response
    Proxy-->>Client: gRPC Response

    Note over Client,Backend: All spans linked by<br/>trace_id: abc123
```text

### Trace Context Propagation

Use **W3C Trace Context** standard:

traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
             │  │                                │                └─ flags (sampled)
             │  │                                └─ span_id (16 hex)
             │  └─ trace_id (32 hex)
             └─ version
```

**Propagation Flow**:
1. Client → Proxy: gRPC metadata `traceparent`
2. Proxy → Plugin: gRPC metadata `traceparent`
3. Plugin → Backend: SQL comments or protocol-specific tags

## Implementation

### 1. Rust Proxy Integration

#### Proxy Initialization

```rust
// proxy/src/telemetry.rs
use opentelemetry::sdk::trace::{self, Tracer, Sampler};
use opentelemetry::global;
use tracing_subscriber::{layer::SubscriberExt, Registry, EnvFilter};
use tracing_opentelemetry::OpenTelemetryLayer;

pub fn init_telemetry() -> Result<()> {
    // Configure OpenTelemetry tracer with Jaeger exporter
    let tracer = opentelemetry_jaeger::new_pipeline()
        .with_service_name("prism-proxy")
        .with_agent_endpoint("jaeger:6831")
        .with_trace_config(
            trace::config()
                .with_sampler(AdaptiveSampler::new())
                .with_resource(opentelemetry::sdk::Resource::new(vec![
                    opentelemetry::KeyValue::new("service.name", "prism-proxy"),
                    opentelemetry::KeyValue::new("service.version", env!("CARGO_PKG_VERSION")),
                ]))
        )
        .install_batch(opentelemetry::runtime::Tokio)?;

    // Create OpenTelemetry tracing layer
    let telemetry_layer = OpenTelemetryLayer::new(tracer);

    // Combine with structured logging
    let subscriber = Registry::default()
        .with(EnvFilter::from_default_env())
        .with(tracing_subscriber::fmt::layer().json())
        .with(telemetry_layer);

    tracing::subscriber::set_global_default(subscriber)?;

    Ok(())
}

/// Adaptive sampler: 100% errors, 100% slow (>100ms), 1% normal
pub struct AdaptiveSampler;

impl AdaptiveSampler {
    pub fn new() -> Self {
        Self
    }
}

impl Sampler for AdaptiveSampler {
    fn should_sample(
        &self,
        parent_context: Option<&opentelemetry::Context>,
        trace_id: opentelemetry::trace::TraceId,
        name: &str,
        _span_kind: &opentelemetry::trace::SpanKind,
        attributes: &[opentelemetry::KeyValue],
        _links: &[opentelemetry::trace::Link],
    ) -> opentelemetry::sdk::trace::SamplingResult {
        use opentelemetry::sdk::trace::SamplingDecision;

        // Always sample if parent is sampled
        if let Some(ctx) = parent_context {
            if ctx.span().span_context().is_sampled() {
                return SamplingResult {
                    decision: SamplingDecision::RecordAndSample,
                    attributes: vec![],
                    trace_state: Default::default(),
                };
            }
        }

        // Check for error attribute
        let has_error = attributes.iter()
            .any(|kv| kv.key.as_str() == "error" && kv.value.as_str() == "true");

        if has_error {
            return SamplingResult {
                decision: SamplingDecision::RecordAndSample,
                attributes: vec![],
                trace_state: Default::default(),
            };
        }

        // Check for high latency (set by instrumentation)
        let is_slow = attributes.iter()
            .any(|kv| kv.key.as_str() == "slow" && kv.value.as_str() == "true");

        if is_slow {
            return SamplingResult {
                decision: SamplingDecision::RecordAndSample,
                attributes: vec![],
                trace_state: Default::default(),
            };
        }

        // Otherwise 1% sample rate
        let sample = (trace_id.to_u128() % 100) == 0;

        SamplingResult {
            decision: if sample {
                SamplingDecision::RecordAndSample
            } else {
                SamplingDecision::Drop
            },
            attributes: vec![],
            trace_state: Default::default(),
        }
    }
}
```

#### gRPC Request Handler

```rust
// proxy/src/grpc/handler.rs
use tonic::{Request, Response, Status};
use opentelemetry::propagation::{Extractor, Injector, TextMapPropagator};
use opentelemetry_sdk::propagation::TraceContextPropagator;
use tracing::{instrument, info, error};

/// Extract trace context from gRPC metadata
struct MetadataExtractor<'a>(&'a tonic::metadata::MetadataMap);

impl<'a> Extractor for MetadataExtractor<'a> {
    fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).and_then(|v| v.to_str().ok())
    }

    fn keys(&self) -> Vec<&str> {
        self.0.keys().map(|k| k.as_str()).collect()
    }
}

/// Inject trace context into gRPC metadata
struct MetadataInjector<'a>(&'a mut tonic::metadata::MetadataMap);

impl<'a> Injector for MetadataInjector<'a> {
    fn set(&mut self, key: &str, value: String) {
        if let Ok(metadata_value) = tonic::metadata::MetadataValue::try_from(&value) {
            self.0.insert(
                tonic::metadata::MetadataKey::from_bytes(key.as_bytes()).unwrap(),
                metadata_value
            );
        }
    }
}

#[instrument(
    skip(request, plugin_client),
    fields(
        request_id = %request.get_ref().request_id,
        namespace = %request.get_ref().namespace,
        operation = %request.get_ref().operation,
        trace_id = tracing::field::Empty,
    )
)]
pub async fn handle_data_request(
    request: Request<DataRequest>,
    plugin_client: &PluginClient,
) -> Result<Response<DataResponse>, Status> {
    // Extract trace context from incoming request
    let propagator = TraceContextPropagator::new();
    let parent_context = propagator.extract(&MetadataExtractor(request.metadata()));

    // Set current trace context
    let span = tracing::Span::current();
    if let Some(span_ref) = parent_context.span().span_context().trace_id().to_string() {
        span.record("trace_id", &tracing::field::display(&span_ref));
    }

    info!("Processing data request");

    // Create child span for plugin call
    let plugin_response = {
        let _plugin_span = tracing::info_span!(
            "plugin.execute",
            plugin = "postgres",
            backend = "postgresql"
        ).entered();

        // Inject trace context into plugin request metadata
        let mut plugin_metadata = tonic::metadata::MetadataMap::new();
        let current_context = opentelemetry::Context::current();
        propagator.inject_context(&current_context, &mut MetadataInjector(&mut plugin_metadata));

        let mut plugin_req = tonic::Request::new(request.into_inner());
        *plugin_req.metadata_mut() = plugin_metadata;

        plugin_client.execute(plugin_req).await
            .map_err(|e| {
                error!(error = %e, "Plugin execution failed");
                Status::internal("Plugin error")
            })?
    };

    info!("Request completed successfully");
    Ok(plugin_response)
}
```

### 2. Go Plugin Core Integration

#### Plugin Core Library (`plugins/core/tracing`)

```go
// plugins/core/tracing/tracing.go
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

// InitTracer initializes OpenTelemetry tracing for a plugin
func InitTracer(pluginName, pluginVersion string) (func(context.Context) error, error) {
	// Create Jaeger exporter
	exporter, err := jaeger.New(jaeger.WithAgentEndpoint())
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(fmt.Sprintf("prism-plugin-%s", pluginName)),
			semconv.ServiceVersionKey.String(pluginVersion),
			attribute.String("plugin.name", pluginName),
		)),
		sdktrace.WithSampler(newAdaptiveSampler()),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)

	// Set global propagator (W3C Trace Context)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return cleanup function
	return tp.Shutdown, nil
}

// ExtractTraceContext extracts trace context from gRPC incoming metadata
func ExtractTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	// Create propagator
	propagator := otel.GetTextMapPropagator()

	// Extract trace context from metadata
	return propagator.Extract(ctx, &metadataSupplier{md})
}

// InjectTraceContext injects trace context into gRPC outgoing metadata
func InjectTraceContext(ctx context.Context) context.Context {
	md := metadata.MD{}

	// Create propagator
	propagator := otel.GetTextMapPropagator()

	// Inject trace context into metadata
	propagator.Inject(ctx, &metadataSupplier{md})

	return metadata.NewOutgoingContext(ctx, md)
}

// metadataSupplier implements TextMapCarrier for gRPC metadata
type metadataSupplier struct {
	metadata metadata.MD
}

func (s *metadataSupplier) Get(key string) string {
	values := s.metadata.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (s *metadataSupplier) Set(key, value string) {
	s.metadata.Set(key, value)
}

func (s *metadataSupplier) Keys() []string {
	keys := make([]string, 0, len(s.metadata))
	for k := range s.metadata {
		keys = append(keys, k)
	}
	return keys
}

// adaptiveSampler implements same logic as Rust proxy
type adaptiveSampler struct {
	fallback sdktrace.Sampler
}

func newAdaptiveSampler() sdktrace.Sampler {
	return &adaptiveSampler{
		fallback: sdktrace.TraceIDRatioBased(0.01), // 1% for normal requests
	}
}

func (s *adaptiveSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	// Always sample if parent is sampled
	if p.ParentContext.IsSampled() {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: trace.SpanContextFromContext(p.ParentContext).TraceState(),
		}
	}

	// Check for error attribute
	for _, attr := range p.Attributes {
		if attr.Key == "error" && attr.Value.AsString() == "true" {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.RecordAndSample,
				Tracestate: trace.SpanContextFromContext(p.ParentContext).TraceState(),
			}
		}
	}

	// Fallback to ratio-based sampling (1%)
	return s.fallback.ShouldSample(p)
}

func (s *adaptiveSampler) Description() string {
	return "AdaptiveSampler{errors=100%, slow=100%, normal=1%}"
}
```

#### Plugin gRPC Interceptor

```go
// plugins/core/tracing/interceptor.go
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// UnaryServerInterceptor creates a gRPC interceptor for automatic tracing
func UnaryServerInterceptor(pluginName string) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(fmt.Sprintf("prism-plugin-%s", pluginName))

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract trace context from incoming metadata
		ctx = ExtractTraceContext(ctx)

		// Start new span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", pluginName),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		// Call handler
		resp, err := handler(ctx, req)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}
```

### 3. Plugin Implementation Example

```go
// plugins/postgres/main.go
package main

import (
	"context"
	"log"

	"github.com/jrepp/prism-data-layer/plugins/core/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
)

func main() {
	// Initialize tracing
	shutdown, err := tracing.InitTracer("postgres", "1.0.0")
	if err != nil {
		log.Fatalf("Failed to initialize tracing: %v", err)
	}
	defer shutdown(context.Background())

	// Create gRPC server with tracing interceptor
	server := grpc.NewServer(
		grpc.UnaryInterceptor(tracing.UnaryServerInterceptor("postgres")),
	)

	// Register plugin service
	RegisterPluginService(server, &PostgresPlugin{})

	// Start server...
}

// PostgresPlugin implements plugin with tracing
type PostgresPlugin struct {
	tracer trace.Tracer
}

func (p *PostgresPlugin) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	// Trace context automatically extracted by interceptor
	// Create child span for database operation
	tracer := otel.Tracer("prism-plugin-postgres")
	ctx, span := tracer.Start(ctx, "postgres.query",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", req.Operation),
			attribute.String("db.table", req.Table),
		),
	)
	defer span.End()

	// Execute query (trace context propagated)
	result, err := p.db.QueryContext(ctx, req.Query)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("db.rows", result.RowsAffected))
	return &ExecuteResponse{Data: result}, nil
}
```

### 4. Backend Trace Context Propagation

#### PostgreSQL (SQL Comments)

```go
// Inject trace context as SQL comment
func addTraceContextToQuery(ctx context.Context, query string) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return query
	}

	comment := fmt.Sprintf(
		"/* traceparent='%s' */",
		spanCtx.TraceID().String(),
	)

	return comment + " " + query
}

// Usage
query := "SELECT * FROM users WHERE id = $1"
tracedQuery := addTraceContextToQuery(ctx, query)
// Result: /* traceparent='0af765...' */ SELECT * FROM users WHERE id = $1
```

#### Redis (Tags)

```go
// Add trace context to Redis commands
func executeWithTracing(ctx context.Context, cmd string, args ...interface{}) error {
	spanCtx := trace.SpanContextFromContext(ctx)

	// Add trace_id as tag
	if spanCtx.IsValid() {
		args = append(args, "trace_id", spanCtx.TraceID().String())
	}

	return redisClient.Do(ctx, cmd, args...)
}
```

### 5. Testing Tracing

```go
// plugins/core/tracing/tracing_test.go
package tracing_test

import (
	"context"
	"testing"

	"github.com/jrepp/prism-data-layer/plugins/core/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc/metadata"
)

func TestTraceContextPropagation(t *testing.T) {
	// Create in-memory span recorder
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)

	// Create parent span
	ctx := context.Background()
	tracer := tp.Tracer("test")
	ctx, parentSpan := tracer.Start(ctx, "parent")
	defer parentSpan.End()

	// Inject trace context into metadata
	ctx = tracing.InjectTraceContext(ctx)

	// Simulate gRPC call - extract on receiver side
	md, _ := metadata.FromOutgoingContext(ctx)
	receiverCtx := metadata.NewIncomingContext(context.Background(), md)
	receiverCtx = tracing.ExtractTraceContext(receiverCtx)

	// Create child span in receiver
	_, childSpan := tracer.Start(receiverCtx, "child")
	childSpan.End()

	// Verify trace lineage
	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("Expected 2 spans, got %d", len(spans))
	}

	parentTraceID := spans[0].SpanContext().TraceID()
	childTraceID := spans[1].SpanContext().TraceID()

	if parentTraceID != childTraceID {
		t.Errorf("Child span has different trace ID: parent=%s, child=%s",
			parentTraceID, childTraceID)
	}

	if spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() {
		t.Error("Child span not linked to parent")
	}
}
```

## Alternatives Considered

### 1. Custom Tracing Implementation
- **Pros**: Full control, minimal dependencies
- **Cons**: Reinventing the wheel, no ecosystem, hard to maintain
- **Rejected**: OpenTelemetry is industry standard

### 2. Jaeger-Only (No OpenTelemetry)
- **Pros**: Simpler, direct Jaeger integration
- **Cons**: Vendor lock-in, no metrics/logs correlation
- **Rejected**: OpenTelemetry provides vendor neutrality

### 3. No Plugin Tracing
- **Pros**: Simpler plugin implementation
- **Cons**: No visibility into plugin internals, hard to debug
- **Rejected**: Plugin performance is critical to debug

### 4. Separate Trace IDs per Component
- **Pros**: Simpler (no context propagation)
- **Cons**: Can't correlate proxy and plugin spans
- **Rejected**: End-to-end correlation essential

## Consequences

### Positive

✅ **Complete Visibility**: Full request path from client to backend
✅ **Bottleneck Identification**: Know exactly where time is spent
✅ **Cross-Language Tracing**: Rust proxy + Go plugins seamlessly connected
✅ **Production Debugging**: Trace sampling captures errors and slow requests
✅ **Plugin Core Simplicity**: Automatic tracing via interceptor, minimal plugin code

### Negative

❌ **Resource Overhead**: Tracing adds CPU/memory cost (~2-5%)
  - *Mitigation*: Adaptive sampling (1% normal requests)

❌ **Complexity**: Developers must understand trace context propagation
  - *Mitigation*: Plugin core library handles propagation automatically

❌ **Storage Cost**: Traces require storage (Jaeger/Tempo backend)
  - *Mitigation*: Retention policies (7 days default)

### Neutral

⚪ **Learning Curve**: Team must learn OpenTelemetry concepts
  - Industry-standard skill, valuable beyond Prism

⚪ **Backend-Specific Propagation**: Each backend (PostgreSQL, Redis) needs custom trace injection
  - One-time implementation per backend type

## Implementation Notes

### Deployment Configuration

```yaml
# docker-compose.yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"  # Jaeger UI
      - "6831:6831/udp"  # Jaeger agent (UDP)
    environment:
      COLLECTOR_ZIPKIN_HOST_PORT: ":9411"

  prism-proxy:
    image: prism/proxy:latest
    environment:
      OTEL_EXPORTER_JAEGER_AGENT_HOST: jaeger
      OTEL_EXPORTER_JAEGER_AGENT_PORT: 6831
      OTEL_SERVICE_NAME: prism-proxy
    depends_on:
      - jaeger

  postgres-plugin:
    image: prism/plugin-postgres:latest
    environment:
      OTEL_EXPORTER_JAEGER_AGENT_HOST: jaeger
      OTEL_EXPORTER_JAEGER_AGENT_PORT: 6831
      OTEL_SERVICE_NAME: prism-plugin-postgres
    depends_on:
      - jaeger
```

### Trace Example (Jaeger UI)

Trace ID: 0af7651916cd43dd8448eb211c80319c
Duration: 142ms
Spans: 7

prism-proxy: handle_data_request [142ms]
├─ prism-proxy: auth.verify [3ms]
├─ prism-proxy: routing.select_plugin [1ms]
├─ prism-proxy: plugin.execute [137ms]
│  │
│  └─ prism-plugin-postgres: Execute [136ms]
│     ├─ prism-plugin-postgres: pool.acquire [2ms]
│     └─ prism-plugin-postgres: postgres.query [134ms]
│        └─ postgresql: SELECT * FROM users WHERE id = $1 [132ms]
```text

## References

- [ADR-008: Observability Strategy](/adr/adr-008) - High-level observability architecture
- [OpenTelemetry Specification](https://opentelemetry.io/docs/specs/otel/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [OpenTelemetry Rust](https://github.com/open-telemetry/opentelemetry-rust)
- [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go)
- [Tracing in Rust with Tokio](https://tokio.rs/tokio/topics/tracing)
- RFC-008: Plugin Architecture - Plugin core library design

## Revision History

- 2025-10-09: Initial draft and acceptance

```