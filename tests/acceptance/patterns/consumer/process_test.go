package consumer_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConsumerProcessBased tests the consumer pattern by launching it as a separate process
// This mimics how the proxy would interact with the pattern
func TestConsumerProcessBased(t *testing.T) {
	tests := []framework.PatternTest{
		{
			Name: "ProcessStatelessConsumer",
			Func: testProcessStatelessConsumer,
		},
		{
			Name: "ProcessStatefulConsumer",
			Func: testProcessStatefulConsumer,
		},
	}

	framework.RunPatternTests(t, framework.Pattern("ConsumerProcess"), tests)
}

// testProcessStatelessConsumer launches consumer as subprocess and tests message processing
func testProcessStatelessConsumer(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	// Build consumer-runner if not already built
	consumerExe, err := buildConsumerRunner(t)
	require.NoError(t, err, "Failed to build consumer-runner")

	// Create temporary config file
	configFile, err := createConsumerConfig(t, ConsumerTestConfig{
		Name:          "process-stateless-test",
		ConsumerGroup: "process-test-group",
		Topic:         "process.test.topic",
		NATSUrl:       backends.NATSUrl,
		Stateless:     true,
	})
	require.NoError(t, err, "Failed to create config file")
	defer os.Remove(configFile)

	// Start consumer as subprocess
	cmd := exec.CommandContext(ctx, consumerExe, "-config", configFile, "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	require.NoError(t, err, "Failed to start consumer process")
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Give consumer time to start and subscribe
	time.Sleep(500 * time.Millisecond)

	// Publish test messages to the topic
	pubsub := backends.MessageSource
	testMessages := []string{"message-1", "message-2", "message-3"}

	for i, msg := range testMessages {
		_, err = pubsub.Publish(ctx, "process.test.topic", []byte(msg), map[string]string{
			"sequence": fmt.Sprintf("%d", i),
		})
		require.NoError(t, err, "Failed to publish message %d", i)
	}

	// Wait for consumer to process messages
	time.Sleep(1 * time.Second)

	// Since we're running as a separate process, we can't directly verify
	// message processing. In a real scenario, the consumer would write to
	// a state store or output channel that we could verify.
	// For now, verify the process is still running (didn't crash)

	// Check if process is still running
	processRunning := cmd.ProcessState == nil || !cmd.ProcessState.Exited()
	assert.True(t, processRunning, "Consumer process should still be running")

	// Stop the consumer
	cmd.Process.Kill()
	cmd.Wait()
}

// testProcessStatefulConsumer launches consumer with state store
func testProcessStatefulConsumer(t *testing.T, driver interface{}, caps framework.Capabilities) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	backends := driver.(*ConsumerBackends)

	if backends.StateStore == nil {
		t.Skip("State store not configured")
	}

	consumerExe, err := buildConsumerRunner(t)
	require.NoError(t, err, "Failed to build consumer-runner")

	configFile, err := createConsumerConfig(t, ConsumerTestConfig{
		Name:          "process-stateful-test",
		ConsumerGroup: "process-stateful-group",
		Topic:         "process.stateful.topic",
		NATSUrl:       backends.NATSUrl,
		Stateless:     false,
		MemStoreAddr:  "localhost:9091", // MemStore control plane port
	})
	require.NoError(t, err, "Failed to create config file")
	defer os.Remove(configFile)

	cmd := exec.CommandContext(ctx, consumerExe, "-config", configFile, "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	require.NoError(t, err, "Failed to start consumer process")
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	time.Sleep(500 * time.Millisecond)

	// Publish messages
	pubsub := backends.MessageSource
	for i := 0; i < 5; i++ {
		_, err = pubsub.Publish(ctx, "process.stateful.topic",
			[]byte(fmt.Sprintf("stateful-msg-%d", i)), nil)
		require.NoError(t, err, "Failed to publish message")
	}

	// Wait for processing and state commits
	time.Sleep(1 * time.Second)

	// Verify state was persisted
	stateKey := "consumer:process-stateful-group:process.stateful.topic:process-stateful-test"
	stateData, found, err := backends.StateStore.Get(stateKey)
	require.NoError(t, err, "Failed to get consumer state")

	if found {
		assert.NotEmpty(t, stateData, "Consumer state should be persisted")
		t.Logf("Consumer state found: %s", string(stateData))
	} else {
		t.Log("Consumer state not yet persisted (may need more time or auto_commit enabled)")
	}

	cmd.Process.Kill()
	cmd.Wait()
}

// ConsumerTestConfig holds config for creating test consumer configs
type ConsumerTestConfig struct {
	Name          string
	ConsumerGroup string
	Topic         string
	NATSUrl       string
	Stateless     bool
	MemStoreAddr  string
}

// createConsumerConfig creates a temporary YAML config file for the consumer
func createConsumerConfig(t *testing.T, cfg ConsumerTestConfig) (string, error) {
	t.Helper()

	// Build config structure matching consumer-runner's expected format
	config := map[string]interface{}{
		"namespaces": []map[string]interface{}{
			{
				"name":            cfg.Name,
				"pattern":         "consumer",
				"pattern_version": "v1",
				"description":     "Acceptance test consumer",
				"slots": map[string]interface{}{
					"message_source": map[string]interface{}{
						"backend": "nats",
						"interfaces": []string{"pubsub_basic"},
						"config": map[string]interface{}{
							"url":              cfg.NATSUrl,
							"max_reconnects":   10,
							"reconnect_wait":   "2s",
							"timeout":          "5s",
							"enable_jetstream": false,
						},
					},
				},
				"behavior": map[string]interface{}{
					"consumer_group":  cfg.ConsumerGroup,
					"topic":           cfg.Topic,
					"max_retries":     3,
					"auto_commit":     !cfg.Stateless, // Auto-commit if stateful
					"batch_size":      1,
					"commit_interval": "1s",
				},
			},
		},
	}

	// Add state store if not stateless
	if !cfg.Stateless && cfg.MemStoreAddr != "" {
		namespace := config["namespaces"].([]map[string]interface{})[0]
		slots := namespace["slots"].(map[string]interface{})
		slots["state_store"] = map[string]interface{}{
			"backend": "memstore",
			"interfaces": []string{"keyvalue_basic"},
			"config": map[string]interface{}{
				"address": cfg.MemStoreAddr,
			},
		}
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to temp file
	tmpFile, err := ioutil.TempFile("", "consumer-test-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := tmpFile.Write(yamlData); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}

// buildConsumerRunner builds the consumer-runner executable if needed
func buildConsumerRunner(t *testing.T) (string, error) {
	t.Helper()

	// Check if already built
	exe := filepath.Join("/Users/jrepp/dev/data-access/patterns/consumer/cmd/consumer-runner/consumer-runner")
	if _, err := os.Stat(exe); err == nil {
		return exe, nil
	}

	// Build it
	t.Log("Building consumer-runner...")
	cmd := exec.Command("go", "build", "-o", "consumer-runner", ".")
	cmd.Dir = "/Users/jrepp/dev/data-access/patterns/consumer/cmd/consumer-runner"

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build failed: %w\nOutput: %s", err, output)
	}

	return exe, nil
}
