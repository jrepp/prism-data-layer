# FR-004: PII Handling & Compliance

**Status**: Draft

**Priority**: P0 (Critical - regulatory requirement)

**Owner**: Security + Core Team

## Overview

Prism must handle Personally Identifiable Information (PII) in compliance with GDPR, CCPA, and other privacy regulations. PII data requires special handling for encryption, masking, audit logging, and deletion.

## Stakeholders

- **Primary**: Application developers storing user data
- **Secondary**: Compliance team, security team, end users

## User Stories

**As an application developer**, I want to mark fields as PII in my protobuf definitions, so that Prism automatically handles encryption, masking, and audit logging.

**As a compliance officer**, I want to audit all access to PII data, so that I can demonstrate regulatory compliance.

**As an end user**, I want my data to be deleted when I request it, so that my privacy rights are respected.

**As a security engineer**, I want PII to be encrypted at rest and in transit, so that data breaches have minimal impact.

## Functional Details

### PII Classification

Use protobuf field options to declare PII:

```protobuf
import "prism/options.proto";

message UserProfile {
  string user_id = 1;  // Not PII

  string email = 2 [
    (prism.pii) = "email",
    (prism.encrypt_at_rest) = true
  ];

  string full_name = 3 [
    (prism.pii) = "name",
    (prism.mask_in_logs) = true
  ];

  string phone = 4 [
    (prism.pii) = "phone",
    (prism.encrypt_at_rest) = true
  ];

  string ssn = 5 [
    (prism.pii) = "ssn",
    (prism.encrypt_at_rest) = true,
    (prism.mask_in_logs) = true,
    (prism.access_audit) = true  // Log every read
  ];

  string ip_address = 6 [
    (prism.pii) = "ip_address"
  ];
}
```

### PII Categories

Standard PII types supported:

| Type | Description | Default Encryption | Default Masking |
|------|-------------|-------------------|-----------------|
| `email` | Email addresses | Yes | Yes |
| `name` | Person names | Optional | Yes |
| `phone` | Phone numbers | Yes | Yes |
| `ssn` | Social Security Numbers | Yes | Yes |
| `address` | Street addresses | Yes | Yes |
| `ip_address` | IP addresses | No | Partial (last octet) |
| `credit_card` | Payment card numbers | Yes | Yes (PCI-DSS) |
| `custom` | Application-specific | Configurable | Configurable |

### Automatic Encryption

**Encryption at Rest**:

```rust
// Application writes plaintext
let profile = UserProfile {
    user_id: "user123",
    email: "alice@example.com",  // Plaintext in app
    ssn: "123-45-6789",           // Plaintext in app
    ..
};

prism_client.put("user123", &profile).await?;

// Prism automatically encrypts before writing to backend
// Database stores:
// {
//   user_id: "user123",
//   email: "ENCRYPTED:AES256:dGVzdCBlbmNyeXB0ZWQgZW1haWw=",
//   ssn: "ENCRYPTED:AES256:ZW5jcnlwdGVkIHNzbg==",
//   ...
// }

// On read, Prism decrypts automatically
let profile = prism_client.get::<UserProfile>("user123").await?;
assert_eq!(profile.email, "alice@example.com");  // Plaintext again
```

**Encryption Implementation**:
- Algorithm: AES-256-GCM
- Key Management: AWS KMS or HashiCorp Vault
- Key Rotation: Automatic, configurable interval
- Per-field encryption (not full-record)

**Encryption in Transit**:
- mTLS for all connections (client ↔ proxy ↔ backend)
- No unencrypted PII ever sent over the wire

### Automatic Masking

**In Logs**:

```rust
// Without masking
log::info!("User profile: {:?}", profile);
// Output: User profile: UserProfile { email: "alice@example.com", ssn: "123-45-6789" }

// With automatic masking
log::info!("User profile: {:?}", profile);
// Output: User profile: UserProfile { email: "a***e@example.com", ssn: "***-**-****" }
```

**In Metrics/Traces**:
- PII fields automatically redacted from Prometheus labels
- Trace spans never contain PII values
- Only record non-PII identifiers (user_id, request_id)

**Masking Strategies**:

| PII Type | Masking Strategy |
|----------|-----------------|
| Email | Show first + last char, domain: `a***e@example.com` |
| Name | Show first name, mask last: `Alice S****` |
| Phone | Mask all but last 4: `***-***-1234` |
| SSN | Mask all: `***-**-****` |
| Credit Card | Last 4 only: `**** **** **** 1234` |
| IP Address | Mask last octet: `192.168.1.***` |

### Audit Logging

**Automatic Audit Trails**:

Every access to a field marked with `(prism.access_audit) = true` generates an audit log:

```json
{
  "timestamp": "2025-10-05T12:34:56Z",
  "event": "pii_access",
  "actor": {
    "service": "user-service",
    "user_id": "service-account-123",
    "ip_address": "10.0.1.5"
  },
  "target": {
    "namespace": "user-profiles",
    "record_id": "user123",
    "field": "ssn",
    "pii_type": "ssn"
  },
  "operation": "read",
  "justification": "user-initiated profile view",
  "request_id": "req-abc-123"
}
```

