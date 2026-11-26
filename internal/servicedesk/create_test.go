package servicedesk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCreateDryRunPerformsNoWrite(t *testing.T) {
	t.Parallel()

	var starts atomic.Int32
	client := newTemplateFixtureClientWithStartCounter(t, mustReadFixture(t, "lading-4003.json"), &starts)
	result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{
		Target: &EditTarget{BusinessID: 15, ServiceID: 4003},
		Input:  TicketInput{"title": "repair", "devices": []any{map[string]any{"host_name": "server-01", "component": "disk"}}},
	})
	if domainErr != nil {
		t.Fatalf("Create() error = %v", domainErr)
	}
	if result.Applied || result.Outcome != "dry_run" || starts.Load() != 0 {
		t.Fatalf("dry-run result = %+v, start calls = %d", result, starts.Load())
	}
	if !result.Plan.Apply.Blocked || result.Plan.Apply.BlockReason != "profile_required" {
		t.Fatalf("URL dry-run did not require a built-in profile for apply: %+v", result.Plan.Apply)
	}
}

func TestCreateDryRunBuildsVerifiedRequestShape(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClientWithStartCounter(t, mustReadFixture(t, "lading-4003.json"), nil)
	result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{
		Target: &EditTarget{BusinessID: 15, ServiceID: 4003},
		Input: TicketInput{
			"title":      "repair",
			"fault_type": "Disk",
			"devices":    []any{map[string]any{"host_name": "server-01", "component": "disk"}},
		},
	})
	if domainErr != nil {
		t.Fatalf("Create() error = %v", domainErr)
	}
	if result.Plan.ContractVersion != ContractVersion || result.Plan.Target.SnapshotID != "218527" || result.Plan.Target.ServiceKey != "request" {
		t.Fatalf("plan target/version = %+v/%d", result.Plan.Target, result.Plan.ContractVersion)
	}
	if result.Plan.Input.ProvidedFieldCount != 3 || result.Plan.Input.TableRowCount != 1 || result.Plan.Input.MissingRequiredCount != 0 {
		t.Fatalf("input summary = %+v", result.Plan.Input)
	}
	if result.Plan.privatePayload.SnapshotID != "218527" || result.Plan.privatePayload.ServiceID != 4003 || result.Plan.privatePayload.Key != "request" {
		t.Fatalf("private payload target = %+v", result.Plan.privatePayload)
	}
	if result.Plan.privatePayload.PriorityID != "" || result.Plan.privatePayload.ImpactID != "" || result.Plan.privatePayload.UrgencyID != "" || result.Plan.privatePayload.SourceSystem != "" || result.Plan.privatePayload.SourceBusiness != "" || result.Plan.privatePayload.TempSubmitID != nil {
		t.Fatalf("private payload outer defaults = %+v", result.Plan.privatePayload)
	}
	if result.Plan.privatePayload.Inputs["retry"] != "true" || result.Plan.privatePayload.Inputs["_loop"] != json.Number("0") && result.Plan.privatePayload.Inputs["_loop"] != 0 {
		t.Fatalf("private payload control inputs = %#v", result.Plan.privatePayload.Inputs)
	}
	if _, ok := result.Plan.privatePayload.Inputs["field_title_1001"]; !ok {
		t.Fatalf("private payload did not use current backend key: %#v", result.Plan.privatePayload.Inputs)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(result) error = %v", err)
	}
	if string(encoded) == "" || string(encoded) == "{}" || containsAny(string(encoded), "private_payload", "field_title_1001", "retry", "_loop") {
		t.Fatalf("result exposes private payload: %s", encoded)
	}
}

func TestCreateApplyRepeatsPreflightWritesOnceAndVerifiesReadback(t *testing.T) {
	t.Parallel()
	var serviceCalls, ladingCalls, starts, gets atomic.Int32
	client := newApplyFixtureClient(t, &serviceCalls, &ladingCalls, &starts, &gets, false, false)
	result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{
		Profile: "server-repair",
		Input:   repairFixtureInput(),
		Apply:   true,
	})
	if domainErr != nil {
		t.Fatalf("Create(apply) error = %v", domainErr)
	}
	if !result.Applied || result.Outcome != "applied" || result.Ticket == nil || result.Ticket.InstanceID != "3463015" {
		t.Fatalf("apply result = %+v", result)
	}
	if serviceCalls.Load() != 2 || ladingCalls.Load() != 2 || starts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("request counts service=%d lading=%d start=%d get=%d, want 2/2/1/1", serviceCalls.Load(), ladingCalls.Load(), starts.Load(), gets.Load())
	}
	if result.Verify == nil || !result.Verify.StateReadable {
		t.Fatalf("verification = %+v", result.Verify)
	}
}

