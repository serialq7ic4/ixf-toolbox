package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ixfbitable "github.com/serialq7ic4/ixf-toolbox/internal/bitable"
	"github.com/serialq7ic4/ixf-toolbox/internal/docspublish"
	ixfupdate "github.com/serialq7ic4/ixf-toolbox/internal/update"
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

func TestDefaultVersionMatchesVersionFile(t *testing.T) {
	content, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(content))
	if version != want {
		t.Fatalf("default version = %q, want VERSION file %q", version, want)
	}
}

func TestRootHelpListsCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"usage: ixf", "docs", "sheets", "bitable", "okr", "messenger", "update"} {
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
		{args: []string{"docs", "--help"}, expected: []string{"usage: ixf docs", "read", "publish", "update", "patch", "structure", "inspect"}},
		{args: []string{"sheets", "--help"}, expected: []string{"usage: ixf sheets", "read", "update"}},
		{args: []string{"bitable", "--help"}, expected: []string{"usage: ixf bitable", "inspect", "read", "attach", "apply"}},
		{args: []string{"okr", "--help"}, expected: []string{"usage: ixf okr", "read", "write"}},
		{args: []string{"messenger", "--help"}, expected: []string{"usage: ixf messenger", "doctor", "open", "read", "send"}},
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

func TestLeafCommandHelpExitsZeroAndPrintsToStdout(t *testing.T) {
	tests := []struct {
		args     []string
		expected []string
	}{
		{
			args:     []string{"docs", "read", "--help"},
			expected: []string{"usage: ixf docs read", "--out-dir", "--print-manifest", "--expand-sheets", "--cookies"},
		},
		{
			args:     []string{"docs", "structure", "--help"},
			expected: []string{"usage: ixf docs structure", "--json", "--cookies", "--space-api"},
		},
		{
			args:     []string{"docs", "publish", "--help"},
			expected: []string{"usage: ixf docs publish", "--base-url", "IXF_DOCS_DEFAULT_BASE_URL", "--dry-run", "--apply"},
		},
		{
			args:     []string{"docs", "update", "--help"},
			expected: []string{"usage: ixf docs update", "--url", "--dry-run", "--apply", "--allow-complex-replace"},
		},
		{
			args:     []string{"docs", "patch", "insert", "--help"},
			expected: []string{"usage: ixf docs patch insert", "--url", "--under-heading", "--position", "--dry-run", "--apply"},
		},
		{
			args:     []string{"docs", "patch", "replace-section", "--help"},
			expected: []string{"usage: ixf docs patch replace-section", "--url", "--under-heading", "--allow-complex-section-replace", "--dry-run", "--apply"},
		},
		{
			args:     []string{"docs", "patch", "delete-section", "--help"},
			expected: []string{"usage: ixf docs patch delete-section", "--url", "--under-heading", "--allow-complex-section-replace", "--dry-run", "--apply"},
		},
		{
			args:     []string{"docs", "table", "append-row", "--help"},
			expected: []string{"Usage of ixf docs table append-row", "-url", "-input", "-table-index", "-dry-run", "-apply", "-json"},
		},
		{
			args:     []string{"sheets", "read", "--help"},
			expected: []string{"usage: ixf sheets read", "--cookies", "--space-api"},
		},
		{
			args:     []string{"sheets", "update", "--help"},
			expected: []string{"Usage of ixf sheets update", "-url", "-host-url", "-range", "-input", "-cookies", "-space-api", "-dry-run", "-apply"},
		},
		{
			args:     []string{"bitable", "inspect", "--help"},
			expected: []string{"Usage of ixf bitable inspect", "-url", "-cookies", "-space-api", "-json"},
		},
		{
			args:     []string{"bitable", "attach", "--help"},
			expected: []string{"Usage of ixf bitable attach", "-url", "-field", "-record-match", "-file", "-dry-run", "-apply", "Upload and bind the attachment", "-json"},
		},
		{
			args:     []string{"bitable", "record", "create", "--help"},
			expected: []string{"Usage of ixf bitable record create", "-url", "-input", "-insert-position", "top|bottom", "default bottom", "-dry-run", "-apply", "-json"},
		},
		{
			args:     []string{"okr", "read", "--help"},
			expected: []string{"usage: ixf okr read", "--cookies", "--csrf-url"},
		},
		{
			args:     []string{"messenger", "send", "--help"},
			expected: []string{"Usage of ixf messenger send", "-to", "-message", "-dry-run", "-apply"},
		},
		{
			args:     []string{"update", "self", "--help"},
			expected: []string{"usage: ixf update self", "--target-path", "--apply", "--json"},
		},
	}
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		code := run(test.args, &stdout, &stderr)

		if code != 0 {
			t.Fatalf("run(%v) exit code = %d, want 0; stderr=%q stdout=%q", test.args, code, stderr.String(), stdout.String())
		}
		if stderr.String() != "" {
			t.Fatalf("run(%v) stderr = %q, want empty", test.args, stderr.String())
		}
		for _, expected := range test.expected {
			if !strings.Contains(stdout.String(), expected) {
				t.Fatalf("run(%v) stdout missing %q:\n%s", test.args, expected, stdout.String())
			}
		}
	}
}

