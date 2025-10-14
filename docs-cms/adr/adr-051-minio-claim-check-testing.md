---
title: "ADR-051: MinIO for Claim Check Pattern Testing"
status: Proposed
date: 2025-10-14
deciders: Core Team
tags: [testing, minio, s3, object-storage, claim-check, testcontainers, local-testing]
id: adr-051
project_id: prism-data-layer
doc_uuid: 6619d625-317a-4b8d-a80a-e225219eaf92
---

# ADR-051: MinIO for Claim Check Pattern Testing

## Status
**Proposed** - Pending review

## Context

The claim check pattern (RFC-033) requires an object storage backend for storing large payloads. For acceptance testing, we need a local object storage solution that:

1. **S3-Compatible**: Uses standard S3 API for portability
2. **Lightweight**: Runs in testcontainers without heavy infrastructure
3. **Fast Startup**: Quick container initialization for rapid test iteration
4. **Feature-Complete**: Supports TTL, metadata, checksums
5. **Production-Like**: Behaves like real S3/GCS for realistic testing

### Evaluation Criteria

| Backend | S3 API | Container | Startup | TTL | Cost | Prod-Like |
|---------|--------|-----------|---------|-----|------|-----------|
| **MinIO** | ✅ Full | ✅ 50MB | ~2s | ✅ Lifecycle | Free | ⭐⭐⭐⭐⭐ |
| LocalStack | ✅ Full | ❌ 800MB | ~10s | ✅ | Free | ⭐⭐⭐⭐ |
| Azurite | ❌ Azure | ✅ 100MB | ~3s | ⚠️ Limited | Free | ⭐⭐⭐ |
| S3Mock | ⚠️ Basic | ✅ 80MB | ~4s | ❌ | Free | ⭐⭐ |
| SeaweedFS | ⚠️ Partial | ✅ 40MB | ~2s | ⚠️ Limited | Free | ⭐⭐⭐ |
| Real S3 | ✅ Full | N/A | N/A | ✅ | $$ | ⭐⭐⭐⭐⭐ |

## Decision

**Use MinIO for claim check pattern acceptance testing.**

MinIO provides:
- **Full S3 compatibility**: Drop-in replacement for production S3/GCS
- **Small footprint**: 50MB Docker image, 2-second startup
- **Complete feature set**: Lifecycle policies (TTL), versioning, encryption
- **Open source**: No licensing concerns, active development
- **Production use**: Many companies use MinIO in production

### MinIO Configuration for Testing

```yaml
# testcontainers configuration
services:
  minio:
    image: minio/minio:latest
    ports:
      - "9000:9000"  # API
      - "9001:9001"  # Console
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    command: server /data --console-address ":9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 5s
      timeout: 3s
      retries: 5
```

### Test Setup Pattern

```go
// tests/acceptance/backends/minio.go
func setupMinIO(t *testing.T, ctx context.Context) (interface{}, func()) {
    t.Helper()

    // Start MinIO testcontainer
    minioContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "minio/minio:RELEASE.2024-10-13T13-34-11Z",  // Pin version
            ExposedPorts: []string{"9000/tcp", "9001/tcp"},
            Env: map[string]string{
                "MINIO_ROOT_USER":     "minioadmin",
                "MINIO_ROOT_PASSWORD": "minioadmin",
            },
            Cmd: []string{"server", "/data", "--console-address", ":9001"},
            WaitingFor: wait.ForHTTP("/minio/health/live").
                WithPort("9000/tcp").
                WithStartupTimeout(30 * time.Second),
        },
        Started: true,
    })
    require.NoError(t, err, "Failed to start MinIO container")

    // Get endpoint
    endpoint, err := minioContainer.Endpoint(ctx, "")
    require.NoError(t, err, "Failed to get MinIO endpoint")

    // Create driver
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
            "use_ssl":    false,  // No TLS in tests
            "region":     "us-east-1",
        },
    }

    err = driver.Initialize(ctx, config)
    require.NoError(t, err, "Failed to initialize MinIO driver")

    err = driver.Start(ctx)
    require.NoError(t, err, "Failed to start MinIO driver")

    // Create test bucket
    err = driver.CreateBucket(ctx, "test-claims")
    require.NoError(t, err, "Failed to create test bucket")

    cleanup := func() {
        driver.Stop(ctx)
        if err := minioContainer.Terminate(ctx); err != nil {
            t.Logf("Failed to terminate MinIO container: %v", err)
        }
    }

    return driver, cleanup
}
```

