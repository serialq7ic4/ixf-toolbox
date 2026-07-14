package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ixftoolbox "github.com/serialq7ic4/ixf-toolbox"
)

const (
	version        = "1.2.0"
	defaultCookies = "/tmp/ixunfei_profile_explorer_cookies.json"
)

var skillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
}

type runtimeTarget struct {
	Key       string
	SkillsDir string
	SourceDir string
}

type skillResult struct {
	Runtime string `json:"runtime"`
	Skill   string `json:"skill"`
	Path    string `json:"path"`
	Reason  string `json:"reason,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printRootHelp(stderr)
		return 2
	}
	if args[0] == "--version" || args[0] == "-version" {
		fmt.Fprintf(stdout, "ixf %s\n", version)
		return 0
	}

	switch args[0] {
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "setup":
		return runSetup(args[1:], stdout, stderr)
	case "cookies":
		return runCookies(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported command: %s\n", args[0])
		printRootHelp(stderr)
		return 2
	}
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: ixf [--version] {doctor,setup,cookies} ...")
}

func runSetup(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "skills" {
		fmt.Fprintln(stderr, "ERROR setup requires subcommand: skills")
		return 2
	}
	flags := flag.NewFlagSet("ixf setup skills", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runtimesRaw := flags.String("runtimes", "auto", "")
	force := flags.Bool("force", false, "")
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}

	runtimes, err := normalizeRuntimes(strings.Split(*runtimesRaw, ","))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := installSkills(runtimes, *force)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 1
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	fmt.Fprintf(stdout, "installed=%d skipped=%d\n", len(payload["installed"].([]skillResult)), len(payload["skipped"].([]skillResult)))
	return 0
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cookiesPath := flags.String("cookies", defaultCookies, "")
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	payload := collectDiagnostics(*cookiesPath)
	if *asJSON {
		writeJSON(stdout, payload)
		if ok, _ := payload["ok"].(bool); ok {
			return 0
		}
		return 1
	}
	formatDiagnostics(stdout, payload)
	if ok, _ := payload["ok"].(bool); ok {
		return 0
	}
	return 1
}

func runCookies(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "export" {
		fmt.Fprintln(stderr, "ERROR cookies requires subcommand: export")
		return 2
	}
	flags := flag.NewFlagSet("ixf cookies export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	provider := flags.String("provider", "auto", "")
	output := flags.String("output", defaultCookies, "")
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	_ = output
	payload := map[string]any{
		"ok": false,
		"error": map[string]any{
			"type":      "cookie",
			"subtype":   "cookie_export_unavailable",
			"message":   fmt.Sprintf("Go POC cookie export is not implemented yet for provider %q.", *provider),
			"hint":      "Use the Python ixf runtime for real cookie export until the Go exporter is ported.",
			"retryable": false,
		},
	}
	if *asJSON {
		writeJSON(stdout, payload)
	} else {
		fmt.Fprintln(stderr, "ERROR Go POC cookie export is not implemented yet.")
	}
	return 6
}

func installSkills(runtimes []string, force bool) (map[string]any, error) {
	installed := []skillResult{}
	skipped := []skillResult{}
	targets := detectRuntimeTargets()
	selected := map[string]bool{}
	for _, runtime := range runtimes {
		selected[runtime] = true
	}

	for _, target := range targets {
		if !selected[target.Key] {
			continue
		}
		for _, skillName := range skillNames {
			source := filepath.ToSlash(filepath.Join(target.SourceDir, skillName, "SKILL.md"))
			content, err := fs.ReadFile(ixftoolbox.SkillFS, source)
			if err != nil {
				return nil, err
			}
			destination := filepath.Join(target.SkillsDir, skillName)
			skillPath := filepath.Join(destination, "SKILL.md")
			if pathExists(destination) && !force {
				skipped = append(skipped, skillResult{Runtime: target.Key, Skill: skillName, Path: destination, Reason: "exists"})
				continue
			}
			if force {
				if err := os.RemoveAll(destination); err != nil {
					return nil, err
				}
			}
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(skillPath, content, 0o644); err != nil {
				return nil, err
			}
			installed = append(installed, skillResult{Runtime: target.Key, Skill: skillName, Path: destination})
		}
	}
	return map[string]any{"ok": true, "installed": installed, "skipped": skipped}, nil
}

func detectRuntimeTargets() []runtimeTarget {
	home := homeDir()
	codexDir := getenvDefault("IXF_TOOLBOX_CODEX_SKILLS_DIR", filepath.Join(home, ".codex", "skills"))
	claudeDir := getenvDefault("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, ".claude", "skills"))
	return []runtimeTarget{
		{Key: "codex", SkillsDir: codexDir, SourceDir: filepath.FromSlash("skills/codex")},
		{Key: "claude-code", SkillsDir: claudeDir, SourceDir: filepath.FromSlash("skills/claude-code")},
	}
}

func normalizeRuntimes(raw []string) ([]string, error) {
	values := []string{}
	for _, value := range raw {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 || contains(values, "auto") || contains(values, "all") {
		return []string{"codex", "claude-code"}, nil
	}
	if contains(values, "none") {
		return []string{}, nil
	}
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		normalized := value
		if value == "claude" || value == "claude_code" {
			normalized = "claude-code"
		}
		if normalized != "codex" && normalized != "claude-code" {
			return nil, fmt.Errorf("unsupported runtime: %s", value)
		}
		if !seen[normalized] {
			result = append(result, normalized)
			seen[normalized] = true
		}
	}
	return result, nil
}

func collectDiagnostics(cookiesPath string) map[string]any {
	skills := skillsStatus()
	cookies := cookieDiagnostics(cookiesPath)
	skillsOK := false
	for _, raw := range skills {
		if status, ok := raw.(map[string]any); ok {
			if value, _ := status["ok"].(bool); value {
				skillsOK = true
			}
		}
	}
	cookiesOK, _ := cookies["ok"].(bool)
	return map[string]any{
		"ok":      skillsOK && cookiesOK,
		"version": version,
		"runtime": "go-poc",
		"capabilities": map[string]bool{
			"docsRead":      true,
			"docsPublish":   true,
			"okrRead":       true,
			"okrWrite":      true,
			"cookiesExport": false,
		},
		"skills":  skills,
		"cookies": cookies,
	}
}

func skillsStatus() map[string]any {
	result := map[string]any{}
	for _, target := range detectRuntimeTargets() {
		installed := map[string]bool{}
		ok := true
		for _, skillName := range skillNames {
			exists := pathExists(filepath.Join(target.SkillsDir, skillName, "SKILL.md"))
			installed[skillName] = exists
			if !exists {
				ok = false
			}
		}
		result[target.Key] = map[string]any{"ok": ok, "dir": target.SkillsDir, "installed": installed}
	}
	return result
}

func cookieDiagnostics(cookiePath string) map[string]any {
	path := expandUser(cookiePath)
	payload := map[string]any{
		"ok":          false,
		"exists":      false,
		"path":        path,
		"cookieCount": 0,
		"cookieNames": []string{},
		"hasCsrf":     false,
		"hasLgwCsrf":  false,
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return payload
	}
	if err != nil {
		payload["exists"] = true
		payload["error"] = fmt.Sprintf("%T: %v", err, err)
		return payload
	}
	var cookies []map[string]any
	if err := json.Unmarshal(content, &cookies); err != nil {
		payload["exists"] = true
		payload["error"] = fmt.Sprintf("%T: %v", err, err)
		return payload
	}
	names := map[string]bool{}
	hasCsrf := false
	hasLgwCsrf := false
	for _, cookie := range cookies {
		name, _ := cookie["name"].(string)
		value, _ := cookie["value"].(string)
		if name == "" {
			continue
		}
		names[name] = true
		if name == "_csrf_token" && value != "" {
			hasCsrf = true
		}
		if name == "lgw_csrf_token" && value != "" {
			hasLgwCsrf = true
		}
	}
	nameList := make([]string, 0, len(names))
	for name := range names {
		nameList = append(nameList, name)
	}
	sort.Strings(nameList)
	payload["ok"] = true
	payload["exists"] = true
	payload["cookieCount"] = len(cookies)
	payload["cookieNames"] = nameList
	payload["hasCsrf"] = hasCsrf
	payload["hasLgwCsrf"] = hasLgwCsrf
	return payload
}

func formatDiagnostics(w io.Writer, payload map[string]any) {
	fmt.Fprintf(w, "ixf-toolbox %s\n", payload["version"])
	if ok, _ := payload["ok"].(bool); ok {
		fmt.Fprintln(w, "overall ok")
	} else {
		fmt.Fprintln(w, "overall fail")
	}
}

func writeJSON(w io.Writer, payload any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getenvDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return expandUser(value)
	}
	return fallback
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

func expandUser(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}
