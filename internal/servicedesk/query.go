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
	Tickets []queryTicketCandidate
}

type queryTicketCandidate struct {
	Ticket          Ticket
	ScopeBusinessID int
	Role            QueryRole
	FlowType        string
}

type mergedQueryTicket struct {
	Candidate queryTicketCandidate
	Roles     map[QueryRole]queryTicketCandidate
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
	if client == nil {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query requires a client", Action: "configure ServiceDesk credentials"}
	}
	if request.BootstrapBusinessID < 0 {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk bootstrap business ID must not be negative", Action: "remove or correct the business ID override"}
	}
	if !request.Mine {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query only supports --mine", Action: "use the mine scope"}
	}
	if request.Role != QueryRoleSubmitted && request.Role != QueryRoleTodo && request.Role != QueryRoleAny {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query role must be submitted, todo, or any", Action: "choose a supported role"}
	}
	if request.State != QueryStateAll && request.State != QueryStateOpen {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk query state must be all or open", Action: "choose a supported state filter"}
	}
	if request.Role == QueryRoleTodo && request.State != QueryStateAll {
		return result, &DomainError{Code: "invalid_request", Message: "ServiceDesk todo queries do not accept a state filter", Action: "remove the state filter"}
	}
	result.Query = strings.TrimSpace(request.Query)
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	userinfo, err := client.UserInfo(ctx, request.BootstrapBusinessID)
	if err != nil {
		return result, ClassifyError("list", err)
	}
	scopesPerBusiness := 1
	if request.Role == QueryRoleAny {
		scopesPerBusiness = 2
	}
	scopes := make([]queryScope, 0, len(userinfo.Business)*scopesPerBusiness)
	for _, business := range userinfo.Business {
		if request.Role == QueryRoleSubmitted || request.Role == QueryRoleAny {
			scopes = append(scopes, queryScope{Business: business, Role: QueryRoleSubmitted, FlowType: "apply"})
		}
		if request.Role == QueryRoleTodo || request.Role == QueryRoleAny {
			scopes = append(scopes, queryScope{Business: business, Role: QueryRoleTodo, FlowType: "wait"})
		}
	}
	outcomes := queryServiceDeskScopes(ctx, client, scopes)
	all := make(map[string]mergedQueryTicket)
	completedScopeCount := 0
	recoveredTicketCount := 0
	for _, outcome := range outcomes {
		result.Scopes = append(result.Scopes, outcome.Scope)
		if !outcome.Scope.Complete {
			result.Complete = false
			result.FailedScopes = append(result.FailedScopes, FailedScope{BusinessID: outcome.Scope.BusinessID, FlowType: outcome.Scope.FlowType, Error: outcome.Scope.Error})
		} else {
			completedScopeCount++
		}
		recoveredTicketCount += len(outcome.Tickets)
		for _, candidate := range outcome.Tickets {
			ticket := candidate.Ticket
			if ticket.InstanceID == "" || ticket.BusinessID <= 0 {
				continue
			}
			key := fmt.Sprintf("%d:%s", ticket.BusinessID, ticket.InstanceID)
			merged, exists := all[key]
			if !exists {
				merged = mergedQueryTicket{Candidate: candidate, Roles: make(map[QueryRole]queryTicketCandidate, 2)}
			}
			roleCandidate, roleExists := merged.Roles[candidate.Role]
			if !roleExists || preferQueryCandidate(candidate, roleCandidate) {
				merged.Roles[candidate.Role] = candidate
			}
			if exists && preferQueryCandidate(candidate, merged.Candidate) {
				merged.Candidate = candidate
			}
			all[key] = merged
		}
	}
	tickets := make([]Ticket, 0, len(all))
	for _, merged := range all {
		ticket := merged.Candidate.Ticket
		if !matchesTicketQuery(ticket, result.Query) {
			continue
		}
		if submitted, exists := merged.Roles[QueryRoleSubmitted]; exists && !(request.State == QueryStateOpen && submitted.Ticket.StateCategory == "terminal") {
			ticket.RoleMatches = append(ticket.RoleMatches, string(QueryRoleSubmitted))
		}
		if _, exists := merged.Roles[QueryRoleTodo]; exists {
			ticket.RoleMatches = append(ticket.RoleMatches, string(QueryRoleTodo))
		}
		if len(ticket.RoleMatches) == 0 {
			continue
		}
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
	if len(scopes) != 0 && completedScopeCount == 0 && recoveredTicketCount == 0 {
		return result, &DomainError{Code: "query_failed", Message: "ServiceDesk ticket query did not return a usable scope", Action: actionForCode("query_failed")}
	}
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

func matchesTicketQuery(ticket Ticket, query string) bool {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return true
	}
	searchable := strings.ToLower(strings.Join([]string{ticket.Name, ticket.Title, ticket.ServiceName}, " "))
	for _, token := range tokens {
		if !strings.Contains(searchable, token) {
			return false
		}
	}
	return true
}

func preferQueryCandidate(candidate, current queryTicketCandidate) bool {
	candidateUpdate := parseTicketTime(candidate.Ticket.UpdateTime)
	currentUpdate := parseTicketTime(current.Ticket.UpdateTime)
	if !candidateUpdate.Equal(currentUpdate) {
		return candidateUpdate.After(currentUpdate)
	}
	candidateDedicated := candidate.ScopeBusinessID == candidate.Ticket.BusinessID
	currentDedicated := current.ScopeBusinessID == current.Ticket.BusinessID
	if candidateDedicated != currentDedicated {
		return candidateDedicated
	}
	candidateWait := candidate.FlowType == "wait"
	currentWait := current.FlowType == "wait"
	if candidateWait != currentWait {
		return candidateWait
	}
	return candidate.ScopeBusinessID < current.ScopeBusinessID
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
			outcome.Tickets = append(outcome.Tickets, queryTicketCandidate{
				Ticket:          normalizeTicket(raw),
				ScopeBusinessID: scope.Business.BusinessID,
				Role:            scope.Role,
				FlowType:        scope.FlowType,
			})
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
