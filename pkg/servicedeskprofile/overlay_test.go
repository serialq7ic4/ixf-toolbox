package servicedeskprofile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeOverlayStrictSchema(t *testing.T) {
	t.Parallel()

	valid := string(mustReadFixture(t, "profile-4003.json"))
	if _, err := DecodeOverlay([]byte(valid)); err != nil {
		t.Fatalf("DecodeOverlay(valid) error = %v", err)
	}

	tests := map[string]string{
		"unknown top-level property": replaceOverlayText(t, valid, `"schemaVersion": 1,`, `"schemaVersion": 1, "network": {},`),
		"unknown routing property":   replaceOverlayText(t, valid, `"displayName": "Server hardware repair",`, `"displayName": "Server hardware repair", "score": 1,`),
		"unknown target property":    replaceOverlayText(t, valid, `"businessId": 15,`, `"businessId": 15, "host": "example.test",`),
		"unknown baseline property":  replaceOverlayText(t, valid, `"snapshotId": "218527",`, `"snapshotId": "218527", "raw": {},`),
		"unknown fingerprint property": replaceOverlayText(t, valid,
			`"stableId": "devices",`, `"stableId": "devices", "backendKey": "private",`),
		"unknown baseline column property": replaceOverlayText(t, valid,
			`"key": "host_name",`, `"key": "host_name", "script": "run()",`),
		"unknown overlay field property": replaceOverlayText(t, valid,
			`"description": "Optional detail shown when the selected fault type requires it"`,
			`"description": "Optional detail shown when the selected fault type requires it", "url": "https://example.test"`),
		"unknown field matcher property": replaceOverlayText(t, valid,
			`"details": {\n      "match": {\n        "stableId": "details",\n        "name": "Failure details"`,
			`"details": {\n      "match": {\n        "stableId": "details",\n        "name": "Failure details",\n        "backendKey": "private"`),
		"unknown overlay column property": replaceOverlayText(t, valid,
			`"part": {\n          "match": {\n            "key": "component",\n            "name": "Component"\n          }\n        }`,
			`"part": {\n          "match": {\n            "key": "component",\n            "name": "Component"\n          },\n          "headers": {}\n        }`),
		"unknown column matcher property": replaceOverlayText(t, valid,
			`"key": "host_name",\n            "name": "Host name"`,
			`"key": "host_name",\n            "name": "Host name",\n            "endpoint": "/private"`),
		"trailing JSON": valid + `{}`,
		"missing description": replaceOverlayText(t, valid,
			`  "description": "Fixture profile for server hardware repair",\n`, ``),
		"missing routing examples": replaceOverlayText(t, valid,
			`,\n    "examples": [\n      "Create a repair ticket for a failed server disk"\n    ]`, ``),
		"missing baseline required": replaceOverlayText(t, valid,
			`        "type": "textarea",\n        "required": false`, `        "type": "textarea"`),
		"null field aliases": replaceOverlayText(t, valid,
			`"aliases": [\n        "ticketTitle"\n      ]`, `"aliases": null`),
		"null field columns": replaceOverlayText(t, valid,
			`"description": "Optional detail shown when the selected fault type requires it"`,
			`"description": "Optional detail shown when the selected fault type requires it", "columns": null`),
		"null column aliases": replaceOverlayText(t, valid,
			`"part": {\n          "match": {\n            "key": "component",\n            "name": "Component"\n          }\n        }`,
			`"part": {\n          "match": {\n            "key": "component",\n            "name": "Component"\n          },\n          "aliases": null\n        }`),
	}

	for name, data := range tests {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if name != "trailing JSON" && !json.Valid([]byte(data)) {
				t.Fatalf("strict-schema mutation is invalid JSON and cannot test %s", name)
			}
			if _, err := DecodeOverlay([]byte(data)); err == nil {
				t.Fatal("DecodeOverlay() succeeded, want strict schema error")
			}
		})
	}
}

