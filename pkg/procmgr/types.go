package procmgr

import (
	"context"
	"sync"
	"time"
)

// ProcessState represents the lifecycle state of a managed process
type ProcessState int

const (
	// ProcessStateStarting - process is initializing
	ProcessStateStarting ProcessState = iota
	// ProcessStateSyncing - process is running and healthy
	ProcessStateSyncing
	// ProcessStateTerminating - process is shutting down
	ProcessStateTerminating
	// ProcessStateTerminated - process has stopped, awaiting cleanup
	ProcessStateTerminated
	// ProcessStateFinished - process fully cleaned up
	ProcessStateFinished
)

// String returns the string representation of a ProcessState
func (ps ProcessState) String() string {
	switch ps {
	case ProcessStateStarting:
		return "Starting"
	case ProcessStateSyncing:
		return "Syncing"
	case ProcessStateTerminating:
		return "Terminating"
	case ProcessStateTerminated:
		return "Terminated"
	case ProcessStateFinished:
		return "Finished"
	default:
		return "Unknown"
	}
}

// ProcessID uniquely identifies a managed process
type ProcessID string

// ProcessUpdate contains state changes for a process
type ProcessUpdate struct {
	ID               ProcessID
	UpdateType       UpdateType
	StartTime        time.Time
	Config           interface{} // Process-specific config
	TerminateOptions *TerminateOptions
}

// UpdateType specifies the kind of update
type UpdateType int

const (
	// UpdateTypeCreate - create a new process
	UpdateTypeCreate UpdateType = iota
	// UpdateTypeUpdate - update existing process
	UpdateTypeUpdate
	// UpdateTypeSync - periodic sync/health check
	UpdateTypeSync
	// UpdateTypeTerminate - terminate process
	UpdateTypeTerminate
)

// String returns the string representation of an UpdateType
func (ut UpdateType) String() string {
	switch ut {
	case UpdateTypeCreate:
		return "Create"
	case UpdateTypeUpdate:
		return "Update"
	case UpdateTypeSync:
		return "Sync"
	case UpdateTypeTerminate:
		return "Terminate"
	default:
		return "Unknown"
	}
}

// TerminateOptions control process termination
type TerminateOptions struct {
	CompletedCh     chan<- struct{}
	Evict           bool
	GracePeriodSecs *int64
	StatusFunc      ProcessStatusFunc
}

// ProcessStatusFunc is called to update process status on termination
type ProcessStatusFunc func(status *ProcessStatus)

// ProcessStatus tracks runtime state of a process
type ProcessStatus struct {
	State        ProcessState
	Healthy      bool
	LastSync     time.Time
	ErrorCount   int
	LastError    error
	RestartCount int
}

// ProcessSyncer defines the interface for process lifecycle hooks
type ProcessSyncer interface {
	// SyncProcess starts/updates the process
	// Returns (terminal, error) where terminal=true means process reached terminal state
	SyncProcess(ctx context.Context, updateType UpdateType, config interface{}) (terminal bool, err error)

	// SyncTerminatingProcess stops the process
	SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn ProcessStatusFunc) error

	// SyncTerminatedProcess cleans up resources
	SyncTerminatedProcess(ctx context.Context, config interface{}) error
}

// ProcessManager manages 0 or more concurrent processes
type ProcessManager struct {
	mu sync.Mutex

	// Process tracking
	processUpdates  map[ProcessID]chan struct{}
	processStatuses map[ProcessID]*processStatus

	// Configuration
	syncer         ProcessSyncer
	resyncInterval time.Duration
	backOffPeriod  time.Duration
	workQueue      WorkQueue

	// Lifecycle
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	wg             sync.WaitGroup
}

// Internal state tracking per process
type processStatus struct {
	ctx      context.Context
	cancelFn context.CancelFunc

	working bool
	pending *ProcessUpdate
	active  *ProcessUpdate

	// Lifecycle timestamps
	syncedAt      time.Time
	startedAt     time.Time
	terminatingAt time.Time
	terminatedAt  time.Time
	finishedAt    time.Time

	// Termination metadata
	gracePeriod int64
	evicted     bool
	finished    bool

	// Health tracking
	errorCount       int
	lastError        error
	restartCount     int
	consecutiveFails int // Track consecutive failures for backoff
}

// State returns the current state of the process
func (ps *processStatus) State() ProcessState {
	if !ps.finishedAt.IsZero() {
		return ProcessStateFinished
	}
	if !ps.terminatedAt.IsZero() {
		return ProcessStateTerminated
	}
	if !ps.terminatingAt.IsZero() {
		return ProcessStateTerminating
	}
	if !ps.syncedAt.IsZero() {
		return ProcessStateSyncing
	}
	return ProcessStateStarting
}

// IsTerminating returns true if process is terminating or beyond
func (ps *processStatus) IsTerminating() bool {
	return !ps.terminatingAt.IsZero()
}

// IsTerminated returns true if process is terminated or beyond
func (ps *processStatus) IsTerminated() bool {
	return !ps.terminatedAt.IsZero()
}

// IsFinished returns true if process is finished
func (ps *processStatus) IsFinished() bool {
	return !ps.finishedAt.IsZero()
}

// Healthy returns true if process is healthy
func (ps *processStatus) Healthy() bool {
	return ps.errorCount < 5 && ps.State() == ProcessStateSyncing
}
