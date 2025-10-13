---
date: 2025-10-07
deciders: Core Team
doc_uuid: c58a9d5a-4fee-45d4-9825-51207d3de59b
id: adr-023
project_id: prism-data-layer
status: Accepted
tags:
- architecture
- grpc
- performance
- api-design
title: gRPC-First Interface Design
---

## Context

Prism needs a high-performance, type-safe API for client-server communication:
- Efficient binary protocol for low latency
- Strong typing with code generation
- Streaming support for large datasets
- HTTP/2 multiplexing for concurrent requests
- Cross-language client support

**Requirements:**
- **Performance**: Sub-millisecond overhead, 10k+ RPS per connection
- **Type safety**: Compile-time validation of requests/responses
- **Streaming**: Bidirectional streaming for pub/sub and pagination
- **Discoverability**: Self-documenting API via protobuf
- **Evolution**: Backward-compatible API changes

## Decision

Use **gRPC as the primary interface** for Prism data access layer:

1. **gRPC over HTTP/2**: Binary protocol with multiplexing
2. **Protobuf messages**: All requests/responses in protobuf
3. **Streaming first-class**: Unary, server-streaming, client-streaming, bidirectional
4. **No REST initially**: Focus on gRPC, add REST gateway later if needed
5. **Service-per-pattern**: Separate gRPC services for each access pattern

## Rationale

### Why gRPC

**Performance benefits:**
- Binary serialization (smaller payloads than JSON)
- HTTP/2 multiplexing (multiple requests per connection)
- Header compression (reduces overhead)
- Connection reuse (lower latency)

**Developer experience:**
- Code generation for multiple languages
- Type safety at compile time
- Self-documenting via `.proto` files
- Built-in deadline/timeout support
- Rich error model with status codes

**Streaming support:**
- Server streaming for pagination and pub/sub
- Client streaming for batch uploads
- Bidirectional streaming for real-time communication

### Architecture

┌─────────────────────────────────────────────────────────┐
│                  Prism gRPC Server                      │
│                   (Port 8980)                           │
│                                                           │
│  ┌─────────────────────────────────────────────────┐   │
│  │              gRPC Services                       │   │
│  │                                                   │   │
│  │  ┌──────────────┐  ┌──────────────┐            │   │
│  │  │ ConfigService│  │ SessionService│            │   │
│  │  └──────────────┘  └──────────────┘            │   │
│  │                                                   │   │
│  │  ┌──────────────┐  ┌──────────────┐            │   │
│  │  │  QueueService│  │ PubSubService│            │   │
│  │  └──────────────┘  └──────────────┘            │   │
│  │                                                   │   │
│  │  ┌──────────────┐  ┌──────────────┐            │   │
│  │  │ ReaderService│  │TransactService│            │   │
│  │  └──────────────┘  └──────────────┘            │   │
│  └─────────────────────────────────────────────────┘   │
│                                                           │
└─────────────────────────────────────────────────────────┘
                           │
                           │ HTTP/2 + Protobuf
                           │
              ┌────────────┴────────────┐
              │                         │
              │                         │
    ┌─────────▼─────────┐     ┌─────────▼─────────┐
    │   Go Client       │     │   Rust Client     │
    │ (generated code)  │     │ (generated code)  │
    └───────────────────┘     └───────────────────┘
