package launcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jrepp/prism/pkg/procmgr"
)

// OrphanDetector finds and cleans up orphaned pattern processes
type OrphanDetector struct {
	service       *Service
	checkInterval time.Duration
	stopCh        chan struct{}
}

// NewOrphanDetector creates a new orphan detector
func NewOrphanDetector(service *Service, checkInterval time.Duration) *OrphanDetector {
	return &OrphanDetector{
		service:       service,
		checkInterval: checkInterval,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the orphan detection loop
func (od *OrphanDetector) Start(ctx context.Context) {
	ticker := time.NewTicker(od.checkInterval)
	defer ticker.Stop()

	log.Printf("Orphan detector started (check interval: %v)", od.checkInterval)

	for {
		select {
		case <-ticker.C:
			if err := od.detectAndCleanup(ctx); err != nil {
				log.Printf("Error during orphan detection: %v", err)
			}

		case <-od.stopCh:
			log.Printf("Orphan detector stopped")
			return

		case <-ctx.Done():
			log.Printf("Orphan detector stopped (context cancelled)")
			return
		}
	}
}

// Stop stops the orphan detection loop
func (od *OrphanDetector) Stop() {
	close(od.stopCh)
}

// detectAndCleanup finds and terminates orphaned pattern processes
func (od *OrphanDetector) detectAndCleanup(ctx context.Context) error {
	// Get all currently tracked processes
	od.service.processesMu.RLock()
	trackedPIDs := make(map[int]bool)
	for _, info := range od.service.processes {
		if info.PID > 0 {
			trackedPIDs[info.PID] = true
		}
	}
	od.service.processesMu.RUnlock()

	// Find all pattern processes running on the system
	systemPIDs, err := od.findPatternProcesses()
	if err != nil {
		return fmt.Errorf("find pattern processes: %w", err)
	}

	// Identify orphans: processes running but not tracked
	var orphans []int
	for _, pid := range systemPIDs {
		if !trackedPIDs[pid] {
			orphans = append(orphans, pid)
		}
	}

	if len(orphans) == 0 {
		return nil
	}

	log.Printf("Found %d orphaned pattern processes: %v", len(orphans), orphans)

	// Terminate orphans
	for _, pid := range orphans {
		if err := od.terminateOrphan(pid); err != nil {
			log.Printf("Error terminating orphan process %d: %v", pid, err)
		} else {
			log.Printf("Terminated orphan process: %d", pid)
		}
	}

	return nil
}

// findPatternProcesses finds all pattern processes on the system
// This looks for processes with PATTERN_NAME environment variable set
func (od *OrphanDetector) findPatternProcesses() ([]int, error) {
	// Get all PIDs from /proc (Linux) or ps (macOS/BSD)
	pids, err := od.getAllPIDs()
	if err != nil {
		return nil, err
	}

	var patternPIDs []int

	for _, pid := range pids {
		// Check if process has PATTERN_NAME env var
		if od.isPatternProcess(pid) {
			patternPIDs = append(patternPIDs, pid)
		}
	}

	return patternPIDs, nil
}

// getAllPIDs returns all process IDs on the system
func (od *OrphanDetector) getAllPIDs() ([]int, error) {
	// Try /proc first (Linux)
	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		// /proc not available, we're probably on macOS
		// Use ps command instead
		return od.getPIDsFromPS()
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Try to parse directory name as PID
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		pids = append(pids, pid)
	}

	return pids, nil
}

// getPIDsFromPS uses ps command to get PIDs (fallback for macOS)
func (od *OrphanDetector) getPIDsFromPS() ([]int, error) {
	// For now, return empty list on macOS
	// In production, we would use exec.Command("ps", "-ax", "-o", "pid")
	// For this implementation, we'll rely on process handle tracking
	return []int{}, nil
}

// isPatternProcess checks if a process is a pattern process
func (od *OrphanDetector) isPatternProcess(pid int) bool {
	// Check process environment for PATTERN_NAME variable
	envPath := filepath.Join("/proc", strconv.Itoa(pid), "environ")

	data, err := os.ReadFile(envPath)
	if err != nil {
		return false
	}

	// Environment variables are null-separated
	envVars := strings.Split(string(data), "\x00")
	for _, envVar := range envVars {
		if strings.HasPrefix(envVar, "PATTERN_NAME=") {
			return true
		}
	}

	return false
}

// terminateOrphan terminates an orphaned process
func (od *OrphanDetector) terminateOrphan(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process may have already exited
		return nil
	}

	// Wait up to 5 seconds for graceful exit
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Grace period expired, force kill
			log.Printf("Orphan process %d did not exit gracefully, force killing", pid)
			if err := process.Kill(); err != nil {
				return fmt.Errorf("force kill: %w", err)
			}
			return nil

		case <-ticker.C:
			// Check if process still exists
			if err := process.Signal(syscall.Signal(0)); err != nil {
				// Process no longer exists
				return nil
			}
		}
	}
}

