package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/client"
	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish messages to a namespace",
	Long: `Publish messages to a namespace for testing purposes.

Examples:
  # Publish a simple text message
  prismctl publish my-namespace my-topic "Hello, World!"

  # Publish JSON data
  prismctl publish my-namespace my-topic '{"user": "alice", "action": "login"}'

  # Publish from a file
  prismctl publish my-namespace my-topic --file message.json

  # Publish with custom headers
  prismctl publish my-namespace my-topic "test data" \
    --header "x-user-id=123" \
    --header "x-trace-id=abc-def"

  # Publish with standard Prism headers
  prismctl publish my-namespace my-topic "test data" \
    --content-type "application/json" \
    --correlation-id "trace-123" \
    --principal "user@example.com"`,
}

var publishFlags struct {
	file          string
	contentType   string
	correlationID string
	principal     string
	schemaID      string
	encryption    string
	headers       []string
	count         int
}

var publishMessageCmd = &cobra.Command{
	Use:   "message NAMESPACE TOPIC DATA",
	Short: "Publish a message to a topic",
	Long:  `Publish a message to a topic in the specified namespace.`,
	Args:  cobra.RangeArgs(2, 3),
	RunE:  runPublishMessage,
}

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.AddCommand(publishMessageCmd)

	// Message publishing flags
	publishMessageCmd.Flags().StringVarP(&publishFlags.file, "file", "f", "", "Read message payload from file")
	publishMessageCmd.Flags().StringVar(&publishFlags.contentType, "content-type", "text/plain", "Content type of the message")
	publishMessageCmd.Flags().StringVar(&publishFlags.correlationID, "correlation-id", "", "Correlation ID for tracing")
	publishMessageCmd.Flags().StringVar(&publishFlags.principal, "principal", "", "Principal (user/service) sending the message")
	publishMessageCmd.Flags().StringVar(&publishFlags.schemaID, "schema-id", "", "Schema registry ID")
	publishMessageCmd.Flags().StringVar(&publishFlags.encryption, "encryption", "", "Encryption algorithm used")
	publishMessageCmd.Flags().StringSliceVarP(&publishFlags.headers, "header", "H", []string{}, "Custom headers (format: key=value)")
	publishMessageCmd.Flags().IntVarP(&publishFlags.count, "count", "n", 1, "Number of messages to publish")
}

func runPublishMessage(cmd *cobra.Command, args []string) error {
	token, err := loadAndValidateToken()
	if err != nil {
		return err
	}

	namespace := args[0]
	topic := args[1]

	// Read message payload
	var payload []byte
	if publishFlags.file != "" {
		// Read from file
		payload, err = os.ReadFile(publishFlags.file)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Failed to read file: %v", err))
			return err
		}
		uiInstance.Info(fmt.Sprintf("Read %d bytes from %s", len(payload), publishFlags.file))
	} else {
		// Use inline data
		if len(args) < 3 {
			uiInstance.Error("DATA argument is required when --file is not specified")
			return fmt.Errorf("missing DATA argument")
		}
		payload = []byte(args[2])
	}

	// Build metadata
	metadata := make(map[string]string)

	// Standard Prism headers
	metadata["prism-content-type"] = publishFlags.contentType
	if publishFlags.correlationID != "" {
		metadata["prism-correlation-id"] = publishFlags.correlationID
	}
	if publishFlags.principal != "" {
		metadata["prism-principal"] = publishFlags.principal
	}
	if publishFlags.schemaID != "" {
		metadata["prism-schema-id"] = publishFlags.schemaID
	}
	if publishFlags.encryption != "" {
		metadata["prism-encryption"] = publishFlags.encryption
	}
	metadata["prism-namespace"] = namespace

	// Custom headers
	for _, header := range publishFlags.headers {
		key, value, found := parseHeader(header)
		if !found {
			uiInstance.Warning(fmt.Sprintf("Skipping invalid header format: %s", header))
			continue
		}
		metadata[key] = value
	}

	// Create client
	c := client.NewClient(&cfg.Proxy, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uiInstance.Info(fmt.Sprintf("Publishing %d message(s) to namespace '%s', topic '%s'", publishFlags.count, namespace, topic))

	// Publish messages
	successCount := 0
	failCount := 0

	for i := 0; i < publishFlags.count; i++ {
		// Add sequence number for multiple messages
		currentMetadata := make(map[string]string)
		for k, v := range metadata {
			currentMetadata[k] = v
		}
		if publishFlags.count > 1 {
			currentMetadata["x-sequence"] = fmt.Sprintf("%d", i+1)
		}

		messageID, err := c.PublishMessage(ctx, namespace, topic, payload, currentMetadata)
		if err != nil {
			uiInstance.Error(fmt.Sprintf("Failed to publish message %d: %v", i+1, err))
			failCount++
			continue
		}

		successCount++

		if publishFlags.count == 1 {
			uiInstance.Success(fmt.Sprintf("Published message"))
			uiInstance.KeyValue("Message ID", messageID)
			uiInstance.KeyValue("Namespace", namespace)
			uiInstance.KeyValue("Topic", topic)
			uiInstance.KeyValue("Payload Size", fmt.Sprintf("%d bytes", len(payload)))

			// Show metadata if verbose
			if len(currentMetadata) > 0 {
				uiInstance.Println("")
				uiInstance.Subtle("Metadata:")
				for k, v := range currentMetadata {
					uiInstance.Subtle(fmt.Sprintf("  %s: %s", k, v))
				}
			}

			// Show payload preview for small messages
			if len(payload) <= 200 {
				uiInstance.Println("")
				uiInstance.Subtle("Payload:")
				// Try to format as JSON for readability
				if isJSON(payload) {
					var prettyJSON interface{}
					if err := json.Unmarshal(payload, &prettyJSON); err == nil {
						formatted, err := json.MarshalIndent(prettyJSON, "  ", "  ")
						if err == nil {
							uiInstance.Subtle(fmt.Sprintf("  %s", string(formatted)))
						}
					}
				} else {
					uiInstance.Subtle(fmt.Sprintf("  %s", string(payload)))
				}
			}
		} else if i == 0 || (i+1)%10 == 0 || i == publishFlags.count-1 {
			uiInstance.Info(fmt.Sprintf("Published message %d/%d (ID: %s)", i+1, publishFlags.count, messageID))
		}
	}

	uiInstance.Println("")
	if failCount == 0 {
		uiInstance.Success(fmt.Sprintf("Successfully published %d message(s)", successCount))
	} else {
		uiInstance.Warning(fmt.Sprintf("Published %d/%d messages (%d failed)", successCount, publishFlags.count, failCount))
	}

	return nil
}

// parseHeader parses a header in "key=value" format
func parseHeader(header string) (key, value string, ok bool) {
	for i := 0; i < len(header); i++ {
		if header[i] == '=' {
			return header[:i], header[i+1:], true
		}
	}
	return "", "", false
}

// isJSON checks if the payload is valid JSON
func isJSON(data []byte) bool {
	var js interface{}
	return json.Unmarshal(data, &js) == nil
}
