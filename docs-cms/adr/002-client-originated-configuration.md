---
title: "ADR-002: Client-Originated Configuration"
status: Accepted
date: 2025-10-05
deciders: Core Team
tags: ['architecture', 'configuration', 'dx']
---

## Context

Traditional data infrastructure requires manual provisioning:

1. Application team estimates data requirements
2. DBA provisions database cluster
3. Application team configures connection details
4. Capacity is often wrong (over or under-provisioned)
5. Changes require coordination between teams

Netflix's Data Gateway improves this with declarative deployment configuration, but still requires infrastructure team involvement to map capacity requirements to hardware.

**Problem**: Manual capacity planning is slow, error-prone, and creates bottlenecks.

## Decision

Implement **client-originated configuration** where applications declare their data access patterns in protobuf definitions, and Prism automatically:

1. Selects optimal backend storage engine
2. Calculates capacity requirements
3. Provisions infrastructure
4. Configures connections and policies

## Rationale

### How It Works

Applications define data models with annotations:

```protobuf
message UserEvents {
  string user_id = 1 [(prism.index) = "partition_key"];
  bytes event_data = 2;
  int64 timestamp = 3 [(prism.index) = "clustering_key"];

  option (prism.access_pattern) = "append_heavy";  // 95% writes, 5% reads
  option (prism.estimated_write_rps) = "10000";     // Peak writes/sec
  option (prism.estimated_read_rps) = "500";        // Peak reads/sec
  option (prism.data_size_estimate_mb) = "1000";    // Total data size
  option (prism.retention_days) = "90";             // Auto-delete old data
  option (prism.consistency) = "eventual";          // Consistency requirement
  option (prism.latency_p99_ms) = "10";             // Latency SLO
}
```

Prism's capacity planner:

1. **Analyzes access pattern**: "append_heavy" → Kafka is ideal
2. **Calculates partition count**: 10k writes/sec → 20 partitions (500 writes/partition/sec)
3. **Provisions cluster**: Creates Kafka cluster with appropriate instance types
4. **Configures retention**: Sets 90-day retention policy
5. **Sets up monitoring**: Alerts if P99 > 10ms or RPS exceeds 10k

### Benefits Over Manual Provisioning

| Aspect | Manual | Client-Originated |
|--------|--------|-------------------|
| Time to provision | Days/weeks | Minutes |
| Accuracy | Often wrong | Data-driven |
| Ownership | Split (app + infra teams) | Clear (app team) |
| Scaling | Manual requests | Automatic |
| Cost optimization | Ad-hoc | Continuous |

### Alternatives Considered

1. **Manual Provisioning** (traditional approach)
   - Pros:
     - Full control
     - Familiar to ops teams
   - Cons:
     - Slow (days/weeks)
     - Error-prone
     - Creates bottlenecks
     - Scales poorly (1 DBA : N teams)
   - Rejected because: Doesn't scale as org grows

