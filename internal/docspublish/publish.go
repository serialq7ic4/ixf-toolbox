package docspublish

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	MarkdownPath string
	BaseURL      string
	CookiesPath  string
	SpaceAPI     string
	MemberID     string
	ParentToken  string
	Title        string
	TitleSuffix  string
	RequiredText []string
	Apply        bool
}

type UpdateConfig struct {
	MarkdownPath string
	URL          string
	CookiesPath  string
	SpaceAPI     string
	RequiredText []string
	Apply        bool
}

type Spec struct {
	Kind string
	Text string
}

func PublishMarkdown(config Config) (map[string]any, error) {
	content, err := os.ReadFile(expandUser(config.MarkdownPath))
	if err != nil {
		return nil, err
	}
	sourceTitle, specs, err := ParseMarkdown(string(content))
	if err != nil {
		return nil, err
	}
	baseURL, err := validateBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	title := config.Title
	if title == "" {
		title = sourceTitle + config.TitleSuffix
	}
	counts := summarizeSpecs(specs)
	if config.Apply {
		return ApplyMarkdown(config, baseURL, title, specs, counts)
	}
	return map[string]any{
		"ok":        true,
		"dryRun":    true,
		"operation": "create_docx",
		"title":     title,
		"counts":    counts,
	}, nil
}

func UpdateMarkdown(config UpdateConfig) (map[string]any, error) {
	if config.Apply {
		return nil, fmt.Errorf("docs update --apply is not supported in this version")
	}
	content, err := os.ReadFile(expandUser(config.MarkdownPath))
	if err != nil {
		return nil, err
	}
	title, specs, err := ParseMarkdown(string(content))
	if err != nil {
		return nil, err
	}
	target, err := parseDocxTarget(config.URL)
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
	state, err := session.clientVars(target.Token, target.Referer)
	if err != nil {
		return nil, err
	}
	summary, err := summarizeExistingDocument(state, target.Token)
	if err != nil {
		return nil, err
	}
	complexTypes := sortedKeys(summary.ComplexTypes)
	return map[string]any{
		"ok":                       true,
		"dryRun":                   true,
		"operation":                "update_docx",
		"mode":                     "replace_body",
		"destructive":              true,
		"willWrite":                false,
		"targetToken":              target.Token,
		"title":                    title,
		"counts":                   summarizeSpecs(specs),
		"currentTopLevelBlocks":    summary.TopLevelCount,
		"plannedTopLevelBlocks":    len(specs),
		"supportedExistingContent": len(complexTypes) == 0,
		"complexBlockCount":        summary.ComplexBlockCount,
		"complexBlockTypes":        complexTypes,
		"requiredTextChecks":       len(config.RequiredText),
	}, nil
}

