---
id: adr-032
title: ADR-032 Object Storage Pattern with MinIO
sidebar_label: ADR-032 Object Storage
status: Accepted
---

# ADR-032: Object Storage Pattern with MinIO

**Status**: Accepted
**Date**: 2025-10-08
**Decision Makers**: System Architecture Team
**Tags**: `object-storage`, `minio`, `s3`, `blobs`, `local-testing`

## Context

Modern applications frequently need to store and retrieve unstructured data (blobs): uploaded files, images, videos, artifacts, backups, ML models, and large payloads. Traditional databases are poorly suited for blob storage due to:

1. **Size Constraints**: Binary data bloats database storage
2. **Performance**: Large blobs slow down queries and backups
3. **Cost**: Database storage is expensive compared to object storage
4. **Access Patterns**: Blobs need streaming, range requests, CDN integration

While cloud providers offer S3, Azure Blob Storage, and GCS, **local development and testing** requires a compatible local implementation. MinIO provides S3-compatible object storage that runs locally, enabling:

- **Realistic Testing**: S3-compatible API without cloud dependencies
- **Cost-Free Development**: No cloud storage charges during development
- **Offline Development**: Work without internet connectivity
- **CI/CD Integration**: Ephemeral MinIO in test pipelines

## Decision

**We will adopt Object Storage as a first-class data access pattern in Prism, using MinIO for local development and testing, with S3-compatible APIs for production deployments.**

### Principles

1. **S3-Compatible API**: Use S3 as the de facto standard
2. **MinIO for Local**: Default to MinIO for local testing
3. **Cloud-Agnostic**: Support AWS S3, GCS, Azure Blob via adapters
4. **Streaming Support**: Handle large files efficiently
5. **Lifecycle Policies**: Automatic expiration and tiering (see ADR-031)
6. **Presigned URLs**: Secure temporary access for direct uploads/downloads

## Object Storage Pattern

### Use Cases

| Use Case                | Pattern          | Example                                    |
|-------------------------|------------------|--------------------------------------------|
| **File Uploads**        | User-generated   | Profile pictures, document attachments     |
| **Build Artifacts**     | CI/CD outputs    | Docker images, compiled binaries           |
| **ML Models**           | Model serving    | Trained models, checkpoints                |
| **Backups**             | Data archives    | Database backups, snapshots                |
| **Large Payloads**      | Offloaded data   | JSON/XML > 1MB, avoiding message queues    |
| **Media Storage**       | Video/Audio      | Streaming media, podcast episodes          |
| **Log Archives**        | Compliance       | Long-term log storage                      |
| **Static Assets**       | CDN integration  | Website images, CSS/JS bundles             |

### Data Access Pattern: Object Store

```protobuf
syntax = "proto3";

package prism.objectstore;

// Object Storage Service
service ObjectStoreService {
  // Upload object
  rpc PutObject(stream PutObjectRequest) returns (PutObjectResponse);

  // Download object
  rpc GetObject(GetObjectRequest) returns (stream GetObjectResponse);

  // Delete object
  rpc DeleteObject(DeleteObjectRequest) returns (DeleteObjectResponse);

  // List objects in bucket/prefix
  rpc ListObjects(ListObjectsRequest) returns (ListObjectsResponse);

  // Get object metadata
  rpc HeadObject(HeadObjectRequest) returns (HeadObjectResponse);

  // Generate presigned URL for direct access
  rpc GetPresignedURL(PresignedURLRequest) returns (PresignedURLResponse);

  // Copy object
  rpc CopyObject(CopyObjectRequest) returns (CopyObjectResponse);
}

// Upload request (streaming for large files)
message PutObjectRequest {
  string namespace = 1;
  string bucket = 2;
  string key = 3;

  // Metadata
  map<string, string> metadata = 4;
  string content_type = 5;

  // Lifecycle (see ADR-031)
  optional int64 ttl_seconds = 6;

  // Data chunk
  bytes chunk = 7;
}

message PutObjectResponse {
  string object_id = 1;
  string etag = 2;
  int64 size_bytes = 3;
  string version_id = 4;  // If versioning enabled
}

// Download request
message GetObjectRequest {
  string namespace = 1;
  string bucket = 2;
  string key = 3;

  // Range request (for partial downloads)
  optional int64 range_start = 4;
  optional int64 range_end = 5;
}

message GetObjectResponse {
  // Metadata
  string content_type = 1;
  int64 size_bytes = 2;
  map<string, string> metadata = 3;
  string etag = 4;

  // Data chunk
  bytes chunk = 5;
}

// Delete request
message DeleteObjectRequest {
  string namespace = 1;
  string bucket = 2;
  string key = 3;
}

message DeleteObjectResponse {
  bool deleted = 1;
}

// List request
message ListObjectsRequest {
  string namespace = 1;
  string bucket = 2;
  string prefix = 3;
  int32 max_results = 4;
  string continuation_token = 5;
}

message ListObjectsResponse {
  repeated ObjectMetadata objects = 1;
  string continuation_token = 2;
  bool is_truncated = 3;
}

message ObjectMetadata {
  string key = 1;
  int64 size_bytes = 2;
  string etag = 3;
  int64 last_modified = 4;  // Unix timestamp
  string content_type = 5;
}

// Presigned URL request
message PresignedURLRequest {
  string namespace = 1;
  string bucket = 2;
  string key = 3;
  string method = 4;  // GET, PUT, DELETE
  int64 expires_in_seconds = 5;  // Default 3600 (1 hour)
}

message PresignedURLResponse {
  string url = 1;
  int64 expires_at = 2;
}
```

