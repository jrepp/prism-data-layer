package core

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/jrepp/prism-data-layer/patterns/core/gen/prism/common"
)

// ErrorBuilder provides a fluent API for constructing rich Error messages.
//
// Example:
//
//	err := core.NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
//		WithMessage("Key 'user:12345' not found").
//		WithRequestID(requestID).
//		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_RESOURCE_ERROR).
//		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
//		NonRetryable().
//		Build()
type ErrorBuilder struct {
	err *pb.Error
}

// NewError creates a new ErrorBuilder with the specified code.
func NewError(code pb.ErrorCode) *ErrorBuilder {
	return &ErrorBuilder{
		err: &pb.Error{
			Code:      code,
			Timestamp: timestamppb.Now(),
			Severity:  pb.ErrorSeverity_ERROR_SEVERITY_ERROR, // Default to ERROR
		},
	}
}

// NewErrorFromError creates an ErrorBuilder from an existing Error.
// Useful for wrapping or modifying existing errors.
func NewErrorFromError(err *pb.Error) *ErrorBuilder {
	return &ErrorBuilder{err: err}
}

// Build returns the constructed Error message.
func (b *ErrorBuilder) Build() *pb.Error {
	return b.err
}

// WithMessage sets the human-readable error message.
func (b *ErrorBuilder) WithMessage(message string) *ErrorBuilder {
	b.err.Message = message
	return b
}

// WithMessagef sets the error message using Printf-style formatting.
func (b *ErrorBuilder) WithMessagef(format string, args ...interface{}) *ErrorBuilder {
	b.err.Message = fmt.Sprintf(format, args...)
	return b
}

// WithRequestID sets the request correlation ID.
func (b *ErrorBuilder) WithRequestID(requestID string) *ErrorBuilder {
	b.err.RequestId = requestID
	return b
}

// WithCategory sets the error category for classification.
func (b *ErrorBuilder) WithCategory(category pb.ErrorCategory) *ErrorBuilder {
	b.err.Category = category
	return b
}

// WithSeverity sets the error severity level.
func (b *ErrorBuilder) WithSeverity(severity pb.ErrorSeverity) *ErrorBuilder {
	b.err.Severity = severity
	return b
}

// WithTimestamp sets the error timestamp (defaults to now).
func (b *ErrorBuilder) WithTimestamp(t time.Time) *ErrorBuilder {
	b.err.Timestamp = timestamppb.New(t)
	return b
}

// WithSource sets the service/component that generated the error.
func (b *ErrorBuilder) WithSource(source string) *ErrorBuilder {
	b.err.Source = source
	return b
}

// WithNamespace sets the namespace/tenant context.
func (b *ErrorBuilder) WithNamespace(namespace string) *ErrorBuilder {
	b.err.Namespace = namespace
	return b
}

// WithRetryPolicy sets the retry policy guidance.
func (b *ErrorBuilder) WithRetryPolicy(policy *pb.RetryPolicy) *ErrorBuilder {
	b.err.RetryPolicy = policy
	return b
}

// Retryable marks the error as retryable with the specified policy.
func (b *ErrorBuilder) Retryable(retryAfter time.Duration, maxRetries int32, strategy pb.BackoffStrategy) *ErrorBuilder {
	b.err.RetryPolicy = &pb.RetryPolicy{
		Retryable:        true,
		RetryAfter:       durationpb.New(retryAfter),
		MaxRetries:       maxRetries,
		BackoffStrategy:  strategy,
		BackoffMultiplier: 2.0, // Default exponential multiplier
	}
	return b
}

// RetryableExponential marks the error as retryable with exponential backoff.
func (b *ErrorBuilder) RetryableExponential(retryAfter time.Duration, maxRetries int32, multiplier float64) *ErrorBuilder {
	b.err.RetryPolicy = &pb.RetryPolicy{
		Retryable:         true,
		RetryAfter:        durationpb.New(retryAfter),
		MaxRetries:        maxRetries,
		BackoffStrategy:   pb.BackoffStrategy_BACKOFF_STRATEGY_EXPONENTIAL,
		BackoffMultiplier: multiplier,
		RetryAdvice:       fmt.Sprintf("Retry with exponential backoff (multiplier: %.1fx)", multiplier),
	}
	return b
}

// NonRetryable marks the error as non-retryable.
func (b *ErrorBuilder) NonRetryable() *ErrorBuilder {
	b.err.RetryPolicy = &pb.RetryPolicy{
		Retryable:       false,
		BackoffStrategy: pb.BackoffStrategy_BACKOFF_STRATEGY_NEVER,
	}
	return b
}

// WithRetryAdvice adds human-readable retry advice.
func (b *ErrorBuilder) WithRetryAdvice(advice string) *ErrorBuilder {
	if b.err.RetryPolicy == nil {
		b.err.RetryPolicy = &pb.RetryPolicy{}
	}
	b.err.RetryPolicy.RetryAdvice = advice
	return b
}

