package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	DefaultCookieJSON   = "/tmp/ixunfei_profile_explorer_cookies.json"
	DefaultMessengerURL = "https://www.xfchat.iflytek.com/messenger/"
	DefaultMacAppRoot   = "~/Library/Application Support/LarkShell-ka-kaahyz17"
)

var ignoredProfileNames = map[string]bool{
	"SingletonLock":     true,
	"SingletonSocket":   true,
	"SingletonCookie":   true,
	"Code Cache":        true,
	"Cache":             true,
	"GPUCache":          true,
	"DawnGraphiteCache": true,
	"DawnWebGPUCache":   true,
	"GraphiteDawnCache": true,
	"ShaderCache":       true,
}

type Config struct {
	GOOS        string
	Home        string
	AppSupport  string
	AppData     string
	ProfileDir  string
	BrowserPath string
	CookiesPath string
}

type ProfileConfig struct {
	GOOS       string
	Home       string
	AppSupport string
	AppData    string
	ProfileDir string
}

type ProfileDiscovery struct {
	OK     bool   `json:"ok"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
	Error  string `json:"error,omitempty"`
}

type BrowserDiscovery struct {
	OK     bool   `json:"ok"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
	Error  string `json:"error,omitempty"`
}

type CloneResult struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type OpenConfig struct {
	Config
	Target               string
	Mode                 string
	DryRun               bool
	Apply                bool
	Headless             bool
	AllowVisibleFallback bool
	KeepProfileClone     bool
	TimeoutMS            int
}

type ReadConfig struct {
	Config
	Scope                string
	DryRun               bool
	Apply                bool
	Headless             bool
	AllowVisibleFallback bool
	KeepProfileClone     bool
	TimeoutMS            int
	Limit                int
	MessagesPerChat      int
	MaxScrolls           int
	IncludeSelfMessages  bool
}

type Automator interface {
	Open(context.Context, BrowserOpenRequest) (BrowserOpenResult, error)
	Read(context.Context, BrowserReadRequest) (BrowserReadResult, error)
}

type BrowserOpenRequest struct {
	BrowserPath          string
	UserDataDir          string
	CookiePath           string
	URL                  string
	Target               string
	Mode                 string
	Headless             bool
	AllowVisibleFallback bool
	TimeoutMS            int
}

type BrowserOpenResult struct {
	OpenedTitle    string `json:"openedTitle,omitempty"`
	TargetVerified bool   `json:"targetVerified"`
	Headless       bool   `json:"headless"`
	FallbackUsed   bool   `json:"fallbackUsed,omitempty"`
}

type BrowserReadRequest struct {
	BrowserPath          string
	UserDataDir          string
	CookiePath           string
	URL                  string
	Scope                string
	Limit                int
	MessagesPerChat      int
	MaxScrolls           int
	IncludeSelfMessages  bool
	Headless             bool
	AllowVisibleFallback bool
	TimeoutMS            int
}

type BrowserReadResult struct {
	Scope                string             `json:"scope"`
	Conversations        []ConversationRead `json:"conversations"`
	SkippedConversations []ConversationSkip `json:"skippedConversations,omitempty"`
	RecentSeen           int                `json:"totalRecentConversationsSeen"`
	UnreadSeen           int                `json:"totalUnreadConversations"`
	Extracted            int                `json:"totalExtractedConversations"`
	Skipped              int                `json:"totalSkippedConversations"`
	Headless             bool               `json:"headless"`
	FallbackUsed         bool               `json:"fallbackUsed,omitempty"`
}

type ConversationRead struct {
	Title       string        `json:"title"`
	Unread      int           `json:"unread"`
	Time        string        `json:"time,omitempty"`
	Preview     string        `json:"preview,omitempty"`
	OpenedTitle string        `json:"openedChatTitle,omitempty"`
	Messages    []MessageRead `json:"messages"`
}

type ConversationSkip struct {
	Title   string `json:"title"`
	Unread  int    `json:"unread"`
	Time    string `json:"time,omitempty"`
	Preview string `json:"preview,omitempty"`
	Error   string `json:"error"`
}

type MessageRead struct {
	Side string `json:"side"`
	Text string `json:"text"`
}

func SupportedOS(goos string) bool {
	return goos == "darwin" || goos == "windows"
}

