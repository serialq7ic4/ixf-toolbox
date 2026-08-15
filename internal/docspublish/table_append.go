package docspublish

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type TableAppendRowConfig struct {
	URL          string
	InputPath    string
	CookiesPath  string
	SpaceAPI     string
	TableIndex   int
	RequiredText []string
	DryRun       bool
	Apply        bool
}

type tableAppendInput struct {
	Fields map[string]any `json:"fields"`
}

type tableAppendCellValue struct {
	Text      string
	ImagePath string
	IsImage   bool
}

type nativeTableInfo struct {
	ID        string
	Version   int
	Index     int
	RowIDs    []string
	ColumnIDs []string
	Headers   []string
	HeaderMap map[string]int
	CellSet   map[string]any
}

type tableAppendBuild struct {
	RowID         string
	CellIDs       []string
	Entries       []blockEntry
	TextCount     int
	ImageCount    int
	RequiredTexts []string
}

func AppendTableRow(config TableAppendRowConfig) (map[string]any, error) {
	input, err := readTableAppendInput(config.InputPath)
	if err != nil {
		return nil, err
	}
	loaded, err := loadPatchState(config.URL, config.CookiesPath, config.SpaceAPI)
	if err != nil {
		return nil, err
	}
	tables, err := nativeTablesFromState(loaded)
	if err != nil {
		return nil, err
	}
	table, err := selectNativeTable(tables, config.TableIndex)
	if err != nil {
		return nil, err
	}
	values, err := tableAppendValues(table, input)
	if err != nil {
		return nil, err
	}
	build := buildTableAppendRow(table, values, newBlockFactory(loaded.memberID))
	payload := tableAppendPayload(loaded, table, tables, input, build, !config.Apply)
	if len(config.RequiredText) > 0 {
		payload["requiredTextChecks"] = len(config.RequiredText)
	}
	if !config.Apply {
		return payload, nil
	}
	if _, err := prepareGeneratedImagePlaceholders(build.Entries); err != nil {
		return nil, err
	}
	beforeImageCount := countGraphBlocksByKind(loaded.graph, "image")
	changeMap := buildTableAppendChangeMap(table, build)
	if err := loaded.session.writeBlocks(loaded.target.Token, loaded.memberID, changeMap, loaded.target.Referer); err != nil {
		return nil, err
	}
	attachedImageCount, err := loaded.session.attachGeneratedImages(loaded.target.Token, loaded.memberID, loaded.target.Referer, build.Entries)
	if err != nil {
		return nil, err
	}
	requiredText := config.RequiredText
	if len(requiredText) == 0 {
		requiredText = build.RequiredTexts
	}
	verify, err := loaded.session.verify(loaded.target.Token, loaded.target.Referer, requiredText, beforeImageCount+build.ImageCount)
	if err != nil {
		return nil, err
	}
	payload["ok"] = asBool(verify["ok"])
	payload["dryRun"] = false
	payload["willWrite"] = true
	payload["appendedRowCount"] = 1
	payload["attachedImageCount"] = attachedImageCount
	payload["verify"] = verify
	return payload, nil
}

func readTableAppendInput(path string) (tableAppendInput, error) {
	if strings.TrimSpace(path) == "" {
		return tableAppendInput{}, fmt.Errorf("docs table append-row requires --input")
	}
	content, err := os.ReadFile(expandUser(path))
	if err != nil {
		return tableAppendInput{}, err
	}
	var input tableAppendInput
	if err := json.Unmarshal(content, &input); err != nil {
		return tableAppendInput{}, fmt.Errorf("docs table append-row input must be JSON")
	}
	if len(input.Fields) == 0 {
		return tableAppendInput{}, fmt.Errorf("docs table append-row input requires non-empty fields")
	}
	return input, nil
}

