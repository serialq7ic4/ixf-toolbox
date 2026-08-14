package bitable

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	attachmentFieldType = 17
	DefaultSpaceAPI     = "https://internal-api-space.xfchat.iflytek.com"
)

type Field struct {
	ID                   string
	Name                 string
	Type                 int
	TypeName             string
	AttachmentCompatible bool
	raw                  map[string]any
}

type Table struct {
	ID   string
	Name string
}

type View struct {
	ID      string
	Name    string
	TableID string
	Type    int
}

type Record struct {
	ID     string
	Values map[string]string
	Raw    map[string]any
}

type Metadata struct {
	BaseToken string
	Title     string
	Tables    []Table
	Views     []View
	Fields    []Field
	Records   []Record
}

type Source struct {
	Kind      string
	RawURL    string
	BaseURL   string
	BaseToken string
	HostToken string
	TableID   string
	ViewID    string
}

type InspectConfig struct {
	URL         string
	CookiesPath string
	SpaceAPI    string
	ClientVars  map[string]any
}

type AttachConfig struct {
	URL         string
	Field       string
	RecordID    string
	RecordMatch string
	FilePath    string
	DryRun      bool
	Apply       bool
	CookiesPath string
	SpaceAPI    string
	ClientVars  map[string]any
}

type RecordCreateConfig struct {
	URL         string
	InputPath   string
	DryRun      bool
	Apply       bool
	CookiesPath string
	SpaceAPI    string
	ClientVars  map[string]any
}

type fileMetadata struct {
	Name     string
	MimeType string
	Size     int64
}

func ParseSource(rawURL string) (Source, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Source{}, fmt.Errorf("--url must be an absolute HTTP(S) bitable, wiki, or docx URL")
	}
	baseToken := firstNonEmptyString(tokenAfterPath(parsed.Path, "/base/"), tokenAfterPath(parsed.Path, "/bitable/"))
	if baseToken != "" {
		query := parsed.Query()
		return Source{
			Kind:      "direct_bitable",
			RawURL:    strings.TrimSpace(rawURL),
			BaseURL:   parsed.Scheme + "://" + parsed.Host,
			BaseToken: baseToken,
			TableID:   firstNonEmptyString(query.Get("table"), query.Get("table_id"), query.Get("tableId")),
			ViewID:    firstNonEmptyString(query.Get("view"), query.Get("view_id"), query.Get("viewId")),
		}, nil
	}
	if strings.Contains(parsed.Path, "/wiki/") {
		return Source{Kind: "wiki_bitable", RawURL: strings.TrimSpace(rawURL), BaseURL: parsed.Scheme + "://" + parsed.Host}, nil
	}
	if tokenAfterPath(parsed.Path, "/docx/") != "" {
		return Source{
			Kind:      "docx_embedded_bitable",
			RawURL:    strings.TrimSpace(rawURL),
			BaseURL:   parsed.Scheme + "://" + parsed.Host,
			HostToken: tokenAfterPath(parsed.Path, "/docx/"),
		}, nil
	}
	return Source{}, fmt.Errorf("unsupported bitable source URL")
}

func Inspect(config InspectConfig) (map[string]any, error) {
	source, err := ParseSource(config.URL)
	if err != nil {
		return nil, err
	}
	clientVars, source, err := resolveClientVars(source, config.ClientVars, config.CookiesPath, config.SpaceAPI)
	if err != nil {
		return nil, err
	}
	meta, err := ParseClientVars(clientVars, source.BaseToken)
	if err != nil {
		return nil, err
	}
	return inspectPayload(source, meta), nil
}

