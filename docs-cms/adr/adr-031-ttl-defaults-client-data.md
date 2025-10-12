---
date: 2025-10-08
deciders: System Architecture Team
doc_uuid: d7f3ac9f-862f-4093-ace3-6dfdbe8a4157
id: adr-031
project_id: prism-data-layer
sidebar_label: ADR-031 TTL Defaults
status: Accepted
tags:
- data-lifecycle
- cache
- cleanup
- operations
title: 'ADR-031: TTL Defaults for Client-Configured Data'
---

# ADR-031: TTL Defaults for Client-Configured Dynamic Data

## Context

Prism manages dynamic data on behalf of clients across multiple backends (Redis cache, session storage, temporary records, etc.). Without proper Time-To-Live (TTL) policies, this data accumulates indefinitely, leading to:

1. **Storage Exhaustion**: Backends run out of disk/memory
2. **Performance Degradation**: Large datasets slow down queries and operations
3. **Cost Overruns**: Cloud storage costs grow unchecked
4. **Stale Data**: Old data persists beyond its useful lifetime
5. **Operational Burden**: Manual cleanup becomes necessary

Clients may not always specify TTL values when creating dynamic data, either due to oversight, lack of domain knowledge, or API design that doesn't enforce it.

## Decision

**We will enforce TTL policies on all client-configured dynamic data with sensible defaults when clients do not specify explicit TTL values.**

### Principles

1. **TTL by Default**: Every piece of dynamic data MUST have a TTL
2. **Client Override**: Clients can specify custom TTL values
3. **Backend-Specific Defaults**: Different backends have different appropriate default TTLs
4. **Pattern-Specific Defaults**: Data access patterns inform default TTL values
5. **Monitoring**: Track TTL distribution and expiration rates
6. **Documentation**: Make defaults visible and well-documented

## TTL Default Strategy

### Default TTL Values by Pattern

| Pattern          | Backend      | Default TTL | Rationale                                  |
|------------------|--------------|-------------|--------------------------------------------|
| **Cache**        | Redis        | 15 minutes  | Most cache hits occur within minutes       |
| **Session**      | Redis        | 24 hours    | Typical user session lifetime              |
| **PubSub**       | NATS/Kafka   | N/A         | Messages consumed immediately              |
| **KeyValue**     | PostgreSQL   | 30 days     | Application data with moderate longevity   |
| **TimeSeries**   | ClickHouse   | 90 days     | Observability data retention standard      |
| **Graph**        | Neptune      | Infinite    | Relationships typically long-lived         |
| **Vector Search**| Redis VSS    | 90 days     | Embedding data for ML/search               |
| **Object Store** | MinIO        | 90 days     | Blob storage for artifacts/uploads         |

### TTL Precedence Rules

Client-Specified TTL
    ↓ (if not provided)
Namespace-Level Default TTL
    ↓ (if not configured)
Pattern-Specific Default TTL
    ↓ (fallback)
System-Wide Default TTL (30 days)
```text

### Implementation in Protobuf

```
// Client-specified TTL in requests
message SetRequest {
  string namespace = 1;
  string key = 2;
  bytes value = 3;

  // Optional: Client-specified TTL in seconds
  // If not provided, uses namespace or pattern defaults
  optional int64 ttl_seconds = 4;

  // Optional: Explicit infinite TTL (use with caution)
  optional bool no_expiration = 5;
}

// Namespace configuration with TTL defaults
message NamespaceConfig {
  string name = 1;
  string backend = 2;
  string pattern = 3;

  // Namespace-level TTL override (applies to all operations)
  optional int64 default_ttl_seconds = 4;

  // Allow clients to specify no expiration
  bool allow_infinite_ttl = 5 [default = false];

  // Warn when data approaches expiration
  bool enable_ttl_warnings = 6 [default = true];
}
```text

### Configuration YAML Example

```
namespaces:
  - name: user-sessions
    backend: redis
    pattern: cache
    default_ttl_seconds: 86400  # 24 hours
    allow_infinite_ttl: false

  - name: analytics-events
    backend: clickhouse
    pattern: timeseries
    default_ttl_seconds: 7776000  # 90 days
    enable_ttl_warnings: true

  - name: permanent-records
    backend: postgres
    pattern: keyvalue
    allow_infinite_ttl: true  # Explicit opt-in for infinite TTL
