package docx

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Result struct {
	Markdown string
	Counts   map[string]int
	Assets   []map[string]any
	Warnings []string
}

type Options struct {
	ResolveImage func(ImageReference) ImageResolution
	ExpandSheet  func(string) []string
}

type ImageReference struct {
	BlockID      string
	Token        string
	Name         string
	MimeType     string
	Width        int
	Height       int
	DeclaredSize int
	Caption      string
}

type ImageResolution struct {
	MarkdownPath string
	AltText      string
	Asset        map[string]any
	Warning      string
}

func ExtractText(value any) string {
	return extractText(value)
}

type block struct {
	id       string
	kind     string
	parentID string
	children []string
	text     string
	raw      map[string]any
}

type blockTree struct {
	blocks map[string]block
	order  []string
	rootID string
}

var headingLevelPattern = regexp.MustCompile(`(\d+)$`)

func ConvertClientVars(clientVars map[string]any, objToken string) Result {
	return ConvertClientVarsWithOptions(clientVars, objToken, Options{})
}

func ConvertClientVarsWithOptions(clientVars map[string]any, objToken string, options Options) Result {
	tree := buildBlockTree(clientVars, objToken)
	seen := map[string]bool{}
	assets := []map[string]any{}
	warnings := []string{}
	orderedCounters := map[string]int{}
	parts := []string{}
	if tree.rootID != "" {
		if root, ok := tree.blocks[tree.rootID]; ok && root.kind == "page" && root.text != "" {
			parts = append(parts, "# "+root.text)
		}
	}
	for _, blockID := range tree.order {
		rendered := renderBlock(tree, blockID, 0, seen, &assets, &warnings, options, orderedCounters)
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, strings.TrimRight(rendered, "\n"))
		}
	}
	markdown := strings.TrimRight(strings.Join(parts, "\n\n"), "\n")
	if markdown != "" {
		markdown += "\n"
	}
	return Result{
		Markdown: markdown,
		Counts:   countBlockTypes(tree.blocks),
		Assets:   assets,
		Warnings: warnings,
	}
}

func buildBlockTree(clientVars map[string]any, objToken string) blockTree {
	rawMap, _ := clientVars["block_map"].(map[string]any)
	blocks := map[string]block{}
	for id, entryValue := range rawMap {
		entry := asMap(entryValue)
		data := asMap(entry["data"])
		if len(data) == 0 {
			data = entry
		}
		kind := stringValue(firstNonEmpty(data["type"], entry["type"]))
		if kind == "" {
			kind = "unknown"
		}
		blocks[id] = block{
			id:       id,
			kind:     kind,
			parentID: stringValue(firstNonEmpty(data["parent_id"], entry["parent_id"])),
			children: readChildren(firstNonEmpty(data["children"], entry["children"])),
			text:     extractText(data["text"]),
			raw:      data,
		}
	}
	rootID := findRootID(blocks, objToken)
	order := []string{}
	if rootID != "" {
		if root, ok := blocks[rootID]; ok {
			for _, childID := range root.children {
				if _, ok := blocks[childID]; ok {
					order = append(order, childID)
				}
			}
		}
	}
	if len(order) == 0 {
		for id := range blocks {
			if id != rootID {
				order = append(order, id)
			}
		}
		sort.Strings(order)
	}
	return blockTree{blocks: blocks, order: order, rootID: rootID}
}

func findRootID(blocks map[string]block, objToken string) string {
	if _, ok := blocks[objToken]; ok {
		return objToken
	}
	for id, block := range blocks {
		if block.kind == "page" {
			return id
		}
	}
	for id, block := range blocks {
		if block.parentID == "" {
			return id
		}
	}
	return ""
}

