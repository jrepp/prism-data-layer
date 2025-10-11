package cmd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prism/patterns/multicast_registry"
	"github.com/spf13/cobra"
	"golang.org/x/time/rate"
)

var (
	multicastFilter   bool
	multicastStatus   string
	multicastPayload  string
)

var multicastCmd = &cobra.Command{
	Use:   "multicast",
	Short: "Load test multicast operations",
	Long: `Load test the Multicast operation by continuously broadcasting messages
to identities matching filter expressions at a specified rate.

This command requires pre-existing identities. Run the register command first
to populate the registry, or use the mixed command for combined testing.

Example:
  # Multicast to all identities
  prism-loadtest multicast -r 50 -d 60s --payload '{"type":"ping"}'

  # Multicast to filtered subset
  prism-loadtest multicast -r 50 -d 60s --filter --status online --payload '{"type":"update"}'
`,
	RunE: runMulticast,
}

func init() {
	rootCmd.AddCommand(multicastCmd)
	multicastCmd.Flags().BoolVar(&multicastFilter, "filter", false, "Use filter expression")
	multicastCmd.Flags().StringVar(&multicastStatus, "status", "online", "Status to filter by (requires --filter)")
	multicastCmd.Flags().StringVar(&multicastPayload, "payload", `{"type":"loadtest","timestamp":0}`, "Message payload (JSON)")
}

func runMulticast(cmd *cobra.Command, args []string) error {
	filterDesc := "none (multicast to all)"
	if multicastFilter {
		filterDesc = fmt.Sprintf("status=%s", multicastStatus)
	}

	fmt.Printf("Starting Multicast load test...\n")
	fmt.Printf("  Rate: %d req/sec\n", rateLimit)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Filter: %s\n", filterDesc)
	fmt.Printf("  Payload: %s\n", multicastPayload)
	fmt.Println()

	// Create coordinator
	coordinator, err := setupCoordinator()
	if err != nil {
		return fmt.Errorf("failed to setup coordinator: %w", err)
	}
	defer coordinator.Close()

	// Create filter if requested
	var filter *multicast_registry.Filter
	if multicastFilter {
		filter = multicast_registry.NewFilter(map[string]interface{}{
			"status": multicastStatus,
		})
	}

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)

	// Create metrics collector
	metrics := NewMetricsCollector()

	// Track delivery stats
	var totalTargets atomic.Int64
	var totalDelivered atomic.Int64
	var totalFailed atomic.Int64

	// Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Start progress reporter
	stopReporter := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reportProgressWithDelivery(ctx, metrics, &totalTargets, &totalDelivered, &totalFailed, stopReporter)
	}()

	// Load test loop
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

			// Generate payload with timestamp
			payload := []byte(fmt.Sprintf(`{"type":"loadtest","timestamp":%d}`, time.Now().Unix()))

			// Measure latency
			start := time.Now()
			response, err := coordinator.Multicast(ctx, filter, payload)
			latency := time.Since(start)

			if err != nil {
				if verbose {
					fmt.Printf("Multicast failed: %v\n", err)
				}
				metrics.RecordFailure()
			} else {
				metrics.RecordSuccess(latency)
				totalTargets.Add(int64(response.TargetCount))
				totalDelivered.Add(int64(response.DeliveredCount))
				totalFailed.Add(int64(response.FailedCount))
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
	report := metrics.Report()

	avgTargets := float64(0)
	if metrics.SuccessfulReqs > 0 {
		avgTargets = float64(totalTargets.Load()) / float64(metrics.SuccessfulReqs)
	}

	deliveryRate := float64(0)
	if totalTargets.Load() > 0 {
		deliveryRate = float64(totalDelivered.Load()) / float64(totalTargets.Load()) * 100
	}

	report += fmt.Sprintf("\nMulticast Statistics:\n")
	report += fmt.Sprintf("  Total Targets:   %d\n", totalTargets.Load())
	report += fmt.Sprintf("  Delivered:       %d (%.2f%%)\n", totalDelivered.Load(), deliveryRate)
	report += fmt.Sprintf("  Failed:          %d\n", totalFailed.Load())
	report += fmt.Sprintf("  Avg Per Multicast: %.2f\n", avgTargets)

	fmt.Println(report)

	return nil
}

func reportProgressWithDelivery(ctx context.Context, metrics *MetricsCollector, totalTargets, totalDelivered, totalFailed *atomic.Int64, stop <-chan struct{}) {
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

			avgTargets := float64(0)
			if metrics.SuccessfulReqs > 0 {
				avgTargets = float64(totalTargets.Load()) / float64(metrics.SuccessfulReqs)
			}

			deliveryRate := float64(0)
			if totalTargets.Load() > 0 {
				deliveryRate = float64(totalDelivered.Load()) / float64(totalTargets.Load()) * 100
			}

			fmt.Printf("[%v] Multicasts: %d, Success: %.1f%%, Throughput: %.2f req/sec, Avg Targets: %.1f, Delivery: %.1f%%\n",
				elapsed.Round(time.Second),
				metrics.TotalRequests,
				successRate,
				throughput,
				avgTargets,
				deliveryRate,
			)
		}
	}
}
