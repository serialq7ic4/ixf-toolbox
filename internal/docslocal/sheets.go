package docslocal

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// sheetTokenPrefix marks a native sheet workbook token. A wiki node carries it as its
// obj_token when the node is backed by a workbook rather than by a docx or a base.
const sheetTokenPrefix = "sht"

const wikiObjectAnchor = "current_space_wiki = Object("

// wikiObjectToken returns obj_token from the populated current_space_wiki assignment.
//
// The anchor occurs many times on a wiki page, and the first occurrence is empty:
// the page emits `current_space_wiki = Object();` and only later, guarded by a test on
// that empty value, the populated `Object({"obj_token":...})`. Reading just the first
// match therefore finds no token at all, so every occurrence is tried and the first one
// that parses with an obj_token wins.
func wikiObjectToken(html string) string {
	offset := 0
	for {
		index := strings.Index(html[offset:], wikiObjectAnchor)
		if index == -1 {
			return ""
		}
		offset += index
		payload, err := extractBalancedObject(html[offset:], wikiObjectAnchor)
		offset += len(wikiObjectAnchor)
		if err != nil {
			continue
		}
		info := map[string]any{}
		if err := json.Unmarshal([]byte(payload), &info); err != nil {
			continue
		}
		if token := strings.TrimSpace(stringValue(info["obj_token"])); token != "" {
			return token
		}
	}
}

// wikiSheetWorkbookToken returns the workbook token when this wiki node is backed by a
// native sheet, and "" when it is not.
//
// Only the structured current_space_wiki object is consulted, never the looser
// `obj_token":"..."` scan that the docx path falls back to. That scan matches the first
// such key anywhere in the page, including tokens of other nodes the page happens to
// mention, and reading a different workbook than the one addressed would be a silently
// wrong document. A page that omits the structured object keeps failing as it does now.
func wikiSheetWorkbookToken(html string) string {
	if token := wikiObjectToken(html); strings.HasPrefix(token, sheetTokenPrefix) {
		return token
	}
	return ""
}

// workbookSheet identifies one sheet inside a workbook.
type workbookSheet struct {
	id    string
	name  string
	index int
}

// workbookInfo is what the sheet-id probe reports about a workbook as a whole.
type workbookInfo struct {
	title string
	// count is the workbook's own sheetCount. It stays 0 when the response does not
	// report one, which has to remain distinguishable from a genuine single sheet so the
	// caller can say it does not know instead of claiming a count.
	count  int
	sheets []workbookSheet
	// active is the sheet the server selects when asked for no particular one.
	active string
}

// first is the sheet a read covers by default.
//
// It is the lowest-indexed sheet, not the server's active one: the active sheet follows
// the workbook's startSheetIndex, so calling it "the first sheet" would be wrong for a
// workbook that opens on a later tab. The active id is the fallback for a response that
// lists no sheets at all.
func (info workbookInfo) first() workbookSheet {
	if len(info.sheets) > 0 {
		return info.sheets[0]
	}
	return workbookSheet{id: info.active, name: info.active}
}

// sheetCount is how many sheets the workbook holds, or 0 when that is unknown.
func (info workbookInfo) sheetCount() int {
	if info.count > len(info.sheets) {
		return info.count
	}
	return len(info.sheets)
}

func (session *remoteReadSession) expandEmbeddedSheet(origin string, hostPageToken string, sheetBlockToken string) []string {
	lines, err := session.renderEmbeddedSheetAsTSV(origin, hostPageToken, sheetBlockToken)
	if err != nil {
		return []string{"[sheet-error]", ""}
	}
	return lines
}

func (session *remoteReadSession) renderEmbeddedSheetAsTSV(origin string, hostPageToken string, sheetBlockToken string) ([]string, error) {
	workbookToken, sheetID, err := splitSheetBlockToken(sheetBlockToken)
	if err != nil {
		return nil, err
	}
	data, err := session.fetchEmbeddedSheet(origin, hostPageToken, workbookToken, sheetID)
	if err != nil {
		return nil, err
	}
	return renderSheetDataAsTSV(data, workbookToken, sheetID)
}

// fetchWorkbookInfo asks for a workbook without naming a sheet.
//
// Sending no sheetRange is what makes this a discovery call: the server answers with the
// sheet it would open, and reports that sheet's id at
// data.formerlySchema.clientvars.extra_data.sheet_id. The id is not derivable from the
// workbook token, and a wiki node exposes only the token, so this is the one step that
// turns a wiki node into something the sheet reader can address.
func (session *remoteReadSession) fetchWorkbookInfo(origin string, workbookToken string, referer string) (workbookInfo, error) {
	data, err := session.fetchSheetClientVars(origin, workbookToken, workbookToken, "", referer)
	if err != nil {
		return workbookInfo{}, err
	}
	return parseWorkbookInfo(data), nil
}

