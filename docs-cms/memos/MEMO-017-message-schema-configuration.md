---
author: Claude
created: 2025-10-11
doc_uuid: 15828b58-345b-4995-bec7-f73476cde62e
id: memo-017
project_id: prism-data-layer
tags:
- messaging
- schema
- validation
- multicast-registry
title: 'MEMO-017: Message Schema Configuration for Publish Slots'
updated: 2025-10-11
---

# MEMO-017: Message Schema Configuration for Publish Slots

## Context

When using the multicast registry pattern (or any pub/sub messaging pattern), **consumers need to know what message format to expect** from published messages. Without schema information, consumers must:
- Reverse-engineer message structure from examples
- Handle unexpected formats with generic error handling
- Maintain separate documentation outside the configuration

**User requirement:** "for publish slots i want to expose a setting which is the message schema for consumers"

## Proposal

Add `message_schema` configuration to messaging backend slots, supporting multiple schema formats.

### Configuration Example

```yaml
pattern: multicast-registry
name: device-notifications

slots:
  registry:
    backend: redis
    config:
      addr: "localhost:6379"

  messaging:
    backend: nats
    config:
      servers: ["nats://localhost:4222"]
      topic_prefix: "devices."

    # NEW: Message schema specification
    message_schema:
      format: "protobuf"  # or "json-schema", "avro", "plaintext"
      schema_ref: "prism.devices.v1.DeviceEvent"
      schema_url: "https://schemas.prism.internal/devices/v1/event.proto"
      validation: "strict"  # or "advisory", "none"

      # Optional: inline schema for simple cases
      inline_schema: |
        syntax = "proto3";
        message DeviceEvent {
          string device_id = 1;
          string event_type = 2;
          int64 timestamp = 3;
          bytes payload = 4;
        }
```

### Schema Format Support

#### 1. **Protobuf** (Recommended)
- **Format**: `protobuf`
- **Ref**: Fully-qualified message name (e.g., `prism.devices.v1.DeviceEvent`)
- **URL**: Link to `.proto` file in schema registry
- **Validation**: Proxy validates messages before publishing
- **Benefits**: Type safety, backward compatibility, code generation

```yaml
message_schema:
  format: protobuf
  schema_ref: prism.devices.v1.DeviceEvent
  schema_url: https://schemas.prism.internal/devices/v1/event.proto
  validation: strict
```

#### 2. **JSON Schema**
- **Format**: `json-schema`
- **Ref**: Schema ID in registry (e.g., `device-event-v1`)
- **URL**: Link to JSON Schema file
- **Validation**: JSON structure validation before publishing

```yaml
message_schema:
  format: json-schema
  schema_ref: device-event-v1
  schema_url: https://schemas.prism.internal/devices/v1/event.json
  validation: strict
```

#### 3. **Avro**
- **Format**: `avro`
- **Ref**: Avro schema name with namespace
- **URL**: Link to `.avsc` file
- **Validation**: Avro binary format validation

```yaml
message_schema:
  format: avro
  schema_ref: com.prism.devices.DeviceEvent
  schema_url: https://schemas.prism.internal/devices/v1/event.avsc
  validation: strict
```

#### 4. **Plaintext/Binary** (No Schema)
- **Format**: `plaintext` or `binary`
- **No validation**, consumers handle parsing
- Useful for opaque payloads (encrypted, custom formats)

```yaml
message_schema:
  format: plaintext
  validation: none
  description: "UTF-8 encoded JSON (consumer-parsed)"
```

### Validation Modes

| Mode | Behavior | Use Case |
|------|----------|----------|
| `strict` | Reject invalid messages, return error to publisher | Production environments |
| `advisory` | Log warnings but allow invalid messages through | Migration/testing |
| `none` | No validation, schema is documentation only | Opaque/encrypted payloads |

### Implementation Phases

#### Phase 1: Configuration Only (Week 2)
- Add `message_schema` field to pattern configuration YAML
- Store schema metadata in pattern registry
- Expose schema info via admin API (`GET /api/patterns/{name}/schema`)
- **No validation yet** - schema is documentation only

#### Phase 2: Schema Registry Integration (Week 4)
- Integrate with schema registry (e.g., Confluent Schema Registry, Buf Schema Registry)
- Fetch schemas from registry by URL/ref
- Cache schemas in proxy memory
- Version schema evolution rules

