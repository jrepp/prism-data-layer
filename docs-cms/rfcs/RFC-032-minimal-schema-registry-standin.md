---
title: "RFC-032: Minimal Prism Schema Registry for Local Testing"
author: Platform Team
created: 2025-10-13
updated: 2025-10-13
status: Draft
tags: [testing, schema-registry, local-development, acceptance-testing, interoperability]
id: rfc-032
project_id: prism-data-layer
doc_uuid: 9b8d0a4f-3c26-4e9b-8a7f-5d2b9c0f8e3a
---

## Abstract

This RFC defines a **minimal Prism Schema Registry** as a local stand-in for testing and acceptance tests. It provides a lightweight implementation of the schema registry interface (RFC-030) that:
- Runs locally without external dependencies (no Confluent, no Apicurio)
- Implements core schema registry operations (register, get, list, validate)
- Serves as baseline for acceptance tests across all backend plugins
- Provides interface compatibility with Confluent and AWS Glue schema registries
- Enables fast developer iteration (&lt;100ms startup, in-memory storage)

This is **not a production schema registry** - it's a testing tool for local development and CI/CD.

## Motivation

### The Problem: External Dependencies in Tests

**Current Testing Challenges:**

```bash
# Test requires running Confluent Schema Registry (JVM, 1GB+ memory, 30s startup)
docker-compose up schema-registry kafka zookeeper  # 3 services for one test!

# Integration test:
pytest test_schema_validation.py --schema-registry http://localhost:8081
# ❌ Flaky: Schema registry not ready yet
# ❌ Slow: 30s startup + 5s per test
# ❌ Heavy: 1GB+ memory for registry alone
```

**Problems:**
1. **External Dependency**: Tests can't run without Confluent/Apicurio
2. **Slow Startup**: 30+ seconds before tests can run
3. **Resource Heavy**: 1GB+ memory for JVM-based registry
4. **Flaky Tests**: Race conditions during startup
5. **CI/CD Cost**: Every test run spawns heavy containers

### What We Need: Minimal Local Registry

```bash
# Ideal test experience:
prism-schema-registry --port 8081 &  # <100ms startup, <10MB memory
pytest test_schema_validation.py      # Tests run immediately
```

**Requirements:**
- ✅ In-memory storage (no persistence needed for tests)
- ✅ Rust-based (fast, small footprint)
- ✅ REST + gRPC APIs (compatible with Confluent clients)
- ✅ Schema validation (protobuf, JSON Schema)
- ✅ Compatibility checking (backward, forward, full)
- ❌ NOT for production (no HA, no persistence, no auth)

## Goals

1. **Fast Local Testing**: &lt;100ms startup, in-memory storage
2. **Acceptance Test Baseline**: All plugin tests use same registry
3. **Interface Compatibility**: Drop-in replacement for Confluent Schema Registry REST API
4. **Schema Format Support**: Protobuf, JSON Schema, Avro
5. **Validation Coverage**: Backward/forward/full compatibility checks
6. **Developer Experience**: Single binary, no external dependencies

## Non-Goals

1. **Production Deployment**: Use Confluent/Apicurio for production
2. **Persistence**: In-memory only (tests recreate schemas)
3. **High Availability**: Single instance, no clustering
4. **Authentication**: No auth/authz (local testing only)
5. **Multi-Tenancy**: Single global namespace

## Proposed Solution: Minimal Prism Schema Registry

### Core Architecture

