---
title: "ADR-053: Claim Check TTL and Garbage Collection"
status: Proposed
date: 2025-10-14
deciders: Core Team
tags: [claim-check, ttl, lifecycle, garbage-collection, operations, cost, s3, minio]
id: adr-053
project_id: prism-data-layer
doc_uuid: 492c4b32-6f67-4676-aa6f-21a8a9aac13c
---

# ADR-053: Claim Check TTL and Garbage Collection

## Status
**Proposed** - Pending review

## Context

The claim check pattern (RFC-033) stores large payloads in object storage. Without proper cleanup, storage costs grow unboundedly as claims accumulate. We need a strategy for:

1. **Automatic Expiration**: Remove claims after consumer retrieval
2. **Orphan Cleanup**: Delete claims from failed/crashed consumers
3. **Cost Control**: Prevent storage bloat from forgotten claims
4. **Audit Trail**: Track claim lifecycle for debugging
5. **Configurable TTL**: Different namespaces have different retention needs

### Problem Statement

**Scenario 1: Happy Path**
```text
Producer → Upload claim → Consumer retrieves → Claim should be deleted
          (claim valid)                       (immediate cleanup)
```

**Scenario 2: Consumer Crash**
```text
Producer → Upload claim → Consumer crashes → Claim orphaned
          (claim valid)   (never retrieved)  (needs TTL cleanup)
```

**Scenario 3: Slow Consumer**
```text
Producer → Upload claim → Long processing → Consumer retrieves → Claim deleted
          (claim valid)   (still valid)                         (delayed cleanup)
```

**Scenario 4: Replay/Redelivery**
```text
Producer → Upload claim → Consumer retrieves → Message redelivered → Claim missing
          (claim valid)   (claim deleted)                            (ERROR!)
```

### Requirements

1. **No Orphans**: All claims must eventually expire
2. **Safe TTL**: TTL must account for max consumer processing time
3. **Immediate Cleanup Option**: Delete claim after successful retrieval
4. **Idempotent Retrieval**: Multiple retrievals should work (for redelivery scenarios)
5. **Cost Effective**: Minimize storage costs without breaking functionality

## Decision

**Use a two-phase TTL strategy: short consumer-driven cleanup + long safety net.**

### Strategy Overview

```text
┌─────────────────────────────────────────────────────────────┐
│  Claim Lifecycle                                             │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Producer Upload                                             │
│  ├─ Set bucket lifecycle policy (safety net: 24h)          │
│  └─ Store claim reference in message                        │
│                                                              │
│  Consumer Retrieval                                          │
│  ├─ Download claim payload                                  │
│  ├─ Verify checksum                                         │
│  ├─ Process message                                         │
│  └─ [Optional] Delete claim immediately                     │
│                                                              │
│  Background Cleanup (if not deleted by consumer)            │
│  └─ Bucket lifecycle policy expires claim after 24h        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Configuration Model

```yaml
# Namespace-level claim check configuration
namespace: video-processing

claim_check:
  enabled: true
  threshold: 1048576  # 1MB

  # TTL Strategy
  ttl:
    # Safety net: hard expiration via bucket lifecycle
    max_age: 86400  # 24 hours

    # Consumer behavior: delete after successful retrieval
    delete_after_read: true  # Default: true

    # Redelivery protection: keep claim briefly after first read
    retention_after_read: 300  # 5 minutes (for message redelivery)

    # Grace period before lifecycle policy kicks in
    # Allows for slow consumers, retries, debugging
    grace_period: 3600  # 1 hour minimum before eligible for cleanup
```

### Implementation

#### 1. Producer: Set Bucket Lifecycle at Startup

```go
func (p *Producer) Start(ctx context.Context) error {
    // ... existing startup logic

    if p.claimCheck != nil {
        // Ensure bucket exists with lifecycle policy
        if err := p.ensureBucketLifecycle(ctx); err != nil {
            return fmt.Errorf("failed to configure claim check bucket: %w", err)
        }
    }

    return nil
}

