package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/client"
	"github.com/spf13/cobra"
)

var mailboxCmd = &cobra.Command{
	Use:   "mailbox",
	Short: "Query messages from mailbox namespaces",
	Long: `Query and retrieve messages stored in mailbox pattern namespaces.

The mailbox pattern stores messages with indexed headers and blob bodies,
enabling efficient searching by metadata while keeping encrypted payloads opaque.`,
}

var mailboxFlags struct {
	limit         int
	offset        int
	startTime     string
	endTime       string
	topics        []string
	principals    []string
	correlationID string
	showPayload   bool
	format        string
}

var mailboxQueryCmd = &cobra.Command{
	Use:   "query NAMESPACE",
	Short: "Query messages from a mailbox",
	Long: `Query messages from a mailbox namespace using filters.

Examples:
  # Query recent messages
  prismctl mailbox query my-mailbox --limit 10

  # Query by time range
  prismctl mailbox query my-mailbox \
    --start-time "2025-10-15T00:00:00Z" \
    --end-time "2025-10-15T23:59:59Z"

  # Query by topic
  prismctl mailbox query my-mailbox --topic "admin.users.*"

  # Query by principal
  prismctl mailbox query my-mailbox --principal "user@example.com"

  # Query with correlation ID (trace)
  prismctl mailbox query my-mailbox --correlation-id "trace-abc-123"

  # Show full payload
  prismctl mailbox query my-mailbox --limit 5 --show-payload`,
	Args: cobra.ExactArgs(1),
	RunE: runMailboxQuery,
}

var mailboxGetCmd = &cobra.Command{
	Use:   "get NAMESPACE MESSAGE_ID",
	Short: "Get a single message by ID",
	Long:  `Retrieve a single message from the mailbox by its message ID.`,
	Args:  cobra.ExactArgs(2),
	RunE:  runMailboxGet,
}

func init() {
	rootCmd.AddCommand(mailboxCmd)
	mailboxCmd.AddCommand(mailboxQueryCmd)
	mailboxCmd.AddCommand(mailboxGetCmd)

	// Query flags
	mailboxQueryCmd.Flags().IntVarP(&mailboxFlags.limit, "limit", "l", 10, "Maximum number of messages to return")
	mailboxQueryCmd.Flags().IntVar(&mailboxFlags.offset, "offset", 0, "Offset for pagination")
	mailboxQueryCmd.Flags().StringVar(&mailboxFlags.startTime, "start-time", "", "Start time filter (RFC3339 format)")
	mailboxQueryCmd.Flags().StringVar(&mailboxFlags.endTime, "end-time", "", "End time filter (RFC3339 format)")
	mailboxQueryCmd.Flags().StringSliceVarP(&mailboxFlags.topics, "topic", "t", []string{}, "Topic filters")
	mailboxQueryCmd.Flags().StringSliceVarP(&mailboxFlags.principals, "principal", "p", []string{}, "Principal filters")
	mailboxQueryCmd.Flags().StringVar(&mailboxFlags.correlationID, "correlation-id", "", "Correlation ID filter")
	mailboxQueryCmd.Flags().BoolVar(&mailboxFlags.showPayload, "show-payload", false, "Show message payload")
	mailboxQueryCmd.Flags().StringVar(&mailboxFlags.format, "format", "table", "Output format (table, json)")

	// Get flags
	mailboxGetCmd.Flags().BoolVar(&mailboxFlags.showPayload, "show-payload", true, "Show message payload")
	mailboxGetCmd.Flags().StringVar(&mailboxFlags.format, "format", "table", "Output format (table, json)")
}

func runMailboxQuery(cmd *cobra.Command, args []string) error {
	token, err := loadAndValidateToken()
	if err != nil {
		return err
	}

	namespace := args[0]

	// Build filter
	filter := make(map[string]interface{})

	if mailboxFlags.limit > 0 {
		filter["limit"] = mailboxFlags.limit
	}
	if mailboxFlags.offset > 0 {
		filter["offset"] = mailboxFlags.offset
	}

	// Parse time filters
	if mailboxFlags.startTime != "" {
		t, err := time.Parse(time.RFC3339, mailboxFlags.startTime)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Invalid start-time format: %v", err))
			return err
		}
		filter["start_time"] = t.UnixMilli()
	}
	if mailboxFlags.endTime != "" {
		t, err := time.Parse(time.RFC3339, mailboxFlags.endTime)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Invalid end-time format: %v", err))
			return err
		}
		filter["end_time"] = t.UnixMilli()
	}

	if len(mailboxFlags.topics) > 0 {
		filter["topics"] = mailboxFlags.topics
	}
	if len(mailboxFlags.principals) > 0 {
		filter["principals"] = mailboxFlags.principals
	}
	if mailboxFlags.correlationID != "" {
		filter["correlation_id"] = mailboxFlags.correlationID
	}

	// Create client
	c := client.NewClient(&cfg.Proxy, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uiInstance.Info(fmt.Sprintf("Querying mailbox '%s'", namespace))

	events, err := c.QueryMailbox(ctx, namespace, filter)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to query mailbox: %v", err))
		return err
	}

	if len(events) == 0 {
		uiInstance.Info("No messages found")
		return nil
	}

	// Output based on format
	if mailboxFlags.format == "json" {
		output, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(output))
		return nil
	}

	// Table format
	uiInstance.Success(fmt.Sprintf("Found %d message(s)", len(events)))
	uiInstance.Println("")

	for i, event := range events {
		displayEvent(event, i+1, mailboxFlags.showPayload)
		if i < len(events)-1 {
			uiInstance.Println("---")
		}
	}

	return nil
}

