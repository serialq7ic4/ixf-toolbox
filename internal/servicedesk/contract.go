package servicedesk

import (
	"context"
	"encoding/json"
	"fmt"
)

const ContractVersion = 1

type InspectRequest struct {
	Target  EditTarget
	Profile *ProfileContext
}

type ProfileContext struct {
	Overlay Overlay
	Source  string
	Stale   bool
}

type ResolvedTarget struct {
	BusinessID  int    `json:"businessId"`
	ServiceID   int    `json:"serviceId"`
	SnapshotID  string `json:"snapshotId"`
	ServiceKey  string `json:"serviceKey,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

type InputColumn struct {
	Name                    string   `json:"name"`
	Aliases                 []string `json:"aliases,omitempty"`
	Description             string   `json:"description,omitempty"`
	DisplayName             string   `json:"displayName"`
	Type                    string   `json:"type"`
	Required                bool     `json:"required"`
	DefaultAvailable        bool     `json:"defaultAvailable"`
	Choices                 []Choice `json:"choices,omitempty"`
	Supported               bool     `json:"supported"`
	includeDefaultAvailable bool
}

func (column InputColumn) MarshalJSON() ([]byte, error) {
	type inputColumnJSON struct {
		Name             string   `json:"name"`
		Aliases          []string `json:"aliases,omitempty"`
		Description      string   `json:"description,omitempty"`
		DisplayName      string   `json:"displayName"`
		Type             string   `json:"type"`
		Required         bool     `json:"required"`
		DefaultAvailable *bool    `json:"defaultAvailable,omitempty"`
		Choices          []Choice `json:"choices,omitempty"`
		Supported        bool     `json:"supported"`
	}
	var defaultAvailable *bool
	if column.includeDefaultAvailable {
		value := column.DefaultAvailable
		defaultAvailable = &value
	}
	return json.Marshal(inputColumnJSON{
		Name:             column.Name,
		Aliases:          column.Aliases,
		Description:      column.Description,
		DisplayName:      column.DisplayName,
		Type:             column.Type,
		Required:         column.Required,
		DefaultAvailable: defaultAvailable,
		Choices:          column.Choices,
		Supported:        column.Supported,
	})
}

type InputField struct {
	Name             string        `json:"name"`
	Aliases          []string      `json:"aliases,omitempty"`
	Description      string        `json:"description,omitempty"`
	DisplayName      string        `json:"displayName"`
	StableID         string        `json:"stableId,omitempty"`
	Type             string        `json:"type"`
	Required         bool          `json:"required"`
	Conditional      bool          `json:"conditional"`
	DefaultAvailable bool          `json:"defaultAvailable"`
	Choices          []Choice      `json:"choices,omitempty"`
	Columns          []InputColumn `json:"columns,omitempty"`
	Conditions       *ConditionSet `json:"conditions,omitempty"`
	Supported        bool          `json:"supported"`
}

type InputContract struct {
	Fields                        []InputField `json:"fields"`
	UnsupportedRequiredFieldCount int          `json:"unsupportedRequiredFieldCount"`
}

type ResolvedProfile struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Stale  bool   `json:"stale"`
}

type ResolvedContract struct {
	ContractVersion     int              `json:"contractVersion"`
	Target              ResolvedTarget   `json:"target"`
	Profile             *ResolvedProfile `json:"profile,omitempty"`
	InputContract       InputContract    `json:"inputContract"`
	TemplateDrift       bool             `json:"templateDrift"`
	DriftClassification string           `json:"driftClassification,omitempty"`
	DriftChanges        []DriftChange    `json:"driftChanges,omitempty"`
	ApplyBlocked        bool             `json:"applyBlocked,omitempty"`
	ContractHash        string           `json:"contractHash,omitempty"`
}

func (contract ResolvedContract) MarshalJSON() ([]byte, error) {
	type resolvedContractJSON struct {
		ContractVersion     int              `json:"contractVersion"`
		Target              ResolvedTarget   `json:"target"`
		Profile             *ResolvedProfile `json:"profile,omitempty"`
		InputContract       InputContract    `json:"inputContract"`
		TemplateDrift       bool             `json:"templateDrift"`
		DriftClassification string           `json:"driftClassification,omitempty"`
		DriftChanges        []DriftChange    `json:"driftChanges,omitempty"`
		ApplyBlocked        *bool            `json:"applyBlocked,omitempty"`
		ContractHash        string           `json:"contractHash,omitempty"`
	}
	var applyBlocked *bool
	if contract.Profile != nil {
		value := contract.ApplyBlocked
		applyBlocked = &value
	}
	return json.Marshal(resolvedContractJSON{
		ContractVersion:     contract.ContractVersion,
		Target:              contract.Target,
		Profile:             contract.Profile,
		InputContract:       contract.InputContract,
		TemplateDrift:       contract.TemplateDrift,
		DriftClassification: contract.DriftClassification,
		DriftChanges:        contract.DriftChanges,
		ApplyBlocked:        applyBlocked,
		ContractHash:        contract.ContractHash,
	})
}

func Inspect(ctx context.Context, client *Client, request InspectRequest) (ResolvedContract, error) {
	template, err := LoadTemplate(ctx, client, request.Target)
	if err != nil {
		return ResolvedContract{}, err
	}
	contract := ResolvedContract{
		ContractVersion: ContractVersion,
		Target: ResolvedTarget{
			BusinessID:  template.Target.BusinessID,
			ServiceID:   template.Target.ServiceID,
			SnapshotID:  template.SnapshotID,
			ServiceKey:  template.ServiceKey,
			ServiceName: template.ServiceName,
		},
	}
	if request.Profile == nil {
		contract.InputContract, err = resolveURLInputContract(template.Fields)
	} else {
		resolved, resolveErr := ResolveOverlay(request.Profile.Overlay, template)
		if resolveErr != nil {
			return ResolvedContract{}, resolveErr
		}
		contract.InputContract, err = resolveProfileInputContract(template.Fields, resolved)
		contract.Profile = &ResolvedProfile{
			Name:   resolved.Overlay.Name,
			Source: request.Profile.Source,
			Stale:  request.Profile.Stale,
		}
		contract.TemplateDrift = resolved.Drift.Changed
		contract.DriftClassification = resolved.Drift.Classification
		projectedDrift, projectErr := projectProfileDrift(resolved.Drift, template.Fields, contract.InputContract)
		if projectErr != nil {
			return ResolvedContract{}, projectErr
		}
		contract.DriftChanges = projectedDrift.Changes
		contract.ApplyBlocked = resolved.Drift.ApplyBlocked
	}
	if err != nil {
		return ResolvedContract{}, err
	}
	contract.ContractHash, err = hashResolvedContract(contract)
	if err != nil {
		return ResolvedContract{}, fmt.Errorf("hash resolved ServiceDesk contract: %w", err)
	}
	return contract, nil
}

func resolveProfileInputContract(fields []Field, overlay ResolvedOverlay) (InputContract, error) {
	contract, err := resolveURLInputContract(fields)
	if err != nil {
		return InputContract{}, err
	}

	urlNames := make([]string, len(contract.Fields))
	for index := range contract.Fields {
		urlNames[index] = contract.Fields[index].Name
	}
	mappedFields := make(map[string]ResolvedField, len(overlay.Fields))
	for _, field := range overlay.Fields {
		mappedFields[field.Field.BackendKey] = field
	}

	for index, liveField := range fields {
		resolved := &contract.Fields[index]
		for columnIndex := range resolved.Columns {
			resolved.Columns[columnIndex].includeDefaultAvailable = true
		}
		mapped, ok := mappedFields[liveField.BackendKey]
		if !ok {
			continue
		}
		resolved.Name = mapped.PublicName
		resolved.Aliases = mapped.Overlay.Aliases
		resolved.Description = mapped.Overlay.Description
		resolved.Required = liveField.Required || overlayRequired(mapped.Overlay.Required)
		resolved.DefaultAvailable = liveField.DefaultAvailable || mapped.Overlay.Default != nil
		for _, column := range mapped.Columns {
			if column.LiveIndex < 0 || column.LiveIndex >= len(resolved.Columns) {
				return InputContract{}, fmt.Errorf("ServiceDesk overlay resolved an invalid table column")
			}
			resolvedColumn := &resolved.Columns[column.LiveIndex]
			resolvedColumn.Name = column.PublicName
			resolvedColumn.Aliases = column.Overlay.Aliases
			resolvedColumn.Description = column.Overlay.Description
			resolvedColumn.Required = column.Column.Required || overlayRequired(column.Overlay.Required)
			resolvedColumn.DefaultAvailable = column.Column.DefaultAvailable || column.Overlay.Default != nil
		}
	}

	conditionReferences := make(map[string]string, len(fields)*4)
	publicNames := make(map[string]struct{}, len(fields)*2)
	for index, liveField := range fields {
		resolved := &contract.Fields[index]
		if err := addProfilePublicNames(publicNames, resolved.Name, resolved.Aliases, "field"); err != nil {
			return InputContract{}, err
		}
		for _, reference := range append([]string{liveField.BackendKey, urlNames[index], resolved.Name}, resolved.Aliases...) {
			if err := addConditionReference(conditionReferences, reference, resolved.Name); err != nil {
				return InputContract{}, err
			}
		}
		if err := validateProfileColumnNames(resolved.Columns); err != nil {
			return InputContract{}, fmt.Errorf("resolve columns for field %q: %w", resolved.Name, err)
		}
	}

	contract.UnsupportedRequiredFieldCount = 0
	for index, liveField := range fields {
		resolved := &contract.Fields[index]
		if resolved.Conditional {
			conditions, err := resolveConditionSources(liveField.Conditions, conditionReferences)
			if err != nil {
				return InputContract{}, err
			}
			resolved.Conditions = &conditions
		}
		if resolved.Required && !resolved.Supported {
			contract.UnsupportedRequiredFieldCount++
		}
		for _, column := range resolved.Columns {
			if column.Required && !column.Supported {
				contract.UnsupportedRequiredFieldCount++
			}
		}
	}
	return contract, nil
}

func projectProfileDrift(report DriftReport, fields []Field, profile InputContract) (DriftReport, error) {
	urlContract, err := resolveURLInputContract(fields)
	if err != nil {
		return DriftReport{}, fmt.Errorf("ServiceDesk drift could not be projected safely")
	}
	if len(urlContract.Fields) != len(profile.Fields) || len(fields) != len(profile.Fields) {
		return DriftReport{}, fmt.Errorf("ServiceDesk drift could not be projected safely")
	}

	fieldNames := make(map[string]string, len(fields)*5)
	columnNames := make(map[string]map[string]string, len(fields))
	for index, liveField := range fields {
		publicName := profile.Fields[index].Name
		for _, identifier := range []string{liveField.BackendKey, liveField.StableID, liveField.Name, urlContract.Fields[index].Name, publicName} {
			if err := addProfileDriftIdentifier(fieldNames, identifier, publicName); err != nil {
				return DriftReport{}, err
			}
		}

		liveColumns := liveField.TableColumns
		profileColumns := profile.Fields[index].Columns
		urlColumns := urlContract.Fields[index].Columns
		if len(liveColumns) != len(profileColumns) || len(urlColumns) != len(profileColumns) {
			return DriftReport{}, fmt.Errorf("ServiceDesk drift could not be projected safely")
		}
		columnPublicNames := make(map[string]string, len(liveColumns)*4)
		for columnIndex, liveColumn := range liveColumns {
			publicColumnName := profileColumns[columnIndex].Name
			for _, identifier := range []string{liveColumn.BackendKey, liveColumn.Name, urlColumns[columnIndex].Name, publicColumnName} {
				if err := addProfileDriftIdentifier(columnPublicNames, identifier, publicColumnName); err != nil {
					return DriftReport{}, err
				}
			}
		}
		if len(columnPublicNames) > 0 {
			columnNames[publicName] = columnPublicNames
		}
	}

	projected := report
	projected.Changes = make([]DriftChange, 0, len(report.Changes))
	for _, change := range report.Changes {
		projectedChange := change
		if change.Field != "" {
			publicField, ok := fieldNames[change.Field]
			if !ok {
				return DriftReport{}, fmt.Errorf("ServiceDesk drift contains an unresolved field reference")
			}
			projectedChange.Field = publicField
			if change.Column != "" {
				columns := columnNames[publicField]
				publicColumn, ok := columns[change.Column]
				if !ok {
					return DriftReport{}, fmt.Errorf("ServiceDesk drift contains an unresolved column reference")
				}
				projectedChange.Column = publicColumn
			}
		} else if change.Column != "" {
			return DriftReport{}, fmt.Errorf("ServiceDesk drift contains an unresolved column reference")
		}
		projected.Changes = append(projected.Changes, projectedChange)
	}
	sortDriftChanges(projected.Changes)
	return projected, nil
}

func addProfileDriftIdentifier(identifiers map[string]string, identifier, publicName string) error {
	if identifier == "" {
		return nil
	}
	if existing, ok := identifiers[identifier]; ok && existing != publicName {
		return fmt.Errorf("ServiceDesk drift public mapping is ambiguous")
	}
	identifiers[identifier] = publicName
	return nil
}

func overlayRequired(required *bool) bool {
	return required != nil && *required
}

func addProfilePublicNames(claimed map[string]struct{}, name string, aliases []string, kind string) error {
	for _, publicName := range append([]string{name}, aliases...) {
		if _, exists := claimed[publicName]; exists {
			return fmt.Errorf("ServiceDesk profile resolves multiple %ss to one public name", kind)
		}
		claimed[publicName] = struct{}{}
	}
	return nil
}

func validateProfileColumnNames(columns []InputColumn) error {
	claimed := make(map[string]struct{}, len(columns)*2)
	for _, column := range columns {
		if err := addProfilePublicNames(claimed, column.Name, column.Aliases, "table column"); err != nil {
			return err
		}
	}
	return nil
}

func resolveURLInputContract(fields []Field) (InputContract, error) {
	displayNameCounts := make(map[string]int, len(fields))
	for _, field := range fields {
		displayNameCounts[field.Name]++
	}

	publicNames := make(map[string]struct{}, len(fields))
	backendToPublic := make(map[string]string, len(fields))
	resolvedNames := make([]string, len(fields))
	for index, field := range fields {
		name := field.StableID
		if name == "" {
			if displayNameCounts[field.Name] != 1 {
				return InputContract{}, fmt.Errorf("ServiceDesk metadata has duplicate display name %q without a stable identifier", field.Name)
			}
			name = field.Name
		}
		if _, exists := publicNames[name]; exists {
			return InputContract{}, fmt.Errorf("ServiceDesk metadata resolves multiple fields to public name %q", name)
		}
		publicNames[name] = struct{}{}
		if err := addConditionReference(backendToPublic, field.BackendKey, name); err != nil {
			return InputContract{}, err
		}
		if err := addConditionReference(backendToPublic, name, name); err != nil {
			return InputContract{}, err
		}
		resolvedNames[index] = name
	}

	contract := InputContract{Fields: make([]InputField, 0, len(fields))}
	for index, field := range fields {
		name := resolvedNames[index]
		columns, err := resolveURLColumns(field.TableColumns)
		if err != nil {
			return InputContract{}, fmt.Errorf("resolve columns for field %q: %w", name, err)
		}
		resolved := InputField{
			Name:             name,
			DisplayName:      field.Name,
			StableID:         field.StableID,
			Type:             field.Type,
			Required:         field.Required,
			Conditional:      len(field.Conditions.Conditions) > 0,
			DefaultAvailable: field.DefaultAvailable,
			Choices:          field.Choices,
			Columns:          columns,
			Supported:        field.Supported,
		}
		if resolved.Conditional {
			conditions, err := resolveConditionSources(field.Conditions, backendToPublic)
			if err != nil {
				return InputContract{}, err
			}
			resolved.Conditions = &conditions
		}
		if field.Required && !field.Supported {
			contract.UnsupportedRequiredFieldCount++
		}
		for _, column := range field.TableColumns {
			if column.Required && !column.Supported {
				contract.UnsupportedRequiredFieldCount++
			}
		}
		contract.Fields = append(contract.Fields, resolved)
	}
	return contract, nil
}

func addConditionReference(references map[string]string, reference, publicName string) error {
	if existing, ok := references[reference]; ok && existing != publicName {
		return fmt.Errorf("ServiceDesk metadata has ambiguous condition field references")
	}
	references[reference] = publicName
	return nil
}

func resolveConditionSources(conditions ConditionSet, backendToPublic map[string]string) (ConditionSet, error) {
	resolved, err := resolveConditions(conditions.Conditions, backendToPublic)
	if err != nil {
		return ConditionSet{}, err
	}
	conditions.Conditions = resolved
	return conditions, nil
}

func resolveConditions(conditions []Condition, backendToPublic map[string]string) ([]Condition, error) {
	resolved := make([]Condition, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Field != "" {
			name, ok := backendToPublic[condition.Field]
			if !ok {
				return nil, fmt.Errorf("ServiceDesk condition references an unknown field")
			}
			condition.Field = name
		}
		if len(condition.Conditions) > 0 {
			children, err := resolveConditions(condition.Conditions, backendToPublic)
			if err != nil {
				return nil, err
			}
			condition.Conditions = children
		}
		resolved = append(resolved, condition)
	}
	return resolved, nil
}

func resolveURLColumns(columns []TableColumn) ([]InputColumn, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	displayNameCounts := make(map[string]int, len(columns))
	for _, column := range columns {
		displayNameCounts[column.Name]++
	}
	resolved := make([]InputColumn, 0, len(columns))
	for _, column := range columns {
		if displayNameCounts[column.Name] != 1 {
			return nil, fmt.Errorf("ServiceDesk metadata has duplicate table column display name %q", column.Name)
		}
		resolved = append(resolved, InputColumn{
			Name:             column.Name,
			DisplayName:      column.Name,
			Type:             column.Type,
			Required:         column.Required,
			DefaultAvailable: column.DefaultAvailable,
			Choices:          column.Choices,
			Supported:        column.Supported,
		})
	}
	return resolved, nil
}

func hashResolvedContract(contract ResolvedContract) (string, error) {
	contract.ContractHash = ""
	return canonicalHash(contract)
}
