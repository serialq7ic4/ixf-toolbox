package sheets

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/serialq7ic4/ixf-toolbox/internal/docslocal"
)

type ReadConfig struct {
	Source      string
	CookiesPath string
	SpaceAPI    string
}

type UpdateConfig struct {
	URL       string
	Range     string
	InputPath string
	DryRun    bool
	Apply     bool
}

type Target struct {
	RawURL        string
	BaseURL       string
	WorkbookToken string
	SheetID       string
}

var rangeStartPattern = regexp.MustCompile(`^[A-Z]+[1-9][0-9]*$`)

func Read(config ReadConfig) (string, error) {
	results, err := docslocal.ReadSourcesWithOptions([]string{config.Source}, docslocal.ReadOptions{
		CookiesPath: config.CookiesPath,
		SpaceAPI:    config.SpaceAPI,
	})
	if err != nil {
		return "", err
	}
	if len(results) != 1 {
		return "", fmt.Errorf("sheets read expected one result")
	}
	result := results[0]
	if result.Kind != "sheet" {
		return "", fmt.Errorf("sheets read requires a direct sheets URL")
	}
	return result.Content, nil
}

func PlanUpdate(config UpdateConfig) (map[string]any, error) {
	if config.Apply {
		return nil, fmt.Errorf("sheets update --apply is not available until the sheet write API contract is captured")
	}
	if !config.DryRun {
		return nil, fmt.Errorf("sheets update requires --dry-run")
	}
	target, err := ParseTarget(config.URL)
	if err != nil {
		return nil, err
	}
	rangeStart, err := NormalizeRangeStart(config.Range)
	if err != nil {
		return nil, err
	}
	rows, cols, err := TSVShape(config.InputPath)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":          true,
		"dryRun":      true,
		"operation":   "update_sheet",
		"willWrite":   false,
		"targetToken": target.WorkbookToken,
		"sheetId":     target.SheetID,
		"range":       rangeStart,
		"rows":        rows,
		"cols":        cols,
		"input":       config.InputPath,
	}, nil
}

func ParseTarget(rawURL string) (Target, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Target{}, fmt.Errorf("--url must be an absolute HTTP(S) sheets URL")
	}
	token := tokenAfterPath(parsed.Path, "/sheets/")
	if token == "" {
		return Target{}, fmt.Errorf("sheets update requires a direct sheets URL")
	}
	sheetID := strings.TrimSpace(parsed.Query().Get("sheet"))
	if sheetID == "" {
		return Target{}, fmt.Errorf("sheets URL missing sheet query parameter")
	}
	return Target{
		RawURL:        strings.TrimSpace(rawURL),
		BaseURL:       parsed.Scheme + "://" + parsed.Host,
		WorkbookToken: token,
		SheetID:       sheetID,
	}, nil
}

func NormalizeRangeStart(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return "", fmt.Errorf("--range is required")
	}
	if !rangeStartPattern.MatchString(value) {
		return "", fmt.Errorf("--range must be an A1-style start cell such as B2")
	}
	return value, nil
}

func TSVShape(path string) (int, int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return 0, 0, fmt.Errorf("--input TSV is empty")
	}
	rows := strings.Split(text, "\n")
	maxCols := 0
	for _, row := range rows {
		cols := len(strings.Split(row, "\t"))
		if cols > maxCols {
			maxCols = cols
		}
	}
	return len(rows), maxCols, nil
}

func tokenAfterPath(path string, marker string) string {
	index := strings.Index(path, marker)
	if index < 0 {
		return ""
	}
	rest := strings.Trim(path[index+len(marker):], "/")
	if rest == "" {
		return ""
	}
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return rest
}
