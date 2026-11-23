package servicedesk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueryTicketsValidatesRoleStateAndMine(t *testing.T) {
	t.Parallel()
	client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport called for invalid query")
		return nil, nil
	})})
	tests := []QueryRequest{
		{BootstrapBusinessID: 0, Mine: true, Role: QueryRoleSubmitted, State: QueryStateAll},
		{BootstrapBusinessID: 9, Mine: false, Role: QueryRoleSubmitted, State: QueryStateAll},
		{BootstrapBusinessID: 9, Mine: true, Role: "", State: QueryStateAll},
		{BootstrapBusinessID: 9, Mine: true, Role: "unknown", State: QueryStateAll},
		{BootstrapBusinessID: 9, Mine: true, Role: QueryRoleTodo, State: QueryStateOpen},
		{BootstrapBusinessID: 9, Mine: true, Role: QueryRoleSubmitted, State: "unknown"},
	}
	for _, request := range tests {
		if _, domainErr := QueryTickets(context.Background(), client, request); domainErr == nil || domainErr.Code != "invalid_request" {
			t.Errorf("QueryTickets(%+v) error = %+v, want invalid_request", request, domainErr)
		}
	}
}

func TestQueryTicketsMapsSubmittedAndTodoFlows(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		role     QueryRole
		flowType string
	}{
		{role: QueryRoleSubmitted, flowType: "apply"},
		{role: QueryRoleTodo, flowType: "wait"},
	} {
		test := test
		t.Run(string(test.role), func(t *testing.T) {
			t.Parallel()
			var flowsMu sync.Mutex
			var flows []string
			client := queryFixtureClient(t, []Business{{BusinessID: 9, Name: "Fixture"}}, func(request *http.Request) []byte {
				flowsMu.Lock()
				flows = append(flows, request.URL.Query().Get("flow_type"))
				flowsMu.Unlock()
				return queryPage(t, []TicketResponse{queryTicket(9, "101", "RUNNING", "other.user", "2026-11-20 09:00:00")}, 0, nil)
			})
			result, domainErr := QueryTickets(context.Background(), client, QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: test.role, State: QueryStateAll, Limit: 20})
			if domainErr != nil {
				t.Fatalf("QueryTickets() error = %+v", domainErr)
			}
			if len(result.Tickets) != 1 || result.Tickets[0].Creator != "other.user" {
				t.Fatalf("tickets = %+v", result.Tickets)
			}
			flowsMu.Lock()
			defer flowsMu.Unlock()
			if len(flows) != 1 || flows[0] != test.flowType {
				t.Fatalf("flows = %v, want [%s]", flows, test.flowType)
			}
		})
	}
}

func TestQueryTicketsFullyPaginatesScopesBeforeGlobalLimit(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := queryFixtureClient(t, []Business{{BusinessID: 1, Name: "Aggregate"}, {BusinessID: 9, Name: "Dedicated"}}, func(request *http.Request) []byte {
		calls.Add(1)
		businessID, _ := strconv.Atoi(request.Header.Get("businessId"))
		offset, _ := strconv.Atoi(request.URL.Query().Get("offset"))
		switch {
		case businessID == 1 && offset == 0:
			next := "/api/taskflow3/v3/taskflow/?offset=100"
			return queryPage(t, []TicketResponse{
				queryTicket(9, "101", "RUNNING", "fixture.user", "2026-11-20 09:00:00"),
				queryTicket(1, "102", "RUNNING", "fixture.user", "2026-11-20 11:00:00"),
			}, 0, &next)
		case businessID == 1 && offset == 100:
			return queryPage(t, []TicketResponse{queryTicket(1, "103", "RUNNING", "fixture.user", "2026-11-20 10:00:00")}, 100, nil)
		case businessID == 9 && offset == 0:
			return queryPage(t, []TicketResponse{
				queryTicket(9, "101", "RUNNING", "fixture.user", "2026-11-20 09:00:00"),
				queryTicket(9, "104", "RUNNING", "fixture.user", "2026-11-20 12:00:00"),
			}, 0, nil)
		default:
			t.Fatalf("unexpected list request business=%d offset=%d", businessID, offset)
			return nil
		}
	})
	result, domainErr := QueryTickets(context.Background(), client, QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: QueryRoleSubmitted, State: QueryStateAll, Limit: 2})
	if domainErr != nil {
		t.Fatalf("QueryTickets() error = %+v", domainErr)
	}
	if calls.Load() != 3 || !result.Complete || result.HasMore == nil || !*result.HasMore {
		t.Fatalf("result metadata = %+v, calls=%d", result, calls.Load())
	}
	if got := queryTicketIDs(result.Tickets); fmt.Sprint(got) != "[104 102]" {
		t.Fatalf("ticket order = %v", got)
	}
	if len(result.Scopes) != 2 || result.Scopes[0].BusinessID != 1 || result.Scopes[0].FlowType != "apply" || result.Scopes[0].TicketCount != 3 || result.Scopes[1].BusinessID != 9 || result.Scopes[1].TicketCount != 2 {
		t.Fatalf("scopes = %+v", result.Scopes)
	}
}

