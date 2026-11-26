package servicedesk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestServerRepairProfilePinsBranchAndRejectsExtraFields(t *testing.T) {
	t.Parallel()
	client := repairFixtureClient(t, nil)
	input := repairFixtureInput()
	req := CreateRequest{Profile: "server-repair", Input: input}
	plan, err := Create(t.Context(), Dependencies{Client: client}, req)
	if err != nil || plan.Plan.Apply.Blocked {
		t.Fatalf("server repair preflight = %+v, err = %v", plan.Plan.Apply, err)
	}
	if got := plan.Plan.Input; got.ProvidedFieldCount != 8 || got.DefaultedFieldCount != 2 {
		t.Fatalf("input summary = %+v", got)
	}
	wire := plan.Plan.privatePayload.Inputs
	if len(wire) != 12 || wire["catalog-key"] != "GUZHANGBAOXIULEI20260416204401815" ||
		wire["branch-key"] != "FUWUQIBAOXIU20250704162135675" ||
		wire["room-key"] != "room" || wire["owner-key"] != "user.example" || wire["line-key"] != "Fixture Line" ||
		wire["details-key"] != "<p>Disk failure</p>" {
		t.Fatalf("unexpected request fields: %#v", wire)
	}
	contract, inspectErr := InspectBuiltinProfile(t.Context(), client, req.Profile)
	if inspectErr != nil || len(contract.InputContract.Fields) != 8 {
		t.Fatalf("unexpected public input contract: %+v, %v", contract.InputContract, inspectErr)
	}
	for _, field := range contract.InputContract.Fields {
		if field.Name == "businessLine" && len(field.Choices) != 0 {
			t.Fatalf("script dependencies were exposed as choices: %+v", field.Choices)
		}
	}
	for _, field := range []string{"branch", "GUZHANGBAOXIULEILIEBIAO20250704174956781", "networkDetails"} {
		input[field] = "network"
		if _, err := Create(t.Context(), Dependencies{Client: client}, req); err == nil || err.Code != "unknown_field" {
			t.Fatalf("override %q error = %v", field, err)
		}
		delete(input, field)
	}
	delete(input, "businessLine")
	if _, err := Create(t.Context(), Dependencies{Client: client}, req); err == nil || err.Field != "businessLine" {
		t.Fatalf("missing business line error = %+v", err)
	}
}

func TestServerRepairProfileBlocksChangesBeforeWriting(t *testing.T) {
	t.Parallel()
	for _, edit := range []func(Lading){
		func(fields Lading) { delete(fields, "branch-key") },
		func(fields Lading) {
			fields["branch-key"] = json.RawMessage(`{"name":"Branch","type":"single_checkbox","field_unique_Identifier":"GUZHANGBAOXIULEILIEBIAO20250704174956781","field_choice":{"network":"Network"}}`)
		},
		func(fields Lading) {
			fields["new"] = json.RawMessage(`{"name":"New required field","type":"input","required":true}`)
		},
	} {
		result, err := Create(t.Context(), Dependencies{Client: repairFixtureClient(t, edit)}, CreateRequest{Profile: "server-repair", Input: repairFixtureInput(), Apply: true})
		if err == nil || err.Code != "destructive_drift" || result.Applied {
			t.Fatalf("changed form result = %+v, error = %+v", result, err)
		}
	}
	target := EditTarget{BusinessID: 1, ServiceID: 463}
	for _, req := range []CreateRequest{{Target: &target, Apply: true}, {Profile: "network-repair", Apply: true}} {
		if _, err := Create(t.Context(), Dependencies{Client: repairFixtureClient(t, nil)}, req); err == nil ||
			(err.Code != "profile_required" && err.Code != "profile_not_found") {
			t.Fatalf("unapproved create target error = %+v", err)
		}
	}
}

func TestServerRepairLoadsDictionaryChoices(t *testing.T) {
	t.Parallel()
	client := newRepairFixtureClient(t, func(fields Lading) {
		fields["room-key"] = json.RawMessage(`{"name":"Site","type":"single_select","required":true,"field_unique_Identifier":"MUBIAOJIFANG20250702092613639","field_source":"dataDictionary","field_source_dictionary":42,"field_choice":{"key":"last","name":"Last"}}`)
	}, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/workflow/v3/sys_dict/42/" {
			return nil, nil
		}
		if req.Method != http.MethodGet || req.Header.Get("businessId") != "1" {
			t.Errorf("dictionary request = %s, businessId=%q", req.Method, req.Header.Get("businessId"))
		}
		return fixtureHTTPResponse([]byte(`{"result":true,"data":{"children":[{"key":"room","name":"Room"},{"key":"last","name":"Last"}]}}`)), nil
	})
	contract, err := InspectBuiltinProfile(t.Context(), client, "server-repair")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range contract.InputContract.Fields {
		if field.Name == "site" && len(field.Choices) != 2 {
			t.Fatalf("site options = %+v", field.Choices)
		}
	}
	created, domainErr := Create(t.Context(), Dependencies{Client: client}, CreateRequest{Profile: "server-repair", Input: repairFixtureInput()})
	if domainErr != nil || created.Plan.privatePayload.Inputs["room-key"] != "room" {
		t.Fatalf("dictionary choice was not encoded: %v", domainErr)
	}
}

func repairFixtureInput() TicketInput {
	return TicketInput{"title": "Fixture repair", "details": "Disk failure", "businessUnit": "Storage", "site": "Room",
		"deviceUser": "user.example", "businessLine": "Fixture Line", "deviceCount": json.Number("1"), "expectedCompletion": "2026-12-01 10:00:00"}
}

func repairFixtureClient(t *testing.T, edit func(Lading)) *Client {
	return newRepairFixtureClient(t, edit, nil)
}

func newRepairFixtureClient(t *testing.T, edit func(Lading), intercept func(*http.Request) (*http.Response, error)) *Client {
	t.Helper()
	var fields Lading
	if err := json.Unmarshal(mustReadFixture(t, "lading-server-repair.json"), &fields); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(fields)
	}
	lading, err := json.Marshal(map[string]any{"result": true, "data": fields})
	if err != nil {
		t.Fatal(err)
	}
	return NewClient(Credential{APIKey: testAPIKey}, ClientOptions{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if intercept != nil {
			if response, err := intercept(req); response != nil || err != nil {
				return response, err
			}
		}
		switch req.URL.Path {
		case "/api/service/v3/service/463/":
			return fixtureHTTPResponse([]byte(`{"result":true,"data":{"id":463,"business_id":1,"key":"request","name":"Hardware repair","snapshot_id":"9000"}}`)), nil
		case "/api/tasktmpl3/v3/lading/":
			return fixtureHTTPResponse(lading), nil
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, context.Canceled
		}
	})})
}

func TestBuiltinProfileCatalog(t *testing.T) {
	profiles := BuiltinProfiles()
	if len(profiles) != 1 || profiles[0].Name != "server-repair" || profiles[0].Target != (OverlayTarget{BusinessID: 1, ServiceID: 463}) {
		t.Fatalf("catalog = %+v", profiles)
	}
	if !strings.Contains(profiles[0].Routing.DisplayName, "服务器报修") {
		t.Fatalf("display name = %q", profiles[0].Routing.DisplayName)
	}
}
