---
id: adr-002
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
- **Organizational Scalability**: Infrastructure team doesn't become bottleneck as company grows

### Negative

- **More Complex Tooling**: Capacity planner must be sophisticated
  - *Mitigation*: Start with conservative heuristics; refine over time
- **Protobuf Coupling**: Configuration embedded in data models
  - *Mitigation*: This is intentional; keeps requirements close to code
- **Requires Estimation**: Teams must estimate RPS, data size
  - *Mitigation*: Provide estimation tools; Prism adapts based on actual metrics
- **Configuration Authority**: Need authorization boundaries to prevent misuse
  - *Mitigation*: Policy-driven configuration limits (see Organizational Scalability section)

### Neutral

- **Shifts Responsibility**: From infra team to app teams
  - Some teams will prefer this (autonomy)
  - Others may miss having an expert provision for them
  - *Plan*: Provide templates and examples for common patterns

## Organizational Scalability and Authorization Boundaries

### The Scalability Challenge

As organizations grow, traditional manual provisioning breaks down:

| Organization Size | Manual Provisioning Model | Bottleneck |
|-------------------|---------------------------|------------|
| **Startup (1-5 teams)** | 1 DBA provisions all databases | Works initially |
| **Growing (10-20 teams)** | 2-3 DBAs, ticket queue | 1-2 week delays |
| **Scale (50+ teams)** | 5-10 DBAs, complex approval process | 2-4 week delays, team burnout |
| **Large (500+ teams)** | 20+ DBAs, dedicated infrastructure org | Infrastructure team larger than feature teams |

**Client-originated configuration solves this**: Infrastructure team size remains **constant** (maintain Prism platform) while application teams scale **linearly**.

**Key Insight**: Client configurability is essential for organizational scalability, but requires authorization boundaries to prevent misuse.

### Authorization Boundaries: Expressibility vs Security/Reliability

**The Tension**: Allow teams enough expressibility to move fast, but prevent configurations that compromise security or reliability.

**Guiding Principles**:
1. **Default to Safe**: Conservative defaults prevent common misconfigurations
2. **Progressive Permission**: Teams earn more configurability through demonstrated responsibility
3. **Policy as Code**: Configuration limits defined in version-controlled policies
4. **Fail Loudly**: Invalid configurations rejected at deploy-time, not runtime

### Configuration Permission Levels

**Level 1: Guided (Default for All Teams)**
- ✅ **Allowed**: Choose from pre-approved backends (Postgres, Kafka, Redis)
- ✅ **Allowed**: Set access patterns (`read_heavy`, `write_heavy`, `balanced`)
- ✅ **Allowed**: Declare capacity estimates (within reasonable bounds)
- ✅ **Allowed**: Configure retention (up to organization maximum)
- ❌ **Restricted**: Backend-specific tuning parameters
- ❌ **Restricted**: Replication factors, partition counts

**Example**:
```protobuf
message UserEvents {
  option (prism.backend) = "kafka";           // ✅ Allowed
  option (prism.access_pattern) = "append_heavy";  // ✅ Allowed
  option (prism.estimated_write_rps) = "10000";    // ✅ Allowed (within limits)
  option (prism.retention_days) = "90";            // ✅ Allowed (< 180 day max)
}
```

**Level 2: Advanced (Requires Platform Team Approval)**
- ✅ **Allowed**: All Level 1 permissions
- ✅ **Allowed**: Backend-specific tuning (e.g., Kafka partition count)
- ✅ **Allowed**: Custom replication factors
- ✅ **Allowed**: Extended retention (up to 1 year)
- ❌ **Restricted**: Cross-region replication
- ❌ **Restricted**: Encryption key management overrides

**Example**:
```protobuf
message HighThroughputLogs {
  option (prism.backend) = "kafka";
  option (prism.kafka_partitions) = "50";          // ✅ Advanced permission required
  option (prism.kafka_replication_factor) = "5";   // ✅ Advanced permission required
  option (prism.retention_days) = "365";           // ✅ Advanced permission required
}
```

**Level 3: Expert (Platform Team Only)**
- ✅ **Allowed**: All Level 1 & 2 permissions
- ✅ **Allowed**: Cross-region replication
- ✅ **Allowed**: Custom encryption keys (BYOK)
- ✅ **Allowed**: Low-level performance tuning
- ✅ **Allowed**: Override safety limits

### Policy Enforcement Mechanism

**Configuration Validation at Deploy Time**:

```rust
pub struct ConfigurationValidator {
    policies: HashMap<String, TeamPolicy>,
}

pub struct TeamPolicy {
    team_name: String,
    permission_level: PermissionLevel,
    limits: ConfigurationLimits,
}

pub struct ConfigurationLimits {
    max_write_rps: i64,
    max_read_rps: i64,
    max_retention_days: i32,
    max_data_size_gb: i64,
    allowed_backends: Vec<String>,
    backend_specific_tuning: bool,
}

impl ConfigurationValidator {
    pub fn validate(&self, config: &MessageConfig, team: &str) -> Result<(), ValidationError> {
        let policy = self.policies.get(team)
            .ok_or(ValidationError::UnknownTeam(team.to_string()))?;

        let limits = &policy.limits;

        // Check RPS within limits
        if config.estimated_write_rps > limits.max_write_rps {
            return Err(ValidationError::ExceedsLimit {
                field: "estimated_write_rps",
                value: config.estimated_write_rps,
                max: limits.max_write_rps,
                message: format!(
                    "Team {} limited to {}k writes/sec. Request platform team approval for higher capacity.",
                    team, limits.max_write_rps / 1000
                ),
            });
        }

        // Check retention within limits
        if config.retention_days > limits.max_retention_days {
            return Err(ValidationError::ExceedsLimit {
                field: "retention_days",
                value: config.retention_days,
                max: limits.max_retention_days,
                message: format!(
                    "Team {} limited to {} day retention. Longer retention requires compliance review.",
                    team, limits.max_retention_days
                ),
            });
        }

        // Check backend in allowed list
        if let Some(backend) = &config.backend {
            if !limits.allowed_backends.contains(backend) {
                return Err(ValidationError::DisallowedBackend {
                    backend: backend.clone(),
                    allowed: limits.allowed_backends.clone(),
                    message: format!(
                        "Backend '{}' not approved for team {}. Allowed backends: {}",
                        backend, team, limits.allowed_backends.join(", ")
                    ),
                });
            }
        }

        // Check backend-specific tuning permissions
        if config.has_backend_tuning() && !limits.backend_specific_tuning {
            return Err(ValidationError::PermissionDenied {
                field: "backend tuning parameters",
                message: format!(
                    "Team {} does not have permission for backend-specific tuning. Request 'Advanced' permission level.",
                    team
                ),
            });
        }

        Ok(())
    }
}
```

