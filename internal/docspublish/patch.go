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

type patchTarget struct {
	Token   string
	BaseURL string
	Referer string
	Kind    string
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
	verify, err := session.verify(target.Token, target.Referer, patchVerifyRequiredText(specs, config.RequiredText))
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
	payload["verify"] = verify
	return withTableFallbackMetadata(payload, specs), nil
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