func Doctor(config Config) map[string]any {
	goos := normalizedGOOS(config.GOOS)
	profile := DiscoverProfile(ProfileConfig{
		GOOS:       goos,
		Home:       config.Home,
		AppSupport: config.AppSupport,
		AppData:    config.AppData,
		ProfileDir: config.ProfileDir,
	})
	browser := DiscoverBrowser(config)
	cookies := CookieDiagnostics(config.CookiesPath)
	supported := SupportedOS(goos)
	profileOK := profile.OK
	browserOK := browser.OK
	cookiesOK, _ := cookies["ok"].(bool)
	return map[string]any{
		"ok":      supported && profileOK && browserOK && cookiesOK,
		"runtime": "go",
		"domain":  "messenger",
		"messenger": map[string]any{
			"supportedPlatform":        supported,
			"goos":                     goos,
			"entryURL":                 DefaultMessengerURL,
			"defaultHeadless":          true,
			"visibleFallbackByDefault": false,
			"profileCloneRequired":     true,
			"realSendAvailable":        false,
		},
		"profile": profile,
		"browser": browser,
		"cookies": cookies,
	}
}

func PlanOpen(config OpenConfig) (map[string]any, error) {
	target := strings.TrimSpace(config.Target)
	if target == "" {
		return nil, fmt.Errorf("--to is required")
	}
	mode := strings.TrimSpace(config.Mode)
	if mode == "" {
		return nil, fmt.Errorf("--mode is required")
	}
	if mode != "person" && mode != "conversation" {
		return nil, fmt.Errorf("--mode must be person or conversation")
	}
	if config.DryRun && config.Apply {
		return nil, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	if !config.DryRun && !config.Apply {
		return nil, fmt.Errorf("messenger open requires --dry-run or --apply")
	}
	goos := normalizedGOOS(config.GOOS)
	profile := DiscoverProfile(ProfileConfig{
		GOOS:       goos,
		Home:       config.Home,
		AppSupport: config.AppSupport,
		AppData:    config.AppData,
		ProfileDir: config.ProfileDir,
	})
	browser := DiscoverBrowser(config.Config)
	payload := map[string]any{
		"ok":             SupportedOS(goos) && profile.OK && browser.OK,
		"action":         "open",
		"dryRun":         config.DryRun,
		"apply":          config.Apply,
		"target":         target,
		"mode":           mode,
		"willSend":       false,
		"targetVerified": false,
		"browserLaunch":  false,
		"note":           "messenger open never sends messages; --apply may mark the opened chat as read.",
		"messenger": map[string]any{
			"supportedPlatform": SupportedOS(goos),
			"goos":              goos,
			"entryURL":          DefaultMessengerURL,
		},
		"profile": profile,
		"browser": browser,
	}
	return payload, nil
}

func PlanRead(config ReadConfig) (map[string]any, error) {
	scope, err := normalizedReadScope(config.Scope)
	if err != nil {
		return nil, err
	}
	if config.DryRun && config.Apply {
		return nil, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	if !config.DryRun && !config.Apply {
		return nil, fmt.Errorf("messenger read requires --dry-run or --apply")
	}
	limit := normalizedPositive(config.Limit, 20)
	messagesPerChat := normalizedPositive(config.MessagesPerChat, 5)
	maxScrolls := normalizedPositive(config.MaxScrolls, 18)
	goos := normalizedGOOS(config.GOOS)
	profile := DiscoverProfile(ProfileConfig{
		GOOS:       goos,
		Home:       config.Home,
		AppSupport: config.AppSupport,
		AppData:    config.AppData,
		ProfileDir: config.ProfileDir,
	})
	browser := DiscoverBrowser(config.Config)
	payload := map[string]any{
		"ok":                           SupportedOS(goos) && profile.OK && browser.OK,
		"action":                       "read",
		"scope":                        scope,
		"dryRun":                       config.DryRun,
		"apply":                        config.Apply,
		"limit":                        limit,
		"messagesPerChat":              messagesPerChat,
		"maxScrolls":                   maxScrolls,
		"includeSelfMessages":          config.IncludeSelfMessages,
		"willSend":                     false,
		"browserLaunch":                false,
		"totalRecentConversationsSeen": 0,
		"totalUnreadConversations":     0,
		"totalExtractedConversations":  0,
		"totalSkippedConversations":    0,
		"conversations":                []ConversationRead{},
		"skippedConversations":         []ConversationSkip{},
		"note":                         "messenger read never sends messages; --apply may mark opened chats as read.",
		"messenger": map[string]any{
			"supportedPlatform": SupportedOS(goos),
			"goos":              goos,
			"entryURL":          DefaultMessengerURL,
		},
		"profile": profile,
		"browser": browser,
	}
	return payload, nil
}

func OpenTarget(ctx context.Context, config OpenConfig, automator Automator) (map[string]any, error) {
	payload, err := PlanOpen(config)
	if err != nil {
		return nil, err
	}
	if !config.Apply {
		return payload, nil
	}
	if automator == nil {
		return nil, fmt.Errorf("browser automator is required for --apply")
	}
	if ok, _ := payload["ok"].(bool); !ok {
		return payload, fmt.Errorf("messenger open prerequisites are not ready")
	}
	profile, ok := payload["profile"].(ProfileDiscovery)
	if !ok || !profile.OK {
		return payload, fmt.Errorf("messenger profile is not ready")
	}
	browser, ok := payload["browser"].(BrowserDiscovery)
	if !ok || !browser.OK {
		return payload, fmt.Errorf("messenger browser is not ready")
	}
	clone, err := CloneProfile(profile.Path, "")
	if err != nil {
		return nil, err
	}
	cloneRoot := filepath.Dir(clone.Path)
	defer func() {
		if !config.KeepProfileClone {
			_ = os.RemoveAll(cloneRoot)
		}
	}()
	headless := true
	if !config.Headless && config.AllowVisibleFallback {
		headless = false
	}
	timeoutMS := config.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 45_000
	}
	result, err := automator.Open(ctx, BrowserOpenRequest{
		BrowserPath:          browser.Path,
		UserDataDir:          clone.Path,
		CookiePath:           expandUser(config.CookiesPath),
		URL:                  DefaultMessengerURL,
		Target:               strings.TrimSpace(config.Target),
		Mode:                 strings.TrimSpace(config.Mode),
		Headless:             headless,
		AllowVisibleFallback: config.AllowVisibleFallback,
		TimeoutMS:            timeoutMS,
	})
	if err != nil {
		return nil, err
	}
	payload["browserLaunch"] = true
	payload["targetVerified"] = result.TargetVerified
	payload["openedTitle"] = result.OpenedTitle
	payload["headless"] = result.Headless
	payload["fallbackUsed"] = result.FallbackUsed
	payload["ok"] = result.TargetVerified
	payload["clone"] = map[string]any{
		"kept": config.KeepProfileClone,
		"path": clone.Path,
	}
	return payload, nil
}

func ReadMessages(ctx context.Context, config ReadConfig, automator Automator) (map[string]any, error) {
	payload, err := PlanRead(config)
	if err != nil {
		return nil, err
	}
	if !config.Apply {
		return payload, nil
	}
	if automator == nil {
		return nil, fmt.Errorf("browser automator is required for --apply")
	}
	if ok, _ := payload["ok"].(bool); !ok {
		return payload, fmt.Errorf("messenger read prerequisites are not ready")
	}
	profile, ok := payload["profile"].(ProfileDiscovery)
	if !ok || !profile.OK {
		return payload, fmt.Errorf("messenger profile is not ready")
	}
	browser, ok := payload["browser"].(BrowserDiscovery)
	if !ok || !browser.OK {
		return payload, fmt.Errorf("messenger browser is not ready")
	}
	clone, err := CloneProfile(profile.Path, "")
	if err != nil {
		return nil, err
	}
	cloneRoot := filepath.Dir(clone.Path)
	defer func() {
		if !config.KeepProfileClone {
			_ = os.RemoveAll(cloneRoot)
		}
	}()
	headless := true
	if !config.Headless && config.AllowVisibleFallback {
		headless = false
	}
	timeoutMS := config.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 60_000
	}
	scope, _ := payload["scope"].(string)
	result, err := automator.Read(ctx, BrowserReadRequest{
		BrowserPath:          browser.Path,
		UserDataDir:          clone.Path,
		CookiePath:           expandUser(config.CookiesPath),
		URL:                  DefaultMessengerURL,
		Scope:                scope,
		Limit:                intFromPayload(payload, "limit"),
		MessagesPerChat:      intFromPayload(payload, "messagesPerChat"),
		MaxScrolls:           intFromPayload(payload, "maxScrolls"),
		IncludeSelfMessages:  config.IncludeSelfMessages,
		Headless:             headless,
		AllowVisibleFallback: config.AllowVisibleFallback,
		TimeoutMS:            timeoutMS,
	})
	if err != nil {
		return nil, err
	}
	extracted := result.Extracted
	if extracted == 0 {
		extracted = len(result.Conversations)
	}
	skipped := result.Skipped
	if skipped == 0 {
		skipped = len(result.SkippedConversations)
	}
	payload["browserLaunch"] = true
	payload["ok"] = true
	payload["scope"] = valueOrDefault(result.Scope, scope)
	payload["conversations"] = result.Conversations
	payload["skippedConversations"] = result.SkippedConversations
	payload["totalRecentConversationsSeen"] = result.RecentSeen
	payload["totalUnreadConversations"] = result.UnreadSeen
	payload["totalExtractedConversations"] = extracted
	payload["totalSkippedConversations"] = skipped
	payload["headless"] = result.Headless
	payload["fallbackUsed"] = result.FallbackUsed
	payload["clone"] = map[string]any{
		"kept": config.KeepProfileClone,
		"path": clone.Path,
	}
	return payload, nil
}

