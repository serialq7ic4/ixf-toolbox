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
	expectedTitle       string
}

type CreateVerification struct {
	StateReadable bool `json:"stateReadable"`
}

type CreateResult struct {
	Plan      CreatePlan          `json:"plan"`
	Applied   bool                `json:"applied"`
	Ticket    *TicketResponse     `json:"ticket,omitempty"`
	DetailURL string              `json:"detailUrl,omitempty"`
	Verify    *CreateVerification `json:"verify,omitempty"`
	Outcome   string              `json:"outcome"`
}

type TicketReference struct {
	InstanceID       string `json:"instanceId,omitempty"`
	Name             string `json:"name,omitempty"`
	Title            string `json:"title,omitempty"`
	ServiceID        int    `json:"serviceId,omitempty"`
	ServiceName      string `json:"serviceName,omitempty"`
	DetailURL        string `json:"detailUrl,omitempty"`
	RequestStartedAt string `json:"requestStartedAt,omitempty"`
}

type Dependencies struct {
	Client *Client
	Now    func() time.Time
}

type DomainError struct {
	Code      string           `json:"code"`
	Message   string           `json:"message"`
	Action    string           `json:"action,omitempty"`
	Field     string           `json:"field,omitempty"`
	Column    string           `json:"column,omitempty"`
	RowIndex  *int             `json:"rowIndex,omitempty"`
	Ticket    *TicketReference `json:"ticket,omitempty"`
	Retryable bool             `json:"retryable"`
	Outcome   string           `json:"outcome,omitempty"`
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
	if request.Apply && request.Target != nil {
		result.Plan.Apply = ApplyPlan{Blocked: true, BlockReason: "profile_required"}
		return result, &DomainError{Code: "profile_required", Message: "Creating a ServiceDesk ticket requires a built-in profile", Action: actionForCode("profile_required")}
	}
	if request.Apply {
		preflightRequest := request
		preflightRequest.Apply = false
		first, firstErr := Create(ctx, deps, preflightRequest)
		if firstErr != nil {
			return first, firstErr
		}
		second, secondErr := Create(ctx, deps, preflightRequest)
		if secondErr != nil {
			return second, secondErr
		}
		if second.Plan.Apply.Blocked {
			return second, &DomainError{Code: second.Plan.Apply.BlockReason, Message: "ServiceDesk create apply is blocked by preflight", Action: actionForCode(second.Plan.Apply.BlockReason)}
		}
		return applyCreate(ctx, deps, second)
	}
	var profileSource string
	var target EditTarget
	var template Template
	input := request.Input
	var inputIssues []FieldIssue
	if request.Target != nil {
		target = *request.Target
		var err error
		template, err = LoadTemplate(ctx, deps.Client, target)
		if err != nil {
			return result, domainFromAPIError(err)
		}
	} else {
		profile, profileErr := findBuiltinProfile(request.Profile)
		if profileErr != nil {
			return result, profileErr
		}
		profileSource = builtinProfileSource
		target = EditTarget(profile.Target)
		var fixed map[string]string
		var err error
		template, fixed, err = profile.load(ctx, deps.Client)
		if err != nil {
			if de, ok := err.(*DomainError); ok && de.Code == "destructive_drift" {
				result.Plan.TemplateDrift = true
				result.Plan.DriftClassification = driftDestructive
				result.Plan.Apply = ApplyPlan{Blocked: true, BlockReason: de.Code, ProfileSource: profileSource}
			}
			return result, ClassifyError("inspect", err)
		}
		input, inputIssues = prepareBuiltinInput(template, fixed, request.Input)
	}
	normalized, fieldIssues := NormalizeInput(template, nil, input)
	conditions := EvaluateConditions(template, normalized)
	issues := append(inputIssues, fieldIssues...)
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
	privateInputs := cloneInputs(normalized.Inputs)
	privateInputs["retry"] = "true"
	privateInputs["_loop"] = json.Number("0")
	plan.privatePayload = StartPayload{SnapshotID: template.SnapshotID, ServiceID: target.ServiceID, Key: template.ServiceKey, Inputs: privateInputs}
	plan.expectedTitle = expectedTitle(template, normalized)
	if template.ServiceKey != "request" {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = "unsupported_service_key"
	}
	if request.Target != nil && !plan.Apply.Blocked && len(issues) == 0 {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = "profile_required"
	}
	if len(issues) > 0 && !plan.Apply.Blocked {
		plan.Apply.Blocked = true
		plan.Apply.BlockReason = issues[0].Code
	}
	result.Plan = plan
	if len(issues) > 0 && !request.Apply {
		return result, domainFromFieldIssue(issues[0])
	}
	return result, nil
}

