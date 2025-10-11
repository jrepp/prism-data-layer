package cmd

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	coordinatorAddr string
	duration        time.Duration
	rateLimit       int
	verbose         bool
	reportInterval  time.Duration

	// Redis backend flags
	redisAddr     string
	redisPassword string
	redisDB       int

	// NATS backend flags
	natsServers []string
)

var rootCmd = &cobra.Command{
	Use:   "prism-loadtest",
	Short: "Load testing tool for Prism multicast registry pattern",
	Long: `prism-loadtest is a comprehensive load testing tool for the Prism multicast registry pattern.
It supports multiple workload patterns including:
  - register: Load test identity registration
  - enumerate: Load test enumeration queries
  - multicast: Load test multicast operations
  - mixed: Combined load test of all operations

The tool supports configurable rate limiting, duration, and backend configuration.`,
	Version: "1.0.0",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global persistent flags
	rootCmd.PersistentFlags().StringVar(&coordinatorAddr, "coordinator", "", "Coordinator address (if using remote proxy)")
	rootCmd.PersistentFlags().DurationVarP(&duration, "duration", "d", 60*time.Second, "Test duration")
	rootCmd.PersistentFlags().IntVarP(&rateLimit, "rate", "r", 100, "Request rate (req/sec)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().DurationVar(&reportInterval, "report-interval", 5*time.Second, "Progress report interval")

	// Redis backend flags
	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis-addr", "localhost:6379", "Redis address")
	rootCmd.PersistentFlags().StringVar(&redisPassword, "redis-password", "", "Redis password")
	rootCmd.PersistentFlags().IntVar(&redisDB, "redis-db", 0, "Redis database")

	// NATS backend flags
	rootCmd.PersistentFlags().StringSliceVar(&natsServers, "nats-servers", []string{"nats://localhost:4222"}, "NATS server URLs")
}

// MetricsCollector tracks performance metrics
type MetricsCollector struct {
	TotalRequests   int64
	SuccessfulReqs  int64
	FailedReqs      int64
	TotalLatencyNs  int64
	MinLatencyNs    int64
	MaxLatencyNs    int64
	StartTime       time.Time

	// Latency histogram buckets (in microseconds)
	LatencyBuckets map[int]int64 // bucket_us -> count

	// Mutex for thread-safe operations
	mu sync.Mutex
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		StartTime:      time.Now(),
		MinLatencyNs:   1<<63 - 1, // max int64
		LatencyBuckets: make(map[int]int64),
	}
}

func (m *MetricsCollector) RecordSuccess(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests++
	m.SuccessfulReqs++
	latencyNs := latency.Nanoseconds()
	m.TotalLatencyNs += latencyNs

	if latencyNs < m.MinLatencyNs {
		m.MinLatencyNs = latencyNs
	}
	if latencyNs > m.MaxLatencyNs {
		m.MaxLatencyNs = latencyNs
	}

	// Record in histogram buckets
	latencyUs := latency.Microseconds()
	bucket := getLatencyBucket(latencyUs)
	m.LatencyBuckets[bucket]++
}

func (m *MetricsCollector) RecordFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests++
	m.FailedReqs++
}

func (m *MetricsCollector) Report() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	elapsed := time.Since(m.StartTime)
	throughput := float64(m.TotalRequests) / elapsed.Seconds()
	successRate := float64(m.SuccessfulReqs) / float64(m.TotalRequests) * 100

	avgLatency := time.Duration(0)
	if m.SuccessfulReqs > 0 {
		avgLatency = time.Duration(m.TotalLatencyNs / m.SuccessfulReqs)
	}

	report := fmt.Sprintf(`
Load Test Results
-----------------
Duration:        %v
Total Requests:  %d
Successful:      %d (%.2f%%)
Failed:          %d
Throughput:      %.2f req/sec
Latency:
  Min:           %v
  Max:           %v
  Avg:           %v
`,
		elapsed.Round(time.Millisecond),
		m.TotalRequests,
		m.SuccessfulReqs,
		successRate,
		m.FailedReqs,
		throughput,
		time.Duration(m.MinLatencyNs).Round(time.Microsecond),
		time.Duration(m.MaxLatencyNs).Round(time.Microsecond),
		avgLatency.Round(time.Microsecond),
	)

	// Add percentile calculations
	if m.SuccessfulReqs > 0 {
		p50, p95, p99 := m.calculatePercentiles()
		report += fmt.Sprintf("  P50:           %v\n", p50.Round(time.Microsecond))
		report += fmt.Sprintf("  P95:           %v\n", p95.Round(time.Microsecond))
		report += fmt.Sprintf("  P99:           %v\n", p99.Round(time.Microsecond))
	}

	return report
}

func (m *MetricsCollector) calculatePercentiles() (p50, p95, p99 time.Duration) {
	// Note: caller must hold m.mu lock

	// Sort buckets and calculate percentiles
	var buckets []int
	for bucket := range m.LatencyBuckets {
		buckets = append(buckets, bucket)
	}

	// Simple percentile calculation (approximate from histogram)
	totalCount := m.SuccessfulReqs
	var cumulative int64

	p50Threshold := totalCount * 50 / 100
	p95Threshold := totalCount * 95 / 100
	p99Threshold := totalCount * 99 / 100

	for bucket := 0; bucket <= 100000; bucket = nextBucket(bucket) {
		count := m.LatencyBuckets[bucket]
		if count == 0 {
			continue
		}
		cumulative += count

		bucketDuration := time.Duration(bucket) * time.Microsecond

		if p50 == 0 && cumulative >= p50Threshold {
			p50 = bucketDuration
		}
		if p95 == 0 && cumulative >= p95Threshold {
			p95 = bucketDuration
		}
		if p99 == 0 && cumulative >= p99Threshold {
			p99 = bucketDuration
			break
		}
	}

	return p50, p95, p99
}

func getLatencyBucket(latencyUs int64) int {
	// Exponential buckets: 0-10us, 10-50us, 50-100us, 100-500us, 500-1000us, 1-5ms, 5-10ms, 10-50ms, 50-100ms, 100+ms
	switch {
	case latencyUs < 10:
		return 10
	case latencyUs < 50:
		return 50
	case latencyUs < 100:
		return 100
	case latencyUs < 500:
		return 500
	case latencyUs < 1000:
		return 1000
	case latencyUs < 5000:
		return 5000
	case latencyUs < 10000:
		return 10000
	case latencyUs < 50000:
		return 50000
	case latencyUs < 100000:
		return 100000
	default:
		return 100000 // 100ms+ bucket
	}
}

func nextBucket(current int) int {
	buckets := []int{10, 50, 100, 500, 1000, 5000, 10000, 50000, 100000}
	for _, b := range buckets {
		if b > current {
			return b
		}
	}
	return 100001 // sentinel
}
