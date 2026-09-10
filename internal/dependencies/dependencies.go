package dependencies

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/serialq7ic4/ixf-toolbox/internal/docspublish"
	"github.com/serialq7ic4/ixf-toolbox/internal/messenger"
	ixfupdate "github.com/serialq7ic4/ixf-toolbox/internal/update"
)

type Runner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, ...string) ([]byte, error)
}

type ReleaseLoader func(string, string) (ixfupdate.Release, error)
type MermaidStatus func() map[string]any
type MessengerStatus func(string) map[string]any

type Options struct {
	CurrentVersion  string
	CookiesPath     string
	ReleaseLoader   ReleaseLoader
	MermaidStatus   MermaidStatus
	MessengerStatus MessengerStatus
	Runner          Runner
}

type InstallOptions struct {
	Options
	Apply bool
}

type processRunner struct{}

type installCommand struct {
	Name    string
	Args    []string
	Display string
}

func (processRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (processRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func Diagnose(options Options) map[string]any {
	return diagnose(options, true)
}

func Install(options InstallOptions) (map[string]any, error) {
	before := diagnose(options.Options, false)
	commands := plannedCommands(before)
	payload := map[string]any{
		"ok":           true,
		"dryRun":       !options.Apply,
		"apply":        options.Apply,
		"applied":      false,
		"commands":     commandDisplays(commands),
		"dependencies": before,
	}
	if !options.Apply || len(commands) == 0 {
		return payload, nil
	}

	runner := options.Runner
	if runner == nil {
		runner = processRunner{}
	}
	for _, command := range commands {
		if _, err := runner.LookPath(command.Name); err != nil {
			payload["ok"] = false
			payload["failedCommand"] = command.Display
			payload["remediation"] = "Install Node.js with npm/npx available in PATH, then retry the same allowlisted dependency command."
			return payload, fmt.Errorf("dependency executable %q is unavailable for allowlisted command %q", command.Name, command.Display)
		}
	}
	for _, command := range commands {
		if _, err := runner.Run(context.Background(), command.Name, command.Args...); err != nil {
			payload["ok"] = false
			payload["failedCommand"] = command.Display
			payload["remediation"] = "Run the named allowlisted command manually, resolve its local toolchain error, and retry `ixf deps install --apply`."
			return payload, fmt.Errorf("allowlisted dependency command failed: %s", command.Display)
		}
	}
	payload["applied"] = true
	payload["dependencies"] = diagnose(options.Options, false)
	return payload, nil
}

func diagnose(options Options, includeUpdate bool) map[string]any {
	mermaidStatus := options.MermaidStatus
	if mermaidStatus == nil {
		mermaidStatus = docspublish.MermaidDependencyStatus
	}
	messengerStatus := options.MessengerStatus
	if messengerStatus == nil {
		messengerStatus = messengerDependencyStatus
	}
	mermaidResult := mermaidStatus()
	messengerResult := messengerStatus(options.CookiesPath)
	updateResult := map[string]any{
		"ok":      true,
		"checked": false,
		"reason":  "not-used-by-installer",
	}
	if includeUpdate {
		updateResult = updateDependencyStatus(options)
	}
	return map[string]any{
		"ok":        boolFromMap(mermaidResult, "ok") && boolFromMap(messengerResult, "ok") && boolFromMap(updateResult, "ok"),
		"mermaid":   mermaidResult,
		"messenger": messengerResult,
		"update":    updateResult,
	}
}

func plannedCommands(dependencies map[string]any) []installCommand {
	mermaid := mapFromAny(dependencies["mermaid"])
	allowlist := allowedInstallCommands()
	commands := make([]installCommand, 0, len(allowlist))
	if !boolFromMap(mermaid, "available") {
		commands = append(commands, allowlist[0])
	}
	if !boolFromMap(mermaid, "ready") {
		commands = append(commands, allowlist[1])
	}
	return commands
}

func allowedInstallCommands() []installCommand {
	return []installCommand{
		{Name: "npm", Args: []string{"install", "-g", "@mermaid-js/mermaid-cli"}, Display: "npm install -g @mermaid-js/mermaid-cli"},
		{Name: "npx", Args: []string{"puppeteer", "browsers", "install", "chrome-headless-shell"}, Display: "npx puppeteer browsers install chrome-headless-shell"},
	}
}

func commandDisplays(commands []installCommand) []string {
	displays := make([]string, 0, len(commands))
	for _, command := range commands {
		displays = append(displays, command.Display)
	}
	return displays
}

func messengerDependencyStatus(cookiesPath string) map[string]any {
	payload := messenger.Doctor(messenger.Config{CookiesPath: cookiesPath})
	profile := map[string]any{"ok": false}
	if value, ok := payload["profile"].(messenger.ProfileDiscovery); ok {
		profile["ok"] = value.OK
		if value.Source != "" {
			profile["source"] = value.Source
		}
		if value.Error != "" {
			profile["error"] = value.Error
		}
	}
	browser := map[string]any{"ok": false}
	if value, ok := payload["browser"].(messenger.BrowserDiscovery); ok {
		browser["ok"] = value.OK
		if value.Source != "" {
			browser["source"] = value.Source
		}
		if value.Error != "" {
			browser["error"] = value.Error
		}
	}
	cookies := map[string]any{
		"ok":          boolFromMap(payload["cookies"], "ok"),
		"exists":      boolFromMap(payload["cookies"], "exists"),
		"cookieCount": intFromMap(payload["cookies"], "cookieCount"),
		"hasCsrf":     boolFromMap(payload["cookies"], "hasCsrf"),
		"hasLgwCsrf":  boolFromMap(payload["cookies"], "hasLgwCsrf"),
	}
	result := map[string]any{
		"ok":          boolFromMap(payload, "ok"),
		"installable": false,
		"requiredFor": "messenger browser automation",
		"profile":     profile,
		"browser":     browser,
		"cookies":     cookies,
	}
	if messengerInfo, ok := payload["messenger"].(map[string]any); ok {
		result["supportedPlatform"] = boolFromMap(messengerInfo, "supportedPlatform")
		if goosValue, _ := messengerInfo["goos"].(string); goosValue != "" {
			result["goos"] = goosValue
		}
	}
	if remediation, ok := payload["remediation"].([]string); ok && len(remediation) > 0 {
		result["remediation"] = remediation
	}
	return result
}

func updateDependencyStatus(options Options) map[string]any {
	result := map[string]any{
		"ok":          false,
		"repo":        ixfupdate.DefaultReleaseRepo,
		"requiredFor": "update check and self-update",
		"installable": false,
	}
	loader := options.ReleaseLoader
	if loader == nil {
		loader = ixfupdate.LoadRelease
	}
	release, err := loader(ixfupdate.DefaultReleaseRepo, "")
	if err != nil {
		result["error"] = err.Error()
		result["remediation"] = "Ensure GitHub Releases are reachable. If a proxy is required, set HTTPS_PROXY, HTTP_PROXY, and ALL_PROXY before running update commands."
		return result
	}
	check, err := ixfupdate.CheckLatestRelease(ixfupdate.DefaultReleaseRepo, options.CurrentVersion, release)
	if err != nil {
		result["error"] = err.Error()
		result["remediation"] = "Check the release metadata returned by GitHub and retry `ixf update check --json`."
		return result
	}
	for key, value := range check {
		result[key] = value
	}
	result["ok"] = true
	if boolFromMap(result, "updateAvailable") {
		result["remediation"] = "Run `ixf update self --apply --json`."
	}
	return result
}

func boolFromMap(raw any, key string) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	result, _ := values[key].(bool)
	return result
}

func mapFromAny(raw any) map[string]any {
	if values, ok := raw.(map[string]any); ok {
		return values
	}
	return map[string]any{}
}

func intFromMap(raw any, key string) int {
	values, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
