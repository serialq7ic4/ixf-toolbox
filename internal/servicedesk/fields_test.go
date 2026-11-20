package servicedesk

import (
	"encoding/json"
	"html"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeTicketInputDirectAndFieldsWrapper(t *testing.T) {
	t.Parallel()

	direct, err := DecodeTicketInput([]byte(`{"title":"direct","count":2}`))
	if err != nil {
		t.Fatalf("DecodeTicketInput(direct) error = %v", err)
	}
	if direct["title"] != "direct" || direct["count"] != json.Number("2") {
		t.Fatalf("direct input = %#v", direct)
	}

	wrapper, err := DecodeTicketInput([]byte(`{"fields":{"title":"wrapped"}}`))
	if err != nil {
		t.Fatalf("DecodeTicketInput(wrapper) error = %v", err)
	}
	if wrapper["title"] != "wrapped" || len(wrapper) != 1 {
		t.Fatalf("wrapped input = %#v", wrapper)
	}
}

func TestDecodeTicketInputRejectsUnknownWrapperProperties(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"fields":{},"metadata":{}}`,
		`{"fields":{},"fields":{}}`,
		`{"fields":[]}`,
	} {
		if _, err := DecodeTicketInput([]byte(raw)); err == nil {
			t.Errorf("DecodeTicketInput(%s) succeeded, want wrapper error", raw)
		}
	}
}

func TestDecodeTicketInputRejectsDuplicateJSONKeys(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"title":"one","title":"two"}`,
		`{"fields":{"title":"one","title":"two"}}`,
		`{"fields":{"editor":{"html":"one","html":"two"}}}`,
	} {
		if _, err := DecodeTicketInput([]byte(raw)); err == nil {
			t.Errorf("DecodeTicketInput(%s) succeeded, want duplicate-key error", raw)
		}
	}
}

func TestNormalizeSupportedFieldTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		field      Field
		input      any
		want       any
		wantCode   string
		wantSource string
	}{
		{name: "string", field: Field{BackendKey: "f_string", StableID: "title", Name: "Title", Type: "input", Required: true, Supported: true}, input: "hello", want: "hello", wantSource: "input"},
		{name: "number", field: Field{BackendKey: "f_number", StableID: "count", Name: "Count", Type: "number", Supported: true}, input: json.Number("12.5"), want: json.Number("12.5"), wantSource: "input"},
		{name: "number rejects string", field: Field{BackendKey: "f_number", StableID: "count", Name: "Count", Type: "number", Supported: true}, input: "12.5", wantCode: "invalid_type"},
		{name: "date", field: Field{BackendKey: "f_date", StableID: "date", Name: "Date", Type: "date", Supported: true}, input: "2026-11-20", want: "2026-11-20", wantSource: "input"},
		{name: "time", field: Field{BackendKey: "f_time", StableID: "time", Name: "Time", Type: "time", Supported: true}, input: "2026-11-20 12:30:45", want: "2026-11-20 12:30:45", wantSource: "input"},
		{name: "only time", field: Field{BackendKey: "f_only_time", StableID: "onlyTime", Name: "Only time", Type: "only_time", Supported: true}, input: "12:30:45", want: "12:30:45", wantSource: "input"},
		{name: "datetime range", field: Field{BackendKey: "f_date_range", StableID: "dateRange", Name: "Date range", Type: "date_time_range", Supported: true}, input: []any{"2026-11-20 12:00:00", "2026-11-20 13:00:00"}, want: []any{"2026-11-20 12:00:00", "2026-11-20 13:00:00"}, wantSource: "input"},
		{name: "time range", field: Field{BackendKey: "f_time_range", StableID: "timeRange", Name: "Time range", Type: "time_range", Supported: true}, input: []any{"12:00:00", "13:00:00"}, want: []any{"12:00:00", "13:00:00"}, wantSource: "input"},
		{name: "switch", field: Field{BackendKey: "f_switch", StableID: "enabled", Name: "Enabled", Type: "switch", Supported: true}, input: true, want: true, wantSource: "input"},
		{name: "single choice label", field: Field{BackendKey: "f_choice", StableID: "kind", Name: "Kind", Type: "select", Supported: true, Choices: []Choice{{Label: "Disk", Value: "disk"}}}, input: "Disk", want: "disk", wantSource: "input"},
		{name: "single choice value", field: Field{BackendKey: "f_choice", StableID: "kind", Name: "Kind", Type: "select", Supported: true, Choices: []Choice{{Label: "Disk", Value: "disk"}}}, input: "disk", want: "disk", wantSource: "input"},
		{name: "multi choice", field: Field{BackendKey: "f_multi", StableID: "kinds", Name: "Kinds", Type: "multi_select", Supported: true, Choices: []Choice{{Label: "Disk", Value: "disk"}, {Label: "Memory", Value: "memory"}}}, input: []any{"Disk", "memory"}, want: []any{"disk", "memory"}, wantSource: "input"},
		{name: "optional null", field: Field{BackendKey: "f_optional", StableID: "optional", Name: "Optional", Type: "input", Supported: true}, input: nil, wantSource: "explicit_null"},
		{name: "required null", field: Field{BackendKey: "f_required", StableID: "required", Name: "Required", Type: "input", Required: true, Supported: true}, input: nil, wantCode: "required_null"},
		{name: "tui editor plain text", field: Field{BackendKey: "f_editor", StableID: "editor", Name: "Editor", Type: "tui_editor", Supported: true}, input: `<b>&`, want: "<p>&lt;b&gt;&amp;</p>", wantSource: "input"},
		{name: "tui editor exact html object", field: Field{BackendKey: "f_editor", StableID: "editor", Name: "Editor", Type: "tui_editor", Supported: true}, input: map[string]any{"html": "<p>ok</p>"}, want: map[string]any{"html": "<p>ok</p>"}, wantSource: "input"},
		{name: "empty attachments", field: Field{BackendKey: "f_attachment", StableID: "attachments", Name: "Attachments", Type: "attachments", Supported: false}, input: []any{}, want: []any{}, wantSource: "input"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			template := Template{Fields: []Field{test.field}}
			got, issues := NormalizeInput(template, nil, TicketInput{test.field.StableID: test.input})
			if test.wantCode != "" {
				if !hasFieldIssue(issues, test.wantCode, test.field.StableID) {
					t.Fatalf("issues = %#v, want %s for %s", issues, test.wantCode, test.field.StableID)
				}
				return
			}
			if len(issues) != 0 {
				t.Fatalf("NormalizeInput() issues = %#v", issues)
			}
			if !reflect.DeepEqual(got.Inputs[test.field.BackendKey], test.want) {
				t.Fatalf("normalized value = %#v, want %#v", got.Inputs[test.field.BackendKey], test.want)
			}
			if got.Sources[test.field.StableID] != test.wantSource {
				t.Fatalf("source = %q, want %q", got.Sources[test.field.StableID], test.wantSource)
			}
		})
	}

	_ = html.EscapeString
}

