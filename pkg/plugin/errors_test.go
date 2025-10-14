package plugin

import (
	"testing"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewError(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
		WithMessage("Resource not found").
		Build()

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_FOUND, err.Code)
	assert.Equal(t, "Resource not found", err.Message)
	assert.Equal(t, pb.ErrorSeverity_ERROR_SEVERITY_ERROR, err.Severity) // Default severity
	assert.NotNil(t, err.Timestamp)
}

func TestErrorBuilder_WithMessagef(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
		WithMessagef("Key '%s' not found in namespace '%s'", "user:123", "prod").
		Build()

	assert.Equal(t, "Key 'user:123' not found in namespace 'prod'", err.Message)
}

func TestErrorBuilder_WithRequestID(t *testing.T) {
	requestID := "req-abc123"
	err := NewError(pb.ErrorCode_ERROR_CODE_INTERNAL_ERROR).
		WithRequestID(requestID).
		Build()

	assert.Equal(t, requestID, err.RequestId)
}

func TestErrorBuilder_WithCategoryAndSeverity(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_CRITICAL).
		Build()

	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR, err.Category)
	assert.Equal(t, pb.ErrorSeverity_ERROR_SEVERITY_CRITICAL, err.Severity)
}

func TestErrorBuilder_WithTimestamp(t *testing.T) {
	customTime := time.Date(2025, 10, 10, 12, 0, 0, 0, time.UTC)
	err := NewError(pb.ErrorCode_ERROR_CODE_INTERNAL_ERROR).
		WithTimestamp(customTime).
		Build()

	assert.Equal(t, customTime.Unix(), err.Timestamp.Seconds)
}

func TestErrorBuilder_WithSourceAndNamespace(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		WithSource("redis-plugin").
		WithNamespace("user-profiles").
		Build()

	assert.Equal(t, "redis-plugin", err.Source)
	assert.Equal(t, "user-profiles", err.Namespace)
}

func TestErrorBuilder_Retryable(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		Retryable(5*time.Second, 3, pb.BackoffStrategy_BACKOFF_STRATEGY_EXPONENTIAL).
		Build()

	require.NotNil(t, err.RetryPolicy)
	assert.True(t, err.RetryPolicy.Retryable)
	assert.Equal(t, int64(5), err.RetryPolicy.RetryAfter.Seconds)
	assert.Equal(t, int32(3), err.RetryPolicy.MaxRetries)
	assert.Equal(t, pb.BackoffStrategy_BACKOFF_STRATEGY_EXPONENTIAL, err.RetryPolicy.BackoffStrategy)
	assert.Equal(t, 2.0, err.RetryPolicy.BackoffMultiplier) // Default multiplier
}

func TestErrorBuilder_RetryableExponential(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		RetryableExponential(5*time.Second, 5, 3.0).
		Build()

	require.NotNil(t, err.RetryPolicy)
	assert.True(t, err.RetryPolicy.Retryable)
	assert.Equal(t, 3.0, err.RetryPolicy.BackoffMultiplier)
	assert.Contains(t, err.RetryPolicy.RetryAdvice, "exponential backoff")
	assert.Contains(t, err.RetryPolicy.RetryAdvice, "3.0x")
}

func TestErrorBuilder_NonRetryable(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
		NonRetryable().
		Build()

	require.NotNil(t, err.RetryPolicy)
	assert.False(t, err.RetryPolicy.Retryable)
	assert.Equal(t, pb.BackoffStrategy_BACKOFF_STRATEGY_NEVER, err.RetryPolicy.BackoffStrategy)
}

func TestErrorBuilder_WithRetryAdvice(t *testing.T) {
	advice := "Wait for backend to recover"
	err := NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		Retryable(5*time.Second, 3, pb.BackoffStrategy_BACKOFF_STRATEGY_EXPONENTIAL).
		WithRetryAdvice(advice).
		Build()

	require.NotNil(t, err.RetryPolicy)
	assert.Equal(t, advice, err.RetryPolicy.RetryAdvice)
}

