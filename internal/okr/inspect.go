package okr

import (
	"fmt"
	"net/http"
	"time"
)

// Inspect reports the current objectives of an OKR page as machine-readable JSON.
//
// It exists because the write path addresses objectives positionally, so a caller
// has to know what is at a position before writing to it. `okr read` renders
// Markdown, which a human can check but an agent cannot reliably parse into
// indexes, identifiers, and KR counts. Every destructive write should be preceded
// by a read of this, and the values it reports are what a confirmation flag can be
// checked against.
func Inspect(config ReadConfig) (map[string]any, error) {
	if !DetectURL(config.Source) {
		return nil, fmt.Errorf("source is not an OKR page URL")
	}
	okrID, err := IDFromURL(config.Source)
	if err != nil {
		return nil, err
	}
	origin, err := OriginFor(config.Source)
	if err != nil {
		return nil, err
	}
	cookies, err := loadCookieObjects(config.CookiesPath)
	if err != nil {
		return nil, err
	}
	csrfURL := config.CSRFURL
	if csrfURL == "" {
		csrfURL = DefaultCSRFURL
	}
	client := &http.Client{Timeout: 30 * time.Second}
	lgwToken, cookies, err := ensureLGWCSRFToken(client, csrfURL, cookies)
	if err != nil {
		return nil, err
	}
	detail, err := getDetail(client, origin, config.Source, okrID, lgwToken, cookies)
	if err != nil {
		return nil, err
	}
	return inspectPayload(okrID, summarizeObjectives(detail)), nil
}

// inspectPayload shapes the report. Indexes are 1-based to match --objective-index.
func inspectPayload(okrID string, state []objectiveState) map[string]any {
	objectives := make([]map[string]any, 0, len(state))
	totalKRs := 0
	for index, item := range state {
		krs := make([]map[string]any, 0, len(item.KRs))
		for krIndex, kr := range item.KRs {
			krs = append(krs, map[string]any{
				"index": krIndex + 1,
				"id":    kr.ID,
				"text":  kr.Text,
			})
		}
		totalKRs += len(item.KRs)
		objectives = append(objectives, map[string]any{
			"index":       index + 1,
			"id":          item.ID,
			"objective":   item.Objective,
			"krCount":     len(item.KRs),
			"krs":         krs,
			"krCapacity":  maxKRsPerObjective,
			"krRemaining": positiveOrZero(maxKRsPerObjective - len(item.KRs)),
		})
	}
	return map[string]any{
		"ok":              true,
		"operation":       "inspect_okr",
		"okrId":           okrID,
		"objectiveCount":  len(state),
		"krCount":         totalKRs,
		"objectives":      objectives,
		"maxKRsPerObject": maxKRsPerObjective,
		// The index one past the end creates a new objective rather than replacing
		// one, and which of those a given index means depends on this count. Naming
		// it here means a caller does not have to derive it.
		"nextObjectiveIndex": len(state) + 1,
	}
}

func positiveOrZero(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
