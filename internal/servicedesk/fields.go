package servicedesk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
	"time"
)

type TicketInput map[string]any

type FieldIssue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Column   string `json:"column,omitempty"`
	RowIndex int    `json:"-"`
}

func (issue FieldIssue) MarshalJSON() ([]byte, error) {
	type wire struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		Field    string `json:"field,omitempty"`
		Column   string `json:"column,omitempty"`
		RowIndex *int   `json:"rowIndex,omitempty"`
	}
	var rowIndex *int
	if issue.Column != "" {
		value := issue.RowIndex
		rowIndex = &value
	}
	return json.Marshal(wire{Code: issue.Code, Message: issue.Message, Field: issue.Field, Column: issue.Column, RowIndex: rowIndex})
}

type NormalizedInput struct {
	Inputs              map[string]any
	Sources             map[string]string
	ProvidedFieldCount  int
	DefaultedFieldCount int
	TableRowCount       int
	required            map[string]bool
	publicNames         map[string]string
}

type fieldView struct {
	PublicName string
	Aliases    []string
	Field      Field
	Overlay    *ResolvedField
}

func DecodeTicketInput(data []byte) (TicketInput, error) {
	value, err := decodeUniqueJSON(data)
	if err != nil {
		return nil, fmt.Errorf("decode ServiceDesk ticket input: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ServiceDesk ticket input must be a JSON object")
	}
	if fields, wrapped := object["fields"]; wrapped {
		if len(object) != 1 {
			return nil, fmt.Errorf("ServiceDesk ticket input fields wrapper cannot contain other properties")
		}
		wrappedObject, ok := fields.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ServiceDesk ticket input fields must be a JSON object")
		}
		return TicketInput(wrappedObject), nil
	}
	return TicketInput(object), nil
}

func decodeUniqueJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeUniqueValue(decoder)
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func decodeUniqueValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			result := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key := keyToken.(string)
				if _, exists := result[key]; exists {
					return nil, fmt.Errorf("duplicate JSON property %q", key)
				}
				value, err := decodeUniqueValue(decoder)
				if err != nil {
					return nil, err
				}
				result[key] = value
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return result, nil
		case '[':
			result := make([]any, 0)
			for decoder.More() {
				value, err := decodeUniqueValue(decoder)
				if err != nil {
					return nil, err
				}
				result = append(result, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return result, nil
		}
	}
	return token, nil
}

