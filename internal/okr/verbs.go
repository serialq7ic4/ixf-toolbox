package okr

import "fmt"

// VerbConfig is the common input for every intent-named OKR write verb.
type VerbConfig struct {
	URL         string
	CookiesPath string
	CSRFURL     string
	// Objective is the 1-based index of the target objective.
	Objective int
	// ExpectTitle is the title the caller believes sits at that index. It is
	// required for verbs that target an existing objective.
	ExpectTitle string
	// Texts are the KR texts the verb operates on, or the objective title for
	// retitle.
	Texts []string
	Title string
	// ConfirmKRDeletes must equal the number of KRs a destructive verb will
	// remove. It is the caller restating the blast radius from a value only this
	// target's current state can supply.
	ConfirmKRDeletes int
	Confirmed        bool
	Apply            bool
}

// AddKRs appends KRs to an objective without removing any existing ones.
//
// This is the non-destructive way to grow an objective. It exists as its own verb
// because the alternative is expressing an addition as a full replacement, which
// would route a caller through a destructive operation to do a safe thing.
func AddKRs(config VerbConfig) (map[string]any, error) {
	if len(config.Texts) == 0 {
		return nil, fmt.Errorf("kr add requires at least one KR text")
	}
	session, err := openSession(config.URL, config.CookiesPath, config.CSRFURL)
	if err != nil {
		return nil, err
	}
	target, err := session.resolveObjective(config.Objective, config.ExpectTitle)
	if err != nil {
		return nil, err
	}

	existingTexts := krTexts(target.KRs)
	additions := []string{}
	alreadyPresent := []string{}
	for _, text := range config.Texts {
		if containsText(existingTexts, text) || containsText(additions, text) {
			alreadyPresent = append(alreadyPresent, text)
			continue
		}
		additions = append(additions, text)
	}

	resulting := len(target.KRs) + len(additions)
	payload := map[string]any{
		"ok":          true,
		"operation":   "okr_kr_add",
		"destructive": false,
		"okrId":       session.okrID,
		"target": map[string]any{
			"objectiveIndex":          config.Objective,
			"objectiveId":             target.ID,
			"resolvedTitle":           target.Objective,
			"expectedTitle":           config.ExpectTitle,
			"titleMatchesExpectation": true,
		},
		"current": map[string]any{"krCount": len(target.KRs), "krs": existingTexts},
		"diff": map[string]any{
			"krsToCreate":      len(additions),
			"krsToDelete":      0,
			"resultingKrCount": resulting,
			"alreadyPresent":   alreadyPresent,
		},
	}

	// The ceiling is a property of the objective, so it has to be checked against
	// the live count rather than against the input alone.
	if resulting > maxKRsPerObjective {
		return nil, fmt.Errorf(
			"objective %d already has %d KRs; adding %d would reach %d and the limit is %d",
			config.Objective, len(target.KRs), len(additions), resulting, maxKRsPerObjective)
	}
	if len(additions) == 0 {
		payload["willWrite"] = false
		payload["dryRun"] = !config.Apply
		payload["note"] = "every KR given is already present on this objective; nothing to add"
		return payload, nil
	}
	if !config.Apply {
		payload["dryRun"] = true
		payload["willWrite"] = false
		return payload, nil
	}

	if err := session.enableDraft(target.ID); err != nil {
		return nil, err
	}
	created, err := session.createKRs(target.ID, additions)
	if err != nil {
		return nil, err
	}
	// Existing KRs keep their identifiers and stay ahead of the new ones.
	order := append(krIDs(target.KRs), created...)
	if err := session.order(target.ID, order); err != nil {
		return nil, err
	}
	if err := session.publish(target.ID, nil); err != nil {
		return nil, err
	}

	payload["dryRun"] = false
	payload["willWrite"] = true
	verify, err := session.verifyObjectiveKRs(config.Objective, append(existingTexts, additions...))
	if err != nil {
		return nil, err
	}
	payload["verify"] = verify
	payload["ok"] = asBoolValue(verify["ok"])
	return payload, nil
}

// verifyObjectiveKRs re-reads the page and compares one objective's stored KRs
// against what the verb intended them to be.
//
// The comparison is against the intended set, not against values read back out of
// the stored state. Deriving the expectation from the state being checked is what
// made the previous verification tautological.
func (session *okrSession) verifyObjectiveKRs(index int, expected []string) (map[string]any, error) {
	if err := session.reload(); err != nil {
		return nil, err
	}
	items := session.objectives()
	if index > len(items) {
		return map[string]any{
			"ok":    false,
			"error": fmt.Sprintf("objective %d no longer exists after writing", index),
		}, nil
	}
	stored := krTexts(items[index-1].KRs)
	result := map[string]any{
		"comparedAgainst": "intended KR set",
		"krsStored":       len(stored),
		"krsExpected":     len(expected),
		"scope":           "post-publish read-back: this objective's KR texts and order match what was intended; it does not confirm other objectives were untouched",
	}
	if len(stored) != len(expected) {
		result["ok"] = false
		result["error"] = fmt.Sprintf("objective %d has %d KRs after writing but %d were intended", index, len(stored), len(expected))
		return result, nil
	}
	for position, text := range expected {
		if stored[position] != text {
			result["ok"] = false
			result["error"] = fmt.Sprintf("KR %d does not match what was intended", position+1)
			return result, nil
		}
	}
	result["ok"] = true
	return result, nil
}

func krTexts(krs []krState) []string {
	texts := make([]string, 0, len(krs))
	for _, kr := range krs {
		texts = append(texts, kr.Text)
	}
	return texts
}

