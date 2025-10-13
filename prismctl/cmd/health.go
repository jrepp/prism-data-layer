package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/client"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check Prism proxy health",
	Long:  `Check the health status of the Prism proxy server.`,
	RunE:  runHealth,
}

func init() {
	rootCmd.AddCommand(healthCmd)
}

func runHealth(cmd *cobra.Command, args []string) error {
	c := client.NewClient(&cfg.Proxy, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthData, err := c.Health(ctx)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Health check failed: %v", err))
		return err
	}

	uiInstance.Success("Proxy is healthy")

	if status, ok := healthData["status"].(string); ok {
		uiInstance.KeyValue("Status", status)
	}

	if version, ok := healthData["version"].(string); ok {
		uiInstance.KeyValue("Version", version)
	}

	if uptime, ok := healthData["uptime"].(string); ok {
		uiInstance.KeyValue("Uptime", uptime)
	}

	return nil
}