func TestCreateApplyTimeoutReturnsUnknownWithoutRetry(t *testing.T) {
	t.Parallel()
	var starts, gets atomic.Int32
	client := newApplyFixtureClient(t, nil, nil, &starts, &gets, true, false)
	result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{
		Profile: "server-repair",
		Input:   repairFixtureInput(),
		Apply:   true,
	})
	if domainErr == nil || domainErr.Code != "unknown_outcome" || domainErr.Outcome != "unknown" {
		t.Fatalf("timeout error = %+v, want unknown_outcome", domainErr)
	}
	if result.Applied || result.Outcome != "unknown" || starts.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("timeout result = %+v, starts=%d gets=%d", result, starts.Load(), gets.Load())
	}
	if domainErr.Ticket == nil || domainErr.Ticket.Title != "Fixture repair" || domainErr.Ticket.DetailURL != "" {
		t.Fatalf("unknown reconciliation reference = %+v", domainErr.Ticket)
	}
}

func TestCreateApplyReadbackFailureRetainsTicketReference(t *testing.T) {
	t.Parallel()
	var starts, gets atomic.Int32
	client := newApplyFixtureClient(t, nil, nil, &starts, &gets, false, true)
	result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{
		Profile: "server-repair",
		Input:   repairFixtureInput(),
		Apply:   true,
	})
	if domainErr == nil || domainErr.Code != "readback_failed" || result.Applied || result.Outcome != "created_unverified" || starts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("readback result=%+v error=%+v starts=%d gets=%d", result, domainErr, starts.Load(), gets.Load())
	}
	if domainErr.Ticket == nil || domainErr.Ticket.InstanceID != "3463015" || domainErr.Ticket.DetailURL != "" {
		t.Fatalf("readback ticket reference = %+v", domainErr.Ticket)
	}
}

