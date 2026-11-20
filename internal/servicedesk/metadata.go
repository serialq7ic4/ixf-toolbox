package servicedesk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type Choice struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type TableColumn struct {
	BackendKey string   `json:"-"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	Choices    []Choice `json:"choices,omitempty"`
	ChoiceHash string   `json:"-"`
	Supported  bool     `json:"supported"`
}

type Condition struct {
	Field      string      `json:"field,omitempty"`
	Operator   string      `json:"operator"`
	Value      any         `json:"value,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

type ConditionSet struct {
	Source     string      `json:"source,omitempty"`
	Operator   string      `json:"operator,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

type Field struct {
	BackendKey       string        `json:"-"`
	StableID         string        `json:"stableId,omitempty"`
	Name             string        `json:"name"`
	Type             string        `json:"type"`
	Required         bool          `json:"required"`
	DefaultValue     any           `json:"-"`
	DefaultAvailable bool          `json:"defaultAvailable"`
	Choices          []Choice      `json:"choices,omitempty"`
	ChoiceHash       string        `json:"-"`
	Conditions       ConditionSet  `json:"conditions,omitempty"`
	TableColumns     []TableColumn `json:"tableColumns,omitempty"`
	Supported        bool          `json:"supported"`
	DisplayOnly      bool          `json:"-"`
}

type Template struct {
	Target       EditTarget
	SnapshotID   string
	ServiceKey   string
	ServiceName  string
	Fields       []Field
	MetadataHash string
}

type metadataFieldFingerprint struct {
	BackendKey       string                      `json:"backendKey"`
	StableID         string                      `json:"stableId,omitempty"`
	Name             string                      `json:"name"`
	Type             string                      `json:"type"`
	Required         bool                        `json:"required"`
	DefaultValue     any                         `json:"defaultValue"`
	DefaultAvailable bool                        `json:"defaultAvailable"`
	Choices          []Choice                    `json:"choices,omitempty"`
	Conditions       ConditionSet                `json:"conditions,omitempty"`
	TableColumns     []metadataColumnFingerprint `json:"tableColumns,omitempty"`
	Supported        bool                        `json:"supported"`
	DisplayOnly      bool                        `json:"displayOnly"`
}

type metadataColumnFingerprint struct {
	BackendKey string   `json:"backendKey"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	Choices    []Choice `json:"choices,omitempty"`
	Supported  bool     `json:"supported"`
}

type rawField struct {
	StableID   string           `json:"field_unique_Identifier"`
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Required   bool             `json:"required"`
	Default    *json.RawMessage `json:"default"`
	Choices    []Choice         `json:"choices"`
	Conditions *rawConditionSet `json:"conditions_expression"`
	Columns    []rawTableColumn `json:"columns"`
}

type rawTableColumn struct {
	BackendKey string   `json:"key"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	Choices    []Choice `json:"choices"`
}

type rawConditionSet struct {
	Operator   string         `json:"operator"`
	Conditions []rawCondition `json:"conditions"`
}

type rawCondition struct {
	Field      string         `json:"field"`
	Operator   string         `json:"operator"`
	Value      any            `json:"value"`
	Conditions []rawCondition `json:"conditions"`
}

func LoadTemplate(ctx context.Context, client *Client, target EditTarget) (Template, error) {
	if client == nil {
		return Template{}, invalidRequest("ServiceDesk client is required")
	}
	service, err := client.Service(ctx, target)
	if err != nil {
		return Template{}, err
	}
	lading, err := client.Lading(ctx, target, service.SnapshotID)
	if err != nil {
		return Template{}, err
	}

	fields, err := normalizeLading(lading)
	if err != nil {
		return Template{}, err
	}
	metadataHash, err := hashMetadata(service.Key, fields)
	if err != nil {
		return Template{}, &APIError{Code: "invalid_response", Message: "ServiceDesk metadata could not be normalized"}
	}

	return Template{
		Target:       target,
		SnapshotID:   service.SnapshotID,
		ServiceKey:   service.Key,
		ServiceName:  service.Name,
		Fields:       fields,
		MetadataHash: metadataHash,
	}, nil
}

func hashMetadata(serviceKey string, fields []Field) (string, error) {
	fingerprints := make([]metadataFieldFingerprint, 0, len(fields))
	for _, field := range fields {
		columns := make([]metadataColumnFingerprint, 0, len(field.TableColumns))
		for _, column := range field.TableColumns {
			columns = append(columns, metadataColumnFingerprint{
				BackendKey: column.BackendKey,
				Name:       column.Name,
				Type:       column.Type,
				Required:   column.Required,
				Choices:    column.Choices,
				Supported:  column.Supported,
			})
		}
		fingerprints = append(fingerprints, metadataFieldFingerprint{
			BackendKey:       field.BackendKey,
			StableID:         field.StableID,
			Name:             field.Name,
			Type:             field.Type,
			Required:         field.Required,
			DefaultValue:     field.DefaultValue,
			DefaultAvailable: field.DefaultAvailable,
			Choices:          field.Choices,
			Conditions:       field.Conditions,
			TableColumns:     columns,
			Supported:        field.Supported,
			DisplayOnly:      field.DisplayOnly,
		})
	}
	return canonicalHash(struct {
		ServiceKey string                     `json:"serviceKey"`
		Fields     []metadataFieldFingerprint `json:"fields"`
	}{ServiceKey: serviceKey, Fields: fingerprints})
}

func normalizeLading(lading Lading) ([]Field, error) {
	keys := make([]string, 0, len(lading))
	for key := range lading {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fields := make([]Field, 0, len(keys))
	for _, key := range keys {
		var raw rawField
		if err := decodeMetadataJSON(lading[key], &raw); err != nil {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has an unexpected field shape"}
		}

		field := Field{
			BackendKey:  key,
			StableID:    raw.StableID,
			Name:        raw.Name,
			Type:        raw.Type,
			Required:    raw.Required,
			Choices:     nonNilChoices(raw.Choices),
			Supported:   supportedFieldType(raw.Type),
			DisplayOnly: displayOnlyFieldType(raw.Type),
		}
		if raw.Default != nil && string(*raw.Default) != "null" {
			if err := decodeMetadataJSON(*raw.Default, &field.DefaultValue); err != nil {
				return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has an invalid default value"}
			}
			field.DefaultAvailable = true
		}
		if len(field.Choices) > 0 {
			field.ChoiceHash, _ = canonicalHash(field.Choices)
		}
		if raw.Conditions != nil {
			field.Conditions = normalizeConditionSet(*raw.Conditions)
		}
		field.TableColumns = normalizeColumns(raw.Columns)
		fields = append(fields, field)
	}
	return fields, nil
}

func decodeMetadataJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func normalizeColumns(rawColumns []rawTableColumn) []TableColumn {
	if len(rawColumns) == 0 {
		return nil
	}
	columns := make([]TableColumn, 0, len(rawColumns))
	for _, raw := range rawColumns {
		column := TableColumn{
			BackendKey: raw.BackendKey,
			Name:       raw.Name,
			Type:       raw.Type,
			Required:   raw.Required,
			Choices:    nonNilChoices(raw.Choices),
			Supported:  supportedTableColumnType(raw.Type),
		}
		if len(column.Choices) > 0 {
			column.ChoiceHash, _ = canonicalHash(column.Choices)
		}
		columns = append(columns, column)
	}
	return columns
}

func normalizeConditionSet(raw rawConditionSet) ConditionSet {
	conditions := make([]Condition, 0, len(raw.Conditions))
	for _, rawCondition := range raw.Conditions {
		conditions = append(conditions, normalizeCondition(rawCondition))
	}
	return ConditionSet{Operator: raw.Operator, Conditions: conditions}
}

func normalizeCondition(raw rawCondition) Condition {
	condition := Condition{Field: raw.Field, Operator: raw.Operator, Value: raw.Value}
	if len(raw.Conditions) > 0 {
		condition.Conditions = make([]Condition, 0, len(raw.Conditions))
		for _, child := range raw.Conditions {
			condition.Conditions = append(condition.Conditions, normalizeCondition(child))
		}
	}
	return condition
}

func supportedFieldType(fieldType string) bool {
	switch strings.ToLower(fieldType) {
	case "input", "multi_input", "textarea", "number", "date", "time", "only_time",
		"date_time_range", "time_range", "select", "single_select", "radio", "checkbox",
		"multi_select", "multi_checkbox", "switch", "basis_table", "extend_table", "tui_editor",
		"hyperlink":
		return true
	default:
		return false
	}
}

func supportedTableColumnType(fieldType string) bool {
	switch strings.ToLower(fieldType) {
	case "input", "multi_input", "textarea", "number", "date", "time", "only_time",
		"date_time_range", "time_range", "select", "single_select", "radio", "checkbox",
		"multi_select", "multi_checkbox", "switch", "tui_editor":
		return true
	default:
		return false
	}
}

func displayOnlyFieldType(fieldType string) bool {
	return strings.EqualFold(fieldType, "hyperlink")
}

func nonNilChoices(choices []Choice) []Choice {
	if len(choices) == 0 {
		return nil
	}
	return choices
}

func canonicalHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
