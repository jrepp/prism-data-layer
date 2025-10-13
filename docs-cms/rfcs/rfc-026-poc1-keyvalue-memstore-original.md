---
author: Platform Team
created: 2025-10-09
doc_uuid: a9c5b30d-fe19-40dc-aebc-866125dbb6d1
id: rfc-026
project_id: prism-data-layer
status: Superseded
tags:
- poc
- implementation
- keyvalue
- memstore
- walking-skeleton
- workstreams
- superseded
title: POC 1 - KeyValue with MemStore Implementation Plan (Original)
updated: 2025-10-09
---

# RFC-026: POC 1 - KeyValue with MemStore Implementation Plan (Original)

**Note**: This RFC has been superseded by [RFC-021: Three Minimal Plugins](/rfc/rfc-021), which provides a more focused approach with three minimal plugins instead of a single complex MemStore plugin.

## Summary

Detailed implementation plan for POC 1: **KeyValue with MemStore (Walking Skeleton)**. This RFC expands RFC-018's high-level POC strategy into actionable work streams with clearly defined tasks, acceptance criteria, and dependencies. POC 1 establishes the foundational end-to-end architecture by implementing the thinnest possible slice demonstrating proxy → plugin → backend → client integration.

**Timeline**: 2 weeks (10 working days)
**Team Size**: 2-3 engineers
**Approach**: Walking Skeleton - build thinnest end-to-end slice, then iterate

## Motivation

### Problem

RFC-018 provides a comprehensive POC strategy across 5 POCs, but POC 1 needs detailed task breakdown for execution. Teams need:
- **Clear work streams**: Parallelizable tracks for efficient development
- **Specific tasks**: Actionable items with acceptance criteria
- **Dependency mapping**: Understanding what blocks what
- **Estimation granularity**: Day-level estimates for sprint planning

### Goals

1. **Actionable Plan**: Break POC 1 into tasks assignable to engineers
2. **Parallel Execution**: Identify independent work streams for parallel development
3. **Risk Mitigation**: Surface blocking dependencies early
4. **Quality Gates**: Define testable acceptance criteria per task
5. **Tracking**: Enable progress monitoring and velocity measurement

## Objective: Walking Skeleton

Build the **thinnest possible end-to-end slice** demonstrating:
- ✅ Rust proxy receiving gRPC client requests
- ✅ Go MemStore plugin handling KeyValue operations
- ✅ In-memory backend (sync.Map + List slice)
- ✅ Python client library with ergonomic API
- ✅ Minimal admin API for configuration
- ✅ Local development setup with Docker Compose

**What "Walking Skeleton" Means**:
- Implements ONE pattern (KeyValue) with ONE backend (MemStore)
- No authentication, no observability, no multi-tenancy
- Manual logs only (no structured logging)
- Single namespace ("default")
- Focus: **prove the architecture works end-to-end**

## Architecture Overview

### Component Diagram

```text
┌────────────────────────────────────────────────────────────┐
│                    POC 1 Architecture                      │
│                                                            │
│  ┌──────────────────────────────────────────────────────┐ │
│  │  Python Client (clients/python/)                     │ │
│  │  - KeyValue API: set(), get(), delete(), scan()      │ │
│  └────────────────┬─────────────────────────────────────┘ │
│                   │                                        │
│                   │ gRPC (KeyValueService)                 │
│                   ▼                                        │
│  ┌──────────────────────────────────────────────────────┐ │
│  │  Rust Proxy (proxy/)                                 │ │
│  │  - gRPC server on :8980                             │ │
│  │  - Load plugin from config                          │ │
│  │  - Forward requests to plugin                       │ │
│  └────────────────┬─────────────────────────────────────┘ │
│                   │                                        │
│                   │ gRPC (KeyValueInterface)               │
│                   ▼                                        │
│  ┌──────────────────────────────────────────────────────┐ │
│  │  Go MemStore Plugin (plugins/memstore/)              │ │
│  │  - gRPC server on dynamic port                      │ │
│  │  - sync.Map for KeyValue storage                    │ │
│  │  - []interface{} slice for List storage             │ │
│  │  - TTL cleanup with time.AfterFunc                  │ │
│  └──────────────────────────────────────────────────────┘ │
│                                                            │
│  ┌──────────────────────────────────────────────────────┐ │
│  │  Admin API (admin/)                                  │ │
│  │  - FastAPI server on :8090                          │ │
│  │  - POST /namespaces (create namespace)              │ │
│  │  - Writes proxy config file                         │ │
│  └──────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

### Technology Stack

| Component | Language | Framework/Library | Protocol |
|-----------|----------|-------------------|----------|
| Proxy | Rust | tokio, tonic (gRPC) | gRPC |
| MemStore Plugin | Go | google.golang.org/grpc | gRPC |
| Python Client | Python 3.11+ | grpcio, asyncio | gRPC |
| Admin API | Python 3.11+ | FastAPI | HTTP |

## Work Streams

### Work Stream 1: Protobuf Schema and Code Generation

**Owner**: 1 engineer
**Duration**: 1 day
**Dependencies**: None (can start immediately)

#### Tasks

**Task 1.1: Define KeyValue protobuf interface** (2 hours)
```protobuf
// proto/interfaces/keyvalue_basic.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

