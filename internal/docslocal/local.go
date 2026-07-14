package docslocal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var remoteTokenPattern = regexp.MustCompile(`/([^/?#]+)`)

func InspectSource(source string) (map[string]any, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return inspectRemoteSource(source)
	}
	return inspectLocalSource(source)
}

func CleanupOutputs(outDir string) error {
	root := expandUser(outDir)
	manifestPath := filepath.Join(root, "manifest.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest not found: %s", manifestPath)
	}
	var manifest map[string]struct {
		File   string `json:"file"`
		Assets []struct {
			Path string `json:"path"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return fmt.Errorf("invalid manifest: %s", manifestPath)
	}
	for _, item := range manifest {
		if item.File != "" {
			if err := removeInside(root, item.File); err != nil {
				return err
			}
		}
		for _, asset := range item.Assets {
			if asset.Path == "" {
				continue
			}
			if err := removeInside(root, filepath.Join(root, asset.Path)); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func inspectLocalSource(source string) (map[string]any, error) {
	path := expandUser(source)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local file not found: %s", path)
		}
		return nil, err
	}
	readable := false
	if file, err := os.Open(path); err == nil {
		readable = true
		_ = file.Close()
	}
	return map[string]any{
		"ok":        true,
		"source":    source,
		"remote":    false,
		"kind":      "local_markdown",
		"path":      path,
		"exists":    true,
		"readable":  readable,
		"sizeBytes": info.Size(),
		"suffix":    filepath.Ext(path),
	}, nil
}

func inspectRemoteSource(source string) (map[string]any, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	pathType := "remote"
	kind := "remote"
	route := "remote_read"
	token := ""
	ownerID := ""

	if strings.Contains(parsed.Path, "/okr/user/") {
		pathType = "okr"
		kind = "okr"
		route = "okr_detail"
		ownerID = tokenAfter(parsed.Path, "/okr/user/")
		query := parsed.Query()
		token = query.Get("okrId")
		if token == "" {
			token = query.Get("okr_id")
		}
	} else {
		for _, candidate := range []string{"docx", "wiki", "mindnotes"} {
			if value := tokenAfter(parsed.Path, "/"+candidate+"/"); value != "" {
				pathType = candidate
				token = value
				break
			}
		}
		switch pathType {
		case "docx":
			kind = "docx"
			route = "docx_client_vars"
		case "wiki":
			kind = "wiki"
			route = "wiki_resolve_then_read"
		case "mindnotes":
			kind = "mindnote"
			route = "mindnote_client_vars"
		}
	}

	return map[string]any{
		"ok":          true,
		"sourceRef":   redactedRemoteSource(parsed.Path, parsed.Host, parsed.RawQuery, []string{ownerID, token}),
		"remote":      true,
		"kind":        kind,
		"host":        parsed.Host,
		"pathType":    pathType,
		"tokenPrefix": tokenPrefix(token),
		"tokenLength": len(token),
		"route":       route,
	}, nil
}

func tokenAfter(path string, marker string) string {
	index := strings.Index(path, marker)
	if index == -1 {
		return ""
	}
	rest := path[index+len(marker):]
	match := remoteTokenPattern.FindStringSubmatch("/" + rest)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func redactedRemoteSource(path string, host string, rawQuery string, tokens []string) string {
	redactedPath := path
	redactedQuery := rawQuery
	for _, token := range tokens {
		if token == "" {
			continue
		}
		redactedPath = strings.ReplaceAll(redactedPath, token, "<redacted>")
		redactedQuery = strings.ReplaceAll(redactedQuery, token, "<redacted>")
	}
	suffix := ""
	if redactedQuery != "" {
		suffix = "?" + redactedQuery
	}
	return "https://" + host + redactedPath + suffix
}

func tokenPrefix(token string) string {
	if len(token) <= 3 {
		return token
	}
	return token[:3]
}

func removeInside(root string, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to remove path outside output directory: %s", target)
	}
	if err := os.Remove(absTarget); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func expandUser(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