### Test Bucket Strategy

Each test suite gets isolated buckets:

```go
// Pattern: {suite}-{backend}-{timestamp}
// Example: claimcheck-nats-1697234567

func createTestBucket(t *testing.T, driver ObjectStoreInterface) string {
    bucketName := fmt.Sprintf("%s-%s-%d",
        t.Name(),
        driver.Name(),
        time.Now().Unix())

    // Sanitize bucket name (S3 rules: lowercase, no underscores)
    bucketName = strings.ToLower(bucketName)
    bucketName = strings.ReplaceAll(bucketName, "_", "-")
    bucketName = strings.ReplaceAll(bucketName, "/", "-")

    err := driver.CreateBucket(ctx, bucketName)
    require.NoError(t, err)

    // Set lifecycle policy for automatic cleanup
    err = driver.SetBucketLifecycle(ctx, bucketName, &LifecyclePolicy{
        Rules: []LifecycleRule{
            {
                ID:         "expire-after-1-hour",
                Expiration: 3600, // 1 hour
                Status:     "Enabled",
            },
        },
    })
    require.NoError(t, err)

    t.Cleanup(func() {
        // Best effort cleanup - lifecycle will handle stragglers
        if err := driver.DeleteBucket(ctx, bucketName); err != nil {
            t.Logf("Failed to delete test bucket: %v", err)
        }
    })

    return bucketName
}
```

## Consequences

### Positive

1. **Fast Tests**: 2-second container startup keeps test suite fast
2. **Reliable**: MinIO widely used, well-tested, stable
3. **S3-Compatible**: Tests transfer directly to production S3/GCS/Azure
4. **No Mocks**: Real object storage behavior, catches edge cases
5. **TTL Support**: Test claim expiration and lifecycle policies
6. **Local Development**: Developers can run full test suite locally
7. **CI/CD Friendly**: Lightweight enough for GitHub Actions

### Negative

1. **Not Real S3**: Subtle behavioral differences may exist
2. **Container Overhead**: Adds ~2s to test startup time
3. **Resource Usage**: Each test needs MinIO container (managed by testcontainers)
4. **Version Pinning**: Must pin MinIO version for reproducible tests

### Neutral

1. **Additional Dependency**: Another driver to maintain
2. **S3 API Learning**: Team must understand S3 concepts (buckets, keys, lifecycle)
3. **Docker Required**: Tests require Docker/Podman (already required)

## Implementation Plan

### Phase 1: MinIO Driver (Week 1)
```text
pkg/drivers/minio/
├── driver.go          # ObjectStoreInterface implementation
├── config.go          # MinIO-specific configuration
├── client.go          # S3 client wrapper
├── lifecycle.go       # TTL and lifecycle policies
└── driver_test.go     # Unit tests
```

### Phase 2: Test Framework Integration (Week 1)
```text
tests/acceptance/backends/
└── minio.go           # Backend registration

tests/acceptance/framework/
└── types.go           # Add PatternObjectStore constant
```

### Phase 3: Claim Check Tests (Week 2)
```text
tests/acceptance/patterns/claimcheck/
├── claimcheck_test.go      # Multi-pattern tests
├── large_payload_test.go   # 5MB+ payloads
├── threshold_test.go       # Boundary conditions
├── compression_test.go     # Gzip/zstd compression
└── ttl_test.go            # Expiration behavior
```

## Testing the Tests

Validate MinIO setup with smoke tests:

```go
func TestMinIOSetup(t *testing.T) {
    ctx := context.Background()

    driver, cleanup := setupMinIO(t, ctx)
    defer cleanup()

    // Test basic operations
    bucket := "smoke-test"
    err := driver.CreateBucket(ctx, bucket)
    require.NoError(t, err)

    // Put object
    data := []byte("hello world")
    err = driver.Put(ctx, bucket, "test-key", data)
    require.NoError(t, err)

    // Get object
    retrieved, err := driver.Get(ctx, bucket, "test-key")
    require.NoError(t, err)
    assert.Equal(t, data, retrieved)

    // Delete object
    err = driver.Delete(ctx, bucket, "test-key")
    require.NoError(t, err)

    // Verify deletion
    exists, err := driver.Exists(ctx, bucket, "test-key")
    require.NoError(t, err)
    assert.False(t, exists)
}
```

## Alternatives Considered

### 1. LocalStack (S3 Emulator)
**Pros**: Full AWS service suite (S3, SQS, SNS, etc.)
**Cons**: 800MB image, 10s+ startup, overkill for S3-only needs
**Verdict**: Too heavy for our use case

### 2. Azurite (Azure Blob Emulator)
**Pros**: Official Microsoft emulator, lightweight
**Cons**: Azure Blob API != S3 API, different semantics
**Verdict**: Would require Azure-specific driver

### 3. S3Mock (Java-based)
**Pros**: Lightweight, Docker-friendly
**Cons**: Limited S3 API coverage, no lifecycle policies
**Verdict**: Missing critical features

### 4. Real AWS S3
**Pros**: 100% production behavior
**Cons**: Costs money, requires AWS credentials, slower, internet dependency
**Verdict**: Use for integration tests, not unit tests

### 5. In-Memory Fake
**Pros**: Fastest possible, no dependencies
**Cons**: No S3 API compatibility, won't catch integration issues
**Verdict**: Use for unit tests, not acceptance tests

## Monitoring and Debugging

### Container Logs
```bash
# View MinIO logs during test failures
docker logs <container-id>

# Or via testcontainers
t.Logf("MinIO logs: %s", minioContainer.Logs(ctx))
```

### MinIO Console
Access web UI for debugging:
```bash
# Get console URL
echo "http://$(docker port <container-id> 9001)"

# Login: minioadmin / minioadmin
# View buckets, objects, lifecycle policies
```

### Performance Metrics
```go
// Track MinIO operation latency
func (d *MinioDriver) Put(ctx context.Context, bucket, key string, data []byte) error {
    start := time.Now()
    defer func() {
        d.metrics.PutLatency.Observe(time.Since(start).Seconds())
    }()

    _, err := d.client.PutObject(ctx, bucket, key, ...)
    return err
}
```

## Migration to Production

When moving from MinIO tests to production S3:

1. **Configuration Change**: Update backend config from `minio` to `s3`
2. **Credentials**: Use IAM roles instead of access keys
3. **Region**: Specify correct AWS region
4. **Bucket Names**: Use production bucket naming convention
5. **Lifecycle Policies**: Match test TTLs to production requirements
6. **Encryption**: Enable S3-SSE or KMS encryption

```yaml
# Test (MinIO)
object_store:
  backend: minio
  endpoint: localhost:9000
  access_key: minioadmin
  secret_key: minioadmin
  use_ssl: false

# Production (S3)
object_store:
  backend: s3
  region: us-west-2
  # Credentials from IAM role
  use_ssl: true
  server_side_encryption: AES256
```

## Open Questions

1. **Multi-Region Testing**: Should we test S3 cross-region behavior? (MinIO doesn't support this)
2. **Large File Performance**: What's the largest payload we should test? (10GB?)
3. **Concurrent Access**: Should we test concurrent claim check operations?
4. **MinIO Version Policy**: Pin exact version or use `latest`?

## References

- [MinIO Documentation](https://min.io/docs/minio/linux/index.html)
- [MinIO Docker Hub](https://hub.docker.com/r/minio/minio)
- [testcontainers-go](https://golang.testcontainers.org/)
- [AWS S3 API Reference](https://docs.aws.amazon.com/AmazonS3/latest/API/Welcome.html)
- [S3 Lifecycle Configuration](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lifecycle-mgmt.html)

## Related Documents

- RFC-033: Claim Check Pattern for Large Payloads
- ADR-052: Object Store Interface Design
- ADR-004: Local-First Testing Strategy
- ADR-049: Podman Container Optimization
