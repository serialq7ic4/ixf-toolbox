package servicedesk

import (
	"errors"
	"testing"
)

func TestProfileBridgeRejectsNilClient(t *testing.T) {
	_, loadErr := LoadTemplate(t.Context(), nil, EditTarget{})
	_, inspectErr := Inspect(t.Context(), nil, InspectRequest{})
	for _, err := range []error{loadErr, inspectErr} {
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "invalid_request" {
			t.Fatalf("expected invalid_request, got %v", err)
		}
	}
}