// AddDetail adds a structured error detail.
func (b *ErrorBuilder) AddDetail(detail *pb.ErrorDetail) *ErrorBuilder {
	b.err.Details = append(b.err.Details, detail)
	return b
}

// AddFieldViolation adds a field validation error.
func (b *ErrorBuilder) AddFieldViolation(field, description, invalidValue, constraint string) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_FieldViolation{
			FieldViolation: &pb.FieldViolation{
				Field:        field,
				Description:  description,
				InvalidValue: invalidValue,
				Constraint:   constraint,
			},
		},
	})
}

// AddBackendError adds backend-specific error context.
func (b *ErrorBuilder) AddBackendError(backendType, instance, errCode, errMsg, operation string) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_BackendError{
			BackendError: &pb.BackendError{
				BackendType:         backendType,
				BackendInstance:     instance,
				BackendErrorCode:    errCode,
				BackendErrorMessage: errMsg,
				Operation:           operation,
			},
		},
	})
}

// AddBackendErrorWithPool adds backend error with connection pool state.
func (b *ErrorBuilder) AddBackendErrorWithPool(
	backendType, instance, errCode, errMsg, operation string,
	active, idle, max, waitCount int32,
	waitDuration time.Duration,
) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_BackendError{
			BackendError: &pb.BackendError{
				BackendType:         backendType,
				BackendInstance:     instance,
				BackendErrorCode:    errCode,
				BackendErrorMessage: errMsg,
				Operation:           operation,
				PoolState: &pb.ConnectionPoolState{
					ActiveConnections: active,
					IdleConnections:   idle,
					MaxConnections:    max,
					WaitCount:         waitCount,
					WaitDuration:      durationpb.New(waitDuration),
				},
			},
		},
	})
}

// AddPatternError adds pattern-specific error context.
func (b *ErrorBuilder) AddPatternError(patternType, interfaceName, semanticError string, supportedOps ...string) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_PatternError{
			PatternError: &pb.PatternError{
				PatternType:         patternType,
				InterfaceName:       interfaceName,
				SemanticError:       semanticError,
				SupportedOperations: supportedOps,
			},
		},
	})
}

// AddQuotaViolation adds quota/rate limit violation details.
func (b *ErrorBuilder) AddQuotaViolation(dimension string, current, limit int64, resetTime time.Time) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_QuotaViolation{
			QuotaViolation: &pb.QuotaViolation{
				Dimension: dimension,
				Current:   current,
				Limit:     limit,
				ResetTime: timestamppb.New(resetTime),
			},
		},
	})
}

// AddPreconditionFailure adds precondition failure details (CAS, version conflicts).
func (b *ErrorBuilder) AddPreconditionFailure(precondType, field, expected, actual, description string) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_PreconditionFailure{
			PreconditionFailure: &pb.PreconditionFailure{
				Type:        precondType,
				Field:       field,
				Expected:    expected,
				Actual:      actual,
				Description: description,
			},
		},
	})
}

// AddResourceInfo adds resource context information.
func (b *ErrorBuilder) AddResourceInfo(resourceType, resourceID, state string, metadata map[string]string) *ErrorBuilder {
	return b.AddDetail(&pb.ErrorDetail{
		Detail: &pb.ErrorDetail_ResourceInfo{
			ResourceInfo: &pb.ResourceInfo{
				ResourceType: resourceType,
				ResourceId:   resourceID,
				State:        state,
				Metadata:     metadata,
			},
		},
	})
}

// AddCause adds a cause to the error chain.
func (b *ErrorBuilder) AddCause(cause *pb.Error) *ErrorBuilder {
	b.err.Causes = append(b.err.Causes, cause)
	return b
}

// WithCauses sets the entire cause chain.
func (b *ErrorBuilder) WithCauses(causes ...*pb.Error) *ErrorBuilder {
	b.err.Causes = causes
	return b
}

// AddHelpLink adds a documentation/help link.
func (b *ErrorBuilder) AddHelpLink(title, url, linkType string) *ErrorBuilder {
	b.err.HelpLinks = append(b.err.HelpLinks, &pb.ErrorLink{
		Title:    title,
		Url:      url,
		LinkType: linkType,
	})
	return b
}

// AddDocLink adds a documentation link (convenience method).
func (b *ErrorBuilder) AddDocLink(title, url string) *ErrorBuilder {
	return b.AddHelpLink(title, url, "documentation")
}

// AddTroubleshootingLink adds a troubleshooting guide link.
func (b *ErrorBuilder) AddTroubleshootingLink(title, url string) *ErrorBuilder {
	return b.AddHelpLink(title, url, "troubleshooting")
}

