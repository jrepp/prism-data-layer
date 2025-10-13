---
author: Platform Team
created: 2025-10-10
doc_uuid: dc5657bb-d97b-43c6-a424-0bc2ce52f5ac
id: memo-011
project_id: prism-data-layer
tags:
- errors
- observability
- distributed-systems
- reliability
- best-practices
title: Distributed Error Handling Best Practices
updated: 2025-10-10
---

# MEMO-011: Distributed Error Handling Best Practices

## Purpose

Document comprehensive error handling best practices for distributed systems and explain the design of Prism's enhanced error proto (`prism.common.Error`).

## Problem Statement

Basic error messages like `Error { code: 500, message: "Internal error" }` are insufficient for distributed systems because:

1. **Lack of Context**: No information about where/when/why the error occurred
2. **Not Actionable**: Clients don't know if they should retry or give up
3. **Poor Observability**: Can't categorize, aggregate, or alert on errors effectively
4. **Debugging Difficulty**: Missing correlation IDs, trace context, and cause chains
5. **Backend Opacity**: Can't distinguish Redis errors from Kafka errors
6. **No Remediation**: No hints on how to fix the problem

## Solution: Rich Structured Errors

Prism's `Error` message captures distributed systems best practices from:
- Google's API error model (`google.rpc.Status`, `ErrorInfo`, `RetryInfo`)
- Stripe's error design (detailed, actionable)
- AWS error patterns (retryable classification, throttling guidance)
- gRPC error handling (status codes + rich details)

## Core Design Principles

### 1. Simple by Default, Rich When Needed

**Basic error** (minimal fields):
```protobuf
Error {
  code: ERROR_CODE_NOT_FOUND
  message: "Key 'user:12345' not found"
  request_id: "req-abc123"
}
```

**Rich error** (with full context):
```protobuf
Error {
  code: ERROR_CODE_BACKEND_ERROR
  message: "Redis connection timeout"
  request_id: "req-abc123"
  category: ERROR_CATEGORY_BACKEND_ERROR
  severity: ERROR_SEVERITY_ERROR
  timestamp: { seconds: 1696867200 }
  source: "prism-proxy-pod-3"
  namespace: "user-profiles"

  retry_policy: {
    retryable: true
    retry_after: { seconds: 5 }
    max_retries: 3
    backoff_strategy: BACKOFF_STRATEGY_EXPONENTIAL
    backoff_multiplier: 2.0
  }

  details: [{
    backend_error: {
      backend_type: "redis"
      backend_instance: "redis-master-1"
      backend_error_code: "ETIMEDOUT"
      backend_error_message: "Connection timeout after 5000ms"
      operation: "GET"
      pool_state: {
        active_connections: 50
        idle_connections: 0
        max_connections: 50
        wait_count: 12
        wait_duration: { seconds: 3 }
      }
    }
  }]

  help_links: [{
    title: "Troubleshooting Redis Timeouts"
    url: "https://docs.prism.io/troubleshooting/redis-timeout"
    link_type: "troubleshooting"
  }]
}
```

### 2. Machine-Readable and Human-Readable

**Machine-readable** (for clients to handle programmatically):
- `code` - HTTP-style error codes
- `category` - Classification for metrics/alerting
- `severity` - Impact level
- `retry_policy` - Actionable retry guidance

**Human-readable** (for developers debugging):
- `message` - Clear English description
- `help_links` - Links to documentation/runbooks
- `retry_policy.retry_advice` - Human-readable retry guidance

### 3. Error Chaining for Distributed Context

Errors can have **causes** (errors that led to this error):

```protobuf
Error {
  code: ERROR_CODE_GATEWAY_TIMEOUT
  message: "Pattern execution timed out"
  source: "prism-proxy"

  causes: [{
    code: ERROR_CODE_BACKEND_ERROR
    message: "Redis SET operation timed out"
    source: "redis-plugin"

    causes: [{
      code: ERROR_CODE_NETWORK_ERROR
      message: "Connection refused"
      source: "redis-client"
    }]
  }]
}
```

This enables **root cause analysis** across service boundaries.

### 4. Retry Guidance

Clients shouldn't guess whether to retry. The error tells them:

```protobuf
RetryPolicy {
  retryable: true                   // Yes, retry
  retry_after: { seconds: 10 }      // Wait 10 seconds
  max_retries: 3                    // Try up to 3 times
  backoff_strategy: EXPONENTIAL     // Use exponential backoff
  backoff_multiplier: 2.0           // Double delay each time
  retry_advice: "Backend is temporarily overloaded. Retry with exponential backoff."
}
```

