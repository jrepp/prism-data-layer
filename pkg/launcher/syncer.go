package launcher

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	"github.com/jrepp/prism/pkg/procmgr"
)

// patternProcessSyncer implements ProcessSyncer for actual process launching
type patternProcessSyncer struct {
	service *Service

	// Process handle tracking
	mu        sync.RWMutex
	processes map[procmgr.ProcessID]*processHandle
}

// processHandle tracks OS-level process information
type processHandle struct {
	Process      *os.Process
	Cmd          *exec.Cmd
	Config       *processConfig
	StartTime    time.Time
	GRPCAddress  string
	HealthURL    string
	RestartCount int       // Number of times this process has been restarted
	LastError    error     // Last error that occurred
	ErrorCount   int       // Number of consecutive errors
	LastHealthy  time.Time // Last time health check succeeded
}

// processConfig contains pattern-specific launch configuration
type processConfig struct {
	PatternName string
	Manifest    *Manifest
	Key         isolation.IsolationKey
	Config      map[string]string
	GRPCPort    int
	HealthPort  int
}

// newPatternProcessSyncer creates a new syncer for actual process launching
func newPatternProcessSyncer(service *Service) *patternProcessSyncer {
	return &patternProcessSyncer{
		service:   service,
		processes: make(map[procmgr.ProcessID]*processHandle),
	}
}

