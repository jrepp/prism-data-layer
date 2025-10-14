package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/auth"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication status",
	Long:  `Display information about the currently authenticated user and token expiration.`,
	RunE:  runWhoami,
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}

func runWhoami(cmd *cobra.Command, args []string) error {
	tokenManager := auth.NewTokenManager(cfg.Token.Path)

	token, err := tokenManager.Load()
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to load token: %v", err))
		return err
	}

	if token == nil {
		uiInstance.Error("Not authenticated. Run 'prism login' first.")
		return fmt.Errorf("not authenticated")
	}

	if token.IsExpired() {
		uiInstance.Warning("Token expired. Run 'prism login' again.")
		return fmt.Errorf("token expired")
	}

	authenticator, err := auth.NewOIDCAuthenticator(&cfg.OIDC)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to initialize OIDC: %v", err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userinfo, err := authenticator.GetUserinfo(ctx, token)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to get user info: %v", err))
		return err
	}

	// Display user info
	uiInstance.Success("Authenticated")
	uiInstance.Println("")

	if name, ok := userinfo["name"].(string); ok {
		uiInstance.KeyValue("Name", name)
	}
	if email, ok := userinfo["email"].(string); ok {
		uiInstance.KeyValue("Email", email)
	}
	uiInstance.KeyValue("Token expires", token.ExpiresAt.Format(time.RFC3339))

	if token.NeedsRefresh() {
		uiInstance.Println("")
		uiInstance.Warning("Token expires soon, consider refreshing")
	}

	return nil
}