func krIDs(krs []krState) []string {
	ids := make([]string, 0, len(krs))
	for _, kr := range krs {
		ids = append(ids, kr.ID)
	}
	return ids
}

func containsText(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func asBoolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

// RetitleObjective rewrites one objective's title and never touches its KRs.
//
// Separated from any KR operation so that changing wording cannot be the thing
// that also rewrites content. The payload states krsUntouched for the same reason.
func RetitleObjective(config VerbConfig) (map[string]any, error) {
	if config.Title == "" {
		return nil, fmt.Errorf("objective retitle requires --title")
	}
	session, err := openSession(config.URL, config.CookiesPath, config.CSRFURL)
	if err != nil {
		return nil, err
	}
	target, err := session.resolveObjective(config.Objective, config.ExpectTitle)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"ok":          true,
		"operation":   "okr_objective_retitle",
		"destructive": false,
		"okrId":       session.okrID,
		"target": map[string]any{
			"objectiveIndex":          config.Objective,
			"objectiveId":             target.ID,
			"resolvedTitle":           target.Objective,
			"expectedTitle":           config.ExpectTitle,
			"titleMatchesExpectation": true,
		},
		"currentTitle":   target.Objective,
		"plannedTitle":   config.Title,
		"titleChanged":   target.Objective != config.Title,
		"krsUntouched":   true,
		"currentKrCount": len(target.KRs),
	}
	if target.Objective == config.Title {
		payload["dryRun"] = !config.Apply
		payload["willWrite"] = false
		payload["note"] = "the objective already has this title; nothing to write"
		return payload, nil
	}
	if !config.Apply {
		payload["dryRun"] = true
		payload["willWrite"] = false
		return payload, nil
	}
	if err := session.enableDraft(target.ID); err != nil {
		return nil, err
	}
	if err := session.setObjectiveTitle(target.ID, config.Title); err != nil {
		return nil, err
	}
	if err := session.publish(target.ID, nil); err != nil {
		return nil, err
	}
	if err := session.reload(); err != nil {
		return nil, err
	}
	items := session.objectives()
	verify := map[string]any{
		"comparedAgainst": "intended title",
		"scope":           "post-publish read-back: this objective's title and KR count are unchanged from what was intended; it does not confirm other objectives were untouched",
	}
	switch {
	case config.Objective > len(items):
		verify["ok"] = false
		verify["error"] = "the objective no longer exists after writing"
	case items[config.Objective-1].Objective != config.Title:
		verify["ok"] = false
		verify["error"] = "the stored title does not match the intended title"
	case len(items[config.Objective-1].KRs) != len(target.KRs):
		verify["ok"] = false
		verify["error"] = fmt.Sprintf("KR count changed from %d to %d during a retitle",
			len(target.KRs), len(items[config.Objective-1].KRs))
	default:
		verify["ok"] = true
	}
	payload["dryRun"] = false
	payload["willWrite"] = true
	payload["verify"] = verify
	payload["ok"] = asBoolValue(verify["ok"])
	return payload, nil
}

// CreateObjective appends a new objective with its KRs.
//
// It refuses to target an existing objective at all, which is what separates it
// from the old behaviour where one index meant create or replace depending on a
// remote count the caller could not see.
func CreateObjective(config VerbConfig) (map[string]any, error) {
	if config.Title == "" {
		return nil, fmt.Errorf("objective create requires --title")
	}
	if len(config.Texts) == 0 {
		return nil, fmt.Errorf("objective create requires at least one KR; use --kr")
	}
	if len(config.Texts) > maxKRsPerObjective {
		return nil, fmt.Errorf("objective create was given %d KRs; the limit is %d",
			len(config.Texts), maxKRsPerObjective)
	}
	session, err := openSession(config.URL, config.CookiesPath, config.CSRFURL)
	if err != nil {
		return nil, err
	}
	existing := session.objectives()
	appendIndex := len(existing) + 1
	if config.Objective != 0 && config.Objective != appendIndex {
		return nil, fmt.Errorf(
			"objective create appends at index %d, but --objective %d was given; "+
				"to change an existing objective use `ixf okr objective retitle` or `ixf okr kr replace`",
			appendIndex, config.Objective)
	}
	payload := map[string]any{
		"ok":                     true,
		"operation":              "okr_objective_create",
		"destructive":            false,
		"okrId":                  session.okrID,
		"willAppendAtIndex":      appendIndex,
		"existingObjectiveCount": len(existing),
		"plannedTitle":           config.Title,
		"plannedKrCount":         len(config.Texts),
		"plannedKrs":             config.Texts,
	}
	if !config.Apply {
		payload["dryRun"] = true
		payload["willWrite"] = false
		return payload, nil
	}
	objectiveID, err := session.createObjectiveWithTitle(config.Title)
	if err != nil {
		return nil, err
	}
	created, err := session.createKRs(objectiveID, config.Texts)
	if err != nil {
		return nil, err
	}
	if err := session.order(objectiveID, created); err != nil {
		return nil, err
	}
	if err := session.publish(objectiveID, nil); err != nil {
		return nil, err
	}
	payload["dryRun"] = false
	payload["willWrite"] = true
	payload["objectiveId"] = objectiveID
	verify, err := session.verifyObjectiveKRs(appendIndex, config.Texts)
	if err != nil {
		return nil, err
	}
	payload["verify"] = verify
	payload["ok"] = asBoolValue(verify["ok"])
	return payload, nil
}
