package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommandPrintsUnifiedCLIName(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	want := "ixf " + version
	if strings.TrimSpace(stdout.String()) != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRootHelpListsCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"usage: ixf", "docs", "okr", "messenger", "update"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout missing %q: %s", expected, stdout.String())
		}
	}
}

func TestDocsAndOKRHelpListSupportedSubcommands(t *testing.T) {
	tests := []struct {
		args     []string
		expected []string
	}{
		{args: []string{"docs", "--help"}, expected: []string{"usage: ixf docs", "read", "publish", "inspect"}},
		{args: []string{"okr", "--help"}, expected: []string{"usage: ixf okr", "read", "write"}},
		{args: []string{"messenger", "--help"}, expected: []string{"usage: ixf messenger", "doctor", "open"}},
	}
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		code := run(test.args, &stdout, &stderr)

		if code != 0 {
			t.Fatalf("run(%v) exit code = %d, want 0; stderr=%q", test.args, code, stderr.String())
		}
		for _, expected := range test.expected {
			if !strings.Contains(stdout.String(), expected) {
				t.Fatalf("run(%v) stdout missing %q: %s", test.args, expected, stdout.String())
			}
		}
	}
}

func TestMessengerDoctorJSONIsSecretSafe(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	browser := filepath.Join(home, "chrome")
	cookiesPath := filepath.Join(home, "cookies.json")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	if err := os.WriteFile(browser, []byte("browser"), 0o600); err != nil {
		t.Fatalf("write browser: %v", err)
	}
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"dummy-csrf"}]`), 0o600); err != nil {
		t.Fatalf("write cookies: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{
		"messenger", "doctor",
		"--profile-dir", profile,
		"--browser-path", browser,
		"--cookies", cookiesPath,
		"--goos", "darwin",
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("messenger doctor exit code = %d, stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "dummy-csrf") {
		t.Fatalf("messenger doctor leaked cookie value: %s", stdout.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode messenger doctor json: %v\n%s", err, stdout.String())
	}
	if payload["runtime"] != "go" || payload["domain"] != "messenger" {
		t.Fatalf("messenger doctor payload = %+v", payload)
	}
}

func TestMessengerOpenDryRunValidatesArgumentsAndPrintsPlan(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"messenger", "open", "--mode", "conversation", "--dry-run", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing target exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--to is required") {
		t.Fatalf("missing target stderr = %q", stderr.String())
	}

	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	browser := filepath.Join(home, "chrome")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	if err := os.WriteFile(browser, []byte("browser"), 0o600); err != nil {
		t.Fatalf("write browser: %v", err)
	}
	stdout.Reset()
	stderr.Reset()

	code := run([]string{
		"messenger", "open",
		"--to", "示例群聊",
		"--mode", "conversation",
		"--profile-dir", profile,
		"--browser-path", browser,
		"--goos", "darwin",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("messenger open dry-run exit code = %d, stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode messenger open json: %v\n%s", err, stdout.String())
	}
	if payload["target"] != "示例群聊" || payload["mode"] != "conversation" || payload["dryRun"] != true {
		t.Fatalf("messenger open payload = %+v", payload)
	}
	if payload["willSend"] != false || payload["targetVerified"] != false {
		t.Fatalf("messenger open should not send or claim verification: %+v", payload)
	}
}

func TestMessengerOpenApplyFlagsAreAcceptedAndValidatedBeforeBrowserLaunch(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	missingBrowser := filepath.Join(home, "missing-chrome")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{
		"messenger", "open",
		"--to", "示例群聊",
		"--mode", "conversation",
		"--profile-dir", profile,
		"--browser-path", missingBrowser,
		"--goos", "darwin",
		"--apply",
		"--allow-visible-fallback",
		"--timeout-ms", "1000",
		"--json",
	}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("messenger open --apply with missing browser exit code = %d, want 2; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("messenger open did not parse apply flags: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "prerequisites") {
		t.Fatalf("messenger open --apply stderr missing prerequisite failure: %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"messenger", "open",
		"--to", "示例群聊",
		"--mode", "conversation",
		"--profile-dir", profile,
		"--browser-path", missingBrowser,
		"--goos", "darwin",
		"--dry-run",
		"--apply",
		"--json",
	}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Fatalf("messenger open --dry-run --apply code=%d stderr=%q", code, stderr.String())
	}
}

func TestNormalizeRuntimesSupportsAutoAliasesAndValidation(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
		want []string
	}{
		{name: "auto", raw: []string{"auto"}, want: []string{"codex", "claude-code"}},
		{name: "all", raw: []string{"all"}, want: []string{"codex", "claude-code"}},
		{name: "empty", raw: []string{""}, want: []string{"codex", "claude-code"}},
		{name: "claude alias", raw: []string{"claude"}, want: []string{"claude-code"}},
		{name: "claude underscore alias", raw: []string{"claude_code"}, want: []string{"claude-code"}},
		{name: "dedupe", raw: []string{"codex", "codex", "claude"}, want: []string{"codex", "claude-code"}},
		{name: "none", raw: []string{"none"}, want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeRuntimes(test.raw)
			if err != nil {
				t.Fatalf("normalizeRuntimes(%v) error = %v", test.raw, err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("normalizeRuntimes(%v) = %v, want %v", test.raw, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("normalizeRuntimes(%v) = %v, want %v", test.raw, got, test.want)
				}
			}
		})
	}

	if _, err := normalizeRuntimes([]string{"unknown"}); err == nil {
		t.Fatal("normalizeRuntimes accepted unsupported runtime")
	}
}

func TestInstallSkillsWritesEmbeddedCodexSkillsAndPreservesExistingWithoutForce(t *testing.T) {
	home := t.TempDir()
	codexDir := filepath.Join(home, "codex-skills")
	t.Setenv("HOME", home)
	t.Setenv("IXF_TOOLBOX_CODEX_SKILLS_DIR", codexDir)
	t.Setenv("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, "claude-skills"))

	payload, err := installSkills([]string{"codex"}, false)
	if err != nil {
		t.Fatalf("installSkills returned error: %v", err)
	}
	installed := payload["installed"].([]skillResult)
	skipped := payload["skipped"].([]skillResult)
	if len(installed) != len(skillNames) || len(skipped) != 0 {
		t.Fatalf("installed=%d skipped=%d, want installed=%d skipped=0", len(installed), len(skipped), len(skillNames))
	}
	for _, skillName := range skillNames {
		content, err := os.ReadFile(filepath.Join(codexDir, skillName, "SKILL.md"))
		if err != nil {
			t.Fatalf("installed skill %s missing: %v", skillName, err)
		}
		if !strings.Contains(string(content), "name: "+skillName) {
			t.Fatalf("installed skill %s missing frontmatter name", skillName)
		}
	}

	marker := filepath.Join(codexDir, "ixf-docs-reader", "marker.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	payload, err = installSkills([]string{"codex"}, false)
	if err != nil {
		t.Fatalf("second installSkills returned error: %v", err)
	}
	if string(mustReadFile(t, marker)) != "keep" {
		t.Fatal("installSkills overwrote an existing skill without --force")
	}
	skipped = payload["skipped"].([]skillResult)
	if len(skipped) != len(skillNames) || skipped[0].Reason != "exists" {
		t.Fatalf("skipped = %+v, want every skill skipped because it exists", skipped)
	}
}

func TestCollectDiagnosticsReportsGoRuntimeSkillsCookiesAndNoSecrets(t *testing.T) {
	home := t.TempDir()
	cookiesPath := filepath.Join(home, "cookies.json")
	if err := os.WriteFile(
		cookiesPath,
		[]byte(`[{"name":"_csrf_token","value":"dummy-csrf"},{"name":"session","value":"dummy-session"}]`),
		0o644,
	); err != nil {
		t.Fatalf("write cookies: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("IXF_TOOLBOX_CODEX_SKILLS_DIR", filepath.Join(home, "codex-skills"))
	t.Setenv("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, "claude-skills"))
	if _, err := installSkills([]string{"codex"}, false); err != nil {
		t.Fatalf("installSkills returned error: %v", err)
	}

	payload := collectDiagnostics(cookiesPath)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, "dummy-csrf") || strings.Contains(text, "dummy-session") {
		t.Fatalf("diagnostics leaked cookie values: %s", text)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("payload ok = false: %+v", payload)
	}
	if payload["runtime"] != "go" {
		t.Fatalf("runtime = %v, want go", payload["runtime"])
	}
	if _, exists := payload["engines"]; exists {
		t.Fatalf("diagnostics should not report legacy engines: %+v", payload["engines"])
	}
	capabilities := payload["capabilities"].(map[string]bool)
	for _, name := range []string{"docsRead", "docsPublish", "okrRead", "okrWrite", "cookiesExport", "messengerDoctor", "messengerOpenPlan", "messengerOpenApply"} {
		if !capabilities[name] {
			t.Fatalf("capability %s = false", name)
		}
	}
	cookies := payload["cookies"].(map[string]any)
	if cookies["cookieCount"] != 2 || cookies["hasCsrf"] != true {
		t.Fatalf("cookies diagnostics = %+v, want count=2 csrf=true", cookies)
	}
}

func TestCollectDiagnosticsMarksMissingAndInvalidCookieFilesUnhealthy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("IXF_TOOLBOX_CODEX_SKILLS_DIR", filepath.Join(home, "codex-skills"))
	t.Setenv("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, "claude-skills"))

	missing := collectDiagnostics(filepath.Join(home, "missing.json"))
	if ok, _ := missing["ok"].(bool); ok {
		t.Fatalf("missing setup should be unhealthy: %+v", missing)
	}
	missingCookies := missing["cookies"].(map[string]any)
	if missingCookies["exists"] != false || missingCookies["cookieCount"] != 0 {
		t.Fatalf("missing cookies diagnostics = %+v", missingCookies)
	}

	invalidPath := filepath.Join(home, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write invalid cookie file: %v", err)
	}
	invalid := collectDiagnostics(invalidPath)
	invalidCookies := invalid["cookies"].(map[string]any)
	if invalidCookies["exists"] != true || invalidCookies["ok"] != false || invalidCookies["error"] == nil {
		t.Fatalf("invalid cookies diagnostics = %+v", invalidCookies)
	}
}

func TestFormatDiagnosticsIncludesCapabilitiesAndCookieMetadataWithoutCookieNames(t *testing.T) {
	payload := map[string]any{
		"ok":      false,
		"version": version,
		"capabilities": map[string]bool{
			"docsRead":           true,
			"docsPublish":        true,
			"okrRead":            true,
			"okrWrite":           true,
			"cookiesExport":      true,
			"messengerDoctor":    true,
			"messengerOpenPlan":  true,
			"messengerOpenApply": true,
		},
		"skills": map[string]any{
			"codex": map[string]any{
				"ok":        true,
				"dir":       "/tmp/skills",
				"installed": map[string]bool{"ixf-docs-reader": true},
			},
		},
		"cookies": map[string]any{
			"ok":          true,
			"exists":      true,
			"path":        "/tmp/cookies.json",
			"cookieCount": 1,
			"hasCsrf":     true,
			"hasLgwCsrf":  false,
			"cookieNames": []string{"_csrf_token"},
		},
	}
	var stdout bytes.Buffer

	formatDiagnostics(&stdout, payload)

	text := stdout.String()
	for _, expected := range []string{
		"ixf-toolbox " + version,
		"overall fail",
		"native docsRead=true docsPublish=true okrRead=true okrWrite=true cookiesExport=true messengerDoctor=true messengerOpenPlan=true messengerOpenApply=true",
		"skill codex ok=true",
		"cookies ok count=1 csrf=true lgw_csrf=false",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("diagnostics text missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "_csrf_token") {
		t.Fatalf("diagnostics text leaked cookie names:\n%s", text)
	}
}

func TestDoctorCommandJSONAndTextUseGoDiagnostics(t *testing.T) {
	home := t.TempDir()
	cookiesPath := filepath.Join(home, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"dummy-csrf"}]`), 0o644); err != nil {
		t.Fatalf("write cookies: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("IXF_TOOLBOX_CODEX_SKILLS_DIR", filepath.Join(home, "codex-skills"))
	t.Setenv("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, "claude-skills"))
	if _, err := installSkills([]string{"codex"}, false); err != nil {
		t.Fatalf("installSkills returned error: %v", err)
	}

	var jsonOut bytes.Buffer
	var jsonErr bytes.Buffer
	if code := run([]string{"doctor", "--cookies", cookiesPath, "--json"}, &jsonOut, &jsonErr); code != 0 {
		t.Fatalf("doctor --json exit code = %d, stderr=%q", code, jsonErr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("decode doctor json: %v\n%s", err, jsonOut.String())
	}
	if payload["runtime"] != "go" || payload["version"] != version {
		t.Fatalf("doctor json payload = %+v", payload)
	}

	var textOut bytes.Buffer
	var textErr bytes.Buffer
	if code := run([]string{"doctor", "--cookies", cookiesPath}, &textOut, &textErr); code != 0 {
		t.Fatalf("doctor text exit code = %d, stderr=%q", code, textErr.String())
	}
	if !strings.Contains(textOut.String(), "native docsRead=true") {
		t.Fatalf("doctor text missing native capabilities:\n%s", textOut.String())
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}
