package procmgr

import (
	"context"
	"fmt"
	"log"
	"time"
)

// NewProcessManager creates a new process manager
func NewProcessManager(opts ...Option) *ProcessManager {
	ctx, cancel := context.WithCancel(context.Background())

	pm := &ProcessManager{
		processUpdates:  make(map[ProcessID]chan struct{}),
		processStatuses: make(map[ProcessID]*processStatus),
		resyncInterval:  30 * time.Second,
		backOffPeriod:   5 * time.Second,
		shutdownCtx:     ctx,
		shutdownCancel:  cancel,
	}

	for _, opt := range opts {
		opt(pm)
	}

	return pm
}

// UpdateProcess submits a process update
func (pm *ProcessManager) UpdateProcess(update ProcessUpdate) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if update.StartTime.IsZero() {
		update.StartTime = time.Now()
	}

	// Get or create process status
	status, exists := pm.processStatuses[update.ID]
	if !exists {
		status = &processStatus{}
		pm.processStatuses[update.ID] = status
	}

	// Check if process is finished - cannot update finished processes
	if status.IsFinished() {
		log.Printf("Process %s is finished, ignoring update", update.ID)
		return
	}

	// Handle termination request
	if update.UpdateType == UpdateTypeTerminate {
		pm.handleTerminationRequest(update.ID, status, update.TerminateOptions)
	}

	// Store pending update
	status.pending = &update

	// Get or create update channel
	updateCh, exists := pm.processUpdates[update.ID]
	if !exists {
		// Create buffered channel (size 1) to prevent blocking
		updateCh = make(chan struct{}, 1)
		pm.processUpdates[update.ID] = updateCh

		// Start worker goroutine
		pm.wg.Add(1)
		go pm.processWorkerLoop(update.ID, updateCh)
	}

	// Signal worker (non-blocking due to buffer)
	select {
	case updateCh <- struct{}{}:
	default:
		// Channel already has pending signal, skip
	}
}

// handleTerminationRequest handles termination request
func (pm *ProcessManager) handleTerminationRequest(id ProcessID, status *processStatus, opts *TerminateOptions) {
	// Check if already terminating
	alreadyTerminating := status.IsTerminating()

	// Set termination timestamp if not already set
	if status.terminatingAt.IsZero() {
		status.terminatingAt = time.Now()
	}

	// Handle grace period
	if opts != nil && opts.GracePeriodSecs != nil {
		gracePeriod := *opts.GracePeriodSecs

		// Grace period can only decrease, never increase
		if status.gracePeriod == 0 || gracePeriod < status.gracePeriod {
			status.gracePeriod = gracePeriod

			// Cancel context to interrupt long-running sync
			if !alreadyTerminating && status.cancelFn != nil {
				log.Printf("Process %s: cancelling context due to termination request", id)
				status.cancelFn()
			}
		}

		// Track eviction
		if opts.Evict {
			status.evicted = true
		}
	} else {
		// Default grace period: 10 seconds
		if status.gracePeriod == 0 {
			status.gracePeriod = 10
		}
	}

	// Cancel context if becoming terminating
	if !alreadyTerminating && status.cancelFn != nil {
		log.Printf("Process %s: cancelling context due to termination", id)
		status.cancelFn()
	}
}

// processWorkerLoop runs in a goroutine, processing updates for a single process
func (pm *ProcessManager) processWorkerLoop(id ProcessID, updateCh <-chan struct{}) {
	defer pm.wg.Done()
	defer log.Printf("Process worker stopped: %s", id)

	log.Printf("Process worker started: %s", id)

	for {
		select {
		case <-pm.shutdownCtx.Done():
			// Manager shutting down
			return
		case <-updateCh:
			// Process update available
			if !pm.processUpdate(id) {
				// Process finished, exit worker
				return
			}
		}
	}
}