func NormalizeInput(template Template, overlay *ResolvedOverlay, input TicketInput) (NormalizedInput, []FieldIssue) {
	normalized := NormalizedInput{Inputs: make(map[string]any), Sources: make(map[string]string), required: make(map[string]bool), publicNames: make(map[string]string)}
	views, aliases := buildFieldViews(template, overlay)
	issues := make([]FieldIssue, 0)
	resolvedInput := make(map[string]any, len(input))
	for key, value := range input {
		publicName, ok := aliases[key]
		if !ok {
			issueField := key
			if overlay != nil {
				for _, view := range views {
					if view.Field.BackendKey == key {
						issueField = view.PublicName
						break
					}
				}
			}
			issues = append(issues, FieldIssue{Code: "unknown_field", Field: issueField, Message: "unknown ticket field"})
			continue
		}
		if publicName == "" {
			issues = append(issues, FieldIssue{Code: "ambiguous_field", Field: key, Message: "field name is ambiguous"})
			continue
		}
		if _, exists := resolvedInput[publicName]; exists {
			issues = append(issues, FieldIssue{Code: "duplicate_field", Field: publicName, Message: "field was provided more than once"})
			continue
		}
		resolvedInput[publicName] = value
	}

	for _, view := range views {
		normalized.required[view.Field.BackendKey] = view.Field.Required
		normalized.publicNames[view.Field.BackendKey] = view.PublicName
		value, provided := resolvedInput[view.PublicName]
		source := ""
		if provided {
			normalized.ProvidedFieldCount++
			source = "input"
		} else if view.Overlay != nil && view.Overlay.Overlay.Default != nil {
			value, _ = decodeRawValue(view.Overlay.Overlay.Default)
			source = "overlay_default"
			normalized.DefaultedFieldCount++
		} else if view.Field.DefaultAvailable {
			value = view.Field.DefaultValue
			source = "live_default"
			normalized.DefaultedFieldCount++
		}

		if !provided && source == "" {
			if view.Field.Required && !view.Field.Supported && !view.Field.DisplayOnly {
				issues = append(issues, fieldIssue(view, "unsupported_field", "required field type is not supported for writing"))
				continue
			}
			if view.Field.Required && len(view.Field.Conditions.Conditions) == 0 && !view.Field.DisplayOnly {
				issues = append(issues, fieldIssue(view, "missing_required", "required field is missing"))
			}
			continue
		}
		if view.Field.DisplayOnly && provided {
			issues = append(issues, fieldIssue(view, "display_only", "display-only field cannot be provided"))
			if view.Field.DefaultAvailable {
				value = view.Field.DefaultValue
				source = "live_default"
			} else {
				continue
			}
		}
		if value == nil && strings.Contains(strings.ToLower(view.Field.Type), "attachment") && !view.Field.Required {
			normalized.Inputs[view.Field.BackendKey] = []any{}
			normalized.Sources[view.PublicName] = source
			continue
		}
		if value == nil {
			if view.Field.Required {
				issues = append(issues, fieldIssue(view, "required_null", "required field cannot be null"))
			} else {
				normalized.Sources[view.PublicName] = "explicit_null"
			}
			continue
		}
		if !view.Field.Supported {
			if isEmptyAttachment(view.Field.Type, value) {
				normalized.Inputs[view.Field.BackendKey] = []any{}
				normalized.Sources[view.PublicName] = source
				continue
			}
			issues = append(issues, fieldIssue(view, "unsupported_field", "field type is not supported for writing"))
			continue
		}
		value, fieldIssues := normalizeValue(value, view.Field, view, -1)
		if len(fieldIssues) > 0 {
			issues = append(issues, fieldIssues...)
			continue
		}
		if view.Field.BackendKey != "" {
			normalized.Inputs[view.Field.BackendKey] = value
		}
		normalized.Sources[view.PublicName] = source
		if table, ok := value.([]any); ok && tableFieldType(view.Field.Type) {
			normalized.TableRowCount += len(table)
		}
	}
	return normalized, issues
}

func buildFieldViews(template Template, overlay *ResolvedOverlay) ([]fieldView, map[string]string) {
	views := make([]fieldView, 0, len(template.Fields))
	aliases := make(map[string]string, len(template.Fields)*2)
	for _, field := range template.Fields {
		view := fieldView{PublicName: preferredFieldName(field), Field: field}
		if overlay != nil {
			for _, resolved := range overlay.Fields {
				if sameLiveField(resolved.Field, field) {
					copy := resolved
					view.PublicName = resolved.PublicName
					view.Aliases = append(view.Aliases, resolved.Overlay.Aliases...)
					view.Overlay = &copy
					view.Field.Required = view.Field.Required || overlayRequired(resolved.Overlay.Required)
					break
				}
			}
		}
		views = append(views, view)
		addFieldAlias(aliases, view.PublicName, view.PublicName)
		for _, alias := range view.Aliases {
			addFieldAlias(aliases, alias, view.PublicName)
		}
		if overlay == nil {
			if field.StableID != "" {
				addFieldAlias(aliases, field.StableID, view.PublicName)
			}
			if field.Name != "" {
				addFieldAlias(aliases, field.Name, view.PublicName)
			}
		}
	}
	return views, aliases
}

func addFieldAlias(aliases map[string]string, alias, publicName string) {
	if existing, ok := aliases[alias]; ok && existing != publicName {
		aliases[alias] = ""
		return
	}
	if _, ambiguous := aliases[alias]; !ambiguous {
		aliases[alias] = publicName
	}
}

