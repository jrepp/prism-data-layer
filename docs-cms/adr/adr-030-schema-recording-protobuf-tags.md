---
date: 2025-10-08
deciders: Core Team
doc_uuid: 544db4ef-3d6f-4ca3-b651-97241748b9fd
id: adr-030
project_id: prism-data-layer
status: Accepted
tags:
- protobuf
- schema
- versioning
- evolution
- registry
title: Schema Recording with Protobuf Tagging
---

## Context

Prism uses protobuf for all data models and client configurations. Need to:
- Track schema evolution over time
- Validate compatibility during deployments
- Provide schema discovery for clients
- Audit schema changes
- Enable schema-aware tooling

**Requirements:**
- Record schema deployments automatically
- Detect breaking changes
- Query schema history
- Generate migration scripts
- Support schema branching (dev/staging/prod)

## Decision

Use **Protobuf custom options for schema metadata tagging**:

1. **Custom option `(prism.schema)`**: Tag messages with schema metadata
2. **Schema versioning**: Semantic versioning with compatibility rules
3. **Schema registry**: Centralized storage for all deployed schemas
4. **Compatibility checking**: Forward, backward, full compatibility modes
5. **Migration tracking**: Link schemas to database migrations

## Rationale

### Why Custom Protobuf Options

Protobuf options allow **declarative schema metadata**:
- Version controlled alongside code
- Type-safe annotations
- Code generation aware
- Centralized schema policy
- No runtime overhead

### Schema Option Definition

```protobuf
// proto/prism/options.proto
syntax = "proto3";

package prism;

import "google/protobuf/descriptor.proto";

// Schema metadata options
extend google.protobuf.MessageOptions {
  SchemaOptions schema = 50101;
}

extend google.protobuf.FieldOptions {
  FieldSchemaOptions field_schema = 50102;
}

message SchemaOptions {
  // Schema version (semantic versioning)
  string version = 1;

  // Schema category
  string category = 2;  // "entity", "event", "config", "command"

  // Compatibility mode
  CompatibilityMode compatibility = 3;

  // Storage backend
  string backend = 4;  // "postgres", "kafka", "nats", "neptune"

  // Enable schema evolution tracking
  bool track_evolution = 5 [default = true];

  // Migration script reference
  optional string migration = 6;

  // Schema owner/team
  string owner = 7;

  // Tags for discovery
  repeated string tags = 8;

  // Deprecation notice
  optional DeprecationInfo deprecation = 9;
}

enum CompatibilityMode {
  COMPATIBILITY_MODE_UNSPECIFIED = 0;
  COMPATIBILITY_MODE_NONE = 1;         // No compatibility checks
  COMPATIBILITY_MODE_BACKWARD = 2;     // New schema can read old data
  COMPATIBILITY_MODE_FORWARD = 3;      // Old schema can read new data
  COMPATIBILITY_MODE_FULL = 4;         // Both backward and forward
}

message DeprecationInfo {
  string reason = 1;
  string deprecated_at = 2;  // ISO 8601 date
  string removed_at = 3;     // Planned removal date
  string replacement = 4;    // Replacement schema name
}

message FieldSchemaOptions {
  // Field-level indexing hint
  IndexType index = 1;

  // PII classification
  PIIType pii = 2;

  // Required for creation
  bool required_for_create = 3;

  // Immutable after creation
  bool immutable = 4;

  // Encryption at rest
  bool encrypted = 5;

  // Default value generation
  optional string default_generator = 6;  // "uuid", "timestamp", "sequence"
}

enum IndexType {
  INDEX_TYPE_UNSPECIFIED = 0;
  INDEX_TYPE_NONE = 1;
  INDEX_TYPE_PRIMARY = 2;
  INDEX_TYPE_SECONDARY = 3;
  INDEX_TYPE_UNIQUE = 4;
  INDEX_TYPE_FULLTEXT = 5;
}

enum PIIType {
  PII_TYPE_UNSPECIFIED = 0;
  PII_TYPE_NONE = 1;
  PII_TYPE_EMAIL = 2;
  PII_TYPE_PHONE = 3;
  PII_TYPE_NAME = 4;
  PII_TYPE_ADDRESS = 5;
  PII_TYPE_SSN = 6;
  PII_TYPE_CREDIT_CARD = 7;
}
```

