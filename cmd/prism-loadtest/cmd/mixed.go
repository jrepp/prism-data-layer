package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prism/patterns/multicast_registry"
	"github.com/spf13/cobra"
	"golang.org/x/time/rate"
)

var (
	mixedRegisterPct  int
	mixedEnumeratePct int
	mixedMulticastPct int
)

var mixedCmd = &cobra.Command{
	Use:   "mixed",
	Short: "Run mixed workload with all operations",
	Long: `Run a mixed workload load test that combines register, enumerate, and multicast
operations with configurable percentages.

The percentages determine the probability distribution of operations:
  --register-pct:  Percentage of register operations (default 50%)
  --enumerate-pct: Percentage of enumerate operations (default 30%)
  --multicast-pct: Percentage of multicast operations (default 20%)

Total percentages should add up to 100.

Example:
  # Default mix (50% register, 30% enumerate, 20% multicast)
  prism-loadtest mixed -r 100 -d 120s

  # Custom mix (60% register, 30% enumerate, 10% multicast)
  prism-loadtest mixed -r 100 -d 120s --register-pct 60 --enumerate-pct 30 --multicast-pct 10
`,
	RunE: runMixed,
}

func init() {
	rootCmd.AddCommand(mixedCmd)
	mixedCmd.Flags().IntVar(&mixedRegisterPct, "register-pct", 50, "Percentage of register operations")
	mixedCmd.Flags().IntVar(&mixedEnumeratePct, "enumerate-pct", 30, "Percentage of enumerate operations")
	mixedCmd.Flags().IntVar(&mixedMulticastPct, "multicast-pct", 20, "Percentage of multicast operations")
}

func runMixed(cmd *cobra.Command, args []string) error {
	// Validate percentages
	totalPct := mixedRegisterPct + mixedEnumeratePct + mixedMulticastPct
	if totalPct != 100 {
		return fmt.Errorf("percentages must add up to 100, got %d", totalPct)
	}

	fmt.Printf("Starting Mixed Workload load test...\n")
	fmt.Printf("  Rate: %d req/sec\n", rateLimit)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Mix: %d%% register, %d%% enumerate, %d%% multicast\n",
		mixedRegisterPct, mixedEnumeratePct, mixedMulticastPct)
	fmt.Println()

	// Create coordinator
	coordinator, err := setupCoordinator()
	if err != nil {
		return fmt.Errorf("failed to setup coordinator: %w", err)
	}
	defer coordinator.Close()

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)

	// Create metrics collectors for each operation type
	registerMetrics := NewMetricsCollector()
	enumerateMetrics := NewMetricsCollector()
	multicastMetrics := NewMetricsCollector()

	// Track operation counts
	var registerCount atomic.Int64
	var enumerateCount atomic.Int64
	var multicastCount atomic.Int64
	var identityCounter atomic.Int64

	// Track multicast delivery stats
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
		reportMixedProgress(ctx, registerMetrics, enumerateMetrics, multicastMetrics, &registerCount, &enumerateCount, &multicastCount, stopReporter)
	}()

	// Load test loop
	var workerWg sync.WaitGroup
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

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

		// Select operation based on percentages
		roll := rng.Intn(100)
		var operation string

		if roll < mixedRegisterPct {
			operation = "register"
		} else if roll < mixedRegisterPct+mixedEnumeratePct {
			operation = "enumerate"
		} else {
			operation = "multicast"
		}

		// Launch worker goroutine
		workerWg.Add(1)
		go func(op string) {
			defer workerWg.Done()

			switch op {
			case "register":
				registerCount.Add(1)
				executeRegister(ctx, coordinator, &identityCounter, registerMetrics)

			case "enumerate":
				enumerateCount.Add(1)
				executeEnumerate(ctx, coordinator, enumerateMetrics)

			case "multicast":
				multicastCount.Add(1)
				executeMulticast(ctx, coordinator, multicastMetrics, &totalTargets, &totalDelivered, &totalFailed)
			}
		}(operation)
	}

cleanup:
	elapsed := time.Since(startTime)
	fmt.Printf("\nWaiting for workers to finish (%v elapsed)...\n", elapsed.Round(time.Second))
	workerWg.Wait()

	// Stop progress reporter
	close(stopReporter)
	wg.Wait()

	// Print final report
	printMixedReport(registerMetrics, enumerateMetrics, multicastMetrics,
		registerCount.Load(), enumerateCount.Load(), multicastCount.Load(),
		totalTargets.Load(), totalDelivered.Load(), totalFailed.Load())

	return nil
}

