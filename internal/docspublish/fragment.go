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
			buffer := []string{}
			index++
			for index < len(lines) && !strings.HasPrefix(lines[index], "```") {
				buffer = append(buffer, lines[index])
				index++
			}
			if index < len(lines) {
				index++
			}
			specs = append(specs, Spec{Kind: "code", Text: strings.Join(buffer, "\n")})
			continue
		}
		if isTableStart(lines, index) {
			text, rows, nextIndex := parseMarkdownTable(lines, index)
			index = nextIndex
			if text == "" {
				continue
			}
			specs = append(specs, Spec{Kind: "table", Text: text, Rows: rows})
			continue
		}
		if match := headingPattern.FindStringSubmatch(line); len(match) == 3 {
			specs = append(specs, Spec{Kind: fmt.Sprintf("heading%d", len(match[1])), Text: cleanInline(match[2])})
			index++
			continue
		}
		if strings.HasPrefix(line, "- ") {
			specs = append(specs, Spec{Kind: "bullet", Text: cleanInline(strings.TrimPrefix(line, "- "))})
			index++
			continue
		}
		if orderedPattern.MatchString(line) {
			specs = append(specs, Spec{Kind: "ordered", Text: cleanInline(orderedPattern.ReplaceAllString(line, ""))})
			index++
			continue
		}
		paragraph := []string{strings.TrimSpace(line)}
		index++
		for index < len(lines) {
			next := lines[index]
			if strings.TrimSpace(next) == "" || strings.HasPrefix(next, "```") || strings.HasPrefix(next, "#") ||
				strings.HasPrefix(next, "- ") || orderedPattern.MatchString(next) || strings.HasPrefix(next, "|") {
				break
			}
			paragraph = append(paragraph, strings.TrimSpace(next))
			index++
		}
		text := cleanInline(strings.Join(paragraph, " "))
		if text == "" {
			continue
		}
		switch {
		case strings.HasPrefix(text, "案例类型："):
			specs = append(specs, Spec{Kind: "callout", Text: text})
		case text == "完整因果链可以收敛为：":
			specs = append(specs, Spec{Kind: "callout", Text: text})
		case strings.HasPrefix(text, "换句话说") || strings.HasPrefix(text, "本质上") || strings.HasPrefix(text, "所以"):
			specs = append(specs, Spec{Kind: "quote", Text: text})
		default:
			specs = append(specs, Spec{Kind: "text", Text: text})
		}
	}
	return specs, nil
}