func applyCreate(ctx context.Context, deps Dependencies, planResult CreateResult) (CreateResult, *DomainError) {
	startedAt := time.Now()
	if deps.Now != nil {
		startedAt = deps.Now()
	}
	plan := planResult.Plan
	ref := &TicketReference{
		Title: plan.expectedTitle, ServiceID: plan.Target.ServiceID, ServiceName: plan.Target.ServiceName,
		RequestStartedAt: startedAt.UTC().Format(time.RFC3339Nano),
	}
	start, err := deps.Client.Start(ctx, plan.Target.BusinessID, plan.privatePayload)
	if err != nil {
		classified := ClassifyError("create", err)
		if classified.Code == "unknown_outcome" {
			planResult.Outcome = "unknown"
			classified.Ticket = ref
		}
		return planResult, classified
	}
	ticket, readbackErr := readCreatedTicket(ctx, deps.Client, start, plan)
	if readbackErr != nil {
		ref.InstanceID = start.InstanceID
		ref.Name = start.Name
		planResult.Outcome = "created_unverified"
		planResult.Ticket = &TicketResponse{
			InstanceID: ref.InstanceID, Name: ref.Name, Title: ref.Title,
			ServiceID: ref.ServiceID, ServiceName: ref.ServiceName,
		}
		readbackErr.Action = "check the ticket in your submitted tickets before retrying"
		readbackErr.Ticket = ref
		readbackErr.Outcome = planResult.Outcome
		return planResult, readbackErr
	}
	planResult.DetailURL = serviceDeskDetailURL(plan.Target.BusinessID, ticket.detailRecordID())
	if ticket.ResourceURI == "" {
		ticket.ResourceURI = planResult.DetailURL
	}
	planResult.Ticket = &ticket
	planResult.Applied = true
	planResult.Outcome = "applied"
	planResult.Verify = &CreateVerification{StateReadable: true}
	return planResult, nil
}

func readCreatedTicket(ctx context.Context, client *Client, start StartResponse, plan CreatePlan) (TicketResponse, *DomainError) {
	match := func(ticket TicketResponse) bool {
		if _, err := parsePositiveID(ticket.InstanceID); err != nil || ticket.BusinessID <= 0 ||
			ticket.ServiceID != plan.Target.ServiceID || strings.TrimSpace(ticket.State) == "" {
			return false
		}
		if plan.expectedTitle != "" && plan.expectedTitle != ticket.Title {
			return false
		}
		if start.Name != "" && ticket.Name != "" && start.Name != ticket.Name {
			return false
		}
		idMatches := ticket.InstanceID == start.InstanceID
		if start.detailID != "" && ticket.DetailID != "" {
			idMatches = start.detailID == ticket.DetailID
		}
		return idMatches ||
			(plan.expectedTitle != "" && start.Name != "" && start.Name == ticket.Name)
	}
	businessID := start.BusinessID
	if businessID == 0 {
		businessID = plan.Target.BusinessID
	}
	ticket, detailErr := client.getTicketDetail(ctx, DetailTarget{BusinessID: businessID, TicketID: start.InstanceID})
	if detailErr == nil && match(ticket) {
		return ticket, nil
	}
	if plan.expectedTitle != "" {
		// The submitted view can include tickets whose business differs from the form's.
		ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if user, err := client.UserInfo(ctx, 0); err == nil {
			scopes := make([]queryScope, 0, len(user.Business))
			for _, business := range user.Business {
				if business.BusinessID > 0 {
					scopes = append(scopes, queryScope{Business: business, Role: QueryRoleSubmitted, FlowType: "apply"})
				}
			}
			var found TicketResponse
			for _, outcome := range queryServiceDeskScopes(ctx, client, scopes, match) {
				for _, candidate := range outcome.Tickets {
					if found.BusinessID != 0 && (found.BusinessID != candidate.BusinessID || found.InstanceID != candidate.InstanceID) {
						return TicketResponse{}, &DomainError{Code: "readback_mismatch", Message: "ServiceDesk returned multiple matching tickets"}
					}
					if found.BusinessID == 0 || parseTicketTime(candidate.UpdateTime).After(parseTicketTime(found.UpdateTime)) {
						found = candidate
					}
				}
			}
			if found.BusinessID > 0 {
				return found, nil
			}
		}
	}
	if detailErr == nil {
		return TicketResponse{}, &DomainError{Code: "readback_mismatch", Message: "ServiceDesk readback did not match the created ticket"}
	}
	return TicketResponse{}, &DomainError{Code: "readback_failed", Message: "ServiceDesk ticket was created but readback failed"}
}

func expectedTitle(template Template, normalized NormalizedInput) string {
	for _, field := range template.Fields {
		if strings.EqualFold(field.StableID, "title") || strings.EqualFold(field.Name, "title") {
			if value, ok := normalized.Inputs[field.BackendKey].(string); ok {
				return value
			}
		}
	}
	return ""
}

func serviceDeskDetailURL(businessID int, ticketID string) string {
	if _, err := parsePositiveID(ticketID); err != nil {
		return ""
	}
	return fmt.Sprintf("%s/#/order/order-detail/%d?id=%s", serviceDeskOrigin, businessID, ticketID)
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
	return ClassifyError("read", err)
}
