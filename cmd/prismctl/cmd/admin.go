package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(adminCmd)
	adminCmd.AddCommand(adminStatusCmd)
}

// adminCmd represents the admin command
var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Interact with prism-admin control plane",
	Long: `Commands for interacting with the prism-admin control plane.

View status, inspect resources, and manage the admin server.`,
}

// adminStatusCmd shows the status of admin resources
var adminStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show admin control plane status and resources",
	Long: `Display a comprehensive view of the prism-admin control plane including:
- Namespaces
- Proxy connections
- Launcher connections
- Recent audit logs

Example:
  prismctl admin status
  prismctl admin status --endpoint localhost:8981`,
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint := cmd.Flag("endpoint").Value.String()
		if endpoint == "" {
			endpoint = "localhost:8981"
		}
		return showAdminStatus(endpoint)
	},
}

func init() {
	adminStatusCmd.Flags().StringP("endpoint", "e", "localhost:8981", "Admin control plane endpoint")
}

// showAdminStatus displays the admin control plane status
func showAdminStatus(endpoint string) error {
	fmt.Printf("🔍 Prism Admin Control Plane Status\n")
	fmt.Printf("   Endpoint: %s\n\n", endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to admin's storage directly for now
	// In a production system, this would be a gRPC Admin API call

	// For POC, we'll read directly from the SQLite database
	storage, err := connectToAdminStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to admin storage: %w", err)
	}
	defer storage.Close()

	// Display Namespaces
	fmt.Println("📦 Namespaces")
	namespaces, err := storage.ListNamespaces(ctx)
	if err != nil {
		fmt.Printf("   ⚠️  Error listing namespaces: %v\n", err)
	} else if len(namespaces) == 0 {
		fmt.Println("   (none)")
	} else {
		for _, ns := range namespaces {
			fmt.Printf("   • %s\n", ns.Name)
			if ns.Description != "" {
				fmt.Printf("     Description: %s\n", ns.Description)
			}
			fmt.Printf("     Created: %s\n", ns.CreatedAt.Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Println()

	// Display Proxies
	fmt.Println("🔌 Proxy Connections")
	proxies, err := storage.ListProxies(ctx)
	if err != nil {
		fmt.Printf("   ⚠️  Error listing proxies: %v\n", err)
	} else if len(proxies) == 0 {
		fmt.Println("   (none)")
	} else {
		for _, proxy := range proxies {
			statusIcon := "✅"
			if proxy.Status != "healthy" {
				statusIcon = "❌"
			}

			lastSeenStr := "never"
			if proxy.LastSeen != nil {
				elapsed := time.Since(*proxy.LastSeen)
				if elapsed < time.Minute {
					lastSeenStr = fmt.Sprintf("%ds ago", int(elapsed.Seconds()))
				} else if elapsed < time.Hour {
					lastSeenStr = fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
				} else {
					lastSeenStr = proxy.LastSeen.Format("2006-01-02 15:04:05")
				}
			}

			fmt.Printf("   %s %s (%s)\n", statusIcon, proxy.ProxyID, proxy.Status)
			fmt.Printf("     Address: %s\n", proxy.Address)
			fmt.Printf("     Version: %s\n", proxy.Version)
			fmt.Printf("     Last Seen: %s\n", lastSeenStr)
		}
	}
	fmt.Println()

	// Display Launchers (for now, we'll show this is coming)
	fmt.Println("🚀 Launcher Connections")
	fmt.Println("   (launcher storage table not yet added to schema)")
	fmt.Println()

	// Display Recent Audit Logs
	fmt.Println("📋 Recent Audit Logs (last 10)")
	logs, err := storage.QueryAuditLogs(ctx, AuditQueryOptions{
		Limit: 10,
	})
	if err != nil {
		fmt.Printf("   ⚠️  Error listing audit logs: %v\n", err)
	} else if len(logs) == 0 {
		fmt.Println("   (none)")
	} else {
		for _, log := range logs {
			timestamp := log.Timestamp.Format("15:04:05")
			statusIcon := "✅"
			if log.StatusCode >= 400 {
				statusIcon = "❌"
			}

			fmt.Printf("   %s [%s] %s %s\n", statusIcon, timestamp, log.Method, log.Path)
			if log.User != "" {
				fmt.Printf("     User: %s\n", log.User)
			}
			if log.Namespace != "" {
				fmt.Printf("     Namespace: %s\n", log.Namespace)
			}
			if log.Error != "" {
				fmt.Printf("     Error: %s\n", log.Error)
			}
			fmt.Printf("     Duration: %dms\n", log.DurationMs)
		}
	}
	fmt.Println()

	fmt.Println("✅ Status check complete")
	return nil
}

// connectToAdminStorage connects directly to the admin SQLite database
func connectToAdminStorage() (*Storage, error) {
	// Default admin database path
	cfg := &DatabaseConfig{
		Type: "sqlite",
		Path: defaultDatabasePath(),
	}

	return NewStorage(context.Background(), cfg)
}
