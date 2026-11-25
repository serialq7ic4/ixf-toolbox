package servicedeskprofile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

type Choice struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type TableColumn struct {
	BackendKey       string       `json:"-"`
	Name             string       `json:"name"`
	Type             string       `json:"type"`
	Required         bool         `json:"required"`
	DefaultValue     any          `json:"-"`
	DefaultAvailable bool         `json:"defaultAvailable"`
	Choices          []Choice     `json:"choices,omitempty"`
	ChoiceHash       string       `json:"-"`
	Conditions       ConditionSet `json:"conditions,omitempty"`
	Supported        bool         `json:"supported"`
}

type Condition struct {
	Field      string      `json:"field,omitempty"`
	Operator   string      `json:"operator"`
	Value      any         `json:"value,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

type ConditionSet struct {
	Source      string      `json:"source,omitempty"`
	Operator    string      `json:"operator,omitempty"`
	Conditions  []Condition `json:"conditions,omitempty"`
	Unsupported bool        `json:"unsupported,omitempty"`
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
	BackendKey       string       `json:"backendKey"`
	Name             string       `json:"name"`
	Type             string       `json:"type"`
	Required         bool         `json:"required"`
	DefaultValue     any          `json:"defaultValue"`
	DefaultAvailable bool         `json:"defaultAvailable"`
	Choices          []Choice     `json:"choices,omitempty"`
	Conditions       ConditionSet `json:"conditions,omitempty"`
	Supported        bool         `json:"supported"`
}

type rawField struct {
	StableID     string           `json:"field_unique_Identifier"`
	Name         string           `json:"name"`
	LiveName     string           `json:"field_name"`
	Type         string           `json:"type"`
	LiveType     string           `json:"field_type"`
	Required     *json.RawMessage `json:"required"`
	LiveRequired *json.RawMessage `json:"field_required"`
	Default      *json.RawMessage `json:"default"`
	LiveDefault  *json.RawMessage `json:"field_default"`
	Choices      []Choice         `json:"choices"`
	LiveChoices  json.RawMessage  `json:"field_choice"`
	Conditions   *rawConditionSet `json:"conditions_expression"`
	Columns      []rawTableColumn `json:"columns"`
}

type rawTableColumn struct {
	BackendKey string           `json:"key"`
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Required   bool             `json:"required"`
	Default    *json.RawMessage `json:"default"`
	Choices    []Choice         `json:"choices"`
	Conditions *rawConditionSet `json:"conditions_expression"`
}

type rawConditionSet struct {
	Operator                   string          `json:"operator"`
	Conditions                 []rawCondition  `json:"conditions"`
	Hide                       *bool           `json:"hide"`
	FieldDisplay               *bool           `json:"field_display"`
	GroupConditionRelation     string          `json:"group_condition_relation"`
	ConditionGroupsListAllDict json.RawMessage `json:"condition_groups_list_all_dict"`
}

type rawCondition struct {
	Field      string         `json:"field"`
	Operator   string         `json:"operator"`
	Value      any            `json:"value"`
	Conditions []rawCondition `json:"conditions"`
}

type rawLiveChoice struct {
	Name         string           `json:"name"`
	Label        string           `json:"label"`
	Key          string           `json:"key"`
	Value        string           `json:"value"`
	Type         string           `json:"type"`
	LiveType     string           `json:"field_type"`
	Required     *json.RawMessage `json:"required"`
	LiveRequired *json.RawMessage `json:"field_required"`
	IsRequest    *bool            `json:"isRequest"`
	Default      *json.RawMessage `json:"default"`
	LiveDefault  *json.RawMessage `json:"field_default"`
	Choices      []Choice         `json:"choices"`
	LiveChoices  json.RawMessage  `json:"field_choice"`
	Conditions   *rawConditionSet `json:"conditions_expression"`
}

func LoadTemplate(ctx context.Context, source MetadataSource, target EditTarget) (Template, error) {
	if source == nil {
		return Template{}, invalidRequest("ServiceDesk client is required")
	}
	service, err := source.Service(ctx, target)
	if err != nil {
		return Template{}, err
	}
	lading, err := source.Lading(ctx, target, service.SnapshotID)
	if err != nil {
		return Template{}, err
	}

	return BuildTemplate(target, service, lading)
}

func BuildTemplate(target EditTarget, service Service, lading Lading) (Template, error) {
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
				BackendKey:       column.BackendKey,
				Name:             column.Name,
				Type:             column.Type,
				Required:         column.Required,
				DefaultValue:     column.DefaultValue,
				DefaultAvailable: column.DefaultAvailable,
				Choices:          column.Choices,
				Conditions:       column.Conditions,
				Supported:        column.Supported,
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

		name := firstNonEmpty(raw.LiveName, raw.Name)
		fieldType := firstNonEmpty(raw.LiveType, raw.Type)
		if strings.TrimSpace(name) == "" || strings.TrimSpace(fieldType) == "" {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata is missing a field name or type"}
		}
		required, err := normalizeRequired(raw.LiveRequired, raw.Required)
		if err != nil {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has an invalid required marker"}
		}
		field := Field{
			BackendKey:  key,
			StableID:    raw.StableID,
			Name:        name,
			Type:        fieldType,
			Required:    required,
			Choices:     nonNilChoices(raw.Choices),
			Supported:   supportedFieldType(fieldType),
			DisplayOnly: displayOnlyFieldType(fieldType),
		}
		defaultRaw, liveDefault := preferredRaw(raw.LiveDefault, raw.Default)
		if defaultRaw != nil && string(*defaultRaw) != "null" {
			if err := decodeMetadataJSON(*defaultRaw, &field.DefaultValue); err != nil {
				return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has an invalid default value"}
			}
			field.DefaultAvailable = !liveDefault || !blankString(field.DefaultValue)
		}
		if len(field.Choices) == 0 && len(raw.LiveChoices) > 0 && !tableFieldType(fieldType) {
			field.Choices, err = normalizeLiveChoices(raw.LiveChoices)
			if err != nil {
				return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has invalid choices"}
			}
		}
		if len(field.Choices) > 0 {
			field.ChoiceHash, _ = canonicalHash(field.Choices)
		}
		if raw.Conditions != nil {
			field.Conditions = normalizeConditionSet(*raw.Conditions)
		}
		columns, err := normalizeColumns(raw.Columns)
		if err != nil {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has an invalid table column default value"}
		}
		if len(columns) == 0 && tableFieldType(fieldType) && len(raw.LiveChoices) > 0 {
			columns, err = normalizeLiveColumns(raw.LiveChoices)
			if err != nil {
				return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk lading metadata has invalid table columns"}
			}
		}
		field.TableColumns = columns
		fields = append(fields, field)
	}
	return fields, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func preferredRaw(primary, fallback *json.RawMessage) (*json.RawMessage, bool) {
	if primary != nil {
		return primary, true
	}
	return fallback, false
}

func blankString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func normalizeRequired(primary, fallback *json.RawMessage) (bool, error) {
	raw, _ := preferredRaw(primary, fallback)
	if raw == nil || string(*raw) == "null" {
		return false, nil
	}
	var value any
	if err := decodeMetadataJSON(*raw, &value); err != nil {
		return false, err
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		switch strings.ToUpper(strings.TrimSpace(typed)) {
		case "REQUIRE", "REQUIRED", "TRUE":
			return true, nil
		case "OPTION", "OPTIONAL", "FALSE":
			return false, nil
		}
	}
	return false, &APIError{Code: "invalid_response", Message: "unknown ServiceDesk required marker"}
}

func normalizeLiveChoices(data json.RawMessage) ([]Choice, error) {
	entries, err := decodeLiveChoiceEntries(data, true)
	if err != nil {
		return nil, err
	}
	choices := make([]Choice, 0, len(entries))
	for _, entry := range entries {
		label := firstNonEmpty(entry.Name, entry.Label)
		value := firstNonEmpty(entry.Key, entry.Value)
		if label == "" || value == "" {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk choice is missing a label or value"}
		}
		choices = append(choices, Choice{Label: label, Value: value})
	}
	return choices, nil
}

func normalizeLiveColumns(data json.RawMessage) ([]TableColumn, error) {
	entries, err := decodeLiveChoiceEntries(data, false)
	if err != nil {
		return nil, err
	}
	columns := make([]TableColumn, 0, len(entries))
	for _, entry := range entries {
		key := firstNonEmpty(entry.Key, entry.Value)
		name := firstNonEmpty(entry.Name, entry.Label)
		if key == "" || name == "" {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk table column is missing a key or name"}
		}
		required := false
		if entry.IsRequest != nil {
			required = *entry.IsRequest
		} else {
			required, err = normalizeRequired(entry.LiveRequired, entry.Required)
			if err != nil {
				return nil, err
			}
		}
		columnType := firstNonEmpty(entry.LiveType, entry.Type, "input")
		column := TableColumn{
			BackendKey: key,
			Name:       name,
			Type:       columnType,
			Required:   required,
			Choices:    nonNilChoices(entry.Choices),
			Supported:  supportedTableColumnType(columnType),
		}
		defaultRaw, liveDefault := preferredRaw(entry.LiveDefault, entry.Default)
		if defaultRaw != nil && string(*defaultRaw) != "null" {
			if err := decodeMetadataJSON(*defaultRaw, &column.DefaultValue); err != nil {
				return nil, err
			}
			column.DefaultAvailable = !liveDefault || !blankString(column.DefaultValue)
		}
		if len(column.Choices) == 0 && len(entry.LiveChoices) > 0 {
			column.Choices, err = normalizeLiveChoices(entry.LiveChoices)
			if err != nil {
				return nil, err
			}
		}
		if len(column.Choices) > 0 {
			column.ChoiceHash, _ = canonicalHash(column.Choices)
		}
		if entry.Conditions != nil {
			column.Conditions = normalizeConditionSet(*entry.Conditions)
		}
		columns = append(columns, column)
	}
	return columns, nil
}

func decodeLiveChoiceEntries(data json.RawMessage, allowStringValues bool) ([]rawLiveChoice, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	switch trimmed[0] {
	case '{':
		var values map[string]json.RawMessage
		if err := decodeMetadataJSON(trimmed, &values); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(left, right int) bool {
			leftNumber, leftErr := strconv.Atoi(keys[left])
			rightNumber, rightErr := strconv.Atoi(keys[right])
			if leftErr == nil && rightErr == nil {
				return leftNumber < rightNumber
			}
			return keys[left] < keys[right]
		})
		entries := make([]rawLiveChoice, 0, len(keys))
		for _, key := range keys {
			var entry rawLiveChoice
			value := bytes.TrimSpace(values[key])
			if allowStringValues && len(value) > 0 && value[0] == '"' {
				entry.Key = key
				if err := decodeMetadataJSON(value, &entry.Name); err != nil {
					return nil, err
				}
			} else if err := decodeMetadataJSON(value, &entry); err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
		return entries, nil
	case '[':
		var entries []rawLiveChoice
		if err := decodeMetadataJSON(trimmed, &entries); err != nil {
			return nil, err
		}
		return entries, nil
	default:
		return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk choices must be an object or array"}
	}
}

func decodeMetadataJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func normalizeColumns(rawColumns []rawTableColumn) ([]TableColumn, error) {
	if len(rawColumns) == 0 {
		return nil, nil
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
		if raw.Default != nil && string(*raw.Default) != "null" {
			if err := decodeMetadataJSON(*raw.Default, &column.DefaultValue); err != nil {
				return nil, err
			}
			column.DefaultAvailable = true
		}
		if len(column.Choices) > 0 {
			column.ChoiceHash, _ = canonicalHash(column.Choices)
		}
		if raw.Conditions != nil {
			column.Conditions = normalizeConditionSet(*raw.Conditions)
		}
		columns = append(columns, column)
	}
	return columns, nil
}

func normalizeConditionSet(raw rawConditionSet) ConditionSet {
	if raw.liveConditionExpression() {
		groups := bytes.TrimSpace(raw.ConditionGroupsListAllDict)
		if len(groups) == 0 || bytes.Equal(groups, []byte("null")) {
			return ConditionSet{Unsupported: true}
		}
		var object map[string]json.RawMessage
		if err := decodeMetadataJSON(groups, &object); err != nil || len(object) != 0 {
			return ConditionSet{Unsupported: true}
		}
		return ConditionSet{}
	}
	conditions := make([]Condition, 0, len(raw.Conditions))
	for _, rawCondition := range raw.Conditions {
		conditions = append(conditions, normalizeCondition(rawCondition))
	}
	return ConditionSet{Operator: raw.Operator, Conditions: conditions}
}

func (raw rawConditionSet) liveConditionExpression() bool {
	return raw.Hide != nil || raw.FieldDisplay != nil || raw.GroupConditionRelation != "" || raw.ConditionGroupsListAllDict != nil
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