func (p *Producer) ensureBucketLifecycle(ctx context.Context) error {
    bucket := p.claimCheck.Bucket

    // Create bucket if needed
    if err := p.objectStore.CreateBucket(ctx, bucket); err != nil {
        return err
    }

    // Set lifecycle policy (idempotent)
    policy := &LifecyclePolicy{
        Rules: []LifecycleRule{
            {
                ID:     "expire-claims",
                Status: "Enabled",
                Filter: &LifecycleFilter{
                    Prefix: p.namespace + "/", // Only claims from this namespace
                },
                Expiration: &LifecycleExpiration{
                    Days: p.claimCheck.TTL.MaxAge / 86400, // Convert seconds to days
                },
            },
        },
    }

    return p.objectStore.SetBucketLifecycle(ctx, bucket, policy)
}
```

#### 2. Producer: Upload Claim with Metadata

```go
func (p *Producer) uploadClaim(ctx context.Context, payload []byte) (*ClaimCheckMessage, error) {
    claimID := generateClaimID()
    objectKey := fmt.Sprintf("%s/%s/%s", p.namespace, topic, claimID)

    // Compress if configured
    data := payload
    if p.claimCheck.Compression != "none" {
        data = compress(payload, p.claimCheck.Compression)
    }

    // Upload with metadata
    metadata := map[string]string{
        "prism-namespace":     p.namespace,
        "prism-created-at":    time.Now().Format(time.RFC3339),
        "prism-original-size": strconv.FormatInt(int64(len(payload)), 10),
        "prism-compression":   p.claimCheck.Compression,
        "prism-checksum":      hex.EncodeToString(sha256.Sum256(payload)),
    }

    if err := p.objectStore.PutWithMetadata(ctx, p.claimCheck.Bucket, objectKey, data, metadata); err != nil {
        return nil, fmt.Errorf("claim upload failed: %w", err)
    }

    return &ClaimCheckMessage{
        ClaimID:      claimID,
        Bucket:       p.claimCheck.Bucket,
        ObjectKey:    objectKey,
        OriginalSize: int64(len(payload)),
        Compression:  p.claimCheck.Compression,
        Checksum:     sha256.Sum256(payload),
        CreatedAt:    time.Now().Unix(),
        ExpiresAt:    time.Now().Add(time.Duration(p.claimCheck.TTL.MaxAge) * time.Second).Unix(),
    }, nil
}
```

#### 3. Consumer: Retrieve and Conditionally Delete

```go
func (c *Consumer) retrieveClaim(ctx context.Context, claim *ClaimCheckMessage) ([]byte, error) {
    // Check if claim has expired
    if time.Now().Unix() > claim.ExpiresAt {
        return nil, fmt.Errorf("claim expired: %s", claim.ClaimID)
    }

    // Download claim
    data, err := c.objectStore.Get(ctx, claim.Bucket, claim.ObjectKey)
    if err != nil {
        if errors.Is(err, ErrObjectNotFound) {
            return nil, fmt.Errorf("claim not found (may have expired): %s", claim.ClaimID)
        }
        return nil, fmt.Errorf("claim retrieval failed: %w", err)
    }

    // Verify checksum
    actualChecksum := sha256.Sum256(data)
    if !bytes.Equal(actualChecksum[:], claim.Checksum[:]) {
        return nil, fmt.Errorf("claim checksum mismatch: %s", claim.ClaimID)
    }

    // Decompress if needed
    if claim.Compression != "none" {
        data, err = decompress(data, claim.Compression)
        if err != nil {
            return nil, fmt.Errorf("claim decompression failed: %w", err)
        }
    }

    // Delete claim based on configuration
    if c.claimCheck.TTL.DeleteAfterRead {
        // Option 1: Immediate deletion (default)
        go c.deleteClaim(context.Background(), claim)

    } else if c.claimCheck.TTL.RetentionAfterRead > 0 {
        // Option 2: Delayed deletion (for redelivery protection)
        go c.scheduleClaimDeletion(context.Background(), claim,
            time.Duration(c.claimCheck.TTL.RetentionAfterRead)*time.Second)
    }

    // Otherwise: Let bucket lifecycle policy handle cleanup

    return data, nil
}

