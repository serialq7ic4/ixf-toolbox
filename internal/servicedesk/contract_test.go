package servicedesk

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectURLResolvedContract(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if contract.ContractVersion != 1 {
		t.Fatalf("contract version = %d, want 1", contract.ContractVersion)
	}
	if contract.Target.BusinessID != 15 || contract.Target.ServiceID != 4003 || contract.Target.SnapshotID != "218527" {
		t.Fatalf("resolved target = %+v", contract.Target)
	}
	if contract.Target.ServiceKey != "request" || contract.Target.ServiceName != "Fixture Server Hardware Repair" {
		t.Fatalf("resolved service = %+v", contract.Target)
	}
	if len(contract.InputContract.Fields) != 4 {
		t.Fatalf("contract field count = %d, want 4", len(contract.InputContract.Fields))
	}
	if contract.TemplateDrift || contract.DriftClassification != "" {
		t.Fatalf("URL-mode drift = %v/%q", contract.TemplateDrift, contract.DriftClassification)
	}
	if !strings.HasPrefix(contract.ContractHash, "sha256:") || len(contract.ContractHash) != len("sha256:")+64 {
		t.Fatalf("contract hash = %q", contract.ContractHash)
	}

	fields := contractFieldsByName(contract.InputContract.Fields)
	for _, name := range []string{"title", "fault_type", "devices", "details"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("resolved contract is missing field %q: %+v", name, contract.InputContract.Fields)
		}
	}
	if fields["title"].DisplayName != "Title" || fields["title"].StableID != "title" || !fields["title"].Required {
		t.Fatalf("title contract field = %+v", fields["title"])
	}

	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	for _, forbidden := range []string{"field_title_1001", "field_fault_1002", "fixture-api-key-value", "defaultValue", "metadataHash"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("public contract contains forbidden private value %q: %s", forbidden, encoded)
		}
	}
}

func TestResolvedContractIncludesChoicesTablesAndConditions(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	fields := contractFieldsByName(contract.InputContract.Fields)

	wantChoices := []Choice{{Label: "Disk", Value: "disk"}, {Label: "Memory", Value: "memory"}}
	if got := fields["fault_type"].Choices; !equalChoices(got, wantChoices) {
		t.Fatalf("fault choices = %#v, want %#v", got, wantChoices)
	}
	columns := fields["devices"].Columns
	if len(columns) != 2 || columns[0].Name != "host_name" || columns[0].DisplayName != "Host name" || columns[0].Type != "input" || !columns[0].Required {
		t.Fatalf("table columns = %+v", columns)
	}
	if columns[1].Name != "component" || columns[1].DisplayName != "Component" {
		t.Fatalf("second table column = %+v", columns[1])
	}
	conditions := fields["details"].Conditions
	if conditions == nil || conditions.Source != "conditions_expression" || conditions.Operator != "and" || len(conditions.Conditions) != 1 {
		t.Fatalf("details conditions = %+v", conditions)
	}
	if conditions.Conditions[0].Field != "fault_type" || conditions.Conditions[0].Operator != "==" || conditions.Conditions[0].Value != "disk" {
		t.Fatalf("details condition = %+v", conditions.Conditions[0])
	}
}

func TestResolvedContractRejectsDuplicateDisplayName(t *testing.T) {
	t.Parallel()

	lading := map[string]any{
		"field_first": map[string]any{
			"name": "Duplicate", "type": "input", "required": false,
		},
		"field_second": map[string]any{
			"name": "Duplicate", "type": "textarea", "required": false,
		},
	}
	_, err := inspectSyntheticContract(t, lading)
	if err == nil || !strings.Contains(err.Error(), "duplicate display name") {
		t.Fatalf("Inspect() error = %v, want duplicate display name error", err)
	}
}

func TestContractHashIsStable(t *testing.T) {
	t.Parallel()

	first := []byte(`{"result":true,"data":{"field_b":{"field_unique_Identifier":"b","name":"B","type":"input","required":false},"field_a":{"field_unique_Identifier":"a","name":"A","type":"select","required":true,"choices":[{"label":"One","value":"1"}]}}}`)
	second := []byte(`{"data":{"field_a":{"choices":[{"value":"1","label":"One"}],"required":true,"type":"select","name":"A","field_unique_Identifier":"a"},"field_b":{"required":false,"type":"input","name":"B","field_unique_Identifier":"b"}},"result":true}`)

	firstContract, err := Inspect(context.Background(), newTemplateFixtureClient(t, first), InspectRequest{Target: EditTarget{BusinessID: 15, ServiceID: 4003}})
	if err != nil {
		t.Fatalf("first Inspect() error = %v", err)
	}
	secondContract, err := Inspect(context.Background(), newTemplateFixtureClient(t, second), InspectRequest{Target: EditTarget{BusinessID: 15, ServiceID: 4003}})
	if err != nil {
		t.Fatalf("second Inspect() error = %v", err)
	}
	if firstContract.ContractHash != secondContract.ContractHash {
		t.Fatalf("contract hash depends on map order: %q != %q", firstContract.ContractHash, secondContract.ContractHash)
	}
}

