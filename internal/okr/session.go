package okr

import (
	"fmt"
	"net/http"
	"time"
)

// okrSession carries the request context every OKR operation needs: an
// authenticated client, the resolved origin, and the page's current state.
//
// The same prologue was repeated in each entry point, which made adding an
// operation mean copying authentication rather than describing the operation. It
// also meant the state fetch lived only in the apply paths, so a dry run had no
// access to the target and could only describe its own input.
type okrSession struct {
	client   *http.Client
	origin   string
	url      string
	okrID    string
	lgwToken string
	cookies  []http.Cookie
	detail   map[string]any
	// connID identifies this run to the collaborative editor. One value per
	// session keeps a multi-step operation attributable to a single actor.
	connID string
	cache  *draftVersionCache
}

// openSession authenticates and reads the page once.
func openSession(url string, cookiesPath string, csrfURL string) (*okrSession, error) {
	okrID, err := IDFromURL(url)
	if err != nil {
		return nil, err
	}
	origin, err := OriginFor(url)
	if err != nil {
		return nil, err
	}
	cookies, err := loadCookieObjects(cookiesPath)
	if err != nil {
		return nil, err
	}
	if csrfURL == "" {
		csrfURL = DefaultCSRFURL
	}
	client := &http.Client{Timeout: 30 * time.Second}
	lgwToken, cookies, err := ensureLGWCSRFToken(client, csrfURL, cookies)
	if err != nil {
		return nil, err
	}
	detail, err := getDetail(client, origin, url, okrID, lgwToken, cookies)
	if err != nil {
		return nil, err
	}
	return &okrSession{
		client:   client,
		origin:   origin,
		url:      url,
		okrID:    okrID,
		lgwToken: lgwToken,
		cookies:  cookies,
		detail:   detail,
		connID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		cache:    &draftVersionCache{},
	}, nil
}

// objectives reports the page's current objectives.
func (session *okrSession) objectives() []objectiveState {
	return summarizeObjectives(session.detail)
}

// reload re-reads the page, for verifying after a write.
func (session *okrSession) reload() error {
	detail, err := getDetail(session.client, session.origin, session.url, session.okrID, session.lgwToken, session.cookies)
	if err != nil {
		return err
	}
	session.detail = detail
	return nil
}

// resolveObjective finds the objective a verb targets, by 1-based index, and
// asserts the caller's expectation of its title before anything is written.
//
// The expectation is what makes a positional target safe: indexes shift as
// objectives are added, so an index alone can silently name a different objective
// than the caller inspected. Comparing the title turns that into a refusal rather
// than a write to the wrong place.
func (session *okrSession) resolveObjective(index int, expectTitle string) (objectiveState, error) {
	items := session.objectives()
	if index <= 0 {
		return objectiveState{}, fmt.Errorf("--objective must be a positive 1-based index")
	}
	if index > len(items) {
		return objectiveState{}, fmt.Errorf(
			"--objective %d does not exist; the page has %d objective(s). Use `ixf okr inspect` to see them",
			index, len(items))
	}
	target := items[index-1]
	if expectTitle == "" {
		return objectiveState{}, fmt.Errorf(
			"--expect-title is required: it confirms which objective index %d refers to, "+
				"since indexes shift as objectives are added. Copy the title from `ixf okr inspect`", index)
	}
	if target.Objective != expectTitle {
		return objectiveState{}, fmt.Errorf(
			"objective %d is %q, not %q; refusing to write to an objective the caller did not expect",
			index, target.Objective, expectTitle)
	}
	return target, nil
}

// enableDraft opens the collaborative draft for one objective. Every write to an
// objective goes through a draft and then a publish.
func (session *okrSession) enableDraft(objectiveID string) error {
	_, err := okrAPIWithVersion(
		session.client, "POST", session.origin, session.url, session.okrID,
		"/okrx/api/draft_v2/enable/"+objectiveID+"/",
		session.lgwToken, session.cookies, session.cache, session.connID,
		func(version string, conn string) map[string]any {
			return draftBody(version, conn)
		})
	return err
}

// createKRs creates the given KR texts under an objective and reports their new
// identifiers in order.
func (session *okrSession) createKRs(objectiveID string, texts []string) ([]string, error) {
	created := make([]string, 0, len(texts))
	for _, text := range texts {
		krID, err := createKR(
			session.client, session.origin, session.url, session.okrID,
			session.lgwToken, session.cookies, session.cache, session.connID, objectiveID, text)
		if err != nil {
			return created, err
		}
		created = append(created, krID)
	}
	return created, nil
}

// deleteKRs removes the given KRs from the draft. The publish that follows commits
// the removal.
func (session *okrSession) deleteKRs(krIDs []string) error {
	for _, krID := range krIDs {
		if _, err := okrAPIWithVersionParams(
			session.client, "DELETE", session.origin, session.url, session.okrID,
			"/okrx/api/draft_v2/kr/"+krID+"/",
			session.lgwToken, session.cookies, session.cache, session.connID, deleteParams); err != nil {
			return err
		}
	}
	return nil
}

// order sets the KR order for an objective.
func (session *okrSession) order(objectiveID string, krIDs []string) error {
	_, err := orderKRs(
		session.client, session.origin, session.url, session.okrID,
		session.lgwToken, session.cookies, session.cache, session.connID, objectiveID, krIDs)
	return err
}

// publish commits the draft for one objective, including any KR deletions.
func (session *okrSession) publish(objectiveID string, deletedKRIDs []string) error {
	_, err := publishObjective(
		session.client, session.origin, session.url, session.okrID,
		session.lgwToken, session.cookies, session.cache, session.connID, objectiveID, deletedKRIDs)
	return err
}
