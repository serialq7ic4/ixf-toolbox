package servicedesk

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const (
	driftNonDestructive = "non_destructive"
	driftDestructive    = "destructive"
)

func CompareBaseline(baseline Baseline, template Template) DriftReport {
	report := DriftReport{}
	if baseline.SnapshotID != template.SnapshotID {
		appendDrift(&report, DriftChange{
			Kind:           "snapshot_changed",
			Classification: driftNonDestructive,
			Message:        "The ServiceDesk template snapshot changed",
		})
	}
	if baseline.ServiceKey != template.ServiceKey {
		appendDrift(&report, DriftChange{
			Kind:           "service_key_changed",
			Classification: driftDestructive,
			Message:        "The ServiceDesk service model changed",
		})
	}

	fingerprintChanged := false
	matchedFields := make(map[int]struct{}, len(baseline.Fields))
	for _, baselineField := range baseline.Fields {
		index, fallback, err := matchBaselineField(baselineField, template.Fields)
		if err != nil {
			kind := "field_missing"
			message := "A baseline field no longer resolves"
			if errors.Is(err, errAmbiguousMatch) {
				kind = "field_ambiguous"
				message = "A baseline field resolves ambiguously"
			}
			appendDrift(&report, DriftChange{
				Kind:           kind,
				Field:          baselineField.Name,
				Classification: driftDestructive,
				Message:        message,
			})
			fingerprintChanged = true
			continue
		}
		if _, duplicate := matchedFields[index]; duplicate {
			appendDrift(&report, DriftChange{
				Kind:           "field_ambiguous",
				Field:          baselineField.Name,
				Classification: driftDestructive,
				Message:        "Multiple baseline fields resolve to one live field",
			})
			fingerprintChanged = true
			continue
		}
		matchedFields[index] = struct{}{}
		current := template.Fields[index]
		if fallback {
			appendDrift(&report, DriftChange{
				Kind:           "field_name_fallback",
				Field:          current.Name,
				Classification: driftNonDestructive,
				Message:        "A baseline field resolved by its unique display name",
			})
			fingerprintChanged = true
		}
		if baselineField.Name != current.Name {
			appendDrift(&report, DriftChange{
				Kind:           "field_name_changed",
				Field:          current.Name,
				Classification: driftNonDestructive,
				Message:        "A field display name changed",
			})
			fingerprintChanged = true
		}
		if baselineField.Type != current.Type {
			appendDrift(&report, DriftChange{
				Kind:           "field_type_changed",
				Field:          current.Name,
				Classification: driftDestructive,
				Message:        "A field type changed",
			})
			fingerprintChanged = true
		}
		if baselineField.Required != current.Required {
			appendDrift(&report, DriftChange{
				Kind:           "field_required_changed",
				Field:          current.Name,
				Classification: driftDestructive,
				Message:        "A field required constraint changed",
			})
			fingerprintChanged = true
		}
		if baselineField.ChoiceHash != current.ChoiceHash {
			appendDrift(&report, DriftChange{
				Kind:           "field_choices_changed",
				Field:          current.Name,
				Classification: driftDestructive,
				Message:        "A field choice set changed",
			})
			fingerprintChanged = true
		}
		if compareBaselineColumns(&report, baselineField, current) {
			fingerprintChanged = true
		}
	}
	for index, field := range template.Fields {
		if _, matched := matchedFields[index]; matched {
			continue
		}
		classification := driftNonDestructive
		kind := "optional_field_added"
		message := "A new optional field was added"
		if field.Required && !field.DefaultAvailable {
			classification = driftDestructive
			kind = "required_field_added"
			message = "A new required field without a live default was added"
		}
		appendDrift(&report, DriftChange{
			Kind:           kind,
			Field:          field.Name,
			Classification: classification,
			Message:        message,
		})
		fingerprintChanged = true
	}

	metadataChanged := baseline.MetadataHash != template.MetadataHash
	consequentialMetadata := metadataChanged && liveMetadataMayAffectInput(template.Fields)
	if consequentialMetadata || (metadataChanged && !fingerprintChanged) {
		classification := driftNonDestructive
		message := "ServiceDesk metadata changed without changing the baseline field structure"
		if consequentialMetadata {
			classification = driftDestructive
			message = "ServiceDesk condition or unsupported metadata may affect input validation"
		}
		appendDrift(&report, DriftChange{
			Kind:           "metadata_changed",
			Classification: classification,
			Message:        message,
		})
	}
	sortDriftChanges(report.Changes)
	return report
}

func liveMetadataMayAffectInput(fields []Field) bool {
	for _, field := range fields {
		if !field.Supported || len(field.Conditions.Conditions) > 0 {
			return true
		}
		for _, column := range field.TableColumns {
			if !column.Supported {
				return true
			}
		}
	}
	return false
}

