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

var namespaceCmd = &cobra.Command{
	Use:   "namespace",
	Short: "Manage namespaces",
	Long:  `Create, list, update, and delete Prism namespaces.`,
}

var namespaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all namespaces",
	RunE:  runNamespaceList,
}

var namespaceCreateCmd = &cobra.Command{
	Use:   "create NAME",
	Short: "Create a new namespace",
	Args:  cobra.ExactArgs(1),
	RunE:  runNamespaceCreate,
}

var namespaceDeleteCmd = &cobra.Command{
	Use:   "delete NAME",
	Short: "Delete a namespace",
	Args:  cobra.ExactArgs(1),
	RunE:  runNamespaceDelete,
}

func init() {
	namespaceCmd.AddCommand(namespaceListCmd)
	namespaceCmd.AddCommand(namespaceCreateCmd)
	namespaceCmd.AddCommand(namespaceDeleteCmd)

	// Flags for create command
	namespaceCreateCmd.Flags().String("description", "", "Namespace description")
	namespaceCreateCmd.Flags().Int64("max-sessions", 0, "Maximum sessions (0 = unlimited)")
	namespaceCreateCmd.Flags().Int64("max-storage", 0, "Maximum storage bytes (0 = unlimited)")

	// Flags for delete command
	namespaceDeleteCmd.Flags().Bool("force", false, "Force deletion without confirmation")
}

func runNamespaceList(cmd *cobra.Command, args []string) error {
	endpoint := viper.GetString("admin.endpoint")

	// Connect to Admin API
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}
	defer conn.Close()

	// TODO: Replace with generated protobuf client when proto files are ready
	// For now, just show the structure
	fmt.Println("Namespaces:")
	fmt.Println("  (Admin API connection successful)")
	fmt.Printf("  Endpoint: %s\n", endpoint)
	fmt.Println("  (Proto definitions needed for full implementation)")

	return nil
}

func runNamespaceCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	description, _ := cmd.Flags().GetString("description")
	maxSessions, _ := cmd.Flags().GetInt64("max-sessions")
	maxStorage, _ := cmd.Flags().GetInt64("max-storage")

	endpoint := viper.GetString("admin.endpoint")

	// Connect with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}
	defer conn.Close()

	// TODO: Call CreateNamespace RPC when proto is available
	fmt.Printf("Creating namespace: %s\n", name)
	if description != "" {
		fmt.Printf("  Description: %s\n", description)
	}
	if maxSessions > 0 {
		fmt.Printf("  Max sessions: %d\n", maxSessions)
	}
	if maxStorage > 0 {
		fmt.Printf("  Max storage: %d bytes\n", maxStorage)
	}
	fmt.Println("  (Proto definitions needed for full implementation)")

	return nil
}

func runNamespaceDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	force, _ := cmd.Flags().GetBool("force")

	if !force {
		fmt.Printf("Are you sure you want to delete namespace '%s'? (y/N): ", name)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	endpoint := viper.GetString("admin.endpoint")

	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}
	defer conn.Close()

	// TODO: Call DeleteNamespace RPC when proto is available
	fmt.Printf("Deleting namespace: %s\n", name)
	fmt.Println("  (Proto definitions needed for full implementation)")

	return nil
}
