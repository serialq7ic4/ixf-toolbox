package servicedesk

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const maxConcurrentServiceDeskScopes = 4

type QueryRole string

const (
	QueryRoleSubmitted QueryRole = "submitted"
	QueryRoleTodo      QueryRole = "todo"
	QueryRoleAny       QueryRole = "any"
)

type QueryStateFilter string

const (
	QueryStateAll  QueryStateFilter = "all"
	QueryStateOpen QueryStateFilter = "open"
)

type QueryRequest struct {
	BootstrapBusinessID int
	Mine                bool
	Role                QueryRole
	State               QueryStateFilter
	Query               string
	Limit               int
}

// ListRequest keeps the unreleased CLI buildable until its parser migrates to QueryRequest.
type ListRequest struct {
	BusinessID int
	Mine       bool
	View       string
	Limit      int
	Cursor     string
}

type ScopeResult struct {
	BusinessID  int    `json:"businessId"`
	Name        string `json:"name,omitempty"`
	FlowType    string `json:"flowType"`
	TicketCount int    `json:"ticketCount"`
	Complete    bool   `json:"complete"`
	Error       string `json:"error,omitempty"`
}

type FailedScope struct {
	BusinessID int    `json:"businessId"`
	FlowType   string `json:"flowType"`
	Error      string `json:"error"`
}

type QueryResult struct {
	ContractVersion     int              `json:"contractVersion"`
	Role                QueryRole        `json:"role"`
	StateFilter         QueryStateFilter `json:"stateFilter"`
	Query               string           `json:"query,omitempty"`
	BootstrapBusinessID int              `json:"bootstrapBusinessId"`
	Tickets             []Ticket         `json:"tickets"`
	Scopes              []ScopeResult    `json:"scopes"`
	FailedScopes        []FailedScope    `json:"failedScopes"`
	Sort                string           `json:"sort"`
	HasMore             *bool            `json:"hasMore"`
	Complete            bool             `json:"complete"`
}

type queryScope struct {
	Business Business
	Role     QueryRole
	FlowType string
}

type queryScopeOutcome struct {
	Scope   ScopeResult
	Tickets []Ticket
}

