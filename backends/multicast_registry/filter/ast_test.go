package filter

import (
	"testing"
)

// Test 1: EqualityNode with matching value
func TestEqualityNode_Match(t *testing.T) {
	node := &EqualityNode{Field: "status", Value: "online"}
	metadata := map[string]interface{}{"status": "online"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for status=online")
	}
}

// Test 2: EqualityNode with non-matching value
func TestEqualityNode_NoMatch(t *testing.T) {
	node := &EqualityNode{Field: "status", Value: "online"}
	metadata := map[string]interface{}{"status": "offline"}

	if node.Evaluate(metadata) {
		t.Error("Expected false for status!=online")
	}
}

// Test 3: EqualityNode with missing field
func TestEqualityNode_MissingField(t *testing.T) {
	node := &EqualityNode{Field: "status", Value: "online"}
	metadata := map[string]interface{}{}

	if node.Evaluate(metadata) {
		t.Error("Expected false for missing field")
	}
}

// Test 4: NotEqualNode with different value
func TestNotEqualNode_Match(t *testing.T) {
	node := &NotEqualNode{Field: "status", Value: "offline"}
	metadata := map[string]interface{}{"status": "online"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for status!=offline")
	}
}

// Test 5: NotEqualNode with same value
func TestNotEqualNode_NoMatch(t *testing.T) {
	node := &NotEqualNode{Field: "status", Value: "online"}
	metadata := map[string]interface{}{"status": "online"}

	if node.Evaluate(metadata) {
		t.Error("Expected false for status==online")
	}
}

// Test 6: LessThanNode with int
func TestLessThanNode_Int(t *testing.T) {
	node := &LessThanNode{Field: "age", Value: 30}
	metadata := map[string]interface{}{"age": 25}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for age=25 < 30")
	}
}

// Test 7: LessThanNode with float64
func TestLessThanNode_Float(t *testing.T) {
	node := &LessThanNode{Field: "temperature", Value: 30.5}
	metadata := map[string]interface{}{"temperature": 25.3}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for temperature=25.3 < 30.5")
	}
}

// Test 8: LessThanNode false case
func TestLessThanNode_False(t *testing.T) {
	node := &LessThanNode{Field: "age", Value: 20}
	metadata := map[string]interface{}{"age": 30}

	if node.Evaluate(metadata) {
		t.Error("Expected false for age=30 < 20")
	}
}

// Test 9: LessOrEqualNode with equal values
func TestLessOrEqualNode_Equal(t *testing.T) {
	node := &LessOrEqualNode{Field: "count", Value: 10}
	metadata := map[string]interface{}{"count": 10}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for count=10 <= 10")
	}
}

// Test 10: LessOrEqualNode with less value
func TestLessOrEqualNode_Less(t *testing.T) {
	node := &LessOrEqualNode{Field: "count", Value: 10}
	metadata := map[string]interface{}{"count": 5}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for count=5 <= 10")
	}
}

// Test 11: GreaterThanNode with int
func TestGreaterThanNode_Int(t *testing.T) {
	node := &GreaterThanNode{Field: "score", Value: 50}
	metadata := map[string]interface{}{"score": 75}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for score=75 > 50")
	}
}

// Test 12: GreaterOrEqualNode with equal values
func TestGreaterOrEqualNode_Equal(t *testing.T) {
	node := &GreaterOrEqualNode{Field: "version", Value: 2}
	metadata := map[string]interface{}{"version": 2}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for version=2 >= 2")
	}
}

// Test 13: StartsWithNode matching prefix
func TestStartsWithNode_Match(t *testing.T) {
	node := &StartsWithNode{Field: "name", Prefix: "alice"}
	metadata := map[string]interface{}{"name": "alice-session-1"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for name startswith alice")
	}
}

// Test 14: StartsWithNode non-matching prefix
func TestStartsWithNode_NoMatch(t *testing.T) {
	node := &StartsWithNode{Field: "name", Prefix: "bob"}
	metadata := map[string]interface{}{"name": "alice-session-1"}

	if node.Evaluate(metadata) {
		t.Error("Expected false for name does not start with bob")
	}
}

// Test 15: StartsWithNode with non-string field
func TestStartsWithNode_NonString(t *testing.T) {
	node := &StartsWithNode{Field: "age", Prefix: "30"}
	metadata := map[string]interface{}{"age": 30}

	if node.Evaluate(metadata) {
		t.Error("Expected false for non-string field")
	}
}

