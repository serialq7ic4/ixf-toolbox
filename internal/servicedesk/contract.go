package servicedesk

import (
	"context"
	"fmt"
)

const ContractVersion = 1

type InspectRequest struct {
	Target EditTarget
}

type ResolvedTarget struct {
	BusinessID  int    `json:"businessId"`
	ServiceID   int    `json:"serviceId"`
	SnapshotID  string `json:"snapshotId"`
	ServiceKey  string `json:"serviceKey,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

type InputColumn struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Choices     []Choice `json:"choices,omitempty"`
	Supported   bool     `json:"supported"`
}

type InputField struct {
	Name             string        `json:"name"`
	Aliases          []string      `json:"aliases,omitempty"`
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

type ResolvedContract struct {
	ContractVersion     int            `json:"contractVersion"`
	Target              ResolvedTarget `json:"target"`
	InputContract       InputContract  `json:"inputContract"`
	TemplateDrift       bool           `json:"templateDrift"`
	DriftClassification string         `json:"driftClassification,omitempty"`
	ContractHash        string         `json:"contractHash,omitempty"`
}

func Inspect(ctx context.Context, client *Client, request InspectRequest) (ResolvedContract, error) {
	template, err := LoadTemplate(ctx, client, request.Target)
	if err != nil {
		return ResolvedContract{}, err
	}
	inputContract, err := resolveURLInputContract(template.Fields)
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
		InputContract: inputContract,
	}
	contract.ContractHash, err = hashResolvedContract(contract)
	if err != nil {
		return ResolvedContract{}, fmt.Errorf("hash resolved ServiceDesk contract: %w", err)
	}
	return contract, nil
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
		backendToPublic[field.BackendKey] = name
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
			conditions := resolveConditionSources(field.Conditions, backendToPublic)
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

func resolveConditionSources(conditions ConditionSet, backendToPublic map[string]string) ConditionSet {
	conditions.Conditions = resolveConditions(conditions.Conditions, backendToPublic)
	return conditions
}

func resolveConditions(conditions []Condition, backendToPublic map[string]string) []Condition {
	resolved := make([]Condition, 0, len(conditions))
	for _, condition := range conditions {
		if name, ok := backendToPublic[condition.Field]; ok {
			condition.Field = name
		}
		if len(condition.Conditions) > 0 {
			condition.Conditions = resolveConditions(condition.Conditions, backendToPublic)
		}
		resolved = append(resolved, condition)
	}
	return resolved
}

func resolveURLColumns(columns []TableColumn) ([]InputColumn, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	displayNameCounts := make(map[string]int, len(columns))
	for _, column := range columns {
		displayNameCounts[column.Name]++
	}
	publicNames := make(map[string]struct{}, len(columns))
	resolved := make([]InputColumn, 0, len(columns))
	for _, column := range columns {
		name := column.BackendKey
		if name == "" {
			if displayNameCounts[column.Name] != 1 {
				return nil, fmt.Errorf("duplicate display name %q without a column key", column.Name)
			}
			name = column.Name
		}
		if _, exists := publicNames[name]; exists {
			return nil, fmt.Errorf("multiple columns resolve to public name %q", name)
		}
		publicNames[name] = struct{}{}
		resolved = append(resolved, InputColumn{
			Name:        name,
			DisplayName: column.Name,
			Type:        column.Type,
			Required:    column.Required,
			Choices:     column.Choices,
			Supported:   column.Supported,
		})
	}
	return resolved, nil
}

func hashResolvedContract(contract ResolvedContract) (string, error) {
	contract.ContractHash = ""
	return canonicalHash(contract)
}
