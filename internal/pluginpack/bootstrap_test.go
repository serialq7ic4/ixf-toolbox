package pluginpack

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

type bootstrapResult struct {
	OK          bool   `json:"ok"`
	DryRun      bool   `json:"dryRun"`
	Apply       bool   `json:"apply"`
	Version     string `json:"version"`
	Asset       string `json:"asset"`
	ReleaseHost string `json:"releaseHost"`
	TargetPath  string `json:"targetPath"`
}

func generateRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := newPluginFixture(t, "1.2.3")
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	if err := copyRegularTree(filepath.Join(repositoryRoot, "plugin-src"), filepath.Join(root, "plugin-src")); err != nil {
		t.Fatal(err)
	}
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return root
}

func bootstrapFixtureServer(t *testing.T, version string, asset, checksumAsset []byte, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	assetName := fmt.Sprintf("ixf_%s_%s_%s", version, runtime.GOOS, runtime.GOARCH)
	checksumName := fmt.Sprintf("ixf_%s_checksums.txt", version)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	digest := sha256.Sum256(checksumAsset)
	checksum := fmt.Sprintf("%x  %s\n", digest, assetName)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+assetName):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(asset)
		case strings.HasSuffix(r.URL.Path, "/"+checksumName):
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, checksum)
		default:
			http.NotFound(w, r)
		}
	}))
}

func bootstrapEnv(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	if _, ok := overrides["HOME"]; !ok {
		values["HOME"] = t.TempDir()
	}
	if _, ok := overrides["LOCALAPPDATA"]; !ok {
		values["LOCALAPPDATA"] = values["HOME"]
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func runBootstrapCommand(t *testing.T, root string, overrides map[string]string, args ...string) ([]byte, error) {
	t.Helper()
	pluginRoot := filepath.Join(root, "plugins", "codex", "ixf-toolbox")
	command := exec.Command(filepath.Join(pluginRoot, "scripts", "bootstrap-runtime.sh"), args...)
	command.Env = bootstrapEnv(t, overrides)
	return command.CombinedOutput()
}

func runBootstrap(t *testing.T, root string, overrides map[string]string, args ...string) bootstrapResult {
	t.Helper()
	output, err := runBootstrapCommand(t, root, overrides, args...)
	if err != nil {
		t.Fatalf("bootstrap %v: %v\n%s", args, err, output)
	}
	var result bootstrapResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode bootstrap output: %v\n%s", err, output)
	}
	return result
}

func readBootstrapFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func runPowerShellBootstrapCommand(t *testing.T, root string, overrides map[string]string, args ...string) ([]byte, error) {
	t.Helper()
	powershell, err := exec.LookPath("pwsh")
	if err != nil {
		powershell, err = exec.LookPath("powershell")
		if err != nil {
			t.Skip("PowerShell is not installed")
		}
	}
	script := filepath.Join(root, "plugins", "codex", "ixf-toolbox", "scripts", "bootstrap-runtime.ps1")
	commandArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(powershell, commandArgs...)
	command.Env = bootstrapEnv(t, overrides)
	return command.CombinedOutput()
}

func runSessionStart(t *testing.T, pluginRoot, input string) []byte {
	t.Helper()
	dispatcher := filepath.Join(pluginRoot, "hooks", "run-hook.cmd")
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		cmdExe, err := exec.LookPath("cmd.exe")
		if err != nil {
			t.Fatal("cmd.exe is required on Windows")
		}
		command = exec.Command(cmdExe, "/d", "/c", dispatcher, "session-start")
	} else {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is required to execute the polyglot hook dispatcher")
		}
		command = exec.Command(bash, dispatcher, "session-start")
	}
	command.Stdin = strings.NewReader(input)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("session-start: %v\n%s", err, output)
	}
	if runtime.GOOS == "windows" && len(bytes.TrimSpace(output)) == 0 {
		t.Skip("Windows hook dispatcher found no supported Bash executable")
	}
	return output
}