### Tagged Schema Examples

**Entity Schema:**
```protobuf
// proto/prism/data/v1/user.proto
import "prism/options.proto";

message UserProfile {
  option (prism.schema) = {
    version: "1.2.0"
    category: "entity"
    compatibility: COMPATIBILITY_MODE_BACKWARD
    backend: "postgres"
    migration: "migrations/002_add_user_verified.sql"
    owner: "identity-team"
    tags: ["user", "identity", "core"]
  };

  string user_id = 1 [
    (prism.field_schema) = {
      index: INDEX_TYPE_PRIMARY
      required_for_create: true
      immutable: true
      default_generator: "uuid"
    }
  ];

  string email = 2 [
    (prism.field_schema) = {
      index: INDEX_TYPE_UNIQUE
      pii: PII_TYPE_EMAIL
      encrypted: true
      required_for_create: true
    }
  ];

  string name = 3 [
    (prism.field_schema) = {
      pii: PII_TYPE_NAME
    }
  ];

  bool verified = 4 [
    (prism.field_schema) = {
      required_for_create: false
    }
  ];  // Added in v1.2.0

  int64 created_at = 5 [
    (prism.field_schema) = {
      index: INDEX_TYPE_SECONDARY
      immutable: true
      default_generator: "timestamp"
    }
  ];

  int64 updated_at = 6 [
    (prism.field_schema) = {
      default_generator: "timestamp"
    }
  ];
}
```

**Event Schema:**
```protobuf
// proto/prism/events/v1/user_events.proto
message UserCreatedEvent {
  option (prism.schema) = {
    version: "1.0.0"
    category: "event"
    compatibility: COMPATIBILITY_MODE_FORWARD
    backend: "kafka"
    owner: "identity-team"
    tags: ["event", "user", "lifecycle"]
  };

  string event_id = 1 [
    (prism.field_schema) = {
      index: INDEX_TYPE_PRIMARY
      default_generator: "uuid"
    }
  ];

  string user_id = 2 [
    (prism.field_schema) = {
      index: INDEX_TYPE_SECONDARY
      required_for_create: true
    }
  ];

  int64 timestamp = 3 [
    (prism.field_schema) = {
      index: INDEX_TYPE_SECONDARY
      default_generator: "timestamp"
    }
  ];

  UserProfile user_data = 4;
}
```

**Deprecated Schema:**
```protobuf
message UserProfileV1 {
  option (prism.schema) = {
    version: "1.0.0"
    category: "entity"
    backend: "postgres"
    owner: "identity-team"
    deprecation: {
      reason: "Replaced by UserProfile with email verification"
      deprecated_at: "2025-09-01"
      removed_at: "2026-01-01"
      replacement: "prism.data.v1.UserProfile"
    }
  };

  string user_id = 1;
  string email = 2;
  string name = 3;
}
```

### Schema Registry

