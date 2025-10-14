---
title: "ADR-052: Object Store Interface Design"
status: Proposed
date: 2025-10-14
deciders: Core Team
tags: [architecture, interfaces, object-storage, s3, abstraction, plugin]
id: adr-052
project_id: prism-data-layer
doc_uuid: 9efef801-ccc1-4e28-a5f3-3c3c13b1994b
---

# ADR-052: Object Store Interface Design

## Status
**Proposed** - Pending review

## Context

The claim check pattern (RFC-033) requires storing large payloads in object storage (S3, MinIO, GCS, Azure Blob). We need a unified interface that:

1. **Abstracts Backend Differences**: S3, GCS, Azure Blob have similar but different APIs
2. **Supports Claim Check Operations**: Put, Get, Delete, TTL management
3. **Enables Testing**: Works with MinIO for local testing and real backends in production
4. **Maintains Performance**: Efficient for both small metadata and large payloads
5. **Follows Plugin Architecture**: Consistent with existing driver patterns

### Design Constraints

- Must support S3-compatible backends (MinIO, DigitalOcean Spaces, Wasabi)
- Must support native cloud APIs (GCS, Azure Blob)
- Should handle objects from 1KB to 5GB+
- Must support object metadata (content-type, checksums, custom headers)
- Must support TTL/expiration via lifecycle policies
- Should enable streaming for large objects
- Must be testable without real cloud accounts

## Decision

**Define a minimal `ObjectStoreInterface` focused on claim check use cases.**

### Core Interface

```go
// pkg/plugin/interfaces.go

// ObjectStoreInterface defines object storage operations for claim check pattern
type ObjectStoreInterface interface {
    // Put stores an object at the given key
    // Returns error if bucket doesn't exist or operation fails
    Put(ctx context.Context, bucket, key string, data []byte) error

    // PutStream stores an object from a reader (for large payloads)
    // Caller is responsible for closing the reader
    PutStream(ctx context.Context, bucket, key string, reader io.Reader, size int64) error

    // Get retrieves an object
    // Returns error if object doesn't exist
    Get(ctx context.Context, bucket, key string) ([]byte, error)

    // GetStream retrieves an object as a stream (for large payloads)
    // Caller must close the returned reader
    GetStream(ctx context.Context, bucket, key string) (io.ReadCloser, error)

    // Delete removes an object
    // Returns nil if object doesn't exist (idempotent)
    Delete(ctx context.Context, bucket, key string) error

    // Exists checks if an object exists without downloading
    Exists(ctx context.Context, bucket, key string) (bool, error)

    // GetMetadata retrieves object metadata without downloading content
    GetMetadata(ctx context.Context, bucket, key string) (*ObjectMetadata, error)

    // SetTTL sets object expiration (seconds from now)
    // Not all backends support per-object TTL - may use bucket lifecycle policies
    SetTTL(ctx context.Context, bucket, key string, ttlSeconds int) error

    // CreateBucket creates a bucket if it doesn't exist (idempotent)
    CreateBucket(ctx context.Context, bucket string) error

    // DeleteBucket deletes a bucket and all its contents
    // Returns error if bucket doesn't exist or isn't empty (unless force=true)
    DeleteBucket(ctx context.Context, bucket string) error

    // BucketExists checks if a bucket exists
    BucketExists(ctx context.Context, bucket string) (bool, error)
}

// ObjectMetadata contains object metadata without the content
type ObjectMetadata struct {
    // Size in bytes
    Size int64

    // Content type (MIME)
    ContentType string

    // Last modification time
    LastModified time.Time

    // ETag (typically MD5 hash)
    ETag string

    // Content encoding (e.g., "gzip")
    ContentEncoding string

    // Custom metadata (headers starting with x-amz-meta-, x-goog-meta-, etc.)
    UserMetadata map[string]string

    // Expiration time (if set via TTL)
    ExpiresAt *time.Time
}
```

### Design Principles

#### 1. Minimal Surface Area
Only operations needed for claim check - no listing, versioning, ACLs, etc.

#### 2. Bucket-Scoped
All operations require explicit bucket parameter - no default bucket magic.

#### 3. Streaming Support
Large payload operations use `io.Reader`/`io.ReadCloser` to avoid loading entire object into memory.

#### 4. Idempotent Deletes
`Delete()` returns nil if object doesn't exist - simplifies cleanup logic.

#### 5. Metadata Separation
`GetMetadata()` allows checking object properties without downloading (useful for size/checksum validation).

#### 6. TTL Abstraction
`SetTTL()` abstracts per-object expiration vs bucket lifecycle policies.