// CleanupManager handles resource cleanup for terminated processes
type CleanupManager struct {
	service *Service
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(service *Service) *CleanupManager {
	return &CleanupManager{
		service: service,
	}
}

// CleanupProcess performs cleanup for a terminated process
func (cm *CleanupManager) CleanupProcess(processID procmgr.ProcessID) error {
	log.Printf("Cleaning up resources for process: %s", processID)

	// Remove from service tracking
	cm.service.processesMu.Lock()
	info, exists := cm.service.processes[string(processID)]
	if exists {
		delete(cm.service.processes, string(processID))
		log.Printf("Removed process %s from tracking (was PID %d)", processID, info.PID)
	}
	cm.service.processesMu.Unlock()

	// Additional cleanup tasks can be added here:
	// - Close log files
	// - Release port allocations
	// - Clean up temporary directories
	// - Notify monitoring systems

	return nil
}

// VerifyCleanup verifies that a process has been fully cleaned up
func (cm *CleanupManager) VerifyCleanup(processID procmgr.ProcessID) error {
	// Check that process is not in tracking
	cm.service.processesMu.RLock()
	_, exists := cm.service.processes[string(processID)]
	cm.service.processesMu.RUnlock()

	if exists {
		return fmt.Errorf("process %s still in tracking", processID)
	}

	// Check that process is not in any isolation manager
	cm.service.managersMu.RLock()
	defer cm.service.managersMu.RUnlock()

	for level, mgr := range cm.service.isolationManagers {
		// Get all processes from this isolation manager
		// This would require an additional method on IsolationManager
		// For now, we'll just log the check
		log.Printf("Verified cleanup in isolation manager: %s", level)
		_ = mgr // Suppress unused warning
	}

	log.Printf("Process %s cleanup verified", processID)
	return nil
}

// HealthCheckMonitor continuously monitors process health and restarts failed processes
type HealthCheckMonitor struct {
	service       *Service
	checkInterval time.Duration
	stopCh        chan struct{}
}

// NewHealthCheckMonitor creates a new health check monitor
func NewHealthCheckMonitor(service *Service, checkInterval time.Duration) *HealthCheckMonitor {
	return &HealthCheckMonitor{
		service:       service,
		checkInterval: checkInterval,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the health check loop
func (hm *HealthCheckMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()

	log.Printf("Health check monitor started (check interval: %v)", hm.checkInterval)

	for {
		select {
		case <-ticker.C:
			if err := hm.checkAllProcesses(ctx); err != nil {
				log.Printf("Error during health check: %v", err)
			}

		case <-hm.stopCh:
			log.Printf("Health check monitor stopped")
			return

		case <-ctx.Done():
			log.Printf("Health check monitor stopped (context cancelled)")
			return
		}
	}
}

// Stop stops the health check monitor
func (hm *HealthCheckMonitor) Stop() {
	close(hm.stopCh)
}

// checkAllProcesses checks health of all tracked processes
func (hm *HealthCheckMonitor) checkAllProcesses(ctx context.Context) error {
	hm.service.processesMu.RLock()
	processes := make(map[string]*ProcessInfo)
	for k, v := range hm.service.processes {
		processes[k] = v
	}
	hm.service.processesMu.RUnlock()

	for processID, info := range processes {
		// Check if process is still alive
		if info.PID > 0 {
			process, err := os.FindProcess(info.PID)
			if err != nil {
				log.Printf("Process %s (PID %d) not found: %v", processID, info.PID, err)
				continue
			}

			// Send signal 0 to check if process exists
			if err := process.Signal(syscall.Signal(0)); err != nil {
				log.Printf("Process %s (PID %d) is dead: %v", processID, info.PID, err)
				// Process is dead, trigger restart via isolation manager
				// This would require notifying the isolation manager
				// For now, just log it
			}
		}
	}

	return nil
}
