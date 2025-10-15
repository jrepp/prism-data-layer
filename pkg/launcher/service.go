package launcher

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/isolation"
	"github.com/jrepp/prism-data-layer/pkg/procmgr"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service implements the PatternLauncher gRPC service
type Service struct {
	pb.UnimplementedPatternLauncherServer

	// Configuration
	config *Config

	// Pattern registry
	registry *Registry

	// Isolation managers (one per isolation level)
	isolationManagers map[isolation.IsolationLevel]*isolation.IsolationManager
	managersMu        sync.RWMutex

	// Process tracking
	processes   map[string]*ProcessInfo // process_id -> info
	processesMu sync.RWMutex

	// Lifecycle management
	cleanupManager   *CleanupManager
	orphanDetector   *OrphanDetector
	healthMonitor    *HealthCheckMonitor

	// Metrics
	metricsCollector *MetricsCollector

	// Lifecycle
	startTime      time.Time
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// ProcessInfo tracks information about a running process
type ProcessInfo struct {
	ProcessID    string
	PatternName  string
	Namespace    string
	SessionID    string
	Isolation    isolation.IsolationLevel
	Address      string
	PID          int
	StartTime    time.Time
	Config       map[string]string
	RestartCount int    // Number of times restarted
	ErrorCount   int    // Number of consecutive errors
	LastError    string // Last error message
}

// Config holds launcher configuration
type Config struct {
	// Patterns directory
	PatternsDir string

	// Default isolation level
	DefaultIsolation isolation.IsolationLevel

	// Process manager options
	ResyncInterval time.Duration
	BackOffPeriod  time.Duration

	// Resource limits
	CPULimit    float64
	MemoryLimit string
}

// NewService creates a new launcher service
func NewService(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create pattern registry
	registry := NewRegistry(config.PatternsDir)

	// Discover patterns
	if err := registry.Discover(); err != nil {
		return nil, fmt.Errorf("discover patterns: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	svc := &Service{
		config:            config,
		registry:          registry,
		isolationManagers: make(map[isolation.IsolationLevel]*isolation.IsolationManager),
		processes:         make(map[string]*ProcessInfo),
		startTime:         time.Now(),
		shutdownCtx:       ctx,
		shutdownCancel:    cancel,
	}

	// Initialize lifecycle management components
	svc.cleanupManager = NewCleanupManager(svc)
	svc.orphanDetector = NewOrphanDetector(svc, 60*time.Second) // Check every minute
	svc.healthMonitor = NewHealthCheckMonitor(svc, 30*time.Second) // Check every 30 seconds

	// Initialize metrics collector
	svc.metricsCollector = NewMetricsCollector(svc)

	// Initialize isolation managers for each level
	svc.initIsolationManagers()

	// Start background monitoring (non-blocking)
	go svc.orphanDetector.Start(ctx)
	go svc.healthMonitor.Start(ctx)

	log.Printf("Pattern launcher service created with %d patterns", registry.Count())

	return svc, nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		PatternsDir:      "./patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
		CPULimit:         2.0,
		MemoryLimit:      "1Gi",
	}
}

// initIsolationManagers creates isolation managers for each level
func (s *Service) initIsolationManagers() {
	// Create the real process syncer
	syncer := newPatternProcessSyncer(s)

	levels := []isolation.IsolationLevel{
		isolation.IsolationNone,
		isolation.IsolationNamespace,
		isolation.IsolationSession,
	}

	opts := []procmgr.Option{
		procmgr.WithResyncInterval(s.config.ResyncInterval),
		procmgr.WithBackOffPeriod(s.config.BackOffPeriod),
	}

	for _, level := range levels {
		mgr := isolation.NewIsolationManager(level, syncer, opts...)
		s.isolationManagers[level] = mgr
		log.Printf("Initialized isolation manager for level: %s", level)
	}
}

// LaunchPattern implements the LaunchPattern RPC
func (s *Service) LaunchPattern(ctx context.Context, req *pb.LaunchRequest) (*pb.LaunchResponse, error) {
	log.Printf("LaunchPattern request: pattern=%s, isolation=%s, namespace=%s, session=%s",
		req.PatternName, req.Isolation, req.Namespace, req.SessionId)

	// Validate request
	if req.PatternName == "" {
		return nil, status.Error(codes.InvalidArgument, "pattern_name is required")
	}

	// Get pattern manifest
	manifest, ok := s.registry.GetPattern(req.PatternName)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "pattern not found: %s", req.PatternName)
	}

	log.Printf("Found manifest for pattern %s: version=%s, isolation=%s",
		manifest.Name, manifest.Version, manifest.IsolationLevel)

	// Determine isolation level
	isolationLevel := s.protoToIsolationLevel(req.Isolation)

	// Validate namespace/session requirements
	if isolationLevel == isolation.IsolationNamespace && req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required for NAMESPACE isolation")
	}

	if isolationLevel == isolation.IsolationSession && req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required for SESSION isolation")
	}

	// Get isolation manager
	s.managersMu.RLock()
	mgr, ok := s.isolationManagers[isolationLevel]
	s.managersMu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.Internal, "isolation manager not found for level: %s", isolationLevel)
	}

	// Create isolation key
	key := isolation.IsolationKey{
		Namespace: req.Namespace,
		Session:   req.SessionId,
	}

	// Create process config for syncer
	processConfig := &processConfig{
		PatternName: req.PatternName,
		Manifest:    manifest,
		Key:         key,
		Config:      req.Config,
	}

	// Wrap in isolation.ProcessConfig
	isoConfig := &isolation.ProcessConfig{
		Key:            key,
		BackendConfig:  processConfig,
		GracePeriodSec: req.GracePeriodSecs,
	}

	// Track launch start time for metrics
	launchStart := time.Now()

	// Get or create process
	handle, err := mgr.GetOrCreateProcess(ctx, key, isoConfig)
	if err != nil {
		// Record failed launch
		s.metricsCollector.RecordProcessFailure(req.PatternName, isolationLevel)
		s.metricsCollector.RecordLaunchDuration(req.PatternName, isolationLevel, time.Since(launchStart), false)
		return nil, status.Errorf(codes.Internal, "failed to launch process: %v", err)
	}

	// Record successful launch
	s.metricsCollector.RecordProcessStart(req.PatternName, isolationLevel)
	s.metricsCollector.RecordLaunchDuration(req.PatternName, isolationLevel, time.Since(launchStart), true)

	// Store process info (will be updated with PID/address by syncer)
	s.processesMu.Lock()
	processInfo := &ProcessInfo{
		ProcessID:   string(handle.ID),
		PatternName: req.PatternName,
		Namespace:   req.Namespace,
		SessionID:   req.SessionId,
		Isolation:   isolationLevel,
		Address:     "", // Will be set by syncer
		StartTime:   time.Now(),
		Config:      req.Config,
	}
	s.processes[string(handle.ID)] = processInfo
	s.processesMu.Unlock()

	// Wait briefly for syncer to update address
	time.Sleep(100 * time.Millisecond)

	// Get updated info
	s.processesMu.RLock()
	info := s.processes[string(handle.ID)]
	s.processesMu.RUnlock()

	// Return response
	return &pb.LaunchResponse{
		ProcessId: string(handle.ID),
		State:     pb.ProcessState_STATE_RUNNING,
		Address:   info.Address,
		Healthy:   handle.Health,
	}, nil
}

