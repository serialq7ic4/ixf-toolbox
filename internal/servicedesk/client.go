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
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBytes = 8 << 20
	defaultTimeout   = 30 * time.Second
)

type ClientOptions struct {
	HTTPClient *http.Client
	Transport  http.RoundTripper
	Now        func() time.Time
}

type Client struct {
	credential Credential
	httpClient *http.Client
	now        func() time.Time
}

type APIError struct {
	Code      string
	Status    int
	Retryable bool
	Message   string
	Cause     error
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type Business struct {
	BusinessID int    `json:"business_id"`
	Name       string `json:"name"`
}

type UserInfo struct {
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Business    []Business `json:"business"`
}

type Service struct {
	ID         int    `json:"id"`
	BusinessID int    `json:"business_id"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	SnapshotID string `json:"snapshot_id"`
}

type Lading map[string]json.RawMessage

type StartPayload struct {
	SnapshotID     string         `json:"snapshot_id"`
	ServiceID      int            `json:"service_id"`
	Key            string         `json:"key"`
	PriorityID     string         `json:"priority_id"`
	ImpactID       string         `json:"impact_id"`
	UrgencyID      string         `json:"urgency_id"`
	SourceSystem   string         `json:"source_system"`
	SourceBusiness string         `json:"source_business"`
	Inputs         map[string]any `json:"inputs"`
	TempSubmitID   any            `json:"temp_submit_id"`
}

type StartResponse struct {
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
}

type ListQuery struct {
	Limit  int
	Offset int
}

type PageMeta struct {
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	Next       *string `json:"next"`
	Previous   *string `json:"previous"`
	TotalCount int     `json:"total_count"`
}

type TicketResponse struct {
	BusinessID        int            `json:"business_id"`
	InstanceID        string         `json:"instance_id"`
	Name              string         `json:"name"`
	Title             string         `json:"title"`
	ServiceID         int            `json:"service_id"`
	ServiceName       string         `json:"service_name"`
	State             string         `json:"state"`
	Step              string         `json:"step"`
	Creator           string         `json:"creator"`
	CurrentProcessors []any          `json:"current_processors"`
	CreatedTime       string         `json:"created_time"`
	UpdateTime        string         `json:"update_time"`
	FinishTime        any            `json:"finish_time"`
	ResourceURI       string         `json:"resource_uri"`
	Inputs            map[string]any `json:"inputs"`
}

type TicketPage struct {
	Data []TicketResponse `json:"data"`
	Meta PageMeta         `json:"meta"`
}

type WithdrawPayload struct {
	InstanceID string `json:"instance_id"`
	Message    string `json:"message"`
}

func NewClient(credential Credential, options ClientOptions) *Client {
	httpClient := newDirectHTTPClient(defaultTimeout)
	if options.HTTPClient != nil {
		clone := *options.HTTPClient
		httpClient = &clone
		if httpClient.Transport == nil {
			httpClient.Transport = newDirectHTTPClient(defaultTimeout).Transport
		}
	}
	if options.Transport != nil {
		httpClient.Transport = options.Transport
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Client{credential: credential, httpClient: httpClient, now: now}
}

func newDirectHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Timeout: timeout, Transport: transport}
}

func (c *Client) UserInfo(ctx context.Context, businessID int) (UserInfo, error) {
	var response UserInfo
	err := c.do(ctx, http.MethodGet, "/api/user/v3/userinfo/", nil, businessID, nil, &response, nil, true)
	return response, err
}

func (c *Client) Service(ctx context.Context, target EditTarget) (Service, error) {
	var response Service
	if target.ServiceID <= 0 {
		return response, invalidRequest("ServiceDesk service ID must be positive")
	}
	path := "/api/service/v3/service/" + strconv.Itoa(target.ServiceID) + "/"
	err := c.do(ctx, http.MethodGet, path, nil, target.BusinessID, nil, &response, nil, true)
	return response, err
}

func (c *Client) Lading(ctx context.Context, target EditTarget, snapshotID string) (Lading, error) {
	if target.ServiceID <= 0 {
		return nil, invalidRequest("ServiceDesk service ID must be positive")
	}
	if strings.TrimSpace(snapshotID) == "" {
		return nil, invalidRequest("ServiceDesk snapshot ID is required")
	}
	query := url.Values{
		"service_id":  {strconv.Itoa(target.ServiceID)},
		"snapshot_id": {snapshotID},
	}
	var response Lading
	err := c.do(ctx, http.MethodGet, "/api/tasktmpl3/v3/lading/", query, target.BusinessID, nil, &response, nil, true)
	return response, err
}

func (c *Client) Start(ctx context.Context, businessID int, payload StartPayload) (StartResponse, error) {
	var response StartResponse
	err := c.do(ctx, http.MethodPost, "/api/tasktmpl3/v3/start/", nil, businessID, payload, &response, nil, false)
	return response, err
}

func (c *Client) List(ctx context.Context, businessID int, query ListQuery) (TicketPage, error) {
	values := url.Values{
		"key":       {"request"},
		"flow_type": {"apply"},
		"limit":     {strconv.Itoa(query.Limit)},
		"offset":    {strconv.Itoa(query.Offset)},
	}
	var response TicketPage
	err := c.do(ctx, http.MethodGet, "/api/taskflow3/v3/taskflow/", values, businessID, nil, &response.Data, &response.Meta, true)
	return response, err
}

func (c *Client) Get(ctx context.Context, target DetailTarget) (TicketResponse, error) {
	var response TicketResponse
	if _, err := parsePositiveID(target.TicketID); err != nil {
		return response, invalidRequest("ServiceDesk ticket ID must be a positive integer")
	}
	path := "/api/taskflow3/v3/taskflow/" + target.TicketID + "/"
	err := c.do(ctx, http.MethodGet, path, nil, target.BusinessID, nil, &response, nil, true)
	return response, err
}

func (c *Client) Withdraw(ctx context.Context, businessID int, payload WithdrawPayload) error {
	var response json.RawMessage
	return c.do(ctx, http.MethodPost, "/api/tasktmpl3/v3/withdraw/", nil, businessID, payload, &response, nil, false)
}

type responseEnvelope struct {
	Result *bool           `json:"result"`
	Data   json.RawMessage `json:"data"`
	Meta   json.RawMessage `json:"meta"`
}

func (c *Client) do(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	businessID int,
	payload any,
	dataTarget any,
	metaTarget any,
	retryable bool,
) error {
	if businessID <= 0 {
		return invalidRequest("ServiceDesk business ID must be positive")
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &APIError{Code: "invalid_request", Message: "ServiceDesk request could not be encoded", Cause: safeCause(err, c.credential.APIKey)}
		}
		body = bytes.NewReader(encoded)
	}
	requestURL := serviceDeskOrigin + path
	if len(query) != 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return &APIError{Code: "invalid_request", Message: "ServiceDesk request could not be created", Cause: safeCause(err, c.credential.APIKey)}
	}
	request.Header.Set("businessId", strconv.Itoa(businessID))
	request.Header.Set("apikey", c.credential.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return transportError(err, retryable, c.credential.APIKey)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		code, message := statusError(response.StatusCode)
		return &APIError{
			Code:      code,
			Status:    response.StatusCode,
			Retryable: retryable && (response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError),
			Message:   message,
		}
	}

	raw, err := readBoundedResponse(response.Body)
	if err != nil {
		if errors.Is(err, errResponseTooLarge) {
			return &APIError{Code: "response_too_large", Status: response.StatusCode, Message: "ServiceDesk response exceeded the size limit"}
		}
		return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk response could not be read", Cause: safeCause(err, c.credential.APIKey)}
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Result == nil {
		return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk returned a malformed response envelope"}
	}
	if !*envelope.Result {
		return &APIError{Code: "api_rejected", Status: response.StatusCode, Message: "ServiceDesk rejected the request"}
	}
	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk response envelope is missing data"}
	}
	if dataTarget != nil {
		if err := json.Unmarshal(envelope.Data, dataTarget); err != nil {
			return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk response data has an unexpected shape"}
		}
	}
	if metaTarget != nil {
		if len(envelope.Meta) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Meta), []byte("null")) {
			return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk response envelope is missing pagination metadata"}
		}
		if err := json.Unmarshal(envelope.Meta, metaTarget); err != nil {
			return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk pagination metadata has an unexpected shape"}
		}
	}
	return nil
}

var errResponseTooLarge = errors.New("ServiceDesk response too large")

func readBoundedResponse(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBytes {
		return nil, errResponseTooLarge
	}
	return raw, nil
}

func invalidRequest(message string) *APIError {
	return &APIError{Code: "invalid_request", Message: message}
}

func statusError(status int) (string, string) {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_failed", "ServiceDesk rejected the configured credential"
	case http.StatusForbidden:
		return "permission_denied", "ServiceDesk denied access to the requested business"
	default:
		return "http_error", fmt.Sprintf("ServiceDesk returned HTTP status %d", status)
	}
}

func transportError(err error, retryable bool, secret string) *APIError {
	code := "network_error"
	message := "ServiceDesk request failed"
	isTimeout := errors.Is(err, context.DeadlineExceeded)
	var netErr net.Error
	if !isTimeout && errors.As(err, &netErr) {
		isTimeout = netErr.Timeout()
	}
	if isTimeout {
		code = "timeout"
		message = "ServiceDesk request timed out"
	} else if errors.Is(err, context.Canceled) {
		code = "canceled"
		message = "ServiceDesk request was canceled"
		retryable = false
	}
	return &APIError{Code: code, Retryable: retryable, Message: message, Cause: safeCause(err, secret)}
}

type sanitizedError struct {
	message  string
	identity error
}

func (e sanitizedError) Error() string {
	return e.message
}

func (e sanitizedError) Unwrap() error {
	return e.identity
}

func safeCause(err error, secret string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if secret != "" {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	var identity error
	if errors.Is(err, context.DeadlineExceeded) {
		identity = context.DeadlineExceeded
	} else if errors.Is(err, context.Canceled) {
		identity = context.Canceled
	}
	return sanitizedError{message: message, identity: identity}
}