func Attach(config AttachConfig) (map[string]any, error) {
	if config.Apply && config.DryRun {
		return nil, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	if !config.Apply && !config.DryRun {
		return nil, fmt.Errorf("bitable attach requires --dry-run")
	}
	source, err := ParseSource(config.URL)
	if err != nil {
		return nil, err
	}
	clientVars, source, err := resolveClientVars(source, config.ClientVars, config.CookiesPath, config.SpaceAPI)
	if err != nil {
		return nil, err
	}
	meta, err := ParseClientVars(clientVars, source.BaseToken)
	if err != nil {
		return nil, err
	}
	field := meta.FieldByName(config.Field)
	if field == nil {
		return nil, fmt.Errorf("bitable field %q was not found", config.Field)
	}
	if !field.AttachmentCompatible {
		return nil, fmt.Errorf("bitable field %q is not attachment-compatible", field.Name)
	}
	matches, err := matchingRecords(meta, config.RecordID, config.RecordMatch)
	if err != nil {
		return nil, err
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("record selector matched %d records; expected exactly 1", len(matches))
	}
	file, err := inspectLocalFile(config.FilePath)
	if err != nil {
		return nil, err
	}
	if config.Apply {
		return nil, fmt.Errorf("bitable attach --apply is not available until the bitable upload API contract is captured")
	}
	return attachDryRunPayload(source, meta, *field, matches[0], file), nil
}

func RecordCreate(config RecordCreateConfig) (map[string]any, error) {
	if config.Apply && config.DryRun {
		return nil, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	if !config.Apply && !config.DryRun {
		return nil, fmt.Errorf("bitable record create requires --dry-run")
	}
	source, err := ParseSource(config.URL)
	if err != nil {
		return nil, err
	}
	clientVars, source, err := resolveClientVars(source, config.ClientVars, config.CookiesPath, config.SpaceAPI)
	if err != nil {
		return nil, err
	}
	meta, err := ParseClientVars(clientVars, source.BaseToken)
	if err != nil {
		return nil, err
	}
	fields, err := readRecordCreateFields(config.InputPath)
	if err != nil {
		return nil, err
	}
	plan, attachments, err := planRecordCreateFields(meta, fields)
	if err != nil {
		return nil, err
	}
	if config.Apply {
		return nil, fmt.Errorf("bitable record create --apply is not available until the bitable record API contract is captured")
	}
	return recordCreateDryRunPayload(source, meta, plan, attachments), nil
}

type session struct {
	client    *http.Client
	cookies   []http.Cookie
	csrfToken string
	spaceAPI  string
}

type cookieObject struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

func resolveClientVars(source Source, injected map[string]any, cookiesPath string, spaceAPI string) (map[string]any, Source, error) {
	if len(injected) > 0 {
		return injected, source, nil
	}
	session, err := newSession(cookiesPath, source.BaseURL, spaceAPI)
	if err != nil {
		return nil, source, err
	}
	switch source.Kind {
	case "direct_bitable":
		if source.BaseToken == "" {
			return nil, source, fmt.Errorf("bitable source did not include a base token")
		}
	case "wiki_bitable":
		source, err = session.resolveWikiBitable(source)
		if err != nil {
			return nil, source, err
		}
	case "docx_embedded_bitable":
		source, err = session.resolveDocxEmbeddedBitable(source)
		if err != nil {
			return nil, source, err
		}
	default:
		return nil, source, fmt.Errorf("%s resolver is not supported", source.Kind)
	}
	clientVars, err := session.clientVars(source)
	return clientVars, source, err
}

func newSession(cookiesPath string, defaultSpaceAPI string, spaceAPI string) (*session, error) {
	if strings.TrimSpace(cookiesPath) == "" {
		cookiesPath = "/tmp/ixunfei_profile_explorer_cookies.json"
	}
	cookieObjects, err := loadCookieObjects(cookiesPath)
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
	resolvedSpaceAPI := strings.TrimRight(strings.TrimSpace(spaceAPI), "/")
	if resolvedSpaceAPI == "" {
		resolvedSpaceAPI = strings.TrimRight(strings.TrimSpace(defaultSpaceAPI), "/")
	}
	if resolvedSpaceAPI == "" {
		resolvedSpaceAPI = DefaultSpaceAPI
	}
	return &session{
		client:    &http.Client{Timeout: 30 * time.Second},
		cookies:   cookies,
		csrfToken: csrf,
		spaceAPI:  resolvedSpaceAPI,
	}, nil
}

func (session *session) clientVars(source Source) (map[string]any, error) {
	requestURL := session.spaceAPI + "/space/api/v1/bitable/" + url.PathEscape(source.BaseToken) + "/clientvars" +
		"?tableID=&viewID=&recordLimit=2000&ondemandLimit=200" +
		"&needBase=true&viewLazyLoad=true&ondemandVer=2" +
		"&openType=0&noMissCS=true&optimizationFlag=1"
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	session.addHeaders(request, source.BaseURL, source.RawURL)
	payload, err := session.doJSON(request, "bitable clientvars")
	if err != nil {
		return nil, err
	}
	if intValue(payload["code"]) != 0 {
		return nil, fmt.Errorf("bitable clientvars failed")
	}
	return asMap(payload["data"]), nil
}

func (session *session) resolveWikiBitable(source Source) (Source, error) {
	request, err := http.NewRequest(http.MethodGet, source.RawURL, nil)
	if err != nil {
		return source, err
	}
	session.addHeaders(request, source.BaseURL, source.RawURL)
	body, err := session.doBytes(request, "wiki bitable resolve")
	if err != nil {
		return source, err
	}
	token, err := extractWikiBitableBaseToken(string(body))
	if err != nil {
		return source, err
	}
	source.BaseToken = token
	return source, nil
}

func (session *session) resolveDocxEmbeddedBitable(source Source) (Source, error) {
	if source.HostToken == "" {
		return source, fmt.Errorf("docx embedded bitable source did not include a docx token")
	}
	state, err := session.docxClientVars(source.HostToken, source.BaseURL, source.RawURL)
	if err != nil {
		return source, err
	}
	baseToken, tableID, viewID := discoverDocxEmbeddedBitable(state)
	if baseToken == "" {
		return source, fmt.Errorf("docx embedded bitable resolver could not locate base token in supported view/file blocks")
	}
	source.BaseToken = baseToken
	if source.TableID == "" {
		source.TableID = tableID
	}
	if source.ViewID == "" {
		source.ViewID = viewID
	}
	return source, nil
}

func (session *session) docxClientVars(token string, origin string, referer string) (map[string]any, error) {
	data := map[string]any{}
	cursor := ""
	for {
		query := url.Values{}
		query.Set("id", token)
		query.Set("open_type", "1")
		if cursor != "" {
			query.Set("mode", "4")
			query.Set("cursor", cursor)
		}
		request, err := http.NewRequest(http.MethodGet, session.spaceAPI+"/space/api/docx/pages/client_vars?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		session.addHeaders(request, origin, referer)
		payload, err := session.doJSON(request, "docx client_vars")
		if err != nil {
			return nil, err
		}
		if intValue(payload["code"]) != 0 {
			return nil, fmt.Errorf("docx client_vars failed")
		}
		page := asMap(payload["data"])
		mergeClientVarsPage(data, page)
		cursor = stringValue(page["cursor"])
		if !readBool(page["has_more"]) || cursor == "" {
			return data, nil
		}
	}
}

func (session *session) addHeaders(request *http.Request, origin string, referer string) {
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-CSRFToken", session.csrfToken)
	for _, cookie := range session.cookies {
		request.AddCookie(&cookie)
	}
}

func (session *session) doJSON(request *http.Request, label string) (map[string]any, error) {
	response, err := session.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s http status %d", label, response.StatusCode)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", label)
	}
	return payload, nil
}

func (session *session) doBytes(request *http.Request, label string) ([]byte, error) {
	response, err := session.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s http status %d", label, response.StatusCode)
	}
	return body, nil
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

func extractWikiBitableBaseToken(html string) (string, error) {
	wikiInfoJSON, err := extractBalancedObject(html, "current_space_wiki = Object(")
	if err != nil {
		return "", err
	}
	wikiInfo := map[string]any{}
	if err := json.Unmarshal([]byte(wikiInfoJSON), &wikiInfo); err != nil {
		return "", err
	}
	baseToken := strings.TrimSpace(stringValue(wikiInfo["obj_token"]))
	if baseToken == "" {
		return "", fmt.Errorf("unable to locate bitable base token from wiki HTML")
	}
	return baseToken, nil
}

func discoverDocxEmbeddedBitable(clientVars map[string]any) (string, string, string) {
	blockMap := asMap(clientVars["block_map"])
	for _, entryValue := range blockMap {
		entry := asMap(entryValue)
		candidate := asMap(entry["data"])
		if len(candidate) == 0 {
			candidate = entry
		}
		if !looksLikeBitableBlock(candidate) {
			continue
		}
		baseToken := firstStringByKeysDeep(candidate, []string{
			"base_token", "baseToken", "bitable_token", "bitableToken", "obj_token", "objToken", "token",
		})
		if baseToken == "" {
			continue
		}
		return baseToken,
			firstStringByKeysDeep(candidate, []string{"table_id", "tableId", "tableID"}),
			firstStringByKeysDeep(candidate, []string{"view_id", "viewId", "viewID"})
	}
	baseToken := firstStringByKeysDeep(clientVars, []string{"base_token", "baseToken", "bitable_token", "bitableToken"})
	if baseToken == "" {
		return "", "", ""
	}
	return baseToken,
		firstStringByKeysDeep(clientVars, []string{"table_id", "tableId", "tableID"}),
		firstStringByKeysDeep(clientVars, []string{"view_id", "viewId", "viewID"})
}

func looksLikeBitableBlock(value map[string]any) bool {
	kind := strings.ToLower(firstNonEmptyString(
		value["type"],
		value["block_type"],
		value["blockType"],
		value["suite_type"],
		value["suiteType"],
	))
	if strings.Contains(kind, "bitable") {
		return true
	}
	if firstStringByKeysDeep(value, []string{"base_token", "baseToken", "bitable_token", "bitableToken"}) != "" {
		return true
	}
	if firstStringByKeysDeep(value, []string{"table_id", "tableId", "tableID"}) != "" &&
		firstStringByKeysDeep(value, []string{"view_id", "viewId", "viewID"}) != "" {
		return true
	}
	return false
}

func firstStringByKeysDeep(value any, keys []string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if text := strings.TrimSpace(stringValue(typed[key])); text != "" {
				return text
			}
		}
		for _, item := range typed {
			if text := firstStringByKeysDeep(item, keys); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := firstStringByKeysDeep(item, keys); text != "" {
				return text
			}
		}
	}
	return ""
}

