package docslocal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func isBitableWikiHTML(html string) bool {
	return strings.Contains(html, "wiki_suite_type") && strings.Contains(html, "bitable")
}

func (session *remoteReadSession) readBitableWiki(source string, origin string, html string) (Result, error) {
	wikiInfoJSON, err := extractBalancedObject(html, "current_space_wiki = Object(")
	if err != nil {
		return Result{}, err
	}
	wikiInfo := map[string]any{}
	if err := json.Unmarshal([]byte(wikiInfoJSON), &wikiInfo); err != nil {
		return Result{}, err
	}
	baseToken := strings.TrimSpace(stringValue(wikiInfo["obj_token"]))
	if baseToken == "" {
		return Result{}, fmt.Errorf("unable to locate bitable base token from wiki HTML")
	}
	clientVarsData, err := session.bitableClientVars(origin, baseToken, source)
	if err != nil {
		return Result{}, err
	}
	title, lines, counts, err := renderBitableAsTSV(clientVarsData, baseToken)
	if err != nil {
		return Result{}, err
	}
	contentLines := []string{"# " + title, "", "[bitable token=" + baseToken + "]"}
	contentLines = append(contentLines, lines...)
	return Result{
		Source:   source,
		Kind:     "wiki_bitable",
		Title:    title,
		Token:    baseToken,
		Content:  strings.Join(normalizeLines(contentLines), "\n") + "\n",
		Counts:   counts,
		Assets:   []map[string]any{},
		Warnings: []string{},
	}, nil
}

func (session *remoteReadSession) bitableClientVars(origin string, baseToken string, referer string) (map[string]any, error) {
	requestURL := origin + "/space/api/v1/bitable/" + url.PathEscape(baseToken) + "/clientvars" +
		"?tableID=&viewID=&recordLimit=2000&ondemandLimit=200" +
		"&needBase=true&viewLazyLoad=true&ondemandVer=2" +
		"&openType=0&noMissCS=true&optimizationFlag=1"
	payload, err := session.getJSON(requestURL, origin, referer)
	if err != nil {
		return nil, err
	}
	if codeNumber(payload["code"]) != 0 {
		return nil, fmt.Errorf("bitable clientvars failed")
	}
	return asMap(payload["data"]), nil
}

func renderBitableAsTSV(clientVarsData map[string]any, baseToken string) (string, []string, map[string]int, error) {
	oldSchema, err := decodeGzipJSON(stringValue(asMap(clientVarsData["oldSchema"])["gzipSchema"]))
	if err != nil {
		return "", nil, nil, err
	}
	base := asMap(oldSchema["base"])
	tableData := asMap(asMap(oldSchema["data"])["table"])
	recordMap := asMap(asMap(oldSchema["data"])["recordMap"])
	tableIDs := stringsFromSlice(asSlice(base["tables"]))
	if len(tableIDs) == 0 {
		return "", nil, nil, fmt.Errorf("unable to locate bitable table")
	}
	tableID := tableIDs[0]
	tableInfo := asMap(asMap(base["tableInfos"])[tableID])
	tableName := strings.TrimSpace(stringValue(tableInfo["name"]))
	if tableName == "" {
		tableName = tableID
	}
	viewIDs := stringsFromSlice(asSlice(tableData["views"]))
	selectedView := selectBitableView(viewIDs, asMap(tableData["viewMap"]))
	if len(selectedView) == 0 {
		return "", nil, nil, fmt.Errorf("unable to locate a renderable bitable view")
	}
	viewID := strings.TrimSpace(stringValue(selectedView["id"]))
	viewName := strings.TrimSpace(stringValue(selectedView["name"]))
	if viewName == "" {
		viewName = viewID
	}
	property := asMap(selectedView["property"])
	fieldIDs := stringsFromSlice(asSlice(property["fields"]))
	recordIDs := stringsFromSlice(asSlice(property["records"]))
	fieldMap := asMap(tableData["fieldMap"])
	userMap := bitableUserMap(clientVarsData, oldSchema)
	tzName := strings.TrimSpace(stringValue(base["timezone"]))
	if tzName == "" {
		tzName = strings.TrimSpace(stringValue(clientVarsData["timeZone"]))
	}

	headers := make([]string, 0, len(fieldIDs))
	for _, fieldID := range fieldIDs {
		field := asMap(fieldMap[fieldID])
		header := strings.TrimSpace(stringValue(field["name"]))
		if header == "" {
			header = fieldID
		}
		headers = append(headers, escapeTSVCell(header))
	}
	lines := []string{
		fmt.Sprintf(
			"[bitable-meta base_token=%s table_id=%s table_name=\"%s\" view_id=%s view_name=\"%s\" rows=%d cols=%d views=%d]",
			baseToken,
			tableID,
			tableName,
			viewID,
			viewName,
			len(recordIDs),
			len(fieldIDs),
			len(viewIDs),
		),
		"```tsv",
		strings.Join(headers, "\t"),
	}
	for _, recordID := range recordIDs {
		record := asMap(recordMap[recordID])
		row := make([]string, 0, len(fieldIDs))
		for _, fieldID := range fieldIDs {
			field := asMap(fieldMap[fieldID])
			cell := record[fieldID]
			if cellMap := asMap(cell); len(cellMap) > 0 {
				cell = cellMap["value"]
			}
			rendered := renderBitableValue(cell, field, userMap, tzName)
			row = append(row, escapeTSVCell(normalizeSheetText(rendered)))
		}
		lines = append(lines, strings.Join(row, "\t"))
	}
	lines = append(lines, "```", "")
	title := strings.TrimSpace(stringValue(base["name"]))
	if title == "" {
		title = baseToken
	}
	counts := map[string]int{
		"bitable":         1,
		"bitable_views":   len(viewIDs),
		"bitable_fields":  len(fieldIDs),
		"bitable_records": len(recordIDs),
	}
	return title, lines, counts, nil
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

func renderBitableValue(value any, field map[string]any, userMap map[string]string, tzName string) string {
	switch intValue(field["type"]) {
	case 3:
		return renderBitableOptionValue(value, bitableFieldOptionMap(field))
	case 5:
		return formatBitableDatetime(value, field, tzName)
	case 11:
		return renderBitableUserValue(value, userMap)
	default:
		return renderSheetValue(value)
	}
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
			name := firstNonEmptyString(user["name"], user["enName"], user["displayName"])
			if name != "" {
				userMap[userID] = name
			}
		}
	}
	addUsers(asMap(clientVarsData["users"]))
	for userID, userValue := range asMap(asMap(clientVarsData["oldSchema"])["users"]) {
		if _, exists := userMap[userID]; exists {
			continue
		}
		user := asMap(userValue)
		if name := firstNonEmptyString(user["name"], user["enName"], user["displayName"]); name != "" {
			userMap[userID] = name
		}
	}
	for userID, userValue := range asMap(oldSchema["users"]) {
		if _, exists := userMap[userID]; exists {
			continue
		}
		user := asMap(userValue)
		if name := firstNonEmptyString(user["name"], user["enName"], user["displayName"]); name != "" {
			userMap[userID] = name
		}
	}
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

func stringsFromSlice(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, stringValue(value))
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