// Test 16: EndsWithNode matching suffix
func TestEndsWithNode_Match(t *testing.T) {
	node := &EndsWithNode{Field: "filename", Suffix: ".txt"}
	metadata := map[string]interface{}{"filename": "document.txt"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for filename endswith .txt")
	}
}

// Test 17: EndsWithNode non-matching suffix
func TestEndsWithNode_NoMatch(t *testing.T) {
	node := &EndsWithNode{Field: "filename", Suffix: ".pdf"}
	metadata := map[string]interface{}{"filename": "document.txt"}

	if node.Evaluate(metadata) {
		t.Error("Expected false for filename does not end with .pdf")
	}
}

// Test 18: ContainsNode matching substring
func TestContainsNode_Match(t *testing.T) {
	node := &ContainsNode{Field: "description", Substring: "urgent"}
	metadata := map[string]interface{}{"description": "This is an urgent message"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for description contains urgent")
	}
}

// Test 19: ContainsNode non-matching substring
func TestContainsNode_NoMatch(t *testing.T) {
	node := &ContainsNode{Field: "description", Substring: "critical"}
	metadata := map[string]interface{}{"description": "This is an urgent message"}

	if node.Evaluate(metadata) {
		t.Error("Expected false for description does not contain critical")
	}
}

// Test 20: AndNode with all children matching
func TestAndNode_AllMatch(t *testing.T) {
	node := &AndNode{
		Children: []FilterNode{
			&EqualityNode{Field: "status", Value: "online"},
			&EqualityNode{Field: "room", Value: "engineering"},
		},
	}
	metadata := map[string]interface{}{
		"status": "online",
		"room":   "engineering",
	}

	if !node.Evaluate(metadata) {
		t.Error("Expected true when all children match")
	}
}

// Test 21: AndNode with one child not matching
func TestAndNode_OneNoMatch(t *testing.T) {
	node := &AndNode{
		Children: []FilterNode{
			&EqualityNode{Field: "status", Value: "online"},
			&EqualityNode{Field: "room", Value: "sales"},
		},
	}
	metadata := map[string]interface{}{
		"status": "online",
		"room":   "engineering",
	}

	if node.Evaluate(metadata) {
		t.Error("Expected false when one child doesn't match")
	}
}

// Test 22: OrNode with one child matching
func TestOrNode_OneMatch(t *testing.T) {
	node := &OrNode{
		Children: []FilterNode{
			&EqualityNode{Field: "status", Value: "online"},
			&EqualityNode{Field: "status", Value: "away"},
		},
	}
	metadata := map[string]interface{}{"status": "online"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true when one child matches")
	}
}

// Test 23: OrNode with no children matching
func TestOrNode_NoMatch(t *testing.T) {
	node := &OrNode{
		Children: []FilterNode{
			&EqualityNode{Field: "status", Value: "online"},
			&EqualityNode{Field: "status", Value: "away"},
		},
	}
	metadata := map[string]interface{}{"status": "offline"}

	if node.Evaluate(metadata) {
		t.Error("Expected false when no children match")
	}
}

// Test 24: NotNode negating true
func TestNotNode_NegateTrue(t *testing.T) {
	node := &NotNode{
		Child: &EqualityNode{Field: "status", Value: "online"},
	}
	metadata := map[string]interface{}{"status": "online"}

	if node.Evaluate(metadata) {
		t.Error("Expected false when negating true")
	}
}

// Test 25: NotNode negating false
func TestNotNode_NegateFalse(t *testing.T) {
	node := &NotNode{
		Child: &EqualityNode{Field: "status", Value: "offline"},
	}
	metadata := map[string]interface{}{"status": "online"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true when negating false")
	}
}

// Test 26: ExistsNode with existing field
func TestExistsNode_Exists(t *testing.T) {
	node := &ExistsNode{Field: "timestamp"}
	metadata := map[string]interface{}{"timestamp": "2025-10-11T10:00:00Z"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for existing field")
	}
}

// Test 27: ExistsNode with missing field
func TestExistsNode_Missing(t *testing.T) {
	node := &ExistsNode{Field: "timestamp"}
	metadata := map[string]interface{}{"status": "online"}

	if node.Evaluate(metadata) {
		t.Error("Expected false for missing field")
	}
}

