package docslocal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

func TestReadSourcesWithOptionsReadsRemoteDocxClientVars(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.URL.Path != "/space/api/docx/pages/client_vars" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("id"); got != "page_1" {
			t.Fatalf("id query = %q, want page_1", got)
		}
		if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
			t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
		}
		if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "session-fixture" {
			t.Fatalf("session cookie = %#v, %v; want session-fixture", cookie, err)
		}
		if r.URL.Query().Get("cursor") == "next-cursor" {
			writeJSONResponse(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"block_map": map[string]any{
						"text_2": map[string]any{
							"data": map[string]any{
								"type":      "text",
								"parent_id": "page_1",
								"text":      attributedTextValue("Later"),
							},
						},
					},
					"has_more": false,
				},
			})
			return
		}
		writeJSONResponse(t, w, map[string]any{
			"code": 0,
			"data": map[string]any{
				"block_map": map[string]any{
					"page_1": map[string]any{
						"data": map[string]any{
							"type":     "page",
							"children": []any{"text_1", "text_2"},
							"text":     attributedTextValue("Remote Doc"),
						},
					},
					"text_1": map[string]any{
						"data": map[string]any{
							"type":      "text",
							"parent_id": "page_1",
							"text":      attributedTextValue("First"),
						},
					},
				},
				"has_more": true,
				"cursor":   "next-cursor",
			},
		})
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/docx/page_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	result := results[0]
	if result.Kind != "docx" || result.Title != "Remote Doc" || result.Token != "page_1" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Content != "# Remote Doc\n\nFirst\n\nLater\n" {
		t.Fatalf("content = %q", result.Content)
	}
	assertResultCounts(t, result.Counts, map[string]int{"page": 1, "text": 2})
	if len(requested) != 2 || requested[1] != "/space/api/docx/pages/client_vars?id=page_1&open_type=1&mode=4&cursor=next-cursor" {
		t.Fatalf("requested URLs = %#v", requested)
	}
}

func TestReadSourcesWithOptionsDownloadsRemoteDocxImages(t *testing.T) {
	token := "boxr-image-token"
	var downloadURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			writeJSONResponse(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"block_map": map[string]any{
						"page_1": map[string]any{
							"data": map[string]any{
								"type":     "page",
								"children": []any{"image_1"},
								"text":     attributedTextValue("Image Doc"),
							},
						},
						"image_1": map[string]any{
							"data": map[string]any{
								"type":      "image",
								"parent_id": "page_1",
								"image": map[string]any{
									"token":    token,
									"name":     "architecture.png",
									"mimeType": "image/png",
									"width":    1200,
									"height":   800,
									"size":     len(pngBytes),
									"caption":  attributedTextValue("Architecture diagram"),
								},
							},
						},
					},
					"has_more": false,
				},
			})
		case "/space/api/box/stream/download/all/" + token + "/":
			downloadURL = r.URL.String()
			if got := r.URL.Query().Get("mount_node_token"); got != "page_1" {
				t.Fatalf("mount_node_token = %q, want page_1", got)
			}
			if got := r.URL.Query().Get("mount_point"); got != "docx_image" {
				t.Fatalf("mount_point = %q, want docx_image", got)
			}
			if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
				t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/docx/page_1"}, ReadOptions{
		CookiesPath:    cookiesPath,
		SpaceAPI:       server.URL,
		DownloadImages: true,
		OutputRoot:     tmpDir,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	result := results[0]
	if result.Content != "# Image Doc\n\n![Architecture diagram](assets/docx_1/image-001.png)\n" {
		t.Fatalf("content = %q", result.Content)
	}
	if len(result.Assets) != 1 {
		t.Fatalf("assets = %#v, want one", result.Assets)
	}
	asset := result.Assets[0]
	if asset["path"] != "assets/docx_1/image-001.png" || asset["mimeType"] != "image/png" ||
		asset["width"] != 1200 || asset["height"] != 800 || asset["sizeBytes"] != len(pngBytes) ||
		asset["status"] != "downloaded" || asset["ordinal"] != 1 {
		t.Fatalf("asset = %#v", asset)
	}
	assetPath := filepath.Join(tmpDir, "assets", "docx_1", "image-001.png")
	content, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(pngBytes) {
		t.Fatalf("asset bytes = %q, want png bytes", content)
	}
	if downloadURL == "" {
		t.Fatal("image download endpoint was not requested")
	}
	serialized, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if stringContains(string(serialized), token) || stringContains(string(serialized), downloadURL) {
		t.Fatalf("private image token leaked in result: %s", serialized)
	}
}

func writeCookieFixture(t *testing.T, path string) {
	t.Helper()
	content, err := json.Marshal([]map[string]string{
		{"name": "_csrf_token", "value": "csrf-fixture"},
		{"name": "session", "value": "session-fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func attributedTextValue(text string) map[string]any {
	return map[string]any{
		"initialAttributedTexts": map[string]any{
			"text": map[string]any{"0": text},
		},
	}
}

func assertResultCounts(t *testing.T, got map[string]int, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("counts = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("counts[%q] = %d, want %d; all counts %#v", key, got[key], wantValue, got)
		}
	}
}

func stringContains(value string, needle string) bool {
	return needle != "" && len(value) >= len(needle) && stringIndex(value, needle) >= 0
}

func stringIndex(value string, needle string) int {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}

func TestReadOptionsRejectsDownloadImagesWithoutOutputRoot(t *testing.T) {
	_, err := ReadSourcesWithOptions([]string{"https://tenant.example.test/docx/page_1"}, ReadOptions{
		CookiesPath:    filepath.Join(t.TempDir(), "missing.json"),
		DownloadImages: true,
	})
	if err == nil || err.Error() != "download_images requires output_root" {
		t.Fatalf("error = %v, want output_root requirement", err)
	}
}
