---
title: "RFC-030: Schema Evolution and Validation for Decoupled Pub/Sub"
author: Platform Team
created: 2025-10-13
updated: 2025-10-13T18:00:00Z
status: Draft
tags: [schema, pubsub, validation, evolution, governance, developer-experience, internet-scale]
id: rfc-030
project_id: prism-data-layer
doc_uuid: 57465edc-4a60-43d5-8963-9198b3facc96
---

## Abstract

This RFC addresses schema evolution and validation for publisher/consumer patterns in Prism where producers and consumers are decoupled across async teams with different workflows and GitHub repositories. It proposes a schema registry approach that enables producers to declare publish schemas (GitHub or dedicated registry), consumers to validate compatibility at runtime, and platform teams to enforce governance while maintaining development velocity.

## Motivation

### The Decoupling Problem

Prism's pub/sub and queue patterns intentionally decouple producers from consumers:

**Current Architecture:**
```text
┌─────────────────┐         ┌─────────────────┐
│  Producer App   │         │  Consumer App   │
│  (Team A, Repo 1)│        │  (Team B, Repo 2)│
└────────┬────────┘         └────────┬────────┘
         │                           │
         │  Publish                  │  Subscribe
         │  events                   │  events
         └───────────┐     ┌─────────┘
                     ▼     ▼
              ┌──────────────────┐
              │  Prism Proxy     │
              │  NATS/Kafka      │
              └──────────────────┘
```

**Problems This Creates:**

1. **Schema Discovery**: Consumer teams don't know what schema producers use
   - No centralized documentation
   - Tribal knowledge or Slack asks: "Hey, what fields does `user.created` have?"
   - Breaking changes discovered at runtime

2. **Version Mismatches**: Producer evolves schema, consumer breaks
   - Producer adds required field → consumers crash on deserialization
   - Producer removes field → consumers get `null` unexpectedly
   - Producer changes field type → silent data corruption

3. **Cross-Repo Workflows**: Teams can't coordinate deploys
   - Producer Team A deploys v2 schema on Monday
   - Consumer Team B still running v1 code on Friday
   - No visibility into downstream breakage

4. **Testing Challenges**: Consumers can't test against producer changes
   - Integration tests use mock data
   - Mocks drift from real schemas
   - Production is first place incompatibility detected

5. **Governance Vacuum**: No platform control over data quality
   - No PII tagging enforcement
   - No backward compatibility checks
   - No schema approval workflows

### Why This Matters for PRD-001 Goals

**PRD-001 Core Goals This Blocks:**

| Goal | Blocked By | Impact |
|------|------------|--------|
| **Accelerate Development** | Waiting for schema docs from other teams | Delays feature delivery |
| **Enable Migrations** | Can't validate consumers before backend change | Risky migrations |
| **Reduce Operational Cost** | Runtime failures from schema mismatches | Incident toil |
| **Improve Reliability** | Silent data corruption from type changes | Data quality issues |
| **Foster Innovation** | Fear of breaking downstream consumers | Slows experimentation |

### Real-World Scenarios

**Scenario 1: E-Commerce Order Events**

```text
Producer: Order Service (Team A)
  - Publishes: orders.created
  - Schema: {order_id, user_id, items[], total, currency}

Consumers:
  - Fulfillment Service (Team B): Needs order_id, items[]
  - Analytics Pipeline (Team C): Needs all fields
  - Email Service (Team D): Needs order_id, user_id, total

Problem: Team A wants to add `tax_amount` field (required)
  - How do they know which consumers will break?
  - How do consumers discover this change before deploy?
  - What happens if Team D deploys before Team A?
```

**Scenario 2: IoT Sensor Data**

```text
Producer: IoT Gateway (Team A)
  - Publishes: sensor.readings
  - Schema: {sensor_id, timestamp, temperature, humidity}

Consumers:
  - Alerting Service (Team B): Needs sensor_id, temperature
  - Data Lake (Team C): Needs all fields
  - Dashboard (Team D): Needs sensor_id, timestamp, temperature

Problem: Team A changes `temperature` from int (Celsius) to float (Fahrenheit)
  - Type change breaks deserialization
  - Semantic change breaks business logic
  - How to test this without breaking production?
```

**Scenario 3: User Profile Updates**

```text
Producer: User Service (Team A)
  - Publishes: user.profile.updated
  - Schema: {user_id, email, name, avatar_url}
  - Contains PII: email, name

Consumer: Search Indexer (Team B)
  - Stores ALL fields in Elasticsearch (public-facing search)

Problem: PII leak due to missing governance
  - Producer doesn't tag PII fields
  - Consumer indexes email addresses
  - Compliance violation, data breach risk
```

## Goals

1. **Schema Discovery**: Consumers can find producer schemas without asking humans
2. **Compatibility Validation**: Consumers detect breaking changes before deploy
3. **Decoupled Evolution**: Producers evolve schemas without coordinating deploys
4. **Testing Support**: Consumers test against real schemas in CI/CD
5. **Governance Enforcement**: Platform enforces PII tagging, compatibility rules
6. **Developer Velocity**: Schema changes take minutes, not days of coordination

## Non-Goals

1. **Runtime Schema Transformation**: No automatic v1 → v2 translation (use separate topics)
2. **Cross-Language Type System**: Won't solve Go struct ↔ Python dict ↔ Rust enum mapping
3. **Schema Inference**: Won't auto-generate schemas from published data
4. **Global Schema Uniqueness**: Same event type can have different schemas per namespace
5. **Zero Downtime Schema Migration**: Producers/consumers must handle overlapping schema versions

## Proposed Solution: Layered Schema Registry

### Architecture Overview

