package launcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestIsolationLevels_Integration is a comprehensive integration test
// that verifies all three isolation levels work correctly with real process launching.
//
// Test matrix:
// - ISOLATION_NONE: All requests share single process
// - ISOLATION_NAMESPACE: One process per namespace
// - ISOLATION_SESSION: One process per session
func TestIsolationLevels_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create service with real syncer
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

	// Verify test-pattern was discovered
	manifest, ok := service.registry.GetPattern("test-pattern")
	if !ok {
		t.Fatal("test-pattern not found in registry")
	}
	t.Logf("Found test-pattern: version=%s, isolation=%s", manifest.Version, manifest.IsolationLevel)

	// Test 1: ISOLATION_NONE - all requests share single process
	t.Run("IsolationNone_ProcessReuse", func(t *testing.T) {
		ctx := context.Background()

		// Launch first request with ISOLATION_NONE
		req1 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_NONE,
			Config:      map[string]string{"request": "first"},
		}

		resp1, err := service.LaunchPattern(ctx, req1)
		if err != nil {
			t.Fatalf("First launch failed: %v", err)
		}

		if resp1.State != pb.ProcessState_STATE_RUNNING {
			t.Errorf("Expected STATE_RUNNING, got %v", resp1.State)
		}

		if !resp1.Healthy {
			t.Error("Process should be healthy")
		}

		t.Logf("First process: id=%s, address=%s", resp1.ProcessId, resp1.Address)

		// Launch second request with ISOLATION_NONE
		req2 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_NONE,
			Config:      map[string]string{"request": "second"},
		}

		resp2, err := service.LaunchPattern(ctx, req2)
		if err != nil {
			t.Fatalf("Second launch failed: %v", err)
		}

		// CRITICAL: For ISOLATION_NONE, both requests should get same process ID
		if resp1.ProcessId != resp2.ProcessId {
			t.Errorf("ISOLATION_NONE should reuse process: first=%s, second=%s",
				resp1.ProcessId, resp2.ProcessId)
		}

		t.Logf("Second process (reused): id=%s, address=%s", resp2.ProcessId, resp2.Address)
	})

	// Test 2: ISOLATION_NAMESPACE - one process per namespace
	t.Run("IsolationNamespace_PerTenant", func(t *testing.T) {
		ctx := context.Background()

		// Launch two requests in same namespace
		req1 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
			Namespace:   "tenant-a",
			Config:      map[string]string{"tenant": "a"},
		}

		resp1, err := service.LaunchPattern(ctx, req1)
		if err != nil {
			t.Fatalf("First namespace launch failed: %v", err)
		}

		req2 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
			Namespace:   "tenant-a", // Same namespace
			Config:      map[string]string{"tenant": "a"},
		}

		resp2, err := service.LaunchPattern(ctx, req2)
		if err != nil {
			t.Fatalf("Second namespace launch failed: %v", err)
		}

		// CRITICAL: Same namespace should reuse process
		if resp1.ProcessId != resp2.ProcessId {
			t.Errorf("Same namespace should reuse process: first=%s, second=%s",
				resp1.ProcessId, resp2.ProcessId)
		}

		t.Logf("Tenant A process (reused): id=%s, address=%s", resp2.ProcessId, resp2.Address)

		// Launch third request in different namespace
		req3 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
			Namespace:   "tenant-b", // Different namespace
			Config:      map[string]string{"tenant": "b"},
		}

		resp3, err := service.LaunchPattern(ctx, req3)
		if err != nil {
			t.Fatalf("Third namespace launch failed: %v", err)
		}

		// CRITICAL: Different namespace should get separate process
		if resp1.ProcessId == resp3.ProcessId {
			t.Errorf("Different namespace should get separate process: tenant-a=%s, tenant-b=%s",
				resp1.ProcessId, resp3.ProcessId)
		}

		t.Logf("Tenant B process (separate): id=%s, address=%s", resp3.ProcessId, resp3.Address)
	})

	// Test 3: ISOLATION_SESSION - one process per session
	t.Run("IsolationSession_PerUser", func(t *testing.T) {
		ctx := context.Background()

		// Launch two requests in same session
		req1 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
			Namespace:   "tenant-a",
			SessionId:   "user-123",
			Config:      map[string]string{"user": "123"},
		}

		resp1, err := service.LaunchPattern(ctx, req1)
		if err != nil {
			t.Fatalf("First session launch failed: %v", err)
		}

		req2 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
			Namespace:   "tenant-a",
			SessionId:   "user-123", // Same session
			Config:      map[string]string{"user": "123"},
		}

		resp2, err := service.LaunchPattern(ctx, req2)
		if err != nil {
			t.Fatalf("Second session launch failed: %v", err)
		}

		// CRITICAL: Same session should reuse process
		if resp1.ProcessId != resp2.ProcessId {
			t.Errorf("Same session should reuse process: first=%s, second=%s",
				resp1.ProcessId, resp2.ProcessId)
		}

		t.Logf("User 123 process (reused): id=%s, address=%s", resp2.ProcessId, resp2.Address)

		// Launch third request with different session
		req3 := &pb.LaunchRequest{
			PatternName: "test-pattern",
			Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
			Namespace:   "tenant-a",
			SessionId:   "user-456", // Different session
			Config:      map[string]string{"user": "456"},
		}

		resp3, err := service.LaunchPattern(ctx, req3)
		if err != nil {
			t.Fatalf("Third session launch failed: %v", err)
		}

		// CRITICAL: Different session should get separate process
		if resp1.ProcessId == resp3.ProcessId {
			t.Errorf("Different session should get separate process: user-123=%s, user-456=%s",
				resp1.ProcessId, resp3.ProcessId)
		}

		t.Logf("User 456 process (separate): id=%s, address=%s", resp3.ProcessId, resp3.Address)

		// Verify both sessions are still running
		listReq := &pb.ListPatternsRequest{
			PatternName: "test-pattern",
			Namespace:   "tenant-a",
		}

		listResp, err := service.ListPatterns(ctx, listReq)
		if err != nil {
			t.Fatalf("ListPatterns failed: %v", err)
		}

		if len(listResp.Patterns) < 2 {
			t.Errorf("Expected at least 2 processes for tenant-a, got %d", len(listResp.Patterns))
		}

		for _, pattern := range listResp.Patterns {
			t.Logf("Running process: id=%s, session=%s, pid=%d, uptime=%ds",
				pattern.ProcessId, pattern.SessionId, pattern.Pid, pattern.UptimeSeconds)
		}
	})
}

