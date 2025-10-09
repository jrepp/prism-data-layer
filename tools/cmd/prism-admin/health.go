package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check backend health",
	Long:  `Query the health status of Prism backends (Postgres, Kafka, NATS, etc.).`,
	RunE:  runHealth,
}

func init() {
	healthCmd.Flags().String("backend", "", "Specific backend to check (e.g., postgres, kafka)")
}

func runHealth(cmd *cobra.Command, args []string) error {
	endpoint := viper.GetString("admin.endpoint")
	backend, _ := cmd.Flags().GetString("backend")

	// Connect with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to Admin API at %s: %w", endpoint, err)
	}
	defer conn.Close()

	// Connection successful - API is reachable
	fmt.Println("Backend Health Status")
	fmt.Println("=====================")
	fmt.Printf("Admin API: %s ✓\n", endpoint)
	fmt.Println()

	if backend != "" {
		fmt.Printf("Backend: %s\n", backend)
	} else {
		fmt.Println("All backends:")
	}

	// TODO: Call GetBackendStatus RPC when proto is available
	fmt.Println("  (Proto definitions needed for full implementation)")
	fmt.Println()
	fmt.Println("Expected backends:")
	fmt.Println("  - Postgres")
	fmt.Println("  - Kafka")
	fmt.Println("  - NATS")
	fmt.Println("  - SQLite")

	return nil
}
