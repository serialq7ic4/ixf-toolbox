package servicedesk

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveOverlayStableIDThenUniqueName(t *testing.T) {
	t.Parallel()

	t.Run("stable ID wins over changed display name", func(t *testing.T) {
		t.Parallel()
		overlay := decodeFixtureOverlay(t)
		template := overlayValidationTemplate()
		template.Fields[3].Name = "Renamed title"

		resolved, err := ResolveOverlay(overlay, template)
		if err != nil {
			t.Fatalf("ResolveOverlay() error = %v", err)
		}
		if got := resolved.Fields["title"].Field.Name; got != "Renamed title" {
			t.Fatalf("resolved title name = %q, want stable-ID match", got)
		}
	})

	t.Run("unique name is fallback and records drift", func(t *testing.T) {
		t.Parallel()
		overlay := decodeFixtureOverlay(t)
		template := overlayValidationTemplate()
		template.Fields[1].StableID = "details-v2"

		resolved, err := ResolveOverlay(overlay, template)
		if err != nil {
			t.Fatalf("ResolveOverlay() error = %v", err)
		}
		if got := resolved.Fields["details"].Field.StableID; got != "details-v2" {
			t.Fatalf("resolved details stable ID = %q, want name fallback", got)
		}
		assertDriftChange(t, resolved.Drift, "matcher_name_fallback", "non_destructive")
	})
}

func TestResolveOverlayColumnKeyThenUniqueName(t *testing.T) {
	t.Parallel()

	overlay := decodeFixtureOverlay(t)
	template := overlayValidationTemplate()
	template.Fields[0].TableColumns[0].BackendKey = "host_name_v2"
	template.Fields[0].TableColumns[1].Name = "Part"

	resolved, err := ResolveOverlay(overlay, template)
	if err != nil {
		t.Fatalf("ResolveOverlay() error = %v", err)
	}
	devices := resolved.Fields["devices"]
	if got := devices.Columns["hostName"].Column.BackendKey; got != "host_name_v2" {
		t.Fatalf("resolved hostName key = %q, want name fallback", got)
	}
	if got := devices.Columns["part"].Column.Name; got != "Part" {
		t.Fatalf("resolved component name = %q, want key match", got)
	}
	assertDriftChange(t, resolved.Drift, "column_matcher_name_fallback", "non_destructive")

	encoded, err := json.Marshal(resolved.Drift)
	if err != nil {
		t.Fatalf("json.Marshal(drift) error = %v", err)
	}
	for _, privateKey := range []string{"host_name", "host_name_v2", `"column":"component"`} {
		if strings.Contains(string(encoded), privateKey) {
			t.Errorf("public drift contains backend column key %q: %s", privateKey, encoded)
		}
	}
}

func TestResolveOverlayRejectsAmbiguity(t *testing.T) {
	t.Parallel()

	tests := map[string]func(Template) Template{
		"duplicate stable ID": func(template Template) Template {
			template.Fields = append(template.Fields, Field{StableID: "details", Name: "Other", Type: "textarea", Supported: true})
			return template
		},
		"duplicate fallback name": func(template Template) Template {
			template.Fields[1].StableID = "details-v2"
			template.Fields = append(template.Fields, Field{StableID: "details-v3", Name: "Failure details", Type: "textarea", Supported: true})
			return template
		},
		"duplicate column key": func(template Template) Template {
			template.Fields[0].TableColumns = append(template.Fields[0].TableColumns,
				TableColumn{BackendKey: "host_name", Name: "Other host", Type: "input", Supported: true})
			return template
		},
		"duplicate column fallback name": func(template Template) Template {
			template.Fields[0].TableColumns[0].BackendKey = "host_name_v2"
			template.Fields[0].TableColumns = append(template.Fields[0].TableColumns,
				TableColumn{BackendKey: "host_name_v3", Name: "Host name", Type: "input", Supported: true})
			return template
		},
	}

	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolved, err := ResolveOverlay(decodeFixtureOverlay(t), mutate(overlayValidationTemplate()))
			if err == nil || !strings.Contains(err.Error(), "ambiguous") {
				t.Fatalf("ResolveOverlay() result = %+v, error = %v; want ambiguity error", resolved, err)
			}
			if !resolved.Drift.ApplyBlocked || resolved.Drift.Classification != "destructive" {
				t.Fatalf("ambiguity drift = %+v, want destructive apply block", resolved.Drift)
			}
		})
	}
}

