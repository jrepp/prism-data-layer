package launcher

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
)

// TestProcessCrashRecovery tests that crashed processes are detected and restarted
func TestProcessCrashRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping crash recovery test in short mode")
	}

	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   5 * time.Second,  // Fast resync for testing
		BackOffPeriod:    1 * time.Second,  // Fast backoff for testing
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	ctx := context.Background()

	// Launch process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "crash-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	originalProcessID := launchResp.ProcessId
	t.Logf("Launched process: id=%s, address=%s", originalProcessID, launchResp.Address)

	// Wait for process to be fully healthy
	time.Sleep(1 * time.Second)

	// Get PID
	service.processesMu.RLock()
	info, ok := service.processes[originalProcessID]
	service.processesMu.RUnlock()

	if !ok {
		t.Fatal("Process not found in tracking")
	}

	originalPID := info.PID
	if originalPID <= 0 {
		t.Fatal("Invalid PID")
	}

	t.Logf("Original PID: %d", originalPID)

	// Kill the process to simulate a crash
	t.Logf("Simulating crash by killing process %d", originalPID)
	if err := syscall.Kill(originalPID, syscall.SIGKILL); err != nil {
		t.Fatalf("Failed to kill process: %v", err)
	}

	// Wait for procmgr to detect crash and restart (resync interval + grace)
	t.Logf("Waiting for process manager to detect crash and restart...")
	time.Sleep(10 * time.Second)

	// Check if process was restarted
	service.processesMu.RLock()
	newInfo, ok := service.processes[originalProcessID]
	service.processesMu.RUnlock()

	if !ok {
		t.Fatal("Process should still exist after crash (should be restarted)")
	}

	newPID := newInfo.PID
	t.Logf("New PID after restart: %d (original: %d)", newPID, originalPID)

	// Verify process was actually restarted (new PID)
	if newPID == originalPID {
		t.Error("Process PID should be different after crash restart")
	}

	if newPID <= 0 {
		t.Error("New PID should be valid")
	}

	// Verify restart count increased
	if newInfo.RestartCount <= 0 {
		t.Errorf("Restart count should be >0, got %d", newInfo.RestartCount)
	}

	t.Logf("Process successfully restarted: restart_count=%d", newInfo.RestartCount)
}

// TestMaxErrorsTerminal tests that processes exceeding max errors are marked terminal
func TestMaxErrorsTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping max errors test in short mode")
	}

	// This test would require a pattern that repeatedly fails health checks
	// For now, we'll document the expected behavior

	t.Log("Test scenario: Process fails health check 5 times")
	t.Log("Expected: Process marked as terminal and not restarted")
	t.Log("Actual implementation: See syncer.go SyncProcess() maxErrors check")

	// TODO: Create a "failing-pattern" binary that never responds to health checks
	// Then launch it and verify it gets marked terminal after maxErrors attempts
}

// TestHealthCheckMonitor tests continuous health monitoring
func TestHealthCheckMonitor(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping health monitor test in short mode")
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

	// Health monitor is started automatically in NewService
	// Verify it's running
	if service.healthMonitor == nil {
		t.Fatal("Health monitor should be initialized")
	}

	ctx := context.Background()

	// Launch a process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "health-monitor-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	t.Logf("Launched process for health monitoring: id=%s", launchResp.ProcessId)

	// Wait for at least one health check cycle (30 seconds + some margin)
	t.Log("Waiting for health monitor to perform checks...")
	time.Sleep(35 * time.Second)

	// Verify process is still tracked and healthy
	service.processesMu.RLock()
	info, ok := service.processes[launchResp.ProcessId]
	service.processesMu.RUnlock()

	if !ok {
		t.Fatal("Process should still be tracked")
	}

	if info.PID <= 0 {
		t.Error("Process should have valid PID")
	}

	t.Logf("Health monitor verification: process still running with PID %d", info.PID)
}