service KeyValueBasicInterface {
  rpc Set(SetRequest) returns (SetResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Exists(ExistsRequest) returns (ExistsResponse);
}

message SetRequest {
  string namespace = 1;
  string key = 2;
  bytes value = 3;
  optional int64 ttl_seconds = 4;  // Optional TTL
}

message SetResponse {
  bool success = 1;
}

message GetRequest {
  string namespace = 1;
  string key = 2;
}

message GetResponse {
  bytes value = 1;
}

message DeleteRequest {
  string namespace = 1;
  string key = 2;
}

message DeleteResponse {
  bool found = 1;
}

message ExistsRequest {
  string namespace = 1;
  string key = 2;
}

message ExistsResponse {
  bool exists = 1;
}
```

**Acceptance Criteria**:
- [ ] `keyvalue_basic.proto` file created
- [ ] Compiles with `protoc` without errors
- [ ] Includes Set, Get, Delete, Exists operations
- [ ] TTL field in SetRequest (optional)

**Task 1.2: Define KeyValue Scan interface** (1 hour)
```protobuf
// proto/interfaces/keyvalue_scan.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

service KeyValueScanInterface {
  rpc Scan(ScanRequest) returns (stream ScanResponse);
  rpc ScanKeys(ScanKeysRequest) returns (stream KeyResponse);
  rpc Count(CountRequest) returns (CountResponse);
}

message ScanRequest {
  string namespace = 1;
  string prefix = 2;
  int32 limit = 3;  // Max keys to return (0 = unlimited)
}

message ScanResponse {
  string key = 1;
  bytes value = 2;
}

message ScanKeysRequest {
  string namespace = 1;
  string prefix = 2;
  int32 limit = 3;
}

message KeyResponse {
  string key = 1;
}

message CountRequest {
  string namespace = 1;
  string prefix = 2;
}

message CountResponse {
  int64 count = 1;
}
```

**Acceptance Criteria**:
- [ ] `keyvalue_scan.proto` file created
- [ ] Streaming response for Scan operation
- [ ] Prefix-based filtering supported

**Task 1.3: Define List protobuf interface** (2 hours)
```protobuf
// proto/interfaces/list_basic.proto
syntax = "proto3";
package prism.interfaces.list;

service ListBasicInterface {
  rpc PushLeft(PushLeftRequest) returns (PushLeftResponse);
  rpc PushRight(PushRightRequest) returns (PushRightResponse);
  rpc PopLeft(PopLeftRequest) returns (PopLeftResponse);
  rpc PopRight(PopRightRequest) returns (PopRightResponse);
  rpc Length(LengthRequest) returns (LengthResponse);
}

message PushLeftRequest {
  string namespace = 1;
  string list_key = 2;
  bytes value = 3;
}

message PushLeftResponse {
  int64 new_length = 1;
}

message PushRightRequest {
  string namespace = 1;
  string list_key = 2;
  bytes value = 3;
}

message PushRightResponse {
  int64 new_length = 1;
}

message PopLeftRequest {
  string namespace = 1;
  string list_key = 2;
}

message PopLeftResponse {
  bytes value = 1;
  bool found = 2;
}

message PopRightRequest {
  string namespace = 1;
  string list_key = 2;
}

message PopRightResponse {
  bytes value = 1;
  bool found = 2;
}

message LengthRequest {
  string namespace = 1;
  string list_key = 2;
}

message LengthResponse {
  int64 length = 1;
}
```

**Acceptance Criteria**:
- [ ] `list_basic.proto` file created
- [ ] Compiles with `protoc` without errors
- [ ] Includes PushLeft, PushRight, PopLeft, PopRight, Length operations

**Task 1.4: Generate code for all languages** (2 hours)
```bash
# Makefile targets
proto-generate:
	# Generate Rust code
	protoc --rust_out=proto/rust/ --grpc-rust_out=proto/rust/ proto/**/*.proto

	# Generate Go code
	protoc --go_out=proto/go/ --go-grpc_out=proto/go/ proto/**/*.proto

	# Generate Python code
	python -m grpc_tools.protoc -I proto/ --python_out=clients/python/ \
		--grpc_python_out=clients/python/ proto/**/*.proto
```

**Acceptance Criteria**:
- [ ] Makefile target `proto-generate` works
- [ ] Rust code generated in `proto/rust/`
- [ ] Go code generated in `proto/go/`
- [ ] Python code generated in `clients/python/prism_pb2.py`
- [ ] No compilation errors in any language

### Work Stream 2: Rust Proxy Implementation

**Owner**: 1 engineer (Rust experience required)
**Duration**: 4 days
**Dependencies**: Task 1.4 (protobuf generation)

#### Tasks

**Task 2.1: Setup Rust project structure** (1 day)
```text
proxy/
├── Cargo.toml          # Dependencies: tokio, tonic, serde
├── src/
│   ├── main.rs         # Entry point
│   ├── config.rs       # Configuration loading
│   ├── server.rs       # gRPC server setup
│   ├── plugin.rs       # Plugin client management
│   └── error.rs        # Error types
└── tests/
    └── integration_test.rs
```

```toml
# proxy/Cargo.toml
[package]
name = "prism-proxy"
version = "0.1.0"
edition = "2021"

[dependencies]
tokio = { version = "1", features = ["full"] }
tonic = "0.10"
serde = { version = "1.0", features = ["derive"] }
serde_yaml = "0.9"
anyhow = "1.0"
tracing = "0.1"
tracing-subscriber = "0.3"

[build-dependencies]
tonic-build = "0.10"
```

**Acceptance Criteria**:
- [ ] `cargo build` succeeds
- [ ] Project structure created
- [ ] Dependencies resolved
- [ ] Hello world binary runs

**Task 2.2: Implement configuration loading** (half day)
```yaml
# proxy/config.yaml
server:
  listen_address: "0.0.0.0:8980"

namespaces:
  - name: default
    pattern: keyvalue
    plugin:
      endpoint: "localhost:50051"  # MemStore plugin address
```

```rust
// proxy/src/config.rs
use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize, Serialize)]
pub struct Config {
    pub server: ServerConfig,
    pub namespaces: Vec<NamespaceConfig>,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct ServerConfig {
    pub listen_address: String,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct NamespaceConfig {
    pub name: String,
    pub pattern: String,
    pub plugin: PluginConfig,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct PluginConfig {
    pub endpoint: String,
}

impl Config {
    pub fn load(path: &str) -> anyhow::Result<Self> {
        let content = std::fs::read_to_string(path)?;
        let config: Config = serde_yaml::from_str(&content)?;
        Ok(config)
    }
}
```

**Acceptance Criteria**:
- [ ] Config file parsing works
- [ ] Errors on missing fields
- [ ] Returns structured Config object
- [ ] Unit tests for valid and invalid configs

**Task 2.3: Implement gRPC server skeleton** (1 day)
```rust
// proxy/src/server.rs
use tonic::{transport::Server, Request, Response, Status};
use prism_pb::keyvalue_service_server::{KeyValueService, KeyValueServiceServer};
use prism_pb::{GetRequest, GetResponse, SetRequest, SetResponse};

pub struct ProxyService {
    plugin_client: PluginClient,
}

#[tonic::async_trait]
impl KeyValueService for ProxyService {
    async fn set(&self, request: Request<SetRequest>) -> Result<Response<SetResponse>, Status> {
        // Forward to plugin
        let req = request.into_inner();
        let resp = self.plugin_client.set(req).await?;
        Ok(Response::new(resp))
    }

    async fn get(&self, request: Request<GetRequest>) -> Result<Response<GetResponse>, Status> {
        let req = request.into_inner();
        let resp = self.plugin_client.get(req).await?;
        Ok(Response::new(resp))
    }

    // ... delete, exists, scan
}

pub async fn start_server(config: Config) -> anyhow::Result<()> {
    let addr = config.server.listen_address.parse()?;

    // Create plugin client
    let plugin_client = PluginClient::connect(config.namespaces[0].plugin.endpoint).await?;

    let service = ProxyService { plugin_client };

    Server::builder()
        .add_service(KeyValueServiceServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
```

**Acceptance Criteria**:
- [ ] gRPC server starts on configured port
- [ ] Health check endpoint responds
- [ ] Graceful shutdown on SIGTERM
- [ ] Logs server startup

**Task 2.4: Implement plugin client forwarding** (1 day)
```rust
// proxy/src/plugin.rs
use prism_pb::key_value_basic_interface_client::KeyValueBasicInterfaceClient;
use tonic::transport::Channel;

pub struct PluginClient {
    client: KeyValueBasicInterfaceClient<Channel>,
}

impl PluginClient {
    pub async fn connect(endpoint: String) -> anyhow::Result<Self> {
        let client = KeyValueBasicInterfaceClient::connect(endpoint).await?;
        Ok(Self { client })
    }

    pub async fn set(&self, req: SetRequest) -> Result<SetResponse, tonic::Status> {
        let mut client = self.client.clone();
        let response = client.set(req).await?;
        Ok(response.into_inner())
    }

    pub async fn get(&self, req: GetRequest) -> Result<GetResponse, tonic::Status> {
        let mut client = self.client.clone();
        let response = client.get(req).await?;
        Ok(response.into_inner())
    }

    // ... delete, exists, scan
}
```

**Acceptance Criteria**:
- [ ] Plugin client connects to Go plugin
- [ ] Forwards Set, Get, Delete, Exists operations
- [ ] Handles gRPC errors correctly
- [ ] Retries connection on failure

**Task 2.5: Add basic error handling** (half day)
```rust
// proxy/src/error.rs
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ProxyError {
    #[error("Configuration error: {0}")]
    Config(String),

    #[error("Plugin connection error: {0}")]
    PluginConnection(String),

    #[error("gRPC error: {0}")]
    Grpc(#[from] tonic::Status),

    #[error("Internal error: {0}")]
    Internal(String),
}

impl From<ProxyError> for tonic::Status {
    fn from(err: ProxyError) -> Self {
        match err {
            ProxyError::Config(msg) => tonic::Status::invalid_argument(msg),
            ProxyError::PluginConnection(msg) => tonic::Status::unavailable(msg),
            ProxyError::Grpc(status) => status,
            ProxyError::Internal(msg) => tonic::Status::internal(msg),
        }
    }
}
```

**Acceptance Criteria**:
- [ ] Errors mapped to gRPC status codes
- [ ] Error messages logged
- [ ] Client receives meaningful error responses

### Work Stream 3: Go MemStore Plugin Implementation

**Owner**: 1 engineer (Go experience required)
**Duration**: 3 days
**Dependencies**: Task 1.4 (protobuf generation)
**Can run in parallel with**: Work Stream 2

#### Tasks

**Task 3.1: Setup Go project structure** (half day)
```text
plugins/memstore/
├── go.mod                  # Module definition
├── main.go                 # Entry point
├── server.go               # gRPC server
├── storage/
│   ├── keyvalue.go         # KeyValue sync.Map storage
│   ├── list.go             # List slice storage
│   └── ttl.go              # TTL cleanup
└── tests/
    └── memstore_test.go
```

```go
// plugins/memstore/go.mod
module github.com/prism/plugins/memstore

go 1.21

require (
    google.golang.org/grpc v1.58.0
    google.golang.org/protobuf v1.31.0
)
```

**Acceptance Criteria**:
- [ ] `go build` succeeds
- [ ] Project structure created
- [ ] Dependencies resolved
- [ ] Hello world binary runs

**Task 3.2: Implement KeyValue storage with sync.Map** (1 day)
```go
// plugins/memstore/storage/keyvalue.go
package storage

import (
    "sync"
    "time"
)

type KeyValueStore struct {
    data    sync.Map               // map[string][]byte
    ttls    sync.Map               // map[string]*time.Timer
    mu      sync.RWMutex
}

func NewKeyValueStore() *KeyValueStore {
    return &KeyValueStore{}
}

func (kv *KeyValueStore) Set(key string, value []byte, ttlSeconds int64) error {
    kv.data.Store(key, value)

    if ttlSeconds > 0 {
        kv.setTTL(key, time.Duration(ttlSeconds)*time.Second)
    }

    return nil
}

func (kv *KeyValueStore) Get(key string) ([]byte, bool) {
    // Check if key exists and not expired
    value, ok := kv.data.Load(key)
    if !ok {
        return nil, false
    }

    return value.([]byte), true
}

func (kv *KeyValueStore) Delete(key string) bool {
    _, ok := kv.data.LoadAndDelete(key)

    // Cancel TTL timer if exists
    if timer, found := kv.ttls.LoadAndDelete(key); found {
        timer.(*time.Timer).Stop()
    }

    return ok
}

func (kv *KeyValueStore) Exists(key string) bool {
    _, ok := kv.data.Load(key)
    return ok
}

func (kv *KeyValueStore) Scan(prefix string, limit int) []string {
    keys := []string{}
    kv.data.Range(func(k, v interface{}) bool {
        key := k.(string)
        if strings.HasPrefix(key, prefix) {
            keys = append(keys, key)
            if limit > 0 && len(keys) >= limit {
                return false  // Stop iteration
            }
        }
        return true
    })
    return keys
}

func (kv *KeyValueStore) setTTL(key string, duration time.Duration) {
    timer := time.AfterFunc(duration, func() {
        kv.Delete(key)
    })
    kv.ttls.Store(key, timer)
}
```

**Acceptance Criteria**:
- [ ] Set, Get, Delete, Exists operations work
- [ ] TTL expiration deletes keys automatically
- [ ] Thread-safe (passes race detector)
- [ ] Scan supports prefix matching
- [ ] Unit tests for all operations

**Task 3.3: Implement List storage with slices** (1 day)
```go
// plugins/memstore/storage/list.go
package storage

import (
    "sync"
)

type ListStore struct {
    lists sync.Map  // map[string]*List
}

type List struct {
    mu    sync.RWMutex
    items [][]byte
}

func NewListStore() *ListStore {
    return &ListStore{}
}

func (ls *ListStore) getOrCreate(listKey string) *List {
    if val, ok := ls.lists.Load(listKey); ok {
        return val.(*List)
    }

    list := &List{items: [][]byte{}}
    ls.lists.Store(listKey, list)
    return list
}

func (ls *ListStore) PushLeft(listKey string, value []byte) int64 {
    list := ls.getOrCreate(listKey)
    list.mu.Lock()
    defer list.mu.Unlock()

    // Prepend (expensive - requires copy)
    list.items = append([][]byte{value}, list.items...)
    return int64(len(list.items))
}

func (ls *ListStore) PushRight(listKey string, value []byte) int64 {
    list := ls.getOrCreate(listKey)
    list.mu.Lock()
    defer list.mu.Unlock()

    // Append (efficient)
    list.items = append(list.items, value)
    return int64(len(list.items))
}

func (ls *ListStore) PopLeft(listKey string) ([]byte, bool) {
    list := ls.getOrCreate(listKey)
    list.mu.Lock()
    defer list.mu.Unlock()

    if len(list.items) == 0 {
        return nil, false
    }

    value := list.items[0]
    list.items = list.items[1:]  // Reslice
    return value, true
}

func (ls *ListStore) PopRight(listKey string) ([]byte, bool) {
    list := ls.getOrCreate(listKey)
    list.mu.Lock()
    defer list.mu.Unlock()

    if len(list.items) == 0 {
        return nil, false
    }

    value := list.items[len(list.items)-1]
    list.items = list.items[:len(list.items)-1]  // Reslice
    return value, true
}

func (ls *ListStore) Length(listKey string) int64 {
    list := ls.getOrCreate(listKey)
    list.mu.RLock()
    defer list.mu.RUnlock()

    return int64(len(list.items))
}
```

**Acceptance Criteria**:
- [ ] PushLeft, PushRight, PopLeft, PopRight, Length operations work
- [ ] Thread-safe (passes race detector)
- [ ] Empty list returns (nil, false) for pops
- [ ] Unit tests for all operations

**Task 3.4: Implement gRPC server** (1 day)
```go
// plugins/memstore/server.go
package main

import (
    "context"
    "fmt"
    "net"

    "google.golang.org/grpc"
    pb "github.com/prism/proto/go"
    "github.com/prism/plugins/memstore/storage"
)

type MemStoreServer struct {
    pb.UnimplementedKeyValueBasicInterfaceServer
    pb.UnimplementedKeyValueScanInterfaceServer
    pb.UnimplementedListBasicInterfaceServer

    keyvalue *storage.KeyValueStore
    lists    *storage.ListStore
}

func NewMemStoreServer() *MemStoreServer {
    return &MemStoreServer{
        keyvalue: storage.NewKeyValueStore(),
        lists:    storage.NewListStore(),
    }
}

func (s *MemStoreServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
    err := s.keyvalue.Set(req.Key, req.Value, req.TtlSeconds)
    if err != nil {
        return nil, err
    }
    return &pb.SetResponse{Success: true}, nil
}

func (s *MemStoreServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
    value, found := s.keyvalue.Get(req.Key)
    if !found {
        return nil, status.Error(codes.NotFound, "key not found")
    }
    return &pb.GetResponse{Value: value}, nil
}

// ... Delete, Exists, Scan, PushLeft, PushRight, PopLeft, PopRight, Length

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    s := grpc.NewServer()
    pb.RegisterKeyValueBasicInterfaceServer(s, NewMemStoreServer())
    pb.RegisterKeyValueScanInterfaceServer(s, NewMemStoreServer())
    pb.RegisterListBasicInterfaceServer(s, NewMemStoreServer())

    log.Printf("MemStore plugin listening on :50051")
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
```

**Acceptance Criteria**:
- [ ] gRPC server starts on :50051
- [ ] All KeyValue operations work
- [ ] All List operations work
- [ ] Health check responds
- [ ] Graceful shutdown

### Work Stream 4: Python Client Library

**Owner**: 1 engineer (Python experience required)
**Duration**: 2 days
**Dependencies**: Task 1.4 (protobuf generation)
**Can run in parallel with**: Work Streams 2 and 3

#### Tasks

**Task 4.1: Setup Python project structure** (half day)
```text
clients/python/
├── pyproject.toml           # Poetry/pip dependencies
├── prism/
│   ├── __init__.py
│   ├── client.py            # Main client class
│   ├── keyvalue.py          # KeyValue API
│   ├── list.py              # List API
│   └── errors.py            # Custom exceptions
├── prism_pb2.py             # Generated protobuf (from Task 1.4)
├── prism_pb2_grpc.py        # Generated gRPC (from Task 1.4)
└── tests/
    ├── test_keyvalue.py
    └── test_list.py
```

```toml
# clients/python/pyproject.toml
[project]
name = "prism-client"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = [
    "grpcio>=1.58.0",
    "grpcio-tools>=1.58.0",
    "protobuf>=4.24.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=7.4.0",
    "pytest-asyncio>=0.21.0",
]
```

**Acceptance Criteria**:
- [ ] Python package installs (`pip install -e .`)
- [ ] Dependencies resolved
- [ ] Project structure created
- [ ] Can import `from prism import PrismClient`

**Task 4.2: Implement PrismClient main class** (half day)
```python
# clients/python/prism/client.py
import grpc
from prism.keyvalue import KeyValueAPI
from prism.list import ListAPI

class PrismClient:
    """Prism data access client."""

    def __init__(self, proxy_address: str):
        """
        Initialize Prism client.

        Args:
            proxy_address: Proxy address (e.g., "localhost:8980")
        """
        self.proxy_address = proxy_address
        self.channel = grpc.aio.insecure_channel(proxy_address)

        # Pattern APIs
        self.keyvalue = KeyValueAPI(self.channel)
        self.list = ListAPI(self.channel)

    async def close(self):
        """Close gRPC channel."""
        await self.channel.close()

    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        await self.close()
```

**Acceptance Criteria**:
- [ ] Client initializes with proxy address
- [ ] gRPC channel created
- [ ] Context manager support
- [ ] Exposes `keyvalue` and `list` APIs

**Task 4.3: Implement KeyValue API** (1 day)
```python
# clients/python/prism/keyvalue.py
from typing import Optional, AsyncIterator
import prism_pb2
import prism_pb2_grpc
from prism.errors import KeyNotFoundError

class KeyValueAPI:
    """KeyValue pattern API."""

    def __init__(self, channel):
        self.stub = prism_pb2_grpc.KeyValueBasicInterfaceStub(channel)
        self.scan_stub = prism_pb2_grpc.KeyValueScanInterfaceStub(channel)

    async def set(
        self,
        key: str,
        value: bytes,
        namespace: str = "default",
        ttl_seconds: Optional[int] = None
    ) -> None:
        """
        Set a key-value pair.

        Args:
            key: Key to set
            value: Value bytes
            namespace: Namespace (default: "default")
            ttl_seconds: Optional TTL in seconds

        Raises:
            grpc.RpcError: On gRPC error
        """
        request = prism_pb2.SetRequest(
            namespace=namespace,
            key=key,
            value=value,
        )
        if ttl_seconds is not None:
            request.ttl_seconds = ttl_seconds

        await self.stub.Set(request)

    async def get(
        self,
        key: str,
        namespace: str = "default"
    ) -> bytes:
        """
        Get value for a key.

        Args:
            key: Key to get
            namespace: Namespace (default: "default")

        Returns:
            Value bytes

        Raises:
            KeyNotFoundError: If key doesn't exist
            grpc.RpcError: On gRPC error
        """
        request = prism_pb2.GetRequest(namespace=namespace, key=key)

        try:
            response = await self.stub.Get(request)
            return response.value
        except grpc.RpcError as e:
            if e.code() == grpc.StatusCode.NOT_FOUND:
                raise KeyNotFoundError(f"Key not found: {key}")
            raise

    async def delete(
        self,
        key: str,
        namespace: str = "default"
    ) -> bool:
        """
        Delete a key.

        Args:
            key: Key to delete
            namespace: Namespace (default: "default")

        Returns:
            True if key was found and deleted, False otherwise
        """
        request = prism_pb2.DeleteRequest(namespace=namespace, key=key)
        response = await self.stub.Delete(request)
        return response.found

    async def exists(
        self,
        key: str,
        namespace: str = "default"
    ) -> bool:
        """
        Check if a key exists.

        Args:
            key: Key to check
            namespace: Namespace (default: "default")

        Returns:
            True if key exists, False otherwise
        """
        request = prism_pb2.ExistsRequest(namespace=namespace, key=key)
        response = await self.stub.Exists(request)
        return response.exists

    async def scan(
        self,
        prefix: str = "",
        namespace: str = "default",
        limit: int = 0
    ) -> AsyncIterator[tuple[str, bytes]]:
        """
        Scan keys by prefix (streaming).

        Args:
            prefix: Key prefix to match (empty = all keys)
            namespace: Namespace (default: "default")
            limit: Max keys to return (0 = unlimited)

        Yields:
            Tuples of (key, value)
        """
        request = prism_pb2.ScanRequest(
            namespace=namespace,
            prefix=prefix,
            limit=limit
        )

        async for response in self.scan_stub.Scan(request):
            yield (response.key, response.value)
```

**Acceptance Criteria**:
- [ ] All methods (set, get, delete, exists, scan) work
- [ ] Async/await support
- [ ] TTL parameter optional
- [ ] scan() is async iterator
- [ ] Custom KeyNotFoundError exception
- [ ] Type hints for all methods

**Task 4.4: Implement List API** (half day)
```python
# clients/python/prism/list.py
from typing import Optional
import prism_pb2
import prism_pb2_grpc

class ListAPI:
    """List pattern API."""

    def __init__(self, channel):
        self.stub = prism_pb2_grpc.ListBasicInterfaceStub(channel)

    async def push_left(
        self,
        list_key: str,
        value: bytes,
        namespace: str = "default"
    ) -> int:
        """
        Push value to left (head) of list.

        Returns:
            New list length
        """
        request = prism_pb2.PushLeftRequest(
            namespace=namespace,
            list_key=list_key,
            value=value
        )
        response = await self.stub.PushLeft(request)
        return response.new_length

    async def push_right(
        self,
        list_key: str,
        value: bytes,
        namespace: str = "default"
    ) -> int:
        """
        Push value to right (tail) of list.

        Returns:
            New list length
        """
        request = prism_pb2.PushRightRequest(
            namespace=namespace,
            list_key=list_key,
            value=value
        )
        response = await self.stub.PushRight(request)
        return response.new_length

    async def pop_left(
        self,
        list_key: str,
        namespace: str = "default"
    ) -> Optional[bytes]:
        """
        Pop value from left (head) of list.

        Returns:
            Value bytes, or None if list is empty
        """
        request = prism_pb2.PopLeftRequest(
            namespace=namespace,
            list_key=list_key
        )
        response = await self.stub.PopLeft(request)
        return response.value if response.found else None

    async def pop_right(
        self,
        list_key: str,
        namespace: str = "default"
    ) -> Optional[bytes]:
        """
        Pop value from right (tail) of list.

        Returns:
            Value bytes, or None if list is empty
        """
        request = prism_pb2.PopRightRequest(
            namespace=namespace,
            list_key=list_key
        )
        response = await self.stub.PopRight(request)
        return response.value if response.found else None

    async def length(
        self,
        list_key: str,
        namespace: str = "default"
    ) -> int:
        """
        Get list length.

        Returns:
            List length (0 if list doesn't exist)
        """
        request = prism_pb2.LengthRequest(
            namespace=namespace,
            list_key=list_key
        )
        response = await self.stub.Length(request)
        return response.length
```

**Acceptance Criteria**:
- [ ] All methods (push_left, push_right, pop_left, pop_right, length) work
- [ ] Async/await support
- [ ] Returns None for empty list pops
- [ ] Type hints for all methods

### Work Stream 5: Integration Tests and Demo

**Owner**: 1 engineer
**Duration**: 2 days
**Dependencies**: Work Streams 2, 3, 4 complete

#### Tasks

**Task 5.1: Write integration tests** (1 day)
```python
# tests/poc1/test_keyvalue_memstore.py
import pytest
from prism import PrismClient
from prism.errors import KeyNotFoundError

@pytest.mark.asyncio
async def test_set_get():
    """Test basic set/get operation."""
    async with PrismClient("localhost:8980") as client:
        await client.keyvalue.set("test-key", b"test-value")
        value = await client.keyvalue.get("test-key")
        assert value == b"test-value"

@pytest.mark.asyncio
async def test_delete():
    """Test delete operation."""
    async with PrismClient("localhost:8980") as client:
        await client.keyvalue.set("delete-me", b"data")

        found = await client.keyvalue.delete("delete-me")
        assert found == True

        with pytest.raises(KeyNotFoundError):
            await client.keyvalue.get("delete-me")

@pytest.mark.asyncio
async def test_ttl():
    """Test TTL expiration."""
    async with PrismClient("localhost:8980") as client:
        await client.keyvalue.set("expires", b"soon", ttl_seconds=1)

        # Key exists initially
        assert await client.keyvalue.exists("expires") == True

        # Wait for expiration
        await asyncio.sleep(1.5)

        # Key should be gone
        assert await client.keyvalue.exists("expires") == False

@pytest.mark.asyncio
async def test_scan():
    """Test scan operation."""
    async with PrismClient("localhost:8980") as client:
        await client.keyvalue.set("user:1", b"alice")
        await client.keyvalue.set("user:2", b"bob")
        await client.keyvalue.set("post:1", b"hello")

        keys = []
        async for key, value in client.keyvalue.scan("user:"):
            keys.append(key)

        assert len(keys) == 2
        assert "user:1" in keys
        assert "user:2" in keys
        assert "post:1" not in keys

@pytest.mark.asyncio
async def test_list_fifo():
    """Test list FIFO operations."""
    async with PrismClient("localhost:8980") as client:
        # Push to right, pop from left (FIFO queue)
        await client.list.push_right("queue", b"first")
        await client.list.push_right("queue", b"second")
        await client.list.push_right("queue", b"third")

        assert await client.list.length("queue") == 3

        assert await client.list.pop_left("queue") == b"first"
        assert await client.list.pop_left("queue") == b"second"
        assert await client.list.pop_left("queue") == b"third"

        # Empty list
        assert await client.list.pop_left("queue") is None

@pytest.mark.asyncio
async def test_list_stack():
    """Test list LIFO operations."""
    async with PrismClient("localhost:8980") as client:
        # Push to right, pop from right (LIFO stack)
        await client.list.push_right("stack", b"first")
        await client.list.push_right("stack", b"second")
        await client.list.push_right("stack", b"third")

        assert await client.list.pop_right("stack") == b"third"
        assert await client.list.pop_right("stack") == b"second"
        assert await client.list.pop_right("stack") == b"first"
```

**Acceptance Criteria**:
- [ ] All tests pass
- [ ] Tests run with `pytest tests/poc1/`
- [ ] Test coverage &gt;80%
- [ ] Tests run in CI

**Task 5.2: Create demo script** (half day)
```python
# examples/poc1-demo.py
"""
POC 1 Demo: KeyValue and List operations with MemStore backend.

Shows basic CRUD operations, TTL, scanning, and list FIFO/LIFO patterns.
"""
import asyncio
from prism import PrismClient

async def demo_keyvalue():
    print("=== KeyValue Pattern Demo ===\n")

    async with PrismClient("localhost:8980") as client:
        # Set and get
        print("1. Setting key-value pairs...")
        await client.keyvalue.set("user:alice", b'{"name": "Alice", "age": 30}')
        await client.keyvalue.set("user:bob", b'{"name": "Bob", "age": 25}')
        print("   ✓ Set user:alice and user:bob")

        # Get
        print("\n2. Getting value...")
        value = await client.keyvalue.get("user:alice")
        print(f"   ✓ user:alice = {value.decode()}")

        # Scan
        print("\n3. Scanning keys with prefix 'user:'...")
        async for key, value in client.keyvalue.scan("user:"):
            print(f"   ✓ {key} = {value.decode()}")

        # TTL
        print("\n4. Setting key with TTL (expires in 5 seconds)...")
        await client.keyvalue.set("session:123", b"temporary-data", ttl_seconds=5)
        print(f"   ✓ session:123 exists: {await client.keyvalue.exists('session:123')}")

        print("   Waiting 5 seconds for expiration...")
        await asyncio.sleep(5.5)
        print(f"   ✓ session:123 exists: {await client.keyvalue.exists('session:123')}")

        # Delete
        print("\n5. Deleting key...")
        found = await client.keyvalue.delete("user:bob")
        print(f"   ✓ Deleted user:bob (found: {found})")

async def demo_list():
    print("\n\n=== List Pattern Demo ===\n")

    async with PrismClient("localhost:8980") as client:
        # FIFO queue
        print("1. FIFO Queue (push right, pop left)...")
        await client.list.push_right("queue", b"task-1")
        await client.list.push_right("queue", b"task-2")
        await client.list.push_right("queue", b"task-3")
        print(f"   ✓ Queue length: {await client.list.length('queue')}")

        print("   Processing queue:")
        while True:
            task = await client.list.pop_left("queue")
            if task is None:
                break
            print(f"   ✓ Processed: {task.decode()}")

        # LIFO stack
        print("\n2. LIFO Stack (push right, pop right)...")
        await client.list.push_right("stack", b"page-1")
        await client.list.push_right("stack", b"page-2")
        await client.list.push_right("stack", b"page-3")
        print(f"   ✓ Stack length: {await client.list.length('stack')}")

        print("   Popping stack (most recent first):")
        while True:
            page = await client.list.pop_right("stack")
            if page is None:
                break
            print(f"   ✓ Popped: {page.decode()}")

async def main():
    await demo_keyvalue()
    await demo_list()
    print("\n✅ POC 1 Demo Complete!")

if __name__ == "__main__":
    asyncio.run(main())
```

**Acceptance Criteria**:
- [ ] Demo script runs without errors
- [ ] Shows all KeyValue operations
- [ ] Shows all List operations
- [ ] Outputs clear, user-friendly messages
- [ ] Demonstrates TTL expiration

**Task 5.3: Create README and documentation** (half day)
```text
# POC 1: KeyValue with MemStore

Walking skeleton demonstrating Prism's end-to-end architecture.

## Quick Start

### 1. Start MemStore plugin:
    cd plugins/memstore
    go run main.go

### 2. Start Rust proxy:
    cd proxy
    cargo run -- --config config.yaml

### 3. Run demo:
    cd examples
    python poc1-demo.py

## Architecture

[Include component diagram here]

## Running Tests

    pytest tests/poc1/

## What's Implemented

- ✅ KeyValue pattern (Set, Get, Delete, Exists, Scan)
- ✅ List pattern (PushLeft, PushRight, PopLeft, PopRight, Length)
- ✅ TTL expiration
- ✅ Prefix-based scanning
- ✅ gRPC communication (Rust ↔ Go)
- ✅ Python async client library

## What's NOT Implemented

- ❌ Authentication
- ❌ Observability
- ❌ Multi-tenancy
- ❌ Multiple backends
- ❌ Retry logic
```

**Acceptance Criteria**:
- [ ] README.md created
- [ ] Quick start instructions work
- [ ] Architecture diagram included
- [ ] Lists implemented and not-implemented features

### Work Stream 6: Local Development Setup

**Owner**: 1 engineer (DevOps/Infrastructure)
**Duration**: 1 day
**Dependencies**: Work Streams 2, 3, 4 complete (for testing)
**Can run in parallel with**: Work Stream 5

#### Tasks

**Task 6.1: Create Docker Compose setup** (half day)
```yaml
# docker-compose.yml
version: '3.8'

services:
  memstore-plugin:
    build:
      context: ./plugins/memstore
      dockerfile: Dockerfile
    ports:
      - "50051:50051"
    healthcheck:
      test: ["CMD", "grpcurl", "-plaintext", "localhost:50051", "grpc.health.v1.Health/Check"]
      interval: 10s
      timeout: 5s
      retries: 3

  proxy:
    build:
      context: ./proxy
      dockerfile: Dockerfile
    ports:
      - "8980:8980"
    depends_on:
      - memstore-plugin
    volumes:
      - ./proxy/config.yaml:/app/config.yaml:ro
    environment:
      - RUST_LOG=info
    healthcheck:
      test: ["CMD", "grpcurl", "-plaintext", "localhost:8980", "grpc.health.v1.Health/Check"]
      interval: 10s
      timeout: 5s
      retries: 3

  admin-api:
    build:
      context: ./admin
      dockerfile: Dockerfile
    ports:
      - "8090:8090"
    environment:
      - PROXY_CONFIG_PATH=/app/proxy/config.yaml
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
      interval: 10s
      timeout: 5s
      retries: 3

networks:
  default:
    name: prism-poc1
```

**Acceptance Criteria**:
- [ ] `docker-compose up` starts all services
- [ ] Services can communicate
- [ ] Health checks pass
- [ ] `docker-compose down` stops cleanly

**Task 6.2: Create Makefiles for development** (half day)
```makefile
# Makefile (root)
.PHONY: all proto dev-up dev-down test demo clean

all: proto

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	protoc --rust_out=proto/rust/ --grpc-rust_out=proto/rust/ proto/**/*.proto
	protoc --go_out=proto/go/ --go-grpc_out=proto/go/ proto/**/*.proto
	python -m grpc_tools.protoc -I proto/ --python_out=clients/python/ \
		--grpc_python_out=clients/python/ proto/**/*.proto

# Start development environment
dev-up:
	docker-compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@echo "✅ POC 1 environment ready!"
	@echo "   Proxy:      http://localhost:8980"
	@echo "   Admin API:  http://localhost:8090"

# Stop development environment
dev-down:
	docker-compose down

# Run tests
test:
	pytest tests/poc1/ -v

# Run demo
demo:
	python examples/poc1-demo.py

# Clean build artifacts
clean:
	rm -rf proto/rust/*.rs proto/go/*.go clients/python/prism_pb2*.py
	cd proxy && cargo clean
	cd plugins/memstore && go clean
```

**Acceptance Criteria**:
- [ ] `make proto` generates code
- [ ] `make dev-up` starts environment
- [ ] `make test` runs tests
- [ ] `make demo` runs demo
- [ ] `make clean` removes artifacts

## Timeline and Dependencies

### Gantt Chart

```text
Day 1    2    3    4    5    6    7    8    9    10
     │   │   │   │   │   │   │   │   │   │
WS1  ████                                           Protobuf (1 day)
        │
        ├──────────────────────────────────────>
        │
WS2     ████████████████                           Rust Proxy (4 days)
WS3     ████████████                               Go Plugin (3 days)
WS4     ████████                                   Python Client (2 days)
        │
        │                   ████████               Integration Tests (2 days)
        │                   │
        │                   └────────────────>     Demo & Docs
        │
        │                   ████                   Docker Compose (1 day)
```

### Day-by-Day Plan

**Day 1**: Protobuf (WS1 complete)
- Morning: Define KeyValue + List protobuf interfaces
- Afternoon: Generate code for all languages, validate compilation

**Days 2-5**: Core Implementation (WS2, WS3, WS4 in parallel)
- Rust Proxy (WS2): 4 days
- Go Plugin (WS3): 3 days
- Python Client (WS4): 2 days

**Day 6**: Integration Point
- All components ready for integration testing
- Smoke test: Can client talk to proxy talk to plugin?

**Days 7-8**: Integration Tests (WS5)
- Write comprehensive integration tests
- Create demo script
- Documentation

**Days 9-10**: Polish and Docker (WS6)
- Docker Compose setup
- Makefile targets
- CI integration
- Final testing

## Success Criteria

### Functional Requirements

| Requirement | Test | Status |
|-------------|------|--------|
| Client can SET key-value | `test_set_get` | ⬜ |
| Client can GET key-value | `test_set_get` | ⬜ |
| Client can DELETE key | `test_delete` | ⬜ |
| Client can check EXISTS | `test_exists` | ⬜ |
| Client can SCAN with prefix | `test_scan` | ⬜ |
| TTL expiration works | `test_ttl` | ⬜ |
| List FIFO works | `test_list_fifo` | ⬜ |
| List LIFO works | `test_list_stack` | ⬜ |
| Empty list pops return None | `test_list_fifo` | ⬜ |

### Non-Functional Requirements

| Requirement | Target | Status |
|-------------|--------|--------|
| End-to-end latency | &lt;5ms P99 | ⬜ |
| All components start | &lt;10 seconds | ⬜ |
| Graceful shutdown | No errors | ⬜ |
| Test coverage | &gt;80% | ⬜ |

### Deliverables Checklist

- [ ] Protobuf interfaces defined and code generated
- [ ] Rust proxy compiled and running
- [ ] Go MemStore plugin compiled and running
- [ ] Python client library installable
- [ ] Integration tests passing
- [ ] Demo script working
- [ ] Docker Compose setup functional
- [ ] Makefile targets working
- [ ] Documentation complete

## Risk Mitigation

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Rust gRPC learning curve** | Medium | High | Start with minimal example, iterate |
| **Cross-language serialization issues** | Low | High | Use protobuf, test early |
| **Plugin discovery complexity** | Low | Medium | Hard-code path initially |
| **TTL cleanup performance** | Low | Low | Profile if issues arise |
| **Integration test flakiness** | Medium | Medium | Add retries, timeouts |

## Open Questions

1. **Should proxy load plugins dynamically or require restart?**
   - **Proposal**: Require restart for POC 1 (simplicity)
   - **Future**: Hot reload in POC 2+

2. **How to handle plugin crashes?**
   - **Proposal**: Proxy returns error, logs crash (no retry in POC 1)
   - **Future**: Circuit breaker, retries in POC 2+

3. **Should TTL cleanup be background or on-access?**
   - **Proposal**: On-access (simpler, lazy cleanup)
   - **Future**: Background goroutine if performance issue

## Related Documents

- [RFC-018: POC Implementation Strategy](/rfc/rfc-018-poc-implementation-strategy) - Overall POC roadmap
- [RFC-008: Proxy Plugin Architecture](/rfc/rfc-008-proxy-plugin-architecture) - Proxy design
- [RFC-014: Layered Data Access Patterns](/rfc/rfc-014-layered-data-access-patterns) - KeyValue pattern spec
- [MEMO-004: Backend Plugin Implementation Guide](/memos/memo-004-backend-plugin-implementation-guide) - MemStore priority rationale
- [MEMO-006: Backend Interface Decomposition](/memos/memo-006-backend-interface-decomposition-schema-registry) - MemStore interfaces

## Revision History

- 2025-10-09: Initial RFC with detailed work streams for POC 1