package servicedesk

import (
	"encoding/json"
	"testing"
)

func TestEvaluateConditionsOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		operator string
		value    any
		actual   any
		want     bool
	}{
		{name: "equal", operator: "==", value: "ready", actual: "ready", want: true},
		{name: "not equal", operator: "!=", value: "ready", actual: "blocked", want: true},
		{name: "in", operator: "in", value: []any{"ready", "queued"}, actual: "queued", want: true},
		{name: "not in", operator: "notin", value: []any{"ready", "queued"}, actual: "blocked", want: true},
		{name: "greater", operator: ">", value: json.Number("2"), actual: json.Number("3"), want: true},
		{name: "less", operator: "<", value: json.Number("4"), actual: json.Number("3"), want: true},
		{name: "greater equal", operator: ">=", value: json.Number("3"), actual: json.Number("3"), want: true},
		{name: "less equal", operator: "<=", value: json.Number("3"), actual: json.Number("3"), want: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			field := Field{BackendKey: "f_dependent", StableID: "dependent", Name: "Dependent", Type: "input", Supported: true, Conditions: ConditionSet{Operator: "and", Conditions: []Condition{{Field: "f_actual", Operator: test.operator, Value: test.value}}}}
			template := Template{Fields: []Field{field}}
			result := EvaluateConditions(template, NormalizedInput{Inputs: map[string]any{"f_actual": test.actual}})
			if result.Active["dependent"] != test.want {
				t.Fatalf("active = %#v, want %v", result.Active, test.want)
			}
			if result.ConditionalFieldCount != 1 || result.ActiveConditionalFieldCount != boolToInt(test.want) || result.UnsupportedConditionCount != 0 {
				t.Fatalf("condition counts = %+v", result)
			}
		})
	}
}

func TestEvaluateConditionsNestedAndOr(t *testing.T) {
	t.Parallel()

	template := Template{Fields: []Field{{
		BackendKey: "f_result", StableID: "result", Name: "Result", Type: "input", Supported: true,
		Conditions: ConditionSet{Operator: "or", Conditions: []Condition{
			{Operator: "and", Conditions: []Condition{
				{Field: "a", Operator: "==", Value: "yes"},
				{Field: "b", Operator: "==", Value: json.Number("2")},
			}},
			{Field: "c", Operator: "==", Value: "fallback"},
		}},
	}}}
	result := EvaluateConditions(template, NormalizedInput{Inputs: map[string]any{"a": "yes", "b": json.Number("2")}})
	if !result.Active["result"] || result.UnsupportedConditionCount != 0 {
		t.Fatalf("nested result = %+v", result)
	}
}

func TestEvaluateConditionsInactiveAndActiveRequiredFields(t *testing.T) {
	t.Parallel()

	template := Template{Fields: []Field{
		{BackendKey: "mode", StableID: "mode", Name: "Mode", Type: "input", Supported: true},
		{BackendKey: "details", StableID: "details", Name: "Details", Type: "input", Required: true, Supported: true, Conditions: ConditionSet{Operator: "and", Conditions: []Condition{{Field: "mode", Operator: "==", Value: "active"}}}},
	}}
	inactive := EvaluateConditions(template, NormalizedInput{Inputs: map[string]any{"mode": "inactive"}})
	if inactive.Active["details"] || len(inactive.Blocking) != 0 {
		t.Fatalf("inactive result = %+v, want no blocking issue", inactive)
	}
	active := EvaluateConditions(template, NormalizedInput{Inputs: map[string]any{"mode": "active"}})
	if !active.Active["details"] || !hasFieldIssue(active.Blocking, "missing_required", "details") {
		t.Fatalf("active result = %+v, want missing required issue", active)
	}
}

func TestEvaluateConditionsUnsupportedConsequentialConditionBlocks(t *testing.T) {
	t.Parallel()

	template := Template{Fields: []Field{{
		BackendKey: "details", StableID: "details", Name: "Details", Type: "input", Required: true, Supported: true,
		Conditions: ConditionSet{Operator: "and", Conditions: []Condition{{Field: "mode", Operator: "contains", Value: "active"}}},
	}}}
	result := EvaluateConditions(template, NormalizedInput{Inputs: map[string]any{"mode": "active"}})
	if result.UnsupportedConditionCount != 1 || len(result.Blocking) == 0 || !hasFieldIssue(result.Blocking, "unsupported_condition", "details") {
		t.Fatalf("unsupported condition result = %+v", result)
	}
}

func TestEvaluateConditionsUnsupportedLiveExpressionBlocks(t *testing.T) {
	t.Parallel()

	template := Template{Fields: []Field{{
		BackendKey: "details", StableID: "details", Name: "Details", Type: "input", Supported: true,
		Conditions: ConditionSet{Unsupported: true},
	}}}
	result := EvaluateConditions(template, NormalizedInput{})
	if result.ConditionalFieldCount != 1 || result.UnsupportedConditionCount != 1 || !hasFieldIssue(result.Blocking, "unsupported_condition", "details") {
		t.Fatalf("unsupported live expression result = %+v", result)
	}
}

func TestEvaluateConditionsUsesOverlayPublicNameAndRequiredStrengthening(t *testing.T) {
	t.Parallel()
	template := Template{Fields: []Field{
		{BackendKey: "mode_key", StableID: "mode", Name: "Mode", Type: "input", Supported: true},
		{BackendKey: "details_key", StableID: "details", Name: "Details", Type: "input", Supported: true, Conditions: ConditionSet{Operator: "and", Conditions: []Condition{{Field: "mode_key", Operator: "==", Value: "active"}}}},
	}}
	normalized := NormalizedInput{
		Inputs:      map[string]any{"mode_key": "active"},
		required:    map[string]bool{"details_key": true},
		publicNames: map[string]string{"details_key": "failureDetails"},
	}
	result := EvaluateConditions(template, normalized)
	if !hasFieldIssue(result.Blocking, "missing_required", "failureDetails") {
		t.Fatalf("result = %+v, want strengthened public required issue", result)
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
