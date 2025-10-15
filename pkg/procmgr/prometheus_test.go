package procmgr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrometheusMetricsCollector_StateTransitions tests state transition metrics
func TestPrometheusMetricsCollector_StateTransitions(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("test")

	// Record some state transitions
	pmc.ProcessStateTransition("proc-1", ProcessStateStarting, ProcessStateSyncing)
	pmc.ProcessStateTransition("proc-1", ProcessStateSyncing, ProcessStateTerminating)
	pmc.ProcessStateTransition("proc-2", ProcessStateStarting, ProcessStateSyncing)

	// Verify metric exists and has correct value
	count, err := testutil.GatherAndCount(pmc.registry, "test_process_state_transitions_total")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify specific labels
	expected := `
		# HELP test_process_state_transitions_total Total number of process state transitions
		# TYPE test_process_state_transitions_total counter
		test_process_state_transitions_total{from_state="Starting",process_id="proc-1",to_state="Syncing"} 1
		test_process_state_transitions_total{from_state="Syncing",process_id="proc-1",to_state="Terminating"} 1
		test_process_state_transitions_total{from_state="Starting",process_id="proc-2",to_state="Syncing"} 1
	`
	err = testutil.GatherAndCompare(pmc.registry, strings.NewReader(expected), "test_process_state_transitions_total")
	assert.NoError(t, err)
}

// TestPrometheusMetricsCollector_SyncDuration tests sync duration metrics
func TestPrometheusMetricsCollector_SyncDuration(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("test")

	// Record some sync durations
	pmc.ProcessSyncDuration("proc-1", UpdateTypeCreate, 100*time.Millisecond, nil)
	pmc.ProcessSyncDuration("proc-1", UpdateTypeSync, 50*time.Millisecond, nil)
	pmc.ProcessSyncDuration("proc-2", UpdateTypeCreate, 200*time.Millisecond, errors.New("failed"))

	// Verify metric exists
	count, err := testutil.GatherAndCount(pmc.registry, "test_process_sync_duration_seconds")
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// Verify histogram has observations
	metricFamilies, err := pmc.registry.Gather()
	require.NoError(t, err)

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_process_sync_duration_seconds" {
			found = true
			assert.Greater(t, len(mf.GetMetric()), 0)
		}
	}
	assert.True(t, found, "Should have sync duration metric")
}

// TestPrometheusMetricsCollector_Errors tests error metrics
func TestPrometheusMetricsCollector_Errors(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("test")

	// Record some errors
	pmc.ProcessError("proc-1", "sync_error")
	pmc.ProcessError("proc-1", "sync_error")
	pmc.ProcessError("proc-1", "termination_error")
	pmc.ProcessError("proc-2", "sync_error")

	// Verify metric
	expected := `
		# HELP test_process_errors_total Total number of process errors
		# TYPE test_process_errors_total counter
		test_process_errors_total{error_type="sync_error",process_id="proc-1"} 2
		test_process_errors_total{error_type="termination_error",process_id="proc-1"} 1
		test_process_errors_total{error_type="sync_error",process_id="proc-2"} 1
	`
	err := testutil.GatherAndCompare(pmc.registry, strings.NewReader(expected), "test_process_errors_total")
	assert.NoError(t, err)
}

// TestPrometheusMetricsCollector_WorkQueue tests work queue metrics
func TestPrometheusMetricsCollector_WorkQueue(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("test")

	// Record work queue operations
	pmc.WorkQueueDepth(5)
	pmc.WorkQueueAdd("proc-1", 1*time.Second)
	pmc.WorkQueueAdd("proc-2", 2*time.Second)
	pmc.WorkQueueRetry("proc-1")
	pmc.WorkQueueBackoffDuration("proc-1", 2*time.Second)

	// Verify queue depth gauge
	expected := `
		# HELP test_work_queue_depth Current depth of the work queue
		# TYPE test_work_queue_depth gauge
		test_work_queue_depth 5
	`
	err := testutil.GatherAndCompare(pmc.registry, strings.NewReader(expected), "test_work_queue_depth")
	assert.NoError(t, err)

	// Verify queue adds counter
	count, err := testutil.GatherAndCount(pmc.registry, "test_work_queue_adds_total")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify retries counter
	expectedRetries := `
		# HELP test_work_queue_retries_total Total number of work queue retry operations
		# TYPE test_work_queue_retries_total counter
		test_work_queue_retries_total{process_id="proc-1"} 1
	`
	err = testutil.GatherAndCompare(pmc.registry, strings.NewReader(expectedRetries), "test_work_queue_retries_total")
	assert.NoError(t, err)
}

// TestPrometheusMetricsCollector_Integration tests metrics with real ProcessManager
func TestPrometheusMetricsCollector_Integration(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("test")
	syncer := &mockSyncer{}

	pm := NewProcessManager(
		WithSyncer(syncer),
		WithMetricsCollector(pmc),
		WithResyncInterval(10*time.Second),
		WithBackOffPeriod(2*time.Second),
	)
	defer pm.Shutdown(context.Background())

	// Create a process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-proc",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Verify metrics were collected
	metricFamilies, err := pmc.registry.Gather()
	require.NoError(t, err)
	assert.Greater(t, len(metricFamilies), 0, "Should have collected metrics")

	// Check for specific metrics
	var foundStateTransitions, foundSyncDuration, foundQueueDepth bool
	for _, mf := range metricFamilies {
		switch mf.GetName() {
		case "test_process_state_transitions_total":
			foundStateTransitions = true
		case "test_process_sync_duration_seconds":
			foundSyncDuration = true
		case "test_work_queue_depth":
			foundQueueDepth = true
		}
	}

	assert.True(t, foundStateTransitions, "Should have state transition metrics")
	assert.True(t, foundSyncDuration, "Should have sync duration metrics")
	assert.True(t, foundQueueDepth, "Should have work queue depth metrics")
}

// TestPrometheusMetricsCollector_CustomNamespace tests custom namespace
func TestPrometheusMetricsCollector_CustomNamespace(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("custom")

	pmc.ProcessStateTransition("proc-1", ProcessStateStarting, ProcessStateSyncing)

	// Verify custom namespace
	metricFamilies, err := pmc.registry.Gather()
	require.NoError(t, err)

	var found bool
	for _, mf := range metricFamilies {
		if strings.HasPrefix(mf.GetName(), "custom_") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should use custom namespace")
}

// TestPrometheusMetricsCollector_Registry tests Registry() accessor
func TestPrometheusMetricsCollector_Registry(t *testing.T) {
	pmc := NewPrometheusMetricsCollector("test")

	registry := pmc.Registry()
	assert.NotNil(t, registry)
	assert.IsType(t, &prometheus.Registry{}, registry)
}