func TestResolveOverlayRejectsDuplicateTargets(t *testing.T) {
	t.Parallel()

	t.Run("fields", func(t *testing.T) {
		t.Parallel()
		overlay := decodeFixtureOverlay(t)
		overlay.Fields["titleCopy"] = OverlayField{Match: FieldMatcher{StableID: "title", Name: "Title"}}

		resolved, err := ResolveOverlay(overlay, overlayValidationTemplate())
		if err == nil || !strings.Contains(err.Error(), "resolve ambiguously") {
			t.Fatalf("ResolveOverlay() error = %v, want duplicate field target failure", err)
		}
		assertDriftChange(t, resolved.Drift, "matcher_ambiguous", "destructive")
	})

	t.Run("columns", func(t *testing.T) {
		t.Parallel()
		overlay := decodeFixtureOverlay(t)
		devices := overlay.Fields["devices"]
		devices.Columns["hostCopy"] = OverlayColumn{Match: ColumnMatcher{Key: "host_name", Name: "Host name"}}
		overlay.Fields["devices"] = devices

		resolved, err := ResolveOverlay(overlay, overlayValidationTemplate())
		if err == nil || !strings.Contains(err.Error(), "resolve ambiguously") {
			t.Fatalf("ResolveOverlay() error = %v, want duplicate column target failure", err)
		}
		assertDriftChange(t, resolved.Drift, "column_matcher_ambiguous", "destructive")
	})
}

func TestResolveOverlayAccumulatesMatcherFailures(t *testing.T) {
	t.Parallel()

	overlay := decodeFixtureOverlay(t)
	template := overlayValidationTemplate()
	template.Fields = template.Fields[2:]

	resolved, err := ResolveOverlay(overlay, template)
	if err == nil {
		t.Fatal("ResolveOverlay() succeeded, want unresolved matcher errors")
	}
	if got := countDriftKind(resolved.Drift, "matcher_unresolved"); got != 2 {
		t.Fatalf("unresolved matcher changes = %d, want 2; drift = %+v", got, resolved.Drift)
	}
}

func TestValidateOverlayUsesLiveTemplate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		defaultValue any
		mutate       func(*Template)
	}{
		"type changed": {
			defaultValue: "ticket title",
			mutate: func(template *Template) {
				template.Fields[3].Type = "number"
			},
		},
		"choice removed": {
			defaultValue: "disk",
			mutate: func(template *Template) {
				template.Fields[2].Choices = []Choice{{Label: "Memory", Value: "memory"}}
				template.Fields[2].ChoiceHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
			},
		},
	}

	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			overlay := decodeFixtureOverlay(t)
			fieldName := "title"
			if name == "choice removed" {
				fieldName = "faultType"
			}
			field := overlay.Fields[fieldName]
			field.Default = mustJSONValue(t, test.defaultValue)
			overlay.Fields[fieldName] = field
			template := overlayValidationTemplate()
			test.mutate(&template)

			if _, err := ValidateOverlay(template, overlay); err == nil || !strings.Contains(err.Error(), "default") {
				t.Fatalf("ValidateOverlay() error = %v, want live default validation error", err)
			}
		})
	}
}

