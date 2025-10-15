package procmgr

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusMetricsCollector implements MetricsCollector using Prometheus metrics
type PrometheusMetricsCollector struct {
	// State transition metrics
	stateTransitions *prometheus.CounterVec

	// Performance metrics
	syncDuration        *prometheus.HistogramVec
	terminationDuration *prometheus.HistogramVec

	// Error metrics
	errors  *prometheus.CounterVec
	restarts *prometheus.CounterVec

	// Work queue metrics
	queueDepth          prometheus.Gauge
	queueAdds           *prometheus.CounterVec
	queueRetries        *prometheus.CounterVec
	backoffDuration     *prometheus.HistogramVec

	registry *prometheus.Registry
}

// NewPrometheusMetricsCollector creates a new Prometheus metrics collector
func NewPrometheusMetricsCollector(namespace string) *PrometheusMetricsCollector {
	if namespace == "" {
		namespace = "procmgr"
	}

	pmc := &PrometheusMetricsCollector{
		registry: prometheus.NewRegistry(),
	}

	// State transitions
	pmc.stateTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "process_state_transitions_total",
			Help:      "Total number of process state transitions",
		},
		[]string{"process_id", "from_state", "to_state"},
	)

	// Sync duration
	pmc.syncDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "process_sync_duration_seconds",
			Help:      "Duration of process sync operations",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"process_id", "update_type", "status"},
	)

	// Termination duration
	pmc.terminationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "process_termination_duration_seconds",
			Help:      "Duration of process termination operations",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"process_id"},
	)

	// Errors
	pmc.errors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "process_errors_total",
			Help:      "Total number of process errors",
		},
		[]string{"process_id", "error_type"},
	)

	// Restarts
	pmc.restarts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "process_restarts_total",
			Help:      "Total number of process restarts",
		},
		[]string{"process_id"},
	)

	// Queue depth
	pmc.queueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "work_queue_depth",
			Help:      "Current depth of the work queue",
		},
	)

	// Queue adds
	pmc.queueAdds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "work_queue_adds_total",
			Help:      "Total number of items added to work queue",
		},
		[]string{"process_id"},
	)

	// Queue retries
	pmc.queueRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "work_queue_retries_total",
			Help:      "Total number of work queue retry operations",
		},
		[]string{"process_id"},
	)

	// Backoff duration
	pmc.backoffDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "work_queue_backoff_duration_seconds",
			Help:      "Duration of backoff delays for failed processes",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"process_id"},
	)

	// Register all metrics
	pmc.registry.MustRegister(
		pmc.stateTransitions,
		pmc.syncDuration,
		pmc.terminationDuration,
		pmc.errors,
		pmc.restarts,
		pmc.queueDepth,
		pmc.queueAdds,
		pmc.queueRetries,
		pmc.backoffDuration,
	)

	return pmc
}

// ProcessStateTransition records a state transition
func (pmc *PrometheusMetricsCollector) ProcessStateTransition(id ProcessID, fromState, toState ProcessState) {
	pmc.stateTransitions.WithLabelValues(
		string(id),
		fromState.String(),
		toState.String(),
	).Inc()
}

// ProcessSyncDuration records the duration of a sync operation
func (pmc *PrometheusMetricsCollector) ProcessSyncDuration(id ProcessID, updateType UpdateType, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	pmc.syncDuration.WithLabelValues(
		string(id),
		updateType.String(),
		status,
	).Observe(duration.Seconds())
}

// ProcessTerminationDuration records the duration of a termination operation
func (pmc *PrometheusMetricsCollector) ProcessTerminationDuration(id ProcessID, duration time.Duration) {
	pmc.terminationDuration.WithLabelValues(
		string(id),
	).Observe(duration.Seconds())
}

// ProcessError records a process error
func (pmc *PrometheusMetricsCollector) ProcessError(id ProcessID, errorType string) {
	pmc.errors.WithLabelValues(
		string(id),
		errorType,
	).Inc()
}

// ProcessRestart records a process restart
func (pmc *PrometheusMetricsCollector) ProcessRestart(id ProcessID) {
	pmc.restarts.WithLabelValues(
		string(id),
	).Inc()
}

// WorkQueueDepth records the current work queue depth
func (pmc *PrometheusMetricsCollector) WorkQueueDepth(depth int) {
	pmc.queueDepth.Set(float64(depth))
}

// WorkQueueAdd records an item added to the work queue
func (pmc *PrometheusMetricsCollector) WorkQueueAdd(id ProcessID, delay time.Duration) {
	pmc.queueAdds.WithLabelValues(
		string(id),
	).Inc()
}

// WorkQueueRetry records a work queue retry operation
func (pmc *PrometheusMetricsCollector) WorkQueueRetry(id ProcessID) {
	pmc.queueRetries.WithLabelValues(
		string(id),
	).Inc()
}

// WorkQueueBackoffDuration records the duration of a backoff delay
func (pmc *PrometheusMetricsCollector) WorkQueueBackoffDuration(id ProcessID, duration time.Duration) {
	pmc.backoffDuration.WithLabelValues(
		string(id),
	).Observe(duration.Seconds())
}

// Registry returns the Prometheus registry for HTTP handler setup
func (pmc *PrometheusMetricsCollector) Registry() *prometheus.Registry {
	return pmc.registry
}

// Compile-time interface compliance check
var _ MetricsCollector = (*PrometheusMetricsCollector)(nil)
