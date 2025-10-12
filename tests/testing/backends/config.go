package backends

import (
	"os"
)

// QuietMode controls whether testcontainers logs are suppressed
var QuietMode = false

func init() {
	// Check environment variable for quiet mode
	if os.Getenv("PRISM_TEST_QUIET") != "" {
		QuietMode = true
	}

	// Note: testcontainers logger configuration removed as API changed
	// Use TESTCONTAINERS_RYUK_DISABLED=true or other env vars for quieter operation
}

// SetQuietMode enables or disables quiet mode for testcontainers
func SetQuietMode(enabled bool) {
	QuietMode = enabled
	// Note: testcontainers logger configuration removed as API changed
	// Logging behavior can be controlled via testcontainers environment variables
}
