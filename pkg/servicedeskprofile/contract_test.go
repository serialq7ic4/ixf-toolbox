package servicedeskprofile

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

func TestInspectProfileResolvedContract(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	overlay.Baseline = DiscoverOverlay(template, overlay.Name).Baseline
	required := true
	detailsOverlay := overlay.Fields["details"]
	detailsOverlay.Default = json.RawMessage(`"No diagnostics supplied"`)
	detailsOverlay.Required = &required
	overlay.Fields["details"] = detailsOverlay

	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{
			Overlay: overlay,
			Source:  "cache",
			Stale:   true,
		},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if contract.Profile == nil || contract.Profile.Name != "fixture-server-hardware-repair" || contract.Profile.Source != "cache" || !contract.Profile.Stale {
		t.Fatalf("resolved profile = %+v", contract.Profile)
	}
	if contract.Target.BusinessID != 15 || contract.Target.ServiceID != 4003 {
		t.Fatalf("resolved target = %+v", contract.Target)
	}
	if contract.TemplateDrift || contract.DriftClassification != "" || contract.ApplyBlocked || len(contract.DriftChanges) != 0 {
		t.Fatalf("matching profile drift = %v/%q blocked=%v changes=%+v", contract.TemplateDrift, contract.DriftClassification, contract.ApplyBlocked, contract.DriftChanges)
	}

	fields := contractFieldsByName(contract.InputContract.Fields)
	for _, name := range []string{"title", "faultType", "devices", "details"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("resolved profile contract is missing field %q: %+v", name, contract.InputContract.Fields)
		}
	}
	if _, ok := fields["fault_type"]; ok {
		t.Fatalf("resolved profile retained URL-mode name fault_type: %+v", fields["fault_type"])
	}
	if len(fields["title"].Aliases) != 1 || fields["title"].Aliases[0] != "ticketTitle" {
		t.Fatalf("title aliases = %#v", fields["title"].Aliases)
	}
	details := fields["details"]
	if details.Description != "Optional detail shown when the selected fault type requires it" || !details.DefaultAvailable || !details.Required {
		t.Fatalf("details contract field = %+v", details)
	}
	if details.Conditions == nil || len(details.Conditions.Conditions) != 1 || details.Conditions.Conditions[0].Field != "faultType" {
		t.Fatalf("details conditions = %+v", details.Conditions)
	}
	columns := fields["devices"].Columns
	if len(columns) != 2 || columns[0].Name != "hostName" || columns[1].Name != "part" {
		t.Fatalf("device columns = %+v", columns)
	}

	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	for _, forbidden := range []string{"field_title_1001", "field_fault_1002", "host_name", "component", "metadataHash", "No diagnostics supplied"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("public profile contract contains forbidden private value %q: %s", forbidden, encoded)
		}
	}

	delete(overlay.Fields, "faultType")
	devicesOverlay := overlay.Fields["devices"]
	delete(devicesOverlay.Columns, "part")
	overlay.Fields["devices"] = devicesOverlay
	selective, err := Inspect(context.Background(), client, InspectRequest{
		Target:  EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{Overlay: overlay, Source: "cache"},
	})
	if err != nil {
		t.Fatalf("Inspect(selective profile) error = %v", err)
	}
	selectiveFields := contractFieldsByName(selective.InputContract.Fields)
	if _, ok := selectiveFields["fault_type"]; !ok {
		t.Fatalf("selective profile hid unmapped required field: %+v", selective.InputContract.Fields)
	}
	selectiveDetails := selectiveFields["details"]
	if selectiveDetails.Conditions == nil || len(selectiveDetails.Conditions.Conditions) != 1 || selectiveDetails.Conditions.Conditions[0].Field != "fault_type" {
		t.Fatalf("selective profile condition source = %+v", selectiveDetails.Conditions)
	}
	selectiveColumns := selectiveFields["devices"].Columns
	if len(selectiveColumns) != 2 || selectiveColumns[0].Name != "hostName" || selectiveColumns[1].Name != "Component" {
		t.Fatalf("selective profile hid unmapped required column: %+v", selectiveColumns)
	}
}