func sortDriftChanges(changes []DriftChange) {
	sort.SliceStable(changes, func(left, right int) bool {
		leftChange := changes[left]
		rightChange := changes[right]
		if leftChange.Kind != rightChange.Kind {
			return leftChange.Kind < rightChange.Kind
		}
		if leftChange.Field != rightChange.Field {
			return leftChange.Field < rightChange.Field
		}
		if leftChange.Column != rightChange.Column {
			return leftChange.Column < rightChange.Column
		}
		if leftChange.Classification != rightChange.Classification {
			return leftChange.Classification < rightChange.Classification
		}
		return leftChange.Message < rightChange.Message
	})
}

func compareBaselineColumns(report *DriftReport, baselineField BaselineField, current Field) bool {
	if len(baselineField.Columns) == 0 && len(current.TableColumns) == 0 {
		return false
	}
	changed := false
	matchedColumns := make(map[int]struct{}, len(baselineField.Columns))
	for _, baselineColumn := range baselineField.Columns {
		index, fallback, err := matchBaselineColumn(baselineColumn, current.TableColumns)
		if err != nil {
			kind := "column_missing"
			message := "A baseline table column no longer resolves"
			if errors.Is(err, errAmbiguousMatch) {
				kind = "column_ambiguous"
				message = "A baseline table column resolves ambiguously"
			}
			appendDrift(report, DriftChange{
				Kind:           kind,
				Field:          current.Name,
				Column:         baselineColumn.Name,
				Classification: driftDestructive,
				Message:        message,
			})
			changed = true
			continue
		}
		if _, duplicate := matchedColumns[index]; duplicate {
			appendDrift(report, DriftChange{
				Kind:           "column_ambiguous",
				Field:          current.Name,
				Column:         baselineColumn.Name,
				Classification: driftDestructive,
				Message:        "Multiple baseline columns resolve to one live column",
			})
			changed = true
			continue
		}
		matchedColumns[index] = struct{}{}
		column := current.TableColumns[index]
		if fallback {
			appendDrift(report, DriftChange{
				Kind:           "column_name_fallback",
				Field:          current.Name,
				Column:         column.Name,
				Classification: driftNonDestructive,
				Message:        "A baseline table column resolved by its unique display name",
			})
			changed = true
		}
		if baselineColumn.Name != column.Name {
			appendDrift(report, DriftChange{
				Kind:           "column_name_changed",
				Field:          current.Name,
				Column:         column.Name,
				Classification: driftNonDestructive,
				Message:        "A table column display name changed",
			})
			changed = true
		}
		if baselineColumn.Type != column.Type {
			appendDrift(report, DriftChange{
				Kind:           "column_type_changed",
				Field:          current.Name,
				Column:         column.Name,
				Classification: driftDestructive,
				Message:        "A table column type changed",
			})
			changed = true
		}
		if baselineColumn.Required != column.Required {
			appendDrift(report, DriftChange{
				Kind:           "column_required_changed",
				Field:          current.Name,
				Column:         column.Name,
				Classification: driftDestructive,
				Message:        "A table column required constraint changed",
			})
			changed = true
		}
		if baselineColumn.ChoiceHash != column.ChoiceHash {
			appendDrift(report, DriftChange{
				Kind:           "column_choices_changed",
				Field:          current.Name,
				Column:         column.Name,
				Classification: driftDestructive,
				Message:        "A table column choice set changed",
			})
			changed = true
		}
	}
	for index, column := range current.TableColumns {
		if _, matched := matchedColumns[index]; matched {
			continue
		}
		classification := driftNonDestructive
		kind := "optional_column_added"
		message := "A new optional table column was added"
		if column.Required {
			classification = driftDestructive
			kind = "required_column_added"
			message = "A new required table column was added"
		}
		appendDrift(report, DriftChange{
			Kind:           kind,
			Field:          current.Name,
			Column:         column.Name,
			Classification: classification,
			Message:        message,
		})
		changed = true
	}
	return changed
}

var (
	errUnresolvedMatch = errors.New("matcher did not resolve")
	errAmbiguousMatch  = errors.New("matcher is ambiguous")
)

func matchBaselineField(baseline BaselineField, fields []Field) (int, bool, error) {
	return matchField(FieldMatcher{StableID: baseline.StableID, Name: baseline.Name}, fields)
}

func matchField(matcher FieldMatcher, fields []Field) (int, bool, error) {
	if matcher.StableID != "" {
		matches := matchingFieldIndexes(fields, func(field Field) bool { return field.StableID == matcher.StableID })
		if len(matches) == 1 {
			return matches[0], false, nil
		}
		if len(matches) > 1 {
			return -1, false, errAmbiguousMatch
		}
	}
	if matcher.Name != "" {
		matches := matchingFieldIndexes(fields, func(field Field) bool { return field.Name == matcher.Name })
		if len(matches) == 1 {
			return matches[0], matcher.StableID != "", nil
		}
		if len(matches) > 1 {
			return -1, false, errAmbiguousMatch
		}
	}
	return -1, false, errUnresolvedMatch
}