#### Phase 3: Runtime Validation (Week 6)
- Validate messages against schema before publishing
- Return structured errors for schema violations
- Metrics: `prism_schema_validation_errors{pattern,format}`
- Support `validation: strict|advisory|none` modes

### Consumer Discovery

Consumers can discover message schemas via:

#### 1. **Admin API**
```bash
GET /api/patterns/device-notifications/schema

Response:
{
  "format": "protobuf",
  "schema_ref": "prism.devices.v1.DeviceEvent",
  "schema_url": "https://schemas.prism.internal/devices/v1/event.proto",
  "validation": "strict",
  "inline_schema": "..."
}
```

#### 2. **gRPC Metadata** (Phase 2)
- Proxy includes schema ref in gRPC response metadata
- Header: `x-prism-message-schema: protobuf:prism.devices.v1.DeviceEvent`

#### 3. **Pattern Documentation**
- Auto-generate schema docs from pattern configuration
- Include schema in pattern README

### Example: End-to-End Flow

**1. Operator configures pattern with schema:**
```yaml
pattern: multicast-registry
name: iot-telemetry

slots:
  messaging:
    backend: nats
    message_schema:
      format: protobuf
      schema_ref: prism.iot.v1.TelemetryEvent
      validation: strict
```

**2. Consumer queries schema:**
```bash
$ prism-cli pattern schema iot-telemetry

Format: protobuf
Schema: prism.iot.v1.TelemetryEvent
URL: https://schemas.prism.internal/iot/v1/telemetry.proto

message TelemetryEvent {
  string device_id = 1;
  double temperature = 2;
  double humidity = 3;
  int64 timestamp = 4;
}
```

**3. Consumer generates client code:**
```bash
$ buf generate https://schemas.prism.internal/iot/v1/telemetry.proto

Generated: iot/v1/telemetry_pb2.py
```

**4. Consumer subscribes with typed handler:**
```python
from iot.v1 import telemetry_pb2

def handle_telemetry(event: telemetry_pb2.TelemetryEvent):
    print(f"Device {event.device_id}: {event.temperature}°C")

client.subscribe("iot-telemetry", handle_telemetry)
```

**5. Publisher sends validated message:**
```python
event = telemetry_pb2.TelemetryEvent(
    device_id="sensor-42",
    temperature=23.5,
    humidity=65.2,
    timestamp=int(time.time())
)

# Proxy validates against schema before publishing
client.publish("iot-telemetry", event.SerializeToString())
```

## Benefits

1. **Self-Documenting**: Schema is part of pattern configuration, always in sync
2. **Type Safety**: Publishers and consumers use generated code from schema
3. **Evolution**: Schema registry tracks versions, validates backward compatibility
4. **Discovery**: Consumers query schema via API, no separate documentation needed
5. **Validation**: Catch schema errors at publish time, not consumer runtime

## Open Questions

1. **Schema Registry Backend**: Which registry to use?
   - Confluent Schema Registry (Kafka-focused, mature)
   - Buf Schema Registry (Protobuf-focused, modern)
   - Custom SQLite-based registry (simple, local-first)

2. **Schema Evolution Rules**: How to handle breaking changes?
   - Require new topic/pattern for breaking changes?
   - Support schema compatibility checks (backward, forward, full)?

3. **Performance Impact**: Validation overhead?
   - Benchmark: Protobuf validation ~1-10µs per message
   - Cache schemas in memory to avoid registry lookups
   - Make validation opt-in per pattern

4. **Schema Storage**: Where to store inline schemas?
   - Embed in pattern configuration YAML?
   - Store in separate schema registry?
   - Hybrid: simple schemas inline, complex schemas in registry?

## Recommendations

**POC 4 (Week 2)**:
- Add `message_schema` configuration field (documentation only, no validation)
- Implement admin API endpoint to query schema
- Update pattern YAML examples to include schema

**POC 5 (Weeks 3-4)**:
- Integrate with Buf Schema Registry (best for Protobuf)
- Implement schema validation with `strict|advisory|none` modes
- Add gRPC metadata header with schema ref

**Production**:
- Support multiple schema formats (Protobuf, JSON Schema, Avro)
- Schema registry with version management
- Automated schema compatibility checks in CI/CD
- Metrics and alerting for schema validation failures

## Related

- [RFC-017: Multicast Registry Pattern](/rfc/rfc-017)
- [ADR-003: Protobuf as Single Source of Truth](/adr/adr-003)
- [RFC-008: Proxy Plugin Architecture](/rfc/rfc-008)