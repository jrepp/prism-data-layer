package cmd

import (
	"fmt"

	"github.com/jrepp/prism-data-layer/prismctl/internal/auth"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored authentication token",
	Long:  `Remove the stored OIDC authentication token from ~/.prism/token.`,
	RunE:  runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	tokenManager := auth.NewTokenManager(cfg.Token.Path)

	if !tokenManager.Exists() {
		uiInstance.Info("No token found (already logged out)")
		return nil
	}

	if err := tokenManager.Delete(); err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to delete token: %v", err))
		return err
	}

	uiInstance.Success(fmt.Sprintf("Token removed from %s", cfg.Token.Path))
	return nil
}
