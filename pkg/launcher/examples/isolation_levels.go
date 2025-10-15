package main

import (
	"context"
	"fmt"
	"log"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Example: Isolation levels demonstration
//
// This example demonstrates the three isolation levels:
// 1. ISOLATION_NONE: All requests share one process
// 2. ISOLATION_NAMESPACE: Each tenant gets its own process
// 3. ISOLATION_SESSION: Each user gets its own process

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

	fmt.Println("=== Isolation Level Demonstration ===\n")

	// 1. ISOLATION_NONE: Shared process for all requests
	fmt.Println("1. ISOLATION_NONE: Shared process")
	fmt.Println("   Use case: Stateless patterns, read-only lookups")
	fmt.Println()

	// Launch schema-registry with NONE isolation
	resp1, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "schema-registry",
		Isolation:   pb.IsolationLevel_ISOLATION_NONE,
	})
	fmt.Printf("   Request 1 → Process ID: %s\n", resp1.ProcessId)

	// Launch again - should get SAME process
	resp2, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "schema-registry",
		Isolation:   pb.IsolationLevel_ISOLATION_NONE,
	})
	fmt.Printf("   Request 2 → Process ID: %s\n", resp2.ProcessId)

	if resp1.ProcessId == resp2.ProcessId {
		fmt.Println("   ✓ Both requests share the same process!")
	}
	fmt.Println()

	// 2. ISOLATION_NAMESPACE: One process per tenant
	fmt.Println("2. ISOLATION_NAMESPACE: Per-tenant processes")
	fmt.Println("   Use case: Multi-tenant SaaS, fault isolation")
	fmt.Println()

	// Launch consumer for tenant-a
	resp3, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "consumer",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "tenant-a",
	})
	fmt.Printf("   tenant-a → Process ID: %s\n", resp3.ProcessId)

	// Launch consumer for tenant-b
	resp4, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "consumer",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "tenant-b",
	})
	fmt.Printf("   tenant-b → Process ID: %s\n", resp4.ProcessId)

	// Launch again for tenant-a - should reuse existing process
	resp5, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "consumer",
		Isolation:   pb.IsolationLevel_ISOLATION_NAMESPACE,
		Namespace:   "tenant-a",
	})
	fmt.Printf("   tenant-a (again) → Process ID: %s\n", resp5.ProcessId)

	if resp3.ProcessId == resp5.ProcessId {
		fmt.Println("   ✓ tenant-a requests share the same process!")
	}
	if resp3.ProcessId != resp4.ProcessId {
		fmt.Println("   ✓ tenant-a and tenant-b have separate processes!")
	}
	fmt.Println()

	// 3. ISOLATION_SESSION: One process per user
	fmt.Println("3. ISOLATION_SESSION: Per-user processes")
	fmt.Println("   Use case: High-security, compliance (PCI-DSS, HIPAA)")
	fmt.Println()

	// Launch producer for user-1 in tenant-a
	resp6, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "producer",
		Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
		Namespace:   "tenant-a",
		SessionId:   "user-1",
	})
	fmt.Printf("   tenant-a:user-1 → Process ID: %s\n", resp6.ProcessId)

	// Launch producer for user-2 in tenant-a
	resp7, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "producer",
		Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
		Namespace:   "tenant-a",
		SessionId:   "user-2",
	})
	fmt.Printf("   tenant-a:user-2 → Process ID: %s\n", resp7.ProcessId)

	// Launch producer for user-1 in tenant-b
	resp8, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "producer",
		Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
		Namespace:   "tenant-b",
		SessionId:   "user-1",
	})
	fmt.Printf("   tenant-b:user-1 → Process ID: %s\n", resp8.ProcessId)

	// Launch again for user-1 in tenant-a - should reuse
	resp9, _ := client.LaunchPattern(ctx, &pb.LaunchRequest{
		PatternName: "producer",
		Isolation:   pb.IsolationLevel_ISOLATION_SESSION,
		Namespace:   "tenant-a",
		SessionId:   "user-1",
	})
	fmt.Printf("   tenant-a:user-1 (again) → Process ID: %s\n", resp9.ProcessId)

	if resp6.ProcessId == resp9.ProcessId {
		fmt.Println("   ✓ Same user's requests share the same process!")
	}
	if resp6.ProcessId != resp7.ProcessId {
		fmt.Println("   ✓ Different users have separate processes!")
	}
	if resp6.ProcessId != resp8.ProcessId {
		fmt.Println("   ✓ Same user in different tenants have separate processes!")
	}
	fmt.Println()

	// Summary
	fmt.Println("=== Summary ===")
	listResp, _ := client.ListPatterns(ctx, &pb.ListPatternsRequest{})
	fmt.Printf("Total running processes: %d\n", listResp.TotalCount)

	healthResp, _ := client.Health(ctx, &pb.HealthRequest{})
	fmt.Println("Isolation distribution:")
	for level, count := range healthResp.IsolationDistribution {
		fmt.Printf("  %s: %d processes\n", level, count)
	}
}