```text
┌────────────────────────────────────────────────────────────┐
│                  Producer Workflow                          │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  1. Define Schema (protobuf/json-schema/avro)             │
│     ├─ orders.created.v2.proto                            │
│     ├─ PII tags: @prism.pii(type="email")                 │
│     └─ Backward compat: optional new fields               │
│                                                            │
│  2. Register Schema                                        │
│     ├─ Option A: Push to GitHub (git tag release)        │
│     ├─ Option B: POST to Prism Schema Registry           │
│     └─ CI/CD validates compat                             │
│                                                            │
│  3. Publish with Schema Reference                         │
│     client.publish(topic="orders.created", payload=data,  │
│                    schema_url="github.com/.../v2.proto")  │
│                                                            │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│                  Consumer Workflow                          │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  1. Discover Schema                                        │
│     ├─ List available schemas for topic                   │
│     ├─ GET github.com/.../orders.created.v2.proto         │
│     └─ Generate client code (protoc)                      │
│                                                            │
│  2. Validate Compatibility (CI/CD)                        │
│     ├─ prism schema check --consumer my-schema.proto      │
│     ├─ Fails if producer added required fields           │
│     └─ Warns if producer removed fields                   │
│                                                            │
│  3. Subscribe with Schema Assertion                       │
│     client.subscribe(topic="orders.created",              │
│                      expected_schema="v2",                │
│                      on_mismatch="warn")                  │
│                                                            │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│              Prism Proxy (Schema Enforcement)               │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  - Caches schemas from registry/GitHub                    │
│  - Validates published messages match declared schema     │
│  - Attaches schema metadata to messages                   │
│  - Enforces PII tagging policy                            │
│  - Tracks schema versions per topic                       │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### Three-Tier Schema Storage

#### Tier 1: GitHub (Developer-Friendly, Git-Native)

**Use Case**: Open-source workflows, multi-repo teams, audit trail via Git history

```text
# Producer repository structure
my-service/
├── schemas/
│   └── events/
│       ├── orders.created.v1.proto
│       ├── orders.created.v2.proto
│       └── orders.updated.v1.proto
├── prism-config.yaml
└── README.md

# prism-config.yaml
namespaces:
  - name: orders
    pattern: pubsub
    schema:
      registry_type: github
      repository: github.com/myorg/my-service
      path: schemas/events
      branch: main  # or use git tags for immutability
```

**Schema URL Format:**
```text
github.com/myorg/my-service/blob/main/schemas/events/orders.created.v2.proto
github.com/myorg/my-service/blob/v2.1.0/schemas/events/orders.created.v2.proto  # Tagged release
```

**Pros**:
- ✅ Familiar Git workflow (PR reviews, version tags)
- ✅ Public schemas for open-source projects
- ✅ Free (GitHub hosts)
- ✅ Change history and blame
- ✅ CI/CD integration via GitHub Actions

**Cons**:
- ❌ Requires GitHub access (not suitable for air-gapped envs)
- ❌ Rate limits (5000 req/hour authenticated)
- ❌ Latency (300-500ms per fetch)

#### Tier 2: Prism Schema Registry (Platform-Managed, High Performance)

**Use Case**: Enterprise, high-throughput, governance controls, private networks

```text
# POST /v1/schemas
POST https://prism-registry.example.com/v1/schemas

{
  "namespace": "orders",
  "topic": "orders.created",
  "version": "v2",
  "format": "protobuf",
  "schema": "<base64-encoded proto>",
  "metadata": {
    "owner_team": "order-team",
    "pii_fields": ["email", "billing_address"],
    "compatibility_mode": "backward"
  }
}

# Response
{
  "schema_id": "schema-abc123",
  "schema_url": "prism-registry.example.com/v1/schemas/schema-abc123",
  "validation": {
    "compatible_with_v1": true,
    "breaking_changes": [],
    "warnings": ["Field 'tax_amount' added as optional"]
  }
}
```

**Pros**:
- ✅ Low latency (&lt;10ms, in-cluster)
- ✅ No external dependencies
- ✅ Governance hooks (approval workflows)
- ✅ Caching (aggressive, TTL=1h)
- ✅ Observability (metrics, audit logs)

**Cons**:
- ❌ Requires infrastructure (deploy + maintain registry service)
- ❌ Not Git-native (must integrate with Git repos separately)

#### Tier 3: Confluent Schema Registry (Kafka-Native)

**Use Case**: Kafka-heavy deployments, existing Confluent infrastructure

```text
# Use Confluent REST API
POST http://kafka-schema-registry:8081/subjects/orders.created-value/versions

{
  "schema": "{...protobuf IDL...}",
  "schemaType": "PROTOBUF"
}

# Prism adapter translates to Confluent API
prism-config.yaml:
  schema:
    registry_type: confluent
    url: http://kafka-schema-registry:8081
    compatibility: BACKWARD