// ListPatterns implements the ListPatterns RPC
func (s *Service) ListPatterns(ctx context.Context, req *pb.ListPatternsRequest) (*pb.ListPatternsResponse, error) {
	log.Printf("ListPatterns request: pattern=%s, namespace=%s, state=%s",
		req.PatternName, req.Namespace, req.State)

	s.processesMu.RLock()
	defer s.processesMu.RUnlock()

	patterns := make([]*pb.PatternInfo, 0, len(s.processes))

	for _, info := range s.processes {
		// Apply filters
		if req.PatternName != "" && info.PatternName != req.PatternName {
			continue
		}

		if req.Namespace != "" && info.Namespace != req.Namespace {
			continue
		}

		// For Phase 1, all processes are RUNNING (mock)
		if req.State != pb.ProcessState_STATE_RUNNING && req.State != 0 {
			continue
		}

		uptime := time.Since(info.StartTime).Seconds()

		patterns = append(patterns, &pb.PatternInfo{
			PatternName:   info.PatternName,
			ProcessId:     info.ProcessID,
			State:         pb.ProcessState_STATE_RUNNING,
			Address:       info.Address,
			Healthy:       true,
			UptimeSeconds: int64(uptime),
			Namespace:     info.Namespace,
			SessionId:     info.SessionID,
			Pid:           int32(info.PID),
		})
	}

	return &pb.ListPatternsResponse{
		Patterns:   patterns,
		TotalCount: int32(len(s.processes)),
	}, nil
}

// TerminatePattern implements the TerminatePattern RPC
func (s *Service) TerminatePattern(ctx context.Context, req *pb.TerminateRequest) (*pb.TerminateResponse, error) {
	log.Printf("TerminatePattern request: process_id=%s, grace_period=%d, force=%v",
		req.ProcessId, req.GracePeriodSecs, req.Force)

	// Get process info
	s.processesMu.RLock()
	info, ok := s.processes[req.ProcessId]
	s.processesMu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "process not found: %s", req.ProcessId)
	}

	// Get isolation manager
	s.managersMu.RLock()
	mgr, ok := s.isolationManagers[info.Isolation]
	s.managersMu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.Internal, "isolation manager not found")
	}

	// Create isolation key
	key := isolation.IsolationKey{
		Namespace: info.Namespace,
		Session:   info.SessionID,
	}

	// Terminate process
	gracePeriod := req.GracePeriodSecs
	if gracePeriod == 0 {
		gracePeriod = 10 // Default 10 seconds
	}

	if err := mgr.TerminateProcess(ctx, key, gracePeriod); err != nil {
		return &pb.TerminateResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Record process stop
	s.metricsCollector.RecordProcessStop(info.PatternName, info.Isolation)

	// Remove from tracking
	s.processesMu.Lock()
	delete(s.processes, req.ProcessId)
	s.processesMu.Unlock()

	return &pb.TerminateResponse{
		Success: true,
	}, nil
}

