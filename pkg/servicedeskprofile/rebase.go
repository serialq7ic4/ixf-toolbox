package servicedeskprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

type RebaseResult struct {
	Overlay    Overlay             `json:"overlay"`
	Preserved  []RebaseFieldChange `json:"preserved"`
	Added      []RebaseFieldChange `json:"added"`
	Removed    []RebaseFieldChange `json:"removed"`
	Unresolved []RebaseUnresolved  `json:"unresolved"`
}

type RebaseFieldChange struct {
	Field      string   `json:"field"`
	Column     string   `json:"column,omitempty"`
	MatchedBy  string   `json:"matchedBy,omitempty"`
	Properties []string `json:"properties,omitempty"`
}

type RebaseUnresolved struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Field      string          `json:"field"`
	Column     string          `json:"column,omitempty"`
	Properties []string        `json:"properties"`
	Previous   json.RawMessage `json:"previous"`
	Message    string          `json:"message"`
}

type rebaseFieldCandidate struct {
	publicName string
	overlay    OverlayField
	baseline   BaselineField
	liveIndex  int
	matchedBy  string
	properties []string
}

type rebaseColumnCandidate struct {
	publicName string
	overlay    OverlayColumn
	baseline   BaselineColumn
	liveIndex  int
	matchedBy  string
	properties []string
}

func RebaseOverlay(previous Overlay, template Template) (RebaseResult, error) {
	result := RebaseResult{
		Preserved:  []RebaseFieldChange{},
		Added:      []RebaseFieldChange{},
		Removed:    []RebaseFieldChange{},
		Unresolved: []RebaseUnresolved{},
	}
	if err := ValidateOverlaySchema(previous); err != nil {
		return result, fmt.Errorf("validate previous ServiceDesk profile: %w", err)
	}
	if template.Target.BusinessID != previous.Target.BusinessID || template.Target.ServiceID != previous.Target.ServiceID {
		return result, errors.New("ServiceDesk profile target does not match the live template")
	}

	discovered := DiscoverOverlay(template, previous.Name)
	discovered.SchemaVersion = previous.SchemaVersion
	discovered.Description = previous.Description
	discovered.Routing = cloneRouting(previous.Routing)
	discovered.Target = previous.Target
	discovered.Fields = make(map[string]OverlayField, len(template.Fields))
	result.Overlay = discovered

	claimed := make(map[int]struct{}, len(previous.Fields))
	groups := make(map[int][]rebaseFieldCandidate)
	for _, publicName := range sortedMapKeys(previous.Fields) {
		overlayField := previous.Fields[publicName]
		baselineField, err := resolveBaselineField(overlayField.Match, previous.Baseline.Fields)
		if err != nil {
			return result, fmt.Errorf("resolve previous ServiceDesk field %q: %w", publicName, err)
		}
		properties, err := fieldCustomizationProperties(publicName, overlayField, baselineField)
		if err != nil {
			return result, err
		}
		liveIndex, fallback, matchErr := matchBaselineField(baselineField, template.Fields)
		if matchErr != nil {
			kind := "field_missing"
			if errors.Is(matchErr, errAmbiguousMatch) {
				kind = "field_ambiguous"
			}
			if err := recordFieldFailure(&result, kind, publicName, overlayField, properties); err != nil {
				return result, err
			}
			continue
		}
		claimed[liveIndex] = struct{}{}
		groups[liveIndex] = append(groups[liveIndex], rebaseFieldCandidate{
			publicName: publicName,
			overlay:    overlayField,
			baseline:   baselineField,
			liveIndex:  liveIndex,
			matchedBy:  fieldMatchKind(baselineField, fallback),
			properties: properties,
		})
	}

	winners := make([]rebaseFieldCandidate, 0, len(groups))
	for liveIndex := range template.Fields {
		candidates := groups[liveIndex]
		if len(candidates) == 0 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].publicName < candidates[j].publicName })
		winner := -1
		if len(candidates) == 1 {
			winner = 0
		} else {
			for index, candidate := range candidates {
				if candidate.matchedBy != "stableId" {
					continue
				}
				if winner >= 0 {
					winner = -1
					break
				}
				winner = index
			}
		}
		for index, candidate := range candidates {
			if index == winner {
				winners = append(winners, candidate)
				continue
			}
			if err := recordFieldFailure(&result, "field_ambiguous", candidate.publicName, candidate.overlay, candidate.properties); err != nil {
				return result, err
			}
		}
	}
	sort.Slice(winners, func(i, j int) bool { return winners[i].publicName < winners[j].publicName })

	usedNames := make(map[string]struct{}, len(previous.Fields)*2)
	for _, candidate := range winners {
		liveField := template.Fields[candidate.liveIndex]
		if liveField.Type != candidate.baseline.Type {
			if err := recordFieldFailure(&result, "field_type_changed", candidate.publicName, candidate.overlay, candidate.properties); err != nil {
				return result, err
			}
			continue
		}
		migrated, err := migrateRebaseField(&result, candidate, liveField)
		if err != nil {
			return result, err
		}
		result.Overlay.Fields[candidate.publicName] = migrated
		reserveNames(usedNames, candidate.publicName, migrated.Aliases)
	}

	for liveIndex, liveField := range template.Fields {
		if _, exists := claimed[liveIndex]; exists {
			continue
		}
		publicName := uniqueTechnicalName(preferredFieldName(liveField), "field", liveIndex, usedNames)
		added := OverlayField{Match: liveFieldMatcher(liveField)}
		if len(liveField.TableColumns) > 0 {
			added.Columns = make(map[string]OverlayColumn, len(liveField.TableColumns))
			usedColumns := make(map[string]struct{}, len(liveField.TableColumns))
			for columnIndex, liveColumn := range liveField.TableColumns {
				columnName := uniqueTechnicalName(liveColumn.Name, "column", columnIndex, usedColumns)
				added.Columns[columnName] = OverlayColumn{Match: liveColumnMatcher(liveColumn)}
				result.Added = append(result.Added, RebaseFieldChange{Field: publicName, Column: columnName})
			}
		}
		result.Overlay.Fields[publicName] = added
		result.Added = append(result.Added, RebaseFieldChange{Field: publicName})
	}

	finalizeRebaseResult(&result)
	if err := ValidateOverlaySchema(result.Overlay); err != nil {
		return result, fmt.Errorf("validate rebased ServiceDesk profile: %w", err)
	}
	if _, err := ResolveOverlay(result.Overlay, template); err != nil {
		return result, fmt.Errorf("resolve rebased ServiceDesk profile: %w", err)
	}
	return result, nil
}