**Schema Registry Service:**
```protobuf
// proto/prism/schema/v1/registry.proto
syntax = "proto3";

package prism.schema.v1;

import "google/protobuf/descriptor.proto";
import "google/protobuf/timestamp.proto";

service SchemaRegistry {
  // Register new schema version
  rpc RegisterSchema(RegisterSchemaRequest) returns (RegisterSchemaResponse);

  // Get schema by name and version
  rpc GetSchema(GetSchemaRequest) returns (GetSchemaResponse);

  // List all schemas
  rpc ListSchemas(ListSchemasRequest) returns (ListSchemasResponse);

  // Check compatibility
  rpc CheckCompatibility(CheckCompatibilityRequest) returns (CheckCompatibilityResponse);

  // Get schema evolution history
  rpc GetSchemaHistory(GetSchemaHistoryRequest) returns (stream SchemaVersion);

  // Search schemas by tags
  rpc SearchSchemas(SearchSchemasRequest) returns (SearchSchemasResponse);
}

message RegisterSchemaRequest {
  string name = 1;
  string version = 2;
  google.protobuf.FileDescriptorSet descriptor_set = 3;
  string environment = 4;  // "dev", "staging", "production"
  map<string, string> metadata = 5;
}

message RegisterSchemaResponse {
  string schema_id = 1;
  google.protobuf.Timestamp registered_at = 2;
  CompatibilityResult compatibility = 3;
}

message GetSchemaRequest {
  string name = 1;
  optional string version = 2;  // If not specified, get latest
  optional string environment = 3;
}

message GetSchemaResponse {
  SchemaVersion schema = 1;
}

message ListSchemasRequest {
  optional string category = 1;
  optional string backend = 2;
  optional string owner = 3;
  int32 page_size = 4;
  optional string page_token = 5;
}

message ListSchemasResponse {
  repeated SchemaInfo schemas = 1;
  optional string next_page_token = 2;
}

message SchemaInfo {
  string name = 1;
  string current_version = 2;
  string category = 3;
  string backend = 4;
  string owner = 5;
  repeated string tags = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
  int32 version_count = 9;
  optional DeprecationInfo deprecation = 10;
}

message SchemaVersion {
  string schema_id = 1;
  string name = 2;
  string version = 3;
  google.protobuf.FileDescriptorSet descriptor_set = 4;
  google.protobuf.Timestamp registered_at = 5;
  string environment = 6;
  map<string, string> metadata = 7;
  CompatibilityMode compatibility_mode = 8;
}

message CheckCompatibilityRequest {
  string name = 1;
  string new_version = 2;
  google.protobuf.FileDescriptorSet new_descriptor_set = 3;
  optional string compare_version = 4;  // If not specified, compare with latest
}

message CheckCompatibilityResponse {
  bool compatible = 1;
  CompatibilityResult result = 2;
}

message CompatibilityResult {
  bool backward_compatible = 1;
  bool forward_compatible = 2;
  repeated string breaking_changes = 3;
  repeated string warnings = 4;
}

message GetSchemaHistoryRequest {
  string name = 1;
  optional string environment = 2;
}

message SearchSchemasRequest {
  repeated string tags = 1;
  optional string category = 2;
  optional string owner = 3;
}

message SearchSchemasResponse {
  repeated SchemaInfo schemas = 1;
}
```

### Schema Registry Implementation

**Registry Storage:**
```rust
// proxy/src/schema/registry.rs
use prost::Message;
use prost_types::FileDescriptorSet;

#[async_trait]
pub trait SchemaRegistry: Send + Sync {
    async fn register(&self, req: RegisterSchemaRequest) -> Result<RegisterSchemaResponse>;
    async fn get(&self, name: &str, version: Option<&str>) -> Result<SchemaVersion>;
    async fn list(&self, filter: SchemaFilter) -> Result<Vec<SchemaInfo>>;
    async fn check_compatibility(&self, req: CheckCompatibilityRequest) -> Result<CompatibilityResult>;
    async fn get_history(&self, name: &str) -> Result<Vec<SchemaVersion>>;
}

pub struct PostgresSchemaRegistry {
    pool: PgPool,
}

impl SchemaRegistry for PostgresSchemaRegistry {
    async fn register(&self, req: RegisterSchemaRequest) -> Result<RegisterSchemaResponse> {
        // Check compatibility with existing schemas
        let compatibility = if let Ok(existing) = self.get(&req.name, None).await {
            self.check_compatibility(CheckCompatibilityRequest {
                name: req.name.clone(),
                new_version: req.version.clone(),
                new_descriptor_set: req.descriptor_set.clone(),
                compare_version: Some(existing.version),
            }).await?
        } else {
            CompatibilityResult::default()
        };

        // Serialize descriptor set
        let descriptor_bytes = req.descriptor_set.encode_to_vec();

        // Store schema
        let schema_id = sqlx::query_scalar::<_, String>(
            r#"
            INSERT INTO schemas
            (name, version, descriptor_set, environment, metadata, registered_at)
            VALUES ($1, $2, $3, $4, $5, NOW())
            RETURNING id
            "#
        )
        .bind(&req.name)
        .bind(&req.version)
        .bind(&descriptor_bytes)
        .bind(&req.environment)
        .bind(&req.metadata)
        .fetch_one(&self.pool)
        .await?;

        Ok(RegisterSchemaResponse {
            schema_id,
            registered_at: Utc::now(),
            compatibility,
        })
    }

    async fn get(&self, name: &str, version: Option<&str>) -> Result<SchemaVersion> {
        let row = if let Some(ver) = version {
            sqlx::query_as::<_, SchemaRow>(
                "SELECT * FROM schemas WHERE name = $1 AND version = $2"
            )
            .bind(name)
            .bind(ver)
            .fetch_one(&self.pool)
            .await?
        } else {
            sqlx::query_as::<_, SchemaRow>(
                "SELECT * FROM schemas WHERE name = $1 ORDER BY registered_at DESC LIMIT 1"
            )
            .bind(name)
            .fetch_one(&self.pool)
            .await?
        };

        Ok(row.into_schema_version()?)
    }
}
```

