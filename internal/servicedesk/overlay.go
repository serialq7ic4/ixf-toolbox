package servicedesk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const OverlaySchemaVersion = 1

var (
	overlayNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	hashPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Overlay struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Routing       Routing                 `json:"routing"`
	Target        OverlayTarget           `json:"target"`
	Baseline      Baseline                `json:"baseline"`
	Fields        map[string]OverlayField `json:"fields"`
}

type Routing struct {
	DisplayName  string   `json:"displayName"`
	UseWhen      string   `json:"useWhen"`
	DoNotUseWhen string   `json:"doNotUseWhen"`
	Examples     []string `json:"examples"`
}

type OverlayTarget struct {
	BusinessID int `json:"businessId"`
	ServiceID  int `json:"serviceId"`
}

type Baseline struct {
	SnapshotID   string          `json:"snapshotId"`
	ServiceKey   string          `json:"serviceKey"`
	MetadataHash string          `json:"metadataHash"`
	Fields       []BaselineField `json:"fields"`
}

type BaselineField struct {
	StableID      string           `json:"stableId,omitempty"`
	Name          string           `json:"name"`
	Type          string           `json:"type"`
	Required      bool             `json:"required"`
	ChoiceHash    string           `json:"choiceHash,omitempty"`
	DefaultHash   string           `json:"defaultHash,omitempty"`
	ConditionHash string           `json:"conditionHash,omitempty"`
	Columns       []BaselineColumn `json:"columns,omitempty"`

	columnsPresent bool
}

func (field BaselineField) MarshalJSON() ([]byte, error) {
	type baselineFieldJSON struct {
		StableID      string            `json:"stableId,omitempty"`
		Name          string            `json:"name"`
		Type          string            `json:"type"`
		Required      bool              `json:"required"`
		ChoiceHash    string            `json:"choiceHash,omitempty"`
		DefaultHash   string            `json:"defaultHash,omitempty"`
		ConditionHash string            `json:"conditionHash,omitempty"`
		Columns       *[]BaselineColumn `json:"columns,omitempty"`
	}
	var columns *[]BaselineColumn
	if field.columnsPresent || field.Columns != nil || tableFieldType(field.Type) {
		value := field.Columns
		if value == nil {
			value = []BaselineColumn{}
		}
		columns = &value
	}
	return json.Marshal(baselineFieldJSON{
		StableID:      field.StableID,
		Name:          field.Name,
		Type:          field.Type,
		Required:      field.Required,
		ChoiceHash:    field.ChoiceHash,
		DefaultHash:   field.DefaultHash,
		ConditionHash: field.ConditionHash,
		Columns:       columns,
	})
}

type BaselineColumn struct {
	Key           string `json:"key,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Required      bool   `json:"required"`
	ChoiceHash    string `json:"choiceHash,omitempty"`
	DefaultHash   string `json:"defaultHash,omitempty"`
	ConditionHash string `json:"conditionHash,omitempty"`
}

type FieldMatcher struct {
	StableID string `json:"stableId,omitempty"`
	Name     string `json:"name,omitempty"`
}

type ColumnMatcher struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

type OverlayField struct {
	Match       FieldMatcher             `json:"match"`
	Aliases     []string                 `json:"aliases,omitempty"`
	Description string                   `json:"description,omitempty"`
	Default     json.RawMessage          `json:"default,omitempty"`
	Required    *bool                    `json:"required,omitempty"`
	Columns     map[string]OverlayColumn `json:"columns,omitempty"`
}

type OverlayColumn struct {
	Match       ColumnMatcher   `json:"match"`
	Aliases     []string        `json:"aliases,omitempty"`
	Description string          `json:"description,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
	Required    *bool           `json:"required,omitempty"`
}

type ResolvedOverlay struct {
	Overlay Overlay
	Fields  map[string]ResolvedField
	Aliases map[string]string
	Drift   DriftReport
}

type ResolvedField struct {
	PublicName string
	Field      Field
	Overlay    OverlayField
	Columns    map[string]ResolvedColumn
}

type ResolvedColumn struct {
	PublicName string
	Column     TableColumn
	Overlay    OverlayColumn
	LiveIndex  int
}

type DriftReport struct {
	Changed        bool          `json:"changed"`
	Classification string        `json:"classification,omitempty"`
	Changes        []DriftChange `json:"changes,omitempty"`
	ApplyBlocked   bool          `json:"applyBlocked"`
}