**Example Policy Configuration** (`policies/teams.yaml`):

```yaml
teams:
  # Most teams start here
  - name: user-platform-team
    permission_level: guided
    limits:
      max_write_rps: 50000
      max_read_rps: 100000
      max_retention_days: 180
      max_data_size_gb: 1000
      allowed_backends: [postgres, kafka, redis]
      backend_specific_tuning: false

  # Teams with demonstrated expertise
  - name: data-infrastructure-team
    permission_level: advanced
    limits:
      max_write_rps: 500000
      max_read_rps: 1000000
      max_retention_days: 365
      max_data_size_gb: 10000
      allowed_backends: [postgres, kafka, redis, nats, clickhouse]
      backend_specific_tuning: true

  # Platform team has unrestricted access
  - name: platform-team
    permission_level: expert
    limits:
      max_write_rps: unlimited
      max_read_rps: unlimited
      max_retention_days: unlimited
      max_data_size_gb: unlimited
      allowed_backends: [all]
      backend_specific_tuning: true
      cross_region_replication: true
      custom_encryption_keys: true
```

### Permission Escalation Workflow

**Scenario**: Team needs higher capacity than allowed by policy.

**Workflow**:
1. Team deploys configuration with `estimated_write_rps: 100000`
2. Validation fails: `Team user-platform-team limited to 50k writes/sec`
3. Team opens request: "Increase RPS limit to 100k for user-events namespace"
4. Platform team reviews:
   - Is the estimate reasonable? (check current metrics)
   - Will this impact cluster capacity? (check resource availability)
   - Is the backend choice optimal? (suggest alternatives if not)
5. If approved, update `policies/teams.yaml`:
   ```yaml
   - name: user-platform-team
     permission_level: guided
     limits:
       max_write_rps: 100000  # ← Increased
   ```
6. Team redeploys successfully

**Key Benefits**:
- **Audit Trail**: All permission changes version-controlled
- **Gradual Escalation**: Teams earn trust over time
- **Central Oversight**: Platform team maintains visibility
- **Fast Approval**: Simple cases auto-approved via policy updates

### Common Configuration Mistakes Prevented

**1. Excessive Retention Leading to Cost Overruns**
```protobuf
// ❌ Rejected at deploy time
message DebugLogs {
  option (prism.retention_days) = "3650";  // 10 years!
  // Error: Team limited to 180 days. Compliance review required for >1 year retention.
}
```

**2. Wrong Backend for Access Pattern**
```protobuf
// ⚠️ Warning at deploy time
message HighThroughputEvents {
  option (prism.backend) = "postgres";
  option (prism.access_pattern) = "append_heavy";
  option (prism.estimated_write_rps) = "50000";
  // Warning: Postgres may struggle with 50k writes/sec. Consider Kafka for append-heavy workloads.
}
```

**3. Over-Provisioning Resources**
```protobuf
// ❌ Rejected at deploy time
message UserSessions {
  option (prism.estimated_write_rps) = "100000";
  option (prism.kafka_partitions) = "500";  // Way too many!
  // Error: 500 partitions for 100k writes/sec is excessive. Recommended: 200 partitions (500 writes/partition/sec).
}
```

### Organizational Benefits

**Before Client-Originated Configuration**:
- Infrastructure team: 10 people
- Application teams: 50 teams (500 engineers)
- Bottleneck: 2-4 week provisioning delays
- Cost: Infrastructure team growth required to scale

**After Client-Originated Configuration with Authorization Boundaries**:
- Infrastructure team: 10 people (maintain Prism platform)
- Application teams: 50 teams (self-service)
- Bottleneck: Eliminated for 90% of requests, escalation path for 10%
- Cost: Infrastructure team size **stays constant**

**Scaling Math**:
- Without Prism: 1 DBA per 10 teams → 50 teams needs 5 DBAs
- With Prism: Platform team of 10 supports 500+ teams (50x improvement)

### Future Enhancements

**Automated Permission Elevation**:
```yaml
auto_approve_conditions:
  - if: team.track_record > 6_months && team.incidents == 0
    then: grant permission_level: advanced

  - if: config.estimated_write_rps < current_metrics.write_rps * 1.5
    then: auto_approve  # Only 50% increase, low risk
```

**Cost Budgeting Integration**:
```protobuf
message ExpensiveData {
  option (prism.estimated_cost_per_month) = 5000;  // $5k/month
  option (prism.team_budget_limit) = 10000;        // $10k/month
  // Auto-approved if within budget, requires approval if over
}
```

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
