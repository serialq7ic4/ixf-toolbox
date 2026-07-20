package sheets

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/serialq7ic4/ixf-toolbox/internal/docslocal"
)

type ReadConfig struct {
	Source      string
	CookiesPath string
	SpaceAPI    string
}

type UpdateConfig struct {
	URL         string
	HostURL     string
	Range       string
	InputPath   string
	DryRun      bool
	Apply       bool
	CookiesPath string
	SpaceAPI    string
}

type Target struct {
	RawURL        string
	BaseURL       string
	WorkbookToken string
	SheetID       string
}

type sheetSession struct {
	client    *http.Client
	cookies   []http.Cookie
	csrfToken string
	spaceAPI  string
}

type sheetState struct {
	MemberID string
	Revision int
	Values   [][]string
}

type cellUpdate struct {
	SheetID string
	Row     int
	Col     int
	Value   string
}

var rangeStartPattern = regexp.MustCompile(`^[A-Z]+[1-9][0-9]*$`)

func Read(config ReadConfig) (string, error) {
	results, err := docslocal.ReadSourcesWithOptions([]string{config.Source}, docslocal.ReadOptions{
		CookiesPath: config.CookiesPath,
		SpaceAPI:    config.SpaceAPI,
	})
	if err != nil {
		return "", err
	}
	if len(results) != 1 {
		return "", fmt.Errorf("sheets read expected one result")
	}
	result := results[0]
	if result.Kind != "sheet" {
		return "", fmt.Errorf("sheets read requires a direct sheets URL")
	}
	return result.Content, nil
}

func PlanUpdate(config UpdateConfig) (map[string]any, error) {
	if config.Apply {
		return nil, fmt.Errorf("sheets update --apply is not available until the sheet write API contract is captured")
	}
	if !config.DryRun {
		return nil, fmt.Errorf("sheets update requires --dry-run")
	}
	target, err := ParseTarget(config.URL)
	if err != nil {
		return nil, err
	}
	rangeStart, err := NormalizeRangeStart(config.Range)
	if err != nil {
		return nil, err
	}
	rows, cols, err := TSVShape(config.InputPath)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":          true,
		"dryRun":      true,
		"operation":   "update_sheet",
		"willWrite":   false,
		"targetToken": target.WorkbookToken,
		"sheetId":     target.SheetID,
		"range":       rangeStart,
		"rows":        rows,
		"cols":        cols,
		"input":       config.InputPath,
	}, nil
}

func Update(config UpdateConfig) (map[string]any, error) {
	if config.Apply && config.DryRun {
		return nil, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	if config.Apply {
		return applyUpdate(config)
	}
	return PlanUpdate(config)
}

func applyUpdate(config UpdateConfig) (map[string]any, error) {
	target, err := ParseTarget(config.URL)
	if err != nil {
		return nil, err
	}
	hostToken, referer, err := resolveHost(config.HostURL, target)
	if err != nil {
		return nil, err
	}
	rangeStart, err := NormalizeRangeStart(config.Range)
	if err != nil {
		return nil, err
	}
	startRow, startCol, err := parseA1Start(rangeStart)
	if err != nil {
		return nil, err
	}
	values, err := readTSV(config.InputPath)
	if err != nil {
		return nil, err
	}
	session, err := newSheetSession(config, target.BaseURL)
	if err != nil {
		return nil, err
	}
	state, err := session.fetchState(target, hostToken, referer)
	if err != nil {
		return nil, err
	}
	if state.MemberID == "" {
		return nil, fmt.Errorf("could not determine the authenticated sheet member identifier")
	}
	updates := make([]cellUpdate, 0, len(values)*len(values[0]))
	for rowOffset, row := range values {
		for colOffset, value := range row {
			updates = append(updates, cellUpdate{
				SheetID: target.SheetID,
				Row:     startRow + rowOffset,
				Col:     startCol + colOffset,
				Value:   value,
			})
		}
	}
	content, err := buildUserChangesContent(updates)
	if err != nil {
		return nil, err
	}
	if err := session.postUserChanges(target, hostToken, referer, state.MemberID, state.Revision, content); err != nil {
		return nil, err
	}
	verifyState, err := session.fetchState(target, hostToken, referer)
	if err != nil {
		return nil, err
	}
	if err := verifyCells(verifyState.Values, startRow, startCol, values); err != nil {
		return nil, err
	}
	rows := len(values)
	cols := 0
	for _, row := range values {
		if len(row) > cols {
			cols = len(row)
		}
	}
	return map[string]any{
		"ok":          true,
		"dryRun":      false,
		"operation":   "update_sheet",
		"willWrite":   true,
		"targetToken": target.WorkbookToken,
		"sheetId":     target.SheetID,
		"hostToken":   hostToken,
		"range":       rangeStart,
		"rows":        rows,
		"cols":        cols,
		"input":       config.InputPath,
		"verify": map[string]any{
			"ok":      true,
			"checked": rows * cols,
		},
	}, nil
}

func ParseTarget(rawURL string) (Target, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Target{}, fmt.Errorf("--url must be an absolute HTTP(S) sheets URL")
	}
	token := tokenAfterPath(parsed.Path, "/sheets/")
	if token == "" {
		return Target{}, fmt.Errorf("sheets update requires a direct sheets URL")
	}
	sheetID := strings.TrimSpace(parsed.Query().Get("sheet"))
	if sheetID == "" {
		return Target{}, fmt.Errorf("sheets URL missing sheet query parameter")
	}
	return Target{
		RawURL:        strings.TrimSpace(rawURL),
		BaseURL:       parsed.Scheme + "://" + parsed.Host,
		WorkbookToken: token,
		SheetID:       sheetID,
	}, nil
}

