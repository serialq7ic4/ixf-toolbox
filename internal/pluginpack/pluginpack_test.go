package pluginpack

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCreatesHostSpecificPackages(t *testing.T) {
	root := newPluginFixture(t, "1.2.3")
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	codex := readJSONMap(t, filepath.Join(root, "plugins/codex/ixf-toolbox/.codex-plugin/plugin.json"))
	if codex["name"] != "ixf-toolbox" || codex["version"] != "1.2.3" || codex["skills"] != "./skills/" {
		t.Fatalf("codex manifest = %#v", codex)
	}
	assertCodexInterface(t, codex)
	if _, exists := codex["hooks"]; exists {
		t.Fatalf("codex manifest must not declare hooks: %#v", codex)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins/codex/ixf-toolbox/hooks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("codex package unexpectedly contains hooks: %v", err)
	}

	claude := readJSONMap(t, filepath.Join(root, "plugins/claude/ixf-toolbox/.claude-plugin/plugin.json"))
	if claude["name"] != "ixf-toolbox" || claude["version"] != "1.2.3" {
		t.Fatalf("claude manifest = %#v", claude)
	}
	for _, key := range []string{"hooks", "skills", "interface"} {
		if _, exists := claude[key]; exists {
			t.Fatalf("Claude manifest must rely on root discovery and omit %q: %#v", key, claude)
		}
	}
	assertSameFile(t,
		filepath.Join(root, "skills/using-ixf-toolbox/SKILL.md"),
		filepath.Join(root, "plugins/codex/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md"),
	)
	assertSameFile(t,
		filepath.Join(root, "skills/using-ixf-toolbox/SKILL.md"),
		filepath.Join(root, "plugins/claude/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md"),
	)
}

func TestGeneratedMarketplacesPointAtHostPackages(t *testing.T) {
	root := newPluginFixture(t, "1.2.3")
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	codexMarketplace := readJSONMap(t, filepath.Join(root, ".agents/plugins/marketplace.json"))
	claudeMarketplace := readJSONMap(t, filepath.Join(root, ".claude-plugin/marketplace.json"))
	if codexMarketplace["name"] != "ixf-toolbox" || claudeMarketplace["name"] != "ixf-toolbox" {
		t.Fatalf("marketplace names = %#v, %#v", codexMarketplace["name"], claudeMarketplace["name"])
	}
	codexInterface, ok := codexMarketplace["interface"].(map[string]any)
	if !ok || codexInterface["displayName"] != "i讯飞 Toolbox" {
		t.Fatalf("Codex marketplace interface = %#v", codexMarketplace["interface"])
	}
	codexEntry := codexMarketplace["plugins"].([]any)[0].(map[string]any)
	codexSource := codexEntry["source"].(map[string]any)
	if codexSource["source"] != "local" || codexSource["path"] != "./plugins/codex/ixf-toolbox" {
		t.Fatalf("Codex source = %#v", codexEntry["source"])
	}
	if codexEntry["policy"].(map[string]any)["installation"] != "AVAILABLE" ||
		codexEntry["policy"].(map[string]any)["authentication"] != "ON_USE" {
		t.Fatalf("Codex policy = %#v", codexEntry["policy"])
	}
	if codexEntry["category"] != "Productivity" {
		t.Fatalf("Codex marketplace metadata = %#v", codexEntry)
	}
	claudeEntry := claudeMarketplace["plugins"].([]any)[0].(map[string]any)
	if claudeEntry["source"] != "./plugins/claude/ixf-toolbox" {
		t.Fatalf("Claude source = %#v", claudeEntry["source"])
	}
	if claudeEntry["category"] != "Productivity" {
		t.Fatalf("Claude marketplace metadata = %#v", claudeEntry)
	}
}

func TestCheckRejectsGeneratedPackageDrift(t *testing.T) {
	root := newPluginFixture(t, "1.2.3")
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "plugins/codex/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md")
	if err := os.WriteFile(path, []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Generate(Options{Root: root, Check: true}); err == nil || !strings.Contains(err.Error(), "generated plugin packages are stale") {
		t.Fatalf("check error = %v", err)
	}
}

func newPluginFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := `{"name":"ixf-toolbox","description":"fixture","author":{"name":"fixture"},"homepage":"https://example.test/ixf-toolbox","repository":"https://example.test/ixf-toolbox","license":"Apache-2.0","keywords":["ixf"],"interface":{"displayName":"i讯飞 Toolbox","shortDescription":"Authorized i讯飞 workflows","longDescription":"Use authorized i讯飞 and LarkShell workflows through the Go ixf runtime.","developerName":"fixture","category":"Productivity","capabilities":["Read","Write"],"defaultPrompt":["Read an i讯飞 document","Plan an approved document update"]}}`
	if err := os.MkdirAll(filepath.Join(root, "plugin-src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin-src", "metadata.json"), []byte(metadata+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range canonicalSkillNames {
		dir := filepath.Join(root, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nfixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return payload
}

func assertSameFile(t *testing.T, wantPath, gotPath string) {
	t.Helper()
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("%s and %s differ", wantPath, gotPath)
	}
}

func assertCodexInterface(t *testing.T, manifest map[string]any) {
	t.Helper()
	interfaceValue, ok := manifest["interface"].(map[string]any)
	if !ok {
		t.Fatalf("Codex manifest has no interface object: %#v", manifest)
	}
	for _, key := range []string{
		"displayName", "shortDescription", "longDescription", "developerName",
		"category", "capabilities", "defaultPrompt",
	} {
		if _, ok := interfaceValue[key]; !ok {
			t.Fatalf("Codex interface missing %q: %#v", key, interfaceValue)
		}
	}
}
