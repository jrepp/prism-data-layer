package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/auth"
	"github.com/jrepp/prism-data-layer/prismctl/internal/config"
	"github.com/spf13/cobra"
)

var (
	useDeviceCode bool
	username      string
	password      string
	openBrowser   bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Prism using OIDC",
	Long: `Authenticate with Prism using OIDC device code flow (recommended) or password flow (testing only).

By default, uses device code flow which is secure for CLI applications. The user
will be shown a verification URL and code to enter in their browser.

For local testing, you can use the password flow with --username and --password.`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().BoolVar(&useDeviceCode, "device-code", true, "Use device code flow (recommended)")
	loginCmd.Flags().StringVar(&username, "username", "", "Username for password flow (testing only)")
	loginCmd.Flags().StringVar(&password, "password", "", "Password for password flow (testing only)")
	loginCmd.Flags().BoolVar(&openBrowser, "open-browser", true, "Automatically open browser for device code flow")
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Ensure ~/.prism directory exists
	if err := config.EnsurePrismDir(); err != nil {
		return fmt.Errorf("ensure prism directory: %w", err)
	}

	authenticator, err := auth.NewOIDCAuthenticator(&cfg.OIDC)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to initialize OIDC: %v", err))
		return err
	}

	tokenManager := auth.NewTokenManager(cfg.Token.Path)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var token *auth.Token

	if useDeviceCode && username == "" && password == "" {
		// Device code flow
		uiInstance.Info("Starting device code authentication...")

		deviceResp, _, err := authenticator.LoginDeviceCode(ctx)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Device code request failed: %v", err))
			return err
		}

		// Display prompt to user
		uiInstance.DeviceCodePrompt(deviceResp.VerificationURI, deviceResp.UserCode, deviceResp.ExpiresIn)

		// Open browser if requested
		if openBrowser && deviceResp.VerificationURIComplete != "" {
			if err := openURL(deviceResp.VerificationURIComplete); err != nil {
				uiInstance.Warning(fmt.Sprintf("Failed to open browser: %v", err))
			}
		}

		// Poll for token
		token, err = authenticator.PollForToken(ctx, deviceResp.DeviceCode, deviceResp.Interval)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Authentication failed: %v", err))
			return err
		}

	} else if username != "" && password != "" {
		// Password flow
		uiInstance.Warning("Using password flow (testing only)")

		token, err = authenticator.LoginPassword(ctx, username, password)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Authentication failed: %v", err))
			return err
		}

	} else {
		return fmt.Errorf("either use device code (default) or provide --username and --password")
	}

	// Save token
	if err := tokenManager.Save(token); err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to save token: %v", err))
		return err
	}

	// Get user info
	userinfo, err := authenticator.GetUserinfo(ctx, token)
	if err != nil {
		uiInstance.Warning(fmt.Sprintf("Failed to get user info: %v", err))
	}

	// Success message
	uiInstance.Println("")
	uiInstance.Success("Authenticated successfully!")
	if userinfo != nil {
		if name, ok := userinfo["name"].(string); ok {
			uiInstance.KeyValue("User", name)
		} else if email, ok := userinfo["email"].(string); ok {
			uiInstance.KeyValue("User", email)
		}
	}
	uiInstance.KeyValue("Token expires", token.ExpiresAt.Format(time.RFC3339))
	uiInstance.KeyValue("Token saved to", cfg.Token.Path)

	return nil
}

// openURL opens a URL in the default browser (platform-specific)
func openURL(url string) error {
	var cmd string
	var args []string

	switch {
	case isCommandAvailable("open"): // macOS
		cmd = "open"
		args = []string{url}
	case isCommandAvailable("xdg-open"): // Linux
		cmd = "xdg-open"
		args = []string{url}
	case isCommandAvailable("start"): // Windows
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("no browser opener found")
	}

	return exec.Command(cmd, args...).Start()
}

// isCommandAvailable checks if a command is available in PATH
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
