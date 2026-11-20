package servicedesk

import (
	"context"
	"encoding/json"
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
	if !fields["count"].DefaultAvailable || fields["count"].DefaultValue != float64(0) {
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

func newTemplateFixtureClient(t *testing.T, lading []byte) *Client {
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
