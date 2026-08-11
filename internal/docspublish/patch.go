package docspublish

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/serialq7ic4/ixf-toolbox/internal/docxgraph"
)

type PatchInsertConfig struct {
	MarkdownPath string
	URL          string
	CookiesPath  string
	SpaceAPI     string
	UnderHeading string
	Position     string
	RequiredText []string
	Apply        bool
}

type PatchSectionConfig struct {
	MarkdownPath string
	URL          string
	CookiesPath  string
	SpaceAPI     string
	UnderHeading string
	RequiredText []string
	AllowComplex bool
	Apply        bool
	DeleteOnly   bool
}

type patchTarget struct {
	Token   string
	BaseURL string
	Referer string
	Kind    string
}

type patchState struct {
	target   patchTarget
	session  *publishSession
	graph    docxgraph.Graph
	root     map[string]any
	memberID string
}

var patchDocTokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`obj_token":"([^"]+)"`),
	regexp.MustCompile(`token":"(dox[a-zA-Z0-9]+)"`),
	regexp.MustCompile(`url_token":"(dox[a-zA-Z0-9]+)"`),
}

func PatchInsertMarkdown(config PatchInsertConfig) (map[string]any, error) {
	content, err := os.ReadFile(expandUser(config.MarkdownPath))
	if err != nil {
		return nil, err
	}
	specs, err := ParseMarkdownFragment(string(content))
	if err != nil {
		return nil, err
	}
	if config.Apply {
		if err := requireMermaidRendererForApply(specs); err != nil {
			return nil, err
		}
	}
	position, err := normalizePatchInsertPosition(config.Position)
	if err != nil {
		return nil, err
	}
	target, err := parsePatchTarget(config.URL)
	if err != nil {
		return nil, err
	}
	session, err := newPublishSession(Config{
		CookiesPath: config.CookiesPath,
		SpaceAPI:    config.SpaceAPI,
	}, target.BaseURL)
	if err != nil {
		return nil, err
	}
	if target.Token == "" && target.Kind == "wiki" {
		token, err := session.resolveWikiDocxToken(target.Referer)
		if err != nil {
			return nil, err
		}
		target.Token = token
	}
	if target.Token == "" {
		return nil, fmt.Errorf("docs patch insert requires a docx URL or wiki-backed docx URL")
	}
	state, err := session.clientVars(target.Token, target.Referer)
	if err != nil {
		return nil, err
	}
	graph, err := docxgraph.Build(state, target.Token)
	if err != nil {
		return nil, err
	}
	structure := graph.SafeSummary()
	anchor, err := graph.FindHeadingByText(config.UnderHeading)
	if err != nil {
		return nil, err
	}
	insertIndex, err := graph.InsertIndex(anchor, position)
	if err != nil {
		return nil, err
	}
	rootData := dataForBlock(asMap(state["block_map"])[target.Token])
	memberID := updateMemberID(state, target.Token, rootData)
	if memberID == "" {
		return nil, fmt.Errorf("could not determine the authenticated document member identifier")
	}
	topIDs, entries := buildBlocks(specs, target.Token, newBlockFactory(memberID))
	section := graph.SectionRange(anchor)
	insertFingerprint := fingerprintSpecs(specs)
	duplicateCandidate := graph.SectionContainsFingerprint(section, insertFingerprint)
	payload := map[string]any{
		"ok":                    true,
		"dryRun":                true,
		"operation":             "docs_patch_insert",
		"mode":                  "block_insert",
		"destructive":           false,
		"willWrite":             false,
		"targetKind":            target.Kind,
		"anchorHeading":         config.UnderHeading,
		"resolvedAnchorHeading": anchor.Text,
		"position":              string(position),
		"insertIndex":           insertIndex,
		"plannedTopLevelBlocks": len(topIDs),
		"plannedBlockEntries":   len(entries),
		"currentTopLevelBlocks": len(graph.RootChildren),
		"existingBlocksTouched": false,
		"duplicateCandidate":    duplicateCandidate,
		"insertFingerprint":     insertFingerprint,
		"requiredTextChecks":    len(config.RequiredText),
		"counts":                summarizeSpecs(specs),
		"structure":             structure,
	}
	if !config.Apply {
		return withTableFallbackMetadata(payload, specs), nil
	}
	if duplicateCandidate {
		return nil, fmt.Errorf("duplicate insert candidate under heading %q", config.UnderHeading)
	}
	root := asMap(asMap(state["block_map"])[target.Token])
	changeMap := buildPatchInsertChangeMap(target.Token, root, insertIndex, topIDs, entries)
	if err := session.writeBlocks(target.Token, memberID, changeMap, target.Referer); err != nil {
		return nil, err
	}
	attachedImageCount, err := session.attachGeneratedImages(target.Token, memberID, target.Referer, entries)
	if err != nil {
		return nil, err
	}
	verify, err := session.verify(target.Token, target.Referer, patchVerifyRequiredText(specs, config.RequiredText), countSpecsByKind(specs, "image"))
	if err != nil {
		return nil, err
	}
	afterState, err := session.clientVars(target.Token, target.Referer)
	if err != nil {
		return nil, err
	}
	afterGraph, err := docxgraph.Build(afterState, target.Token)
	if err != nil {
		return nil, err
	}
	unchangedExistingBlocks := patchExistingBlocksUnchanged(graph, afterGraph, insertIndex, topIDs)
	verify["unchangedExistingBlocks"] = unchangedExistingBlocks
	if !unchangedExistingBlocks {
		verify["ok"] = false
	}
	payload["ok"] = asBool(verify["ok"])
	payload["dryRun"] = false
	payload["willWrite"] = true
	payload["insertedTopLevelBlocks"] = len(topIDs)
	payload["insertedBlockCount"] = len(entries)
	payload["attachedImageCount"] = attachedImageCount
	payload["verify"] = verify
	return withTableFallbackMetadata(payload, specs), nil
}

