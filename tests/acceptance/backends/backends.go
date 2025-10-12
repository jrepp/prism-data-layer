// Package backends provides backend implementations for the Prism acceptance test framework.
//
// Import this package to automatically register all available backends:
//
//	import _ "github.com/jrepp/prism-data-layer/tests/acceptance/backends"
//
// Each backend file (memstore.go, redis.go, etc.) registers itself via init() functions.
// Tests in patterns/ automatically discover and run against all registered backends.
package backends

// This file exists to provide package documentation.
// Actual backend registrations are in individual files:
//   - memstore.go: In-memory KeyValue backend
//   - redis.go: Redis KeyValue backend
//   - nats.go: NATS PubSub backend
//   - postgres.go: PostgreSQL KeyValue backend (TODO)
//   - kafka.go: Kafka PubSub/Queue backend (TODO)
