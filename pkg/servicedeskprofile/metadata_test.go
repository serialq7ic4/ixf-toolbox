package servicedeskprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNormalizeVerifiedLadingFixture(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}

	if template.Target != (EditTarget{BusinessID: 15, ServiceID: 4003}) || template.SnapshotID != "218527" {
		t.Fatalf("template target/snapshot = %+v/%q", template.Target, template.SnapshotID)
	}
	if template.ServiceKey != "request" || template.ServiceName != "Fixture Server Hardware Repair" {
		t.Fatalf("template service = %q/%q", template.ServiceKey, template.ServiceName)
	}
	if len(template.Fields) != 4 {
		t.Fatalf("field count = %d, want 4", len(template.Fields))
	}

	title := template.Fields[3]
	if title.BackendKey != "field_title_1001" || title.StableID != "title" || title.Name != "Title" || title.Type != "input" {
		t.Fatalf("title field = %+v", title)
	}
	if !title.Required || !title.DefaultAvailable || title.DefaultValue != "" || !title.Supported {
		t.Fatalf("title flags/default = %+v", title)
	}
	if !strings.HasPrefix(template.MetadataHash, "sha256:") || len(template.MetadataHash) != len("sha256:")+64 {
		t.Fatalf("metadata hash = %q", template.MetadataHash)
	}
}

func TestNormalizeServiceDeskWireSchema(t *testing.T) {
	t.Parallel()

	lading := map[string]any{
		"field_title": map[string]any{
			"field_unique_Identifier": "title",
			"field_name":              "Title",
			"field_type":              "input",
			"field_required":          "REQUIRE",
			"field_default":           "",
			"field_choice":            map[string]any{},
		},
		"field_devices": map[string]any{
			"field_unique_Identifier": "devices",
			"field_name":              "Affected devices",
			"field_type":              "extend_table",
			"field_required":          "OPTION",
			"field_default":           "",
			"field_choice": map[string]any{
				"2": map[string]any{"name": "Component", "key": "component", "isRequest": false},
				"1": map[string]any{"name": "Host name", "key": "host_name", "isRequest": true},
			},
		},
		"field_fault": map[string]any{
			"field_unique_Identifier": "fault_type",
			"field_name":              "Fault type",
			"field_type":              "select",
			"field_required":          "REQUIRE",
			"field_default":           "disk",
			"field_choice": map[string]any{
				"2": map[string]any{"name": "Memory", "key": "memory"},
				"1": map[string]any{"name": "Disk", "key": "disk"},
			},
		},
	}

	template := loadSyntheticTemplate(t, lading)
	fields := fieldsByStableIDOrName(template.Fields)

	title := fields["title"]
	if title.Name != "Title" || title.Type != "input" || !title.Required || !title.Supported {
		t.Fatalf("live title field = %+v", title)
	}
	if title.DefaultAvailable {
		t.Fatalf("blank live default must not satisfy a required field: %+v", title)
	}

	devices := fields["devices"]
	if devices.Name != "Affected devices" || devices.Type != "extend_table" || devices.Required || len(devices.TableColumns) != 2 {
		t.Fatalf("live table field = %+v", devices)
	}
	if got := devices.TableColumns[0]; got.BackendKey != "host_name" || got.Name != "Host name" || got.Type != "input" || !got.Required {
		t.Fatalf("first live table column = %+v", got)
	}
	if got := devices.TableColumns[1]; got.BackendKey != "component" || got.Name != "Component" || got.Type != "input" || got.Required {
		t.Fatalf("second live table column = %+v", got)
	}

	fault := fields["fault_type"]
	wantChoices := []Choice{{Label: "Disk", Value: "disk"}, {Label: "Memory", Value: "memory"}}
	if fault.Name != "Fault type" || fault.Type != "select" || !fault.Required || fmt.Sprint(fault.Choices) != fmt.Sprint(wantChoices) {
		t.Fatalf("live choice field = %+v, want choices %#v", fault, wantChoices)
	}
	if !fault.DefaultAvailable || fault.DefaultValue != "disk" {
		t.Fatalf("live choice default = %+v", fault)
	}
}