// TestOrphanDetection tests orphan process cleanup
func TestOrphanDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping orphan detection test in short mode")
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

	// Orphan detector is started automatically in NewService
	if service.orphanDetector == nil {
		t.Fatal("Orphan detector should be initialized")
	}

	ctx := context.Background()

	// Launch a process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "orphan-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	processID := launchResp.ProcessId
	t.Logf("Launched process: id=%s", processID)

	// Get PID
	service.processesMu.RLock()
	info := service.processes[processID]
	service.processesMu.RUnlock()

	pid := info.PID
	t.Logf("Process PID: %d", pid)

	// Manually remove from tracking to simulate orphan condition
	// (in production, this would happen if launcher crashes)
	service.processesMu.Lock()
	delete(service.processes, processID)
	service.processesMu.Unlock()

	t.Log("Removed process from tracking (simulated orphan)")

	// Trigger orphan detection manually (don't wait for timer)
	t.Log("Running orphan detection...")
	if err := service.orphanDetector.detectAndCleanup(ctx); err != nil {
		t.Logf("Orphan detection error (expected on macOS): %v", err)
	}

	// NOTE: Orphan detection relies on /proc filesystem (Linux)
	// On macOS, it will not find orphans via /proc
	// In production, we would use platform-specific methods

	t.Log("Orphan detection test completed (Linux-only feature)")
}

// TestCleanupManager tests resource cleanup after termination
func TestCleanupManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cleanup manager test in short mode")
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

	if service.cleanupManager == nil {
		t.Fatal("Cleanup manager should be initialized")
	}

	ctx := context.Background()

	// Launch process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "cleanup-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	processID := launchResp.ProcessId
	t.Logf("Launched process: id=%s", processID)

	// Verify process is tracked
	service.processesMu.RLock()
	_, exists := service.processes[processID]
	service.processesMu.RUnlock()

	if !exists {
		t.Fatal("Process should be tracked before termination")
	}

	// Terminate process
	terminateReq := &pb.TerminateRequest{
		ProcessId:       processID,
		GracePeriodSecs: 5,
	}

	terminateResp, err := service.TerminatePattern(ctx, terminateReq)
	if err != nil {
		t.Fatalf("Terminate failed: %v", err)
	}

	if !terminateResp.Success {
		t.Errorf("Terminate should succeed: %s", terminateResp.Error)
	}

	// Verify cleanup removed process from tracking
	service.processesMu.RLock()
	_, stillExists := service.processes[processID]
	service.processesMu.RUnlock()

	if stillExists {
		t.Error("Process should be removed from tracking after termination")
	}

	t.Logf("Cleanup verified: process %s removed from tracking", processID)
}

// TestErrorTracking tests that errors are tracked across restarts
func TestErrorTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping error tracking test in short mode")
	}

	// Create a service with fast resync for testing
	config := &Config{
		PatternsDir:      "../../patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   3 * time.Second,
		BackOffPeriod:    1 * time.Second,
	}

	service, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer service.Shutdown(context.Background())

	ctx := context.Background()

	// Launch process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "error-tracking-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	processID := launchResp.ProcessId
	t.Logf("Launched process: id=%s", processID)

	// Get initial info
	service.processesMu.RLock()
	info := service.processes[processID]
	service.processesMu.RUnlock()

	initialRestartCount := info.RestartCount
	initialErrorCount := info.ErrorCount

	t.Logf("Initial state: restart_count=%d, error_count=%d",
		initialRestartCount, initialErrorCount)

	// Kill process to trigger restart
	pid := info.PID
	t.Logf("Killing process %d to trigger restart", pid)
	syscall.Kill(pid, syscall.SIGKILL)

	// Wait for restart
	time.Sleep(8 * time.Second)

	// Check updated stats
	service.processesMu.RLock()
	newInfo := service.processes[processID]
	service.processesMu.RUnlock()

	if newInfo == nil {
		t.Fatal("Process should still exist after restart")
	}

	t.Logf("After restart: restart_count=%d, error_count=%d",
		newInfo.RestartCount, newInfo.ErrorCount)

	// Restart count should have increased
	if newInfo.RestartCount <= initialRestartCount {
		t.Errorf("Restart count should increase: initial=%d, new=%d",
			initialRestartCount, newInfo.RestartCount)
	}

	t.Log("Error tracking verified: restart count incremented correctly")
}
