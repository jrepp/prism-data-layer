---
id: rfc-027
title: "RFC-027: Namespace Configuration and Client Request Flow"
status: Proposed
author: Platform Team
created: 2025-10-10
updated: 2025-10-10
tags: [namespace, configuration, client-api, patterns, self-service]
---

## Abstract

This RFC specifies how application owners request and configure namespaces in Prism from a client perspective. It defines the limited configuration surface area available to clients, explains how client requirements map to the three-layer architecture (Client API → Patterns → Backends), and clarifies the separation of concerns between client-controlled and platform-controlled configuration.

## Motivation

### Problem Statement

Application teams need to use Prism data access services, but the current documentation is spread across multiple RFCs and ADRs (RFC-001, RFC-014, ADR-006, RFC-003). Teams have questions:

1. **"How do I request a namespace?"** - What's the process for getting started?
2. **"What can I configure?"** - What options are under my control vs platform control?
3. **"How do patterns get selected?"** - How do my requirements translate to implementation?
4. **"What's the three-layer architecture?"** - How does Client API → Patterns → Backends work from my perspective?

### Goals

- **Clear Client Perspective**: Document namespace configuration from application owner's viewpoint
- **Limited Configuration Surface**: Define exactly what clients can configure (prevents misconfiguration)
- **Self-Service Enablement**: Teams can request namespaces without platform team intervention
- **Pattern Selection Transparency**: Explain how requirements map to patterns and backends
- **Three-Layer Clarity**: Show how client concerns map to architecture layers

### Non-Goals

- **Platform Configuration**: Internal backend connection strings, resource pools (platform-controlled)
- **Pattern Implementation**: How patterns work internally (see RFC-014)
- **Backend Selection Logic**: Algorithm for choosing backends (platform-controlled)
- **Multi-Cluster Management**: Cross-region namespace configuration (see RFC-012)

## Three-Layer Architecture Recap

Before diving into client configuration, let's establish the architecture model:

```text
┌────────────────────────────────────┐
│   Client API (What)                │  ← Application declares what they need
│   KeyValue | PubSub | Queue        │     "I need a durable message queue"
└────────────────────────────────────┘
               ↓
┌────────────────────────────────────┐
│   Patterns (How)                   │  ← Prism selects how to implement it
│   Outbox | CDC | Claim Check       │     "Use Outbox + WAL patterns"
└────────────────────────────────────┘
               ↓
┌────────────────────────────────────┐
│   Backends (Where)                 │  ← Prism provisions where to store data
│   Kafka | Postgres | Redis | NATS  │     "Provision Kafka topic + Postgres table"
└────────────────────────────────────┘
```

**Key Principle**: Clients declare **what** they need (Client API + requirements). Prism decides **how** to implement it (Patterns) and **where** to store data (Backends).

## Client Configuration Surface

### What Clients Control

Clients have a **limited, safe configuration surface** to declare their needs:

1. **Client API Type** (required): `keyvalue`, `pubsub`, `queue`, `reader`, `transact`
2. **Functional Requirements** (needs): Durability, message size, retention, consistency
3. **Capacity Estimates** (needs): RPS, data size, concurrent connections
4. **Access Control** (ownership): Team ownership, consumer services
5. **Compliance** (policies): Retention, PII handling, audit requirements

### What Platform Controls

Platform team controls implementation details (clients never see these):

1. **Pattern Selection**: Which patterns to apply (Outbox, CDC, Claim Check)
2. **Backend Selection**: Which backend to use (Kafka vs NATS, Postgres vs DynamoDB)
3. **Resource Provisioning**: Topic partitions, connection pools, replica counts
4. **Network Configuration**: VPC settings, service mesh configuration
5. **Observability**: Metrics, tracing, alerting infrastructure

## Namespace Request Flow

### Step 1: Namespace Creation Request

Application owner creates a namespace by declaring their needs:

```yaml
# namespace-request.yaml
namespace: order-processing
team: payments-team

# Layer 1: Client API (What)
client_api: queue  # KeyValue, PubSub, Queue, Reader, Transact

# Layer 2: Requirements (Needs)
needs:
  # Durability
  durability: strong              # strong | eventual | best-effort
  replay: enabled                 # Enable replaying messages

  # Capacity
  write_rps: 5000                 # Estimated writes per second
  read_rps: 10000                 # Estimated reads per second
  data_size: 100GB                # Estimated total data size
  retention: 30days               # How long to keep data

  # Message Characteristics
  max_message_size: 1MB           # Largest message size
  ordered: true                   # Preserve message order

  # Consistency
  consistency: strong             # strong | eventual | bounded_staleness

# Layer 3: Access Control
access:
  owners:
    - team: payments-team
  consumers:
    - service: order-api         # Read-write access
    - service: analytics-pipeline # Read-only access

# Compliance & Policies
policies:
  pii_handling: enabled           # Enable PII detection/encryption
  audit_logging: enabled          # Log all access
  compliance: pci                 # PCI-DSS compliance requirements
```

**What's Happening:**
- **Client declares**: "I need a durable queue for order processing with strong consistency"
- **Platform receives**: Namespace request with requirements
- **Platform decides**: Which patterns/backends to use based on needs

### Step 2: Platform Pattern Selection