```

**Pros**:
- ✅ Kafka ecosystem integration
- ✅ Mature, battle-tested (100k+ deployments)
- ✅ Built-in compatibility checks

**Cons**:
- ❌ Kafka-specific (doesn't work with NATS)
- ❌ Licensing (Confluent Community vs Enterprise)
- ❌ Heavy (JVM-based, 1GB+ memory)

### Comparison with Kafka Ecosystem Registries

**Validation Against Existing Standards:**

Prism's schema registry approach is validated against three major Kafka ecosystem registries:

| Feature | Confluent Schema Registry | AWS Glue Schema Registry | Apicurio Registry | Prism Schema Registry |
|---------|---------------------------|--------------------------|-------------------|----------------------|
| **Protocol Support** | REST | REST | REST | gRPC + REST |
| **Schema Formats** | Avro, Protobuf, JSON Schema | Avro, JSON Schema, Protobuf | Avro, Protobuf, JSON, OpenAPI, AsyncAPI | Protobuf, JSON Schema, Avro |
| **Backend Lock-In** | Kafka-specific | AWS-specific | Multi-backend | Multi-backend (NATS, Kafka, etc.) |
| **Compatibility Checking** | ✅ Backward, Forward, Full | ✅ Backward, Forward, Full, None | ✅ Backward, Forward, Full | ✅ Backward, Forward, Full, None |
| **Schema Evolution** | ✅ Subject-based versioning | ✅ Version-based | ✅ Artifact-based | ✅ Topic + namespace versioning |
| **Language-agnostic** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |
| **Storage Backend** | Kafka topic | DynamoDB | PostgreSQL, Kafka, Infinispan | SQLite (dev), Postgres (prod) |
| **Git Integration** | ❌ No | ❌ No | ⚠️ External only | ✅ Native GitHub support |
| **Client-Side Caching** | ⚠️ Manual | ⚠️ Manual | ⚠️ Manual | ✅ Built-in (namespace config) |
| **PII Governance** | ❌ No | ❌ No | ❌ No | ✅ Prism annotations |
| **Deployment** | JVM (1GB+) | Managed service | JVM or native | Rust (<50MB) |
| **Latency (P99)** | 10-20ms | 20-50ms | 10-30ms | &lt;10ms (in-cluster) |
| **Pricing** | Free (OSS) / Enterprise $$ | Per API call | Free (OSS) | Free (OSS) |

**Key Differentiators:**

1. **Multi-Backend Support**: Prism works with NATS, Kafka, RabbitMQ, etc. (not Kafka-specific)
2. **Git-Native**: Schemas can live in GitHub repos (no separate registry infrastructure for OSS)
3. **Config-Time Resolution**: Schema validated once at namespace config, not per-message
4. **PII Governance**: Built-in `@prism.pii` annotations for compliance
5. **Lightweight**: Rust-based registry (50MB) vs JVM-based (1GB+)

**Standard Compatibility:**

Prism implements the same compatibility modes as Confluent:
- **BACKWARD**: New schema can read old data (add optional fields)
- **FORWARD**: Old schema can read new data (delete optional fields)
- **FULL**: Both backward and forward
- **NONE**: No compatibility checks

Prism can also **interoperate** with Confluent Schema Registry via Tier 3 adapter (see above).

### Internet-Scale Decoupled Usage Scenarios

**CRITICAL DESIGN REQUIREMENT**: System must support truly independent producers/consumers across organizational boundaries.

**Scenario 1: Open-Source Data Exchange**

```text
Producer: IoT Device Manufacturer (Acme Corp)
  - Ships devices that publish telemetry to customer's Prism proxy
  - Schema: github.com/acme/device-schemas/telemetry.v1.proto
  - Public GitHub repo with MIT license

Consumer: Independent Developer (Alice)
  - Builds monitoring dashboard for Acme devices
  - Discovers schema via GitHub
  - Never talks to Acme directly

Key Challenge: Alice discovers schema change (v2) 6 months after Acme ships it
  - Solution: Backward compatibility enforced at Acme's CI/CD
  - Alice's v1 consumer continues working
  - Alice upgrades to v2 when ready (no coordination)
```

**Scenario 2: Multi-Tenant SaaS Platform**

```text
Producers: 1000s of customer applications (different companies)
  - Each publishes events to their isolated namespace
  - Schemas registered per-customer: customer123.orders.created

Consumers: Platform analytics service (SaaS vendor)
  - Subscribes to events from all customers
  - Needs to handle schema drift per customer

Key Challenge: Customer A uses v1 schema, Customer B uses v3 schema
  - Solution: Schema metadata in message headers
  - Consumer deserializes per-message using attached schema
  - No cross-customer coordination needed
```

**Scenario 3: Public API Webhooks**

```text
Producer: Payment Gateway (Stripe-like)
  - Sends webhook events to merchant endpoints
  - Schema: stripe.com/schemas/payment.succeeded.v2.json

Consumers: 100k+ merchants worldwide
  - Implement webhook handlers in various languages
  - Download JSON schema from public URL

Key Challenge: Payment gateway evolves schema, merchants deploy asynchronously
  - Solution: Public schema registry (read-only for merchants)
  - Merchants use prism schema check in CI/CD
  - Breaking changes trigger merchant notifications
```

**Scenario 4: Federated Event Bus**

```text
Producers: Multiple organizations in supply chain
  - Manufacturer publishes: mfg.shipment.created
  - Distributor publishes: dist.delivery.scheduled
  - Retailer publishes: retail.order.fulfilled

Consumers: Each organization subscribes to others' events
  - No direct contracts between organizations
  - Schema discovery via public registry

Key Challenge: No central authority to enforce schemas
  - Solution: Each organization runs own Prism Schema Registry
  - Cross-organization schema discovery via DNS (schema-registry.mfg.example.com)
  - Federation via schema URLs (like ActivityPub for events)
```

**Internet-Scale Design Principles:**

1. **No Coordination Assumption**: Producers/consumers never talk directly
2. **Public Schema Discovery**: Schemas must be fetchable via HTTPS
3. **Long Version Lifetimes**: Schemas supported for years (not weeks)
4. **Graceful Degradation**: Old consumers ignore new fields silently
5. **Namespace Isolation**: Per-tenant/organization namespaces prevent conflicts

### Schema Declaration in Namespace Config

**CRITICAL ARCHITECTURAL DECISION**: Schema is declared ONCE in namespace configuration, not per-message. The proxy automatically attaches schema metadata to all published messages.

**Client-Originated Configuration (RFC-014):**

```yaml
# Producer namespace config - schema declared at configuration time
namespaces:
  - name: order-events
    pattern: pubsub
    backend:
      type: nats
      topic: orders.created

    # Schema declaration (ONCE per namespace, not per publish)
    schema:
      # Option 1: GitHub reference
      registry_type: github
      url: github.com/myorg/order-service/schemas/orders.created.v2.proto
      version: v2  # Explicit version for this namespace

      # When validation happens:
      validation:
        config_time: true   # Validate schema exists when namespace is configured
        build_time: true    # Generate typed clients at build time
        publish_time: false # NO per-message validation (performance)

      # Option 2: Prism Schema Registry reference
      registry_type: prism
      registry_url: https://schema-registry.example.com
      subject: orders.created  # Subject name in registry
      version: v2

      # Compatibility policy
      compatibility: backward  # v2 consumers can read v1 data

      # PII enforcement (checked at registration time, not runtime)
      pii_validation: enforce  # fail if PII fields not tagged
