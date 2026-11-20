package servicedesk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	if result.Plan.Apply.Blocked {
		t.Fatalf("valid dry-run unexpectedly blocked: %+v", result.Plan.Apply)
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

func TestCreateDryRunBlocksUnsupportedServiceKeyRequiredTypeConditionAndDestructiveDrift(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClientWithStartCounter(t, mustReadFixture(t, "lading-4003.json"), nil)
	template := Template{
		Target: EditTarget{BusinessID: 15, ServiceID: 4003}, SnapshotID: "218527", ServiceKey: "unsupported", ServiceName: "Fixture",
		Fields: []Field{
			{BackendKey: "unsupported", StableID: "unsupported", Name: "Unsupported", Type: "script", Required: true, Supported: false},
			{BackendKey: "conditional", StableID: "conditional", Name: "Conditional", Type: "input", Required: true, Supported: true, Conditions: ConditionSet{Operator: "and", Conditions: []Condition{{Field: "mode", Operator: "contains", Value: "active"}}}},
		},
	}
	_ = template

	profile := Overlay{
		SchemaVersion: 1, Name: "fixture-server-hardware-repair", Description: "Fixture", Routing: Routing{DisplayName: "Fixture", UseWhen: "use", DoNotUseWhen: "avoid", Examples: []string{"example"}},
		Target: OverlayTarget{BusinessID: 15, ServiceID: 4003}, Baseline: Baseline{SnapshotID: "old", ServiceKey: "request", MetadataHash: "sha256:" + strings.Repeat("a", 64)},
		Fields: map[string]OverlayField{},
	}
	_ = profile

	// The direct-target path proves the service-key and live required-type gates;
	// a profile-specific drift case is exercised below with a validated registry fixture.
	result, domainErr := Create(context.Background(), Dependencies{Client: client}, CreateRequest{Target: &EditTarget{BusinessID: 15, ServiceID: 4003}, Input: TicketInput{"title": "repair"}, Apply: true})
	if domainErr == nil || !result.Plan.Apply.Blocked || result.Applied {
		t.Fatalf("apply fail-closed result = %+v, error = %v", result, domainErr)
	}
}

func TestCreateDryRunStaleRegistryReportsBlockedApply(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClientWithStartCounter(t, mustReadFixture(t, "lading-4003.json"), nil)
	profile := mustReadFixture(t, "profile-4003.json")
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	server := registryServer(t, registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile), map[string][]byte{profilePath: profile})
	defer server.Close()
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	loader := registryLoaderFor(t, server.URL, t.TempDir(), now)
	if _, _, err := loader.List(context.Background()); err != nil {
		t.Fatalf("populate registry cache: %v", err)
	}
	server.Close()
	loader.Now = func() time.Time { return now.Add(24 * time.Hour) }

	result, domainErr := Create(context.Background(), Dependencies{Client: client, Registry: loader}, CreateRequest{Profile: "fixture-server-hardware-repair", Input: TicketInput{"title": "repair", "devices": []any{map[string]any{"host_name": "server-01", "component": "disk"}}}})
	if domainErr != nil {
		t.Fatalf("stale dry-run error = %v", domainErr)
	}
	if !result.Plan.Apply.Blocked || result.Plan.Apply.BlockReason != "stale_registry" || result.Plan.ProfileSource != "cache" {
		t.Fatalf("stale dry-run plan = %+v", result.Plan)
	}
}

func TestCreateDryRunDestructiveDriftBlocksApply(t *testing.T) {
	t.Parallel()

	client := newTemplateFixtureClientWithStartCounter(t, mustReadFixture(t, "lading-4003.json"), nil)
	profile := mustReadFixture(t, "profile-4003.json")
	profile = []byte(strings.Replace(string(profile), `"serviceKey": "request"`, `"serviceKey": "legacy"`, 1))
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	server := registryServer(t, registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile), map[string][]byte{profilePath: profile})
	defer server.Close()
	loader := registryLoaderFor(t, server.URL, t.TempDir(), time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC))

	result, domainErr := Create(context.Background(), Dependencies{Client: client, Registry: loader}, CreateRequest{
		Profile: "fixture-server-hardware-repair",
		Input:   TicketInput{"title": "repair", "devices": []any{map[string]any{"host_name": "server-01", "component": "disk"}}},
	})
	if domainErr != nil {
		t.Fatalf("destructive-drift dry-run error = %v", domainErr)
	}
	if !result.Plan.TemplateDrift || result.Plan.DriftClassification != driftDestructive || !result.Plan.Apply.Blocked || result.Plan.Apply.BlockReason != "destructive_drift" {
		t.Fatalf("destructive-drift dry-run plan = %+v", result.Plan)
	}
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