func nativeTablesFromState(loaded patchState) ([]nativeTableInfo, error) {
	blockMap := asMap(loaded.state["block_map"])
	tables := []nativeTableInfo{}
	seen := map[string]bool{}
	var collect func(string)
	collect = func(blockID string) {
		if blockID == "" || seen[blockID] {
			return
		}
		seen[blockID] = true
		block, ok := loaded.graph.Blocks[blockID]
		if !ok {
			return
		}
		if block.Kind == "table" {
			table := nativeTableFromBlock(block.ID, block.Version, block.Raw, blockMap, len(tables)+1)
			if len(table.RowIDs) > 0 && len(table.ColumnIDs) > 0 {
				tables = append(tables, table)
			}
		}
		for _, childID := range block.Children {
			collect(childID)
		}
	}
	for _, childID := range loaded.graph.RootChildren {
		collect(childID)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("target document contains no native docx table blocks")
	}
	for _, table := range tables {
		if len(table.Headers) != len(table.ColumnIDs) {
			return nil, fmt.Errorf("native table %d header extraction failed", table.Index)
		}
	}
	return tables, nil
}

func nativeTableFromBlock(tableID string, version int, data map[string]any, blockMap map[string]any, index int) nativeTableInfo {
	rowIDs := stringSlice(asSlice(data["rows_id"]))
	columnIDs := stringSlice(asSlice(data["columns_id"]))
	cellSet := asMap(data["cell_set"])
	headers := make([]string, 0, len(columnIDs))
	headerMap := map[string]int{}
	if len(rowIDs) > 0 {
		headerRowID := rowIDs[0]
		for columnIndex, columnID := range columnIDs {
			cell := asMap(cellSet[headerRowID+columnID])
			header := strings.TrimSpace(blockSubtreeText(blockMap, asString(cell["block_id"]), map[string]bool{}))
			headers = append(headers, header)
			if header != "" {
				headerMap[header] = columnIndex
			}
		}
	}
	return nativeTableInfo{
		ID:        tableID,
		Version:   version,
		Index:     index,
		RowIDs:    rowIDs,
		ColumnIDs: columnIDs,
		Headers:   headers,
		HeaderMap: headerMap,
		CellSet:   cellSet,
	}
}

func selectNativeTable(tables []nativeTableInfo, tableIndex int) (nativeTableInfo, error) {
	validate := func(table nativeTableInfo) (nativeTableInfo, error) {
		if duplicates := duplicateHeaderNames(table.Headers); len(duplicates) > 0 {
			return nativeTableInfo{}, fmt.Errorf("native table %d has duplicate headers: %s", table.Index, strings.Join(duplicates, ", "))
		}
		return table, nil
	}
	if tableIndex > 0 {
		if tableIndex > len(tables) {
			return nativeTableInfo{}, fmt.Errorf("--table-index %d out of range; document has %d native tables", tableIndex, len(tables))
		}
		return validate(tables[tableIndex-1])
	}
	if len(tables) != 1 {
		return nativeTableInfo{}, fmt.Errorf("document has %d native tables; pass --table-index", len(tables))
	}
	return validate(tables[0])
}

func duplicateHeaderNames(headers []string) []string {
	seen := map[string]bool{}
	duplicates := []string{}
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if seen[header] {
			duplicates = append(duplicates, header)
			continue
		}
		seen[header] = true
	}
	return duplicates
}

func tableAppendValues(table nativeTableInfo, input tableAppendInput) ([]tableAppendCellValue, error) {
	values := make([]tableAppendCellValue, len(table.ColumnIDs))
	for field, rawValue := range input.Fields {
		columnIndex, ok := table.HeaderMap[field]
		if !ok {
			return nil, fmt.Errorf("input field %q does not match any table header", field)
		}
		value, err := parseTableAppendCellValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("input field %q: %w", field, err)
		}
		values[columnIndex] = value
	}
	return values, nil
}

func parseTableAppendCellValue(value any) (tableAppendCellValue, error) {
	switch typed := value.(type) {
	case string:
		return tableAppendCellValue{Text: typed}, nil
	case float64:
		return tableAppendCellValue{Text: strconv.FormatFloat(typed, 'f', -1, 64)}, nil
	case bool:
		return tableAppendCellValue{Text: strconv.FormatBool(typed)}, nil
	case nil:
		return tableAppendCellValue{}, nil
	case map[string]any:
		filePath := strings.TrimSpace(asString(typed["file"]))
		if filePath == "" {
			return tableAppendCellValue{}, fmt.Errorf("object cell values currently require a non-empty file property")
		}
		return tableAppendCellValue{ImagePath: filePath, IsImage: true}, nil
	default:
		return tableAppendCellValue{}, fmt.Errorf("unsupported value type %T", value)
	}
}

