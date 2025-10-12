package integration_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	pb "github.com/jrepp/prism-data-layer/patterns/core/gen/prism/interfaces/keyvalue"
	"github.com/jrepp/prism-data-layer/patterns/memstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestDataPlaneKeyValueOperations tests the complete data plane flow:
// 1. Start memstore backend with data plane gRPC server
// 2. Connect gRPC client to data plane
// 3. Perform Set operation via gRPC
// 4. Perform Get operation via gRPC
// 5. Verify data roundtrip works correctly
// 6. Test Delete and Exists operations
func TestDataPlaneKeyValueOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Start memstore backend
	t.Log("Step 1: Starting memstore backend")

	driver := memstore.New()
	require.NotNil(t, driver)

	// Initialize driver with config
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 0, // Dynamic port allocation
		},
		Backend: map[string]any{
			"max_keys":       10000,
			"cleanup_period": "60s",
		},
	}

	err := driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize driver")

	// Start data plane server (this is what the proxy would connect to)
	dataPlane := core.NewDataPlaneServer(driver, 0) // 0 for dynamic port
	err = dataPlane.Start(ctx)
	require.NoError(t, err, "Failed to start data plane")
	defer dataPlane.Stop(ctx)

	dataPort := dataPlane.Port()
	t.Logf("Data plane listening on port: %d", dataPort)

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Step 2: Connect gRPC client to data plane (simulates proxy → backend communication)
	t.Log("Step 2: Connecting gRPC client to data plane")

	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", dataPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err, "Failed to create gRPC connection")
	defer conn.Close()

	client := pb.NewKeyValueBasicInterfaceClient(conn)

	// Step 3: Test Set operation via gRPC
	t.Log("Step 3: Testing Set operation via gRPC")

	setReq := &pb.SetRequest{
		Key:   "test-key-1",
		Value: []byte("test-value-1"),
	}

	setResp, err := client.Set(ctx, setReq)
	require.NoError(t, err, "Set RPC failed")
	assert.True(t, setResp.Success, "Set should succeed")
	assert.Empty(t, setResp.Error, "Set should not have errors")

	t.Logf("Set succeeded: key=%s", setReq.Key)

	// Step 4: Test Get operation via gRPC
	t.Log("Step 4: Testing Get operation via gRPC")

	getReq := &pb.GetRequest{
		Key: "test-key-1",
	}

	getResp, err := client.Get(ctx, getReq)
	require.NoError(t, err, "Get RPC failed")
	assert.True(t, getResp.Found, "Key should be found")
	assert.Empty(t, getResp.Error, "Get should not have errors")
	assert.Equal(t, "test-value-1", string(getResp.Value), "Value should match")

	t.Logf("Get succeeded: key=%s, value=%s", getReq.Key, string(getResp.Value))

	// Step 5: Test Get on non-existent key
	t.Log("Step 5: Testing Get on non-existent key")

	getNonExistentReq := &pb.GetRequest{
		Key: "non-existent-key",
	}

	getNonExistentResp, err := client.Get(ctx, getNonExistentReq)
	require.NoError(t, err, "Get RPC should not error")
	assert.False(t, getNonExistentResp.Found, "Key should not be found")
	assert.Empty(t, getNonExistentResp.Error, "Get should not have errors")

	t.Logf("Get non-existent key correctly returned not found")

	// Step 6: Test Exists operation
	t.Log("Step 6: Testing Exists operation")

	existsReq := &pb.ExistsRequest{
		Key: "test-key-1",
	}

	existsResp, err := client.Exists(ctx, existsReq)
	require.NoError(t, err, "Exists RPC failed")
	assert.True(t, existsResp.Exists, "Key should exist")
	assert.Empty(t, existsResp.Error, "Exists should not have errors")

	t.Logf("Exists succeeded: key=%s, exists=%v", existsReq.Key, existsResp.Exists)

	// Step 7: Test Delete operation
	t.Log("Step 7: Testing Delete operation")

	deleteReq := &pb.DeleteRequest{
		Key: "test-key-1",
	}

	deleteResp, err := client.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete RPC failed")
	assert.True(t, deleteResp.Success, "Delete should succeed")
	assert.Empty(t, deleteResp.Error, "Delete should not have errors")

	t.Logf("Delete succeeded: key=%s", deleteReq.Key)

	// Step 8: Verify key no longer exists
	t.Log("Step 8: Verifying key no longer exists after delete")

	existsAfterDeleteResp, err := client.Exists(ctx, &pb.ExistsRequest{Key: "test-key-1"})
	require.NoError(t, err, "Exists RPC failed")
	assert.False(t, existsAfterDeleteResp.Exists, "Key should not exist after delete")

	t.Logf("Verified key does not exist after delete")

	t.Log("✅ Complete data plane test passed")
}

