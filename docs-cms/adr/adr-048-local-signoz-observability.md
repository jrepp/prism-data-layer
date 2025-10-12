---
date: 2025-10-09
deciders: Platform Team
doc_uuid: 117474f6-d253-4b91-a96f-298f750e9fd3
id: adr-048
project_id: prism-data-layer
status: Accepted
tags:
- observability
- testing
- local-development
- opentelemetry
- signoz
title: 'ADR-048: Local Signoz Instance for Observability Testing'
---

# ADR-048: Local Signoz Instance for Observability Testing

## Status

**Accepted** - 2025-10-09

## Context

### Problem Statement

Developers need comprehensive observability during local development and testing:
- **Traces**: See request flows through proxy → plugin → backend
- **Metrics**: Monitor latency, throughput, error rates
- **Logs**: Correlate structured logs with traces
- **OpenTelemetry**: Native OTLP support for Prism instrumentation

**Current Situation:**
- No local observability stack
- Developers rely on console logs and printf debugging
- Hard to debug distributed systems issues (proxy + plugins + backends)
- No way to validate OpenTelemetry instrumentation before production

**Requirements:**
1. Full OpenTelemetry stack (traces, metrics, logs)
2. Runs independently from other local infrastructure
3. Minimal resource footprint (< 2GB RAM)
4. Fast startup (< 30 seconds)
5. Pre-configured for Prism components
6. Persist data across restarts

### Why Signoz?

**Evaluated Options:**
- Jaeger (traces only)
- Prometheus + Grafana (metrics + visualization, complex setup)
- Elastic Stack (heavy, 4GB+ RAM)
- **Signoz** (all-in-one OpenTelemetry platform)

**Signoz Advantages:**
- ✅ Native OpenTelemetry: OTLP gRPC/HTTP receivers built-in
- ✅ All-in-one: Traces, metrics, logs in single UI
- ✅ Lightweight: ~1.5GB RAM with ClickHouse backend
- ✅ Fast queries: ClickHouse columnar storage
- ✅ APM features: Service maps, dependency graphs, alerts
- ✅ Open source: Apache 2.0 license
- ✅ Docker Compose: Pre-built stack available

**Comparison:**

| Feature | Signoz | Jaeger | Prometheus + Grafana | Elastic Stack |
|---------|--------|--------|---------------------|---------------|
| Traces | ✅ | ✅ | ❌ | ✅ |
| Metrics | ✅ | ❌ | ✅ | ✅ |
| Logs | ✅ | ❌ | ❌ (via Loki) | ✅ |
| OpenTelemetry Native | ✅ | ✅ | ❌ | ⚠️ (via collector) |
| Resource Usage | ~1.5GB | ~500MB | ~2GB | ~4GB |
| Setup Complexity | Low | Low | Medium | High |
| Query Performance | Fast (ClickHouse) | Medium | Fast (Prometheus) | Medium |
| APM Features | ✅ | ⚠️ Basic | ⚠️ Manual | ✅ |

**Signoz Architecture:**
Signoz Components:
├── OTLP Receiver (gRPC :4317, HTTP :4318)
├── Query Service (API + UI :3301)
├── ClickHouse (storage :9000)
└── AlertManager (optional)
```text

## Decision

**We will provide a local Signoz instance as part of the development support tooling.**

### Key Decisions

1. **Signoz as Standard Observability Platform**
   - Use Signoz for local development observability
   - Pre-configured for Prism proxy, plugins, and backends
   - Standardize on OpenTelemetry for all instrumentation

2. **Independent Docker Compose Stack**
   - Separate `docker-compose.signoz.yml` file
   - Not bundled with backend testing compose files
   - Can run independently or alongside other stacks
   - Uses dedicated Docker network with bridge to Prism

3. **Pre-configured Services**
   - Prism proxy: Auto-configured OTLP endpoint
   - Backend plugins: Environment variables for OTLP
   - Admin service: Traces and metrics enabled
   - Example applications: Sample instrumentation

4. **Data Persistence**
   - Volume mounts for ClickHouse data
   - Survives container restarts
   - Optional: Reset script to clear all data

5. **Resource Management**
   - Memory limit: 2GB total (1.5GB ClickHouse + 500MB services)
   - CPU limit: 2 cores
   - Port conflicts avoided (custom port range)

## Implementation Details

### Docker Compose Configuration

Location: `local-dev/signoz/docker-compose.signoz.yml`

```
version: '3.8'