func TestCreateApplyReadsBackFrontendTicketID(t *testing.T) {
	t.Parallel()
	const matching = `{"instance_id":3463015,"business_id":9,"service_id":463,"title":"Fixture repair","state":"RUNNING"}`
	const ambiguous = `{"instance_id":3463015,"business_id":15,"service_id":463,"title":"Fixture repair","state":"RUNNING"}`
	const wrongTitle = `{"instance_id":3463015,"business_id":9,"service_id":463,"title":"Different title","state":"RUNNING"}`
	const wrongService = `{"instance_id":3463015,"business_id":9,"service_id":4003,"title":"Fixture repair","state":"RUNNING"}`
	const wrongID = `{"instance_id":3463016,"business_id":9,"service_id":463,"title":"Fixture repair","state":"RUNNING"}`
	const canonical = `{"id":3463001,"instance_id":3463016,"business_id":9,"service_id":463,"name":"REQ123","title":"Fixture repair","state":"RUNNING"}`
	const wrongName = `{"instance_id":3463016,"business_id":9,"service_id":463,"name":"REQ456","title":"Fixture repair","state":"RUNNING"}`
	const duplicateName = `{"instance_id":3463017,"business_id":9,"service_id":463,"name":"REQ123","title":"Fixture repair","state":"RUNNING"}`
	const startWithName = `{"id":3463015,"name":"REQ123"}`
	for _, tt := range []struct {
		name            string
		startData       string
		detailData      string
		submittedRows   string
		wantDetailScope string
		wantInstanceID  string
		wantCode        string
	}{
		{name: "explicit instance business", startData: `{"id":3463015,"business_id":9}`, wantDetailScope: "9"},
		{name: "resolve from submitted tickets", startData: `{"id":3463015}`, submittedRows: matching, wantDetailScope: "1"},
		{name: "response business points to form", startData: `{"id":3463015,"business_id":1}`, submittedRows: matching, wantDetailScope: "1"},
		{name: "both ID fields agree", startData: `{"id":3463015,"instance_id":"3463015"}`, submittedRows: matching, wantDetailScope: "1"},
		{name: "detail resolves canonical ID", startData: startWithName, detailData: canonical, wantDetailScope: "1", wantInstanceID: "3463016"},
		{name: "submitted list resolves canonical ID", startData: startWithName, submittedRows: canonical, wantDetailScope: "1", wantInstanceID: "3463016"},
		{name: "canonical ID requires matching ticket number", startData: startWithName, detailData: wrongName, wantDetailScope: "1", wantCode: "readback_mismatch"},
		{name: "duplicate ticket number", startData: startWithName, submittedRows: canonical + "," + duplicateName, wantDetailScope: "1", wantCode: "readback_mismatch"},
		{name: "missing ID", startData: `{"name":"REQ123"}`, wantCode: "unknown_outcome"},
		{name: "missing data", startData: `null`, wantCode: "unknown_outcome"},
		{name: "invalid ID", startData: `{"id":-1}`, wantCode: "unknown_outcome"},
		{name: "conflicting IDs", startData: `{"id":3463015,"instance_id":"42"}`, wantCode: "unknown_outcome"},
		{name: "no exact match", startData: `{"id":3463015}`, submittedRows: wrongTitle + "," + wrongService + "," + wrongID, wantDetailScope: "1", wantCode: "readback_failed"},
		{name: "ambiguous match", startData: `{"id":3463015}`, submittedRows: matching + "," + ambiguous, wantDetailScope: "1", wantCode: "readback_mismatch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var starts, details, lists atomic.Int32
			client := newRepairFixtureClient(t, nil, func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/tasktmpl3/v3/start/":
					starts.Add(1)
					if req.Method != http.MethodPost || req.Header.Get("businessId") != "1" {
						t.Errorf("start used %s, businessId=%q", req.Method, req.Header.Get("businessId"))
					}
					return fixtureHTTPResponse([]byte(`{"result":true,"data":` + tt.startData + `}`)), nil
				case "/api/taskflow3/v3/taskflow/3463015/":
					details.Add(1)
					scope := req.Header.Get("businessId")
					if req.Method != http.MethodGet || scope != tt.wantDetailScope {
						t.Errorf("detail used %s, businessId=%q, want GET %q", req.Method, scope, tt.wantDetailScope)
					}
					if tt.detailData != "" {
						return fixtureHTTPResponse([]byte(`{"result":true,"data":` + tt.detailData + `}`)), nil
					}
					if scope == "9" {
						return fixtureHTTPResponse([]byte(`{"result":true,"data":` + matching + `}`)), nil
					}
					response := fixtureHTTPResponse(nil)
					response.StatusCode = http.StatusNotFound
					return response, nil
				case "/api/user/v3/userinfo/":
					return fixtureHTTPResponse([]byte(`{"result":true,"data":{"business":[{"business_id":1},{"business_id":9}]}}`)), nil
				case "/api/taskflow3/v3/taskflow/":
					lists.Add(1)
					if req.Method != http.MethodGet || req.URL.Query().Get("flow_type") != "apply" {
						t.Errorf("unexpected submitted list request: %s %s", req.Method, req.URL)
					}
					// The submitted list can include an instance from another business.
					rows := ""
					if req.Header.Get("businessId") == "1" {
						rows = tt.submittedRows
					}
					return fixtureHTTPResponse([]byte(`{"result":true,"data":[` + rows + `],"meta":{"limit":100,"offset":0}}`)), nil
				default:
					return nil, nil
				}
			})
			result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{
				Profile: "server-repair", Input: repairFixtureInput(), Apply: true,
			})
			if starts.Load() != 1 {
				t.Fatalf("start calls = %d, want one", starts.Load())
			}
			switch tt.wantCode {
			case "":
				wantID := tt.wantInstanceID
				wantDetailID := "3463001"
				if wantID == "" {
					wantID = "3463015"
					wantDetailID = "3463015"
				}
				if domainErr != nil || !result.Applied || result.Outcome != "applied" || result.Verify == nil || !result.Verify.StateReadable ||
					result.Ticket == nil || result.Ticket.BusinessID != 9 || result.Ticket.InstanceID != wantID ||
					result.DetailURL != "https://servicedesk.iflytek.com/#/order/order-detail/1?id="+wantDetailID {
					t.Fatalf("result = %+v, ticket = %+v, error = %v", result, result.Ticket, domainErr)
				}
				if details.Load() != 1 || (tt.submittedRows == "") != (lists.Load() == 0) {
					t.Fatalf("readback calls: details=%d lists=%d", details.Load(), lists.Load())
				}
			case "unknown_outcome":
				if domainErr == nil || domainErr.Code != tt.wantCode || domainErr.Retryable || domainErr.Outcome != "unknown" ||
					result.Applied || result.Outcome != "unknown" || details.Load() != 0 || lists.Load() != 0 ||
					domainErr.Ticket == nil || domainErr.Ticket.Title != "Fixture repair" || domainErr.Ticket.RequestStartedAt == "" {
					t.Fatalf("result = %+v, error = %+v, details=%d lists=%d", result, domainErr, details.Load(), lists.Load())
				}
			default:
				if domainErr == nil || domainErr.Code != tt.wantCode || domainErr.Retryable || result.Applied ||
					result.Outcome != "created_unverified" || domainErr.Ticket == nil || domainErr.Ticket.InstanceID != "3463015" || domainErr.Ticket.DetailURL != "" {
					t.Fatalf("result = %+v, error = %+v", result, domainErr)
				}
			}
		})
	}
}

