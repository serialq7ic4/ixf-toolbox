package servicedesk

import (
	"context"
	"errors"
)

func ClassifyError(operation string, err error) *DomainError {
	if err == nil {
		return nil
	}
	if domainErr, ok := err.(*DomainError); ok {
		return domainErr
	}
	if apiErr, ok := err.(*APIError); ok {
		if operation == "create" && uncertainCreateResponse(apiErr) {
			return &DomainError{Code: "unknown_outcome", Message: "ServiceDesk create outcome is unknown after request dispatch", Action: "reconcile the ticket by title and request time before retrying", Retryable: false, Outcome: "unknown"}
		}
		return &DomainError{Code: apiErr.Code, Message: apiErr.Message, Action: actionForCode(apiErr.Code), Retryable: apiErr.Retryable}
	}
	if operation == "create" && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		return &DomainError{Code: "unknown_outcome", Message: "ServiceDesk create outcome is unknown after request dispatch", Action: "reconcile the ticket by title and request time before retrying", Retryable: false, Outcome: "unknown"}
	}
	return &DomainError{Code: "protocol_error", Message: "ServiceDesk returned an unexpected response", Action: "inspect the ServiceDesk response and retry only after the cause is understood", Retryable: false}
}

func uncertainCreateResponse(err *APIError) bool {
	switch err.Code {
	case "timeout", "network_error", "canceled", "invalid_response", "response_too_large":
		return true
	case "http_error":
		return err.Status >= 500 || err.Status >= 200 && err.Status < 300
	default:
		return false
	}
}

func actionForCode(code string) string {
	switch code {
	case "authentication_failed":
		return "refresh the ServiceDesk API Key"
	case "permission_denied":
		return "use a credential with access to this business"
	case "invalid_request", "validation_failed":
		return "correct the request and run dry-run again"
	case "profile_required", "profile_not_found":
		return "run ixf servicedesk profile list --json"
	case "destructive_drift":
		return "review the live form and update the profile in ixf-toolbox"
	case "query_failed":
		return "verify ServiceDesk access and retry the read-only query"
	default:
		return "inspect the error and retry only when it is safe"
	}
}