func (c *Consumer) deleteClaim(ctx context.Context, claim *ClaimCheckMessage) {
    if err := c.objectStore.Delete(ctx, claim.Bucket, claim.ObjectKey); err != nil {
        // Log but don't fail - lifecycle policy will clean up eventually
        slog.Warn("failed to delete claim after read",
            "claim_id", claim.ClaimID,
            "error", err)

        // Emit metric for monitoring
        c.metrics.ClaimDeleteFailures.Inc()
    } else {
        slog.Debug("claim deleted after retrieval", "claim_id", claim.ClaimID)
        c.metrics.ClaimsDeleted.Inc()
    }
}

func (c *Consumer) scheduleClaimDeletion(ctx context.Context, claim *ClaimCheckMessage, delay time.Duration) {
    time.Sleep(delay)
    c.deleteClaim(ctx, claim)
}
```

#### 4. Proxy Validation: TTL Compatibility Check

```go
func (p *Proxy) validateClaimCheckTTL(producerTTL, consumerTTL ClaimCheckTTL) error {
    // Both must use same max age
    if producerTTL.MaxAge != consumerTTL.MaxAge {
        return fmt.Errorf("producer/consumer max_age mismatch: %d != %d",
            producerTTL.MaxAge, consumerTTL.MaxAge)
    }

    // If consumer keeps claims after read, ensure TTL is longer than retention
    if consumerTTL.RetentionAfterRead > 0 {
        if consumerTTL.RetentionAfterRead > producerTTL.MaxAge {
            return fmt.Errorf("retention_after_read (%d) exceeds max_age (%d)",
                consumerTTL.RetentionAfterRead, producerTTL.MaxAge)
        }
    }

    // Warn if delete_after_read differs (not fatal, but inconsistent)
    if producerTTL.DeleteAfterRead != consumerTTL.DeleteAfterRead {
        slog.Warn("producer/consumer delete_after_read mismatch",
            "producer", producerTTL.DeleteAfterRead,
            "consumer", consumerTTL.DeleteAfterRead)
    }

    return nil
}
```

### TTL Configuration Examples

#### Example 1: Aggressive Cleanup (Minimize Storage Cost)
```yaml
claim_check:
  ttl:
    max_age: 3600            # 1 hour safety net
    delete_after_read: true  # Immediate deletion
    retention_after_read: 0  # No redelivery protection
    grace_period: 300        # 5 min before eligible for lifecycle
```

**Use Case**: High-throughput, reliable consumers, no message redelivery.

#### Example 2: Conservative (Handle Slow Consumers)
```yaml
claim_check:
  ttl:
    max_age: 86400           # 24 hour safety net
    delete_after_read: false # Let lifecycle policy handle cleanup
    retention_after_read: 0  # N/A (not deleting after read)
    grace_period: 7200       # 2 hours for slow processing
```

**Use Case**: Long-running ML processing, batch jobs, debugging.

#### Example 3: Redelivery Protection (Handle Message Broker Retries)
```yaml
claim_check:
  ttl:
    max_age: 7200            # 2 hour safety net
    delete_after_read: true  # Delete to save costs
    retention_after_read: 600 # Keep 10 min for retries
    grace_period: 600        # 10 min before eligible
```

**Use Case**: NATS/Kafka with message redelivery on failure.

## Consequences

### Positive

1. **No Orphaned Claims**: Bucket lifecycle policy ensures eventual cleanup
2. **Cost Optimization**: Immediate deletion reduces storage costs
3. **Flexible**: Configuration adapts to different use cases
4. **Redelivery Safe**: Retention window protects against message redelivery edge cases
5. **Fail-Safe**: If consumer deletion fails, lifecycle policy backstops
6. **Namespace Isolation**: Each namespace controls its own TTL policy
7. **Debugging Friendly**: Long TTLs enable post-mortem investigation

### Negative

1. **Complexity**: Multiple cleanup mechanisms (consumer + lifecycle)
2. **Redelivery Edge Case**: If claim deleted and message redelivered, consumer fails
3. **Storage Cost**: Retention windows increase storage usage
4. **Clock Skew**: TTL relies on accurate system clocks
5. **Lifecycle Granularity**: S3 lifecycle runs once per day (not immediate)

### Neutral

1. **Configuration Surface**: More TTL knobs = more tuning required
2. **Monitoring Need**: Must track claim creation/deletion rates
3. **Testing Complexity**: TTL tests require time simulation

## Lifecycle Policy Details

### S3/MinIO Lifecycle Behavior

**Lifecycle Rules**:
```xml
<LifecycleConfiguration>
  <Rule>
    <ID>expire-claims</ID>
    <Status>Enabled</Status>
    <Filter>
      <Prefix>video-processing/</Prefix>
    </Filter>
    <Expiration>
      <Days>1</Days>
    </Expiration>
  </Rule>