**Non-retryable errors**:
```protobuf
RetryPolicy {
  retryable: false
  backoff_strategy: NEVER
  retry_advice: "Key validation failed. Fix the key format and retry."
}
```

### 5. Structured Error Details

Use `ErrorDetail` oneof for type-safe error information:

#### FieldViolation (validation errors)
```protobuf
field_violation: {
  field: "ttl_seconds"
  description: "TTL must be positive"
  invalid_value: "-100"
  constraint: "ttl_seconds > 0"
}
```

#### BackendError (backend-specific context)
```protobuf
backend_error: {
  backend_type: "kafka"
  backend_instance: "kafka-broker-2"
  backend_error_code: "OFFSET_OUT_OF_RANGE"
  backend_error_message: "Offset 12345 is out of range [0, 10000]"
  operation: "CONSUME"
}
```

#### PatternError (pattern-level semantics)
```protobuf
pattern_error: {
  pattern_type: "keyvalue"
  interface_name: "KeyValueTTLInterface"
  semantic_error: "TTL not supported by PostgreSQL backend"
  supported_operations: ["Set", "Get", "Delete", "Exists"]
}
```

#### QuotaViolation (rate limiting)
```protobuf
quota_violation: {
  dimension: "requests_per_second"
  current: 1500
  limit: 1000
  reset_time: { seconds: 1696867260 }  // 60 seconds from now
}
```

#### PreconditionFailure (CAS, version conflicts)
```protobuf
precondition_failure: {
  type: "ETAG_MISMATCH"
  field: "etag"
  expected: "abc123"
  actual: "def456"
  description: "Key was modified by another client"
}
```

### 6. Error Categorization for Observability

**ErrorCategory** enables metrics aggregation:
- `CLIENT_ERROR` - User made a mistake (400-level)
- `SERVER_ERROR` - Internal service failure (500-level)
- `BACKEND_ERROR` - Backend storage issue
- `NETWORK_ERROR` - Connectivity problem
- `TIMEOUT_ERROR` - Operation took too long
- `RATE_LIMIT_ERROR` - Quota exceeded
- `AUTHORIZATION_ERROR` - Permission denied
- `VALIDATION_ERROR` - Input validation failed
- `RESOURCE_ERROR` - Resource not found/unavailable
- `CONCURRENCY_ERROR` - Concurrent modification conflict

**Prometheus metrics**:
```text
prism_errors_total{category="backend_error", backend="redis", code="503"} 42
prism_errors_total{category="timeout_error", pattern="keyvalue", code="504"} 12
prism_errors_total{category="rate_limit_error", namespace="prod", code="429"} 156
```

**ErrorSeverity** for prioritization:
- `DEBUG` - Informational, no action needed
- `INFO` - Notable but expected (e.g., cache miss)
- `WARNING` - Degraded but functional
- `ERROR` - Operation failed, action may be needed
- `CRITICAL` - Severe failure, immediate action required

### 7. Traceability and Correlation

**Request ID**: Correlate errors across services
```protobuf
request_id: "req-abc123-def456"
```

**Source**: Which service generated the error
```protobuf
source: "prism-proxy-pod-3"
```

**Timestamp**: When the error occurred
```protobuf
timestamp: { seconds: 1696867200 }
```

**Debug Info** (development only):
```protobuf
debug_info: {
  trace_id: "abc123def456"
  span_id: "span-789"
  stack_entries: [
    "at handleRequest (proxy.rs:123)",
    "at executePattern (pattern.rs:456)",
    "at redisGet (redis.rs:789)"
  ]
}
```

### 8. Batch Error Handling

For batch operations, use `ErrorResponse`:

```protobuf
ErrorResponse {
  error: {
    code: ERROR_CODE_UNPROCESSABLE_ENTITY
    message: "Batch operation partially failed"
  }
  partial_success: true
  success_count: 8
  failure_count: 2

  item_errors: [
    {
      index: 3
      item_id: "user:12345"
      error: {
        code: ERROR_CODE_NOT_FOUND
        message: "Key not found"
      }
    },
    {
      index: 7
      item_id: "user:67890"
      error: {
        code: ERROR_CODE_PRECONDITION_FAILED
        message: "Version conflict"
      }
    }
  ]
}
```

## Error Code Design

### HTTP-Style Codes (Broad Compatibility)

