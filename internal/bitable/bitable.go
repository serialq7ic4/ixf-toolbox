package bitable

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"io"
	"math/big"
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
	attachmentFieldType   = 17
	bitableVerifyTimeout  = 10 * time.Second
	bitableVerifyInterval = 750 * time.Millisecond
	recordInsertTop       = "top"
	recordInsertBottom    = "bottom"
	DefaultSpaceAPI       = "https://internal-api-space.xfchat.iflytek.com"
	DefaultDriveStreamAPI = "https://internal-api-drive-stream.xfchat.iflytek.com"
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
	TableRev  int
	Tables    []Table
	Views     []View
	Fields    []Field
	Records   []Record
}

type bitableUploadedFile struct {
	Token     string
	Name      string
	MimeType  string
	Size      int64
	Timestamp int64
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
	URL            string
	InputPath      string
	InsertPosition string
	DryRun         bool
	Apply          bool
	CookiesPath    string
	SpaceAPI       string
	ClientVars     map[string]any
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
		return nil, fmt.Errorf("bitable attach requires --dry-run or --apply")
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
		session, err := newSession(config.CookiesPath, source.BaseURL, config.SpaceAPI)
		if err != nil {
			return nil, err
		}
		return session.applyAttach(source, meta, *field, matches[0], config.FilePath, file)
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
	insertPosition, err := normalizeRecordInsertPosition(config.InsertPosition)
	if err != nil {
		return nil, err
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
		if err := validateRecordCreateApplyFields(meta, fields); err != nil {
			return nil, err
		}
		session, err := newSession(config.CookiesPath, source.BaseURL, config.SpaceAPI)
		if err != nil {
			return nil, err
		}
		return session.applyRecordCreate(source, meta, fields, plan, insertPosition)
	}
	return recordCreateDryRunPayload(source, meta, plan, attachments, insertPosition), nil
}

type session struct {
	client         *http.Client
	cookies        []http.Cookie
	csrfToken      string
	spaceAPI       string
	driveStreamAPI string
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
	resolvedDriveStreamAPI := DefaultDriveStreamAPI
	if resolvedSpaceAPI == "" {
		resolvedSpaceAPI = strings.TrimRight(strings.TrimSpace(defaultSpaceAPI), "/")
	} else {
		resolvedDriveStreamAPI = resolvedSpaceAPI
	}
	if resolvedSpaceAPI == "" {
		resolvedSpaceAPI = DefaultSpaceAPI
	}
	return &session{
		client:         &http.Client{Timeout: 30 * time.Second},
		cookies:        cookies,
		csrfToken:      csrf,
		spaceAPI:       resolvedSpaceAPI,
		driveStreamAPI: resolvedDriveStreamAPI,
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
		TableRev:  intValue(asMap(tableData["meta"])["rev"]),
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

func attachApplyPayload(source Source, meta Metadata, field Field, recordID string, uploadedFileCount int, verify map[string]any) map[string]any {
	return map[string]any{
		"ok":                true,
		"dryRun":            false,
		"applied":           true,
		"operation":         "bitable_attach",
		"sourceKind":        source.Kind,
		"target":            map[string]any{"baseTokenPrefix": tokenPrefix(meta.BaseToken), "tableId": firstTableID(meta), "viewId": firstViewID(meta)},
		"fieldName":         field.Name,
		"recordId":          recordID,
		"uploadedFileCount": uploadedFileCount,
		"willUpload":        true,
		"willUpdateRecord":  true,
		"verify":            verify,
	}
}

func normalizeRecordInsertPosition(position string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "", recordInsertBottom:
		return recordInsertBottom, nil
	case recordInsertTop:
		return recordInsertTop, nil
	default:
		return "", fmt.Errorf("unsupported bitable record insert position %q; use top or bottom", position)
	}
}

func plannedRecordIndex(meta Metadata, insertPosition string) int {
	if insertPosition == recordInsertTop {
		return 0
	}
	return len(meta.Records)
}

func recordCreateDryRunPayload(source Source, meta Metadata, fields []map[string]any, attachments []map[string]any, insertPosition string) map[string]any {
	recordIndex := plannedRecordIndex(meta, insertPosition)
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
		"insertPosition":         insertPosition,
		"plannedRecordIndex":     recordIndex,
		"willCreateRecord":       true,
	}
}

