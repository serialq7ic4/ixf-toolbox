package docslocal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/serialq7ic4/ixf-toolbox/internal/docx"
)

var (
	remoteTokenPattern = regexp.MustCompile(`/([^/?#]+)`)
	slugPattern        = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	docTokenPatterns   = []*regexp.Regexp{
		regexp.MustCompile(`obj_token":"([^"]+)"`),
		regexp.MustCompile(`token":"(dox[a-zA-Z0-9]+)"`),
		regexp.MustCompile(`url_token":"(dox[a-zA-Z0-9]+)"`),
	}
)

const DefaultSpaceAPI = "https://internal-api-space.xfchat.iflytek.com"

type Result struct {
	Source   string           `json:"source"`
	Kind     string           `json:"kind"`
	Title    string           `json:"title"`
	Token    string           `json:"token"`
	Content  string           `json:"content"`
	Counts   map[string]int   `json:"counts"`
	Assets   []map[string]any `json:"assets"`
	Warnings []string         `json:"warnings"`
}

type ReadOptions struct {
	CookiesPath    string
	SpaceAPI       string
	DownloadImages bool
	OutputRoot     string
	ExpandSheets   bool
}

func InspectSource(source string) (map[string]any, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return inspectRemoteSource(source)
	}
	return inspectLocalSource(source)
}

func ReadLocalSources(sources []string) ([]Result, error) {
	return ReadSourcesWithOptions(sources, ReadOptions{})
}

func ReadSourcesWithOptions(sources []string, options ReadOptions) ([]Result, error) {
	if options.DownloadImages && options.OutputRoot == "" {
		return nil, fmt.Errorf("download_images requires output_root")
	}
	results := []Result{}
	var remoteSession *remoteReadSession
	for index, source := range sources {
		if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
			kindHint := remoteKindFromSource(source)
			if err := unsupportedRemoteDocsReadError(kindHint); err != nil {
				return nil, err
			}
			if remoteSession == nil {
				session, err := newRemoteReadSession(options)
				if err != nil {
					return nil, err
				}
				remoteSession = session
			}
			result, err := remoteSession.readRemote(source, fmt.Sprintf("%s_%d", kindHint, index+1))
			if err != nil {
				return nil, err
			}
			results = append(results, result)
			continue
		}
		result, err := readLocalSource(source)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func unsupportedRemoteDocsReadError(kind string) error {
	if kind == "okr" {
		return fmt.Errorf("docs read does not support OKR URLs; use `ixf okr read <url>` for OKR pages")
	}
	return nil
}

func readLocalSource(source string) (Result, error) {
	path := expandUser(source)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("local file not found: %s", path)
		}
		return Result{}, err
	}
	return Result{
		Source:   source,
		Kind:     "local_markdown",
		Title:    filepath.Base(path),
		Token:    "",
		Content:  string(content),
		Counts:   map[string]int{},
		Assets:   []map[string]any{},
		Warnings: []string{},
	}, nil
}

type remoteReadSession struct {
	client         *http.Client
	cookies        []http.Cookie
	csrfToken      string
	spaceAPI       string
	downloadImages bool
	outputRoot     string
	expandSheets   bool
}

type cookieObject struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