func TestOverlayNameAndRoutingValidation(t *testing.T) {
	t.Parallel()

	valid := string(mustReadFixture(t, "profile-4003.json"))
	tests := map[string]string{
		"uppercase name": replaceOverlayText(t, valid,
			`"name": "fixture-server-hardware-repair"`, `"name": "Fixture-Server"`),
		"name too long": replaceOverlayText(t, valid,
			`"name": "fixture-server-hardware-repair"`, `"name": "`+strings.Repeat("a", 65)+`"`),
		"empty display name": replaceOverlayText(t, valid,
			`"displayName": "Server hardware repair"`, `"displayName": " "`),
		"oversized useWhen": replaceOverlayText(t, valid,
			`"useWhen": "A user asks to create a repair ticket for physical server hardware"`, `"useWhen": "`+strings.Repeat("x", 501)+`"`),
		"duplicate examples": replaceOverlayText(t, valid,
			`"Create a repair ticket for a failed server disk"`,
			`"Create a repair ticket for a failed server disk", "Create a repair ticket for a failed server disk"`),
		"zero target": replaceOverlayText(t, valid, `"serviceId": 4003`, `"serviceId": 0`),
		"bad metadata hash": replaceOverlayText(t, valid,
			`sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`, `sha256:ABC`),
		"bad default hash": replaceOverlayText(t, valid,
			`"type": "textarea",\n        "required": false`,
			`"type": "textarea",\n        "required": false,\n        "defaultHash": "sha256:ABC"`),
		"bad condition hash": replaceOverlayText(t, valid,
			`sha256:788e7b8a1b8f77979f40e39437584f1f57ef62ecf7f5989bd19ca45a18e82da6`,
			`sha256:ABC`),
		"columns on non-table fingerprint": replaceOverlayText(t, valid,
			`"type": "textarea",\n        "required": false`,
			`"type": "textarea",\n        "required": false,\n        "columns": []`),
		"missing table fingerprint columns": replaceOverlayText(t, valid,
			`,\n        "columns": [\n          {\n            "key": "host_name",\n            "name": "Host name",\n            "type": "input",\n            "required": true\n          },\n          {\n            "key": "component",\n            "name": "Component",\n            "type": "input",\n            "required": true\n          }\n        ]`, ``),
		"choice hash on plain input": replaceOverlayText(t, valid,
			`"type": "input",\n        "required": true,\n        "defaultHash": "sha256:12ae32cb1ec02d01eda3581b127c1fee3b0dc53572ed6baf239721a03d82e126"`,
			`"type": "input",\n        "required": true,\n        "choiceHash": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",\n        "defaultHash": "sha256:12ae32cb1ec02d01eda3581b127c1fee3b0dc53572ed6baf239721a03d82e126"`),
		"empty matcher": replaceOverlayText(t, valid,
			`"details": {\n      "match": {\n        "stableId": "details",\n        "name": "Failure details"\n      }`,
			`"details": {\n      "match": {}`),
	}

	for name, data := range tests {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !json.Valid([]byte(data)) {
				t.Fatalf("validation mutation is invalid JSON and cannot test %s", name)
			}
			if _, err := DecodeOverlay([]byte(data)); err == nil {
				t.Fatal("DecodeOverlay() succeeded, want validation error")
			}
		})
	}
}

func TestDecodeOverlayAcceptsDefaultAndConditionFingerprints(t *testing.T) {
	t.Parallel()

	data := string(mustReadFixture(t, "profile-4003.json"))
	data = replaceOverlayText(t, data,
		`"type": "textarea",\n        "required": false`,
		`"type": "textarea",\n        "required": false,\n        "defaultHash": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",\n        "conditionHash": "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`)
	data = replaceOverlayText(t, data,
		`"key": "host_name",\n            "name": "Host name",\n            "type": "input",\n            "required": true`,
		`"key": "host_name",\n            "name": "Host name",\n            "type": "input",\n            "required": true,\n            "defaultHash": "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",\n            "conditionHash": "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`)

	if _, err := DecodeOverlay([]byte(data)); err != nil {
		t.Fatalf("DecodeOverlay(default/condition fingerprints) error = %v", err)
	}
}

