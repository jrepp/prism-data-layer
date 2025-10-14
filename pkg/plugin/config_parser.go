package plugin

import (
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

// ParseConfigFromStruct converts a protobuf Struct to a plugin Config.
// This handles the conversion from the dynamic protobuf structure to the
// strongly-typed Config format used by patterns.
func ParseConfigFromStruct(name, version string, pbStruct *structpb.Struct) (*Config, error) {
	if pbStruct == nil {
		return nil, fmt.Errorf("config struct is nil")
	}

	config := &Config{
		Plugin: PluginConfig{
			Name:    name,
			Version: version,
		},
		Backend: make(map[string]any),
	}

	fields := pbStruct.GetFields()

	// Parse slots configuration (if present)
	if slotsValue, ok := fields["slots"]; ok {
		slotsStruct := slotsValue.GetStructValue()
		if slotsStruct != nil {
			slotsMap := structToMap(slotsStruct)
			config.Backend["slots"] = slotsMap
		}
	}

	// Parse behavior configuration (if present)
	if behaviorValue, ok := fields["behavior"]; ok {
		behaviorStruct := behaviorValue.GetStructValue()
		if behaviorStruct != nil {
			behaviorMap := structToMap(behaviorStruct)
			config.Backend["behavior"] = behaviorMap
		}
	}

	// Parse any additional backend-specific fields
	for key, value := range fields {
		if key == "slots" || key == "behavior" {
			continue // Already handled
		}
		config.Backend[key] = structValueToAny(value)
	}

	return config, nil
}

// structToMap recursively converts a protobuf Struct to a map[string]interface{}.
func structToMap(s *structpb.Struct) map[string]interface{} {
	if s == nil {
		return nil
	}

	result := make(map[string]interface{})
	for key, value := range s.GetFields() {
		result[key] = structValueToAny(value)
	}
	return result
}

// structValueToAny converts a protobuf Value to a Go interface{}.
func structValueToAny(v *structpb.Value) interface{} {
	if v == nil {
		return nil
	}

	switch kind := v.GetKind().(type) {
	case *structpb.Value_NullValue:
		return nil
	case *structpb.Value_NumberValue:
		return kind.NumberValue
	case *structpb.Value_StringValue:
		return kind.StringValue
	case *structpb.Value_BoolValue:
		return kind.BoolValue
	case *structpb.Value_StructValue:
		return structToMap(kind.StructValue)
	case *structpb.Value_ListValue:
		list := kind.ListValue.GetValues()
		result := make([]interface{}, len(list))
		for i, item := range list {
			result[i] = structValueToAny(item)
		}
		return result
	default:
		return nil
	}
}

// BuildConfigStruct creates a protobuf Struct from a map (for testing/client use).
// This is the inverse operation of structToMap.
func BuildConfigStruct(config map[string]interface{}) (*structpb.Struct, error) {
	return structpb.NewStruct(config)
}
