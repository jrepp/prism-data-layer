package launcher

import (
	"fmt"
	"strings"
)

// LauncherError represents an error with additional context for troubleshooting.
type LauncherError struct {
	// Code identifies the error type
	Code ErrorCode

	// Message is the primary error message
	Message string

	// Context provides additional details
	Context map[string]interface{}

	// Cause is the underlying error (if any)
	Cause error

	// Suggestion provides actionable guidance for resolving the error
	Suggestion string
}

// ErrorCode identifies categories of errors
type ErrorCode string

const (
	// Pattern discovery and validation errors
	ErrorCodePatternNotFound        ErrorCode = "PATTERN_NOT_FOUND"
	ErrorCodeInvalidManifest        ErrorCode = "INVALID_MANIFEST"
	ErrorCodeExecutableNotFound     ErrorCode = "EXECUTABLE_NOT_FOUND"
	ErrorCodeExecutableNotRunnable  ErrorCode = "EXECUTABLE_NOT_RUNNABLE"

	// Process lifecycle errors
	ErrorCodeProcessStartFailed     ErrorCode = "PROCESS_START_FAILED"
	ErrorCodeProcessCrashed         ErrorCode = "PROCESS_CRASHED"
	ErrorCodeHealthCheckFailed      ErrorCode = "HEALTH_CHECK_FAILED"
	ErrorCodeTerminationFailed      ErrorCode = "TERMINATION_FAILED"
	ErrorCodeMaxErrorsExceeded      ErrorCode = "MAX_ERRORS_EXCEEDED"

	// Configuration errors
	ErrorCodeInvalidConfiguration   ErrorCode = "INVALID_CONFIGURATION"
	ErrorCodeInvalidIsolationLevel  ErrorCode = "INVALID_ISOLATION_LEVEL"
	ErrorCodeMissingNamespace       ErrorCode = "MISSING_NAMESPACE"
	ErrorCodeMissingSessionID       ErrorCode = "MISSING_SESSION_ID"

	// Resource errors
	ErrorCodePortAllocationFailed   ErrorCode = "PORT_ALLOCATION_FAILED"
	ErrorCodeResourceLimitExceeded  ErrorCode = "RESOURCE_LIMIT_EXCEEDED"

	// Internal errors
	ErrorCodeInternalError          ErrorCode = "INTERNAL_ERROR"
)

// Error implements the error interface
func (e *LauncherError) Error() string {
	var parts []string

	// Start with code and message
	parts = append(parts, fmt.Sprintf("[%s] %s", e.Code, e.Message))

	// Add context if present
	if len(e.Context) > 0 {
		var contextParts []string
		for k, v := range e.Context {
			contextParts = append(contextParts, fmt.Sprintf("%s=%v", k, v))
		}
		parts = append(parts, fmt.Sprintf("Context: %s", strings.Join(contextParts, ", ")))
	}

	// Add underlying cause if present
	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("Cause: %v", e.Cause))
	}

	// Add suggestion if present
	if e.Suggestion != "" {
		parts = append(parts, fmt.Sprintf("Suggestion: %s", e.Suggestion))
	}

	return strings.Join(parts, "; ")
}

// Unwrap returns the underlying error for errors.Is/As compatibility
func (e *LauncherError) Unwrap() error {
	return e.Cause
}

