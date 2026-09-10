package agentinstall_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/serialq7ic4/ixf-toolbox/internal/agentinstall"
)

var legacySkillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
	"ixf-messenger-reader",
	"ixf-messenger-writer",
}

func fixtureOptions(t *testing.T, codexJSON, claudeJSON string) agentinstall.Options {
	t.Helper()
	home := t.TempDir()
	return agentinstall.Options{
		Home:    home,
		Timeout: time.Second,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			command := strings.Join(append([]string{name}, args...), " ")
			switch command {
			case "codex plugin list --json":
				return []byte(codexJSON), nil
			case "claude plugin list --json":
				return []byte(claudeJSON), nil
			default:
				return nil, fmt.Errorf("unexpected command %q", command)
			}
		},
	}
}

func writeLegacySkill(t *testing.T, home, host, skillName string) string {
	t.Helper()
	hostDir := map[string]string{"codex": ".codex", "claude": ".claude"}[host]
	if hostDir == "" {
		t.Fatalf("unsupported legacy host %q", host)
	}
	directory := filepath.Join(home, hostDir, "skills", skillName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "SKILL.md")
	if err := os.WriteFile(path, []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type treeEntry struct {
	Mode os.FileMode
	Data []byte
}

func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	result := map[string]treeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not regular", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = treeEntry{Mode: info.Mode().Perm(), Data: data}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func cmpTree(before, after map[string]treeEntry) string {
	paths := map[string]bool{}
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	var sorted []string
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	for _, path := range sorted {
		oldValue, oldOK := before[path]
		newValue, newOK := after[path]
		if !oldOK || !newOK || oldValue.Mode != newValue.Mode || !bytes.Equal(oldValue.Data, newValue.Data) {
			return fmt.Sprintf("changed %s", path)
		}
	}
	return ""
}

func lifecycleRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func runLifecycleHook(t *testing.T, pluginRoot string, input []byte) []byte {
	t.Helper()
	dispatcher := filepath.Join(pluginRoot, "hooks", "run-hook.cmd")
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		cmdExe, err := exec.LookPath("cmd.exe")
		if err != nil {
			t.Skip("cmd.exe is required for the Windows hook lifecycle test")
		}
		command = exec.Command(cmdExe, "/d", "/c", dispatcher, "session-start")
	} else {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is required for the POSIX hook lifecycle test")
		}
		command = exec.Command(bash, dispatcher, "session-start")
	}
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("session-start: %v\n%s", err, output)
	}
	if runtime.GOOS == "windows" && len(bytes.TrimSpace(output)) == 0 {
		t.Skip("Windows hook dispatcher found no supported Bash executable")
	}
	return output
}

func TestDiagnoseNativePluginStates(t *testing.T) {
	tests := []struct {
		name       string
		codexJSON  string
		claudeJSON string
		codexWant  string
		claudeWant string
	}{
		{"installed", `{"installed":[{"pluginId":"ixf-toolbox@ixf-toolbox","version":"3.27.0"}]}`, `[{"id":"ixf-toolbox@ixf-toolbox","version":"3.27.0","enabled":true}]`, "installed", "installed"},
		{"absent", `{"installed":[]}`, `[]`, "not-installed", "not-installed"},
		{"different-plugin", `{"installed":[{"id":"other@catalog","version":"9.0.0"}]}`, `[{"pluginId":"other@catalog","version":"9.0.0"}]`, "not-installed", "not-installed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := agentinstall.Diagnose(context.Background(), fixtureOptions(t, test.codexJSON, test.claudeJSON))
			if report.NativePlugin["codex"].Status != test.codexWant || report.NativePlugin["claudeCode"].Status != test.claudeWant {
				t.Fatalf("report = %#v", report)
			}
		})
	}
}