func migrateRebaseField(result *RebaseResult, candidate rebaseFieldCandidate, liveField Field) (OverlayField, error) {
	migrated := OverlayField{
		Match:       liveFieldMatcher(liveField),
		Aliases:     cloneStrings(candidate.overlay.Aliases),
		Description: candidate.overlay.Description,
		Required:    trueOnly(candidate.overlay.Required),
	}
	successful := make(map[string]bool)
	for _, property := range candidate.properties {
		if property != "default" && property != "columns" {
			successful[property] = true
		}
	}

	resolvedColumns := map[string]ResolvedColumn{}
	customColumnsMigrated := false
	if tableFieldType(liveField.Type) {
		var err error
		migrated.Columns, resolvedColumns, customColumnsMigrated, err = migrateRebaseColumns(result, candidate, liveField.TableColumns)
		if err != nil {
			return OverlayField{}, err
		}
		if customColumnsMigrated {
			successful["columns"] = true
		}
	}

	if candidate.overlay.Default != nil {
		valid := validateLiveDefault(candidate.overlay.Default, liveField.Type, liveField.Choices)
		if tableFieldType(liveField.Type) && valid == nil {
			valid = validateTableDefaultRows(candidate.overlay.Default, ResolvedField{
				PublicName: candidate.publicName,
				Field:      liveField,
				Overlay:    migrated,
				Columns:    resolvedColumns,
			}, liveField.TableColumns)
		}
		if valid == nil {
			migrated.Default = cloneRaw(candidate.overlay.Default)
			successful["default"] = true
		} else if err := recordInvalidDefault(result, candidate.publicName, "", candidate.overlay.Default); err != nil {
			return OverlayField{}, err
		}
	}

	properties := orderedProperties(successful)
	if len(properties) > 0 {
		result.Preserved = append(result.Preserved, RebaseFieldChange{
			Field:      candidate.publicName,
			MatchedBy:  candidate.matchedBy,
			Properties: properties,
		})
	}
	return migrated, nil
}