func DiscoverProfile(config ProfileConfig) ProfileDiscovery {
	if strings.TrimSpace(config.ProfileDir) != "" {
		path := expandUserWithHome(config.ProfileDir, config.Home)
		if isDir(path) {
			return ProfileDiscovery{OK: true, Path: path, Source: "explicit"}
		}
		return ProfileDiscovery{OK: false, Path: path, Source: "explicit", Error: "profile directory not found"}
	}
	goos := normalizedGOOS(config.GOOS)
	switch goos {
	case "darwin":
		return discoverMacProfile(config)
	case "windows":
		return discoverWindowsProfile(config)
	default:
		return ProfileDiscovery{OK: false, Error: "messenger supports macOS and Windows desktop profiles only"}
	}
}

func normalizedReadScope(scope string) (string, error) {
	normalized := strings.TrimSpace(scope)
	if normalized == "" {
		normalized = "unread"
	}
	if normalized != "unread" && normalized != "recent" {
		return "", fmt.Errorf("--scope must be unread or recent")
	}
	return normalized, nil
}

func normalizedPositive(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func intFromPayload(payload map[string]any, key string) int {
	value, _ := payload[key].(int)
	return value
}

func DiscoverBrowser(config Config) BrowserDiscovery {
	if strings.TrimSpace(config.BrowserPath) != "" {
		path := expandUserWithHome(config.BrowserPath, config.Home)
		if exists(path) {
			return BrowserDiscovery{OK: true, Path: path, Source: "explicit"}
		}
		return BrowserDiscovery{OK: false, Path: path, Source: "explicit", Error: "browser executable not found"}
	}
	if envPath := strings.TrimSpace(os.Getenv("IXF_MESSENGER_BROWSER_PATH")); envPath != "" {
		path := expandUserWithHome(envPath, config.Home)
		if exists(path) {
			return BrowserDiscovery{OK: true, Path: path, Source: "env"}
		}
		return BrowserDiscovery{OK: false, Path: path, Source: "env", Error: "browser executable not found"}
	}
	for _, candidate := range browserCandidates(normalizedGOOS(config.GOOS), config) {
		if exists(candidate) {
			return BrowserDiscovery{OK: true, Path: candidate, Source: "auto"}
		}
	}
	for _, name := range []string{"google-chrome", "chrome", "chromium", "chromium-browser", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return BrowserDiscovery{OK: true, Path: path, Source: "path"}
		}
	}
	return BrowserDiscovery{OK: false, Error: "Chrome, Chromium, or Edge executable not found"}
}

