package procmgr

import (
	"time"
)

// MetricsCollector defines the interface for collecting process manager metrics
type MetricsCollector interface {
	// ProcessStateTransition records a state transition for a process
	ProcessStateTransition(id ProcessID, fromState, toState ProcessState)

	// ProcessSyncDuration records the duration of a sync operation
	ProcessSyncDuration(id ProcessID, updateType UpdateType, duration time.Duration, err error)

	// ProcessTerminationDuration records the duration of termination
	ProcessTerminationDuration(id ProcessID, duration time.Duration)

	// ProcessError records an error for a process
	ProcessError(id ProcessID, errorType string)

	// ProcessRestart records a process restart
	ProcessRestart(id ProcessID)

	// WorkQueueDepth records the current work queue depth
	WorkQueueDepth(depth int)

	// WorkQueueAdd records an item added to the work queue
	WorkQueueAdd(id ProcessID, delay time.Duration)

	// WorkQueueRetry records a retry from the work queue
	WorkQueueRetry(id ProcessID)

	// WorkQueueBackoffDuration records backoff duration
	WorkQueueBackoffDuration(id ProcessID, duration time.Duration)
}

// noopMetricsCollector is a no-op implementation of MetricsCollector
type noopMetricsCollector struct{}

func (n *noopMetricsCollector) ProcessStateTransition(id ProcessID, fromState, toState ProcessState) {}
func (n *noopMetricsCollector) ProcessSyncDuration(id ProcessID, updateType UpdateType, duration time.Duration, err error) {
}
func (n *noopMetricsCollector) ProcessTerminationDuration(id ProcessID, duration time.Duration) {}
func (n *noopMetricsCollector) ProcessError(id ProcessID, errorType string)                     {}
func (n *noopMetricsCollector) ProcessRestart(id ProcessID)                                     {}
func (n *noopMetricsCollector) WorkQueueDepth(depth int)                                        {}
func (n *noopMetricsCollector) WorkQueueAdd(id ProcessID, delay time.Duration)                  {}
func (n *noopMetricsCollector) WorkQueueRetry(id ProcessID)                                     {}
func (n *noopMetricsCollector) WorkQueueBackoffDuration(id ProcessID, duration time.Duration)   {}

// NewNoopMetricsCollector creates a no-op metrics collector
func NewNoopMetricsCollector() MetricsCollector {
	return &noopMetricsCollector{}
}
