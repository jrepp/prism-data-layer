package backends

import (
	"io"
	"log"
	"os"

	"github.com/testcontainers/testcontainers-go"
)

// QuietMode controls whether testcontainers logs are suppressed
var QuietMode = false

func init() {
	// Check environment variable for quiet mode
	if os.Getenv("PRISM_TEST_QUIET") != "" {
		QuietMode = true
	}

	// Suppress testcontainers logs if quiet mode is enabled
	if QuietMode {
		testcontainers.Logger = log.New(io.Discard, "", log.LstdFlags)
	}
}

// SetQuietMode enables or disables quiet mode for testcontainers
func SetQuietMode(enabled bool) {
	QuietMode = enabled
	if enabled {
		testcontainers.Logger = log.New(io.Discard, "", log.LstdFlags)
	} else {
		testcontainers.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}
}