## MinIO for Local Development

### Why MinIO?

- **S3-Compatible**: Drop-in replacement for AWS S3 API
- **Lightweight**: Runs in Docker with minimal resources
- **Open Source**: Apache License 2.0, free for all use
- **Feature-Rich**: Versioning, encryption, lifecycle policies
- **Easy Setup**: Single Docker command to run locally

### Local Docker Setup

```bash
# Start MinIO in Docker
docker run -d \
  -p 9000:9000 \
  -p 9001:9001 \
  --name prism-minio \
  -e "MINIO_ROOT_USER=prism" \
  -e "MINIO_ROOT_PASSWORD=prismpassword" \
  -v /tmp/minio-data:/data \
  minio/minio server /data --console-address ":9001"

# Create bucket
docker exec prism-minio mc alias set local http://localhost:9000 prism prismpassword
docker exec prism-minio mc mb local/prism-objects
```

### Configuration

```yaml
# Namespace configuration for object storage
namespaces:
  - name: user-uploads
    backend: minio
    pattern: objectstore
    config:
      endpoint: "localhost:9000"
      access_key: "prism"
      secret_key: "prismpassword"
      bucket: "prism-objects"
      use_ssl: false  # Local dev
      default_ttl_seconds: 7776000  # 90 days

  - name: build-artifacts
    backend: s3
    pattern: objectstore
    config:
      endpoint: "s3.amazonaws.com"
      region: "us-east-1"
      bucket: "prism-builds"
      use_ssl: true
      default_ttl_seconds: 2592000  # 30 days
```

## Backend Implementations

### MinIO Backend (Rust)

