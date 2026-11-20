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
		if operation == "create" && (apiErr.Code == "timeout" || apiErr.Code == "network_error") {
			return &DomainError{Code: "unknown_outcome", Message: "ServiceDesk create outcome is unknown after request dispatch", Action: "reconcile the ticket by title and request time before retrying", Retryable: false, Outcome: "unknown"}
		}
		return &DomainError{Code: apiErr.Code, Message: apiErr.Message, Action: actionForCode(apiErr.Code), Retryable: apiErr.Retryable}
	}
	if operation == "create" && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		return &DomainError{Code: "unknown_outcome", Message: "ServiceDesk create outcome is unknown after request dispatch", Action: "reconcile the ticket by title and request time before retrying", Retryable: false, Outcome: "unknown"}
	}
	return &DomainError{Code: "protocol_error", Message: "ServiceDesk returned an unexpected response", Action: "inspect the ServiceDesk response and retry only after the cause is understood", Retryable: false}
}

func actionForCode(code string) string {
	switch code {
	case "authentication_failed":
		return "refresh the ServiceDesk API Key"
	case "permission_denied":
		return "use a credential with access to this business"
	case "invalid_request", "validation_failed":
		return "correct the request and run dry-run again"
	case "registry_error", "profile_invalid", "stale_registry", "destructive_drift":
		return "refresh or revalidate the ServiceDesk profile"
	default:
		return "inspect the error and retry only when it is safe"
	}
}
