package servicedesk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAPIKey = "fixture-api-key-value"

func TestSanitizedFixtures(t *testing.T) {
	t.Parallel()

	names := []string{
		"userinfo.json",
		"service-4003.json",
		"lading-4003.json",
		"list.json",
		"start.json",
		"ticket-running.json",
		"ticket-withdrawn.json",
		"withdraw.json",
	}
	liveKey, hasLiveKey := os.LookupEnv("IXF_SERVICEDESK_API_KEY")
	liveKey = strings.TrimSpace(liveKey)

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if !json.Valid(raw) {
				t.Fatal("fixture is not valid JSON")
			}

			lower := bytes.ToLower(raw)
			for _, forbidden := range [][]byte{[]byte("apikey"), []byte("cookie")} {
				if bytes.Contains(lower, forbidden) {
					t.Fatalf("fixture contains forbidden credential marker %q", forbidden)
				}
			}
			if hasLiveKey && liveKey != "" && bytes.Contains(raw, []byte(liveKey)) {
				t.Fatal("fixture contains the configured ServiceDesk credential")
			}
		})
	}
}

func TestClientEndpointRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    string
		path      string
		rawQuery  string
		fixture   string
		checkBody func(*testing.T, []byte)
		call      func(context.Context, *Client) error
	}{
		{
			name: "userinfo", method: http.MethodGet, path: "/api/user/v3/userinfo/", fixture: "userinfo.json",
			call: func(ctx context.Context, client *Client) error {
				got, err := client.UserInfo(ctx, 15)
				if err == nil && got.Username != "fixture.user" {
					return fmt.Errorf("username = %q", got.Username)
				}
				return err
			},
		},
		{
			name: "service", method: http.MethodGet, path: "/api/service/v3/service/4003/", fixture: "service-4003.json",
			call: func(ctx context.Context, client *Client) error {
				got, err := client.Service(ctx, EditTarget{BusinessID: 15, ServiceID: 4003})
				if err == nil && got.SnapshotID != "218527" {
					return fmt.Errorf("snapshot ID = %q", got.SnapshotID)
				}
				return err
			},
		},
		{
			name: "lading", method: http.MethodGet, path: "/api/tasktmpl3/v3/lading/", rawQuery: "service_id=4003&snapshot_id=218527", fixture: "lading-4003.json",
			call: func(ctx context.Context, client *Client) error {
				got, err := client.Lading(ctx, EditTarget{BusinessID: 15, ServiceID: 4003}, "218527")
				if err == nil && len(got) != 4 {
					return fmt.Errorf("lading field count = %d", len(got))
				}
				return err
			},
		},
		{
			name: "start", method: http.MethodPost, path: "/api/tasktmpl3/v3/start/", fixture: "start.json",
			checkBody: func(t *testing.T, body []byte) {
				t.Helper()
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Errorf("decode body: %v", err)
					return
				}
				if got["snapshot_id"] != "218527" || got["service_id"] != float64(4003) || got["key"] != "request" {
					t.Errorf("start body = %v", got)
				}
			},
			call: func(ctx context.Context, client *Client) error {
				got, err := client.Start(ctx, 15, StartPayload{
					SnapshotID: "218527",
					ServiceID:  4003,
					Key:        "request",
					Inputs:     map[string]any{"title": "Fixture repair"},
				})
				if err == nil && got.InstanceID != "3463015" {
					return fmt.Errorf("instance ID = %q", got.InstanceID)
				}
				return err
			},
		},
		{
			name: "list", method: http.MethodGet, path: "/api/taskflow3/v3/taskflow/", rawQuery: "flow_type=apply&key=request&limit=20&offset=40", fixture: "list.json",
			call: func(ctx context.Context, client *Client) error {
				got, err := client.List(ctx, 15, ListQuery{FlowType: "apply", Limit: 20, Offset: 40})
				if err == nil && (len(got.Data) != 1 || got.Data[0].InstanceID != "3463015" || got.Data[0].Step != "Fixture review" || got.Meta.TotalCount != 1) {
					return fmt.Errorf("ticket page = %+v", got)
				}
				return err
			},
		},
		{
			name: "get", method: http.MethodGet, path: "/api/taskflow3/v3/taskflow/3463015/", fixture: "ticket-running.json",
			call: func(ctx context.Context, client *Client) error {
				got, err := client.Get(ctx, DetailTarget{BusinessID: 15, TicketID: "3463015"})
				if err == nil && got.State != "RUNNING" {
					return fmt.Errorf("ticket state = %q", got.State)
				}
				return err
			},
		},
		{
			name: "withdraw", method: http.MethodPost, path: "/api/tasktmpl3/v3/withdraw/", fixture: "withdraw.json",
			checkBody: func(t *testing.T, body []byte) {
				t.Helper()
				var got WithdrawPayload
				if err := json.Unmarshal(body, &got); err != nil {
					t.Errorf("decode body: %v", err)
					return
				}
				if got.InstanceID != "3463015" || got.Message != "fixture reason" {
					t.Errorf("withdraw body = %+v", got)
				}
			},
			call: func(ctx context.Context, client *Client) error {
				return client.Withdraw(ctx, 15, WithdrawPayload{InstanceID: "3463015", Message: "fixture reason"})
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := mustReadFixture(t, test.fixture)
			client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Host != serviceDeskHost {
					t.Errorf("request host = %q, want %q", request.Host, serviceDeskHost)
				}
				if request.Method != test.method || request.URL.Path != test.path || request.URL.RawQuery != test.rawQuery {
					t.Errorf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), test.method, test.path)
				}
				if request.Header.Get("businessId") != "15" {
					t.Errorf("businessId header = %q", request.Header.Get("businessId"))
				}
				if request.Header.Get("apikey") != testAPIKey {
					t.Error("apikey header is missing or incorrect")
				}
				if request.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Content-Type header = %q", request.Header.Get("Content-Type"))
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				if test.checkBody != nil {
					test.checkBody(t, body)
				} else if len(body) != 0 {
					t.Errorf("unexpected request body %q", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixture)
			}))

			if err := test.call(context.Background(), client); err != nil {
				t.Fatalf("client call: %v", err)
			}
		})
	}
}