func QueryTickets(ctx context.Context, client *Client, request QueryRequest) (QueryResult, *DomainError) {
	result := QueryResult{
		ContractVersion:     ContractVersion,
		Role:                request.Role,
		StateFilter:         request.State,
		Query:               request.Query,
		BootstrapBusinessID: request.BootstrapBusinessID,
		Tickets:             []Ticket{},
		Scopes:              []ScopeResult{},
		FailedScopes:        []FailedScope{},
		Sort:                "update_time_desc",
		Complete:            true,
	}
	if client == nil || request.BootstrapBusinessID <= 0 {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query requires a positive bootstrap business ID", Action: "configure or provide a bootstrap business ID"}
	}
	if !request.Mine {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query only supports --mine", Action: "use the mine scope"}
	}
	if request.Role != QueryRoleSubmitted && request.Role != QueryRoleTodo {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query role must be submitted or todo", Action: "choose a supported role"}
	}
	if request.State != QueryStateAll && request.State != QueryStateOpen {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query state must be all or open", Action: "choose a supported state filter"}
	}
	if request.Role == QueryRoleTodo && request.State != QueryStateAll {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk todo queries do not accept a state filter", Action: "remove the state filter"}
	}
	if strings.TrimSpace(request.Query) != "" {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk contextual search is not available in this query mode", Action: "use an empty list query"}
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	userinfo, err := client.UserInfo(ctx, request.BootstrapBusinessID)
	if err != nil {
		return result, ClassifyError("list", err)
	}
	flowType := "apply"
	if request.Role == QueryRoleTodo {
		flowType = "wait"
	}
	scopes := make([]queryScope, 0, len(userinfo.Business))
	for _, business := range userinfo.Business {
		scopes = append(scopes, queryScope{Business: business, Role: request.Role, FlowType: flowType})
	}
	outcomes := queryServiceDeskScopes(ctx, client, scopes)
	all := make(map[string]Ticket)
	for _, outcome := range outcomes {
		result.Scopes = append(result.Scopes, outcome.Scope)
		if !outcome.Scope.Complete {
			result.Complete = false
			result.FailedScopes = append(result.FailedScopes, FailedScope{BusinessID: outcome.Scope.BusinessID, FlowType: outcome.Scope.FlowType, Error: outcome.Scope.Error})
		}
		for _, ticket := range outcome.Tickets {
			if request.Role == QueryRoleSubmitted && request.State == QueryStateOpen && ticket.StateCategory == "terminal" {
				continue
			}
			if ticket.InstanceID == "" || ticket.BusinessID <= 0 {
				continue
			}
			ticket.RoleMatches = []string{string(request.Role)}
			key := fmt.Sprintf("%d:%s", ticket.BusinessID, ticket.InstanceID)
			if _, exists := all[key]; !exists {
				all[key] = ticket
			}
		}
	}
	tickets := make([]Ticket, 0, len(all))
	for _, ticket := range all {
		tickets = append(tickets, ticket)
	}
	sort.SliceStable(tickets, func(left, right int) bool {
		leftUpdate := parseTicketTime(tickets[left].UpdateTime)
		rightUpdate := parseTicketTime(tickets[right].UpdateTime)
		if !leftUpdate.Equal(rightUpdate) {
			return leftUpdate.After(rightUpdate)
		}
		leftCreated := parseTicketTime(tickets[left].CreatedTime)
		rightCreated := parseTicketTime(tickets[right].CreatedTime)
		if !leftCreated.Equal(rightCreated) {
			return leftCreated.After(rightCreated)
		}
		if tickets[left].BusinessID != tickets[right].BusinessID {
			return tickets[left].BusinessID < tickets[right].BusinessID
		}
		return tickets[left].InstanceID < tickets[right].InstanceID
	})
	if result.Complete {
		hasMore := len(tickets) > limit
		result.HasMore = &hasMore
	}
	if len(tickets) > limit {
		tickets = tickets[:limit]
	}
	result.Tickets = tickets
	return result, nil
}

func ListTickets(ctx context.Context, client *Client, request ListRequest) (QueryResult, *DomainError) {
	state := QueryStateAll
	switch strings.ToLower(strings.TrimSpace(request.View)) {
	case "", "recent":
	case "open":
		state = QueryStateOpen
	default:
		return QueryResult{}, &DomainError{Code: "invalid_request", Message: "ServiceDesk list view must be recent or open", Action: "choose a supported view"}
	}
	return QueryTickets(ctx, client, QueryRequest{
		BootstrapBusinessID: request.BusinessID,
		Mine:                request.Mine,
		Role:                QueryRoleSubmitted,
		State:               state,
		Limit:               request.Limit,
	})
}

func queryServiceDeskScopes(ctx context.Context, client *Client, scopes []queryScope) []queryScopeOutcome {
	outcomes := make([]queryScopeOutcome, len(scopes))
	if len(scopes) == 0 {
		return outcomes
	}
	workerCount := maxConcurrentServiceDeskScopes
	if len(scopes) < workerCount {
		workerCount = len(scopes)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				outcomes[index] = queryServiceDeskScope(ctx, client, scopes[index])
			}
		}()
	}
	for index := range scopes {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return outcomes
}

func queryServiceDeskScope(ctx context.Context, client *Client, scope queryScope) queryScopeOutcome {
	outcome := queryScopeOutcome{Scope: ScopeResult{BusinessID: scope.Business.BusinessID, Name: scope.Business.Name, FlowType: scope.FlowType, Complete: true}}
	offset := 0
	for {
		page, err := client.List(ctx, scope.Business.BusinessID, ListQuery{FlowType: scope.FlowType, Limit: 100, Offset: offset})
		if err != nil {
			outcome.Scope.Complete = false
			outcome.Scope.Error = ClassifyError("list", err).Code
			return outcome
		}
		for _, raw := range page.Data {
			outcome.Tickets = append(outcome.Tickets, normalizeTicket(raw))
		}
		outcome.Scope.TicketCount += len(page.Data)
		if page.Meta.Next == nil || strings.TrimSpace(*page.Meta.Next) == "" {
			return outcome
		}
		next := page.Meta.Offset + page.Meta.Limit
		if next <= offset {
			next = offset + len(page.Data)
		}
		if next <= offset {
			outcome.Scope.Complete = false
			outcome.Scope.Error = "invalid_response"
			return outcome
		}
		offset = next
	}
}