func TestNormalizeInputReportsStablePublicFieldMetadata(t *testing.T) {
	t.Parallel()

	template := Template{Fields: []Field{{BackendKey: "private_title", StableID: "title", Name: "Title", Type: "input", Supported: true}}}
	resolved := &ResolvedOverlay{
		Fields: map[string]ResolvedField{
			"title": {PublicName: "title", Field: template.Fields[0], Overlay: OverlayField{Aliases: []string{"ticketTitle"}}},
		},
		Aliases: map[string]string{"title": "title", "ticketTitle": "title"},
	}
	_, issues := NormalizeInput(template, resolved, TicketInput{"unknown": "x", "title": "a", "ticketTitle": "b"})
	if !hasFieldIssue(issues, "unknown_field", "unknown") {
		t.Fatalf("issues = %#v, want unknown field", issues)
	}
	if !hasFieldIssue(issues, "duplicate_field", "title") {
		t.Fatalf("issues = %#v, want duplicate public field", issues)
	}
	for _, issue := range issues {
		if strings.Contains(issue.Message, "private_title") {
			t.Fatalf("issue leaked backend field key: %#v", issue)
		}
	}
}

func TestNormalizeTableColumnsToBackendKeys(t *testing.T) {
	t.Parallel()

	field := Field{BackendKey: "f_devices", StableID: "devices", Name: "Devices", Type: "extend_table", Required: true, Supported: true, TableColumns: []TableColumn{
		{BackendKey: "host_name", Name: "Host name", Type: "input", Required: true, Supported: true},
		{BackendKey: "component", Name: "Component", Type: "select", Required: true, Supported: true, Choices: []Choice{{Label: "Disk", Value: "disk"}}},
	}}
	resolved := &ResolvedOverlay{Fields: map[string]ResolvedField{
		"devices": {
			PublicName: "devices", Field: field,
			Columns: map[string]ResolvedColumn{
				"hostName": {PublicName: "hostName", Column: field.TableColumns[0], LiveIndex: 0, Overlay: OverlayColumn{Aliases: []string{"host"}}},
				"part":     {PublicName: "part", Column: field.TableColumns[1], LiveIndex: 1},
			},
		},
	}, Aliases: map[string]string{"devices": "devices"}}
	input := TicketInput{"devices": []any{map[string]any{"host": "server-01", "component": "Disk"}}}
	got, issues := NormalizeInput(Template{Fields: []Field{field}}, resolved, input)
	if len(issues) != 0 {
		t.Fatalf("NormalizeInput() issues = %#v", issues)
	}
	rows := got.Inputs["f_devices"].([]any)
	row := rows[0].(map[string]any)
	if row["host_name"] != "server-01" || row["component"] != "disk" {
		t.Fatalf("normalized row = %#v", row)
	}
	if got.TableRowCount != 1 {
		t.Fatalf("TableRowCount = %d, want 1", got.TableRowCount)
	}
}