func TestDriftClassifiesSnapshotOnlyAndOptionalFieldAsNonDestructive(t *testing.T) {
	t.Parallel()

	baselineTemplate := overlayValidationTemplate()
	baseline := DiscoverOverlay(baselineTemplate, "drift-test").Baseline

	t.Run("snapshot only", func(t *testing.T) {
		t.Parallel()
		current := overlayValidationTemplate()
		current.SnapshotID = "218528"
		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "snapshot_changed", "non_destructive")
		if report.ApplyBlocked {
			t.Fatalf("snapshot-only drift blocked apply: %+v", report)
		}
	})

	t.Run("new optional field", func(t *testing.T) {
		t.Parallel()
		current := overlayValidationTemplate()
		current.MetadataHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		current.Fields = append(current.Fields, Field{StableID: "optional_note", Name: "Optional note", Type: "textarea", Supported: true})
		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "optional_field_added", "non_destructive")
		if report.ApplyBlocked || report.Classification != "non_destructive" {
			t.Fatalf("optional field drift = %+v, want non-destructive", report)
		}
	})
}

func TestDriftClassifiesMissingMatcherTypeChoiceRequiredAndConditionChangesAsDestructive(t *testing.T) {
	t.Parallel()

	baselineTemplate := overlayValidationTemplate()
	baseline := DiscoverOverlay(baselineTemplate, "drift-test").Baseline
	tests := map[string]struct {
		kind   string
		mutate func(*Template)
	}{
		"missing field": {
			kind: "field_missing",
			mutate: func(template *Template) {
				template.Fields = template.Fields[1:]
			},
		},
		"type changed": {
			kind: "field_type_changed",
			mutate: func(template *Template) {
				template.Fields[3].Type = "number"
			},
		},
		"choices changed": {
			kind: "field_choices_changed",
			mutate: func(template *Template) {
				template.Fields[2].ChoiceHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
			},
		},
		"required changed": {
			kind: "field_required_changed",
			mutate: func(template *Template) {
				template.Fields[1].Required = true
			},
		},
		"conditions changed": {
			kind: "field_condition_changed",
			mutate: func(template *Template) {
				template.MetadataHash = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
				template.Fields[1].Conditions = ConditionSet{}
			},
		},
		"new required field without default": {
			kind: "required_field_added",
			mutate: func(template *Template) {
				template.Fields = append(template.Fields, Field{StableID: "approval", Name: "Approval", Type: "input", Required: true, Supported: true})
			},
		},
	}

	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			current := overlayValidationTemplate()
			current.MetadataHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
			test.mutate(&current)
			report := CompareBaseline(baseline, current)
			assertDriftChange(t, report, test.kind, "destructive")
			if !report.ApplyBlocked || report.Classification != "destructive" {
				t.Fatalf("destructive drift = %+v, want apply block", report)
			}
		})
	}
}

func TestDriftClassifiesMetadataOnlyChangesByCurrentConsequence(t *testing.T) {
	t.Parallel()

	baselineTemplate := templateWithMetadataHash(t, overlayValidationTemplate())
	baseline := DiscoverOverlay(baselineTemplate, "drift-test").Baseline

	t.Run("unused live default is non-destructive", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[1].DefaultValue = "new optional default"
		current.Fields[1].DefaultAvailable = true
		current = templateWithMetadataHash(t, current)

		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "field_default_changed", "non_destructive")
		if report.ApplyBlocked {
			t.Fatalf("unused default drift blocked apply: %+v", report)
		}
	})

	t.Run("condition metadata is destructive", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[3].Conditions = ConditionSet{Operator: "and", Conditions: []Condition{{Field: "fault_type", Operator: "==", Value: "disk"}}}
		current = templateWithMetadataHash(t, current)

		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "field_condition_changed", "destructive")
		if !report.ApplyBlocked {
			t.Fatalf("condition drift did not block apply: %+v", report)
		}
	})

	t.Run("condition metadata is not hidden by display-name drift", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[3].Name = "Renamed title"
		current.Fields[3].Conditions = ConditionSet{Operator: "and", Conditions: []Condition{{Field: "fault_type", Operator: "==", Value: "disk"}}}
		current = templateWithMetadataHash(t, current)

		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "field_name_changed", "non_destructive")
		assertDriftChange(t, report, "field_condition_changed", "destructive")
		if !report.ApplyBlocked {
			t.Fatalf("condition drift with display-name change did not block apply: %+v", report)
		}
	})

	t.Run("unsupported current metadata is destructive", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[1].Supported = false
		current = templateWithMetadataHash(t, current)

		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "metadata_changed", "destructive")
	})
}