func migrateRebaseColumns(
	result *RebaseResult,
	fieldCandidate rebaseFieldCandidate,
	liveColumns []TableColumn,
) (map[string]OverlayColumn, map[string]ResolvedColumn, bool, error) {
	claimed := make(map[int]struct{}, len(fieldCandidate.overlay.Columns))
	groups := make(map[int][]rebaseColumnCandidate)
	for _, publicName := range sortedMapKeys(fieldCandidate.overlay.Columns) {
		overlayColumn := fieldCandidate.overlay.Columns[publicName]
		baselineColumn, err := resolveBaselineColumn(overlayColumn.Match, fieldCandidate.baseline.Columns)
		if err != nil {
			return nil, nil, false, fmt.Errorf("resolve previous ServiceDesk field %q column %q: %w", fieldCandidate.publicName, publicName, err)
		}
		properties := columnCustomizationProperties(publicName, overlayColumn, baselineColumn)
		liveIndex, fallback, matchErr := matchBaselineColumn(baselineColumn, liveColumns)
		if matchErr != nil {
			kind := "column_missing"
			if errors.Is(matchErr, errAmbiguousMatch) {
				kind = "column_ambiguous"
			}
			if err := recordColumnFailure(result, kind, fieldCandidate.publicName, publicName, overlayColumn, properties); err != nil {
				return nil, nil, false, err
			}
			continue
		}
		claimed[liveIndex] = struct{}{}
		groups[liveIndex] = append(groups[liveIndex], rebaseColumnCandidate{
			publicName: publicName,
			overlay:    overlayColumn,
			baseline:   baselineColumn,
			liveIndex:  liveIndex,
			matchedBy:  columnMatchKind(baselineColumn, fallback),
			properties: properties,
		})
	}

	winners := make([]rebaseColumnCandidate, 0, len(groups))
	for liveIndex := range liveColumns {
		candidates := groups[liveIndex]
		if len(candidates) == 0 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].publicName < candidates[j].publicName })
		winner := -1
		if len(candidates) == 1 {
			winner = 0
		} else {
			for index, candidate := range candidates {
				if candidate.matchedBy != "key" {
					continue
				}
				if winner >= 0 {
					winner = -1
					break
				}
				winner = index
			}
		}
		for index, candidate := range candidates {
			if index == winner {
				winners = append(winners, candidate)
				continue
			}
			if err := recordColumnFailure(result, "column_ambiguous", fieldCandidate.publicName, candidate.publicName, candidate.overlay, candidate.properties); err != nil {
				return nil, nil, false, err
			}
		}
	}
	sort.Slice(winners, func(i, j int) bool { return winners[i].publicName < winners[j].publicName })

	migrated := make(map[string]OverlayColumn, len(liveColumns))
	resolved := make(map[string]ResolvedColumn, len(liveColumns))
	usedNames := make(map[string]struct{}, len(liveColumns)*2)
	customMigrated := false
	for _, candidate := range winners {
		liveColumn := liveColumns[candidate.liveIndex]
		if liveColumn.Type != candidate.baseline.Type {
			if err := recordColumnFailure(result, "column_type_changed", fieldCandidate.publicName, candidate.publicName, candidate.overlay, candidate.properties); err != nil {
				return nil, nil, false, err
			}
			continue
		}
		column := OverlayColumn{
			Match:       liveColumnMatcher(liveColumn),
			Aliases:     cloneStrings(candidate.overlay.Aliases),
			Description: candidate.overlay.Description,
			Required:    trueOnly(candidate.overlay.Required),
		}
		successful := make(map[string]bool)
		for _, property := range candidate.properties {
			if property != "default" {
				successful[property] = true
			}
		}
		if candidate.overlay.Default != nil {
			if err := validateLiveDefault(candidate.overlay.Default, liveColumn.Type, liveColumn.Choices); err == nil {
				column.Default = cloneRaw(candidate.overlay.Default)
				successful["default"] = true
			} else if err := recordInvalidDefault(result, fieldCandidate.publicName, candidate.publicName, candidate.overlay.Default); err != nil {
				return nil, nil, false, err
			}
		}
		properties := orderedProperties(successful)
		if len(properties) > 0 {
			customMigrated = true
			result.Preserved = append(result.Preserved, RebaseFieldChange{
				Field:      fieldCandidate.publicName,
				Column:     candidate.publicName,
				MatchedBy:  candidate.matchedBy,
				Properties: properties,
			})
		}
		migrated[candidate.publicName] = column
		resolved[candidate.publicName] = ResolvedColumn{
			PublicName: candidate.publicName,
			Column:     liveColumn,
			Overlay:    column,
			LiveIndex:  candidate.liveIndex,
		}
		reserveNames(usedNames, candidate.publicName, column.Aliases)
	}

	for liveIndex, liveColumn := range liveColumns {
		if _, exists := claimed[liveIndex]; exists {
			continue
		}
		publicName := uniqueTechnicalName(liveColumn.Name, "column", liveIndex, usedNames)
		column := OverlayColumn{Match: liveColumnMatcher(liveColumn)}
		migrated[publicName] = column
		resolved[publicName] = ResolvedColumn{PublicName: publicName, Column: liveColumn, Overlay: column, LiveIndex: liveIndex}
		result.Added = append(result.Added, RebaseFieldChange{Field: fieldCandidate.publicName, Column: publicName})
	}
	if len(migrated) == 0 {
		migrated = nil
	}
	return migrated, resolved, customMigrated, nil
}

