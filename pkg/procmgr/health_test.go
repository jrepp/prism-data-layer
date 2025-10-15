package procmgr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessManager_Health tests the Health() API
func TestProcessManager_Health(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Initial health - no processes
	health := pm.Health()
	assert.Equal(t, 0, health.TotalProcesses)
	assert.Equal(t, 0, health.RunningProcesses)
	assert.Equal(t, 0, health.TerminatingProcesses)
	assert.Equal(t, 0, health.FailedProcesses)

	// Create 3 processes
	for i := 1; i <= 3; i++ {
		pm.UpdateProcess(ProcessUpdate{
			ID:         ProcessID("test-" + string(rune('0'+i))),
			UpdateType: UpdateTypeCreate,
			Config:     &struct{}{},
		})
	}

	// Wait for all to sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 3
	}, 2*time.Second, 50*time.Millisecond)

	// Check health
	health = pm.Health()
	assert.Equal(t, 3, health.TotalProcesses)
	assert.Equal(t, 3, health.RunningProcesses)
	assert.Equal(t, 0, health.TerminatingProcesses)
	assert.Equal(t, 0, health.FailedProcesses)
	assert.Len(t, health.Processes, 3)

	// Check individual process health
	for i := 1; i <= 3; i++ {
		id := ProcessID("test-" + string(rune('0'+i)))
		procHealth, exists := health.Processes[id]
		require.True(t, exists)
		assert.Equal(t, ProcessStateSyncing, procHealth.State)
		assert.True(t, procHealth.Healthy)
		assert.Greater(t, procHealth.Uptime, time.Duration(0))
	}
}

// TestProcessManager_HealthWithErrors tests health reporting with errors
func TestProcessManager_HealthWithErrors(t *testing.T) {
	syncer := &mockSyncer{
		syncErr: assert.AnError,
	}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithBackOffPeriod(2*time.Second), // Shorter backoff for faster test
	)
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for multiple sync attempts (errors)
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 6
	}, 20*time.Second, 100*time.Millisecond)

	health := pm.Health()
	assert.Equal(t, 1, health.TotalProcesses)
	assert.Equal(t, 1, health.FailedProcesses) // errorCount > 5

	procHealth := health.Processes["test-1"]
	assert.False(t, procHealth.Healthy)
	assert.Greater(t, procHealth.ErrorCount, 5)
}

// TestProcessManager_HealthWithTerminating tests health with terminating processes
func TestProcessManager_HealthWithTerminating(t *testing.T) {
	syncer := &mockSyncer{
		syncTerminatingDuration: 2 * time.Second, // Slow termination
	}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Terminate
	gracePeriod := int64(5)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod,
		},
	})

	// Wait for terminating state
	require.Eventually(t, func() bool {
		health := pm.Health()
		return health.TerminatingProcesses > 0
	}, 2*time.Second, 50*time.Millisecond)

	health := pm.Health()
	assert.Equal(t, 1, health.TotalProcesses)
	assert.Equal(t, 0, health.RunningProcesses)
	assert.Equal(t, 1, health.TerminatingProcesses)

	procHealth := health.Processes["test-1"]
	assert.Equal(t, ProcessStateTerminating, procHealth.State)
}

// TestProcessManager_HealthWorkQueueDepth tests work queue depth reporting
func TestProcessManager_HealthWorkQueueDepth(t *testing.T) {
	syncer := &mockSyncer{
		syncErr: assert.AnError,
	}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithBackOffPeriod(10*time.Second), // Long backoff
	)
	defer pm.Shutdown(context.Background())

	// Create multiple processes that will fail and be requeued
	for i := 1; i <= 5; i++ {
		pm.UpdateProcess(ProcessUpdate{
			ID:         ProcessID("test-" + string(rune('0'+i))),
			UpdateType: UpdateTypeCreate,
			Config:     &struct{}{},
		})
	}

	// Wait for initial syncs
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 5
	}, 2*time.Second, 50*time.Millisecond)

	// Give time for work queue to fill up with retry items
	time.Sleep(500 * time.Millisecond)

	health := pm.Health()
	assert.GreaterOrEqual(t, health.WorkQueueDepth, 0)
	// Work queue depth should be reasonable (processes are waiting for retry)
	assert.LessOrEqual(t, health.WorkQueueDepth, 5)
}
