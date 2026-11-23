package servicedesk

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type ConditionResult struct {
	ConditionalFieldCount       int
	ActiveConditionalFieldCount int
	UnsupportedConditionCount   int
	Active                      map[string]bool
	Blocking                    []FieldIssue
}

func EvaluateConditions(template Template, normalized NormalizedInput) ConditionResult {
	result := ConditionResult{Active: make(map[string]bool)}
	lookup := make(map[string]any, len(normalized.Inputs)*2)
	for _, field := range template.Fields {
		value, exists := normalized.Inputs[field.BackendKey]
		if !exists {
			continue
		}
		for _, key := range []string{field.BackendKey, field.StableID, field.Name} {
			if key != "" {
				lookup[key] = value
			}
		}
	}
	for key, value := range normalized.Inputs {
		lookup[key] = value
	}
	for _, field := range template.Fields {
		if !field.Conditions.Unsupported && len(field.Conditions.Conditions) == 0 {
			continue
		}
		result.ConditionalFieldCount++
		if field.Conditions.Unsupported {
			name := preferredFieldName(field)
			if normalized.publicNames != nil && normalized.publicNames[field.BackendKey] != "" {
				name = normalized.publicNames[field.BackendKey]
			}
			result.UnsupportedConditionCount++
			result.Blocking = append(result.Blocking, FieldIssue{Code: "unsupported_condition", Field: name, Message: "field visibility or requiredness depends on an unsupported condition"})
			continue
		}
		active, unsupported := evaluateConditionSet(field.Conditions, lookup)
		name := preferredFieldName(field)
		if normalized.publicNames != nil && normalized.publicNames[field.BackendKey] != "" {
			name = normalized.publicNames[field.BackendKey]
		}
		result.Active[name] = active
		if active {
			result.ActiveConditionalFieldCount++
		}
		if unsupported {
			result.UnsupportedConditionCount++
			result.Blocking = append(result.Blocking, FieldIssue{Code: "unsupported_condition", Field: name, Message: "field visibility or requiredness depends on an unsupported condition"})
			continue
		}
		required := field.Required
		if normalized.required != nil {
			if effective, ok := normalized.required[field.BackendKey]; ok {
				required = effective
			}
		}
		if active && required {
			if _, present := normalized.Inputs[field.BackendKey]; !present {
				result.Blocking = append(result.Blocking, FieldIssue{Code: "missing_required", Field: name, Message: "active required field is missing"})
			}
		}
	}
	return result
}

func evaluateConditionSet(set ConditionSet, values map[string]any) (bool, bool) {
	operator := strings.ToLower(strings.TrimSpace(set.Operator))
	if operator == "" {
		operator = "and"
	}
	return evaluateConditionGroup(operator, set.Conditions, values)
}

func evaluateConditionGroup(operator string, conditions []Condition, values map[string]any) (bool, bool) {
	if len(conditions) == 0 {
		return true, false
	}
	if operator != "and" && operator != "or" {
		return false, true
	}
	result := operator == "and"
	unsupported := false
	for _, condition := range conditions {
		active, unknown := evaluateCondition(condition, values)
		unsupported = unsupported || unknown
		if operator == "and" {
			result = result && active
		} else {
			result = result || active
		}
	}
	return result, unsupported
}

func evaluateCondition(condition Condition, values map[string]any) (bool, bool) {
	if len(condition.Conditions) > 0 {
		return evaluateConditionGroup(strings.ToLower(strings.TrimSpace(condition.Operator)), condition.Conditions, values)
	}
	actual, exists := values[condition.Field]
	if !exists {
		return false, false
	}
	operator := strings.ToLower(strings.TrimSpace(condition.Operator))
	switch operator {
	case "==":
		return equalCondition(actual, condition.Value), false
	case "!=":
		return !equalCondition(actual, condition.Value), false
	case "in", "notin":
		values, ok := condition.Value.([]any)
		if !ok {
			return false, true
		}
		found := false
		for _, candidate := range values {
			if equalCondition(actual, candidate) {
				found = true
				break
			}
		}
		if operator == "notin" {
			return !found, false
		}
		return found, false
	case ">", "<", ">=", "<=":
		comparison, ok := compareCondition(actual, condition.Value)
		if !ok {
			return false, true
		}
		switch operator {
		case ">":
			return comparison > 0, false
		case "<":
			return comparison < 0, false
		case ">=":
			return comparison >= 0, false
		default:
			return comparison <= 0, false
		}
	default:
		return false, true
	}
}

func equalCondition(left, right any) bool {
	if leftNumber, ok := numberCondition(left); ok {
		if rightNumber, ok := numberCondition(right); ok {
			return leftNumber == rightNumber
		}
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func compareCondition(left, right any) (int, bool) {
	if leftNumber, ok := numberCondition(left); ok {
		if rightNumber, ok := numberCondition(right); ok {
			if math.Abs(leftNumber-rightNumber) < 1e-12 {
				return 0, true
			}
			if leftNumber < rightNumber {
				return -1, true
			}
			return 1, true
		}
	}
	leftText, leftOK := left.(string)
	rightText, rightOK := right.(string)
	if leftOK && rightOK {
		if leftText < rightText {
			return -1, true
		}
		if leftText > rightText {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func numberCondition(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case float64:
		return number, true
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}