func PatchSectionMarkdown(config PatchSectionConfig) (map[string]any, error) {
	specs := []Spec{}
	if !config.DeleteOnly {
		content, err := os.ReadFile(expandUser(config.MarkdownPath))
		if err != nil {
			return nil, err
		}
		var parseErr error
		specs, parseErr = ParseMarkdownFragment(string(content))
		if parseErr != nil {
			return nil, parseErr
		}
	}
	if config.Apply && !config.DeleteOnly {
		if err := requireMermaidRendererForApply(specs); err != nil {
			return nil, err
		}
	}
	loaded, err := loadPatchState(config.URL, config.CookiesPath, config.SpaceAPI)
	if err != nil {
		return nil, err
	}
	anchor, err := loaded.graph.FindHeadingByText(config.UnderHeading)
	if err != nil {
		return nil, err
	}
	section := loaded.graph.SectionRange(anchor)
	complexTypes, complexCount := sectionComplexTypes(loaded.graph, section)
	if len(complexTypes) > 0 && !config.AllowComplex {
		return nil, fmt.Errorf("complex section content requires --allow-complex-section-replace: %s", strings.Join(complexTypes, ","))
	}
	topIDs := []string{}
	entries := []blockEntry{}
	if !config.DeleteOnly {
		topIDs, entries = buildBlocks(specs, loaded.target.Token, newBlockFactory(loaded.memberID))
	}
	mode := "section_replace"
	operation := "docs_patch_replace_section"
	if config.DeleteOnly {
		mode = "section_delete"
		operation = "docs_patch_delete_section"
	}
	payload := map[string]any{
		"ok":                          true,
		"dryRun":                      true,
		"operation":                   operation,
		"mode":                        mode,
		"destructive":                 true,
		"willWrite":                   false,
		"targetKind":                  loaded.target.Kind,
		"anchorHeading":               config.UnderHeading,
		"resolvedAnchorHeading":       anchor.Text,
		"sectionStart":                section.Start,
		"sectionEnd":                  section.End,
		"deletedTopLevelBlocks":       len(section.IDs),
		"plannedTopLevelBlocks":       len(topIDs),
		"plannedBlockEntries":         len(entries),
		"currentTopLevelBlocks":       len(loaded.graph.RootChildren),
		"complexBlockCount":           complexCount,
		"complexBlockTypes":           complexTypes,
		"allowComplexSectionReplace":  config.AllowComplex,
		"outsideSectionBlocksTouched": false,
		"requiredTextChecks":          len(config.RequiredText),
		"counts":                      summarizeSpecs(specs),
		"structure":                   loaded.graph.SafeSummary(),
	}
	if !config.Apply {
		return withTableFallbackMetadata(payload, specs), nil
	}
	var changeMap map[string]any
	if config.DeleteOnly {
		changeMap = buildDeleteSectionChangeMap(loaded.target.Token, loaded.root, section)
	} else {
		changeMap = buildReplaceSectionChangeMap(loaded.target.Token, loaded.root, section, topIDs, entries)
	}
	if err := loaded.session.writeBlocks(loaded.target.Token, loaded.memberID, changeMap, loaded.target.Referer); err != nil {
		return nil, err
	}
	attachedImageCount := 0
	if !config.DeleteOnly {
		var attachErr error
		attachedImageCount, attachErr = loaded.session.attachGeneratedImages(loaded.target.Token, loaded.memberID, loaded.target.Referer, entries)
		if attachErr != nil {
			return nil, attachErr
		}
	}
	required := patchVerifyRequiredText(specs, config.RequiredText)
	verify, err := loaded.session.verify(loaded.target.Token, loaded.target.Referer, required, countSpecsByKind(specs, "image"))
	if err != nil {
		return nil, err
	}
	afterState, err := loaded.session.clientVars(loaded.target.Token, loaded.target.Referer)
	if err != nil {
		return nil, err
	}
	afterGraph, err := docxgraph.Build(afterState, loaded.target.Token)
	if err != nil {
		return nil, err
	}
	unchangedOutside := patchOutsideSectionUnchanged(loaded.graph, afterGraph, section, topIDs)
	verify["unchangedOutsideSectionBlocks"] = unchangedOutside
	if !unchangedOutside {
		verify["ok"] = false
	}
	payload["ok"] = asBool(verify["ok"])
	payload["dryRun"] = false
	payload["willWrite"] = true
	payload["replacementTopLevelBlocks"] = len(topIDs)
	payload["replacementBlockEntries"] = len(entries)
	payload["attachedImageCount"] = attachedImageCount
	payload["verify"] = verify
	return withTableFallbackMetadata(payload, specs), nil
}

