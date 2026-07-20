package sheets

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTargetRequiresDirectSheetURL(t *testing.T) {
	target, err := ParseTarget("https://tenant.example.test/sheets/shtr_fixture?sheet=sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if target.BaseURL != "https://tenant.example.test" || target.WorkbookToken != "shtr_fixture" || target.SheetID != "sheet1" {
		t.Fatalf("target = %+v", target)
	}

	if _, err := ParseTarget("https://tenant.example.test/docx/dox_fixture"); err == nil {
		t.Fatal("ParseTarget accepted non-sheets URL")
	}
	if _, err := ParseTarget("https://tenant.example.test/sheets/shtr_fixture"); err == nil {
		t.Fatal("ParseTarget accepted missing sheet query")
	}
}

func TestPlanUpdateDryRunReportsTSVShape(t *testing.T) {
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "cells.tsv")
	if err := os.WriteFile(input, []byte("A\tB\n1\t2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := PlanUpdate(UpdateConfig{
		URL:       "https://tenant.example.test/sheets/shtr_fixture?sheet=sheet1",
		Range:     "b2",
		InputPath: input,
		DryRun:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["range"] != "B2" || payload["rows"] != 2 || payload["cols"] != 2 || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPlanUpdateApplyIsExplicitlyUnavailable(t *testing.T) {
	_, err := PlanUpdate(UpdateConfig{
		URL:       "https://tenant.example.test/sheets/shtr_fixture?sheet=sheet1",
		Range:     "A1",
		InputPath: "cells.tsv",
		Apply:     true,
	})
	if err == nil {
		t.Fatal("PlanUpdate accepted --apply without a write API contract")
	}
}

func TestBuildUserChangesContentEncodesCapturedCellShape(t *testing.T) {
	content, err := buildUserChangesContent([]cellUpdate{{
		SheetID: "QpGnfU",
		Row:     1,
		Col:     0,
		Value:   "Hello",
	}})
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeGzipBase64(t, content)
	if decoded[0] != 0x0a {
		t.Fatalf("content starts with %#x, want protobuf field 1", decoded[0])
	}
	innerLength, innerOffset := readTestVarint(t, decoded, 1)
	inner := decoded[innerOffset : innerOffset+int(innerLength)]
	if !bytes.HasPrefix(inner, []byte{0x08, 0x04, 0x12, 0x06, 'Q', 'p', 'G', 'n', 'f', 'U', 0x22, 0x04, 0x08, 0x01, 0x18, 0x00, 0x2a}) {
		t.Fatalf("inner protobuf prefix = %x", inner[:min(len(inner), 24)])
	}
	if !strings.Contains(string(inner), `"value":"Hello"`) {
		t.Fatalf("inner protobuf JSON missing cell value: %q", string(inner))
	}
}

func TestUpdateApplyPostsUserChangesAndVerifiesReadback(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"csrf-fixture"},{"name":"session","value":"session-fixture"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(tmpDir, "cells.tsv")
	if err := os.WriteFile(input, []byte("New"), 0o644); err != nil {
		t.Fatal(err)
	}

	clientVarsCalls := 0
	var sawUserChanges bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v3/sheet/client_vars":
			clientVarsCalls++
			if got := r.URL.Query().Get("synced_block_host_token"); got != "dox_host" {
				t.Fatalf("synced host token = %q, want dox_host", got)
			}
			if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
				t.Fatalf("csrf header = %q", got)
			}
			request := decodeJSONRequest(t, r)
			if request["token"] != "shtr_fixture" {
				t.Fatalf("client_vars token = %#v", request["token"])
			}
			value := "Old"
			if sawUserChanges {
				value = "New"
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetApplyFixtureData(t, "sheet1", value)})
		case "/space/api/v2/sheet/user_changes":
			sawUserChanges = true
			if r.Method != http.MethodPost {
				t.Fatalf("user_changes method = %s, want POST", r.Method)
			}
			if got := r.URL.Query().Get("token"); got != "shtr_fixture" {
				t.Fatalf("user_changes token = %q", got)
			}
			if got := r.URL.Query().Get("member_id"); got != "12345" {
				t.Fatalf("user_changes member_id = %q", got)
			}
			if got := r.URL.Query().Get("synced_block_host_token"); got != "dox_host" {
				t.Fatalf("user_changes host token = %q", got)
			}
			body := decodeJSONRequest(t, r)
			if body["base_rev"] != float64(7) || body["mode"] != float64(0) || body["retryCount"] != float64(0) {
				t.Fatalf("user_changes body metadata = %#v", body)
			}
			content, ok := body["content"].(string)
			if !ok || content == "" {
				t.Fatalf("user_changes content = %#v", body["content"])
			}
			decoded := decodeGzipBase64(t, content)
			if !strings.Contains(string(decoded), `"value":"New"`) {
				t.Fatalf("decoded user_changes content missing value: %q", string(decoded))
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"revision": 8}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := Update(UpdateConfig{
		URL:         server.URL + "/sheets/shtr_fixture?sheet=sheet1",
		HostURL:     server.URL + "/docx/dox_host",
		Range:       "B2",
		InputPath:   input,
		Apply:       true,
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawUserChanges {
		t.Fatal("user_changes endpoint was not called")
	}
	if clientVarsCalls != 2 {
		t.Fatalf("client_vars calls = %d, want 2", clientVarsCalls)
	}
	if payload["ok"] != true || payload["dryRun"] != false || payload["willWrite"] != true {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestUpdateApplyPollsReadbackUntilSheetChangeIsVisible(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"csrf-fixture"},{"name":"session","value":"session-fixture"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(tmpDir, "cells.tsv")
	if err := os.WriteFile(input, []byte("New"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldPollInterval := sheetVerifyPollInterval
	oldTimeout := sheetVerifyTimeout
	sheetVerifyPollInterval = 0
	sheetVerifyTimeout = time.Second
	defer func() {
		sheetVerifyPollInterval = oldPollInterval
		sheetVerifyTimeout = oldTimeout
	}()

	var sawUserChanges bool
	postWriteReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v3/sheet/client_vars":
			value := "Old"
			if sawUserChanges {
				postWriteReads++
				if postWriteReads >= 3 {
					value = "New"
				}
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetApplyFixtureData(t, "sheet1", value)})
		case "/space/api/v2/sheet/user_changes":
			sawUserChanges = true
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"revision": 8}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := Update(UpdateConfig{
		URL:         server.URL + "/sheets/shtr_fixture?sheet=sheet1",
		HostURL:     server.URL + "/docx/dox_host",
		Range:       "B2",
		InputPath:   input,
		Apply:       true,
		CookiesPath: cookiesPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	if postWriteReads != 3 {
		t.Fatalf("post-write reads = %d, want 3", postWriteReads)
	}
}

func TestUpdateApplyUsesConfiguredSpaceAPIEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"csrf-fixture"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(tmpDir, "cells.tsv")
	if err := os.WriteFile(input, []byte("New"), 0o644); err != nil {
		t.Fatal(err)
	}

	targetServerCalled := false
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetServerCalled = true
		http.Error(w, "target origin should not receive API traffic", http.StatusBadGateway)
	}))
	defer targetServer.Close()

	var apiServer *httptest.Server
	sawUserChanges := false
	apiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v3/sheet/client_vars":
			if got := r.Header.Get("Origin"); got != targetServer.URL {
				t.Fatalf("origin = %q, want target origin", got)
			}
			value := "Old"
			if sawUserChanges {
				value = "New"
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": sheetApplyFixtureData(t, "sheet1", value)})
		case "/space/api/v2/sheet/user_changes":
			sawUserChanges = true
			writeJSONResponse(t, w, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	if _, err := Update(UpdateConfig{
		URL:         targetServer.URL + "/sheets/shtr_fixture?sheet=sheet1",
		HostURL:     targetServer.URL + "/docx/dox_host",
		Range:       "B2",
		InputPath:   input,
		Apply:       true,
		CookiesPath: cookiesPath,
		SpaceAPI:    apiServer.URL,
	}); err != nil {
		t.Fatal(err)
	}
	if targetServerCalled {
		t.Fatal("sheet apply ignored SpaceAPI and called the target origin")
	}
	if !sawUserChanges {
		t.Fatal("sheet apply did not call API user_changes")
	}
}

func TestUpdateApplyRequiresMemberIDBeforePostingChanges(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[{"name":"_csrf_token","value":"csrf-fixture"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(tmpDir, "cells.tsv")
	if err := os.WriteFile(input, []byte("New"), 0o644); err != nil {
		t.Fatal(err)
	}

	sawUserChanges := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v3/sheet/client_vars":
			data := sheetApplyFixtureData(t, "sheet1", "Old")
			delete(data["formerlySchema"].(map[string]any), "member_id")
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": data})
		case "/space/api/v2/sheet/user_changes":
			sawUserChanges = true
			writeJSONResponse(t, w, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := Update(UpdateConfig{
		URL:         server.URL + "/sheets/shtr_fixture?sheet=sheet1",
		HostURL:     server.URL + "/docx/dox_host",
		Range:       "B2",
		InputPath:   input,
		Apply:       true,
		CookiesPath: cookiesPath,
	})
	if err == nil || !strings.Contains(err.Error(), "member identifier") {
		t.Fatalf("err = %v, want missing member identifier", err)
	}
	if sawUserChanges {
		t.Fatal("sheet apply posted user_changes without member_id")
	}
}

func decodeGzipBase64(t *testing.T, encoded string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func readTestVarint(t *testing.T, data []byte, offset int) (uint64, int) {
	t.Helper()
	var result uint64
	var shift uint
	for index := offset; index < len(data); index++ {
		b := data[index]
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, index + 1
		}
		shift += 7
	}
	t.Fatal("unterminated varint")
	return 0, 0
}

func decodeJSONRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode JSON request: %v; body=%s", err, body)
	}
	return payload
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func sheetApplyFixtureData(t *testing.T, sheetID string, b2 string) map[string]any {
	t.Helper()
	return map[string]any{
		"formerlySchema": map[string]any{
			"member_id": float64(12345),
			"clientvars": map[string]any{
				"version":  float64(7),
				"revision": float64(7),
				"gzip_snapshot": gzipTestJSON(t, map[string]any{
					"sheets": map[string]any{
						sheetID: map[string]any{"rowCount": 2, "columnCount": 2},
					},
				}),
				"extra_data": map[string]any{
					"blocks": []any{
						map[string]any{
							"row": float64(0),
							"gzip_datatable": gzipTestJSON(t, map[string]any{
								"rows": []any{
									map[string]any{"columns": []any{
										map[string]any{"value": "Name"},
										map[string]any{"value": "Value"},
									}},
									map[string]any{"columns": []any{
										map[string]any{"value": "Alpha"},
										map[string]any{"value": b2},
									}},
								},
							}),
						},
					},
				},
			},
		},
	}
}

func gzipTestJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}