func newRemoteReadSession(options ReadOptions) (*remoteReadSession, error) {
	cookiesPath := options.CookiesPath
	if cookiesPath == "" {
		cookiesPath = defaultCookiesPath()
	}
	cookieObjects, err := loadCookieObjects(cookiesPath)
	if err != nil {
		return nil, err
	}
	csrfToken := csrfFromCookieObjects(cookieObjects)
	if csrfToken == "" {
		return nil, fmt.Errorf("cookie jar does not contain _csrf_token")
	}
	spaceAPI := strings.TrimRight(options.SpaceAPI, "/")
	if spaceAPI == "" {
		spaceAPI = DefaultSpaceAPI
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
	return &remoteReadSession{
		client:         &http.Client{Timeout: 30 * time.Second},
		cookies:        cookies,
		csrfToken:      csrfToken,
		spaceAPI:       spaceAPI,
		downloadImages: options.DownloadImages,
		outputRoot:     options.OutputRoot,
		expandSheets:   options.ExpandSheets,
	}, nil
}

func (session *remoteReadSession) readRemote(source string, assetGroup string) (Result, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return Result{}, err
	}
	origin := originForURL(parsed)
	kind := remoteKindFromPath(parsed.Path)
	if kind == "mindnote" {
		return session.readMindnote(source, origin, parsed.Path)
	}
	if kind == "sheet" {
		return session.readDirectSheet(source, origin, parsed)
	}
	token := tokenAfter(parsed.Path, "/docx/")
	if token == "" && kind == "wiki" {
		html, err := session.fetchHTML(source, origin)
		if err != nil {
			return Result{}, err
		}
		if isBitableWikiHTML(html) {
			return session.readBitableWiki(source, origin, html)
		}
		token = extractDocTokenFromHTML(html)
	}
	if token == "" {
		return Result{}, fmt.Errorf("remote source is not supported by Go docs read yet")
	}
	data, err := session.clientVars(token, origin, source)
	if err != nil {
		return Result{}, err
	}
	conversionOptions := docx.Options{}
	if session.downloadImages {
		writer := newImageAssetWriter(session, origin, source, token, session.outputRoot, assetGroup)
		conversionOptions.ResolveImage = writer.resolve
	}
	sheetCache := map[string][]string{}
	if session.expandSheets {
		conversionOptions.ExpandSheet = func(sheetBlockToken string) []string {
			if lines, ok := sheetCache[sheetBlockToken]; ok {
				return lines
			}
			lines := session.expandEmbeddedSheet(origin, token, sheetBlockToken)
			sheetCache[sheetBlockToken] = lines
			return lines
		}
	}
	conversion := docx.ConvertClientVarsWithOptions(data, token, conversionOptions)
	if session.expandSheets && len(sheetCache) > 0 {
		conversion.Counts["sheet_expanded"] = len(sheetCache)
	}
	title := docxTitle(data, token)
	return Result{
		Source:   source,
		Kind:     kind,
		Title:    title,
		Token:    token,
		Content:  conversion.Markdown,
		Counts:   conversion.Counts,
		Assets:   conversion.Assets,
		Warnings: conversion.Warnings,
	}, nil
}

func remoteKindFromSource(source string) string {
	parsed, err := url.Parse(source)
	if err != nil {
		return "remote"
	}
	return remoteKindFromPath(parsed.Path)
}

func remoteKindFromPath(path string) string {
	if tokenAfter(path, "/docx/") != "" {
		return "docx"
	}
	if strings.Contains(path, "/wiki/") {
		return "wiki"
	}
	if tokenAfter(path, "/mindnotes/") != "" {
		return "mindnote"
	}
	if tokenAfter(path, "/sheets/") != "" {
		return "sheet"
	}
	if strings.Contains(path, "/okr/user/") {
		return "okr"
	}
	return "remote"
}