func TestDriftClassifiesFingerprintDefaultAndConditionChanges(t *testing.T) {
	t.Parallel()

	baselineTemplate := overlayValidationTemplate()
	baselineTemplate.Fields[0].TableColumns[0].DefaultAvailable = true
	baselineTemplate.Fields[0].TableColumns[0].DefaultValue = "host-1"
	baselineTemplate.Fields[0].TableColumns[0].Conditions = ConditionSet{Operator: "or", Conditions: []Condition{{Field: "enabled", Operator: "==", Value: true}}}
	baselineTemplate.Fields[1].Conditions = ConditionSet{Operator: "and", Conditions: []Condition{{Field: "fault_type", Operator: "==", Value: "disk"}}}
	baseline := DiscoverOverlay(baselineTemplate, "drift-test").Baseline

	t.Run("field default changes are non-destructive", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[1].DefaultAvailable = true
		current.Fields[1].DefaultValue = "new default"
		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "field_default_changed", "non_destructive")
		if report.ApplyBlocked {
			t.Fatalf("field default drift blocked apply: %+v", report)
		}
	})

	t.Run("field condition removal is destructive", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[1].Conditions = ConditionSet{}
		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "field_condition_changed", "destructive")
		if !report.ApplyBlocked {
			t.Fatalf("field condition removal did not block apply: %+v", report)
		}
	})

	t.Run("column hashes are compared", func(t *testing.T) {
		t.Parallel()
		current := baselineTemplate
		current.Fields = append([]Field(nil), baselineTemplate.Fields...)
		current.Fields[0].TableColumns = append([]TableColumn(nil), baselineTemplate.Fields[0].TableColumns...)
		current.Fields[0].TableColumns[0].Conditions = ConditionSet{}
		current.Fields[0].TableColumns[0].DefaultValue = "host-2"
		report := CompareBaseline(baseline, current)
		assertDriftChange(t, report, "column_default_changed", "non_destructive")
		assertDriftChange(t, report, "column_condition_changed", "destructive")
		if !report.ApplyBlocked {
			t.Fatalf("column condition removal did not block apply: %+v", report)
		}
	})
}