```text
┌────────────────────────────────────────────────────────────┐
│  prism-schema-registry (Rust binary, <10MB)                │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  REST API (Confluent-compatible)                          │
│  ├─ POST /subjects/:subject/versions                      │
│  ├─ GET  /subjects/:subject/versions/:version             │
│  ├─ GET  /subjects/:subject/versions                      │
│  ├─ POST /compatibility/subjects/:subject/versions/:ver   │
│  └─ DELETE /subjects/:subject/versions/:version           │
│                                                            │
│  gRPC API (Prism-native)                                   │
│  ├─ RegisterSchema()                                       │
│  ├─ GetSchema()                                            │
│  ├─ ListSchemas()                                          │
│  └─ CheckCompatibility()                                   │
│                                                            │
│  In-Memory Storage                                         │
│  └─ HashMap<SubjectVersion, Schema>                       │
│                                                            │
│  Schema Validators                                         │
│  ├─ Protobuf (via prost)                                  │
│  ├─ JSON Schema (via jsonschema crate)                    │
│  └─ Avro (via apache-avro)                                │
│                                                            │
│  Compatibility Checker                                     │
│  └─ Backward/Forward/Full validation logic                │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### Confluent Schema Registry REST API Compatibility

**Why Confluent API:** Most widely adopted, rich client library ecosystem

**Core Endpoints (Subset):**

```yaml
# Register new schema version
POST /subjects/{subject}/versions
{
  "schema": "{...protobuf IDL...}",
  "schemaType": "PROTOBUF"
}
→ 200 OK
{
  "id": 1,
  "version": 1
}

# Get schema by version
GET /subjects/{subject}/versions/{version}
→ 200 OK
{
  "id": 1,
  "version": 1,
  "schema": "{...protobuf IDL...}",
  "schemaType": "PROTOBUF"
}

# List all versions for subject
GET /subjects/{subject}/versions
→ 200 OK
[1, 2, 3]

# Check compatibility
POST /compatibility/subjects/{subject}/versions/{version}
{
  "schema": "{...new schema...}",
  "schemaType": "PROTOBUF"
}
→ 200 OK
{
  "is_compatible": true
}

# Delete schema version
DELETE /subjects/{subject}/versions/{version}
→ 200 OK
1
```

**Not Implemented (Out of Scope for Minimal Registry):**
- `/config` endpoints (global/subject compatibility settings)
- `/mode` endpoints (READONLY, READWRITE modes)
- `/schemas/ids/:id` (lookup by global schema ID)
- Advanced compatibility modes (TRANSITIVE, NONE_TRANSITIVE)

### Schema Format Support

**Protobuf (Primary):**

```rust
use prost_reflect::DescriptorPool;

fn validate_protobuf(schema: &str) -> Result<(), ValidationError> {
    // Parse protobuf schema
    let descriptor = DescriptorPool::decode(schema.as_bytes())?;

    // Validate syntax
    for msg in descriptor.all_messages() {
        // Check for required fields (backward compat violation)
        for field in msg.fields() {
            if field.is_required() {
                return Err(ValidationError::RequiredField(field.name()));
            }
        }
    }

    Ok(())
}
```

**JSON Schema (Secondary):**

```rust
use jsonschema::JSONSchema;

fn validate_json_schema(schema: &str) -> Result<(), ValidationError> {
    let schema_json: serde_json::Value = serde_json::from_str(schema)?;
    let compiled = JSONSchema::compile(&schema_json)?;
    Ok(())
}
```

**Avro (Tertiary - Basic Support):**

```rust
use apache_avro::Schema as AvroSchema;

fn validate_avro(schema: &str) -> Result<(), ValidationError> {
    let avro_schema = AvroSchema::parse_str(schema)?;
    Ok(())
}
```

### Compatibility Checking

**Backward Compatibility (Most Common):**

```rust
fn check_backward_compatible(old_schema: &Schema, new_schema: &Schema) -> CompatibilityResult {
    match (old_schema.schema_type, new_schema.schema_type) {
        (SchemaType::Protobuf, SchemaType::Protobuf) => {
            check_protobuf_backward(old_schema, new_schema)
        }
        _ => CompatibilityResult::Incompatible("Type mismatch")
    }
}