// Health implements the Health RPC
func (s *Service) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	s.processesMu.RLock()
	totalProcesses := len(s.processes)
	runningProcesses := 0
	terminatingProcesses := 0
	failedProcesses := 0

	var processes []*pb.PatternInfo
	if req.IncludeProcesses {
		processes = make([]*pb.PatternInfo, 0, totalProcesses)
	}

	for _, info := range s.processes {
		// For Phase 1, all processes are running (mock)
		runningProcesses++

		if req.IncludeProcesses {
			uptime := time.Since(info.StartTime).Seconds()
			processes = append(processes, &pb.PatternInfo{
				PatternName:   info.PatternName,
				ProcessId:     info.ProcessID,
				State:         pb.ProcessState_STATE_RUNNING,
				Address:       info.Address,
				Healthy:       true,
				UptimeSeconds: int64(uptime),
				Namespace:     info.Namespace,
				SessionId:     info.SessionID,
				Pid:           int32(info.PID),
			})
		}
	}
	s.processesMu.RUnlock()

	// Calculate isolation distribution
	isolationDist := make(map[string]int32)
	s.processesMu.RLock()
	for _, info := range s.processes {
		key := info.Isolation.String()
		isolationDist[key]++
	}
	s.processesMu.RUnlock()

	uptime := time.Since(s.startTime).Seconds()

	return &pb.HealthResponse{
		Healthy:                 true,
		TotalProcesses:          int32(totalProcesses),
		RunningProcesses:        int32(runningProcesses),
		TerminatingProcesses:    int32(terminatingProcesses),
		FailedProcesses:         int32(failedProcesses),
		IsolationDistribution:   isolationDist,
		Processes:               processes,
		UptimeSeconds:           int64(uptime),
	}, nil
}

// GetProcessStatus implements the GetProcessStatus RPC
func (s *Service) GetProcessStatus(ctx context.Context, req *pb.GetProcessStatusRequest) (*pb.GetProcessStatusResponse, error) {
	log.Printf("GetProcessStatus request: process_id=%s", req.ProcessId)

	s.processesMu.RLock()
	info, ok := s.processes[req.ProcessId]
	s.processesMu.RUnlock()

	if !ok {
		return &pb.GetProcessStatusResponse{
			NotFound: true,
		}, nil
	}

	uptime := time.Since(info.StartTime).Seconds()

	return &pb.GetProcessStatusResponse{
		Process: &pb.PatternInfo{
			PatternName:   info.PatternName,
			ProcessId:     info.ProcessID,
			State:         pb.ProcessState_STATE_RUNNING,
			Address:       info.Address,
			Healthy:       true,
			UptimeSeconds: int64(uptime),
			Namespace:     info.Namespace,
			SessionId:     info.SessionID,
			Pid:           int32(info.PID),
		},
		NotFound: false,
	}, nil
}

// Shutdown gracefully stops all processes
func (s *Service) Shutdown(ctx context.Context) error {
	log.Printf("Shutting down pattern launcher service")

	// Cancel shutdown context
	s.shutdownCancel()

	// Shutdown all isolation managers
	s.managersMu.RLock()
	managers := make([]*isolation.IsolationManager, 0, len(s.isolationManagers))
	for _, mgr := range s.isolationManagers {
		managers = append(managers, mgr)
	}
	s.managersMu.RUnlock()

	for _, mgr := range managers {
		if err := mgr.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down isolation manager: %v", err)
		}
	}

	log.Printf("Pattern launcher service shutdown complete")
	return nil
}

// GetMetrics returns current metrics snapshot
func (s *Service) GetMetrics() *MetricsSnapshot {
	return s.metricsCollector.GetMetrics()
}

// ExportPrometheusMetrics exports metrics in Prometheus text format
func (s *Service) ExportPrometheusMetrics() string {
	metrics := s.GetMetrics()
	return metrics.ExportPrometheus()
}

// ExportJSONMetrics exports metrics in JSON format
func (s *Service) ExportJSONMetrics() string {
	metrics := s.GetMetrics()
	return metrics.ExportJSON()
}

// protoToIsolationLevel converts protobuf enum to isolation.IsolationLevel
func (s *Service) protoToIsolationLevel(level pb.IsolationLevel) isolation.IsolationLevel {
	switch level {
	case pb.IsolationLevel_ISOLATION_NONE:
		return isolation.IsolationNone
	case pb.IsolationLevel_ISOLATION_NAMESPACE:
		return isolation.IsolationNamespace
	case pb.IsolationLevel_ISOLATION_SESSION:
		return isolation.IsolationSession
	default:
		return s.config.DefaultIsolation
	}
}