func loadPatchState(rawURL string, cookiesPath string, spaceAPI string) (patchState, error) {
	target, err := parsePatchTarget(rawURL)
	if err != nil {
		return patchState{}, err
	}
	session, err := newPublishSession(Config{
		CookiesPath: cookiesPath,
		SpaceAPI:    spaceAPI,
	}, target.BaseURL)
	if err != nil {
		return patchState{}, err
	}
	if target.Token == "" && target.Kind == "wiki" {
		token, err := session.resolveWikiDocxToken(target.Referer)
		if err != nil {
			return patchState{}, err
		}
		target.Token = token
	}
	if target.Token == "" {
		return patchState{}, fmt.Errorf("docs patch requires a docx URL or wiki-backed docx URL")
	}
	state, err := session.clientVars(target.Token, target.Referer)
	if err != nil {
		return patchState{}, err
	}
	graph, err := docxgraph.Build(state, target.Token)
	if err != nil {
		return patchState{}, err
	}
	root := asMap(asMap(state["block_map"])[target.Token])
	rootData := dataForBlock(root)
	memberID := updateMemberID(state, target.Token, rootData)
	if memberID == "" {
		return patchState{}, fmt.Errorf("could not determine the authenticated document member identifier")
	}
	return patchState{
		target:   target,
		session:  session,
		graph:    graph,
		root:     root,
		memberID: memberID,
	}, nil
}