type DriftChange struct {
	Kind           string `json:"kind"`
	Field          string `json:"field,omitempty"`
	Column         string `json:"column,omitempty"`
	Classification string `json:"classification"`
	Message        string `json:"message"`
}

type overlayWire struct {
	SchemaVersion *int                         `json:"schemaVersion"`
	Name          *string                      `json:"name"`
	Description   *string                      `json:"description"`
	Routing       *routingWire                 `json:"routing"`
	Target        *overlayTargetWire           `json:"target"`
	Baseline      *baselineWire                `json:"baseline"`
	Fields        *map[string]overlayFieldWire `json:"fields"`
}

type routingWire struct {
	DisplayName  *string   `json:"displayName"`
	UseWhen      *string   `json:"useWhen"`
	DoNotUseWhen *string   `json:"doNotUseWhen"`
	Examples     *[]string `json:"examples"`
}

type overlayTargetWire struct {
	BusinessID *int `json:"businessId"`
	ServiceID  *int `json:"serviceId"`
}

type baselineWire struct {
	SnapshotID   *string              `json:"snapshotId"`
	ServiceKey   *string              `json:"serviceKey"`
	MetadataHash *string              `json:"metadataHash"`
	Fields       *[]baselineFieldWire `json:"fields"`
}

type baselineFieldWire struct {
	StableID      string          `json:"stableId"`
	Name          *string         `json:"name"`
	Type          *string         `json:"type"`
	Required      *bool           `json:"required"`
	ChoiceHash    string          `json:"choiceHash"`
	DefaultHash   string          `json:"defaultHash"`
	ConditionHash string          `json:"conditionHash"`
	Columns       json.RawMessage `json:"columns"`
}

type baselineColumnWire struct {
	Key           string  `json:"key"`
	Name          *string `json:"name"`
	Type          *string `json:"type"`
	Required      *bool   `json:"required"`
	ChoiceHash    string  `json:"choiceHash"`
	DefaultHash   string  `json:"defaultHash"`
	ConditionHash string  `json:"conditionHash"`
}

type overlayFieldWire struct {
	Match       *FieldMatcher                               `json:"match"`
	Aliases     optionalValue[[]string]                     `json:"aliases"`
	Description optionalValue[string]                       `json:"description"`
	Default     json.RawMessage                             `json:"default"`
	Required    optionalValue[bool]                         `json:"required"`
	Columns     optionalValue[map[string]overlayColumnWire] `json:"columns"`
}

type overlayColumnWire struct {
	Match       *ColumnMatcher          `json:"match"`
	Aliases     optionalValue[[]string] `json:"aliases"`
	Description optionalValue[string]   `json:"description"`
	Default     json.RawMessage         `json:"default"`
	Required    optionalValue[bool]     `json:"required"`
}

type optionalValue[T any] struct {
	Value   T
	Present bool
}

func (value *optionalValue[T]) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("optional ServiceDesk overlay property must not be null")
	}
	return decodeStrictJSON(data, &value.Value)
}

func DecodeOverlay(data []byte) (Overlay, error) {
	var wire overlayWire
	if err := decodeStrictJSON(data, &wire); err != nil {
		return Overlay{}, fmt.Errorf("decode ServiceDesk overlay: %w", err)
	}
	if wire.SchemaVersion == nil || wire.Name == nil || wire.Description == nil || wire.Routing == nil ||
		wire.Target == nil || wire.Baseline == nil || wire.Fields == nil {
		return Overlay{}, errors.New("ServiceDesk overlay is missing a required top-level property")
	}

	routing, err := routingFromWire(*wire.Routing)
	if err != nil {
		return Overlay{}, err
	}
	target, err := targetFromWire(*wire.Target)
	if err != nil {
		return Overlay{}, err
	}
	baseline, err := baselineFromWire(*wire.Baseline)
	if err != nil {
		return Overlay{}, err
	}
	fields := make(map[string]OverlayField, len(*wire.Fields))
	for name, field := range *wire.Fields {
		if field.Match == nil {
			return Overlay{}, fmt.Errorf("ServiceDesk overlay field %q is missing match", name)
		}
		columns, err := overlayColumnsFromWire(name, field.Columns)
		if err != nil {
			return Overlay{}, err
		}
		fields[name] = OverlayField{
			Match:       *field.Match,
			Aliases:     field.Aliases.Value,
			Description: field.Description.Value,
			Default:     field.Default,
			Required:    optionalBoolPointer(field.Required),
			Columns:     columns,
		}
	}
	overlay := Overlay{
		SchemaVersion: *wire.SchemaVersion,
		Name:          *wire.Name,
		Description:   *wire.Description,
		Routing:       routing,
		Target:        target,
		Baseline:      baseline,
		Fields:        fields,
	}
	if err := ValidateOverlaySchema(overlay); err != nil {
		return Overlay{}, err
	}
	return overlay, nil
}

