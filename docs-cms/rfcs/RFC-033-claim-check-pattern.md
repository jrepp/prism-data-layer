---
title: "RFC-033: Claim Check Pattern for Large Payloads"
status: Proposed
author: Prism Team
created: 2025-10-14
updated: 2025-10-14
tags: [patterns, claim-check, object-storage, performance, architecture, producer, consumer, minio, s3]
id: rfc-033
project_id: prism-data-layer
doc_uuid: 7660cafd-abcf-44f3-82d8-e612a7cf6630
---

# RFC-033: Claim Check Pattern for Large Payloads

## Status
**Proposed** - Design phase, awaiting review

## Context

Messaging systems typically have message size limits (NATS: 1MB default, Kafka: 1MB default, Redis: 512MB max). Sending large payloads (videos, images, ML models, datasets) through message queues creates several problems:

1. **Performance Degradation**: Large messages slow down message brokers and increase network congestion
2. **Memory Pressure**: Brokers must buffer large messages, causing memory issues
3. **Increased Latency**: Large message serialization/deserialization adds latency
4. **Size Limits**: Hard limits prevent sending certain payloads
5. **Cost**: Cloud message brokers charge per GB transferred

The **Claim Check Pattern** solves this by:
- Storing large payloads in object storage (S3, MinIO)
- Sending only a reference (claim check) through the message queue
- Consumer retrieves payload from object storage using the claim check

This is a standard enterprise integration pattern (EIP) used by Azure Service Bus, AWS EventBridge, and Google Pub/Sub.

## Proposal

Add optional **Claim Check slot** to Producer and Consumer patterns, coordinating through namespace-level requirements.

### Architecture

```text
┌─────────────┐
│  Producer   │
│             │
│ 1. Check    │──────┐
│    payload  │      │
│    size     │      │
│             │      ▼
│ 2. Upload   │  ┌──────────────┐
│    if > X   │─▶│ Object Store │
│             │  │  (MinIO/S3)  │
│ 3. Send     │  └──────────────┘
│    claim    │      │
│    check    │      │
└─────────────┘      │
       │             │
       │ Message     │
       │ (small)     │
       ▼             │
┌─────────────┐      │
│   Message   │      │
│   Broker    │      │
│  (NATS/     │      │
│   Kafka)    │      │
└─────────────┘      │
       │             │
       │             │
       ▼             │
┌─────────────┐      │
│  Consumer   │      │
│             │      │
│ 1. Receive  │      │
│    message  │      │
│             │      │
│ 2. Check    │      │
│    claim    │      │
│             │      │
│ 3. Download │◀─────┘
│    if claim │
│    exists   │
│             │
│ 4. Process  │
│    payload  │
└─────────────┘
```

### Namespace-Level Coordination

Producers and consumers in the same namespace share claim check requirements:

```yaml
namespace: video-processing
claim_check:
  enabled: true
  threshold: 1048576  # 1MB - messages larger trigger claim check
  backend: minio
  bucket: prism-claims-video-processing
  ttl: 3600  # Claim expires after 1 hour
  compression: gzip  # Optional compression before upload
```

The proxy validates that:
1. Producer and consumer claim check configurations match
2. Both have access to the same object store backend
3. Bucket exists and is accessible
4. TTL policies are compatible

### Producer Behavior

```go
// Producer configuration with claim check slot
type Config struct {
    Name        string
    Behavior    BehaviorConfig
    Slots       SlotConfig
    ClaimCheck  *ClaimCheckConfig  // NEW: Optional claim check
}

type ClaimCheckConfig struct {
    Enabled       bool
    Threshold     int64   // Bytes - payloads > threshold use claim check
    Backend       string  // "minio", "s3", "gcs", etc.
    Bucket        string
    TTL           int     // Seconds - how long claim is valid
    Compression   string  // "none", "gzip", "zstd"
}

// Producer slots with optional object store
type SlotConfig struct {
    MessageSink string   // Required: NATS, Kafka, Redis
    StateStore  string   // Optional: for deduplication
    ObjectStore string   // Optional: for claim check
}
```