</LifecycleConfiguration>
```

**Timing**:
- S3 processes lifecycle rules once per day (typically midnight UTC)
- Objects become eligible for deletion after `Days` have passed
- Deletion is not immediate - may take up to 48 hours
- MinIO processes rules hourly (more responsive)

**Limitations**:
- Cannot set expiration &lt;1 day on S3 (MinIO supports minutes)
- Lifecycle applies to entire bucket or prefix
- Cannot set per-object expiration (workaround: use object tags)

### Workaround for Fine-Grained TTL

If sub-day TTL needed:

```go
// Use object tagging for fine-grained expiration
func (p *Producer) uploadClaimWithTag(ctx context.Context, payload []byte) error {
    // Upload with expiration tag
    tags := map[string]string{
        "expires-at": strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10),
    }

    err := p.objectStore.PutWithTags(ctx, bucket, key, data, tags)
    // ...

    // Separate cleanup service reads tags and deletes expired claims
    // (More complex, but enables hour/minute-level TTL)
}
```

## Monitoring and Alerting

### Metrics to Track

```go
// Claim lifecycle metrics
type ClaimCheckMetrics struct {
    // Producer metrics
    ClaimsCreated         prometheus.Counter
    ClaimUploadDuration   prometheus.Histogram
    ClaimUploadBytes      prometheus.Histogram

    // Consumer metrics
    ClaimsRetrieved       prometheus.Counter
    ClaimsDeleted         prometheus.Counter
    ClaimDeleteFailures   prometheus.Counter
    ClaimRetrievalDuration prometheus.Histogram
    ClaimNotFoundErrors   prometheus.Counter  // Expired or missing

    // Lifecycle metrics
    ClaimsExpired         prometheus.Counter  // From lifecycle policy
    OrphanedClaims        prometheus.Gauge    // Claims > max_age not deleted
}
```

### Alerts

```yaml
# Alert if claims not being deleted (storage leak)
- alert: ClaimCheckStorageLeak
  expr: rate(claim_check_claims_created[5m]) > rate(claim_check_claims_deleted[5m]) * 1.5
  for: 30m
  labels:
    severity: warning
  annotations:
    summary: "Claim check storage leak detected"
    description: "Claims being created faster than deleted for {{ $labels.namespace }}"

# Alert if many claims not found (TTL too short)
- alert: ClaimCheckTTLTooShort
  expr: rate(claim_check_claim_not_found_errors[5m]) > 0.01
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Claim check TTL may be too short"
    description: "Consumers encountering expired claims in {{ $labels.namespace }}"

