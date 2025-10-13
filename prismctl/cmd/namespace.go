package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/auth"
	"github.com/jrepp/prism-data-layer/prismctl/internal/client"
	"github.com/spf13/cobra"
)

var namespaceCmd = &cobra.Command{
	Use:   "namespace",
	Short: "Manage namespaces",
	Long:  `List and view namespace details in the Prism proxy.`,
}

var namespaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all namespaces",
	Long:  `List all namespaces available in the Prism proxy.`,
	RunE:  runNamespaceList,
}

var namespaceShowCmd = &cobra.Command{
	Use:   "show NAME",
	Short: "Show namespace details",
	Long:  `Display detailed information about a specific namespace.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runNamespaceShow,
}

func init() {
	rootCmd.AddCommand(namespaceCmd)
	namespaceCmd.AddCommand(namespaceListCmd)
	namespaceCmd.AddCommand(namespaceShowCmd)
}

func runNamespaceList(cmd *cobra.Command, args []string) error {
	token, err := loadAndValidateToken()
	if err != nil {
		return err
	}

	c := client.NewClient(&cfg.Proxy, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	namespaces, err := c.ListNamespaces(ctx)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to list namespaces: %v", err))
		return err
	}

	if len(namespaces) == 0 {
		uiInstance.Info("No namespaces found")
		return nil
	}

	uiInstance.Success(fmt.Sprintf("Found %d namespace(s)", len(namespaces)))
	uiInstance.Println("")

	for _, ns := range namespaces {
		name, _ := ns["name"].(string)
		desc, _ := ns["description"].(string)

		uiInstance.ListItem(name)
		if desc != "" {
			uiInstance.Subtle("    " + desc)
		}
	}

	return nil
}

func runNamespaceShow(cmd *cobra.Command, args []string) error {
	token, err := loadAndValidateToken()
	if err != nil {
		return err
	}

	name := args[0]
	c := client.NewClient(&cfg.Proxy, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ns, err := c.GetNamespace(ctx, name)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to get namespace: %v", err))
		return err
	}

	// Display namespace details
	uiInstance.Header(fmt.Sprintf("Namespace: %s", name))
	uiInstance.Println("")

	if desc, ok := ns["description"].(string); ok && desc != "" {
		uiInstance.KeyValue("Description", desc)
	}

	if created, ok := ns["created_at"].(string); ok {
		uiInstance.KeyValue("Created", created)
	}

	if backends, ok := ns["backends"].([]interface{}); ok && len(backends) > 0 {
		backendNames := make([]string, len(backends))
		for i, b := range backends {
			if s, ok := b.(string); ok {
				backendNames[i] = s
			}
		}
		uiInstance.KeyValue("Backends", fmt.Sprintf("%v", backendNames))
	}

	return nil
}

// loadAndValidateToken loads and validates the authentication token
func loadAndValidateToken() (*auth.Token, error) {
	tokenManager := auth.NewTokenManager(cfg.Token.Path)

	token, err := tokenManager.Load()
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to load token: %v", err))
		return nil, err
	}

	if token == nil {
		uiInstance.Error("Not authenticated. Run 'prism login' first.")
		return nil, fmt.Errorf("not authenticated")
	}

	if token.IsExpired() {
		uiInstance.Warning("Token expired. Run 'prism login' again.")
		return nil, fmt.Errorf("token expired")
	}

	return token, nil
}