**2xx Success**:
- `200 OK` - Success (shouldn't appear in errors)

**4xx Client Errors** (caller should fix):
- `400 BAD_REQUEST` - Invalid request syntax/parameters
- `401 UNAUTHORIZED` - Authentication required
- `403 FORBIDDEN` - Authenticated but not authorized
- `404 NOT_FOUND` - Resource doesn't exist
- `405 METHOD_NOT_ALLOWED` - Operation not supported
- `409 CONFLICT` - Resource state conflict
- `410 GONE` - Resource permanently deleted
- `412 PRECONDITION_FAILED` - Precondition not met (CAS)
- `413 PAYLOAD_TOO_LARGE` - Request exceeds size limits
- `422 UNPROCESSABLE_ENTITY` - Validation failed
- `429 TOO_MANY_REQUESTS` - Rate limit exceeded

**5xx Server Errors** (caller should retry):
- `500 INTERNAL_ERROR` - Unexpected internal error
- `501 NOT_IMPLEMENTED` - Feature not implemented
- `502 BAD_GATEWAY` - Upstream backend error
- `503 SERVICE_UNAVAILABLE` - Temporarily unavailable
- `504 GATEWAY_TIMEOUT` - Upstream timeout
- `507 INSUFFICIENT_STORAGE` - Backend storage full

**6xx Prism-Specific** (custom errors):
- `600 BACKEND_ERROR` - Backend-specific error
- `601 PATTERN_ERROR` - Pattern-level semantic error
- `602 INTERFACE_NOT_SUPPORTED` - Backend doesn't implement interface
- `603 SLOT_ERROR` - Pattern slot configuration error
- `604 CIRCUIT_BREAKER_OPEN` - Circuit breaker preventing requests

### Mapping to gRPC Status Codes

| HTTP Code | gRPC Status | Description |
|-----------|-------------|-------------|
| 400 | `INVALID_ARGUMENT` | Bad request |
| 401 | `UNAUTHENTICATED` | Missing auth |
| 403 | `PERMISSION_DENIED` | No permission |
| 404 | `NOT_FOUND` | Resource missing |
| 409 | `ALREADY_EXISTS` | Conflict |
| 412 | `FAILED_PRECONDITION` | Precondition failed |
| 429 | `RESOURCE_EXHAUSTED` | Rate limited |
| 500 | `INTERNAL` | Internal error |
| 501 | `UNIMPLEMENTED` | Not implemented |
| 503 | `UNAVAILABLE` | Service down |
| 504 | `DEADLINE_EXCEEDED` | Timeout |

## Usage Examples

### Example 1: Validation Error

```go
return &Error{
    Code: ErrorCode_ERROR_CODE_UNPROCESSABLE_ENTITY,
    Message: "Invalid key format",
    RequestId: requestID,
    Category: ErrorCategory_ERROR_CATEGORY_VALIDATION_ERROR,
    Severity: ErrorSeverity_ERROR_SEVERITY_ERROR,
    Source: "prism-proxy",
    Namespace: req.Namespace,
    RetryPolicy: &RetryPolicy{
        Retryable: false,
        BackoffStrategy: BackoffStrategy_BACKOFF_STRATEGY_NEVER,
        RetryAdvice: "Fix the key format and retry. Keys must match pattern: [a-zA-Z0-9:_-]+",
    },
    Details: []*ErrorDetail{{
        Detail: &ErrorDetail_FieldViolation{
            FieldViolation: &FieldViolation{
                Field: "key",
                Description: "Key contains invalid characters",
                InvalidValue: "user@#$%",
                Constraint: "key must match regex: [a-zA-Z0-9:_-]+",
            },
        },
    }},
    HelpLinks: []*ErrorLink{{
        Title: "Key Naming Conventions",
        Url: "https://docs.prism.io/keyvalue/naming",
        LinkType: "documentation",
    }},
}
```

### Example 2: Backend Connection Pool Exhaustion

```go
return &Error{
    Code: ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE,
    Message: "Redis connection pool exhausted",
    RequestId: requestID,
    Category: ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR,
    Severity: ErrorSeverity_ERROR_SEVERITY_CRITICAL,
    Timestamp: timestamppb.Now(),
    Source: "redis-plugin",
    Namespace: req.Namespace,
    RetryPolicy: &RetryPolicy{
        Retryable: true,
        RetryAfter: durationpb.New(10 * time.Second),
        MaxRetries: 5,
        BackoffStrategy: BackoffStrategy_BACKOFF_STRATEGY_EXPONENTIAL,
        BackoffMultiplier: 2.0,
        RetryAdvice: "Connection pool is full. Retry with exponential backoff.",
    },
    Details: []*ErrorDetail{{
        Detail: &ErrorDetail_BackendError{
            BackendError: &BackendError{
                BackendType: "redis",
                BackendInstance: "redis-master-1",
                BackendErrorCode: "POOL_EXHAUSTED",
                BackendErrorMessage: "All 50 connections in use",
                Operation: "GET",
                PoolState: &ConnectionPoolState{
                    ActiveConnections: 50,
                    IdleConnections: 0,
                    MaxConnections: 50,
                    WaitCount: 28,
                    WaitDuration: durationpb.New(5 * time.Second),
                },
            },
        },
    }},
    HelpLinks: []*ErrorLink{{
        Title: "Scaling Redis Connection Pools",
        Url: "https://docs.prism.io/backends/redis/connection-pools",
        LinkType: "documentation",
    }},
}
```

### Example 3: Interface Not Supported

```go
return &Error{
    Code: ErrorCode_ERROR_CODE_INTERFACE_NOT_SUPPORTED,
    Message: "TTL operations not supported by PostgreSQL backend",
    RequestId: requestID,
    Category: ErrorCategory_ERROR_CATEGORY_CLIENT_ERROR,
    Severity: ErrorSeverity_ERROR_SEVERITY_ERROR,
    Source: "postgres-plugin",
    Namespace: req.Namespace,
    RetryPolicy: &RetryPolicy{
        Retryable: false,
        BackoffStrategy: BackoffStrategy_BACKOFF_STRATEGY_NEVER,
        RetryAdvice: "Use Redis or DynamoDB for TTL support",
    },
    Details: []*ErrorDetail{{
        Detail: &ErrorDetail_PatternError{
            PatternError: &PatternError{
                PatternType: "keyvalue",
                InterfaceName: "KeyValueTTLInterface",
                SemanticError: "PostgreSQL does not implement TTL interface",
                SupportedOperations: []string{"Set", "Get", "Delete", "Exists", "BatchSet", "BatchGet"},
            },
        },
    }},
    HelpLinks: []*ErrorLink{{
        Title: "Backend Interface Support Matrix",
        Url: "https://docs.prism.io/backends/interface-matrix",
        LinkType: "documentation",
    }},
}
```

### Example 4: Rate Limit Exceeded

```go
return &Error{
    Code: ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS,
    Message: "Rate limit exceeded for namespace 'user-profiles'",
    RequestId: requestID,
    Category: ErrorCategory_ERROR_CATEGORY_RATE_LIMIT_ERROR,
    Severity: ErrorSeverity_ERROR_SEVERITY_WARNING,
    Timestamp: timestamppb.Now(),
    Source: "prism-proxy",
    Namespace: "user-profiles",
    RetryPolicy: &RetryPolicy{
        Retryable: true,
        RetryAfter: durationpb.New(60 * time.Second),
        MaxRetries: 10,
        BackoffStrategy: BackoffStrategy_BACKOFF_STRATEGY_LINEAR,
        RetryAdvice: "Wait 60 seconds for quota reset",
    },
    Details: []*ErrorDetail{{
        Detail: &ErrorDetail_QuotaViolation{
            QuotaViolation: &QuotaViolation{
                Dimension: "requests_per_second",
                Current: 1500,
                Limit: 1000,
                ResetTime: timestamppb.New(time.Now().Add(60 * time.Second)),
            },
        },
    }},
    HelpLinks: []*ErrorLink{{
        Title: "Rate Limiting Policies",
        Url: "https://docs.prism.io/quotas-and-limits",
        LinkType: "documentation",
    }},
}
```

## Error Handling Patterns

### Pattern 1: Client Retry Logic

```go
func retryWithBackoff(req *Request, maxRetries int) (*Response, error) {
    var lastErr *Error

    for attempt := 0; attempt <= maxRetries; attempt++ {
        resp, err := client.Call(req)
        if err == nil {
            return resp, nil
        }

        // Extract Prism error
        lastErr = extractPrismError(err)

        // Check if retryable
        if lastErr.RetryPolicy == nil || !lastErr.RetryPolicy.Retryable {
            return nil, fmt.Errorf("non-retryable error: %s", lastErr.Message)
        }

        // Respect max retries from server
        if lastErr.RetryPolicy.MaxRetries > 0 && attempt >= int(lastErr.RetryPolicy.MaxRetries) {
            break
        }

        // Calculate backoff delay
        delay := calculateBackoff(lastErr.RetryPolicy, attempt)

        log.Warn("Retrying after error",
            "attempt", attempt,
            "error", lastErr.Message,
            "delay", delay)

        time.Sleep(delay)
    }

    return nil, fmt.Errorf("max retries exceeded: %s", lastErr.Message)
}

func calculateBackoff(policy *RetryPolicy, attempt int) time.Duration {
    baseDelay := policy.RetryAfter.AsDuration()

    switch policy.BackoffStrategy {
    case BackoffStrategy_BACKOFF_STRATEGY_IMMEDIATE:
        return 0
    case BackoffStrategy_BACKOFF_STRATEGY_LINEAR:
        return baseDelay * time.Duration(attempt+1)
    case BackoffStrategy_BACKOFF_STRATEGY_EXPONENTIAL:
        return baseDelay * time.Duration(math.Pow(policy.BackoffMultiplier, float64(attempt)))
    case BackoffStrategy_BACKOFF_STRATEGY_JITTER:
        exp := baseDelay * time.Duration(math.Pow(policy.BackoffMultiplier, float64(attempt)))
        jitter := time.Duration(rand.Int63n(int64(exp / 2)))
        return exp + jitter
    default:
        return baseDelay
    }
}
```

### Pattern 2: Structured Logging

```go
func logError(err *Error) {
    logger.Error("Request failed",
        "code", err.Code.String(),
        "category", err.Category.String(),
        "severity", err.Severity.String(),
        "message", err.Message,
        "request_id", err.RequestId,
        "source", err.Source,
        "namespace", err.Namespace,
        "timestamp", err.Timestamp.AsTime(),
        "retryable", err.RetryPolicy != nil && err.RetryPolicy.Retryable,
    )

    // Log backend-specific details
    for _, detail := range err.Details {
        if backendErr := detail.GetBackendError(); backendErr != nil {
            logger.Error("Backend error details",
                "backend", backendErr.BackendType,
                "instance", backendErr.BackendInstance,
                "backend_code", backendErr.BackendErrorCode,
                "operation", backendErr.Operation,
            )
        }
    }

    // Log cause chain
    for i, cause := range err.Causes {
        logger.Error("Error cause",
            "depth", i+1,
            "message", cause.Message,
            "source", cause.Source,
        )
    }
}
```

### Pattern 3: Prometheus Metrics

```go
var (
    errorCounter = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "prism_errors_total",
            Help: "Total number of errors by category, code, and backend",
        },
        []string{"category", "code", "backend", "namespace"},
    )

    errorSeverity = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "prism_errors_by_severity",
            Help: "Errors by severity level",
        },
        []string{"severity", "namespace"},
    )
)

func recordError(err *Error) {
    backend := extractBackendType(err)

    errorCounter.WithLabelValues(
        err.Category.String(),
        strconv.Itoa(int(err.Code)),
        backend,
        err.Namespace,
    ).Inc()

    errorSeverity.WithLabelValues(
        err.Severity.String(),
        err.Namespace,
    ).Inc()
}
```

## Best Practices Summary

### DO:
✅ Use structured error details (`ErrorDetail` oneof)
✅ Provide retry guidance (`RetryPolicy`)
✅ Chain errors across services (`causes`)
✅ Include correlation IDs (`request_id`)
✅ Add help links for common errors
✅ Set appropriate severity levels
✅ Populate backend context (`BackendError`)
✅ Use semantic error codes (not just 500)
✅ Sanitize sensitive data from error messages
✅ Log errors with structured fields

### DON'T:
❌ Return generic "Internal error" without context
❌ Expose internal implementation details to clients
❌ Use string error codes (use enums)
❌ Forget to set `category` and `severity`
❌ Include stack traces in production responses
❌ Make all errors retryable (guide clients)
❌ Leak backend credentials or internal IPs
❌ Use HTTP status codes incorrectly
❌ Ignore error cause chains
❌ Skip setting `namespace` for multi-tenant errors

## Related Documents

- [RFC-001: Prism Architecture](/rfc/rfc-001) - Overall architecture
- [MEMO-006: Backend Interface Decomposition](/memos/memo-006) - Interface design
- [Google API Error Model](https://cloud.google.com/apis/design/errors) - Industry best practices
- [gRPC Error Handling](https://grpc.io/docs/guides/error/) - gRPC status codes

## Revision History

- 2025-10-10: Initial draft with comprehensive error proto design