func (session *remoteReadSession) fetchHTML(requestURL string, origin string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}
	session.addCommonHeaders(request, origin, requestURL)
	response, err := session.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("remote html http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func extractDocTokenFromHTML(html string) string {
	for _, pattern := range docTokenPatterns {
		if match := pattern.FindStringSubmatch(html); len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

func (session *remoteReadSession) readMindnote(source string, origin string, path string) (Result, error) {
	html, err := session.fetchHTML(source, origin)
	if err != nil {
		return Result{}, err
	}
	encodedPayload, err := extractBalancedObject(html, "clientVars: Object(")
	if err != nil {
		return Result{}, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(encodedPayload), &payload); err != nil {
		return Result{}, err
	}
	data := asMap(payload["data"])
	token := strings.TrimSpace(stringValue(payload["token"]))
	if token == "" {
		token = tokenAfter(path, "/mindnotes/")
	}
	title := strings.TrimSpace(stringValue(data["title"]))
	if title == "" {
		title = token
	}
	if title == "" {
		title = filepath.Base(path)
	}
	nodes := asSlice(asMap(data["collab_client_vars"])["nodes"])
	lines := []string{"# " + title, ""}
	lines = append(lines, renderMindnoteNodes(nodes, 0)...)
	return Result{
		Source:   source,
		Kind:     "mindnote",
		Title:    title,
		Token:    token,
		Content:  strings.Join(normalizeLines(lines), "\n") + "\n",
		Counts:   map[string]int{"mindnote_nodes": len(nodes)},
		Assets:   []map[string]any{},
		Warnings: []string{},
	}, nil
}

func extractBalancedObject(text string, anchor string) (string, error) {
	start := strings.Index(text, anchor)
	if start == -1 {
		return "", fmt.Errorf("anchor not found: %s", anchor)
	}
	start += len(anchor)
	for start < len(text) && (text[start] == ' ' || text[start] == '\n' || text[start] == '\r' || text[start] == '\t') {
		start++
	}
	if start >= len(text) || text[start] != '{' {
		return "", fmt.Errorf("expected object after anchor: %s", anchor)
	}
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(text); index++ {
		character := text[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			continue
		}
		if character == '{' {
			depth++
			continue
		}
		if character == '}' {
			depth--
			if depth == 0 {
				return text[start : index+1], nil
			}
		}
	}
	return "", fmt.Errorf("unterminated object after anchor: %s", anchor)
}

func renderMindnoteNodes(nodes []any, depth int) []string {
	lines := []string{}
	indent := strings.Repeat("  ", depth)
	for _, node := range nodes {
		nodeMap := asMap(node)
		if text := docx.ExtractText(nodeMap["text"]); text != "" {
			lines = append(lines, indent+"- "+text)
		}
		lines = append(lines, renderMindnoteNodes(asSlice(nodeMap["children"]), depth+1)...)
	}
	return lines
}

func normalizeLines(lines []string) []string {
	cleaned := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t\r\n")
		if strings.TrimSpace(trimmedRight) == "" {
			blankRun++
			if blankRun <= 1 {
				cleaned = append(cleaned, "")
			}
			continue
		}
		blankRun = 0
		cleaned = append(cleaned, trimmedRight)
	}
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return cleaned
}

func (session *remoteReadSession) clientVars(token string, origin string, referer string) (map[string]any, error) {
	data := map[string]any{}
	cursor := ""
	for {
		query := "id=" + url.QueryEscape(token) + "&open_type=1"
		if cursor != "" {
			query += "&mode=4&cursor=" + url.QueryEscape(cursor)
		}
		requestURL := session.spaceAPI + "/space/api/docx/pages/client_vars?" + query
		payload, err := session.getJSON(requestURL, origin, referer)
		if err != nil {
			return nil, err
		}
		if codeNumber(payload["code"]) != 0 {
			return nil, fmt.Errorf("client_vars failed")
		}
		page := asMap(payload["data"])
		mergeClientVarsPage(data, page)
		cursor = stringValue(page["cursor"])
		if !readBool(page["has_more"]) || cursor == "" {
			return data, nil
		}
	}
}

func (session *remoteReadSession) getJSON(requestURL string, origin string, referer string) (map[string]any, error) {
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	session.addCommonHeaders(request, origin, referer)
	response, err := session.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("client_vars http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (session *remoteReadSession) addCommonHeaders(request *http.Request, origin string, referer string) {
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-CSRFToken", session.csrfToken)
	for _, cookie := range session.cookies {
		request.AddCookie(&cookie)
	}
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
		if key != "has_more" && key != "cursor" {
			target[key] = value
		}
	}
}

func docxTitle(data map[string]any, token string) string {
	root := asMap(asMap(data["block_map"])[token])
	rootData := asMap(root["data"])
	if len(rootData) == 0 {
		rootData = root
	}
	title := docx.ExtractText(rootData["text"])
	if title == "" {
		return token
	}
	return title
}

func WriteOutputs(results []Result, outDir string) (map[string]any, error) {
	root := expandUser(outDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	manifest := map[string]any{}
	usedStems := map[string]bool{}
	for index, result := range results {
		stem := fmt.Sprintf("%s_%d", result.Kind, index+1)
		fileStem := outputFileStem(result, stem, usedStems)
		filePath := filepath.Join(root, fileStem+".md")
		if err := os.WriteFile(filePath, []byte(result.Content), 0o644); err != nil {
			return nil, err
		}
		manifest[stem] = map[string]any{
			"title":    result.Title,
			"token":    result.Token,
			"kind":     result.Kind,
			"counts":   result.Counts,
			"file":     filePath,
			"source":   result.Source,
			"assets":   result.Assets,
			"warnings": result.Warnings,
		}
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), content, 0o644); err != nil {
		return nil, err
	}
	return manifest, nil
}

func CleanupOutputs(outDir string) error {
	root := expandUser(outDir)
	manifestPath := filepath.Join(root, "manifest.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest not found: %s", manifestPath)
	}
	var manifest map[string]struct {
		File   string `json:"file"`
		Assets []struct {
			Path string `json:"path"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return fmt.Errorf("invalid manifest: %s", manifestPath)
	}
	for _, item := range manifest {
		if item.File != "" {
			if err := removeInside(root, item.File); err != nil {
				return err
			}
		}
		for _, asset := range item.Assets {
			if asset.Path == "" {
				continue
			}
			if err := removeInside(root, filepath.Join(root, asset.Path)); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func inspectLocalSource(source string) (map[string]any, error) {
	path := expandUser(source)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local file not found: %s", path)
		}
		return nil, err
	}
	readable := false
	if file, err := os.Open(path); err == nil {
		readable = true
		_ = file.Close()
	}
	return map[string]any{
		"ok":        true,
		"source":    source,
		"remote":    false,
		"kind":      "local_markdown",
		"path":      path,
		"exists":    true,
		"readable":  readable,
		"sizeBytes": info.Size(),
		"suffix":    filepath.Ext(path),
	}, nil
}

func inspectRemoteSource(source string) (map[string]any, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	pathType := "remote"
	kind := "remote"
	route := "remote_read"
	token := ""
	ownerID := ""
	sheetID := ""

	if strings.Contains(parsed.Path, "/okr/user/") {
		pathType = "okr"
		kind = "okr"
		route = "okr_detail"
		ownerID = tokenAfter(parsed.Path, "/okr/user/")
		query := parsed.Query()
		token = query.Get("okrId")
		if token == "" {
			token = query.Get("okr_id")
		}
	} else {
		for _, candidate := range []string{"docx", "wiki", "mindnotes", "sheets"} {
			if value := tokenAfter(parsed.Path, "/"+candidate+"/"); value != "" {
				pathType = candidate
				token = value
				break
			}
		}
		switch pathType {
		case "docx":
			kind = "docx"
			route = "docx_client_vars"
		case "wiki":
			kind = "wiki"
			route = "wiki_resolve_then_read"
		case "mindnotes":
			kind = "mindnote"
			route = "mindnote_client_vars"
		case "sheets":
			kind = "sheet"
			route = "sheet_client_vars"
			sheetID = parsed.Query().Get("sheet")
		}
	}

	return map[string]any{
		"ok":          true,
		"sourceRef":   redactedRemoteSource(parsed.Path, parsed.Host, parsed.RawQuery, []string{ownerID, token, sheetID}),
		"remote":      true,
		"kind":        kind,
		"host":        parsed.Host,
		"pathType":    pathType,
		"tokenPrefix": tokenPrefix(token),
		"tokenLength": len(token),
		"route":       route,
	}, nil
}

func tokenAfter(path string, marker string) string {
	index := strings.Index(path, marker)
	if index == -1 {
		return ""
	}
	rest := path[index+len(marker):]
	match := remoteTokenPattern.FindStringSubmatch("/" + rest)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func redactedRemoteSource(path string, host string, rawQuery string, tokens []string) string {
	redactedPath := path
	redactedQuery := rawQuery
	for _, token := range tokens {
		if token == "" {
			continue
		}
		redactedPath = strings.ReplaceAll(redactedPath, token, "<redacted>")
		redactedQuery = strings.ReplaceAll(redactedQuery, token, "<redacted>")
	}
	suffix := ""
	if redactedQuery != "" {
		suffix = "?" + redactedQuery
	}
	return "https://" + host + redactedPath + suffix
}

func tokenPrefix(token string) string {
	if len(token) <= 3 {
		return token
	}
	return token[:3]
}

func originForURL(parsed *url.URL) string {
	return parsed.Scheme + "://" + parsed.Host
}

func codeNumber(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return -1
		}
		return int(parsed)
	default:
		return -1
	}
}

func asMap(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func readBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes":
			return true
		default:
			return false
		}
	case float64:
		return typed != 0
	default:
		return value != nil
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func defaultCookiesPath() string {
	return "/tmp/ixunfei_profile_explorer_cookies.json"
}

func slugify(value string) string {
	text := slugPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	text = strings.Trim(text, "-")
	if text == "" {
		return "doc"
	}
	return text
}

func outputFileStem(result Result, fallback string, usedStems map[string]bool) string {
	base := slugify(fallback)
	if result.Kind == "local_markdown" {
		sourcePath := expandUser(result.Source)
		stem := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		if stem == "" {
			stem = result.Title
		}
		if stem != "" {
			base = slugify(stem)
		}
	}
	candidate := base
	suffix := 2
	for usedStems[candidate] {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
		suffix++
	}
	usedStems[candidate] = true
	return candidate
}

func removeInside(root string, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to remove path outside output directory: %s", target)
	}
	if err := os.Remove(absTarget); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func expandUser(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