```

**Key Design Principles:**

1. **Configuration-Time Schema Resolution**: When namespace is configured, Prism:
   - Fetches schema from registry/GitHub
   - Validates schema exists and is parseable
   - Caches schema definition in proxy memory
   - Generates code gen artifacts (if requested)

2. **Zero Per-Message Overhead**: Proxy attaches cached schema metadata to every message without re-validation

3. **Build-Time Assertions**: Client code generation ensures type safety at compile time

4. **Optional Runtime Validation**: Only enabled explicitly for debugging (huge performance cost)

### Schema Attachment at Publish Time

**Configuration-Time Schema Resolution (ONCE):**

```text
sequenceDiagram
    participant App as Producer App
    participant Client as Prism Client SDK
    participant Proxy as Prism Proxy
    participant Registry as Schema Registry

    Note over App,Proxy: 1. Namespace Configuration (happens ONCE at startup)

    App->>Client: Configure namespace "order-events"
    Client->>Proxy: ConfigureNamespace(schema_url="github.com/.../v2.proto")

    Proxy->>Registry: GET schema (with cache)
    Registry-->>Proxy: Schema definition + metadata

    Proxy->>Proxy: Cache schema in memory
    Proxy-->>Client: Namespace configured
    Client-->>App: Ready to publish
```

**Publish Flow (NO per-message validation):**

```text
sequenceDiagram
    participant App as Producer App
    participant Client as Prism Client SDK
    participant Proxy as Prism Proxy
    participant NATS as NATS Backend

    Note over App,NATS: 2. Publishing Messages (fast path, no validation)

    App->>Client: publish(payload=OrderCreated{...})

    Note over Client: Serialize using generated code (build-time types)

    Client->>Proxy: PublishRequest(payload bytes)

    Note over Proxy: Lookup cached schema for namespace

    Proxy->>Proxy: Attach schema metadata (0 cost lookup)
    Proxy->>NATS: Publish + headers{schema_url, schema_version, schema_hash}
    NATS-->>Proxy: ACK
    Proxy-->>Client: Success
    Client-->>App: Published
```

**Optional Runtime Validation (debugging only):**

```text
# Enable ONLY for debugging - huge performance cost
validation:
  config_time: true
  build_time: true
  publish_time: true  # ⚠️ WARNING: +50% latency overhead

# Proxy validates every message against schema
# Use only when debugging schema issues
```

**Message Format with Schema Metadata:**

```text
# NATS message headers
X-Prism-Schema-URL: github.com/myorg/order-service/schemas/orders.created.v2.proto
X-Prism-Schema-Version: v2
X-Prism-Schema-Hash: sha256:abc123...  # For immutability check
X-Prism-Namespace: order-events
X-Prism-Published-At: 2025-10-13T10:30:00Z

# Payload (protobuf binary)
<binary protobuf OrderCreated>
```

### Consumer Schema Discovery and Validation

**Discovery API:**

```bash
# List all schemas for a topic
prism schema list --topic orders.created

# Output:
# VERSION   URL                                                    PUBLISHED     CONSUMERS
# v2        github.com/.../orders.created.v2.proto                 2025-10-13    3 active
# v1        github.com/.../orders.created.v1.proto                 2025-09-01    1 active (deprecated)

# Get schema definition
prism schema get --topic orders.created --version v2

# Output: (downloads proto file)
syntax = "proto3";
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  string email = 3;
  repeated OrderItem items = 4;
  double total = 5;
  string currency = 6;
  optional double tax_amount = 7;  // Added in v2
}

# Generate client code
prism schema codegen --topic orders.created --version v2 --language go --output ./proto
# Generates: orders_created.pb.go
```

**Consumer Compatibility Check (CI/CD):**

```bash
# In consumer CI pipeline
prism schema check \
  --topic orders.created \
  --consumer-schema ./schemas/my_consumer_schema.proto \
  --mode strict

# Output:
✅ Compatible with producer schema v2
⚠️  Warning: Producer added optional field 'tax_amount' (not in consumer schema)
❌ Error: Consumer expects required field 'discount_code' (not in producer schema)

# Exit code: 1 (fail CI)
```

**Consumer Subscription with Schema Assertion:**

```python
# Python consumer with schema validation
from prism_sdk import PrismClient
from prism_sdk.schema import SchemaValidator

client = PrismClient(namespace="order-events")

# Option 1: Validate at subscribe time (fail-fast)
stream = client.subscribe(
    topic="orders.created",
    schema_assertion={
        "expected_version": "v2",
        "on_mismatch": "error",  # Options: error | warn | ignore
        "compatibility_mode": "forward"  # v1 consumer reads v2 data
    }
)

# Option 2: Validate per-message (flexible)
for event in stream:
    try:
        # Client SDK deserializes using schema from message headers
        order = event.payload  # Typed OrderCreated object

        # Explicit validation
        if event.schema_version != "v2":
            logger.warning(f"Unexpected schema version: {event.schema_version}")
            continue

        process_order(order)
        event.ack()

    except SchemaValidationError as e:
        logger.error(f"Schema mismatch: {e}")
        event.nack()  # Reject message, will retry or DLQ
