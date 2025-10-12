---
date: 2025-10-05
deciders: Core Team
doc_uuid: fefcdbcd-6c7e-4e3c-8ef0-4652fb513085
id: adr-009
project_id: prism-data-layer
status: Accepted
tags:
- operations
- reliability
- backend
title: 'ADR-009: Shadow Traffic for Migrations'
---

## Context

Database migrations are risky and common:
- Upgrade Postgres 14 → 16
- Move from Cassandra 2 → 3 (Netflix did this for 250 clusters)
- Migrate data from Postgres → Kafka for event sourcing
- Change data model (add indexes, change schema)

Traditional migration approaches:
1. **Stop-the-world**: Take outage, migrate, restart
   - ❌ Downtime unacceptable for critical services
2. **Blue-green deployment**: Run both, switch traffic
   - ❌ Data synchronization issues, expensive
3. **Gradual rollout**: Migrate % of traffic
   - ✅ Better, but still risk of inconsistency

**Problem**: How do we migrate data and backends with zero downtime and high confidence?

## Decision

Use **shadow traffic** pattern: Duplicate writes to old and new backends, compare results, promote new backend when confident.

## Rationale

### Shadow Traffic Pattern

Client Request
      │
      ▼
   Prism Proxy
      │
      ├──► Primary Backend (old) ──► Response to client
      │
      └──► Shadow Backend (new)  ──► Log comparison
```text

**Phases**:

1. **Shadow Write**: Write to both, read from primary
2. **Backfill**: Copy existing data to new backend
3. **Shadow Read**: Read from both, compare, serve from primary
4. **Promote**: Switch primary to new backend
5. **Decommission**: Remove old backend

### Detailed Migration Flow

**Phase 1: Setup Shadow (Week 1)**

```
namespace: user-profiles

backends:
  primary:
    type: postgres-old
    connection: postgres://old-cluster/prism

  shadow:
    type: postgres-new
    connection: postgres://new-cluster/prism
    mode: shadow_write  # Write only, don't read
```text

All writes go to both:
```
async fn put(&self, request: PutRequest) -> Result<PutResponse> {
    // Write to primary (blocking)
    let primary_result = self.primary_backend.put(&request).await?;

    // Write to shadow (async, don't block response)
    let shadow_request = request.clone();
    tokio::spawn(async move {
        match self.shadow_backend.put(&shadow_request).await {
            Ok(_) => {
                metrics::SHADOW_WRITES_SUCCESS.inc();
            }
            Err(e) => {
                metrics::SHADOW_WRITES_ERRORS.inc();
                tracing::warn!(error = %e, "Shadow write failed");
            }
        }
    });

    Ok(primary_result)
}
```text

**Phase 2: Backfill (Week 2-3)**

Copy existing data:
```
# Scan all data from primary
prism-cli backfill \
  --namespace user-profiles \
  --from postgres-old \
  --to postgres-new \
  --parallelism 10 \
  --throttle-rps 1000
```text

```
async fn backfill(
    from: &dyn Backend,
    to: &dyn Backend,
    namespace: &str,
) -> Result<BackfillStats> {
    let mut cursor = None;
    let mut total_copied = 0;

    loop {
        // Scan batch from source
        let batch = from.scan(namespace, cursor.as_ref(), 1000).await?;
        if batch.items.is_empty() {
            break;
        }

        // Write batch to destination
        to.put_batch(namespace, &batch.items).await?;

        total_copied += batch.items.len();
        cursor = batch.next_cursor;

        metrics::BACKFILL_ITEMS.inc_by(batch.items.len() as u64);
    }

    Ok(BackfillStats { items_copied: total_copied })
}
```text

**Phase 3: Shadow Read (Week 4)**

Read from both, compare:
```
namespace: user-profiles

backends:
  primary:
    type: postgres-old

  shadow:
    type: postgres-new
    mode: shadow_read  # Read and compare
```text

```
async fn get(&self, request: GetRequest) -> Result<GetResponse> {
    // Read from primary (blocking)
    let primary_response = self.primary_backend.get(&request).await?;

    // Read from shadow (async comparison)
    let shadow_request = request.clone();
    let primary_items = primary_response.items.clone();
    tokio::spawn(async move {
        match self.shadow_backend.get(&shadow_request).await {
            Ok(shadow_response) => {
                // Compare results
                if shadow_response.items == primary_items {
                    metrics::SHADOW_READS_MATCH.inc();
                } else {
                    metrics::SHADOW_READS_MISMATCH.inc();
                    tracing::error!(
                        "Shadow read mismatch for {}",
                        shadow_request.id
                    );
                    // Log differences for analysis
                }
            }
            Err(e) => {
                metrics::SHADOW_READS_ERRORS.inc();
                tracing::warn!(error = %e, "Shadow read failed");
            }
        }
    });

    Ok(primary_response)
}
```text

**Monitor mismatch rate**:
shadow_reads_mismatch_rate =
    shadow_reads_mismatch / (shadow_reads_match + shadow_reads_mismatch)

Target: < 0.1% (1 in 1000)
```

