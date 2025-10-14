package consumer_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestConsumerProxyArchitecture tests the consumer connecting back to proxy
// This is the CORRECT architecture - pattern connects TO proxy, not the other way around
func TestConsumerProxyArchitecture(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name: "ProxyControlPlane",
			Func: testProxyControlPlane,
		},
	}

	framework.RunPatternTests(t, framework.Pattern("ConsumerProcess"), tests)
}

// testProxyControlPlane demonstrates correct proxy-to-pattern architecture
func testProxyControlPlane(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// 1. Start PROXY control plane server (this is what the real proxy does)
	// Use port 0 for dynamic allocation to avoid conflicts in parallel tests
	proxyControlPlane := plugin.NewProxyControlPlaneServer(0)
	err := proxyControlPlane.Start(ctx)
	require.NoError(t, err, "Failed to start proxy control plane")
	defer proxyControlPlane.Stop(ctx)

	// Get the actual port that was assigned
	proxyPort := proxyControlPlane.GetPort()
	t.Logf("✅ Proxy control plane started on :%d", proxyPort)

	// 2. Build consumer-runner
	consumerExe, err := buildConsumerRunner(t)
	require.NoError(t, err, "Failed to build consumer-runner")

	// 3. Launch consumer with proxy address (consumer connects BACK to proxy)
	proxyAddr := fmt.Sprintf("localhost:%d", proxyPort)
	cmd := exec.CommandContext(ctx, consumerExe,
		"-proxy-addr", proxyAddr,
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

	t.Log("✅ Consumer launched, connecting to proxy...")

	// 4. Wait for consumer to connect and register
	time.Sleep(1 * time.Second)

	instances := proxyControlPlane.GetPatternInstances()
	require.Len(t, instances, 1, "Expected 1 connected pattern")
	instanceID := instances[0]

	t.Logf("✅ Consumer registered with instance ID: %s", instanceID)

	// 5. Send Initialize command to consumer
	initCmd := &pb.ProxyCommand{
		Command: &pb.ProxyCommand_Initialize{
			Initialize: &pb.InitializeRequest{
				Name:    "proxy-test-consumer",
				Version: "0.1.0",
				Config:  buildProtoConfig(backends.NATSUrl, backends.MemStoreAddr),
			},
		},
	}

	resp, err := proxyControlPlane.SendCommand(instanceID, initCmd)
	require.NoError(t, err, "Failed to send Initialize command")

	initResp := resp.GetInitializeResponse()
	require.NotNil(t, initResp, "Expected InitializeResponse")
	assert.True(t, initResp.Success, "Initialize should succeed: %s", initResp.Error)

	t.Log("✅ Consumer initialized successfully")

	// 6. Send Start command
	startCmd := &pb.ProxyCommand{
		Command: &pb.ProxyCommand_Start{
			Start: &pb.StartRequest{},
		},
	}

	resp, err = proxyControlPlane.SendCommand(instanceID, startCmd)
	require.NoError(t, err, "Failed to send Start command")

	startResp := resp.GetStartResponse()
	require.NotNil(t, startResp, "Expected StartResponse")
	assert.True(t, startResp.Success, "Start should succeed: %s", startResp.Error)

	t.Log("✅ Consumer started successfully")

	// 7. Give consumer time to start consuming
	time.Sleep(500 * time.Millisecond)

	// 8. Check health via proxy
	healthCmd := &pb.ProxyCommand{
		Command: &pb.ProxyCommand_HealthCheck{
			HealthCheck: &pb.HealthCheckRequest{},
		},
	}

	resp, err = proxyControlPlane.SendCommand(instanceID, healthCmd)
	require.NoError(t, err, "Failed to send HealthCheck command")

	healthResp := resp.GetHealthResponse()
	require.NotNil(t, healthResp, "Expected HealthCheckResponse")

	t.Logf("✅ Consumer health: %s - %s", healthResp.Status, healthResp.Message)

	// 9. Publish test messages
	pubsub := backends.MessageSource
	for i := 0; i < 3; i++ {
		_, err = pubsub.Publish(ctx, "proxy.test.topic",
			[]byte(fmt.Sprintf("msg-%d", i)), nil)
		require.NoError(t, err, "Failed to publish message %d", i)
	}

	t.Log("✅ Published 3 test messages")

	// Give consumer time to process
	time.Sleep(1 * time.Second)

	// 10. Stop consumer via proxy
	stopCmd := &pb.ProxyCommand{
		Command: &pb.ProxyCommand_Stop{
			Stop: &pb.StopRequest{
				TimeoutSeconds: 5,
			},
		},
	}

	resp, err = proxyControlPlane.SendCommand(instanceID, stopCmd)
	require.NoError(t, err, "Failed to send Stop command")

	stopResp := resp.GetStopResponse()
	require.NotNil(t, stopResp, "Expected StopResponse")
	assert.True(t, stopResp.Success, "Stop should succeed: %s", stopResp.Error)

	t.Log("✅ Consumer stopped successfully")

	// Wait for process to exit
	time.Sleep(500 * time.Millisecond)

	t.Log("✅ Proxy-to-pattern control plane architecture validated!")
	t.Log("Architecture flow:")
	t.Logf("  ✅ 1. Proxy starts control plane server (:%d)", proxyPort)
	t.Logf("  ✅ 2. Pattern launched with -proxy-addr %s", proxyAddr)
	t.Log("  ✅ 3. Pattern connects BACK to proxy")
	t.Log("  ✅ 4. Pattern registers itself with proxy")
	t.Log("  ✅ 5. Proxy sends Initialize command via bidirectional stream")
	t.Log("  ✅ 6. Proxy sends Start command")
	t.Log("  ✅ 7. Proxy monitors health")
	t.Log("  ✅ 8. Proxy sends Stop command")
}

// buildProtoConfig creates a protobuf Struct for consumer configuration
func buildProtoConfig(natsUrl, memstoreAddr string) *structpb.Struct {
	config := map[string]interface{}{
		"slots": map[string]interface{}{
			"message_source": map[string]interface{}{
				"driver": "nats",
				"config": map[string]interface{}{
					"url":              natsUrl,
					"max_reconnects":   10,
					"reconnect_wait":   "2s",
					"timeout":          "5s",
					"enable_jetstream": false,
				},
			},
		},
		"behavior": map[string]interface{}{
			"consumer_group":  "proxy-test-group",
			"topic":           "proxy.test.topic",
			"max_retries":     3,
			"auto_commit":     false,
			"batch_size":      1,
			"commit_interval": "1s",
		},
	}

	// Add state store if address provided
	if memstoreAddr != "" {
		slots := config["slots"].(map[string]interface{})
		slots["state_store"] = map[string]interface{}{
			"driver": "memstore",
			"config": map[string]interface{}{
				"address": memstoreAddr,
			},
		}
		// Enable auto-commit when stateful
		behavior := config["behavior"].(map[string]interface{})
		behavior["auto_commit"] = true
	}

	configStruct, err := plugin.BuildConfigStruct(config)
	if err != nil {
		panic(fmt.Sprintf("failed to build config struct: %v", err))
	}

	return configStruct
}