func ApplyMarkdown(config Config, baseURL string, title string, specs []Spec, counts map[string]int) (map[string]any, error) {
	session, err := newPublishSession(config, baseURL)
	if err != nil {
		return nil, err
	}
	pageID, err := session.createDocument(title, config.ParentToken)
	if err != nil {
		return nil, err
	}
	finalURL := baseURL + "/docx/" + pageID
	varsBefore, err := session.clientVars(pageID, finalURL)
	if err != nil {
		return nil, err
	}
	blockMap := asMap(varsBefore["block_map"])
	root := asMap(blockMap[pageID])
	rootData := asMap(root["data"])
	author := asString(rootData["author"])
	if author == "" {
		return nil, fmt.Errorf("could not determine the authenticated document member identifier")
	}
	memberID := config.MemberID
	if memberID == "" {
		memberID = author
	}
	rootChildren := asSlice(rootData["children"])
	rootVersion := asInt(root["version"])
	topIDs, entries := buildBlocks(specs, pageID, newBlockFactory(author))
	changeMap := map[string]any{
		pageID: map[string]any{
			"id":      pageID,
			"version": rootVersion,
			"payload": map[string]any{
				"ops": insertChildOps(rootChildren, topIDs),
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
	if err := session.writeBlocks(pageID, memberID, changeMap, finalURL); err != nil {
		return nil, err
	}
	verify, err := session.verify(pageID, finalURL, config.RequiredText)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":        asBool(verify["ok"]),
		"dryRun":    false,
		"operation": "create_docx",
		"title":     title,
		"counts":    counts,
		"verify":    verify,
		"url":       finalURL,
	}, nil
}

func ParseMarkdown(markdown string) (string, []Spec, error) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		return "", nil, fmt.Errorf("Markdown must start with a level-1 title (`# title`).")
	}
	title := strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
	specs := []Spec{}
	for index := 1; index < len(lines); {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			index++
			continue
		}
		if strings.HasPrefix(line, "```") {
			buffer := []string{}
			index++
			for index < len(lines) && !strings.HasPrefix(lines[index], "```") {
				buffer = append(buffer, lines[index])
				index++
			}
			if index < len(lines) {
				index++
			}
			specs = append(specs, Spec{Kind: "code", Text: strings.Join(buffer, "\n")})
			continue
		}
		if isTableStart(lines, index) {
			index += 2
			for index < len(lines) && strings.HasPrefix(lines[index], "|") {
				index++
			}
			specs = append(specs, Spec{Kind: "callout"})
			continue
		}
		if match := headingPattern.FindStringSubmatch(line); len(match) == 3 {
			specs = append(specs, Spec{Kind: fmt.Sprintf("heading%d", len(match[1])), Text: cleanInline(match[2])})
			index++
			continue
		}
		if strings.HasPrefix(line, "- ") {
			specs = append(specs, Spec{Kind: "bullet", Text: cleanInline(strings.TrimPrefix(line, "- "))})
			index++
			continue
		}
		if orderedPattern.MatchString(line) {
			specs = append(specs, Spec{Kind: "ordered", Text: cleanInline(orderedPattern.ReplaceAllString(line, ""))})
			index++
			continue
		}
		paragraph := []string{strings.TrimSpace(line)}
		index++
		for index < len(lines) {
			next := lines[index]
			if strings.TrimSpace(next) == "" || strings.HasPrefix(next, "```") || strings.HasPrefix(next, "#") ||
				strings.HasPrefix(next, "- ") || orderedPattern.MatchString(next) || strings.HasPrefix(next, "|") {
				break
			}
			paragraph = append(paragraph, strings.TrimSpace(next))
			index++
		}
		text := cleanInline(strings.Join(paragraph, " "))
		if text == "" {
			continue
		}
		switch {
		case strings.HasPrefix(text, "案例类型："):
			specs = append(specs, Spec{Kind: "callout", Text: text})
		case text == "完整因果链可以收敛为：":
			specs = append(specs, Spec{Kind: "callout", Text: text})
		case strings.HasPrefix(text, "换句话说") || strings.HasPrefix(text, "本质上") || strings.HasPrefix(text, "所以"):
			specs = append(specs, Spec{Kind: "quote", Text: text})
		default:
			specs = append(specs, Spec{Kind: "text", Text: text})
		}
	}
	return title, specs, nil
}

var (
	headingPattern        = regexp.MustCompile(`^(#{1,9})\s+(.*)$`)
	orderedPattern        = regexp.MustCompile(`^\d+\.\s+`)
	inlineCodePattern     = regexp.MustCompile("`([^`]+)`")
	tableSeparatorPattern = regexp.MustCompile(`^\|\s*-+`)
)

func isTableStart(lines []string, index int) bool {
	return index+1 < len(lines) && strings.HasPrefix(lines[index], "|") && tableSeparatorPattern.MatchString(lines[index+1])
}

func cleanInline(text string) string {
	return strings.TrimSpace(inlineCodePattern.ReplaceAllString(text, "$1"))
}

func summarizeSpecs(specs []Spec) map[string]int {
	counts := map[string]int{}
	for _, spec := range specs {
		counts[spec.Kind]++
	}
	return counts
}

type cookieObject struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type publishSession struct {
	client   *http.Client
	cookies  []http.Cookie
	csrf     string
	baseURL  string
	spaceAPI string
}

type blockEntry struct {
	ID   string
	Data map[string]any
}