func TestDecodeOverlayFingerprintHashPresence(t *testing.T) {
	t.Parallel()

	const validHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	levels := []string{"field", "column"}
	attributes := []string{"defaultHash", "conditionHash"}
	tests := map[string]struct {
		present bool
		value   any
		valid   bool
	}{
		"omitted":   {valid: true},
		"valid":     {present: true, value: validHash, valid: true},
		"null":      {present: true, value: nil},
		"empty":     {present: true, value: ""},
		"malformed": {present: true, value: "sha256:ABC"},
	}

	for _, level := range levels {
		level := level
		for _, attribute := range attributes {
			attribute := attribute
			for name, test := range tests {
				name, test := name, test
				t.Run(level+"/"+attribute+"/"+name, func(t *testing.T) {
					t.Parallel()
					data := overlayWithFingerprintHash(t, level, attribute, test.value, test.present)
					_, err := DecodeOverlay(data)
					if test.valid && err != nil {
						t.Fatalf("DecodeOverlay(valid %s) error = %v", name, err)
					}
					if !test.valid && err == nil {
						t.Fatalf("DecodeOverlay(%s) succeeded, want strict fingerprint hash error", name)
					}
				})
			}
		}
	}
}

func overlayWithFingerprintHash(t *testing.T, level, attribute string, value any, present bool) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(mustReadFixture(t, "profile-4003.json"), &document); err != nil {
		t.Fatalf("json.Unmarshal(profile fixture) error = %v", err)
	}
	baseline := document["baseline"].(map[string]any)
	field := baseline["fields"].([]any)[0].(map[string]any)
	target := field
	if level == "column" {
		target = field["columns"].([]any)[0].(map[string]any)
	}
	delete(target, attribute)
	if present {
		target[attribute] = value
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("json.Marshal(profile fixture) error = %v", err)
	}
	return data
}

func TestOverlayRejectsDuplicatePublicNamesAndAliases(t *testing.T) {
	t.Parallel()

	valid := string(mustReadFixture(t, "profile-4003.json"))
	tests := map[string]string{
		"alias duplicates own name": replaceOverlayText(t, valid,
			`"ticketTitle"`, `"title"`),
		"alias duplicates another public name": replaceOverlayText(t, valid,
			`"ticketTitle"`, `"details"`),
		"duplicate alias": replaceOverlayText(t, valid,
			`"ticketTitle"`, `"ticketTitle", "ticketTitle"`),
		"column alias duplicates public name": replaceOverlayText(t, valid,
			`"key": "host_name",\n            "name": "Host name"\n          }`,
			`"key": "host_name",\n            "name": "Host name"\n          },\n          "aliases": ["part"]`),
	}

	for name, data := range tests {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeOverlay([]byte(data)); err == nil {
				t.Fatal("DecodeOverlay() succeeded, want duplicate public name error")
			}
		})
	}
}

func TestOverlayRejectsExecutableOrNetworkFields(t *testing.T) {
	t.Parallel()

	valid := string(mustReadFixture(t, "profile-4003.json"))
	tests := map[string]string{
		"script": replaceOverlayText(t, valid,
			`"aliases": [`, `"script": "return secret", "aliases": [`),
		"endpoint": replaceOverlayText(t, valid,
			`"businessId": 15,`, `"endpoint": "https://example.test", "businessId": 15,`),
		"api key": replaceOverlayText(t, valid,
			`"schemaVersion": 1,`, `"schemaVersion": 1, "apiKey": "secret",`),
		"cookie": replaceOverlayText(t, valid,
			`"useWhen":`, `"cookie": "private", "useWhen":`),
		"headers": replaceOverlayText(t, valid,
			`"serviceId": 4003`, `"serviceId": 4003, "headers": {"Authorization": "private"}`),
	}

	for name, data := range tests {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeOverlay([]byte(data)); err == nil {
				t.Fatal("DecodeOverlay() accepted executable or network configuration")
			}
		})
	}
}

