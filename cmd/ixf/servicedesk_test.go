package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ixfsd "github.com/serialq7ic4/ixf-toolbox/internal/servicedesk"
)

func TestServiceDeskHelpListsCommandTree(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"servicedesk", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"doctor", "template", "profile", "create", "list", "get", "withdraw"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("help missing %q: %s", expected, stdout.String())
		}
	}
}

func TestServiceDeskProfileLeafHelpReturnsSuccess(t *testing.T) {
	for _, args := range [][]string{
		{"servicedesk", "profile", "list", "--help"},
		{"servicedesk", "profile", "show", "--help"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exit code = %d, stderr=%q", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "usage: ixf servicedesk profile") {
			t.Fatalf("%v help = %q", args, stdout.String())
		}
	}
}

func TestServiceDeskProfileShowAcceptsJSONFlagBeforeOrAfterName(t *testing.T) {
	profilePath := filepath.Join("..", "..", "internal", "servicedesk", "testdata", "profile-4003.json")
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile fixture: %v", err)
	}
	digest := sha256.Sum256(profile)
	index := []byte(fmt.Sprintf(`{"schemaVersion":1,"updatedAt":"2026-11-23T00:00:00Z","profiles":[{"name":"fixture-server-hardware-repair","path":"profiles/fixture-server-hardware-repair.json","sha256":"%x"}]}`, digest))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/files/ixf-toolbox/servicedesk/v1/index.json":
			_, _ = w.Write(index)
		case "/files/ixf-toolbox/servicedesk/v1/profiles/fixture-server-hardware-repair.json":
			_, _ = w.Write(profile)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	cacheDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve cache directory: %v", err)
	}

	original := serviceDeskRegistryLoader
	serviceDeskRegistryLoader = func() ixfsd.RegistryLoader {
		return ixfsd.RegistryLoader{
			HTTPClient: &http.Client{Transport: serviceDeskRewriteTransport(server.URL)},
			CacheDir:   cacheDir,
		}
	}
	t.Cleanup(func() { serviceDeskRegistryLoader = original })

	for _, args := range [][]string{
		{"servicedesk", "profile", "show", "fixture-server-hardware-repair", "--json"},
		{"servicedesk", "profile", "show", "--json", "fixture-server-hardware-repair"},
	} {
		stdout, stderr, code := runCLITest(t, args...)
		if code != 0 {
			t.Fatalf("%v exit code = %d, stderr=%q stdout=%q", args, code, stderr, stdout)
		}
		var envelope serviceDeskEnvelope
		if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
			t.Fatalf("%v decode output: %v; stdout=%q", args, err, stdout)
		}
		if !envelope.OK || envelope.Operation != "servicedesk_profile_show" {
			t.Fatalf("%v envelope = %+v", args, envelope)
		}
	}
}

func TestServiceDeskProfileShowRejectsInvalidArgumentsBeforeRegistryLookup(t *testing.T) {
	original := serviceDeskRegistryLoader
	serviceDeskRegistryLoader = func() ixfsd.RegistryLoader {
		t.Fatal("registry loader called for invalid profile show arguments")
		return ixfsd.RegistryLoader{}
	}
	t.Cleanup(func() { serviceDeskRegistryLoader = original })

	tests := []struct {
		name string
		args []string
	}{
		{name: "missing name", args: []string{"servicedesk", "profile", "show"}},
		{name: "multiple names", args: []string{"servicedesk", "profile", "show", "first", "second"}},
		{name: "empty then valid names", args: []string{"servicedesk", "profile", "show", "", "valid-name"}},
		{name: "uppercase name", args: []string{"servicedesk", "profile", "show", "Bad-Name"}},
		{name: "blank name", args: []string{"servicedesk", "profile", "show", " "}},
		{name: "long name", args: []string{"servicedesk", "profile", "show", strings.Repeat("a", 65)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, _, code := runCLITest(t, test.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q", code, stdout)
			}
		})
	}
}