func TestInspectProfileSerializesApplyBlockedFalse(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	overlay.Baseline = DiscoverOverlay(template, overlay.Name).Baseline

	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target:  EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{Overlay: overlay, Source: "network"},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal contract: %v", err)
	}
	applyBlocked, ok := payload["applyBlocked"]
	if !ok || string(applyBlocked) != "false" {
		t.Fatalf("profile applyBlocked = %s, present=%v; contract=%s", applyBlocked, ok, encoded)
	}
}

func TestInspectProfileSerializesColumnDefaultAvailabilityForUnmappedTable(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	overlay.Baseline = DiscoverOverlay(template, overlay.Name).Baseline
	delete(overlay.Fields, "devices")

	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target:  EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{Overlay: overlay, Source: "network"},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	var payload struct {
		InputContract struct {
			Fields []struct {
				Name    string                       `json:"name"`
				Columns []map[string]json.RawMessage `json:"columns"`
			} `json:"fields"`
		} `json:"inputContract"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal contract: %v", err)
	}
	for _, field := range payload.InputContract.Fields {
		if field.Name != "devices" {
			continue
		}
		if len(field.Columns) != 2 {
			t.Fatalf("unmapped table columns = %+v", field.Columns)
		}
		for _, column := range field.Columns {
			defaultAvailable, ok := column["defaultAvailable"]
			if !ok || string(defaultAvailable) != "false" {
				t.Fatalf("unmapped profile column defaultAvailable = %s, present=%v; contract=%s", defaultAvailable, ok, encoded)
			}
		}
		return
	}
	t.Fatalf("profile contract is missing unmapped table field: %s", encoded)
}

func TestInspectProfileDestructiveDrift(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	overlay.Baseline = DiscoverOverlay(template, overlay.Name).Baseline
	for index := range overlay.Baseline.Fields {
		if overlay.Baseline.Fields[index].StableID == "title" {
			overlay.Baseline.Fields[index].Type = "number"
		}
	}

	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{
			Overlay: overlay,
			Source:  "network",
		},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if contract.Profile == nil || contract.Profile.Source != "network" {
		t.Fatalf("resolved profile = %+v", contract.Profile)
	}
	if !contract.TemplateDrift || contract.DriftClassification != "destructive" || !contract.ApplyBlocked || len(contract.DriftChanges) == 0 {
		t.Fatalf("destructive profile drift = %v/%q blocked=%v changes=%+v", contract.TemplateDrift, contract.DriftClassification, contract.ApplyBlocked, contract.DriftChanges)
	}
}

func TestInspectProfileProjectsDriftReferencesToPublicNames(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	overlay.Baseline = DiscoverOverlay(template, overlay.Name).Baseline
	for index := range overlay.Baseline.Fields {
		if overlay.Baseline.Fields[index].StableID == "title" {
			overlay.Baseline.Fields[index].Type = "number"
		}
		if overlay.Baseline.Fields[index].StableID == "devices" {
			for columnIndex := range overlay.Baseline.Fields[index].Columns {
				if overlay.Baseline.Fields[index].Columns[columnIndex].Key == "host_name" {
					overlay.Baseline.Fields[index].Columns = append(overlay.Baseline.Fields[index].Columns,
						BaselineColumn{Key: "stale_host_name", Name: "host_name", Type: "input", Required: true})
					break
				}
			}
		}
	}
	overlay.Baseline.Fields = append(overlay.Baseline.Fields,
		BaselineField{StableID: "field_title_1001", Name: "field_title_1001", Type: "input", Required: true})

	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target:  EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{Overlay: overlay, Source: "network"},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	for _, privateName := range []string{"field_title_1001", "host_name"} {
		if strings.Contains(string(encoded), privateName) {
			t.Fatalf("profile drift exposes private identifier %q: %s", privateName, encoded)
		}
	}
	var typeDrift *DriftChange
	for index := range contract.DriftChanges {
		if contract.DriftChanges[index].Kind == "field_type_changed" {
			typeDrift = &contract.DriftChanges[index]
			break
		}
	}
	if typeDrift == nil || typeDrift.Field != "title" {
		t.Fatalf("mapped type drift = %+v, want public field title", typeDrift)
	}
	columnDriftFound := false
	for _, change := range contract.DriftChanges {
		if change.Kind == "column_missing" && change.Column != "hostName" {
			t.Fatalf("stale column drift = %+v, want public column hostName", change)
		}
		if change.Kind == "column_missing" {
			columnDriftFound = true
		}
	}
	if !columnDriftFound {
		t.Fatalf("profile drift changes = %+v, want stale column change", contract.DriftChanges)
	}
}

func TestInspectProfileRejectsUnresolvableDriftReferenceWithoutDisclosure(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	template, err := LoadTemplate(context.Background(), client, EditTarget{BusinessID: 15, ServiceID: 4003})
	if err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	overlay.Baseline = DiscoverOverlay(template, overlay.Name).Baseline
	const privateName = "private_baseline_field_991"
	overlay.Baseline.Fields = append(overlay.Baseline.Fields,
		BaselineField{StableID: privateName, Name: privateName, Type: "input", Required: false})

	_, err = Inspect(context.Background(), client, InspectRequest{
		Target:  EditTarget{BusinessID: 15, ServiceID: 4003},
		Profile: &ProfileContext{Overlay: overlay, Source: "network"},
	})
	if err == nil {
		t.Fatal("Inspect() succeeded, want unresolved drift reference error")
	}
	if strings.Contains(err.Error(), privateName) {
		t.Fatalf("Inspect() error exposes private drift identifier %q: %v", privateName, err)
	}
	if err.Error() != "ServiceDesk drift contains an unresolved field reference" {
		t.Fatalf("Inspect() error = %q, want generic unresolved field error", err)
	}
}

func TestInspectURLContractPreservesPreProfileJSONAndHash(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClient(t, mustReadFixture(t, "lading-4003.json"))
	contract, err := Inspect(context.Background(), client, InspectRequest{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003},
	})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	legacy := legacyURLContract(contract)
	legacy.ContractHash = ""
	legacyHash, err := canonicalHash(legacy)
	if err != nil {
		t.Fatalf("hash legacy URL contract: %v", err)
	}
	const wantHash = "sha256:b77bf746d0334fbbc62b9d23d6bde0825f5221c5d1c3e2d9dbc5079f4acd8e43"
	if legacyHash != wantHash {
		t.Fatalf("pre-profile URL fixture hash = %q, want %q", legacyHash, wantHash)
	}
	legacy.ContractHash = wantHash
	wantJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy URL contract: %v", err)
	}
	gotJSON, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal URL contract: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("URL contract JSON changed from pre-profile shape:\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
	if contract.ContractHash != wantHash {
		t.Fatalf("URL contract hash = %q, want pre-profile hash %q", contract.ContractHash, wantHash)
	}
}

type legacyURLResolvedContract struct {
	ContractVersion     int                    `json:"contractVersion"`
	Target              ResolvedTarget         `json:"target"`
	InputContract       legacyURLInputContract `json:"inputContract"`
	TemplateDrift       bool                   `json:"templateDrift"`
	DriftClassification string                 `json:"driftClassification,omitempty"`
	ContractHash        string                 `json:"contractHash,omitempty"`
}

type legacyURLInputContract struct {
	Fields                        []legacyURLInputField `json:"fields"`
	UnsupportedRequiredFieldCount int                   `json:"unsupportedRequiredFieldCount"`
}

type legacyURLInputField struct {
	Name             string                 `json:"name"`
	Aliases          []string               `json:"aliases,omitempty"`
	DisplayName      string                 `json:"displayName"`
	StableID         string                 `json:"stableId,omitempty"`
	Type             string                 `json:"type"`
	Required         bool                   `json:"required"`
	Conditional      bool                   `json:"conditional"`
	DefaultAvailable bool                   `json:"defaultAvailable"`
	Choices          []Choice               `json:"choices,omitempty"`
	Columns          []legacyURLInputColumn `json:"columns,omitempty"`
	Conditions       *ConditionSet          `json:"conditions,omitempty"`
	Supported        bool                   `json:"supported"`
}

type legacyURLInputColumn struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Choices     []Choice `json:"choices,omitempty"`
	Supported   bool     `json:"supported"`
}

func legacyURLContract(contract ResolvedContract) legacyURLResolvedContract {
	legacy := legacyURLResolvedContract{
		ContractVersion:     contract.ContractVersion,
		Target:              contract.Target,
		TemplateDrift:       contract.TemplateDrift,
		DriftClassification: contract.DriftClassification,
		ContractHash:        contract.ContractHash,
		InputContract: legacyURLInputContract{
			UnsupportedRequiredFieldCount: contract.InputContract.UnsupportedRequiredFieldCount,
			Fields:                        make([]legacyURLInputField, 0, len(contract.InputContract.Fields)),
		},
	}
	for _, field := range contract.InputContract.Fields {
		legacyField := legacyURLInputField{
			Name:             field.Name,
			Aliases:          field.Aliases,
			DisplayName:      field.DisplayName,
			StableID:         field.StableID,
			Type:             field.Type,
			Required:         field.Required,
			Conditional:      field.Conditional,
			DefaultAvailable: field.DefaultAvailable,
			Choices:          field.Choices,
			Conditions:       field.Conditions,
			Supported:        field.Supported,
			Columns:          make([]legacyURLInputColumn, 0, len(field.Columns)),
		}
		for _, column := range field.Columns {
			legacyField.Columns = append(legacyField.Columns, legacyURLInputColumn{
				Name:        column.Name,
				DisplayName: column.DisplayName,
				Type:        column.Type,
				Required:    column.Required,
				Choices:     column.Choices,
				Supported:   column.Supported,
			})
		}
		legacy.InputContract.Fields = append(legacy.InputContract.Fields, legacyField)
	}
	return legacy
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
	if len(columns) != 2 || columns[0].Name != "Host name" || columns[0].DisplayName != "Host name" || columns[0].Type != "input" || !columns[0].Required {
		t.Fatalf("table columns = %+v", columns)
	}
	if columns[1].Name != "Component" || columns[1].DisplayName != "Component" {
		t.Fatalf("second table column = %+v", columns[1])
	}
	conditions := fields["details"].Conditions
	if conditions == nil || conditions.Source != "" || conditions.Operator != "and" || len(conditions.Conditions) != 1 {
		t.Fatalf("details conditions = %+v", conditions)
	}
	if conditions.Conditions[0].Field != "fault_type" || conditions.Conditions[0].Operator != "==" || conditions.Conditions[0].Value != "disk" {
		t.Fatalf("details condition = %+v", conditions.Conditions[0])
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	for _, privateName := range []string{"host_name", `"name":"component"`, "conditions_expression"} {
		if strings.Contains(string(encoded), privateName) {
			t.Errorf("public contract contains backend metadata name %q: %s", privateName, encoded)
		}
	}
}

func TestResolvedContractRejectsDuplicateTableColumnDisplayName(t *testing.T) {
	t.Parallel()

	_, err := inspectSyntheticContract(t, map[string]any{
		"field_devices": map[string]any{
			"field_unique_Identifier": "devices",
			"name":                    "Devices",
			"type":                    "extend_table",
			"required":                true,
			"columns": []any{
				map[string]any{"key": "first_private_key", "name": "Duplicate", "type": "input", "required": true},
				map[string]any{"key": "second_private_key", "name": "Duplicate", "type": "input", "required": true},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate table column display name") {
		t.Fatalf("Inspect() error = %v, want duplicate table column display name error", err)
	}
	for _, privateKey := range []string{"first_private_key", "second_private_key"} {
		if strings.Contains(err.Error(), privateKey) {
			t.Fatalf("Inspect() error exposes backend column key %q: %v", privateKey, err)
		}
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

func TestResolvedContractRejectsUnknownConditionSourceWithoutDisclosure(t *testing.T) {
	t.Parallel()

	const privateField = "field_private_credential_lookup_991"
	_, err := inspectSyntheticContract(t, map[string]any{
		"field_details": map[string]any{
			"field_unique_Identifier": "details",
			"name":                    "Details",
			"type":                    "textarea",
			"required":                false,
			"conditions_expression": map[string]any{
				"operator": "and",
				"conditions": []any{
					map[string]any{"field": privateField, "operator": "==", "value": "enabled"},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "condition references an unknown field") {
		t.Fatalf("Inspect() error = %v, want safe unknown condition field error", err)
	}
	if strings.Contains(err.Error(), privateField) {
		t.Fatalf("Inspect() error exposes unknown backend field %q: %v", privateField, err)
	}
}

func TestResolvedContractRejectsBackendPublicNameCollision(t *testing.T) {
	t.Parallel()

	_, err := inspectSyntheticContract(t, map[string]any{
		"a": map[string]any{
			"field_unique_Identifier": "z",
			"name":                    "Field Z",
			"type":                    "input",
			"required":                false,
		},
		"b": map[string]any{
			"field_unique_Identifier": "a",
			"name":                    "Field A",
			"type":                    "input",
			"required":                false,
		},
		"c": map[string]any{
			"field_unique_Identifier": "details",
			"name":                    "Details",
			"type":                    "textarea",
			"required":                false,
			"conditions_expression": map[string]any{
				"operator": "and",
				"conditions": []any{
					map[string]any{"field": "a", "operator": "==", "value": "ready"},
				},
			},
		},
	})
	const wantError = "ServiceDesk metadata has ambiguous condition field references"
	if err == nil || err.Error() != wantError {
		t.Fatalf("Inspect() error = %v, want generic ambiguous condition reference error", err)
	}
}

func TestResolvedContractOmitsBackendConditionSourceName(t *testing.T) {
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
					map[string]any{"field": "enabled", "operator": "==", "value": true},
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
	if strings.Contains(string(encoded), "conditions_expression") {
		t.Fatalf("public contract exposes backend condition property: %s", encoded)
	}
	conditions := contractFieldsByName(contract.InputContract.Fields)["details"].Conditions
	if conditions == nil || conditions.Source != "" {
		t.Fatalf("public condition source = %+v, want omitted source", conditions)
	}
}

func TestContractHashPreservesLargeConditionIntegers(t *testing.T) {
	t.Parallel()

	const (
		lower = "9007199254740992"
		upper = "9007199254740993"
	)
	lowerContract, err := inspectSyntheticContract(t, precisionLading(lower, lower))
	if err != nil {
		t.Fatalf("Inspect(lower) error = %v", err)
	}
	upperContract, err := inspectSyntheticContract(t, precisionLading(lower, upper))
	if err != nil {
		t.Fatalf("Inspect(upper) error = %v", err)
	}
	condition := contractFieldsByName(upperContract.InputContract.Fields)["details"].Conditions.Conditions[0]
	value, ok := condition.Value.(json.Number)
	if !ok || value.String() != upper {
		t.Fatalf("large contract condition = %#v, want json.Number(%q)", condition.Value, upper)
	}
	if lowerContract.ContractHash == upperContract.ContractHash {
		t.Fatal("contract hash collapses adjacent large integer condition values")
	}
	encoded, err := json.Marshal(upperContract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	if !strings.Contains(string(encoded), `"value":`+upper) {
		t.Fatalf("public contract lost large integer precision: %s", encoded)
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