func renderBlock(
	tree blockTree,
	blockID string,
	depth int,
	seen map[string]bool,
	assets *[]map[string]any,
	warnings *[]string,
	options Options,
	orderedCounters map[string]int,
) string {
	if seen[blockID] {
		return ""
	}
	seen[blockID] = true
	block, ok := tree.blocks[blockID]
	if !ok {
		return ""
	}
	switch {
	case block.kind == "page":
		return renderChildren(tree, block, depth, seen, assets, warnings, options, orderedCounters)
	case strings.HasPrefix(block.kind, "heading"):
		level := 1
		if match := headingLevelPattern.FindStringSubmatch(block.kind); len(match) == 2 {
			if parsed, err := strconv.Atoi(match[1]); err == nil {
				level = parsed
			}
		}
		if level > 6 {
			level = 6
		}
		return strings.TrimRight(strings.Repeat("#", level)+" "+block.text, " ")
	case block.kind == "text":
		return block.text
	case block.kind == "bullet":
		line := strings.TrimRight(strings.Repeat("  ", depth)+"- "+block.text, " ")
		children := renderChildren(tree, block, depth+1, seen, assets, warnings, options, orderedCounters)
		return joinNonEmpty("\n\n", line, children)
	case block.kind == "ordered":
		parentKey := block.parentID
		if parentKey == "" {
			parentKey = "__root__"
		}
		orderedCounters[parentKey]++
		return strings.TrimRight(fmt.Sprintf("%s%d. %s", strings.Repeat("  ", depth), orderedCounters[parentKey], block.text), " ")
	case block.kind == "code":
		language := normalizeCodeLanguage(block.raw["language"])
		return fmt.Sprintf("```%s\n%s\n```", language, block.text)
	case block.kind == "todo":
		marker := " "
		if readBool(block.raw["checked"]) {
			marker = "x"
		}
		return strings.TrimRight(strings.Repeat("  ", depth)+"- ["+marker+"] "+block.text, " ")
	case block.kind == "divider":
		return "---"
	case block.kind == "quote_container":
		inner := renderChildren(tree, block, depth, seen, assets, warnings, options, orderedCounters)
		if strings.TrimSpace(inner) == "" {
			return ">"
		}
		lines := strings.Split(inner, "\n")
		for index, line := range lines {
			if line == "" {
				lines[index] = ">"
				continue
			}
			lines[index] = "> " + line
		}
		return strings.Join(lines, "\n")
	case block.kind == "callout":
		children := renderChildren(tree, block, depth+1, seen, assets, warnings, options, orderedCounters)
		return joinNonEmpty("\n\n", "[callout]", strings.TrimRight(children, "\n"))
	case block.kind == "sheet":
		token := stringValue(block.raw["token"])
		marker := "[sheet]"
		if token == "" {
			return marker
		}
		marker = "[sheet token=" + token + "]"
		if options.ExpandSheet == nil {
			return marker
		}
		expanded := options.ExpandSheet(token)
		return joinNonEmpty("\n", marker, strings.TrimSpace(strings.Join(expanded, "\n")))
	case block.kind == "table":
		return renderTable(tree, block, seen, assets, warnings, options, orderedCounters)
	case block.kind == "image":
		return renderImage(block, assets, warnings, options)
	case block.kind == "table_cell" || block.kind == "whiteboard" || block.kind == "mindnote" || block.kind == "isv":
		return "[" + block.kind + "]"
	default:
		childDepth := depth
		if block.kind != "" {
			childDepth++
		}
		children := renderChildren(tree, block, childDepth, seen, assets, warnings, options, orderedCounters)
		if block.kind != "" && block.kind != "unknown" {
			appendWarning(warnings, "unsupported block type: "+block.kind)
		}
		return joinNonEmpty("\n\n", block.text, children)
	}
}

func renderChildren(
	tree blockTree,
	block block,
	depth int,
	seen map[string]bool,
	assets *[]map[string]any,
	warnings *[]string,
	options Options,
	orderedCounters map[string]int,
) string {
	parts := []string{}
	for _, childID := range block.children {
		rendered := renderBlock(tree, childID, depth, seen, assets, warnings, options, orderedCounters)
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, strings.TrimRight(rendered, "\n"))
		}
	}
	return strings.Join(parts, "\n\n")
}