func newPublishSession(config Config, baseURL string) (*publishSession, error) {
	cookieObjects, err := loadCookieObjects(config.CookiesPath)
	if err != nil {
		return nil, err
	}
	csrf := csrfFromCookieObjects(cookieObjects)
	if csrf == "" {
		return nil, fmt.Errorf("cookie jar does not contain _csrf_token")
	}
	cookies := make([]http.Cookie, 0, len(cookieObjects))
	for _, cookie := range cookieObjects {
		if cookie.Name == "" {
			continue
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		cookies = append(cookies, http.Cookie{
			Name:   cookie.Name,
			Value:  cookie.Value,
			Domain: cookie.Domain,
			Path:   path,
		})
	}
	spaceAPI := strings.TrimRight(config.SpaceAPI, "/")
	if spaceAPI == "" {
		spaceAPI = "https://internal-api-space.xfchat.iflytek.com"
	}
	return &publishSession{
		client:   &http.Client{Timeout: 30 * time.Second},
		cookies:  cookies,
		csrf:     csrf,
		baseURL:  baseURL,
		spaceAPI: spaceAPI,
	}, nil
}

func (session *publishSession) createDocument(title string, parentToken string) (string, error) {
	form := url.Values{
		"type":         {"22"},
		"source":       {"0"},
		"uuid":         {randomUUID()},
		"name":         {title},
		"parent_token": {parentToken},
	}
	request, err := http.NewRequest("POST", session.spaceAPI+"/space/api/explorer/v2/create/object/", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	session.addCommonHeaders(request, session.baseURL+"/drive/home/")
	payload, err := session.doJSON(request, "document creation")
	if err != nil {
		return "", err
	}
	if asInt(payload["code"]) != 0 {
		return "", fmt.Errorf("document creation failed")
	}
	pageID := findDocToken(payload)
	if pageID == "" {
		return "", fmt.Errorf("document creation did not return a document token")
	}
	return pageID, nil
}

func (session *publishSession) clientVars(pageID string, referer string) (map[string]any, error) {
	data := map[string]any{}
	cursor := ""
	for {
		query := url.Values{"id": {pageID}, "open_type": {"1"}}
		if cursor != "" {
			query.Set("mode", "4")
			query.Set("cursor", cursor)
		}
		request, err := http.NewRequest("GET", session.spaceAPI+"/space/api/docx/pages/client_vars?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		session.addCommonHeaders(request, referer)
		payload, err := session.doJSON(request, "client_vars")
		if err != nil {
			return nil, err
		}
		if asInt(payload["code"]) != 0 {
			return nil, fmt.Errorf("could not load document state")
		}
		page := asMap(payload["data"])
		mergeClientVarsPage(data, page)
		cursor = asString(page["cursor"])
		if !asBool(page["has_more"]) || cursor == "" {
			return data, nil
		}
	}
}

func (session *publishSession) writeBlocks(pageID string, memberID string, changeMap map[string]any, referer string) error {
	body, err := json.Marshal(map[string]any{
		"member_id":  memberID,
		"uuid":       randomUUID(),
		"page_id":    pageID,
		"change_map": changeMap,
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", session.baseURL+"/space/api/docx/blocks/user_change", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	session.addCommonHeaders(request, referer)
	payload, err := session.doJSON(request, "document content write")
	if err != nil {
		return err
	}
	if asInt(payload["code"]) != 0 {
		return fmt.Errorf("document content write failed")
	}
	return nil
}

func (session *publishSession) verify(pageID string, referer string, requiredText []string) (map[string]any, error) {
	last := map[string]any{"ok": false, "counts": map[string]int{}, "textChars": 0}
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		payload, err := session.clientVars(pageID, referer)
		if err != nil {
			return nil, err
		}
		counts := map[string]int{}
		textValues := []string{}
		codeTexts := []string{}
		for _, raw := range asMap(payload["block_map"]) {
			data := asMap(asMap(raw)["data"])
			blockType := asString(data["type"])
			if blockType != "" {
				counts[blockType]++
			}
			text := textFromBlockData(data)
			if text == "" {
				continue
			}
			textValues = append(textValues, text)
			if blockType == "code" {
				codeTexts = append(codeTexts, text)
			}
		}
		allText := strings.Join(textValues, "\n")
		ok := true
		for _, required := range requiredText {
			if !strings.Contains(allText, required) {
				ok = false
				break
			}
		}
		if ok && len(codeTexts) > 0 {
			ok = false
			for _, code := range codeTexts {
				if strings.Contains(code, "\n") {
					ok = true
					break
				}
			}
		}
		last = map[string]any{
			"ok":        ok,
			"counts":    counts,
			"textChars": len(allText),
		}
		if ok {
			return last, nil
		}
	}
	return last, nil
}

func (session *publishSession) addCommonHeaders(request *http.Request, referer string) {
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", session.baseURL)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-CSRFToken", session.csrf)
	for _, cookie := range session.cookies {
		request.AddCookie(&cookie)
	}
}

func (session *publishSession) doJSON(request *http.Request, label string) (map[string]any, error) {
	response, err := session.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s request failed", label)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s http status %d", label, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", label)
	}
	return payload, nil
}

type blockFactory struct {
	author string
	used   map[string]bool
}

func newBlockFactory(author string) *blockFactory {
	return &blockFactory{author: author, used: map[string]bool{}}
}

func (factory *blockFactory) blockID() string {
	for {
		id := "doxrz" + randomHex(11)
		if !factory.used[id] {
			factory.used[id] = true
			return id
		}
	}
}

func (factory *blockFactory) textObject(text string) map[string]any {
	return map[string]any{
		"initialAttributedTexts": map[string]any{
			"text":    map[string]any{"0": text},
			"attribs": map[string]any{"0": attribFor(text)},
		},
		"apool": map[string]any{
			"numToAttrib": map[string]any{"0": []any{"author", factory.author}},
			"nextNum":     1,
		},
	}
}

func (factory *blockFactory) baseBlock(blockType string, parentID string, text string) map[string]any {
	data := map[string]any{
		"type":      blockType,
		"parent_id": parentID,
		"children":  []any{},
		"comments":  []any{},
		"revisions": []any{},
		"locked":    false,
		"hidden":    false,
		"author":    factory.author,
		"align":     "",
	}
	if strings.HasPrefix(blockType, "heading") || blockType == "text" || blockType == "bullet" || blockType == "ordered" || blockType == "code" {
		data["text"] = factory.textObject(text)
		data["folded"] = false
	}
	if blockType == "code" {
		data["language"] = "Plain Text"
		data["wrap"] = false
		data["caption"] = map[string]any{
			"text": map[string]any{
				"apool": map[string]any{"nextNum": 0, "numToAttrib": map[string]any{}},
				"initialAttributedTexts": map[string]any{
					"attribs": map[string]any{"0": "|1+1"},
					"text":    map[string]any{"0": "\n"},
				},
			},
		}
	}
	return data
}

func (factory *blockFactory) quoteBlocks(parentID string, text string) ([]blockEntry, string) {
	quoteID := factory.blockID()
	childID := factory.blockID()
	quote := map[string]any{
		"type":      "quote_container",
		"parent_id": parentID,
		"children":  []any{childID},
		"comments":  []any{},
		"revisions": []any{},
		"locked":    false,
		"hidden":    false,
		"author":    factory.author,
	}
	return []blockEntry{
		{ID: quoteID, Data: quote},
		{ID: childID, Data: factory.baseBlock("text", quoteID, text)},
	}, quoteID
}

func (factory *blockFactory) calloutBlocks(parentID string, text string) ([]blockEntry, string) {
	calloutID := factory.blockID()
	childID := factory.blockID()
	callout := map[string]any{
		"type":             "callout",
		"parent_id":        parentID,
		"children":         []any{childID},
		"comments":         []any{},
		"revisions":        []any{},
		"locked":           false,
		"hidden":           false,
		"author":           factory.author,
		"background_color": "",
		"border_color":     "",
		"text_color":       "",
		"align":            "left",
		"emoji_id":         "memo",
		"emoji_value":      "1f4dd",
	}
	return []blockEntry{
		{ID: calloutID, Data: callout},
		{ID: childID, Data: factory.baseBlock("text", calloutID, text)},
	}, calloutID
}

func buildBlocks(specs []Spec, pageID string, factory *blockFactory) ([]string, []blockEntry) {
	topIDs := []string{}
	entries := []blockEntry{}
	for _, spec := range specs {
		switch spec.Kind {
		case "quote":
			newEntries, topID := factory.quoteBlocks(pageID, spec.Text)
			topIDs = append(topIDs, topID)
			entries = append(entries, newEntries...)
		case "callout":
			newEntries, topID := factory.calloutBlocks(pageID, spec.Text)
			topIDs = append(topIDs, topID)
			entries = append(entries, newEntries...)
		default:
			blockID := factory.blockID()
			topIDs = append(topIDs, blockID)
			entries = append(entries, blockEntry{ID: blockID, Data: factory.baseBlock(spec.Kind, pageID, spec.Text)})
		}
	}
	return topIDs, entries
}

type docxTarget struct {
	Token   string
	BaseURL string
	Referer string
}

type documentSummary struct {
	TopLevelCount     int
	ComplexBlockCount int
	ComplexTypes      map[string]bool
}

func parseDocxTarget(rawURL string) (docxTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return docxTarget{}, fmt.Errorf("--url must be an absolute HTTP(S) docx URL")
	}
	token := tokenAfterPath(parsed.Path, "/docx/")
	if token == "" {
		return docxTarget{}, fmt.Errorf("docs update requires a direct docx URL")
	}
	return docxTarget{
		Token:   token,
		BaseURL: parsed.Scheme + "://" + parsed.Host,
		Referer: strings.TrimSpace(rawURL),
	}, nil
}

func summarizeExistingDocument(clientVars map[string]any, pageID string) (documentSummary, error) {
	blockMap := asMap(clientVars["block_map"])
	rootData := dataForBlock(blockMap[pageID])
	if len(rootData) == 0 {
		return documentSummary{}, fmt.Errorf("could not load target document root block")
	}
	children := asSlice(rootData["children"])
	summary := documentSummary{
		TopLevelCount: len(children),
		ComplexTypes:  map[string]bool{},
	}
	seen := map[string]bool{}
	for _, child := range children {
		collectComplexBlocks(blockMap, asString(child), seen, &summary)
	}
	return summary, nil
}

func collectComplexBlocks(blockMap map[string]any, blockID string, seen map[string]bool, summary *documentSummary) {
	if blockID == "" || seen[blockID] {
		return
	}
	seen[blockID] = true
	data := dataForBlock(blockMap[blockID])
	blockType := asString(data["type"])
	if blockType == "" {
		blockType = "unknown"
	}
	if !isSupportedMarkdownBlockType(blockType) {
		summary.ComplexBlockCount++
		summary.ComplexTypes[blockType] = true
	}
	for _, child := range asSlice(data["children"]) {
		collectComplexBlocks(blockMap, asString(child), seen, summary)
	}
}

func isSupportedMarkdownBlockType(blockType string) bool {
	switch blockType {
	case "page", "text", "bullet", "ordered", "code", "quote_container", "callout":
		return true
	}
	return strings.HasPrefix(blockType, "heading")
}

func dataForBlock(value any) map[string]any {
	entry := asMap(value)
	data := asMap(entry["data"])
	if len(data) > 0 {
		return data
	}
	return entry
}

func mergeClientVarsPage(target map[string]any, page map[string]any) {
	for key, value := range page {
		if key == "block_map" {
			targetBlockMap := asMap(target["block_map"])
			if len(targetBlockMap) == 0 {
				targetBlockMap = map[string]any{}
				target["block_map"] = targetBlockMap
			}
			for blockID, blockValue := range asMap(value) {
				targetBlockMap[blockID] = blockValue
			}
			continue
		}
		target[key] = value
	}
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func tokenAfterPath(path string, marker string) string {
	index := strings.Index(path, marker)
	if index < 0 {
		return ""
	}
	rest := strings.Trim(path[index+len(marker):], "/")
	if rest == "" {
		return ""
	}
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return rest
}

func insertChildOps(rootChildren []any, topIDs []string) []map[string]any {
	ops := make([]map[string]any, 0, len(topIDs))
	for index, blockID := range topIDs {
		ops = append(ops, map[string]any{
			"p":      []any{"children", len(rootChildren) + index},
			"action": map[string]any{"li": blockID},
		})
	}
	return ops
}

func attribFor(text string) string {
	if text == "" {
		return "*0+0"
	}
	parts := strings.Split(text, "\n")
	if len(parts) == 1 {
		return "*0+" + strconv.FormatInt(int64(len(text)), 36)
	}
	prefixLen := 0
	for _, part := range parts[:len(parts)-1] {
		prefixLen += len(part) + 1
	}
	return "*0|" + strconv.FormatInt(int64(len(parts)-1), 36) + "+" +
		strconv.FormatInt(int64(prefixLen), 36) + "*0+" +
		strconv.FormatInt(int64(len(parts[len(parts)-1])), 36)
}

func loadCookieObjects(path string) ([]cookieObject, error) {
	expanded := expandUser(path)
	content, err := os.ReadFile(expanded)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cookie file not found: %s", expanded)
		}
		return nil, err
	}
	cookies := []cookieObject{}
	if err := json.Unmarshal(content, &cookies); err != nil {
		return nil, fmt.Errorf("cookie file invalid: %s", expanded)
	}
	return cookies, nil
}