func mergeClientVarsPage(target map[string]any, page map[string]any) {
	for key, value := range page {
		if key == "block_map" {
			targetBlockMap := asMap(target["block_map"])
			if targetBlockMap == nil {
				targetBlockMap = map[string]any{}
			}
			for blockID, blockValue := range asMap(value) {
				targetBlockMap[blockID] = blockValue
			}
			target["block_map"] = targetBlockMap
			continue
		}
		target[key] = value
	}
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

func ParseClientVars(data map[string]any, baseToken string) (Metadata, error) {
	oldSchema, err := decodeGzipJSON(stringValue(asMap(data["oldSchema"])["gzipSchema"]))
	if err != nil {
		return Metadata{}, err
	}
	base := asMap(oldSchema["base"])
	tableData := asMap(asMap(oldSchema["data"])["table"])
	recordMap := asMap(asMap(oldSchema["data"])["recordMap"])
	tableIDs := stringsFromSlice(asSlice(base["tables"]))
	if len(tableIDs) == 0 {
		return Metadata{}, fmt.Errorf("unable to locate bitable table")
	}

	meta := Metadata{
		BaseToken: strings.TrimSpace(baseToken),
		Title:     firstNonEmptyString(base["name"], baseToken),
	}
	for _, tableID := range tableIDs {
		tableInfo := asMap(asMap(base["tableInfos"])[tableID])
		meta.Tables = append(meta.Tables, Table{
			ID:   tableID,
			Name: firstNonEmptyString(tableInfo["name"], tableID),
		})
	}

	viewIDs := stringsFromSlice(asSlice(tableData["views"]))
	viewMap := asMap(tableData["viewMap"])
	selectedView := selectBitableView(viewIDs, viewMap)
	if len(selectedView) == 0 {
		return Metadata{}, fmt.Errorf("unable to locate a renderable bitable view")
	}
	selectedViewID := strings.TrimSpace(stringValue(selectedView["id"]))
	property := asMap(selectedView["property"])
	fieldIDs := stringsFromSlice(asSlice(property["fields"]))
	recordIDs := stringsFromSlice(asSlice(property["records"]))
	fieldMap := asMap(tableData["fieldMap"])
	userMap := bitableUserMap(data, oldSchema)
	tzName := firstNonEmptyString(base["timezone"], data["timeZone"])

	for _, viewID := range viewIDs {
		view := asMap(viewMap[viewID])
		if len(view) == 0 {
			continue
		}
		meta.Views = append(meta.Views, View{
			ID:      firstNonEmptyString(view["id"], viewID),
			Name:    firstNonEmptyString(view["name"], viewID),
			TableID: tableIDs[0],
			Type:    intValue(view["type"]),
		})
	}
	for _, fieldID := range fieldIDs {
		field := asMap(fieldMap[fieldID])
		fieldType := intValue(field["type"])
		meta.Fields = append(meta.Fields, Field{
			ID:                   firstNonEmptyString(field["id"], fieldID),
			Name:                 firstNonEmptyString(field["name"], fieldID),
			Type:                 fieldType,
			TypeName:             bitableFieldTypeName(fieldType),
			AttachmentCompatible: fieldType == attachmentFieldType,
			raw:                  field,
		})
	}

	fieldByID := map[string]Field{}
	for _, field := range meta.Fields {
		fieldByID[field.ID] = field
	}
	for _, recordID := range recordIDs {
		recordRaw := asMap(recordMap[recordID])
		record := Record{
			ID:     recordID,
			Values: map[string]string{},
			Raw:    recordRaw,
		}
		for _, fieldID := range fieldIDs {
			field := fieldByID[fieldID]
			cell := recordRaw[fieldID]
			if cellMap := asMap(cell); len(cellMap) > 0 {
				cell = cellMap["value"]
			}
			record.Values[fieldID] = RenderValue(cell, field, userMap, tzName)
		}
		meta.Records = append(meta.Records, record)
	}

	if selectedViewID != "" && len(meta.Views) == 0 {
		meta.Views = append(meta.Views, View{
			ID:      selectedViewID,
			Name:    firstNonEmptyString(selectedView["name"], selectedViewID),
			TableID: tableIDs[0],
			Type:    intValue(selectedView["type"]),
		})
	}
	return meta, nil
}

func inspectPayload(source Source, meta Metadata) map[string]any {
	fields := make([]map[string]any, 0, len(meta.Fields))
	for _, field := range meta.Fields {
		fields = append(fields, map[string]any{
			"id":                   field.ID,
			"name":                 field.Name,
			"type":                 field.Type,
			"typeName":             field.TypeName,
			"attachmentCompatible": field.AttachmentCompatible,
		})
	}
	tables := make([]map[string]any, 0, len(meta.Tables))
	for _, table := range meta.Tables {
		tables = append(tables, map[string]any{"id": table.ID, "name": table.Name})
	}
	views := make([]map[string]any, 0, len(meta.Views))
	for _, view := range meta.Views {
		views = append(views, map[string]any{"id": view.ID, "name": view.Name, "tableId": view.TableID, "type": view.Type})
	}
	return map[string]any{
		"ok":            true,
		"operation":     "bitable_inspect",
		"sourceKind":    source.Kind,
		"target":        map[string]any{"baseTokenPrefix": tokenPrefix(meta.BaseToken), "tableId": firstTableID(meta), "viewId": firstViewID(meta)},
		"title":         meta.Title,
		"tables":        tables,
		"views":         views,
		"fields":        fields,
		"recordCount":   len(meta.Records),
		"hasAttachment": metadataHasAttachment(meta),
	}
}

func attachDryRunPayload(source Source, meta Metadata, field Field, record Record, file fileMetadata) map[string]any {
	return map[string]any{
		"ok":               true,
		"dryRun":           true,
		"operation":        "bitable_attach",
		"sourceKind":       source.Kind,
		"target":           map[string]any{"baseTokenPrefix": tokenPrefix(meta.BaseToken), "tableId": firstTableID(meta), "viewId": firstViewID(meta)},
		"recordMatchCount": 1,
		"record":           map[string]any{"id": record.ID},
		"field": map[string]any{
			"id":                   field.ID,
			"name":                 field.Name,
			"type":                 field.TypeName,
			"attachmentCompatible": field.AttachmentCompatible,
		},
		"file": map[string]any{
			"name":      file.Name,
			"mimeType":  file.MimeType,
			"sizeBytes": file.Size,
		},
		"willUpload":       true,
		"willUpdateRecord": true,
	}
}

func recordCreateDryRunPayload(source Source, meta Metadata, fields []map[string]any, attachments []map[string]any) map[string]any {
	return map[string]any{
		"ok":                     true,
		"dryRun":                 true,
		"operation":              "bitable_record_create",
		"sourceKind":             source.Kind,
		"target":                 map[string]any{"baseTokenPrefix": tokenPrefix(meta.BaseToken), "tableId": firstTableID(meta), "viewId": firstViewID(meta)},
		"fieldCount":             len(fields),
		"fields":                 fields,
		"plannedAttachmentCount": len(attachments),
		"attachments":            attachments,
		"willCreateRecord":       true,
	}
}

func readRecordCreateFields(path string) (map[string]any, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("bitable record create requires --input")
	}
	content, err := os.ReadFile(expandUser(path))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("record create input must be JSON")
	}
	fields := asMap(payload["fields"])
	if len(fields) == 0 {
		fields = payload
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("record create input must include at least one field")
	}
	return fields, nil
}