func parseWorkbookInfo(data map[string]any) workbookInfo {
	formerlySchema := asMap(data["formerlySchema"])
	clientvars := asMap(formerlySchema["clientvars"])
	info := workbookInfo{
		title:  strings.TrimSpace(stringValue(formerlySchema["title"])),
		active: strings.TrimSpace(stringValue(asMap(clientvars["extra_data"])["sheet_id"])),
	}
	snapshot, err := decodeGzipJSON(stringValue(clientvars["gzip_snapshot"]))
	if err != nil {
		// A snapshot that will not decode costs the sheet list, not the read: the active
		// id above is enough to address one sheet. sheetCount then stays 0 and the caller
		// reports the count as unavailable.
		return info
	}
	info.count = intValue(snapshot["sheetCount"])
	for key, value := range asMap(snapshot["sheets"]) {
		sheet := asMap(value)
		// The live server keys this map by sheet name and carries the id in an "id"
		// field; other shapes key it by id. Falling back to the map key covers both
		// without having to know which one answered.
		info.sheets = append(info.sheets, workbookSheet{
			id:    firstNonEmptyString(sheet["id"], key),
			name:  firstNonEmptyString(sheet["name"], sheet["title"], key),
			index: intValue(sheet["index"]),
		})
	}
	// Map iteration is unordered, so index alone would leave ties resolved at random.
	sort.Slice(info.sheets, func(left int, right int) bool {
		if info.sheets[left].index != info.sheets[right].index {
			return info.sheets[left].index < info.sheets[right].index
		}
		return info.sheets[left].id < info.sheets[right].id
	})
	return info
}

// readWikiSheet reads a wiki node whose obj_token is a workbook token.
//
// Only the first sheet is read. Reading the rest is an opt-in that does not exist yet,
// so a workbook holding more than one sheet says so in its header line and in a warning:
// a caller given 1 of 5 sheets with no indication is the failure worth preventing here.
func (session *remoteReadSession) readWikiSheet(source string, origin string, workbookToken string) (Result, error) {
	info, err := session.fetchWorkbookInfo(origin, workbookToken, source)
	if err != nil {
		return Result{}, err
	}
	sheet := info.first()
	if sheet.id == "" {
		return Result{}, fmt.Errorf("unable to resolve a sheet id for workbook %s", workbookToken)
	}
	data, err := session.fetchSheetClientVars(origin, workbookToken, workbookToken, sheet.id, source)
	if err != nil {
		return Result{}, err
	}
	lines, err := renderSheetDataAsTSV(data, workbookToken, sheet.id)
	if err != nil {
		return Result{}, err
	}

	title := info.title
	if title == "" {
		title = workbookToken
	}
	count := info.sheetCount()
	warnings := []string{}
	countField := strconv.Itoa(count)
	if count == 0 {
		countField = "unknown"
		warnings = append(warnings, "workbook sheet count unavailable; read the first sheet only")
	} else if count > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"workbook holds %d sheets; read the first sheet only (%s)", count, sheet.name))
	}
	header := fmt.Sprintf("[sheet-workbook token=%s sheets=%s sheet_id=%s sheet_name=%q]",
		workbookToken, countField, sheet.id, sheet.name)
	counts := map[string]int{"sheet": 1}
	if count > 0 {
		counts["sheet_total"] = count
	}
	// Joined verbatim rather than through normalizeLines, for the reason readBitableBase
	// records: that normalizer right-trims each line, and a trailing tab in a TSV body is
	// a column rather than whitespace.
	contentLines := append([]string{"# " + title, "", header}, lines...)
	return Result{
		Source:   source,
		Kind:     "wiki_sheet",
		Title:    title,
		Token:    workbookToken + "_" + sheet.id,
		Content:  strings.Join(contentLines, "\n"),
		Counts:   counts,
		Assets:   []map[string]any{},
		Warnings: warnings,
	}, nil
}

