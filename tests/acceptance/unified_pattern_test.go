package acceptance_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	pb_kv "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces/keyvalue"
	pb_pubsub "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// This is a unified acceptance test that works with ANY pattern executable.
// It:
// 1. Connects to a running pattern executable
// 2. Queries it for supported interfaces
// 3. Dynamically runs the appropriate test suites

// TestUnifiedPattern demonstrates the dynamic interface-based testing approach
//
// To test a specific pattern executable:
//   1. Start the pattern executable (e.g., multicast-registry-runner, kafka driver, etc.)
//   2. Set the address via environment variable or test flag
//   3. Run this test - it will automatically discover interfaces and run appropriate tests
//
// Example:
//   # Start pattern executable
//   ./patterns/multicast_registry/cmd/multicast-registry-runner/multicast-registry-runner --port 50051
//
//   # Run tests (they'll connect to localhost:50051)
//   go test -v ./tests/acceptance -run TestUnifiedPattern
//
func TestUnifiedPattern(t *testing.T) {
	// Skip if pattern address not provided
	// In CI/CD, this would be set after starting the pattern executable
	patternAddr := "localhost:50051"

	t.Logf("Testing pattern at address: %s", patternAddr)

	// Configure the pattern test
	config := framework.PatternTestConfig{
		Name:    "test-pattern",
		Address: patternAddr,
		Config: map[string]interface{}{
			"test_mode": true,
			"timeout":   30,
		},
		Timeout: 30 * time.Second,
	}

	// Run unified tests - this will:
	// 1. Connect to the pattern
	// 2. Call Initialize() to get interface list
	// 3. Look up test suites for those interfaces
	// 4. Run the appropriate tests
	framework.RunUnifiedPatternTests(t, config)
}

// TestUnifiedPattern_WithFilter demonstrates filtering specific interfaces
func TestUnifiedPattern_WithFilter(t *testing.T) {
	t.Skip("Example test - enable when pattern executable is running")

	config := framework.PatternTestConfig{
		Name:    "test-pattern",
		Address: "localhost:50051",
		Config: map[string]interface{}{
			"test_mode": true,
		},
		Timeout: 30 * time.Second,
	}

	opts := framework.UnifiedTestOptions{
		Config: config,
		// Only test KeyValue interfaces, skip PubSub
		FilterInterfaces: []string{
			"KeyValueBasicInterface",
			"KeyValueTTLInterface",
		},
		Sequential:     false,
		FailFast:       false,
		CollectResults: true,
	}

	report := framework.RunUnifiedPatternTestsWithOptions(t, opts)

	// Print summary
	if report != nil {
		t.Logf("Test Results:")
		t.Logf("  Total: %d", report.TotalTests)
		t.Logf("  Passed: %d", report.PassedTests)
		t.Logf("  Failed: %d", report.FailedTests)
		t.Logf("  Skipped: %d", report.SkippedTests)
	}
}

// TestDiscoverInterfaces is a utility test to discover what interfaces a pattern supports
// Useful for debugging and understanding a pattern's capabilities
func TestDiscoverInterfaces(t *testing.T) {
	t.Skip("Example test - enable to discover interfaces of a running pattern")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := framework.PatternTestConfig{
		Name:    "test-pattern",
		Address: "localhost:50051",
		Config:  map[string]interface{}{},
	}

	err := framework.DiscoverAndPrintInterfaces(ctx, config)
	require.NoError(t, err, "Failed to discover interfaces")
}

// Example: Register test suites for KeyValue interfaces
// These would normally be in separate files in tests/acceptance/suites/
func init() {
	// Register KeyValueBasicInterface test suite
	framework.MustRegisterTestSuite(framework.TestSuite{
		InterfaceName: "KeyValueBasicInterface",
		Pattern:       framework.PatternKeyValueBasic,
		Description:   "Tests basic KeyValue operations: Set, Get, Delete, Exists",
		Tests: []framework.PatternTest{
			{
				Name: "SetAndGet",
				Func: testKeyValueSetGet,
			},
			{
				Name: "GetNonExistent",
				Func: testKeyValueGetNonExistent,
			},
			{
				Name: "Delete",
				Func: testKeyValueDelete,
			},
			{
				Name: "Exists",
				Func: testKeyValueExists,
			},
		},
	})

	// Register PubSubBasicInterface test suite
	framework.MustRegisterTestSuite(framework.TestSuite{
		InterfaceName: "PubSubBasicInterface",
		Pattern:       framework.PatternPubSubBasic,
		Description:   "Tests basic PubSub operations: Publish, Subscribe",
		Tests: []framework.PatternTest{
			{
				Name:    "PublishAndSubscribe",
				Func:    testPubSubPublishSubscribe,
				Timeout: 10 * time.Second,
			},
		},
	})
}