func TestOverlayRequiredCannotRelaxServiceRequired(t *testing.T) {
	t.Parallel()

	data := string(mustReadFixture(t, "profile-4003.json"))
	data = replaceOverlayText(t, data,
		`"stableId": "title",\n        "name": "Title",\n        "type": "input",\n        "required": true`,
		`"stableId": "title",\n        "name": "Title",\n        "type": "input",\n        "required": false`)
	data = replaceOverlayText(t, data,
		`"aliases": [\n        "ticketTitle"\n      ]`,
		`"aliases": [\n        "ticketTitle"\n      ],\n      "required": false`)
	overlay, err := DecodeOverlay([]byte(data))
	if err != nil {
		t.Fatalf("DecodeOverlay() error = %v", err)
	}
	template := overlayValidationTemplate()

	if _, err := ValidateOverlay(template, overlay); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("ValidateOverlay() error = %v, want live required-field relaxation error", err)
	}
}

func TestOverlayRejectsWrongPublicDefaultType(t *testing.T) {
	t.Parallel()

	data := string(mustReadFixture(t, "profile-4003.json"))
	data = replaceOverlayText(t, data,
		`"aliases": [\n        "ticketTitle"\n      ]`,
		`"aliases": [\n        "ticketTitle"\n      ],\n      "default": 42`)
	if _, err := DecodeOverlay([]byte(data)); err == nil || !strings.Contains(err.Error(), "default") {
		t.Fatalf("DecodeOverlay() error = %v, want public JSON type error", err)
	}
}

func TestOverlayDiscoverProducesIncompleteRoutingDraft(t *testing.T) {
	t.Parallel()

	template := overlayValidationTemplate()
	overlay := DiscoverOverlay(template, "fixture-discovery")
	if len(overlay.Baseline.Fields) != len(template.Fields) || len(overlay.Fields) != len(template.Fields) {
		t.Fatalf("DiscoverOverlay() field coverage = baseline %d, fields %d; want %d", len(overlay.Baseline.Fields), len(overlay.Fields), len(template.Fields))
	}
	if overlay.Routing.DisplayName != "" || overlay.Routing.UseWhen != "" || overlay.Routing.DoNotUseWhen != "" {
		t.Fatalf("DiscoverOverlay() invented routing semantics: %+v", overlay.Routing)
	}
	if err := ValidateOverlaySchema(overlay); err == nil || !strings.Contains(err.Error(), "routing") {
		t.Fatalf("ValidateOverlaySchema(discovery draft) error = %v, want incomplete routing error", err)
	}
	for publicName, field := range overlay.Fields {
		if field.Default != nil || field.Required != nil || len(field.Aliases) != 0 {
			t.Errorf("discovered field %q invented business policy: %+v", publicName, field)
		}
	}
}

func TestOverlayDiscoverUsesDisplayNamesForPublicColumns(t *testing.T) {
	t.Parallel()

	template := overlayValidationTemplate()
	template.Fields[0].TableColumns = []TableColumn{
		{BackendKey: "private_key_a", Name: "Device", Type: "input", Supported: true},
		{BackendKey: "private_key_b", Name: "Device", Type: "input", Supported: true},
	}
	overlay := DiscoverOverlay(template, "fixture-discovery")
	columns := overlay.Fields["devices"].Columns
	if _, ok := columns["Device"]; !ok {
		t.Fatalf("DiscoverOverlay() columns = %#v, want display-derived public name Device", columns)
	}
	if _, ok := columns["Device-2"]; !ok {
		t.Fatalf("DiscoverOverlay() columns = %#v, want deterministic duplicate name Device-2", columns)
	}
	for _, privateKey := range []string{"private_key_a", "private_key_b"} {
		if _, leaked := columns[privateKey]; leaked {
			t.Fatalf("DiscoverOverlay() used backend key %q as a public column name", privateKey)
		}
	}
}

