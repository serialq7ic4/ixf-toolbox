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
	"strings"
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
	if result.Structure == nil || result.Structure["topLevelBlocks"] != 2 {
		t.Fatalf("structure summary = %#v", result.Structure)
	}
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

func TestReadSourcesWithOptionsFallsBackFromEmptyBitableWikiObjectToDocxToken(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		switch r.URL.Path {
		case "/wiki/wiki_1":
			_, _ = w.Write([]byte(`<html><script>
				current_space_wiki = Object();
				window.wiki_suite_type = 'docx';
				window.bootstrap = {"obj_token":"dox_page_1"};
			</script><script src="/static/bitable-runtime.js"></script></html>`))
		case "/space/api/docx/pages/client_vars":
			if got := r.URL.Query().Get("id"); got != "dox_page_1" {
				t.Fatalf("id query = %q, want dox_page_1", got)
			}
			writeJSONResponse(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"block_map": map[string]any{
						"dox_page_1": map[string]any{
							"data": map[string]any{
								"type":     "page",
								"children": []any{"text_1"},
								"text":     attributedTextValue("Wiki Doc"),
							},
						},
						"text_1": map[string]any{
							"data": map[string]any{
								"type":      "text",
								"parent_id": "dox_page_1",
								"text":      attributedTextValue("From fallback"),
							},
						},
					},
					"has_more": false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/wiki/wiki_1"}, ReadOptions{
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
	if result.Kind != "wiki" || result.Title != "Wiki Doc" || result.Token != "dox_page_1" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Content != "# Wiki Doc\n\nFrom fallback\n" {
		t.Fatalf("content = %q", result.Content)
	}
	assertResultCounts(t, result.Counts, map[string]int{"page": 1, "text": 1})
	if len(requested) != 2 || requested[1] != "/space/api/docx/pages/client_vars?id=dox_page_1&open_type=1" {
		t.Fatalf("requested URLs = %#v", requested)
	}
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

func TestRemoteKindFromPathRecognizesDirectBitableLinks(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/base/bas_fixture", "bitable"},
		{"/base/bas_fixture/tbl_main", "bitable"},
		{"/docx/dox_page_1", "docx"},
		{"/wiki/wiki_1", "wiki"},
		{"/sheets/shtr_fixture", "sheet"},
		{"/base/", "remote"},
	}
	for _, testCase := range cases {
		if got := remoteKindFromPath(testCase.path); got != testCase.want {
			t.Fatalf("remoteKindFromPath(%q) = %q, want %q", testCase.path, got, testCase.want)
		}
	}
	if got := remoteKindFromSource("https://tenant.example.test/base/bas_fixture?table=tbl_main"); got != "bitable" {
		t.Fatalf("remoteKindFromSource(direct base URL) = %q, want bitable", got)
	}
}

func TestReadSourcesWithOptionsReadsDirectBitableBaseLinks(t *testing.T) {
	var requested []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			requested = append(requested, r.URL.String())
			if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
				t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/base/bas_fixture?table=tbl_main" {
				t.Fatalf("referer = %q, want direct base URL", got)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableFixtureData(t)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/base/bas_fixture?table=tbl_main"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if len(requested) != 1 {
		t.Fatalf("bitable clientvars requests = %#v, want exactly one", requested)
	}
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	result := results[0]
	if result.Kind != "bitable" || result.Title != "Bug Tracker" || result.Token != "bas_fixture" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Content != wantBitableFixtureContent {
		t.Fatalf("content = %q, want %q", result.Content, wantBitableFixtureContent)
	}
	assertResultCounts(t, result.Counts, map[string]int{
		"bitable":         1,
		"bitable_views":   1,
		"bitable_fields":  3,
		"bitable_records": 2,
	})
	if len(result.Assets) != 0 {
		t.Fatalf("assets = %#v, want empty", result.Assets)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty", result.Warnings)
	}
	if result.Structure != nil {
		t.Fatalf("structure = %#v, want nil for a bitable read", result.Structure)
	}
}

func TestReadSourcesWithOptionsStillReadsWikiHostedBitable(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch r.URL.Path {
		case "/wiki/wiki_1":
			_, _ = w.Write([]byte(`<html><script>
				current_space_wiki = Object({"obj_token":"bas_fixture","wiki_token":"wiki_1"});
				window.wiki_suite_type = 'bitable';
			</script></html>`))
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableFixtureData(t)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/wiki/wiki_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	result := results[0]
	if result.Kind != "wiki_bitable" || result.Title != "Bug Tracker" || result.Token != "bas_fixture" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Content != wantBitableFixtureContent {
		t.Fatalf("content = %q, want %q", result.Content, wantBitableFixtureContent)
	}
	assertResultCounts(t, result.Counts, map[string]int{
		"bitable":         1,
		"bitable_views":   1,
		"bitable_fields":  3,
		"bitable_records": 2,
	})
	want := []string{"/wiki/wiki_1", "/space/api/v1/bitable/bas_fixture/clientvars"}
	if len(requested) != len(want) {
		t.Fatalf("requested paths = %#v, want %#v", requested, want)
	}
	for index, path := range want {
		if requested[index] != path {
			t.Fatalf("requested paths = %#v, want %#v", requested, want)
		}
	}
}

const wantBitableFixtureContent = "# Bug Tracker\n\n[bitable token=bas_fixture]\n" +
	"[bitable-meta base_token=bas_fixture table_id=tbl_main table_name=\"Issues\" " +
	"view_id=vew_grid view_name=\"Grid\" rows=2 cols=3 views=1]\n" +
	"```tsv\nTitle\tOwner\tModule\nImage bug\tMei\tCollab\nResume failed\t\tDocs\n```\n"

func bitableFixtureData(t *testing.T) map[string]any {
	t.Helper()
	schema := map[string]any{
		"base": map[string]any{
			"name":       "Bug Tracker",
			"timezone":   "Asia/Shanghai",
			"tables":     []any{"tbl_main"},
			"tableInfos": map[string]any{"tbl_main": map[string]any{"name": "Issues"}},
		},
		"data": map[string]any{
			"table": map[string]any{
				"id":    "tbl_main",
				"views": []any{"vew_grid"},
				"viewMap": map[string]any{
					"vew_grid": map[string]any{
						"id":   "vew_grid",
						"name": "Grid",
						"type": float64(1),
						"property": map[string]any{
							"fields":  []any{"fld_title", "fld_owner", "fld_module"},
							"records": []any{"rec_1", "rec_2"},
						},
					},
				},
				"fieldMap": map[string]any{
					"fld_title": map[string]any{"id": "fld_title", "name": "Title", "type": float64(1)},
					"fld_owner": map[string]any{"id": "fld_owner", "name": "Owner", "type": float64(11)},
					"fld_module": map[string]any{
						"id":   "fld_module",
						"name": "Module",
						"type": float64(3),
						"property": map[string]any{
							"options": []any{
								map[string]any{"id": "opt_collab", "name": "Collab"},
								map[string]any{"id": "opt_docs", "name": "Docs"},
							},
						},
					},
				},
			},
			"recordMap": map[string]any{
				"rec_1": map[string]any{
					"fld_title":  map[string]any{"value": "Image bug"},
					"fld_owner":  map[string]any{"value": []any{"usr_1"}},
					"fld_module": map[string]any{"value": "opt_collab"},
				},
				"rec_2": map[string]any{
					"fld_title":  map[string]any{"value": "Resume failed"},
					"fld_owner":  map[string]any{"value": []any{}},
					"fld_module": map[string]any{"value": "opt_docs"},
				},
			},
		},
	}
	return map[string]any{
		"users":     map[string]any{"usr_1": map[string]any{"name": "Mei"}},
		"oldSchema": map[string]any{"gzipSchema": gzipJSON(t, schema)},
	}
}

// wikiSheetHTML mirrors the live wiki page of a native-sheet node.
//
// Two details are load-bearing. current_space_wiki is assigned empty first and populated
// only later, guarded by a test on that empty value, so reading just the first
// occurrence of the anchor finds no token. And the page mentions bitable in an unrelated
// feature flag, which is all isBitableWikiHTML looks for, so a sheet node reaches the
// bitable branch unless the obj_token prefix is checked first.
func wikiSheetHTML(objToken string) string {
	return `<html><script>
		window.current_space_wiki = Object(); window.wiki_suite_type = 'sheet';
	</script><script>
		if (!window.current_space_wiki.space_id) { window.current_space_wiki = Object(` +
		`{"space_id":"7433","wiki_token":"wiki_sheet_1","obj_token":"` + objToken + `","obj_type":3}); }
		var r = window.User.clientFeatures["ccm.bitable.wiki_sidebar_collapsed.enable"];
	</script></html>`
}

type fixtureSheet struct {
	id    string
	name  string
	index int
}

// sheetWorkbookFixture answers the sheet-id probe in the live server's shape: the
// snapshot's sheets map is keyed by sheet NAME with the id in an inner "id" field, the
// workbook reports its own sheetCount, and the sheet the server would open is named at
// extra_data.sheet_id.
func sheetWorkbookFixture(t *testing.T, title string, active string, sheetCount int, sheets ...fixtureSheet) map[string]any {
	t.Helper()
	sheetMap := map[string]any{}
	for _, sheet := range sheets {
		sheetMap[sheet.name] = map[string]any{
			"id":          sheet.id,
			"name":        sheet.name,
			"index":       sheet.index,
			"rowCount":    2,
			"columnCount": 2,
		}
	}
	snapshot := map[string]any{"sheets": sheetMap}
	if sheetCount > 0 {
		snapshot["sheetCount"] = sheetCount
	}
	return map[string]any{
		"token": "workbook",
		"formerlySchema": map[string]any{
			"title": title,
			"clientvars": map[string]any{
				"gzip_snapshot": gzipJSON(t, snapshot),
				"extra_data":    map[string]any{"sheet_id": active, "sub_type": 0},
			},
		},
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

// A TSV body keeps every delimiter, so a row stays as wide as its header.
//
// This is what the pre-existing fixture cannot show: its only empty cell sits
// mid-row, where a following tab protects it. The damage needs a trailing empty
// cell, and an all-empty record, neither of which that fixture has.
//
// The rendered body used to pass through normalizeLines, a Markdown normalizer.
// Right-trimming deleted trailing tabs, which are columns. Blank-run collapsing
// was worse: a record whose every cell is empty renders as tabs alone, trims to
// "", and was then treated as a blank line, so consecutive empty records merged
// into one and trailing ones vanished -- while the meta line kept reporting the
// original row count.
func TestReadSourcesWithOptionsKeepsEmptyTrailingBitableCells(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/v1/bitable/bas_ragged/clientvars" {
			http.NotFound(w, r)
			return
		}
		writeJSONResponse(t, w, map[string]any{
			"code": 0,
			"data": raggedBitableFixtureData(t),
		})
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/base/bas_ragged"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}
	content := results[0].Content

	// rows=4 is computed before rendering, so every one of the four must be present
	// as its own line or the document contradicts its own meta field.
	if !strings.Contains(content, "rows=4 cols=3") {
		t.Fatalf("content lost its meta line:\n%s", content)
	}
	body, ok := tsvBodyRows(content)
	if !ok {
		t.Fatalf("content has no tsv fence:\n%s", content)
	}
	if len(body) != 5 {
		t.Fatalf("tsv body has %d lines, want a header and 4 records:\n%q", len(body), body)
	}
	for index, row := range body {
		if got := strings.Count(row, "\t"); got != 2 {
			t.Fatalf("row %d = %q has %d tabs, want 2 so it stays as wide as the header",
				index, row, got)
		}
	}
	if body[2] != "\t\t" {
		t.Fatalf("all-empty record = %q, want a row of delimiters alone", body[2])
	}
	if body[3] != "\t\t" {
		t.Fatalf("second all-empty record = %q, want it kept rather than collapsed into the first", body[3])
	}
	if body[4] != "Trailing gone\t\t" {
		t.Fatalf("trailing-empty record = %q, want its empty columns kept", body[4])
	}
}

// tsvBodyRows returns the lines inside the first ```tsv fence.
func tsvBodyRows(content string) ([]string, bool) {
	_, after, found := strings.Cut(content, "```tsv\n")
	if !found {
		return nil, false
	}
	body, _, found := strings.Cut(after, "\n```")
	if !found {
		return nil, false
	}
	return strings.Split(body, "\n"), true
}

// raggedBitableFixtureData has what the main fixture lacks: a record with empty
// trailing cells, and two adjacent records that are empty across every column.
func raggedBitableFixtureData(t *testing.T) map[string]any {
	t.Helper()
	emptyRecord := map[string]any{
		"fld_title":  map[string]any{"value": ""},
		"fld_owner":  map[string]any{"value": []any{}},
		"fld_module": map[string]any{"value": ""},
	}
	schema := map[string]any{
		"base": map[string]any{
			"name":       "Ragged",
			"timezone":   "Asia/Shanghai",
			"tables":     []any{"tbl_main"},
			"tableInfos": map[string]any{"tbl_main": map[string]any{"name": "Rows"}},
		},
		"data": map[string]any{
			"table": map[string]any{
				"id":    "tbl_main",
				"views": []any{"vew_grid"},
				"viewMap": map[string]any{
					"vew_grid": map[string]any{
						"id":   "vew_grid",
						"name": "Grid",
						"type": float64(1),
						"property": map[string]any{
							"fields":  []any{"fld_title", "fld_owner", "fld_module"},
							"records": []any{"rec_full", "rec_blank_1", "rec_blank_2", "rec_trailing"},
						},
					},
				},
				"fieldMap": map[string]any{
					"fld_title":  map[string]any{"id": "fld_title", "name": "Title", "type": float64(1)},
					"fld_owner":  map[string]any{"id": "fld_owner", "name": "Owner", "type": float64(11)},
					"fld_module": map[string]any{"id": "fld_module", "name": "Module", "type": float64(1)},
				},
			},
			"recordMap": map[string]any{
				"rec_full": map[string]any{
					"fld_title":  map[string]any{"value": "Complete"},
					"fld_owner":  map[string]any{"value": []any{"usr_1"}},
					"fld_module": map[string]any{"value": "Core"},
				},
				"rec_blank_1": emptyRecord,
				"rec_blank_2": emptyRecord,
				"rec_trailing": map[string]any{
					"fld_title":  map[string]any{"value": "Trailing gone"},
					"fld_owner":  map[string]any{"value": []any{}},
					"fld_module": map[string]any{"value": ""},
				},
			},
		},
	}
	return map[string]any{
		"users":     map[string]any{"usr_1": map[string]any{"name": "Mei"}},
		"oldSchema": map[string]any{"gzipSchema": gzipJSON(t, schema)},
	}
}

// A wiki node backed by a native sheet reads end to end: wiki HTML -> workbook token ->
// sheet id -> content. Before this path existed the read died at `client_vars failed`,
// because the wiki branch found no docx token and called the docx endpoint anyway.
func TestReadSourcesWithOptionsReadsWikiHostedNativeSheet(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch r.URL.Path {
		case "/wiki/wiki_sheet_1":
			_, _ = w.Write([]byte(wikiSheetHTML("shtrWorkbook")))
		case "/space/api/v3/sheet/client_vars":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if _, named := request["sheetRange"]; !named {
				writeJSONResponse(t, w, map[string]any{
					"code": 0,
					"data": sheetWorkbookFixture(t, "Q4 Rollout", "l0tsDa", 1,
						fixtureSheet{id: "l0tsDa", name: "问题报告", index: 0}),
				})
				return
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetFixtureData(t, "l0tsDa")})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/wiki/wiki_sheet_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	result := results[0]
	if result.Kind != "wiki_sheet" || result.Title != "Q4 Rollout" || result.Token != "shtrWorkbook_l0tsDa" {
		t.Fatalf("result metadata = %#v", result)
	}
	want := "# Q4 Rollout\n\n[sheet-workbook token=shtrWorkbook sheets=1 sheet_id=l0tsDa sheet_name=\"问题报告\"]\n" +
		"[sheet-meta workbook_token=shtrWorkbook sheet_id=l0tsDa rows=2 cols=2]\n" +
		"```tsv\nName\tValue\nAlpha\t42\n```\n"
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	assertResultCounts(t, result.Counts, map[string]int{"sheet": 1, "sheet_total": 1})
	// A single-sheet workbook has nothing to caveat.
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty for a one-sheet workbook", result.Warnings)
	}
	wantPaths := []string{"/wiki/wiki_sheet_1", "/space/api/v3/sheet/client_vars", "/space/api/v3/sheet/client_vars"}
	if len(requested) != len(wantPaths) {
		t.Fatalf("requested paths = %#v, want %#v", requested, wantPaths)
	}
}

// The sheet-id lookup is a POST that names no sheet.
//
// Sending no sheetRange at all is what makes the server answer with the sheet it would
// open; a sheetRange naming an id it does not have is not the same request. An empty
// sheetId string is also wrong, so the body must not carry the key at all.
func TestWikiHostedSheetResolvesTheSheetIDWithoutNamingOne(t *testing.T) {
	type sheetCall struct {
		method    string
		query     string
		hasRange  bool
		rangeData map[string]any
	}
	var calls []sheetCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wiki/wiki_sheet_1":
			_, _ = w.Write([]byte(wikiSheetHTML("shtrWorkbook")))
		case "/space/api/v3/sheet/client_vars":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			rangeValue, hasRange := request["sheetRange"]
			call := sheetCall{method: r.Method, query: r.URL.RawQuery, hasRange: hasRange}
			if hasRange {
				call.rangeData = asMap(rangeValue)
			}
			calls = append(calls, call)
			if request["token"] != "shtrWorkbook" {
				t.Fatalf("token = %#v, want the workbook token", request["token"])
			}
			if !hasRange {
				writeJSONResponse(t, w, map[string]any{
					"code": 0,
					"data": sheetWorkbookFixture(t, "Probe", "l0tsDa", 1,
						fixtureSheet{id: "l0tsDa", name: "Only", index: 0}),
				})
				return
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetFixtureData(t, "l0tsDa")})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	if _, err := ReadSourcesWithOptions([]string{server.URL + "/wiki/wiki_sheet_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	}); err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("sheet client_vars calls = %#v, want a lookup then a content read", calls)
	}
	lookup := calls[0]
	if lookup.method != http.MethodPost {
		t.Fatalf("lookup method = %s, want POST", lookup.method)
	}
	if lookup.hasRange {
		t.Fatalf("lookup body carried sheetRange = %#v, want no sheet named at all", lookup.rangeData)
	}
	wantQuery := "synced_block_host_token=shtrWorkbook&synced_block_host_type=22"
	if lookup.query != wantQuery {
		t.Fatalf("lookup query = %q, want %q", lookup.query, wantQuery)
	}
	// The content read is the existing sheet reader, addressing the resolved id.
	if !calls[1].hasRange || calls[1].rangeData["sheetId"] != "l0tsDa" {
		t.Fatalf("content read range = %#v, want sheetId l0tsDa", calls[1].rangeData)
	}
}

// A workbook with more than one sheet says so, in the header line and in a warning.
//
// Only the first sheet is read, and a caller handed 1 of 3 sheets with no indication
// that 2 are missing is the silent-wrong-output failure worth preventing. "First" is the
// lowest-indexed sheet, not the sheet the server happens to open: this fixture opens on
// index 1, and the read must still cover index 0 so the default does not depend on the
// workbook's saved cursor position.
func TestReadSourcesWithOptionsReportsMultiSheetWorkbooks(t *testing.T) {
	var contentSheetID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wiki/wiki_sheet_1":
			_, _ = w.Write([]byte(wikiSheetHTML("shtrMulti")))
		case "/space/api/v3/sheet/client_vars":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			rangeValue, hasRange := request["sheetRange"]
			if !hasRange {
				writeJSONResponse(t, w, map[string]any{
					"code": 0,
					"data": sheetWorkbookFixture(t, "Three Tabs", "sheetB", 3,
						fixtureSheet{id: "sheetC", name: "Gamma", index: 2},
						fixtureSheet{id: "sheetA", name: "Alpha", index: 0},
						fixtureSheet{id: "sheetB", name: "Beta", index: 1}),
				})
				return
			}
			contentSheetID = stringValue(asMap(rangeValue)["sheetId"])
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetFixtureData(t, contentSheetID)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/wiki/wiki_sheet_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	if contentSheetID != "sheetA" {
		t.Fatalf("content read sheet %q, want sheetA, the lowest-indexed sheet", contentSheetID)
	}
	result := results[0]
	wantHeader := "[sheet-workbook token=shtrMulti sheets=3 sheet_id=sheetA sheet_name=\"Alpha\"]"
	if !strings.Contains(result.Content, wantHeader) {
		t.Fatalf("content = %q, want it to carry %q", result.Content, wantHeader)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one naming the unread sheets", result.Warnings)
	}
	// The warning has to carry the count, not just say "more than one".
	for _, fragment := range []string{"3 sheets", "first sheet only"} {
		if !strings.Contains(result.Warnings[0], fragment) {
			t.Fatalf("warning = %q, want it to contain %q", result.Warnings[0], fragment)
		}
	}
	assertResultCounts(t, result.Counts, map[string]int{"sheet": 1, "sheet_total": 3})
}

// A response that reports no sheet count says so rather than claiming one.
//
// Inventing "sheets=1" for a workbook whose count could not be read would be the same
// silent-wrong-output failure as omitting the count entirely.
func TestReadSourcesWithOptionsReportsAnUnknownSheetCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wiki/wiki_sheet_1":
			_, _ = w.Write([]byte(wikiSheetHTML("shtrBare")))
		case "/space/api/v3/sheet/client_vars":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if _, hasRange := request["sheetRange"]; !hasRange {
				// No sheetCount, no sheets map: only the active sheet id is on offer.
				writeJSONResponse(t, w, map[string]any{
					"code": 0,
					"data": map[string]any{
						"formerlySchema": map[string]any{
							"title": "Bare",
							"clientvars": map[string]any{
								"extra_data": map[string]any{"sheet_id": "03df21"},
							},
						},
					},
				})
				return
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetFixtureData(t, "03df21")})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	results, err := ReadSourcesWithOptions([]string{server.URL + "/wiki/wiki_sheet_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}

	result := results[0]
	if !strings.Contains(result.Content, "sheets=unknown") {
		t.Fatalf("content = %q, want the count reported as unknown", result.Content)
	}
	// Falling back to the active sheet id is what keeps the read possible at all here.
	if !strings.Contains(result.Content, "sheet_id=03df21") {
		t.Fatalf("content = %q, want the active sheet id used", result.Content)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "unavailable") {
		t.Fatalf("warnings = %#v, want one saying the count is unavailable", result.Warnings)
	}
	// No sheet_total key, rather than sheet_total: 0, which would read as "no sheets".
	assertResultCounts(t, result.Counts, map[string]int{"sheet": 1})
}

func TestWikiSheetWorkbookTokenOnlyClaimsSheetBackedNodes(t *testing.T) {
	for _, test := range []struct {
		name string
		html string
		want string
	}{
		{
			name: "skips the empty first assignment",
			html: wikiSheetHTML("shtrWorkbook"),
			want: "shtrWorkbook",
		},
		{
			name: "docx node is not a sheet",
			html: `<html><script>current_space_wiki = Object({"obj_token":"dox_page_1"});</script></html>`,
			want: "",
		},
		{
			name: "bitable node is not a sheet",
			html: `<html><script>current_space_wiki = Object({"obj_token":"bas_fixture"});</script></html>`,
			want: "",
		},
		{
			// The loose `obj_token":"..."` scan the docx path falls back to would match
			// this; the structured object is empty, so no sheet is claimed.
			name: "a bare obj_token elsewhere on the page is not enough",
			html: `<html><script>current_space_wiki = Object();
				window.other = {"obj_token":"shtrSomeOtherNode"};</script></html>`,
			want: "",
		},
		{
			name: "no wiki object at all",
			html: `<html><body>nothing here</body></html>`,
			want: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := wikiSheetWorkbookToken(test.html); got != test.want {
				t.Fatalf("wikiSheetWorkbookToken() = %q, want %q", got, test.want)
			}
		})
	}
}

// A wiki-hosted sheet page reaches the sheet branch even though it also looks like a
// bitable page to isBitableWikiHTML, and a docx wiki node is unaffected by the new
// branch. Both pages are built from the same wiki template, which is why the sheet check
// has to come first and has to key off the obj_token rather than the page text.
func TestWikiDispatchSeparatesSheetFromBitableAndDocxPages(t *testing.T) {
	sheetPage := wikiSheetHTML("shtrWorkbook")
	if !isBitableWikiHTML(sheetPage) {
		t.Fatal("fixture no longer reproduces the overlap: a sheet page must still look " +
			"like a bitable page to isBitableWikiHTML, or this ordering stops mattering")
	}
	if got := wikiSheetWorkbookToken(sheetPage); got != "shtrWorkbook" {
		t.Fatalf("sheet page resolved %q, want the workbook token", got)
	}
	// The pre-existing docx fallback fixture must not be claimed by the sheet branch.
	docxPage := `<html><script>
		current_space_wiki = Object();
		window.wiki_suite_type = 'docx';
		window.bootstrap = {"obj_token":"dox_page_1"};
	</script><script src="/static/bitable-runtime.js"></script></html>`
	if got := wikiSheetWorkbookToken(docxPage); got != "" {
		t.Fatalf("docx page resolved %q, want no workbook token", got)
	}
}

// An override naming a host this read cannot use is refused rather than ignored.
//
// Only the docx path sends its request to spaceAPI. Every other media type here
// addresses the document's own origin, so an override pointing elsewhere used to be
// accepted and then silently dropped: the read succeeded against the origin and
// looked like it had honoured the flag.
func TestReadSourcesWithOptionsRefusesASpaceAPIItCannotApply(t *testing.T) {
	// Any request at all means the refusal came too late.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to %s; the override should be refused first", r.URL.Path)
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{name: "direct sheet", source: "https://tenant.example.test/sheets/shtFixture?sheet=abc", want: "sheet source"},
		{name: "direct bitable", source: "https://tenant.example.test/base/basFixture", want: "bitable source"},
		{name: "mindnote", source: "https://tenant.example.test/mindnotes/bmnFixture", want: "mindnote source"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadSourcesWithOptions([]string{test.source}, ReadOptions{
				CookiesPath: cookiesPath,
				SpaceAPI:    server.URL,
			})
			if err == nil {
				t.Fatal("ReadSourcesWithOptions succeeded, want a refusal naming the override")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want it to name the %s", err, test.want)
			}
			// The message has to be actionable: both hosts, and what to do.
			for _, fragment := range []string{server.URL, "https://tenant.example.test", "Drop --space-api"} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %v, want it to contain %q", err, fragment)
				}
			}
		})
	}
}