// NewError creates a new LauncherError with the given code and message
func NewError(code ErrorCode, message string) *LauncherError {
	return &LauncherError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context information to the error
func (e *LauncherError) WithContext(key string, value interface{}) *LauncherError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithCause adds the underlying cause to the error
func (e *LauncherError) WithCause(cause error) *LauncherError {
	e.Cause = cause
	return e
}

// WithSuggestion adds an actionable suggestion to the error
func (e *LauncherError) WithSuggestion(suggestion string) *LauncherError {
	e.Suggestion = suggestion
	return e
}

// Common error constructors with helpful suggestions

// ErrPatternNotFound creates an error for when a pattern cannot be found
func ErrPatternNotFound(patternName, patternsDir string) *LauncherError {
	return NewError(ErrorCodePatternNotFound,
		fmt.Sprintf("Pattern '%s' not found", patternName)).
		WithContext("pattern_name", patternName).
		WithContext("patterns_dir", patternsDir).
		WithSuggestion(fmt.Sprintf(
			"Verify pattern exists: ls -la %s/%s/manifest.yaml",
			patternsDir, patternName))
}

// ErrInvalidManifest creates an error for invalid pattern manifests
func ErrInvalidManifest(patternName string, cause error) *LauncherError {
	return NewError(ErrorCodeInvalidManifest,
		fmt.Sprintf("Pattern '%s' has invalid manifest", patternName)).
		WithContext("pattern_name", patternName).
		WithCause(cause).
		WithSuggestion(
			"Check manifest.yaml syntax and ensure all required fields are present:\n" +
				"  - name\n" +
				"  - version\n" +
				"  - executable\n" +
				"  - isolation_level\n" +
				"  - healthcheck.port\n" +
				"  - healthcheck.path")
}

// ErrExecutableNotFound creates an error for missing executables
func ErrExecutableNotFound(patternName, execPath string) *LauncherError {
	return NewError(ErrorCodeExecutableNotFound,
		fmt.Sprintf("Pattern '%s' executable not found", patternName)).
		WithContext("pattern_name", patternName).
		WithContext("executable_path", execPath).
		WithSuggestion(fmt.Sprintf(
			"Build the pattern executable:\n"+
				"  cd patterns/%s && go build -o %s",
			patternName, execPath))
}

// ErrExecutableNotRunnable creates an error for non-executable files
func ErrExecutableNotRunnable(patternName, execPath string) *LauncherError {
	return NewError(ErrorCodeExecutableNotRunnable,
		fmt.Sprintf("Pattern '%s' executable is not runnable", patternName)).
		WithContext("pattern_name", patternName).
		WithContext("executable_path", execPath).
		WithSuggestion(fmt.Sprintf(
			"Make executable runnable:\n"+
				"  chmod +x %s",
			execPath))
}

// ErrProcessStartFailed creates an error for process start failures
func ErrProcessStartFailed(patternName string, cause error) *LauncherError {
	return NewError(ErrorCodeProcessStartFailed,
		fmt.Sprintf("Failed to start pattern '%s' process", patternName)).
		WithContext("pattern_name", patternName).
		WithCause(cause).
		WithSuggestion(
			"Common causes:\n" +
				"  1. Executable not found or not runnable\n" +
				"  2. Missing dependencies (libraries, environment variables)\n" +
				"  3. Insufficient permissions\n" +
				"  4. Port already in use\n" +
				"Check pattern logs for more details")
}

// ErrHealthCheckFailed creates an error for health check failures
func ErrHealthCheckFailed(patternName, healthURL string, cause error) *LauncherError {
	return NewError(ErrorCodeHealthCheckFailed,
		fmt.Sprintf("Pattern '%s' health check failed", patternName)).
		WithContext("pattern_name", patternName).
		WithContext("health_url", healthURL).
		WithCause(cause).
		WithSuggestion(fmt.Sprintf(
			"Verify health endpoint is responding:\n"+
				"  curl %s\n"+
				"Ensure pattern implements /health endpoint correctly",
			healthURL))
}

// ErrProcessCrashed creates an error for crashed processes
func ErrProcessCrashed(patternName string, processID string, restartCount int) *LauncherError {
	suggestion := "Check pattern logs for crash details"
	if restartCount >= 3 {
		suggestion = "Process has crashed multiple times. This may indicate a persistent issue:\n" +
			"  1. Check pattern logs for errors\n" +
			"  2. Verify dependencies are available\n" +
			"  3. Check resource limits (CPU, memory)\n" +
			"  4. Test pattern standalone: ./patterns/<name>/<executable>"
	}

	return NewError(ErrorCodeProcessCrashed,
		fmt.Sprintf("Pattern '%s' process crashed", patternName)).
		WithContext("pattern_name", patternName).
		WithContext("process_id", processID).
		WithContext("restart_count", restartCount).
		WithSuggestion(suggestion)
}

// ErrMaxErrorsExceeded creates an error for circuit breaker activation
func ErrMaxErrorsExceeded(patternName string, errorCount int) *LauncherError {
	return NewError(ErrorCodeMaxErrorsExceeded,
		fmt.Sprintf("Pattern '%s' exceeded maximum consecutive errors", patternName)).
		WithContext("pattern_name", patternName).
		WithContext("error_count", errorCount).
		WithContext("max_errors", 5).
		WithSuggestion(
			"Process has been marked terminal due to repeated failures.\n" +
				"This is a circuit breaker to prevent infinite restart loops.\n" +
				"Manual intervention required:\n" +
				"  1. Investigate why the pattern keeps failing\n" +
				"  2. Fix the underlying issue\n" +
				"  3. Terminate the pattern: prismctl pattern terminate <process_id>\n" +
				"  4. Relaunch the pattern: prismctl pattern launch <pattern_name>")
}

// ErrMissingNamespace creates an error for missing namespace with NAMESPACE isolation
func ErrMissingNamespace(patternName string) *LauncherError {
	return NewError(ErrorCodeMissingNamespace,
		"Namespace is required for NAMESPACE isolation level").
		WithContext("pattern_name", patternName).
		WithSuggestion(
			"Provide a namespace in the launch request:\n" +
				"  LaunchRequest{\n" +
				"    PatternName: \"" + patternName + "\",\n" +
				"    Isolation:   IsolationLevel_ISOLATION_NAMESPACE,\n" +
				"    Namespace:   \"tenant-a\",  // Required\n" +
				"  }")
}

// ErrMissingSessionID creates an error for missing session ID with SESSION isolation
func ErrMissingSessionID(patternName string) *LauncherError {
	return NewError(ErrorCodeMissingSessionID,
		"Session ID is required for SESSION isolation level").
		WithContext("pattern_name", patternName).
		WithSuggestion(
			"Provide a session ID in the launch request:\n" +
				"  LaunchRequest{\n" +
				"    PatternName: \"" + patternName + "\",\n" +
				"    Isolation:   IsolationLevel_ISOLATION_SESSION,\n" +
				"    Namespace:   \"tenant-a\",\n" +
				"    SessionId:   \"user-123\",  // Required\n" +
				"  }")
}

// ErrInvalidConfiguration creates an error for configuration validation failures
func ErrInvalidConfiguration(field string, value interface{}, reason string) *LauncherError {
	return NewError(ErrorCodeInvalidConfiguration,
		fmt.Sprintf("Invalid configuration: %s", reason)).
		WithContext("field", field).
		WithContext("value", value).
		WithSuggestion(
			"Review launcher configuration and ensure all values are valid.\n" +
				"See Config struct documentation for valid ranges.")
}

// ErrTerminationFailed creates an error for process termination failures
func ErrTerminationFailed(processID string, cause error) *LauncherError {
	return NewError(ErrorCodeTerminationFailed,
		fmt.Sprintf("Failed to terminate process '%s'", processID)).
		WithContext("process_id", processID).
		WithCause(cause).
		WithSuggestion(
			"Try force termination:\n" +
				"  1. Find process: ps aux | grep <pattern-name>\n" +
				"  2. Force kill: kill -9 <pid>\n" +
				"If process is stuck, check for zombie processes or file descriptor leaks")
}

// IsErrorCode checks if an error has the specified error code
func IsErrorCode(err error, code ErrorCode) bool {
	if launcherErr, ok := err.(*LauncherError); ok {
		return launcherErr.Code == code
	}
	return false
}

// GetErrorCode returns the error code from an error, or empty string if not a LauncherError
func GetErrorCode(err error) ErrorCode {
	if launcherErr, ok := err.(*LauncherError); ok {
		return launcherErr.Code
	}
	return ""
}

// GetSuggestion returns the suggestion from an error, or empty string if not available
func GetSuggestion(err error) string {
	if launcherErr, ok := err.(*LauncherError); ok {
		return launcherErr.Suggestion
	}
	return ""
}