func parsePatchTarget(rawURL string) (patchTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return patchTarget{}, fmt.Errorf("--url must be an absolute HTTP(S) docx or wiki URL")
	}
	baseURL := parsed.Scheme + "://" + parsed.Host
	if token := tokenAfterPath(parsed.Path, "/docx/"); token != "" {
		return patchTarget{Token: token, BaseURL: baseURL, Referer: strings.TrimSpace(rawURL), Kind: "docx"}, nil
	}
	if tokenAfterPath(parsed.Path, "/wiki/") != "" {
		return patchTarget{BaseURL: baseURL, Referer: strings.TrimSpace(rawURL), Kind: "wiki"}, nil
	}
	return patchTarget{}, fmt.Errorf("docs patch insert requires a docx URL or wiki URL")
}

func normalizePatchInsertPosition(raw string) (docxgraph.InsertPosition, error) {
	switch docxgraph.InsertPosition(strings.TrimSpace(raw)) {
	case "":
		return docxgraph.PositionSectionEnd, nil
	case docxgraph.PositionSectionEnd:
		return docxgraph.PositionSectionEnd, nil
	case docxgraph.PositionAfterHeading:
		return docxgraph.PositionAfterHeading, nil
	default:
		return "", fmt.Errorf("unsupported insert position: %s", raw)
	}
}

func (session *publishSession) resolveWikiDocxToken(referer string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, referer, nil)
	if err != nil {
		return "", err
	}
	session.addCommonHeaders(request, referer)
	response, err := session.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("wiki resolve request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("wiki resolve http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	for _, pattern := range patchDocTokenPatterns {
		if match := pattern.FindStringSubmatch(string(body)); len(match) == 2 {
			return match[1], nil
		}
	}
	return "", fmt.Errorf("wiki page did not expose a backing docx token")
}

