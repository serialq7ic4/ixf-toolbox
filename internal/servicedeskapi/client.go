package servicedeskapi

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
	Origin           = "https://servicedesk.iflytek.com"
	MaxResponseBytes = 8 << 20
	defaultTimeout   = 30 * time.Second
)

type Options struct {
	HTTPClient *http.Client
	Transport  http.RoundTripper
}

type Client struct {
	apiKey     string
	httpClient *http.Client
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

type Service struct {
	ID         int    `json:"id"`
	BusinessID int    `json:"business_id"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	SnapshotID string `json:"snapshot_id"`
}

type Lading map[string]json.RawMessage

func NewClient(apiKey string, options Options) *Client {
	httpClient := directHTTPClient(defaultTimeout)
	if options.HTTPClient != nil {
		clone := *options.HTTPClient
		httpClient = &clone
		if httpClient.Transport == nil {
			httpClient.Transport = directHTTPClient(defaultTimeout).Transport
		}
	}
	if options.Transport != nil {
		httpClient.Transport = options.Transport
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{apiKey: apiKey, httpClient: httpClient}
}

func directHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Timeout: timeout, Transport: transport}
}

func (c *Client) Service(ctx context.Context, businessID, serviceID int) (Service, error) {
	var response Service
	if serviceID <= 0 {
		return response, InvalidRequest("ServiceDesk service ID must be positive")
	}
	path := "/api/service/v3/service/" + strconv.Itoa(serviceID) + "/"
	err := c.Request(ctx, http.MethodGet, path, nil, businessID, nil, &response, nil, true, true)
	return response, err
}

func (c *Client) Lading(ctx context.Context, businessID, serviceID int, snapshotID string) (Lading, error) {
	if serviceID <= 0 {
		return nil, InvalidRequest("ServiceDesk service ID must be positive")
	}
	if strings.TrimSpace(snapshotID) == "" {
		return nil, InvalidRequest("ServiceDesk snapshot ID is required")
	}
	query := url.Values{
		"service_id":  {strconv.Itoa(serviceID)},
		"snapshot_id": {snapshotID},
	}
	var response Lading
	err := c.Request(ctx, http.MethodGet, "/api/tasktmpl3/v3/lading/", query, businessID, nil, &response, nil, true, true)
	return response, err
}

func (c *Client) Request(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	businessID int,
	payload any,
	dataTarget any,
	metaTarget any,
	retryable bool,
	requireBusiness bool,
) error {
	if requireBusiness && businessID <= 0 {
		return InvalidRequest("ServiceDesk business ID must be positive")
	}
	if businessID < 0 {
		return InvalidRequest("ServiceDesk business ID must not be negative")
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &APIError{Code: "invalid_request", Message: "ServiceDesk request could not be encoded", Cause: safeCause(err, c.apiKey)}
		}
		body = bytes.NewReader(encoded)
	}
	requestURL := Origin + path
	if len(query) != 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return &APIError{Code: "invalid_request", Message: "ServiceDesk request could not be created", Cause: safeCause(err, c.apiKey)}
	}
	if businessID > 0 {
		request.Header.Set("businessId", strconv.Itoa(businessID))
	}
	request.Header.Set("apikey", c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return transportError(err, retryable, c.apiKey)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		code, message := statusError(response.StatusCode)
		return &APIError{Code: code, Status: response.StatusCode, Retryable: retryable && (response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError), Message: message}
	}
	raw, err := readBoundedResponse(response.Body)
	if err != nil {
		if errors.Is(err, errResponseTooLarge) {
			return &APIError{Code: "response_too_large", Status: response.StatusCode, Message: "ServiceDesk response exceeded the size limit"}
		}
		return &APIError{Code: "invalid_response", Status: response.StatusCode, Message: "ServiceDesk response could not be read", Cause: safeCause(err, c.apiKey)}
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

func InvalidRequest(message string) *APIError {
	return &APIError{Code: "invalid_request", Message: message}
}

type responseEnvelope struct {
	Result *bool           `json:"result"`
	Data   json.RawMessage `json:"data"`
	Meta   json.RawMessage `json:"meta"`
}

var errResponseTooLarge = errors.New("ServiceDesk response too large")

func readBoundedResponse(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxResponseBytes {
		return nil, errResponseTooLarge
	}
	return raw, nil
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

func (e sanitizedError) Error() string { return e.message }
func (e sanitizedError) Unwrap() error { return e.identity }

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