func TestOverlayDiscoverFailsValidationWhenColumnsCannotMatchUniquely(t *testing.T) {
	t.Parallel()

	template := overlayValidationTemplate()
	template.Fields[0].TableColumns = []TableColumn{
		{Name: "Device", Type: "input", Supported: true},
		{Name: "Device", Type: "input", Supported: true},
	}
	overlay := DiscoverOverlay(template, "fixture-discovery")
	overlay.Routing = validDraftRouting()

	if _, err := ValidateOverlay(template, overlay); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ValidateOverlay(discovered duplicate columns) error = %v, want ambiguity failure", err)
	}
}

func TestOverlayDiscoverPreservesEmptyTableColumnsInJSON(t *testing.T) {
	t.Parallel()

	template := overlayValidationTemplate()
	template.Fields[0].TableColumns = nil
	overlay := DiscoverOverlay(template, "fixture-discovery")
	overlay.Routing = validDraftRouting()
	data, err := json.Marshal(overlay)
	if err != nil {
		t.Fatalf("json.Marshal(discovered overlay) error = %v", err)
	}
	decoded, err := DecodeOverlay(data)
	if err != nil {
		t.Fatalf("DecodeOverlay(discovered empty table) error = %v; JSON = %s", err, data)
	}
	if decoded.Baseline.Fields[0].Columns == nil || len(decoded.Baseline.Fields[0].Columns) != 0 {
		t.Fatalf("decoded empty table columns = %#v, want present empty array", decoded.Baseline.Fields[0].Columns)
	}
}

func TestDiscoverOverlayIncludesMetadataFingerprintHashes(t *testing.T) {
	t.Parallel()

	template := overlayValidationTemplate()
	template.Fields[1].Conditions = ConditionSet{Operator: "and", Conditions: []Condition{{Field: "fault_type", Operator: "==", Value: "disk"}}}
	template.Fields[0].TableColumns[0].DefaultAvailable = true
	template.Fields[0].TableColumns[0].DefaultValue = "host-1"
	template.Fields[0].TableColumns[0].Conditions = ConditionSet{Operator: "or", Conditions: []Condition{{Field: "enabled", Operator: "==", Value: true}}}
	overlay := DiscoverOverlay(template, "fixture-discovery")

	field := findBaselineField(overlay.Baseline.Fields, "details")
	if field.DefaultHash != "" || field.ConditionHash == "" {
		t.Fatalf("details fingerprint hashes = %+v, want conditionHash only", field)
	}
	column := findBaselineField(overlay.Baseline.Fields, "devices").Columns[0]
	if column.DefaultHash == "" || column.ConditionHash == "" {
		t.Fatalf("table column fingerprint hashes = %+v, want defaultHash and conditionHash", column)
	}
	for _, value := range []string{field.ConditionHash, column.DefaultHash, column.ConditionHash} {
		if !hashPattern.MatchString(value) {
			t.Fatalf("fingerprint hash = %q, want sha256 digest", value)
		}
	}
}

func findBaselineField(fields []BaselineField, stableID string) BaselineField {
	for _, field := range fields {
		if field.StableID == stableID {
			return field
		}
	}
	return BaselineField{}
}

