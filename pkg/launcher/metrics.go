package launcher

import (
	"fmt"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/isolation"
)

// MetricsCollector collects and exports Prometheus-compatible metrics
type MetricsCollector struct {
	service *Service
	mu      sync.RWMutex

	// Lifecycle counters
	processStartsTotal      map[string]int64 // pattern -> count
	processStopsTotal       map[string]int64 // pattern -> count
	processFailuresTotal    map[string]int64 // pattern -> count
	processRestartsTotal    map[string]int64 // pattern -> count
	healthCheckSuccessTotal map[string]int64 // pattern -> count
	healthCheckFailureTotal map[string]int64 // pattern -> count

	// Launch latency tracking
	launchDurations []launchDuration

	// Start time for uptime calculation
	startTime time.Time
}

// launchDuration tracks a single launch operation
type launchDuration struct {
	PatternName string
	Isolation   isolation.IsolationLevel
	Duration    time.Duration
	Success     bool
	Timestamp   time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(service *Service) *MetricsCollector {
	return &MetricsCollector{
		service:                 service,
		processStartsTotal:      make(map[string]int64),
		processStopsTotal:       make(map[string]int64),
		processFailuresTotal:    make(map[string]int64),
		processRestartsTotal:    make(map[string]int64),
		healthCheckSuccessTotal: make(map[string]int64),
		healthCheckFailureTotal: make(map[string]int64),
		launchDurations:         make([]launchDuration, 0, 1000),
		startTime:               time.Now(),
	}
}

// RecordProcessStart records a process start event
func (mc *MetricsCollector) RecordProcessStart(patternName string, isolation isolation.IsolationLevel) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := fmt.Sprintf("%s:%s", patternName, isolation)
	mc.processStartsTotal[key]++
}

// RecordProcessStop records a process stop event
func (mc *MetricsCollector) RecordProcessStop(patternName string, isolation isolation.IsolationLevel) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := fmt.Sprintf("%s:%s", patternName, isolation)
	mc.processStopsTotal[key]++
}

// RecordProcessFailure records a process failure event
func (mc *MetricsCollector) RecordProcessFailure(patternName string, isolation isolation.IsolationLevel) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := fmt.Sprintf("%s:%s", patternName, isolation)
	mc.processFailuresTotal[key]++
}

// RecordProcessRestart records a process restart event
func (mc *MetricsCollector) RecordProcessRestart(patternName string, isolation isolation.IsolationLevel) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := fmt.Sprintf("%s:%s", patternName, isolation)
	mc.processRestartsTotal[key]++
}

// RecordHealthCheckSuccess records a successful health check
func (mc *MetricsCollector) RecordHealthCheckSuccess(patternName string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.healthCheckSuccessTotal[patternName]++
}

// RecordHealthCheckFailure records a failed health check
func (mc *MetricsCollector) RecordHealthCheckFailure(patternName string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.healthCheckFailureTotal[patternName]++
}

// RecordLaunchDuration records the duration of a launch operation
func (mc *MetricsCollector) RecordLaunchDuration(patternName string, isolation isolation.IsolationLevel, duration time.Duration, success bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Keep only last 1000 durations to prevent unbounded growth
	if len(mc.launchDurations) >= 1000 {
		mc.launchDurations = mc.launchDurations[1:]
	}

	mc.launchDurations = append(mc.launchDurations, launchDuration{
		PatternName: patternName,
		Isolation:   isolation,
		Duration:    duration,
		Success:     success,
		Timestamp:   time.Now(),
	})
}