```text

### Service Organization

Each access pattern gets its own service:

```
// proto/prism/session/v1/session_service.proto
service SessionService {
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
  rpc CloseSession(CloseSessionRequest) returns (CloseSessionResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}

// proto/prism/queue/v1/queue_service.proto
service QueueService {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (stream Message);
  rpc Acknowledge(AcknowledgeRequest) returns (AcknowledgeResponse);
  rpc Commit(CommitRequest) returns (CommitResponse);
}

// proto/prism/pubsub/v1/pubsub_service.proto
service PubSubService {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (stream Event);
  rpc Unsubscribe(UnsubscribeRequest) returns (UnsubscribeResponse);
}

// proto/prism/reader/v1/reader_service.proto
service ReaderService {
  rpc Read(ReadRequest) returns (stream Page);
  rpc Query(QueryRequest) returns (stream Row);
}

// proto/prism/transact/v1/transact_service.proto
service TransactService {
  rpc Write(WriteRequest) returns (WriteResponse);
  rpc Transaction(stream TransactRequest) returns (stream TransactResponse);
}
```text

### Streaming Patterns

**Server streaming** (pagination, pub/sub):
```
service ReaderService {
  // Server streams pages to client
  rpc Read(ReadRequest) returns (stream Page) {
    option (google.api.http) = {
      post: "/v1/reader/read"
      body: "*"
    };
  }
}
```text

```
// Server implementation
async fn read(&self, req: Request<ReadRequest>) -> Result<Response<Self::ReadStream>, Status> {
    let (tx, rx) = mpsc::channel(100);

    tokio::spawn(async move {
        let mut offset = 0;
        loop {
            let page = fetch_page(offset, 100).await?;
            if page.items.is_empty() {
                break;
            }
            tx.send(Ok(page)).await?;
            offset += 100;
        }
    });

    Ok(Response::new(ReceiverStream::new(rx)))
}
```text

**Client streaming** (batch writes):
```
service TransactService {
  // Client streams write batches
  rpc BatchWrite(stream WriteRequest) returns (WriteResponse);
}
```text

**Bidirectional streaming** (pub/sub with acks):
```
service PubSubService {
  // Client subscribes, server streams events, client sends acks
  rpc Stream(stream ClientMessage) returns (stream ServerMessage);
}
```text

### Error Handling

Use gRPC status codes:

```
use tonic::{Code, Status};

// Not found
return Err(Status::not_found(format!("namespace {} not found", namespace)));

// Invalid argument
return Err(Status::invalid_argument("page size must be > 0"));

// Unavailable
return Err(Status::unavailable("backend connection failed"));

// Deadline exceeded
return Err(Status::deadline_exceeded("operation timed out"));

// Permission denied
return Err(Status::permission_denied("insufficient permissions"));
```text

Structured error details:

```
import "google/rpc/error_details.proto";

message ErrorInfo {
  string reason = 1;
  string domain = 2;
  map<string, string> metadata = 3;
}
```text

### Metadata and Context

Use gRPC metadata for cross-cutting concerns:

```
// Server: extract session token from metadata
let session_token = req.metadata()
    .get("x-session-token")
    .and_then(|v| v.to_str().ok())
    .ok_or_else(|| Status::unauthenticated("missing session token"))?;

// Client: add session token to metadata
let mut request = Request::new(read_request);
request.metadata_mut().insert(
    "x-session-token",
    session_token.parse().unwrap(),
);
```text

Common metadata:
- `x-session-token`: Session identifier
- `x-namespace`: Namespace for multi-tenancy
- `x-request-id`: Request tracing
- `x-client-version`: Client version for compatibility

### Performance Optimizations

**Connection pooling:**
```
// Reuse connections
let channel = Channel::from_static("http://localhost:8980")
    .connect_lazy();

let client = QueueServiceClient::new(channel.clone());
```text

**Compression:**
```
// Enable gzip compression
let channel = Channel::from_static("http://localhost:8980")
    .http2_keep_alive_interval(Duration::from_secs(30))
    .http2_adaptive_window(true)
    .connect_lazy();
```text

**Timeouts:**
```
service QueueService {
  rpc Publish(PublishRequest) returns (PublishResponse) {
    option (google.api.method_signature) = "timeout=5s";
  }
}
```text

### Alternatives Considered

1. **REST/HTTP JSON API**
   - Pros: Simple, widespread tooling, human-readable
   - Cons: Slower serialization, no streaming, manual typing
   - Rejected: Performance critical for Prism

2. **GraphQL**
   - Pros: Flexible queries, single endpoint
   - Cons: Complexity, performance overhead, limited streaming
   - Rejected: Over-engineered for data access patterns

3. **WebSockets**
   - Pros: Bidirectional, real-time
   - Cons: No type safety, manual protocol design
   - Rejected: gRPC bidirectional streaming provides same benefits

4. **Thrift or Avro**
   - Pros: Binary protocols, similar performance
   - Cons: Smaller ecosystems, less tooling
   - Rejected: gRPC has better ecosystem and HTTP/2 benefits

## Consequences

### Positive

- **High performance**: Binary protocol, HTTP/2 multiplexing
- **Type safety**: Compile-time validation via protobuf
- **Streaming**: First-class support for all streaming patterns
- **Multi-language**: Generated clients for Go, Rust, Python, etc.
- **Self-documenting**: `.proto` files serve as API documentation
- **Evolution**: Backward-compatible changes via protobuf
- **Observability**: Built-in tracing, metrics integration

### Negative

- **Debugging complexity**: Binary format harder to inspect than JSON
- **Tooling required**: Need `grpcurl`, `grpcui` for manual testing
- **Learning curve**: Teams unfamiliar with gRPC/protobuf
- **Browser limitations**: No native browser support (need gRPC-Web)

### Neutral

- **HTTP/2 required**: Not compatible with HTTP/1.1-only infrastructure
- **REST gateway optional**: Can add later with `grpc-gateway`

## Implementation Notes

### Server Implementation (Rust)

```
// proxy/src/main.rs
use tonic::transport::Server;

#[tokio::main]
async fn main() -> Result<()> {
    let addr = "0.0.0.0:8980".parse()?;

    let session_service = SessionServiceImpl::default();
    let queue_service = QueueServiceImpl::default();
    let pubsub_service = PubSubServiceImpl::default();

    Server::builder()
        .add_service(SessionServiceServer::new(session_service))
        .add_service(QueueServiceServer::new(queue_service))
        .add_service(PubSubServiceServer::new(pubsub_service))
        .serve(addr)
        .await?;

    Ok(())
}
```text

### Client Implementation (Go)

```
// Client connection
conn, err := grpc.Dial(
    "localhost:8980",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:    30 * time.Second,
        Timeout: 10 * time.Second,
    }),
)
defer conn.Close()

// Create typed client
client := queue.NewQueueServiceClient(conn)

// Make request
resp, err := client.Publish(ctx, &queue.PublishRequest{
    Topic:   "events",
    Payload: data,
})
```text

### Testing with grpcurl

```
# List services
grpcurl -plaintext localhost:8980 list

# Describe service
grpcurl -plaintext localhost:8980 describe prism.queue.v1.QueueService

# Make request
grpcurl -plaintext -d '{"topic":"events","payload":"dGVzdA=="}' \
  localhost:8980 prism.queue.v1.QueueService/Publish
```text

### Code Generation

```
# Generate Rust code
buf generate --template proxy/buf.gen.rust.yaml

# Generate Go code
buf generate --template tools/buf.gen.go.yaml

# Generate Python code
buf generate --template clients/python/buf.gen.python.yaml
```text

## References

- [gRPC Documentation](https://grpc.io/docs/)
- [gRPC Performance Best Practices](https://grpc.io/docs/guides/performance/)
- [tonic (Rust gRPC)](https://github.com/hyperium/tonic)
- ADR-003: Protobuf as Single Source of Truth
- ADR-019: Rust Async Concurrency Patterns

## Revision History

- 2025-10-07: Initial draft and acceptance

```