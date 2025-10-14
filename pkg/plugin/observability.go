package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	// ServiceName is the name of the service (e.g., "memstore", "redis")
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// MetricsPort is the port for the Prometheus metrics endpoint
	// Set to 0 to disable metrics HTTP server
	MetricsPort int

	// EnableTracing enables OpenTelemetry tracing
	EnableTracing bool

	// TraceExporter specifies the trace exporter ("stdout", "jaeger", "otlp")
	// Default: "stdout" (for development)
	TraceExporter string
}

// ObservabilityManager manages observability components (tracing, metrics, health)
type ObservabilityManager struct {
	config         *ObservabilityConfig
	tracerProvider *sdktrace.TracerProvider
	metricsServer  *http.Server
	shutdownOnce   sync.Once
}

// NewObservabilityManager creates a new observability manager
func NewObservabilityManager(config *ObservabilityConfig) *ObservabilityManager {
	if config == nil {
		config = &ObservabilityConfig{
			ServiceName:    "unknown",
			ServiceVersion: "0.0.0",
			MetricsPort:    0,
			EnableTracing:  false,
			TraceExporter:  "stdout",
		}
	}

	return &ObservabilityManager{
		config: config,
	}
}

// Initialize sets up observability components
func (o *ObservabilityManager) Initialize(ctx context.Context) error {
	slog.Info("initializing observability",
		"service_name", o.config.ServiceName,
		"service_version", o.config.ServiceVersion,
		"metrics_port", o.config.MetricsPort,
		"enable_tracing", o.config.EnableTracing)

	// Initialize tracing if enabled
	if o.config.EnableTracing {
		if err := o.initializeTracing(ctx); err != nil {
			return fmt.Errorf("failed to initialize tracing: %w", err)
		}
		slog.Info("OpenTelemetry tracing initialized",
			"service_name", o.config.ServiceName,
			"exporter", o.config.TraceExporter)
	}

	// Initialize metrics HTTP server if port is specified
	if o.config.MetricsPort > 0 {
		if err := o.startMetricsServer(); err != nil {
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
		slog.Info("metrics server started",
			"port", o.config.MetricsPort,
			"endpoint", fmt.Sprintf("http://localhost:%d/metrics", o.config.MetricsPort))
	}

	return nil
}

// initializeTracing sets up OpenTelemetry tracing
func (o *ObservabilityManager) initializeTracing(ctx context.Context) error {
	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(o.config.ServiceName),
			semconv.ServiceVersion(o.config.ServiceVersion),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace exporter based on configuration
	var exporter sdktrace.SpanExporter

	switch o.config.TraceExporter {
	case "stdout":
		// Stdout exporter for development
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return fmt.Errorf("failed to create stdout exporter: %w", err)
		}

	case "jaeger":
		// TODO: Implement Jaeger exporter
		// exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(...))
		slog.Warn("Jaeger exporter not yet implemented, falling back to stdout")
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return fmt.Errorf("failed to create stdout exporter: %w", err)
		}

	case "otlp":
		// TODO: Implement OTLP exporter for production
		// exporter, err = otlptrace.New(ctx, otlptracegrpc.NewClient(...))
		slog.Warn("OTLP exporter not yet implemented, falling back to stdout")
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return fmt.Errorf("failed to create stdout exporter: %w", err)
		}

	default:
		// Default to stdout
		slog.Warn("unknown trace exporter, falling back to stdout",
			"exporter", o.config.TraceExporter)
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return fmt.Errorf("failed to create stdout exporter: %w", err)
		}
	}

	// Create tracer provider
	o.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // TODO: Make configurable
	)

	// Register as global tracer provider
	otel.SetTracerProvider(o.tracerProvider)

	return nil
}

// GetTracer returns a tracer for the given name
func (o *ObservabilityManager) GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// startMetricsServer starts the HTTP server for Prometheus metrics
func (o *ObservabilityManager) startMetricsServer() error {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Readiness check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	// Metrics endpoint (Prometheus format)
	// TODO: Integrate Prometheus client library
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)

		// Stub metrics in Prometheus format
		fmt.Fprintf(w, "# HELP backend_driver_info Backend driver information\n")
		fmt.Fprintf(w, "# TYPE backend_driver_info gauge\n")
		fmt.Fprintf(w, "backend_driver_info{name=\"%s\",version=\"%s\"} 1\n",
			o.config.ServiceName, o.config.ServiceVersion)

		fmt.Fprintf(w, "# HELP backend_driver_uptime_seconds Backend driver uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE backend_driver_uptime_seconds counter\n")
		fmt.Fprintf(w, "backend_driver_uptime_seconds %.2f\n", time.Since(time.Now()).Seconds())

		// TODO: Add real metrics:
		// - backend_driver_requests_total
		// - backend_driver_request_duration_seconds
		// - backend_driver_errors_total
		// - backend_driver_connections_active
	})

	// Create HTTP server
	o.metricsServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", o.config.MetricsPort),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Start server in background
	go func() {
		slog.Info("metrics server listening", "port", o.config.MetricsPort)
		if err := o.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down observability components
func (o *ObservabilityManager) Shutdown(ctx context.Context) error {
	var shutdownErr error

	o.shutdownOnce.Do(func() {
		slog.Info("shutting down observability components")

		// Shutdown metrics server
		if o.metricsServer != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			if err := o.metricsServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("failed to shutdown metrics server", "error", err)
				shutdownErr = fmt.Errorf("metrics server shutdown: %w", err)
			} else {
				slog.Info("metrics server shut down successfully")
			}
		}

		// Shutdown tracer provider
		if o.tracerProvider != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			if err := o.tracerProvider.Shutdown(shutdownCtx); err != nil {
				slog.Error("failed to shutdown tracer provider", "error", err)
				if shutdownErr == nil {
					shutdownErr = fmt.Errorf("tracer provider shutdown: %w", err)
				}
			} else {
				slog.Info("tracer provider shut down successfully")
			}
		}
	})

	return shutdownErr
}

// DefaultObservabilityConfig creates a default observability configuration
func DefaultObservabilityConfig(serviceName, serviceVersion string) *ObservabilityConfig {
	return &ObservabilityConfig{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		MetricsPort:    0,        // Disabled by default
		EnableTracing:  false,    // Disabled by default
		TraceExporter:  "stdout", // Development mode
	}
}
