package procmgr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSyncer is a mock implementation of ProcessSyncer for testing
type mockSyncer struct {
	mu sync.Mutex

	syncCalled            int
	syncTerminatingCalled int
	syncTerminatedCalled  int

	syncErr            error
	syncTerminatingErr error
	syncTerminatedErr  error

	syncDuration            time.Duration
	syncTerminatingDuration time.Duration
	syncTerminatedDuration  time.Duration

	syncTerminal bool
}

func (ms *mockSyncer) SyncProcess(ctx context.Context, updateType UpdateType, config interface{}) (bool, error) {
	ms.mu.Lock()
	ms.syncCalled++
	terminal := ms.syncTerminal
	err := ms.syncErr
	duration := ms.syncDuration
	ms.mu.Unlock()

	if duration > 0 {
		select {
		case <-time.After(duration):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	return terminal, err
}

func (ms *mockSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn ProcessStatusFunc) error {
	ms.mu.Lock()
	ms.syncTerminatingCalled++
	err := ms.syncTerminatingErr
	duration := ms.syncTerminatingDuration
	ms.mu.Unlock()

	if duration > 0 {
		select {
		case <-time.After(duration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return err
}

func (ms *mockSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
	ms.mu.Lock()
	ms.syncTerminatedCalled++
	err := ms.syncTerminatedErr
	duration := ms.syncTerminatedDuration
	ms.mu.Unlock()

	if duration > 0 {
		select {
		case <-time.After(duration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return err
}

func (ms *mockSyncer) getSyncCalled() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.syncCalled
}

func (ms *mockSyncer) getSyncTerminatingCalled() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.syncTerminatingCalled
}

func (ms *mockSyncer) getSyncTerminatedCalled() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.syncTerminatedCalled
}

// TestProcessManager_CreateProcess tests creating a single process
func TestProcessManager_CreateProcess(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync to complete
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond, "Sync should be called")

	status, ok := pm.GetProcessStatus("test-1")
	require.True(t, ok, "Process should exist")
	assert.Equal(t, ProcessStateSyncing, status.State, "Process should be in Syncing state")
	assert.Equal(t, 1, syncer.getSyncCalled(), "Sync should be called once")
}

// TestProcessManager_UpdateProcess tests updating an existing process
func TestProcessManager_UpdateProcess(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     map[string]string{"version": "1.0"},
	})

	// Wait for initial sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Update process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeUpdate,
		Config:     map[string]string{"version": "2.0"},
	})

	// Wait for update sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 2
	}, 2*time.Second, 50*time.Millisecond)

	assert.GreaterOrEqual(t, syncer.getSyncCalled(), 2, "Sync should be called at least twice")
}

// TestProcessManager_GracefulTermination tests graceful process termination
func TestProcessManager_GracefulTermination(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Terminate with grace period
	completedCh := make(chan struct{})
	gracePeriod := int64(10)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			CompletedCh:     completedCh,
			GracePeriodSecs: &gracePeriod,
		},
	})

	// Should complete within grace period
	select {
	case <-completedCh:
		// Success
	case <-time.After(15 * time.Second):
		t.Fatal("termination timeout")
	}

	assert.True(t, pm.IsProcessTerminated("test-1"), "Process should be terminated")
	assert.Equal(t, 1, syncer.getSyncTerminatingCalled(), "SyncTerminating should be called once")
}

// TestProcessManager_TerminationPhases tests multi-phase termination
func TestProcessManager_TerminationPhases(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync
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

	// Wait for terminating phase
	require.Eventually(t, func() bool {
		return syncer.getSyncTerminatingCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Wait for terminated phase
	require.Eventually(t, func() bool {
		return syncer.getSyncTerminatedCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Wait for finished state
	require.Eventually(t, func() bool {
		return pm.IsProcessFinished("test-1")
	}, 2*time.Second, 50*time.Millisecond)

	assert.Equal(t, 1, syncer.getSyncTerminatingCalled(), "SyncTerminating should be called once")
	assert.Equal(t, 1, syncer.getSyncTerminatedCalled(), "SyncTerminated should be called once")
}

// TestProcessManager_ContextCancellation tests context cancellation during sync
func TestProcessManager_ContextCancellation(t *testing.T) {
	syncer := &mockSyncer{
		syncDuration: 5 * time.Second, // Long-running sync
	}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait a bit for sync to start
	time.Sleep(100 * time.Millisecond)

	// Terminate (should cancel context)
	gracePeriod := int64(1)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod,
		},
	})

	// Should complete quickly due to context cancellation
	require.Eventually(t, func() bool {
		return syncer.getSyncTerminatingCalled() > 0
	}, 3*time.Second, 50*time.Millisecond, "Should transition to terminating quickly")
}

// TestProcessManager_ConcurrentProcesses tests multiple processes running concurrently
func TestProcessManager_ConcurrentProcesses(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create 10 processes concurrently
	numProcesses := 10
	var wg sync.WaitGroup
	for i := 0; i < numProcesses; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pm.UpdateProcess(ProcessUpdate{
				ID:         ProcessID(fmt.Sprintf("test-%d", id)),
				UpdateType: UpdateTypeCreate,
				Config:     &struct{}{},
			})
		}(i)
	}
	wg.Wait()

	// Wait for all to sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= numProcesses
	}, 3*time.Second, 50*time.Millisecond, "All processes should sync")

	// Check all processes exist and are in syncing state
	for i := 0; i < numProcesses; i++ {
		status, ok := pm.GetProcessStatus(ProcessID(fmt.Sprintf("test-%d", i)))
		assert.True(t, ok, "Process test-%d should exist", i)
		assert.Equal(t, ProcessStateSyncing, status.State, "Process test-%d should be syncing", i)
	}
}