func TestDocsTableAppendRowDryRunJSONRoutesFlags(t *testing.T) {
	original := docsTableAppendRow
	var captured docspublish.TableAppendRowConfig
	docsTableAppendRow = func(config docspublish.TableAppendRowConfig) (map[string]any, error) {
		captured = config
		return map[string]any{
			"ok":        true,
			"dryRun":    true,
			"operation": "docs_table_append_row",
			"willWrite": false,
		}, nil
	}
	t.Cleanup(func() {
		docsTableAppendRow = original
	})

	input := filepath.Join(t.TempDir(), "row.json")
	if err := os.WriteFile(input, []byte(`{"fields":{"Title":"New docs row"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLITest(t,
		"docs", "table", "append-row",
		"--url", "https://tenant.example/docx/page_1",
		"--input", input,
		"--table-index", "2",
		"--require", "New docs row",
		"--dry-run",
		"--json",
	)
	if code != 0 {
		t.Fatalf("docs table append-row dry-run exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["ok"] != true || payload["operation"] != "docs_table_append_row" {
		t.Fatalf("payload = %+v", payload)
	}
	if captured.URL != "https://tenant.example/docx/page_1" ||
		captured.InputPath != input ||
		captured.TableIndex != 2 ||
		len(captured.RequiredText) != 1 ||
		captured.RequiredText[0] != "New docs row" ||
		!captured.DryRun ||
		captured.Apply {
		t.Fatalf("captured config = %+v", captured)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestBitableAttachDryRunJSONRoutesFlags(t *testing.T) {
	original := bitableAttach
	var captured ixfbitable.AttachConfig
	bitableAttach = func(config ixfbitable.AttachConfig) (map[string]any, error) {
		captured = config
		return map[string]any{
			"ok":               true,
			"dryRun":           true,
			"operation":        "bitable_attach",
			"willUpload":       true,
			"willUpdateRecord": true,
		}, nil
	}
	t.Cleanup(func() {
		bitableAttach = original
	})

	stdout, stderr, code := runCLITest(t,
		"bitable", "attach",
		"--url", "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		"--field", "Screenshot",
		"--record-match", "Title=Image bug",
		"--file", "/tmp/ceph_logo.jpeg",
		"--dry-run",
		"--json",
	)
	if code != 0 {
		t.Fatalf("bitable attach dry-run exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["ok"] != true || payload["operation"] != "bitable_attach" {
		t.Fatalf("payload = %+v", payload)
	}
	if captured.URL != "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid" ||
		captured.Field != "Screenshot" ||
		captured.RecordMatch != "Title=Image bug" ||
		captured.FilePath != "/tmp/ceph_logo.jpeg" ||
		!captured.DryRun ||
		captured.Apply {
		t.Fatalf("captured config = %+v", captured)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestBitableRecordCreateDryRunJSONRoutesFlags(t *testing.T) {
	original := bitableRecordCreate
	var captured ixfbitable.RecordCreateConfig
	bitableRecordCreate = func(config ixfbitable.RecordCreateConfig) (map[string]any, error) {
		captured = config
		return map[string]any{
			"ok":               true,
			"dryRun":           true,
			"operation":        "bitable_record_create",
			"willCreateRecord": true,
		}, nil
	}
	t.Cleanup(func() {
		bitableRecordCreate = original
	})

	input := filepath.Join(t.TempDir(), "row.json")
	if err := os.WriteFile(input, []byte(`{"fields":{"Title":"New image bug"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLITest(t,
		"bitable", "record", "create",
		"--url", "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		"--input", input,
		"--insert-position", "top",
		"--dry-run",
		"--json",
	)
	if code != 0 {
		t.Fatalf("bitable record create dry-run exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["ok"] != true || payload["operation"] != "bitable_record_create" {
		t.Fatalf("payload = %+v", payload)
	}
	if captured.URL != "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid" ||
		captured.InputPath != input ||
		captured.InsertPosition != "top" ||
		!captured.DryRun ||
		captured.Apply {
		t.Fatalf("captured config = %+v", captured)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestDocsPublishUsesDefaultBaseURLFromEnvironment(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "note.md")
	if err := os.WriteFile(source, []byte("# Default Target\n\nBody.\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("IXF_DOCS_DEFAULT_BASE_URL", "https://tenant.example.test")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"docs", "publish", source, "--dry-run"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("docs publish exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode publish dry-run json: %v\n%s", err, stdout.String())
	}
	if payload["baseURLSource"] != "env:IXF_DOCS_DEFAULT_BASE_URL" {
		t.Fatalf("baseURLSource = %v, want env source; payload=%+v", payload["baseURLSource"], payload)
	}
	if payload["targetHost"] != "tenant.example.test" {
		t.Fatalf("targetHost = %v, want tenant.example.test; payload=%+v", payload["targetHost"], payload)
	}
}

func TestDocsPublishUsesDefaultBaseURLFromConfig(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "note.md")
	if err := os.WriteFile(source, []byte("# Config Target\n\nBody.\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	configDir := filepath.Join(home, ".config", "ixf-toolbox")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"docs":{"defaultBaseURL":"https://configured.example.test"}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("IXF_DOCS_DEFAULT_BASE_URL", "")
	t.Setenv("IXF_DEFAULT_BASE_URL", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"docs", "publish", source, "--dry-run"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("docs publish exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode publish dry-run json: %v\n%s", err, stdout.String())
	}
	if payload["baseURLSource"] != "config:docs.defaultBaseURL" {
		t.Fatalf("baseURLSource = %v, want config source; payload=%+v", payload["baseURLSource"], payload)
	}
	if payload["targetHost"] != "configured.example.test" {
		t.Fatalf("targetHost = %v, want configured.example.test; payload=%+v", payload["targetHost"], payload)
	}
}

func TestDocsPublishExplicitBaseURLOverridesDefault(t *testing.T) {
	t.Setenv("IXF_DOCS_DEFAULT_BASE_URL", "https://default.example.test")

	parsed, err := parseDocsPublishArgs([]string{"note.md", "--base-url", "https://explicit.example.test"})
	if err != nil {
		t.Fatalf("parse docs publish args: %v", err)
	}
	if parsed.baseURL != "https://explicit.example.test" || parsed.baseURLSource != "flag" {
		t.Fatalf("parsed baseURL=%q source=%q, want explicit flag", parsed.baseURL, parsed.baseURLSource)
	}
}

func TestParseDocsPatchInsertArgs(t *testing.T) {
	parsed, err := parseDocsPatchInsertArgs([]string{
		"table.md",
		"--url", "https://tenant.example.test/wiki/example",
		"--under-heading", "1.1 账号全集群初始化",
		"--position", "section-end",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.markdown != "table.md" || parsed.url != "https://tenant.example.test/wiki/example" ||
		parsed.underHeading != "1.1 账号全集群初始化" || parsed.position != "section-end" || !parsed.dryRun {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseDocsPatchSectionArgs(t *testing.T) {
	replacement, err := parseDocsPatchSectionArgs([]string{
		"section.md",
		"--url", "https://tenant.example.test/docx/doxExample",
		"--under-heading", "1.1 Target",
		"--allow-complex-section-replace",
		"--dry-run",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.markdown != "section.md" || replacement.url != "https://tenant.example.test/docx/doxExample" ||
		replacement.underHeading != "1.1 Target" || !replacement.allowComplex || !replacement.dryRun || replacement.deleteOnly {
		t.Fatalf("replacement = %#v", replacement)
	}

	deletion, err := parseDocsPatchSectionArgs([]string{
		"--url", "https://tenant.example.test/wiki/example",
		"--under-heading", "Obsolete",
		"--apply",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if deletion.markdown != "" || deletion.url != "https://tenant.example.test/wiki/example" ||
		deletion.underHeading != "Obsolete" || !deletion.apply || !deletion.deleteOnly {
		t.Fatalf("deletion = %#v", deletion)
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
	messengerPayload := payload["messenger"].(map[string]any)
	stability, ok := messengerPayload["stability"].(map[string]any)
	if !ok {
		t.Fatalf("messenger stability = %#v, want map", messengerPayload["stability"])
	}
	if stability["operatingModel"] != "local-browser-automation" || stability["macOS"] != "tier1" || stability["windows"] != "experimental" {
		t.Fatalf("messenger stability metadata = %+v", stability)
	}
	criteria, ok := stability["sendSuccessCriteria"].([]any)
	if !ok || len(criteria) != 4 {
		t.Fatalf("sendSuccessCriteria = %#v, want four criteria", stability["sendSuccessCriteria"])
	}
	for _, expected := range []string{"targetVerified:true", "sent:true", "localEchoMatched:true", "verifiedPresent:true"} {
		if !containsAnyString(criteria, expected) {
			t.Fatalf("sendSuccessCriteria missing %q: %+v", expected, criteria)
		}
	}
}

func TestMessengerDoctorTextPrintsRemediation(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	missingBrowser := filepath.Join(home, "missing-chrome")
	missingCookies := filepath.Join(home, "missing-cookies.json")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{
		"messenger", "doctor",
		"--profile-dir", profile,
		"--browser-path", missingBrowser,
		"--cookies", missingCookies,
		"--goos", "darwin",
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("messenger doctor text exit code = %d, want 1; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	for _, expected := range []string{
		"overall fail",
		"stability operating_model=local-browser-automation macos=tier1 windows=experimental",
		"remediation Install Google Chrome or Chromium",
		"remediation Run ixf cookies export",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("messenger doctor text missing %q:\n%s", expected, stdout.String())
		}
	}
	if count := strings.Count(stdout.String(), "remediation Install Google Chrome or Chromium"); count != 1 {
		t.Fatalf("browser remediation count = %d, want 1:\n%s", count, stdout.String())
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

func TestMessengerReadDryRunValidatesArgumentsAndPrintsPlan(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"messenger", "read", "--scope", "unknown", "--dry-run", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid scope exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--scope must be unread or recent") {
		t.Fatalf("invalid scope stderr = %q", stderr.String())
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
		"messenger", "read",
		"--scope", "unread",
		"--limit", "2",
		"--messages-per-chat", "3",
		"--profile-dir", profile,
		"--browser-path", browser,
		"--goos", "darwin",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("messenger read dry-run exit code = %d, stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode messenger read json: %v\n%s", err, stdout.String())
	}
	if payload["action"] != "read" || payload["scope"] != "unread" || payload["dryRun"] != true {
		t.Fatalf("messenger read payload = %+v", payload)
	}
	if payload["willSend"] != false || payload["browserLaunch"] != false {
		t.Fatalf("messenger read should not send or launch in dry-run: %+v", payload)
	}
}

func TestMessengerReadApplyFlagsAreAcceptedAndValidatedBeforeBrowserLaunch(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	missingBrowser := filepath.Join(home, "missing-chrome")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{
		"messenger", "read",
		"--scope", "recent",
		"--profile-dir", profile,
		"--browser-path", missingBrowser,
		"--goos", "darwin",
		"--apply",
		"--allow-visible-fallback",
		"--timeout-ms", "1000",
		"--json",
	}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("messenger read --apply with missing browser exit code = %d, want 2; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("messenger read did not parse apply flags: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "prerequisites") {
		t.Fatalf("messenger read --apply stderr missing prerequisite failure: %q", stderr.String())
	}
}

func TestMessengerSendDryRunValidatesArgumentsAndPrintsPlanWithoutEchoingMessage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"messenger", "send", "--mode", "conversation", "--message", "secret body", "--dry-run", "--json"}, &stdout, &stderr); code != 2 {
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
		"messenger", "send",
		"--to", "示例群聊",
		"--mode", "conversation",
		"--message", "secret body",
		"--profile-dir", profile,
		"--browser-path", browser,
		"--goos", "darwin",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("messenger send dry-run exit code = %d, stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "secret body") {
		t.Fatalf("messenger send dry-run echoed message body: %s", stdout.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode messenger send json: %v\n%s", err, stdout.String())
	}
	if payload["action"] != "send" || payload["target"] != "示例群聊" || payload["mode"] != "conversation" || payload["dryRun"] != true {
		t.Fatalf("messenger send payload = %+v", payload)
	}
	if payload["willSend"] != true || payload["sent"] != false || payload["verifiedPresent"] != false {
		t.Fatalf("messenger send should plan send without sending: %+v", payload)
	}
}

func TestMessengerSendApplyFlagsAreAcceptedAndValidatedBeforeBrowserLaunch(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	missingBrowser := filepath.Join(home, "missing-chrome")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{
		"messenger", "send",
		"--to", "示例群聊",
		"--mode", "conversation",
		"--message", "secret body",
		"--profile-dir", profile,
		"--browser-path", missingBrowser,
		"--goos", "darwin",
		"--apply",
		"--allow-visible-fallback",
		"--timeout-ms", "1000",
		"--json",
	}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("messenger send --apply with missing browser exit code = %d, want 2; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("messenger send did not parse apply flags: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "prerequisites") {
		t.Fatalf("messenger send --apply stderr missing prerequisite failure: %q", stderr.String())
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
	docsWriterContent := string(mustReadFile(t, filepath.Join(codexDir, "ixf-docs-writer", "SKILL.md")))
	for _, expected := range []string{
		"Do not treat top-level `doctor.ok=false` alone as an authentication failure",
		"Inspect `.cookies.ok` and `.capabilities.docsPublish`",
		"derive the tenant base URL from the user's i讯飞 link",
	} {
		if !strings.Contains(docsWriterContent, expected) {
			t.Fatalf("ixf-docs-writer skill missing publish guard %q", expected)
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
	stubDependencyRelease(t, version)
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
	emptyBin := filepath.Join(home, "empty-bin")
	if err := os.MkdirAll(emptyBin, 0o755); err != nil {
		t.Fatalf("mkdir empty bin: %v", err)
	}
	t.Setenv("PATH", emptyBin)
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
	if legacy, ok := payload["legacyCommands"].([]map[string]string); !ok || len(legacy) != 0 {
		t.Fatalf("legacy commands should be absent by default: %+v", payload["legacyCommands"])
	}
	capabilities := payload["capabilities"].(map[string]bool)
	for _, name := range []string{"docsRead", "docsPublish", "sheetsRead", "sheetsUpdateDryRun", "sheetsUpdateApply", "okrRead", "okrWrite", "cookiesExport", "messengerDoctor", "messengerOpenPlan", "messengerOpenApply", "messengerReadPlan", "messengerReadApply", "messengerSendPlan", "messengerSendApply"} {
		if !capabilities[name] {
			t.Fatalf("capability %s = false", name)
		}
	}
	cookies := payload["cookies"].(map[string]any)
	if cookies["cookieCount"] != 2 || cookies["hasCsrf"] != true {
		t.Fatalf("cookies diagnostics = %+v, want count=2 csrf=true", cookies)
	}
	docs := payload["docs"].(map[string]any)
	defaultBaseURL := docs["defaultBaseURL"].(map[string]any)
	if defaultBaseURL["configured"] != false || defaultBaseURL["configPath"] == "" {
		t.Fatalf("docs default base URL diagnostics = %+v", defaultBaseURL)
	}
	dependencies := payload["dependencies"].(map[string]any)
	if dependencies["ok"] != false {
		t.Fatalf("dependencies ok = %#v, want false because optional full-function deps are absent", dependencies["ok"])
	}
	mermaid := dependencies["mermaid"].(map[string]any)
	if mermaid["ok"] != false || mermaid["available"] != false || mermaid["installable"] != true {
		t.Fatalf("mermaid dependency = %+v, want missing but installable", mermaid)
	}
	if mermaid["renderer"] != "mmdc" || mermaid["requiredFor"] != "docs mermaid image rendering" {
		t.Fatalf("mermaid dependency metadata = %+v", mermaid)
	}
	messengerDep := dependencies["messenger"].(map[string]any)
	if messengerDep["ok"] != false {
		t.Fatalf("messenger dependency = %+v, want false in empty temp home", messengerDep)
	}
	updateDep := dependencies["update"].(map[string]any)
	if updateDep["ok"] != true || updateDep["currentVersion"] != version || updateDep["latestVersion"] != version || updateDep["updateAvailable"] != false {
		t.Fatalf("update dependency = %+v, want reachable current release", updateDep)
	}
}

func TestCollectDiagnosticsReportsAgentRoutingContract(t *testing.T) {
	stubDependencyRelease(t, version)
	payload := collectDiagnostics(filepath.Join(t.TempDir(), "missing-cookies.json"))

	routing, ok := payload["agentRouting"].(map[string]any)
	if !ok {
		t.Fatalf("agentRouting = %#v, want map", payload["agentRouting"])
	}
	if routing["goOnly"] != true || routing["backgroundRouting"] != true {
		t.Fatalf("agent routing contract = %+v, want go-only background routing", routing)
	}
	if routing["defaultAmbiguousIntent"] != "read-only" {
		t.Fatalf("defaultAmbiguousIntent = %v, want read-only", routing["defaultAmbiguousIntent"])
	}
	guidance, ok := routing["currentGuidance"].([]string)
	if !ok {
		t.Fatalf("currentGuidance = %#v, want []string", routing["currentGuidance"])
	}
	for _, expected := range []string{"AGENTS.md", "docs/agent-routing.md", "skills/*/*/SKILL.md"} {
		if !containsString(guidance, expected) {
			t.Fatalf("currentGuidance missing %q: %+v", expected, guidance)
		}
	}

	var stdout bytes.Buffer
	formatDiagnostics(&stdout, map[string]any{
		"ok":           false,
		"version":      version,
		"capabilities": map[string]bool{},
		"agentRouting": routing,
	})
	if !strings.Contains(stdout.String(), "agent_routing go_only=true background=true default=read-only") {
		t.Fatalf("diagnostics text missing agent routing line:\n%s", stdout.String())
	}
}

func TestCollectDiagnosticsReportsLegacyCommandShimsAsIgnored(t *testing.T) {
	stubDependencyRelease(t, version)
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	cookiesPath := filepath.Join(home, "cookies.json")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	for _, name := range []string{"ixfdoc", "ixfwrite"} {
		if err := writeLegacyCommandShim(bin, name); err != nil {
			t.Fatalf("write legacy shim %s: %v", name, err)
		}
	}
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"dummy-csrf"}]`), 0o644); err != nil {
		t.Fatalf("write cookies: %v", err)
	}
	t.Setenv("PATH", bin)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	}
	t.Setenv("HOME", home)
	t.Setenv("IXF_TOOLBOX_CODEX_SKILLS_DIR", filepath.Join(home, "codex-skills"))
	t.Setenv("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, "claude-skills"))
	if _, err := installSkills([]string{"codex"}, false); err != nil {
		t.Fatalf("installSkills returned error: %v", err)
	}

	payload := collectDiagnostics(cookiesPath)
	legacy, ok := payload["legacyCommands"].([]map[string]string)
	if !ok || len(legacy) != 2 {
		t.Fatalf("legacyCommands = %#v, want two ignored commands", payload["legacyCommands"])
	}
	for _, item := range legacy {
		if item["status"] != "ignored" || item["runtime"] != "go-only" {
			t.Fatalf("legacy command should be marked ignored/go-only: %+v", item)
		}
	}

	var stdout bytes.Buffer
	formatDiagnostics(&stdout, payload)
	for _, expected := range []string{
		"legacy ixfdoc ignored",
		"legacy ixfwrite ignored",
		"skills use ixf only",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("doctor text missing %q:\n%s", expected, stdout.String())
		}
	}
}

func writeLegacyCommandShim(bin string, name string) error {
	filename := name
	content := []byte("#!/bin/sh\nexit 0\n")
	if runtime.GOOS == "windows" {
		filename += ".exe"
		content = []byte("legacy shim placeholder")
	}
	return os.WriteFile(filepath.Join(bin, filename), content, 0o755)
}

func TestCollectDiagnosticsMarksMissingAndInvalidCookieFilesUnhealthy(t *testing.T) {
	stubDependencyRelease(t, version)
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
			"sheetsRead":         true,
			"sheetsUpdateDryRun": true,
			"sheetsUpdateApply":  true,
			"okrRead":            true,
			"okrWrite":           true,
			"cookiesExport":      true,
			"messengerDoctor":    true,
			"messengerOpenPlan":  true,
			"messengerOpenApply": true,
			"messengerReadPlan":  true,
			"messengerReadApply": true,
			"messengerSendPlan":  true,
			"messengerSendApply": true,
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
		"agentRouting": map[string]any{
			"goOnly":                 true,
			"backgroundRouting":      true,
			"defaultAmbiguousIntent": "read-only",
		},
		"dependencies": map[string]any{
			"ok": false,
			"mermaid": map[string]any{
				"ok":        false,
				"available": false,
				"ready":     false,
			},
			"messenger": map[string]any{
				"ok": false,
			},
			"update": map[string]any{
				"ok": true,
			},
		},
	}
	var stdout bytes.Buffer

	formatDiagnostics(&stdout, payload)

	text := stdout.String()
	for _, expected := range []string{
		"ixf-toolbox " + version,
		"overall fail",
		"native docsRead=true docsPublish=true sheetsRead=true sheetsUpdateDryRun=true sheetsUpdateApply=true okrRead=true okrWrite=true cookiesExport=true messengerDoctor=true messengerOpenPlan=true messengerOpenApply=true messengerReadPlan=true messengerReadApply=true messengerSendPlan=true messengerSendApply=true",
		"skill codex ok=true",
		"cookies ok count=1 csrf=true lgw_csrf=false",
		"dependencies ok=false mermaid=false messenger=false update=true",
		"agent_routing go_only=true background=true default=read-only",
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
	stubDependencyRelease(t, version)
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
	if _, ok := payload["dependencies"].(map[string]any); !ok {
		t.Fatalf("doctor json missing dependencies: %+v", payload)
	}

	var textOut bytes.Buffer
	var textErr bytes.Buffer
	if code := run([]string{"doctor", "--cookies", cookiesPath}, &textOut, &textErr); code != 0 {
		t.Fatalf("doctor text exit code = %d, stderr=%q", code, textErr.String())
	}
	if !strings.Contains(textOut.String(), "native docsRead=true") {
		t.Fatalf("doctor text missing native capabilities:\n%s", textOut.String())
	}
	if !strings.Contains(textOut.String(), "docs_default_base_url configured=false") {
		t.Fatalf("doctor text missing docs default base URL diagnostics:\n%s", textOut.String())
	}
	if !strings.Contains(textOut.String(), "dependencies ok=") {
		t.Fatalf("doctor text missing dependency diagnostics:\n%s", textOut.String())
	}
}

func TestSetupDepsDryRunPlansMermaidDependencyInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	stubDependencyRelease(t, version)
	home := t.TempDir()
	emptyBin := filepath.Join(home, "empty-bin")
	if err := os.MkdirAll(emptyBin, 0o755); err != nil {
		t.Fatalf("mkdir empty bin: %v", err)
	}
	t.Setenv("PATH", emptyBin)

	stdout, stderr, code := runCLITest(t, "setup", "deps", "--json")
	if code != 0 {
		t.Fatalf("setup deps dry-run exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["dryRun"] != true || payload["apply"] != false {
		t.Fatalf("setup deps payload = %+v, want dry-run", payload)
	}
	commands := payload["commands"].([]any)
	text := strings.Join(anyStrings(commands), "\n")
	for _, expected := range []string{"npm install -g @mermaid-js/mermaid-cli", "npx puppeteer browsers install chrome-headless-shell"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("setup deps commands missing %q: %+v", expected, commands)
		}
	}
}

func TestSetupDepsApplyInstallsMermaidToolchainWithExplicitApply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	stubDependencyRelease(t, version)
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeSetupDepsFixtureCommand(t, bin, "npm", "npm-ran", bin)
	writeSetupDepsFixtureCommand(t, bin, "npx", "npx-ran", bin)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/bin"+string(os.PathListSeparator)+"/usr/bin")

	stdout, stderr, code := runCLITest(t, "setup", "deps", "--apply", "--json")
	if code != 0 {
		t.Fatalf("setup deps apply exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["dryRun"] != false || payload["apply"] != true || payload["applied"] != true {
		t.Fatalf("setup deps payload = %+v, want applied", payload)
	}
	assertFileText(t, filepath.Join(bin, "npm-ran"), "ran\n")
	assertFileText(t, filepath.Join(bin, "npx-ran"), "ran\n")
	dependencies := payload["dependencies"].(map[string]any)
	mermaid := dependencies["mermaid"].(map[string]any)
	if mermaid["ok"] != true || mermaid["ready"] != true {
		t.Fatalf("mermaid dependency after setup = %+v, want ready", mermaid)
	}
}

func stubDependencyRelease(t *testing.T, latest string) {
	t.Helper()
	original := dependencyReleaseLoader
	dependencyReleaseLoader = func(repo string, releaseFile string) (ixfupdate.Release, error) {
		return ixfupdate.Release{
			TagName: "v" + latest,
			HTMLURL: "https://github.example/releases/v" + latest,
			Assets:  []ixfupdate.Asset{},
		}, nil
	}
	t.Cleanup(func() {
		dependencyReleaseLoader = original
	})
}

func writeSetupDepsFixtureCommand(t *testing.T, dir string, name string, marker string, bin string) {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" +
		"printf 'ran\\n' > " + shellQuote(filepath.Join(dir, marker)) + "\n" +
		"cat > " + shellQuote(filepath.Join(bin, "mmdc")) + " <<'EOS'\n" +
		"#!/bin/sh\n" +
		"out=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    -o) out=\"$2\"; shift 2 ;;\n" +
		"    *) shift ;;\n" +
		"  esac\n" +
		"done\n" +
		"printf '<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 10 10\"></svg>' > \"$out\"\n" +
		"EOS\n" +
		"chmod +x " + shellQuote(filepath.Join(bin, "mmdc")) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write %s fixture: %v", name, err)
	}
}

func anyStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAnyString(values []any, target string) bool {
	for _, value := range values {
		if text, ok := value.(string); ok && text == target {
			return true
		}
	}
	return false
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}