2. **Declarative Deployment Config** (Netflix's approach)
   - Pros:
     - Better than manual
     - Infrastructure as code
     - Version controlled
   - Cons:
     - Still requires capacity planning expertise
     - Separate from application code
     - Changes require infra team review
   - Rejected because: Still creates coordination overhead

3. **Fully Automatic** (no application hints)
   - Pros:
     - Zero configuration burden
     - Ultimate simplicity
   - Cons:
     - Cannot optimize for known patterns
     - Over-provisions to be safe
     - Higher costs
   - Rejected because: Loses optimization opportunities

4. **Runtime Metrics-Based** (scale based on observed load)
   - Pros:
     - Responds to actual usage
     - No estimation needed
   - Cons:
     - Reactive not proactive
     - Poor for spiky workloads
     - Doesn't help initial provisioning
   - Rejected because: Can be combined with client-originated config for continuous optimization

## Consequences

### Positive

- **Faster Development**: No waiting for database provisioning
- **Self-Service**: Application teams are empowered
- **Accurate Capacity**: Based on actual requirements, not guesses
- **Cost Optimization**: Right-sized infrastructure from day one
- **Living Documentation**: Protobuf definitions document requirements
- **Easier Migrations**: Change `option (prism.backend) = "postgres"` to `"kafka"` and redeploy

### Negative

- **More Complex Tooling**: Capacity planner must be sophisticated
  - *Mitigation*: Start with conservative heuristics; refine over time
- **Protobuf Coupling**: Configuration embedded in data models
  - *Mitigation*: This is intentional; keeps requirements close to code
- **Requires Estimation**: Teams must estimate RPS, data size
  - *Mitigation*: Provide estimation tools; Prism adapts based on actual metrics

### Neutral

- **Shifts Responsibility**: From infra team to app teams
  - Some teams will prefer this (autonomy)
  - Others may miss having an expert provision for them
  - *Plan*: Provide templates and examples for common patterns

## Implementation Notes

### Protobuf Extensions

Define custom options in `prism/options.proto`:

```protobuf
syntax = "proto3";

package prism;

import "google/protobuf/descriptor.proto";

extend google.protobuf.MessageOptions {
  // Access pattern hint
  string access_pattern = 50001;  // "read_heavy" | "write_heavy" | "append_heavy" | "balanced"

  // Capacity estimates
  int64 estimated_read_rps = 50002;
  int64 estimated_write_rps = 50003;
  int64 data_size_estimate_mb = 50004;

  // Policies
  int32 retention_days = 50005;
  string consistency = 50006;  // "strong" | "eventual" | "causal"
  int32 latency_p99_ms = 50007;

  // Backend override (optional)
  string backend = 50008;  // "postgres" | "kafka" | "sqlite" | etc.
}

extend google.protobuf.FieldOptions {
  // Index type
  string index = 50101;  // "primary" | "secondary" | "partition_key" | "clustering_key"

  // PII tagging
  string pii = 50102;  // "email" | "name" | "ssn" | etc.

  // Encryption
  bool encrypt_at_rest = 50103;
}
```

### Capacity Planner Algorithm

```rust
struct CapacityPlanner;

impl CapacityPlanner {
    fn plan(&self, config: &MessageConfig) -> InfrastructureSpec {
        // 1. Select backend based on access pattern
        let backend = self.select_backend(config);

        // 2. Calculate required capacity
        let capacity = match backend {
            Backend::Kafka => self.plan_kafka(config),
            Backend::Postgres => self.plan_postgres(config),
            Backend::Nats => self.plan_nats(config),
            // ...
        };

        // 3. Return infrastructure specification
        InfrastructureSpec {
            backend,
            capacity,
            policies: self.extract_policies(config),
        }
    }

    fn select_backend(&self, config: &MessageConfig) -> Backend {
        if let Some(explicit) = config.backend {
            return explicit;
        }

        match config.access_pattern {
            "append_heavy" => Backend::Kafka,
            "read_heavy" if config.supports_sql() => Backend::Postgres,
            "balanced" => Backend::Postgres,
            "graph" => Backend::Neptune,
            _ => Backend::Postgres, // Safe default
        }
    }

    fn plan_kafka(&self, config: &MessageConfig) -> KafkaCapacity {
        // Rule of thumb: 500 writes/sec per partition
        let partitions = (config.estimated_write_rps / 500).max(1);

        // Calculate retention storage
        let daily_data_mb = (config.estimated_write_rps * 86400 * config.avg_message_size_bytes) / 1_000_000;
        let retention_storage_gb = daily_data_mb * config.retention_days / 1000;

        KafkaCapacity {
            partitions,
            replication_factor: 3,  // Default for durability
            retention_storage_gb,
            instance_type: self.select_kafka_instance_type(config),
        }
    }
}
```

### Evolution Strategy

**Phase 1** (MVP): Support explicit backend selection
```protobuf
option (prism.backend) = "postgres";
```

**Phase 2**: Add access pattern hints
```protobuf
option (prism.access_pattern) = "read_heavy";
option (prism.estimated_read_rps) = "10000";
```

**Phase 3**: Automatic backend selection based on patterns

**Phase 4**: Continuous optimization using runtime metrics

## References

- Netflix Data Gateway Deployment Configuration
- [AWS Well-Architected Framework - Capacity Planning](https://wa.aws.amazon.com/)
- [Google SRE Book - Capacity Planning](https://sre.google/sre-book/handling-overload/)
- ADR-003: Protobuf as Single Source of Truth

## Revision History

- 2025-10-05: Initial draft and acceptance