func TestDiscoverOverlayTerminatesWithInvalidOccupiedNames(t *testing.T) {
	t.Parallel()

	t.Run("fields", func(t *testing.T) {
		t.Parallel()
		template := overlayValidationTemplate()
		template.Fields[0].StableID = "field-2"
		template.Fields[1].StableID = strings.Repeat("x", 129)

		overlay := discoverOverlayWithin(t, template, "fixture-discovery")
		if _, ok := overlay.Fields["field-2"]; !ok {
			t.Fatalf("discovered fields = %#v, want occupied candidate", overlay.Fields)
		}
		if _, ok := overlay.Fields["field-2-2"]; !ok {
			t.Fatalf("discovered fields = %#v, want progressed fallback", overlay.Fields)
		}
	})

	t.Run("columns", func(t *testing.T) {
		t.Parallel()
		template := overlayValidationTemplate()
		template.Fields[0].TableColumns = []TableColumn{
			{BackendKey: "first", Name: "column-2", Type: "input", Supported: true},
			{BackendKey: "second", Name: strings.Repeat("x", 129), Type: "input", Supported: true},
		}

		overlay := discoverOverlayWithin(t, template, "fixture-discovery")
		columns := overlay.Fields["devices"].Columns
		if _, ok := columns["column-2"]; !ok {
			t.Fatalf("discovered columns = %#v, want occupied candidate", columns)
		}
		if _, ok := columns["column-2-2"]; !ok {
			t.Fatalf("discovered columns = %#v, want progressed fallback", columns)
		}
	})
}

func discoverOverlayWithin(t *testing.T, template Template, name string) Overlay {
	t.Helper()
	done := make(chan Overlay, 1)
	go func() { done <- DiscoverOverlay(template, name) }()
	select {
	case overlay := <-done:
		return overlay
	case <-time.After(500 * time.Millisecond):
		t.Fatal("DiscoverOverlay() did not terminate")
		return Overlay{}
	}
}

func replaceOverlayText(t *testing.T, input, old, replacement string) string {
	t.Helper()
	input = strings.ReplaceAll(input, "\r\n", "\n")
	old = strings.ReplaceAll(old, `\n`, "\n")
	replacement = strings.ReplaceAll(replacement, `\n`, "\n")
	if !strings.Contains(input, old) {
		t.Fatalf("test fixture does not contain mutation target %q", old)
	}
	return strings.Replace(input, old, replacement, 1)
}

func overlayValidationTemplate() Template {
	return Template{
		Target:       EditTarget{BusinessID: 15, ServiceID: 4003},
		SnapshotID:   "218527",
		ServiceKey:   "request",
		ServiceName:  "Fixture Server Hardware Repair",
		MetadataHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Fields: []Field{
			{
				BackendKey: "field_devices_1003", StableID: "devices", Name: "Affected devices",
				Type: "extend_table", Required: true, Supported: true,
				TableColumns: []TableColumn{
					{BackendKey: "host_name", Name: "Host name", Type: "input", Required: true, Supported: true},
					{BackendKey: "component", Name: "Component", Type: "input", Required: true, Supported: true},
				},
			},
			{
				BackendKey: "field_details_1004", StableID: "details", Name: "Failure details", Type: "textarea", Supported: true,
				Conditions: ConditionSet{Operator: "and", Conditions: []Condition{{Field: "fault_type", Operator: "==", Value: "disk"}}},
			},
			{
				BackendKey: "field_fault_1002", StableID: "fault_type", Name: "Fault type", Type: "select", Required: true,
				DefaultValue: "disk", DefaultAvailable: true,
				Choices:    []Choice{{Label: "Disk", Value: "disk"}, {Label: "Memory", Value: "memory"}},
				ChoiceHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Supported: true,
			},
			{BackendKey: "field_title_1001", StableID: "title", Name: "Title", Type: "input", Required: true, DefaultValue: "", DefaultAvailable: true, Supported: true},
		},
	}
}

func mustJSONValue(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%#v): %v", value, err)
	}
	return data
}

func validDraftRouting() Routing {
	return Routing{
		DisplayName:  "Maintainer-completed profile",
		UseWhen:      "The maintainer has confirmed this ticket type applies",
		DoNotUseWhen: "The maintainer has confirmed another ticket type applies",
		Examples:     []string{},
	}
}