func runMailboxGet(cmd *cobra.Command, args []string) error {
	token, err := loadAndValidateToken()
	if err != nil {
		return err
	}

	namespace := args[0]
	messageID := args[1]

	// Create client
	c := client.NewClient(&cfg.Proxy, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uiInstance.Info(fmt.Sprintf("Retrieving message '%s' from mailbox '%s'", messageID, namespace))

	event, err := c.GetMailboxEvent(ctx, namespace, messageID)
	if err != nil {
		uiInstance.Error(fmt.Sprintf("Failed to get message: %v", err))
		return err
	}

	// Output based on format
	if mailboxFlags.format == "json" {
		output, err := json.MarshalIndent(event, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(output))
		return nil
	}

	// Table format
	uiInstance.Success("Message found")
	uiInstance.Println("")
	displayEvent(event, 0, mailboxFlags.showPayload)

	return nil
}

func displayEvent(event map[string]interface{}, index int, showPayload bool) {
	if index > 0 {
		uiInstance.Header(fmt.Sprintf("Message #%d", index))
	}

	// Display indexed headers
	if messageID, ok := event["message_id"].(string); ok {
		uiInstance.KeyValue("Message ID", messageID)
	}

	if timestamp, ok := event["timestamp"].(float64); ok {
		t := time.UnixMilli(int64(timestamp))
		uiInstance.KeyValue("Timestamp", t.Format(time.RFC3339))
	}

	if topic, ok := event["topic"].(string); ok {
		uiInstance.KeyValue("Topic", topic)
	}

	if contentType, ok := event["content_type"].(string); ok && contentType != "" {
		uiInstance.KeyValue("Content Type", contentType)
	}

	if schemaID, ok := event["schema_id"].(string); ok && schemaID != "" {
		uiInstance.KeyValue("Schema ID", schemaID)
	}

	if encryption, ok := event["encryption"].(string); ok && encryption != "" {
		uiInstance.KeyValue("Encryption", encryption)
	}

	if correlationID, ok := event["correlation_id"].(string); ok && correlationID != "" {
		uiInstance.KeyValue("Correlation ID", correlationID)
	}

	if principal, ok := event["principal"].(string); ok && principal != "" {
		uiInstance.KeyValue("Principal", principal)
	}

	if namespace, ok := event["namespace"].(string); ok && namespace != "" {
		uiInstance.KeyValue("Namespace", namespace)
	}

	// Display custom headers
	if customHeaders, ok := event["custom_headers"].(map[string]interface{}); ok && len(customHeaders) > 0 {
		uiInstance.Println("")
		uiInstance.Subtle("Custom Headers:")
		for k, v := range customHeaders {
			uiInstance.Subtle(fmt.Sprintf("  %s: %v", k, v))
		}
	}

	// Display payload if requested
	if showPayload {
		if body, ok := event["body"].(string); ok && body != "" {
			uiInstance.Println("")

			// Check if body is base64 encoded
			if decoded, err := base64.StdEncoding.DecodeString(body); err == nil {
				body = string(decoded)
			}

			// Try to format as JSON
			if isJSON([]byte(body)) {
				var prettyJSON interface{}
				if err := json.Unmarshal([]byte(body), &prettyJSON); err == nil {
					formatted, err := json.MarshalIndent(prettyJSON, "", "  ")
					if err == nil {
						uiInstance.Subtle("Payload:")
						uiInstance.Println(string(formatted))
					} else {
						uiInstance.Subtle("Payload:")
						displayTruncatedPayload(body)
					}
				}
			} else {
				uiInstance.Subtle("Payload:")
				displayTruncatedPayload(body)
			}
		}
	} else {
		if body, ok := event["body"].(string); ok && body != "" {
			size := len(body)
			uiInstance.Subtle(fmt.Sprintf("Payload: %d bytes (use --show-payload to display)", size))
		}
	}
}

func displayTruncatedPayload(payload string) {
	maxLen := 500
	if len(payload) > maxLen {
		uiInstance.Println(payload[:maxLen] + "...")
		uiInstance.Subtle(fmt.Sprintf("(truncated, %d bytes total)", len(payload)))
	} else {
		uiInstance.Println(payload)
	}
}

// Helper function to check if string is valid JSON (already defined in publish.go but duplicated here for safety)
func isJSON(data []byte) bool {
	var js interface{}
	return json.Unmarshal(data, &js) == nil
}