func CookieDiagnostics(cookiePath string) map[string]any {
	path := expandUser(cookiePath)
	if strings.TrimSpace(path) == "" {
		path = DefaultCookieJSON
	}
	payload := map[string]any{
		"ok":          false,
		"exists":      false,
		"path":        path,
		"cookieCount": 0,
		"hasCsrf":     false,
		"hasLgwCsrf":  false,
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
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
		payload["error"] = "invalid cookie JSON"
		return payload
	}
	hasCsrf := false
	hasLgwCsrf := false
	for _, cookie := range cookies {
		name, _ := cookie["name"].(string)
		value, _ := cookie["value"].(string)
		if name == "_csrf_token" && value != "" {
			hasCsrf = true
		}
		if name == "lgw_csrf_token" && value != "" {
			hasLgwCsrf = true
		}
	}
	payload["ok"] = true
	payload["exists"] = true
	payload["cookieCount"] = len(cookies)
	payload["hasCsrf"] = hasCsrf
	payload["hasLgwCsrf"] = hasLgwCsrf
	return payload
}

func CloneProfile(source string, tempRoot string) (CloneResult, error) {
	source = expandUser(source)
	if !isDir(source) {
		return CloneResult{}, fmt.Errorf("profile directory not found: %s", source)
	}
	root := tempRoot
	var err error
	if strings.TrimSpace(root) == "" {
		root, err = os.MkdirTemp("", "ixf_messenger_profile_")
		if err != nil {
			return CloneResult{}, err
		}
	} else if err := os.MkdirAll(root, 0o700); err != nil {
		return CloneResult{}, err
	}
	destination := filepath.Join(root, "profile_explorer")
	if err := copyProfileTree(source, destination); err != nil {
		return CloneResult{}, err
	}
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		_ = os.Remove(filepath.Join(destination, name))
	}
	return CloneResult{Source: source, Path: destination}, nil
}