func (session *remoteReadSession) readDirectSheet(source string, origin string, parsed *url.URL) (Result, error) {
	workbookToken := tokenAfter(parsed.Path, "/sheets/")
	sheetID := parsed.Query().Get("sheet")
	if workbookToken == "" {
		return Result{}, fmt.Errorf("direct sheet URL missing workbook token")
	}
	if sheetID == "" {
		return Result{}, fmt.Errorf("direct sheet URL missing sheet query parameter")
	}
	data, err := session.fetchSheetClientVars(origin, workbookToken, workbookToken, sheetID, source)
	if err != nil {
		return Result{}, err
	}
	lines, err := renderSheetDataAsTSV(data, workbookToken, sheetID)
	if err != nil {
		return Result{}, err
	}
	title := workbookToken + " " + sheetID
	return Result{
		Source:   source,
		Kind:     "sheet",
		Title:    title,
		Token:    workbookToken + "_" + sheetID,
		Content:  "# " + title + "\n\n" + strings.Join(lines, "\n"),
		Counts:   map[string]int{"sheet": 1},
		Assets:   []map[string]any{},
		Warnings: []string{},
	}, nil
}

func renderSheetDataAsTSV(data map[string]any, workbookToken string, sheetID string) ([]string, error) {
	clientvars := asMap(asMap(data["formerlySchema"])["clientvars"])
	snapshot, err := decodeGzipJSON(stringValue(clientvars["gzip_snapshot"]))
	if err != nil {
		return nil, err
	}
	sheetMeta := asMap(asMap(asMap(snapshot["sheets"])[sheetID]))
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
		for offset, rowValue := range asSlice(datatable["rows"]) {
			row := asMap(rowValue)
			values := []string{}
			for _, column := range asSlice(row["columns"]) {
				values = append(values, cellToText(column))
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

	lines := []string{
		fmt.Sprintf("[sheet-meta workbook_token=%s sheet_id=%s rows=%d cols=%d]", workbookToken, sheetID, rowCount, columnCount),
		"```tsv",
	}
	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		values := append([]string{}, rowMap[rowIndex]...)
		for len(values) < columnCount {
			values = append(values, "")
		}
		for index, value := range values {
			values[index] = escapeTSVCell(value)
		}
		lines = append(lines, strings.Join(values, "\t"))
	}
	lines = append(lines, "```", "")
	return lines, nil
}

func (session *remoteReadSession) fetchEmbeddedSheet(origin string, hostPageToken string, workbookToken string, sheetID string) (map[string]any, error) {
	return session.fetchSheetClientVars(origin, hostPageToken, workbookToken, sheetID, origin+"/docx/"+hostPageToken)
}

func (session *remoteReadSession) fetchSheetClientVars(origin string, hostToken string, workbookToken string, sheetID string, referer string) (map[string]any, error) {
	// The URL goes to the resolved API host so --space-api is honoured; Origin and
	// Referer below stay tied to the document, matching internal/bitable.
	requestURL := session.apiHostFor(origin) + "/space/api/v3/sheet/client_vars?synced_block_host_token=" +
		url.QueryEscape(hostToken) + "&synced_block_host_type=22"
	requestBody := map[string]any{
		"memberId":      0,
		"schemaVersion": 9,
		"openType":      1,
		"token":         workbookToken,
		"clientVersion": "v0.0.1",
	}
	// An empty sheetID means "whichever sheet you would open", and the server answers
	// that only when sheetRange is absent altogether.
	if sheetID != "" {
		requestBody["sheetRange"] = map[string]any{"sheetId": sheetID}
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-CSRFToken", session.csrfToken)
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range session.cookies {
		request.AddCookie(&cookie)
	}
	response, err := session.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("sheet client_vars http status %d", response.StatusCode)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, err
	}
	if codeNumber(payload["code"]) != 0 {
		return nil, fmt.Errorf("sheet client_vars failed")
	}
	return asMap(payload["data"]), nil
}

func splitSheetBlockToken(sheetBlockToken string) (string, string, error) {
	index := strings.LastIndex(sheetBlockToken, "_")
	if index <= 0 || index == len(sheetBlockToken)-1 {
		return "", "", fmt.Errorf("invalid embedded sheet token")
	}
	return sheetBlockToken[:index], sheetBlockToken[index+1:], nil
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
		return normalizeSheetText(renderSheetValue(cell))
	}
	for _, key := range []string{"value", "formattedValue", "displayValue"} {
		if value, ok := cellMap[key]; ok {
			return normalizeSheetText(renderSheetValue(value))
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
	case int:
		return strconv.Itoa(typed)
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

func normalizeSheetText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func escapeTSVCell(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\t", "\\t")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func asSlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return []any{}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}