func TestDomainErrorsSerializeWithoutCredential(t *testing.T) {
	t.Parallel()
	secret := testAPIKey
	err := &DomainError{Code: "unknown_outcome", Message: "request outcome is unknown", Outcome: "unknown", Ticket: &TicketReference{Title: "safe title", DetailURL: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015"}}
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil || strings.Contains(string(encoded), secret) || strings.Contains(err.Error(), secret) {
		t.Fatalf("serialized domain error = %s, marshal error=%v", encoded, marshalErr)
	}
}

func newApplyFixtureClient(t *testing.T, service, lading, starts, gets *atomic.Int32, timeoutStart, failReadback bool) *Client {
	t.Helper()
	return newRepairFixtureClient(t, nil, func(req *http.Request) (*http.Response, error) {
		increment := func(counter *atomic.Int32) {
			if counter != nil {
				counter.Add(1)
			}
		}
		switch req.URL.Path {
		case "/api/service/v3/service/463/":
			increment(service)
		case "/api/tasktmpl3/v3/lading/":
			increment(lading)
		case "/api/tasktmpl3/v3/start/":
			increment(starts)
			if timeoutStart {
				return nil, errors.New("connection reset after request dispatch")
			}
			var payload StartPayload
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Inputs["branch-key"] != serverRepairBranch {
				t.Errorf("unexpected start payload: %+v, %v", payload, err)
			}
			if failReadback {
				return fixtureHTTPResponse([]byte(`{"result":true,"data":{"instance_id":"3463015","business_id":1}}`)), nil
			}
			return fixtureHTTPResponse(mustReadFixture(t, "start.json")), nil
		case "/api/taskflow3/v3/taskflow/3463015/":
			increment(gets)
			if failReadback {
				return fixtureHTTPResponse([]byte(`{"result":false,"data":{}}`)), nil
			}
			return fixtureHTTPResponse([]byte(`{"result":true,"data":{"instance_id":"3463015","service_id":463,"business_id":1,"title":"Fixture repair","state":"RUNNING"}}`)), nil
		case "/api/user/v3/userinfo/":
			return fixtureHTTPResponse([]byte(`{"result":true,"data":{"business":[]}}`)), nil
		}
		return nil, nil
	})
}

func fixtureHTTPResponse(body []byte) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}
}

func newTemplateFixtureClientWithStartCounter(t *testing.T, lading []byte, starts *atomic.Int32) *Client {
	t.Helper()
	service := mustReadFixture(t, "service-4003.json")
	return newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/service/v3/service/4003/":
			_, _ = w.Write(service)
		case "/api/tasktmpl3/v3/lading/":
			_, _ = w.Write(lading)
		case "/api/tasktmpl3/v3/start/":
			if starts != nil {
				starts.Add(1)
			}
			_, _ = w.Write([]byte(`{"result":true,"data":{"instance_id":"3463015","name":"REQ"}}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func containsAny(value string, values ...string) bool {
	for _, candidate := range values {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