func TestResolveOverlayValidatesTableDefaultRows(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value any
		want  string
	}{
		"valid public names": {
			value: []any{map[string]any{"hostName": "host-1", "part": "disk"}},
		},
		"unknown name": {
			value: []any{map[string]any{"hostName": "host-1", "unknown": "disk"}},
			want:  "unknown table column",
		},
		"backend name": {
			value: []any{map[string]any{"host_name": "host-1", "part": "disk"}},
			want:  "backend table column",
		},
		"wrong type": {
			value: []any{map[string]any{"hostName": 42, "part": "disk"}},
			want:  "must be a string",
		},
		"duplicate public names": {
			value: []byte(`[{"hostName":"host-1","part":"disk","part":"memory"}]`),
			want:  "duplicate table column",
		},
		"missing required": {
			value: []any{map[string]any{"hostName": "host-1"}},
			want:  "missing required table column",
		},
	}

	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			overlay := decodeFixtureOverlay(t)
			field := overlay.Fields["devices"]
			if data, ok := test.value.([]byte); ok {
				field.Default = data
			} else {
				field.Default = mustJSONValue(t, test.value)
			}
			overlay.Fields["devices"] = field

			_, err := ResolveOverlay(overlay, overlayValidationTemplate())
			if test.want == "" {
				if err != nil {
					t.Fatalf("ResolveOverlay(valid table default) error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveOverlay() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveOverlayValidatesTableChoiceDefaults(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		candidate string
		choices   []Choice
		wantError bool
	}{
		"label accepted": {
			candidate: "Disk",
			choices:   []Choice{{Label: "Disk", Value: "disk"}},
		},
		"backend value accepted": {
			candidate: "disk",
			choices:   []Choice{{Label: "Disk", Value: "disk"}},
		},
		"label equals value accepted once": {
			candidate: "disk",
			choices:   []Choice{{Label: "disk", Value: "disk"}},
		},
		"unknown rejected with empty choices": {
			candidate: "disk",
			wantError: true,
		},
		"duplicate labels are ambiguous": {
			candidate: "Disk",
			choices: []Choice{
				{Label: "Disk", Value: "disk-1"},
				{Label: "Disk", Value: "disk-2"},
			},
			wantError: true,
		},
		"duplicate values are ambiguous": {
			candidate: "disk",
			choices: []Choice{
				{Label: "Disk one", Value: "disk"},
				{Label: "Disk two", Value: "disk"},
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			overlay := decodeFixtureOverlay(t)
			devices := overlay.Fields["devices"]
			devices.Default = mustJSONValue(t, []any{map[string]any{
				"hostName": "host-1",
				"part":     test.candidate,
			}})
			overlay.Fields["devices"] = devices

			template := overlayValidationTemplate()
			template.Fields[0].TableColumns[1].Type = "select"
			template.Fields[0].TableColumns[1].Choices = test.choices
			_, err := ResolveOverlay(overlay, template)
			if test.wantError && (err == nil || !strings.Contains(err.Error(), "does not uniquely match a live choice")) {
				t.Fatalf("ResolveOverlay() error = %v, want unique live choice error", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("ResolveOverlay(valid table choice default) error = %v", err)
			}
		})
	}
}

func TestDriftOutputIsDeterministicAndPrivate(t *testing.T) {
	t.Parallel()

	baselineTemplate := overlayValidationTemplate()
	baseline := DiscoverOverlay(baselineTemplate, "drift-test").Baseline
	first := overlayValidationTemplate()
	first.SnapshotID = "218528"
	first.MetadataHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	first.Fields[0].TableColumns[0].Type = "number"
	first.Fields = append(first.Fields,
		Field{BackendKey: "private_optional_b", StableID: "optional_b", Name: "Optional B", Type: "input", Supported: true},
		Field{BackendKey: "private_optional_a", StableID: "optional_a", Name: "Optional A", Type: "input", Supported: true},
	)
	second := first
	second.Fields = append([]Field(nil), first.Fields...)
	for left, right := 0, len(second.Fields)-1; left < right; left, right = left+1, right-1 {
		second.Fields[left], second.Fields[right] = second.Fields[right], second.Fields[left]
	}

	firstJSON := mustMarshalDrift(t, CompareBaseline(baseline, first))
	secondJSON := mustMarshalDrift(t, CompareBaseline(baseline, second))
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("drift output depends on live field order:\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	for _, privateKey := range []string{"private_optional_a", "private_optional_b", "host_name"} {
		if bytes.Contains(firstJSON, []byte(privateKey)) {
			t.Errorf("drift output contains backend key %q: %s", privateKey, firstJSON)
		}
	}
}

func decodeFixtureOverlay(t *testing.T) Overlay {
	t.Helper()
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatalf("DecodeOverlay(fixture) error = %v", err)
	}
	return overlay
}

func assertDriftChange(t *testing.T, report DriftReport, kind, classification string) {
	t.Helper()
	for _, change := range report.Changes {
		if change.Kind == kind {
			if change.Classification != classification {
				t.Fatalf("drift change %q classification = %q, want %q", kind, change.Classification, classification)
			}
			return
		}
	}
	t.Fatalf("drift = %+v, want change kind %q", report, kind)
}

func countDriftKind(report DriftReport, kind string) int {
	count := 0
	for _, change := range report.Changes {
		if change.Kind == kind {
			count++
		}
	}
	return count
}

func templateWithMetadataHash(t *testing.T, template Template) Template {
	t.Helper()
	hash, err := hashMetadata(template.ServiceKey, template.Fields)
	if err != nil {
		t.Fatalf("hashMetadata() error = %v", err)
	}
	template.MetadataHash = hash
	return template
}

func mustMarshalDrift(t *testing.T, report DriftReport) []byte {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(drift) error = %v", err)
	}
	return data
}
