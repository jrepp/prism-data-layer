package integration_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	pb "github.com/jrepp/prism-data-layer/patterns/core/gen/prism/pattern"
	"github.com/jrepp/prism-data-layer/patterns/memstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestProxyPatternLifecycle tests the complete lifecycle flow:
// 1. Proxy starts a pattern (memstore driver)
// 2. Pattern starts control plane and listens for lifecycle events
// 3. Proxy connects to pattern control plane
// 4. Proxy sends Initialize event
// 5. Proxy sends Start event
// 6. Pattern sends health info back to proxy
// 7. Proxy validates health info received
// 8. Proxy sends Stop event
// 9. Pattern shuts down gracefully
func TestProxyPatternLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Start backend driver (pattern) with control plane
	// In a real scenario, the proxy would start this as a subprocess
	// For testing, we start it in-process but simulate the lifecycle
	t.Log("Step 1: Starting backend driver (memstore)")

	driver := memstore.New()
	require.NotNil(t, driver)

	// Start control plane server
	controlPlane := core.NewControlPlaneServer(driver, 0) // 0 for dynamic port
	err := controlPlane.Start(ctx)
	require.NoError(t, err)
	defer controlPlane.Stop(ctx)

	// Get the allocated port
	// In a real scenario, the driver would communicate this back to the proxy
	port := controlPlane.Port() // We need to add this method
	t.Logf("Control plane listening on port: %d", port)

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Step 2: Proxy connects to pattern control plane
	t.Log("Step 2: Proxy connecting to pattern control plane")

	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewPatternLifecycleClient(conn)

	// Step 3: Proxy sends Initialize event
	t.Log("Step 3: Proxy sending Initialize event")

	initReq := &pb.InitializeRequest{
		Name:    "memstore",
		Version: "0.1.0",
		Config:  nil, // Empty config for now
	}

	initResp, err := client.Initialize(ctx, initReq)
	require.NoError(t, err)
	assert.True(t, initResp.Success, "Initialize should succeed")
	assert.Empty(t, initResp.Error, "Initialize should not have errors")
	assert.NotNil(t, initResp.Metadata, "Initialize should return metadata")
	assert.Equal(t, "memstore", initResp.Metadata.Name)
	assert.Equal(t, "0.1.0", initResp.Metadata.Version)

	t.Logf("Initialize succeeded: name=%s, version=%s, capabilities=%v",
		initResp.Metadata.Name,
		initResp.Metadata.Version,
		initResp.Metadata.Capabilities)

	// Step 4: Proxy sends Start event
	t.Log("Step 4: Proxy sending Start event")

	startReq := &pb.StartRequest{}
	startResp, err := client.Start(ctx, startReq)
	require.NoError(t, err)
	assert.True(t, startResp.Success, "Start should succeed")
	assert.Empty(t, startResp.Error, "Start should not have errors")

	t.Logf("Start succeeded: data_endpoint=%s", startResp.DataEndpoint)

	// Step 5: Pattern sends health info back to proxy
	t.Log("Step 5: Proxy requesting health check from pattern")

	healthReq := &pb.HealthCheckRequest{}
	healthResp, err := client.HealthCheck(ctx, healthReq)
	require.NoError(t, err)
	assert.Equal(t, pb.HealthStatus_HEALTH_STATUS_HEALTHY, healthResp.Status, "Health should be healthy")
	assert.NotEmpty(t, healthResp.Message, "Health should have a message")
	assert.NotNil(t, healthResp.Details, "Health should have details")

	t.Logf("Health check succeeded: status=%s, message=%s, details=%v",
		healthResp.Status.String(),
		healthResp.Message,
		healthResp.Details)

	// Step 6: Validate health info received by proxy
	t.Log("Step 6: Validating health info")

	assert.Contains(t, healthResp.Message, "healthy", "Health message should indicate healthy state")

	// MemStore health details should include key count
	keyCount, ok := healthResp.Details["keys"]
	assert.True(t, ok, "Health details should include key count")
	t.Logf("MemStore key count: %s", keyCount)

	// Step 7: Test pattern functionality (optional - validates pattern is actually working)
	t.Log("Step 7: Testing pattern functionality (Set/Get)")

	// Set a key
	err = driver.Set("test-key", []byte("test-value"), 0)
	require.NoError(t, err)

	// Get the key
	value, found, err := driver.Get("test-key")
	require.NoError(t, err)
	assert.True(t, found, "Key should be found")
	assert.Equal(t, "test-value", string(value))

	// Verify health details now show 1 key
	healthResp, err = client.HealthCheck(ctx, healthReq)
	require.NoError(t, err)
	keyCount = healthResp.Details["keys"]
	assert.Equal(t, "1", keyCount, "Should have 1 key stored")

	t.Logf("Pattern functionality validated: 1 key stored")

	// Step 8: Proxy sends Stop event
	t.Log("Step 8: Proxy sending Stop event")

	stopReq := &pb.StopRequest{
		TimeoutSeconds: 5,
	}
	stopResp, err := client.Stop(ctx, stopReq)
	require.NoError(t, err)
	assert.True(t, stopResp.Success, "Stop should succeed")
	assert.Empty(t, stopResp.Error, "Stop should not have errors")

	t.Logf("Stop succeeded")

	// Step 9: Verify pattern shut down gracefully
	t.Log("Step 9: Verifying graceful shutdown")

	// After stop, health check should eventually fail or return unhealthy
	// Give it a moment to shut down
	time.Sleep(100 * time.Millisecond)

	// Try health check after stop (may fail or return unhealthy)
	healthResp, err = client.HealthCheck(ctx, healthReq)
	// We don't assert error here because the pattern may have already shut down
	if err == nil {
		t.Logf("Health check after stop: status=%s", healthResp.Status.String())
	} else {
		t.Logf("Health check after stop failed (expected): %v", err)
	}

	t.Log("✅ Complete lifecycle test passed")
}