func buildTableAppendRow(table nativeTableInfo, values []tableAppendCellValue, factory *blockFactory) tableAppendBuild {
	rowID := "row" + randomUUID()
	build := tableAppendBuild{
		RowID:   rowID,
		CellIDs: make([]string, 0, len(table.ColumnIDs)),
		Entries: []blockEntry{},
	}
	imageOrdinal := 0
	for columnIndex := range table.ColumnIDs {
		cellID := factory.blockID()
		build.CellIDs = append(build.CellIDs, cellID)
		value := tableAppendCellValue{}
		if columnIndex < len(values) {
			value = values[columnIndex]
		}
		childID := factory.blockID()
		if value.IsImage {
			imageOrdinal++
			build.Entries = append(build.Entries,
				blockEntry{ID: cellID, Data: factory.tableCellBlock(table.ID, childID)},
				blockEntry{
					ID:   childID,
					Data: factory.imageBlock(cellID),
					Image: &imageSource{
						Kind:    "file",
						Path:    value.ImagePath,
						Ordinal: imageOrdinal,
					},
				},
			)
			build.ImageCount++
			continue
		}
		build.Entries = append(build.Entries,
			blockEntry{ID: cellID, Data: factory.tableCellBlock(table.ID, childID)},
			blockEntry{ID: childID, Data: factory.baseBlock("text", cellID, value.Text)},
		)
		if strings.TrimSpace(value.Text) != "" {
			build.TextCount++
			build.RequiredTexts = append(build.RequiredTexts, value.Text)
		}
	}
	return build
}

func buildTableAppendChangeMap(table nativeTableInfo, build tableAppendBuild) map[string]any {
	ops := []map[string]any{
		{
			"p":      []any{"rows_id", len(table.RowIDs)},
			"action": map[string]any{"li": build.RowID},
		},
	}
	for columnIndex, columnID := range table.ColumnIDs {
		cellID := build.CellIDs[columnIndex]
		ops = append(ops, map[string]any{
			"p": []any{"cell_set", build.RowID + columnID},
			"action": map[string]any{
				"oi": map[string]any{
					"block_id": cellID,
					"merge_info": map[string]any{
						"row_span": 1,
						"col_span": 1,
					},
				},
			},
		})
	}
	changeMap := map[string]any{
		table.ID: map[string]any{
			"id":      table.ID,
			"version": table.Version,
			"payload": map[string]any{
				"ops": ops,
			},
		},
	}
	addNewBlockEntries(changeMap, build.Entries)
	return changeMap
}

func tableAppendPayload(loaded patchState, table nativeTableInfo, tables []nativeTableInfo, input tableAppendInput, build tableAppendBuild, dryRun bool) map[string]any {
	requiredTextChecks := len(build.RequiredTexts)
	payload := map[string]any{
		"ok":                  true,
		"dryRun":              dryRun,
		"operation":           "docs_table_append_row",
		"mode":                "native_table_append_row",
		"destructive":         false,
		"willWrite":           !dryRun,
		"targetKind":          loaded.target.Kind,
		"tableCount":          len(tables),
		"tableIndex":          table.Index,
		"currentRowCount":     len(table.RowIDs),
		"columnCount":         len(table.ColumnIDs),
		"headers":             append([]string(nil), table.Headers...),
		"fieldCount":          len(input.Fields),
		"plannedCellCount":    len(build.CellIDs),
		"plannedBlockEntries": len(build.Entries),
		"plannedTextCount":    build.TextCount,
		"plannedImageCount":   build.ImageCount,
		"requiredTextChecks":  requiredTextChecks,
	}
	return payload
}

func stringSlice(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text := asString(value); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func countGraphBlocksByKind(graph interface{ SafeSummary() map[string]any }, kind string) int {
	summary := graph.SafeSummary()
	counts := asIntMap(summary["counts"])
	return counts[kind]
}

func asIntMap(value any) map[string]int {
	if typed, ok := value.(map[string]int); ok {
		return typed
	}
	result := map[string]int{}
	if typed, ok := value.(map[string]any); ok {
		for key, raw := range typed {
			result[key] = asInt(raw)
		}
	}
	return result
}
