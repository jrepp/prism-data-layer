package procmgr

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ProcessManager manages lifecycle of external processes for integration testing.
// Provides controlled startup, shutdown, crash simulation, and health monitoring.
type ProcessManager struct {
	mu        sync.RWMutex
	processes map[string]*ManagedProcess
	logDir    string
}

// ManagedProcess represents a single managed process with its lifecycle
type ManagedProcess struct {
	ID      string
	Cmd     *exec.Cmd
	LogFile *os.File
	Started time.Time
	Stopped bool
	mu      sync.Mutex
}

// ProcessConfig defines how to start a process
type ProcessConfig struct {
	ID          string            // Unique identifier for this process
	Executable  string            // Path to executable
	Args        []string          // Command line arguments
	Env         map[string]string // Environment variables
	WorkDir     string            // Working directory
	LogFile     string            // Path to log file (optional)
	Stdout      io.Writer         // Custom stdout writer (optional)
	Stderr      io.Writer         // Custom stderr writer (optional)
	StartupWait time.Duration     // How long to wait before considering started
}

// NewProcessManager creates a new process manager
func NewProcessManager(logDir string) *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ManagedProcess),
		logDir:    logDir,
	}
}

// Start launches a process with the given configuration
func (pm *ProcessManager) Start(ctx context.Context, cfg ProcessConfig) (*ManagedProcess, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if _, exists := pm.processes[cfg.ID]; exists {
		return nil, fmt.Errorf("process %s already running", cfg.ID)
	}

	// Create command
	cmd := exec.CommandContext(ctx, cfg.Executable, cfg.Args...)
	cmd.Dir = cfg.WorkDir

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Setup logging
	var logFile *os.File
	if cfg.LogFile != "" {
		var err error
		logFile, err = os.Create(cfg.LogFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create log file: %w", err)
		}
	}

	// Setup stdout/stderr
	if cfg.Stdout != nil {
		if logFile != nil {
			cmd.Stdout = io.MultiWriter(cfg.Stdout, logFile)
		} else {
			cmd.Stdout = cfg.Stdout
		}
	} else if logFile != nil {
		cmd.Stdout = logFile
	}

	if cfg.Stderr != nil {
		if logFile != nil {
			cmd.Stderr = io.MultiWriter(cfg.Stderr, logFile)
		} else {
			cmd.Stderr = cfg.Stderr
		}
	} else if logFile != nil {
		cmd.Stderr = logFile
	}

	// Start process
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	proc := &ManagedProcess{
		ID:      cfg.ID,
		Cmd:     cmd,
		LogFile: logFile,
		Started: time.Now(),
		Stopped: false,
	}

	pm.processes[cfg.ID] = proc

	// Wait for startup if configured
	if cfg.StartupWait > 0 {
		time.Sleep(cfg.StartupWait)
	}

	return proc, nil
}

// Stop gracefully stops a process by sending SIGTERM
func (pm *ProcessManager) Stop(id string) error {
	pm.mu.Lock()
	proc, exists := pm.processes[id]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("process %s not found", id)
	}
	delete(pm.processes, id)
	pm.mu.Unlock()

	return proc.Stop()
}

// Kill forcefully kills a process by sending SIGKILL
func (pm *ProcessManager) Kill(id string) error {
	pm.mu.Lock()
	proc, exists := pm.processes[id]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("process %s not found", id)
	}
	delete(pm.processes, id)
	pm.mu.Unlock()

	return proc.Kill()
}

// StopAll stops all managed processes gracefully
func (pm *ProcessManager) StopAll() error {
	pm.mu.Lock()
	procs := make([]*ManagedProcess, 0, len(pm.processes))
	for _, proc := range pm.processes {
		procs = append(procs, proc)
	}
	pm.processes = make(map[string]*ManagedProcess)
	pm.mu.Unlock()

	var firstErr error
	for _, proc := range procs {
		if err := proc.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// KillAll forcefully kills all managed processes
func (pm *ProcessManager) KillAll() error {
	pm.mu.Lock()
	procs := make([]*ManagedProcess, 0, len(pm.processes))
	for _, proc := range pm.processes {
		procs = append(procs, proc)
	}
	pm.processes = make(map[string]*ManagedProcess)
	pm.mu.Unlock()

	var firstErr error
	for _, proc := range procs {
		if err := proc.Kill(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Get retrieves a managed process by ID
func (pm *ProcessManager) Get(id string) (*ManagedProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	proc, ok := pm.processes[id]
	return proc, ok
}

// List returns all managed process IDs
func (pm *ProcessManager) List() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	ids := make([]string, 0, len(pm.processes))
	for id := range pm.processes {
		ids = append(ids, id)
	}
	return ids
}

// ManagedProcess methods

// Stop gracefully stops the process (SIGTERM)
func (mp *ManagedProcess) Stop() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.Stopped {
		return nil
	}

	mp.Stopped = true

	// Send SIGTERM
	if mp.Cmd.Process != nil {
		if err := mp.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to send SIGTERM: %w", err)
		}

		// Wait for graceful shutdown (max 5s)
		done := make(chan error, 1)
		go func() {
			done <- mp.Cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill after timeout
			mp.Cmd.Process.Kill()
		}
	}

	if mp.LogFile != nil {
		mp.LogFile.Close()
	}

	return nil
}

// Kill forcefully kills the process (SIGKILL)
func (mp *ManagedProcess) Kill() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.Stopped {
		return nil
	}

	mp.Stopped = true

	if mp.Cmd.Process != nil {
		if err := mp.Cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		mp.Cmd.Wait() // Reap zombie
	}

	if mp.LogFile != nil {
		mp.LogFile.Close()
	}

	return nil
}

// IsRunning checks if the process is still running
func (mp *ManagedProcess) IsRunning() bool {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.Stopped {
		return false
	}

	if mp.Cmd.Process == nil {
		return false
	}

	// Try to send signal 0 to check if process exists
	err := mp.Cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// PID returns the process ID
func (mp *ManagedProcess) PID() int {
	if mp.Cmd.Process == nil {
		return 0
	}
	return mp.Cmd.Process.Pid
}

// Uptime returns how long the process has been running
func (mp *ManagedProcess) Uptime() time.Duration {
	if mp.Stopped {
		return 0
	}
	return time.Since(mp.Started)
}