func TestQueryTicketsSubmittedOpenExcludesTerminalAndKeepsUnknown(t *testing.T) {
	t.Parallel()
	states := []string{"FINISHED", "WITHDRAWN", "REVOKE", "RUNNING", "PAUSE", "NEW_STATE"}
	tickets := make([]TicketResponse, 0, len(states))
	for index, state := range states {
		tickets = append(tickets, queryTicket(9, strconv.Itoa(index+1), state, "fixture.user", fmt.Sprintf("2026-11-20 %02d:00:00", index+1)))
	}
	client := queryFixtureClient(t, []Business{{BusinessID: 9, Name: "Fixture"}}, func(*http.Request) []byte {
		return queryPage(t, tickets, 0, nil)
	})
	result, domainErr := QueryTickets(context.Background(), client, QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: QueryRoleSubmitted, State: QueryStateOpen, Limit: 20})
	if domainErr != nil {
		t.Fatalf("QueryTickets() error = %+v", domainErr)
	}
	got := make([]string, 0, len(result.Tickets))
	for _, ticket := range result.Tickets {
		got = append(got, ticket.State)
	}
	sort.Strings(got)
	if fmt.Sprint(got) != "[NEW_STATE PAUSE RUNNING]" {
		t.Fatalf("states = %v", got)
	}
}

func TestQueryTicketsZeroBusinessScopesIsCompleteEmpty(t *testing.T) {
	t.Parallel()
	client := queryFixtureClient(t, nil, func(*http.Request) []byte {
		t.Fatal("list called without a business scope")
		return nil
	})
	result, domainErr := QueryTickets(context.Background(), client, QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: QueryRoleSubmitted, State: QueryStateAll, Limit: 20})
	if domainErr != nil {
		t.Fatalf("QueryTickets() error = %+v", domainErr)
	}
	if !result.Complete || len(result.Tickets) != 0 || len(result.Scopes) != 0 || result.HasMore == nil || *result.HasMore {
		t.Fatalf("result = %+v", result)
	}
}

func TestQueryTicketsBoundsScopeConcurrency(t *testing.T) {
	businesses := make([]Business, 0, 6)
	for businessID := 1; businessID <= 6; businessID++ {
		businesses = append(businesses, Business{BusinessID: businessID, Name: fmt.Sprintf("Business %d", businessID)})
	}
	started := make(chan struct{}, 6)
	release := make(chan struct{})
	var inFlight atomic.Int32
	var maximum atomic.Int32
	client := queryFixtureClient(t, businesses, func(*http.Request) []byte {
		current := inFlight.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		inFlight.Add(-1)
		return queryPage(t, nil, 0, nil)
	})
	resultChannel := make(chan QueryResult, 1)
	errorChannel := make(chan *DomainError, 1)
	go func() {
		result, domainErr := QueryTickets(context.Background(), client, QueryRequest{BootstrapBusinessID: 1, Mine: true, Role: QueryRoleSubmitted, State: QueryStateAll, Limit: 20})
		resultChannel <- result
		errorChannel <- domainErr
	}()
	for count := 0; count < maxConcurrentServiceDeskScopes; count++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("scope query did not start")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d scope queries started concurrently", maxConcurrentServiceDeskScopes)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	result := <-resultChannel
	if domainErr := <-errorChannel; domainErr != nil {
		t.Fatalf("QueryTickets() error = %+v", domainErr)
	}
	if maximum.Load() <= 1 || maximum.Load() > maxConcurrentServiceDeskScopes {
		t.Fatalf("maximum concurrency = %d", maximum.Load())
	}
	if len(result.Scopes) != 6 {
		t.Fatalf("scopes = %+v", result.Scopes)
	}
	for index, scope := range result.Scopes {
		if scope.BusinessID != index+1 {
			t.Fatalf("scope order = %+v", result.Scopes)
		}
	}
}

func queryFixtureClient(t *testing.T, businesses []Business, list func(*http.Request) []byte) *Client {
	t.Helper()
	return NewClient(Credential{APIKey: testAPIKey}, ClientOptions{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/user/v3/userinfo/":
			return fixtureHTTPResponse(queryEnvelope(t, UserInfo{Username: "fixture.user", DisplayName: "Fixture User", Business: businesses}, nil)), nil
		case "/api/taskflow3/v3/taskflow/":
			return fixtureHTTPResponse(list(request)), nil
		default:
			return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
		}
	})})
}

func queryPage(t *testing.T, tickets []TicketResponse, offset int, next *string) []byte {
	t.Helper()
	return queryEnvelope(t, tickets, &PageMeta{Limit: 100, Offset: offset, Next: next, TotalCount: len(tickets)})
}

func queryEnvelope(t *testing.T, data any, meta *PageMeta) []byte {
	t.Helper()
	envelope := map[string]any{"result": true, "data": data}
	if meta != nil {
		envelope["meta"] = meta
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal query fixture: %v", err)
	}
	return encoded
}

func queryTicket(businessID int, instanceID, state, creator, updateTime string) TicketResponse {
	return TicketResponse{
		BusinessID:  businessID,
		InstanceID:  instanceID,
		Name:        "REQ" + instanceID,
		Title:       "Fixture " + instanceID,
		ServiceID:   4003,
		ServiceName: "Fixture Service",
		State:       state,
		Step:        "Fixture step",
		Creator:     creator,
		CreatedTime: updateTime,
		UpdateTime:  updateTime,
	}
}

func queryTicketIDs(tickets []Ticket) []string {
	result := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		result = append(result, ticket.InstanceID)
	}
	return result
}