func TestNormalizeRejectsIncompleteServiceDeskWireField(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		field map[string]any
	}{
		{
			name: "missing name and type",
			field: map[string]any{
				"field_unique_Identifier": "title",
				"field_required":          "REQUIRE",
			},
		},
		{
			name: "unknown required marker",
			field: map[string]any{
				"field_unique_Identifier": "title",
				"field_name":              "Title",
				"field_type":              "input",
				"field_required":          "MAYBE",
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(test.field)
			if err != nil {
				t.Fatalf("encode field: %v", err)
			}
			_, err = normalizeLading(Lading{"field_title": raw})
			if err == nil {
				t.Fatal("normalizeLading() error = nil, want invalid response")
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Code != "invalid_response" {
				t.Fatalf("normalizeLading() error = %v, want invalid_response", err)
			}
		})
	}
}

func TestNormalizeNestedTableAndChoices(t *testing.T) {
	t.Parallel()

	lading := map[string]any{
		"field_devices": map[string]any{
			"field_unique_Identifier": "devices",
			"name":                    "Affected devices",
			"type":                    "extend_table",
			"required":                true,
			"columns": []any{
				map[string]any{
					"key":      "component",
					"name":     "Component",
					"type":     "select",
					"required": true,
					"choices": []any{
						map[string]any{"label": "Disk", "value": "disk"},
						map[string]any{"label": "Memory", "value": "memory"},
					},
				},
			},
		},
	}
	template := loadSyntheticTemplate(t, lading)
	if len(template.Fields) != 1 || len(template.Fields[0].TableColumns) != 1 {
		t.Fatalf("normalized table = %+v", template.Fields)
	}

	column := template.Fields[0].TableColumns[0]
	if column.BackendKey != "component" || column.Name != "Component" || column.Type != "select" || !column.Required {
		t.Fatalf("table column = %+v", column)
	}
	wantChoices := []Choice{{Label: "Disk", Value: "disk"}, {Label: "Memory", Value: "memory"}}
	if fmt.Sprint(column.Choices) != fmt.Sprint(wantChoices) {
		t.Fatalf("column choices = %#v, want %#v", column.Choices, wantChoices)
	}
	if !strings.HasPrefix(column.ChoiceHash, "sha256:") || len(column.ChoiceHash) != len("sha256:")+64 {
		t.Fatalf("column choice hash = %q", column.ChoiceHash)
	}
}

func TestNormalizeTableColumnDefaultsAndConditions(t *testing.T) {
	t.Parallel()

	lading := map[string]any{
		"field_devices": map[string]any{
			"field_unique_Identifier": "devices",
			"name":                    "Affected devices",
			"type":                    "extend_table",
			"required":                true,
			"columns": []any{
				map[string]any{
					"key":      "component",
					"name":     "Component",
					"type":     "input",
					"required": true,
					"default":  "disk",
					"conditions_expression": map[string]any{
						"operator": "and",
						"conditions": []any{
							map[string]any{"field": "other", "operator": "==", "value": "enabled"},
						},
					},
				},
			},
		},
	}
	template := loadSyntheticTemplate(t, lading)
	column := template.Fields[0].TableColumns[0]
	if !column.DefaultAvailable || column.DefaultValue != "disk" {
		t.Fatalf("table column default = %+v", column)
	}
	if column.Conditions.Operator != "and" || len(column.Conditions.Conditions) != 1 {
		t.Fatalf("table column conditions = %+v", column.Conditions)
	}
}

func TestNormalizeDefaultsAndDisplayOnlyFields(t *testing.T) {
	t.Parallel()

	lading := map[string]any{
		"field_count": map[string]any{
			"field_unique_Identifier": "count",
			"name":                    "Count",
			"type":                    "number",
			"required":                false,
			"default":                 0,
		},
		"field_enabled": map[string]any{
			"field_unique_Identifier": "enabled",
			"name":                    "Enabled",
			"type":                    "switch",
			"required":                false,
			"default":                 false,
		},
		"field_help": map[string]any{
			"name":     "Help",
			"type":     "hyperlink",
			"required": false,
			"default":  "https://example.invalid/help",
		},
		"field_optional": map[string]any{
			"name":     "Optional",
			"type":     "input",
			"required": false,
			"default":  nil,
		},
	}
	template := loadSyntheticTemplate(t, lading)

	fields := fieldsByStableIDOrName(template.Fields)
	if !fields["count"].DefaultAvailable || fields["count"].DefaultValue != json.Number("0") {
		t.Fatalf("number default = %#v", fields["count"])
	}
	if !fields["enabled"].DefaultAvailable || fields["enabled"].DefaultValue != false {
		t.Fatalf("switch default = %#v", fields["enabled"])
	}
	if !fields["Help"].DefaultAvailable || !fields["Help"].DisplayOnly || !fields["Help"].Supported {
		t.Fatalf("display-only field = %#v", fields["Help"])
	}
	if fields["Optional"].DefaultAvailable {
		t.Fatalf("null default reported as available: %#v", fields["Optional"])
	}
}