**Compatibility Checker:**
```rust
// proxy/src/schema/compatibility.rs
use prost_types::FileDescriptorSet;

pub struct CompatibilityChecker;

impl CompatibilityChecker {
    pub fn check(
        old_set: &FileDescriptorSet,
        new_set: &FileDescriptorSet,
        mode: CompatibilityMode,
    ) -> CompatibilityResult {
        let mut result = CompatibilityResult {
            backward_compatible: true,
            forward_compatible: true,
            breaking_changes: vec![],
            warnings: vec![],
        };

        // Extract message descriptors
        let old_messages = Self::extract_messages(old_set);
        let new_messages = Self::extract_messages(new_set);

        // Check backward compatibility (new schema can read old data)
        if matches!(mode, CompatibilityMode::Backward | CompatibilityMode::Full) {
            for (name, old_msg) in &old_messages {
                if let Some(new_msg) = new_messages.get(name) {
                    // Check for removed fields
                    for old_field in &old_msg.field {
                        if !new_msg.field.iter().any(|f| f.number == old_field.number) {
                            result.backward_compatible = false;
                            result.breaking_changes.push(format!(
                                "Field {} removed from {}",
                                old_field.name, name
                            ));
                        }
                    }

                    // Check for type changes
                    for old_field in &old_msg.field {
                        if let Some(new_field) = new_msg.field.iter().find(|f| f.number == old_field.number) {
                            if old_field.r#type != new_field.r#type {
                                result.backward_compatible = false;
                                result.breaking_changes.push(format!(
                                    "Field {}.{} type changed",
                                    name, old_field.name
                                ));
                            }
                        }
                    }
                } else {
                    result.backward_compatible = false;
                    result.breaking_changes.push(format!("Message {} removed", name));
                }
            }
        }

        // Check forward compatibility (old schema can read new data)
        if matches!(mode, CompatibilityMode::Forward | CompatibilityMode::Full) {
            for (name, new_msg) in &new_messages {
                if let Some(old_msg) = old_messages.get(name) {
                    // Check for new required fields
                    for new_field in &new_msg.field {
                        if !old_msg.field.iter().any(|f| f.number == new_field.number) {
                            // New field should be optional for forward compatibility
                            if !Self::is_optional(new_field) {
                                result.forward_compatible = false;
                                result.breaking_changes.push(format!(
                                    "Required field {} added to {}",
                                    new_field.name, name
                                ));
                            }
                        }
                    }
                }
            }
        }

        result
    }

    fn extract_messages(descriptor_set: &FileDescriptorSet) -> HashMap<String, MessageDescriptor> {
        // Extract all message descriptors from FileDescriptorSet
        // ...implementation details...
        HashMap::new()
    }

    fn is_optional(field: &FieldDescriptor) -> bool {
        // Check if field is optional (proto3 optional keyword or non-required)
        true
    }
}
```

### Build-time Schema Registration

