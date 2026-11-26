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

	sdp "github.com/serialq7ic4/ixf-toolbox/pkg/servicedeskprofile"
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
	metadata   *sdp.MetadataClient
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
	BusinessID int    `json:"business_id,omitempty"`
}

func (response *StartResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		ID         json.RawMessage `json:"id"`
		InstanceID json.RawMessage `json:"instance_id"`
		BusinessID json.RawMessage `json:"business_id"`
		Name       string          `json:"name"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var instanceID int
	for _, raw := range []json.RawMessage{wire.ID, wire.InstanceID} {
		if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		value, err := decodeTicketInstanceID(raw)
		if err != nil {
			return err
		}
		id, err := parsePositiveID(value)
		if err != nil {
			return err
		}
		if instanceID != 0 && instanceID != id {
			return errors.New("ServiceDesk returned conflicting ticket IDs")
		}
		instanceID = id
	}
	if instanceID == 0 {
		return errors.New("ServiceDesk did not return a ticket ID")
	}
	business, _ := decodeTicketInstanceID(wire.BusinessID)
	businessID, _ := parsePositiveID(business)
	*response = StartResponse{InstanceID: strconv.Itoa(instanceID), Name: wire.Name, BusinessID: businessID}
	return nil
}

type ListQuery struct {
	FlowType string
	Limit    int
	Offset   int
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
	DetailID          string         `json:"id,omitempty"`
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

func (response *TicketResponse) UnmarshalJSON(data []byte) error {
	type ticketResponseAlias TicketResponse
	var wire struct {
		ticketResponseAlias
		DetailID   json.RawMessage `json:"id"`
		InstanceID json.RawMessage `json:"instance_id"`
		Step       json.RawMessage `json:"step"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	instanceID, err := decodeTicketInstanceID(wire.InstanceID)
	if err != nil {
		return fmt.Errorf("decode ServiceDesk ticket instance_id: %w", err)
	}
	step, err := decodeTicketStep(wire.Step)
	if err != nil {
		return fmt.Errorf("decode ServiceDesk ticket step: %w", err)
	}
	*response = TicketResponse(wire.ticketResponseAlias)
	response.DetailID, _ = decodeTicketInstanceID(wire.DetailID)
	response.InstanceID = instanceID
	response.Step = step
	return nil
}

func (response TicketResponse) detailRecordID() string {
	if response.DetailID != "" {
		return response.DetailID
	}
	return response.InstanceID
}

func decodeTicketInstanceID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", errors.New("must be a positive integer")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if !isPositiveDecimal(text) {
			return "", errors.New("must be a positive integer")
		}
		return text, nil
	}
	number := string(bytes.TrimSpace(raw))
	if !isPositiveDecimal(number) {
		return "", errors.New("must be a positive integer")
	}
	return number, nil
}

func isPositiveDecimal(value string) bool {
	nonzero := false
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
		if digit != '0' {
			nonzero = true
		}
	}
	return nonzero
}

func decodeTicketStep(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var rawValues []json.RawMessage
	if err := json.Unmarshal(raw, &rawValues); err != nil {
		return "", errors.New("must be a string or string array")
	}
	values := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		if bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
			return "", errors.New("must be a string or string array")
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return "", errors.New("must be a string or string array")
		}
		values = append(values, value)
	}
	return strings.Join(values, " / "), nil
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
	return &Client{
		credential: credential,
		httpClient: httpClient,
		now:        now,
		metadata: sdp.NewMetadataClient(credential.APIKey, sdp.ClientOptions{
			HTTPClient: httpClient,
		}),
	}
}

func newDirectHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Timeout: timeout, Transport: transport}
}

func (c *Client) UserInfo(ctx context.Context, businessID int) (UserInfo, error) {
	var response UserInfo
	if businessID < 0 {
		return response, invalidRequest("ServiceDesk business ID must not be negative")
	}
	var err error
	if businessID == 0 {
		err = c.doRequest(ctx, http.MethodGet, "/api/user/v3/userinfo/", nil, 0, nil, &response, nil, true)
	} else {
		err = c.do(ctx, http.MethodGet, "/api/user/v3/userinfo/", nil, businessID, nil, &response, nil, true)
	}
	return response, err
}

func (c *Client) Service(ctx context.Context, target EditTarget) (Service, error) {
	return c.metadata.Service(ctx, target)
}

func (c *Client) Lading(ctx context.Context, target EditTarget, snapshotID string) (Lading, error) {
	return c.metadata.Lading(ctx, target, snapshotID)
}