### Implementation Strategy

```go
// pkg/drivers/minio/driver.go (example)
type MinioDriver struct {
    client *minio.Client
    config MinioConfig

    // Lifecycle policy cache (avoid repeated bucket policy queries)
    lifecycleMu sync.RWMutex
    lifecycles  map[string]*LifecyclePolicy
}

func (d *MinioDriver) Put(ctx context.Context, bucket, key string, data []byte) error {
    _, err := d.client.PutObject(ctx, bucket, key,
        bytes.NewReader(data), int64(len(data)),
        minio.PutObjectOptions{
            ContentType: "application/octet-stream",
        })
    return err
}

func (d *MinioDriver) PutStream(ctx context.Context, bucket, key string, reader io.Reader, size int64) error {
    _, err := d.client.PutObject(ctx, bucket, key, reader, size,
        minio.PutObjectOptions{
            ContentType: "application/octet-stream",
        })
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

func (d *MinioDriver) GetStream(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
    obj, err := d.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
    if err != nil {
        return nil, err
    }

    // Caller must close
    return obj, nil
}

func (d *MinioDriver) Delete(ctx context.Context, bucket, key string) error {
    err := d.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})

    // MinIO returns error if object doesn't exist - make it idempotent
    if minio.ToErrorResponse(err).Code == "NoSuchKey" {
        return nil
    }

    return err
}

func (d *MinioDriver) Exists(ctx context.Context, bucket, key string) (bool, error) {
    _, err := d.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
    if err != nil {
        if minio.ToErrorResponse(err).Code == "NoSuchKey" {
            return false, nil
        }
        return false, err
    }
    return true, nil
}

func (d *MinioDriver) GetMetadata(ctx context.Context, bucket, key string) (*ObjectMetadata, error) {
    stat, err := d.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
    if err != nil {
        return nil, err
    }

    return &ObjectMetadata{
        Size:            stat.Size,
        ContentType:     stat.ContentType,
        LastModified:    stat.LastModified,
        ETag:            stat.ETag,
        ContentEncoding: stat.Metadata.Get("Content-Encoding"),
        UserMetadata:    extractUserMetadata(stat.Metadata),
    }, nil
}

func (d *MinioDriver) SetTTL(ctx context.Context, bucket, key string, ttlSeconds int) error {
    // MinIO doesn't support per-object TTL - use bucket lifecycle policies
    // This is a common limitation of S3-compatible stores

    d.lifecycleMu.Lock()
    defer d.lifecycleMu.Unlock()

    // Check if bucket already has lifecycle policy for this TTL
    policy, exists := d.lifecycles[bucket]
    if exists && policy.HasRule(ttlSeconds) {
        return nil // Already configured
    }

    // Create or update lifecycle policy
    config := lifecycle.NewConfiguration()
    config.Rules = []lifecycle.Rule{
        {
            ID:         fmt.Sprintf("expire-after-%d", ttlSeconds),
            Status:     "Enabled",
            Expiration: lifecycle.Expiration{Days: ttlSeconds / 86400},
        },
    }

    err := d.client.SetBucketLifecycle(ctx, bucket, config)
    if err != nil {
        return err
    }

    // Cache policy
    d.lifecycles[bucket] = &LifecyclePolicy{Rules: config.Rules}

    return nil
}

func (d *MinioDriver) CreateBucket(ctx context.Context, bucket string) error {
    exists, err := d.client.BucketExists(ctx, bucket)
    if err != nil {
        return err
    }
    if exists {
        return nil // Idempotent
    }

    return d.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{
        Region: d.config.Region,
    })
}

func (d *MinioDriver) DeleteBucket(ctx context.Context, bucket string) error {
    // Remove all objects first
    objectsCh := d.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
        Recursive: true,
    })

    for object := range objectsCh {
        if object.Err != nil {
            return object.Err
        }
        if err := d.client.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
            return err
        }
    }

    // Remove bucket
    err := d.client.RemoveBucket(ctx, bucket)
    if err != nil && minio.ToErrorResponse(err).Code == "NoSuchBucket" {
        return nil // Idempotent
    }

    return err
}

func (d *MinioDriver) BucketExists(ctx context.Context, bucket string) (bool, error) {
    return d.client.BucketExists(ctx, bucket)
}
```

## Consequences

### Positive

1. **Backend Portability**: Same interface works with S3, GCS, Azure Blob, MinIO
2. **Testable**: Easy to mock or use in-memory implementation for unit tests
3. **Streaming Support**: Efficient for multi-GB payloads
4. **Minimal Dependencies**: Small interface surface = fewer breaking changes
5. **Consistent**: Follows existing plugin interface patterns
6. **Metadata Access**: Can validate size/checksum before downloading