func recordCreateApplyPayload(source Source, meta Metadata, fields []map[string]any, recordID string, uploadedFileCount int, insertPosition string, recordIndex int, verify map[string]any) map[string]any {
	return map[string]any{
		"ok":                 true,
		"dryRun":             false,
		"applied":            true,
		"operation":          "bitable_record_create",
		"sourceKind":         source.Kind,
		"target":             map[string]any{"baseTokenPrefix": tokenPrefix(meta.BaseToken), "tableId": firstTableID(meta), "viewId": firstViewID(meta)},
		"fieldCount":         len(fields),
		"fields":             fields,
		"recordId":           recordID,
		"uploadedFileCount":  uploadedFileCount,
		"insertPosition":     insertPosition,
		"plannedRecordIndex": recordIndex,
		"verify":             verify,
	}
}

func validateRecordCreateApplyFields(meta Metadata, fields map[string]any) error {
	names := sortedFieldNames(fields)
	for _, name := range names {
		field := meta.FieldByName(name)
		if field == nil {
			return fmt.Errorf("field %q was not found", name)
		}
		if field.Type != 1 && !field.AttachmentCompatible {
			return fmt.Errorf("unsupported apply field type %q for field %q", field.TypeName, field.Name)
		}
	}
	return nil
}

func (session *session) applyRecordCreate(source Source, meta Metadata, fields map[string]any, plan []map[string]any, insertPosition string) (map[string]any, error) {
	tableID := targetTableID(source, meta)
	viewID := targetViewID(source, meta)
	if tableID == "" || viewID == "" {
		return nil, fmt.Errorf("bitable record create apply requires a table and view")
	}
	recordIndex := plannedRecordIndex(meta, insertPosition)
	memberID, err := randomDecimalID(14)
	if err != nil {
		return nil, err
	}
	if err := session.watchBitableEntity(source, memberID, "BITABLE_BASE", source.BaseToken, 2); err != nil {
		return nil, err
	}
	if err := session.watchBitableEntity(source, memberID, "BITABLE_TABLE", tableID, 0); err != nil {
		return nil, err
	}
	if err := session.prepareAddRecordToken(source, tableID); err != nil {
		return nil, err
	}
	uploads := map[string][]bitableUploadedFile{}
	uploadedFileCount := 0
	for _, name := range sortedFieldNames(fields) {
		field := meta.FieldByName(name)
		if field == nil || !field.AttachmentCompatible {
			continue
		}
		paths, err := attachmentPathsFromValue(fields[name])
		if err != nil {
			return nil, fmt.Errorf("attachment field %q: %w", field.Name, err)
		}
		for _, path := range paths {
			file, err := inspectLocalFile(path)
			if err != nil {
				return nil, err
			}
			uploaded, err := session.uploadBitableFile(source, path, file)
			if err != nil {
				return nil, err
			}
			uploads[field.ID] = append(uploads[field.ID], uploaded)
			uploadedFileCount++
		}
	}
	recordID, err := session.writeRecordCreate(source, meta, fields, uploads, memberID, recordIndex)
	if err != nil {
		return nil, err
	}
	verify, err := session.waitForRecordCreateVerification(source, fields, uploads, recordID, recordIndex)
	if err != nil {
		return nil, err
	}
	return recordCreateApplyPayload(source, meta, plan, recordID, uploadedFileCount, insertPosition, recordIndex, verify), nil
}