func planRecordCreateFields(meta Metadata, fields map[string]any) ([]map[string]any, []map[string]any, error) {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	plannedFields := make([]map[string]any, 0, len(names))
	attachments := []map[string]any{}
	for _, name := range names {
		field := meta.FieldByName(name)
		if field == nil {
			return nil, nil, fmt.Errorf("field %q was not found", name)
		}
		value := fields[name]
		entry := map[string]any{
			"id":                   field.ID,
			"name":                 field.Name,
			"type":                 field.TypeName,
			"attachmentCompatible": field.AttachmentCompatible,
		}
		if field.AttachmentCompatible {
			files, err := inspectAttachmentValueFiles(value)
			if err != nil {
				return nil, nil, fmt.Errorf("attachment field %q: %w", field.Name, err)
			}
			entry["attachmentCount"] = len(files)
			for _, file := range files {
				attachments = append(attachments, map[string]any{
					"fieldId":   field.ID,
					"fieldName": field.Name,
					"file": map[string]any{
						"name":      file.Name,
						"mimeType":  file.MimeType,
						"sizeBytes": file.Size,
					},
				})
			}
		} else {
			entry["valuePreview"] = renderSheetValue(value)
		}
		plannedFields = append(plannedFields, entry)
	}
	return plannedFields, attachments, nil
}