func discoverMacProfile(config ProfileConfig) ProfileDiscovery {
	root := expandUserWithHome(valueOrDefault(config.AppSupport, DefaultMacAppRoot), config.Home)
	pattern := filepath.Join(root, "aha", "users", "*", "profile_explorer")
	candidates, err := filepath.Glob(pattern)
	if err != nil {
		return ProfileDiscovery{OK: false, Error: err.Error()}
	}
	candidates = existingDirs(candidates)
	if len(candidates) == 0 {
		return ProfileDiscovery{OK: false, Error: "no profile_explorer found under " + root}
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, _ := os.Stat(candidates[i])
		right, _ := os.Stat(candidates[j])
		return left.ModTime().After(right.ModTime())
	})
	return ProfileDiscovery{OK: true, Path: candidates[0], Source: "macos-profile-explorer"}
}

func discoverWindowsProfile(config ProfileConfig) ProfileDiscovery {
	appData := strings.TrimSpace(config.AppData)
	if appData == "" {
		appData = os.Getenv("APPDATA")
	}
	if appData == "" {
		return ProfileDiscovery{OK: false, Error: "APPDATA is not set"}
	}
	root := expandUserWithHome(appData, config.Home)
	candidates := []string{
		filepath.Join(root, "LarkShell", "User Data", "Default"),
		filepath.Join(root, "LarkShell", "User Data", "Default", "profile_explorer"),
	}
	for _, candidate := range candidates {
		if isDir(candidate) {
			return ProfileDiscovery{OK: true, Path: candidate, Source: "windows-larkshell"}
		}
	}
	return ProfileDiscovery{OK: false, Error: "Windows LarkShell profile not found under APPDATA"}
}

func copyProfileTree(source string, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if path != source && ignoredProfileNames[name] {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source string, destination string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func browserCandidates(goos string, config Config) []string {
	switch goos {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		return []string{
			filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(local, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
		}
	default:
		return nil
	}
}

func existingDirs(paths []string) []string {
	var result []string
	for _, path := range paths {
		if isDir(path) {
			result = append(result, path)
		}
	}
	return result
}

func normalizedGOOS(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return runtime.GOOS
}

func valueOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func expandUser(path string) string {
	return expandUserWithHome(path, "")
}

func expandUserWithHome(path string, home string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if strings.TrimSpace(home) == "" {
			if value := os.Getenv("HOME"); value != "" {
				home = value
			} else if userHome, err := os.UserHomeDir(); err == nil {
				home = userHome
			}
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