func (session *session) prepareAddRecordToken(source Source, tableID string) error {
	payload, err := session.postJSON(session.spaceAPI+"/space/api/bitable/"+url.PathEscape(source.BaseToken)+"/add_record/token", source, map[string]any{
		"tableID": tableID,
	}, "bitable add_record token")
	if err != nil {
		return err
	}
	if intValue(payload["code"]) != 0 {
		return fmt.Errorf("bitable add_record token failed")
	}
	return nil
}

func (session *session) uploadBitableFile(source Source, path string, file fileMetadata) (bitableUploadedFile, error) {
	content, err := os.ReadFile(expandUser(path))
	if err != nil {
		return bitableUploadedFile{}, err
	}
	preparePayload, err := session.postJSON(session.spaceAPI+"/space/api/box/upload/prepare/", source, map[string]any{
		"mount_point":      "bitable_image",
		"mount_node_token": source.BaseToken,
		"name":             file.Name,
		"size":             file.Size,
		"size_checker":     false,
	}, "bitable upload prepare")
	if err != nil {
		return bitableUploadedFile{}, err
	}
	if intValue(preparePayload["code"]) != 0 {
		return bitableUploadedFile{}, fmt.Errorf("bitable upload prepare failed")
	}
	prepareData := asMap(preparePayload["data"])
	uploadID := firstNonEmptyString(prepareData["upload_id"], prepareData["uploadId"])
	if uploadID == "" {
		return bitableUploadedFile{}, fmt.Errorf("bitable upload prepare did not return upload_id")
	}
	blockSize := intValue(prepareData["block_size"])
	if blockSize <= 0 {
		blockSize = len(content)
	}
	if blockSize <= 0 {
		blockSize = 1
	}
	numBlocks := intValue(prepareData["num_blocks"])
	computedBlocks := (len(content) + blockSize - 1) / blockSize
	if numBlocks <= 0 {
		numBlocks = computedBlocks
	}
	for seq := 0; seq < computedBlocks; seq++ {
		start := seq * blockSize
		end := start + blockSize
		if end > len(content) {
			end = len(content)
		}
		chunk := content[start:end]
		if err := session.uploadBitableChunk(source, uploadID, seq, blockSize, chunk); err != nil {
			return bitableUploadedFile{}, err
		}
	}
	finishPayload, err := session.postJSON(session.spaceAPI+"/space/api/box/upload/finish/", source, map[string]any{
		"upload_id":                uploadID,
		"num_blocks":               numBlocks,
		"mount_point":              "bitable_image",
		"push_open_history_record": 0,
	}, "bitable upload finish")
	if err != nil {
		return bitableUploadedFile{}, err
	}
	if intValue(finishPayload["code"]) != 0 {
		return bitableUploadedFile{}, fmt.Errorf("bitable upload finish failed")
	}
	finishData := asMap(finishPayload["data"])
	token := firstNonEmptyString(finishData["file_token"], finishData["fileToken"], finishData["token"])
	if token == "" {
		return bitableUploadedFile{}, fmt.Errorf("bitable upload finish did not return file_token")
	}
	return bitableUploadedFile{
		Token:     token,
		Name:      file.Name,
		MimeType:  file.MimeType,
		Size:      file.Size,
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func (session *session) uploadBitableChunk(source Source, uploadID string, seq int, blockSize int, chunk []byte) error {
	query := url.Values{}
	query.Set("upload_id", uploadID)
	query.Set("mount_point", "bitable_image")
	requestURL := session.driveStreamAPI + "/space/api/box/stream/upload/merge_block/?" + query.Encode()
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(chunk))
	if err != nil {
		return err
	}
	session.addHeaders(request, source.BaseURL, source.RawURL)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("x-seq-list", strconv.Itoa(seq))
	request.Header.Set("x-block-list-checksum", strconv.FormatUint(uint64(adler32.Checksum(chunk)), 10))
	request.Header.Set("x-block-origin-size", strconv.Itoa(blockSize))
	payload, err := session.doJSON(request, "bitable upload merge_block")
	if err != nil {
		return err
	}
	if intValue(payload["code"]) != 0 {
		return fmt.Errorf("bitable upload merge_block failed")
	}
	return nil
}

func (session *session) watchBitableEntity(source Source, memberID string, entityType string, token string, schemaVersion int) error {
	memberNumber, err := strconv.ParseInt(memberID, 10, 64)
	if err != nil {
		return err
	}
	body := map[string]any{
		"type": "COLLABROOM",
		"data": map[string]any{
			"member_id":   memberNumber,
			"user_ticket": "",
			"type":        "WATCH",
			"entities": []any{map[string]any{
				"route_key":      source.BaseToken,
				"route_type":     "token",
				"type":           entityType,
				"token":          token,
				"schema_version": schemaVersion,
			}},
		},
		"version": 2,
		"req_id":  randomSmallInt(),
		"context": bitableRCEContext(),
	}
	payload, err := session.postJSON(session.rceMessagesURL(memberID), source, body, "bitable rce watch")
	if err != nil {
		return err
	}
	if intValue(payload["code"]) != 0 {
		return fmt.Errorf("bitable rce watch failed")
	}
	return nil
}

func (session *session) applyAttach(source Source, meta Metadata, field Field, record Record, path string, file fileMetadata) (map[string]any, error) {
	tableID := targetTableID(source, meta)
	viewID := targetViewID(source, meta)
	if tableID == "" || viewID == "" {
		return nil, fmt.Errorf("bitable attach apply requires a table and view")
	}
	memberID, err := randomDecimalID(14)
	if err != nil {
		return nil, err
	}
	if err := session.watchBitableEntity(source, memberID, "BITABLE_BASE", source.BaseToken, 2); err != nil {
		return nil, err
	}
	if err := session.watchBitableEntity(source, memberID, "BITABLE_TABLE", tableID, 0); err != nil {
		return nil, err
	}
	uploaded, err := session.uploadBitableFile(source, path, file)
	if err != nil {
		return nil, err
	}
	uploads := []bitableUploadedFile{uploaded}
	if err := session.writeRecordAttachmentUpdate(source, meta, field, record, uploads, memberID); err != nil {
		return nil, err
	}
	verify, err := session.waitForAttachVerification(source, field, record.ID, uploads)
	if err != nil {
		return nil, err
	}
	return attachApplyPayload(source, meta, field, record.ID, len(uploads), verify), nil
}

func (session *session) writeRecordAttachmentUpdate(source Source, meta Metadata, field Field, record Record, uploads []bitableUploadedFile, memberID string) error {
	tableID := targetTableID(source, meta)
	viewID := targetViewID(source, meta)
	values := recordAttachmentValues(record, field.ID)
	for _, upload := range uploads {
		values = append(values, uploadedFileCellValue(upload))
	}
	operations := []any{map[string]any{
		"command": "SetRecord",
		"type":    2,
		"actions": []any{map[string]any{
			"action":   "data.setRecord",
			"type":     2,
			"tableId":  tableID,
			"viewId":   viewID,
			"recordId": record.ID,
			"viewType": targetViewType(source, meta),
			"data": map[string]any{
				field.ID: map[string]any{"type": attachmentFieldType, "value": values},
			},
		}},
		"syncFlag": 0,
	}}
	encodedOperations, err := encodeGzipBase64JSON(operations)
	if err != nil {
		return err
	}
	memberNumber, err := strconv.ParseInt(memberID, 10, 64)
	if err != nil {
		return err
	}
	body := map[string]any{
		"type": "BITABLE_TABLE",
		"data": map[string]any{
			"member_id":    memberNumber,
			"user_ticket":  "",
			"type":         "USER_CHANGES",
			"token":        tableID,
			"lang":         "zh",
			"localRev":     meta.TableRev,
			"operations":   encodedOperations,
			"signature":    randomUUID(),
			"content_type": "gzip/base64",
			"route_key":    source.BaseToken,
		},
		"version": 2,
		"req_id":  randomSmallInt(),
		"context": bitableRCEContext(),
	}
	payload, err := session.postJSON(session.rceMessagesURL(memberID), source, body, "bitable rce user_changes")
	if err != nil {
		return err
	}
	if intValue(payload["code"]) != 0 {
		return fmt.Errorf("bitable rce user_changes failed")
	}
	if dataType := stringValue(asMap(payload["data"])["type"]); dataType != "" && dataType != "ACCEPT_COMMIT" {
		return fmt.Errorf("bitable rce user_changes returned %s", dataType)
	}
	return nil
}

func (session *session) writeRecordCreate(source Source, meta Metadata, fields map[string]any, uploads map[string][]bitableUploadedFile, memberID string, recordIndex int) (string, error) {
	tableID := targetTableID(source, meta)
	viewID := targetViewID(source, meta)
	recordID, err := randomRecordID()
	if err != nil {
		return "", err
	}
	cellData := map[string]any{}
	for _, name := range sortedFieldNames(fields) {
		field := meta.FieldByName(name)
		if field == nil {
			return "", fmt.Errorf("field %q was not found", name)
		}
		if field.AttachmentCompatible {
			values := []any{}
			for _, upload := range uploads[field.ID] {
				values = append(values, map[string]any{
					"id":              upload.Token,
					"attachmentToken": upload.Token,
					"name":            upload.Name,
					"mimeType":        upload.MimeType,
					"size":            upload.Size,
					"timeStamp":       upload.Timestamp,
				})
			}
			cellData[field.ID] = map[string]any{"type": attachmentFieldType, "value": values}
			continue
		}
		cellData[field.ID] = map[string]any{
			"type": 1,
			"value": []any{map[string]any{
				"type": "text",
				"text": renderSheetValue(fields[name]),
			}},
		}
	}
	operations := []any{map[string]any{
		"command": "AddRecord",
		"type":    2,
		"actions": []any{map[string]any{
			"action":   "data.addRecord",
			"type":     2,
			"tableId":  tableID,
			"viewId":   viewID,
			"recordId": recordID,
			"data": map[string]any{
				"indexes":          map[string]any{viewID: recordIndex},
				"cellData":         cellData,
				"createdExtraInfo": map[string]any{"name": "", "enName": "", "avatarUrl": ""},
				"total":            len(meta.Records) + 1,
			},
		}},
		"syncFlag": 0,
	}}
	encodedOperations, err := encodeGzipBase64JSON(operations)
	if err != nil {
		return "", err
	}
	memberNumber, err := strconv.ParseInt(memberID, 10, 64)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"type": "BITABLE_TABLE",
		"data": map[string]any{
			"member_id":    memberNumber,
			"user_ticket":  "",
			"type":         "USER_CHANGES",
			"token":        tableID,
			"lang":         "zh",
			"localRev":     meta.TableRev,
			"operations":   encodedOperations,
			"signature":    randomUUID(),
			"content_type": "gzip/base64",
			"route_key":    source.BaseToken,
		},
		"version": 2,
		"req_id":  randomSmallInt(),
		"context": bitableRCEContext(),
	}
	payload, err := session.postJSON(session.rceMessagesURL(memberID), source, body, "bitable rce user_changes")
	if err != nil {
		return "", err
	}
	if intValue(payload["code"]) != 0 {
		return "", fmt.Errorf("bitable rce user_changes failed")
	}
	if dataType := stringValue(asMap(payload["data"])["type"]); dataType != "" && dataType != "ACCEPT_COMMIT" {
		return "", fmt.Errorf("bitable rce user_changes returned %s", dataType)
	}
	return recordID, nil
}

