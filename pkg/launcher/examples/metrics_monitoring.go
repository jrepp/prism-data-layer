package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Example: Metrics and monitoring
//
// This example demonstrates how to:
// 1. Launch patterns and generate metrics
// 2. Monitor process health
// 3. Export metrics in Prometheus and JSON formats
// 4. Track launch latency and process lifecycle

func main() {
	// Connect to launcher
	conn, err := grpc.Dial("localhost:8080",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewPatternLauncherClient(conn)
	ctx := context.Background()

	fmt.Println("=== Metrics and Monitoring Demo ===\n")

	// 1. Get initial health status
	fmt.Println("1. Initial Health Status")
	printHealth(client, ctx)
	fmt.Println()

	// 2. Launch several patterns to generate metrics
	fmt.Println("2. Launching patterns...")

	patterns := []struct {
		name      string
		isolation pb.IsolationLevel
		namespace string
		session   string
	}{
		{"consumer", pb.IsolationLevel_ISOLATION_NAMESPACE, "tenant-a", ""},
		{"consumer", pb.IsolationLevel_ISOLATION_NAMESPACE, "tenant-b", ""},
		{"producer", pb.IsolationLevel_ISOLATION_SESSION, "tenant-a", "user-1"},
		{"producer", pb.IsolationLevel_ISOLATION_SESSION, "tenant-a", "user-2"},
		{"schema-registry", pb.IsolationLevel_ISOLATION_NONE, "", ""},
	}

	var processIDs []string
	for _, p := range patterns {
		fmt.Printf("   Launching %s (isolation=%s, namespace=%s, session=%s)...\n",
			p.name, p.isolation, p.namespace, p.session)

		start := time.Now()
		resp, err := client.LaunchPattern(ctx, &pb.LaunchRequest{
			PatternName: p.name,
			Isolation:   p.isolation,
			Namespace:   p.namespace,
			SessionId:   p.session,
		})
		duration := time.Since(start)

		if err != nil {
			log.Printf("   ✗ Failed: %v\n", err)
			continue
		}

		processIDs = append(processIDs, resp.ProcessId)
		fmt.Printf("   ✓ Launched in %v (process_id=%s)\n", duration, resp.ProcessId)
	}
	fmt.Println()

	// 3. Wait for processes to stabilize
	fmt.Println("3. Waiting for processes to stabilize...")
	time.Sleep(3 * time.Second)
	fmt.Println()

	// 4. Check health again with process details
	fmt.Println("4. Health Status After Launch")
	printHealthWithProcesses(client, ctx)
	fmt.Println()

	// 5. Get specific process status
	if len(processIDs) > 0 {
		fmt.Println("5. Individual Process Status")
		processID := processIDs[0]
		statusResp, err := client.GetProcessStatus(ctx, &pb.GetProcessStatusRequest{
			ProcessId: processID,
		})
		if err != nil {
			log.Printf("   Failed to get status: %v\n", err)
		} else if !statusResp.NotFound {
			process := statusResp.Process
			fmt.Printf("   Process: %s\n", process.ProcessId)
			fmt.Printf("     Pattern: %s\n", process.PatternName)
			fmt.Printf("     Namespace: %s\n", process.Namespace)
			fmt.Printf("     PID: %d\n", process.Pid)
			fmt.Printf("     Address: %s\n", process.Address)
			fmt.Printf("     State: %s\n", process.State)
			fmt.Printf("     Healthy: %v\n", process.Healthy)
			fmt.Printf("     Uptime: %ds\n", process.UptimeSeconds)
		}
		fmt.Println()
	}

	// 6. Monitor for a while
	fmt.Println("6. Monitoring for 10 seconds...")
	ticker := time.NewTicker(2 * time.Second)
	done := time.After(10 * time.Second)

	for {
		select {
		case <-done:
			ticker.Stop()
			goto cleanup
		case <-ticker.C:
			healthResp, _ := client.Health(ctx, &pb.HealthRequest{})
			fmt.Printf("   [%s] Running: %d, Failed: %d, Uptime: %ds\n",
				time.Now().Format("15:04:05"),
				healthResp.RunningProcesses,
				healthResp.FailedProcesses,
				healthResp.UptimeSeconds)
		}
	}

cleanup:
	fmt.Println()

	// 7. Terminate all processes
	fmt.Println("7. Terminating all processes...")
	for _, processID := range processIDs {
		_, err := client.TerminatePattern(ctx, &pb.TerminateRequest{
			ProcessId:       processID,
			GracePeriodSecs: 5,
		})
		if err != nil {
			log.Printf("   ✗ Failed to terminate %s: %v\n", processID, err)
		} else {
			fmt.Printf("   ✓ Terminated %s\n", processID)
		}
	}
	fmt.Println()

	// 8. Final health check
	fmt.Println("8. Final Health Status")
	printHealth(client, ctx)

	fmt.Println("\n✓ Metrics monitoring demo completed!")
}

func printHealth(client pb.PatternLauncherClient, ctx context.Context) {
	resp, err := client.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		log.Printf("Failed to get health: %v\n", err)
		return
	}

	fmt.Printf("   Healthy: %v\n", resp.Healthy)
	fmt.Printf("   Total processes: %d\n", resp.TotalProcesses)
	fmt.Printf("   Running: %d\n", resp.RunningProcesses)
	fmt.Printf("   Terminating: %d\n", resp.TerminatingProcesses)
	fmt.Printf("   Failed: %d\n", resp.FailedProcesses)
	fmt.Printf("   Uptime: %ds\n", resp.UptimeSeconds)

	if len(resp.IsolationDistribution) > 0 {
		fmt.Println("   Isolation distribution:")
		for level, count := range resp.IsolationDistribution {
			fmt.Printf("     %s: %d\n", level, count)
		}
	}
}

func printHealthWithProcesses(client pb.PatternLauncherClient, ctx context.Context) {
	resp, err := client.Health(ctx, &pb.HealthRequest{
		IncludeProcesses: true,
	})
	if err != nil {
		log.Printf("Failed to get health: %v\n", err)
		return
	}

	printHealth(client, ctx)

	if len(resp.Processes) > 0 {
		fmt.Println("\n   Running processes:")
		for _, p := range resp.Processes {
			fmt.Printf("     - %s (%s): %s [ns=%s, uptime=%ds]\n",
				p.PatternName,
				p.ProcessId,
				p.State,
				p.Namespace,
				p.UptimeSeconds)
		}
	}
}
