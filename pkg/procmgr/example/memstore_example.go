package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism/pkg/procmgr"
)

// MemStoreSyncer implements ProcessSyncer for managing MemStore plugin lifecycle
type MemStoreSyncer struct {
	plugins map[procmgr.ProcessID]*memstore.MemStore
}

// NewMemStoreSyncer creates a new syncer for MemStore plugins
func NewMemStoreSyncer() *MemStoreSyncer {
	return &MemStoreSyncer{
		plugins: make(map[procmgr.ProcessID]*memstore.MemStore),
	}
}

// SyncProcess starts or updates a MemStore process
func (s *MemStoreSyncer) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (terminal bool, err error) {
	processConfig, ok := config.(*ProcessConfig)
	if !ok {
		return false, fmt.Errorf("invalid config type: expected *ProcessConfig")
	}

	// Get or create plugin instance
	mem, exists := s.plugins[processConfig.ID]
	if !exists {
		log.Printf("Creating new MemStore instance: %s", processConfig.ID)
		mem = memstore.New()
		s.plugins[processConfig.ID] = mem
	}

	// Initialize if needed
	if updateType == procmgr.UpdateTypeCreate || updateType == procmgr.UpdateTypeUpdate {
		log.Printf("Initializing MemStore %s with config: %+v", processConfig.ID, processConfig.PluginConfig)
		if err := mem.Initialize(ctx, processConfig.PluginConfig); err != nil {
			return false, fmt.Errorf("initialize failed: %w", err)
		}

		log.Printf("Starting MemStore %s", processConfig.ID)
		if err := mem.Start(ctx); err != nil {
			return false, fmt.Errorf("start failed: %w", err)
		}
	}

	// Health check
	log.Printf("Checking health of MemStore %s", processConfig.ID)
	health, err := mem.Health(ctx)
	if err != nil {
		return false, fmt.Errorf("health check failed: %w", err)
	}

	log.Printf("MemStore %s health: %s - %s", processConfig.ID, health.Status, health.Message)

	// Return terminal=true if health is unhealthy (for demonstration)
	if health.Status == plugin.HealthUnhealthy {
		return true, fmt.Errorf("plugin is unhealthy")
	}

	return false, nil
}

// SyncTerminatingProcess stops a MemStore process
func (s *MemStoreSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error {
	processConfig, ok := config.(*ProcessConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *ProcessConfig")
	}

	mem, exists := s.plugins[processConfig.ID]
	if !exists {
		log.Printf("MemStore %s not found (already stopped?)", processConfig.ID)
		return nil
	}

	log.Printf("Stopping MemStore %s (grace period: %d seconds)", processConfig.ID, *gracePeriodSecs)

	// Create timeout context based on grace period
	stopCtx := ctx
	if gracePeriodSecs != nil && *gracePeriodSecs > 0 {
		var cancel context.CancelFunc
		stopCtx, cancel = context.WithTimeout(ctx, time.Duration(*gracePeriodSecs)*time.Second)
		defer cancel()
	}

	if err := mem.Stop(stopCtx); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}

	log.Printf("MemStore %s stopped successfully", processConfig.ID)
	return nil
}

// SyncTerminatedProcess cleans up resources for a stopped process
func (s *MemStoreSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
	processConfig, ok := config.(*ProcessConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *ProcessConfig")
	}

	log.Printf("Cleaning up MemStore %s", processConfig.ID)
	delete(s.plugins, processConfig.ID)
	return nil
}