**Publish Flow**:

```go
func (p *Producer) Publish(ctx context.Context, topic string, payload []byte, metadata map[string]string) error {
    // Check if payload exceeds threshold
    if p.claimCheck != nil && int64(len(payload)) > p.claimCheck.Threshold {
        // 1. Compress if configured
        data := payload
        if p.claimCheck.Compression != "none" {
            data = compress(payload, p.claimCheck.Compression)
        }

        // 2. Upload to object store
        claimID := generateClaimID()
        objectKey := fmt.Sprintf("%s/%s/%s", p.namespace, topic, claimID)

        if err := p.objectStore.Put(ctx, p.claimCheck.Bucket, objectKey, data); err != nil {
            return fmt.Errorf("claim check upload failed: %w", err)
        }

        // 3. Set TTL for automatic cleanup
        if p.claimCheck.TTL > 0 {
            if err := p.objectStore.SetTTL(ctx, p.claimCheck.Bucket, objectKey, p.claimCheck.TTL); err != nil {
                // Non-fatal: log warning and continue
                slog.Warn("failed to set claim check TTL", "error", err)
            }
        }

        // 4. Send small message with claim reference
        claimPayload := ClaimCheckMessage{
            ClaimID:     claimID,
            Bucket:      p.claimCheck.Bucket,
            ObjectKey:   objectKey,
            OriginalSize: len(payload),
            Compression: p.claimCheck.Compression,
            ContentType: metadata["content-type"],
            Checksum:    sha256.Sum256(payload),
        }

        smallPayload, _ := json.Marshal(claimPayload)
        metadata["prism-claim-check"] = "true"

        return p.messageSink.Publish(ctx, topic, smallPayload, metadata)
    }

    // Normal path: small payload sent directly
    return p.messageSink.Publish(ctx, topic, payload, metadata)
}
```

### Consumer Behavior

```go
// Consumer configuration with claim check slot
type Config struct {
    Name        string
    Behavior    BehaviorConfig
    Slots       SlotConfig
    ClaimCheck  *ClaimCheckConfig  // NEW: Optional claim check
}

// Consumer slots with optional object store
type SlotConfig struct {
    MessageSource string  // Required: NATS, Kafka, Redis
    StateStore    string  // Optional: for offset tracking
    ObjectStore   string  // Optional: for claim check
}
```

**Consumption Flow**:

```go
func (c *Consumer) processMessage(ctx context.Context, msg *plugin.PubSubMessage) error {
    // Check if message is a claim check
    if msg.Metadata["prism-claim-check"] == "true" {
        // 1. Deserialize claim check
        var claim ClaimCheckMessage
        if err := json.Unmarshal(msg.Payload, &claim); err != nil {
            return fmt.Errorf("invalid claim check message: %w", err)
        }

        // 2. Download from object store
        data, err := c.objectStore.Get(ctx, claim.Bucket, claim.ObjectKey)
        if err != nil {
            return fmt.Errorf("claim check download failed: %w", err)
        }

        // 3. Verify checksum
        actualChecksum := sha256.Sum256(data)
        if !bytes.Equal(actualChecksum[:], claim.Checksum[:]) {
            return fmt.Errorf("claim check checksum mismatch")
        }

        // 4. Decompress if needed
        if claim.Compression != "none" {
            data, err = decompress(data, claim.Compression)
            if err != nil {
                return fmt.Errorf("claim check decompression failed: %w", err)
            }
        }

        // 5. Replace payload with actual data
        msg.Payload = data
        msg.Metadata["content-type"] = claim.ContentType
        delete(msg.Metadata, "prism-claim-check")

        // 6. Optional: Delete claim after successful retrieval
        if c.claimCheck.DeleteAfterRead {
            go func() {
                if err := c.objectStore.Delete(ctx, claim.Bucket, claim.ObjectKey); err != nil {
                    slog.Warn("failed to delete claim after read", "error", err)
                }
            }()
        }
    }

    // Process message (with original or retrieved payload)
    return c.processor(ctx, msg)
}
```

### Message Format

**Claim Check Message**:

```protobuf
message ClaimCheckMessage {
  // Unique claim identifier
  string claim_id = 1;

  // Object store location
  string bucket = 2;
  string object_key = 3;

  // Metadata about original payload
  int64 original_size = 4;
  string content_type = 5;
  bytes checksum = 6;  // SHA-256 of uncompressed payload

  // Compression info
  string compression = 7;  // "none", "gzip", "zstd"

  // Expiration
  int64 expires_at = 8;  // Unix timestamp

  // Optional: multipart for very large files
  optional MultipartInfo multipart = 9;
}

message MultipartInfo {
  int32 part_count = 1;
  repeated string part_keys = 2;
}
```

### Proxy Validation

The proxy validates claim check coordination at pattern registration:

```go
// When producer registers
func (p *Proxy) RegisterProducer(ctx context.Context, req *RegisterRequest) error {
    // Load namespace configuration
    ns := p.getNamespace(req.Namespace)

    // If namespace requires claim check, validate producer config
    if ns.ClaimCheck.Required {
        if req.Config.ClaimCheck == nil {
            return status.Error(codes.FailedPrecondition,
                "namespace requires claim check but producer does not support it")
        }

        // Validate configuration matches namespace requirements
        if err := validateClaimCheckConfig(req.Config.ClaimCheck, ns.ClaimCheck); err != nil {
            return status.Errorf(codes.InvalidArgument,
                "claim check config mismatch: %v", err)
        }

        // Verify object store backend is accessible
        if err := p.verifyObjectStoreAccess(ctx, req.Config.ClaimCheck); err != nil {
            return status.Errorf(codes.FailedPrecondition,
                "object store not accessible: %v", err)
        }
    }

    return nil
}

// Similar validation for consumer registration
```

### Object Store Interface

```go
// ObjectStoreInterface defines operations needed for claim check
type ObjectStoreInterface interface {
    // Put stores an object
    Put(ctx context.Context, bucket, key string, data []byte) error

    // Get retrieves an object
    Get(ctx context.Context, bucket, key string) ([]byte, error)

    // Delete removes an object
    Delete(ctx context.Context, bucket, key string) error

    // SetTTL sets object expiration
    SetTTL(ctx context.Context, bucket, key string, ttlSeconds int) error

    // Exists checks if object exists
    Exists(ctx context.Context, bucket, key string) (bool, error)

    // GetMetadata retrieves object metadata without downloading
    GetMetadata(ctx context.Context, bucket, key string) (*ObjectMetadata, error)
}

type ObjectMetadata struct {
    Size         int64
    ContentType  string
    LastModified time.Time
    ETag         string
}
```

### MinIO Driver for Testing

For acceptance testing, we'll use MinIO (S3-compatible) via testcontainers:

```go
// pkg/drivers/minio/driver.go
type MinioDriver struct {
    client *minio.Client
    config MinioConfig
}

func (d *MinioDriver) Put(ctx context.Context, bucket, key string, data []byte) error {
    _, err := d.client.PutObject(ctx, bucket, key,
        bytes.NewReader(data), int64(len(data)),
        minio.PutObjectOptions{})
    return err
}

func (d *MinioDriver) Get(ctx context.Context, bucket, key string) ([]byte, error) {
    obj, err := d.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
    if err != nil {
        return nil, err
    }
    defer obj.Close()
    return io.ReadAll(obj)
}

// ... other methods
```

**Acceptance Test Setup**:

```go
// tests/acceptance/backends/minio.go
func init() {
    framework.MustRegisterBackend(framework.Backend{
        Name:      "MinIO",
        SetupFunc: setupMinIO,
        SupportedPatterns: []framework.Pattern{
            framework.PatternObjectStore,  // New pattern
        },
        Capabilities: framework.Capabilities{
            SupportsObjectStore: true,
            MaxObjectSize:       5 * 1024 * 1024 * 1024, // 5GB
        },
    })
}

func setupMinIO(t *testing.T, ctx context.Context) (interface{}, func()) {
    // Start MinIO testcontainer
    minioContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "minio/minio:latest",
            ExposedPorts: []string{"9000/tcp"},
            Env: map[string]string{
                "MINIO_ROOT_USER":     "minioadmin",
                "MINIO_ROOT_PASSWORD": "minioadmin",
            },
            Cmd: []string{"server", "/data"},
            WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp"),
        },
        Started: true,
    })
    require.NoError(t, err)

    // Get connection details
    endpoint, err := minioContainer.Endpoint(ctx, "")
    require.NoError(t, err)

    // Create MinIO driver
    driver := minio.New()
    config := &plugin.Config{
        Plugin: plugin.PluginConfig{
            Name:    "minio-test",
            Version: "0.1.0",
        },
        Backend: map[string]any{
            "endpoint":   endpoint,
            "access_key": "minioadmin",
            "secret_key": "minioadmin",
            "use_ssl":    false,
        },
    }

    err = driver.Initialize(ctx, config)
    require.NoError(t, err)

    err = driver.Start(ctx)
    require.NoError(t, err)

    cleanup := func() {
        driver.Stop(ctx)
        minioContainer.Terminate(ctx)
    }

    return driver, cleanup
}
```

### Acceptance Test Scenarios

```go
// tests/acceptance/patterns/claimcheck/claimcheck_test.go
func TestClaimCheckPattern(t *testing.T) {
    tests := []framework.MultiPatternTest{
        {
            Name: "LargePayloadClaimCheck",
            RequiredPatterns: map[string]framework.Pattern{
                "producer":    framework.PatternProducer,
                "consumer":    framework.PatternConsumer,
                "objectstore": framework.PatternObjectStore,
            },
            Func:    testLargePayloadClaimCheck,
            Timeout: 60 * time.Second,
            Tags:    []string{"claim-check", "large-payload"},
        },
        {
            Name: "ThresholdBoundary",
            RequiredPatterns: map[string]framework.Pattern{
                "producer":    framework.PatternProducer,
                "consumer":    framework.PatternConsumer,
                "objectstore": framework.PatternObjectStore,
            },
            Func:    testThresholdBoundary,
            Timeout: 30 * time.Second,
            Tags:    []string{"claim-check", "boundary"},
        },
        {
            Name: "Compression",
            RequiredPatterns: map[string]framework.Pattern{
                "producer":    framework.PatternProducer,
                "consumer":    framework.PatternConsumer,
                "objectstore": framework.PatternObjectStore,
            },
            Func:    testCompression,
            Timeout: 30 * time.Second,
            Tags:    []string{"claim-check", "compression"},
        },
        {
            Name: "TTLExpiration",
            RequiredPatterns: map[string]framework.Pattern{
                "producer":    framework.PatternProducer,
                "consumer":    framework.PatternConsumer,
                "objectstore": framework.PatternObjectStore,
            },
            Func:    testTTLExpiration,
            Timeout: 45 * time.Second,
            Tags:    []string{"claim-check", "ttl"},
        },
        {
            Name: "ChecksumValidation",
            RequiredPatterns: map[string]framework.Pattern{
                "producer":    framework.PatternProducer,
                "consumer":    framework.PatternConsumer,
                "objectstore": framework.PatternObjectStore,
            },
            Func:    testChecksumValidation,
            Timeout: 30 * time.Second,
            Tags:    []string{"claim-check", "security"},
        },
    }

    framework.RunMultiPatternTests(t, tests)
}

func testLargePayloadClaimCheck(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
    ctx := context.Background()

    // Setup producer with claim check
    prod := setupProducerWithClaimCheck(t, ctx, drivers["producer"], drivers["objectstore"])
    cons := setupConsumerWithClaimCheck(t, ctx, drivers["consumer"], drivers["objectstore"])

    // Generate 5MB payload (exceeds 1MB threshold)
    largePayload := make([]byte, 5*1024*1024)
    rand.Read(largePayload)

    // Publish
    err := prod.Publish(ctx, "test-topic", largePayload, map[string]string{
        "content-type": "application/octet-stream",
    })
    require.NoError(t, err)

    // Consumer should receive full payload via claim check
    received := <-cons.Messages()
    assert.Equal(t, len(largePayload), len(received.Payload))
    assert.Equal(t, largePayload, received.Payload)
}

func testThresholdBoundary(t *testing.T, drivers map[string]interface{}, caps framework.Capabilities) {
    ctx := context.Background()

    prod := setupProducerWithClaimCheck(t, ctx, drivers["producer"], drivers["objectstore"])
    cons := setupConsumerWithClaimCheck(t, ctx, drivers["consumer"], drivers["objectstore"])

    // Payload just under threshold (should NOT use claim check)
    smallPayload := make([]byte, 1048575) // 1MB - 1 byte
    err := prod.Publish(ctx, "test-topic", smallPayload, nil)
    require.NoError(t, err)

    msg1 := <-cons.Messages()
    assert.Empty(t, msg1.Metadata["prism-claim-check"])

    // Payload exactly at threshold (should use claim check)
    thresholdPayload := make([]byte, 1048576) // 1MB
    err = prod.Publish(ctx, "test-topic", thresholdPayload, nil)
    require.NoError(t, err)

    msg2 := <-cons.Messages()
    assert.Equal(t, thresholdPayload, msg2.Payload)
}
```