```

### Backward/Forward Compatibility Modes

**Compatibility Matrix:**

| Mode | Producer Changes Allowed | Consumer Requirement |
|------|-------------------------|---------------------|
| **Backward** | Add optional fields | Old consumers work with new data |
| **Forward** | Delete optional fields | New consumers work with old data |
| **Full** | Add/delete optional fields | Bidirectional compatibility |
| **None** | Any changes | No compatibility guarantees |

**Example: Backward Compatibility**

```text
# Producer v1 schema
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  double total = 3;
}

# Producer v2 schema (backward compatible)
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  double total = 3;
  optional double tax_amount = 4;  # NEW: Optional field
  optional string promo_code = 5;  # NEW: Optional field
}

# Consumer still on v1 code
order = OrderCreated.decode(payload)
print(order.total)  # Works! Ignores unknown fields (tax_amount, promo_code)
```

**Example: Forward Compatibility**

```text
# Producer v1 schema
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  double total = 3;
  optional string notes = 4;  # Optional field
}

# Producer v2 schema (forward compatible)
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  double total = 3;
  # Removed: optional string notes = 4;
}

# Consumer on v2 code reads v1 message
order = OrderCreated.decode(payload)
print(order.notes)  # Empty/default value, no error
```

### Governance: PII Tagging and Enforcement

**Problem**: Producers publish PII without awareness, consumers leak it

**Solution**: Mandatory PII tags validated at schema registration

```protobuf
syntax = "proto3";

import "prism/annotations.proto";

message UserProfileUpdated {
  string user_id = 1 [(prism.index) = "primary"];

  // PII fields MUST be tagged
  string email = 2 [(prism.pii) = "email"];
  string full_name = 3 [(prism.pii) = "name"];
  string phone = 4 [(prism.pii) = "phone"];
  string ssn = 5 [(prism.pii) = "ssn", (prism.encrypt) = "aes256"];

  // Non-PII fields
  string avatar_url = 6;
  int64 created_at = 7;
}
```

**Enforcement at Schema Registration:**

```bash
# Producer tries to register schema
prism schema register --file user_profile.proto

# Prism validation:
❌ Error: Field 'email' contains PII but lacks @prism.pii annotation
❌ Error: Field 'phone' contains PII but lacks @prism.pii annotation
ℹ️  Hint: Add annotation: [(prism.pii) = "email"]

# Exit code: 1 (registration fails)
```

**Consumer PII Awareness:**

```python
# Consumer auto-detects PII fields
for event in client.subscribe("user.profile.updated"):
    user = event.payload

    # SDK warns about PII access
    print(user.email)  # Warning: Accessing PII field 'email'

    # Check if field is PII
    if event.schema.is_pii_field("email"):
        # Mask for logging
        logger.info(f"User updated: email={mask_pii(user.email)}")
```

### Schema Evolution Workflow

**Scenario: Add Optional Field (Backward Compatible)**

```bash
# Step 1: Producer team updates schema
# schemas/orders.created.v2.proto
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  double total = 3;
  optional double tax_amount = 4;  # NEW
}

# Step 2: Validate compatibility
prism schema validate --file orders.created.v2.proto --check-backward

# Output:
✅ Backward compatible with v1
   - Added optional field 'tax_amount' (safe)

# Step 3: Register new schema version
prism schema register \
  --file orders.created.v2.proto \
  --topic orders.created \
  --version v2 \
  --compatibility backward

# Output:
✅ Schema registered: schema-xyz789
   URL: github.com/myorg/order-service/schemas/orders.created.v2.proto
   Compatible consumers: v1 (3 instances)

# Step 4: Update producer code to publish v2
# Producer code change
client.publish(
    topic="orders.created",
    payload=order_v2,  # Includes tax_amount
    schema_version="v2"
)

# Step 5: Deploy producer (v1 consumers still work!)
kubectl apply -f producer-deployment.yaml

# Step 6: Consumers discover new schema
prism schema list --topic orders.created
# v2 now available, v1 consumers keep working

# Step 7: Consumer teams upgrade when ready (no coordination!)
# Consumer Team B: Updates code to use tax_amount
# Consumer Team C: Ignores new field (still works)
```

**Scenario: Add Required Field (Breaking Change)**

```bash
# Producer wants to add required field
message OrderCreated {
  string order_id = 1;
  string user_id = 2;
  double total = 3;
  string payment_method = 4;  # NEW: Required field
}

# Validation fails
prism schema validate --file orders.created.v3.proto --check-backward

# Output:
❌ NOT backward compatible with v2
   - Added required field 'payment_method' (BREAKING)

ℹ️  Recommendation: Use new topic 'orders.created.v3' or make field optional

# Producer options:
# Option A: New topic (clean separation)
client.publish(topic="orders.created.v3", payload=order_v3)

# Option B: Make field optional (non-breaking)
optional string payment_method = 4;

# Option C: Parallel publish (transition period)
client.publish(topic="orders.created", payload=order_v2)  # Old consumers
client.publish(topic="orders.created.v3", payload=order_v3)  # New consumers
```

### Schema Registry API Specification

**gRPC Service:**

```protobuf
syntax = "proto3";

package prism.schema.v1;

service SchemaRegistryService {
  // Register new schema version
  rpc RegisterSchema(RegisterSchemaRequest) returns (RegisterSchemaResponse);

  // Get schema by topic + version
  rpc GetSchema(GetSchemaRequest) returns (GetSchemaResponse);

  // List all schema versions for topic
  rpc ListSchemas(ListSchemasRequest) returns (ListSchemasResponse);

  // Check compatibility between schemas
  rpc CheckCompatibility(CheckCompatibilityRequest) returns (CheckCompatibilityResponse);

  // Delete schema version (with safety checks)
  rpc DeleteSchema(DeleteSchemaRequest) returns (DeleteSchemaResponse);

  // Get active consumers for schema version
  rpc GetConsumers(GetConsumersRequest) returns (GetConsumersResponse);
}