func executeRegister(ctx context.Context, coordinator *multicast_registry.Coordinator, identityCounter *atomic.Int64, metrics *MetricsCollector) {
	// Generate unique identity
	idNum := identityCounter.Add(1)
	identity := fmt.Sprintf("loadtest-user-%d", idNum)

	// Generate metadata
	metadata := map[string]interface{}{
		"status":    "online",
		"loadtest":  true,
		"timestamp": time.Now().Unix(),
		"worker_id": idNum % 100,
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
}

func executeEnumerate(ctx context.Context, coordinator *multicast_registry.Coordinator, metrics *MetricsCollector) {
	// Use simple filter for online status
	filter := multicast_registry.NewFilter(map[string]interface{}{
		"status": "online",
	})

	// Measure latency
	start := time.Now()
	_, err := coordinator.Enumerate(ctx, filter)
	latency := time.Since(start)

	if err != nil {
		if verbose {
			fmt.Printf("Enumerate failed: %v\n", err)
		}
		metrics.RecordFailure()
	} else {
		metrics.RecordSuccess(latency)
	}
}

func executeMulticast(ctx context.Context, coordinator *multicast_registry.Coordinator, metrics *MetricsCollector, totalTargets, totalDelivered, totalFailed *atomic.Int64) {
	// Use filter for online status
	filter := multicast_registry.NewFilter(map[string]interface{}{
		"status": "online",
	})

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
}

func reportMixedProgress(ctx context.Context, registerMetrics, enumerateMetrics, multicastMetrics *MetricsCollector, registerCount, enumerateCount, multicastCount *atomic.Int64, stop <-chan struct{}) {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			elapsed := time.Since(registerMetrics.StartTime)
			totalRequests := registerCount.Load() + enumerateCount.Load() + multicastCount.Load()
			throughput := float64(totalRequests) / elapsed.Seconds()

			fmt.Printf("[%v] Total: %d (%.2f req/sec) | Register: %d, Enumerate: %d, Multicast: %d\n",
				elapsed.Round(time.Second),
				totalRequests,
				throughput,
				registerCount.Load(),
				enumerateCount.Load(),
				multicastCount.Load(),
			)
		}
	}
}

func printMixedReport(registerMetrics, enumerateMetrics, multicastMetrics *MetricsCollector, registerCount, enumerateCount, multicastCount int64, totalTargets, totalDelivered, totalFailed int64) {
	totalRequests := registerCount + enumerateCount + multicastCount

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Mixed Workload Load Test Results")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nOverall:\n")
	fmt.Printf("  Total Operations: %d\n", totalRequests)
	fmt.Printf("  Register:         %d (%.1f%%)\n", registerCount, float64(registerCount)/float64(totalRequests)*100)
	fmt.Printf("  Enumerate:        %d (%.1f%%)\n", enumerateCount, float64(enumerateCount)/float64(totalRequests)*100)
	fmt.Printf("  Multicast:        %d (%.1f%%)\n", multicastCount, float64(multicastCount)/float64(totalRequests)*100)

	if registerCount > 0 {
		fmt.Println("\nRegister Operations:")
		printOperationStats(registerMetrics)
	}

	if enumerateCount > 0 {
		fmt.Println("\nEnumerate Operations:")
		printOperationStats(enumerateMetrics)
	}

	if multicastCount > 0 {
		fmt.Println("\nMulticast Operations:")
		printOperationStats(multicastMetrics)
		deliveryRate := float64(0)
		if totalTargets > 0 {
			deliveryRate = float64(totalDelivered) / float64(totalTargets) * 100
		}
		fmt.Printf("  Total Targets:    %d\n", totalTargets)
		fmt.Printf("  Delivered:        %d (%.2f%%)\n", totalDelivered, deliveryRate)
		fmt.Printf("  Failed:           %d\n", totalFailed)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
}

func printOperationStats(metrics *MetricsCollector) {
	successRate := float64(100)
	if metrics.TotalRequests > 0 {
		successRate = float64(metrics.SuccessfulReqs) / float64(metrics.TotalRequests) * 100
	}

	avgLatency := time.Duration(0)
	if metrics.SuccessfulReqs > 0 {
		avgLatency = time.Duration(metrics.TotalLatencyNs / metrics.SuccessfulReqs)
	}

	p50, p95, p99 := metrics.calculatePercentiles()

	fmt.Printf("  Total Requests:   %d\n", metrics.TotalRequests)
	fmt.Printf("  Successful:       %d (%.2f%%)\n", metrics.SuccessfulReqs, successRate)
	fmt.Printf("  Failed:           %d\n", metrics.FailedReqs)
	fmt.Printf("  Latency Min:      %v\n", time.Duration(metrics.MinLatencyNs).Round(time.Microsecond))
	fmt.Printf("  Latency Max:      %v\n", time.Duration(metrics.MaxLatencyNs).Round(time.Microsecond))
	fmt.Printf("  Latency Avg:      %v\n", avgLatency.Round(time.Microsecond))
	fmt.Printf("  Latency P50:      %v\n", p50.Round(time.Microsecond))
	fmt.Printf("  Latency P95:      %v\n", p95.Round(time.Microsecond))
	fmt.Printf("  Latency P99:      %v\n", p99.Round(time.Microsecond))
}
