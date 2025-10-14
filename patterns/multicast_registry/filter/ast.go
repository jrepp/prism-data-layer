package filter

// FilterNode represents a node in the filter expression AST
type FilterNode interface {
	// Evaluate returns true if the metadata matches the filter
	Evaluate(metadata map[string]interface{}) bool
}

// EqualityNode represents an equality comparison (field == value)
type EqualityNode struct {
	Field string
	Value interface{}
}

func (n *EqualityNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	return equals(actual, n.Value)
}

// NotEqualNode represents inequality comparison (field != value)
type NotEqualNode struct {
	Field string
	Value interface{}
}

func (n *NotEqualNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	return !equals(actual, n.Value)
}

// LessThanNode represents less than comparison (field < value)
type LessThanNode struct {
	Field string
	Value interface{}
}

func (n *LessThanNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	return lessThan(actual, n.Value)
}

// LessOrEqualNode represents less than or equal comparison (field <= value)
type LessOrEqualNode struct {
	Field string
	Value interface{}
}

func (n *LessOrEqualNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	return lessThan(actual, n.Value) || equals(actual, n.Value)
}

// GreaterThanNode represents greater than comparison (field > value)
type GreaterThanNode struct {
	Field string
	Value interface{}
}

func (n *GreaterThanNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	return greaterThan(actual, n.Value)
}

// GreaterOrEqualNode represents greater than or equal comparison (field >= value)
type GreaterOrEqualNode struct {
	Field string
	Value interface{}
}

func (n *GreaterOrEqualNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	return greaterThan(actual, n.Value) || equals(actual, n.Value)
}

// StartsWithNode represents string prefix matching (field.startswith value)
type StartsWithNode struct {
	Field  string
	Prefix string
}

func (n *StartsWithNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	str, ok := actual.(string)
	if !ok {
		return false
	}
	return len(str) >= len(n.Prefix) && str[:len(n.Prefix)] == n.Prefix
}

// EndsWithNode represents string suffix matching (field.endswith value)
type EndsWithNode struct {
	Field  string
	Suffix string
}

func (n *EndsWithNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	str, ok := actual.(string)
	if !ok {
		return false
	}
	return len(str) >= len(n.Suffix) && str[len(str)-len(n.Suffix):] == n.Suffix
}

// ContainsNode represents substring matching (field.contains value)
type ContainsNode struct {
	Field     string
	Substring string
}

func (n *ContainsNode) Evaluate(metadata map[string]interface{}) bool {
	actual, exists := metadata[n.Field]
	if !exists {
		return false
	}
	str, ok := actual.(string)
	if !ok {
		return false
	}
	// Simple substring search
	return contains(str, n.Substring)
}

// AndNode represents logical AND (all children must match)
type AndNode struct {
	Children []FilterNode
}

func (n *AndNode) Evaluate(metadata map[string]interface{}) bool {
	for _, child := range n.Children {
		if !child.Evaluate(metadata) {
			return false
		}
	}
	return true
}

// OrNode represents logical OR (at least one child must match)
type OrNode struct {
	Children []FilterNode
}

func (n *OrNode) Evaluate(metadata map[string]interface{}) bool {
	for _, child := range n.Children {
		if child.Evaluate(metadata) {
			return true
		}
	}
	return false
}

// NotNode represents logical NOT (negates child)
type NotNode struct {
	Child FilterNode
}

func (n *NotNode) Evaluate(metadata map[string]interface{}) bool {
	return !n.Child.Evaluate(metadata)
}

// ExistsNode checks if a field exists (regardless of value)
type ExistsNode struct {
	Field string
}

func (n *ExistsNode) Evaluate(metadata map[string]interface{}) bool {
	_, exists := metadata[n.Field]
	return exists
}

// Helper functions for type-aware comparisons

func equals(a, b interface{}) bool {
	// Type-specific equality
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case int64:
		bv, ok := b.(int64)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	default:
		// Fallback to interface{} equality
		return a == b
	}
}

func lessThan(a, b interface{}) bool {
	switch av := a.(type) {
	case int:
		bv, ok := b.(int)
		return ok && av < bv
	case int64:
		bv, ok := b.(int64)
		return ok && av < bv
	case float64:
		bv, ok := b.(float64)
		return ok && av < bv
	case string:
		bv, ok := b.(string)
		return ok && av < bv
	default:
		return false
	}
}

func greaterThan(a, b interface{}) bool {
	switch av := a.(type) {
	case int:
		bv, ok := b.(int)
		return ok && av > bv
	case int64:
		bv, ok := b.(int64)
		return ok && av > bv
	case float64:
		bv, ok := b.(float64)
		return ok && av > bv
	case string:
		bv, ok := b.(string)
		return ok && av > bv
	default:
		return false
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