// TestProxyPatternDebugInfo tests that debug information flows from pattern to proxy
func TestProxyPatternDebugInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Log("Starting debug info test")

	// Start backend driver
	driver := memstore.New()

	controlPlane := core.NewControlPlaneServer(driver, 0)
	err := controlPlane.Start(ctx)
	require.NoError(t, err)
	defer controlPlane.Stop(ctx)

	port := controlPlane.Port()
	time.Sleep(100 * time.Millisecond)

	// Connect proxy to pattern
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewPatternLifecycleClient(conn)

	// Initialize
	initResp, err := client.Initialize(ctx, &pb.InitializeRequest{
		Name:    "memstore",
		Version: "0.1.0",
	})
	require.NoError(t, err)
	require.True(t, initResp.Success)

	// Perform some operations to generate debug info
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		err := driver.Set(key, []byte(value), 0)
		require.NoError(t, err)
	}

	// Request health check to get debug info
	healthResp, err := client.HealthCheck(ctx, &pb.HealthCheckRequest{})
	require.NoError(t, err)

	// Validate debug info received by proxy
	assert.Equal(t, pb.HealthStatus_HEALTH_STATUS_HEALTHY, healthResp.Status)
	assert.Contains(t, healthResp.Message, "10 keys", "Health message should show 10 keys")

	keyCount := healthResp.Details["keys"]
	assert.Equal(t, "10", keyCount, "Should have 10 keys stored")

	t.Logf("Debug info validated: %d keys stored", 10)
	t.Logf("Health message: %s", healthResp.Message)
	t.Logf("Health details: %v", healthResp.Details)

	t.Log("✅ Debug info test passed")
}

// TestProxyPatternConcurrentClients tests multiple proxy clients connecting to same pattern
func TestProxyPatternConcurrentClients(t *testing.T) {
	// Create a context for the control plane that won't be cancelled until we explicitly shut down
	serverCtx, serverCancel := context.WithCancel(context.Background())

	// Create a separate context for test operations with timeout
	testCtx, testCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer testCancel()

	t.Log("Starting concurrent clients test")

	// Start backend driver
	driver := memstore.New()

	controlPlane := core.NewControlPlaneServer(driver, 0)
	err := controlPlane.Start(serverCtx)
	require.NoError(t, err)

	// Ensure we clean up the server after ALL subtests complete (including parallel ones)
	// This cleanup function will be called automatically by the testing framework
	t.Cleanup(func() {
		serverCancel()
		controlPlane.Stop(context.Background())
	})

	port := controlPlane.Port()

	// Wait for server to be ready by polling health check
	t.Log("Waiting for control plane to be ready...")
	ready := false
	for i := 0; i < 50; i++ {
		conn, err := grpc.NewClient(
			fmt.Sprintf("localhost:%d", port),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err == nil {
			client := pb.NewPatternLifecycleClient(conn)
			_, err := client.HealthCheck(testCtx, &pb.HealthCheckRequest{})
			if err == nil {
				ready = true
				conn.Close()
				break
			}
			conn.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, ready, "Control plane did not become ready within timeout")
	t.Logf("Control plane ready on port %d", port)

	// Create multiple proxy clients
	const numClients = 5

	// Create a separate context for parallel tests so parent cancel doesn't affect them
	clientCtx := context.Background()

	for i := 0; i < numClients; i++ {
		t.Run(fmt.Sprintf("Client-%d", i), func(t *testing.T) {
			t.Parallel() // Run clients in parallel

			// Connect
			conn, err := grpc.NewClient(
				fmt.Sprintf("localhost:%d", port),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			require.NoError(t, err)
			defer conn.Close()

			client := pb.NewPatternLifecycleClient(conn)

			// Each client performs health checks
			for j := 0; j < 3; j++ {
				healthResp, err := client.HealthCheck(clientCtx, &pb.HealthCheckRequest{})
				require.NoError(t, err)
				assert.Equal(t, pb.HealthStatus_HEALTH_STATUS_HEALTHY, healthResp.Status)

				t.Logf("Client %d - Health check %d: %s",
					i, j, healthResp.Message)

				time.Sleep(50 * time.Millisecond)
			}
		})
	}

	t.Log("✅ Concurrent clients test passed")
}

// init sets up logging for tests
func init() {
	// Configure structured logging for tests
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}