func csrfFromCookieObjects(cookies []cookieObject) string {
	for _, cookie := range cookies {
		if cookie.Name == "_csrf_token" && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

func findDocToken(value any) string {
	switch typed := value.(type) {
	case string:
		if strings.HasPrefix(typed, "doxrz") {
			return typed
		}
	case map[string]any:
		for _, key := range []string{"token", "obj_token", "url_token", "node_token", "id"} {
			if found := findDocToken(typed[key]); found != "" {
				return found
			}
		}
		for _, child := range typed {
			if found := findDocToken(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findDocToken(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func textFromBlockData(data map[string]any) string {
	text := asMap(data["text"])
	initial := asMap(text["initialAttributedTexts"])
	values := asMap(initial["text"])
	return asString(values["0"])
}

func asMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func asSlice(value any) []any {
	if value == nil {
		return []any{}
	}
	if typed, ok := value.([]any); ok {
		return typed
	}
	return []any{}
}

func asString(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func asBool(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func randomUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func randomHex(byteCount int) string {
	raw := make([]byte, byteCount)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}

func validateBaseURL(baseURL string) (string, error) {
	normalized := strings.TrimRight(baseURL, "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("--base-url must be an absolute HTTP(S) URL")
	}
	return normalized, nil
}

func expandUser(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