// WithMetadata sets additional context metadata.
func (b *ErrorBuilder) WithMetadata(metadata map[string]string) *ErrorBuilder {
	b.err.Metadata = metadata
	return b
}

// AddMetadata adds a single metadata key-value pair.
func (b *ErrorBuilder) AddMetadata(key, value string) *ErrorBuilder {
	if b.err.Metadata == nil {
		b.err.Metadata = make(map[string]string)
	}
	b.err.Metadata[key] = value
	return b
}

// WithDebugInfo adds debug information (only for development).
func (b *ErrorBuilder) WithDebugInfo(stackEntries []string, detail, traceID, spanID string) *ErrorBuilder {
	b.err.DebugInfo = &pb.DebugInfo{
		StackEntries: stackEntries,
		Detail:       detail,
		TraceId:      traceID,
		SpanId:       spanID,
	}
	return b
}

// Common Error Constructors

// NotFoundError creates a NOT_FOUND error.
func NotFoundError(resource, resourceID, requestID string) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_NOT_FOUND).
		WithMessagef("%s '%s' not found", resource, resourceID).
		WithRequestID(requestID).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_RESOURCE_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
		NonRetryable().
		AddResourceInfo(resource, resourceID, "not_found", nil).
		Build()
}

// ValidationError creates an UNPROCESSABLE_ENTITY error for validation failures.
func ValidationError(field, description, invalidValue, constraint string) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_UNPROCESSABLE_ENTITY).
		WithMessagef("Validation failed for field '%s': %s", field, description).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_VALIDATION_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
		NonRetryable().
		WithRetryAdvice(fmt.Sprintf("Fix the '%s' field and retry", field)).
		AddFieldViolation(field, description, invalidValue, constraint).
		Build()
}

// BackendUnavailableError creates a SERVICE_UNAVAILABLE error for backend failures.
func BackendUnavailableError(backendType, backendInstance, operation string, retryAfter time.Duration) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE).
		WithMessagef("%s backend unavailable", backendType).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_BACKEND_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_CRITICAL).
		RetryableExponential(retryAfter, 5, 2.0).
		AddBackendError(backendType, backendInstance, "UNAVAILABLE", "Backend service is unavailable", operation).
		Build()
}

// TimeoutError creates a GATEWAY_TIMEOUT error.
func TimeoutError(operation string, timeout time.Duration, requestID string) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_GATEWAY_TIMEOUT).
		WithMessagef("Operation '%s' timed out after %s", operation, timeout).
		WithRequestID(requestID).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_TIMEOUT_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
		RetryableExponential(5*time.Second, 3, 2.0).
		WithRetryAdvice("Operation timed out. Retry with exponential backoff.").
		Build()
}

// RateLimitError creates a TOO_MANY_REQUESTS error.
func RateLimitError(dimension string, current, limit int64, resetTime time.Time, namespace string) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_TOO_MANY_REQUESTS).
		WithMessagef("Rate limit exceeded for %s: %d/%d", dimension, current, limit).
		WithNamespace(namespace).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_RATE_LIMIT_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_WARNING).
		Retryable(time.Until(resetTime), 10, pb.BackoffStrategy_BACKOFF_STRATEGY_LINEAR).
		WithRetryAdvice(fmt.Sprintf("Wait until %s for quota reset", resetTime.Format(time.RFC3339))).
		AddQuotaViolation(dimension, current, limit, resetTime).
		Build()
}

// InterfaceNotSupportedError creates an INTERFACE_NOT_SUPPORTED error.
func InterfaceNotSupportedError(patternType, interfaceName, backendType string, supportedOps []string) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_INTERFACE_NOT_SUPPORTED).
		WithMessagef("%s interface not supported by %s backend", interfaceName, backendType).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_CLIENT_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
		NonRetryable().
		WithRetryAdvice(fmt.Sprintf("Use a backend that supports %s interface", interfaceName)).
		AddPatternError(patternType, interfaceName, fmt.Sprintf("%s does not implement %s", backendType, interfaceName), supportedOps...).
		Build()
}

// CASConflictError creates a PRECONDITION_FAILED error for Compare-And-Swap failures.
func CASConflictError(resourceID, expected, actual string) *pb.Error {
	return NewError(pb.ErrorCode_ERROR_CODE_PRECONDITION_FAILED).
		WithMessagef("Version conflict for resource '%s'", resourceID).
		WithCategory(pb.ErrorCategory_ERROR_CATEGORY_CONCURRENCY_ERROR).
		WithSeverity(pb.ErrorSeverity_ERROR_SEVERITY_ERROR).
		Retryable(100*time.Millisecond, 5, pb.BackoffStrategy_BACKOFF_STRATEGY_JITTER).
		WithRetryAdvice("Retry with updated version").
		AddPreconditionFailure("VERSION_CONFLICT", "version", expected, actual, "Resource was modified by another client").
		Build()
}