func TestClientListUsesRequestedFlowType(t *testing.T) {
	t.Parallel()
	for _, flowType := range []string{"apply", "wait"} {
		flowType := flowType
		t.Run(flowType, func(t *testing.T) {
			t.Parallel()
			client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.URL.RawQuery != "flow_type="+flowType+"&key=request&limit=20&offset=40" {
					t.Errorf("query = %q", request.URL.RawQuery)
				}
				_, _ = w.Write(mustReadFixture(t, "list.json"))
			}))
			if _, err := client.List(context.Background(), 9, ListQuery{FlowType: flowType, Limit: 20, Offset: 40}); err != nil {
				t.Fatalf("List() error = %v", err)
			}
		})
	}
}

func TestClientListRejectsMalformedTicketAndPagination(t *testing.T) {
	t.Parallel()
	validMeta := `{"limit":100,"offset":0,"next":null,"previous":null,"total_count":1}`
	tests := []struct {
		name string
		body string
	}{
		{name: "missing ticket ID", body: `{"result":true,"data":[{"business_id":9}],"meta":` + validMeta + `}`},
		{name: "null ticket ID", body: `{"result":true,"data":[{"business_id":9,"instance_id":null}],"meta":` + validMeta + `}`},
		{name: "nonnumeric ticket ID", body: `{"result":true,"data":[{"business_id":9,"instance_id":"REQ123"}],"meta":` + validMeta + `}`},
		{name: "invalid business ID", body: `{"result":true,"data":[{"business_id":0,"instance_id":"123"}],"meta":` + validMeta + `}`},
		{name: "missing pagination fields", body: `{"result":true,"data":[],"meta":{}}`},
		{name: "missing next before total count", body: `{"result":true,"data":[{"business_id":9,"instance_id":"123"}],"meta":{"limit":100,"offset":0,"previous":null,"total_count":2}}`},
		{name: "null next before total count", body: `{"result":true,"data":[{"business_id":9,"instance_id":"123"}],"meta":{"limit":100,"offset":0,"next":null,"previous":null,"total_count":2}}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, test.body)
			}))
			_, err := client.List(context.Background(), 9, ListQuery{FlowType: "apply", Limit: 100})
			assertAPIError(t, err, "invalid_response", http.StatusOK, false)
		})
	}
}

func TestClientListAcceptsTerminalPageWithoutNext(t *testing.T) {
	t.Parallel()
	client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"result":true,"data":[{"business_id":9,"instance_id":"123"}],"meta":{"limit":100,"offset":100,"next":null,"previous":"?offset=0","total_count":101}}`)
	}))

	page, err := client.List(context.Background(), 9, ListQuery{FlowType: "apply", Limit: 100, Offset: 100})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Data) != 1 || page.Meta.TotalCount != 101 || page.Meta.Next != nil {
		t.Fatalf("List() = %+v", page)
	}
}