func TestErrorBuilder_AddFieldViolation(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_UNPROCESSABLE_ENTITY).
		AddFieldViolation("ttl_seconds", "TTL must be positive", "-100", "ttl_seconds > 0").
		Build()

	require.Len(t, err.Details, 1)
	fv := err.Details[0].GetFieldViolation()
	require.NotNil(t, fv)
	assert.Equal(t, "ttl_seconds", fv.Field)
	assert.Equal(t, "TTL must be positive", fv.Description)
	assert.Equal(t, "-100", fv.InvalidValue)
	assert.Equal(t, "ttl_seconds > 0", fv.Constraint)
}

func TestErrorBuilder_AddBackendError(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		AddBackendError("redis", "redis-master-1", "ETIMEDOUT", "Connection timeout", "GET").
		Build()

	require.Len(t, err.Details, 1)
	be := err.Details[0].GetBackendError()
	require.NotNil(t, be)
	assert.Equal(t, "redis", be.BackendType)
	assert.Equal(t, "redis-master-1", be.BackendInstance)
	assert.Equal(t, "ETIMEDOUT", be.BackendErrorCode)
	assert.Equal(t, "Connection timeout", be.BackendErrorMessage)
	assert.Equal(t, "GET", be.Operation)
}

func TestErrorBuilder_AddBackendErrorWithPool(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		AddBackendErrorWithPool(
			"redis", "redis-master-1",
			"POOL_EXHAUSTED", "All connections in use",
			"SET",
			50, 0, 50, 12,
			3*time.Second,
		).
		Build()

	require.Len(t, err.Details, 1)
	be := err.Details[0].GetBackendError()
	require.NotNil(t, be)
	require.NotNil(t, be.PoolState)

	ps := be.PoolState
	assert.Equal(t, int32(50), ps.ActiveConnections)
	assert.Equal(t, int32(0), ps.IdleConnections)
	assert.Equal(t, int32(50), ps.MaxConnections)
	assert.Equal(t, int32(12), ps.WaitCount)
	assert.Equal(t, int64(3), ps.WaitDuration.Seconds)
}

func TestErrorBuilder_AddPatternError(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_INTERFACE_NOT_SUPPORTED).
		AddPatternError(
			"keyvalue",
			"KeyValueTTLInterface",
			"TTL not supported by PostgreSQL",
			"Set", "Get", "Delete",
		).
		Build()

	require.Len(t, err.Details, 1)
	pe := err.Details[0].GetPatternError()
	require.NotNil(t, pe)
	assert.Equal(t, "keyvalue", pe.PatternType)
	assert.Equal(t, "KeyValueTTLInterface", pe.InterfaceName)
	assert.Equal(t, "TTL not supported by PostgreSQL", pe.SemanticError)
	assert.Equal(t, []string{"Set", "Get", "Delete"}, pe.SupportedOperations)
}

func TestErrorBuilder_AddQuotaViolation(t *testing.T) {
	resetTime := time.Now().Add(60 * time.Second)
	err := NewError(pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS).
		AddQuotaViolation("requests_per_second", 1500, 1000, resetTime).
		Build()

	require.Len(t, err.Details, 1)
	qv := err.Details[0].GetQuotaViolation()
	require.NotNil(t, qv)
	assert.Equal(t, "requests_per_second", qv.Dimension)
	assert.Equal(t, int64(1500), qv.Current)
	assert.Equal(t, int64(1000), qv.Limit)
	assert.Equal(t, resetTime.Unix(), qv.ResetTime.Seconds)
}

func TestErrorBuilder_AddPreconditionFailure(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_PRECONDITION_FAILED).
		AddPreconditionFailure("ETAG_MISMATCH", "etag", "abc123", "def456", "Resource was modified").
		Build()

	require.Len(t, err.Details, 1)
	pf := err.Details[0].GetPreconditionFailure()
	require.NotNil(t, pf)
	assert.Equal(t, "ETAG_MISMATCH", pf.Type)
	assert.Equal(t, "etag", pf.Field)
	assert.Equal(t, "abc123", pf.Expected)
	assert.Equal(t, "def456", pf.Actual)
	assert.Equal(t, "Resource was modified", pf.Description)
}