services:
  # ClickHouse database for storing telemetry data
  clickhouse:
    image: clickhouse/clickhouse-server:23.7-alpine
    container_name: prism-signoz-clickhouse
    volumes:
      - signoz-clickhouse-data:/var/lib/clickhouse
      - ./clickhouse-config.xml:/etc/clickhouse-server/config.d/logging.xml:ro
    environment:
      - CLICKHOUSE_DB=signoz
    ports:
      - "9001:9000"  # Avoid conflict with MinIO (9000)
      - "8124:8123"  # HTTP interface
    mem_limit: 1.5g
    cpus: 1.5
    networks:
      - signoz
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8123/ping"]
      interval: 10s
      timeout: 5s
      retries: 3

  # Signoz Query Service (API + UI)
  query-service:
    image: signoz/query-service:0.39.0
    container_name: prism-signoz-query
    command: ["-config=/root/config/prometheus.yml"]
    volumes:
      - ./signoz-config.yaml:/root/config/prometheus.yml
    environment:
      - ClickHouseUrl=tcp://clickhouse:9000
      - STORAGE=clickhouse
    ports:
      - "3301:8080"  # Query Service API + UI
    depends_on:
      clickhouse:
        condition: service_healthy
    mem_limit: 256m
    cpus: 0.5
    networks:
      - signoz
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8080/api/v1/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  # OpenTelemetry Collector (OTLP receiver)
  otel-collector:
    image: signoz/signoz-otel-collector:0.79.9
    container_name: prism-signoz-otel
    command: ["--config=/etc/otel-collector-config.yaml"]
    volumes:
      - ./otel-collector-config.yaml:/etc/otel-collector-config.yaml
    environment:
      - CLICKHOUSE_URL=clickhouse:9000
    ports:
      - "4317:4317"  # OTLP gRPC receiver
      - "4318:4318"  # OTLP HTTP receiver
    depends_on:
      clickhouse:
        condition: service_healthy
    mem_limit: 256m
    cpus: 0.5
    networks:
      - signoz
      - prism  # Bridge to Prism components
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:13133/"]
      interval: 10s
      timeout: 5s
      retries: 3

  # AlertManager (optional, for local alert testing)
  alertmanager:
    image: signoz/alertmanager:0.23.4
    container_name: prism-signoz-alertmanager
    volumes:
      - ./alertmanager-config.yaml:/etc/alertmanager/alertmanager.yml
    ports:
      - "9093:9093"
    mem_limit: 128m
    cpus: 0.25
    networks:
      - signoz

volumes:
  signoz-clickhouse-data:
    driver: local

networks:
  signoz:
    driver: bridge
  prism:
    external: true  # Connect to Prism components
```text

### OpenTelemetry Collector Configuration

Location: `local-dev/signoz/otel-collector-config.yaml`

```
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
        cors:
          allowed_origins:
            - "http://localhost:*"
            - "http://127.0.0.1:*"

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024

  # Add resource attributes for Prism components
  resource:
    attributes:
      - key: deployment.environment
        value: "local"
        action: upsert
      - key: service.namespace
        value: "prism"
        action: upsert

  # Memory limiter to prevent OOM
  memory_limiter:
    check_interval: 5s
    limit_mib: 256
    spike_limit_mib: 64

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000?database=signoz
    ttl: 72h  # Keep data for 3 days in local dev

  # Debug exporter for troubleshooting
  logging:
    loglevel: info
    sampling_initial: 5
    sampling_thereafter: 200

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, resource, batch]
      exporters: [clickhouse]

    metrics:
      receivers: [otlp]
      processors: [memory_limiter, resource, batch]
      exporters: [clickhouse]

    logs:
      receivers: [otlp]
      processors: [memory_limiter, resource, batch]
      exporters: [clickhouse]
```text

### Prism Proxy Integration

The proxy automatically detects and uses Signoz when available:

```
// proxy/src/observability/tracer.rs