**Automatic registration during build:**
```rust
// build.rs
fn main() {
    // Compile protobuf files
    let descriptor_set_path = std::env::var("OUT_DIR").unwrap() + "/descriptor_set.bin";

    prost_build::Config::new()
        .file_descriptor_set_path(&descriptor_set_path)
        .compile_protos(&["proto/prism/data/v1/user.proto"], &["proto/"])
        .unwrap();

    // Register schema with registry (if SCHEMA_REGISTRY_ENDPOINT is set)
    if let Ok(registry_endpoint) = std::env::var("SCHEMA_REGISTRY_ENDPOINT") {
        register_schema_from_descriptor(&descriptor_set_path, &registry_endpoint);
    }
}

fn register_schema_from_descriptor(path: &str, endpoint: &str) {
    // Read descriptor set
    let bytes = std::fs::read(path).unwrap();
    let descriptor_set = FileDescriptorSet::decode(&*bytes).unwrap();

    // Extract schema metadata from custom options
    let schema_info = extract_schema_metadata(&descriptor_set);

    // Register with registry
    let client = SchemaRegistryClient::connect(endpoint).unwrap();
    client.register_schema(RegisterSchemaRequest {
        name: schema_info.name,
        version: schema_info.version,
        descriptor_set,
        environment: std::env::var("ENVIRONMENT").unwrap_or("dev".to_string()),
        metadata: HashMap::new(),
    }).await.unwrap();
}
```

### Database Schema

```sql
CREATE TABLE schemas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    descriptor_set BYTEA NOT NULL,
    environment TEXT NOT NULL,
    metadata JSONB,
    registered_at TIMESTAMPTZ NOT NULL,

    -- Indexes
    UNIQUE(name, version, environment),
    INDEX idx_schemas_name ON schemas(name),
    INDEX idx_schemas_env ON schemas(environment),
    INDEX idx_schemas_registered ON schemas(registered_at DESC)
);

-- Schema metadata extracted from protobuf options
CREATE TABLE schema_metadata (
    schema_id UUID REFERENCES schemas(id) ON DELETE CASCADE,
    category TEXT,
    compatibility_mode TEXT,
    backend TEXT,
    owner TEXT,
    tags TEXT[],
    migration TEXT,
    deprecated BOOLEAN DEFAULT FALSE,
    deprecation_info JSONB,

    PRIMARY KEY (schema_id),
    INDEX idx_schema_category ON schema_metadata(category),
    INDEX idx_schema_backend ON schema_metadata(backend),
    INDEX idx_schema_tags ON schema_metadata USING GIN(tags)
);

-- Field-level metadata
CREATE TABLE field_metadata (
    schema_id UUID REFERENCES schemas(id) ON DELETE CASCADE,
    field_number INT NOT NULL,
    field_name TEXT NOT NULL,
    index_type TEXT,
    pii_type TEXT,
    required_for_create BOOLEAN,
    immutable BOOLEAN,
    encrypted BOOLEAN,
    default_generator TEXT,

    PRIMARY KEY (schema_id, field_number),
    INDEX idx_field_pii ON field_metadata(pii_type) WHERE pii_type IS NOT NULL
);
```

### CLI Integration

```bash
# Register schema manually
prism-admin schema register \
  --proto proto/prism/data/v1/user.proto \
  --version 1.2.0 \
  --environment production

# Check compatibility
prism-admin schema check \
  --proto proto/prism/data/v1/user.proto \
  --against 1.1.0

# List schemas
prism-admin schema list --category entity --backend postgres

# Get schema history
prism-admin schema history prism.data.v1.UserProfile

# Search by tags
prism-admin schema search --tags user,identity

# Generate migration
prism-admin schema migrate \
  --from prism.data.v1.UserProfile:1.1.0 \
  --to prism.data.v1.UserProfile:1.2.0 \
  --output migrations/
```

### Migration Generation

