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
	"strconv"
	"strings"
)

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
	requestURL := origin + "/space/api/v3/sheet/client_vars?synced_block_host_token=" +
		url.QueryEscape(hostToken) + "&synced_block_host_type=22"
	body, err := json.Marshal(map[string]any{
		"memberId":      0,
		"schemaVersion": 9,
		"openType":      1,
		"token":         workbookToken,
		"sheetRange": map[string]any{
			"sheetId": sheetID,
		},
		"clientVersion": "v0.0.1",
	})
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
