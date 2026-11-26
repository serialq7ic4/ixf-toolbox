package servicedesk

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	sdp "github.com/serialq7ic4/ixf-toolbox/pkg/servicedeskprofile"
)

const (
	builtinProfileSource = "builtin"
	serverRepairBranch   = "FUWUQIBAOXIU20250704162135675"
)

type BuiltinProfileSummary struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Routing     Routing       `json:"routing"`
	Target      OverlayTarget `json:"target"`
}

type profileField struct {
	name, stableID, fieldType string
	fixed                     string
}

type builtinProfile struct {
	BuiltinProfileSummary
	fields []profileField
}

var builtinProfiles = []builtinProfile{{
	BuiltinProfileSummary: BuiltinProfileSummary{
		Name: "server-repair", Description: "分布式存储服务器硬件故障报修。",
		Routing: Routing{
			DisplayName:  "服务器报修",
			UseWhen:      "用户明确要求报修分布式存储服务器的硬件故障，例如硬盘或网卡故障、服务器宕机。",
			DoNotUseWhen: "网络设备报修、独立的网络链路或线缆检修、设备上下架以及单纯分析故障原因。",
			Examples:     []string{"这台存储服务器的硬盘坏了，帮我提单报修。"},
		},
		Target: OverlayTarget{BusinessID: 1, ServiceID: 463},
	},
	fields: []profileField{
		{name: "catalog", stableID: "FUWUMULU20250704161620433", fieldType: "single_checkbox", fixed: "GUZHANGBAOXIULEI20260416204401815"},
		{name: "branch", stableID: "GUZHANGBAOXIULEILIEBIAO20250704174956781", fieldType: "single_checkbox", fixed: serverRepairBranch},
		{name: "title", stableID: "title", fieldType: "input"},
		{name: "details", stableID: "FUWUQIBAOXIUXUQIUMINGXI20250704164905466", fieldType: "tui_editor"},
		{name: "businessUnit", stableID: "SUOSHUBGBU20250625154051508", fieldType: "single_select"},
		{name: "businessLine", stableID: "SUOSHUYEWUXIAN20250625154131108", fieldType: "script_field"},
		{name: "site", stableID: "MUBIAOJIFANG20250702092613639", fieldType: "single_select"},
		{name: "deviceUser", stableID: "SHEBEISHIYONGREN20250625154344112", fieldType: "single_personnel"},
		{name: "deviceCount", stableID: "SHEBEISHULIANG20250702092713813", fieldType: "number"},
		{name: "expectedCompletion", stableID: "QIWANGWANCHENGSHIJIAN20250702092603252", fieldType: "time"},
	},
}}

func BuiltinProfiles() []BuiltinProfileSummary {
	profiles := make([]BuiltinProfileSummary, 0, len(builtinProfiles))
	for _, profile := range builtinProfiles {
		profiles = append(profiles, profile.summary())
	}
	return profiles
}

func BuiltinProfile(name string) (BuiltinProfileSummary, *DomainError) {
	profile, err := findBuiltinProfile(name)
	if err != nil {
		return BuiltinProfileSummary{}, err
	}
	return profile.summary(), nil
}

func (p builtinProfile) summary() BuiltinProfileSummary {
	summary := p.BuiltinProfileSummary
	summary.Routing.Examples = slices.Clone(summary.Routing.Examples)
	return summary
}

func findBuiltinProfile(name string) (builtinProfile, *DomainError) {
	for _, profile := range builtinProfiles {
		if name == profile.Name {
			return profile, nil
		}
	}
	return builtinProfile{}, &DomainError{
		Code: "profile_not_found", Message: "ServiceDesk profile is not built into this version",
		Action: "run ixf servicedesk profile list --json",
	}
}

func InspectBuiltinProfile(ctx context.Context, client *Client, name string) (ResolvedContract, error) {
	profile, profileErr := findBuiltinProfile(name)
	if profileErr != nil {
		return ResolvedContract{}, profileErr
	}
	template, fixed, err := profile.load(ctx, client)
	if err != nil {
		return ResolvedContract{}, err
	}
	template.Fields = slices.DeleteFunc(template.Fields, func(field Field) bool { return fixed[field.StableID] != "" })
	return sdp.InspectTemplate(template, nil)
}

func (p builtinProfile) load(ctx context.Context, client *Client) (Template, map[string]string, error) {
	live, err := LoadTemplate(ctx, client, EditTarget(p.Target))
	if err != nil {
		return Template{}, nil, err
	}
	projected, fixed, drift := p.project(live)
	if drift != nil {
		return Template{}, nil, drift
	}
	for index := range projected.Fields {
		field := &projected.Fields[index]
		if field.ChoiceDictionary == 0 {
			continue
		}
		if field.ChoiceDictionary < 0 {
			return Template{}, nil, repairFormDrift(field.StableID)
		}
		field.Choices, err = client.dictionaryChoices(ctx, p.Target.BusinessID, field.ChoiceDictionary)
		if err != nil {
			return Template{}, nil, err
		}
	}
	return projected, fixed, nil
}

