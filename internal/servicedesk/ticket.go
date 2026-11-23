package servicedesk

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

type Ticket struct {
	BusinessID        int      `json:"businessId"`
	InstanceID        string   `json:"id"`
	Name              string   `json:"name"`
	Title             string   `json:"title"`
	ServiceID         int      `json:"serviceId"`
	ServiceName       string   `json:"serviceName"`
	State             string   `json:"state"`
	StateCategory     string   `json:"stateCategory"`
	Step              string   `json:"step"`
	Creator           string   `json:"creator"`
	CurrentProcessors []any    `json:"currentProcessors,omitempty"`
	CreatedTime       string   `json:"createdTime"`
	UpdateTime        string   `json:"updateTime"`
	FinishTime        any      `json:"finishTime,omitempty"`
	DetailURL         string   `json:"detailUrl"`
	RoleMatches       []string `json:"roleMatches,omitempty"`
}

func CategorizeState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "RUNNING", "PAUSE":
		return "active"
	case "WITHDRAWN", "FINISHED", "REVOKE":
		return "terminal"
	default:
		return "unknown"
	}
}

func GetTicket(ctx context.Context, client *Client, target DetailTarget) (Ticket, *DomainError) {
	if client == nil {
		return Ticket{}, &DomainError{Code: "invalid_request", Message: "ServiceDesk client is required"}
	}
	raw, err := client.Get(ctx, target)
	if err != nil {
		return Ticket{}, ClassifyError("get", err)
	}
	return normalizeTicket(raw), nil
}

type WithdrawRequest struct {
	Target DetailTarget
	Reason string
	Apply  bool
}

type WithdrawVerification struct {
	StateWithdrawn bool `json:"stateWithdrawn"`
}

type WithdrawResult struct {
	ContractVersion int                  `json:"contractVersion"`
	DryRun          bool                 `json:"dryRun"`
	Applied         bool                 `json:"applied"`
	Ticket          Ticket               `json:"ticket"`
	BeforeState     string               `json:"beforeState"`
	AfterState      string               `json:"afterState,omitempty"`
	Verify          WithdrawVerification `json:"verify"`
}

func WithdrawTicket(ctx context.Context, client *Client, request WithdrawRequest) (WithdrawResult, *DomainError) {
	result := WithdrawResult{ContractVersion: ContractVersion, DryRun: !request.Apply}
	reason := strings.TrimSpace(request.Reason)
	if client == nil {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk client is required"}
	}
	if request.Target.BusinessID <= 0 || strings.TrimSpace(request.Target.TicketID) == "" {
		return result, &DomainError{Code: "invalid_request", Message: "withdraw requires a business ID and ticket ID", Action: "provide an explicit ticket target"}
	}
	if utf8.RuneCountInString(reason) < 1 || utf8.RuneCountInString(reason) > 500 {
		return result, &DomainError{Code: "invalid_request", Message: "withdraw reason must be 1 to 500 Unicode characters", Action: "adjust the reason and retry"}
	}
	userinfo, err := client.UserInfo(ctx, request.Target.BusinessID)
	if err != nil {
		return result, ClassifyError("withdraw", err)
	}
	raw, err := client.Get(ctx, request.Target)
	if err != nil {
		return result, ClassifyError("withdraw", err)
	}
	candidate := normalizeTicket(raw)
	if err := validateWithdrawCandidate(userinfo.Username, candidate); err != nil {
		return result, err
	}
	result.Ticket, result.BeforeState = candidate, candidate.State
	if !request.Apply {
		return result, nil
	}

	// Re-read identity and state immediately before the write.
	userinfo, err = client.UserInfo(ctx, request.Target.BusinessID)
	if err != nil {
		return result, ClassifyError("withdraw", err)
	}
	raw, err = client.Get(ctx, request.Target)
	if err != nil {
		return result, ClassifyError("withdraw", err)
	}
	candidate = normalizeTicket(raw)
	if err := validateWithdrawCandidate(userinfo.Username, candidate); err != nil {
		return result, err
	}
	result.Ticket, result.BeforeState = candidate, candidate.State
	if err := client.Withdraw(ctx, request.Target.BusinessID, WithdrawPayload{InstanceID: candidate.InstanceID, Message: reason}); err != nil {
		return result, ClassifyError("withdraw", err)
	}
	result.Applied = true
	verifiedRaw, err := client.Get(ctx, request.Target)
	if err != nil {
		return result, ClassifyError("withdraw", err)
	}
	verified := normalizeTicket(verifiedRaw)
	result.AfterState = verified.State
	result.Ticket = verified
	if verified.State != "WITHDRAWN" {
		return result, &DomainError{Code: "readback_mismatch", Message: "ServiceDesk ticket was not read back as WITHDRAWN", Action: "verify the ticket state before retrying"}
	}
	result.Verify.StateWithdrawn = true
	return result, nil
}

func validateWithdrawCandidate(username string, ticket Ticket) *DomainError {
	if username == "" || ticket.Creator == "" || username != ticket.Creator {
		return &DomainError{Code: "state_conflict", Message: "ServiceDesk ticket is owned by a different user", Action: "select one of your own tickets"}
	}
	if ticket.StateCategory != "active" {
		return &DomainError{Code: "state_conflict", Message: "ServiceDesk ticket is not in a withdrawable state", Action: "refresh the ticket and choose a RUNNING or PAUSE ticket"}
	}
	return nil
}

func normalizeTicket(raw TicketResponse) Ticket {
	return Ticket{BusinessID: raw.BusinessID, InstanceID: raw.InstanceID, Name: raw.Name, Title: raw.Title, ServiceID: raw.ServiceID, ServiceName: raw.ServiceName, State: raw.State, StateCategory: CategorizeState(raw.State), Step: raw.Step, Creator: raw.Creator, CurrentProcessors: raw.CurrentProcessors, CreatedTime: raw.CreatedTime, UpdateTime: raw.UpdateTime, FinishTime: raw.FinishTime, DetailURL: serviceDeskDetailURL(raw.BusinessID, raw.InstanceID)}
}

func parseTicketTime(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