func fingerprintSpecs(specs []Spec) string {
	parts := []string{}
	for _, spec := range specs {
		if strings.TrimSpace(spec.Text) != "" {
			parts = append(parts, spec.Text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return docxgraph.FingerprintText(strings.Join(parts, "\n"))
}

func patchVerifyRequiredText(specs []Spec, requiredText []string) []string {
	if len(requiredText) > 0 {
		return append([]string(nil), requiredText...)
	}
	seen := map[string]bool{}
	values := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		values = append(values, value)
	}
	for _, spec := range specs {
		if spec.Kind == "table" {
			for _, row := range spec.Rows {
				for _, cell := range row {
					add(cell)
				}
			}
			continue
		}
		if spec.Kind == "image" {
			continue
		}
		add(spec.Text)
	}
	return values
}

func patchExistingBlocksUnchanged(before docxgraph.Graph, after docxgraph.Graph, insertIndex int, insertedTopIDs []string) bool {
	if before.RootID != after.RootID {
		return false
	}
	expectedChildren := make([]string, 0, len(before.RootChildren)+len(insertedTopIDs))
	expectedChildren = append(expectedChildren, before.RootChildren[:insertIndex]...)
	expectedChildren = append(expectedChildren, insertedTopIDs...)
	expectedChildren = append(expectedChildren, before.RootChildren[insertIndex:]...)
	if !reflect.DeepEqual(expectedChildren, after.RootChildren) {
		return false
	}
	for blockID, beforeBlock := range before.Blocks {
		if blockID == before.RootID {
			continue
		}
		afterBlock, ok := after.Blocks[blockID]
		if !ok {
			return false
		}
		if beforeBlock.Kind != afterBlock.Kind || beforeBlock.ParentID != afterBlock.ParentID ||
			beforeBlock.Text != afterBlock.Text || !reflect.DeepEqual(beforeBlock.Children, afterBlock.Children) ||
			!reflect.DeepEqual(beforeBlock.Raw, afterBlock.Raw) {
			return false
		}
	}
	return true
}

func buildPatchInsertChangeMap(
	pageID string,
	root map[string]any,
	insertIndex int,
	topIDs []string,
	entries []blockEntry,
) map[string]any {
	changeMap := map[string]any{
		pageID: map[string]any{
			"id":      pageID,
			"version": asInt(root["version"]),
			"payload": map[string]any{
				"ops": insertChildOpsAt(insertIndex, topIDs),
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

func buildReplaceSectionChangeMap(
	pageID string,
	root map[string]any,
	section docxgraph.SectionRange,
	topIDs []string,
	entries []blockEntry,
) map[string]any {
	changeMap := newRootChildrenChangeMap(pageID, root, replaceSectionChildOps(section, topIDs))
	addNewBlockEntries(changeMap, entries)
	return changeMap
}

func buildDeleteSectionChangeMap(pageID string, root map[string]any, section docxgraph.SectionRange) map[string]any {
	return newRootChildrenChangeMap(pageID, root, replaceSectionChildOps(section, nil))
}

func sectionComplexTypes(graph docxgraph.Graph, section docxgraph.SectionRange) ([]string, int) {
	types := map[string]bool{}
	seen := map[string]bool{}
	count := 0
	for _, blockID := range section.IDs {
		count += collectSectionComplexTypes(graph, blockID, seen, types)
	}
	return sortedKeys(types), count
}

func collectSectionComplexTypes(graph docxgraph.Graph, blockID string, seen map[string]bool, types map[string]bool) int {
	if blockID == "" || seen[blockID] {
		return 0
	}
	seen[blockID] = true
	block, ok := graph.Blocks[blockID]
	if !ok {
		return 0
	}
	count := 0
	if !isSupportedMarkdownBlockType(block.Kind) {
		types[block.Kind] = true
		count++
	}
	for _, childID := range block.Children {
		count += collectSectionComplexTypes(graph, childID, seen, types)
	}
	return count
}

func patchOutsideSectionUnchanged(
	before docxgraph.Graph,
	after docxgraph.Graph,
	section docxgraph.SectionRange,
	replacementTopIDs []string,
) bool {
	if before.RootID != after.RootID {
		return false
	}
	expectedChildren := make([]string, 0, len(before.RootChildren)-len(section.IDs)+len(replacementTopIDs))
	expectedChildren = append(expectedChildren, before.RootChildren[:section.Start]...)
	expectedChildren = append(expectedChildren, replacementTopIDs...)
	expectedChildren = append(expectedChildren, before.RootChildren[section.End:]...)
	if !reflect.DeepEqual(expectedChildren, after.RootChildren) {
		return false
	}
	sectionBlocks := sectionBlockIDs(before, section)
	for blockID, beforeBlock := range before.Blocks {
		if blockID == before.RootID || sectionBlocks[blockID] {
			continue
		}
		afterBlock, ok := after.Blocks[blockID]
		if !ok {
			return false
		}
		if beforeBlock.Kind != afterBlock.Kind || beforeBlock.ParentID != afterBlock.ParentID ||
			beforeBlock.Text != afterBlock.Text || !reflect.DeepEqual(beforeBlock.Children, afterBlock.Children) ||
			!reflect.DeepEqual(beforeBlock.Raw, afterBlock.Raw) {
			return false
		}
	}
	return true
}

func sectionBlockIDs(graph docxgraph.Graph, section docxgraph.SectionRange) map[string]bool {
	ids := map[string]bool{}
	var collect func(string)
	collect = func(blockID string) {
		if blockID == "" || ids[blockID] {
			return
		}
		ids[blockID] = true
		for _, childID := range graph.Blocks[blockID].Children {
			collect(childID)
		}
	}
	for _, blockID := range section.IDs {
		collect(blockID)
	}
	return ids
}

func newRootChildrenChangeMap(pageID string, root map[string]any, ops []map[string]any) map[string]any {
	return map[string]any{
		pageID: map[string]any{
			"id":      pageID,
			"version": asInt(root["version"]),
			"payload": map[string]any{
				"ops": ops,
			},
		},
	}
}

func replaceSectionChildOps(section docxgraph.SectionRange, topIDs []string) []map[string]any {
	ops := make([]map[string]any, 0, len(section.IDs)+len(topIDs))
	for index := len(section.IDs) - 1; index >= 0; index-- {
		ops = append(ops, map[string]any{
			"p":      []any{"children", section.Start + index},
			"action": map[string]any{"ld": section.IDs[index]},
		})
	}
	ops = append(ops, insertChildOpsAt(section.Start, topIDs)...)
	return ops
}

func addNewBlockEntries(changeMap map[string]any, entries []blockEntry) {
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
}
