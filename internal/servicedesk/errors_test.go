package servicedesk

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyErrorHasStableCodeRetryableAndAction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     error
		code      string
		retryable bool
	}{
		{name: "auth", input: &APIError{Code: "authentication_failed", Message: "rejected"}, code: "authentication_failed"},
		{name: "permission", input: &APIError{Code: "permission_denied", Message: "denied"}, code: "permission_denied"},
		{name: "timeout", input: &APIError{Code: "timeout", Message: "timed out"}, code: "unknown_outcome", retryable: false},
		{name: "context", input: context.DeadlineExceeded, code: "unknown_outcome", retryable: false},
		{name: "validation", input: &APIError{Code: "invalid_request", Message: "invalid"}, code: "invalid_request"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyError("create", test.input)
			if got == nil || got.Code != test.code || got.Retryable != test.retryable || got.Action == "" {
				t.Fatalf("ClassifyError() = %+v, want code=%q retryable=%v with action", got, test.code, test.retryable)
			}
		})
	}
}

func TestDomainErrorUnwrapDoesNotExposeRawCredential(t *testing.T) {
	t.Parallel()
	secret := testAPIKey
	err := ClassifyError("create", &APIError{Code: "protocol_error", Message: "safe", Cause: errors.New(secret)})
	if err == nil || err.Error() == secret {
		t.Fatalf("error exposed credential: %+v", err)
	}
}
