package servicedesk

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	serviceDeskOrigin = "https://servicedesk.iflytek.com"
	serviceDeskHost   = "servicedesk.iflytek.com"
)

type DetailTarget struct {
	BusinessID int
	TicketID   string
}

func ParseDetailURL(raw string) (DetailTarget, error) {
	businessID, ticketID, err := parseServiceDeskURL(raw, "/order/order-detail/", "id")
	if err != nil {
		return DetailTarget{}, fmt.Errorf("invalid ServiceDesk detail URL: %w", err)
	}
	return DetailTarget{BusinessID: businessID, TicketID: fmt.Sprintf("%d", ticketID)}, nil
}

func parseServiceDeskURL(raw string, pathPrefix string, queryKey string) (int, int, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return 0, 0, errors.New("URL is empty or has surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return 0, 0, errors.New("URL cannot be parsed")
	}
	if parsed.Scheme != "https" || parsed.Host != serviceDeskHost || parsed.User != nil {
		return 0, 0, errors.New("URL must use the fixed ServiceDesk HTTPS origin")
	}
	if parsed.Opaque != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery {
		return 0, 0, errors.New("URL contains unsupported data before the hash path")
	}
	if parsed.Fragment == "" || parsed.RawFragment != "" {
		return 0, 0, errors.New("URL must contain an unescaped ServiceDesk hash path")
	}

	fragment, err := url.Parse(parsed.Fragment)
	if err != nil || fragment.Scheme != "" || fragment.Host != "" || fragment.User != nil || fragment.Fragment != "" {
		return 0, 0, errors.New("URL hash path is malformed")
	}
	if !strings.HasPrefix(fragment.Path, pathPrefix) {
		return 0, 0, errors.New("URL hash path targets the wrong endpoint")
	}
	businessRaw := strings.TrimPrefix(fragment.Path, pathPrefix)
	if businessRaw == "" || strings.Contains(businessRaw, "/") {
		return 0, 0, errors.New("URL hash path must end with one business ID")
	}
	businessID, err := parsePositiveID(businessRaw)
	if err != nil {
		return 0, 0, errors.New("business ID must be a positive integer")
	}

	query, err := url.ParseQuery(fragment.RawQuery)
	if err != nil || len(query) != 1 {
		return 0, 0, errors.New("URL hash query must contain only the required ID")
	}
	values, ok := query[queryKey]
	if !ok || len(values) != 1 {
		return 0, 0, fmt.Errorf("URL hash query must contain exactly one %s", queryKey)
	}
	queryID, err := parsePositiveID(values[0])
	if err != nil {
		return 0, 0, fmt.Errorf("%s must be a positive integer", queryKey)
	}
	return businessID, queryID, nil
}