func TestClientListRejectsUnsupportedFlowTypeBeforeTransport(t *testing.T) {
	t.Parallel()
	for _, flowType := range []string{"", "any", "unknown"} {
		flowType := flowType
		t.Run(flowType, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("transport should not be called")
			})})
			_, err := client.List(context.Background(), 9, ListQuery{FlowType: flowType, Limit: 20})
			assertAPIError(t, err, "invalid_request", 0, false)
			if calls.Load() != 0 {
				t.Fatalf("transport calls = %d, want 0", calls.Load())
			}
		})
	}
}

func TestClientGetFallsBackToVisibleSubmittedTicketWhenDetailIsForbidden(t *testing.T) {
	t.Parallel()
	var detailCalls, applyCalls, waitCalls atomic.Int32
	client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/taskflow3/v3/taskflow/123/":
			detailCalls.Add(1)
			w.WriteHeader(http.StatusForbidden)
		case "/api/taskflow3/v3/taskflow/":
			switch request.URL.Query().Get("flow_type") {
			case "apply":
				applyCalls.Add(1)
				writeClientListPage(t, w, 0, nil, clientListTicketJSON("123"))
			case "wait":
				waitCalls.Add(1)
				writeClientListPage(t, w, 0, nil)
			default:
				t.Fatalf("unexpected flow_type %q", request.URL.Query().Get("flow_type"))
			}
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))

	got, err := client.Get(context.Background(), DetailTarget{BusinessID: 9, TicketID: "123"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.InstanceID != "123" || got.BusinessID != 9 || got.State != "RUNNING" {
		t.Fatalf("Get() = %+v", got)
	}
	if detailCalls.Load() != 1 || applyCalls.Load() != 1 || waitCalls.Load() != 0 {
		t.Fatalf("calls detail=%d apply=%d wait=%d", detailCalls.Load(), applyCalls.Load(), waitCalls.Load())
	}
}

func TestClientGetFallbackPaginatesAndSearchesTodoTickets(t *testing.T) {
	t.Parallel()
	var waitOffsets []string
	client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/taskflow3/v3/taskflow/456/" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if request.URL.Path != "/api/taskflow3/v3/taskflow/" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		flowType := request.URL.Query().Get("flow_type")
		offset := request.URL.Query().Get("offset")
		switch {
		case flowType == "apply":
			writeClientListPage(t, w, 0, nil)
		case flowType == "wait" && offset == "0":
			waitOffsets = append(waitOffsets, offset)
			next := "/api/taskflow3/v3/taskflow/?offset=100"
			writeClientListPage(t, w, 0, &next, clientListTicketJSON("111"))
		case flowType == "wait" && offset == "100":
			waitOffsets = append(waitOffsets, offset)
			writeClientListPage(t, w, 100, nil, clientListTicketJSON("456"))
		default:
			t.Fatalf("unexpected list query %q", request.URL.RawQuery)
		}
	}))

	got, err := client.Get(context.Background(), DetailTarget{BusinessID: 9, TicketID: "456"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.InstanceID != "456" || fmt.Sprint(waitOffsets) != "[0 100]" {
		t.Fatalf("Get() = %+v, wait offsets = %v", got, waitOffsets)
	}
}

func TestClientGetPreservesPermissionErrorWhenTicketIsNotVisible(t *testing.T) {
	t.Parallel()
	client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/taskflow3/v3/taskflow/789/" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeClientListPage(t, w, 0, nil)
	}))

	_, err := client.Get(context.Background(), DetailTarget{BusinessID: 9, TicketID: "789"})
	assertAPIError(t, err, "permission_denied", http.StatusForbidden, false)
}

func TestClientGetFallbackRejectsTruncatedListPage(t *testing.T) {
	t.Parallel()
	var waitCalls atomic.Int32
	client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/taskflow3/v3/taskflow/789/" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if request.URL.Query().Get("flow_type") == "wait" {
			waitCalls.Add(1)
		}
		_, _ = io.WriteString(w, `{"result":true,"data":[`+clientListTicketJSON("123")+`],"meta":{"limit":100,"offset":0,"next":null,"previous":null,"total_count":2}}`)
	}))

	_, err := client.Get(context.Background(), DetailTarget{BusinessID: 9, TicketID: "789"})
	assertAPIError(t, err, "invalid_response", http.StatusOK, false)
	if waitCalls.Load() != 0 {
		t.Fatalf("wait list calls = %d, want 0 after invalid apply page", waitCalls.Load())
	}
}