```text

## Backend-Specific Implementation

### Redis (Cache, Session, Vector)

```
// Rust implementation in Prism proxy
async fn set_with_ttl(
    &self,
    key: &str,
    value: &[u8],
    ttl: Option<Duration>,
    config: &NamespaceConfig,
) -> Result<()> {
    let effective_ttl = ttl
        .or(config.default_ttl)
        .unwrap_or(DEFAULT_CACHE_TTL); // 15 minutes

    self.redis
        .set_ex(key, value, effective_ttl.as_secs() as usize)
        .await
}
```text

### PostgreSQL (KeyValue)

```
-- Table schema with TTL support
CREATE TABLE keyvalue (
    namespace VARCHAR(255) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value BYTEA NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL, -- Always required
    PRIMARY KEY (namespace, key)
);

-- Index for efficient TTL-based cleanup
CREATE INDEX idx_keyvalue_expires ON keyvalue(expires_at)
WHERE expires_at IS NOT NULL;

-- Background job to delete expired records
DELETE FROM keyvalue
WHERE expires_at < NOW()
AND expires_at IS NOT NULL;
```text

### ClickHouse (TimeSeries)

```
-- ClickHouse table with TTL
CREATE TABLE events (
    timestamp DateTime64(9),
    event_type LowCardinality(String),
    namespace LowCardinality(String),
    payload String
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (namespace, event_type, timestamp)
TTL timestamp + INTERVAL 90 DAY; -- Default 90 days
```text

### MinIO (Object Storage)

```
# MinIO lifecycle policy
lifecycle_config = {
    "Rules": [
        {
            "ID": "default-ttl",
            "Status": "Enabled",
            "Expiration": {
                "Days": 90  # Default 90 days
            },
            "Filter": {
                "Prefix": "tmp/"  # Apply to temporary objects
            }
        }
    ]
}
minio_client.set_bucket_lifecycle("prism-objects", lifecycle_config)
```text

## Monitoring and Observability

### Metrics

```
message TTLMetrics {
  string namespace = 1;

  // TTL distribution
  int64 items_with_ttl = 2;
  int64 items_without_ttl = 3;  // Should be 0
  int64 items_infinite_ttl = 4;

  // Expiration stats
  int64 expired_last_hour = 5;
  int64 expiring_next_hour = 6;
  int64 expiring_next_day = 7;

  // Storage impact
  int64 total_bytes = 8;
  int64 bytes_to_expire_soon = 9;
}
```text

### Alerting Rules

```
# Prometheus alert rules
groups:
  - name: ttl_alerts
    rules:
      - alert: HighInfiniteTTLRatio
        expr: |
          (prism_items_infinite_ttl / prism_items_total) > 0.1
        for: 1h
        annotations:
          summary: "More than 10% of data has infinite TTL"

      - alert: StorageGrowthUnbounded
        expr: |
          rate(prism_total_bytes[1d]) > 0 AND
          rate(prism_expired_bytes[1d]) == 0
        for: 6h
        annotations:
          summary: "Storage growing without expiration"
```text

### Admin CLI Commands

```
# Show TTL distribution for namespace
prism namespace describe my-app --show-ttl-stats

# List items expiring soon
prism data list my-app --expiring-within 24h

# Override TTL for specific keys
prism data set-ttl my-app key123 --ttl 1h

# Disable expiration for specific item (requires permission)
prism data set-ttl my-app key456 --infinite
```text

## Client SDK Examples

### Python SDK

```
from prism_sdk import PrismClient

client = PrismClient(namespace="user-sessions")

# Explicit TTL (recommended)
client.set("session:abc123", session_data, ttl_seconds=3600)

# Uses namespace default (24 hours for sessions)
client.set("session:def456", session_data)

# Infinite TTL (requires namespace config: allow_infinite_ttl=true)
client.set("permanent:record", data, no_expiration=True)
```text

### Go SDK

```
client := prism.NewClient("user-sessions")

// Explicit TTL
client.Set(ctx, "session:abc123", sessionData,
    prism.WithTTL(1*time.Hour))

// Uses namespace default
client.Set(ctx, "session:def456", sessionData)

// Infinite TTL (opt-in required)
client.Set(ctx, "permanent:record", data,
    prism.WithNoExpiration())
```text

## Migration Path

### Phase 1: Audit and Baseline (Week 1)

1. **Audit existing data**: Identify data without TTL
2. **Baseline metrics**: Measure current storage usage and growth rates
3. **Document patterns**: Map data types to appropriate TTL ranges

### Phase 2: Implement Defaults (Week 2-3)

1. **Add protobuf fields**: `ttl_seconds`, `no_expiration`
2. **Update namespace configs**: Set pattern-specific defaults
3. **Backend implementations**: Apply TTL at storage layer
4. **Generate SDKs**: Update client libraries with TTL support

### Phase 3: Enforcement and Monitoring (Week 4)

1. **Enable TTL enforcement**: All new data gets TTL
2. **Backfill existing data**: Apply default TTL to legacy data
3. **Deploy monitoring**: Grafana dashboards, Prometheus alerts
4. **Documentation**: Update API docs and client guides

### Phase 4: Optimization (Ongoing)

1. **Review TTL distributions**: Adjust defaults based on usage
2. **Client feedback**: Refine TTL ranges per use case
3. **Cost analysis**: Track storage cost reductions
4. **Performance**: Measure impact on backend performance

## Consequences

### Positive

- **Bounded Storage**: Data automatically expires, preventing unbounded growth
- **Lower Costs**: Reduced storage requirements, especially in cloud environments
- **Better Performance**: Smaller datasets improve query and scan performance
- **Operational Safety**: No more manual cleanup scripts or emergency interventions
- **Clear Expectations**: Clients understand data lifecycle from the start

### Negative

- **Breaking Change**: Existing clients may rely on infinite TTL behavior
- **Migration Effort**: Backfilling TTL on existing data requires careful planning
- **Complexity**: TTL precedence rules and overrides add API surface area
- **Edge Cases**: Some use cases genuinely need infinite TTL (configuration, schemas)

### Mitigations

1. **Gradual Rollout**: Enable TTL enforcement gradually, namespace by namespace
2. **Opt-In Infinite TTL**: Require explicit configuration for no-expiration data
3. **Warning Period**: Emit warnings before enforcing TTL on existing namespaces
4. **Documentation**: Comprehensive guides and migration playbooks
5. **Client Support**: SDK helpers for common TTL patterns

## Alternatives Considered

### Alternative 1: Manual Cleanup Scripts

**Approach**: Rely on periodic batch jobs to clean up old data.

**Rejected because**:
- Reactive rather than proactive
- Requires custom logic per backend
- Risk of deleting wrong data
- Operational burden

### Alternative 2: Infinite TTL by Default

**Approach**: Default to no expiration, require clients to opt-in to TTL.

**Rejected because**:
- Clients often forget to set TTL
- Storage grows unbounded
- Contradicts "safe by default" principle

### Alternative 3: Separate TTL Service

**Approach**: Build a standalone service to manage TTL across backends.

**Rejected because**:
- Additional operational complexity
- Backends already support TTL natively
- Adds latency to data operations

## Related ADRs

- ADR-010: Redis Integration (Cache pattern TTL)
- ADR-015: PostgreSQL Integration (KeyValue pattern TTL)
- ADR-020: ClickHouse Integration (TimeSeries TTL)
- ADR-032: Object Storage Pattern (Blob storage TTL) *(Pending)*

## References

- [Redis EXPIRE command](https://redis.io/commands/expire/)
- [PostgreSQL DELETE with TTL](https://wiki.postgresql.org/wiki/Deleting_expired_rows)
- [ClickHouse TTL](https://clickhouse.com/docs/en/engines/table-engines/mergetree-family/mergetree#table_engine-mergetree-ttl)
- [MinIO Lifecycle Management](https://min.io/docs/minio/linux/administration/object-management/lifecycle-management.html)
- [AWS DynamoDB TTL](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/TTL.html)

## Appendix: Default TTL Reference Table

| Use Case                    | Pattern      | Backend      | Suggested TTL  |
|-----------------------------|--------------|--------------|----------------|
| HTTP session storage        | Cache        | Redis        | 24 hours       |
| API response cache          | Cache        | Redis        | 5-15 minutes   |
| User preferences            | KeyValue     | PostgreSQL   | Infinite       |
| Temporary upload tokens     | Cache        | Redis        | 1 hour         |
| Observability metrics       | TimeSeries   | ClickHouse   | 90 days        |
| Application logs            | TimeSeries   | ClickHouse   | 30 days        |
| ML feature vectors          | Vector       | Redis VSS    | 90 days        |
| User profile cache          | Cache        | Redis        | 1 hour         |
| Rate limiting counters      | Cache        | Redis        | 1 minute       |
| Uploaded file artifacts     | Object Store | MinIO        | 90 days        |
| Build artifacts (CI/CD)     | Object Store | MinIO        | 30 days        |
| Database connection pool    | N/A          | Internal     | 10 minutes     |

---

**Status**: Accepted
**Next Steps**:
1. Update namespace configuration schema with TTL fields
2. Implement TTL defaults in proxy for each backend
3. Add TTL metrics to monitoring dashboards
4. Document TTL best practices in client SDK guides

```