// TestDataPlaneMultipleKeys tests handling multiple keys through gRPC
func TestDataPlaneMultipleKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start memstore backend
	driver := memstore.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 0,
		},
		Backend: map[string]any{
			"max_keys":       10000,
			"cleanup_period": "60s",
		},
	}

	err := driver.Initialize(ctx, config)
	require.NoError(t, err)

	// Start data plane
	dataPlane := core.NewDataPlaneServer(driver, 0)
	err = dataPlane.Start(ctx)
	require.NoError(t, err)
	defer dataPlane.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Connect client
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", dataPlane.Port()),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewKeyValueBasicInterfaceClient(conn)

	// Write 100 keys
	t.Log("Writing 100 keys via gRPC")
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%03d", i)
		value := fmt.Sprintf("value-%03d", i)

		setResp, err := client.Set(ctx, &pb.SetRequest{
			Key:   key,
			Value: []byte(value),
		})
		require.NoError(t, err, "Set failed for key %s", key)
		assert.True(t, setResp.Success, "Set should succeed for key %s", key)
	}

	t.Log("Successfully wrote 100 keys")

	// Read back all 100 keys
	t.Log("Reading back 100 keys via gRPC")
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%03d", i)
		expectedValue := fmt.Sprintf("value-%03d", i)

		getResp, err := client.Get(ctx, &pb.GetRequest{Key: key})
		require.NoError(t, err, "Get failed for key %s", key)
		assert.True(t, getResp.Found, "Key should be found: %s", key)
		assert.Equal(t, expectedValue, string(getResp.Value), "Value mismatch for key %s", key)
	}

	t.Log("Successfully read back all 100 keys with correct values")

	// Delete half the keys
	t.Log("Deleting 50 keys via gRPC")
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("key-%03d", i)
		deleteResp, err := client.Delete(ctx, &pb.DeleteRequest{Key: key})
		require.NoError(t, err, "Delete failed for key %s", key)
		assert.True(t, deleteResp.Success, "Delete should succeed for key %s", key)
	}

	t.Log("Successfully deleted 50 keys")

	// Verify first 50 don't exist, last 50 do exist
	t.Log("Verifying deletion results")
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%03d", i)
		existsResp, err := client.Exists(ctx, &pb.ExistsRequest{Key: key})
		require.NoError(t, err, "Exists check failed for key %s", key)

		if i < 50 {
			assert.False(t, existsResp.Exists, "Key should not exist: %s", key)
		} else {
			assert.True(t, existsResp.Exists, "Key should exist: %s", key)
		}
	}

	t.Log("✅ Multiple keys test passed")
}

// TestDataPlaneConcurrentOperations tests concurrent gRPC operations
func TestDataPlaneConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start memstore backend
	driver := memstore.New()
	config := &core.Config{
		Plugin: core.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		ControlPlane: core.ControlPlaneConfig{
			Port: 0,
		},
		Backend: map[string]any{
			"max_keys":       10000,
			"cleanup_period": "60s",
		},
	}

	err := driver.Initialize(ctx, config)
	require.NoError(t, err)

	// Start data plane
	dataPlane := core.NewDataPlaneServer(driver, 0)
	err = dataPlane.Start(ctx)
	require.NoError(t, err)
	defer dataPlane.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Connect client
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", dataPlane.Port()),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewKeyValueBasicInterfaceClient(conn)

	// Launch 10 concurrent goroutines, each writing and reading 10 keys
	const numGoroutines = 10
	const keysPerGoroutine = 10

	errChan := make(chan error, numGoroutines)
	doneChan := make(chan struct{}, numGoroutines)

	t.Logf("Launching %d concurrent goroutines", numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			for k := 0; k < keysPerGoroutine; k++ {
				key := fmt.Sprintf("goroutine-%d-key-%d", goroutineID, k)
				value := fmt.Sprintf("goroutine-%d-value-%d", goroutineID, k)

				// Set
				setResp, err := client.Set(ctx, &pb.SetRequest{
					Key:   key,
					Value: []byte(value),
				})
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d: Set failed: %w", goroutineID, err)
					return
				}
				if !setResp.Success {
					errChan <- fmt.Errorf("goroutine %d: Set returned success=false", goroutineID)
					return
				}

				// Get
				getResp, err := client.Get(ctx, &pb.GetRequest{Key: key})
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d: Get failed: %w", goroutineID, err)
					return
				}
				if !getResp.Found {
					errChan <- fmt.Errorf("goroutine %d: Get returned found=false", goroutineID)
					return
				}
				if string(getResp.Value) != value {
					errChan <- fmt.Errorf("goroutine %d: Value mismatch: expected %s, got %s",
						goroutineID, value, string(getResp.Value))
					return
				}
			}
			doneChan <- struct{}{}
		}(g)
	}

	// Wait for all goroutines
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errChan:
			t.Errorf("Goroutine error: %v", err)
		case <-doneChan:
			successCount++
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for goroutines")
		}
	}

	assert.Equal(t, numGoroutines, successCount, "All goroutines should complete successfully")

	t.Logf("✅ Concurrent operations test passed: %d goroutines completed", successCount)
}

// init sets up logging for tests
func init() {
	// Configure structured logging for tests
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}