// TestConcurrentLaunches verifies that concurrent launch requests are handled correctly
func TestConcurrentLaunches(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
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

	// Launch 5 concurrent requests in same namespace
	const numRequests = 5
	results := make(chan *pb.LaunchResponse, numRequests)
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(index int) {
			req := &pb.LaunchRequest{
				PatternName: "test-pattern",
				Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
				Namespace:   "concurrent-test",
				Config:      map[string]string{"request": fmt.Sprintf("%d", index)},
			}

			resp, err := service.LaunchPattern(ctx, req)
			if err != nil {
				errors <- err
				return
			}
			results <- resp
		}(i)
	}

	// Collect results
	var responses []*pb.LaunchResponse
	for i := 0; i < numRequests; i++ {
		select {
		case resp := <-results:
			responses = append(responses, resp)
		case err := <-errors:
			t.Errorf("Concurrent launch failed: %v", err)
		case <-time.After(60 * time.Second):
			t.Fatal("Timeout waiting for concurrent launches")
		}
	}

	// CRITICAL: All requests in same namespace should get same process ID
	if len(responses) > 0 {
		firstProcessID := responses[0].ProcessId
		for i, resp := range responses {
			if resp.ProcessId != firstProcessID {
				t.Errorf("Request %d got different process ID: expected=%s, got=%s",
					i, firstProcessID, resp.ProcessId)
			}
		}

		t.Logf("All %d concurrent requests correctly reused process: id=%s",
			numRequests, firstProcessID)
	}
}

