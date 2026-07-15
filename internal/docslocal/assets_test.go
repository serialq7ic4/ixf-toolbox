package docslocal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/serialq7ic4/ixf-toolbox/internal/docx"
)

func TestImageAssetWriterDownloadsPNGToSafeRelativePath(t *testing.T) {
	var requestedPath string
	var csrfHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.String()
		csrfHeader = r.Header.Get("X-CSRFToken")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer server.Close()
	writer := imageWriterFixture(server.URL, t.TempDir())

	resolution := writer.resolve(imageReferenceFixture("boxr-fixture-token"))

	if resolution.MarkdownPath != "assets/docx_1/image-001.png" || resolution.AltText != "Architecture diagram" {
		t.Fatalf("resolution = %#v", resolution)
	}
	if resolution.Warning != "" {
		t.Fatalf("warning = %q", resolution.Warning)
	}
	asset := resolution.Asset
	if asset["path"] != "assets/docx_1/image-001.png" || asset["mimeType"] != "image/png" ||
		asset["width"] != 1200 || asset["height"] != 800 || asset["sizeBytes"] != len(pngBytes) ||
		asset["status"] != "downloaded" || asset["ordinal"] != 1 {
		t.Fatalf("asset = %#v", asset)
	}
	assetPath := filepath.Join(writer.outputRoot, "assets", "docx_1", "image-001.png")
	if content, err := os.ReadFile(assetPath); err != nil || string(content) != string(pngBytes) {
		t.Fatalf("asset content = %q, err = %v", content, err)
	}
	if requestedPath != "/space/api/box/stream/download/all/boxr-fixture-token/?mount_node_token=dox-fixture-document&mount_point=docx_image" {
		t.Fatalf("requested path = %q", requestedPath)
	}
	if csrfHeader != "fixture-csrf" {
		t.Fatalf("csrf header = %q", csrfHeader)
	}
	serialized, err := json.Marshal(resolution.Asset)
	if err != nil {
		t.Fatal(err)
	}
	if stringContains(string(serialized), "boxr-fixture-token") || stringContains(string(serialized), "fixture-csrf") {
		t.Fatalf("fixture values leaked in asset: %s", serialized)
	}
}

func TestImageAssetWriterDeduplicatesTokensAndPreservesCaptions(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer server.Close()
	writer := imageWriterFixture(server.URL, t.TempDir())

	first := writer.resolve(imageReferenceFixture("boxr-fixture-token"))
	secondReference := imageReferenceFixture("boxr-fixture-token")
	secondReference.Caption = "Second caption"
	second := writer.resolve(secondReference)

	if first.MarkdownPath != second.MarkdownPath {
		t.Fatalf("markdown paths = %q and %q", first.MarkdownPath, second.MarkdownPath)
	}
	if first.Asset == nil || second.Asset != nil {
		t.Fatalf("assets = %#v and %#v", first.Asset, second.Asset)
	}
	if first.AltText != "Architecture diagram" || second.AltText != "Second caption" {
		t.Fatalf("alt text = %q and %q", first.AltText, second.AltText)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestImageAssetWriterReturnsSafeWarningsForHTTPAndContentFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stringContains(r.URL.Path, "http-fail") {
			http.Error(w, "sensitive fixture body", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not-a-png"))
	}))
	defer server.Close()
	writer := imageWriterFixture(server.URL, t.TempDir())

	httpFailure := writer.resolve(imageReferenceFixture("http-fail-token"))
	contentFailure := writer.resolve(imageReferenceFixture("content-fail-token"))

	if httpFailure.Warning != "image 1 download failed: http_error" {
		t.Fatalf("http warning = %q", httpFailure.Warning)
	}
	if contentFailure.Warning != "image 2 download failed: content_error" {
		t.Fatalf("content warning = %q", contentFailure.Warning)
	}
	serialized := httpFailure.Warning + contentFailure.Warning
	if stringContains(serialized, "sensitive fixture body") || stringContains(serialized, "http-fail-token") {
		t.Fatalf("fixture values leaked in warnings: %s", serialized)
	}
}

func TestImageAssetWriterValidatesSafeFilenameFallbackMagic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/custom")
		_, _ = w.Write(pngBytes)
	}))
	defer server.Close()
	writer := imageWriterFixture(server.URL, t.TempDir())
	reference := imageReferenceFixture("boxr-fixture-token")
	reference.Name = "architecture.png"

	resolution := writer.resolve(reference)

	if resolution.MarkdownPath != "assets/docx_1/image-001.png" {
		t.Fatalf("markdown path = %q", resolution.MarkdownPath)
	}
	if resolution.Asset == nil || resolution.Asset["mimeType"] != "image/custom" {
		t.Fatalf("asset = %#v", resolution.Asset)
	}
}

func TestImageAssetWriterRemovesStaleGeneratedFilesBeforeDownload(t *testing.T) {
	tmpDir := t.TempDir()
	assetDir := filepath.Join(tmpDir, "assets", "docx_1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePNG := filepath.Join(assetDir, "image-001.png")
	staleHTML := filepath.Join(assetDir, "image-002.html")
	stalePartial := filepath.Join(assetDir, "image-1000.webp.part")
	keep := filepath.Join(assetDir, "keep.txt")
	for path, content := range map[string][]byte{
		stalePNG:     []byte("old fixture image"),
		staleHTML:    []byte("old fixture content"),
		stalePartial: []byte("old partial image"),
		keep:         []byte("keep"),
	} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("sensitive fixture response body"))
	}))
	defer server.Close()
	writer := imageWriterFixture(server.URL, tmpDir)

	resolution := writer.resolve(imageReferenceFixture("boxr-fixture-token"))

	if resolution.Warning != "image 1 download failed: mime_error" {
		t.Fatalf("warning = %q", resolution.Warning)
	}
	for _, path := range []string{stalePNG, staleHTML, stalePartial} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale generated file still exists: %s", path)
		}
	}
	if content, err := os.ReadFile(keep); err != nil || string(content) != "keep" {
		t.Fatalf("keep content = %q, err = %v", content, err)
	}
}

func imageWriterFixture(origin string, outputRoot string) *imageAssetWriter {
	return newImageAssetWriter(
		&remoteReadSession{
			client:    http.DefaultClient,
			csrfToken: "fixture-csrf",
			cookies:   []http.Cookie{{Name: "session", Value: "fixture-session"}},
		},
		origin,
		origin+"/docx/dox-fixture-document",
		"dox-fixture-document",
		outputRoot,
		"docx_1",
	)
}

func imageReferenceFixture(token string) docx.ImageReference {
	return docx.ImageReference{
		BlockID:      "image-block",
		Token:        token,
		Name:         "architecture.png",
		MimeType:     "image/png",
		Width:        1200,
		Height:       800,
		DeclaredSize: len(pngBytes),
		Caption:      "Architecture diagram",
	}
}