// Test 28: Complex nested AND/OR
func TestComplexNested_AndOr(t *testing.T) {
	// (status=online OR status=away) AND room=engineering
	node := &AndNode{
		Children: []FilterNode{
			&OrNode{
				Children: []FilterNode{
					&EqualityNode{Field: "status", Value: "online"},
					&EqualityNode{Field: "status", Value: "away"},
				},
			},
			&EqualityNode{Field: "room", Value: "engineering"},
		},
	}

	metadata := map[string]interface{}{
		"status": "away",
		"room":   "engineering",
	}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for (status=away) AND room=engineering")
	}
}

// Test 29: Type mismatch in comparison
func TestTypeMismatch_IntString(t *testing.T) {
	node := &EqualityNode{Field: "age", Value: "30"}
	metadata := map[string]interface{}{"age": 30}

	if node.Evaluate(metadata) {
		t.Error("Expected false for type mismatch (int vs string)")
	}
}

// Test 30: String comparison lexicographic
func TestLessThan_StringLexicographic(t *testing.T) {
	node := &LessThanNode{Field: "version", Value: "2.0.0"}
	metadata := map[string]interface{}{"version": "1.9.5"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for version 1.9.5 < 2.0.0 (lexicographic)")
	}
}

// Test 31: Equality with bool type
func TestEquality_Bool(t *testing.T) {
	nodeTrue := &EqualityNode{Field: "active", Value: true}
	nodeFalse := &EqualityNode{Field: "active", Value: false}

	metadata := map[string]interface{}{"active": true}

	if !nodeTrue.Evaluate(metadata) {
		t.Error("Expected true for active=true")
	}

	if nodeFalse.Evaluate(metadata) {
		t.Error("Expected false for active=false when actual is true")
	}
}

// Test 32: Equality with int64 type
func TestEquality_Int64(t *testing.T) {
	node := &EqualityNode{Field: "timestamp", Value: int64(1234567890)}
	metadata := map[string]interface{}{"timestamp": int64(1234567890)}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for int64 timestamp equality")
	}
}

// Test 33: GreaterThan with int64
func TestGreaterThan_Int64(t *testing.T) {
	node := &GreaterThanNode{Field: "size", Value: int64(1000)}
	metadata := map[string]interface{}{"size": int64(2000)}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for size=2000 > 1000 (int64)")
	}
}

// Test 34: GreaterThan with float64
func TestGreaterThan_Float64(t *testing.T) {
	node := &GreaterThanNode{Field: "price", Value: 99.99}
	metadata := map[string]interface{}{"price": 149.99}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for price=149.99 > 99.99 (float64)")
	}
}

// Test 35: GreaterThan with string (lexicographic)
func TestGreaterThan_String(t *testing.T) {
	node := &GreaterThanNode{Field: "name", Value: "alice"}
	metadata := map[string]interface{}{"name": "bob"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for name=bob > alice (lexicographic)")
	}
}

// Test 36: LessThan with int64
func TestLessThan_Int64(t *testing.T) {
	node := &LessThanNode{Field: "offset", Value: int64(10000)}
	metadata := map[string]interface{}{"offset": int64(5000)}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for offset=5000 < 10000 (int64)")
	}
}

// Test 37: GreaterThan with unsupported type (should return false)
func TestGreaterThan_UnsupportedType(t *testing.T) {
	node := &GreaterThanNode{Field: "data", Value: []int{1, 2, 3}}
	metadata := map[string]interface{}{"data": []int{4, 5, 6}}

	if node.Evaluate(metadata) {
		t.Error("Expected false for unsupported type comparison")
	}
}

// Test 38: LessThan with unsupported type (should return false)
func TestLessThan_UnsupportedType(t *testing.T) {
	node := &LessThanNode{Field: "data", Value: []int{1, 2, 3}}
	metadata := map[string]interface{}{"data": []int{4, 5, 6}}

	if node.Evaluate(metadata) {
		t.Error("Expected false for unsupported type comparison")
	}
}

// Test 39: Equality with nil values (default case)
func TestEquality_NilValues(t *testing.T) {
	node := &EqualityNode{Field: "nullable", Value: nil}
	metadata := map[string]interface{}{"nullable": nil}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for nil == nil (default case)")
	}
}

// Test 40: Contains with empty substring
func TestContains_EmptySubstring(t *testing.T) {
	node := &ContainsNode{Field: "text", Substring: ""}
	metadata := map[string]interface{}{"text": "hello world"}

	if !node.Evaluate(metadata) {
		t.Error("Expected true for empty substring (always matches)")
	}
}