func (c *Client) dictionaryChoices(ctx context.Context, businessID, dictionaryID int) ([]Choice, error) {
	var response struct {
		Children []struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"children"`
	}
	path := "/api/workflow/v3/sys_dict/" + strconv.Itoa(dictionaryID) + "/"
	if err := c.do(ctx, http.MethodGet, path, nil, businessID, nil, &response, nil, true); err != nil {
		return nil, err
	}
	choices := make([]Choice, 0, len(response.Children))
	for _, entry := range response.Children {
		if strings.TrimSpace(entry.Key) == "" || strings.TrimSpace(entry.Name) == "" {
			return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk dictionary returned an incomplete option"}
		}
		choices = append(choices, Choice{Label: entry.Name, Value: entry.Key})
	}
	if len(choices) == 0 {
		return nil, &APIError{Code: "invalid_response", Message: "ServiceDesk dictionary returned no options"}
	}
	return choices, nil
}

func (c *Client) Start(ctx context.Context, businessID int, payload StartPayload) (StartResponse, error) {
	var response StartResponse
	err := c.do(ctx, http.MethodPost, "/api/tasktmpl3/v3/start/", nil, businessID, payload, &response, nil, false)
	return response, err
}

func (c *Client) List(ctx context.Context, businessID int, query ListQuery) (TicketPage, error) {
	if query.FlowType != "apply" && query.FlowType != "wait" {
		return TicketPage{}, invalidRequest("ServiceDesk list flow type must be apply or wait")
	}
	if query.Limit <= 0 || query.Offset < 0 {
		return TicketPage{}, invalidRequest("ServiceDesk list pagination must use a positive limit and nonnegative offset")
	}
	values := url.Values{
		"key":       {"request"},
		"flow_type": {query.FlowType},
		"limit":     {strconv.Itoa(query.Limit)},
		"offset":    {strconv.Itoa(query.Offset)},
	}
	var response TicketPage
	err := c.do(ctx, http.MethodGet, "/api/taskflow3/v3/taskflow/", values, businessID, nil, &response.Data, &response.Meta, true)
	if err != nil {
		return response, err
	}
	if response.Meta.Limit <= 0 || response.Meta.Offset < 0 || response.Meta.TotalCount < 0 {
		return TicketPage{}, &APIError{Code: "invalid_response", Status: http.StatusOK, Message: "ServiceDesk pagination metadata is incomplete or inconsistent"}
	}
	hasNext := response.Meta.Next != nil && strings.TrimSpace(*response.Meta.Next) != ""
	if !hasNext && response.Meta.Offset+len(response.Data) < response.Meta.TotalCount {
		return TicketPage{}, &APIError{Code: "invalid_response", Status: http.StatusOK, Message: "ServiceDesk pagination ended before the advertised total count"}
	}
	for _, ticket := range response.Data {
		if ticket.BusinessID <= 0 {
			return TicketPage{}, &APIError{Code: "invalid_response", Status: http.StatusOK, Message: "ServiceDesk ticket record is missing a valid business ID"}
		}
	}
	return response, nil
}

func (c *Client) Get(ctx context.Context, target DetailTarget) (TicketResponse, error) {
	response, err := c.getTicketDetail(ctx, target)
	if err == nil {
		return response, nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "permission_denied" {
		return response, err
	}

	visible, found, fallbackErr := c.findVisibleTicket(ctx, target)
	if fallbackErr != nil {
		return response, fallbackErr
	}
	if found {
		return visible, nil
	}
	return response, err
}

func (c *Client) getTicketDetail(ctx context.Context, target DetailTarget) (TicketResponse, error) {
	var response TicketResponse
	if _, err := parsePositiveID(target.TicketID); err != nil {
		return response, invalidRequest("ServiceDesk ticket ID must be a positive integer")
	}
	path := "/api/taskflow3/v3/taskflow/" + target.TicketID + "/"
	err := c.do(ctx, http.MethodGet, path, nil, target.BusinessID, nil, &response, nil, true)
	return response, err
}

func (c *Client) findVisibleTicket(ctx context.Context, target DetailTarget) (TicketResponse, bool, error) {
	for _, flowType := range []string{"apply", "wait"} {
		offset := 0
		for {
			page, err := c.List(ctx, target.BusinessID, ListQuery{FlowType: flowType, Limit: 100, Offset: offset})
			if err != nil {
				return TicketResponse{}, false, err
			}
			for _, ticket := range page.Data {
				if ticket.BusinessID == target.BusinessID && ticket.InstanceID == target.TicketID {
					return ticket, true, nil
				}
			}
			if page.Meta.Next == nil || strings.TrimSpace(*page.Meta.Next) == "" {
				break
			}
			next := page.Meta.Offset + page.Meta.Limit
			if next <= offset {
				next = offset + len(page.Data)
			}
			if next <= offset {
				return TicketResponse{}, false, &APIError{Code: "invalid_response", Status: http.StatusOK, Message: "ServiceDesk pagination did not advance"}
			}
			offset = next
		}
	}
	return TicketResponse{}, false, nil
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
	return c.doRequest(ctx, method, path, query, businessID, payload, dataTarget, metaTarget, retryable)
}

func (c *Client) doRequest(
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
	if businessID > 0 {
		request.Header.Set("businessId", strconv.Itoa(businessID))
	}
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