func overlayColumnsFromWire(fieldName string, wire optionalValue[map[string]overlayColumnWire]) (map[string]OverlayColumn, error) {
	if !wire.Present {
		return nil, nil
	}
	columns := make(map[string]OverlayColumn, len(wire.Value))
	for name, column := range wire.Value {
		if column.Match == nil {
			return nil, fmt.Errorf("ServiceDesk overlay field %q column %q is missing match", fieldName, name)
		}
		columns[name] = OverlayColumn{
			Match:       *column.Match,
			Aliases:     column.Aliases.Value,
			Description: column.Description.Value,
			Default:     column.Default,
			Required:    optionalBoolPointer(column.Required),
		}
	}
	return columns, nil
}

func optionalBoolPointer(value optionalValue[bool]) *bool {
	if !value.Present {
		return nil
	}
	result := value.Value
	return &result
}

func routingFromWire(wire routingWire) (Routing, error) {
	if wire.DisplayName == nil || wire.UseWhen == nil || wire.DoNotUseWhen == nil || wire.Examples == nil {
		return Routing{}, errors.New("ServiceDesk overlay routing is missing a required property")
	}
	return Routing{
		DisplayName:  *wire.DisplayName,
		UseWhen:      *wire.UseWhen,
		DoNotUseWhen: *wire.DoNotUseWhen,
		Examples:     *wire.Examples,
	}, nil
}

func targetFromWire(wire overlayTargetWire) (OverlayTarget, error) {
	if wire.BusinessID == nil || wire.ServiceID == nil {
		return OverlayTarget{}, errors.New("ServiceDesk overlay target is missing a required property")
	}
	return OverlayTarget{BusinessID: *wire.BusinessID, ServiceID: *wire.ServiceID}, nil
}