func sameLiveField(left, right Field) bool {
	if left.BackendKey != "" && right.BackendKey != "" {
		return left.BackendKey == right.BackendKey
	}
	if left.StableID != "" && right.StableID != "" {
		return left.StableID == right.StableID
	}
	return left.Name != "" && left.Name == right.Name
}

func fieldIssue(view fieldView, code, message string) FieldIssue {
	return FieldIssue{Code: code, Field: view.PublicName, Message: message}
}

func normalizeValue(value any, field Field, view fieldView, rowIndex int) (any, []FieldIssue) {
	issues := make([]FieldIssue, 0)
	issue := func(code, message string) {
		issues = append(issues, FieldIssue{Code: code, Field: view.PublicName, Message: message, RowIndex: maxInt(rowIndex, 0)})
	}
	typeName := strings.ToLower(field.Type)
	switch typeName {
	case "input", "multi_input", "textarea":
		if _, ok := value.(string); !ok {
			issue("invalid_type", "field requires a string")
			return nil, issues
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			issue("invalid_type", "field requires a JSON number")
			return nil, issues
		}
	case "date":
		if text, ok := value.(string); !ok || !validTime(text, "2006-01-02") {
			issue("invalid_format", "field requires YYYY-MM-DD")
			return nil, issues
		}
	case "time":
		if text, ok := value.(string); !ok || !validTime(text, "2006-01-02 15:04:05") {
			issue("invalid_format", "field requires YYYY-MM-DD HH:mm:ss")
			return nil, issues
		}
	case "only_time":
		if text, ok := value.(string); !ok || !validTime(text, "15:04:05") {
			issue("invalid_format", "field requires HH:mm:ss")
			return nil, issues
		}
	case "date_time_range":
		if !validRange(value, "2006-01-02 15:04:05") {
			issue("invalid_format", "field requires two datetime strings")
			return nil, issues
		}
	case "time_range":
		if !validRange(value, "15:04:05") {
			issue("invalid_format", "field requires two time strings")
			return nil, issues
		}
	case "switch":
		if _, ok := value.(bool); !ok {
			issue("invalid_type", "field requires a boolean")
			return nil, issues
		}
	case "select", "single_select", "radio", "checkbox":
		text, ok := value.(string)
		if !ok {
			issue("invalid_type", "field requires a choice string")
			return nil, issues
		}
		converted, code := normalizeChoice(text, field.Choices)
		if code != "" {
			issue(code, "choice does not resolve to exactly one option")
			return nil, issues
		}
		value = converted
	case "multi_select", "multi_checkbox":
		values, ok := value.([]any)
		if !ok {
			issue("invalid_type", "field requires a choice array")
			return nil, issues
		}
		converted := make([]any, 0, len(values))
		seen := make(map[string]struct{}, len(values))
		for _, item := range values {
			text, ok := item.(string)
			if !ok {
				issue("invalid_type", "choice array items must be strings")
				return nil, issues
			}
			choice, code := normalizeChoice(text, field.Choices)
			if code != "" {
				issue(code, "choice does not resolve to exactly one option")
				return nil, issues
			}
			if _, exists := seen[choice]; !exists {
				converted = append(converted, choice)
				seen[choice] = struct{}{}
			}
		}
		value = converted
	case "tui_editor":
		switch editor := value.(type) {
		case string:
			value = "<p>" + html.EscapeString(editor) + "</p>"
		case map[string]any:
			if len(editor) != 1 {
				issue("invalid_shape", "tui_editor object must contain only html")
				return nil, issues
			}
			if text, ok := editor["html"].(string); ok {
				value = map[string]any{"html": text}
			} else {
				issue("invalid_type", "tui_editor html must be a string")
				return nil, issues
			}
		default:
			issue("invalid_type", "tui_editor requires a string or html object")
			return nil, issues
		}
	case "basis_table", "extend_table":
		rows, ok := value.([]any)
		if !ok {
			issue("invalid_type", "table field requires an object array")
			return nil, issues
		}
		converted := make([]any, 0, len(rows))
		for index, row := range rows {
			rowObject, ok := row.(map[string]any)
			if !ok {
				issue("invalid_type", "table rows must be objects")
				return nil, issues
			}
			convertedRow, rowIssues := normalizeTableRow(rowObject, field, view, index)
			if len(rowIssues) > 0 {
				issues = append(issues, rowIssues...)
				continue
			}
			converted = append(converted, convertedRow)
		}
		value = converted
	}
	return value, issues
}