```rust
use aws_sdk_s3::{Client, Config, Credentials, Endpoint};
use tokio::io::AsyncReadExt;

pub struct MinIOBackend {
    client: Client,
    bucket: String,
    default_ttl: Option<Duration>,
}

impl MinIOBackend {
    pub async fn new(config: &NamespaceConfig) -> Result<Self> {
        let credentials = Credentials::new(
            &config.access_key,
            &config.secret_key,
            None,
            None,
            "prism-minio",
        );

        let endpoint = Endpoint::immutable(
            format!("http://{}", config.endpoint).parse()?,
        );

        let s3_config = Config::builder()
            .credentials_provider(credentials)
            .endpoint_resolver(endpoint)
            .region(Region::new("us-east-1"))  // MinIO doesn't care
            .build();

        let client = Client::from_conf(s3_config);

        Ok(Self {
            client,
            bucket: config.bucket.clone(),
            default_ttl: config.default_ttl,
        })
    }

    pub async fn put_object(
        &self,
        key: &str,
        data: impl AsyncRead + Send,
        content_type: Option<String>,
        ttl: Option<Duration>,
    ) -> Result<PutObjectOutput> {
        let mut builder = self
            .client
            .put_object()
            .bucket(&self.bucket)
            .key(key)
            .body(ByteStream::from(data));

        if let Some(ct) = content_type {
            builder = builder.content_type(ct);
        }

        // Apply TTL via tagging (lifecycle policy handles expiration)
        if let Some(ttl) = ttl.or(self.default_ttl) {
            let expires_at = Utc::now() + chrono::Duration::from_std(ttl)?;
            builder = builder.tagging(&format!("ttl={}", expires_at.timestamp()));
        }

        builder.send().await.map_err(Into::into)
    }

    pub async fn get_object(&self, key: &str) -> Result<GetObjectOutput> {
        self.client
            .get_object()
            .bucket(&self.bucket)
            .key(key)
            .send()
            .await
            .map_err(Into::into)
    }

    pub async fn presigned_url(
        &self,
        key: &str,
        method: &str,
        expires_in: Duration,
    ) -> Result<String> {
        let presigning_config = PresigningConfig::expires_in(expires_in)?;

        let url = match method {
            "GET" => {
                self.client
                    .get_object()
                    .bucket(&self.bucket)
                    .key(key)
                    .presigned(presigning_config)
                    .await?
            }
            "PUT" => {
                self.client
                    .put_object()
                    .bucket(&self.bucket)
                    .key(key)
                    .presigned(presigning_config)
                    .await?
            }
            _ => return Err("Unsupported method".into()),
        };

        Ok(url.uri().to_string())
    }
}
```

### S3 Backend Adapter

```rust
// Same implementation as MinIO, different endpoint/config
pub struct S3Backend {
    inner: MinIOBackend,  // Reuse MinIO implementation
}

impl S3Backend {
    pub async fn new(config: &NamespaceConfig) -> Result<Self> {
        // Override endpoint for AWS S3
        let mut s3_config = config.clone();
        s3_config.endpoint = format!("s3.{}.amazonaws.com", config.region);
        s3_config.use_ssl = true;

        Ok(Self {
            inner: MinIOBackend::new(&s3_config).await?,
        })
    }

    // Delegate all operations to MinIO backend
    pub async fn put_object(&self, /* ... */) -> Result<PutObjectOutput> {
        self.inner.put_object(/* ... */).await
    }
}
```

## Lifecycle Management (Integration with ADR-031)

### MinIO Lifecycle Policy

```json
{
  "Rules": [
    {
      "ID": "expire-after-ttl",
      "Status": "Enabled",
      "Filter": {
        "Tag": {
          "Key": "ttl",
          "Value": "*"
        }
      },
      "Expiration": {
        "Days": 1  // Check daily for expired objects
      }
    },
    {
      "ID": "default-90-day-expiration",
      "Status": "Enabled",
      "Filter": {
        "Prefix": "tmp/"
      },
      "Expiration": {
        "Days": 90
      }
    }
  ]
}
```

### Setting Lifecycle Policies via Admin CLI

```bash
# Set lifecycle policy on bucket
prism objectstore set-lifecycle user-uploads --policy lifecycle.json

# View current lifecycle policy
prism objectstore get-lifecycle user-uploads

# List objects expiring soon
prism objectstore list user-uploads --expiring-within 7d
```

## Client SDK Examples

### Python SDK

```python
from prism_sdk import PrismClient

client = PrismClient(namespace="user-uploads", pattern="objectstore")

# Upload file
with open("profile.jpg", "rb") as f:
    response = client.put_object(
        bucket="prism-objects",
        key="users/123/profile.jpg",
        data=f,
        content_type="image/jpeg",
        ttl_seconds=86400 * 90,  # 90 days
    )
    print(f"Uploaded: {response.object_id}, ETag: {response.etag}")

# Download file
with client.get_object(bucket="prism-objects", key="users/123/profile.jpg") as obj:
    with open("downloaded.jpg", "wb") as f:
        for chunk in obj.stream():
            f.write(chunk)

# Generate presigned URL (for direct browser upload)
url = client.get_presigned_url(
    bucket="prism-objects",
    key="users/123/upload.jpg",
    method="PUT",
    expires_in_seconds=3600,  # 1 hour
)
print(f"Upload to: {url}")

# List objects
objects = client.list_objects(bucket="prism-objects", prefix="users/123/")
for obj in objects:
    print(f"{obj.key}: {obj.size_bytes} bytes, {obj.last_modified}")
```

