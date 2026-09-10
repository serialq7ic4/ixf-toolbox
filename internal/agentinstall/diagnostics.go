package agentinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultTimeout = 2 * time.Second

var skillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
	"ixf-messenger-reader",
	"ixf-messenger-writer",
}

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type Options struct {
	Home    string
	Timeout time.Duration
	Run     CommandRunner
}

type HostStatus struct {
	Status         string   `json:"status"`
	ID             string   `json:"id,omitempty"`
	Version        string   `json:"version,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Dir            string   `json:"dir,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	InstalledCount int      `json:"installedCount,omitempty"`
	ExpectedCount  int      `json:"expectedCount,omitempty"`
}

type Report struct {
	LegacyRawSkills   map[string]HostStatus `json:"legacyRawSkills"`
	NativePlugin      map[string]HostStatus `json:"nativePlugin"`
	DuplicateLoadRisk bool                  `json:"duplicateLoadRisk"`
	Remediation       []string              `json:"remediation"`
}

type pluginEntry struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	Version  string `json:"version"`
}

type hostSpec struct {
	Key       string
	Command   string
	LegacyDir string
	Decode    func([]byte) ([]pluginEntry, error)
}

func Diagnose(ctx context.Context, options Options) Report {
	home := strings.TrimSpace(options.Home)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runner := options.Run
	if runner == nil {
		runner = runCommand
	}

	report := Report{
		LegacyRawSkills: map[string]HostStatus{},
		NativePlugin:    map[string]HostStatus{},
		Remediation:     []string{},
	}
	hosts := []hostSpec{
		{Key: "codex", Command: "codex", LegacyDir: ".codex", Decode: decodeCodexPlugins},
		{Key: "claudeCode", Command: "claude", LegacyDir: ".claude", Decode: decodeClaudePlugins},
	}
	for _, host := range hosts {
		report.LegacyRawSkills[host.Key] = diagnoseLegacySkills(home, host.LegacyDir)
		report.NativePlugin[host.Key] = diagnoseNativePlugin(ctx, timeout, runner, host)
		if report.LegacyRawSkills[host.Key].Status == "installed" && report.NativePlugin[host.Key].Status == "installed" {
			report.DuplicateLoadRisk = true
			report.Remediation = append(report.Remediation, remediationFor(host.Key))
		}
	}
	return report
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func diagnoseNativePlugin(parent context.Context, timeout time.Duration, runner CommandRunner, host hostSpec) HostStatus {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	output, err := runner(ctx, host.Command, "plugin", "list", "--json")
	if err != nil {
		return HostStatus{Status: "unknown", Reason: classifyCommandError(ctx, err)}
	}
	entries, err := host.Decode(output)
	if err != nil {
		return HostStatus{Status: "unknown", Reason: "invalid-json"}
	}
	for _, entry := range entries {
		id := entry.PluginID
		if id == "" {
			id = entry.ID
		}
		if pluginName(id) == "ixf-toolbox" {
			return HostStatus{Status: "installed", ID: id, Version: entry.Version}
		}
	}
	return HostStatus{Status: "not-installed"}
}

func classifyCommandError(ctx context.Context, err error) string {
	var unavailable *exec.Error
	if errors.As(err, &unavailable) {
		return "command-unavailable"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	return "command-failed"
}

func decodeCodexPlugins(content []byte) ([]pluginEntry, error) {
	var envelope struct {
		Installed json.RawMessage `json:"installed"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil || len(envelope.Installed) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Installed), []byte("null")) {
		return nil, errors.New("invalid Codex plugin list")
	}
	var entries []pluginEntry
	if err := json.Unmarshal(envelope.Installed, &entries); err != nil {
		return nil, errors.New("invalid Codex plugin list")
	}
	return entries, nil
}

func decodeClaudePlugins(content []byte) ([]pluginEntry, error) {
	var entries []pluginEntry
	if err := json.Unmarshal(content, &entries); err != nil || entries == nil {
		return nil, errors.New("invalid Claude plugin list")
	}
	return entries, nil
}

func pluginName(id string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(id), "@")
	return name
}

func diagnoseLegacySkills(home, hostDir string) HostStatus {
	root := filepath.Join(home, hostDir, "skills")
	status := HostStatus{
		Status:        "not-installed",
		Dir:           root,
		ExpectedCount: len(skillNames),
	}
	for _, name := range skillNames {
		path := filepath.Join(root, name, "SKILL.md")
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			status.Paths = append(status.Paths, path)
		}
	}
	sort.Strings(status.Paths)
	status.InstalledCount = len(status.Paths)
	if status.InstalledCount > 0 {
		status.Status = "installed"
	}
	return status
}

func remediationFor(host string) string {
	switch host {
	case "codex":
		return "Codex has both the native ixf-toolbox plugin and legacy raw skills; manually keep only one installation path. ixf doctor does not delete either installation."
	case "claudeCode":
		return "Claude Code has both the native ixf-toolbox plugin and legacy raw skills; manually keep only one installation path. ixf doctor does not delete either installation."
	default:
		return "Manually keep only one ixf-toolbox installation path. ixf doctor does not delete installations."
	}
}
