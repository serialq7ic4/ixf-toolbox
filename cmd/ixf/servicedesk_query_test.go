package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ixfsd "github.com/serialq7ic4/ixf-toolbox/internal/servicedesk"
)

func TestServiceDeskListAndSearchBuildQueryRequests(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		operation string
		want      ixfsd.QueryRequest
	}{
		{
			name:      "submitted open list",
			args:      []string{"servicedesk", "list", "--mine", "--role", "submitted", "--state", "open", "--business-id", "9", "--limit", "7", "--json"},
			operation: "servicedesk_list",
			want:      ixfsd.QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: ixfsd.QueryRoleSubmitted, State: ixfsd.QueryStateOpen, Limit: 7},
		},
		{
			name:      "todo list without state flag",
			args:      []string{"servicedesk", "list", "--mine", "--role", "todo", "--business-id", "9", "--json"},
			operation: "servicedesk_list",
			want:      ixfsd.QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: ixfsd.QueryRoleTodo, State: ixfsd.QueryStateAll, Limit: 20},
		},
		{
			name:      "search defaults to any all",
			args:      []string{"servicedesk", "search", "--mine", "--query", "  disk repair  ", "--business-id", "9", "--json"},
			operation: "servicedesk_search",
			want:      ixfsd.QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: ixfsd.QueryRoleAny, State: ixfsd.QueryStateAll, Query: "disk repair", Limit: 20},
		},
		{
			name:      "search any open",
			args:      []string{"servicedesk", "search", "--mine", "--role", "any", "--state", "open", "--query", "server-01", "--business-id", "9", "--limit", "50", "--json"},
			operation: "servicedesk_search",
			want:      ixfsd.QueryRequest{BootstrapBusinessID: 9, Mine: true, Role: ixfsd.QueryRoleAny, State: ixfsd.QueryStateOpen, Query: "server-01", Limit: 50},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var captured ixfsd.QueryRequest
			installServiceDeskQueryTestDoubles(t, func(request ixfsd.QueryRequest) {
				captured = request
			})
			stdout, stderr, code := runCLITest(t, test.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
			}
			if captured != test.want {
				t.Fatalf("request = %+v, want %+v", captured, test.want)
			}
			var envelope serviceDeskEnvelope
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if !envelope.OK || envelope.Operation != test.operation {
				t.Fatalf("envelope = %+v", envelope)
			}
		})
	}
}

func TestServiceDeskQueryRejectsInvalidContractsBeforeCredentialLookup(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "list missing mine", args: []string{"servicedesk", "list", "--role", "submitted", "--business-id", "9"}},
		{name: "list missing role", args: []string{"servicedesk", "list", "--mine", "--business-id", "9"}},
		{name: "list any role", args: []string{"servicedesk", "list", "--mine", "--role", "any", "--business-id", "9"}},
		{name: "todo explicit state", args: []string{"servicedesk", "list", "--mine", "--role", "todo", "--state", "all", "--business-id", "9"}},
		{name: "empty search query", args: []string{"servicedesk", "search", "--mine", "--query", "  ", "--business-id", "9"}},
		{name: "nonpositive limit", args: []string{"servicedesk", "search", "--mine", "--query", "disk", "--business-id", "9", "--limit", "0"}},
		{name: "removed view", args: []string{"servicedesk", "list", "--mine", "--role", "submitted", "--business-id", "9", "--view", "open"}},
		{name: "removed cursor", args: []string{"servicedesk", "list", "--mine", "--role", "submitted", "--business-id", "9", "--cursor", "next"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentialCalls := 0
			originalClientLoader := serviceDeskClientLoader
			serviceDeskClientLoader = func(string) (*ixfsd.Client, *ixfsd.DomainError) {
				credentialCalls++
				return &ixfsd.Client{}, nil
			}
			t.Cleanup(func() { serviceDeskClientLoader = originalClientLoader })
			_, _, code := runCLITest(t, test.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if credentialCalls != 0 {
				t.Fatalf("credential loader calls = %d, want 0", credentialCalls)
			}
		})
	}
}