fn check_protobuf_backward(old: &Schema, new: &Schema) -> CompatibilityResult {
    let old_desc = parse_protobuf(&old.content)?;
    let new_desc = parse_protobuf(&new.content)?;

    // Check rules:
    // 1. New schema can read old data
    // 2. Can't remove required fields
    // 3. Can't change field types
    // 4. Can add optional fields

    for old_field in old_desc.fields() {
        if let Some(new_field) = new_desc.get_field(old_field.number()) {
            // Field exists in both - check type compatibility
            if old_field.type_name() != new_field.type_name() {
                return CompatibilityResult::Incompatible(
                    format!("Field {} changed type", old_field.name())
                );
            }
        } else {
            // Field removed - check if it was required
            if old_field.is_required() {
                return CompatibilityResult::Incompatible(
                    format!("Required field {} removed", old_field.name())
                );
            }
        }
    }

    CompatibilityResult::Compatible
}
```

### Acceptance Test Integration

**Test Setup (Minimal):**

```go
// Test fixture: Start registry before tests
func TestMain(m *testing.M) {
    // Start minimal registry (in-memory)
    registry := StartMinimalRegistry(&RegistryConfig{
        Port: 8081,
        InMemory: true,
    })
    defer registry.Stop()

    // Wait for ready (<100ms)
    registry.WaitReady(100 * time.Millisecond)

    // Run tests
    exitCode := m.Run()
    os.Exit(exitCode)
}

// Plugin acceptance test
func TestKafkaPluginSchemaValidation(t *testing.T) {
    // Register schema with minimal registry
    schemaID := registerSchema(t, "orders.created", orderCreatedProto)

    // Configure Kafka plugin to use minimal registry
    plugin := NewKafkaPlugin(&KafkaConfig{
        SchemaRegistry: "http://localhost:8081",
    })

    // Test: Publish with schema validation
    err := plugin.Publish(ctx, "orders.created", orderBytes, map[string]string{
        "schema-id": fmt.Sprint(schemaID),
    })
    require.NoError(t, err)

    // Test: Schema compatibility check
    incompatibleSchema := modifySchema(orderCreatedProto, RemoveRequiredField("order_id"))
    compat := checkCompatibility(t, "orders.created", incompatibleSchema)
    assert.False(t, compat.IsCompatible)
}
```

**Parallel Test Execution:**

```go
// Each test gets isolated registry instance (fast startup)
func TestPlugins(t *testing.T) {
    tests := []struct{
        name string
        plugin Plugin
    }{
        {"Kafka", NewKafkaPlugin()},
        {"NATS", NewNATSPlugin()},
        {"Redis", NewRedisPlugin()},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // Tests run concurrently

            // Each test gets own registry on random port
            registry := StartMinimalRegistry(&RegistryConfig{
                Port: 0,  // Random port
                InMemory: true,
            })
            defer registry.Stop()

            // Configure plugin with test registry
            tt.plugin.SetSchemaRegistry(registry.URL())

            // Test plugin schema validation
            testPluginSchemaValidation(t, tt.plugin, registry)
        })
    }
}
```

### Interface Coverage: Confluent vs AWS Glue vs Apicurio

| Feature | Confluent SR | AWS Glue SR | Apicurio | Prism Minimal | Priority |
|---------|-------------|-------------|----------|---------------|----------|
| **Register Schema** | ✅ | ✅ | ✅ | ✅ | HIGH |
| **Get Schema** | ✅ | ✅ | ✅ | ✅ | HIGH |
| **List Versions** | ✅ | ✅ | ✅ | ✅ | HIGH |
| **Delete Schema** | ✅ | ✅ | ✅ | ✅ | MEDIUM |
| **Compatibility Check** | ✅ | ✅ | ✅ | ✅ | HIGH |
| **Subject-based Versioning** | ✅ | ❌ | ✅ | ✅ | HIGH |
| **Global Config** | ✅ | ✅ | ✅ | ❌ | LOW |
| **READONLY Mode** | ✅ | ❌ | ✅ | ❌ | LOW |
| **Schema References** | ✅ | ❌ | ✅ | ⚠️  | MEDIUM |
| **Protobuf Support** | ✅ | ✅ | ✅ | ✅ | HIGH |
| **JSON Schema** | ✅ | ✅ | ✅ | ✅ | HIGH |
| **Avro** | ✅ | ✅ | ✅ | ⚠️  | MEDIUM |
| **High Availability** | ✅ | ✅ | ✅ | ❌ | N/A (testing) |
| **Authentication** | ✅ | ✅ | ✅ | ❌ | N/A (local) |
| **Persistence** | ✅ | ✅ | ✅ | ❌ | N/A (in-memory) |

**Legend:**
- ✅ Fully supported
- ⚠️ Partial support (basic functionality only)
- ❌ Not supported (out of scope)

**Coverage Target:** 80% of Confluent REST API for core operations

### Use Cases for Minimal Schema Registry

**1. Local Development**

```bash
# Developer workflow: Start registry in background
prism-schema-registry --port 8081 &