func TestDiagnoseLegacySkillsAndDuplicateLoadRisk(t *testing.T) {
	options := fixtureOptions(t,
		`{"installed":[{"pluginId":"ixf-toolbox@team","version":"3.27.0"}]}`,
		`[]`,
	)
	codexPath := writeLegacySkill(t, options.Home, "codex", legacySkillNames[0])
	claudePath := writeLegacySkill(t, options.Home, "claude", legacySkillNames[1])
	before := snapshotTree(t, options.Home)
	report := agentinstall.Diagnose(context.Background(), options)
	after := snapshotTree(t, options.Home)
	if diff := cmpTree(before, after); diff != "" {
		t.Fatalf("diagnosis modified legacy skills: %s", diff)
	}
	if !report.DuplicateLoadRisk {
		t.Fatalf("duplicate load risk = false: %#v", report)
	}
	if report.LegacyRawSkills["codex"].Status != "installed" || report.LegacyRawSkills["codex"].InstalledCount != 1 || report.LegacyRawSkills["codex"].ExpectedCount != len(legacySkillNames) {
		t.Fatalf("Codex legacy status = %#v", report.LegacyRawSkills["codex"])
	}
	if report.LegacyRawSkills["claudeCode"].Status != "installed" || report.NativePlugin["claudeCode"].Status != "not-installed" {
		t.Fatalf("Claude status = legacy %#v native %#v", report.LegacyRawSkills["claudeCode"], report.NativePlugin["claudeCode"])
	}
	if !containsPath(report.LegacyRawSkills["codex"].Paths, codexPath) || !containsPath(report.LegacyRawSkills["claudeCode"].Paths, claudePath) {
		t.Fatalf("legacy paths = %#v", report.LegacyRawSkills)
	}
	if len(report.Remediation) == 0 {
		t.Fatalf("duplicate report has no remediation: %#v", report)
	}
}

func TestDiagnoseClassifiesHostInspectionFailuresWithoutLeakingOutput(t *testing.T) {
	tests := []struct {
		name   string
		run    agentinstall.CommandRunner
		reason string
	}{
		{
			name: "command-unavailable",
			run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
				if name == "codex" {
					return nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
				}
				return []byte(`[]`), nil
			},
			reason: "command-unavailable",
		},
		{
			name: "timeout",
			run: func(ctx context.Context, name string, _ ...string) ([]byte, error) {
				if name == "codex" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return []byte(`[]`), nil
			},
			reason: "timeout",
		},
		{
			name: "command-failed",
			run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
				if name == "codex" {
					return []byte("SECRET stdout https://private.example"), errors.New("SECRET stderr --token=abc")
				}
				return []byte(`[]`), nil
			},
			reason: "command-failed",
		},
		{
			name: "malformed-json",
			run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
				if name == "codex" {
					return []byte(`{not-json SECRET`), nil
				}
				return []byte(`[]`), nil
			},
			reason: "invalid-json",
		},
		{
			name: "wrong-schema",
			run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
				if name == "codex" {
					return []byte(`{"available":[]}`), nil
				}
				return []byte(`[]`), nil
			},
			reason: "invalid-json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			timeout := time.Second
			if test.name == "timeout" {
				timeout = 20 * time.Millisecond
			}
			started := time.Now()
			report := agentinstall.Diagnose(context.Background(), agentinstall.Options{
				Home:    t.TempDir(),
				Timeout: timeout,
				Run:     test.run,
			})
			if test.name == "timeout" && time.Since(started) < 15*time.Millisecond {
				t.Fatalf("timeout runner did not wait for context cancellation")
			}
			status := report.NativePlugin["codex"]
			if status.Status != "unknown" || status.Reason != test.reason {
				t.Fatalf("status = %#v", status)
			}
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"SECRET", "private.example", "--token", "plugin list --json"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("report leaked %q: %s", forbidden, encoded)
				}
			}
		})
	}
}

func TestDiagnoseDefaultTimeoutIsTwoSeconds(t *testing.T) {
	var remaining time.Duration
	report := agentinstall.Diagnose(context.Background(), agentinstall.Options{
		Home: t.TempDir(),
		Run: func(ctx context.Context, name string, _ ...string) ([]byte, error) {
			if name == "codex" {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("runner context has no deadline")
				}
				remaining = time.Until(deadline)
				return []byte(`{"installed":[]}`), nil
			}
			return []byte(`[]`), nil
		},
	})
	if report.NativePlugin["codex"].Status != "not-installed" || remaining < 1500*time.Millisecond || remaining > 2100*time.Millisecond {
		t.Fatalf("default timeout remaining=%s report=%#v", remaining, report)
	}
}