# Alert if claim delete failures
- alert: ClaimCheckDeleteFailures
  expr: rate(claim_check_claim_delete_failures[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Claim check delete failures"
    description: "Consumer failing to delete claims in {{ $labels.namespace }}"
```

### Dashboard Panels

```promql
# Claim creation rate
rate(claim_check_claims_created[5m])

# Claim deletion rate
rate(claim_check_claims_deleted[5m])

# Outstanding claims (created - deleted)
sum(claim_check_claims_created) - sum(claim_check_claims_deleted)

# Average claim lifetime (creation to deletion)
histogram_quantile(0.50, claim_check_claim_lifetime_seconds_bucket)

# Storage usage (estimated)
sum(claim_check_claim_upload_bytes) * (1 - rate(claim_check_claims_deleted[5m]) / rate(claim_check_claims_created[5m]))
```

## Testing Strategy

### Unit Tests (Time Simulation)

```go
func TestClaimExpiration(t *testing.T) {
    // Use mock clock
    clock := clockwork.NewFakeClock()

    producer := NewProducerWithClock(config, clock)
    claim, err := producer.uploadClaim(ctx, largePayload)
    require.NoError(t, err)

    // Advance clock past expiration
    clock.Advance(25 * time.Hour)

    // Consumer should fail to retrieve
    _, err = consumer.retrieveClaim(ctx, claim)
    assert.ErrorIs(t, err, ErrClaimExpired)
}

func TestDeleteAfterRead(t *testing.T) {
    config := Config{
        ClaimCheck: &ClaimCheckConfig{
            TTL: ClaimCheckTTL{
                DeleteAfterRead: true,
            },
        },
    }

    consumer := NewConsumer(config)
    payload, err := consumer.retrieveClaim(ctx, claim)
    require.NoError(t, err)

    // Wait for async deletion
    time.Sleep(100 * time.Millisecond)

    // Claim should be gone
    exists, err := objectStore.Exists(ctx, claim.Bucket, claim.ObjectKey)
    require.NoError(t, err)
    assert.False(t, exists)
}
```

### Integration Tests (MinIO Lifecycle)

```go
func TestLifecycleCleanup(t *testing.T) {
    // Start MinIO with lifecycle enabled
    driver, cleanup := setupMinIOWithLifecycle(t)
    defer cleanup()

    // Upload claim
    claim, err := producer.uploadClaim(ctx, largePayload)
    require.NoError(t, err)

    // Verify claim exists
    exists, _ := driver.Exists(ctx, claim.Bucket, claim.ObjectKey)
    assert.True(t, exists)

    // Fast-forward lifecycle (MinIO test mode can run lifecycle on-demand)
    driver.TriggerLifecycle(ctx, claim.Bucket)

    // Claim should be deleted after lifecycle runs
    exists, _ = driver.Exists(ctx, claim.Bucket, claim.ObjectKey)
    assert.False(t, exists)
}
```

## Alternatives Considered

### 1. No Automatic Cleanup
Require manual claim deletion by consumers.

**Rejected**: Error-prone, orphans accumulate, unbounded storage cost.

### 2. Separate Cleanup Service
Background service scans object store and deletes expired claims.

**Rejected**: Adds operational complexity, lifecycle policies are simpler.

### 3. Database-Tracked TTL
Store claim metadata in database with TTL, delete objects based on DB.

**Rejected**: Adds database dependency, lifecycle policies are native to object stores.

### 4. Always Delete Immediately
No retention window, delete claim as soon as consumer retrieves.

**Rejected**: Breaks message redelivery scenarios (NATS retries, Kafka rebalancing).

### 5. Never Delete Explicitly
Rely entirely on lifecycle policies for cleanup.

**Rejected**: Storage costs higher, lifecycle granularity insufficient (daily runs).

## Open Questions

1. **Cross-Namespace Claims**: Can producer in namespace A store claim for consumer in namespace B?
   - **Answer**: No - namespace isolation enforced by bucket prefix
2. **Multipart Cleanup**: How are abandoned multipart uploads cleaned?
   - **Answer**: Separate lifecycle rule for incomplete multipart uploads
3. **Claim Reuse**: Should we support updating/extending claim TTL?
   - **Answer**: No - simplicity over flexibility
4. **Storage Class**: Should old claims move to cheaper storage (Glacier)?
   - **Answer**: Deferred - typically deleted before archival makes sense

## References

- [S3 Lifecycle Configuration](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lifecycle-mgmt.html)
- [MinIO Lifecycle Management](https://min.io/docs/minio/linux/administration/object-management/object-lifecycle-management.html)
- [GCS Object Lifecycle](https://cloud.google.com/storage/docs/lifecycle)
- [Azure Blob Lifecycle](https://learn.microsoft.com/en-us/azure/storage/blobs/lifecycle-management-overview)

## Related Documents

- RFC-033: Claim Check Pattern for Large Payloads
- ADR-051: MinIO for Claim Check Testing
- ADR-052: Object Store Interface Design