// processUpdate processes a pending update for a process
// Returns false if worker should exit
func (pm *ProcessManager) processUpdate(id ProcessID) bool {
	pm.mu.Lock()

	status, exists := pm.processStatuses[id]
	if !exists {
		pm.mu.Unlock()
		return false
	}

	// Check if finished
	if status.IsFinished() {
		pm.mu.Unlock()
		return false
	}

	// Check if already working
	if status.working {
		pm.mu.Unlock()
		return true
	}

	// Move pending to active
	if status.pending == nil {
		pm.mu.Unlock()
		return true
	}

	status.active = status.pending
	status.pending = nil
	status.working = true

	// Initialize context if needed
	if status.ctx == nil || status.ctx.Err() == context.Canceled {
		status.ctx, status.cancelFn = context.WithCancel(pm.shutdownCtx)
	}

	// Get active update (need copy for use outside lock)
	update := *status.active
	state := status.State()

	pm.mu.Unlock()

	// Execute sync outside of lock
	err := pm.executeSync(id, status, update, state)

	// Complete work
	pm.completeWork(id, err)

	return true
}

// executeSync executes the appropriate sync method based on process state
func (pm *ProcessManager) executeSync(id ProcessID, status *processStatus, update ProcessUpdate, state ProcessState) error {
	if pm.syncer == nil {
		return fmt.Errorf("no syncer configured")
	}

	switch state {
	case ProcessStateStarting, ProcessStateSyncing:
		// Check if terminating (within lock earlier)
		pm.mu.Lock()
		isTerminating := status.IsTerminating()
		pm.mu.Unlock()

		if isTerminating {
			// Transition to terminating
			return pm.syncTerminating(id, status, update)
		}
		// Normal sync
		return pm.syncProcess(id, status, update)

	case ProcessStateTerminating:
		return pm.syncTerminating(id, status, update)

	case ProcessStateTerminated:
		return pm.syncTerminated(id, status, update)

	case ProcessStateFinished:
		// Already finished, nothing to do
		return nil

	default:
		return fmt.Errorf("unknown state: %v", state)
	}
}

// syncProcess executes SyncProcess
func (pm *ProcessManager) syncProcess(id ProcessID, status *processStatus, update ProcessUpdate) error {
	startTime := time.Now()

	terminal, err := pm.syncer.SyncProcess(status.ctx, update.UpdateType, update.Config)

	duration := time.Since(startTime)
	log.Printf("Process %s: sync completed in %v (terminal=%v, err=%v)", id, duration, terminal, err)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err != nil {
		status.errorCount++
		status.lastError = err
	} else {
		status.errorCount = 0
		status.lastError = nil
		status.syncedAt = time.Now()

		if status.startedAt.IsZero() {
			status.startedAt = time.Now()
		}
	}

	// Handle terminal state
	if terminal {
		log.Printf("Process %s: reached terminal state, initiating termination", id)
		status.terminatingAt = time.Now()
		if status.gracePeriod == 0 {
			status.gracePeriod = 10
		}
	}

	return err
}

// syncTerminating executes SyncTerminatingProcess
func (pm *ProcessManager) syncTerminating(id ProcessID, status *processStatus, update ProcessUpdate) error {
	startTime := time.Now()

	var statusFunc ProcessStatusFunc
	if update.TerminateOptions != nil {
		statusFunc = update.TerminateOptions.StatusFunc
	}

	gracePeriod := status.gracePeriod
	err := pm.syncer.SyncTerminatingProcess(status.ctx, update.Config, &gracePeriod, statusFunc)

	duration := time.Since(startTime)
	log.Printf("Process %s: terminating sync completed in %v (err=%v)", id, duration, err)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err != nil {
		status.errorCount++
		status.lastError = err
	} else {
		// Transition to terminated
		status.terminatedAt = time.Now()
		status.errorCount = 0
		status.lastError = nil

		// Notify completion channel if present
		if update.TerminateOptions != nil && update.TerminateOptions.CompletedCh != nil {
			close(update.TerminateOptions.CompletedCh)
		}

		// Automatically trigger terminated phase
		// Create a sync update to trigger cleanup
		status.pending = &ProcessUpdate{
			ID:         id,
			UpdateType: UpdateTypeSync,
			StartTime:  time.Now(),
			Config:     update.Config,
		}
	}

	return err
}

// syncTerminated executes SyncTerminatedProcess
func (pm *ProcessManager) syncTerminated(id ProcessID, status *processStatus, update ProcessUpdate) error {
	startTime := time.Now()

	err := pm.syncer.SyncTerminatedProcess(status.ctx, update.Config)

	duration := time.Since(startTime)
	log.Printf("Process %s: terminated sync completed in %v (err=%v)", id, duration, err)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err != nil {
		status.errorCount++
		status.lastError = err
	} else {
		// Transition to finished
		status.finishedAt = time.Now()
		status.finished = true
		status.errorCount = 0
		status.lastError = nil
	}

	return err
}