func TestGeneratedRuntimeMetadataAndHostBoundaries(t *testing.T) {
	root := generateRepositoryFixture(t)
	for _, host := range []string{"codex", "claude"} {
		path := filepath.Join(root, "plugins", host, "ixf-toolbox", "runtime.json")
		payload := readJSONMap(t, path)
		if payload["schemaVersion"] != float64(1) || payload["repository"] != "serialq7ic4/ixf-toolbox" || payload["releaseVersion"] != "1.2.3" || payload["minimumVersion"] != "1.2.3" {
			t.Fatalf("%s runtime metadata = %#v", host, payload)
		}
		artifacts, ok := payload["artifacts"].([]any)
		if !ok || len(artifacts) != 5 {
			t.Fatalf("%s artifacts = %#v", host, payload["artifacts"])
		}
		for _, script := range []string{"bootstrap-runtime.sh", "bootstrap-runtime.ps1"} {
			if _, err := os.Stat(filepath.Join(root, "plugins", host, "ixf-toolbox", "scripts", script)); err != nil {
				t.Fatalf("%s script %s: %v", host, script, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "plugins", "codex", "ixf-toolbox", "hooks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Codex package unexpectedly contains hooks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins", "claude", "ixf-toolbox", "hooks", "hooks.json")); err != nil {
		t.Fatalf("Claude package hooks: %v", err)
	}
}

func TestUnixBootstrapRequiresExplicitApplyAndVerifiesChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, asset, &requests)
	t.Cleanup(server.Close)
	home := t.TempDir()
	target := filepath.Join(home, "bin", "ixf")
	pathBefore := os.Getenv("PATH")

	dryRun := runBootstrap(t, root, map[string]string{
		"HOME":                           home,
		"PATH":                           pathBefore,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}, "--dry-run", "--install-dir", filepath.Dir(target))
	if !dryRun.OK || dryRun.Apply || !dryRun.DryRun || dryRun.Version != "1.2.3" || dryRun.ReleaseHost == "" {
		t.Fatalf("dry-run payload = %#v", dryRun)
	}
	if requests.Load() != 0 {
		t.Fatalf("dry-run contacted release server %d times", requests.Load())
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote target: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run created install directory: %v", err)
	}

	apply := runBootstrap(t, root, map[string]string{
		"HOME":                           home,
		"PATH":                           pathBefore,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}, "--apply", "--install-dir", filepath.Dir(target))
	if !apply.OK || !apply.Apply || apply.DryRun || apply.TargetPath != target || requests.Load() != 2 {
		t.Fatalf("apply payload = %#v, requests=%d", apply, requests.Load())
	}
	if got := readBootstrapFile(t, target); !bytes.Equal(got, asset) {
		t.Fatalf("installed bytes = %q", got)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary is not executable: info=%v err=%v", info, err)
	}
	if os.Getenv("PATH") != pathBefore {
		t.Fatalf("parent PATH changed from %q to %q", pathBefore, os.Getenv("PATH"))
	}
	for _, profile := range []string{".profile", ".bashrc", ".zprofile", ".zshrc"} {
		if _, err := os.Stat(filepath.Join(home, profile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("bootstrap modified shell profile %s: %v", profile, err)
		}
	}
}

func TestUnixBootstrapDefaultsToDryRunAndRejectsConflictingModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	home := t.TempDir()
	for _, args := range [][]string{{}, {"--dry-run", "--apply"}} {
		output, err := runBootstrapCommand(t, root, map[string]string{"HOME": home}, args...)
		if len(args) == 0 {
			if err != nil {
				t.Fatalf("bootstrap without mode failed: %v\n%s", err, output)
			}
			var result bootstrapResult
			if json.Unmarshal(output, &result) != nil || !result.OK || !result.DryRun || result.Apply {
				t.Fatalf("default mode output = %s", output)
			}
			continue
		}
		if err == nil || !strings.Contains(string(output), "mutually exclusive") {
			t.Fatalf("conflicting modes accepted: err=%v output=%s", err, output)
		}
	}
}

func TestUnixBootstrapRejectsInstallDirOutsideHomeAndSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	home := t.TempDir()
	outside := t.TempDir()
	for _, installDir := range []string{outside, filepath.Join(home, "escape", "bin")} {
		if strings.Contains(installDir, "escape") {
			if err := os.Symlink(outside, filepath.Join(home, "escape")); err != nil {
				t.Fatal(err)
			}
		}
		output, err := runBootstrapCommand(t, root, map[string]string{"HOME": home}, "--apply", "--install-dir", installDir)
		if err == nil || !strings.Contains(string(output), "inside the current user directory") {
			t.Fatalf("unsafe install directory accepted: dir=%s err=%v output=%s", installDir, err, output)
		}
	}
}

func TestUnixBootstrapRefusesChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, []byte("different bytes"), &requests)
	t.Cleanup(server.Close)
	home := t.TempDir()
	targetDir := filepath.Join(home, "bin")
	output, err := runBootstrapCommand(t, root, map[string]string{
		"HOME":                           home,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}, "--apply", "--install-dir", targetDir)
	if err == nil || strings.Contains(string(output), `"ok":true`) {
		t.Fatalf("checksum mismatch unexpectedly succeeded: err=%v output=%s", err, output)
	}
	if entries, readErr := os.ReadDir(targetDir); readErr == nil && len(entries) != 0 {
		t.Fatalf("checksum mismatch wrote install directory: %v", entries)
	}
}

func TestUnixBootstrapRejectsUntrustedReleaseOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	output, err := runBootstrapCommand(t, root, map[string]string{
		"HOME":                           t.TempDir(),
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": "http://127.0.0.1:1",
	}, "--dry-run")
	if err == nil || !strings.Contains(string(output), "HTTPS") {
		t.Fatalf("untrusted release override accepted: err=%v output=%s", err, output)
	}
}