func (session *session) waitForRecordCreateVerification(source Source, fields map[string]any, uploads map[string][]bitableUploadedFile, recordID string, expectedRecordIndex int) (map[string]any, error) {
	deadline := time.Now().Add(bitableVerifyTimeout)
	var lastErr error
	for {
		clientVars, err := session.clientVars(source)
		if err != nil {
			return nil, err
		}
		meta, err := ParseClientVars(clientVars, source.BaseToken)
		if err != nil {
			return nil, err
		}
		verify, err := verifyCreatedRecord(meta, fields, uploads, recordID, expectedRecordIndex)
		if err == nil {
			return verify, nil
		}
		lastErr = err
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("%w after waiting %s", lastErr, bitableVerifyTimeout)
		}
		time.Sleep(bitableVerifyInterval)
	}
}

func (session *session) waitForAttachVerification(source Source, field Field, recordID string, uploads []bitableUploadedFile) (map[string]any, error) {
	deadline := time.Now().Add(bitableVerifyTimeout)
	var lastErr error
	for {
		clientVars, err := session.clientVars(source)
		if err != nil {
			return nil, err
		}
		meta, err := ParseClientVars(clientVars, source.BaseToken)
		if err != nil {
			return nil, err
		}
		verify, err := verifyAttachedRecord(meta, field, recordID, uploads)
		if err == nil {
			return verify, nil
		}
		lastErr = err
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("%w after waiting %s", lastErr, bitableVerifyTimeout)
		}
		time.Sleep(bitableVerifyInterval)
	}
}

