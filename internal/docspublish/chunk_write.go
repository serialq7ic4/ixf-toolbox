package docspublish

import "fmt"

// writeBlockGroupsConfig carries everything needed to write one document body as
// one or more sequential writes.
type writeBlockGroupsConfig struct {
	PageID       string
	MemberID     string
	FinalURL     string
	RootChildren []any
	RootVersion  int
	TopIDs       []string
	Entries      []blockEntry
	Groups       []writeGroup
}

// writeBlockGroups writes the planned groups in order and reports how many writes
// were issued.
//
// A single group takes the original single-write path unchanged, so an ordinary
// document is byte-identical to before. For multiple groups each write appends its
// top-level blocks after the children already present, which preserves document
// order by construction and needs no heading anchor.
func (session *publishSession) writeBlockGroups(config writeBlockGroupsConfig) (int, error) {
	groups := config.Groups
	if len(groups) == 0 {
		groups = []writeGroup{{
			EndSpec:      len(config.TopIDs),
			EndEntry:     len(config.Entries),
			BlockEntries: len(config.Entries),
		}}
	}

	rootChildCount := len(config.RootChildren)
	rootVersion := config.RootVersion
	for index, group := range groups {
		groupTopIDs := config.TopIDs[group.StartSpec:group.EndSpec]
		groupEntries := config.Entries[group.StartEntry:group.EndEntry]
		changeMap := buildGroupChangeMap(config.PageID, rootVersion, rootChildCount, groupTopIDs, groupEntries)
		if err := session.writeBlocks(config.PageID, config.MemberID, changeMap, config.FinalURL); err != nil {
			return index, newPartialPublishError(config.FinalURL, index, len(groups), err)
		}
		rootChildCount += len(groupTopIDs)
		if index+1 == len(groups) {
			break
		}
		// Each write bumps the page root version, so the next write needs the
		// current value rather than an assumed increment.
		nextVersion, err := session.currentRootVersion(config.PageID, config.FinalURL)
		if err != nil {
			return index + 1, newPartialPublishError(config.FinalURL, index+1, len(groups), err)
		}
		rootVersion = nextVersion
	}
	return len(groups), nil
}

// buildGroupChangeMap builds the change_map for one write: the page root entry
// carrying the child insert ops, plus one entry per new block.
func buildGroupChangeMap(pageID string, rootVersion, startIndex int, topIDs []string, entries []blockEntry) map[string]any {
	changeMap := map[string]any{
		pageID: map[string]any{
			"id":      pageID,
			"version": rootVersion,
			"payload": map[string]any{
				"ops": insertChildOpsAt(startIndex, topIDs),
			},
		},
	}
	for _, entry := range entries {
		changeMap[entry.ID] = map[string]any{
			"id":      entry.ID,
			"version": 0,
			"payload": map[string]any{
				"ops": []map[string]any{
					{
						"p":      []any{},
						"action": map[string]any{"oi": entry.Data},
					},
				},
			},
		}
	}
	return changeMap
}

// currentRootVersion re-reads the page root version between writes.
func (session *publishSession) currentRootVersion(pageID, referer string) (int, error) {
	state, err := session.clientVars(pageID, referer)
	if err != nil {
		return 0, err
	}
	root := asMap(asMap(state["block_map"])[pageID])
	if len(root) == 0 {
		return 0, fmt.Errorf("could not re-read the document root between writes")
	}
	return asInt(root["version"]), nil
}

// partialPublishError reports a failure that left a created document behind.
//
// The document is created before any content is written and this deployment has
// no delete-document capability, so a failed write always leaves a document in
// place. Naming its URL is the only way the caller can find or clean it up.
type partialPublishError struct {
	URL             string
	WritesCompleted int
	WritesPlanned   int
	Err             error
}

func newPartialPublishError(url string, completed, planned int, err error) error {
	if err == nil {
		return nil
	}
	return &partialPublishError{URL: url, WritesCompleted: completed, WritesPlanned: planned, Err: err}
}

func (e *partialPublishError) Error() string {
	progress := ""
	if e.WritesPlanned > 1 {
		progress = fmt.Sprintf(" after %d of %d writes", e.WritesCompleted, e.WritesPlanned)
	}
	return fmt.Sprintf(
		"%v\nthe document was created%s and is incomplete: %s\nit cannot be removed by this tool; delete it in the web UI if it is not wanted",
		e.Err, progress, e.URL)
}

func (e *partialPublishError) Unwrap() error { return e.Err }