// GetMetrics returns current metrics snapshot
func (mc *MetricsCollector) GetMetrics() *MetricsSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Get current process counts by state
	mc.service.processesMu.RLock()
	totalProcesses := len(mc.service.processes)
	runningProcesses := 0
	terminatingProcesses := 0
	failedProcesses := 0

	// Count by isolation level
	isolationDist := make(map[string]int)

	for _, info := range mc.service.processes {
		// For now, assume all tracked processes are running
		// In a real implementation, we'd check actual state
		runningProcesses++

		levelKey := info.Isolation.String()
		isolationDist[levelKey]++
	}
	mc.service.processesMu.RUnlock()

	// Calculate launch latency percentiles
	p50, p95, p99 := mc.calculateLatencyPercentiles()

	return &MetricsSnapshot{
		// Process counts
		TotalProcesses:       int64(totalProcesses),
		RunningProcesses:     int64(runningProcesses),
		TerminatingProcesses: int64(terminatingProcesses),
		FailedProcesses:      int64(failedProcesses),

		// Isolation distribution
		IsolationDistribution: isolationDist,

		// Lifecycle counters
		ProcessStartsTotal:      mc.copyCounterMap(mc.processStartsTotal),
		ProcessStopsTotal:       mc.copyCounterMap(mc.processStopsTotal),
		ProcessFailuresTotal:    mc.copyCounterMap(mc.processFailuresTotal),
		ProcessRestartsTotal:    mc.copyCounterMap(mc.processRestartsTotal),
		HealthCheckSuccessTotal: mc.copyCounterMap(mc.healthCheckSuccessTotal),
		HealthCheckFailureTotal: mc.copyCounterMap(mc.healthCheckFailureTotal),

		// Launch latency
		LaunchDurationP50: p50,
		LaunchDurationP95: p95,
		LaunchDurationP99: p99,

		// Uptime
		UptimeSeconds: int64(time.Since(mc.startTime).Seconds()),
	}
}

// copyCounterMap creates a copy of a counter map
func (mc *MetricsCollector) copyCounterMap(m map[string]int64) map[string]int64 {
	copy := make(map[string]int64, len(m))
	for k, v := range m {
		copy[k] = v
	}
	return copy
}