**Phase 4: Promote (Week 5)**

Flip primary when confident:
```yaml
namespace: user-profiles

backends:
  primary:
    type: postgres-new  # ← Changed!

  shadow:
    type: postgres-old  # Keep old as shadow for safety
    mode: shadow_write
```

Monitor for issues. If problems, flip back instantly.

**Phase 5: Decommission (Week 6+)**

After confidence period (e.g., 2 weeks):
```yaml
namespace: user-profiles

backends:
  primary:
    type: postgres-new
  # shadow removed
```

Delete old backend resources.

### Configuration Management

```rust
#[derive(Deserialize)]
pub struct NamespaceConfig {
    pub name: String,
    pub backends: BackendConfig,
}

#[derive(Deserialize)]
pub struct BackendConfig {
    pub primary: BackendSpec,
    pub shadow: Option<ShadowBackendSpec>,
}

#[derive(Deserialize)]
pub struct ShadowBackendSpec {
    #[serde(flatten)]
    pub backend: BackendSpec,

    pub mode: ShadowMode,
    pub sample_rate: f64,  // 0.0-1.0, default 1.0
}

#[derive(Deserialize)]
pub enum ShadowMode {
    ShadowWrite,  // Write to both, read from primary
    ShadowRead,   // Read from both, compare
}
```

### Alternatives Considered

1. **Stop-the-World Migration**
   - Pros: Simple, guaranteed consistent
   - Cons: Downtime unacceptable
   - Rejected: Not viable for critical services

2. **Application-Level Dual Writes**
   - Pros: Application has full control
   - Cons: Every app must implement, error-prone
   - Rejected: Platform should handle this

3. **Database Replication**
   - Pros: Database-native
   - Cons: Tied to specific databases, not all support it
   - Rejected: Doesn't work for Postgres → Kafka migration

4. **Event Sourcing + Replay**
   - Pros: Can replay events to new backend
   - Cons: Requires event log, complex
   - Rejected: Too heavy for simple migrations

## Consequences

### Positive

- **Zero Downtime**: No service interruption
- **High Confidence**: Validate new backend with prod traffic before switching
- **Rollback**: Easy to revert if issues found
- **Gradual**: Can shadow 10% of traffic first, then 100%

### Negative

- **Write Amplification**: 2x writes during shadow phase
  - *Mitigation*: Shadow writes async, don't block
- **Cost**: Running two backends simultaneously
  - *Mitigation*: Migration is temporary (weeks, not months)
- **Complexity**: More code, more config
  - *Mitigation*: Platform handles it, not app developers

### Neutral

- **Mismatch Debugging**: Need to investigate mismatches
  - Provides valuable validation

## Implementation Notes

### Metrics Dashboard

```yaml
# Grafana dashboard
panels:
  - title: "Shadow Write Success Rate"
    expr: |
      sum(rate(prism_shadow_writes_success[5m]))
      /
      sum(rate(prism_shadow_writes_total[5m]))

  - title: "Shadow Read Mismatch Rate"
    expr: |
      sum(rate(prism_shadow_reads_mismatch[5m]))
      /
      sum(rate(prism_shadow_reads_total[5m]))

  - title: "Backfill Progress"
    expr: prism_backfill_items_total
```

### Automated Promotion

```rust
pub struct MigrationOrchestrator {
    config: MigrationConfig,
}

impl MigrationOrchestrator {
    pub async fn execute(&self) -> Result<()> {
        // Phase 1: Enable shadow writes
        self.update_config(ShadowMode::ShadowWrite).await?;
        metrics::wait_for_shadow_write_success_rate(0.99, Duration::from_hours(24)).await?;

        // Phase 2: Backfill
        self.backfill().await?;

        // Phase 3: Enable shadow reads
        self.update_config(ShadowMode::ShadowRead).await?;
        metrics::wait_for_shadow_read_mismatch_rate(0.001, Duration::from_days(3)).await?;

        // Phase 4: Promote
        self.promote().await?;
        metrics::wait_for_no_errors(Duration::from_days(7)).await?;

        // Phase 5: Decommission
        self.decommission_old().await?;

        Ok(())
    }
}
```

## References

- Netflix Data Gateway: Shadow Traffic
- [GitHub: How We Ship Code Faster and Safer with Feature Flags](https://github.blog/2021-04-27-ship-code-faster-safer-feature-flags/)
- [Stripe: Online Migrations at Scale](https://stripe.com/blog/online-migrations)
- ADR-005: Backend Plugin Architecture
- ADR-006: Namespace and Multi-Tenancy

## Revision History

- 2025-10-05: Initial draft and acceptance