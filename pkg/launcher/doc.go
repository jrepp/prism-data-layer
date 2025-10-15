// Package launcher provides a lightweight process manager for pattern executables.
//
// The launcher implements the bulkhead isolation pattern to manage pattern processes
// with configurable isolation levels, automatic crash recovery, and comprehensive
// observability through Prometheus metrics.
//
// # Quick Start
//
// Create and start a launcher service:
//
//	config := &launcher.Config{
//	    PatternsDir:      "./patterns",
//	    DefaultIsolation: isolation.IsolationNamespace,
//	    ResyncInterval:   30 * time.Second,
//	    BackOffPeriod:    5 * time.Second,
//	}
//
//	service, err := launcher.NewService(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer service.Shutdown(context.Background())
//
//	// Start gRPC server
//	grpcServer := grpc.NewServer()
//	pb.RegisterPatternLauncherServer(grpcServer, service)
//
//	listener, _ := net.Listen("tcp", ":8080")
//	grpcServer.Serve(listener)
//
// # Isolation Levels
//
// The launcher supports three isolation levels:
//
// ISOLATION_NONE: All requests share a single process instance.
// Use for stateless patterns with no tenant data isolation requirements.
// Lowest resource usage (1 process total).
//
//	req := &pb.LaunchRequest{
//	    PatternName: "schema-registry",
//	    Isolation:   pb.IsolationLevel_ISOLATION_NONE,
//	}
//
// ISOLATION_NAMESPACE: Each namespace (tenant) gets its own process instance.
// Use for multi-tenant SaaS applications requiring fault and resource isolation.
// Medium resource usage (N processes for N tenants).
//
//	req := &pb.LaunchRequest{
//	    PatternName: "consumer",
//	    Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
//	    Namespace:   "tenant-a",
//	}
//
// ISOLATION_SESSION: Each session (user) gets its own process instance.
// Use for high-security environments or compliance requirements (PCI-DSS, HIPAA).
// Highest resource usage (M×N processes for M users across N tenants).
//
//	req := &pb.LaunchRequest{
//	    PatternName: "producer",
//	    Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
//	    Namespace:   "tenant-a",
//	    SessionId:   "user-123",
//	}
//
// # Pattern Manifests
//
// Each pattern must have a manifest.yaml file defining its configuration:
//
//	# patterns/consumer/manifest.yaml
//	name: consumer
//	version: 1.0.0
//	executable: ./consumer
//	isolation_level: namespace
//
//	healthcheck:
//	  port: 9090
//	  path: /health
//	  interval: 30s
//	  timeout: 5s
//	  failure_threshold: 3
//
//	resources:
//	  cpu_limit: 1.0
//	  memory_limit: 512Mi
//
//	environment:
//	  LOG_LEVEL: info
//
// The launcher discovers patterns by scanning the patterns directory for
// subdirectories containing manifest.yaml files.
//
// # Error Handling and Recovery
//
// The launcher implements automatic crash recovery with circuit breaker protection:
//
//   - Crashed processes are automatically restarted
//   - Health checks verify process readiness before routing traffic
//   - After 5 consecutive failures, processes are marked terminal (circuit breaker)
//   - Orphan processes are detected and cleaned up every 60 seconds
//   - Health monitoring runs every 30 seconds
//
// Error tracking across restarts:
//
//	info := service.GetProcessStatus(ctx, &pb.GetProcessStatusRequest{
//	    ProcessId: "ns:tenant-a:consumer",
//	})
//	// info.Process contains RestartCount, ErrorCount, LastError
//
// # Metrics and Observability
//
// Prometheus metrics are exported for monitoring:
//
//	// Get Prometheus text format
//	prometheusText := service.ExportPrometheusMetrics()
//
//	// Get JSON format for custom dashboards
//	jsonMetrics := service.ExportJSONMetrics()
//
//	// Get structured snapshot
//	metrics := service.GetMetrics()
//	fmt.Printf("Launch p95: %v\n", metrics.LaunchDurationP95)
//
// Available metrics:
//   - pattern_launcher_processes_total{state} - Process counts by state
//   - pattern_launcher_process_starts_total{pattern_isolation} - Lifecycle counters
//   - pattern_launcher_health_checks_total{pattern,result} - Health check results
//   - pattern_launcher_launch_duration_seconds{quantile} - Launch latency percentiles
//   - pattern_launcher_uptime_seconds - Launcher availability
//
// # Process Lifecycle
//
// Launch a pattern:
//
//	resp, err := client.LaunchPattern(ctx, &pb.LaunchRequest{
//	    PatternName: "consumer",
//	    Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
//	    Namespace:   "tenant-a",
//	    Config: map[string]string{
//	        "kafka_brokers": "localhost:9092",
//	    },
//	})
//	processID := resp.ProcessId
//	address := resp.Address  // gRPC address to connect to pattern
//
// List running patterns:
//
//	resp, err := client.ListPatterns(ctx, &pb.ListPatternsRequest{
//	    Namespace: "tenant-a",
//	})
//	for _, pattern := range resp.Patterns {
//	    fmt.Printf("%s: %s (pid=%d, uptime=%ds)\n",
//	        pattern.PatternName, pattern.ProcessId,
//	        pattern.Pid, pattern.UptimeSeconds)
//	}
//
// Terminate a pattern:
//
//	resp, err := client.TerminatePattern(ctx, &pb.TerminateRequest{
//	    ProcessId:       "ns:tenant-a:consumer",
//	    GracePeriodSecs: 10,  // SIGTERM → wait → SIGKILL
//	})
//
// Check launcher health:
//
//	resp, err := client.Health(ctx, &pb.HealthRequest{
//	    IncludeProcesses: true,
//	})
//	fmt.Printf("Total processes: %d\n", resp.TotalProcesses)
//	fmt.Printf("Running: %d, Failed: %d\n",
//	    resp.RunningProcesses, resp.FailedProcesses)
//
// # Development Workflow
//
// 1. Create pattern executable with /health endpoint:
//
//	func main() {
//	    patternName := os.Getenv("PATTERN_NAME")
//	    healthPort := os.Getenv("HEALTH_PORT")
//
//	    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
//	        w.WriteHeader(http.StatusOK)
//	        fmt.Fprint(w, "OK")
//	    })
//
//	    http.ListenAndServe(":"+healthPort, nil)
//	}
//
// 2. Create manifest.yaml in patterns/<name>/
//
// 3. Start launcher:
//
//	go run cmd/pattern-launcher/main.go \
//	    --patterns-dir ./patterns \
//	    --grpc-port 8080
//
// 4. Launch pattern via gRPC:
//
//	grpcurl -plaintext \
//	    -d '{"pattern_name":"consumer","isolation":"ISOLATION_NAMESPACE","namespace":"test"}' \
//	    localhost:8080 prism.launcher.PatternLauncher/LaunchPattern
//
// # Testing
//
// Run unit tests (fast):
//
//	go test -v -short ./pkg/launcher
//
// Run integration tests (requires actual process launching):
//
//	go test -v ./pkg/launcher
//
// Run specific test suite:
//
//	go test -v -run TestIsolationLevels_Integration ./pkg/launcher
//
// # Configuration Best Practices
//
// Development (fast iteration):
//
//	config := &launcher.Config{
//	    ResyncInterval: 5 * time.Second,   // Fast failure detection
//	    BackOffPeriod:  1 * time.Second,   // Quick retries
//	}
//
// Production (stability):
//
//	config := &launcher.Config{
//	    ResyncInterval: 30 * time.Second,  // Balanced monitoring
//	    BackOffPeriod:  5 * time.Second,   // Avoid retry storms
//	}
//
// # Troubleshooting
//
// Process won't start:
//   - Check pattern executable exists and is executable: ls -la patterns/<name>/<executable>
//   - Verify manifest.yaml syntax: cat patterns/<name>/manifest.yaml
//   - Check logs for validation errors: look for "validate manifest" errors
//
// Process keeps restarting:
//   - Check health endpoint is responding: curl http://localhost:<health_port>/health
//   - Verify process isn't crashing on startup: check stdout/stderr
//   - Review error tracking: info.ErrorCount, info.LastError
//
// High latency on launch:
//   - Check launch duration metrics: metrics.LaunchDurationP95
//   - Verify pattern health check timeout: manifest.healthcheck.timeout
//   - Ensure sufficient resources: check CPU/memory limits
//
// Orphan processes:
//   - Orphan detector runs every 60s and cleans up untracked processes
//   - Manual cleanup: pkill -9 <pattern-name>
//   - Check orphan detector logs: look for "Found N orphaned pattern processes"
//
// # Architecture
//
// The launcher consists of:
//
//   - Service: gRPC API layer, request routing, metrics recording
//   - Syncer: Process lifecycle (exec.Command, health checks, termination)
//   - Registry: Pattern discovery and manifest validation
//   - IsolationManager: Bulkhead isolation per level (none/namespace/session)
//   - ProcessManager: Robust state machine with work queue and backoff
//   - CleanupManager: Resource cleanup after termination
//   - OrphanDetector: Platform-aware orphan process detection
//   - HealthCheckMonitor: Continuous process health monitoring
//   - MetricsCollector: Prometheus-compatible metrics export
//
// Data flow:
//
//	Client → Service → IsolationManager → ProcessManager → Syncer → exec.Command() → Pattern
//	                                                           ↓
//	                                                    Health Check (HTTP)
//	                                                           ↓
//	                                                    MetricsCollector
//
// # See Also
//
//   - RFC-035: Pattern Process Launcher architecture and design
//   - pkg/isolation: Bulkhead isolation implementation
//   - pkg/procmgr: Kubernetes-inspired process manager
//   - proto/prism/launcher: gRPC service definition
package launcher
