package launcher

import (
	"errors"
	"strings"
	"testing"
)

func TestLauncherError(t *testing.T) {
	err := NewError(ErrorCodePatternNotFound, "Pattern not found")

	if err.Code != ErrorCodePatternNotFound {
		t.Errorf("Expected code %s, got %s", ErrorCodePatternNotFound, err.Code)
	}

	if err.Message != "Pattern not found" {
		t.Errorf("Expected message 'Pattern not found', got %s", err.Message)
	}

	errStr := err.Error()
	if !strings.Contains(errStr, string(ErrorCodePatternNotFound)) {
		t.Errorf("Error string should contain error code: %s", errStr)
	}

	if !strings.Contains(errStr, "Pattern not found") {
		t.Errorf("Error string should contain message: %s", errStr)
	}
}

func TestLauncherErrorWithContext(t *testing.T) {
	err := NewError(ErrorCodePatternNotFound, "Pattern not found").
		WithContext("pattern_name", "test-pattern").
		WithContext("patterns_dir", "./patterns")

	errStr := err.Error()

	if !strings.Contains(errStr, "pattern_name=test-pattern") {
		t.Errorf("Error should contain context: %s", errStr)
	}

	if !strings.Contains(errStr, "patterns_dir=./patterns") {
		t.Errorf("Error should contain context: %s", errStr)
	}
}

func TestLauncherErrorWithCause(t *testing.T) {
	cause := errors.New("file not found")
	err := NewError(ErrorCodeExecutableNotFound, "Executable not found").
		WithCause(cause)

	if err.Cause != cause {
		t.Error("Cause should be set")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "file not found") {
		t.Errorf("Error should contain cause: %s", errStr)
	}

	// Test Unwrap for errors.Is/As compatibility
	if !errors.Is(err, cause) {
		t.Error("errors.Is should work with Unwrap")
	}
}

func TestLauncherErrorWithSuggestion(t *testing.T) {
	err := NewError(ErrorCodeExecutableNotRunnable, "Not executable").
		WithSuggestion("chmod +x ./patterns/test/test")

	if err.Suggestion == "" {
		t.Error("Suggestion should be set")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "chmod +x") {
		t.Errorf("Error should contain suggestion: %s", errStr)
	}
}

func TestErrPatternNotFound(t *testing.T) {
	err := ErrPatternNotFound("test-pattern", "./patterns")

	if err.Code != ErrorCodePatternNotFound {
		t.Errorf("Expected code %s, got %s", ErrorCodePatternNotFound, err.Code)
	}

	// Check context
	if err.Context["pattern_name"] != "test-pattern" {
		t.Error("Context should contain pattern_name")
	}

	if err.Context["patterns_dir"] != "./patterns" {
		t.Error("Context should contain patterns_dir")
	}

	// Check suggestion
	if !strings.Contains(err.Suggestion, "ls -la") {
		t.Error("Suggestion should contain verification command")
	}

	// Check full error message
	errStr := err.Error()
	if !strings.Contains(errStr, "test-pattern") {
		t.Errorf("Error should mention pattern name: %s", errStr)
	}
}

func TestErrInvalidManifest(t *testing.T) {
	cause := errors.New("missing field: version")
	err := ErrInvalidManifest("test-pattern", cause)

	if err.Code != ErrorCodeInvalidManifest {
		t.Errorf("Expected code %s", ErrorCodeInvalidManifest)
	}

	if err.Cause != cause {
		t.Error("Cause should be set")
	}

	// Check suggestion mentions required fields
	requiredFields := []string{"name", "version", "executable", "isolation_level"}
	for _, field := range requiredFields {
		if !strings.Contains(err.Suggestion, field) {
			t.Errorf("Suggestion should mention required field '%s'", field)
		}
	}
}

func TestErrExecutableNotFound(t *testing.T) {
	err := ErrExecutableNotFound("test-pattern", "./patterns/test-pattern/test")

	if err.Code != ErrorCodeExecutableNotFound {
		t.Errorf("Expected code %s", ErrorCodeExecutableNotFound)
	}

	// Check suggestion mentions build command
	if !strings.Contains(err.Suggestion, "go build") {
		t.Error("Suggestion should mention build command")
	}

	if !strings.Contains(err.Suggestion, "test-pattern") {
		t.Error("Suggestion should mention pattern name")
	}
}

