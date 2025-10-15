package procmgr

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMetricsCollector records all metrics calls for testing
type mockMetricsCollector struct {
	mu sync.Mutex

	stateTransitions   []stateTransition
	syncDurations      []syncDuration
	terminationDurs    []terminationDur
	errors             []processError
	restarts           []ProcessID
	queueDepths        []int
	queueAdds          []queueAdd
	queueRetries       []ProcessID
	backoffDurations   []backoffDuration
}

type stateTransition struct {
	id        ProcessID
	fromState ProcessState
	toState   ProcessState
}

type syncDuration struct {
	id         ProcessID
	updateType UpdateType
	duration   time.Duration
	err        error
}

type terminationDur struct {
	id       ProcessID
	duration time.Duration
}

type processError struct {
	id        ProcessID
	errorType string
}

type queueAdd struct {
	id    ProcessID
	delay time.Duration
}

type backoffDuration struct {
	id       ProcessID
	duration time.Duration
}

func (m *mockMetricsCollector) ProcessStateTransition(id ProcessID, fromState, toState ProcessState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateTransitions = append(m.stateTransitions, stateTransition{id, fromState, toState})
}

func (m *mockMetricsCollector) ProcessSyncDuration(id ProcessID, updateType UpdateType, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncDurations = append(m.syncDurations, syncDuration{id, updateType, duration, err})
}

func (m *mockMetricsCollector) ProcessTerminationDuration(id ProcessID, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terminationDurs = append(m.terminationDurs, terminationDur{id, duration})
}

func (m *mockMetricsCollector) ProcessError(id ProcessID, errorType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, processError{id, errorType})
}

func (m *mockMetricsCollector) ProcessRestart(id ProcessID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restarts = append(m.restarts, id)
}

func (m *mockMetricsCollector) WorkQueueDepth(depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDepths = append(m.queueDepths, depth)
}

func (m *mockMetricsCollector) WorkQueueAdd(id ProcessID, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueAdds = append(m.queueAdds, queueAdd{id, delay})
}

func (m *mockMetricsCollector) WorkQueueRetry(id ProcessID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueRetries = append(m.queueRetries, id)
}

func (m *mockMetricsCollector) WorkQueueBackoffDuration(id ProcessID, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backoffDurations = append(m.backoffDurations, backoffDuration{id, duration})
}

func (m *mockMetricsCollector) getStateTransitionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.stateTransitions)
}

func (m *mockMetricsCollector) getSyncDurationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.syncDurations)
}

func (m *mockMetricsCollector) getErrorCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.errors)
}

func (m *mockMetricsCollector) getQueueRetryCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queueRetries)
}

// TestMetricsCollector_StateTransitions tests state transition metrics
func TestMetricsCollector_StateTransitions(t *testing.T) {
	metrics := &mockMetricsCollector{}
	syncer := &mockSyncer{}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithMetricsCollector(metrics),
	)
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Should record transition to Syncing
	require.Eventually(t, func() bool {
		return metrics.getStateTransitionCount() >= 1
	}, 2*time.Second, 50*time.Millisecond)

	metrics.mu.Lock()
	transitions := metrics.stateTransitions
	metrics.mu.Unlock()

	// Should have at least one transition to Syncing
	found := false
	for _, t := range transitions {
		if t.id == "test-1" && t.toState == ProcessStateSyncing {
			found = true
			break
		}
	}
	assert.True(t, found, "Should record transition to Syncing state")
}

// TestMetricsCollector_SyncDuration tests sync duration metrics
func TestMetricsCollector_SyncDuration(t *testing.T) {
	metrics := &mockMetricsCollector{}
	syncer := &mockSyncer{}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithMetricsCollector(metrics),
	)
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for sync
	require.Eventually(t, func() bool {
		return metrics.getSyncDurationCount() >= 1
	}, 2*time.Second, 50*time.Millisecond)

	metrics.mu.Lock()
	durations := metrics.syncDurations
	metrics.mu.Unlock()

	assert.GreaterOrEqual(t, len(durations), 1)
	assert.Equal(t, ProcessID("test-1"), durations[0].id)
	assert.Equal(t, UpdateTypeCreate, durations[0].updateType)
	assert.Greater(t, durations[0].duration, time.Duration(0))
}

// TestMetricsCollector_Errors tests error metrics
func TestMetricsCollector_Errors(t *testing.T) {
	metrics := &mockMetricsCollector{}
	syncer := &mockSyncer{
		syncErr: errors.New("sync failed"),
	}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithMetricsCollector(metrics),
		WithBackOffPeriod(2*time.Second),
	)
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for error
	require.Eventually(t, func() bool {
		return metrics.getErrorCount() >= 1
	}, 2*time.Second, 50*time.Millisecond)

	metrics.mu.Lock()
	errors := metrics.errors
	metrics.mu.Unlock()

	assert.GreaterOrEqual(t, len(errors), 1)
	assert.Equal(t, ProcessID("test-1"), errors[0].id)
	assert.Equal(t, "sync_error", errors[0].errorType)
}

// TestMetricsCollector_WorkQueueRetries tests work queue retry metrics
func TestMetricsCollector_WorkQueueRetries(t *testing.T) {
	metrics := &mockMetricsCollector{}
	syncer := &mockSyncer{
		syncErr: errors.New("sync failed"),
	}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithMetricsCollector(metrics),
		WithBackOffPeriod(2*time.Second),
	)
	defer pm.Shutdown(context.Background())

	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeCreate,
		Config:     &struct{}{},
	})

	// Wait for initial sync
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 1
	}, 2*time.Second, 50*time.Millisecond)

	// Clear error to allow retry to succeed
	syncer.mu.Lock()
	syncer.syncErr = nil
	syncer.mu.Unlock()

	// Wait for retry
	require.Eventually(t, func() bool {
		return metrics.getQueueRetryCount() >= 1
	}, 5*time.Second, 100*time.Millisecond)

	metrics.mu.Lock()
	retries := metrics.queueRetries
	metrics.mu.Unlock()

	assert.GreaterOrEqual(t, len(retries), 1)
	assert.Equal(t, ProcessID("test-1"), retries[0])
}

// TestMetricsCollector_TerminationDuration tests termination duration metrics
func TestMetricsCollector_TerminationDuration(t *testing.T) {
	metrics := &mockMetricsCollector{}
	syncer := &mockSyncer{}
	pm := NewProcessManager(
		WithSyncer(syncer),
		WithMetricsCollector(metrics),
	)
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
	gracePeriod := int64(1)
	pm.UpdateProcess(ProcessUpdate{
		ID:         "test-1",
		UpdateType: UpdateTypeTerminate,
		TerminateOptions: &TerminateOptions{
			GracePeriodSecs: &gracePeriod,
		},
	})

	// Wait for termination metrics
	require.Eventually(t, func() bool {
		metrics.mu.Lock()
		count := len(metrics.terminationDurs)
		metrics.mu.Unlock()
		return count >= 1
	}, 3*time.Second, 50*time.Millisecond)

	metrics.mu.Lock()
	termDurs := metrics.terminationDurs
	metrics.mu.Unlock()

	assert.GreaterOrEqual(t, len(termDurs), 1)
	assert.Equal(t, ProcessID("test-1"), termDurs[0].id)
	assert.Greater(t, termDurs[0].duration, time.Duration(0))
}
