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
	enumerateFilter bool
	enumerateStatus string
)

var enumerateCmd = &cobra.Command{
	Use:   "enumerate",
	Short: "Load test enumerate operations",
	Long: `Load test the Enumerate operation by continuously querying identities
with optional filter expressions at a specified rate.

This command requires pre-existing identities. Run the register command first
to populate the registry, or use the mixed command for combined testing.

Example:
  # Without filter (enumerate all)
  prism-loadtest enumerate -r 100 -d 60s

  # With filter (enumerate online users)
  prism-loadtest enumerate -r 100 -d 60s --filter --status online
`,
	RunE: runEnumerate,
}

func init() {
	rootCmd.AddCommand(enumerateCmd)
	enumerateCmd.Flags().BoolVar(&enumerateFilter, "filter", false, "Use filter expression")
	enumerateCmd.Flags().StringVar(&enumerateStatus, "status", "online", "Status to filter by (requires --filter)")
}

func runEnumerate(cmd *cobra.Command, args []string) error {
	filterDesc := "none (enumerate all)"
	if enumerateFilter {
		filterDesc = fmt.Sprintf("status=%s", enumerateStatus)
	}

	fmt.Printf("Starting Enumerate load test...\n")
	fmt.Printf("  Rate: %d req/sec\n", rateLimit)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Filter: %s\n", filterDesc)
	fmt.Println()

	// Create coordinator
	coordinator, err := setupCoordinator()
	if err != nil {
		return fmt.Errorf("failed to setup coordinator: %w", err)
	}
	defer coordinator.Close()

	// Create filter if requested
	var filter *multicast_registry.Filter
	if enumerateFilter {
		filter = multicast_registry.NewFilter(map[string]interface{}{
			"status": enumerateStatus,
		})
	}

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)

	// Create metrics collector
	metrics := NewMetricsCollector()

	// Track result counts
	var totalResults atomic.Int64

	// Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Start progress reporter
	stopReporter := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reportProgressWithResults(ctx, metrics, &totalResults, stopReporter)
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

			// Measure latency
			start := time.Now()
			identities, err := coordinator.Enumerate(ctx, filter)
			latency := time.Since(start)

			if err != nil {
				if verbose {
					fmt.Printf("Enumerate failed: %v\n", err)
				}
				metrics.RecordFailure()
			} else {
				metrics.RecordSuccess(latency)
				totalResults.Add(int64(len(identities)))
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
	avgResults := float64(0)
	if metrics.SuccessfulReqs > 0 {
		avgResults = float64(totalResults.Load()) / float64(metrics.SuccessfulReqs)
	}
	report += fmt.Sprintf("\nEnumerate Statistics:\n")
	report += fmt.Sprintf("  Total Results:   %d\n", totalResults.Load())
	report += fmt.Sprintf("  Avg Per Query:   %.2f\n", avgResults)

	fmt.Println(report)

	return nil
}

func reportProgressWithResults(ctx context.Context, metrics *MetricsCollector, totalResults *atomic.Int64, stop <-chan struct{}) {
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

			avgResults := float64(0)
			if metrics.SuccessfulReqs > 0 {
				avgResults = float64(totalResults.Load()) / float64(metrics.SuccessfulReqs)
			}

			fmt.Printf("[%v] Requests: %d, Success: %.1f%%, Throughput: %.2f req/sec, Avg Results: %.1f\n",
				elapsed.Round(time.Second),
				metrics.TotalRequests,
				successRate,
				throughput,
				avgResults,
			)
		}
	}
}