### Go SDK

```go
client := prism.NewClient("user-uploads", prism.WithPattern("objectstore"))

// Upload file
file, _ := os.Open("profile.jpg")
defer file.Close()

resp, err := client.PutObject(ctx, &prism.PutObjectRequest{
    Bucket:      "prism-objects",
    Key:         "users/123/profile.jpg",
    Data:        file,
    ContentType: "image/jpeg",
    TTLSeconds:  90 * 24 * 3600,
})

// Download file
obj, err := client.GetObject(ctx, &prism.GetObjectRequest{
    Bucket: "prism-objects",
    Key:    "users/123/profile.jpg",
})
defer obj.Body.Close()

output, _ := os.Create("downloaded.jpg")
defer output.Close()
io.Copy(output, obj.Body)
```

## Testing Strategy

### Local Testing with MinIO

```yaml
# docker-compose.test.yml
version: '3.8'
services:
  minio:
    image: minio/minio:latest
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: prism-test
      MINIO_ROOT_PASSWORD: prism-test-password
    command: server /data --console-address ":9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 5s
      timeout: 3s
      retries: 3

  prism-proxy:
    build: ./proxy
    ports:
      - "50051:50051"
    depends_on:
      minio:
        condition: service_healthy
    environment:
      PRISM_OBJECTSTORE_ENDPOINT: "minio:9000"
      PRISM_OBJECTSTORE_ACCESS_KEY: "prism-test"
      PRISM_OBJECTSTORE_SECRET_KEY: "prism-test-password"
```

### Integration Tests

```rust
#[tokio::test]
async fn test_object_storage_lifecycle() {
    let client = PrismTestClient::new("user-uploads").await;

    // Upload
    let data = b"Hello, MinIO!";
    let response = client
        .put_object("test.txt", data, Some("text/plain"), None)
        .await
        .unwrap();
    assert!(!response.etag.is_empty());

    // Download
    let downloaded = client.get_object("test.txt").await.unwrap();
    assert_eq!(downloaded.content, data);

    // List
    let objects = client.list_objects("", 10).await.unwrap();
    assert_eq!(objects.len(), 1);
    assert_eq!(objects[0].key, "test.txt");

    // Delete
    client.delete_object("test.txt").await.unwrap();

    // Verify deleted
    let result = client.get_object("test.txt").await;
    assert!(result.is_err());
}
```

## Performance Considerations

### Streaming for Large Files

```rust
// Stream upload (avoid loading entire file into memory)
pub async fn put_object_stream(
    &self,
    key: &str,
    mut stream: impl Stream<Item = Result<Bytes>> + Send + Unpin,
) -> Result<PutObjectOutput> {
    let body = ByteStream::new(SdkBody::from_body_0_4(Body::wrap_stream(stream)));

    self.client
        .put_object()
        .bucket(&self.bucket)
        .key(key)
        .body(body)
        .send()
        .await
        .map_err(Into::into)
}

// Stream download (for large files, range requests)
pub async fn get_object_range(
    &self,
    key: &str,
    start: u64,
    end: u64,
) -> Result<ByteStream> {
    let range = format!("bytes={}-{}", start, end);

    let output = self
        .client
        .get_object()
        .bucket(&self.bucket)
        .key(key)
        .range(range)
        .send()
        .await?;

    Ok(output.body)
}
```

### Performance Targets

| Operation       | Target Latency | Throughput      | Notes                          |
|-----------------|----------------|-----------------|--------------------------------|
| **Put (1MB)**   | < 100ms        | 100 MB/s        | Single stream                  |
| **Get (1MB)**   | < 50ms         | 200 MB/s        | Cached reads                   |
| **List (1000)** | < 200ms        | 5000 items/s    | Paginated                      |
| **Presigned**   | < 10ms         | 1000 req/s      | No data transfer               |
| **Delete**      | < 50ms         | 500 req/s       | Async backend deletion         |

## Security Considerations

### Access Control

1. **Namespace Isolation**: Each namespace has separate credentials
2. **Presigned URLs**: Time-limited, scoped to specific operations
3. **Encryption at Rest**: MinIO supports server-side encryption
4. **Encryption in Transit**: TLS for production deployments
5. **Audit Logging**: All operations logged with user/namespace context