func TestServiceDeskQueryBusinessIDPrecedence(t *testing.T) {
	home := t.TempDir()
	setServiceDeskQueryTestHome(t, home)
	configDir := filepath.Join(home, ".config", "ixf-toolbox")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "servicedesk-business-id"), []byte("167\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		environment *string
		args        []string
		want        int
	}{
		{name: "explicit wins", environment: stringPointer("141"), args: []string{"servicedesk", "list", "--mine", "--role", "submitted", "--business-id", "9", "--json"}, want: 9},
		{name: "environment fallback", environment: stringPointer("141"), args: []string{"servicedesk", "list", "--mine", "--role", "submitted", "--json"}, want: 141},
		{name: "file fallback", args: []string{"servicedesk", "list", "--mine", "--role", "submitted", "--json"}, want: 167},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setOptionalEnvironment(t, "IXF_SERVICEDESK_BUSINESS_ID", test.environment)
			var captured ixfsd.QueryRequest
			installServiceDeskQueryTestDoubles(t, func(request ixfsd.QueryRequest) {
				captured = request
			})
			stdout, stderr, code := runCLITest(t, test.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
			}
			if captured.BootstrapBusinessID != test.want {
				t.Fatalf("bootstrap business ID = %d, want %d", captured.BootstrapBusinessID, test.want)
			}
			if !strings.Contains(stdout, `"businessId":77`) {
				t.Fatalf("candidate business ID changed: %s", stdout)
			}
		})
	}
}

func TestServiceDeskQueryMissingBusinessIDUsesAutomaticDiscovery(t *testing.T) {
	home := t.TempDir()
	setServiceDeskQueryTestHome(t, home)
	setOptionalEnvironment(t, "IXF_SERVICEDESK_BUSINESS_ID", nil)
	for _, command := range []string{"list", "search"} {
		t.Run(command, func(t *testing.T) {
			var captured ixfsd.QueryRequest
			installServiceDeskQueryTestDoubles(t, func(request ixfsd.QueryRequest) {
				captured = request
			})
			args := []string{"servicedesk", command, "--mine", "--json"}
			if command == "list" {
				args = append(args, "--role", "submitted")
			} else {
				args = append(args, "--query", "disk")
			}
			stdout, stderr, code := runCLITest(t, args...)
			if code != 0 || stderr != "" {
				t.Fatalf("exit code=%d stderr=%q stdout=%q", code, stderr, stdout)
			}
			if captured.BootstrapBusinessID != 0 {
				t.Fatalf("bootstrap business ID = %d, want automatic discovery", captured.BootstrapBusinessID)
			}
			var envelope serviceDeskEnvelope
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if !envelope.OK || envelope.Operation != "servicedesk_"+command || envelope.Error != nil {
				t.Fatalf("envelope = %+v", envelope)
			}
			if strings.Contains(stdout, home) {
				t.Fatalf("output exposed configuration path: %s", stdout)
			}
		})
	}
}

func setServiceDeskQueryTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func installServiceDeskQueryTestDoubles(t *testing.T, capture func(ixfsd.QueryRequest)) {
	t.Helper()
	originalClientLoader := serviceDeskClientLoader
	originalRunner := serviceDeskQueryRunner
	serviceDeskClientLoader = func(string) (*ixfsd.Client, *ixfsd.DomainError) {
		return &ixfsd.Client{}, nil
	}
	serviceDeskQueryRunner = func(_ context.Context, _ *ixfsd.Client, request ixfsd.QueryRequest) (ixfsd.QueryResult, *ixfsd.DomainError) {
		capture(request)
		hasMore := false
		return ixfsd.QueryResult{
			ContractVersion:     ixfsd.ContractVersion,
			Role:                request.Role,
			StateFilter:         request.State,
			Query:               request.Query,
			BootstrapBusinessID: request.BootstrapBusinessID,
			Tickets:             []ixfsd.Ticket{{BusinessID: 77, InstanceID: "123", Name: "REQ123"}},
			Scopes:              []ixfsd.ScopeResult{},
			FailedScopes:        []ixfsd.FailedScope{},
			Sort:                "update_time_desc",
			HasMore:             &hasMore,
			Complete:            true,
		}, nil
	}
	t.Cleanup(func() {
		serviceDeskClientLoader = originalClientLoader
		serviceDeskQueryRunner = originalRunner
	})
}

func setOptionalEnvironment(t *testing.T, name string, value *string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	if value == nil {
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	} else if err := os.Setenv(name, *value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func stringPointer(value string) *string {
	return &value
}