func TestNormalizeUnknownRequiredType(t *testing.T) {
	t.Parallel()

	lading := map[string]any{
		"field_script": map[string]any{
			"field_unique_Identifier": "script_result",
			"name":                    "Script result",
			"type":                    "private_script_widget",
			"required":                true,
			"default":                 nil,
		},
	}
	template := loadSyntheticTemplate(t, lading)
	if len(template.Fields) != 1 {
		t.Fatalf("field count = %d, want 1", len(template.Fields))
	}
	field := template.Fields[0]
	if field.Type != "private_script_widget" || field.Supported || !field.Required {
		t.Fatalf("unknown required field = %+v", field)
	}

	unsupportedRequired := 0
	for _, field := range template.Fields {
		if field.Required && !field.Supported {
			unsupportedRequired++
		}
	}
	if unsupportedRequired != 1 {
		t.Fatalf("unsupported required count = %d, want 1", unsupportedRequired)
	}
}

func TestNormalizeConditionsAndMapOrderAreStable(t *testing.T) {
	t.Parallel()

	first := map[string]any{
		"field_details": map[string]any{
			"field_unique_Identifier": "details",
			"name":                    "Details",
			"type":                    "textarea",
			"required":                false,
			"conditions_expression": map[string]any{
				"operator": "and",
				"conditions": []any{
					map[string]any{"field": "fault_type", "operator": "==", "value": "disk"},
				},
			},
		},
		"field_fault": map[string]any{
			"field_unique_Identifier": "fault_type",
			"name":                    "Fault type",
			"type":                    "select",
			"required":                true,
			"choices": []any{
				map[string]any{"value": "disk", "label": "Disk"},
			},
		},
	}
	second := map[string]any{
		"field_fault":   first["field_fault"],
		"field_details": first["field_details"],
	}

	firstTemplate := loadSyntheticTemplate(t, first)
	secondTemplate := loadSyntheticTemplate(t, second)
	if firstTemplate.MetadataHash != secondTemplate.MetadataHash {
		t.Fatalf("metadata hash depends on map order: %q != %q", firstTemplate.MetadataHash, secondTemplate.MetadataHash)
	}
	details := fieldsByStableIDOrName(firstTemplate.Fields)["details"]
	if details.Conditions.Operator != "and" || len(details.Conditions.Conditions) != 1 {
		t.Fatalf("conditions = %+v", details.Conditions)
	}
	condition := details.Conditions.Conditions[0]
	if condition.Field != "fault_type" || condition.Operator != "==" || condition.Value != "disk" {
		t.Fatalf("condition = %+v", condition)
	}
}

func TestNormalizeLiveConditionExpressionTreatsEmptyGroupsAsNoCondition(t *testing.T) {
	t.Parallel()

	template := loadSyntheticTemplate(t, map[string]any{
		"field_details": map[string]any{
			"field_unique_Identifier": "details",
			"field_name":              "Details",
			"field_type":              "textarea",
			"field_required":          "OPTION",
			"conditions_expression": map[string]any{
				"hide":                           true,
				"field_display":                  false,
				"group_condition_relation":       "and",
				"condition_groups_list_all_dict": map[string]any{},
			},
		},
	})

	conditions := template.Fields[0].Conditions
	if conditions.Unsupported || conditions.Operator != "" || len(conditions.Conditions) != 0 {
		t.Fatalf("empty live conditions = %+v, want no condition", conditions)
	}
}

func TestNormalizeLiveConditionExpressionMarksNonEmptyGroupsUnsupported(t *testing.T) {
	t.Parallel()

	template := loadSyntheticTemplate(t, map[string]any{
		"field_details": map[string]any{
			"field_unique_Identifier": "details",
			"field_name":              "Details",
			"field_type":              "textarea",
			"field_required":          "OPTION",
			"conditions_expression": map[string]any{
				"hide":                     true,
				"field_display":            false,
				"group_condition_relation": "and",
				"condition_groups_list_all_dict": map[string]any{
					"group_1": map[string]any{"field": "fault_type", "operator": "==", "value": "disk"},
				},
			},
		},
	})

	conditions := template.Fields[0].Conditions
	if !conditions.Unsupported {
		t.Fatalf("non-empty live conditions = %+v, want unsupported", conditions)
	}
}

