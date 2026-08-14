package bitable

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

func TestParseClientVarsReturnsTablesViewsFieldsAndRecords(t *testing.T) {
	data := bitableClientVarsFixture(t)

	meta, err := ParseClientVars(data, "bas_fixture")
	if err != nil {
		t.Fatalf("ParseClientVars returned error: %v", err)
	}

	if meta.BaseToken != "bas_fixture" || meta.Title != "Bug Tracker" {
		t.Fatalf("metadata identity = %+v", meta)
	}
	if len(meta.Tables) != 1 || meta.Tables[0].ID != "tbl_main" || meta.Tables[0].Name != "Issues" {
		t.Fatalf("tables = %+v", meta.Tables)
	}
	if len(meta.Views) != 1 || meta.Views[0].ID != "vew_grid" || meta.Views[0].Name != "Grid" || meta.Views[0].TableID != "tbl_main" {
		t.Fatalf("views = %+v", meta.Views)
	}
	title := meta.FieldByName("Title")
	if title == nil || title.ID != "fld_title" || title.AttachmentCompatible {
		t.Fatalf("Title field = %+v, want non-attachment field", title)
	}
	screenshot := meta.FieldByName("Screenshot")
	if screenshot == nil || screenshot.ID != "fld_image" || !screenshot.AttachmentCompatible {
		t.Fatalf("Screenshot field = %+v, want attachment-compatible field", screenshot)
	}
	if len(meta.Records) != 2 || meta.Records[0].ID != "rec_1" || meta.Records[1].ID != "rec_2" {
		t.Fatalf("records = %+v", meta.Records)
	}
	if meta.Records[0].Values["fld_title"] != "Image bug" || meta.Records[0].Values["fld_module"] != "Collab" {
		t.Fatalf("record values = %+v", meta.Records[0].Values)
	}
}

func TestParseSourceRecognizesDirectBitableURL(t *testing.T) {
	source, err := ParseSource("https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid")
	if err != nil {
		t.Fatalf("ParseSource returned error: %v", err)
	}
	if source.Kind != "direct_bitable" || source.BaseToken != "bas_fixture" || source.TableID != "tbl_main" || source.ViewID != "vew_grid" {
		t.Fatalf("source = %+v", source)
	}
}

func TestInspectRedactsTokensAndReportsAttachmentFields(t *testing.T) {
	payload, err := Inspect(InspectConfig{
		URL:        "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		ClientVars: bitableClientVarsFixture(t),
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if payload["ok"] != true || payload["operation"] != "bitable_inspect" || payload["sourceKind"] != "direct_bitable" {
		t.Fatalf("payload = %+v", payload)
	}
	if containsPrivateToken(t, payload, "bas_fixture") {
		t.Fatalf("inspect payload leaked base token: %+v", payload)
	}
	fields := payload["fields"].([]map[string]any)
	foundAttachment := false
	for _, field := range fields {
		if field["name"] == "Screenshot" && field["attachmentCompatible"] == true {
			foundAttachment = true
		}
	}
	if !foundAttachment {
		t.Fatalf("fields = %+v, want Screenshot attachment field", fields)
	}
}

func TestAttachDryRunPlansOneUploadWithoutMutation(t *testing.T) {
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})

	payload, err := Attach(AttachConfig{
		URL:         "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		Field:       "Screenshot",
		RecordMatch: "Title=Image bug",
		FilePath:    file,
		DryRun:      true,
		ClientVars:  bitableClientVarsFixture(t),
	})
	if err != nil {
		t.Fatalf("Attach dry-run returned error: %v", err)
	}
	if payload["ok"] != true || payload["dryRun"] != true || payload["willUpload"] != true || payload["willUpdateRecord"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["recordMatchCount"] != 1 {
		t.Fatalf("recordMatchCount = %#v, want 1", payload["recordMatchCount"])
	}
	fileInfo := payload["file"].(map[string]any)
	if fileInfo["name"] != "ceph_logo.jpeg" || fileInfo["mimeType"] != "image/jpeg" {
		t.Fatalf("file metadata = %+v", fileInfo)
	}
}

func TestAttachDryRunRejectsAmbiguousRecordMatch(t *testing.T) {
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})

	_, err := Attach(AttachConfig{
		URL:         "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		Field:       "Screenshot",
		RecordMatch: "Module=Collab",
		FilePath:    file,
		DryRun:      true,
		ClientVars:  bitableClientVarsFixture(t),
	})
	if err == nil || !strings.Contains(err.Error(), "matched 2 records") {
		t.Fatalf("Attach ambiguous match error = %v", err)
	}
}

func TestAttachDryRunRejectsNonAttachmentField(t *testing.T) {
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})

	_, err := Attach(AttachConfig{
		URL:         "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		Field:       "Title",
		RecordMatch: "Title=Image bug",
		FilePath:    file,
		DryRun:      true,
		ClientVars:  bitableClientVarsFixture(t),
	})
	if err == nil || !strings.Contains(err.Error(), "not attachment-compatible") {
		t.Fatalf("Attach non-attachment field error = %v", err)
	}
}

func TestAttachApplyFailsUntilContractCaptured(t *testing.T) {
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})

	_, err := Attach(AttachConfig{
		URL:         "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		Field:       "Screenshot",
		RecordMatch: "Title=Image bug",
		FilePath:    file,
		Apply:       true,
		ClientVars:  bitableClientVarsFixture(t),
	})
	if err == nil || !strings.Contains(err.Error(), "upload API contract is captured") {
		t.Fatalf("Attach apply gate error = %v", err)
	}
}

