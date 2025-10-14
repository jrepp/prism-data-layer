package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/client"
	"github.com/spf13/cobra"
)

var (
	sessionNamespace string
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sessions",
	Long:  `List and monitor active sessions in the Prism proxy.`,
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active sessions",
	Long:  `List all active sessions in the Prism proxy, optionally filtered by namespace.`,
	RunE:  runSessionList,
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionListCmd)

	sessionListCmd.Flags().StringVar(&sessionNamespace, "namespace", "", "Filter by namespace")
}

func runSessionList(cmd *cobra.Command, args []string) error {
	token, err := loadAndValidateToken()
	if err != nil {
		return err
	}

	c := client.NewClient(&cfg.Proxy, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sessions, err := c.ListSessions(ctx, sessionNamespace)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to list sessions: %v", err))
		return err
	}

	if len(sessions) == 0 {
		uiInstance.Info("No active sessions")
		return nil
	}

	uiInstance.Success(fmt.Sprintf("Found %d active session(s)", len(sessions)))
	uiInstance.Println("")

	// Create table
	table := uiInstance.NewTable("Session ID", "Principal", "Namespace", "Started")

	for _, s := range sessions {
		sessionID, _ := s["id"].(string)
		principal, _ := s["principal"].(string)
		ns, _ := s["namespace"].(string)
		started, _ := s["started_at"].(string)

		// Truncate long values
		if len(sessionID) > 36 {
			sessionID = sessionID[:36]
		}
		if len(principal) > 30 {
			principal = principal[:30]
		}
		if len(ns) > 20 {
			ns = ns[:20]
		}
		if len(started) > 19 {
			started = started[:19]
		}

		table.AddRow(sessionID, principal, ns, started)
	}

	table.Render()

	return nil
}