// A wiki-hosted sheet refuses an override it cannot apply, like the other
// origin-addressed reads. The refusal comes after the page fetch, because only the HTML
// says this wiki node is a sheet, but before any sheet request.
func TestReadSourcesWithOptionsRefusesASpaceAPIForWikiHostedSheets(t *testing.T) {
	var paths []string
	wikiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(wikiSheetHTML("shtrWorkbook")))
	}))
	defer wikiServer.Close()
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to the override host at %s", r.URL.Path)
	}))
	defer overrideServer.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	_, err := ReadSourcesWithOptions([]string{wikiServer.URL + "/wiki/wiki_sheet_1"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    overrideServer.URL,
	})
	if err == nil {
		t.Fatal("ReadSourcesWithOptions succeeded, want a refusal naming the override")
	}
	if !strings.Contains(err.Error(), "wiki-hosted sheet source") {
		t.Fatalf("error = %v, want it to name the wiki-hosted sheet source", err)
	}
	// Only the page fetch may have happened; no sheet request.
	for _, path := range paths {
		if path != "/wiki/wiki_sheet_1" {
			t.Fatalf("requested %q before refusing; want only the page fetch", path)
		}
	}
}

// An override equal to the origin is accepted, since the request goes there anyway.
// This is the shape every other test in this package uses.
func TestReadSourcesWithOptionsAcceptsASpaceAPIEqualToTheOrigin(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/v1/bitable/bas_fixture/clientvars" {
			http.NotFound(w, r)
			return
		}
		requests++
		writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableFixtureData(t)})
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	// A trailing slash must not make an otherwise-equal override look different.
	results, err := ReadSourcesWithOptions([]string{server.URL + "/base/bas_fixture"}, ReadOptions{
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL + "/",
	})
	if err != nil {
		t.Fatalf("ReadSourcesWithOptions returned error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want the read to proceed", requests)
	}
	if results[0].Kind != "bitable" {
		t.Fatalf("kind = %q, want bitable", results[0].Kind)
	}
}
