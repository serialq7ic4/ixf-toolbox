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

// confirmDeletes checks the caller's restatement of the blast radius.
//
// --apply alone has been shown insufficient, because it is a constant an agent can
// learn to always pass. A count cannot be cargo-culted: the correct value exists
// only in this target's current state, so supplying it means the caller read the
// diff.
func confirmDeletes(given int, actual int, flag string) error {
	if given == actual {
		return nil
	}
	if given == 0 {
		return fmt.Errorf(
			"this will delete %d KR(s); pass %s %d to confirm that number. "+
				"Run the same command with --dry-run first and read diff.krsToDelete",
			actual, flag, actual)
	}
	return fmt.Errorf(
		"%s %d does not match the %d KR(s) this would delete; "+
			"the page may have changed since it was inspected, so re-run --dry-run before applying",
		flag, given, actual)
}

// ReplaceKRs replaces an objective's entire KR set in a single publish.
//
// Replacement inherently deletes, so this is a destructive verb and says so. It is
// deliberately not expressible as kr delete followed by kr add: decomposing it
// would make "objective with no KRs" a reachable documented state between two
// commands, which is the very outcome that made the original defect destructive.
// The editor commits deletions with the publish via need_delete_kr_ids, so one
// request covers both halves.
func ReplaceKRs(config VerbConfig) (map[string]any, error) {
	if len(config.Texts) == 0 {
		return nil, fmt.Errorf(
			"kr replace requires at least one KR; to remove KRs without replacing them use `ixf okr kr delete`")
	}
	if len(config.Texts) > maxKRsPerObjective {
		return nil, fmt.Errorf("kr replace was given %d KRs; the limit is %d", len(config.Texts), maxKRsPerObjective)
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
	payload := map[string]any{
		"ok":          true,
		"operation":   "okr_kr_replace",
		"mode":        "kr_replace",
		"destructive": true,
		"okrId":       session.okrID,
		"target": map[string]any{
			"objectiveIndex":          config.Objective,
			"objectiveId":             target.ID,
			"resolvedTitle":           target.Objective,
			"expectedTitle":           config.ExpectTitle,
			"titleMatchesExpectation": true,
		},
		"current": map[string]any{"krCount": len(target.KRs), "krs": existingTexts},
		"planned": map[string]any{"krCount": len(config.Texts), "krs": config.Texts},
		"diff": map[string]any{
			"krsToDelete":      len(target.KRs),
			"krsToCreate":      len(config.Texts),
			"resultingKrCount": len(config.Texts),
			"krsToDeleteTexts": existingTexts,
			"netKrChange":      len(config.Texts) - len(target.KRs),
		},
	}
	if !config.Apply {
		payload["dryRun"] = true
		payload["willWrite"] = false
		payload["apply"] = applyGate(config.ConfirmKRDeletes, len(target.KRs), "--confirm-kr-deletes")
		return payload, nil
	}
	if err := confirmDeletes(config.ConfirmKRDeletes, len(target.KRs), "--confirm-kr-deletes"); err != nil {
		return nil, err
	}

	if err := session.enableDraft(target.ID); err != nil {
		return nil, err
	}
	// Create before deleting, so a failure between the two leaves the original KRs
	// in place alongside orphaned drafts rather than an objective with none.
	created, err := session.createKRs(target.ID, config.Texts)
	if err != nil {
		return nil, err
	}
	if len(created) == 0 {
		return nil, fmt.Errorf("refusing to delete %d existing KRs with no replacement created", len(target.KRs))
	}
	if err := session.order(target.ID, created); err != nil {
		return nil, err
	}
	if err := session.publish(target.ID, krIDs(target.KRs)); err != nil {
		return nil, err
	}

	payload["dryRun"] = false
	payload["willWrite"] = true
	verify, err := session.verifyObjectiveKRs(config.Objective, config.Texts)
	if err != nil {
		return nil, err
	}
	payload["verify"] = verify
	payload["ok"] = asBoolValue(verify["ok"])
	return payload, nil
}

// applyGate states, in the dry run, whether apply would be refused and what would
// satisfy it. The decision lives in the tool rather than being left for the caller
// to infer from a diff.
func applyGate(given int, actual int, flag string) map[string]any {
	if err := confirmDeletes(given, actual, flag); err != nil {
		return map[string]any{
			"blocked":       true,
			"reasons":       []string{err.Error()},
			"requiredFlags": []string{fmt.Sprintf("%s %d", flag, actual)},
		}
	}
	return map[string]any{"blocked": false, "reasons": []string{}}
}

// DeleteKRs removes named KRs from an objective. This is the only path to an
// objective with no KRs, and it takes no --input: destructive intent belongs in the
// verb and its confirmation, never in a data file whose shape could be a mistake.
func DeleteKRs(config VerbConfig) (map[string]any, error) {
	if len(config.Texts) == 0 {
		return nil, fmt.Errorf("kr delete requires at least one --kr naming a KR to remove")
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
	matched := []krState{}
	unmatched := []string{}
	for _, text := range config.Texts {
		found := false
		for _, kr := range target.KRs {
			if kr.Text == text && !containsKR(matched, kr.ID) {
				matched = append(matched, kr)
				found = true
				break
			}
		}
		if !found {
			unmatched = append(unmatched, text)
		}
	}
	// A KR named but not present means the caller is working from stale state, so
	// the safe response is to refuse rather than delete the subset that did match.
	if len(unmatched) > 0 {
		return nil, fmt.Errorf(
			"%d of %d KRs named for deletion are not on objective %d; "+
				"re-run `ixf okr inspect` and copy the KR texts exactly. Nothing was deleted",
			len(unmatched), len(config.Texts), config.Objective)
	}

	remaining := []string{}
	for _, kr := range target.KRs {
		if !containsKR(matched, kr.ID) {
			remaining = append(remaining, kr.Text)
		}
	}
	payload := map[string]any{
		"ok":          true,
		"operation":   "okr_kr_delete",
		"mode":        "kr_delete",
		"destructive": true,
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
			"krsToDelete":      len(matched),
			"krsToCreate":      0,
			"resultingKrCount": len(remaining),
			"krsToDeleteTexts": krTexts(matched),
			"remainingKrs":     remaining,
			"emptiesObjective": len(remaining) == 0,
		},
	}
	if !config.Apply {
		payload["dryRun"] = true
		payload["willWrite"] = false
		payload["apply"] = applyGate(config.ConfirmKRDeletes, len(matched), "--confirm-kr-deletes")
		return payload, nil
	}
	if err := confirmDeletes(config.ConfirmKRDeletes, len(matched), "--confirm-kr-deletes"); err != nil {
		return nil, err
	}

	if err := session.enableDraft(target.ID); err != nil {
		return nil, err
	}
	if err := session.order(target.ID, krIDsExcluding(target.KRs, matched)); err != nil {
		return nil, err
	}
	if err := session.publish(target.ID, krIDs(matched)); err != nil {
		return nil, err
	}

	payload["dryRun"] = false
	payload["willWrite"] = true
	verify, err := session.verifyObjectiveKRs(config.Objective, remaining)
	if err != nil {
		return nil, err
	}
	payload["verify"] = verify
	payload["ok"] = asBoolValue(verify["ok"])
	return payload, nil
}

func containsKR(krs []krState, id string) bool {
	for _, kr := range krs {
		if kr.ID == id {
			return true
		}
	}
	return false
}

func krIDsExcluding(krs []krState, excluded []krState) []string {
	ids := []string{}
	for _, kr := range krs {
		if !containsKR(excluded, kr.ID) {
			ids = append(ids, kr.ID)
		}
	}
	return ids
}

// DeleteObjective removes one whole objective and the KRs under it.
//
// It replaces the --prune flag, which removed every objective absent from an input
// file: a deletion whose extent was decided by what the input happened to omit. One
// objective per invocation, named and confirmed, makes the extent something the
// caller states rather than something the data implies.
func DeleteObjective(config VerbConfig) (map[string]any, error) {
	session, err := openSession(config.URL, config.CookiesPath, config.CSRFURL)
	if err != nil {
		return nil, err
	}
	target, err := session.resolveObjective(config.Objective, config.ExpectTitle)
	if err != nil {
		return nil, err
	}
	before := len(session.objectives())
	payload := map[string]any{
		"ok":          true,
		"operation":   "okr_objective_delete",
		"mode":        "objective_delete",
		"destructive": true,
		"okrId":       session.okrID,
		"target": map[string]any{
			"objectiveIndex":          config.Objective,
			"objectiveId":             target.ID,
			"resolvedTitle":           target.Objective,
			"expectedTitle":           config.ExpectTitle,
			"titleMatchesExpectation": true,
		},
		"diff": map[string]any{
			"objectivesToDelete":        1,
			"krsToDelete":               len(target.KRs),
			"krsToDeleteTexts":          krTexts(target.KRs),
			"resultingObjectiveCount":   before - 1,
			"remainingIndexesWillShift": config.Objective < before,
		},
	}
	if !config.Apply {
		payload["dryRun"] = true
		payload["willWrite"] = false
		payload["apply"] = applyGate(config.ConfirmKRDeletes, len(target.KRs), "--confirm-kr-deletes")
		return payload, nil
	}
	if err := confirmDeletes(config.ConfirmKRDeletes, len(target.KRs), "--confirm-kr-deletes"); err != nil {
		return nil, err
	}

	if err := session.enableDraft(target.ID); err != nil {
		return nil, err
	}
	if err := session.deleteObjective(target.ID); err != nil {
		return nil, err
	}
	if err := session.reload(); err != nil {
		return nil, err
	}
	after := session.objectives()
	verify := map[string]any{
		"comparedAgainst":  "intended objective removal",
		"objectivesStored": len(after),
		"scope":            "post-delete read-back: the objective is gone and the remaining count matches; it does not confirm the content of other objectives",
	}
	switch {
	case len(after) != before-1:
		verify["ok"] = false
		verify["error"] = fmt.Sprintf("objective count went from %d to %d; expected %d", before, len(after), before-1)
	case objectiveIDPresent(after, target.ID):
		verify["ok"] = false
		verify["error"] = "the objective is still present after deleting it"
	default:
		verify["ok"] = true
	}
	payload["dryRun"] = false
	payload["willWrite"] = true
	payload["verify"] = verify
	payload["ok"] = asBoolValue(verify["ok"])
	return payload, nil
}

func objectiveIDPresent(items []objectiveState, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