func writeClientListPage(t *testing.T, w http.ResponseWriter, offset int, next *string, tickets ...string) {
	t.Helper()
	nextJSON := "null"
	if next != nil {
		encoded, err := json.Marshal(*next)
		if err != nil {
			t.Fatal(err)
		}
		nextJSON = string(encoded)
	}
	_, _ = fmt.Fprintf(w, `{"result":true,"data":[%s],"meta":{"limit":100,"offset":%d,"next":%s,"previous":null,"total_count":%d}}`, strings.Join(tickets, ","), offset, nextJSON, len(tickets))
}

func clientListTicketJSON(instanceID string) string {
	return fmt.Sprintf(`{"business_id":9,"instance_id":"%s","name":"REQ-FIXTURE","title":"Fixture","service_id":1,"service_name":"Fixture","state":"RUNNING","step":"Review","creator":"fixture.user","created_time":"2026-11-23 10:00:00","update_time":"2026-11-23 11:00:00"}`, instanceID)
}

func TestDefaultHTTPClientDoesNotInheritProxyEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	client := newDirectHTTPClient(defaultTimeout)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("default ServiceDesk client inherits proxy environment")
	}

	client = NewClient(Credential{APIKey: testAPIKey}, ClientOptions{HTTPClient: &http.Client{}}).httpClient
	transport, ok = client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatalf("empty injected HTTP client transport = %#v, want direct transport", client.Transport)
	}
}

func TestClientClassifiesHTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		code   string
	}{
		{status: http.StatusUnauthorized, code: "authentication_failed"},
		{status: http.StatusForbidden, code: "permission_denied"},
	}
	for _, test := range tests {
		test := test
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			t.Parallel()
			client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"result":false,"message":"private server text"}`))
			}))

			_, err := client.UserInfo(context.Background(), 15)
			assertAPIError(t, err, test.code, test.status, false)
		})
	}
}

func TestClientRejectsFalseAndMalformedEnvelopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "result false", body: `{"result":false,"message":"rejected"}`, code: "api_rejected"},
		{name: "invalid JSON", body: `{"result":`, code: "invalid_response"},
		{name: "missing result", body: `{"data":{}}`, code: "invalid_response"},
		{name: "missing data", body: `{"result":true}`, code: "invalid_response"},
		{name: "wrong data shape", body: `{"result":true,"data":[]}`, code: "invalid_response"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			_, err := client.UserInfo(context.Background(), 15)
			assertAPIError(t, err, test.code, http.StatusOK, false)
		})
	}
}

func TestClientListNormalizesAndRejectsTicketFieldVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		instanceID    string
		step          string
		wantID        string
		wantStep      string
		wantErrorCode string
	}{
		{name: "large integer ID and multiple steps", instanceID: `123456789012345678901234567890`, step: `["review","approve"]`, wantID: "123456789012345678901234567890", wantStep: "review / approve"},
		{name: "zero ID", instanceID: `0`, step: `"review"`, wantErrorCode: "invalid_response"},
		{name: "negative ID", instanceID: `-1`, step: `"review"`, wantErrorCode: "invalid_response"},
		{name: "fractional ID", instanceID: `1.5`, step: `"review"`, wantErrorCode: "invalid_response"},
		{name: "null step element", instanceID: `1`, step: `["review",null]`, wantErrorCode: "invalid_response"},
		{name: "mixed step element", instanceID: `1`, step: `["review",2]`, wantErrorCode: "invalid_response"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := fmt.Sprintf(`{"result":true,"data":[{"business_id":9,"instance_id":%s,"step":%s}],"meta":{"limit":20,"offset":0,"next":null,"previous":null,"total_count":1}}`, test.instanceID, test.step)
			client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			page, err := client.List(context.Background(), 9, ListQuery{FlowType: "apply", Limit: 20})
			if test.wantErrorCode != "" {
				assertAPIError(t, err, test.wantErrorCode, http.StatusOK, false)
				return
			}
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(page.Data) != 1 || page.Data[0].InstanceID != test.wantID || page.Data[0].Step != test.wantStep {
				t.Fatalf("ticket page = %+v", page)
			}
		})
	}
}

func TestClientPreservesTimeoutIdentity(t *testing.T) {
	t.Parallel()

	client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	})
	_, err := client.UserInfo(context.Background(), 15)
	assertAPIError(t, err, "timeout", 0, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error does not preserve timeout identity: %v", err)
	}
}

func TestClientDoesNotExposeRawTransportError(t *testing.T) {
	t.Parallel()

	client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport echoed " + testAPIKey)
		}),
	})
	_, err := client.UserInfo(context.Background(), 15)
	assertAPIError(t, err, "network_error", 0, true)

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		t.Fatal("public error chain exposes the raw transport error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Cause == nil || strings.Contains(apiErr.Cause.Error(), testAPIKey) {
		t.Fatal("sanitized transport cause is missing or discloses the API key")
	}
}

func TestClientIgnoresSuppliedCookieJar(t *testing.T) {
	t.Parallel()

	jar := &trackingCookieJar{}
	client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{
		HTTPClient: &http.Client{
			Jar: jar,
			Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if cookie := request.Header.Get("Cookie"); cookie != "" {
					t.Errorf("Cookie header = %q, want empty", cookie)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Set-Cookie": {"session=stored; Path=/"}},
					Body:       io.NopCloser(strings.NewReader(`{"result":true,"data":{"username":"fixture.user"}}`)),
				}, nil
			}),
		},
	})
	if _, err := client.UserInfo(context.Background(), 15); err != nil {
		t.Fatalf("UserInfo() error = %v", err)
	}
	if jar.cookiesCalls != 0 || jar.setCookiesCalls != 0 {
		t.Fatalf("cookie jar calls = Cookies %d, SetCookies %d; want 0, 0", jar.cookiesCalls, jar.setCookiesCalls)
	}
}

func TestClientLimitsResponseSize(t *testing.T) {
	t.Parallel()

	client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(io.LimitReader(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)), int64(maxResponseBytes+1))),
			}, nil
		}),
	})
	_, err := client.UserInfo(context.Background(), 15)
	assertAPIError(t, err, "response_too_large", http.StatusOK, false)
}

func TestClientRedactsEchoedAPIKey(t *testing.T) {
	t.Parallel()

	client := newFixedHostTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `{"result":false,"message":%q}`, request.Header.Get("apikey"))
	}))
	_, err := client.UserInfo(context.Background(), 15)
	if err == nil {
		t.Fatal("UserInfo() error = nil")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Fatal("client error disclosed API key echoed by server")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Cause != nil && strings.Contains(apiErr.Cause.Error(), testAPIKey) {
		t.Fatal("APIError cause disclosed API key echoed by server")
	}
}

func TestClientNeverRetriesCreateTimeout(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := NewClient(Credential{APIKey: testAPIKey}, ClientOptions{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			if request.URL.Scheme != "https" || request.URL.Host != serviceDeskHost || request.URL.Path != "/api/tasktmpl3/v3/start/" {
				t.Errorf("request URL = %s", request.URL)
			}
			return nil, context.DeadlineExceeded
		}),
	})
	_, err := client.Start(context.Background(), 15, StartPayload{ServiceID: 4003})
	assertAPIError(t, err, "timeout", 0, false)
	if got := requests.Load(); got != 1 {
		t.Fatalf("start requests = %d, want 1", got)
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

func newFixedHostTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "example.com"
	serverAddress := server.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, serverAddress)
	}
	return NewClient(Credential{APIKey: testAPIKey}, ClientOptions{
		HTTPClient: &http.Client{Transport: transport, Timeout: 2 * time.Second},
	})
}

func assertAPIError(t *testing.T, err error, code string, status int, retryable bool) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want *APIError")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Code != code || apiErr.Status != status || apiErr.Retryable != retryable {
		t.Fatalf("APIError = %+v, want code %q status %d retryable %v", apiErr, code, status, retryable)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type trackingCookieJar struct {
	cookiesCalls    int
	setCookiesCalls int
}

func (j *trackingCookieJar) Cookies(*url.URL) []*http.Cookie {
	j.cookiesCalls++
	return []*http.Cookie{{Name: "session", Value: "private"}}
}

func (j *trackingCookieJar) SetCookies(*url.URL, []*http.Cookie) {
	j.setCookiesCalls++
}