func renderTable(
	tree blockTree,
	block block,
	seen map[string]bool,
	assets *[]map[string]any,
	warnings *[]string,
	options Options,
	orderedCounters map[string]int,
) string {
	rows := readChildren(block.raw["rows_id"])
	columns := readChildren(block.raw["columns_id"])
	cellSet := asMap(block.raw["cell_set"])
	if len(rows) == 0 || len(columns) == 0 || len(cellSet) == 0 {
		return "[table]"
	}
	renderedRows := make([][]string, 0, len(rows))
	for _, rowID := range rows {
		renderedRow := make([]string, 0, len(columns))
		for _, columnID := range columns {
			cellID := tableCellBlockID(cellSet, rowID, columnID)
			renderedRow = append(renderedRow, renderTableCell(tree, cellID, seen, assets, warnings, options, orderedCounters))
		}
		renderedRows = append(renderedRows, renderedRow)
	}
	if len(renderedRows) == 0 {
		return "[table]"
	}
	lines := []string{
		markdownTableRow(renderedRows[0]),
		markdownTableRow(repeatString("---", len(columns))),
	}
	for _, row := range renderedRows[1:] {
		lines = append(lines, markdownTableRow(row))
	}
	return strings.Join(lines, "\n")
}

func tableCellBlockID(cellSet map[string]any, rowID string, columnID string) string {
	for _, key := range []string{rowID + "_" + columnID, rowID + columnID, "row" + rowID + "col" + columnID} {
		cell := asMap(cellSet[key])
		if blockID := stringValue(cell["block_id"]); blockID != "" {
			return blockID
		}
	}
	for key, value := range cellSet {
		if strings.Contains(key, rowID) && strings.Contains(key, columnID) {
			cell := asMap(value)
			if blockID := stringValue(cell["block_id"]); blockID != "" {
				return blockID
			}
		}
	}
	return ""
}

func renderTableCell(
	tree blockTree,
	cellID string,
	seen map[string]bool,
	assets *[]map[string]any,
	warnings *[]string,
	options Options,
	orderedCounters map[string]int,
) string {
	cell, ok := tree.blocks[cellID]
	if !ok {
		return ""
	}
	rendered := renderChildren(tree, cell, 0, seen, assets, warnings, options, orderedCounters)
	if rendered == "" {
		rendered = cell.text
	}
	return normalizeTableCell(rendered)
}

func renderImage(block block, assets *[]map[string]any, warnings *[]string, options Options) string {
	if options.ResolveImage == nil {
		return "[image]"
	}
	image := asMap(block.raw["image"])
	reference := ImageReference{
		BlockID:      block.id,
		Token:        stringValue(image["token"]),
		Name:         stringValue(image["name"]),
		MimeType:     stringValue(image["mimeType"]),
		Width:        intValue(image["width"]),
		Height:       intValue(image["height"]),
		DeclaredSize: intValue(image["size"]),
		Caption:      extractText(image["caption"]),
	}
	resolution, ok := resolveImageSafely(options.ResolveImage, reference)
	if !ok {
		appendWarning(warnings, "image resolution failed")
		return "[image]"
	}
	if imageResolutionContainsToken(resolution, reference.Token) {
		appendWarning(warnings, "image resolution rejected unsafe output")
		return "[image]"
	}
	if resolution.Asset != nil && assets != nil {
		*assets = append(*assets, resolution.Asset)
	}
	if resolution.Warning != "" {
		appendWarning(warnings, resolution.Warning)
	}
	if resolution.MarkdownPath == "" {
		return "[image]"
	}
	return "![" + resolution.AltText + "](" + resolution.MarkdownPath + ")"
}

func resolveImageSafely(resolve func(ImageReference) ImageResolution, reference ImageReference) (resolution ImageResolution, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return resolve(reference), true
}

func imageResolutionContainsToken(resolution ImageResolution, token string) bool {
	if token == "" {
		return false
	}
	return valueContainsToken(resolution.MarkdownPath, token) ||
		valueContainsToken(resolution.AltText, token) ||
		valueContainsToken(resolution.Asset, token) ||
		valueContainsToken(resolution.Warning, token)
}

func valueContainsToken(value any, token string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, token)
	case map[string]any:
		for key, item := range typed {
			if valueContainsToken(key, token) || valueContainsToken(item, token) {
				return true
			}
		}
	case map[string]string:
		for key, item := range typed {
			if valueContainsToken(key, token) || valueContainsToken(item, token) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if valueContainsToken(item, token) {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if valueContainsToken(item, token) {
				return true
			}
		}
	}
	return false
}

