package isolation

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jrepp/prism/pkg/procmgr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSyncer implements ProcessSyncer for testing
type mockSyncer struct {
	mu          sync.Mutex
	syncCalled  int
	termCalled  int
	cleanCalled int
	syncErr     error
	termErr     error
	cleanErr    error
}

func (m *mockSyncer) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCalled++
	return false, m.syncErr
}

func (m *mockSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.termCalled++
	return m.termErr
}

func (m *mockSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanCalled++
	return m.cleanErr
}

func (m *mockSyncer) getSyncCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.syncCalled
}

func (m *mockSyncer) getTermCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.termCalled
}

func (m *mockSyncer) getCleanCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cleanCalled
}

// TestIsolationLevel_String tests string representation of isolation levels
func TestIsolationLevel_String(t *testing.T) {
	tests := []struct {
		level    IsolationLevel
		expected string
	}{
		{IsolationNone, "None"},
		{IsolationNamespace, "Namespace"},
		{IsolationSession, "Session"},
		{IsolationLevel(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.String())
		})
	}
}

// TestIsolationKey_ProcessID tests process ID generation
func TestIsolationKey_ProcessID(t *testing.T) {
	key := IsolationKey{
		Namespace: "test-ns",
		Session:   "session-123",
	}

	tests := []struct {
		level    IsolationLevel
		expected procmgr.ProcessID
	}{
		{IsolationNone, "shared"},
		{IsolationNamespace, "ns:test-ns"},
		{IsolationSession, "session:session-123"},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			assert.Equal(t, tt.expected, key.ProcessID(tt.level))
		})
	}
}

// TestIsolationManager_None tests isolation level None (shared process)
func TestIsolationManager_None(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationNone, syncer,
		procmgr.WithResyncInterval(10*time.Second),
		procmgr.WithBackOffPeriod(2*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()

	// Create two processes with different keys
	config1 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns1", Session: "s1"},
		BackendConfig:  "config1",
		GracePeriodSec: 5,
	}
	config2 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns2", Session: "s2"},
		BackendConfig:  "config2",
		GracePeriodSec: 5,
	}

	handle1, err := im.GetOrCreateProcess(ctx, config1.Key, config1)
	require.NoError(t, err)
	require.NotNil(t, handle1)

	handle2, err := im.GetOrCreateProcess(ctx, config2.Key, config2)
	require.NoError(t, err)
	require.NotNil(t, handle2)

	// Both should have the same process ID ("shared")
	assert.Equal(t, procmgr.ProcessID("shared"), handle1.ID)
	assert.Equal(t, procmgr.ProcessID("shared"), handle2.ID)
	assert.Equal(t, handle1.ID, handle2.ID)

	// Should have called sync once (both requests use same process)
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 1
	}, 2*time.Second, 50*time.Millisecond)
}

// TestIsolationManager_Namespace tests namespace isolation
func TestIsolationManager_Namespace(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationNamespace, syncer,
		procmgr.WithResyncInterval(10*time.Second),
		procmgr.WithBackOffPeriod(2*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()

	// Create processes for different namespaces
	config1 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns1", Session: "s1"},
		BackendConfig:  "config1",
		GracePeriodSec: 5,
	}
	config2 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns2", Session: "s2"},
		BackendConfig:  "config2",
		GracePeriodSec: 5,
	}
	config3 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns1", Session: "s3"}, // Same namespace as config1
		BackendConfig:  "config3",
		GracePeriodSec: 5,
	}

	handle1, err := im.GetOrCreateProcess(ctx, config1.Key, config1)
	require.NoError(t, err)
	require.NotNil(t, handle1)

	handle2, err := im.GetOrCreateProcess(ctx, config2.Key, config2)
	require.NoError(t, err)
	require.NotNil(t, handle2)

	handle3, err := im.GetOrCreateProcess(ctx, config3.Key, config3)
	require.NoError(t, err)
	require.NotNil(t, handle3)

	// Different namespaces should have different process IDs
	assert.Equal(t, procmgr.ProcessID("ns:ns1"), handle1.ID)
	assert.Equal(t, procmgr.ProcessID("ns:ns2"), handle2.ID)
	assert.Equal(t, procmgr.ProcessID("ns:ns1"), handle3.ID) // Same as handle1

	assert.NotEqual(t, handle1.ID, handle2.ID)
	assert.Equal(t, handle1.ID, handle3.ID) // Same namespace

	// Should have called sync twice (two distinct processes)
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 2
	}, 2*time.Second, 50*time.Millisecond)
}

// TestIsolationManager_Session tests session isolation
func TestIsolationManager_Session(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationSession, syncer,
		procmgr.WithResyncInterval(10*time.Second),
		procmgr.WithBackOffPeriod(2*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()

	// Create processes for different sessions
	config1 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns1", Session: "s1"},
		BackendConfig:  "config1",
		GracePeriodSec: 5,
	}
	config2 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns1", Session: "s2"}, // Same namespace, different session
		BackendConfig:  "config2",
		GracePeriodSec: 5,
	}
	config3 := &ProcessConfig{
		Key:            IsolationKey{Namespace: "ns2", Session: "s1"}, // Same session, different namespace
		BackendConfig:  "config3",
		GracePeriodSec: 5,
	}

	handle1, err := im.GetOrCreateProcess(ctx, config1.Key, config1)
	require.NoError(t, err)
	require.NotNil(t, handle1)

	handle2, err := im.GetOrCreateProcess(ctx, config2.Key, config2)
	require.NoError(t, err)
	require.NotNil(t, handle2)

	handle3, err := im.GetOrCreateProcess(ctx, config3.Key, config3)
	require.NoError(t, err)
	require.NotNil(t, handle3)

	// Different sessions should have different process IDs
	assert.Equal(t, procmgr.ProcessID("session:s1"), handle1.ID)
	assert.Equal(t, procmgr.ProcessID("session:s2"), handle2.ID)
	assert.Equal(t, procmgr.ProcessID("session:s1"), handle3.ID) // Same as handle1

	assert.NotEqual(t, handle1.ID, handle2.ID)
	assert.Equal(t, handle1.ID, handle3.ID) // Same session

	// Should have called sync twice (two distinct processes)
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 2
	}, 2*time.Second, 50*time.Millisecond)
}

