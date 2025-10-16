package launcher

import "context"

// EventPublisher defines the interface for publishing lifecycle events
// to the admin control plane. This allows components to report their state
// changes, errors, and important lifecycle transitions.
//
// Event types:
//   - starting: Component is initializing
//   - ready: Component is ready to accept work
//   - stopping: Component received shutdown signal
//   - stopped: Component has shut down cleanly
//   - crashed: Component terminated unexpectedly
//   - restarting: Component is being restarted
//   - healthy: Component health check passed
//   - unhealthy: Component health check failed
//   - degraded: Component is operational but performance is reduced
type EventPublisher interface {
	// ReportLifecycleEvent sends a lifecycle event to the admin control plane
	//
	// Parameters:
	//   ctx: Context for the operation
	//   eventType: Type of event (starting, stopping, stopped, crashed, etc.)
	//   message: Human-readable description of the event
	//   metadata: Additional context (signal_name, exit_code, error_details, etc.)
	//
	// Returns error if the event could not be delivered or was rejected.
	ReportLifecycleEvent(ctx context.Context, eventType, message string, metadata map[string]string) error
}

// NoopEventPublisher is a no-op implementation for standalone mode
type NoopEventPublisher struct{}

// ReportLifecycleEvent does nothing in standalone mode
func (n *NoopEventPublisher) ReportLifecycleEvent(ctx context.Context, eventType, message string, metadata map[string]string) error {
	// In standalone mode, we just log locally without sending to admin
	return nil
}