**Automatic migration script generation:**
```rust
// proxy/src/schema/migration.rs
pub struct MigrationGenerator;

impl MigrationGenerator {
    pub fn generate(
        old_schema: &SchemaVersion,
        new_schema: &SchemaVersion,
        backend: &str,
    ) -> Result<String> {
        let old_messages = extract_messages(&old_schema.descriptor_set);
        let new_messages = extract_messages(&new_schema.descriptor_set);

        match backend {
            "postgres" => Self::generate_postgres_migration(&old_messages, &new_messages),
            "kafka" => Self::generate_kafka_migration(&old_messages, &new_messages),
            _ => Err(anyhow!("Unsupported backend: {}", backend)),
        }
    }

    fn generate_postgres_migration(
        old: &HashMap<String, MessageDescriptor>,
        new: &HashMap<String, MessageDescriptor>,
    ) -> Result<String> {
        let mut sql = String::new();

        for (name, new_msg) in new {
            if let Some(old_msg) = old.get(name) {
                // Generate ALTER TABLE for changes
                let table_name = to_snake_case(name);

                for new_field in &new_msg.field {
                    if !old_msg.field.iter().any(|f| f.number == new_field.number) {
                        // New field added
                        let col_type = proto_to_sql_type(new_field);
                        sql.push_str(&format!(
                            "ALTER TABLE {} ADD COLUMN {} {};\n",
                            table_name,
                            to_snake_case(&new_field.name),
                            col_type
                        ));

                        // Add index if specified
                        if let Some(index_type) = get_field_option(new_field, "index") {
                            if index_type != "INDEX_TYPE_NONE" {
                                sql.push_str(&format!(
                                    "CREATE INDEX idx_{}_{} ON {}({});\n",
                                    table_name,
                                    to_snake_case(&new_field.name),
                                    table_name,
                                    to_snake_case(&new_field.name)
                                ));
                            }
                        }
                    }
                }
            } else {
                // New table
                sql.push_str(&Self::generate_create_table(name, new_msg));
            }
        }

        Ok(sql)
    }

    fn generate_create_table(name: &str, msg: &MessageDescriptor) -> String {
        let table_name = to_snake_case(name);
        let mut columns = vec![];

        for field in &msg.field {
            let col_name = to_snake_case(&field.name);
            let col_type = proto_to_sql_type(field);
            columns.push(format!("  {} {}", col_name, col_type));
        }

        format!(
            "CREATE TABLE {} (\n{}\n);\n",
            table_name,
            columns.join(",\n")
        )
    }
}
```

### Alternatives Considered

1. **Schema-less / dynamic schemas**
   - Pros: Flexible, no registration needed
   - Cons: No type safety, no compatibility checking, runtime errors
   - Rejected: Type safety is critical for reliability

2. **Manual schema versioning**
   - Pros: Simple, developer-controlled
   - Cons: Error-prone, no automated checks, no discovery
   - Rejected: Need automated compatibility checking

3. **Separate schema registry (Confluent Schema Registry)**
   - Pros: Battle-tested, Kafka ecosystem standard
   - Cons: External dependency, Kafka-centric, limited protobuf support
   - Deferred: May integrate for Kafka backends specifically

## Consequences

### Positive

- **Type safety**: Schemas validated at build time
- **Automated compatibility**: Breaking changes caught early
- **Centralized discovery**: All schemas queryable
- **Migration support**: Automated script generation
- **Audit trail**: Complete schema evolution history
- **PII tracking**: Field-level PII metadata

### Negative

- **Build complexity**: Schema registration in build process
- **Registry dependency**: Central service required
- **Storage overhead**: Descriptor sets stored for each version

### Neutral

- **Learning curve**: Developers must understand compatibility modes
- **Versioning discipline**: Teams must follow semantic versioning

## Implementation Notes

### Code Generation

Extract schema options during build:
```rust
// build.rs
fn extract_schema_metadata(descriptor_set: &FileDescriptorSet) -> SchemaMetadata {
    // Parse custom options from descriptor
    // Generate Rust code for schema info
}
```

### Integration with Admin API

Schema registry accessible via Admin API (ADR-027):
```protobuf
service AdminService {
  // Existing admin operations...

  // Schema operations
  rpc RegisterSchema(RegisterSchemaRequest) returns (RegisterSchemaResponse);
  rpc ListSchemas(ListSchemasRequest) returns (ListSchemasResponse);
  rpc CheckCompatibility(CheckCompatibilityRequest) returns (CheckCompatibilityResponse);
}
```

## References

- [Protobuf Options](https://protobuf.dev/programming-guides/proto3/#options)
- [Confluent Schema Registry](https://docs.confluent.io/platform/current/schema-registry/index.html)
- ADR-003: Protobuf as Single Source of Truth
- ADR-027: Admin API via gRPC
- ADR-029: Protocol Recording with Protobuf Tagging

## Revision History

- 2025-10-08: Initial draft and acceptance