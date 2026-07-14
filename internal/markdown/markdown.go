package markdown

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	headingPattern        = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	fencePattern          = regexp.MustCompile(`^\s*(` + "```" + `+|~~~+)`)
	tableSeparatorPattern = regexp.MustCompile(`^\s*\|?\s*:?-{3,}`)
	imagePattern          = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
)

type atomicBlock struct {
	kind         string
	startLine    int
	endLine      int
	text         string
	headingLevel int
	headingTitle string
}

type Chunk struct {
	Index      int      `json:"index"`
	Breadcrumb string   `json:"breadcrumb"`
	StartLine  int      `json:"startLine"`
	EndLine    int      `json:"endLine"`
	CharCount  int      `json:"charCount"`
	ImagePaths []string `json:"imagePaths"`
}

type Outline struct {
	SelectedHeadingLevel *int    `json:"selectedHeadingLevel"`
	Chunks               []Chunk `json:"chunks"`
}

func BuildOutline(markdown string, targetChars int) (Outline, error) {
	if targetChars <= 0 {
		return Outline{}, fmt.Errorf("target_chars must be positive")
	}
	blocks := parseAtomicBlocks(markdown)
	if len(blocks) == 0 {
		return Outline{}, nil
	}

	selected := selectHeadingLevel(blocks)
	ranges := initialRanges(blocks, selected)
	splitRanges := [][2]int{}
	level := 0
	if selected != nil {
		level = *selected
	}
	for _, item := range ranges {
		splitRanges = append(splitRanges, splitRange(blocks, item[0], item[1], level, targetChars)...)
	}

	chunks := []Chunk{}
	for i, item := range splitRanges {
		start := item[0]
		end := item[1]
		text := ""
		for _, block := range blocks[start:end] {
			text += block.text
		}
		chunks = append(chunks, Chunk{
			Index:      i + 1,
			Breadcrumb: breadcrumbFor(blocks, start),
			StartLine:  blocks[start].startLine,
			EndLine:    blocks[end-1].endLine,
			CharCount:  len(text),
			ImagePaths: imagePaths(text),
		})
	}
	return Outline{SelectedHeadingLevel: selected, Chunks: chunks}, nil
}

func RenderChunk(markdown string, outline Outline, index int) (string, error) {
	if index < 1 || index > len(outline.Chunks) {
		return "", fmt.Errorf("chunk index out of range: %d", index)
	}
	chunk := outline.Chunks[index-1]
	lines := splitLinesKeepEnds(markdown)
	return strings.Join(lines[chunk.StartLine-1:chunk.EndLine], ""), nil
}

func parseAtomicBlocks(markdown string) []atomicBlock {
	lines := splitLinesKeepEnds(markdown)
	blocks := []atomicBlock{}
	index := 0
	for index < len(lines) {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			index++
			continue
		}

		if match := fencePattern.FindStringSubmatch(line); match != nil {
			marker := match[1]
			end := index + 1
			for end < len(lines) {
				if strings.HasPrefix(strings.TrimLeft(lines[end], " \t"), marker[:3]) {
					end++
					break
				}
				end++
			}
			blocks = append(blocks, makeBlock("code", lines, index, end))
			index = end
			continue
		}

		if match := headingPattern.FindStringSubmatch(strings.TrimRight(line, "\r\n")); match != nil {
			blocks = append(blocks, atomicBlock{
				kind:         "heading",
				startLine:    index + 1,
				endLine:      index + 1,
				text:         line,
				headingLevel: len(match[1]),
				headingTitle: strings.TrimSpace(match[2]),
			})
			index++
			continue
		}

		if isTableStart(lines, index) {
			end := index + 2
			for end < len(lines) && strings.TrimSpace(lines[end]) != "" && strings.Contains(lines[end], "|") {
				end++
			}
			blocks = append(blocks, makeBlock("table", lines, index, end))
			index = end
			continue
		}

		if imagePattern.MatchString(line) {
			end := index + 1
			for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
				next := strings.TrimRight(lines[end], "\r\n")
				if headingPattern.MatchString(next) || fencePattern.MatchString(lines[end]) {
					break
				}
				end++
			}
			blocks = append(blocks, makeBlock("image", lines, index, end))
			index = end
			continue
		}

		end := index + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			next := strings.TrimRight(lines[end], "\r\n")
			if headingPattern.MatchString(next) || fencePattern.MatchString(lines[end]) || isTableStart(lines, end) || imagePattern.MatchString(lines[end]) {
				break
			}
			end++
		}
		blocks = append(blocks, makeBlock("text", lines, index, end))
		index = end
	}
	return blocks
}

