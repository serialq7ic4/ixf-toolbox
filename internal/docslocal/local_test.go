package docslocal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadLocalSourcesReadsMarkdownWithoutRemoteSession(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(source, []byte("# Source\n\nHello from local file.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := ReadLocalSources([]string{source})
	if err != nil {
		t.Fatalf("ReadLocalSources returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	got := results[0]
	if got.Source != source || got.Kind != "local_markdown" || got.Title != "source.md" || got.Token != "" {
		t.Fatalf("result metadata = %#v", got)
	}
	if got.Content != "# Source\n\nHello from local file.\n" {
		t.Fatalf("content = %q", got.Content)
	}
	if len(got.Counts) != 0 || len(got.Assets) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("non-empty generated metadata = %#v", got)
	}
}

func TestWriteOutputsUsesSourceStemsAndManifestWithoutTrailingNewline(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	sourceA := filepath.Join(tmpDir, "Project Plan.md")
	sourceB := filepath.Join(tmpDir, "project-plan.md")
	if err := os.WriteFile(sourceA, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceB, []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := ReadLocalSources([]string{sourceA, sourceB})
	if err != nil {
		t.Fatalf("ReadLocalSources returned error: %v", err)
	}

	manifest, err := WriteOutputs(results, outDir)
	if err != nil {
		t.Fatalf("WriteOutputs returned error: %v", err)
	}

	first := manifest["local_markdown_1"].(map[string]any)
	second := manifest["local_markdown_2"].(map[string]any)
	if first["file"] != filepath.Join(outDir, "project-plan.md") {
		t.Fatalf("first file = %#v", first["file"])
	}
	if second["file"] != filepath.Join(outDir, "project-plan-2.md") {
		t.Fatalf("second file = %#v", second["file"])
	}
	content, err := os.ReadFile(filepath.Join(outDir, "project-plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# A\n" {
		t.Fatalf("written markdown = %q", content)
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	actualManifest, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(actualManifest) != string(manifestBytes) {
		t.Fatalf("manifest content = %q, want %q", actualManifest, manifestBytes)
	}
}

func TestCleanupOutputsRemovesOnlyGeneratedManifestPaths(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	asset := filepath.Join(outDir, "assets", "docx_1", "image-001.png")
	markdown := filepath.Join(outDir, "docx-1.md")
	keep := filepath.Join(outDir, "keep.txt")
	if err := os.MkdirAll(filepath.Dir(asset), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asset, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markdown, []byte("# Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"docx_1": map[string]any{
			"file":   markdown,
			"assets": []map[string]any{{"path": "assets/docx_1/image-001.png"}},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CleanupOutputs(outDir); err != nil {
		t.Fatalf("CleanupOutputs returned error: %v", err)
	}

	if _, err := os.Stat(markdown); !os.IsNotExist(err) {
		t.Fatalf("markdown exists after cleanup: %v", err)
	}
	if _, err := os.Stat(asset); !os.IsNotExist(err) {
		t.Fatalf("asset exists after cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest exists after cleanup: %v", err)
	}
	keepBytes, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(keepBytes) != "keep" {
		t.Fatalf("keep file = %q", keepBytes)
	}
}

func TestInspectSourceRedactsRemoteTokensAndAvoidsLocalContentLeakage(t *testing.T) {
	source := filepath.Join(t.TempDir(), "private-source.md")
	content := []byte("# Secret Title\n\nSensitive body should not appear.\n")
	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}

	local, err := InspectSource(source)
	if err != nil {
		t.Fatalf("InspectSource(local) returned error: %v", err)
	}
	remote, err := InspectSource("https://tenant.example.test/docx/doxfixturetoken?from=copy")
	if err != nil {
		t.Fatalf("InspectSource(remote) returned error: %v", err)
	}

	localJSON, err := json.Marshal(local)
	if err != nil {
		t.Fatal(err)
	}
	if local["kind"] != "local_markdown" || local["sizeBytes"] != int64(len(content)) {
		t.Fatalf("local metadata = %#v", local)
	}
	if stringContains(string(localJSON), "Secret Title") || stringContains(string(localJSON), "Sensitive body") {
		t.Fatalf("local inspect leaked content: %s", localJSON)
	}
	want := map[string]any{
		"ok":          true,
		"sourceRef":   "https://tenant.example.test/docx/<redacted>?from=copy",
		"remote":      true,
		"kind":        "docx",
		"host":        "tenant.example.test",
		"pathType":    "docx",
		"tokenPrefix": "dox",
		"tokenLength": len("doxfixturetoken"),
		"route":       "docx_client_vars",
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	remoteJSON, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	if string(remoteJSON) != string(wantJSON) {
		t.Fatalf("remote inspect = %s, want %s", remoteJSON, wantJSON)
	}
}
