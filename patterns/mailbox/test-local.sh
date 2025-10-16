#!/usr/bin/env bash
#
# Local mailbox testing script
# Tests the publish → store → query workflow without external dependencies
#
# This script demonstrates:
# 1. Direct writes to SQLite mailbox (simulating proxy publish API)
# 2. Querying messages via the mailbox query interface
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAILBOX_DB=".prism/mailbox-local.db"

echo "🧪 Mailbox Local Testing"
echo "========================"
echo ""

# Create .prism directory if it doesn't exist
mkdir -p .prism

# Clean up old database
if [ -f "$MAILBOX_DB" ]; then
    echo "🗑️  Removing old database: $MAILBOX_DB"
    rm -f "$MAILBOX_DB"
fi

# Test 1: Write test events directly to SQLite
echo "📝 Test 1: Writing test events to mailbox..."
echo ""

# Create test program that uses SQLite driver to write events
cat > /tmp/mailbox-write-test.go <<'EOF'
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/drivers/sqlite"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

func main() {
	ctx := context.Background()

	// Initialize SQLite driver
	driver := sqlite.New()
	config := &plugin.Config{
		Backend: map[string]interface{}{
			"database_path":  ".prism/mailbox-local.db",
			"table_name":     "mailbox",
			"retention_days": float64(30),
		},
	}

	if err := driver.Initialize(ctx, config); err != nil {
		panic(fmt.Sprintf("failed to initialize driver: %v", err))
	}

	if err := driver.Start(ctx); err != nil {
		panic(fmt.Sprintf("failed to start driver: %v", err))
	}
	defer driver.Stop(ctx)

	// Get table writer interface
	writer, ok := driver.(plugin.TableWriterInterface)
	if !ok {
		panic("driver does not implement TableWriterInterface")
	}

	// Write test events
	events := []*plugin.MailboxEvent{
		{
			MessageID:     "msg-001",
			Timestamp:     time.Now().UnixMilli(),
			Topic:         "test.events",
			ContentType:   "application/json",
			CorrelationID: "trace-abc-123",
			Principal:     "test-user",
			Namespace:     "local-mailbox",
			CustomHeaders: map[string]string{
				"x-test-id": "001",
				"x-source":  "local-test",
			},
			Body: []byte(`{"message": "Hello from test 1", "value": 42}`),
		},
		{
			MessageID:     "msg-002",
			Timestamp:     time.Now().UnixMilli(),
			Topic:         "test.events",
			ContentType:   "application/json",
			CorrelationID: "trace-abc-123",
			Principal:     "test-user",
			Namespace:     "local-mailbox",
			CustomHeaders: map[string]string{
				"x-test-id": "002",
				"x-source":  "local-test",
			},
			Body: []byte(`{"message": "Hello from test 2", "value": 100}`),
		},
		{
			MessageID:     "msg-003",
			Timestamp:     time.Now().UnixMilli(),
			Topic:         "test.events",
			ContentType:   "text/plain",
			CorrelationID: "trace-xyz-456",
			Principal:     "admin-user",
			Namespace:     "local-mailbox",
			CustomHeaders: map[string]string{
				"x-test-id": "003",
				"x-source":  "local-test",
			},
			Body: []byte("Plain text message for testing"),
		},
	}

	for _, event := range events {
		if err := writer.WriteEvent(ctx, event); err != nil {
			fmt.Printf("❌ Failed to write event %s: %v\n", event.MessageID, err)
		} else {
			fmt.Printf("✅ Wrote event: %s (topic: %s, principal: %s)\n",
				event.MessageID, event.Topic, event.Principal)
		}
	}

	// Get table stats
	reader, ok := driver.(plugin.TableReaderInterface)
	if !ok {
		panic("driver does not implement TableReaderInterface")
	}

	stats, err := reader.GetTableStats(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to get stats: %v", err))
	}

	fmt.Printf("\n📊 Mailbox Stats:\n")
	fmt.Printf("   Total Events: %d\n", stats.TotalEvents)
	fmt.Printf("   Total Size: %d bytes\n", stats.TotalSizeBytes)
	fmt.Printf("   Oldest Event: %s\n", time.UnixMilli(stats.OldestEvent).Format(time.RFC3339))
	fmt.Printf("   Newest Event: %s\n", time.UnixMilli(stats.NewestEvent).Format(time.RFC3339))
}
EOF