# Develop against local registry
prism schema register --file orders.proto --subject orders.created
prism schema validate --file orders_v2.proto --subject orders.created --check backward

# Run application locally
my-app --schema-registry http://localhost:8081
```

**2. CI/CD Pipeline**

```yaml
# GitHub Actions
jobs:
  acceptance-tests:
    runs-on: ubuntu-latest
    steps:
      - name: Start minimal schema registry
        run: |
          prism-schema-registry --port 8081 &
          sleep 0.1  # Registry ready in <100ms

      - name: Run acceptance tests
        run: make test-acceptance
        env:
          SCHEMA_REGISTRY_URL: http://localhost:8081
```

**3. Plugin Development**

```go
// New backend plugin development
func TestNewPluginSchemaIntegration(t *testing.T) {
    // Use minimal registry as acceptance test baseline
    registry := test.StartMinimalRegistry(t)

    // Register test schema
    schemaID := registry.RegisterSchema("test.topic", testSchema)

    // Test plugin implements schema validation
    plugin := NewMyPlugin(registry.URL())
    err := plugin.ValidateSchema(context.Background(), schemaID)
    assert.NoError(t, err)
}
```

**4. Schema Evolution Testing**

```python
# Test schema compatibility before deploying to production
def test_schema_evolution():
    registry = MinimalSchemaRegistry()

    # Register v1 schema
    v1_id = registry.register("users", user_v1_schema)

    # Test v2 compatibility
    compat = registry.check_compatibility("users", user_v2_schema)
    assert compat.is_compatible, f"Breaking changes: {compat.errors}"

    # Safe to deploy v2
    v2_id = registry.register("users", user_v2_schema)
```

## Implementation Plan

### Phase 1: Core Registry (Week 1)

**Deliverables:**
- ✅ Rust binary with in-memory storage
- ✅ Confluent REST API (register, get, list)
- ✅ Protobuf validation
- ✅ Basic compatibility checking

### Phase 2: Extended Compatibility (Week 2)

**Deliverables:**
- ✅ JSON Schema support
- ✅ Avro support (basic)
- ✅ Forward/full compatibility modes
- ✅ Delete schema endpoint

### Phase 3: Acceptance Test Integration (Week 3)

**Deliverables:**
- ✅ Go test helper library
- ✅ All plugin acceptance tests use minimal registry
- ✅ Parallel test support (isolated registries)

### Phase 4: Developer Experience (Week 4)

**Deliverables:**
- ✅ CLI wrapper (`prism schema-registry start`)
- ✅ Docker image (distroless, &lt;10MB)
- ✅ Documentation + examples

## Success Criteria

1. **Startup Time**: &lt;100ms cold start
2. **Memory Footprint**: &lt;10MB for registry + 100 schemas
3. **Test Performance**: Acceptance tests 50%+ faster than with Confluent
4. **API Compatibility**: 80%+ of Confluent REST API endpoints supported
5. **Developer Adoption**: 100% of new plugin tests use minimal registry

## References

- **RFC-030**: Schema Evolution and Validation (schema registry requirements)
- **RFC-031**: Message Envelope Protocol (schema context integration)
- [Confluent Schema Registry API](https://docs.confluent.io/platform/current/schema-registry/develop/api.html)
- [AWS Glue Schema Registry](https://docs.aws.amazon.com/glue/latest/dg/schema-registry.html)
- [Apicurio Registry](https://www.apicur.io/registry/)

## Revision History

- 2025-10-13 (v1): Initial draft - Minimal schema registry for local testing and acceptance tests
