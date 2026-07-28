package docxgraph

import (
	"fmt"
	"sort"
	"strings"
)

const summarySnippetLimit = 80

type headingStackItem struct {
	level int
	text  string
}

func (g Graph) SafeSummary() map[string]any {
	counts := map[string]int{}
	for _, block := range g.Blocks {
		counts[block.Kind]++
	}

	headings := g.headingSummaries()
	complexTypes, complexCount := g.complexTypesInIDs(g.RootChildren)

	return map[string]any{
		"root": map[string]any{
			"id":         redactBlockID(g.RootID),
			"kind":       g.Blocks[g.RootID].Kind,
			"version":    g.RootVersion,
			"childCount": len(g.RootChildren),
		},
		"topLevelBlocks":    len(g.RootChildren),
		"counts":            counts,
		"headingCount":      len(headings),
		"headings":          headings,
		"complexBlockCount": complexCount,
		"complexBlockTypes": complexTypes,
		"duplicateHeadings": g.duplicateHeadings(),
	}
}

func (g Graph) headingSummaries() []map[string]any {
	stack := []headingStackItem{}
	summaries := []map[string]any{}
	for index, blockID := range g.RootChildren {
		block := g.Blocks[blockID]
		level := headingLevel(block.Kind)
		if level == 0 {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, headingStackItem{level: level, text: block.Text})
		anchor := HeadingRef{ID: block.ID, Text: block.Text, Level: level, Index: index}
		section := g.SectionRange(anchor)
		sectionComplexTypes, sectionComplexCount := g.complexTypesInIDs(section.IDs)
		summaries = append(summaries, map[string]any{
			"id":                redactBlockID(block.ID),
			"kind":              block.Kind,
			"text":              safeSnippet(block.Text),
			"path":              headingPath(stack),
			"level":             level,
			"rootIndex":         index,
			"sectionStart":      section.Start,
			"sectionEnd":        section.End,
			"sectionBlockCount": len(section.IDs),
			"complexBlockCount": sectionComplexCount,
			"complexBlockTypes": sectionComplexTypes,
			"previousSibling":   g.rootSiblingSummary(index - 1),
			"nextSibling":       g.rootSiblingSummary(index + 1),
		})
	}
	return summaries
}

func (g Graph) duplicateHeadings() []string {
	counts := map[string]int{}
	display := map[string]string{}
	for _, blockID := range g.RootChildren {
		block := g.Blocks[blockID]
		if headingLevel(block.Kind) == 0 {
			continue
		}
		normalized := NormalizeHeading(block.Text)
		if normalized == "" {
			continue
		}
		counts[normalized]++
		display[normalized] = normalized
	}
	duplicates := []string{}
	for heading, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, display[heading])
		}
	}
	sort.Strings(duplicates)
	return duplicates
}

func (g Graph) rootSiblingSummary(index int) map[string]any {
	if index < 0 || index >= len(g.RootChildren) {
		return nil
	}
	block := g.Blocks[g.RootChildren[index]]
	return map[string]any{
		"id":        redactBlockID(block.ID),
		"kind":      block.Kind,
		"text":      safeSnippet(block.Text),
		"rootIndex": index,
	}
}

func (g Graph) complexTypesInIDs(blockIDs []string) ([]string, int) {
	seen := map[string]bool{}
	types := map[string]bool{}
	count := 0
	for _, blockID := range blockIDs {
		count += g.collectComplexTypes(blockID, seen, types)
	}
	return sortedStringKeys(types), count
}

func (g Graph) collectComplexTypes(blockID string, seen map[string]bool, types map[string]bool) int {
	if blockID == "" || seen[blockID] {
		return 0
	}
	seen[blockID] = true
	block := g.Blocks[blockID]
	count := 0
	if !isSupportedStructureBlockType(block.Kind) {
		count++
		types[block.Kind] = true
	}
	for _, childID := range block.Children {
		count += g.collectComplexTypes(childID, seen, types)
	}
	return count
}

func headingPath(stack []headingStackItem) string {
	parts := make([]string, 0, len(stack))
	for _, item := range stack {
		if text := NormalizeHeading(item.text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " > ")
}

func redactBlockID(id string) string {
	if id == "" {
		return ""
	}
	hash := hashString(id)
	return fmt.Sprintf("id_%s_len%d", hash[:12], len(id))
}

func safeSnippet(text string) string {
	normalized := NormalizeHeading(text)
	runes := []rune(normalized)
	if len(runes) <= summarySnippetLimit {
		return normalized
	}
	return string(runes[:summarySnippetLimit]) + "..."
}

func sortedStringKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isSupportedStructureBlockType(blockType string) bool {
	switch blockType {
	case "page", "text", "bullet", "ordered", "code", "quote_container", "callout", "table", "table_cell":
		return true
	}
	return strings.HasPrefix(blockType, "heading")
}