func (p builtinProfile) project(live Template) (Template, map[string]string, *DomainError) {
	projected := live
	projected.Fields = nil
	fixed := make(map[string]string)
	selected := make(map[string]Field)
	fields := make(map[string]Field, len(live.Fields))
	for _, field := range live.Fields {
		fields[field.BackendKey] = field
	}
	var branchKey string
	for _, spec := range p.fields {
		index := slices.IndexFunc(live.Fields, func(field Field) bool { return field.StableID == spec.stableID })
		if index < 0 || slices.ContainsFunc(live.Fields[index+1:], func(field Field) bool { return field.StableID == spec.stableID }) {
			return Template{}, nil, repairFormDrift(spec.name)
		}
		field := live.Fields[index]
		if field.Type != spec.fieldType || !field.Supported || (field.Type == "script_field" && field.ScriptMultiple != "single") {
			return Template{}, nil, repairFormDrift(spec.name)
		}
		if spec.name == "branch" {
			branchKey = field.BackendKey
		}
		if spec.name == "details" {
			if !matchesSingleChoice(field.Conditions, branchKey, serverRepairBranch) {
				return Template{}, nil, repairFormDrift(spec.name)
			}
		} else if hasConditions(field.Conditions) {
			return Template{}, nil, repairFormDrift(spec.name)
		}
		if spec.fixed != "" {
			if choice, code := normalizeChoice(spec.fixed, field.Choices); code != "" || choice != spec.fixed {
				return Template{}, nil, repairFormDrift(spec.name)
			}
			fixed[spec.name] = spec.fixed
		}
		selected[field.BackendKey] = field
		field.StableID, field.Required = spec.name, true
		field.DefaultValue, field.DefaultAvailable = nil, false
		if spec.fixed != "" {
			field.DefaultValue, field.DefaultAvailable = spec.fixed, true
		}
		field.Conditions = ConditionSet{}
		if field.Type == "script_field" {
			// The live choice entries are script dependencies, not selectable results.
			field.Choices = nil
		}
		projected.Fields = append(projected.Fields, field)
	}
	values := make(map[string]string, len(fixed))
	for _, field := range projected.Fields {
		if value, ok := fixed[field.StableID]; ok {
			values[field.BackendKey] = value
		}
	}
	for _, field := range live.Fields {
		if _, ok := selected[field.BackendKey]; !ok && field.Required && !field.DisplayOnly &&
			!inactiveUnderFixedChoices(field, fields, values, make(map[string]bool)) {
			return Template{}, nil, repairFormDrift(field.Name)
		}
	}
	return projected, fixed, nil
}

func hasConditions(set ConditionSet) bool { return set.Unsupported || len(set.Conditions) != 0 }

func matchesSingleChoice(set ConditionSet, field, value string) bool {
	condition, ok := singleChoiceCondition(set)
	return ok && condition.Field == field && condition.Value == value
}

func singleChoiceCondition(set ConditionSet) (Condition, bool) {
	if set.Unsupported || len(set.Conditions) != 1 || set.Operator != "and" {
		return Condition{}, false
	}
	condition := set.Conditions[0]
	for len(condition.Conditions) != 0 {
		if condition.Operator != "and" || len(condition.Conditions) != 1 {
			return Condition{}, false
		}
		condition = condition.Conditions[0]
	}
	_, scalar := condition.Value.(string)
	return condition, scalar && condition.Operator == "==" && condition.Field != ""
}

// Only omit a required field when a fixed choice (possibly through an inactive
// parent branch) proves that the field is hidden.
func inactiveUnderFixedChoices(field Field, fields map[string]Field, values map[string]string, seen map[string]bool) bool {
	condition, ok := singleChoiceCondition(field.Conditions)
	if !ok || seen[field.BackendKey] {
		return false
	}
	if value, fixed := values[condition.Field]; fixed {
		return value != condition.Value
	}
	parent, exists := fields[condition.Field]
	seen[field.BackendKey] = true
	return exists && inactiveUnderFixedChoices(parent, fields, values, seen)
}

func repairFormDrift(field string) *DomainError {
	return &DomainError{
		Code: "destructive_drift", Field: field, Message: "Live ServiceDesk form is incompatible with the built-in profile",
		Action: "review the live form and update the profile in ixf-toolbox",
	}
}

func prepareBuiltinInput(template Template, fixed map[string]string, provided TicketInput) (TicketInput, []FieldIssue) {
	input := make(TicketInput, len(provided))
	allowed := make(map[string]Field, len(template.Fields))
	for _, field := range template.Fields {
		allowed[field.StableID] = field
	}
	var issues []FieldIssue
	for _, name := range slices.Sorted(maps.Keys(provided)) {
		field, ok := allowed[name]
		if !ok || fixed[name] != "" {
			issues = append(issues, FieldIssue{Code: "unknown_field", Field: name, Message: "field is not an input of the built-in profile"})
			continue
		}
		value := provided[name]
		switch field.Type {
		case "input", "tui_editor":
			if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
				issues = append(issues, FieldIssue{Code: "invalid_type", Field: name, Message: "field requires non-empty plain text"})
			}
		case "number":
			number, _ := value.(json.Number)
			if count, err := strconv.ParseInt(string(number), 10, 64); err != nil || count <= 0 {
				issues = append(issues, FieldIssue{Code: "invalid_type", Field: name, Message: "field requires a positive integer"})
			}
		}
		input[name] = value
	}
	for _, field := range template.Fields {
		if _, supplied := input[field.StableID]; !supplied && autoResolvedFieldType(field.Type) {
			issues = append(issues, FieldIssue{Code: "missing_required", Field: field.StableID, Message: "required field is missing"})
		}
	}
	return input, issues
}