func TestErrorBuilder_AddResourceInfo(t *testing.T) {
	metadata := map[string]string{"ttl": "3600", "encoding": "json"}
	err := NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
		AddResourceInfo("key", "user:12345", "not_found", metadata).
		Build()

	require.Len(t, err.Details, 1)
	ri := err.Details[0].GetResourceInfo()
	require.NotNil(t, ri)
	assert.Equal(t, "key", ri.ResourceType)
	assert.Equal(t, "user:12345", ri.ResourceId)
	assert.Equal(t, "not_found", ri.State)
	assert.Equal(t, metadata, ri.Metadata)
}

func TestErrorBuilder_AddCause(t *testing.T) {
	cause1 := NewError(pb.ErrorCode_ERROR_CODE_INTERNAL_ERROR).
		WithMessage("Connection refused").
		WithSource("redis-client").
		Build()

	cause2 := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		WithMessage("Redis operation failed").
		WithSource("redis-plugin").
		AddCause(cause1).
		Build()

	err := NewError(pb.ErrorCode_ERROR_CODE_GATEWAY_TIMEOUT).
		WithMessage("Pattern execution timed out").
		WithSource("prism-proxy").
		AddCause(cause2).
		Build()

	require.Len(t, err.Causes, 1)
	assert.Equal(t, "Redis operation failed", err.Causes[0].Message)
	require.Len(t, err.Causes[0].Causes, 1)
	assert.Equal(t, "Connection refused", err.Causes[0].Causes[0].Message)
}

func TestErrorBuilder_WithCauses(t *testing.T) {
	cause1 := NewError(pb.ErrorCode_ERROR_CODE_INTERNAL_ERROR).WithMessage("Cause 1").Build()
	cause2 := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).WithMessage("Cause 2").Build()

	err := NewError(pb.ErrorCode_ERROR_CODE_GATEWAY_TIMEOUT).
		WithCauses(cause1, cause2).
		Build()

	require.Len(t, err.Causes, 2)
	assert.Equal(t, "Cause 1", err.Causes[0].Message)
	assert.Equal(t, "Cause 2", err.Causes[1].Message)
}

func TestErrorBuilder_AddHelpLink(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		AddHelpLink("Redis Troubleshooting", "https://docs.prism.io/redis", "troubleshooting").
		Build()

	require.Len(t, err.HelpLinks, 1)
	link := err.HelpLinks[0]
	assert.Equal(t, "Redis Troubleshooting", link.Title)
	assert.Equal(t, "https://docs.prism.io/redis", link.Url)
	assert.Equal(t, "troubleshooting", link.LinkType)
}

func TestErrorBuilder_AddDocLink(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
		AddDocLink("Key Naming", "https://docs.prism.io/naming").
		Build()

	require.Len(t, err.HelpLinks, 1)
	assert.Equal(t, "documentation", err.HelpLinks[0].LinkType)
}

func TestErrorBuilder_AddTroubleshootingLink(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		AddTroubleshootingLink("Backend Down", "https://docs.prism.io/troubleshoot").
		Build()

	require.Len(t, err.HelpLinks, 1)
	assert.Equal(t, "troubleshooting", err.HelpLinks[0].LinkType)
}

func TestErrorBuilder_WithMetadata(t *testing.T) {
	metadata := map[string]string{"region": "us-east-1", "zone": "a"}
	err := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		WithMetadata(metadata).
		Build()

	assert.Equal(t, metadata, err.Metadata)
}

func TestErrorBuilder_AddMetadata(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		AddMetadata("region", "us-east-1").
		AddMetadata("zone", "a").
		Build()

	require.Len(t, err.Metadata, 2)
	assert.Equal(t, "us-east-1", err.Metadata["region"])
	assert.Equal(t, "a", err.Metadata["zone"])
}