func TestErrExecutableNotRunnable(t *testing.T) {
	err := ErrExecutableNotRunnable("test-pattern", "./patterns/test-pattern/test")

	if err.Code != ErrorCodeExecutableNotRunnable {
		t.Errorf("Expected code %s", ErrorCodeExecutableNotRunnable)
	}

	// Check suggestion mentions chmod
	if !strings.Contains(err.Suggestion, "chmod +x") {
		t.Error("Suggestion should mention chmod command")
	}
}

func TestErrProcessStartFailed(t *testing.T) {
	cause := errors.New("exec: not found")
	err := ErrProcessStartFailed("test-pattern", cause)

	if err.Code != ErrorCodeProcessStartFailed {
		t.Errorf("Expected code %s", ErrorCodeProcessStartFailed)
	}

	// Check suggestion mentions common causes
	commonCauses := []string{"Executable not found", "dependencies", "permissions", "Port"}
	for _, causeStr := range commonCauses {
		if !strings.Contains(err.Suggestion, causeStr) {
			t.Errorf("Suggestion should mention common cause: %s", causeStr)
		}
	}
}

func TestErrHealthCheckFailed(t *testing.T) {
	cause := errors.New("connection refused")
	err := ErrHealthCheckFailed("test-pattern", "http://localhost:9090/health", cause)

	if err.Code != ErrorCodeHealthCheckFailed {
		t.Errorf("Expected code %s", ErrorCodeHealthCheckFailed)
	}

	// Check context
	if err.Context["health_url"] != "http://localhost:9090/health" {
		t.Error("Context should contain health_url")
	}

	// Check suggestion mentions curl verification
	if !strings.Contains(err.Suggestion, "curl") {
		t.Error("Suggestion should mention curl verification")
	}

	if !strings.Contains(err.Suggestion, "http://localhost:9090/health") {
		t.Error("Suggestion should include actual health URL")
	}
}

func TestErrProcessCrashed(t *testing.T) {
	t.Run("first crash", func(t *testing.T) {
		err := ErrProcessCrashed("test-pattern", "ns:tenant-a:test", 1)

		if err.Code != ErrorCodeProcessCrashed {
			t.Errorf("Expected code %s", ErrorCodeProcessCrashed)
		}

		if err.Context["restart_count"] != 1 {
			t.Error("Context should contain restart_count")
		}

		// Should have basic suggestion
		if !strings.Contains(err.Suggestion, "Check pattern logs") {
			t.Error("Suggestion should mention checking logs")
		}
	})

	t.Run("multiple crashes", func(t *testing.T) {
		err := ErrProcessCrashed("test-pattern", "ns:tenant-a:test", 3)

		if err.Context["restart_count"] != 3 {
			t.Error("Context should contain restart_count")
		}

		// Should have more detailed suggestion for repeated crashes
		if !strings.Contains(err.Suggestion, "crashed multiple times") {
			t.Error("Suggestion should mention multiple crashes")
		}

		if !strings.Contains(err.Suggestion, "persistent issue") {
			t.Error("Suggestion should mention persistent issues")
		}
	})
}

func TestErrMaxErrorsExceeded(t *testing.T) {
	err := ErrMaxErrorsExceeded("test-pattern", 5)

	if err.Code != ErrorCodeMaxErrorsExceeded {
		t.Errorf("Expected code %s", ErrorCodeMaxErrorsExceeded)
	}

	if err.Context["error_count"] != 5 {
		t.Error("Context should contain error_count")
	}

	if err.Context["max_errors"] != 5 {
		t.Error("Context should contain max_errors")
	}

	// Check suggestion mentions circuit breaker and manual intervention
	if !strings.Contains(err.Suggestion, "circuit breaker") {
		t.Error("Suggestion should explain circuit breaker")
	}

	if !strings.Contains(err.Suggestion, "Manual intervention required") {
		t.Error("Suggestion should indicate manual intervention needed")
	}

	if !strings.Contains(err.Suggestion, "prismctl pattern terminate") {
		t.Error("Suggestion should mention termination command")
	}
}

func TestErrMissingNamespace(t *testing.T) {
	err := ErrMissingNamespace("test-pattern")

	if err.Code != ErrorCodeMissingNamespace {
		t.Errorf("Expected code %s", ErrorCodeMissingNamespace)
	}

	// Check suggestion shows example code
	if !strings.Contains(err.Suggestion, "Namespace:") {
		t.Error("Suggestion should show namespace field")
	}

	if !strings.Contains(err.Suggestion, "ISOLATION_NAMESPACE") {
		t.Error("Suggestion should mention correct isolation level")
	}
}