// TestProcessManager_ConcurrentUpdates tests concurrent updates to same process
func TestProcessManager_ConcurrentUpdates(t *testing.T) {
	syncer := &mockSyncer{
		syncDuration: 100 * time.Millisecond, // Slow sync to allow overlap
	}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Send multiple updates concurrently
	numUpdates := 10
	var wg sync.WaitGroup
	for i := 0; i < numUpdates; i++ {
		wg.Add(1)
		go func(version int) {
			defer wg.Done()
			pm.UpdateProcess(ProcessUpdate{
				ID:         "test-1",
				UpdateType: UpdateTypeUpdate,
				Config:     map[string]int{"version": version},
			})
		}(i)
	}
	wg.Wait()

	// Wait for syncs to complete
	time.Sleep(2 * time.Second)

	// Should have called sync at least once, but not necessarily for every update
	// (intermediate updates can be skipped due to pending/active model)
	assert.GreaterOrEqual(t, syncer.getSyncCalled(), 1, "Sync should be called at least once")
	assert.LessOrEqual(t, syncer.getSyncCalled(), numUpdates, "Sync should not exceed number of updates")

	status, ok := pm.GetProcessStatus("test-1")
	require.True(t, ok, "Process should exist")
	assert.Equal(t, ProcessStateSyncing, status.State, "Process should be syncing")
}

// TestProcessManager_SyncError tests error handling during sync
func TestProcessManager_SyncError(t *testing.T) {
	syncer := &mockSyncer{
		syncErr: errors.New("sync failed"),
	}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync attempt
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	status, ok := pm.GetProcessStatus("test-1")
	require.True(t, ok, "Process should exist")
	assert.Greater(t, status.ErrorCount, 0, "Error count should increase")
	assert.NotNil(t, status.LastError, "Last error should be set")
}

// TestProcessManager_TerminalState tests process reaching terminal state
func TestProcessManager_TerminalState(t *testing.T) {
	syncer := &mockSyncer{
		syncTerminal: true, // Process reaches terminal state
	}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync (should return terminal=true)
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Should automatically transition to terminating
	require.Eventually(t, func() bool {
		return syncer.getSyncTerminatingCalled() > 0
	}, 2*time.Second, 50*time.Millisecond, "Should automatically terminate after terminal state")
}

// TestProcessManager_GracePeriodDecrease tests grace period can only decrease
func TestProcessManager_GracePeriodDecrease(t *testing.T) {
	syncer := &mockSyncer{
		syncTerminatingDuration: 100 * time.Millisecond,
	}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Terminate with grace period 10s
	gracePeriod1 := int64(10)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod1,
		},
	})

	time.Sleep(100 * time.Millisecond)

	// Try to increase grace period (should be ignored)
	gracePeriod2 := int64(20)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod2,
		},
	})

	time.Sleep(100 * time.Millisecond)

	// Try to decrease grace period (should be accepted)
	gracePeriod3 := int64(5)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod3,
		},
	})

	// Should complete with final grace period
	require.Eventually(t, func() bool {
		return pm.IsProcessTerminated("test-1")
	}, 10*time.Second, 50*time.Millisecond)
}

// TestProcessManager_SyncKnownProcesses tests orphan detection and cleanup
func TestProcessManager_SyncKnownProcesses(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create 3 processes
	for i := 1; i <= 3; i++ {
		pm.UpdateProcess(ProcessUpdate{
			ID:         ProcessID(fmt.Sprintf("test-%d", i)),
			UpdateType: UpdateTypeCreate,
			Config:     &struct{}{},
		})
	}

	// Wait for all to sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 3
	}, 2*time.Second, 50*time.Millisecond)

	// Sync with only 2 known processes (test-3 is orphan)
	knownIDs := []ProcessID{"test-1", "test-2"}
	synced := pm.SyncKnownProcesses(knownIDs)

	assert.Len(t, synced, 3, "All 3 processes should be in result (orphan not yet terminated)")

	// Wait for orphan to terminate
	require.Eventually(t, func() bool {
		return pm.IsProcessFinished("test-3")
	}, 5*time.Second, 50*time.Millisecond, "Orphan should be terminated and finished")

	// Sync again - orphan should be removed
	synced = pm.SyncKnownProcesses(knownIDs)
	assert.Len(t, synced, 2, "Only 2 known processes should remain")

	_, exists := pm.GetProcessStatus("test-3")
	assert.False(t, exists, "Orphan process should be removed")
}