pub fn init_tracer(config: &ObservabilityConfig) -> Result<Tracer> {
    // Check for Signoz OTLP endpoint
    let otlp_endpoint = env::var("OTEL_EXPORTER_OTLP_ENDPOINT")
        .unwrap_or_else(|_| "http://".to_string() + "localhost:4317");

    let tracer = opentelemetry_otlp::new_pipeline()
        .tracing()
        .with_exporter(
            opentelemetry_otlp::new_exporter()
                .tonic()
                .with_endpoint(otlp_endpoint)
        )
        .with_trace_config(
            trace::config()
                .with_resource(Resource::new(vec![
                    KeyValue::new("service.name", "prism-proxy"),
                    KeyValue::new("service.version", env!("CARGO_PKG_VERSION")),
                    KeyValue::new("deployment.environment", "local"),
                ]))
        )
        .install_batch(opentelemetry::runtime::Tokio)?;

    Ok(tracer)
}
```text

### Plugin Integration

Plugins receive OTLP configuration via environment variables:

```
// plugins/core/observability/tracer.go

func InitTracer(serviceName string) error {
    // Reads OTEL_EXPORTER_OTLP_ENDPOINT from environment
    exporter, err := otlptracegrpc.New(
        context.Background(),
        otlptracegrpc.WithInsecure(),
        otlptracegrpc.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
    )
    if err != nil {
        return err
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
            semconv.DeploymentEnvironment("local"),
        )),
    )

    otel.SetTracerProvider(tp)
    return nil
}
```text

## Usage

### Starting Signoz

```
# Start Signoz stack
cd local-dev/signoz
docker-compose -f docker-compose.signoz.yml up -d

# Wait for healthy state
docker-compose -f docker-compose.signoz.yml ps

# Access UI
open http://localhost:3301
```text

### Starting Prism with Signoz

```
# Set OTLP endpoint environment variable
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# Start Prism proxy
cd proxy
cargo run --release

# Start plugin with OTLP
cd plugins/postgres
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
    go run ./cmd/server
```text

### Viewing Traces

1. Open Signoz UI: http://localhost:3301
2. Navigate to "Traces" tab
3. Filter by service: `prism-proxy`, `prism-plugin-postgres`
4. Click trace to see full waterfall

### Resetting Data

```
# Stop and remove volumes
docker-compose -f docker-compose.signoz.yml down -v

# Restart fresh
docker-compose -f docker-compose.signoz.yml up -d
```text

## Consequences

### Positive

1. **Comprehensive Observability**
   - Full visibility into distributed request flows
   - Correlate traces, metrics, and logs
   - Service dependency mapping

2. **Developer Productivity**
   - Faster debugging with trace waterfalls
   - Identify performance bottlenecks
   - Validate instrumentation locally

3. **Production Parity**
   - Same OpenTelemetry instrumentation local and production
   - Catch telemetry issues before deployment
   - Test alerting rules locally

4. **Low Friction**
   - Single command to start (`docker-compose up`)
   - Auto-discovery by Prism components
   - Pre-configured, zero setup

5. **Resource Efficient**
   - ~1.5GB RAM total
   - Fast queries with ClickHouse
   - Optional: disable when not needed

### Negative

1. **Additional Service to Manage**
   - One more Docker Compose stack
   - Requires keeping Signoz updated
   - Potential port conflicts (mitigated by custom ports)

2. **Learning Curve**
   - Developers need to understand Signoz UI
   - Query language for advanced filtering
   - Trace correlation concepts

3. **Data Cleanup**
   - ClickHouse data accumulates over time
   - Manual cleanup required (or automated scripts)

4. **Not a Full Production Solution**
   - Local instance for development only
   - Production needs scalable Signoz deployment
   - Different configuration for production

### Mitigation Strategies

1. **Documentation**: Comprehensive RFC-016 for setup and usage
2. **Scripts**: Automated start/stop/reset scripts
3. **Resource Limits**: Docker memory/CPU limits to prevent resource exhaustion
4. **Default Off**: Developers opt-in when needed (not running by default)

## Related Decisions

- [ADR-047: OpenTelemetry Tracing Integration](/adr/adr-047)
- [ADR-046: Dex IDP for Local Identity Testing](/adr/adr-046)
- [RFC-016: Local Development Infrastructure](/rfc/rfc-016-local-development-infrastructure) (this ADR's implementation guide)
- [RFC-008: Proxy Plugin Architecture](/rfc/rfc-008-proxy-plugin-architecture) (instrumentation points)

## References

- [Signoz Documentation](https://signoz.io/docs/)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
- [ClickHouse](https://clickhouse.com/)
- [OTLP Protocol Specification](https://opentelemetry.io/docs/specs/otlp/)

## Revision History

- 2025-10-09: Initial decision for local Signoz instance

```