func TestNativePluginHostLifecycle(t *testing.T) {
	root := lifecycleRepoRoot(t)
	codexRoot := filepath.Join(root, "plugins", "codex", "ixf-toolbox")
	claudeRoot := filepath.Join(root, "plugins", "claude", "ixf-toolbox")
	type pluginManifest struct {
		Name    string          `json:"name"`
		Version string          `json:"version"`
		Skills  string          `json:"skills"`
		Hooks   json.RawMessage `json:"hooks"`
	}
	readManifest := func(path string) pluginManifest {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var manifest pluginManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return manifest
	}
	codex := readManifest(filepath.Join(codexRoot, ".codex-plugin", "plugin.json"))
	claude := readManifest(filepath.Join(claudeRoot, ".claude-plugin", "plugin.json"))
	if codex.Name != "ixf-toolbox" || claude.Name != codex.Name || codex.Version == "" || claude.Version != codex.Version {
		t.Fatalf("plugin manifests = %#v, %#v", codex, claude)
	}
	if codex.Skills != "./skills/" || codex.Hooks != nil || claude.Skills != "" || claude.Hooks != nil {
		t.Fatalf("host manifest boundaries = %#v, %#v", codex, claude)
	}

	type hookCommand struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Shell   string `json:"shell"`
		Async   bool   `json:"async"`
	}
	type hookRegistration struct {
		Matcher string        `json:"matcher"`
		Hooks   []hookCommand `json:"hooks"`
	}
	var hookConfig struct {
		Hooks map[string][]hookRegistration `json:"hooks"`
	}
	hookRaw, err := os.ReadFile(filepath.Join(claudeRoot, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(hookRaw, &hookConfig); err != nil {
		t.Fatal(err)
	}
	registrations := hookConfig.Hooks["SessionStart"]
	if len(registrations) != 1 || registrations[0].Matcher != "startup|clear|compact" || len(registrations[0].Hooks) != 1 {
		t.Fatalf("SessionStart registrations = %#v", registrations)
	}
	registration := registrations[0].Hooks[0]
	if registration.Type != "command" || !strings.Contains(registration.Command, "${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd") || registration.Async {
		t.Fatalf("SessionStart command = %#v", registration)
	}

	home := t.TempDir()
	homeBefore := snapshotTree(t, home)
	pluginBefore := snapshotTree(t, claudeRoot)
	codexJSON, err := json.Marshal(map[string]any{
		"installed": []map[string]string{{"pluginId": "ixf-toolbox@ixf-toolbox", "version": codex.Version}},
	})
	if err != nil {
		t.Fatal(err)
	}
	claudeJSON, err := json.Marshal([]map[string]any{{"id": "ixf-toolbox@ixf-toolbox", "version": claude.Version, "enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	report := agentinstall.Diagnose(context.Background(), agentinstall.Options{
		Home:    home,
		Timeout: time.Second,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			command := strings.Join(append([]string{name}, args...), " ")
			switch command {
			case "codex plugin list --json":
				return codexJSON, nil
			case "claude plugin list --json":
				return claudeJSON, nil
			default:
				return nil, fmt.Errorf("unexpected command %q", command)
			}
		},
	})
	if report.NativePlugin["codex"].Status != "installed" || report.NativePlugin["claudeCode"].Status != "installed" || report.DuplicateLoadRisk {
		t.Fatalf("native host report = %#v", report)
	}
	hookOutput := runLifecycleHook(t, claudeRoot, []byte("{}"))
	var hookPayload struct {
		Hook struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(hookOutput, &hookPayload); err != nil {
		t.Fatalf("decode hook output: %v\n%s", err, hookOutput)
	}
	const wantContext = "For i讯飞 / LarkShell links or document, sheets, bitable, OKR, and Messenger requests, consider the ixf-toolbox skills before choosing generic tools."
	if hookPayload.Hook.Event != "SessionStart" || hookPayload.Hook.Context != wantContext {
		t.Fatalf("hook payload = %#v", hookPayload)
	}
	pluginAfter := snapshotTree(t, claudeRoot)
	if diff := cmpTree(pluginBefore, pluginAfter); diff != "" {
		t.Fatalf("hook modified the Claude plugin package: %s", diff)
	}
	homeAfter := snapshotTree(t, home)
	if diff := cmpTree(homeBefore, homeAfter); diff != "" {
		t.Fatalf("host lifecycle modified HOME: %s", diff)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
