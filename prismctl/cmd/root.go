// Package cmd provides the CLI commands for prismctl
package cmd

import (
	"fmt"
	"os"

	"github.com/jrepp/prism-data-layer/prismctl/internal/config"
	"github.com/jrepp/prism-data-layer/prismctl/internal/ui"
	"github.com/spf13/cobra"
)

var (
	cfg *config.Config
	uiInstance *ui.UI
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "prism",
	Short: "Prism CLI - Manage Prism data access gateway",
	Long: `prismctl is the command-line interface for the Prism data access gateway.

It provides OIDC-authenticated access to Prism admin APIs including namespace
management, session monitoring, and health checks.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize UI
		uiInstance = ui.NewUI()

		// Load configuration
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		return nil
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = "0.1.0"
}
