package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "prism-admin",
	Short: "Prism administrative CLI",
	Long: `prism-admin provides administrative commands for Prism data gateway.

Manage namespaces, monitor sessions, check backend health, and perform
operational tasks via the Admin gRPC API (port 8981).`,
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringP("endpoint", "e", "localhost:8981", "Admin API endpoint")
	rootCmd.PersistentFlags().StringP("config", "c", "", "Config file (default: ~/.prism.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")

	// Bind flags to viper
	viper.BindPFlag("admin.endpoint", rootCmd.PersistentFlags().Lookup("endpoint"))
	viper.BindPFlag("logging.level", rootCmd.PersistentFlags().Lookup("log-level"))

	// Add subcommands
	rootCmd.AddCommand(namespaceCmd)
	rootCmd.AddCommand(healthCmd)
}

func initConfig() {
	// Use config file from flag if provided
	if cfgFile := rootCmd.PersistentFlags().Lookup("config").Value.String(); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in home directory and current directory
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigName(".prism")
		viper.SetConfigType("yaml")
	}

	// Environment variables
	viper.SetEnvPrefix("PRISM")
	viper.AutomaticEnv()

	// Read config file (ignore error if not found)
	viper.ReadInConfig()
}