// calculateLatencyPercentiles calculates p50, p95, p99 from launch durations
func (mc *MetricsCollector) calculateLatencyPercentiles() (p50, p95, p99 time.Duration) {
	if len(mc.launchDurations) == 0 {
		return 0, 0, 0
	}

	// Extract durations
	durations := make([]time.Duration, len(mc.launchDurations))
	for i, ld := range mc.launchDurations {
		durations[i] = ld.Duration
	}

	// Sort durations (simple bubble sort, sufficient for 1000 items)
	for i := 0; i < len(durations); i++ {
		for j := i + 1; j < len(durations); j++ {
			if durations[i] > durations[j] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}

	// Calculate percentiles
	p50Index := len(durations) * 50 / 100
	p95Index := len(durations) * 95 / 100
	p99Index := len(durations) * 99 / 100

	if p50Index >= len(durations) {
		p50Index = len(durations) - 1
	}
	if p95Index >= len(durations) {
		p95Index = len(durations) - 1
	}
	if p99Index >= len(durations) {
		p99Index = len(durations) - 1
	}

	return durations[p50Index], durations[p95Index], durations[p99Index]
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	// Process counts
	TotalProcesses       int64
	RunningProcesses     int64
	TerminatingProcesses int64
	FailedProcesses      int64

	// Isolation distribution
	IsolationDistribution map[string]int

	// Lifecycle counters (pattern:isolation -> count)
	ProcessStartsTotal      map[string]int64
	ProcessStopsTotal       map[string]int64
	ProcessFailuresTotal    map[string]int64
	ProcessRestartsTotal    map[string]int64
	HealthCheckSuccessTotal map[string]int64
	HealthCheckFailureTotal map[string]int64

	// Launch latency percentiles
	LaunchDurationP50 time.Duration
	LaunchDurationP95 time.Duration
	LaunchDurationP99 time.Duration

	// Uptime
	UptimeSeconds int64
}

// ExportPrometheus exports metrics in Prometheus text format
func (ms *MetricsSnapshot) ExportPrometheus() string {
	var output string

	// Help and type declarations
	output += "# HELP pattern_launcher_processes_total Total number of processes\n"
	output += "# TYPE pattern_launcher_processes_total gauge\n"
	output += fmt.Sprintf("pattern_launcher_processes_total{state=\"running\"} %d\n", ms.RunningProcesses)
	output += fmt.Sprintf("pattern_launcher_processes_total{state=\"terminating\"} %d\n", ms.TerminatingProcesses)
	output += fmt.Sprintf("pattern_launcher_processes_total{state=\"failed\"} %d\n", ms.FailedProcesses)
	output += "\n"

	// Isolation distribution
	output += "# HELP pattern_launcher_isolation_level Process count by isolation level\n"
	output += "# TYPE pattern_launcher_isolation_level gauge\n"
	for level, count := range ms.IsolationDistribution {
		output += fmt.Sprintf("pattern_launcher_isolation_level{level=\"%s\"} %d\n", level, count)
	}
	output += "\n"

	// Process starts
	output += "# HELP pattern_launcher_process_starts_total Total process starts\n"
	output += "# TYPE pattern_launcher_process_starts_total counter\n"
	for key, count := range ms.ProcessStartsTotal {
		output += fmt.Sprintf("pattern_launcher_process_starts_total{pattern_isolation=\"%s\"} %d\n", key, count)
	}
	output += "\n"

	// Process stops
	output += "# HELP pattern_launcher_process_stops_total Total process stops\n"
	output += "# TYPE pattern_launcher_process_stops_total counter\n"
	for key, count := range ms.ProcessStopsTotal {
		output += fmt.Sprintf("pattern_launcher_process_stops_total{pattern_isolation=\"%s\"} %d\n", key, count)
	}
	output += "\n"

	// Process failures
	output += "# HELP pattern_launcher_process_failures_total Total process failures\n"
	output += "# TYPE pattern_launcher_process_failures_total counter\n"
	for key, count := range ms.ProcessFailuresTotal {
		output += fmt.Sprintf("pattern_launcher_process_failures_total{pattern_isolation=\"%s\"} %d\n", key, count)
	}
	output += "\n"

	// Process restarts
	output += "# HELP pattern_launcher_process_restarts_total Total process restarts\n"
	output += "# TYPE pattern_launcher_process_restarts_total counter\n"
	for key, count := range ms.ProcessRestartsTotal {
		output += fmt.Sprintf("pattern_launcher_process_restarts_total{pattern_isolation=\"%s\"} %d\n", key, count)
	}
	output += "\n"

	// Health checks
	output += "# HELP pattern_launcher_health_checks_total Total health checks\n"
	output += "# TYPE pattern_launcher_health_checks_total counter\n"
	for pattern, count := range ms.HealthCheckSuccessTotal {
		output += fmt.Sprintf("pattern_launcher_health_checks_total{pattern=\"%s\",result=\"success\"} %d\n", pattern, count)
	}
	for pattern, count := range ms.HealthCheckFailureTotal {
		output += fmt.Sprintf("pattern_launcher_health_checks_total{pattern=\"%s\",result=\"failure\"} %d\n", pattern, count)
	}
	output += "\n"

	// Launch duration percentiles
	output += "# HELP pattern_launcher_launch_duration_seconds Launch duration percentiles\n"
	output += "# TYPE pattern_launcher_launch_duration_seconds summary\n"
	output += fmt.Sprintf("pattern_launcher_launch_duration_seconds{quantile=\"0.5\"} %.3f\n", ms.LaunchDurationP50.Seconds())
	output += fmt.Sprintf("pattern_launcher_launch_duration_seconds{quantile=\"0.95\"} %.3f\n", ms.LaunchDurationP95.Seconds())
	output += fmt.Sprintf("pattern_launcher_launch_duration_seconds{quantile=\"0.99\"} %.3f\n", ms.LaunchDurationP99.Seconds())
	output += "\n"

	// Uptime
	output += "# HELP pattern_launcher_uptime_seconds Launcher uptime in seconds\n"
	output += "# TYPE pattern_launcher_uptime_seconds counter\n"
	output += fmt.Sprintf("pattern_launcher_uptime_seconds %d\n", ms.UptimeSeconds)
	output += "\n"

	return output
}

// ExportJSON exports metrics in JSON format
func (ms *MetricsSnapshot) ExportJSON() string {
	return fmt.Sprintf(`{
  "total_processes": %d,
  "running_processes": %d,
  "terminating_processes": %d,
  "failed_processes": %d,
  "isolation_distribution": %v,
  "process_starts_total": %v,
  "process_stops_total": %v,
  "process_failures_total": %v,
  "process_restarts_total": %v,
  "health_check_success_total": %v,
  "health_check_failure_total": %v,
  "launch_duration_p50_seconds": %.3f,
  "launch_duration_p95_seconds": %.3f,
  "launch_duration_p99_seconds": %.3f,
  "uptime_seconds": %d
}`,
		ms.TotalProcesses,
		ms.RunningProcesses,
		ms.TerminatingProcesses,
		ms.FailedProcesses,
		ms.IsolationDistribution,
		ms.ProcessStartsTotal,
		ms.ProcessStopsTotal,
		ms.ProcessFailuresTotal,
		ms.ProcessRestartsTotal,
		ms.HealthCheckSuccessTotal,
		ms.HealthCheckFailureTotal,
		ms.LaunchDurationP50.Seconds(),
		ms.LaunchDurationP95.Seconds(),
		ms.LaunchDurationP99.Seconds(),
		ms.UptimeSeconds,
	)
}