func matchingFieldIndexes(fields []Field, match func(Field) bool) []int {
	indexes := make([]int, 0, 1)
	for index, field := range fields {
		if match(field) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func matchBaselineColumn(baseline BaselineColumn, columns []TableColumn) (int, bool, error) {
	return matchColumn(ColumnMatcher{Key: baseline.Key, Name: baseline.Name}, columns)
}

func matchColumn(matcher ColumnMatcher, columns []TableColumn) (int, bool, error) {
	if matcher.Key != "" {
		matches := matchingColumnIndexes(columns, func(column TableColumn) bool { return column.BackendKey == matcher.Key })
		if len(matches) == 1 {
			return matches[0], false, nil
		}
		if len(matches) > 1 {
			return -1, false, errAmbiguousMatch
		}
	}
	if matcher.Name != "" {
		matches := matchingColumnIndexes(columns, func(column TableColumn) bool { return column.Name == matcher.Name })
		if len(matches) == 1 {
			return matches[0], matcher.Key != "", nil
		}
		if len(matches) > 1 {
			return -1, false, errAmbiguousMatch
		}
	}
	return -1, false, errUnresolvedMatch
}

func matchingColumnIndexes(columns []TableColumn, match func(TableColumn) bool) []int {
	indexes := make([]int, 0, 1)
	for index, column := range columns {
		if match(column) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func ResolveOverlay(overlay Overlay, template Template) (ResolvedOverlay, error) {
	resolved := ResolvedOverlay{
		Overlay: overlay,
		Fields:  make(map[string]ResolvedField, len(overlay.Fields)),
		Aliases: make(map[string]string),
		Drift:   CompareBaseline(overlay.Baseline, template),
	}
	if err := ValidateOverlaySchema(overlay); err != nil {
		return resolved, err
	}
	if template.Target.BusinessID != overlay.Target.BusinessID || template.Target.ServiceID != overlay.Target.ServiceID {
		appendDrift(&resolved.Drift, DriftChange{
			Kind:           "target_changed",
			Classification: driftDestructive,
			Message:        "The overlay target does not match the live template",
		})
		sortDriftChanges(resolved.Drift.Changes)
		return resolved, errors.New("ServiceDesk overlay target does not match the live template")
	}

	var resolutionErrors []error
	matchedFields := make(map[int]string, len(overlay.Fields))
	for _, publicName := range sortedMapKeys(overlay.Fields) {
		overlayField := overlay.Fields[publicName]
		index, fallback, err := matchField(overlayField.Match, template.Fields)
		if err != nil {
			kind := "matcher_unresolved"
			message := "An overlay field matcher no longer resolves"
			if errors.Is(err, errAmbiguousMatch) {
				kind = "matcher_ambiguous"
				message = "An overlay field matcher resolves ambiguously"
			}
			appendDrift(&resolved.Drift, DriftChange{
				Kind:           kind,
				Field:          publicName,
				Classification: driftDestructive,
				Message:        message,
			})
			resolutionErrors = append(resolutionErrors, fmt.Errorf("ServiceDesk overlay field %q matcher is %s", publicName, matchErrorLabel(err)))
			continue
		}
		if existing, duplicate := matchedFields[index]; duplicate {
			appendDrift(&resolved.Drift, DriftChange{
				Kind:           "matcher_ambiguous",
				Field:          publicName,
				Classification: driftDestructive,
				Message:        "Multiple overlay fields resolve to one live field",
			})
			resolutionErrors = append(resolutionErrors,
				fmt.Errorf("ServiceDesk overlay fields %q and %q resolve ambiguously to one live field", existing, publicName))
			continue
		}
		matchedFields[index] = publicName
		field := template.Fields[index]
		if fallback {
			appendDrift(&resolved.Drift, DriftChange{
				Kind:           "matcher_name_fallback",
				Field:          publicName,
				Classification: driftNonDestructive,
				Message:        "An overlay field resolved by its unique display name",
			})
		}

		resolvedField := ResolvedField{
			PublicName: publicName,
			Field:      field,
			Overlay:    overlayField,
			Columns:    make(map[string]ResolvedColumn, len(overlayField.Columns)),
		}
		if overlayField.Required != nil && !*overlayField.Required && field.Required {
			resolutionErrors = append(resolutionErrors,
				fmt.Errorf("ServiceDesk overlay field %q cannot relax a live required field", publicName))
		}
		if overlayField.Default != nil {
			if err := validateLiveDefault(overlayField.Default, field.Type, field.Choices); err != nil {
				resolutionErrors = append(resolutionErrors, fmt.Errorf("ServiceDesk overlay field %q default: %w", publicName, err))
			}
		}
		resolveOverlayColumns(&resolved, &resolvedField, publicName, overlayField.Columns, field.TableColumns, &resolutionErrors)
		resolved.Fields[publicName] = resolvedField
		resolved.Aliases[publicName] = publicName
		for _, alias := range overlayField.Aliases {
			resolved.Aliases[alias] = publicName
		}
	}
	sortDriftChanges(resolved.Drift.Changes)
	return resolved, errors.Join(resolutionErrors...)
}

func resolveOverlayColumns(
	resolved *ResolvedOverlay,
	field *ResolvedField,
	fieldName string,
	overlayColumns map[string]OverlayColumn,
	liveColumns []TableColumn,
	resolutionErrors *[]error,
) {
	matchedColumns := make(map[int]string, len(overlayColumns))
	for _, publicName := range sortedMapKeys(overlayColumns) {
		overlayColumn := overlayColumns[publicName]
		index, fallback, err := matchColumn(overlayColumn.Match, liveColumns)
		if err != nil {
			kind := "column_matcher_unresolved"
			message := "An overlay table column matcher no longer resolves"
			if errors.Is(err, errAmbiguousMatch) {
				kind = "column_matcher_ambiguous"
				message = "An overlay table column matcher resolves ambiguously"
			}
			appendDrift(&resolved.Drift, DriftChange{
				Kind:           kind,
				Field:          fieldName,
				Column:         publicName,
				Classification: driftDestructive,
				Message:        message,
			})
			*resolutionErrors = append(*resolutionErrors,
				fmt.Errorf("ServiceDesk overlay field %q column %q matcher is %s", fieldName, publicName, matchErrorLabel(err)))
			continue
		}
		if existing, duplicate := matchedColumns[index]; duplicate {
			appendDrift(&resolved.Drift, DriftChange{
				Kind:           "column_matcher_ambiguous",
				Field:          fieldName,
				Column:         publicName,
				Classification: driftDestructive,
				Message:        "Multiple overlay columns resolve to one live column",
			})
			*resolutionErrors = append(*resolutionErrors,
				fmt.Errorf("ServiceDesk overlay field %q columns %q and %q resolve ambiguously to one live column", fieldName, existing, publicName))
			continue
		}
		matchedColumns[index] = publicName
		column := liveColumns[index]
		if fallback {
			appendDrift(&resolved.Drift, DriftChange{
				Kind:           "column_matcher_name_fallback",
				Field:          fieldName,
				Column:         publicName,
				Classification: driftNonDestructive,
				Message:        "An overlay table column resolved by its unique display name",
			})
		}
		if overlayColumn.Required != nil && !*overlayColumn.Required && column.Required {
			*resolutionErrors = append(*resolutionErrors,
				fmt.Errorf("ServiceDesk overlay field %q column %q cannot relax a live required column", fieldName, publicName))
		}
		if overlayColumn.Default != nil {
			if err := validateLiveDefault(overlayColumn.Default, column.Type, column.Choices); err != nil {
				*resolutionErrors = append(*resolutionErrors,
					fmt.Errorf("ServiceDesk overlay field %q column %q default: %w", fieldName, publicName, err))
			}
		}
		field.Columns[publicName] = ResolvedColumn{PublicName: publicName, Column: column, Overlay: overlayColumn}
	}
}

func matchErrorLabel(err error) string {
	if errors.Is(err, errAmbiguousMatch) {
		return "ambiguous"
	}
	return "unresolved"
}

func validateLiveDefault(raw json.RawMessage, fieldType string, choices []Choice) error {
	if err := validateDefaultType(raw, fieldType); err != nil {
		return err
	}
	if !choiceFieldType(fieldType) || len(choices) == 0 {
		return nil
	}
	var value any
	if err := decodeStrictJSON(raw, &value); err != nil {
		return err
	}
	values := []string{}
	switch typed := value.(type) {
	case string:
		values = append(values, typed)
	case []any:
		for _, item := range typed {
			values = append(values, item.(string))
		}
	}
	for _, candidate := range values {
		matches := 0
		for _, choice := range choices {
			if candidate == choice.Label || candidate == choice.Value {
				matches++
			}
		}
		if matches != 1 {
			return errors.New("does not uniquely match a live choice")
		}
	}
	return nil
}

func appendDrift(report *DriftReport, change DriftChange) {
	report.Changed = true
	report.Changes = append(report.Changes, change)
	if change.Classification == driftDestructive {
		report.Classification = driftDestructive
		report.ApplyBlocked = true
		return
	}
	if report.Classification == "" {
		report.Classification = driftNonDestructive
	}
}