func verifyCreatedRecord(meta Metadata, fields map[string]any, uploads map[string][]bitableUploadedFile, recordID string, expectedRecordIndex int) (map[string]any, error) {
	for index, record := range meta.Records {
		if recordID != "" && record.ID != recordID {
			continue
		}
		if recordMatchesExpected(meta, record, fields, uploads) {
			return recordCreateVerifyPayload(record.ID, index, expectedRecordIndex)
		}
	}
	if recordID != "" {
		return nil, fmt.Errorf("created record %s was not found or did not match expected values", recordID)
	}
	for index, record := range meta.Records {
		if recordMatchesExpected(meta, record, fields, uploads) {
			return recordCreateVerifyPayload(record.ID, index, expectedRecordIndex)
		}
	}
	return nil, fmt.Errorf("created record was not found")
}

func verifyAttachedRecord(meta Metadata, field Field, recordID string, uploads []bitableUploadedFile) (map[string]any, error) {
	for index, record := range meta.Records {
		if record.ID != recordID {
			continue
		}
		names := recordAttachmentNames(record, field.ID)
		for _, upload := range uploads {
			if !containsString(names, upload.Name) {
				return nil, fmt.Errorf("record %s attachment field %s did not include %s", recordID, field.Name, upload.Name)
			}
		}
		return map[string]any{
			"ok":          true,
			"recordId":    record.ID,
			"recordIndex": index,
		}, nil
	}
	return nil, fmt.Errorf("record %s was not found", recordID)
}