func normalizeTableRow(row map[string]any, field Field, view fieldView, rowIndex int) (map[string]any, []FieldIssue) {
	columns := make([]TableColumn, len(field.TableColumns))
	copy(columns, field.TableColumns)
	publicNames := make(map[string]ResolvedColumn)
	if view.Overlay != nil {
		for name, column := range view.Overlay.Columns {
			publicNames[name] = column
			for _, alias := range column.Overlay.Aliases {
				publicNames[alias] = column
			}
		}
	}
	issues := make([]FieldIssue, 0)
	resolved := make(map[int]string)
	result := make(map[string]any)
	for name, value := range row {
		index, publicName, ok, ambiguous := matchInputColumn(name, columns, publicNames, view.Overlay != nil)
		if ambiguous {
			issues = append(issues, tableIssue(view, "ambiguous_column", name, rowIndex, "table column is ambiguous"))
			continue
		}
		if !ok {
			issueColumn := name
			if view.Overlay != nil {
				for _, column := range columns {
					if column.BackendKey == name || column.Name == name {
						issueColumn = column.Name
						for _, resolved := range publicNames {
							if resolved.Column.BackendKey == column.BackendKey {
								issueColumn = resolved.PublicName
								break
							}
						}
						break
					}
				}
			}
			issues = append(issues, tableIssue(view, "unknown_column", issueColumn, rowIndex, "unknown table column"))
			continue
		}
		if previous, exists := resolved[index]; exists {
			issues = append(issues, tableIssue(view, "duplicate_column", publicName, rowIndex, "table column was provided more than once"))
			_ = previous
			continue
		}
		resolved[index] = name
		column := columns[index]
		columnView := fieldView{PublicName: publicName, Field: Field{BackendKey: column.BackendKey, Type: column.Type, Supported: column.Supported, Choices: column.Choices, Required: column.Required}}
		converted, columnIssues := normalizeValue(value, columnView.Field, columnView, rowIndex)
		for index := range columnIssues {
			columnIssues[index].Field = view.PublicName
			columnIssues[index].Column = publicName
			columnIssues[index].RowIndex = rowIndex
		}
		issues = append(issues, columnIssues...)
		if len(columnIssues) == 0 {
			result[column.BackendKey] = converted
		}
	}
	for index, column := range columns {
		if _, exists := resolved[index]; exists {
			continue
		}
		publicName, resolvedColumn := resolvedColumnForIndex(column, index, publicNames)
		value, source, available := columnDefault(column, resolvedColumn)
		if available {
			columnField := Field{BackendKey: column.BackendKey, Type: column.Type, Supported: column.Supported, Choices: column.Choices, Required: column.Required || overlayColumnRequired(resolvedColumn)}
			columnView := fieldView{PublicName: publicName, Field: columnField}
			converted, columnIssues := normalizeValue(value, columnField, columnView, rowIndex)
			for issueIndex := range columnIssues {
				columnIssues[issueIndex].Field = view.PublicName
				columnIssues[issueIndex].Column = publicName
				columnIssues[issueIndex].RowIndex = rowIndex
			}
			issues = append(issues, columnIssues...)
			if len(columnIssues) == 0 {
				result[column.BackendKey] = converted
				_ = source
			}
			continue
		}
		if column.Required || overlayColumnRequired(resolvedColumn) {
			issues = append(issues, tableIssue(view, "missing_required_column", publicName, rowIndex, "required table column is missing"))
		}
	}
	return result, issues
}