func TestInspectReportsUnsupportedRequiredField(t *testing.T) {
	t.Parallel()

	contract, err := inspectSyntheticContract(t, map[string]any{
		"field_script": map[string]any{
			"field_unique_Identifier": "script_result",
			"name":                    "Script result",
			"type":                    "private_script_widget",
			"required":                true,
		},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if contract.InputContract.UnsupportedRequiredFieldCount != 1 {
		t.Fatalf("unsupported required count = %d, want 1", contract.InputContract.UnsupportedRequiredFieldCount)
	}
	if len(contract.InputContract.Fields) != 1 || contract.InputContract.Fields[0].Supported || contract.InputContract.Fields[0].Type != "private_script_widget" {
		t.Fatalf("unsupported field contract = %+v", contract.InputContract.Fields)
	}
}

func TestInspectReportsUnsupportedRequiredTableColumn(t *testing.T) {
	t.Parallel()

	contract, err := inspectSyntheticContract(t, map[string]any{
		"field_devices": map[string]any{
			"field_unique_Identifier": "devices",
			"name":                    "Devices",
			"type":                    "extend_table",
			"required":                true,
			"columns": []any{
				map[string]any{
					"key":      "script_result",
					"name":     "Script result",
					"type":     "private_script_widget",
					"required": true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if contract.InputContract.UnsupportedRequiredFieldCount != 1 {
		t.Fatalf("unsupported required count = %d, want 1", contract.InputContract.UnsupportedRequiredFieldCount)
	}
	column := contract.InputContract.Fields[0].Columns[0]
	if column.Supported || !column.Required || column.Type != "private_script_widget" {
		t.Fatalf("unsupported required column = %+v", column)
	}
}

func TestResolvedContractPreservesFalseConditionValue(t *testing.T) {
	t.Parallel()

	contract, err := inspectSyntheticContract(t, map[string]any{
		"field_enabled": map[string]any{
			"field_unique_Identifier": "enabled",
			"name":                    "Enabled",
			"type":                    "switch",
			"required":                false,
		},
		"field_details": map[string]any{
			"field_unique_Identifier": "details",
			"name":                    "Details",
			"type":                    "textarea",
			"required":                false,
			"conditions_expression": map[string]any{
				"operator": "and",
				"conditions": []any{
					map[string]any{"field": "enabled", "operator": "==", "value": false},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	if !strings.Contains(string(encoded), `"value":false`) {
		t.Fatalf("public contract omitted false condition value: %s", encoded)
	}
}

func TestResolvedContractMapsConditionSourcesToPublicNames(t *testing.T) {
	t.Parallel()

	contract, err := inspectSyntheticContract(t, map[string]any{
		"field_fault_1002": map[string]any{
			"field_unique_Identifier": "fault_type",
			"name":                    "Fault type",
			"type":                    "select",
			"required":                true,
		},
		"field_details_1004": map[string]any{
			"field_unique_Identifier": "details",
			"name":                    "Details",
			"type":                    "textarea",
			"required":                false,
			"conditions_expression": map[string]any{
				"operator": "and",
				"conditions": []any{
					map[string]any{"field": "field_fault_1002", "operator": "==", "value": "disk"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	condition := contractFieldsByName(contract.InputContract.Fields)["details"].Conditions.Conditions[0]
	if condition.Field != "fault_type" {
		t.Fatalf("condition source = %q, want public name %q", condition.Field, "fault_type")
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	if strings.Contains(string(encoded), "field_fault_1002") {
		t.Fatalf("public contract contains backend condition key: %s", encoded)
	}
}

func inspectSyntheticContract(t *testing.T, lading map[string]any) (ResolvedContract, error) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"result": true, "data": lading})
	if err != nil {
		t.Fatalf("encode synthetic lading: %v", err)
	}
	return Inspect(context.Background(), newTemplateFixtureClient(t, body), InspectRequest{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003},
	})
}

func contractFieldsByName(fields []InputField) map[string]InputField {
	indexed := make(map[string]InputField, len(fields))
	for _, field := range fields {
		indexed[field.Name] = field
	}
	return indexed
}

func equalChoices(got, want []Choice) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