func TestServiceDeskRejectsInvalidListContractBeforeCredentialLookup(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing mine", args: []string{"servicedesk", "list", "--business-id", "15"}, want: "--mine"},
		{name: "missing business id", args: []string{"servicedesk", "list", "--mine"}, want: "--business-id"},
		{name: "nonpositive business id", args: []string{"servicedesk", "list", "--mine", "--business-id", "0"}, want: "positive"},
		{name: "nonpositive limit", args: []string{"servicedesk", "list", "--mine", "--business-id", "15", "--limit", "0"}, want: "positive"},
		{name: "unsupported view", args: []string{"servicedesk", "list", "--mine", "--business-id", "15", "--view", "all"}, want: "recent or open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestServiceDeskRejectsInvalidDetailAndWithdrawInputsBeforeCredentialLookup(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "zero ticket id",
			args: []string{"servicedesk", "get", "--business-id", "15", "--ticket-id", "0"},
			want: "positive",
		},
		{
			name: "missing withdraw reason",
			args: []string{"servicedesk", "withdraw", "--business-id", "15", "--ticket-id", "3463015"},
			want: "--reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestServiceDeskCreateRejectsInvalidURLBeforeCredentialLookup(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "ticket.json")
	if err := os.WriteFile(inputPath, []byte(`{"title":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"servicedesk", "create",
		"--url", "https://servicedesk.invalid/#/order/order-edit/15?service_id=4003",
		"--input", inputPath,
	}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "fixed ServiceDesk HTTPS origin") {
		t.Fatalf("exit code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestServiceDeskJSONDomainFailureUsesStableEnvelope(t *testing.T) {
	t.Setenv("IXF_SERVICEDESK_API_KEY", "")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"servicedesk", "get",
		"--url", "https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015",
		"--json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; stdout=%q", err, stdout.String())
	}
	if envelope["operation"] != "servicedesk_get" || envelope["contractVersion"] != float64(1) || envelope["ok"] != false {
		t.Fatalf("envelope = %+v", envelope)
	}
	errPayload, ok := envelope["error"].(map[string]any)
	if !ok || errPayload["code"] != "credential_unavailable" {
		t.Fatalf("error = %+v", envelope["error"])
	}
}

func TestServiceDeskRootDoctorSummaryIsReadOnlyAndSecretFree(t *testing.T) {
	t.Setenv("IXF_SERVICEDESK_API_KEY", "")
	t.Setenv("IXF_SERVICEDESK_BUSINESS_ID", "")
	result := serviceDeskRootDiagnostics()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	if !strings.Contains(string(encoded), `"configured"`) {
		t.Fatalf("diagnostics missing configured field: %s", encoded)
	}
	if strings.Contains(string(encoded), "IXF_SERVICEDESK_API_KEY") {
		t.Fatalf("diagnostics exposed credential configuration details: %s", encoded)
	}
}

func TestServiceDeskCreateDryRunUsesFixtureAndDoesNotStart(t *testing.T) {
	var starts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/service/v3/service/4003/":
			writeTestJSON(t, w, readServiceDeskFixture(t, "service-4003.json"))
		case "/api/tasktmpl3/v3/lading/":
			writeTestJSON(t, w, readServiceDeskFixture(t, "lading-4003.json"))
		case "/api/tasktmpl3/v3/start/":
			starts++
			t.Fatalf("dry-run sent start request")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	original := serviceDeskClientLoader
	serviceDeskClientLoader = func(string) (*ixfsd.Client, *ixfsd.DomainError) {
		return ixfsd.NewClient(ixfsd.Credential{APIKey: "fixture-key"}, ixfsd.ClientOptions{Transport: serviceDeskRewriteTransport(server.URL)}), nil
	}
	t.Cleanup(func() { serviceDeskClientLoader = original })

	inputPath := filepath.Join(t.TempDir(), "ticket.json")
	if err := os.WriteFile(inputPath, []byte(`{"title":"Fixture disk repair","fault_type":"disk","devices":[{"host_name":"node-a","component":"disk"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLITest(t,
		"servicedesk", "create",
		"--url", "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003",
		"--input", inputPath,
		"--json",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
	}
	if starts != 0 {
		t.Fatalf("start requests = %d, want 0", starts)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; stdout=%q", err, stdout)
	}
	if envelope["operation"] != "servicedesk_create" || envelope["contractVersion"] != float64(1) || envelope["ok"] != true {
		t.Fatalf("envelope = %+v", envelope)
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok || result["outcome"] != "dry_run" || result["applied"] != false {
		t.Fatalf("result = %+v", envelope["result"])
	}
	if strings.Contains(stdout, "fixture-key") || strings.Contains(stdout, `"privatePayload"`) {
		t.Fatalf("dry-run leaked private data: %s", stdout)
	}
}

func TestServiceDeskListUsesFixtureClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/v3/userinfo/":
			writeTestJSON(t, w, readServiceDeskFixture(t, "userinfo.json"))
		case "/api/taskflow3/v3/taskflow/":
			writeTestJSON(t, w, readServiceDeskFixture(t, "list.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	original := serviceDeskClientLoader
	serviceDeskClientLoader = func(string) (*ixfsd.Client, *ixfsd.DomainError) {
		return ixfsd.NewClient(ixfsd.Credential{APIKey: "fixture-key"}, ixfsd.ClientOptions{Transport: serviceDeskRewriteTransport(server.URL)}), nil
	}
	t.Cleanup(func() { serviceDeskClientLoader = original })

	stdout, stderr, code := runCLITest(t, "servicedesk", "list", "--mine", "--business-id", "15", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; stdout=%q", err, stdout)
	}
	if envelope["ok"] != true || envelope["operation"] != "servicedesk_list" {
		t.Fatalf("envelope = %+v", envelope)
	}
	result := envelope["result"].(map[string]any)
	tickets := result["tickets"].([]any)
	if len(tickets) != 1 || tickets[0].(map[string]any)["stateCategory"] != "active" {
		t.Fatalf("tickets = %+v", tickets)
	}
}

func TestServiceDeskDoctorProbesInjectedClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/v3/userinfo/" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(t, w, readServiceDeskFixture(t, "userinfo.json"))
	}))
	defer server.Close()

	original := serviceDeskClientForCredential
	serviceDeskClientForCredential = func(credential ixfsd.Credential) *ixfsd.Client {
		return ixfsd.NewClient(credential, ixfsd.ClientOptions{Transport: serviceDeskRewriteTransport(server.URL)})
	}
	t.Cleanup(func() { serviceDeskClientForCredential = original })
	t.Setenv("IXF_SERVICEDESK_API_KEY", "fixture-key")

	stdout, stderr, code := runCLITest(t, "servicedesk", "doctor", "--business-id", "15", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode doctor result: %v; stdout=%q", err, stdout)
	}
	if result["status"] != "ready" || result["authenticated"] != true || result["apiReachable"] != true {
		t.Fatalf("doctor result = %+v", result)
	}
}

func readServiceDeskFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "internal", "servicedesk", "testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return value
}

func serviceDeskRewriteTransport(target string) http.RoundTripper {
	return serviceDeskRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		parsed, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		clone.URL.Scheme = parsed.Scheme
		clone.URL.Host = parsed.Host
		clone.Host = parsed.Host
		return http.DefaultTransport.RoundTrip(clone)
	})
}

type serviceDeskRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn serviceDeskRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