// TestIsolationManager_GetProcess tests retrieving existing processes
func TestIsolationManager_GetProcess(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationNamespace, syncer,
		procmgr.WithResyncInterval(10*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()
	key := IsolationKey{Namespace: "test-ns", Session: "s1"}
	config := &ProcessConfig{
		Key:            key,
		BackendConfig:  "config",
		GracePeriodSec: 5,
	}

	// Get non-existent process
	handle := im.GetProcess(key)
	assert.Nil(t, handle)

	// Create process
	handle, err := im.GetOrCreateProcess(ctx, key, config)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// Get existing process
	handle2 := im.GetProcess(key)
	require.NotNil(t, handle2)
	assert.Equal(t, handle.ID, handle2.ID)
}

// TestIsolationManager_TerminateProcess tests process termination
func TestIsolationManager_TerminateProcess(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationNamespace, syncer,
		procmgr.WithResyncInterval(10*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()
	key := IsolationKey{Namespace: "test-ns", Session: "s1"}
	config := &ProcessConfig{
		Key:            key,
		BackendConfig:  "config",
		GracePeriodSec: 5,
	}

	// Create process
	handle, err := im.GetOrCreateProcess(ctx, key, config)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// Wait for process to start
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Terminate process
	err = im.TerminateProcess(ctx, key, 5)
	require.NoError(t, err)

	// Process should be removed from tracking
	handle = im.GetProcess(key)
	assert.Nil(t, handle)

	// Syncer should have been called for termination
	require.Eventually(t, func() bool {
		return syncer.getTermCalled() > 0 && syncer.getCleanCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)
}

// TestIsolationManager_ListProcesses tests listing all processes
func TestIsolationManager_ListProcesses(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationNamespace, syncer,
		procmgr.WithResyncInterval(10*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()

	// Initially empty
	handles := im.ListProcesses()
	assert.Len(t, handles, 0)

	// Create multiple processes
	for i := 0; i < 3; i++ {
		key := IsolationKey{Namespace: fmt.Sprintf("ns%d", i), Session: "s1"}
		config := &ProcessConfig{
			Key:            key,
			BackendConfig:  fmt.Sprintf("config%d", i),
			GracePeriodSec: 5,
		}
		_, err := im.GetOrCreateProcess(ctx, key, config)
		require.NoError(t, err)
	}

	// Wait for processes to start
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() >= 3
	}, 2*time.Second, 50*time.Millisecond)

	// List should show all processes
	handles = im.ListProcesses()
	assert.Len(t, handles, 3)
}

// TestIsolationManager_Health tests health reporting
func TestIsolationManager_Health(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationNamespace, syncer,
		procmgr.WithResyncInterval(10*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()

	// Create process
	key := IsolationKey{Namespace: "test-ns", Session: "s1"}
	config := &ProcessConfig{
		Key:            key,
		BackendConfig:  "config",
		GracePeriodSec: 5,
	}
	_, err := im.GetOrCreateProcess(ctx, key, config)
	require.NoError(t, err)

	// Wait for process to start
	require.Eventually(t, func() bool {
		return syncer.getSyncCalled() > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Check health
	health := im.Health()
	require.NotNil(t, health)
	assert.Greater(t, health.TotalProcesses, 0)
}

// TestIsolationManager_ConcurrentAccess tests concurrent process creation
func TestIsolationManager_ConcurrentAccess(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationSession, syncer,
		procmgr.WithResyncInterval(10*time.Second),
	)
	defer im.Shutdown(context.Background())

	ctx := context.Background()
	numGoroutines := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			key := IsolationKey{
				Namespace: fmt.Sprintf("ns%d", id),
				Session:   fmt.Sprintf("session%d", id),
			}
			config := &ProcessConfig{
				Key:            key,
				BackendConfig:  fmt.Sprintf("config%d", id),
				GracePeriodSec: 5,
			}

			handle, err := im.GetOrCreateProcess(ctx, key, config)
			assert.NoError(t, err)
			assert.NotNil(t, handle)
		}(i)
	}

	wg.Wait()

	// Should have created all processes
	require.Eventually(t, func() bool {
		handles := im.ListProcesses()
		return len(handles) == numGoroutines
	}, 3*time.Second, 100*time.Millisecond)
}

// TestIsolationManager_Level tests Level() accessor
func TestIsolationManager_Level(t *testing.T) {
	syncer := &mockSyncer{}
	im := NewIsolationManager(IsolationSession, syncer)
	defer im.Shutdown(context.Background())

	assert.Equal(t, IsolationSession, im.Level())
}
