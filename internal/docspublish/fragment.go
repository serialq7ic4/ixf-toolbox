package docspublish

import (
	"fmt"
	"strings"
)

func ParseMarkdownFragment(markdown string) ([]Spec, error) {
	return parseMarkdownBody(markdownLines(markdown), 0)
}

func markdownLines(markdown string) []string {
	return strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
}

func parseMarkdownBody(lines []string, start int) ([]Spec, error) {
	specs := []Spec{}
	for index := start; index < len(lines); {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			index++
			continue
		}
		if _, _, ok := fenceStart(line); ok {
			fenced, nextIndex, _ := parseFence(lines, index)
			specs = append(specs, fenced)
			index = nextIndex
			continue
		}
		if isBlockquoteLine(line) {
			inline, nextIndex := parseBlockquote(lines, index)
			index = nextIndex
			if inline.Text != "" {
				specs = append(specs, specFromInline("quote", inline))
			}
			continue
		}
		if isTableStart(lines, index) {
			text, rows, rowRuns, nextIndex := parseMarkdownTable(lines, index)
			index = nextIndex
			if text == "" {
				continue
			}
			specs = append(specs, Spec{Kind: "table", Text: text, Rows: rows, RowRuns: rowRuns})
			continue
		}
		if match := headingPattern.FindStringSubmatch(line); len(match) == 3 {
			specs = append(specs, specFromInline(fmt.Sprintf("heading%d", len(match[1])), parseInline(match[2])))
			index++
			continue
		}
		if strings.HasPrefix(line, "- ") {
			specs = append(specs, specFromInline("bullet", parseInline(strings.TrimPrefix(line, "- "))))
			index++
			continue
		}
		if orderedPattern.MatchString(line) {
			ordered := specFromInline("ordered", parseInline(orderedPattern.ReplaceAllString(line, "")))
			index++
			ordered.Children, index = consumeOrderedChildren(lines, index)
			specs = append(specs, ordered)
			continue
		}
		paragraph := []string{strings.TrimSpace(line)}
		index++
		for index < len(lines) {
			next := lines[index]
			if strings.TrimSpace(next) == "" || isFenceLine(next) || strings.HasPrefix(next, "#") ||
				strings.HasPrefix(next, "- ") || orderedPattern.MatchString(next) || strings.HasPrefix(next, "|") || isBlockquoteLine(next) {
				break
			}
			paragraph = append(paragraph, strings.TrimSpace(next))
			index++
		}
		inline := parseInline(strings.Join(paragraph, " "))
		if inline.Text == "" {
			continue
		}
		switch {
		case strings.HasPrefix(inline.Text, "案例类型："):
			specs = append(specs, specFromInline("callout", inline))
		case inline.Text == "完整因果链可以收敛为：":
			specs = append(specs, specFromInline("callout", inline))
		case strings.HasPrefix(inline.Text, "换句话说") || strings.HasPrefix(inline.Text, "本质上") || strings.HasPrefix(inline.Text, "所以"):
			specs = append(specs, specFromInline("quote", inline))
		default:
			specs = append(specs, specFromInline("text", inline))
		}
	}
	return specs, nil
}

func isFenceLine(line string) bool {
	_, _, ok := fenceStart(line)
	return ok
}

func fenceStart(line string) (info string, indent string, ok bool) {
	indentLength := 0
	for indentLength < len(line) && indentLength < 3 && line[indentLength] == ' ' {
		indentLength++
	}
	rest := line[indentLength:]
	if !strings.HasPrefix(rest, "```") {
		return "", "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, "```")), line[:indentLength], true
}

func parseFence(lines []string, index int) (Spec, int, bool) {
	info, indent, ok := fenceStart(lines[index])
	if !ok {
		return Spec{}, index, false
	}
	buffer := []string{}
	index++
	for index < len(lines) {
		if isClosingFence(lines[index], indent) {
			index++
			break
		}
		buffer = append(buffer, removeFenceIndent(lines[index], indent))
		index++
	}
	text := strings.Join(buffer, "\n")
	if isMermaidFence(info, text) {
		return Spec{Kind: "image", SourceKind: "mermaid", Text: text}, index, true
	}
	return Spec{Kind: "code", Text: text}, index, true
}

func isClosingFence(line string, indent string) bool {
	_, actualIndent, ok := fenceStart(line)
	if !ok || actualIndent != indent {
		return false
	}
	rest := line[len(actualIndent):]
	return strings.TrimSpace(rest) == "```"
}

func removeFenceIndent(line string, indent string) string {
	if indent == "" {
		return line
	}
	return strings.TrimPrefix(line, indent)
}

func consumeOrderedChildren(lines []string, index int) ([]Spec, int) {
	children := []Spec{}
	for {
		for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
			index++
		}
		if index >= len(lines) {
			return children, index
		}
		child, nextIndex, ok := parseFence(lines, index)
		if !ok {
			return children, index
		}
		children = append(children, child)
		index = nextIndex
	}
}

func isBlockquoteLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), ">")
}

func parseBlockquote(lines []string, index int) (inlineText, int) {
	quoteLines := []string{}
	for index < len(lines) && isBlockquoteLine(lines[index]) {
		line := strings.TrimSpace(lines[index])
		line = strings.TrimPrefix(line, ">")
		line = strings.TrimPrefix(line, " ")
		quoteLines = append(quoteLines, line)
		index++
	}
	return parseInline(strings.Join(quoteLines, "\n")), index
}

func specFromInline(kind string, inline inlineText) Spec {
	return Spec{Kind: kind, Text: inline.Text, Runs: inline.Runs}
}
