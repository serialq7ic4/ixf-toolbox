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
	// A wiki node addresses the base, not a table, so no table is requested and the
	// server describes its first one.
	return session.readBitableBase(source, origin, baseToken, "", "wiki_bitable")
}

func (session *remoteReadSession) readDirectBitable(source string, origin string, parsed *url.URL) (Result, error) {
	baseToken := tokenAfter(parsed.Path, "/base/")
	if baseToken == "" {
		return Result{}, fmt.Errorf("direct bitable URL missing base token")
	}
	return session.readBitableBase(source, origin, baseToken, requestedBitableTableID(parsed.Query()), "bitable")
}

// requestedBitableTableID reads the table a /base/ URL names, if any.
//
// The keys are the ones internal/bitable's ParseSource accepts, so a URL means the
// same table to both commands. The browser puts ?table= in the address bar when a
// tab other than the first is open.
func requestedBitableTableID(query url.Values) string {
	for _, key := range []string{"table", "table_id", "tableId"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func (session *remoteReadSession) readBitableBase(source string, origin string, baseToken string, tableID string, kind string) (Result, error) {
	clientVarsData, err := session.bitableClientVars(origin, baseToken, tableID, source)
	if err != nil {
		return Result{}, err
	}
	title, lines, counts, err := renderBitableAsTSV(clientVarsData, baseToken, tableID)
	if err != nil {
		return Result{}, err
	}
	// The lines are joined verbatim rather than passed through normalizeLines.
	//
	// That function is a Markdown normalizer: it right-trims each line and collapses
	// blank runs. Both are wrong for a TSV body. A tab is a delimiter here, since
	// escapeTSVCell escapes any tab inside a cell, so trimming a line's trailing
	// tabs deletes columns. Worse, a record whose every cell is empty renders as a
	// line of tabs alone, which trims to "" and is then treated as a blank line:
	// consecutive such records collapse into one and trailing ones are dropped, while
	// the meta line above still reports the pre-normalization row count. The document
	// would contradict its own rows= field.
	//
	// Nothing is lost by skipping it. Every Markdown line this function emits is
	// trim-safe already: the title is TrimSpace'd by renderBitableAsTSV and the rest
	// are literals or a fmt.Sprintf of them, so none can end in the two spaces that
	// would make a Markdown hard break. readDirectSheet joins its lines the same way.
	contentLines := []string{"# " + title, "", "[bitable token=" + baseToken + "]"}
	contentLines = append(contentLines, lines...)
	return Result{
		Source:   source,
		Kind:     kind,
		Title:    title,
		Token:    baseToken,
		Content:  strings.Join(contentLines, "\n"),
		Counts:   counts,
		Assets:   []map[string]any{},
		Warnings: []string{},
	}, nil
}

func (session *remoteReadSession) bitableClientVars(origin string, baseToken string, tableID string, referer string) (map[string]any, error) {
	// The URL goes to the resolved API host so --space-api is honoured; Origin and
	// Referer stay tied to the document, matching internal/bitable.
	//
	// tableID scopes the response to that table, exactly as in internal/bitable, and
	// viewID stays empty for the same reason it does there: view filtering is applied
	// client-side, so the server answers identically for every viewID.
	requestURL := session.apiHostFor(origin) + "/space/api/v1/bitable/" + url.PathEscape(baseToken) + "/clientvars" +
		"?tableID=" + url.QueryEscape(strings.TrimSpace(tableID)) +
		"&viewID=&recordLimit=2000&ondemandLimit=200" +
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

// renderBitableAsTSV renders the table the response describes as a TSV block.
//
// requestedTableID is the table the caller asked for, empty when none was. The
// response describes one table only -- its views, fields, and records all belong
// to it -- and names it at data.table.meta.id. That id, not the first entry of
// base.tables, is what the output must report, because the request may have
// scoped the response to any table in the base.
func renderBitableAsTSV(clientVarsData map[string]any, baseToken string, requestedTableID string) (string, []string, map[string]int, error) {
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
	// meta.id names the described table. The positional fallback covers a payload
	// without it; reading tableIDs[0] unconditionally was the defect, since it
	// labelled every scoped read as the base's first table.
	tableID := firstNonEmptyString(asMap(tableData["meta"])["id"], tableIDs[0])
	if err := checkDescribedBitableTable(requestedTableID, tableID, tableIDs); err != nil {
		return "", nil, nil, err
	}
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

// checkDescribedBitableTable refuses a read that would return a table other than
// the one requested.
//
// Reading the base's default table under a ?table= that named another one is a
// silent wrong answer: the caller gets plausible rows and no sign they belong to a
// different table. It is refused with the same distinction internal/bitable draws,
// between a table the base does not have and one the server did not honour.
func checkDescribedBitableTable(requested string, described string, baseTables []string) error {
	requested = strings.TrimSpace(requested)
	if requested == "" || requested == described {
		return nil
	}
	for _, table := range baseTables {
		if table == requested {
			return fmt.Errorf("bitable table %q exists in this base but the server described %q instead; "+
				"this is unexpected and worth reporting", requested, described)
		}
	}
	return fmt.Errorf("bitable table %q is not one of this base's tables; "+
		"correct the table parameter in the URL, or remove it to read %q", requested, described)
}