## Benefits

1. **Handles Large Payloads**: No message broker size limits
2. **Reduces Broker Load**: Small messages flow through queue
3. **Better Performance**: Less serialization/deserialization overhead
4. **Cost Optimization**: Object storage cheaper than message transfer
5. **Automatic Cleanup**: TTL-based claim expiration prevents storage bloat
6. **Transparent**: Application code unchanged, pattern handles complexity
7. **Namespace Coordination**: Proxy validates producer/consumer compatibility

## Trade-offs

### Advantages
- Decouples message flow from payload storage
- Scales to multi-GB payloads
- Reduces network congestion
- Automatic garbage collection via TTL

### Disadvantages
- Increased latency (additional network hop to object store)
- More infrastructure dependencies (object store required)
- Potential consistency issues if claim expires before consumption
- Additional operational complexity

## Migration Path

1. **Phase 1**: Implement object store interface and MinIO driver
2. **Phase 2**: Add claim check support to producer pattern
3. **Phase 3**: Add claim check support to consumer pattern
4. **Phase 4**: Add namespace validation in proxy
5. **Phase 5**: Create acceptance tests with MinIO
6. **Phase 6**: Document pattern and create examples

## Alternatives Considered

### 1. Inline Chunking
Break large messages into multiple small messages.

**Rejected**: Adds complexity to consumer (reassembly), doesn't reduce broker load proportionally.

### 2. Separate Large Message Queue
Use different queue for large messages.

**Rejected**: Requires dual-queue management, ordering issues, more complex routing.

### 3. Always Use Object Store
Store all payloads in object store, regardless of size.

**Rejected**: Unnecessary overhead for small messages, increased latency.

## Open Questions

1. **Multipart Upload**: Should we support multipart uploads for very large payloads (&gt;5GB)?
2. **Encryption**: Should claims be encrypted in object store? (Separate RFC)
3. **Bandwidth Throttling**: Should we rate-limit object store operations?
4. **Cross-Region**: How do claims work in multi-region deployments?
5. **Claim ID Collision**: Use UUID v4 or content-addressed (hash-based)?

## References

- [Enterprise Integration Patterns: Claim Check](https://www.enterpriseintegrationpatterns.com/patterns/messaging/StoreInLibrary.html)
- [Azure Service Bus: Send large messages using claim check pattern](https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-premium-messaging)
- [AWS EventBridge: Claim check pattern](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-pipes-event-filtering.html)
- [MinIO Documentation](https://min.io/docs/minio/linux/index.html)
- [S3 API Reference](https://docs.aws.amazon.com/AmazonS3/latest/API/Welcome.html)

## Related Documents

- ADR-051: MinIO for Claim Check Testing (to be created)
- ADR-052: Object Store Interface Design (to be created)
- ADR-053: Claim Check TTL and Garbage Collection (to be created)
- RFC-031: Message Envelope Protocol (encryption)
- RFC-008: Proxy-Plugin Architecture
