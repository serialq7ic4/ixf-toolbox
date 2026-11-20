package servicedesk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type CreateRequest struct {
	Target  *EditTarget
	Profile string
	Input   TicketInput
	Apply   bool
}

type TemplateSummary struct {
	SnapshotID  string `json:"snapshotId"`
	ServiceKey  string `json:"serviceKey"`
	ServiceName string `json:"serviceName"`
	FieldCount  int    `json:"fieldCount"`
}

type InputSummary struct {
	ProvidedFieldCount          int `json:"providedFieldCount"`
	DefaultedFieldCount         int `json:"defaultedFieldCount"`
	MissingRequiredCount        int `json:"missingRequiredCount"`
	TableRowCount               int `json:"tableRowCount"`
	ConditionalFieldCount       int `json:"conditionalFieldCount"`
	ActiveConditionalFieldCount int `json:"activeConditionalFieldCount"`
	UnsupportedConditionCount   int `json:"unsupportedConditionCount"`
	UnsupportedFieldCount       int `json:"unsupportedFieldCount"`
}

type ApplyPlan struct {
	Blocked       bool   `json:"blocked"`
	BlockReason   string `json:"blockReason,omitempty"`
	ProfileSource string `json:"profileSource,omitempty"`
}

type CreatePlan struct {
	ContractVersion     int             `json:"contractVersion"`
	Target              ResolvedTarget  `json:"target"`
	Template            TemplateSummary `json:"template"`
	Input               InputSummary    `json:"input"`
	Apply               ApplyPlan       `json:"apply"`
	ProfileSource       string          `json:"profileSource,omitempty"`
	TemplateDrift       bool            `json:"templateDrift"`
	DriftClassification string          `json:"driftClassification,omitempty"`
	DriftChanges        []DriftChange   `json:"driftChanges,omitempty"`
	Issues              []FieldIssue    `json:"issues,omitempty"`
	privatePayload      StartPayload
}

type CreateVerification struct {
	StateReadable bool `json:"stateReadable"`
}

type CreateResult struct {
	Plan    CreatePlan          `json:"plan"`
	Applied bool                `json:"applied"`
	Ticket  *TicketResponse     `json:"ticket,omitempty"`
	Verify  *CreateVerification `json:"verify,omitempty"`
	Outcome string              `json:"outcome"`
}

type Dependencies struct {
	Client   *Client
	Registry RegistryLoader
	Now      func() time.Time
}

type DomainError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Action    string `json:"action,omitempty"`
	Field     string `json:"field,omitempty"`
	Column    string `json:"column,omitempty"`
	RowIndex  *int   `json:"rowIndex,omitempty"`
	Retryable bool   `json:"retryable"`
	Outcome   string `json:"outcome,omitempty"`
}