// Test functions for KeyValue interface
func testKeyValueSetGet(t *testing.T, driver interface{}, caps framework.Capabilities) {
	conn, ok := driver.(*grpc.ClientConn)
	require.True(t, ok, "Driver must be gRPC connection")

	client := pb_kv.NewKeyValueBasicInterfaceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set a value
	setResp, err := client.Set(ctx, &pb_kv.SetRequest{
		Key:   "test-key",
		Value: []byte("test-value"),
		Ttl:   0,
	})
	require.NoError(t, err, "Set operation failed")
	require.True(t, setResp.Success, "Set should succeed")

	// Get the value
	getResp, err := client.Get(ctx, &pb_kv.GetRequest{
		Key: "test-key",
	})
	require.NoError(t, err, "Get operation failed")
	require.True(t, getResp.Found, "Key should be found")
	assert.Equal(t, "test-value", string(getResp.Value), "Value mismatch")
}

func testKeyValueGetNonExistent(t *testing.T, driver interface{}, caps framework.Capabilities) {
	conn, ok := driver.(*grpc.ClientConn)
	require.True(t, ok, "Driver must be gRPC connection")

	client := pb_kv.NewKeyValueBasicInterfaceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	getResp, err := client.Get(ctx, &pb_kv.GetRequest{
		Key: "non-existent-key",
	})
	require.NoError(t, err, "Get operation should not error")
	assert.False(t, getResp.Found, "Key should not be found")
}

func testKeyValueDelete(t *testing.T, driver interface{}, caps framework.Capabilities) {
	conn, ok := driver.(*grpc.ClientConn)
	require.True(t, ok, "Driver must be gRPC connection")

	client := pb_kv.NewKeyValueBasicInterfaceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set a value
	_, err := client.Set(ctx, &pb_kv.SetRequest{
		Key:   "delete-key",
		Value: []byte("delete-value"),
		Ttl:   0,
	})
	require.NoError(t, err)

	// Delete it
	delResp, err := client.Delete(ctx, &pb_kv.DeleteRequest{
		Key: "delete-key",
	})
	require.NoError(t, err, "Delete operation failed")
	require.True(t, delResp.Success, "Delete should succeed")

	// Verify it's gone
	getResp, err := client.Get(ctx, &pb_kv.GetRequest{
		Key: "delete-key",
	})
	require.NoError(t, err)
	assert.False(t, getResp.Found, "Key should be deleted")
}

func testKeyValueExists(t *testing.T, driver interface{}, caps framework.Capabilities) {
	conn, ok := driver.(*grpc.ClientConn)
	require.True(t, ok, "Driver must be gRPC connection")

	client := pb_kv.NewKeyValueBasicInterfaceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check non-existent key
	existsResp, err := client.Exists(ctx, &pb_kv.ExistsRequest{
		Key: "nonexistent",
	})
	require.NoError(t, err)
	assert.False(t, existsResp.Exists, "Non-existent key should not exist")

	// Set a key
	_, err = client.Set(ctx, &pb_kv.SetRequest{
		Key:   "exists-key",
		Value: []byte("value"),
		Ttl:   0,
	})
	require.NoError(t, err)

	// Check it exists
	existsResp, err = client.Exists(ctx, &pb_kv.ExistsRequest{
		Key: "exists-key",
	})
	require.NoError(t, err)
	assert.True(t, existsResp.Exists, "Key should exist")
}

// Test functions for PubSub interface
func testPubSubPublishSubscribe(t *testing.T, driver interface{}, caps framework.Capabilities) {
	conn, ok := driver.(*grpc.ClientConn)
	require.True(t, ok, "Driver must be gRPC connection")

	client := pb_pubsub.NewPubSubBasicInterfaceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topic := fmt.Sprintf("test-topic-%d", time.Now().Unix())

	// Subscribe
	subResp, err := client.Subscribe(ctx, &pb_pubsub.SubscribeRequest{
		Topic:        topic,
		SubscriberId: "test-subscriber",
	})
	require.NoError(t, err, "Subscribe failed")
	require.True(t, subResp.Success, "Subscribe should succeed")

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// Publish a message
	pubResp, err := client.Publish(ctx, &pb_pubsub.PublishRequest{
		Topic:   topic,
		Payload: []byte("test-message"),
		Metadata: map[string]string{
			"test": "true",
		},
	})
	require.NoError(t, err, "Publish failed")
	assert.NotEmpty(t, pubResp.MessageId, "Should get message ID")

	// TODO: Receive message (requires streaming RPC or polling)
	// This is a simplified example - actual implementation would use streaming
}