func inspectAttachmentValueFiles(value any) ([]fileMetadata, error) {
	paths, err := attachmentPathsFromValue(value)
	if err != nil {
		return nil, err
	}
	files := make([]fileMetadata, 0, len(paths))
	for _, path := range paths {
		file, err := inspectLocalFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func attachmentPathsFromValue(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return []string{}, nil
		}
		return []string{typed}, nil
	case map[string]any:
		path := firstNonEmptyString(typed["file"], typed["path"])
		if path == "" {
			return nil, fmt.Errorf("attachment object must include file or path")
		}
		return []string{path}, nil
	case []any:
		paths := []string{}
		for _, item := range typed {
			itemPaths, err := attachmentPathsFromValue(item)
			if err != nil {
				return nil, err
			}
			paths = append(paths, itemPaths...)
		}
		return paths, nil
	case nil:
		return []string{}, nil
	default:
		return nil, fmt.Errorf("attachment value must be a path string, object, or array")
	}
}

func matchingRecords(meta Metadata, recordID string, recordMatch string) ([]Record, error) {
	recordID = strings.TrimSpace(recordID)
	recordMatch = strings.TrimSpace(recordMatch)
	if recordID == "" && recordMatch == "" {
		return nil, fmt.Errorf("bitable attach requires --record-id or --record-match")
	}
	if recordID != "" && recordMatch != "" {
		return nil, fmt.Errorf("use only one of --record-id or --record-match")
	}
	if recordID != "" {
		for _, record := range meta.Records {
			if record.ID == recordID {
				return []Record{record}, nil
			}
		}
		return []Record{}, nil
	}
	name, value, ok := strings.Cut(recordMatch, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("--record-match must use Field=Value")
	}
	field := meta.FieldByName(strings.TrimSpace(name))
	if field == nil {
		return nil, fmt.Errorf("record match field %q was not found", strings.TrimSpace(name))
	}
	expected := strings.TrimSpace(value)
	matches := []Record{}
	for _, record := range meta.Records {
		if strings.TrimSpace(record.Values[field.ID]) == expected {
			matches = append(matches, record)
		}
	}
	return matches, nil
}