func extractText(value any) string {
	switch text := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(text)
	case []any:
		parts := make([]string, 0, len(text))
		for _, item := range text {
			itemMap := asMap(item)
			if itemText, ok := itemMap["text"]; ok {
				parts = append(parts, stringValue(itemText))
				continue
			}
			parts = append(parts, stringValue(item))
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case map[string]any:
		initial := asMap(text["initialAttributedTexts"])
		pieces := asMap(initial["text"])
		return strings.TrimSpace(renderTextPieces(pieces, initial["attribs"], text["apool"]))
	default:
		return ""
	}
}

func renderTextPieces(pieces map[string]any, attribs any, apool any) string {
	keys := make([]string, 0, len(pieces))
	for key := range pieces {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return pieceSortKey(keys[i]) < pieceSortKey(keys[j])
	})
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		text := stringValue(pieces[key])
		url := pieceURL(attribs, apool, key)
		if text != "" && url != "" {
			text = "[" + escapeMarkdownLinkText(text) + "](" + url + ")"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "")
}

func pieceURL(attribs any, apool any, pieceKey string) string {
	attribMap := asMap(attribs)
	attrText := stringValue(attribMap[pieceKey])
	matches := regexp.MustCompile(`\*(\d+)`).FindAllStringSubmatch(attrText, -1)
	if len(matches) == 0 {
		return ""
	}
	numToAttrib := asMap(asMap(apool)["numToAttrib"])
	for _, match := range matches {
		if len(match) == 2 {
			if url := urlFromAttrib(numToAttrib[match[1]]); url != "" {
				return url
			}
		}
	}
	return ""
}

func urlFromAttrib(value any) string {
	switch typed := value.(type) {
	case []any:
		if len(typed) >= 2 && stringValue(typed[0]) == "url" {
			return stringValue(typed[1])
		}
		for _, item := range typed {
			if url := urlFromAttrib(item); url != "" {
				return url
			}
		}
	case map[string]any:
		for _, key := range []string{"url", "href", "link"} {
			if value := stringValue(typed[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func escapeMarkdownLinkText(text string) string {
	text = strings.ReplaceAll(text, "[", `\[`)
	return strings.ReplaceAll(text, "]", `\]`)
}

func pieceSortKey(value string) string {
	if parsed, err := strconv.Atoi(value); err == nil {
		return fmt.Sprintf("0%020d", parsed)
	}
	return "1" + value
}

func readChildren(value any) []string {
	switch raw := value.(type) {
	case []any:
		children := make([]string, 0, len(raw))
		for _, item := range raw {
			children = append(children, stringValue(item))
		}
		return children
	case []string:
		return append([]string{}, raw...)
	case map[string]any:
		children := []string{}
		for _, item := range raw {
			children = append(children, readChildren(item)...)
		}
		return children
	default:
		return []string{}
	}
}

func countBlockTypes(blocks map[string]block) map[string]int {
	counts := map[string]int{}
	for _, block := range blocks {
		counts[block.kind]++
	}
	return counts
}

func normalizeCodeLanguage(value any) string {
	language := strings.TrimSpace(stringValue(value))
	if language == "" {
		return ""
	}
	return strings.Fields(language)[0]
}

func normalizeTableCell(value string) string {
	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.ReplaceAll(strings.Join(lines, " "), "|", `\|`)
}

func markdownTableRow(values []string) string {
	return "| " + strings.Join(values, " | ") + " |"
}

func repeatString(value string, count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = value
	}
	return values
}

func joinNonEmpty(separator string, values ...string) string {
	parts := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, separator)
}

func appendWarning(warnings *[]string, warning string) {
	if warnings == nil {
		return
	}
	for _, existing := range *warnings {
		if existing == warning {
			return
		}
	}
	*warnings = append(*warnings, warning)
}

func readBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "checked", "done":
			return true
		default:
			return false
		}
	default:
		return value != nil && stringValue(value) != "" && stringValue(value) != "0"
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

func asMap(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if stringValue(value) != "" {
			return value
		}
		if value != nil {
			return value
		}
	}
	return nil
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