func baselineFromWire(wire baselineWire) (Baseline, error) {
	if wire.SnapshotID == nil || wire.ServiceKey == nil || wire.MetadataHash == nil || wire.Fields == nil {
		return Baseline{}, errors.New("ServiceDesk overlay baseline is missing a required property")
	}
	baseline := Baseline{
		SnapshotID:   *wire.SnapshotID,
		ServiceKey:   *wire.ServiceKey,
		MetadataHash: *wire.MetadataHash,
		Fields:       make([]BaselineField, 0, len(*wire.Fields)),
	}
	for index, field := range *wire.Fields {
		if field.Name == nil || field.Type == nil || field.Required == nil {
			return Baseline{}, fmt.Errorf("ServiceDesk overlay baseline field %d is missing a required property", index)
		}
		converted := BaselineField{
			StableID:      field.StableID,
			Name:          *field.Name,
			Type:          *field.Type,
			Required:      *field.Required,
			ChoiceHash:    field.ChoiceHash,
			DefaultHash:   field.DefaultHash,
			ConditionHash: field.ConditionHash,
		}
		if field.Columns != nil {
			converted.columnsPresent = true
			if bytes.Equal(bytes.TrimSpace(field.Columns), []byte("null")) {
				return Baseline{}, fmt.Errorf("ServiceDesk overlay baseline field %d has null columns", index)
			}
			var columns []baselineColumnWire
			if err := decodeStrictJSON(field.Columns, &columns); err != nil {
				return Baseline{}, fmt.Errorf("decode ServiceDesk overlay baseline field %d columns: %w", index, err)
			}
			converted.Columns = make([]BaselineColumn, 0, len(columns))
			for columnIndex, column := range columns {
				if column.Name == nil || column.Type == nil || column.Required == nil {
					return Baseline{}, fmt.Errorf("ServiceDesk overlay baseline field %d column %d is missing a required property", index, columnIndex)
				}
				converted.Columns = append(converted.Columns, BaselineColumn{
					Key:           column.Key,
					Name:          *column.Name,
					Type:          *column.Type,
					Required:      *column.Required,
					ChoiceHash:    column.ChoiceHash,
					DefaultHash:   column.DefaultHash,
					ConditionHash: column.ConditionHash,
				})
			}
		}
		baseline.Fields = append(baseline.Fields, converted)
	}
	return baseline, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func ValidateOverlaySchema(overlay Overlay) error {
	if overlay.SchemaVersion != OverlaySchemaVersion {
		return fmt.Errorf("ServiceDesk overlay schemaVersion must be %d", OverlaySchemaVersion)
	}
	if !overlayNamePattern.MatchString(overlay.Name) || len(overlay.Name) > 64 {
		return errors.New("ServiceDesk overlay name must be a lowercase hyphenated slug of at most 64 characters")
	}
	if err := validateRouting(overlay.Routing); err != nil {
		return err
	}
	if overlay.Target.BusinessID <= 0 || overlay.Target.ServiceID <= 0 {
		return errors.New("ServiceDesk overlay target IDs must be positive")
	}
	if strings.TrimSpace(overlay.Baseline.SnapshotID) == "" || strings.TrimSpace(overlay.Baseline.ServiceKey) == "" {
		return errors.New("ServiceDesk overlay baseline snapshotId and serviceKey must be non-empty")
	}
	if !hashPattern.MatchString(overlay.Baseline.MetadataHash) {
		return errors.New("ServiceDesk overlay baseline metadataHash must be a sha256 digest")
	}
	if overlay.Fields == nil {
		return errors.New("ServiceDesk overlay fields must be an object")
	}
	if err := validateBaseline(overlay.Baseline); err != nil {
		return err
	}
	if err := validateOverlayPublicNames(overlay.Fields); err != nil {
		return err
	}
	return validateOverlayDefaults(overlay)
}

func validateRouting(routing Routing) error {
	for name, value := range map[string]string{
		"displayName":  routing.DisplayName,
		"useWhen":      routing.UseWhen,
		"doNotUseWhen": routing.DoNotUseWhen,
	} {
		if value != strings.TrimSpace(value) || value == "" || utf8.RuneCountInString(value) > 500 {
			return fmt.Errorf("ServiceDesk overlay routing.%s must be 1 to 500 trimmed characters", name)
		}
	}
	seen := make(map[string]struct{}, len(routing.Examples))
	for _, example := range routing.Examples {
		if example != strings.TrimSpace(example) || example == "" || utf8.RuneCountInString(example) > 500 {
			return errors.New("ServiceDesk overlay routing examples must be 1 to 500 trimmed characters")
		}
		if _, exists := seen[example]; exists {
			return errors.New("ServiceDesk overlay routing examples must be unique")
		}
		seen[example] = struct{}{}
	}
	return nil
}

func validateBaseline(baseline Baseline) error {
	for index, field := range baseline.Fields {
		if err := validateOptionalName(field.StableID, "baseline field stableId"); err != nil {
			return err
		}
		if err := validateRequiredText(field.Name, "baseline field name"); err != nil {
			return err
		}
		if err := validateRequiredText(field.Type, "baseline field type"); err != nil {
			return err
		}
		isTable := tableFieldType(field.Type)
		if isTable != field.columnsPresent {
			return fmt.Errorf("ServiceDesk overlay baseline field %d columns must be present only for table fields", index)
		}
		if field.ChoiceHash != "" && (!choiceFieldType(field.Type) || !hashPattern.MatchString(field.ChoiceHash)) {
			return fmt.Errorf("ServiceDesk overlay baseline field %d has invalid choiceHash", index)
		}
		if err := validateOptionalFingerprintHash(field.DefaultHash, "defaultHash", index, -1); err != nil {
			return err
		}
		if err := validateOptionalFingerprintHash(field.ConditionHash, "conditionHash", index, -1); err != nil {
			return err
		}
		for columnIndex, column := range field.Columns {
			if err := validateOptionalName(column.Key, "baseline column key"); err != nil {
				return err
			}
			if err := validateRequiredText(column.Name, "baseline column name"); err != nil {
				return err
			}
			if err := validateRequiredText(column.Type, "baseline column type"); err != nil {
				return err
			}
			if column.ChoiceHash != "" && (!choiceFieldType(column.Type) || !hashPattern.MatchString(column.ChoiceHash)) {
				return fmt.Errorf("ServiceDesk overlay baseline field %d column %d has invalid choiceHash", index, columnIndex)
			}
			if err := validateOptionalFingerprintHash(column.DefaultHash, "defaultHash", index, columnIndex); err != nil {
				return err
			}
			if err := validateOptionalFingerprintHash(column.ConditionHash, "conditionHash", index, columnIndex); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOptionalFingerprintHash(value, name string, fieldIndex, columnIndex int) error {
	if value == "" || hashPattern.MatchString(value) {
		return nil
	}
	if columnIndex >= 0 {
		return fmt.Errorf("ServiceDesk overlay baseline field %d column %d has invalid %s", fieldIndex, columnIndex, name)
	}
	return fmt.Errorf("ServiceDesk overlay baseline field %d has invalid %s", fieldIndex, name)
}

func validateOverlayPublicNames(fields map[string]OverlayField) error {
	names := make(map[string]string)
	fieldNames := sortedMapKeys(fields)
	for _, publicName := range fieldNames {
		if err := validateRequiredName(publicName, "public field name"); err != nil {
			return err
		}
		if err := addPublicName(names, publicName, "field"); err != nil {
			return err
		}
		if err := validateFieldMatcher(fields[publicName].Match); err != nil {
			return fmt.Errorf("ServiceDesk overlay field %q: %w", publicName, err)
		}
	}
	for _, publicName := range fieldNames {
		field := fields[publicName]
		for _, alias := range field.Aliases {
			if err := validateRequiredName(alias, "field alias"); err != nil {
				return err
			}
			if err := addPublicName(names, alias, "field alias"); err != nil {
				return err
			}
		}
		if field.Columns != nil {
			if err := validateOverlayColumnNames(publicName, field.Columns); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOverlayColumnNames(fieldName string, columns map[string]OverlayColumn) error {
	names := make(map[string]string)
	columnNames := sortedMapKeys(columns)
	for _, publicName := range columnNames {
		if err := validateRequiredName(publicName, "public column name"); err != nil {
			return err
		}
		if err := addPublicName(names, publicName, "column"); err != nil {
			return err
		}
		if err := validateColumnMatcher(columns[publicName].Match); err != nil {
			return fmt.Errorf("ServiceDesk overlay field %q column %q: %w", fieldName, publicName, err)
		}
	}
	for _, publicName := range columnNames {
		for _, alias := range columns[publicName].Aliases {
			if err := validateRequiredName(alias, "column alias"); err != nil {
				return err
			}
			if err := addPublicName(names, alias, "column alias"); err != nil {
				return err
			}
		}
	}
	return nil
}

func addPublicName(names map[string]string, name, kind string) error {
	if existing, ok := names[name]; ok {
		return fmt.Errorf("ServiceDesk overlay %s %q conflicts with %s", kind, name, existing)
	}
	names[name] = kind
	return nil
}

func validateFieldMatcher(matcher FieldMatcher) error {
	if matcher.StableID == "" && matcher.Name == "" {
		return errors.New("match must contain stableId or name")
	}
	if err := validateOptionalName(matcher.StableID, "matcher stableId"); err != nil {
		return err
	}
	return validateOptionalName(matcher.Name, "matcher name")
}

func validateColumnMatcher(matcher ColumnMatcher) error {
	if matcher.Key == "" && matcher.Name == "" {
		return errors.New("match must contain key or name")
	}
	if err := validateOptionalName(matcher.Key, "matcher key"); err != nil {
		return err
	}
	return validateOptionalName(matcher.Name, "matcher name")
}

func validateRequiredName(value, label string) error {
	if value != strings.TrimSpace(value) || value == "" || utf8.RuneCountInString(value) > 128 {
		return fmt.Errorf("ServiceDesk overlay %s must be 1 to 128 trimmed characters", label)
	}
	return nil
}

func validateOptionalName(value, label string) error {
	if value == "" {
		return nil
	}
	return validateRequiredName(value, label)
}

func validateRequiredText(value, label string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("ServiceDesk overlay %s must be non-empty", label)
	}
	return nil
}

func validateOverlayDefaults(overlay Overlay) error {
	for publicName, field := range overlay.Fields {
		baselineField, err := resolveBaselineField(field.Match, overlay.Baseline.Fields)
		if err != nil {
			return fmt.Errorf("ServiceDesk overlay field %q baseline match: %w", publicName, err)
		}
		if field.Columns != nil && !tableFieldType(baselineField.Type) {
			return fmt.Errorf("ServiceDesk overlay field %q defines columns for non-table type", publicName)
		}
		if field.Default != nil {
			if err := validateDefaultType(field.Default, baselineField.Type); err != nil {
				return fmt.Errorf("ServiceDesk overlay field %q default: %w", publicName, err)
			}
		}
		for columnName, column := range field.Columns {
			baselineColumn, err := resolveBaselineColumn(column.Match, baselineField.Columns)
			if err != nil {
				return fmt.Errorf("ServiceDesk overlay field %q column %q baseline match: %w", publicName, columnName, err)
			}
			if column.Default != nil {
				if err := validateDefaultType(column.Default, baselineColumn.Type); err != nil {
					return fmt.Errorf("ServiceDesk overlay field %q column %q default: %w", publicName, columnName, err)
				}
			}
		}
	}
	return nil
}

func resolveBaselineField(matcher FieldMatcher, fields []BaselineField) (BaselineField, error) {
	if matcher.StableID != "" {
		matches := make([]BaselineField, 0, 1)
		for _, field := range fields {
			if field.StableID == matcher.StableID {
				matches = append(matches, field)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return BaselineField{}, errors.New("stableId is ambiguous")
		}
	}
	if matcher.Name != "" {
		matches := make([]BaselineField, 0, 1)
		for _, field := range fields {
			if field.Name == matcher.Name {
				matches = append(matches, field)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return BaselineField{}, errors.New("name is ambiguous")
		}
	}
	return BaselineField{}, errors.New("matcher did not resolve")
}

func resolveBaselineColumn(matcher ColumnMatcher, columns []BaselineColumn) (BaselineColumn, error) {
	if matcher.Key != "" {
		matches := make([]BaselineColumn, 0, 1)
		for _, column := range columns {
			if column.Key == matcher.Key {
				matches = append(matches, column)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return BaselineColumn{}, errors.New("key is ambiguous")
		}
	}
	if matcher.Name != "" {
		matches := make([]BaselineColumn, 0, 1)
		for _, column := range columns {
			if column.Name == matcher.Name {
				matches = append(matches, column)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return BaselineColumn{}, errors.New("name is ambiguous")
		}
	}
	return BaselineColumn{}, errors.New("matcher did not resolve")
}

func validateDefaultType(raw json.RawMessage, fieldType string) error {
	var value any
	if err := decodeStrictJSON(raw, &value); err != nil {
		return err
	}
	if value == nil {
		return errors.New("must not be null")
	}
	switch strings.ToLower(fieldType) {
	case "input", "multi_input", "textarea", "date", "time", "only_time", "select", "single_select", "radio", "checkbox":
		if _, ok := value.(string); !ok {
			return errors.New("must be a string")
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return errors.New("must be a JSON number")
		}
	case "date_time_range", "time_range":
		values, ok := value.([]any)
		if !ok || len(values) != 2 || !allStrings(values) {
			return errors.New("must be an array of exactly two strings")
		}
	case "multi_select", "multi_checkbox":
		values, ok := value.([]any)
		if !ok || !allStrings(values) {
			return errors.New("must be a string array")
		}
	case "switch":
		if _, ok := value.(bool); !ok {
			return errors.New("must be a boolean")
		}
	case "basis_table", "extend_table":
		rows, ok := value.([]any)
		if !ok {
			return errors.New("must be an object array")
		}
		for _, row := range rows {
			if _, ok := row.(map[string]any); !ok {
				return errors.New("must be an object array")
			}
		}
	case "tui_editor":
		if _, ok := value.(string); ok {
			return nil
		}
		object, ok := value.(map[string]any)
		if !ok || len(object) != 1 {
			return errors.New("must be a string or an object containing only html")
		}
		if html, ok := object["html"].(string); !ok || html == "" {
			return errors.New("must be a string or an object containing only html")
		}
	case "hyperlink":
		return errors.New("display-only fields cannot have overlay defaults")
	default:
		return fmt.Errorf("unsupported field type %q cannot have an overlay default", fieldType)
	}
	return nil
}

func allStrings(values []any) bool {
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func tableFieldType(fieldType string) bool {
	return strings.EqualFold(fieldType, "basis_table") || strings.EqualFold(fieldType, "extend_table")
}

func choiceFieldType(fieldType string) bool {
	switch strings.ToLower(fieldType) {
	case "select", "single_select", "radio", "checkbox", "multi_select", "multi_checkbox":
		return true
	default:
		return false
	}
}

func DiscoverOverlay(template Template, name string) Overlay {
	overlay := Overlay{
		SchemaVersion: OverlaySchemaVersion,
		Name:          name,
		Description:   "",
		Routing:       Routing{Examples: []string{}},
		Target: OverlayTarget{
			BusinessID: template.Target.BusinessID,
			ServiceID:  template.Target.ServiceID,
		},
		Baseline: Baseline{
			SnapshotID:   template.SnapshotID,
			ServiceKey:   template.ServiceKey,
			MetadataHash: template.MetadataHash,
			Fields:       make([]BaselineField, 0, len(template.Fields)),
		},
		Fields: make(map[string]OverlayField, len(template.Fields)),
	}
	usedFieldNames := make(map[string]struct{}, len(template.Fields))
	for index, field := range template.Fields {
		baseline := BaselineField{
			StableID:      field.StableID,
			Name:          field.Name,
			Type:          field.Type,
			Required:      field.Required,
			ChoiceHash:    field.ChoiceHash,
			DefaultHash:   optionalCanonicalHash(field.DefaultValue, field.DefaultAvailable),
			ConditionHash: optionalConditionHash(field.Conditions),
		}
		if tableFieldType(field.Type) {
			baseline.columnsPresent = true
			baseline.Columns = make([]BaselineColumn, 0, len(field.TableColumns))
			for _, column := range field.TableColumns {
				baseline.Columns = append(baseline.Columns, BaselineColumn{
					Key:           column.BackendKey,
					Name:          column.Name,
					Type:          column.Type,
					Required:      column.Required,
					ChoiceHash:    column.ChoiceHash,
					DefaultHash:   optionalCanonicalHash(column.DefaultValue, column.DefaultAvailable),
					ConditionHash: optionalConditionHash(column.Conditions),
				})
			}
		}
		overlay.Baseline.Fields = append(overlay.Baseline.Fields, baseline)

		publicName := uniqueTechnicalName(preferredFieldName(field), "field", index, usedFieldNames)
		overlayField := OverlayField{
			Match: FieldMatcher{StableID: field.StableID, Name: field.Name},
		}
		if len(field.TableColumns) > 0 {
			overlayField.Columns = make(map[string]OverlayColumn, len(field.TableColumns))
			usedColumnNames := make(map[string]struct{}, len(field.TableColumns))
			for columnIndex, column := range field.TableColumns {
				columnName := uniqueTechnicalName(column.Name, "column", columnIndex, usedColumnNames)
				overlayField.Columns[columnName] = OverlayColumn{
					Match: ColumnMatcher{Key: column.BackendKey, Name: column.Name},
				}
			}
		}
		overlay.Fields[publicName] = overlayField
	}
	return overlay
}

func preferredFieldName(field Field) string {
	if field.StableID != "" {
		return field.StableID
	}
	return field.Name
}

func uniqueTechnicalName(preferred, kind string, index int, used map[string]struct{}) string {
	if validateRequiredName(preferred, kind) == nil {
		if _, exists := used[preferred]; !exists {
			used[preferred] = struct{}{}
			return preferred
		}
		for suffix := 2; ; suffix++ {
			candidate := fmt.Sprintf("%s-%d", preferred, suffix)
			if validateRequiredName(candidate, kind) != nil {
				break
			}
			if _, duplicate := used[candidate]; !duplicate {
				used[candidate] = struct{}{}
				return candidate
			}
		}
	}
	fallback := fmt.Sprintf("%s-%d", kind, index+1)
	for suffix := 1; ; suffix++ {
		candidate := fallback
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", fallback, suffix)
		}
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func optionalCanonicalHash(value any, available bool) string {
	if !available {
		return ""
	}
	hash, _ := canonicalHash(value)
	return hash
}

func optionalConditionHash(conditions ConditionSet) string {
	if conditions.Source == "" && conditions.Operator == "" && len(conditions.Conditions) == 0 {
		return ""
	}
	hash, _ := canonicalHash(conditions)
	return hash
}

func ValidateOverlay(template Template, overlay Overlay) (ResolvedOverlay, error) {
	return ResolveOverlay(overlay, template)
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