func TestNormalizeMetadataHashCoversBackendKeysAndDefaults(t *testing.T) {
	t.Parallel()

	baseField := map[string]any{
		"field_unique_Identifier": "title",
		"name":                    "Title",
		"type":                    "input",
		"required":                true,
		"default":                 "first",
	}
	base := loadSyntheticTemplate(t, map[string]any{"field_title_1001": baseField})
	changedKey := loadSyntheticTemplate(t, map[string]any{"field_title_9999": baseField})
	changedDefault := loadSyntheticTemplate(t, map[string]any{
		"field_title_1001": map[string]any{
			"field_unique_Identifier": "title",
			"name":                    "Title",
			"type":                    "input",
			"required":                true,
			"default":                 "second",
		},
	})

	if base.MetadataHash == changedKey.MetadataHash {
		t.Fatal("metadata hash does not cover backend field keys")
	}
	if base.MetadataHash == changedDefault.MetadataHash {
		t.Fatal("metadata hash does not cover default values")
	}
}

func TestNormalizePreservesLargeIntegerPrecision(t *testing.T) {
	t.Parallel()

	const (
		lower = "9007199254740992"
		upper = "9007199254740993"
	)
	defaultLower := loadSyntheticTemplate(t, precisionLading(lower, lower))
	defaultUpper := loadSyntheticTemplate(t, precisionLading(upper, lower))
	conditionUpper := loadSyntheticTemplate(t, precisionLading(lower, upper))

	defaultValue, ok := fieldsByStableIDOrName(defaultUpper.Fields)["limit"].DefaultValue.(json.Number)
	if !ok || defaultValue.String() != upper {
		t.Fatalf("large default = %#v, want json.Number(%q)", defaultValue, upper)
	}
	conditionValue, ok := fieldsByStableIDOrName(conditionUpper.Fields)["details"].Conditions.Conditions[0].Value.(json.Number)
	if !ok || conditionValue.String() != upper {
		t.Fatalf("large condition value = %#v, want json.Number(%q)", conditionValue, upper)
	}
	if defaultLower.MetadataHash == defaultUpper.MetadataHash {
		t.Fatal("metadata hash collapses adjacent large integer defaults")
	}
	if defaultLower.MetadataHash == conditionUpper.MetadataHash {
		t.Fatal("metadata hash collapses adjacent large integer condition values")
	}
}

func precisionLading(defaultValue, conditionValue string) map[string]any {
	return map[string]any{
		"field_limit": map[string]any{
			"field_unique_Identifier": "limit",
			"name":                    "Limit",
			"type":                    "number",
			"required":                false,
			"default":                 json.Number(defaultValue),
		},
		"field_details": map[string]any{
			"field_unique_Identifier": "details",
			"name":                    "Details",
			"type":                    "textarea",
			"required":                false,
			"conditions_expression": map[string]any{
				"operator": "and",
				"conditions": []any{
					map[string]any{"field": "limit", "operator": "==", "value": json.Number(conditionValue)},
				},
			},
		},
	}
}

func loadSyntheticTemplate(t *testing.T, lading map[string]any) Template {
	t.Helper()
	body, err := json.Marshal(map[string]any{"result": true, "data": lading})
	if err != nil {
		t.Fatalf("encode synthetic lading: %v", err)
	}
	client := newTemplateFixtureClient(t, body)
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	return template
}

func newTemplateFixtureClient(t *testing.T, lading []byte) *MetadataClient {
	t.Helper()
	service := mustReadFixture(t, "service-4003.json")
	return newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/service/v3/service/4003/":
			_, _ = w.Write(service)
		case "/api/tasktmpl3/v3/lading/":
			_, _ = w.Write(lading)
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func fieldsByStableIDOrName(fields []Field) map[string]Field {
	indexed := make(map[string]Field, len(fields))
	for _, field := range fields {
		key := field.StableID
		if key == "" {
			key = field.Name
		}
		indexed[key] = field
	}
	return indexed
}