func fieldCustomizationProperties(publicName string, field OverlayField, baseline BaselineField) ([]string, error) {
	set := make(map[string]bool)
	preferred := baseline.StableID
	if preferred == "" {
		preferred = baseline.Name
	}
	if publicName != preferred {
		set["publicName"] = true
	}
	if len(field.Aliases) > 0 {
		set["aliases"] = true
	}
	if field.Description != "" {
		set["description"] = true
	}
	if field.Default != nil {
		set["default"] = true
	}
	if field.Required != nil && *field.Required {
		set["required"] = true
	}
	for publicColumn, overlayColumn := range field.Columns {
		baselineColumn, err := resolveBaselineColumn(overlayColumn.Match, baseline.Columns)
		if err != nil {
			return nil, fmt.Errorf("resolve previous ServiceDesk field %q column %q: %w", publicName, publicColumn, err)
		}
		if len(columnCustomizationProperties(publicColumn, overlayColumn, baselineColumn)) > 0 {
			set["columns"] = true
		}
	}
	return orderedProperties(set), nil
}

func columnCustomizationProperties(publicName string, column OverlayColumn, baseline BaselineColumn) []string {
	set := make(map[string]bool)
	if publicName != baseline.Name {
		set["publicName"] = true
	}
	if len(column.Aliases) > 0 {
		set["aliases"] = true
	}
	if column.Description != "" {
		set["description"] = true
	}
	if column.Default != nil {
		set["default"] = true
	}
	if column.Required != nil && *column.Required {
		set["required"] = true
	}
	return orderedProperties(set)
}

func orderedProperties(set map[string]bool) []string {
	properties := make([]string, 0, len(set))
	for _, property := range []string{"publicName", "aliases", "description", "default", "required", "columns"} {
		if set[property] {
			properties = append(properties, property)
		}
	}
	return properties
}

