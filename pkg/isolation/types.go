package isolation

import (
	"context"

	"github.com/jrepp/prism/pkg/procmgr"
)

// IsolationLevel defines how requests are isolated into processes
type IsolationLevel int

const (
	// IsolationNone means all requests share the same process pool
	IsolationNone IsolationLevel = iota
	// IsolationNamespace means each namespace gets its own process
	IsolationNamespace
	// IsolationSession means each session gets its own process
	IsolationSession
)

// String returns the string representation of the isolation level
func (level IsolationLevel) String() string {
	switch level {
	case IsolationNone:
		return "None"
	case IsolationNamespace:
		return "Namespace"
	case IsolationSession:
		return "Session"
	default:
		return "Unknown"
	}
}

// IsolationKey identifies which process to route a request to
type IsolationKey struct {
	Namespace string
	Session   string
}

// ProcessID returns the process ID for this isolation key based on the level
func (key IsolationKey) ProcessID(level IsolationLevel) procmgr.ProcessID {
	switch level {
	case IsolationNone:
		return "shared"
	case IsolationNamespace:
		return procmgr.ProcessID("ns:" + key.Namespace)
	case IsolationSession:
		return procmgr.ProcessID("session:" + key.Session)
	default:
		return "shared"
	}
}

// ProcessConfig holds configuration for a managed process
type ProcessConfig struct {
	Key            IsolationKey
	BackendConfig  interface{}
	GracePeriodSec int64
}

// ProcessSyncer defines the interface for process lifecycle management
// This abstracts the actual backend driver implementation
type ProcessSyncer interface {
	// SyncProcess starts or updates a process
	SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (terminal bool, err error)

	// SyncTerminatingProcess stops a process gracefully
	SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error

	// SyncTerminatedProcess cleans up resources for a stopped process
	SyncTerminatedProcess(ctx context.Context, config interface{}) error
}

// ProcessHandle represents a handle to a managed process
type ProcessHandle struct {
	ID     procmgr.ProcessID
	Key    IsolationKey
	Config *ProcessConfig
	Health bool
}