**Audit Log Storage**:
- Immutable append-only log
- Stored in separate, secure backend (S3 with write-once-read-many)
- Retained for compliance period (7 years typical)
- Indexed for compliance queries

**Audit Queries**:

```rust
// Find all SSN accesses in last 30 days
let events = prism.audit_log()
    .query()
    .field("ssn")
    .since(30.days().ago())
    .execute()
    .await?;

// Who accessed user123's data?
let events = prism.audit_log()
    .query()
    .record_id("user123")
    .since(90.days().ago())
    .execute()
    .await?;
```

### Data Deletion (Right to be Forgotten)

**API**:

```rust
// Delete all data for a user
prism_client.delete_subject("user123").await?;

// This:
// 1. Deletes all records where user_id = "user123" across all namespaces
// 2. Creates audit log entry
// 3. Schedules hard-delete (overwrite with random data) after retention period
// 4. Returns confirmation with deletion ID
```

**Hard Delete Process**:
1. Soft delete: Mark record as deleted, hide from queries
2. Audit log: Record deletion request
3. Grace period: 30 days for backup recovery
4. Hard delete: Overwrite with random bytes
5. Confirmation: Return deletion certificate

**Backend-Specific Handling**:

| Backend | Deletion Strategy |
|---------|------------------|
| PostgreSQL | `DELETE` + `VACUUM FULL` |
| Kafka | Tombstone message + compaction |
| S3 | Versioned delete + lifecycle policy |
| Neptune | Vertex/edge deletion |

## Acceptance Criteria

- [ ] PII field options defined in `prism/options.proto`
- [ ] Encryption/decryption transparent to applications
- [ ] All PII types have masking implementations
- [ ] Audit log captures every PII access
- [ ] `delete_subject` API works for all backends
- [ ] Compliance queries run in < 1 second for 1M records
- [ ] Documentation includes compliance report examples

## Dependencies

- **ADR-003**: Protobuf field options drive PII handling
- **FR-002**: Authentication needed to identify actors in audit logs
- **FR-003**: Audit logging infrastructure
- **NFR-002**: High availability for compliance systems

## Implementation Notes

### Key Management

Use AWS KMS (or equivalent) for encryption keys:

```rust
use aws_sdk_kms::Client as KmsClient;

struct PiiEncryption {
    kms_client: KmsClient,
    key_id: String,
}

impl PiiEncryption {
    async fn encrypt(&self, plaintext: &[u8]) -> Result<Vec<u8>> {
        let result = self.kms_client
            .encrypt()
            .key_id(&self.key_id)
            .plaintext(Blob::new(plaintext))
            .send()
            .await?;

        Ok(result.ciphertext_blob.unwrap().into_inner())
    }

    async fn decrypt(&self, ciphertext: &[u8]) -> Result<Vec<u8>> {
        let result = self.kms_client
            .decrypt()
            .ciphertext_blob(Blob::new(ciphertext))
            .send()
            .await?;

        Ok(result.plaintext.unwrap().into_inner())
    }
}
```

### Code Generation for PII Handling

Generate encryption/decryption code from protobuf:

```rust
// Generated code
impl UserProfile {
    pub async fn encrypt_pii(&mut self, kms: &PiiEncryption) -> Result<()> {
        // Encrypt email field
        if !self.email.is_empty() {
            let encrypted = kms.encrypt(self.email.as_bytes()).await?;
            self.email = format!("ENCRYPTED:AES256:{}", base64::encode(&encrypted));
        }

        // Encrypt ssn field
        if !self.ssn.is_empty() {
            let encrypted = kms.encrypt(self.ssn.as_bytes()).await?;
            self.ssn = format!("ENCRYPTED:AES256:{}", base64::encode(&encrypted));
        }

        Ok(())
    }

    pub fn mask_pii_for_logs(&self) -> Self {
        let mut masked = self.clone();
        masked.email = mask_email(&self.email);
        masked.ssn = mask_ssn(&self.ssn);
        masked
    }
}
```

### Performance Considerations

- **Encryption overhead**: ~100μs per field with KMS
  - *Mitigation*: Cache data keys; use envelope encryption
- **Audit log writes**: ~1ms per access
  - *Mitigation*: Async writes; batch audit logs

## Open Questions

1. **Should we support user-provided encryption keys?**
   - Pros: Customer control
   - Cons: Key management complexity
   - *Proposal*: Support in v2; KMS only for v1

2. **How long do we retain audit logs?**
   - Legal requirement varies by jurisdiction
   - *Proposal*: Configurable, default 7 years

3. **What about derived PII (anonymized)?**
   - Hashed emails still considered PII in GDPR
   - *Proposal*: Provide anonymization utilities, but not automatic

4. **Do we support PII in Kafka keys?**
   - Encryption makes keys unsearchable
   - *Proposal*: Warn against it; use surrogate IDs

## References

- [GDPR](https://gdpr.eu/)
- [CCPA](https://oag.ca.gov/privacy/ccpa)
- [AWS KMS Best Practices](https://docs.aws.amazon.com/kms/latest/developerguide/best-practices.html)
- [Envelope Encryption](https://docs.aws.amazon.com/wellarchitected/latest/financial-services-industry-lens/use-envelope-encryption-with-customer-master-keys.html)

## Revision History

- 2025-10-05: Initial draft
