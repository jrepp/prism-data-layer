package backends

import "time"

// Identity represents a registered identity with metadata
type Identity struct {
	ID           string                 `json:"identity"`
	Metadata     map[string]interface{} `json:"metadata"`
	RegisteredAt time.Time              `json:"registered_at"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	TTL          time.Duration          `json:"ttl,omitempty"`
}

// Filter represents a metadata filter expression
type Filter struct {
	// Simple equality map for POC 4 (Week 1)
	// Example: {"status": "online", "room": "engineering"}
	Conditions map[string]interface{}

	// Advanced filter AST (Week 3)
	// Will be implemented in filter package
	AST interface{} `json:"ast,omitempty"`
}

// NewFilter creates a simple equality filter
func NewFilter(conditions map[string]interface{}) *Filter {
	return &Filter{
		Conditions: conditions,
	}
}

// Matches evaluates the filter against identity metadata (client-side)
func (f *Filter) Matches(metadata map[string]interface{}) bool {
	if f == nil || len(f.Conditions) == 0 {
		return true // No filter = match all
	}

	// Simple equality matching for POC 4
	for key, expectedValue := range f.Conditions {
		actualValue, exists := metadata[key]
		if !exists {
			return false
		}
		// TODO: Type-aware comparison (Week 3)
		if actualValue != expectedValue {
			return false
		}
	}

	return true
}