func inspectLocalFile(path string) (fileMetadata, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return fileMetadata{}, fmt.Errorf("bitable attach requires --file")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileMetadata{}, err
	}
	if !info.Mode().IsRegular() {
		return fileMetadata{}, fmt.Errorf("--file must point to a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return fileMetadata{}, err
	}
	defer file.Close()
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return fileMetadata{}, err
	}
	return fileMetadata{
		Name:     filepath.Base(path),
		MimeType: http.DetectContentType(buffer[:n]),
		Size:     info.Size(),
	}, nil
}

func firstTableID(meta Metadata) string {
	if len(meta.Tables) == 0 {
		return ""
	}
	return meta.Tables[0].ID
}

func firstViewID(meta Metadata) string {
	if len(meta.Views) == 0 {
		return ""
	}
	return meta.Views[0].ID
}

func metadataHasAttachment(meta Metadata) bool {
	for _, field := range meta.Fields {
		if field.AttachmentCompatible {
			return true
		}
	}
	return false
}

func (m Metadata) FieldByName(name string) *Field {
	normalized := strings.TrimSpace(name)
	for index := range m.Fields {
		field := &m.Fields[index]
		if field.ID == normalized || field.Name == normalized {
			return field
		}
	}
	return nil
}

func RenderValue(value any, field Field, userMap map[string]string, tzName string) string {
	switch field.Type {
	case 3:
		return renderBitableOptionValue(value, bitableFieldOptionMap(field.raw))
	case 5:
		return formatBitableDatetime(value, field.raw, tzName)
	case 11:
		return renderBitableUserValue(value, userMap)
	default:
		return renderSheetValue(value)
	}
}

