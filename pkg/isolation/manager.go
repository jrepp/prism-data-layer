package isolation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/procmgr"
)

// IsolationManager manages process isolation using procmgr
// Implements the bulkhead pattern to isolate requests by namespace or session
type IsolationManager struct {
	level  IsolationLevel
	pm     *procmgr.ProcessManager
	syncer ProcessSyncer

	mu        sync.RWMutex
	processes map[procmgr.ProcessID]*ProcessConfig
}

// NewIsolationManager creates a new isolation manager
func NewIsolationManager(level IsolationLevel, syncer ProcessSyncer, opts ...procmgr.Option) *IsolationManager {
	// Create process manager with syncer adapter
	adapter := &syncerAdapter{syncer: syncer}
	pmOpts := append([]procmgr.Option{procmgr.WithSyncer(adapter)}, opts...)
	pm := procmgr.NewProcessManager(pmOpts...)

	return &IsolationManager{
		level:     level,
		pm:        pm,
		syncer:    syncer,
		processes: make(map[procmgr.ProcessID]*ProcessConfig),
	}
}

// GetOrCreateProcess gets an existing process or creates a new one for the given key
func (im *IsolationManager) GetOrCreateProcess(ctx context.Context, key IsolationKey, config *ProcessConfig) (*ProcessHandle, error) {
	processID := key.ProcessID(im.level)

	im.mu.Lock()
	existing, exists := im.processes[processID]
	if !exists {
		// Store config for this process
		im.processes[processID] = config
	}
	im.mu.Unlock()

	// If process exists, check health
	if exists {
		status, ok := im.pm.GetProcessStatus(processID)
		if ok && status.State == procmgr.ProcessStateSyncing {
			return &ProcessHandle{
				ID:     processID,
				Key:    key,
				Config: existing,
				Health: status.Healthy,
			}, nil
		}
	}

	// Create or restart process
	im.pm.UpdateProcess(procmgr.ProcessUpdate{
		ID:         processID,
		UpdateType: procmgr.UpdateTypeCreate,
		StartTime:  time.Now(),
		Config:     config,
	})

	// Wait for process to start (with timeout)
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("timeout waiting for process %s to start", processID)
		case <-ticker.C:
			status, ok := im.pm.GetProcessStatus(processID)
			if ok && status.State == procmgr.ProcessStateSyncing {
				return &ProcessHandle{
					ID:     processID,
					Key:    key,
					Config: config,
					Health: status.Healthy,
				}, nil
			}
		}
	}
}

// GetProcess returns a handle to an existing process, or nil if not found
func (im *IsolationManager) GetProcess(key IsolationKey) *ProcessHandle {
	processID := key.ProcessID(im.level)

	im.mu.RLock()
	config, exists := im.processes[processID]
	im.mu.RUnlock()

	if !exists {
		return nil
	}

	status, ok := im.pm.GetProcessStatus(processID)
	if !ok {
		return nil
	}

	return &ProcessHandle{
		ID:     processID,
		Key:    key,
		Config: config,
		Health: status.Healthy,
	}
}

// TerminateProcess gracefully terminates a process
func (im *IsolationManager) TerminateProcess(ctx context.Context, key IsolationKey, gracePeriodSec int64) error {
	processID := key.ProcessID(im.level)

	im.mu.Lock()
	_, exists := im.processes[processID]
	if !exists {
		im.mu.Unlock()
		return fmt.Errorf("process %s not found", processID)
	}
	im.mu.Unlock()

	completedCh := make(chan struct{})
	im.pm.UpdateProcess(procmgr.ProcessUpdate{
		ID:         processID,
		UpdateType: procmgr.UpdateTypeTerminate,
		TerminateOptions: &procmgr.TerminateOptions{
			CompletedCh:     completedCh,
			GracePeriodSecs: &gracePeriodSec,
		},
	})

	// Wait for termination (with timeout)
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(gracePeriodSec+5)*time.Second)
	defer cancel()

	select {
	case <-waitCtx.Done():
		return fmt.Errorf("timeout waiting for process %s to terminate", processID)
	case <-completedCh:
		// Remove from tracking
		im.mu.Lock()
		delete(im.processes, processID)
		im.mu.Unlock()
		return nil
	}

	return nil
}

// ListProcesses returns handles to all managed processes
func (im *IsolationManager) ListProcesses() []*ProcessHandle {
	im.mu.RLock()
	defer im.mu.RUnlock()

	handles := make([]*ProcessHandle, 0, len(im.processes))
	for processID, config := range im.processes {
		status, ok := im.pm.GetProcessStatus(processID)
		if !ok {
			continue
		}

		handles = append(handles, &ProcessHandle{
			ID:     processID,
			Key:    config.Key,
			Config: config,
			Health: status.Healthy,
		})
	}

	return handles
}

// Health returns health information for all processes
func (im *IsolationManager) Health() procmgr.HealthCheck {
	return im.pm.Health()
}

// Shutdown gracefully shuts down all processes
func (im *IsolationManager) Shutdown(ctx context.Context) error {
	return im.pm.Shutdown(ctx)
}

// Level returns the current isolation level
func (im *IsolationManager) Level() IsolationLevel {
	return im.level
}

// syncerAdapter adapts ProcessSyncer to procmgr.ProcessSyncer
type syncerAdapter struct {
	syncer ProcessSyncer
}

func (sa *syncerAdapter) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (bool, error) {
	return sa.syncer.SyncProcess(ctx, updateType, config)
}

func (sa *syncerAdapter) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error {
	return sa.syncer.SyncTerminatingProcess(ctx, config, gracePeriodSecs, statusFn)
}

func (sa *syncerAdapter) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
	return sa.syncer.SyncTerminatedProcess(ctx, config)
}