// SyncProcess implements procmgr.ProcessSyncer.SyncProcess
func (s *patternProcessSyncer) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (terminal bool, err error) {
	// Extract process config from isolation.ProcessConfig wrapper
	isoConfig, ok := config.(*isolation.ProcessConfig)
	if !ok {
		return false, fmt.Errorf("invalid config type: expected *isolation.ProcessConfig")
	}

	processConfig, ok := isoConfig.BackendConfig.(*processConfig)
	if !ok {
		return false, fmt.Errorf("invalid backend config type: expected *processConfig")
	}

	log.Printf("SyncProcess: pattern=%s, type=%s", processConfig.PatternName, updateType)

	// Generate process ID
	processID := processConfig.Key.ProcessID(s.service.config.DefaultIsolation)

	s.mu.RLock()
	handle, exists := s.processes[processID]
	s.mu.RUnlock()

	// If process already exists and running, check if still alive
	if exists && handle.Process != nil {
		// Check if process is still running
		if err := handle.Process.Signal(syscall.Signal(0)); err == nil {
			// Process is still alive, check health
			if s.checkHealth(handle) {
				log.Printf("Process %s is healthy, nothing to do", processID)
				// Reset error count on successful health check
				handle.ErrorCount = 0
				handle.LastHealthy = time.Now()
				return false, nil
			}
			log.Printf("Process %s is unhealthy (errors: %d), will restart", processID, handle.ErrorCount+1)

			// Increment error count
			handle.ErrorCount++

			// Check if we've exceeded max consecutive errors
			maxErrors := 5
			if handle.ErrorCount >= maxErrors {
				log.Printf("Process %s has exceeded max errors (%d), marking as terminal", processID, maxErrors)
				// Kill process and let it stay terminated
				if handle.Process != nil {
					handle.Process.Kill()
				}
				return true, fmt.Errorf("process exceeded max consecutive errors: %d", maxErrors)
			}

			// Kill unhealthy process for restart
			if handle.Process != nil {
				handle.Process.Kill()
				// Wait for process to die
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			// Process is dead, need to restart
			log.Printf("Process %s is dead (signal check failed: %v), will restart", processID, err)
			handle.RestartCount++
			// Record restart metric (use isolation from ProcessConfig)
			isolationLevel := s.service.config.DefaultIsolation
			if handle.Config != nil && handle.Config.Manifest != nil {
				// Parse isolation from manifest
				switch handle.Config.Manifest.IsolationLevel {
				case "none":
					isolationLevel = isolation.IsolationNone
				case "namespace":
					isolationLevel = isolation.IsolationNamespace
				case "session":
					isolationLevel = isolation.IsolationSession
				}
			}
			s.service.metricsCollector.RecordProcessRestart(processConfig.PatternName, isolationLevel)
		}
	}

	// Launch new process (or restart existing)
	var newHandle *processHandle
	if exists {
		// Restarting existing process - preserve restart count and error tracking
		newHandle, err = s.launchProcess(ctx, processID, processConfig)
		if err != nil {
			handle.LastError = err
			handle.ErrorCount++
			log.Printf("Failed to restart process %s (attempt %d): %v", processID, handle.RestartCount, err)
			return false, fmt.Errorf("launch process: %w", err)
		}
		// Transfer error tracking to new handle
		newHandle.RestartCount = handle.RestartCount
		newHandle.ErrorCount = handle.ErrorCount
		newHandle.LastError = handle.LastError
	} else {
		// Launching new process
		newHandle, err = s.launchProcess(ctx, processID, processConfig)
		if err != nil {
			log.Printf("Failed to launch new process %s: %v", processID, err)
			return false, fmt.Errorf("launch process: %w", err)
		}
	}

	// Store handle
	s.mu.Lock()
	s.processes[processID] = newHandle
	s.mu.Unlock()

	// Wait for health check to pass
	if err := s.waitForHealthy(ctx, newHandle); err != nil {
		// Health check failed, kill process and track error
		if newHandle.Process != nil {
			newHandle.Process.Kill()
		}
		newHandle.LastError = err
		newHandle.ErrorCount++

		// Check if this is a persistent failure
		if newHandle.ErrorCount >= 3 {
			log.Printf("Process %s failed health check %d times, may need manual intervention",
				processID, newHandle.ErrorCount)
		}

		return false, fmt.Errorf("health check failed: %w", err)
	}

	// Health check passed - reset error count
	newHandle.ErrorCount = 0
	newHandle.LastHealthy = time.Now()
	newHandle.LastError = nil

	// Update service process tracking
	s.service.processesMu.Lock()
	if info, ok := s.service.processes[string(processID)]; ok {
		info.PID = handle.Process.Pid
		info.Address = handle.GRPCAddress
	}
	s.service.processesMu.Unlock()

	return false, nil
}

// launchProcess starts a new pattern process
func (s *patternProcessSyncer) launchProcess(ctx context.Context, processID procmgr.ProcessID, config *processConfig) (*processHandle, error) {
	manifest := config.Manifest

	// Allocate ports for the process
	grpcPort := s.allocatePort()
	healthPort := grpcPort + 1

	// Build command
	cmd := exec.CommandContext(ctx, manifest.ExecutablePath())

	// Set environment variables
	env := os.Environ()
	env = append(env,
		fmt.Sprintf("PATTERN_NAME=%s", config.PatternName),
		fmt.Sprintf("NAMESPACE=%s", config.Key.Namespace),
		fmt.Sprintf("SESSION_ID=%s", config.Key.Session),
		fmt.Sprintf("GRPC_PORT=%d", grpcPort),
		fmt.Sprintf("HEALTH_PORT=%d", healthPort),
		fmt.Sprintf("PROCESS_ID=%s", processID),
	)

	// Add pattern-specific config as environment variables
	for k, v := range config.Config {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add manifest environment variables
	for k, v := range manifest.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd.Env = env

	// Set stdout/stderr to log
	// TODO: In production, redirect to proper log files
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	log.Printf("Launched process: pattern=%s, pid=%d, grpc=%d, health=%d",
		config.PatternName, cmd.Process.Pid, grpcPort, healthPort)

	// Create handle
	handle := &processHandle{
		Process:      cmd.Process,
		Cmd:          cmd,
		Config:       config,
		StartTime:    time.Now(),
		GRPCAddress:  fmt.Sprintf("localhost:%d", grpcPort),
		HealthURL:    fmt.Sprintf("http://localhost:%d%s", healthPort, manifest.HealthCheck.Path),
		RestartCount: 0,
		ErrorCount:   0,
		LastError:    nil,
		LastHealthy:  time.Time{}, // Will be set after first successful health check
	}

	return handle, nil
}

// waitForHealthy polls the health endpoint until it returns success
func (s *patternProcessSyncer) waitForHealthy(ctx context.Context, handle *processHandle) error {
	manifest := handle.Config.Manifest
	timeout := manifest.HealthCheck.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	interval := 100 * time.Millisecond
	maxWait := 30 * time.Second // Maximum time to wait for process to be healthy
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			if s.checkHealth(handle) {
				log.Printf("Process %s is healthy", handle.Config.PatternName)
				return nil
			}
		}
	}

	return fmt.Errorf("health check timeout after %v", maxWait)
}