message RegisterSchemaRequest {
  string namespace = 1;
  string topic = 2;
  string version = 3;  // e.g., "v2", "1.0.0"

  SchemaFormat format = 4;
  bytes schema_content = 5;  // Protobuf IDL, JSON Schema, Avro, etc.

  CompatibilityMode compatibility = 6;
  map<string, string> metadata = 7;  // owner_team, description, etc.
}

enum SchemaFormat {
  SCHEMA_FORMAT_UNSPECIFIED = 0;
  SCHEMA_FORMAT_PROTOBUF = 1;
  SCHEMA_FORMAT_JSON_SCHEMA = 2;
  SCHEMA_FORMAT_AVRO = 3;
}

enum CompatibilityMode {
  COMPATIBILITY_MODE_UNSPECIFIED = 0;
  COMPATIBILITY_MODE_NONE = 1;
  COMPATIBILITY_MODE_BACKWARD = 2;
  COMPATIBILITY_MODE_FORWARD = 3;
  COMPATIBILITY_MODE_FULL = 4;
}

message RegisterSchemaResponse {
  string schema_id = 1;
  string schema_url = 2;
  ValidationResult validation = 3;
}

message ValidationResult {
  bool is_compatible = 1;
  repeated string breaking_changes = 2;
  repeated string warnings = 3;
  repeated string compatible_versions = 4;
}

message GetSchemaRequest {
  string namespace = 1;
  string topic = 2;
  string version = 3;  // or "latest"
}

message GetSchemaResponse {
  string schema_id = 1;
  string version = 2;
  SchemaFormat format = 3;
  bytes schema_content = 4;
  SchemaMetadata metadata = 5;
}

message SchemaMetadata {
  string owner_team = 1;
  string description = 2;
  google.protobuf.Timestamp created_at = 3;
  string created_by = 4;
  repeated string pii_fields = 5;
  CompatibilityMode compatibility = 6;
  int32 active_consumers = 7;
}
```

### Developer Workflows

**Workflow 1: New Producer Team**

```bash
# 1. Create schema file
mkdir -p schemas/events
cat > schemas/events/notification.sent.v1.proto <<EOF
syntax = "proto3";
message NotificationSent {
  string notification_id = 1;
  string user_id = 2;
  string channel = 3;  // email, sms, push
  string status = 4;
}
EOF

# 2. Register schema
prism schema register \
  --file schemas/events/notification.sent.v1.proto \
  --topic notification.sent \
  --version v1 \
  --compatibility backward \
  --owner-team notifications-team

# 3. Generate client code
prism schema codegen \
  --topic notification.sent \
  --version v1 \
  --language python \
  --output ./generated

# 4. Publish with schema reference
from generated import notification_sent_pb2
from prism_sdk import PrismClient

client = PrismClient(namespace="notifications", schema_validation=True)

notification = notification_sent_pb2.NotificationSent(
    notification_id="notif-123",
    user_id="user-456",
    channel="email",
    status="sent"
)

client.publish(
    topic="notification.sent",
    payload=notification,
    schema_version="v1"
)
```

**Workflow 2: Existing Consumer Team**

```bash
# 1. Discover available schemas
prism schema list --topic orders.created

# Output:
# VERSION   STATUS       CONSUMERS   PUBLISHED
# v2        current      3           2025-10-13
# v1        deprecated   1           2025-09-01

# 2. Get schema definition
prism schema get --topic orders.created --version v2 --output ./schemas

# 3. Check compatibility with current consumer code
prism schema check \
  --topic orders.created \
  --consumer-schema ./schemas/my_orders_v1.proto \
  --mode strict

# Output:
⚠️  Warning: Producer added field 'tax_amount' (optional)
✅ Your consumer code will continue to work

# 4. Generate updated client code
prism schema codegen \
  --topic orders.created \
  --version v2 \
  --language rust \
  --output ./src/generated

# 5. Update consumer code
use prism_sdk::PrismClient;
use generated::orders_created::OrderCreated;

let client = PrismClient::new("order-events")
    .with_schema_validation(true);

let stream = client.subscribe("orders.created")
    .with_schema_assertion("v2", OnMismatch::Warn)
    .build()?;

for event in stream {
    let order: OrderCreated = event.payload()?;

    // New field available (optional)
    if let Some(tax) = order.tax_amount {
        println!("Tax: ${}", tax);
    }

    process_order(&order)?;
    event.ack()?;
}
```

**Workflow 3: Platform Team Governance**

```bash
# Audit all schemas for PII tagging
prism schema audit --check-pii

# Output:
❌ orders.created.v2: Field 'email' missing @prism.pii tag
❌ user.profile.updated.v1: Field 'phone' missing @prism.pii tag
✅ notification.sent.v1: All PII fields tagged

# Enforce compatibility policy
prism schema policy set \
  --namespace orders \
  --compatibility backward \
  --require-pii-tags \
  --approval-required-for breaking

# Block incompatible schema registration
prism schema register --file orders.created.v3.proto

# Output:
❌ Registration blocked: Breaking changes detected
   - Removed field 'currency' (required)
   - Added required field 'payment_method'

ℹ️  Policy requires approval for breaking changes
   Create approval request: prism schema approve-request --schema-file orders.created.v3.proto