### Negative

1. **Limited Scope**: Doesn't support advanced features (versioning, multipart, ACLs)
2. **TTL Abstraction**: Per-object TTL on backends that only support bucket policies requires workarounds
3. **Error Semantics**: Different backends return different error types - need careful handling
4. **No Pagination**: List operations not included (not needed for claim check)

### Neutral

1. **No Multipart Upload**: Would add complexity - revisit if needed for &gt;5GB payloads
2. **No Presigned URLs**: Could enable direct client-to-S3 transfers - future enhancement
3. **No Server-Side Encryption**: Handled by backend configuration, not interface

## Implementation Phases

### Phase 1: Interface Definition (1 day)
- Define `ObjectStoreInterface` in `pkg/plugin/interfaces.go`
- Define `ObjectMetadata` type
- Add `PatternObjectStore` constant to framework

### Phase 2: MinIO Driver (3 days)
```text
pkg/drivers/minio/
├── driver.go          # Main implementation
├── config.go          # Configuration parsing
├── lifecycle.go       # TTL handling via lifecycle policies
├── errors.go          # Error type conversion
└── driver_test.go     # Unit tests
```

### Phase 3: S3 Driver (3 days)
```text
pkg/drivers/s3/
├── driver.go          # AWS SDK v2 implementation
├── config.go          # IAM, regions, encryption
├── lifecycle.go       # Native lifecycle API
└── driver_test.go     # Unit tests (requires AWS credentials)
```

### Phase 4: Mock Implementation (1 day)
```text
pkg/drivers/mock/
└── objectstore.go     # In-memory implementation for unit tests
```

## Testing Strategy

### Unit Tests (No External Dependencies)
```go
func TestObjectStoreInterface(t *testing.T) {
    // Use in-memory mock
    store := mock.NewObjectStore()

    // Test basic operations
    err := store.CreateBucket(ctx, "test")
    require.NoError(t, err)

    err = store.Put(ctx, "test", "key1", []byte("data"))
    require.NoError(t, err)

    data, err := store.Get(ctx, "test", "key1")
    require.NoError(t, err)
    assert.Equal(t, []byte("data"), data)
}
```

### Integration Tests (MinIO via testcontainers)
```go
func TestMinIODriver(t *testing.T) {
    driver, cleanup := setupMinIO(t)
    defer cleanup()

    // Run interface compliance tests
    runObjectStoreTests(t, driver)
}

func runObjectStoreTests(t *testing.T, store ObjectStoreInterface) {
    // Shared test suite for all implementations
    t.Run("Put/Get", func(t *testing.T) { ... })
    t.Run("Streaming", func(t *testing.T) { ... })
    t.Run("Metadata", func(t *testing.T) { ... })
    t.Run("TTL", func(t *testing.T) { ... })
    t.Run("Delete", func(t *testing.T) { ... })
}
```

### Contract Tests (Verify Backend Compatibility)
```go
// tests/interface-suites/objectstore/
func TestObjectStoreContract(t *testing.T) {
    backends := []struct {
        name  string
        setup func(t *testing.T) ObjectStoreInterface
    }{
        {"MinIO", setupMinIOBackend},
        {"S3", setupS3Backend},      // Requires AWS creds
        {"GCS", setupGCSBackend},    // Requires GCP creds
        {"Mock", setupMockBackend},
    }

    for _, backend := range backends {
        t.Run(backend.name, func(t *testing.T) {
            store := backend.setup(t)
            runObjectStoreTests(t, store)
        })
    }
}
```

## Error Handling

### Error Types
```go
// pkg/plugin/errors.go

var (
    // ErrObjectNotFound indicates object doesn't exist
    ErrObjectNotFound = errors.New("object not found")

    // ErrBucketNotFound indicates bucket doesn't exist
    ErrBucketNotFound = errors.New("bucket not found")

    // ErrBucketAlreadyExists indicates bucket creation conflict
    ErrBucketAlreadyExists = errors.New("bucket already exists")

    // ErrAccessDenied indicates permission error
    ErrAccessDenied = errors.New("access denied")

    // ErrQuotaExceeded indicates storage quota exceeded
    ErrQuotaExceeded = errors.New("quota exceeded")
)
```

### Error Translation
Each driver must translate backend-specific errors to standard errors:

```go
func (d *MinioDriver) translateError(err error) error {
    if err == nil {
        return nil
    }

    resp := minio.ToErrorResponse(err)
    switch resp.Code {
    case "NoSuchKey":
        return ErrObjectNotFound
    case "NoSuchBucket":
        return ErrBucketNotFound
    case "BucketAlreadyOwnedByYou", "BucketAlreadyExists":
        return ErrBucketAlreadyExists
    case "AccessDenied":
        return ErrAccessDenied
    default:
        return err // Wrap unknown errors
    }
}
```

## Security Considerations

### 1. Access Control
Interface doesn't include ACL operations - manage via backend configuration:
```yaml
minio:
  access_key: ${MINIO_ACCESS_KEY}
  secret_key: ${MINIO_SECRET_KEY}

s3:
  iam_role: arn:aws:iam::123456789:role/prism-s3-access
```

### 2. Encryption
Backend-specific encryption handled via driver configuration:
```yaml
s3:
  server_side_encryption: AES256
  kms_key_id: arn:aws:kms:us-west-2:123456789:key/abc
```

### 3. Network Security
TLS configuration per backend:
```yaml
minio:
  use_ssl: true
  ca_cert: /path/to/ca.pem
```

### 4. Audit Logging
All operations logged via driver observability hooks:
```go
func (d *MinioDriver) Put(ctx context.Context, bucket, key string, data []byte) error {
    start := time.Now()
    defer func() {
        slog.Info("object store put",
            "backend", "minio",
            "bucket", bucket,
            "key", key,
            "size", len(data),
            "duration", time.Since(start))
    }()

    // ... implementation
}
```

## Performance Considerations

### 1. Connection Pooling
```go
type MinioDriver struct {
    client *minio.Client  // Internally connection-pooled

    // Connection pool tuning
    maxIdleConns     int
    maxConnsPerHost  int
    idleConnTimeout  time.Duration
}
```

### 2. Retry Strategy
```go
type Config struct {
    MaxRetries      int           `json:"max_retries"`
    RetryBackoff    time.Duration `json:"retry_backoff"`
    Timeout         time.Duration `json:"timeout"`
}
```

### 3. Streaming Thresholds
```go
const (
    // Use PutStream for payloads > 10MB
    StreamingThreshold = 10 * 1024 * 1024
)

func (p *Producer) uploadClaim(ctx context.Context, payload []byte) error {
    if len(payload) > StreamingThreshold {
        return p.objectStore.PutStream(ctx, bucket, key,
            bytes.NewReader(payload), int64(len(payload)))
    }
    return p.objectStore.Put(ctx, bucket, key, payload)
}
```

### 4. Metadata Caching
```go
// Cache frequently accessed metadata
type MetadataCache struct {
    cache map[string]*ObjectMetadata
    ttl   time.Duration
    mu    sync.RWMutex
}
```

## Alternatives Considered

### 1. Blob-Specific Interfaces
Define separate interfaces per backend (S3Interface, GCSInterface, AzureInterface).

**Rejected**: Defeats portability, increases complexity, harder to test.

### 2. Full S3 API Coverage
Implement complete S3 API (versioning, ACLs, multipart, CORS, etc.).

**Rejected**: Over-engineering for claim check use case, massive scope.

### 3. Generic Key-Value Interface
Treat object storage as key-value store (like KeyValueBasicInterface).

**Rejected**: Misses object-specific concepts (metadata, streaming, buckets).

### 4. Pre-signed URL Generation
Add `GetPresignedURL()` for direct client uploads.

**Deferred**: Useful feature but not needed for MVP. Add in future RFC.

## Open Questions

1. **Multipart Upload**: Do we need multipart upload for &gt;5GB payloads? (Deferred)
2. **Copy Operation**: Should we support server-side copy? (Deferred)
3. **Range Reads**: Do we need partial object reads? (Deferred)
4. **Checksums**: Should we calculate and store checksums automatically? (Yes, in producer)
5. **Compression**: Should object store handle compression or claim check layer? (Claim check layer)

## References

- [AWS S3 API Reference](https://docs.aws.amazon.com/AmazonS3/latest/API/Welcome.html)
- [MinIO Go SDK](https://min.io/docs/minio/linux/developers/go/API.html)
- [GCS Client Library](https://cloud.google.com/storage/docs/reference/libraries)
- [Azure Blob Storage SDK](https://learn.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-go)
- [Enterprise Integration Patterns: Claim Check](https://www.enterpriseintegrationpatterns.com/patterns/messaging/StoreInLibrary.html)

## Related Documents

- RFC-033: Claim Check Pattern for Large Payloads
- ADR-051: MinIO for Claim Check Testing
- ADR-053: Claim Check TTL and Garbage Collection (to be created)
- RFC-008: Proxy-Plugin Architecture