# Build and run the test program
echo "Building test program..."
cd "$SCRIPT_DIR"
go run /tmp/mailbox-write-test.go
echo ""

# Test 2: Query events using the table reader
echo "📖 Test 2: Querying events from mailbox..."
echo ""

cat > /tmp/mailbox-query-test.go <<'EOF'
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jrepp/prism-data-layer/pkg/drivers/sqlite"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

func main() {
	ctx := context.Background()

	// Initialize SQLite driver
	driver := sqlite.New()
	config := &plugin.Config{
		Backend: map[string]interface{}{
			"database_path":  ".prism/mailbox-local.db",
			"table_name":     "mailbox",
			"retention_days": float64(30),
		},
	}

	if err := driver.Initialize(ctx, config); err != nil {
		panic(fmt.Sprintf("failed to initialize driver: %v", err))
	}

	if err := driver.Start(ctx); err != nil {
		panic(fmt.Sprintf("failed to start driver: %v", err))
	}
	defer driver.Stop(ctx)

	// Get table reader interface
	reader, ok := driver.(plugin.TableReaderInterface)
	if !ok {
		panic("driver does not implement TableReaderInterface")
	}

	// Query all events
	filter := &plugin.EventFilter{
		Limit: 10,
	}

	events, err := reader.QueryEvents(ctx, filter)
	if err != nil {
		panic(fmt.Sprintf("failed to query events: %v", err))
	}

	fmt.Printf("Found %d event(s):\n\n", len(events))

	for i, event := range events {
		fmt.Printf("Event #%d:\n", i+1)
		fmt.Printf("  Message ID: %s\n", event.MessageID)
		fmt.Printf("  Topic: %s\n", event.Topic)
		fmt.Printf("  Principal: %s\n", event.Principal)
		fmt.Printf("  Correlation ID: %s\n", event.CorrelationID)
		fmt.Printf("  Content Type: %s\n", event.ContentType)

		if len(event.CustomHeaders) > 0 {
			fmt.Printf("  Custom Headers:\n")
			for k, v := range event.CustomHeaders {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}

		// Try to pretty-print JSON payloads
		if event.ContentType == "application/json" {
			var prettyJSON map[string]interface{}
			if err := json.Unmarshal(event.Body, &prettyJSON); err == nil {
				formatted, _ := json.MarshalIndent(prettyJSON, "    ", "  ")
				fmt.Printf("  Payload:\n    %s\n", string(formatted))
			} else {
				fmt.Printf("  Payload: %s\n", string(event.Body))
			}
		} else {
			fmt.Printf("  Payload: %s\n", string(event.Body))
		}

		fmt.Println()
	}

	// Query specific event by ID
	fmt.Println("🔍 Getting specific event by ID: msg-001")
	event, err := reader.GetEvent(ctx, "msg-001")
	if err != nil {
		fmt.Printf("❌ Failed to get event: %v\n", err)
	} else {
		fmt.Printf("✅ Retrieved event: %s\n", event.MessageID)
		fmt.Printf("   Payload: %s\n", string(event.Body))
	}
}
EOF

go run /tmp/mailbox-query-test.go

echo ""
echo "✅ Mailbox local testing complete!"
echo ""
echo "Database location: $MAILBOX_DB"
echo ""
echo "Next steps:"
echo "  1. Start prism-proxy with mailbox namespace configured"
echo "  2. Use 'prismctl publish message' to publish events"
echo "  3. Use 'prismctl mailbox query' to retrieve events"