// checkHealth performs a single health check
func (s *patternProcessSyncer) checkHealth(handle *processHandle) bool {
	manifest := handle.Config.Manifest
	timeout := manifest.HealthCheck.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(handle.HealthURL)
	if err != nil {
		// Record health check failure
		s.service.metricsCollector.RecordHealthCheckFailure(handle.Config.PatternName)
		return false
	}
	defer resp.Body.Close()

	success := resp.StatusCode == http.StatusOK

	// Record health check result
	if success {
		s.service.metricsCollector.RecordHealthCheckSuccess(handle.Config.PatternName)
	} else {
		s.service.metricsCollector.RecordHealthCheckFailure(handle.Config.PatternName)
	}

	return success
}

// SyncTerminatingProcess implements procmgr.ProcessSyncer.SyncTerminatingProcess
func (s *patternProcessSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error {
	// Extract process config
	isoConfig, ok := config.(*isolation.ProcessConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *isolation.ProcessConfig")
	}

	processConfig, ok := isoConfig.BackendConfig.(*processConfig)
	if !ok {
		return fmt.Errorf("invalid backend config type: expected *processConfig")
	}

	processID := processConfig.Key.ProcessID(s.service.config.DefaultIsolation)

	log.Printf("SyncTerminatingProcess: process=%s, gracePeriod=%d", processID, *gracePeriodSecs)

	s.mu.RLock()
	handle, exists := s.processes[processID]
	s.mu.RUnlock()

	if !exists {
		log.Printf("Process %s not found, already terminated", processID)
		return nil
	}

	// Send SIGTERM for graceful shutdown
	if handle.Process != nil {
		if err := handle.Process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Error sending SIGTERM to process %s: %v", processID, err)
		}
	}

	// Wait for graceful exit
	gracePeriod := time.Duration(*gracePeriodSecs) * time.Second
	done := make(chan error, 1)

	go func() {
		if handle.Cmd != nil {
			done <- handle.Cmd.Wait()
		} else if handle.Process != nil {
			_, err := handle.Process.Wait()
			done <- err
		} else {
			done <- nil
		}
	}()

	select {
	case err := <-done:
		// Process exited gracefully
		log.Printf("Process %s exited gracefully: %v", processID, err)
		return nil

	case <-time.After(gracePeriod):
		// Grace period expired, force kill
		log.Printf("Process %s did not exit within grace period, force killing", processID)
		if handle.Process != nil {
			if err := handle.Process.Kill(); err != nil {
				return fmt.Errorf("force kill: %w", err)
			}
		}

		// Wait a bit for process to die
		select {
		case <-done:
			log.Printf("Process %s killed successfully", processID)
		case <-time.After(5 * time.Second):
			return fmt.Errorf("process did not die after SIGKILL")
		}

		return nil
	}
}

// SyncTerminatedProcess implements procmgr.ProcessSyncer.SyncTerminatedProcess
func (s *patternProcessSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
	// Extract process config
	isoConfig, ok := config.(*isolation.ProcessConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *isolation.ProcessConfig")
	}

	processConfig, ok := isoConfig.BackendConfig.(*processConfig)
	if !ok {
		return fmt.Errorf("invalid backend config type: expected *processConfig")
	}

	processID := processConfig.Key.ProcessID(s.service.config.DefaultIsolation)

	log.Printf("SyncTerminatedProcess: process=%s (cleanup)", processID)

	// Remove from tracking
	s.mu.Lock()
	delete(s.processes, processID)
	s.mu.Unlock()

	return nil
}

// allocatePort allocates a port for a new process
// For Phase 2, we'll use a simple incrementing counter
// In production, this should use a proper port allocator
func (s *patternProcessSyncer) allocatePort() int {
	// Start at 50051 (standard gRPC port)
	basePort := 50051

	s.mu.RLock()
	count := len(s.processes)
	s.mu.RUnlock()

	// Allocate ports in increments of 10 (allows room for health, metrics, etc.)
	return basePort + (count * 10)
}