func resolvedColumnForIndex(column TableColumn, index int, public map[string]ResolvedColumn) (string, *ResolvedColumn) {
	for _, resolved := range public {
		if resolved.LiveIndex == index {
			copy := resolved
			if resolved.PublicName != "" {
				return resolved.PublicName, &copy
			}
			return column.Name, &copy
		}
	}
	if column.Name != "" {
		return column.Name, nil
	}
	return column.BackendKey, nil
}

func overlayColumnRequired(column *ResolvedColumn) bool {
	return column != nil && overlayRequired(column.Overlay.Required)
}

func columnDefault(column TableColumn, resolved *ResolvedColumn) (any, string, bool) {
	if resolved != nil && resolved.Overlay.Default != nil {
		value, err := decodeRawValue(resolved.Overlay.Default)
		return value, "overlay_default", err == nil
	}
	if column.DefaultAvailable {
		return column.DefaultValue, "live_default", true
	}
	return nil, "", false
}

func matchInputColumn(name string, columns []TableColumn, public map[string]ResolvedColumn, profileMode bool) (int, string, bool, bool) {
	if resolved, ok := public[name]; ok {
		return resolved.LiveIndex, resolved.PublicName, true, false
	}
	if profileMode {
		matches := make([]int, 0, 1)
		for index, column := range columns {
			if profileColumnMapped(index, public) {
				continue
			}
			if column.Name == name {
				matches = append(matches, index)
			}
		}
		if len(matches) != 1 {
			return 0, name, false, len(matches) > 1
		}
		return matches[0], columns[matches[0]].Name, true, false
	}
	for index, column := range columns {
		if column.BackendKey == name {
			return index, name, true, false
		}
	}
	matches := make([]int, 0, 1)
	for index, column := range columns {
		if column.Name == name {
			matches = append(matches, index)
		}
	}
	if len(matches) != 1 {
		return 0, name, false, len(matches) > 1
	}
	return matches[0], name, true, false
}

func profileColumnMapped(index int, public map[string]ResolvedColumn) bool {
	for _, resolved := range public {
		if resolved.LiveIndex == index {
			return true
		}
	}
	return false
}

func columnPublicName(column TableColumn, public map[string]ResolvedColumn) string {
	for _, resolved := range public {
		if resolved.Column.BackendKey == column.BackendKey || resolved.LiveIndex >= 0 && resolved.Column.BackendKey == column.BackendKey {
			if resolved.PublicName != "" {
				return resolved.PublicName
			}
		}
	}
	if column.Name != "" {
		return column.Name
	}
	return column.BackendKey
}

func tableIssue(view fieldView, code, column string, row int, message string) FieldIssue {
	return FieldIssue{Code: code, Field: view.PublicName, Column: column, RowIndex: row, Message: message}
}

func normalizeChoice(value string, choices []Choice) (string, string) {
	matches := make(map[string]struct{})
	for _, choice := range choices {
		if choice.Label == value || choice.Value == value {
			matches[choice.Value] = struct{}{}
		}
	}
	if len(matches) == 0 {
		return "", "unknown_choice"
	}
	if len(matches) != 1 {
		return "", "ambiguous_choice"
	}
	for value := range matches {
		return value, ""
	}
	return "", "unknown_choice"
}

func decodeRawValue(raw json.RawMessage) (any, error) { return decodeUniqueJSON(raw) }

func validTime(value, layout string) bool { _, err := time.Parse(layout, value); return err == nil }

func validRange(value any, layout string) bool {
	values, ok := value.([]any)
	if !ok || len(values) != 2 {
		return false
	}
	for _, item := range values {
		text, ok := item.(string)
		if !ok || !validTime(text, layout) {
			return false
		}
	}
	return true
}

func isEmptyAttachment(fieldType string, value any) bool {
	return strings.Contains(strings.ToLower(fieldType), "attachment") && (value == nil || func() bool { values, ok := value.([]any); return ok && len(values) == 0 }())
}

func maxInt(value, fallback int) int {
	if value < fallback {
		return fallback
	}
	return value
}