func recordCreateVerifyPayload(recordID string, recordIndex int, expectedRecordIndex int) (map[string]any, error) {
	if expectedRecordIndex >= 0 && recordIndex != expectedRecordIndex {
		return nil, fmt.Errorf("created record index = %d, want %d", recordIndex, expectedRecordIndex)
	}
	return map[string]any{
		"ok":                  true,
		"recordId":            recordID,
		"recordIndex":         recordIndex,
		"expectedRecordIndex": expectedRecordIndex,
	}, nil
}

func recordMatchesExpected(meta Metadata, record Record, fields map[string]any, uploads map[string][]bitableUploadedFile) bool {
	for _, name := range sortedFieldNames(fields) {
		field := meta.FieldByName(name)
		if field == nil {
			return false
		}
		if field.AttachmentCompatible {
			names := recordAttachmentNames(record, field.ID)
			for _, upload := range uploads[field.ID] {
				if !containsString(names, upload.Name) {
					return false
				}
			}
			continue
		}
		if strings.TrimSpace(record.Values[field.ID]) != strings.TrimSpace(renderSheetValue(fields[name])) {
			return false
		}
	}
	return true
}

func recordAttachmentNames(record Record, fieldID string) []string {
	cell := asMap(record.Raw[fieldID])
	value := cell["value"]
	if len(cell) == 0 {
		value = record.Raw[fieldID]
	}
	names := []string{}
	for _, item := range asSlice(value) {
		name := stringValue(asMap(item)["name"])
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func recordAttachmentValues(record Record, fieldID string) []any {
	cell := asMap(record.Raw[fieldID])
	value := cell["value"]
	if len(cell) == 0 {
		value = record.Raw[fieldID]
	}
	values := []any{}
	for _, item := range asSlice(value) {
		itemMap := asMap(item)
		if len(itemMap) == 0 {
			continue
		}
		copied := map[string]any{}
		for key, value := range itemMap {
			copied[key] = value
		}
		values = append(values, copied)
	}
	return values
}

func uploadedFileCellValue(upload bitableUploadedFile) map[string]any {
	return map[string]any{
		"id":              upload.Token,
		"attachmentToken": upload.Token,
		"name":            upload.Name,
		"mimeType":        upload.MimeType,
		"size":            upload.Size,
		"timeStamp":       upload.Timestamp,
	}
}

func (session *session) postJSON(requestURL string, source Source, body map[string]any, label string) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	session.addHeaders(request, source.BaseURL, source.RawURL)
	request.Header.Set("Content-Type", "application/json")
	return session.doJSON(request, label)
}

func (session *session) rceMessagesURL(memberID string) string {
	query := url.Values{}
	query.Set("member_id", memberID)
	return session.spaceAPI + "/space/api/rce/messages?" + query.Encode()
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
	expandedPath := expandUser(path)
	info, err := os.Stat(expandedPath)
	if err != nil {
		return fileMetadata{}, err
	}
	if !info.Mode().IsRegular() {
		return fileMetadata{}, fmt.Errorf("--file must point to a regular file")
	}
	file, err := os.Open(expandedPath)
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

func sortedFieldNames(fields map[string]any) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func targetTableID(source Source, meta Metadata) string {
	if strings.TrimSpace(source.TableID) != "" {
		return strings.TrimSpace(source.TableID)
	}
	return firstTableID(meta)
}

func targetViewID(source Source, meta Metadata) string {
	if strings.TrimSpace(source.ViewID) != "" {
		return strings.TrimSpace(source.ViewID)
	}
	return firstViewID(meta)
}

func targetViewType(source Source, meta Metadata) int {
	viewID := targetViewID(source, meta)
	for _, view := range meta.Views {
		if view.ID == viewID {
			return view.Type
		}
	}
	return 0
}

func encodeGzipBase64JSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes()), nil
}