// TestProcessTermination verifies graceful termination works correctly
func TestProcessTermination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
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

	// Launch process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "terminate-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	processID := launchResp.ProcessId
	t.Logf("Launched process: id=%s", processID)

	// Verify process is running
	statusReq := &pb.GetProcessStatusRequest{
		ProcessId: processID,
	}

	statusResp, err := service.GetProcessStatus(ctx, statusReq)
	if err != nil {
		t.Fatalf("GetProcessStatus failed: %v", err)
	}

	if statusResp.NotFound {
		t.Fatal("Process should be found")
	}

	if statusResp.Process.State != pb.ProcessState_STATE_RUNNING {
		t.Errorf("Expected STATE_RUNNING, got %v", statusResp.Process.State)
	}

	t.Logf("Process verified running: uptime=%ds", statusResp.Process.UptimeSeconds)

	// Terminate process
	terminateReq := &pb.TerminateRequest{
		ProcessId:       processID,
		GracePeriodSecs: 5,
		Force:           false,
	}

	terminateResp, err := service.TerminatePattern(ctx, terminateReq)
	if err != nil {
		t.Fatalf("Terminate failed: %v", err)
	}

	if !terminateResp.Success {
		t.Errorf("Terminate should succeed: %s", terminateResp.Error)
	}

	t.Logf("Process terminated successfully")

	// Verify process is no longer found
	statusResp2, err := service.GetProcessStatus(ctx, statusReq)
	if err != nil {
		t.Fatalf("GetProcessStatus after terminate failed: %v", err)
	}

	if !statusResp2.NotFound {
		t.Error("Process should not be found after termination")
	}
}

// TestHealthCheck verifies health endpoint monitoring works
func TestHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
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

	// Launch process
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "health-test",
	}

	launchResp, err := service.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	// Process should be healthy immediately after launch (due to waitForHealthy)
	if !launchResp.Healthy {
		t.Error("Process should be healthy after successful launch")
	}

	t.Logf("Process healthy: id=%s, address=%s", launchResp.ProcessId, launchResp.Address)

	// Get overall service health
	healthReq := &pb.HealthRequest{
		IncludeProcesses: true,
	}

	healthResp, err := service.Health(ctx, healthReq)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if !healthResp.Healthy {
		t.Error("Service should be healthy")
	}

	t.Logf("Service health: processes=%d, running=%d, uptime=%ds",
		healthResp.TotalProcesses,
		healthResp.RunningProcesses,
		healthResp.UptimeSeconds)

	// Verify our process is in the health report
	found := false
	for _, process := range healthResp.Processes {
		if process.ProcessId == launchResp.ProcessId {
			found = true
			if !process.Healthy {
				t.Error("Process should be reported as healthy")
			}
			break
		}
	}

	if !found {
		t.Error("Process should appear in health report")
	}
}

// TestGRPCClient verifies we can connect to launcher via gRPC
func TestGRPCClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Note: This test assumes launcher is running on localhost:8080
	// In a real integration test, we would start the launcher server here
	t.Skip("Skipping gRPC client test - requires running launcher server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "localhost:8080",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewPatternLauncherClient(conn)

	// Launch pattern
	launchReq := &pb.LaunchRequest{
		PatternName: "test-pattern",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "grpc-test",
	}

	launchResp, err := client.LaunchPattern(ctx, launchReq)
	if err != nil {
		t.Fatalf("LaunchPattern RPC failed: %v", err)
	}

	t.Logf("Launched via gRPC: id=%s, address=%s", launchResp.ProcessId, launchResp.Address)

	// List patterns
	listReq := &pb.ListPatternsRequest{
		Namespace: "grpc-test",
	}

	listResp, err := client.ListPatterns(ctx, listReq)
	if err != nil {
		t.Fatalf("ListPatterns RPC failed: %v", err)
	}

	t.Logf("Listed %d patterns via gRPC", len(listResp.Patterns))
}
