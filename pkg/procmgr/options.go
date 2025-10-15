package procmgr

import (
	"time"
)

// Option configures the ProcessManager
type Option func(*ProcessManager)

// WithSyncer sets the ProcessSyncer implementation
func WithSyncer(syncer ProcessSyncer) Option {
	return func(pm *ProcessManager) {
		pm.syncer = syncer
	}
}

// WithResyncInterval sets periodic resync interval
func WithResyncInterval(d time.Duration) Option {
	return func(pm *ProcessManager) {
		pm.resyncInterval = d
	}
}

// WithBackOffPeriod sets error backoff period
func WithBackOffPeriod(d time.Duration) Option {
	return func(pm *ProcessManager) {
		pm.backOffPeriod = d
	}
}

// WithMetricsCollector sets the metrics collector
func WithMetricsCollector(mc MetricsCollector) Option {
	return func(pm *ProcessManager) {
		pm.metrics = mc
	}
}
