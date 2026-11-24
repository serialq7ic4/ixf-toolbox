package servicedeskprofile

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestRebaseOverlayUnchangedPreservesCanonicalProfile(t *testing.T) {
	previous := decodeFixtureOverlay(t)
	want, err := EncodeProfile(previous)
	if err != nil {
		t.Fatal(err)
	}

	result, err := RebaseOverlay(previous, overlayValidationTemplate())
	if err != nil {
		t.Fatal(err)
	}
	assertValidRebase(t, result, overlayValidationTemplate())
	got, err := EncodeProfile(result.Overlay)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unchanged rebase changed canonical profile\nwant:\n%s\ngot:\n%s", want, got)
	}

	result.Overlay.Routing.Examples[0] = "mutated"
	if previous.Routing.Examples[0] == "mutated" {
		t.Fatal("RebaseOverlay reused the previous routing examples slice")
	}
}

func TestRebaseOverlayPreservesCustomizationAcrossSnapshotAndDisplayNameChanges(t *testing.T) {
	previous := decodeFixtureOverlay(t)
	template := overlayValidationTemplate()
	template.SnapshotID = "218528"
	template.MetadataHash = "sha256:" + strings.Repeat("b", 64)
	template.Fields[1].Name = "Incident details"
	template.Fields[3].Name = "Ticket title"

	result, err := RebaseOverlay(previous, template)
	if err != nil {
		t.Fatal(err)
	}
	assertValidRebase(t, result, template)
	if got := result.Overlay.Fields["details"]; got.Match.Name != "Incident details" || got.Description == "" {
		t.Fatalf("details = %+v, want renamed matcher and preserved description", got)
	}
	if got := result.Overlay.Fields["title"]; got.Match.Name != "Ticket title" || !reflect.DeepEqual(got.Aliases, []string{"ticketTitle"}) {
		t.Fatalf("title = %+v, want renamed matcher and preserved alias", got)
	}
	assertRebaseChange(t, result.Preserved, "details", "", "stableId", "description")
	assertRebaseChange(t, result.Preserved, "title", "", "stableId", "aliases")
}

func TestRebaseOverlayAddsOptionalFieldWithoutCollidingWithPreservedAlias(t *testing.T) {
	previous := decodeFixtureOverlay(t)
	template := overlayValidationTemplate()
	template.MetadataHash = "sha256:" + strings.Repeat("c", 64)
	template.Fields = append(template.Fields, Field{
		BackendKey: "field_notes", StableID: "ticketTitle", Name: "Notes", Type: "textarea", Supported: true,
	})

	result, err := RebaseOverlay(previous, template)
	if err != nil {
		t.Fatal(err)
	}
	assertValidRebase(t, result, template)
	if _, ok := result.Overlay.Fields["ticketTitle-2"]; !ok {
		t.Fatalf("fields = %#v, want conflict-free ticketTitle-2", result.Overlay.Fields)
	}
	assertRebaseChange(t, result.Added, "ticketTitle-2", "", "")
}

func TestRebaseOverlayClassifiesMissingTypeChangedFallbackAndAmbiguousFields(t *testing.T) {
	t.Run("customized missing is unresolved and plain missing is removed", func(t *testing.T) {
		template := overlayValidationTemplate()
		previous := discoveredRebaseProfile(template)
		details := previous.Fields["details"]
		details.Description = "Keep this guidance"
		previous.Fields["details"] = details
		template.Fields = []Field{template.Fields[0], template.Fields[3]}

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		assertRebaseUnresolved(t, result.Unresolved, "field_missing", "details", "", "description")
		assertRebaseChange(t, result.Removed, "fault_type", "", "")
		if _, ok := result.Overlay.Fields["details"]; ok {
			t.Fatal("missing customized field was silently retained")
		}
	})

	t.Run("type change is unresolved", func(t *testing.T) {
		template := overlayValidationTemplate()
		previous := discoveredRebaseProfile(template)
		title := previous.Fields["title"]
		title.Description = "Keep this title guidance"
		previous.Fields["title"] = title
		template.Fields[3].Type = "number"

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		assertRebaseUnresolved(t, result.Unresolved, "field_type_changed", "title", "", "description")
	})

	t.Run("stable ID change uses unique name fallback", func(t *testing.T) {
		previous := decodeFixtureOverlay(t)
		template := overlayValidationTemplate()
		template.Fields[1].StableID = "details-v2"

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Overlay.Fields["details"].Match.StableID; got != "details-v2" {
			t.Fatalf("details stableId = %q, want name fallback target", got)
		}
		assertRebaseChange(t, result.Preserved, "details", "", "name", "description")
	})

	t.Run("duplicate fallback name is unresolved", func(t *testing.T) {
		previous := decodeFixtureOverlay(t)
		template := overlayValidationTemplate()
		template.Fields[1].StableID = "details-v2"
		template.Fields = append(template.Fields, Field{
			BackendKey: "field_details_duplicate", StableID: "details-v3", Name: "Failure details", Type: "textarea", Supported: true,
		})

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		assertRebaseUnresolved(t, result.Unresolved, "field_ambiguous", "details", "", "description")
	})
}

