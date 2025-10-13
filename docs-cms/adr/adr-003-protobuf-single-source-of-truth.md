---
date: 2025-10-05
deciders: Core Team
doc_uuid: 24c6dbaf-8d7a-4029-a38f-f7f2522de45b
id: adr-003
project_id: prism-data-layer
status: Accepted
tags:
- architecture
- codegen
- dx
- dry
title: Protobuf as Single Source of Truth
---

## Context

In a data gateway system, multiple components need consistent understanding of data models:

- **Proxy**: Routes requests, validates data
- **Backends**: Store and retrieve data
- **Client libraries**: Make requests
- **Admin UI**: Display and manage data
- **Documentation**: Describe APIs

Traditionally, these are defined separately:
- Database schemas (SQL DDL)
- API schemas (OpenAPI/Swagger)
- Client code (hand-written)
- Documentation (hand-written)

This leads to:
- **Drift**: Schemas get out of sync
- **Duplication**: Same model defined 4+ times
- **Errors**: Manual synchronization fails
- **Slow iteration**: Every change requires updating multiple files

**Problem**: How do we maintain consistency across all components while keeping the architecture DRY (Don't Repeat Yourself)?

## Decision

Use **Protocol Buffers (protobuf)** as the single source of truth for all data models, with custom options for Prism-specific metadata. Generate all code, schemas, and configuration from proto definitions.

## Rationale

### Why Protobuf?

1. **Language Agnostic**: Generate code for Rust, Python, JavaScript, TypeScript
2. **Strong Typing**: Catch errors at compile time
3. **Backward Compatible**: Evolve schemas without breaking clients
4. **Compact**: Efficient binary serialization
5. **Extensible**: Custom options for domain-specific metadata
6. **Tooling**: Excellent IDE support, linters, formatters

### Custom Options for Prism

```protobuf
// prism/options.proto
syntax = "proto3";
package prism;

import "google/protobuf/descriptor.proto";

// Message-level options
extend google.protobuf.MessageOptions {
  string access_pattern = 50001;      // read_heavy | write_heavy | append_heavy
  int64 estimated_read_rps = 50002;   // Capacity planning
  int64 estimated_write_rps = 50003;
  string backend = 50004;             // postgres | kafka | nats | sqlite | neptune
  string consistency = 50005;         // strong | eventual | causal
  int32 retention_days = 50006;       // Auto-delete policy
  bool enable_cache = 50007;          // Add caching layer
}

// Field-level options
extend google.protobuf.FieldOptions {
  string index = 50101;               // primary | secondary | partition_key | clustering_key
  string pii = 50102;                 // email | name | ssn | phone | address
  bool encrypt_at_rest = 50103;       // Field-level encryption
  string validation = 50104;          // email | uuid | url | regex:...
  int32 max_length = 50105;           // String length validation
}

// Service-level options (for future gRPC services)
extend google.protobuf.ServiceOptions {
  bool require_auth = 50201;          // All RPCs require auth
  int32 rate_limit_rps = 50202;       // Service-wide rate limit
}

// RPC-level options
extend google.protobuf.MethodOptions {
  bool idempotent = 50301;            // Safe to retry
  int32 timeout_ms = 50302;           // RPC timeout
  string cache_ttl = 50303;           // Cache responses
}
```

### Code Generation Pipeline

proto/*.proto
    │
    ├──> Rust code (prost)
    │    ├── Data structures
    │    ├── gRPC server traits
    │    └── Validation logic
    │
    ├──> Python code (protoc)
    │    ├── Data classes
    │    └── gRPC clients
    │
    ├──> TypeScript code (ts-proto)
    │    ├── Types for admin UI
    │    └── API client
    │
    ├──> SQL schemas
    │    ├── CREATE TABLE statements
    │    ├── Indexes
    │    └── Constraints
    │
    ├──> Kafka schemas
    │    ├── Topic configurations
    │    └── Serialization
    │
    ├──> OpenAPI docs
    │    └── REST API documentation
    │
    └──> Deployment configs
         ├── Capacity specs
         └── Backend routing
```text

### Example: Complete Data Model

```
// user_profile.proto
syntax = "proto3";

package prism.example;

import "prism/options.proto";

message UserProfile {
  option (prism.backend) = "postgres";
  option (prism.consistency) = "strong";
  option (prism.estimated_read_rps) = "5000";
  option (prism.estimated_write_rps) = "500";
  option (prism.enable_cache) = true;

  // Primary key
  string user_id = 1 [
    (prism.index) = "primary",
    (prism.validation) = "uuid"
  ];

  // PII fields
  string email = 2 [
    (prism.pii) = "email",
    (prism.index) = "secondary",
    (prism.validation) = "email"
  ];

  string full_name = 3 [
    (prism.pii) = "name",
    (prism.max_length) = 256
  ];

  // Encrypted field
  string ssn = 4 [
    (prism.pii) = "ssn",
    (prism.encrypt_at_rest) = true
  ];

  // Metadata
  int64 created_at = 5;
  int64 updated_at = 6;

  // Nested message
  ProfileSettings settings = 7;
}

message ProfileSettings {
  bool email_notifications = 1;
  string timezone = 2;
  string language = 3;
}
```text

This **single file** generates:

1. **Rust structs** with validation:
```
#[derive(Clone, PartialEq, Message)]
pub struct UserProfile {
    #[prost(string, tag = "1")]
    pub user_id: String,
    #[prost(string, tag = "2")]
    pub email: String,
    // ... with validation methods
}

impl UserProfile {
    pub fn validate(&self) -> Result<(), ValidationError> {
        validate_uuid(&self.user_id)?;
        validate_email(&self.email)?;
        // ...
    }
}
```text

2. **Postgres schema**:
```
CREATE TABLE user_profile (
    user_id UUID PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    full_name VARCHAR(256),
    ssn_encrypted BYTEA,  -- Encrypted at application layer
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    settings JSONB
);

CREATE INDEX idx_user_profile_email ON user_profile(email);
```text

3. **TypeScript types** for admin UI:
```
export interface UserProfile {
  userId: string;
  email: string;
  fullName: string;
  ssn: string;
  createdAt: number;
  updatedAt: number;
  settings?: ProfileSettings;
}
```text

4. **Deployment config** (auto-generated):
```
name: user-profile
backend: postgres
capacity:
  read_rps: 5000
  write_rps: 500
  estimated_data_size_mb: 1000
cache:
  enabled: true
  ttl_seconds: 300
```text

### Alternatives Considered

1. **OpenAPI/Swagger as Source of Truth**
   - Pros:
     - HTTP-first
     - Good tooling
     - Popular
   - Cons:
     - Doesn't support binary protocols (Kafka, NATS)
     - Weaker typing than protobuf
     - No field-level metadata
   - Rejected because: Doesn't cover all our use cases

2. **SQL DDL as Source of Truth**
   - Pros:
     - Natural for database-first design
     - DBAs comfortable with it
   - Cons:
     - Only works for SQL backends
     - Doesn't describe APIs
     - Poor code generation for clients
   - Rejected because: Too backend-specific

3. **JSON Schema**
   - Pros:
     - Simple
     - Widely understood
     - Works with HTTP APIs
   - Cons:
     - Runtime validation only
     - No compile-time safety
     - Verbose
   - Rejected because: Lack of strong typing

4. **Hand-Written Code**
   - Pros:
     - Full control
     - No code generation complexity
   - Cons:
     - Massive duplication
     - Drift between components
     - Error-prone
   - Rejected because: Doesn't scale

## Consequences

### Positive

- **Single Source of Truth**: One place to change data models
- **Consistency**: All components guaranteed to have same understanding
- **Type Safety**: Compile-time errors across all languages
- **Fast Iteration**: Change proto, regenerate, done
- **Documentation**: Proto files are self-documenting
- **Validation**: Generated validators ensure data integrity
- **Backward Compatibility**: Protobuf's rules prevent breaking changes

### Negative

- **Code Generation Complexity**: Must maintain codegen tooling
  - *Mitigation*: Use existing tools (prost, ts-proto); only customize for Prism options
- **Learning Curve**: Team must learn protobuf
  - *Mitigation*: Good documentation; protobuf is simpler than alternatives
- **Build Step Required**: Can't edit generated code directly
  - *Mitigation*: Fast build times; clear separation of generated vs. hand-written

### Neutral

- **Proto Language Limitations**: Can't express all constraints
  - Use custom options for Prism-specific needs
  - Complex validation logic in hand-written code
- **Version Management**: Proto file changes must be carefully reviewed
  - Enforce backward compatibility checks in CI

## Implementation Notes

### Project Structure

proto/
├── prism/
│   ├── options.proto          # Custom Prism options
│   └── common/
│       ├── types.proto        # Common types (timestamps, UUIDs, etc.)
│       └── errors.proto       # Error definitions
├── examples/
│   ├── user_profile.proto     # Example from above
│   ├── user_events.proto      # Kafka example
│   └── social_graph.proto     # Neptune example
└── BUILD.bazel                # Or build.rs for Rust
```

### Code Generation Tool

```bash
# tooling/codegen/__main__.py

python -m tooling.codegen \
  --proto-path proto \
  --out-rust proxy/src/generated \
  --out-python tooling/generated \
  --out-typescript admin/app/models/generated \
  --out-sql backends/postgres/migrations \
  --out-docs docs/api
```

### CI Integration

```yaml
# .github/workflows/proto.yml
name: Protobuf

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Check backward compatibility
        run: buf breaking --against '.git#branch=main'
      - name: Lint proto files
        run: buf lint
      - name: Generate code
        run: python -m tooling.codegen
      - name: Verify no changes
        run: git diff --exit-code  # Fail if generated code is stale
```

### Migration Strategy

When changing proto definitions:

1. **Additive changes** (new fields): Safe, just regenerate
2. **Renaming fields**: Use `json_name` option for backward compat
3. **Removing fields**: Mark as `reserved` instead
4. **Changing types**: Create new field, migrate data, deprecate old

## References

- [Protocol Buffers Language Guide](https://protobuf.dev/programming-guides/proto3/)
- [Buf Schema Registry](https://buf.build/)
- [prost (Rust protobuf)](https://github.com/tokio-rs/prost)
- [ts-proto (TypeScript)](https://github.com/stephenh/ts-proto)
- ADR-002: Client-Originated Configuration
- ADR-004: Local-First Testing Strategy

## Revision History

- 2025-10-05: Initial draft and acceptance