// TestProcessManager_Shutdown tests graceful shutdown
func TestProcessManager_Shutdown(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))

	// Create 5 processes
	for i := 1; i <= 5; i++ {
		pm.UpdateProcess(ProcessUpdate{
			ID:         ProcessID(fmt.Sprintf("test-%d", i)),
			UpdateType: UpdateTypeCreate,
			Config:     &struct{}{},
		})
	}

	// Wait for all to sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 5
	}, 2*time.Second, 50*time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := pm.Shutdown(ctx)
	require.NoError(t, err, "Shutdown should complete without error")

	// All processes should be terminated
	for i := 1; i <= 5; i++ {
		assert.True(t, pm.IsProcessTerminated(ProcessID(fmt.Sprintf("test-%d", i))),
			"Process test-%d should be terminated", i)
	}
}

// TestProcessManager_ShutdownTimeout tests shutdown timeout
func TestProcessManager_ShutdownTimeout(t *testing.T) {
	syncer := &mockSyncer{
		syncTerminatingDuration: 10 * time.Second, // Very slow termination
	}
	pm := NewProcessManager(WithSyncer(syncer))

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := pm.Shutdown(ctx)
	assert.Error(t, err, "Shutdown should timeout")
	assert.Equal(t, context.DeadlineExceeded, err, "Error should be DeadlineExceeded")
}

// TestProcessManager_UpdateFinishedProcess tests updating a finished process
func TestProcessManager_UpdateFinishedProcess(t *testing.T) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	// Create and terminate process
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	gracePeriod := int64(1)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod,
		},
	})

	// Wait for finished state
	require.Eventually(t, func() bool {
		return pm.IsProcessFinished("test-1")
	}, 5*time.Second, 50*time.Millisecond)

	syncCallsBefore := syncer.getSyncCalled()

	// Try to update finished process (should be ignored)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeUpdate,
		Config:     &struct{}{},
	})

	time.Sleep(500 * time.Millisecond)

	syncCallsAfter := syncer.getSyncCalled()
	assert.Equal(t, syncCallsBefore, syncCallsAfter, "Finished process should not be updated")
}

// TestProcessManager_HighChurn tests high process churn
func TestProcessManager_HighChurn(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high churn test in short mode")
	}

	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	numProcesses := 50
	var counter atomic.Int32

	// Create and terminate processes rapidly
	for i := 0; i < numProcesses; i++ {
		id := ProcessID(fmt.Sprintf("test-%d", i))

		// Create
		pm.UpdateProcess(ProcessUpdate{
			ID:         id,
			UpdateType: UpdateTypeCreate,
			Config:     &struct{}{},
		})

		// Terminate after short delay
		go func(processID ProcessID) {
			time.Sleep(50 * time.Millisecond)
			gracePeriod := int64(1)
			pm.UpdateProcess(ProcessUpdate{
				ID:         processID,
				UpdateType: UpdateTypeTerminate,
				TerminateOptions: &TerminateOptions{
					GracePeriodSecs: &gracePeriod,
				},
			})
			counter.Add(1)
		}(id)
	}

	// Wait for all terminates to be sent
	require.Eventually(t, func() bool {
		return counter.Load() == int32(numProcesses)
	}, 10*time.Second, 50*time.Millisecond, "All termination requests should be sent")

	// Wait for all to finish
	require.Eventually(t, func() bool {
		synced := pm.SyncKnownProcesses([]ProcessID{})
		return len(synced) == 0
	}, 30*time.Second, 100*time.Millisecond, "All processes should finish")
}

// BenchmarkProcessManager_CreateTerminate benchmarks create/terminate cycle
func BenchmarkProcessManager_CreateTerminate(b *testing.B) {
	syncer := &mockSyncer{}
	pm := NewProcessManager(WithSyncer(syncer))
	defer pm.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ProcessID(fmt.Sprintf("test-%d", i))

		// Create
		pm.UpdateProcess(ProcessUpdate{
			ID:         id,
			UpdateType: UpdateTypeCreate,
			Config:     &struct{}{},
		})

		// Terminate
		gracePeriod := int64(1)
		pm.UpdateProcess(ProcessUpdate{
			ID:         id,
			UpdateType: UpdateTypeTerminate,
			TerminateOptions: &TerminateOptions{
				GracePeriodSecs: &gracePeriod,
			},
		})
	}
}