func selectBitableView(viewIDs []string, viewMap map[string]any) map[string]any {
	for _, viewID := range viewIDs {
		view := asMap(viewMap[viewID])
		if intValue(view["type"]) == 1 {
			return view
		}
	}
	if len(viewIDs) == 0 {
		return map[string]any{}
	}
	return asMap(viewMap[viewIDs[0]])
}

func bitableFieldTypeName(fieldType int) string {
	switch fieldType {
	case attachmentFieldType:
		return "attachment"
	case 1:
		return "text"
	case 3:
		return "single_select"
	case 5:
		return "datetime"
	case 11:
		return "user"
	default:
		if fieldType == 0 {
			return ""
		}
		return strconv.Itoa(fieldType)
	}
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

func tokenPrefix(token string) string {
	if len(token) <= 3 {
		return token
	}
	return token[:3]
}

func renderBitableOptionValue(value any, optionMap map[string]string) string {
	if value == nil {
		return ""
	}
	values := stringsFromValue(value)
	rendered := make([]string, 0, len(values))
	for _, item := range values {
		if label := optionMap[item]; label != "" {
			rendered = append(rendered, label)
			continue
		}
		rendered = append(rendered, item)
	}
	return strings.Join(rendered, ", ")
}

func renderBitableUserValue(value any, userMap map[string]string) string {
	if value == nil {
		return ""
	}
	values := stringsFromValue(value)
	rendered := make([]string, 0, len(values))
	for _, item := range values {
		if name := userMap[item]; name != "" {
			rendered = append(rendered, name)
			continue
		}
		rendered = append(rendered, item)
	}
	return strings.Join(rendered, ", ")
}

func bitableFieldOptionMap(field map[string]any) map[string]string {
	optionMap := map[string]string{}
	for _, optionValue := range asSlice(asMap(field["property"])["options"]) {
		option := asMap(optionValue)
		id := strings.TrimSpace(stringValue(option["id"]))
		if id != "" {
			optionMap[id] = stringValue(option["name"])
		}
	}
	return optionMap
}

func bitableUserMap(clientVarsData map[string]any, oldSchema map[string]any) map[string]string {
	userMap := map[string]string{}
	addUsers := func(users map[string]any) {
		for userID, userValue := range users {
			user := asMap(userValue)
			if name := firstNonEmptyString(user["name"], user["enName"], user["displayName"]); name != "" {
				userMap[userID] = name
			}
		}
	}
	addUsers(asMap(clientVarsData["users"]))
	addUsers(asMap(asMap(clientVarsData["oldSchema"])["users"]))
	addUsers(asMap(oldSchema["users"]))
	return userMap
}

func formatBitableDatetime(value any, field map[string]any, tzName string) string {
	timestamp, ok := floatValue(value)
	if !ok {
		return renderSheetValue(value)
	}
	if timestamp > 1e12 {
		timestamp /= 1000
	}
	location := time.UTC
	if tzName != "" {
		if loaded, err := time.LoadLocation(tzName); err == nil {
			location = loaded
		}
	}
	seconds := int64(timestamp)
	nanos := int64((timestamp - float64(seconds)) * 1e9)
	dateTime := time.Unix(seconds, nanos).In(location)
	property := asMap(field["property"])
	dateLayout := bitableTimeLayout(stringValue(property["dateFormat"]))
	if dateLayout == "" {
		dateLayout = "2006/01/02"
	}
	timeLayout := bitableTimeLayout(strings.TrimSpace(stringValue(property["timeFormat"])))
	if timeLayout != "" {
		dateLayout += " " + timeLayout
	}
	return dateTime.Format(dateLayout)
}

func bitableTimeLayout(format string) string {
	replacer := strings.NewReplacer(
		"yyyy", "2006",
		"MM", "01",
		"dd", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)
	return replacer.Replace(format)
}

func decodeGzipJSON(encoded string) (map[string]any, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if err := json.Unmarshal(content, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func renderSheetValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		parts := []string{}
		for _, item := range typed {
			if rendered := renderSheetValue(item); rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		if value, ok := typed["text"]; ok {
			return renderSheetValue(value)
		}
		for _, key := range []string{"value", "formattedValue", "displayValue"} {
			if value, ok := typed[key]; ok {
				if rendered := renderSheetValue(value); rendered != "" {
					return rendered
				}
			}
		}
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func stringsFromSlice(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text := stringValue(value); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func stringsFromValue(value any) []string {
	if slice, ok := value.([]any); ok {
		return stringsFromSlice(slice)
	}
	text := stringValue(value)
	if text == "" {
		return []string{}
	}
	return []string{text}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(stringValue(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func floatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	default:
		return 0
	}
}

func readBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func asSlice(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return []any{}
}
