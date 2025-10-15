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
	Process     *os.Process
	Cmd         *exec.Cmd
	Config      *processConfig
	StartTime   time.Time
	GRPCAddress string
	HealthURL   string
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
				return false, nil
			}
			log.Printf("Process %s is unhealthy, will restart", processID)
			// Kill unhealthy process
			handle.Process.Kill()
		}
	}

	// Launch new process
	handle, err = s.launchProcess(ctx, processID, processConfig)
	if err != nil {
		return false, fmt.Errorf("launch process: %w", err)
	}

	// Store handle
	s.mu.Lock()
	s.processes[processID] = handle
	s.mu.Unlock()

	// Wait for health check to pass
	if err := s.waitForHealthy(ctx, handle); err != nil {
		// Health check failed, kill process
		if handle.Process != nil {
			handle.Process.Kill()
		}
		return false, fmt.Errorf("health check failed: %w", err)
	}

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
		Process:     cmd.Process,
		Cmd:         cmd,
		Config:      config,
		StartTime:   time.Now(),
		GRPCAddress: fmt.Sprintf("localhost:%d", grpcPort),
		HealthURL:   fmt.Sprintf("http://localhost:%d%s", healthPort, manifest.HealthCheck.Path),
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
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
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