// completeWork marks work as complete and handles requeue
func (pm *ProcessManager) completeWork(id ProcessID, syncErr error) {
	pm.mu.Lock()

	status, exists := pm.processStatuses[id]
	if !exists {
		pm.mu.Unlock()
		return
	}

	status.working = false

	// If finished, exit worker
	if status.IsFinished() {
		pm.mu.Unlock()
		return
	}

	// Requeue with backoff if error
	if syncErr != nil {
		// TODO: Implement work queue with backoff
		// For now, just log
		log.Printf("Process %s: sync error, would requeue with backoff: %v", id, syncErr)
	}

	// If there's a pending update, signal worker
	hasPending := status.pending != nil
	updateCh := pm.processUpdates[id]

	pm.mu.Unlock()

	if hasPending {
		select {
		case updateCh <- struct{}{}:
		default:
		}
	}
}

// GetProcessStatus returns current status of a process
func (pm *ProcessManager) GetProcessStatus(id ProcessID) (*ProcessStatus, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	status, exists := pm.processStatuses[id]
	if !exists {
		return nil, false
	}

	return &ProcessStatus{
		State:        status.State(),
		Healthy:      status.Healthy(),
		LastSync:     status.syncedAt,
		ErrorCount:   status.errorCount,
		LastError:    status.lastError,
		RestartCount: status.restartCount,
	}, true
}

// IsProcessTerminated checks if process has terminated
func (pm *ProcessManager) IsProcessTerminated(id ProcessID) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	status, exists := pm.processStatuses[id]
	if !exists {
		return false
	}

	return status.IsTerminated()
}

// IsProcessFinished checks if process cleanup completed
func (pm *ProcessManager) IsProcessFinished(id ProcessID) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	status, exists := pm.processStatuses[id]
	if !exists {
		return false
	}

	return status.IsFinished()
}

// Shutdown gracefully stops all processes
func (pm *ProcessManager) Shutdown(ctx context.Context) error {
	log.Printf("Process manager shutting down")

	// Cancel shutdown context to signal all workers
	pm.shutdownCancel()

	// Terminate all processes
	pm.mu.Lock()
	processIDs := make([]ProcessID, 0, len(pm.processStatuses))
	for id := range pm.processStatuses {
		processIDs = append(processIDs, id)
	}
	pm.mu.Unlock()

	// Send termination requests
	for _, id := range processIDs {
		pm.UpdateProcess(ProcessUpdate{
			ID:         id,
			UpdateType: UpdateTypeTerminate,
		})
	}

	// Wait for all workers to finish
	done := make(chan struct{})
	go func() {
		pm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("Process manager shutdown complete")
		return nil
	case <-ctx.Done():
		log.Printf("Process manager shutdown timeout")
		return ctx.Err()
	}
}

// SyncKnownProcesses reconciles desired vs actual processes
// Returns map of active processes
func (pm *ProcessManager) SyncKnownProcesses(desiredIDs []ProcessID) map[ProcessID]ProcessStatus {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	desired := make(map[ProcessID]bool)
	for _, id := range desiredIDs {
		desired[id] = true
	}

	result := make(map[ProcessID]ProcessStatus)

	for id, status := range pm.processStatuses {
		_, known := desired[id]
		orphan := !known

		// Remove finished processes
		if status.IsFinished() {
			if orphan {
				log.Printf("Process %s: removing finished orphan", id)
				delete(pm.processStatuses, id)
				delete(pm.processUpdates, id)
			}
			continue
		}

		// Terminate orphans
		if orphan && !status.IsTerminating() {
			log.Printf("Process %s: terminating orphan", id)
			pm.handleTerminationRequest(id, status, nil)
		}

		// Add to result
		result[id] = ProcessStatus{
			State:        status.State(),
			Healthy:      status.Healthy(),
			LastSync:     status.syncedAt,
			ErrorCount:   status.errorCount,
			LastError:    status.lastError,
			RestartCount: status.restartCount,
		}
	}

	return result
}