func makeBlock(kind string, lines []string, start int, end int) atomicBlock {
	return atomicBlock{
		kind:      kind,
		startLine: start + 1,
		endLine:   end,
		text:      strings.Join(lines[start:end], ""),
	}
}

func isTableStart(lines []string, index int) bool {
	return index+1 < len(lines) &&
		strings.Contains(lines[index], "|") &&
		tableSeparatorPattern.MatchString(lines[index+1])
}

func selectHeadingLevel(blocks []atomicBlock) *int {
	levels := []int{}
	h1Count := 0
	for _, block := range blocks {
		if block.headingLevel == 0 {
			continue
		}
		levels = append(levels, block.headingLevel)
		if block.headingLevel == 1 {
			h1Count++
		}
	}
	if len(levels) == 0 {
		return nil
	}
	selected := levels[0]
	if h1Count > 1 {
		selected = 1
	} else {
		for _, level := range levels {
			if level == 2 {
				selected = 2
				break
			}
			if level < selected {
				selected = level
			}
		}
	}
	return &selected
}

func initialRanges(blocks []atomicBlock, selected *int) [][2]int {
	if selected == nil {
		return [][2]int{{0, len(blocks)}}
	}
	boundaries := []int{}
	for i, block := range blocks {
		if block.headingLevel == *selected {
			boundaries = append(boundaries, i)
		}
	}
	if len(boundaries) == 0 {
		return [][2]int{{0, len(blocks)}}
	}
	starts := []int{}
	if boundaries[0] > 0 {
		starts = append(starts, 0)
	}
	starts = append(starts, boundaries...)
	ranges := [][2]int{}
	for i, start := range starts {
		end := len(blocks)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		ranges = append(ranges, [2]int{start, end})
	}
	return ranges
}

func splitRange(blocks []atomicBlock, start int, end int, level int, targetChars int) [][2]int {
	if blockChars(blocks, start, end) <= targetChars {
		return [][2]int{{start, end}}
	}
	nextLevel := level + 1
	deeper := []int{}
	for i := start + 1; i < end; i++ {
		if blocks[i].headingLevel == nextLevel {
			deeper = append(deeper, i)
		}
	}
	if len(deeper) > 0 {
		starts := []int{}
		if deeper[0] > start {
			starts = append(starts, start)
		}
		starts = append(starts, deeper...)
		out := [][2]int{}
		for i, itemStart := range starts {
			itemEnd := end
			if i+1 < len(starts) {
				itemEnd = starts[i+1]
			}
			out = append(out, splitRange(blocks, itemStart, itemEnd, nextLevel, targetChars)...)
		}
		return out
	}
	return packAtomicBlocks(blocks, start, end, targetChars)
}

func packAtomicBlocks(blocks []atomicBlock, start int, end int, targetChars int) [][2]int {
	ranges := [][2]int{}
	currentStart := start
	currentChars := 0
	currentHasContent := false
	for i := start; i < end; i++ {
		block := blocks[i]
		blockSize := len(block.text)
		isolateLargeAtomic := currentChars > 0 &&
			!currentHasContent &&
			(block.kind == "code" || block.kind == "table" || block.kind == "image") &&
			blockSize > targetChars
		if isolateLargeAtomic || (currentChars > 0 && currentHasContent && currentChars+blockSize > targetChars) {
			ranges = append(ranges, [2]int{currentStart, i})
			currentStart = i
			currentChars = 0
			currentHasContent = false
		}
		currentChars += blockSize
		if block.kind != "heading" {
			currentHasContent = true
		}
	}
	ranges = append(ranges, [2]int{currentStart, end})
	return ranges
}

func blockChars(blocks []atomicBlock, start int, end int) int {
	total := 0
	for _, block := range blocks[start:end] {
		total += len(block.text)
	}
	return total
}

func breadcrumbFor(blocks []atomicBlock, start int) string {
	stack := map[int]string{}
	for _, block := range blocks[:start+1] {
		if block.headingLevel == 0 {
			continue
		}
		stack[block.headingLevel] = block.headingTitle
		for level := range stack {
			if level > block.headingLevel {
				delete(stack, level)
			}
		}
	}
	levels := make([]int, 0, len(stack))
	for level := range stack {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	parts := []string{}
	for _, level := range levels {
		parts = append(parts, stack[level])
	}
	return strings.Join(parts, " > ")
}

func imagePaths(text string) []string {
	matches := imagePattern.FindAllStringSubmatch(text, -1)
	paths := []string{}
	for _, match := range matches {
		paths = append(paths, match[1])
	}
	return paths
}

func splitLinesKeepEnds(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.SplitAfter(text, "\n")
}