Platform analyzes requirements and selects patterns (client doesn't see this):

```yaml
# INTERNAL: Platform-generated configuration (client never sees this)
namespace: order-processing
client_api: queue

# Platform selects patterns based on needs
patterns:
  - type: wal                     # For durability: strong
    config:
      fsync_enabled: true         # Disk persistence

  - type: replay-store            # For replay: enabled
    config:
      retention_days: 30          # From needs.retention

# Platform selects backend
backend:
  type: kafka                     # Queue → Kafka (best fit)
  config:
    topic: order-processing-queue
    partitions: 20                # Calculated from needs.write_rps
    replication: 3                # Strong durability requirement
```

**Why Client Doesn't See This:**
- **Prevents misconfiguration**: Clients can't accidentally select incompatible patterns/backends
- **Enables evolution**: Platform can change implementation without breaking clients
- **Enforces best practices**: Platform ensures correct pattern composition

### Step 3: Namespace Provisioning

Platform provisions resources:

1. **Create Backend Resources**:
   - Kafka topic `order-processing-queue` with 20 partitions
   - Postgres WAL table `wal_order_processing`
   - Postgres replay store table `replay_order_processing`

2. **Configure Pattern Providers**:
   - Start WAL pattern provider (connects to Postgres WAL table)
   - Start Kafka publisher (connects to Kafka topic)
   - Start replay store provider (connects to Postgres replay table)

3. **Update Proxy Configuration**:
   - Register namespace `order-processing` in proxy
   - Map `queue` client API to pattern chain: `WAL → Replay Store → Kafka`

4. **Setup Access Control**:
   - Create RBAC policies for `payments-team` (admin), `order-api` (read-write), `analytics-pipeline` (read-only)

5. **Enable Observability**:
   - Create Prometheus metrics for namespace
   - Configure tracing spans
   - Setup audit log collection

### Step 4: Client Usage

Once provisioned, client uses the namespace:

```python
# Client code - simple, abstract API
from prism_client import PrismClient

client = PrismClient(namespace="order-processing")

# Publish message (durability handled by platform)
client.publish("orders", {
    "order_id": 12345,
    "amount": 99.99,
    "status": "pending"
})
# Platform handles: WAL → Disk → Kafka → Acknowledge
# Client only sees: message published successfully

# Consume messages (replay enabled by platform)
for message in client.consume("orders"):
    process_order(message.payload)
    message.ack()  # Platform updates consumer offset
```

**Client Experience:**
- Simple API: `publish()`, `consume()`, `ack()`
- No knowledge of WAL, Kafka topics, partitions, or pattern composition
- Platform handles durability, replay, ordering, consistency guarantees

## Configuration Examples

### Example 1: Simple Key-Value Store

**Use Case**: User profile cache with fast reads

```yaml
namespace: user-profiles-cache
team: user-service-team

client_api: keyvalue

needs:
  durability: eventual            # Can tolerate loss
  read_rps: 50000                 # Very high read load
  write_rps: 500                  # Low write load
  data_size: 10GB                 # Moderate size
  ttl: 15min                      # Auto-expire entries
  consistency: eventual           # Cache doesn't need strong consistency

access:
  owners:
    - team: user-service-team
  consumers:
    - service: user-api
    - service: user-profile-service

policies:
  pii_handling: enabled           # Profiles contain PII
  audit_logging: disabled         # Cache reads not audited
```

**Platform Selects**:
- **Client API**: KeyValue
- **Patterns**: Cache (look-aside pattern)
- **Backend**: Redis (best for high-read, low-write with TTL)
- **Implementation**: No WAL (eventual durability), Redis TTL for expiration

### Example 2: Event Streaming with Large Payloads

**Use Case**: Video upload events with large file references

```yaml
namespace: video-uploads
team: media-team

client_api: pubsub

needs:
  durability: strong              # Can't lose upload events
  replay: enabled                 # Reprocess events for analytics
  write_rps: 1000
  read_rps: 5000
  max_message_size: 50MB          # Large video metadata + thumbnails
  retention: 90days
  consistency: eventual           # PubSub is inherently eventual

access:
  owners:
    - team: media-team
  consumers:
    - service: video-processor
    - service: thumbnail-generator
    - service: analytics-pipeline

policies:
  pii_handling: disabled
  audit_logging: enabled
```

**Platform Selects**:
- **Client API**: PubSub
- **Patterns**: Claim Check (for large payloads), WAL (for durability), Tiered Storage (for 90-day retention)
- **Backends**: S3 (claim check storage), Kafka (event stream), Postgres (WAL)
- **Implementation**:
  - Payloads &gt;1MB stored in S3
  - Kafka receives lightweight message with S3 reference
  - WAL ensures durability
  - Old messages tiered to S3 after 7 days (hot → warm → cold)

### Example 3: Transactional Database Operations

**Use Case**: Order processing with inbox/outbox pattern

```yaml
namespace: order-transactions
team: payments-team

client_api: transact

needs:
  durability: strong              # Financial transactions
  consistency: strong             # ACID transactions required
  write_rps: 500
  retention: forever              # Financial compliance
  ordered: true                   # Transaction order matters

access:
  owners:
    - team: payments-team
  consumers:
    - service: payment-service    # Read-write
    - service: order-service      # Read-write
    - service: audit-service      # Read-only

policies:
  pii_handling: enabled           # Customer payment info
  audit_logging: enabled          # Financial compliance
  compliance: pci                 # PCI-DSS requirements
```

**Platform Selects**:
- **Client API**: Transact
- **Patterns**: Outbox (transactional guarantees), WAL (durability)
- **Backend**: Postgres (ACID transactions, strong consistency)
- **Implementation**:
  - Two tables: `orders` (data), `orders_outbox` (mailbox)
  - Transactions ensure atomicity
  - Outbox publisher processes mailbox entries
  - Full audit logging for PCI compliance

## Client vs Platform Responsibility Matrix

| Configuration Area | Client Controls | Platform Controls | Rationale |
|--------------------|-----------------|-------------------|-----------|
| **API Type** | ✅ Yes | ❌ No | Client knows their access pattern |
| **Durability Level** | ✅ Yes (needs.durability) | ❌ No | Client knows data criticality |
| **Consistency Level** | ✅ Yes (needs.consistency) | ❌ No | Client knows consistency requirements |
| **Capacity Estimates** | ✅ Yes (needs.*_rps, data_size) | ❌ No | Client knows workload |
| **Retention Period** | ✅ Yes (needs.retention) | ❌ No | Client knows data lifecycle |
| **Message Size** | ✅ Yes (needs.max_message_size) | ❌ No | Client knows payload size |
| **Team Ownership** | ✅ Yes (access.owners) | ❌ No | Client knows team structure |
| **Consumer Services** | ✅ Yes (access.consumers) | ❌ No | Client knows service dependencies |
| **PII Handling** | ✅ Yes (policies.pii_handling) | ❌ No | Client knows data sensitivity |
| **Compliance** | ✅ Yes (policies.compliance) | ❌ No | Client knows regulatory requirements |
| | | | |
| **Pattern Selection** | ❌ No | ✅ Yes | Platform expertise required |
| **Backend Selection** | ❌ No | ✅ Yes | Platform knows infrastructure |
| **Resource Provisioning** | ❌ No | ✅ Yes | Platform manages capacity |
| **Network Configuration** | ❌ No | ✅ Yes | Platform controls networking |
| **Monitoring Setup** | ❌ No | ✅ Yes | Platform provides observability |
| **Pattern Composition** | ❌ No | ✅ Yes | Platform ensures compatibility |
| **Backend Tuning** | ❌ No | ✅ Yes | Platform optimizes performance |
| **Failover Strategy** | ❌ No | ✅ Yes | Platform manages reliability |

## Authorization Boundaries

Platform enforces authorization boundaries to prevent misconfiguration:

### Boundary 1: Guided (Default)

**Who**: All application teams

**What They Can Configure**:
- Client API type
- Functional requirements (needs.*)
- Capacity estimates (needs.*_rps, needs.data_size)
- Access control (access.*)
- Basic policies (policies.pii_handling, policies.audit_logging)

**What's Restricted**:
- ❌ Cannot select patterns manually
- ❌ Cannot select backends manually
- ❌ Cannot tune backend-specific settings
- ❌ Cannot bypass capacity limits

**Example**:
```yaml
# Allowed
needs:
  durability: strong
  write_rps: 5000

# NOT allowed (would be rejected)
patterns:
  - type: outbox  # ❌ Pattern selection is platform-controlled
backend:
  type: kafka     # ❌ Backend selection is platform-controlled
```

### Boundary 2: Advanced (Requires Approval)

**Who**: Teams with platform approval for specific namespaces

**Additional Capabilities**:
- Backend preference (e.g., "prefer Redis over Postgres")
- Advanced consistency models (e.g., bounded staleness with specific lag tolerance)
- Custom retention policies beyond standard limits

**Example**:
```yaml
# Requires approval annotation
needs:
  durability: strong
  write_rps: 50000           # Above standard limit
  backend_preference: kafka  # Preference (platform can override)

approval:
  approved_by: platform-team
  approval_ticket: JIRA-1234
```

### Boundary 3: Expert (Platform Team Only)

**Who**: Platform team members

**Full Control**:
- Manual pattern selection
- Manual backend selection
- Direct backend tuning
- Bypass capacity guardrails

**Example**:
```yaml
# Platform team only
patterns:
  - type: outbox
    config:
      batch_size: 500     # Backend-specific tuning
  - type: claim-check
    config:
      threshold: 500KB    # Custom threshold

backend:
  type: postgres
  config:
    connection_pool_size: 50  # Direct tuning
    statement_timeout: 30s
```

## Namespace Lifecycle

### 1. Creation

```bash
# Via CLI
prism namespace create \
  --file namespace-request.yaml \
  --team payments-team

# Via API
POST /api/v1/namespaces
{
  "namespace": "order-processing",
  "team": "payments-team",
  "client_api": "queue",
  "needs": { ... },
  "access": { ... },
  "policies": { ... }
}
```

**Platform Actions**:
1. Validate request (schema, authorization)
2. Select patterns based on needs
3. Select backend based on patterns + needs
4. Provision backend resources
5. Configure pattern providers
6. Register namespace in proxy
7. Setup observability
8. Return namespace details to client

**Client Receives**:
```json
{
  "namespace": "order-processing",
  "status": "active",
  "endpoints": [
    "prism-proxy-1.example.com:8980",
    "prism-proxy-2.example.com:8980"
  ],
  "client_api": "queue",
  "created_at": "2025-10-10T10:00:00Z"
}
```

### 2. Usage

Client connects to namespace:

```python
client = PrismClient(
    namespace="order-processing",
    endpoints=["prism-proxy-1.example.com:8980"]
)

# Use client API (queue)
client.publish("orders", payload)
messages = client.consume("orders")
```

### 3. Monitoring

Client monitors namespace health via metrics:

```bash
# Prometheus metrics (namespace-scoped)
prism_namespace_requests_total{namespace="order-processing"} 123456
prism_namespace_latency_ms{namespace="order-processing",p="p99"} 8.5
prism_namespace_error_rate{namespace="order-processing"} 0.001
```

### 4. Updates

Client can update limited configuration:

```bash
# Update capacity estimates
prism namespace update order-processing \
  --needs.write_rps 10000

# Update access control
prism namespace update order-processing \
  --add-consumer analytics-v2-service
```

**Platform Actions**:
- Validate changes
- If capacity increased: Scale backend resources
- If new consumer added: Update RBAC policies
- If patterns need changing: Transparently migrate

### 5. Deletion

```bash
prism namespace delete order-processing --confirm
```

**Platform Actions**:
1. Mark namespace as deleting
2. Stop accepting new requests
3. Drain existing requests (graceful shutdown)
4. Delete pattern providers
5. Delete backend resources (topic, tables)
6. Archive audit logs
7. Unregister from proxy

## Request Validation

Platform validates all namespace requests:

### Schema Validation

```yaml
# Required fields
namespace: string (required, max 64 chars, lowercase-hyphen)
team: string (required)
client_api: enum (required, one of: keyvalue|pubsub|queue|reader|transact)
needs: object (required)
access: object (required)

# Capacity constraints
needs.write_rps: integer (max: 100000 without approval)
needs.read_rps: integer (max: 500000 without approval)
needs.data_size: string (max: 1TB without approval)
needs.max_message_size: string (max: 100MB without approval)
```

### Business Rules

1. **Namespace uniqueness**: Namespace names must be globally unique
2. **Team authorization**: Requesting team must exist and have quota
3. **API compatibility**: Needs must be compatible with client_api
4. **Capacity limits**: Must be within team quota
5. **Consistency constraints**: Some APIs don't support strong consistency

**Example Rejections**:

```yaml
# ❌ Rejected: KeyValue doesn't support max_message_size
namespace: user-cache
client_api: keyvalue
needs:
  max_message_size: 50MB  # ❌ KeyValue uses fixed-size values

# ❌ Rejected: PubSub can't provide strong consistency
namespace: events
client_api: pubsub
needs:
  consistency: strong  # ❌ PubSub is inherently eventual

# ❌ Rejected: Exceeds team quota
namespace: huge-data
client_api: queue
needs:
  write_rps: 200000    # ❌ Team quota: 100k RPS
```

## Pattern Selection Algorithm

Platform selects patterns based on client needs (internal logic, transparent to clients):

```python
# Platform pattern selection (simplified)
def select_patterns(client_api, needs):
    patterns = []

    # Rule 1: Durability
    if needs.durability == "strong":
        patterns.append(Pattern("wal", {"fsync": True}))

    # Rule 2: Large messages
    if needs.max_message_size > 1MB:
        patterns.append(Pattern("claim-check", {
            "threshold": "1MB",
            "storage": "s3"
        }))

    # Rule 3: Transactional consistency
    if needs.consistency == "strong" and client_api in ["queue", "transact"]:
        patterns.append(Pattern("outbox", {
            "database": "postgres"
        }))

    # Rule 4: Replay capability
    if needs.replay == "enabled":
        patterns.append(Pattern("replay-store", {
            "retention_days": needs.retention
        }))

    # Rule 5: Long retention with cost optimization
    if needs.retention > 30:
        patterns.append(Pattern("tiered-storage", {
            "hot_tier_days": 7,
            "warm_tier_days": 30,
            "cold_tier": "s3"
        }))

    return patterns

# Example: Client needs durability + large messages
needs = {
    "durability": "strong",
    "max_message_size": "50MB"
}
patterns = select_patterns("pubsub", needs)
# Result: [WAL, ClaimCheck]
```

## Backend Selection Algorithm

Platform selects backend based on client_api + patterns + needs:

```python
# Platform backend selection (simplified)
def select_backend(client_api, patterns, needs):
    if client_api == "keyvalue":
        if needs.read_rps > 100000:
            return "redis"  # High read throughput
        elif needs.data_size > 100GB:
            return "postgres"  # Large datasets
        else:
            return "redis"  # Default

    elif client_api == "pubsub":
        if "claim-check" in patterns:
            return "kafka"  # Handles large message references well
        elif needs.write_rps > 50000:
            return "kafka"  # High throughput
        else:
            return "nats"  # Lightweight, low latency

    elif client_api == "queue":
        if needs.ordered:
            return "kafka"  # Strong ordering guarantees
        elif "outbox" in patterns:
            return "postgres"  # Transactional outbox needs database
        else:
            return "kafka"  # Default

    elif client_api == "transact":
        return "postgres"  # Only backend with ACID transactions

    elif client_api == "reader":
        if needs.query_complexity == "high":
            return "postgres"  # SQL queries
        elif needs.data_model == "graph":
            return "neptune"  # Graph traversal
        else:
            return "postgres"  # Default

# Example: Queue with strong durability
backend = select_backend("queue", ["wal"], {
    "durability": "strong",
    "write_rps": 5000
})
# Result: "kafka"
```

## Namespace Discovery

Clients discover namespace endpoints via DNS or control plane API:

### Option 1: DNS Discovery (Recommended)

```bash
# Standard DNS
dig prism.example.com
# → 10.0.1.10 (prism-proxy-1)
# → 10.0.2.20 (prism-proxy-2)

# Geo-DNS (region-aware)
dig prism.us-east-1.example.com
# → 10.0.1.10 (us-east-1 proxy)

dig prism.eu-west-1.example.com
# → 10.0.2.20 (eu-west-1 proxy)
```

**Client Usage**:
```python
# Client SDK auto-discovers endpoints
client = PrismClient(
    namespace="order-processing",
    discovery="dns://prism.example.com"
)
```

### Option 2: Control Plane API

```bash
# Query control plane for namespace endpoints
GET /api/v1/namespaces/order-processing/endpoints

Response:
{
  "namespace": "order-processing",
  "endpoints": [
    {
      "address": "prism-proxy-1.example.com:8980",
      "region": "us-east-1",
      "health": "healthy",
      "load": 45
    },
    {
      "address": "prism-proxy-2.example.com:8980",
      "region": "us-east-1",
      "health": "healthy",
      "load": 52
    }
  ]
}
```

**Client Usage**:
```python
client = PrismClient(
    namespace="order-processing",
    discovery="api://control-plane.example.com/api/v1"
)
```

## Error Handling

### Namespace Creation Failures

```yaml
# Request with invalid configuration
namespace: user-cache
client_api: pubsub        # ❌ Incompatible with needs
needs:
  consistency: strong     # PubSub can't provide strong consistency

# Platform response
{
  "error": {
    "code": "INVALID_CONFIGURATION",
    "message": "Consistency 'strong' not supported by client_api 'pubsub'",
    "details": {
      "field": "needs.consistency",
      "supported_values": ["eventual"],
      "recommendation": "Use client_api 'queue' for strong consistency"
    }
  }
}
```

### Capacity Exceeded

```yaml
# Request exceeds team quota
namespace: huge-events
team: small-team
needs:
  write_rps: 200000  # Team quota: 50k RPS

# Platform response
{
  "error": {
    "code": "QUOTA_EXCEEDED",
    "message": "Requested write_rps (200000) exceeds team quota (50000)",
    "details": {
      "requested": 200000,
      "quota": 50000,
      "team": "small-team",
      "recommendation": "Request quota increase via platform team"
    }
  }
}
```

### Namespace Already Exists

```yaml
# Namespace name collision
namespace: order-processing  # Already exists

# Platform response
{
  "error": {
    "code": "NAMESPACE_EXISTS",
    "message": "Namespace 'order-processing' already exists",
    "details": {
      "existing_owner": "payments-team",
      "created_at": "2025-10-01T10:00:00Z",
      "recommendation": "Choose a different namespace name or contact existing owner"
    }
  }
}
```

## Client SDK Integration

Client SDKs abstract namespace configuration:

### Python Client

```python
from prism_client import PrismClient, ClientAPI

# Create client for specific namespace
client = PrismClient(
    namespace="order-processing",
    api=ClientAPI.QUEUE,
    discovery="dns://prism.example.com",
    auth_token="..."
)

# Use high-level API (patterns/backends transparent)
client.publish("orders", {"order_id": 123, "amount": 99.99})

for message in client.consume("orders"):
    process_order(message.payload)
    message.ack()  # Platform handles offset commit
```

### Go Client

```go
package main

import (
    "github.com/prism/client-go"
)

func main() {
    // Create client
    client, err := prism.NewClient(&prism.Config{
        Namespace: "order-processing",
        API: prism.APITypeQueue,
        Discovery: "dns://prism.example.com",
        AuthToken: "...",
    })

    // Publish
    client.Publish(ctx, "orders", &Order{
        OrderID: 123,
        Amount: 99.99,
    })

    // Consume
    messages := client.Consume(ctx, "orders")
    for msg := range messages {
        processOrder(msg.Payload)
        msg.Ack()
    }
}
```

## Migration from Existing Systems

Teams migrating from direct backend usage:

### Before: Direct Kafka Usage

```python
# Application directly uses Kafka
from kafka import KafkaProducer, KafkaConsumer

producer = KafkaProducer(
    bootstrap_servers=['kafka-1:9092', 'kafka-2:9092'],
    value_serializer=lambda v: json.dumps(v).encode('utf-8'),
    acks='all',  # Strong durability
    compression_type='gzip'
)

producer.send('order-events', {
    'order_id': 123,
    'amount': 99.99
})
```

**Problems**:
- Hard-coded Kafka endpoints
- Application manages serialization, compression, acks
- No abstraction (can't switch backends)
- No additional reliability patterns (WAL, Claim Check)

### After: Prism Namespace

```python
# Application uses Prism namespace
from prism_client import PrismClient

client = PrismClient(
    namespace="order-events",  # Platform provisions Kafka
    discovery="dns://prism.example.com"
)

client.publish("orders", {
    'order_id': 123,
    'amount': 99.99
})
# Platform handles: serialization, compression, durability, WAL
```

**Benefits**:
- No Kafka knowledge required
- Platform handles reliability patterns
- Can migrate to NATS without code changes
- Claim Check automatically enabled for large messages

## Self-Service Portal (Future)

Future UI for namespace creation (delegated to Web UI):

```text
┌─────────────────────────────────────────────────────────┐
│                   Prism Namespace Creator                │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Step 1: Basic Information                              │
│  ┌─────────────────────────────────────────────┐        │
│  │ Namespace Name: [order-processing__________]│        │
│  │ Team: [payments-team_____________________▼] │        │
│  │ Client API: [ ] KeyValue  [✓] Queue         │        │
│  │             [ ] PubSub    [ ] Reader        │        │
│  │             [ ] Transact                    │        │
│  └─────────────────────────────────────────────┘        │
│                                                          │
│  Step 2: Requirements                                   │
│  ┌─────────────────────────────────────────────┐        │
│  │ Durability: [Strong_____________________▼] │        │
│  │ Write RPS: [5000_________________________] │        │
│  │ Read RPS: [10000_________________________] │        │
│  │ Message Size: [1MB_____________________▼] │        │
│  │ Retention: [30 days____________________▼] │        │
│  │ Consistency: [Strong___________________▼] │        │
│  │ ☑ Enable replay                             │        │
│  └─────────────────────────────────────────────┘        │
│                                                          │
│  Step 3: Access Control                                 │
│  ┌─────────────────────────────────────────────┐        │
│  │ Consumers:                                  │        │
│  │ • order-api (read-write)       [Remove]    │        │
│  │ • analytics-pipeline (read)    [Remove]    │        │
│  │ [Add Consumer_______________________] [+]   │        │
│  └─────────────────────────────────────────────┘        │
│                                                          │
│  ┌────────────────┐  ┌───────────────────────┐         │
│  │ [Create]       │  │ Platform Recommends:  │         │
│  └────────────────┘  │ • Kafka backend       │         │
│                       │ • WAL + Replay patterns│        │
│                       │ • 20 partitions       │         │
│                       └───────────────────────┘         │
└─────────────────────────────────────────────────────────┘
```

## Related Documents

- [RFC-014: Layered Data Access Patterns](/rfc/rfc-014) - Pattern composition and three-layer architecture
- [ADR-006: Namespace and Multi-Tenancy](/adr/adr-006) - Namespace isolation and sharding
- [RFC-003: Admin Interface](/rfc/rfc-003) - Admin API for namespace CRUD operations
- [RFC-001: Prism Architecture](/rfc/rfc-001) - Overall system architecture
- [MEMO-006: Backend Interface Decomposition](/memos/memo-006) - Backend interface details

## Revision History

- 2025-10-10: Initial draft consolidating client namespace configuration perspective
