package docspublish

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Config struct {
	MarkdownPath string
	BaseURL      string
	Title        string
	TitleSuffix  string
	RequiredText []string
	Apply        bool
}

type Spec struct {
	Kind string
	Text string
}

func PublishMarkdown(config Config) (map[string]any, error) {
	content, err := os.ReadFile(expandUser(config.MarkdownPath))
	if err != nil {
		return nil, err
	}
	sourceTitle, specs, err := ParseMarkdown(string(content))
	if err != nil {
		return nil, err
	}
	if _, err := validateBaseURL(config.BaseURL); err != nil {
		return nil, err
	}
	title := config.Title
	if title == "" {
		title = sourceTitle + config.TitleSuffix
	}
	if config.Apply {
		return nil, fmt.Errorf("go docs publish apply is not implemented yet")
	}
	return map[string]any{
		"ok":     true,
		"dryRun": true,
		"title":  title,
		"counts": summarizeSpecs(specs),
	}, nil
}

func ParseMarkdown(markdown string) (string, []Spec, error) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		return "", nil, fmt.Errorf("Markdown must start with a level-1 title (`# title`).")
	}
	title := strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
	specs := []Spec{}
	for index := 1; index < len(lines); {
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
			index += 2
			for index < len(lines) && strings.HasPrefix(lines[index], "|") {
				index++
			}
			specs = append(specs, Spec{Kind: "callout"})
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
	return title, specs, nil
}

var (
	headingPattern        = regexp.MustCompile(`^(#{1,9})\s+(.*)$`)
	orderedPattern        = regexp.MustCompile(`^\d+\.\s+`)
	inlineCodePattern     = regexp.MustCompile("`([^`]+)`")
	tableSeparatorPattern = regexp.MustCompile(`^\|\s*-+`)
)

func isTableStart(lines []string, index int) bool {
	return index+1 < len(lines) && strings.HasPrefix(lines[index], "|") && tableSeparatorPattern.MatchString(lines[index+1])
}

func cleanInline(text string) string {
	return strings.TrimSpace(inlineCodePattern.ReplaceAllString(text, "$1"))
}

func summarizeSpecs(specs []Spec) map[string]int {
	counts := map[string]int{}
	for _, spec := range specs {
		counts[spec.Kind]++
	}
	return counts
}

func validateBaseURL(baseURL string) (string, error) {
	normalized := strings.TrimRight(baseURL, "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("--base-url must be an absolute HTTP(S) URL")
	}
	return normalized, nil
}

func expandUser(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
