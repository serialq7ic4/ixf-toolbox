package servicedeskprofile

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const serviceDeskHost = "servicedesk.iflytek.com"

type EditTarget struct {
	BusinessID int
	ServiceID  int
}

func ParseEditURL(raw string) (EditTarget, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL is empty or has surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL cannot be parsed")
	}
	if parsed.Scheme != "https" || parsed.Host != serviceDeskHost || parsed.User != nil {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL must use the fixed ServiceDesk HTTPS origin")
	}
	if parsed.Opaque != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL contains unsupported data before the hash path")
	}
	if parsed.Fragment == "" || parsed.RawFragment != "" {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL must contain an unescaped ServiceDesk hash path")
	}
	fragment, err := url.Parse(parsed.Fragment)
	if err != nil || fragment.Scheme != "" || fragment.Host != "" || fragment.User != nil || fragment.Fragment != "" {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL hash path is malformed")
	}
	const prefix = "/order/order-edit/"
	if !strings.HasPrefix(fragment.Path, prefix) {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL hash path targets the wrong endpoint")
	}
	businessRaw := strings.TrimPrefix(fragment.Path, prefix)
	if businessRaw == "" || strings.Contains(businessRaw, "/") {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL hash path must end with one business ID")
	}
	businessID, err := positiveID(businessRaw)
	if err != nil {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: business ID must be a positive integer")
	}
	query, err := url.ParseQuery(fragment.RawQuery)
	if err != nil || len(query) != 1 {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL hash query must contain only the required ID")
	}
	values, ok := query["service_id"]
	if !ok || len(values) != 1 {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: URL hash query must contain exactly one service_id")
	}
	serviceID, err := positiveID(values[0])
	if err != nil {
		return EditTarget{}, errors.New("invalid ServiceDesk edit URL: service_id must be a positive integer")
	}
	return EditTarget{BusinessID: businessID, ServiceID: serviceID}, nil
}

func (t EditTarget) URL() string {
	return fmt.Sprintf("https://%s/#/order/order-edit/%d?service_id=%d", serviceDeskHost, t.BusinessID, t.ServiceID)
}

func positiveID(raw string) (int, error) {
	if raw == "" || strings.HasPrefix(raw, "+") || (len(raw) > 1 && raw[0] == '0') {
		return 0, errors.New("invalid positive integer")
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errors.New("invalid positive integer")
	}
	return value, nil
}
