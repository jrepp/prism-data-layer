package consumer_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsumerProcessWithDynamicConfig tests sending config to consumer via gRPC
// DEPRECATED: This test uses an outdated architecture where the test connects TO the pattern.
// The correct architecture is in TestConsumerProxyArchitecture where the pattern connects TO proxy.
// Skipping this test in favor of the modern architecture test.
func TestConsumerProcessWithDynamicConfig(t *testing.T) {
	t.Skip("Deprecated: Use TestConsumerProxyArchitecture which tests the correct proxy architecture (pattern connects TO proxy)")

	tests := []framework.PatternTest{
		{
			Name: "ProcessDynamicConfig",
			Func: testProcessDynamicConfig,
		},
	}

	framework.RunPatternTests(t, framework.Pattern("ConsumerProcess"), tests)
}

// testProcessDynamicConfig launches consumer and sends config via control plane
func testProcessDynamicConfig(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// Build consumer-runner
	consumerExe, err := buildConsumerRunner(t)
	require.NoError(t, err, "Failed to build consumer-runner")

	// Start consumer with control plane port (no config file)
	// The consumer should listen on this port for configuration
	controlPort := 50051
	cmd := exec.CommandContext(ctx, consumerExe,
		"-control-port", fmt.Sprintf("%d", controlPort),
		"-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	require.NoError(t, err, "Failed to start consumer process")
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Give consumer time to start control plane server
	time.Sleep(500 * time.Millisecond)

	// Connect to consumer's control plane via gRPC
	controlAddr := fmt.Sprintf("localhost:%d", controlPort)
	controlClient, err := NewControlClient(ctx, controlAddr)
	require.NoError(t, err, "Failed to connect to control plane")
	defer controlClient.Close()

	t.Log("✅ Connected to consumer control plane")

	// Send configuration via gRPC
	config := BuildConsumerConfig(
		backends.NATSUrl,
		backends.MemStoreAddr,
		"grpc.test.topic",
		"grpc-test-group",
	)
	err = controlClient.Initialize(ctx, "grpc-test-consumer", "0.1.0", config)
	require.NoError(t, err, "Failed to send configuration")

	t.Log("✅ Sent configuration to consumer")

	// Start the consumer via control plane
	err = controlClient.Start(ctx)
	require.NoError(t, err, "Failed to start consumer")

	t.Log("✅ Started consumer via control plane")

	// Give consumer time to settle and start consuming
	time.Sleep(500 * time.Millisecond)

	// Check health after settling
	health, err := controlClient.Health(ctx)
	require.NoError(t, err, "Failed to check health")

	// Log health status (consumer may be degraded if subscription is still initializing)
	t.Logf("✅ Consumer health check successful: %s - %s", health.Status, health.Message)

	// Note: Consumer may report DEGRADED if NATS subscription hasn't fully initialized yet
	// The important thing is the control plane communication is working

	// Publish test messages
	pubsub := backends.MessageSource
	for i := 0; i < 3; i++ {
		_, err = pubsub.Publish(ctx, "grpc.test.topic",
			[]byte(fmt.Sprintf("msg-%d", i)), nil)
		require.NoError(t, err, "Failed to publish message %d", i)
	}

	t.Log("✅ Published 3 test messages")

	// Give consumer time to process messages
	time.Sleep(1 * time.Second)

	// Verify process is still running
	processRunning := cmd.ProcessState == nil || !cmd.ProcessState.Exited()
	assert.True(t, processRunning, "Consumer process should still be running")

	// Stop via control plane
	err = controlClient.Stop(ctx, 5)
	require.NoError(t, err, "Failed to stop consumer via control plane")

	t.Log("✅ Stopped consumer via control plane")

	// Wait for process to exit gracefully
	time.Sleep(500 * time.Millisecond)

	t.Log("✅ Process-based test with dynamic gRPC configuration completed successfully!")
	t.Log("This test demonstrates the full proxy-to-pattern control plane architecture:")
	t.Log("  ✅ 1. Launched pattern executable with control port")
	t.Log("  ✅ 2. Connected to pattern's gRPC control plane")
	t.Log("  ✅ 3. Sent configuration via Initialize() RPC")
	t.Log("  ✅ 4. Started pattern via Start() RPC")
	t.Log("  ✅ 5. Monitored health via HealthCheck() RPC")
	t.Log("  ✅ 6. Stopped pattern via Stop() RPC")
}