func TestErrMissingSessionID(t *testing.T) {
	err := ErrMissingSessionID("test-pattern")

	if err.Code != ErrorCodeMissingSessionID {
		t.Errorf("Expected code %s", ErrorCodeMissingSessionID)
	}

	// Check suggestion shows example code
	if !strings.Contains(err.Suggestion, "SessionId:") {
		t.Error("Suggestion should show session_id field")
	}

	if !strings.Contains(err.Suggestion, "ISOLATION_SESSION") {
		t.Error("Suggestion should mention correct isolation level")
	}
}

func TestErrInvalidConfiguration(t *testing.T) {
	err := ErrInvalidConfiguration("ResyncInterval", "500ms", "must be at least 1 second")

	if err.Code != ErrorCodeInvalidConfiguration {
		t.Errorf("Expected code %s", ErrorCodeInvalidConfiguration)
	}

	if err.Context["field"] != "ResyncInterval" {
		t.Error("Context should contain field name")
	}

	if err.Context["value"] != "500ms" {
		t.Error("Context should contain field value")
	}

	// Check message contains reason
	if !strings.Contains(err.Message, "must be at least 1 second") {
		t.Error("Message should contain reason")
	}
}

func TestErrTerminationFailed(t *testing.T) {
	cause := errors.New("process not responding")
	err := ErrTerminationFailed("ns:tenant-a:test", cause)

	if err.Code != ErrorCodeTerminationFailed {
		t.Errorf("Expected code %s", ErrorCodeTerminationFailed)
	}

	// Check suggestion mentions force kill
	if !strings.Contains(err.Suggestion, "kill -9") {
		t.Error("Suggestion should mention force kill")
	}

	if !strings.Contains(err.Suggestion, "ps aux") {
		t.Error("Suggestion should mention process listing")
	}
}

func TestIsErrorCode(t *testing.T) {
	err := NewError(ErrorCodePatternNotFound, "test")

	if !IsErrorCode(err, ErrorCodePatternNotFound) {
		t.Error("IsErrorCode should return true for matching code")
	}

	if IsErrorCode(err, ErrorCodeProcessStartFailed) {
		t.Error("IsErrorCode should return false for non-matching code")
	}

	// Test with non-LauncherError
	otherErr := errors.New("other error")
	if IsErrorCode(otherErr, ErrorCodePatternNotFound) {
		t.Error("IsErrorCode should return false for non-LauncherError")
	}
}

func TestGetErrorCode(t *testing.T) {
	err := NewError(ErrorCodePatternNotFound, "test")

	code := GetErrorCode(err)
	if code != ErrorCodePatternNotFound {
		t.Errorf("Expected code %s, got %s", ErrorCodePatternNotFound, code)
	}

	// Test with non-LauncherError
	otherErr := errors.New("other error")
	code = GetErrorCode(otherErr)
	if code != "" {
		t.Errorf("Expected empty code for non-LauncherError, got %s", code)
	}
}

func TestGetSuggestion(t *testing.T) {
	err := NewError(ErrorCodePatternNotFound, "test").
		WithSuggestion("Check the pattern directory")

	suggestion := GetSuggestion(err)
	if suggestion != "Check the pattern directory" {
		t.Errorf("Expected suggestion, got %s", suggestion)
	}

	// Test with non-LauncherError
	otherErr := errors.New("other error")
	suggestion = GetSuggestion(otherErr)
	if suggestion != "" {
		t.Errorf("Expected empty suggestion for non-LauncherError, got %s", suggestion)
	}
}

func TestErrorChaining(t *testing.T) {
	// Test that WithContext, WithCause, WithSuggestion can be chained
	err := NewError(ErrorCodeProcessStartFailed, "Failed to start").
		WithContext("pattern", "test").
		WithContext("pid", 12345).
		WithCause(errors.New("exec failed")).
		WithSuggestion("Check permissions")

	if err.Code != ErrorCodeProcessStartFailed {
		t.Error("Code should be preserved")
	}

	if len(err.Context) != 2 {
		t.Errorf("Expected 2 context entries, got %d", len(err.Context))
	}

	if err.Cause == nil {
		t.Error("Cause should be set")
	}

	if err.Suggestion == "" {
		t.Error("Suggestion should be set")
	}

	// Verify all parts appear in error string
	errStr := err.Error()
	expectedParts := []string{
		"PROCESS_START_FAILED",
		"Failed to start",
		"pattern=test",
		"pid=12345",
		"exec failed",
		"Check permissions",
	}

	for _, part := range expectedParts {
		if !strings.Contains(errStr, part) {
			t.Errorf("Error string missing expected part '%s': %s", part, errStr)
		}
	}
}