func TestErrorBuilder_WithDebugInfo(t *testing.T) {
	stackEntries := []string{"at main.go:123", "at handler.go:456"}
	err := NewError(pb.ErrorCode_ERROR_CODE_INTERNAL_ERROR).
		WithDebugInfo(stackEntries, "Internal state dump", "trace-abc", "span-123").
		Build()

	require.NotNil(t, err.DebugInfo)
	assert.Equal(t, stackEntries, err.DebugInfo.StackEntries)
	assert.Equal(t, "Internal state dump", err.DebugInfo.Detail)
	assert.Equal(t, "trace-abc", err.DebugInfo.TraceId)
	assert.Equal(t, "span-123", err.DebugInfo.SpanId)
}

// Test Common Error Constructors

func TestNotFoundError(t *testing.T) {
	err := NotFoundError("key", "user:12345", "req-abc")

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_FOUND, err.Code)
	assert.Equal(t, "key 'user:12345' not found", err.Message)
	assert.Equal(t, "req-abc", err.RequestId)
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_RESOURCE_ERROR, err.Category)
	assert.Equal(t, pb.ErrorSeverity_ERROR_SEVERITY_ERROR, err.Severity)
	require.NotNil(t, err.RetryPolicy)
	assert.False(t, err.RetryPolicy.Retryable)

	require.Len(t, err.Details, 1)
	ri := err.Details[0].GetResourceInfo()
	require.NotNil(t, ri)
	assert.Equal(t, "key", ri.ResourceType)
	assert.Equal(t, "user:12345", ri.ResourceId)
	assert.Equal(t, "not_found", ri.State)
}

func TestValidationError(t *testing.T) {
	err := ValidationError("ttl_seconds", "TTL must be positive", "-100", "ttl_seconds > 0")

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_UNPROCESSABLE_ENTITY, err.Code)
	assert.Contains(t, err.Message, "ttl_seconds")
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_VALIDATION_ERROR, err.Category)
	assert.False(t, err.RetryPolicy.Retryable)

	require.Len(t, err.Details, 1)
	fv := err.Details[0].GetFieldViolation()
	require.NotNil(t, fv)
	assert.Equal(t, "ttl_seconds", fv.Field)
}

func TestBackendUnavailableError(t *testing.T) {
	err := BackendUnavailableError("redis", "redis-master-1", "GET", 5*time.Second)

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE, err.Code)
	assert.Contains(t, err.Message, "redis")
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR, err.Category)
	assert.Equal(t, pb.ErrorSeverity_ERROR_SEVERITY_CRITICAL, err.Severity)
	assert.True(t, err.RetryPolicy.Retryable)
	assert.Equal(t, int32(5), err.RetryPolicy.MaxRetries)

	require.Len(t, err.Details, 1)
	be := err.Details[0].GetBackendError()
	require.NotNil(t, be)
	assert.Equal(t, "redis", be.BackendType)
}

func TestTimeoutError(t *testing.T) {
	err := TimeoutError("SET", 5*time.Second, "req-xyz")

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_GATEWAY_TIMEOUT, err.Code)
	assert.Contains(t, err.Message, "SET")
	assert.Contains(t, err.Message, "5s")
	assert.Equal(t, "req-xyz", err.RequestId)
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_TIMEOUT_ERROR, err.Category)
	assert.True(t, err.RetryPolicy.Retryable)
}

func TestRateLimitError(t *testing.T) {
	resetTime := time.Now().Add(60 * time.Second)
	err := RateLimitError("requests_per_second", 1500, 1000, resetTime, "prod")

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS, err.Code)
	assert.Contains(t, err.Message, "1500/1000")
	assert.Equal(t, "prod", err.Namespace)
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_RATE_LIMIT_ERROR, err.Category)
	assert.True(t, err.RetryPolicy.Retryable)

	require.Len(t, err.Details, 1)
	qv := err.Details[0].GetQuotaViolation()
	require.NotNil(t, qv)
	assert.Equal(t, int64(1500), qv.Current)
	assert.Equal(t, int64(1000), qv.Limit)
}