func NormalizeRangeStart(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return "", fmt.Errorf("--range is required")
	}
	if !rangeStartPattern.MatchString(value) {
		return "", fmt.Errorf("--range must be an A1-style start cell such as B2")
	}
	return value, nil
}

func readTSV(path string) ([][]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, fmt.Errorf("--input TSV is empty")
	}
	lines := strings.Split(text, "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows, nil
}

func TSVShape(path string) (int, int, error) {
	rows, err := readTSV(path)
	if err != nil {
		return 0, 0, err
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	return len(rows), maxCols, nil
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

func resolveHost(hostURL string, target Target) (string, string, error) {
	if strings.TrimSpace(hostURL) == "" {
		return target.WorkbookToken, target.RawURL, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(hostURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", fmt.Errorf("--host-url must be an absolute HTTP(S) document or sheets URL")
	}
	if parsed.Scheme+"://"+parsed.Host != target.BaseURL {
		return "", "", fmt.Errorf("--host-url must use the same origin as --url")
	}
	for _, marker := range []string{"/docx/", "/wiki/", "/sheets/"} {
		if token := tokenAfterPath(parsed.Path, marker); token != "" {
			return token, strings.TrimSpace(hostURL), nil
		}
	}
	return "", "", fmt.Errorf("--host-url must contain a supported document or sheets token")
}

func parseA1Start(cell string) (int, int, error) {
	cell = strings.ToUpper(strings.TrimSpace(cell))
	index := 0
	col := 0
	for index < len(cell) && cell[index] >= 'A' && cell[index] <= 'Z' {
		col = col*26 + int(cell[index]-'A'+1)
		index++
	}
	if index == 0 || index >= len(cell) {
		return 0, 0, fmt.Errorf("--range must be an A1-style start cell such as B2")
	}
	row, err := strconv.Atoi(cell[index:])
	if err != nil || row < 1 {
		return 0, 0, fmt.Errorf("--range must be an A1-style start cell such as B2")
	}
	return row - 1, col - 1, nil
}

func newSheetSession(config UpdateConfig, defaultSpaceAPI string) (*sheetSession, error) {
	cookiesPath := strings.TrimSpace(config.CookiesPath)
	if cookiesPath == "" {
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
	spaceAPI := strings.TrimRight(strings.TrimSpace(config.SpaceAPI), "/")
	if spaceAPI == "" {
		spaceAPI = strings.TrimRight(defaultSpaceAPI, "/")
	}
	return &sheetSession{
		client:    &http.Client{Timeout: 30 * time.Second},
		cookies:   cookies,
		csrfToken: csrf,
		spaceAPI:  spaceAPI,
	}, nil
}

type cookieObject struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

func loadCookieObjects(path string) ([]cookieObject, error) {
	content, err := os.ReadFile(expandUser(path))
	if err != nil {
		return nil, err
	}
	var cookies []cookieObject
	if err := json.Unmarshal(content, &cookies); err != nil {
		return nil, fmt.Errorf("could not parse cookie jar JSON")
	}
	return cookies, nil
}

func csrfFromCookieObjects(cookies []cookieObject) string {
	for _, cookie := range cookies {
		if cookie.Name == "_csrf_token" {
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

func (session *sheetSession) fetchState(target Target, hostToken string, referer string) (sheetState, error) {
	requestURL := session.spaceAPI + "/space/api/v3/sheet/client_vars?synced_block_host_token=" +
		url.QueryEscape(hostToken) + "&synced_block_host_type=22"
	body, err := json.Marshal(map[string]any{
		"memberId":      0,
		"schemaVersion": 9,
		"openType":      1,
		"token":         target.WorkbookToken,
		"sheetRange": map[string]any{
			"sheetId": target.SheetID,
		},
		"clientVersion": "v0.0.1",
	})
	if err != nil {
		return sheetState{}, err
	}
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return sheetState{}, err
	}
	session.addHeaders(request, target.BaseURL, referer)
	payload, err := session.doJSON(request, "sheet client_vars")
	if err != nil {
		return sheetState{}, err
	}
	if codeNumber(payload["code"]) != 0 {
		return sheetState{}, fmt.Errorf("sheet client_vars failed")
	}
	data := asMap(payload["data"])
	formerly := asMap(data["formerlySchema"])
	clientvars := asMap(formerly["clientvars"])
	values, err := extractValues(data, target.SheetID)
	if err != nil {
		return sheetState{}, err
	}
	revision := intValue(clientvars["revision"])
	if revision == 0 {
		revision = intValue(clientvars["version"])
	}
	return sheetState{
		MemberID: stringValue(formerly["member_id"]),
		Revision: revision,
		Values:   values,
	}, nil
}

func (session *sheetSession) postUserChanges(target Target, hostToken string, referer string, memberID string, baseRev int, content string) error {
	query := url.Values{
		"token":                   {target.WorkbookToken},
		"member_id":               {memberID},
		"synced_block_host_token": {hostToken},
		"synced_block_host_type":  {"22"},
	}
	body, err := json.Marshal(map[string]any{
		"base_rev":      baseRev,
		"content":       content,
		"mode":          0,
		"msg_id":        randomMessageID(),
		"msg_timestamp": time.Now().UnixMilli(),
		"retryCount":    0,
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, session.spaceAPI+"/space/api/v2/sheet/user_changes?"+query.Encode(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	session.addHeaders(request, target.BaseURL, referer)
	payload, err := session.doJSON(request, "sheet user_changes")
	if err != nil {
		return err
	}
	if codeNumber(payload["code"]) != 0 {
		return fmt.Errorf("sheet user_changes failed")
	}
	return nil
}

func (session *sheetSession) addHeaders(request *http.Request, origin string, referer string) {
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-CSRFToken", session.csrfToken)
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range session.cookies {
		request.AddCookie(&cookie)
	}
}

func (session *sheetSession) doJSON(request *http.Request, label string) (map[string]any, error) {
	response, err := session.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s http status %d", label, response.StatusCode)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", label)
	}
	return payload, nil
}

func buildUserChangesContent(updates []cellUpdate) (string, error) {
	var content bytes.Buffer
	for _, update := range updates {
		cellJSON, err := json.Marshal(map[string]any{
			"value":          update.Value,
			"reminder":       nil,
			"dataValidation": nil,
		})
		if err != nil {
			return "", err
		}
		var op bytes.Buffer
		writeVarint(&op, 1<<3|0)
		writeVarint(&op, 4)
		writeBytes(&op, 2, []byte(update.SheetID))
		var position bytes.Buffer
		writeVarint(&position, 1<<3|0)
		writeVarint(&position, uint64(update.Row))
		writeVarint(&position, 3<<3|0)
		writeVarint(&position, uint64(update.Col))
		writeBytes(&op, 4, position.Bytes())
		writeBytes(&op, 5, cellJSON)
		writeBytes(&content, 1, op.Bytes())
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(content.Bytes()); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes()), nil
}

func writeBytes(buffer *bytes.Buffer, fieldNumber int, value []byte) {
	writeVarint(buffer, uint64(fieldNumber<<3|2))
	writeVarint(buffer, uint64(len(value)))
	buffer.Write(value)
}

func writeVarint(buffer *bytes.Buffer, value uint64) {
	for value >= 0x80 {
		buffer.WriteByte(byte(value) | 0x80)
		value >>= 7
	}
	buffer.WriteByte(byte(value))
}

func extractValues(data map[string]any, sheetID string) ([][]string, error) {
	clientvars := asMap(asMap(data["formerlySchema"])["clientvars"])
	snapshot, err := decodeGzipJSON(stringValue(clientvars["gzip_snapshot"]))
	if err != nil {
		return nil, err
	}
	sheetMeta := asMap(asMap(snapshot["sheets"])[sheetID])
	rowCount := intValue(sheetMeta["rowCount"])
	columnCount := intValue(sheetMeta["columnCount"])
	rowMap := map[int][]string{}
	extraData := asMap(clientvars["extra_data"])
	for _, blockValue := range asSlice(extraData["blocks"]) {
		block := asMap(blockValue)
		datatable, err := decodeGzipJSON(stringValue(block["gzip_datatable"]))
		if err != nil {
			return nil, err
		}
		startRow := intValue(block["row"])
		startCol := intValue(block["col"])
		for offset, rowValue := range asSlice(datatable["rows"]) {
			row := asMap(rowValue)
			values := append([]string{}, rowMap[startRow+offset]...)
			for _, column := range asSlice(row["columns"]) {
				values = append(values, "")
				values[len(values)-1] = cellToText(column)
			}
			if startCol > 0 {
				padded := make([]string, startCol+len(values))
				copy(padded[startCol:], values)
				values = padded
			}
			rowMap[startRow+offset] = values
			if len(values) > columnCount {
				columnCount = len(values)
			}
		}
	}
	maxRow := -1
	for rowIndex := range rowMap {
		if rowIndex > maxRow {
			maxRow = rowIndex
		}
	}
	if rowCount <= 0 {
		rowCount = maxRow + 1
	}
	values := make([][]string, rowCount)
	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		row := append([]string{}, rowMap[rowIndex]...)
		for len(row) < columnCount {
			row = append(row, "")
		}
		values[rowIndex] = row
	}
	return values, nil
}

func verifyCells(actual [][]string, startRow int, startCol int, expected [][]string) error {
	for rowOffset, row := range expected {
		for colOffset, want := range row {
			rowIndex := startRow + rowOffset
			colIndex := startCol + colOffset
			got := ""
			if rowIndex < len(actual) && colIndex < len(actual[rowIndex]) {
				got = actual[rowIndex][colIndex]
			}
			if got != want {
				return fmt.Errorf("sheet update verification failed at %s: expected written value", a1Cell(rowIndex, colIndex))
			}
		}
	}
	return nil
}

func a1Cell(row int, col int) string {
	colNumber := col + 1
	letters := ""
	for colNumber > 0 {
		colNumber--
		letters = string(rune('A'+colNumber%26)) + letters
		colNumber /= 26
	}
	return fmt.Sprintf("%s%d", letters, row+1)
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

func cellToText(cell any) string {
	cellMap := asMap(cell)
	if len(cellMap) == 0 {
		return renderSheetValue(cell)
	}
	for _, key := range []string{"value", "formattedValue", "displayValue"} {
		if value, ok := cellMap[key]; ok {
			return renderSheetValue(value)
		}
	}
	return ""
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

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	case string:
		number, _ := strconv.Atoi(typed)
		return number
	default:
		return 0
	}
}

func codeNumber(value any) int {
	return intValue(value)
}

func randomMessageID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(bytes[:])
}
