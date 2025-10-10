// Package backends provides testcontainer-based backend setup utilities
// for integration testing across different patterns.
//
// This package centralizes backend dependency management as a cross-cutting
// concern, making it easy to:
// - Start backend containers (Redis, NATS, etc.)
// - Get connection strings with proper formatting
// - Clean up resources after tests
//
// Example usage:
//
//	func TestMyPattern(t *testing.T) {
//	    ctx := context.Background()
//	    backend := backends.SetupRedis(t, ctx)
//	    defer backend.Cleanup()
//
//	    // Use backend.ConnectionString for pattern configuration
//	    config := &core.Config{
//	        Backend: map[string]any{
//	            "address": backend.ConnectionString,
//	        },
//	    }
//	}
package backends