func TestInterfaceNotSupportedError(t *testing.T) {
	supportedOps := []string{"Set", "Get", "Delete"}
	err := InterfaceNotSupportedError("keyvalue", "KeyValueTTLInterface", "postgres", supportedOps)

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_INTERFACE_NOT_SUPPORTED, err.Code)
	assert.Contains(t, err.Message, "KeyValueTTLInterface")
	assert.Contains(t, err.Message, "postgres")
	assert.False(t, err.RetryPolicy.Retryable)

	require.Len(t, err.Details, 1)
	pe := err.Details[0].GetPatternError()
	require.NotNil(t, pe)
	assert.Equal(t, "keyvalue", pe.PatternType)
	assert.Equal(t, supportedOps, pe.SupportedOperations)
}

func TestCASConflictError(t *testing.T) {
	err := CASConflictError("user:12345", "v1", "v2")

	assert.Equal(t, pb.ErrorCode_ERROR_CODE_PRECONDITION_FAILED, err.Code)
	assert.Contains(t, err.Message, "user:12345")
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_CONCURRENCY_ERROR, err.Category)
	assert.True(t, err.RetryPolicy.Retryable)
	assert.Equal(t, pb.BackoffStrategy_BACKOFF_STRATEGY_JITTER, err.RetryPolicy.BackoffStrategy)

	require.Len(t, err.Details, 1)
	pf := err.Details[0].GetPreconditionFailure()
	require.NotNil(t, pf)
	assert.Equal(t, "VERSION_CONFLICT", pf.Type)
	assert.Equal(t, "v1", pf.Expected)
	assert.Equal(t, "v2", pf.Actual)
}

// Test Fluent API Chaining

func TestErrorBuilder_FluentChaining(t *testing.T) {
	err := NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
		WithMessage("Redis connection failed").
		WithRequestID("req-123").
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_CRITICAL).
		WithSource("redis-plugin").
		WithNamespace("user-profiles").
		RetryableExponential(5*time.Second, 5, 2.0).
		WithRetryAdvice("Wait for backend to recover").
		AddBackendError("redis", "redis-master-1", "ECONNREFUSED", "Connection refused", "GET").
		AddMetadata("region", "us-east-1").
		AddDocLink("Redis Troubleshooting", "https://docs.prism.io/redis").
		Build()

	// Verify all fields were set
	assert.Equal(t, pb.ErrorCode_ERROR_CODE_BACKEND_ERROR, err.Code)
	assert.Equal(t, "Redis connection failed", err.Message)
	assert.Equal(t, "req-123", err.RequestId)
	assert.Equal(t, pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR, err.Category)
	assert.Equal(t, pb.ErrorSeverity_ERROR_SEVERITY_CRITICAL, err.Severity)
	assert.Equal(t, "redis-plugin", err.Source)
	assert.Equal(t, "user-profiles", err.Namespace)

	require.NotNil(t, err.RetryPolicy)
	assert.True(t, err.RetryPolicy.Retryable)
	assert.Contains(t, err.RetryPolicy.RetryAdvice, "Wait for backend")

	require.Len(t, err.Details, 1)
	require.Len(t, err.Metadata, 1)
	require.Len(t, err.HelpLinks, 1)
}

// Benchmark tests

func BenchmarkNewError_Simple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
			WithMessage("Not found").
			Build()
	}
}

func BenchmarkNewError_Complex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewError(pb.ErrorCode_ERROR_CODE_BACKEND_ERROR).
			WithMessage("Backend error").
			WithRequestID("req-123").
			WithCategory(pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR).
			WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
			RetryableExponential(5*time.Second, 5, 2.0).
			AddBackendError("redis", "redis-1", "ERR", "Error", "GET").
			AddMetadata("region", "us-east-1").
			Build()
	}
}

func BenchmarkNotFoundError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NotFoundError("key", "user:123", "req-456")
	}
}