func (e *DomainError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func Create(ctx context.Context, deps Dependencies, request CreateRequest) (CreateResult, *DomainError) {
	result := CreateResult{Outcome: "dry_run"}
	if deps.Client == nil {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk client is required"}
	}
	if request.Target == nil && strings.TrimSpace(request.Profile) == "" || request.Target != nil && strings.TrimSpace(request.Profile) != "" {
		return result, &DomainError{Code: "invalid_request", Message: "exactly one ServiceDesk target or profile is required"}
	}
	var overlay *ResolvedOverlay
	var profileSource string
	var profileStale bool
	var target EditTarget
	var template Template
	if request.Target != nil {
		target = *request.Target
		var err error
		template, err = LoadTemplate(ctx, deps.Client, target)
		if err != nil {
			return result, domainFromAPIError(err)
		}
	} else {
		profile, status, err := deps.Registry.Show(ctx, request.Profile)
		if err != nil {
			return result, &DomainError{Code: "registry_error", Message: "ServiceDesk profile could not be loaded"}
		}
		profileSource = status.Source
		profileStale = status.Stale
		target = EditTarget{BusinessID: profile.Target.BusinessID, ServiceID: profile.Target.ServiceID}
		template, err = LoadTemplate(ctx, deps.Client, target)
		if err != nil {
			return result, domainFromAPIError(err)
		}
		resolved, err := ResolveOverlay(profile, template)
		if err != nil {
			return result, &DomainError{Code: "profile_invalid", Message: "ServiceDesk profile does not match the live template"}
		}
		overlay = &resolved
		profileContract, err := resolveProfileInputContract(template.Fields, resolved)
		if err != nil {
			return result, &DomainError{Code: "profile_invalid", Message: "ServiceDesk profile could not produce a public input contract"}
		}
		projectedDrift, err := projectProfileDrift(resolved.Drift, template.Fields, profileContract)
		if err != nil {
			return result, &DomainError{Code: "profile_invalid", Message: "ServiceDesk profile drift could not be projected safely"}
		}
		overlay.Drift = projectedDrift
	}
	if overlay == nil && request.Profile != "" {
		return result, &DomainError{Code: "profile_invalid", Message: "ServiceDesk profile could not be resolved"}
	}
	if overlay != nil {
		if overlay.FieldCount() == 0 { /* keep the resolved overlay available for normalization */
		}
	}
	normalized, fieldIssues := NormalizeInput(template, overlay, request.Input)
	conditions := EvaluateConditions(template, normalized)
	issues := append([]FieldIssue{}, fieldIssues...)
	issues = append(issues, conditions.Blocking...)
	missingRequired := 0
	for _, issue := range issues {
		if issue.Code == "missing_required" || issue.Code == "required_null" || issue.Code == "missing_required_column" {
			missingRequired++
		}
	}

	plan := CreatePlan{
		ContractVersion: ContractVersion,
		Target:          ResolvedTarget{BusinessID: target.BusinessID, ServiceID: target.ServiceID, SnapshotID: template.SnapshotID, ServiceKey: template.ServiceKey, ServiceName: template.ServiceName},
		Template:        TemplateSummary{SnapshotID: template.SnapshotID, ServiceKey: template.ServiceKey, ServiceName: template.ServiceName, FieldCount: len(template.Fields)},
		Input:           InputSummary{ProvidedFieldCount: normalized.ProvidedFieldCount, DefaultedFieldCount: normalized.DefaultedFieldCount, MissingRequiredCount: missingRequired, TableRowCount: normalized.TableRowCount, ConditionalFieldCount: conditions.ConditionalFieldCount, ActiveConditionalFieldCount: conditions.ActiveConditionalFieldCount, UnsupportedConditionCount: conditions.UnsupportedConditionCount, UnsupportedFieldCount: countIssues(issues, "unsupported_field")},
		Apply:           ApplyPlan{ProfileSource: profileSource},
		ProfileSource:   profileSource,
		Issues:          append([]FieldIssue(nil), issues...),
	}
	if overlay != nil {
		plan.TemplateDrift = overlay.Drift.Changed
		plan.DriftClassification = overlay.Drift.Classification
		plan.DriftChanges = append([]DriftChange(nil), overlay.Drift.Changes...)
		if overlay.Drift.ApplyBlocked {
			plan.Apply.Blocked = true
			plan.Apply.BlockReason = "destructive_drift"
		}
	}
	privateInputs := cloneInputs(normalized.Inputs)
	privateInputs["retry"] = "true"
	privateInputs["_loop"] = json.Number("0")
	plan.privatePayload = StartPayload{SnapshotID: template.SnapshotID, ServiceID: target.ServiceID, Key: template.ServiceKey, Inputs: privateInputs}
	if template.ServiceKey != "request" {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = "unsupported_service_key"
	}
	if profileStale {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = "stale_registry"
	}
	if len(issues) > 0 && !plan.Apply.Blocked {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = issues[0].Code
	}
	if request.Apply && !plan.Apply.Blocked {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = "apply_not_available"
	}
	if request.Apply && plan.Apply.BlockReason == "" {
		plan.Apply.Blocked = true
	}
	result.Plan = plan
	if len(issues) > 0 && !request.Apply {
		return result, domainFromFieldIssue(issues[0])
	}
	if request.Apply {
		return result, &DomainError{Code: plan.Apply.BlockReason, Message: "ServiceDesk create apply is not available until the write verification stage", Action: "review the dry-run and retry after the runtime supports apply"}
	}
	return result, nil
}

func countIssues(issues []FieldIssue, code string) int {
	count := 0
	for _, issue := range issues {
		if issue.Code == code {
			count++
		}
	}
	return count
}

func domainFromFieldIssue(issue FieldIssue) *DomainError {
	code := issue.Code
	if strings.HasPrefix(code, "missing_required") || code == "required_null" {
		code = "missing_required_field"
	}
	var rowIndex *int
	if issue.Column != "" {
		value := issue.RowIndex
		rowIndex = &value
	}
	return &DomainError{Code: code, Message: issue.Message, Action: "correct the field and run dry-run again", Field: issue.Field, Column: issue.Column, RowIndex: rowIndex}
}

func cloneInputs(inputs map[string]any) map[string]any {
	clone := make(map[string]any, len(inputs))
	for key, value := range inputs {
		clone[key] = value
	}
	return clone
}

func domainFromAPIError(err error) *DomainError {
	if apiErr, ok := err.(*APIError); ok {
		return &DomainError{Code: apiErr.Code, Message: apiErr.Message, Retryable: apiErr.Retryable}
	}
	return &DomainError{Code: "service_error", Message: fmt.Sprintf("ServiceDesk request failed: %v", err)}
}

// FieldCount keeps create planning independent of the overlay's private map shape.
func (resolved ResolvedOverlay) FieldCount() int { return len(resolved.Fields) }