func TestPowerShellBootstrapRequiresApplyAndVerifiesChecksum(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell bootstrap test")
	}
	root := generateRepositoryFixture(t)
	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, asset, &requests)
	t.Cleanup(server.Close)
	localAppData := t.TempDir()
	target := filepath.Join(localAppData, "bin", "ixf.exe")
	overrides := map[string]string{
		"LOCALAPPDATA":                   localAppData,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}
	dryOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-DryRun", "-InstallDir", filepath.Dir(target))
	if err != nil {
		t.Fatalf("PowerShell dry-run failed: %v\n%s", err, dryOutput)
	}
	var dryRun bootstrapResult
	if err := json.Unmarshal(dryOutput, &dryRun); err != nil || !dryRun.OK || !dryRun.DryRun || dryRun.Apply || requests.Load() != 0 {
		t.Fatalf("PowerShell dry-run = %s, requests=%d, err=%v", dryOutput, requests.Load(), err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PowerShell dry-run wrote target: %v", err)
	}
	applyOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-Apply", "-InstallDir", filepath.Dir(target))
	if err != nil {
		t.Fatalf("PowerShell apply failed: %v\n%s", err, applyOutput)
	}
	var apply bootstrapResult
	if err := json.Unmarshal(applyOutput, &apply); err != nil || !apply.OK || !apply.Apply || apply.TargetPath != target || requests.Load() != 2 {
		t.Fatalf("PowerShell apply = %s, requests=%d, err=%v", applyOutput, requests.Load(), err)
	}
	if got := readBootstrapFile(t, target); !bytes.Equal(got, asset) {
		t.Fatalf("PowerShell installed bytes = %q", got)
	}
}

func TestPowerShellBootstrapRejectsInvalidModesPathsAndChecksum(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell bootstrap test")
	}
	root := generateRepositoryFixture(t)
	localAppData := t.TempDir()
	for _, args := range [][]string{
		{"-DryRun", "-Apply"},
		{"-Apply", "-InstallDir", t.TempDir()},
		{"-Apply", "-InstallDir", filepath.Join(localAppData, "child", "..", "bin")},
	} {
		output, err := runPowerShellBootstrapCommand(t, root, map[string]string{"LOCALAPPDATA": localAppData}, args...)
		if err == nil || (!strings.Contains(string(output), "mutually exclusive") &&
			!strings.Contains(string(output), "inside the current user directory") &&
			!strings.Contains(string(output), "cannot contain traversal")) {
			t.Fatalf("invalid PowerShell invocation accepted: args=%v err=%v output=%s", args, err, output)
		}
	}

	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, []byte("different bytes"), &requests)
	t.Cleanup(server.Close)
	targetDir := filepath.Join(localAppData, "bin")
	overrides := map[string]string{
		"LOCALAPPDATA":                   localAppData,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}
	dryOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-DryRun", "-InstallDir", targetDir)
	if err != nil {
		t.Fatalf("PowerShell checksum dry-run failed: %v\n%s", err, dryOutput)
	}
	if requests.Load() != 0 {
		t.Fatalf("PowerShell dry-run contacted release server %d times", requests.Load())
	}
	applyOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-Apply", "-InstallDir", targetDir)
	if err == nil || strings.Contains(string(applyOutput), `"ok":true`) {
		t.Fatalf("PowerShell checksum mismatch unexpectedly succeeded: err=%v output=%s", err, applyOutput)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "ixf.exe")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PowerShell checksum mismatch wrote target: %v", err)
	}
}

func TestClaudeSessionStartHookOnlyInjectsRoutingHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook test; Windows host validation uses Claude's shell dispatcher")
	}
	root := generateRepositoryFixture(t)
	output := runSessionStart(t, filepath.Join(root, "plugins", "claude", "ixf-toolbox"), `{}`)
	var payload struct {
		Hook struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatal(err)
	}
	const want = "For i讯飞 / LarkShell links or document, sheets, bitable, OKR, and Messenger requests, consider the ixf-toolbox skills before choosing generic tools."
	if payload.Hook.Event != "SessionStart" || payload.Hook.Context != want {
		t.Fatalf("hook payload = %#v", payload)
	}
	for _, forbidden := range []string{"curl ", "ixf doctor", "cookies export", "setup"} {
		if strings.Contains(payload.Hook.Context, forbidden) {
			t.Fatalf("hook context executes work through %q", forbidden)
		}
	}
}

func TestClaudeSessionStartRegistration(t *testing.T) {
	root := generateRepositoryFixture(t)
	path := filepath.Join(root, "plugins", "claude", "ixf-toolbox", "hooks", "hooks.json")
	var payload struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Shell   string `json:"shell"`
				Async   bool   `json:"async"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(readBootstrapFile(t, path), &payload); err != nil {
		t.Fatal(err)
	}
	registrations := payload.Hooks["SessionStart"]
	if len(registrations) != 1 || registrations[0].Matcher != "startup|clear|compact" || len(registrations[0].Hooks) != 1 {
		t.Fatalf("SessionStart registration = %#v", registrations)
	}
	hook := registrations[0].Hooks[0]
	if hook.Type != "command" || hook.Command != `"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd" session-start` || hook.Shell != "bash" || hook.Async {
		t.Fatalf("SessionStart command = %#v", hook)
	}
}
