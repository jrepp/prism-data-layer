package launcher

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
)

// TestMetricsCollector tests the basic metrics collection
func TestMetricsCollector(t *testing.T) {
	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	if service.metricsCollector == nil {
		t.Fatal("Metrics collector should be initialized")
	}

	// Test recording process start
	service.metricsCollector.RecordProcessStart("test-pattern", isolation.IsolationNamespace)
	service.metricsCollector.RecordProcessStart("test-pattern", isolation.IsolationNamespace)

	// Test recording health checks
	service.metricsCollector.RecordHealthCheckSuccess("test-pattern")
	service.metricsCollector.RecordHealthCheckFailure("test-pattern")
	service.metricsCollector.RecordHealthCheckFailure("test-pattern")

	// Test recording launch duration
	service.metricsCollector.RecordLaunchDuration("test-pattern", isolation.IsolationNamespace, 500*time.Millisecond, true)
	service.metricsCollector.RecordLaunchDuration("test-pattern", isolation.IsolationNamespace, 750*time.Millisecond, true)
	service.metricsCollector.RecordLaunchDuration("test-pattern", isolation.IsolationNamespace, 1*time.Second, true)

	// Get metrics snapshot
	metrics := service.GetMetrics()

	if metrics == nil {
		t.Fatal("Metrics snapshot should not be nil")
	}

	// Verify process starts
	key := "test-pattern:Namespace" // isolation.IsolationNamespace.String() returns "Namespace"
	if count, ok := metrics.ProcessStartsTotal[key]; !ok || count != 2 {
		t.Errorf("Expected 2 process starts for %s, got %d", key, count)
	}

	// Verify health checks
	if success, ok := metrics.HealthCheckSuccessTotal["test-pattern"]; !ok || success != 1 {
		t.Errorf("Expected 1 health check success, got %d", success)
	}

	if failure, ok := metrics.HealthCheckFailureTotal["test-pattern"]; !ok || failure != 2 {
		t.Errorf("Expected 2 health check failures, got %d", failure)
	}

	// Verify launch durations (should have p50, p95, p99)
	if metrics.LaunchDurationP50 == 0 {
		t.Error("LaunchDurationP50 should be non-zero")
	}

	t.Logf("Launch duration p50: %v, p95: %v, p99: %v",
		metrics.LaunchDurationP50, metrics.LaunchDurationP95, metrics.LaunchDurationP99)
}

// TestPrometheusExport tests Prometheus format export
func TestPrometheusExport(t *testing.T) {
	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	// Record some metrics
	service.metricsCollector.RecordProcessStart("test-pattern", isolation.IsolationNamespace)
	service.metricsCollector.RecordHealthCheckSuccess("test-pattern")
	service.metricsCollector.RecordLaunchDuration("test-pattern", isolation.IsolationNamespace, 500*time.Millisecond, true)

	// Export as Prometheus
	prometheusOutput := service.ExportPrometheusMetrics()

	if prometheusOutput == "" {
		t.Fatal("Prometheus output should not be empty")
	}

	// Verify it contains expected metrics
	expectedMetrics := []string{
		"pattern_launcher_processes_total",
		"pattern_launcher_isolation_level",
		"pattern_launcher_process_starts_total",
		"pattern_launcher_health_checks_total",
		"pattern_launcher_launch_duration_seconds",
		"pattern_launcher_uptime_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(prometheusOutput, metric) {
			t.Errorf("Prometheus output should contain metric: %s", metric)
		}
	}

	// Verify it contains HELP and TYPE declarations
	if !strings.Contains(prometheusOutput, "# HELP") {
		t.Error("Prometheus output should contain HELP declarations")
	}

	if !strings.Contains(prometheusOutput, "# TYPE") {
		t.Error("Prometheus output should contain TYPE declarations")
	}

	t.Logf("Prometheus output sample:\n%s", prometheusOutput[:min(500, len(prometheusOutput))])
}