func TestRebaseOverlayStableIDClaimWinsFieldCompetition(t *testing.T) {
	previous := discoveredRebaseProfile(overlayValidationTemplate())
	previous.Baseline.Fields = append(previous.Baseline.Fields, BaselineField{
		StableID: "old-title-copy",
		Name:     "Title",
		Type:     "input",
		Required: true,
	})
	previous.Fields["title-copy"] = OverlayField{
		Match:       FieldMatcher{StableID: "old-title-copy", Name: "Title"},
		Description: "A competing customization",
	}

	result, err := RebaseOverlay(previous, overlayValidationTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Overlay.Fields["title"]; !ok {
		t.Fatal("stableId claimant did not retain the live field")
	}
	assertRebaseUnresolved(t, result.Unresolved, "field_ambiguous", "title-copy", "", "publicName", "description")
}

func TestRebaseOverlayMigratesAndClassifiesTableColumns(t *testing.T) {
	t.Run("key match name fallback and added column", func(t *testing.T) {
		previous := decodeFixtureOverlay(t)
		devices := previous.Fields["devices"]
		host := devices.Columns["hostName"]
		host.Description = "Host identifier"
		devices.Columns["hostName"] = host
		part := devices.Columns["part"]
		part.Description = "Failed component"
		devices.Columns["part"] = part
		previous.Fields["devices"] = devices

		template := overlayValidationTemplate()
		template.Fields[0].TableColumns[0].Name = "Hostname"
		template.Fields[0].TableColumns[1].BackendKey = "component-v2"
		template.Fields[0].TableColumns = append(template.Fields[0].TableColumns,
			TableColumn{BackendKey: "rack", Name: "Rack", Type: "input", Supported: true})

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		assertValidRebase(t, result, template)
		columns := result.Overlay.Fields["devices"].Columns
		if columns["hostName"].Match.Name != "Hostname" || columns["part"].Match.Key != "component-v2" {
			t.Fatalf("columns = %+v, want updated live matchers", columns)
		}
		assertRebaseChange(t, result.Preserved, "devices", "hostName", "key", "publicName", "description")
		assertRebaseChange(t, result.Preserved, "devices", "part", "name", "publicName", "description")
		assertRebaseChange(t, result.Added, "devices", "Rack", "")
	})

	t.Run("customized deletion is unresolved and plain deletion is removed", func(t *testing.T) {
		template := overlayValidationTemplate()
		previous := discoveredRebaseProfile(template)
		devices := previous.Fields["devices"]
		host := devices.Columns["Host name"]
		host.Description = "Keep host semantics"
		devices.Columns["Host name"] = host
		previous.Fields["devices"] = devices
		template.Fields[0].TableColumns = nil

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		assertRebaseUnresolved(t, result.Unresolved, "column_missing", "devices", "Host name", "description")
		assertRebaseChange(t, result.Removed, "devices", "Component", "")
	})

	t.Run("type change and ambiguity are unresolved", func(t *testing.T) {
		for name, mutate := range map[string]func(*Template){
			"type": func(template *Template) { template.Fields[0].TableColumns[1].Type = "number" },
			"ambiguous": func(template *Template) {
				template.Fields[0].TableColumns[1].BackendKey = "component-v2"
				template.Fields[0].TableColumns = append(template.Fields[0].TableColumns,
					TableColumn{BackendKey: "component-v3", Name: "Component", Type: "input", Supported: true})
			},
		} {
			t.Run(name, func(t *testing.T) {
				previous := decodeFixtureOverlay(t)
				devices := previous.Fields["devices"]
				part := devices.Columns["part"]
				part.Description = "Keep component semantics"
				devices.Columns["part"] = part
				previous.Fields["devices"] = devices
				template := overlayValidationTemplate()
				mutate(&template)

				result, err := RebaseOverlay(previous, template)
				if err != nil {
					t.Fatal(err)
				}
				kind := "column_" + name
				if name == "type" {
					kind = "column_type_changed"
				}
				assertRebaseUnresolved(t, result.Unresolved, kind, "devices", "part", "publicName", "description")
			})
		}
	})
}

func TestRebaseOverlayDropsInvalidDefaultsIntoUnresolved(t *testing.T) {
	t.Run("select choice removed", func(t *testing.T) {
		previous := decodeFixtureOverlay(t)
		field := previous.Fields["faultType"]
		field.Default = mustJSONValue(t, "disk")
		previous.Fields["faultType"] = field
		template := overlayValidationTemplate()
		template.Fields[2].Choices = []Choice{{Label: "Memory", Value: "memory"}}

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		if result.Overlay.Fields["faultType"].Default != nil {
			t.Fatal("invalid select default was retained")
		}
		unresolved := assertRebaseUnresolved(t, result.Unresolved, "default_invalid", "faultType", "", "default")
		if strings.Contains(unresolved.Message, "disk") {
			t.Fatalf("unresolved message leaked default value: %q", unresolved.Message)
		}
		assertDefaultOnlyPrevious(t, unresolved.Previous)
	})

	t.Run("table row references deleted column", func(t *testing.T) {
		previous := decodeFixtureOverlay(t)
		devices := previous.Fields["devices"]
		devices.Default = mustJSONValue(t, []any{map[string]any{"hostName": "host-1", "part": "disk"}})
		previous.Fields["devices"] = devices
		template := overlayValidationTemplate()
		template.Fields[0].TableColumns = template.Fields[0].TableColumns[:1]

		result, err := RebaseOverlay(previous, template)
		if err != nil {
			t.Fatal(err)
		}
		if result.Overlay.Fields["devices"].Default != nil {
			t.Fatal("invalid table default was retained")
		}
		unresolved := assertRebaseUnresolved(t, result.Unresolved, "default_invalid", "devices", "", "default")
		assertDefaultOnlyPrevious(t, unresolved.Previous)
	})
}

func TestRebaseOverlayDropsRequiredFalseOverride(t *testing.T) {
	previous := discoveredRebaseProfile(overlayValidationTemplate())
	value := false
	field := previous.Fields["details"]
	field.Required = &value
	previous.Fields["details"] = field

	result, err := RebaseOverlay(previous, overlayValidationTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if result.Overlay.Fields["details"].Required != nil {
		t.Fatal("required:false override was migrated")
	}
	if previous.Fields["details"].Required == nil || *previous.Fields["details"].Required {
		t.Fatal("RebaseOverlay mutated previous required override")
	}
}

func TestRebaseOverlayIsDeterministicWithManyFields(t *testing.T) {
	template := overlayValidationTemplate()
	for index := 0; index < 80; index++ {
		template.Fields = append(template.Fields, Field{
			BackendKey: "generated-backend", StableID: "generated-" + strconv.Itoa(index), Name: "Generated", Type: "input", Supported: true,
		})
	}
	previous := discoveredRebaseProfile(template)
	current := template
	current.Fields = append([]Field(nil), template.Fields...)
	current.Fields = append(current.Fields[:10], current.Fields[20:]...)
	current.Fields = append(current.Fields, Field{BackendKey: "new", StableID: "generated-new", Name: "Generated", Type: "input", Supported: true})

	want, err := RebaseOverlay(previous, current)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 20; run++ {
		got, err := RebaseOverlay(previous, current)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d produced non-deterministic result", run)
		}
	}
	assertSortedChanges(t, want.Preserved)
	assertSortedChanges(t, want.Added)
	assertSortedChanges(t, want.Removed)
	assertSortedUnresolved(t, want.Unresolved)
}

func TestRebaseOverlayRejectsInvalidPreviousAndTargetMismatch(t *testing.T) {
	previous := decodeFixtureOverlay(t)
	invalid := previous
	invalid.Name = "INVALID"
	if _, err := RebaseOverlay(invalid, overlayValidationTemplate()); err == nil {
		t.Fatal("RebaseOverlay accepted invalid previous profile")
	}

	template := overlayValidationTemplate()
	template.Target.ServiceID++
	if _, err := RebaseOverlay(previous, template); err == nil {
		t.Fatal("RebaseOverlay accepted target mismatch")
	}
}

func discoveredRebaseProfile(template Template) Overlay {
	overlay := DiscoverOverlay(template, "rebase-profile")
	overlay.Description = "Rebase test profile"
	overlay.Routing = validDraftRouting()
	return overlay
}

func assertValidRebase(t *testing.T, result RebaseResult, template Template) {
	t.Helper()
	if err := ValidateOverlaySchema(result.Overlay); err != nil {
		t.Fatalf("rebased overlay schema error = %v", err)
	}
	if _, err := ResolveOverlay(result.Overlay, template); err != nil {
		t.Fatalf("rebased overlay resolution error = %v", err)
	}
}

func assertRebaseChange(t *testing.T, changes []RebaseFieldChange, field, column, matchedBy string, properties ...string) RebaseFieldChange {
	t.Helper()
	for _, change := range changes {
		if change.Field != field || change.Column != column {
			continue
		}
		if change.MatchedBy != matchedBy {
			t.Fatalf("change %s/%s matchedBy = %q, want %q", field, column, change.MatchedBy, matchedBy)
		}
		if !reflect.DeepEqual(change.Properties, properties) {
			t.Fatalf("change %s/%s properties = %v, want %v", field, column, change.Properties, properties)
		}
		return change
	}
	t.Fatalf("changes = %+v, want %s/%s", changes, field, column)
	return RebaseFieldChange{}
}

func assertRebaseUnresolved(t *testing.T, unresolved []RebaseUnresolved, kind, field, column string, properties ...string) RebaseUnresolved {
	t.Helper()
	for _, item := range unresolved {
		if item.Kind != kind || item.Field != field || item.Column != column {
			continue
		}
		if !reflect.DeepEqual(item.Properties, properties) {
			t.Fatalf("unresolved %s/%s properties = %v, want %v", field, column, item.Properties, properties)
		}
		if item.ID == "" || len(item.Previous) == 0 || item.Message == "" {
			t.Fatalf("unresolved item is incomplete: %+v", item)
		}
		return item
	}
	t.Fatalf("unresolved = %+v, want %s %s/%s", unresolved, kind, field, column)
	return RebaseUnresolved{}
}

func assertDefaultOnlyPrevious(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshal unresolved previous: %v", err)
	}
	if len(value) != 1 || value["default"] == nil {
		t.Fatalf("default_invalid previous = %s, want only default", raw)
	}
}

func assertSortedChanges(t *testing.T, changes []RebaseFieldChange) {
	t.Helper()
	keys := make([]string, len(changes))
	for index, change := range changes {
		keys[index] = change.Field + "\x00" + change.Column
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("changes are not sorted: %+v", changes)
	}
}

func assertSortedUnresolved(t *testing.T, unresolved []RebaseUnresolved) {
	t.Helper()
	keys := make([]string, len(unresolved))
	for index, item := range unresolved {
		keys[index] = item.Field + "\x00" + item.Column + "\x00" + item.Kind
		if item.ID != "unresolved-"+strconv.Itoa(index+1) {
			t.Fatalf("unresolved ID = %q at index %d", item.ID, index)
		}
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("unresolved items are not sorted: %+v", unresolved)
	}
}