func TestNormalizeTableRejectsUnknownDuplicateAndMissingRequiredColumns(t *testing.T) {
	t.Parallel()

	field := Field{BackendKey: "f_table", StableID: "table", Name: "Table", Type: "extend_table", Required: true, Supported: true, TableColumns: []TableColumn{
		{BackendKey: "host", Name: "Host", Type: "input", Required: true, Supported: true},
		{BackendKey: "part", Name: "Part", Type: "input", Required: true, Supported: true},
	}}
	for name, row := range map[string]any{
		"unknown": map[string]any{"host": "a", "part": "b", "other": "c"},
		"duplicate": map[string]any{"host": "a", "Host": "b", "part": "c"},
		"missing required": map[string]any{"host": "a"},
	} {
		name, row := name, row
		t.Run(name, func(t *testing.T) {
			_, issues := NormalizeInput(Template{Fields: []Field{field}}, nil, TicketInput{"table": []any{row}})
			if len(issues) == 0 || issues[0].Field != "table" || issues[0].Column == "" || issues[0].RowIndex != 0 {
				t.Fatalf("issues = %#v, want actionable table issue", issues)
			}
		})
	}
}

func TestNormalizeRejectsAmbiguousChoiceLabel(t *testing.T) {
	t.Parallel()

	field := Field{BackendKey: "f_kind", StableID: "kind", Name: "Kind", Type: "select", Supported: true, Choices: []Choice{{Label: "Disk", Value: "disk-a"}, {Label: "Disk", Value: "disk-b"}}}
	_, issues := NormalizeInput(Template{Fields: []Field{field}}, nil, TicketInput{"kind": "Disk"})
	if !hasFieldIssue(issues, "ambiguous_choice", "kind") {
		t.Fatalf("issues = %#v, want ambiguous choice", issues)
	}
}

func TestNormalizeDefaultPrecedence(t *testing.T) {
	t.Parallel()

	fields := []Field{
		{BackendKey: "f_explicit", StableID: "explicit", Name: "Explicit", Type: "input", Supported: true, DefaultValue: "live", DefaultAvailable: true},
		{BackendKey: "f_overlay", StableID: "overlay", Name: "Overlay", Type: "input", Supported: true, DefaultValue: "live", DefaultAvailable: true},
		{BackendKey: "f_live", StableID: "live", Name: "Live", Type: "input", Supported: true, DefaultValue: "live", DefaultAvailable: true},
		{BackendKey: "f_optional", StableID: "optional", Name: "Optional", Type: "input", Supported: true},
		{BackendKey: "f_display", StableID: "display", Name: "Display", Type: "hyperlink", Supported: true, DisplayOnly: true, DefaultValue: "https://example.invalid", DefaultAvailable: true},
	}
	resolved := &ResolvedOverlay{Fields: map[string]ResolvedField{
		"explicit": {PublicName: "explicit", Field: fields[0], Overlay: OverlayField{Default: json.RawMessage(`"overlay"`)}},
		"overlay":  {PublicName: "overlay", Field: fields[1], Overlay: OverlayField{Default: json.RawMessage(`"overlay"`)}},
		"display":  {PublicName: "display", Field: fields[4], Overlay: OverlayField{Default: json.RawMessage(`"override"`)}},
	}}
	input := TicketInput{"explicit": "input", "display": "ignored"}
	got, issues := NormalizeInput(Template{Fields: fields}, resolved, input)
	if !hasFieldIssue(issues, "display_only", "display") {
		t.Fatalf("issues = %#v, want display-only issue", issues)
	}
	if got.Inputs["f_explicit"] != "input" || got.Inputs["f_overlay"] != "overlay" || got.Inputs["f_live"] != "live" {
		t.Fatalf("default precedence inputs = %#v", got.Inputs)
	}
	if _, ok := got.Inputs["f_optional"]; ok {
		t.Fatalf("optional field was not omitted: %#v", got.Inputs)
	}
}

func hasFieldIssue(issues []FieldIssue, code, field string) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Field == field {
			return true
		}
	}
	return false
}