```

## Implementation Plan

### Phase 1: GitHub-Based Registry (Weeks 1-3)

**Deliverables:**
- ✅ Schema URL parsing (github.com/org/repo/path)
- ✅ GitHub API client (fetch schema files)
- ✅ Local schema cache (TTL=1h)
- ✅ Publish-time schema attachment (message headers)
- ✅ Consumer schema discovery CLI (`prism schema list/get`)

**Success Criteria:**
- Producer can reference GitHub schema in config
- Consumer can fetch schema from GitHub
- Message headers include schema metadata

### Phase 2: Schema Validation (Weeks 4-6)

**Deliverables:**
- ✅ Protobuf schema parser
- ✅ Publish-time payload validation
- ✅ Compatibility checker (backward/forward)
- ✅ CI/CD integration (`prism schema check`)
- ✅ Consumer-side schema assertion

**Success Criteria:**
- Invalid publish rejected with clear error
- CI pipeline catches breaking changes before merge
- Consumer can opt into strict schema validation

### Phase 3: Prism Schema Registry (Weeks 7-10)

**Deliverables:**
- ✅ Schema Registry gRPC service
- ✅ SQLite storage (local dev), Postgres (prod)
- ✅ REST API adapter (for non-gRPC clients)
- ✅ Admin UI (Ember.js) for browsing schemas
- ✅ Migration from GitHub to registry

**Success Criteria:**
- Schema registry handles 10k req/sec
- &lt;10ms P99 latency for schema fetch
- UI shows schema versions, consumers, compatibility

### Phase 4: Governance and PII (Weeks 11-13)

**Deliverables:**
- ✅ PII annotation parser (`@prism.pii`)
- ✅ PII validation at schema registration
- ✅ Approval workflows for breaking changes
- ✅ Audit logs (who registered what, when)
- ✅ Consumer PII awareness SDK

**Success Criteria:**
- Schema without PII tags rejected
- Breaking changes require approval
- Audit trail for compliance

### Phase 5: Code Generation (Weeks 14-16)

**Deliverables:**
- ✅ `prism schema codegen` CLI
- ✅ Protobuf → Go structs
- ✅ Protobuf → Python dataclasses
- ✅ Protobuf → Rust structs
- ✅ JSON Schema → TypeScript interfaces

**Success Criteria:**
- One command generates client code
- Generated code includes PII awareness
- Works with all supported languages (Go, Python, Rust)

## Trade-Offs and Alternatives

### Alternative 1: No Schema Registry (Status Quo)

**Pros:**
- ✅ Zero infrastructure overhead
- ✅ No coordination needed

**Cons:**
- ❌ Runtime failures from schema mismatches
- ❌ No PII governance
- ❌ Manual coordination for schema changes
- ❌ Testing impossible without mocks

**Verdict:** Unacceptable for PRD-001 reliability goals

### Alternative 2: Confluent Schema Registry Only

**Pros:**
- ✅ Battle-tested at scale
- ✅ Rich compatibility checks
- ✅ Kafka ecosystem integration

**Cons:**
- ❌ Kafka-specific (doesn't work with NATS)
- ❌ JVM-based (1GB+ memory)
- ❌ Licensing complexity
- ❌ Not Git-native

**Verdict:** Good for Kafka-heavy deployments, but too narrow for Prism's multi-backend vision

### Alternative 3: Git-Only (No Registry Service)

**Pros:**
- ✅ Familiar Git workflow
- ✅ Free (GitHub)
- ✅ Version control built-in

**Cons:**
- ❌ GitHub rate limits (5000 req/hour)
- ❌ High latency (300-500ms)
- ❌ No runtime governance
- ❌ Poor observability

**Verdict:** Good for low-throughput, open-source projects, but insufficient for enterprise

### Proposed Hybrid Approach

**Use all three tiers based on context:**

| Scenario | Recommended Registry | Rationale |
|----------|---------------------|-----------|
| **Open-source project** | GitHub | Public schemas, Git workflow |
| **Internal services (&lt;1k RPS)** | GitHub | Simple, no infra |
| **Production (&gt;10k RPS)** | Prism Registry | Performance, governance |
| **Kafka-native pipeline** | Confluent Registry | Ecosystem integration |
| **Air-gapped network** | Prism Registry | No external dependencies |

## Security Considerations

### Schema Tampering

**Risk:** Attacker modifies schema to inject malicious fields

**Mitigation:**
- Schema hash verification (SHA256)
- Immutable schema versions (can't edit v2 after publish)
- Git commit signatures (for GitHub registry)
- Audit logs (who changed what)

### PII Leakage

**Risk:** Consumer accidentally logs PII field

**Mitigation:**
- Mandatory PII tagging at registration
- SDK warnings on PII field access
- Automatic masking in logs (via SDK)
- Compliance scanning of schemas

### Schema Poisoning

**Risk:** Malicious producer registers incompatible schema

**Mitigation:**
- Namespace-based authorization (only owner team can register)
- Approval workflows for breaking changes
- Rollback capability (revert to previous version)
- Canary deployments (gradual rollout)

## Performance Characteristics

### Schema Registry Benchmarks

| Operation | Latency (P99) | Throughput | Caching |
|-----------|---------------|------------|---------|
| **GitHub fetch** | 500ms | 5k req/hour | TTL=1h |
| **Registry fetch** | 10ms | 100k RPS | Aggressive |
| **Compatibility check** | 50ms | 10k RPS | N/A |
| **Schema validation** | 5ms | 50k RPS | In-memory |

### Publish Overhead

**Without schema validation:** 10ms P99
**With schema validation:** 15ms P99 (+50% overhead)

**Rationale:** Acceptable trade-off for correctness

## Observability

### Metrics

```text
# Schema registry
prism_schema_registry_requests_total{operation="get_schema", status="success"}
prism_schema_registry_cache_hit_rate{namespace="orders"}
prism_schema_registry_validation_failures{topic="orders.created", reason="missing_field"}