func TestInspectHTTPFetchesDirectBitableClientVars(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	var requested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/v1/bitable/bas_fixture/clientvars" {
			http.NotFound(w, r)
			return
		}
		requested = true
		if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
			t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
		}
		if got := r.Header.Get("Origin"); got != serverOrigin(r) {
			t.Fatalf("Origin = %q, want %q", got, serverOrigin(r))
		}
		writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableClientVarsFixture(t)})
	}))
	defer server.Close()

	payload, err := Inspect(InspectConfig{
		URL:         server.URL + "/base/bas_fixture?table=tbl_main&view=vew_grid",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !requested {
		t.Fatal("bitable clientvars endpoint was not requested")
	}
	if payload["ok"] != true || payload["sourceKind"] != "direct_bitable" || payload["recordCount"] != 2 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestInspectHTTPFetchesWikiBitableClientVars(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	var wikiRequested bool
	var clientVarsRequested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wiki/wiki_fixture":
			wikiRequested = true
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><script>
				window.wiki_suite_type = 'bitable';
				current_space_wiki = Object({"obj_token":"bas_fixture"});
			</script></html>`))
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			clientVarsRequested = true
			if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
				t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableClientVarsFixture(t)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := Inspect(InspectConfig{
		URL:         server.URL + "/wiki/wiki_fixture",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !wikiRequested || !clientVarsRequested {
		t.Fatalf("wikiRequested=%t clientVarsRequested=%t", wikiRequested, clientVarsRequested)
	}
	if payload["ok"] != true || payload["sourceKind"] != "wiki_bitable" {
		t.Fatalf("payload = %+v", payload)
	}
	target := payload["target"].(map[string]any)
	if target["baseTokenPrefix"] != "bas" {
		t.Fatalf("target = %+v, want redacted base token prefix", target)
	}
}

func TestInspectHTTPFetchesDocxEmbeddedBitableClientVars(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	var docxRequested bool
	var clientVarsRequested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			docxRequested = true
			if got := r.URL.Query().Get("id"); got != "dox_fixture" {
				t.Fatalf("docx id = %q, want dox_fixture", got)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{
				"block_map": map[string]any{
					"dox_fixture": map[string]any{
						"data": map[string]any{
							"type":     "page",
							"children": []any{"blk_bitable"},
						},
					},
					"blk_bitable": map[string]any{
						"data": map[string]any{
							"type":       "bitable",
							"base_token": "bas_fixture",
							"table_id":   "tbl_main",
							"view_id":    "vew_grid",
						},
					},
				},
			}})
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			clientVarsRequested = true
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableClientVarsFixture(t)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := Inspect(InspectConfig{
		URL:         server.URL + "/docx/dox_fixture",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !docxRequested || !clientVarsRequested {
		t.Fatalf("docxRequested=%t clientVarsRequested=%t", docxRequested, clientVarsRequested)
	}
	if payload["ok"] != true || payload["sourceKind"] != "docx_embedded_bitable" {
		t.Fatalf("payload = %+v", payload)
	}
}

func bitableClientVarsFixture(t *testing.T) map[string]any {
	t.Helper()
	schema := map[string]any{
		"base": map[string]any{
			"name":     "Bug Tracker",
			"timezone": "Asia/Shanghai",
			"tables":   []any{"tbl_main"},
			"tableInfos": map[string]any{
				"tbl_main": map[string]any{"name": "Issues"},
			},
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
							"fields":  []any{"fld_title", "fld_module", "fld_image"},
							"records": []any{"rec_1", "rec_2"},
						},
					},
				},
				"fieldMap": map[string]any{
					"fld_title": map[string]any{
						"id":   "fld_title",
						"name": "Title",
						"type": float64(1),
					},
					"fld_module": map[string]any{
						"id":   "fld_module",
						"name": "Module",
						"type": float64(3),
						"property": map[string]any{
							"options": []any{
								map[string]any{"id": "opt_collab", "name": "Collab"},
							},
						},
					},
					"fld_image": map[string]any{
						"id":   "fld_image",
						"name": "Screenshot",
						"type": float64(17),
					},
				},
			},
			"recordMap": map[string]any{
				"rec_1": map[string]any{
					"fld_title":  map[string]any{"value": "Image bug"},
					"fld_module": map[string]any{"value": "opt_collab"},
					"fld_image":  map[string]any{"value": []any{}},
				},
				"rec_2": map[string]any{
					"fld_title":  map[string]any{"value": "Resume failed"},
					"fld_module": map[string]any{"value": "opt_collab"},
					"fld_image":  map[string]any{"value": []any{}},
				},
			},
		},
	}
	return map[string]any{
		"oldSchema": map[string]any{
			"gzipSchema": gzipBase64JSON(t, schema),
		},
		"timeZone": "Asia/Shanghai",
	}
}

func gzipBase64JSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		t.Fatalf("gzip fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip fixture: %v", err)
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes())
}

func containsPrivateToken(t *testing.T, value any, token string) bool {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return strings.Contains(string(raw), token)
}

func writeFixtureFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	return path
}

func writeCookieFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.json")
	raw := []byte(`[{"name":"_csrf_token","value":"csrf-fixture","domain":".example.test","path":"/"}]`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write cookie fixture: %v", err)
	}
	return path
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write JSON response: %v", err)
	}
}

func serverOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
