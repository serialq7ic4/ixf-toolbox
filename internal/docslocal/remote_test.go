package docslocal

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
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

func TestReadSourcesWithOptionsExpandsRemoteDocxSheets(t *testing.T) {
	var sheetRequested bool
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
								"children": []any{"sheet_1"},
								"text":     attributedTextValue("Sheet Doc"),
							},
						},
						"sheet_1": map[string]any{
							"data": map[string]any{
								"type":      "sheet",
								"parent_id": "page_1",
								"token":     "shtr_fixture_sheet1",
							},
						},
					},
					"has_more": false,
				},
			})
		case "/space/api/v3/sheet/client_vars":
			sheetRequested = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if got := r.URL.Query().Get("synced_block_host_token"); got != "page_1" {
				t.Fatalf("synced host token = %q, want page_1", got)
			}
			if got := r.URL.Query().Get("synced_block_host_type"); got != "22" {
				t.Fatalf("synced host type = %q, want 22", got)
			}
			if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
				t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["token"] != "shtr_fixture" {
				t.Fatalf("sheet token = %#v, want shtr_fixture", request["token"])
			}
			rangeValue := request["sheetRange"].(map[string]any)
			if rangeValue["sheetId"] != "sheet1" {
				t.Fatalf("sheet range = %#v, want sheet1", rangeValue)
			}
			writeJSONResponse(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"formerlySchema": map[string]any{
						"clientvars": map[string]any{
							"gzip_snapshot": gzipJSON(t, map[string]any{
								"sheets": map[string]any{
									"sheet1": map[string]any{"rowCount": 2, "columnCount": 2},
								},
							}),
							"extra_data": map[string]any{
								"blocks": []any{
									map[string]any{
										"row": 0,
										"gzip_datatable": gzipJSON(t, map[string]any{
											"rows": []any{
												map[string]any{
													"columns": []any{
														map[string]any{"value": "Name"},
														map[string]any{"value": "Value"},
													},
												},
												map[string]any{
													"columns": []any{
														map[string]any{"value": "Alpha"},
														map[string]any{"value": 42},
													},
												},
											},
										}),
									},
								},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/docx/page_1"}, ReadOptions{
		CookiesPath:  cookiesPath,
		SpaceAPI:     server.URL,
		ExpandSheets: true,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if !sheetRequested {
		t.Fatal("sheet client_vars endpoint was not requested")
	}
	result := results[0]
	want := "# Sheet Doc\n\n[sheet token=shtr_fixture_sheet1]\n[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=2 cols=2]\n```tsv\nName\tValue\nAlpha\t42\n```\n"
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	assertResultCounts(t, result.Counts, map[string]int{"page": 1, "sheet": 1, "sheet_expanded": 1})
}

func TestReadSourcesWithOptionsReadsDirectSheetLinks(t *testing.T) {
	var sheetRequested bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v3/sheet/client_vars":
			sheetRequested = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if got := r.URL.Query().Get("synced_block_host_token"); got != "shtr_fixture" {
				t.Fatalf("synced host token = %q, want shtr_fixture", got)
			}
			if got := r.URL.Query().Get("synced_block_host_type"); got != "22" {
				t.Fatalf("synced host type = %q, want 22", got)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/sheets/shtr_fixture?sheet=sheet1" {
				t.Fatalf("referer = %q, want direct sheet URL", got)
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["token"] != "shtr_fixture" {
				t.Fatalf("sheet token = %#v, want shtr_fixture", request["token"])
			}
			rangeValue := request["sheetRange"].(map[string]any)
			if rangeValue["sheetId"] != "sheet1" {
				t.Fatalf("sheet range = %#v, want sheet1", rangeValue)
			}
			writeJSONResponse(t, w, map[string]any{
				"code": 0,
				"data": sheetFixtureData(t, "sheet1"),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/sheets/shtr_fixture?sheet=sheet1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if !sheetRequested {
		t.Fatal("sheet client_vars endpoint was not requested")
	}
	result := results[0]
	if result.Kind != "sheet" || result.Title != "shtr_fixture sheet1" || result.Token != "shtr_fixture_sheet1" {
		t.Fatalf("result metadata = %#v", result)
	}
	want := "# shtr_fixture sheet1\n\n[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=2 cols=2]\n```tsv\nName\tValue\nAlpha\t42\n```\n"
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	assertResultCounts(t, result.Counts, map[string]int{"sheet": 1})
}

func TestReadSourcesWithOptionsKeepsMindnoteArtifactsEmptyWhenDownloadingImages(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.URL.Path != "/mindnotes/mind_1" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
			t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
		}
		payload := map[string]any{
			"token": "mind_1",
			"data": map[string]any{
				"title": "Q3 Mindnote",
				"collab_client_vars": map[string]any{
					"nodes": []any{
						map[string]any{
							"text":     attributedTextValue("Root"),
							"children": []any{},
						},
					},
				},
			},
		}
		content, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("<html><script>window.bootstrap={clientVars: Object(" + string(content) + ")}</script></html>"))
	}))
	defer server.Close()
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/mindnotes/mind_1?from=copy"}, ReadOptions{
		CookiesPath:    cookiesPath,
		DownloadImages: true,
		OutputRoot:     tmpDir,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if requested[0] != "/mindnotes/mind_1?from=copy" {
		t.Fatalf("requested URLs = %#v", requested)
	}
	result := results[0]
	if result.Kind != "mindnote" || result.Title != "Q3 Mindnote" || result.Token != "mind_1" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Content != "# Q3 Mindnote\n\n- Root\n" {
		t.Fatalf("content = %q", result.Content)
	}
	assertResultCounts(t, result.Counts, map[string]int{"mindnote_nodes": 1})
	if len(result.Assets) != 0 {
		t.Fatalf("assets = %#v, want empty", result.Assets)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty", result.Warnings)
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

func gzipJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func sheetFixtureData(t *testing.T, sheetID string) map[string]any {
	t.Helper()
	return map[string]any{
		"formerlySchema": map[string]any{
			"clientvars": map[string]any{
				"gzip_snapshot": gzipJSON(t, map[string]any{
					"sheets": map[string]any{
						sheetID: map[string]any{"rowCount": 2, "columnCount": 2},
					},
				}),
				"extra_data": map[string]any{
					"blocks": []any{
						map[string]any{
							"row": 0,
							"gzip_datatable": gzipJSON(t, map[string]any{
								"rows": []any{
									map[string]any{
										"columns": []any{
											map[string]any{"value": "Name"},
											map[string]any{"value": "Value"},
										},
									},
									map[string]any{
										"columns": []any{
											map[string]any{"value": "Alpha"},
											map[string]any{"value": 42},
										},
									},
								},
							}),
						},
					},
				},
			},
		},
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
