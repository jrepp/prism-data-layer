package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/isolation"
	"github.com/jrepp/prism-data-layer/pkg/launcher"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Example: Basic pattern launch
//
// This example demonstrates how to:
// 1. Connect to the pattern launcher gRPC service
// 2. Launch a pattern with namespace isolation
// 3. Check the health of launched patterns
// 4. Gracefully terminate patterns

func main() {
	// Connect to launcher service
	conn, err := grpc.Dial("localhost:8080",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to launcher: %v", err)
	}
	defer conn.Close()

	client := pb.NewPatternLauncherClient(conn)
	ctx := context.Background()

	// 1. Launch a consumer pattern for tenant-a
	fmt.Println("Launching consumer pattern for tenant-a...")
	launchResp, err := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "consumer",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "tenant-a",
		Config: map[string]string{
			"kafka_brokers": "localhost:9092",
			"consumer_group": "tenant-a-consumers",
			"topic": "events",
		},
	})
	if err != nil {
		log.Fatalf("Failed to launch pattern: %v", err)
	}

	fmt.Printf("✓ Pattern launched successfully!\n")
	fmt.Printf("  Process ID: %s\n", launchResp.ProcessId)
	fmt.Printf("  Address: %s\n", launchResp.Address)
	fmt.Printf("  State: %s\n", launchResp.State)
	fmt.Printf("  Healthy: %v\n\n", launchResp.Healthy)

	// 2. Launch another instance for tenant-b (different namespace)
	fmt.Println("Launching consumer pattern for tenant-b...")
	launchResp2, err := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "consumer",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "tenant-b",
		Config: map[string]string{
			"kafka_brokers": "localhost:9092",
			"consumer_group": "tenant-b-consumers",
			"topic": "events",
		},
	})
	if err != nil {
		log.Fatalf("Failed to launch pattern: %v", err)
	}

	fmt.Printf("✓ Pattern launched successfully!\n")
	fmt.Printf("  Process ID: %s\n\n", launchResp2.ProcessId)

	// 3. List all running patterns
	fmt.Println("Listing all running patterns...")
	listResp, err := client.ListPatterns(ctx, &pb.ListPatternsRequest{})
	if err != nil {
		log.Fatalf("Failed to list patterns: %v", err)
	}

	fmt.Printf("✓ Found %d running patterns:\n", listResp.TotalCount)
	for _, pattern := range listResp.Patterns {
		fmt.Printf("  - %s (%s): %s [namespace=%s, uptime=%ds]\n",
			pattern.PatternName,
			pattern.ProcessId,
			pattern.State,
			pattern.Namespace,
			pattern.UptimeSeconds)
	}
	fmt.Println()

	// 4. Check launcher health
	fmt.Println("Checking launcher health...")
	healthResp, err := client.Health(ctx, &pb.HealthRequest{
		IncludeProcesses: false,
	})
	if err != nil {
		log.Fatalf("Failed to check health: %v", err)
	}

	fmt.Printf("✓ Launcher is healthy!\n")
	fmt.Printf("  Total processes: %d\n", healthResp.TotalProcesses)
	fmt.Printf("  Running: %d\n", healthResp.RunningProcesses)
	fmt.Printf("  Failed: %d\n", healthResp.FailedProcesses)
	fmt.Printf("  Isolation distribution: %v\n\n", healthResp.IsolationDistribution)

	// 5. Wait a bit for patterns to do work
	fmt.Println("Patterns are running. Waiting 5 seconds...")
	time.Sleep(5 * time.Second)

	// 6. Terminate patterns gracefully
	fmt.Println("\nTerminating tenant-a pattern...")
	terminateResp, err := client.TerminatePattern(ctx, &pb.TerminateRequest{
		ProcessId:       launchResp.ProcessId,
		GracePeriodSecs: 10,
	})
	if err != nil {
		log.Fatalf("Failed to terminate pattern: %v", err)
	}

	if terminateResp.Success {
		fmt.Printf("✓ Pattern terminated successfully\n")
	} else {
		fmt.Printf("✗ Pattern termination failed: %s\n", terminateResp.Error)
	}

	fmt.Println("\nTerminating tenant-b pattern...")
	terminateResp2, err := client.TerminatePattern(ctx, &pb.TerminateRequest{
		ProcessId:       launchResp2.ProcessId,
		GracePeriodSecs: 10,
	})
	if err != nil {
		log.Fatalf("Failed to terminate pattern: %v", err)
	}

	if terminateResp2.Success {
		fmt.Printf("✓ Pattern terminated successfully\n")
	} else {
		fmt.Printf("✗ Pattern termination failed: %s\n", terminateResp2.Error)
	}

	fmt.Println("\n✓ Example completed successfully!")
}
