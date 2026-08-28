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
		if strings.HasPrefix(line, "```") {
			info := strings.TrimSpace(strings.TrimPrefix(line, "```"))
			buffer := []string{}
			index++
			for index < len(lines) && !strings.HasPrefix(lines[index], "```") {
				buffer = append(buffer, lines[index])
				index++
			}
			if index < len(lines) {
				index++
			}
			text := strings.Join(buffer, "\n")
			if isMermaidFence(info, text) {
				specs = append(specs, Spec{Kind: "image", SourceKind: "mermaid", Text: text})
				continue
			}
			specs = append(specs, Spec{Kind: "code", Text: text})
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
			specs = append(specs, specFromInline("ordered", parseInline(orderedPattern.ReplaceAllString(line, ""))))
			index++
			continue
		}
		paragraph := []string{strings.TrimSpace(line)}
		index++
		for index < len(lines) {
			next := lines[index]
			if strings.TrimSpace(next) == "" || strings.HasPrefix(next, "```") || strings.HasPrefix(next, "#") ||
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