### Presigned URL Security

```rust
// Limit presigned URL validity
const MAX_PRESIGNED_EXPIRY: Duration = Duration::from_secs(3600); // 1 hour

pub async fn get_presigned_url(
    &self,
    key: &str,
    method: &str,
    expires_in: Duration,
) -> Result<String> {
    if expires_in > MAX_PRESIGNED_EXPIRY {
        return Err("Presigned URL expiry too long".into());
    }

    // Generate URL with limited scope
    self.backend.presigned_url(key, method, expires_in).await
}
```

## Migration Path

### Phase 1: MinIO Integration (Week 1-2)

1. **Docker Compose**: Add MinIO to local development stack
2. **Rust Backend**: Implement MinIO backend using aws-sdk-s3
3. **Protobuf Service**: Define ObjectStoreService
4. **Basic Operations**: Put, Get, Delete, List

### Phase 2: Advanced Features (Week 3-4)

1. **Streaming**: Large file uploads/downloads
2. **Presigned URLs**: Direct client access
3. **Lifecycle Policies**: TTL integration (ADR-031)
4. **Range Requests**: Partial downloads

### Phase 3: Client SDKs (Week 5-6)

1. **Python SDK**: Object storage client
2. **Go SDK**: Object storage client
3. **Integration Tests**: Full lifecycle tests
4. **Documentation**: API reference, examples

### Phase 4: Production Backends (Week 7-8)

1. **AWS S3 Adapter**: Production backend
2. **GCS Adapter**: Google Cloud Storage
3. **Azure Blob Adapter**: Azure support
4. **Multi-Backend**: Namespace routing

## Alternatives Considered

### Alternative 1: Database Blob Storage

**Approach**: Store blobs in PostgreSQL bytea columns.

**Rejected because**:
- Poor performance for large files
- Database backups become massive
- No streaming support
- Expensive storage costs

### Alternative 2: Filesystem Storage

**Approach**: Store files directly on disk, use file paths.

**Rejected because**:
- Not distributed (single server dependency)
- No replication or durability guarantees
- Hard to scale horizontally
- Complex cleanup and lifecycle management

### Alternative 3: Cloud-Only (No Local Testing)

**Approach**: Always use S3/GCS, even in development.

**Rejected because**:
- Requires internet connectivity
- Incurs cloud costs during development
- Slower test execution
- Harder to reproduce issues locally

## Related ADRs

- ADR-031: TTL Defaults for Client Data (lifecycle policies)
- ADR-010: Redis Integration (for metadata caching)
- ADR-015: PostgreSQL Integration (for object metadata table)
- ADR-020: ClickHouse Integration (for access logs analytics)

## References

- [MinIO Documentation](https://min.io/docs/minio/linux/index.html)
- [AWS S3 API Reference](https://docs.aws.amazon.com/AmazonS3/latest/API/Welcome.html)
- [S3 Presigned URLs](https://docs.aws.amazon.com/AmazonS3/latest/userguide/PresignedUrlUploadObject.html)
- [MinIO Lifecycle Management](https://min.io/docs/minio/linux/administration/object-management/lifecycle-management.html)
- [aws-sdk-rust](https://github.com/awslabs/aws-sdk-rust)

## Appendix: Object Storage Decision Tree

```
Do you need to store binary data > 1MB?
├─ Yes → Object Storage (this ADR)
└─ No
   ├─ Structured data? → KeyValue (PostgreSQL)
   └─ Small JSON/text? → Cache (Redis)

What's the access pattern?
├─ Infrequent, large files → S3 (production), MinIO (local)
├─ Frequent, small files → Redis + Object Storage
└─ Streaming media → Object Storage + CDN

What's the lifecycle?
├─ Temporary (< 7 days) → Object Storage with short TTL
├─ Medium-term (7-90 days) → Object Storage with default TTL
└─ Permanent → Object Storage with infinite TTL (explicit opt-in)
```

---

**Status**: Accepted
**Next Steps**:
1. Add MinIO to Docker Compose local stack
2. Implement MinIO backend in Rust proxy
3. Generate ObjectStoreService gRPC stubs
4. Write integration tests with ephemeral MinIO
5. Document object storage pattern in client SDK guides