func recordFieldFailure(result *RebaseResult, kind, field string, previous OverlayField, properties []string) error {
	if len(properties) == 0 {
		result.Removed = append(result.Removed, RebaseFieldChange{Field: field})
		return nil
	}
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	result.Unresolved = append(result.Unresolved, RebaseUnresolved{
		Kind:       kind,
		Field:      field,
		Properties: cloneStrings(properties),
		Previous:   raw,
		Message:    rebaseMessage(kind),
	})
	return nil
}

func recordColumnFailure(result *RebaseResult, kind, field, column string, previous OverlayColumn, properties []string) error {
	if len(properties) == 0 {
		result.Removed = append(result.Removed, RebaseFieldChange{Field: field, Column: column})
		return nil
	}
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	result.Unresolved = append(result.Unresolved, RebaseUnresolved{
		Kind:       kind,
		Field:      field,
		Column:     column,
		Properties: cloneStrings(properties),
		Previous:   raw,
		Message:    rebaseMessage(kind),
	})
	return nil
}

func recordInvalidDefault(result *RebaseResult, field, column string, previous json.RawMessage) error {
	raw, err := json.Marshal(struct {
		Default json.RawMessage `json:"default"`
	}{Default: previous})
	if err != nil {
		return err
	}
	result.Unresolved = append(result.Unresolved, RebaseUnresolved{
		Kind:       "default_invalid",
		Field:      field,
		Column:     column,
		Properties: []string{"default"},
		Previous:   raw,
		Message:    rebaseMessage("default_invalid"),
	})
	return nil
}

func rebaseMessage(kind string) string {
	switch kind {
	case "field_missing":
		return "The customized field no longer exists"
	case "field_ambiguous":
		return "The customized field no longer matches uniquely"
	case "field_type_changed":
		return "The customized field type changed"
	case "column_missing":
		return "The customized table column no longer exists"
	case "column_ambiguous":
		return "The customized table column no longer matches uniquely"
	case "column_type_changed":
		return "The customized table column type changed"
	case "default_invalid":
		return "The customized default is invalid for the live form"
	default:
		return "The customization requires manual review"
	}
}

func finalizeRebaseResult(result *RebaseResult) {
	sort.Slice(result.Preserved, func(i, j int) bool { return lessRebaseChange(result.Preserved[i], result.Preserved[j]) })
	sort.Slice(result.Added, func(i, j int) bool { return lessRebaseChange(result.Added[i], result.Added[j]) })
	sort.Slice(result.Removed, func(i, j int) bool { return lessRebaseChange(result.Removed[i], result.Removed[j]) })
	sort.Slice(result.Unresolved, func(i, j int) bool {
		left, right := result.Unresolved[i], result.Unresolved[j]
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.Kind < right.Kind
	})
	for index := range result.Unresolved {
		result.Unresolved[index].ID = "unresolved-" + strconv.Itoa(index+1)
	}
}

func lessRebaseChange(left, right RebaseFieldChange) bool {
	if left.Field != right.Field {
		return left.Field < right.Field
	}
	return left.Column < right.Column
}

func fieldMatchKind(baseline BaselineField, fallback bool) string {
	if baseline.StableID != "" && !fallback {
		return "stableId"
	}
	return "name"
}

func columnMatchKind(baseline BaselineColumn, fallback bool) string {
	if baseline.Key != "" && !fallback {
		return "key"
	}
	return "name"
}

func liveFieldMatcher(field Field) FieldMatcher {
	return FieldMatcher{StableID: field.StableID, Name: field.Name}
}

func liveColumnMatcher(column TableColumn) ColumnMatcher {
	return ColumnMatcher{Key: column.BackendKey, Name: column.Name}
}

func cloneRouting(routing Routing) Routing {
	routing.Examples = cloneStrings(routing.Examples)
	return routing
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func trueOnly(value *bool) *bool {
	if value == nil || !*value {
		return nil
	}
	result := true
	return &result
}

func reserveNames(used map[string]struct{}, publicName string, aliases []string) {
	used[publicName] = struct{}{}
	for _, alias := range aliases {
		used[alias] = struct{}{}
	}
}
