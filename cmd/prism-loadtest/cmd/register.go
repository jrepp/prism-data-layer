package cmd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prism/patterns/multicast_registry"
	"github.com/prism/patterns/multicast_registry/backends"
	"github.com/spf13/cobra"
	"golang.org/x/time/rate"
)

var (
	registerTTL int // TTL in seconds
)

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Load test identity registration",
	Long: `Load test the Register operation by continuously registering identities
with metadata and optional TTL at a specified rate.

Example:
  prism-loadtest register -r 100 -d 60s --ttl 300
`,
	RunE: runRegister,
}

func init() {
	rootCmd.AddCommand(registerCmd)
	registerCmd.Flags().IntVar(&registerTTL, "ttl", 300, "TTL for registered identities (seconds)")
}

func runRegister(cmd *cobra.Command, args []string) error {
	fmt.Printf("Starting Register load test...\n")
	fmt.Printf("  Rate: %d req/sec\n", rateLimit)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  TTL: %ds\n", registerTTL)
	fmt.Println()

	// Create coordinator
	coordinator, err := setupCoordinator()
	if err != nil {
		return fmt.Errorf("failed to setup coordinator: %w", err)
	}
	defer coordinator.Close()

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)

	// Create metrics collector
	metrics := NewMetricsCollector()

	// Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Start progress reporter
	stopReporter := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reportProgress(ctx, metrics, stopReporter)
	}()

	// Load test loop
	var identityCounter atomic.Int64
	var workerWg sync.WaitGroup

	fmt.Println("Load test running...")
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			goto cleanup
		default:
		}

		// Rate limit
		if err := limiter.Wait(ctx); err != nil {
			goto cleanup
		}

		// Launch worker goroutine
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()

			// Generate unique identity
			idNum := identityCounter.Add(1)
			identity := fmt.Sprintf("loadtest-user-%d", idNum)

			// Generate metadata
			metadata := map[string]interface{}{
				"status":    "online",
				"loadtest":  true,
				"timestamp": time.Now().Unix(),
				"worker_id": idNum % 100, // Simulate 100 workers
			}

			// Measure latency
			start := time.Now()
			err := coordinator.Register(ctx, identity, metadata, time.Duration(registerTTL)*time.Second)
			latency := time.Since(start)

			if err != nil {
				if verbose {
					fmt.Printf("Register failed: %v\n", err)
				}
				metrics.RecordFailure()
			} else {
				metrics.RecordSuccess(latency)
			}
		}()
	}

cleanup:
	elapsed := time.Since(startTime)
	fmt.Printf("\nWaiting for workers to finish (%v elapsed)...\n", elapsed.Round(time.Second))
	workerWg.Wait()

	// Stop progress reporter
	close(stopReporter)
	wg.Wait()

	// Print final report
	fmt.Println(metrics.Report())

	return nil
}

func setupCoordinator() (*multicast_registry.Coordinator, error) {
	// Create config
	config := multicast_registry.DefaultConfig()
	config.DefaultTTL = time.Duration(registerTTL) * time.Second
	config.MaxIdentities = 1000000 // Allow large number for load testing

	// Create Redis registry backend
	registryBackend, err := backends.NewRedisRegistryBackend(
		redisAddr,
		redisPassword,
		redisDB,
		"loadtest:",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis backend: %w", err)
	}

	// Create NATS messaging backend
	messagingBackend, err := backends.NewNATSMessagingBackend(natsServers)
	if err != nil {
		registryBackend.Close()
		return nil, fmt.Errorf("failed to create NATS backend: %w", err)
	}

	// Create coordinator
	coordinator, err := multicast_registry.NewCoordinator(config, registryBackend, messagingBackend, nil)
	if err != nil {
		registryBackend.Close()
		messagingBackend.Close()
		return nil, fmt.Errorf("failed to create coordinator: %w", err)
	}

	return coordinator, nil
}

func reportProgress(ctx context.Context, metrics *MetricsCollector, stop <-chan struct{}) {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			elapsed := time.Since(metrics.StartTime)
			throughput := float64(metrics.TotalRequests) / elapsed.Seconds()
			successRate := float64(100)
			if metrics.TotalRequests > 0 {
				successRate = float64(metrics.SuccessfulReqs) / float64(metrics.TotalRequests) * 100
			}

			fmt.Printf("[%v] Requests: %d, Success: %.1f%%, Throughput: %.2f req/sec\n",
				elapsed.Round(time.Second),
				metrics.TotalRequests,
				successRate,
				throughput,
			)
		}
	}
}