// ProcessConfig holds configuration for a managed MemStore process
type ProcessConfig struct {
	ID           procmgr.ProcessID
	PluginConfig *plugin.Config
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Printf("=== MemStore ProcessManager Example ===\n")

	// Create process manager with MemStore syncer
	syncer := NewMemStoreSyncer()
	pm := procmgr.NewProcessManager(
		procmgr.WithSyncer(syncer),
		procmgr.WithResyncInterval(10*time.Second),
		procmgr.WithBackOffPeriod(5*time.Second),
	)

	// Create plugin configurations for 3 MemStore instances
	configs := []ProcessConfig{
		{
			ID: "memstore-1",
			PluginConfig: &plugin.Config{
				Plugin: plugin.PluginConfig{
					Name:    "memstore-1",
					Version: "0.1.0",
				},
				Backend: map[string]interface{}{
					"max_keys":       1000,
					"cleanup_period": "30s",
				},
			},
		},
		{
			ID: "memstore-2",
			PluginConfig: &plugin.Config{
				Plugin: plugin.PluginConfig{
					Name:    "memstore-2",
					Version: "0.1.0",
				},
				Backend: map[string]interface{}{
					"max_keys":       5000,
					"cleanup_period": "60s",
				},
			},
		},
		{
			ID: "memstore-3",
			PluginConfig: &plugin.Config{
				Plugin: plugin.PluginConfig{
					Name:    "memstore-3",
					Version: "0.1.0",
				},
				Backend: map[string]interface{}{
					"max_keys":       100,
					"cleanup_period": "15s",
				},
			},
		},
	}

	// Start all processes
	log.Printf("\n--- Starting 3 MemStore processes ---")
	for _, cfg := range configs {
		pm.UpdateProcess(procmgr.ProcessUpdate{
			ID:         cfg.ID,
			UpdateType: procmgr.UpdateTypeCreate,
			Config:     &cfg,
		})
	}

	// Wait for processes to initialize
	time.Sleep(2 * time.Second)

	// Check health
	log.Printf("\n--- Checking process health ---")
	health := pm.Health()
	log.Printf("Total processes: %d", health.TotalProcesses)
	log.Printf("Running processes: %d", health.RunningProcesses)
	log.Printf("Failed processes: %d", health.FailedProcesses)
	log.Printf("Work queue depth: %d", health.WorkQueueDepth)

	for id, procHealth := range health.Processes {
		log.Printf("  Process %s: state=%s, healthy=%v, uptime=%v, errors=%d",
			id, procHealth.State, procHealth.Healthy, procHealth.Uptime, procHealth.ErrorCount)
	}

	// Let processes run for a bit with periodic resyncs
	log.Printf("\n--- Running for 15 seconds (periodic resyncs will occur) ---")
	time.Sleep(15 * time.Second)

	// Update one process
	log.Printf("\n--- Updating memstore-2 configuration ---")
	configs[1].PluginConfig.Backend = map[string]interface{}{
		"max_keys":       10000,
		"cleanup_period": "45s",
	}
	pm.UpdateProcess(procmgr.ProcessUpdate{
		ID:         configs[1].ID,
		UpdateType: procmgr.UpdateTypeUpdate,
		Config:     &configs[1],
	})

	time.Sleep(2 * time.Second)

	// Terminate one process gracefully
	log.Printf("\n--- Terminating memstore-1 gracefully ---")
	gracePeriod := int64(5)
	pm.UpdateProcess(procmgr.ProcessUpdate{
		ID:         "memstore-1",
		UpdateType: procmgr.UpdateTypeTerminate,
		TerminateOptions: &procmgr.TerminateOptions{
			GracePeriodSecs: &gracePeriod,
		},
	})

	// Wait for termination
	time.Sleep(6 * time.Second)

	// Check final health
	log.Printf("\n--- Final health check ---")
	health = pm.Health()
	log.Printf("Total processes: %d", health.TotalProcesses)
	log.Printf("Running processes: %d", health.RunningProcesses)
	log.Printf("Terminating processes: %d", health.TerminatingProcesses)

	for id, procHealth := range health.Processes {
		log.Printf("  Process %s: state=%s, healthy=%v, uptime=%v",
			id, procHealth.State, procHealth.Healthy, procHealth.Uptime)
	}

	// Shutdown - terminates all remaining processes
	log.Printf("\n--- Shutting down process manager ---")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pm.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	} else {
		log.Printf("Shutdown completed successfully")
	}

	log.Printf("\n=== Example complete ===")
}