func randomRecordID() (string, error) {
	suffix, err := randomAlphaNumeric(13)
	if err != nil {
		return "", err
	}
	return "rec" + suffix, nil
}

func randomAlphaNumeric(length int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	limit := big.NewInt(int64(len(alphabet)))
	for index := range result {
		value, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		result[index] = alphabet[value.Int64()]
	}
	return string(result), nil
}

func randomDecimalID(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("random decimal id length must be positive")
	}
	result := make([]byte, length)
	first, err := rand.Int(rand.Reader, big.NewInt(9))
	if err != nil {
		return "", err
	}
	result[0] = byte('1' + first.Int64())
	for index := 1; index < length; index++ {
		value, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		result[index] = byte('0' + value.Int64())
	}
	return string(result), nil
}

func randomUUID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		fallback := strconv.FormatInt(time.Now().UnixNano(), 16)
		return fallback + "-0000-4000-8000-000000000000"
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(data)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func randomSmallInt() int {
	value, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return time.Now().Nanosecond()%900000 + 1
	}
	return int(value.Int64()) + 1
}

func bitableRCEContext() map[string]any {
	return map[string]any{
		"os":          "mac",
		"app_version": "1.0.19.5980",
		"os_version":  "10.15.7",
		"platform":    "web",
		"request_id":  randomUUID(),
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