// TestJSONExport tests JSON format export
func TestJSONExport(t *testing.T) {
	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	// Record some metrics
	service.metricsCollector.RecordProcessStart("test-pattern", isolation.IsolationNamespace)
	service.metricsCollector.RecordHealthCheckSuccess("test-pattern")

	// Export as JSON
	jsonOutput := service.ExportJSONMetrics()

	if jsonOutput == "" {
		t.Fatal("JSON output should not be empty")
	}

	// Verify it contains expected fields
	expectedFields := []string{
		"total_processes",
		"running_processes",
		"process_starts_total",
		"health_check_success_total",
		"launch_duration_p50_seconds",
		"uptime_seconds",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonOutput, field) {
			t.Errorf("JSON output should contain field: %s", field)
		}
	}

	t.Logf("JSON output:\n%s", jsonOutput)
}

// TestMetricsIntegration tests metrics with actual process launch
func TestMetricsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping metrics integration test in short mode")
	}

	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	ctx := context.Background()

	// Get initial metrics
	initialMetrics := service.GetMetrics()
	initialStarts := int64(0)
	key := "test-pattern:Namespace" // isolation.IsolationNamespace.String() returns "Namespace"
	if count, ok := initialMetrics.ProcessStartsTotal[key]; ok {
		initialStarts = count
	}

	// Launch a process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "metrics-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	t.Logf("Launched process: id=%s", launchResp.ProcessId)

	// Wait for metrics to be recorded
	time.Sleep(500 * time.Millisecond)

	// Get updated metrics
	updatedMetrics := service.GetMetrics()

	// Verify process start was recorded
	newStarts := int64(0)
	if count, ok := updatedMetrics.ProcessStartsTotal[key]; ok {
		newStarts = count
	}

	if newStarts != initialStarts+1 {
		t.Errorf("Expected process starts to increase by 1: initial=%d, new=%d",
			initialStarts, newStarts)
	}

	// Verify launch duration was recorded
	if updatedMetrics.LaunchDurationP50 == 0 {
		t.Error("Launch duration should be recorded")
	}

	t.Logf("Launch duration: %v", updatedMetrics.LaunchDurationP50)

	// Terminate process
	terminateReq := &pb.TerminateRequest{
		ProcessId:       launchResp.ProcessId,
		GracePeriodSecs: 5,
	}

	_, err = service.TerminatePattern(ctx, terminateReq)
	if err != nil {
		t.Fatalf("Terminate failed: %v", err)
	}

	// Wait for termination
	time.Sleep(500 * time.Millisecond)

	// Get final metrics
	finalMetrics := service.GetMetrics()

	// Verify process stop was recorded
	stops := int64(0)
	if count, ok := finalMetrics.ProcessStopsTotal[key]; ok {
		stops = count
	}

	if stops == 0 {
		t.Error("Process stops should be recorded")
	}

	t.Logf("Metrics after full lifecycle: starts=%d, stops=%d",
		finalMetrics.ProcessStartsTotal[key],
		finalMetrics.ProcessStopsTotal[key])
}

// TestHealthCheckMetrics tests health check metric recording
func TestHealthCheckMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping health check metrics test in short mode")
	}

	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	ctx := context.Background()

	// Launch a process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "health-metrics-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	t.Logf("Launched process: id=%s", launchResp.ProcessId)

	// Wait for health checks to occur
	// (health checks happen during launch and periodically)
	time.Sleep(2 * time.Second)

	// Get metrics
	metrics := service.GetMetrics()

	// Verify health check metrics were recorded
	successCount := int64(0)
	if count, ok := metrics.HealthCheckSuccessTotal["test-pattern"]; ok {
		successCount = count
	}

	if successCount == 0 {
		t.Error("Health check successes should be recorded")
	}

	t.Logf("Health check metrics: success=%d, failure=%d",
		metrics.HealthCheckSuccessTotal["test-pattern"],
		metrics.HealthCheckFailureTotal["test-pattern"])
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
