package servicedesk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStateCategoryKnownAndUnknown(t *testing.T) {
	t.Parallel()
	for state, want := range map[string]string{"RUNNING": "active", "PAUSE": "active", "WITHDRAWN": "terminal", "FINISHED": "terminal", "REVOKE": "terminal", "NEW_STATE": "unknown"} {
		if got := CategorizeState(state); got != want {
			t.Errorf("CategorizeState(%q) = %q, want %q", state, got, want)
		}
	}
}

func TestListMineMergesDeduplicatesSortsAndReportsComplete(t *testing.T) {
	t.Parallel()
	var listCalls atomic.Int32
	client := newTicketFixtureClient(t, &listCalls, nil, nil, false)
	result, domainErr := ListTickets(context.Background(), client, ListRequest{BusinessID: 15, Mine: true, View: "recent", Limit: 20})
	if domainErr != nil {
		t.Fatalf("ListTickets() error = %v", domainErr)
	}
	if !result.Complete || len(result.Tickets) != 1 || result.Tickets[0].InstanceID != "3463015" || result.Tickets[0].StateCategory != "active" {
		t.Fatalf("list result = %+v", result)
	}
	if listCalls.Load() != 1 || result.Sort != "created_time_desc" || result.Scopes[0].BusinessID != 15 {
		t.Fatalf("list metadata = %+v calls=%d", result, listCalls.Load())
	}
}

func TestWithdrawDryRunReadsButDoesNotWrite(t *testing.T) {
	t.Parallel()
	var withdrawCalls atomic.Int32
	client := newTicketFixtureClient(t, nil, &withdrawCalls, nil, false)
	result, domainErr := WithdrawTicket(context.Background(), client, WithdrawRequest{Target: DetailTarget{BusinessID: 15, TicketID: "3463015"}, Reason: "  fixture reason  "})
	if domainErr != nil {
		t.Fatalf("WithdrawTicket() error = %v", domainErr)
	}
	if result.Applied || !result.DryRun || result.BeforeState != "RUNNING" || withdrawCalls.Load() != 0 {
		t.Fatalf("withdraw dry-run = %+v, calls=%d", result, withdrawCalls.Load())
	}
}

func TestWithdrawApplyRechecksStateAndIdentity(t *testing.T) {
	t.Parallel()
	var withdrawCalls, getCalls atomic.Int32
	client := newTicketFixtureClient(t, nil, &withdrawCalls, &getCalls, true)
	result, domainErr := WithdrawTicket(context.Background(), client, WithdrawRequest{Target: DetailTarget{BusinessID: 15, TicketID: "3463015"}, Reason: "fixture reason", Apply: true})
	if domainErr != nil {
		t.Fatalf("WithdrawTicket(apply) error = %v", domainErr)
	}
	if !result.Applied || result.BeforeState != "RUNNING" || result.AfterState != "WITHDRAWN" || withdrawCalls.Load() != 1 || getCalls.Load() != 3 || !result.Verify.StateWithdrawn {
		t.Fatalf("withdraw apply = %+v calls=%d gets=%d", result, withdrawCalls.Load(), getCalls.Load())
	}
}

func TestWithdrawRejectsOtherCreatorTerminalAndInvalidReason(t *testing.T) {
	t.Parallel()
	other := bytes.Replace(mustReadFixture(t, "ticket-running.json"), []byte(`"creator": "fixture.user"`), []byte(`"creator": "other.user"`), 1)
	client := newTicketFixtureClientWithTicket(t, other, nil, nil, nil, false)
	if _, domainErr := WithdrawTicket(context.Background(), client, WithdrawRequest{Target: DetailTarget{BusinessID: 15, TicketID: "3463015"}, Reason: "reason"}); domainErr == nil || domainErr.Code != "state_conflict" {
		t.Fatalf("other creator error = %+v, want state_conflict", domainErr)
	}
	longReason := strings.Repeat("界", 501)
	if _, domainErr := WithdrawTicket(context.Background(), client, WithdrawRequest{Target: DetailTarget{BusinessID: 15, TicketID: "3463015"}, Reason: longReason}); domainErr == nil || domainErr.Code != "invalid_request" {
		t.Fatalf("long reason error = %+v, want invalid_request", domainErr)
	}
}

func newTicketFixtureClient(t *testing.T, listCalls, withdrawCalls, getCalls *atomic.Int32, apply bool) *Client {
	return newTicketFixtureClientWithTicket(t, mustReadFixture(t, "ticket-running.json"), listCalls, withdrawCalls, getCalls, apply)
}

func newTicketFixtureClientWithTicket(t *testing.T, ticket []byte, listCalls, withdrawCalls, getCalls *atomic.Int32, apply bool) *Client {
	t.Helper()
	userinfo := mustReadFixture(t, "userinfo.json")
	list := mustReadFixture(t, "list.json")
	withdrawn := mustReadFixture(t, "ticket-withdrawn.json")
	var stateCalls atomic.Int32
	return NewClient(Credential{APIKey: testAPIKey}, ClientOptions{HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/user/v3/userinfo/":
			return fixtureHTTPResponse(userinfo), nil
		case "/api/taskflow3/v3/taskflow/":
			if listCalls != nil {
				listCalls.Add(1)
			}
			return fixtureHTTPResponse(list), nil
		case "/api/taskflow3/v3/taskflow/3463015/":
			if getCalls != nil {
				getCalls.Add(1)
			}
			if apply && stateCalls.Add(1) == 3 {
				return fixtureHTTPResponse(withdrawn), nil
			}
			return fixtureHTTPResponse(ticket), nil
		case "/api/tasktmpl3/v3/withdraw/":
			if withdrawCalls != nil {
				withdrawCalls.Add(1)
			}
			return fixtureHTTPResponse(mustReadFixture(t, "withdraw.json")), nil
		default:
			return nil, io.EOF
		}
	})}})
}

func ticketJSON(t *testing.T, value Ticket) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