# Publisher
prism_publish_schema_validation_duration_seconds{topic="orders.created", result="valid"}
prism_publish_schema_mismatch_total{topic="orders.created", error_type="missing_field"}

# Consumer
prism_subscribe_schema_assertion_failures{topic="orders.created", expected_version="v2", actual_version="v1"}
prism_consumer_schema_incompatible_messages{topic="orders.created", action="dropped"}
```

### Logs

```json
{
  "event": "schema_validation_failed",
  "topic": "orders.created",
  "schema_version": "v2",
  "error": "Field 'email' missing (required)",
  "publisher_id": "order-service-pod-123",
  "timestamp": "2025-10-13T10:30:00Z"
}
```

### Traces

```text
Span: PublishWithSchemaValidation [15ms]
├─ Span: FetchSchema (cached) [2ms]
├─ Span: ValidatePayload [8ms]
│  ├─ Check required fields [2ms]
│  ├─ Check PII tags [1ms]
│  └─ Type validation [5ms]
└─ Span: PublishToBackend [5ms]
```

## Testing Strategy

### Unit Tests

```go
func TestSchemaCompatibilityBackward(t *testing.T) {
    v1 := loadSchema("orders.created.v1.proto")
    v2 := loadSchema("orders.created.v2.proto")

    checker := NewCompatibilityChecker(CompatibilityBackward)
    result := checker.Check(v1, v2)

    assert.True(t, result.IsCompatible)
    assert.Contains(t, result.Warnings, "Added optional field 'tax_amount'")
}

func TestSchemaValidationFailure(t *testing.T) {
    schema := loadSchema("orders.created.v2.proto")
    payload := map[string]interface{}{
        "order_id": "order-123",
        "user_id": "user-456",
        // Missing required field 'total'
    }

    validator := NewSchemaValidator(schema)
    err := validator.Validate(payload)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "Field 'total' missing")
}
```

### Integration Tests

```python
def test_publish_with_schema_validation(prism_client, schema_registry):
    # Register schema
    schema_id = schema_registry.register(
        topic="test.events",
        version="v1",
        schema=load_proto("test_events.v1.proto")
    )

    # Publish valid message
    response = prism_client.publish(
        topic="test.events",
        payload={"event_id": "evt-123", "data": "foo"},
        schema_version="v1"
    )
    assert response.success

    # Publish invalid message (should fail)
    with pytest.raises(SchemaValidationError):
        prism_client.publish(
            topic="test.events",
            payload={"event_id": "evt-456"},  # Missing 'data' field
            schema_version="v1"
        )
```

### End-to-End Tests

```bash
# Test full workflow: register → publish → consume → validate
make test-e2e-schema

# Steps:
# 1. Start Prism proxy + schema registry
# 2. Register schema via CLI
# 3. Producer publishes with schema reference
# 4. Consumer subscribes with schema assertion
# 5. Verify consumer receives typed payload
# 6. Verify incompatible message rejected
```

## Migration Path

### Phase 0: No Schemas (Current State)

```yaml
# Producers publish arbitrary payloads
client.publish(topic="orders.created", payload={"order_id": "123", ...})
```

### Phase 1: Optional Schema References (Soft Launch)

```yaml
# Producers optionally declare schema URL
namespaces:
  - name: orders
    schema:
      url: github.com/.../orders.created.v1.proto
      validation: warn  # Log warnings, don't fail
```

### Phase 2: Mandatory Schemas for New Topics

```yaml
# New topics require schema declaration
namespaces:
  - name: new-events
    schema:
      url: github.com/.../new_events.v1.proto
      validation: strict  # Fail on mismatch
```

### Phase 3: Governance Enforcement

```yaml
# All topics require schema + PII tags
prism schema policy set --require-pii-tags --global
```

## Success Criteria

1. **Schema Discovery**: Consumer finds producer schema in <10 seconds
2. **Breaking Change Detection**: CI catches incompatible schema in <30 seconds
3. **Publish Overhead**: &lt;15ms P99 with validation enabled
4. **Developer Adoption**: 80% of new topics use schemas within 6 months
5. **PII Compliance**: 100% of schemas with PII have tags within 12 months

## Open Questions

1. **Schema Versioning**: Semantic versioning (1.0.0) or simple (v1, v2)?
2. **Schema Deletion**: Allow deletion of old versions with active consumers?
3. **Cross-Namespace Schemas**: Can schemas be shared across namespaces?
4. **Schema Testing**: How to test schema changes before production?
5. **Schema Ownership**: Team-based or individual ownership model?

## References

- **PRD-001**: Prism Data Access Gateway (core product goals)
- **RFC-002**: Data Layer Interface Specification (PubSub service)
- **RFC-014**: Layered Data Access Patterns (pub/sub patterns)
- **ADR-003**: Protobuf Single Source of Truth (protobuf strategy)
- [Confluent Schema Registry](https://docs.confluent.io/platform/current/schema-registry/index.html)
- [Google Pub/Sub Schema Validation](https://cloud.google.com/pubsub/docs/schemas)
- [AWS EventBridge Schema Registry](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-schema.html)

## Revision History

- 2025-10-13 (v2): Major architectural revisions based on feedback:
  - **Schema declaration moved to namespace config** (not per-publish) for performance
  - **Validation timing clarified**: Build-time (code gen) + config-time (validation), NOT runtime per-message
  - **Comparison with Kafka ecosystem registries**: Confluent, AWS Glue, Apicurio feature matrix
  - **Internet-scale scenarios added**: Open-source data exchange, multi-tenant SaaS, public webhooks, federated event bus
  - **Key principle**: Zero per-message overhead via config-time schema resolution

- 2025-10-13 (v1): Initial draft exploring schema evolution and validation for decoupled pub/sub
