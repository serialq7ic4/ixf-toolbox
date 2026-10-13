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

// The described table comes from data.table.meta.id, the only place the real
// payload names it. The fixture describes tbl_archive while base.tables[0] is
// tbl_main, so a positional guess cannot pass this.
func TestParseClientVarsReadsDescribedTableFromMetaID(t *testing.T) {
	meta, err := ParseClientVars(bitableClientVarsFixtureDescribing(t, "tbl_archive"), "bas_fixture")
	if err != nil {
		t.Fatalf("ParseClientVars returned error: %v", err)
	}
	if got := describedTableID(meta); got != "tbl_archive" {
		t.Fatalf("describedTableID = %q, want tbl_archive from data.table.meta.id", got)
	}
	// The views in the payload belong to the described table, not to the first
	// table in the base.
	if len(meta.Views) != 1 || meta.Views[0].TableID != "tbl_archive" {
		t.Fatalf("views = %+v, want the described table", meta.Views)
	}
}

// With data.table.meta.id absent, the positional fallback still resolves a
// table rather than leaving the field empty.
func TestParseClientVarsFallsBackToFirstTableWhenMetaIDIsAbsent(t *testing.T) {
	meta, err := ParseClientVars(bitableClientVarsFixtureWithoutDescribedTableID(t), "bas_fixture")
	if err != nil {
		t.Fatalf("ParseClientVars returned error: %v", err)
	}
	if got := describedTableID(meta); got != "tbl_main" {
		t.Fatalf("describedTableID = %q, want the positional fallback tbl_main", got)
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

func TestAttachApplyUploadsAttachmentWritesSetRecordAndVerifies(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})

	clientVarsCalls := 0
	var sawPrepare bool
	var sawMerge bool
	var sawFinish bool
	var sawUserChanges bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			clientVarsCalls++
			data := bitableClientVarsFixture(t)
			if sawUserChanges {
				data = bitableClientVarsFixtureWithUpdatedAttachment(t, "rec_1", "ceph_logo.jpeg")
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": data})
		case "/space/api/box/upload/prepare/":
			sawPrepare = true
			request := decodeJSONRequest(t, r)
			if request["mount_point"] != "bitable_image" || request["mount_node_token"] != "bas_fixture" || request["name"] != "ceph_logo.jpeg" {
				t.Fatalf("prepare request = %#v", request)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{
				"upload_id":  "upload_fixture",
				"block_size": float64(8),
				"num_blocks": float64(1),
			}})
		case "/space/api/box/stream/upload/merge_block/":
			sawMerge = true
			if got := r.URL.Query().Get("upload_id"); got != "upload_fixture" {
				t.Fatalf("merge upload_id = %q", got)
			}
			if got := r.Header.Get("x-block-list-checksum"); got == "" {
				t.Fatal("merge checksum header was empty")
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"success_seq_list": []any{float64(0)}}})
		case "/space/api/box/upload/finish/":
			sawFinish = true
			request := decodeJSONRequest(t, r)
			if request["upload_id"] != "upload_fixture" || request["mount_point"] != "bitable_image" {
				t.Fatalf("finish request = %#v", request)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"file_token": "box_uploaded"}})
		case "/space/api/rce/messages":
			request := decodeJSONRequest(t, r)
			if request["type"] != "BITABLE_TABLE" {
				writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"type": "ACCEPT_WATCH"}})
				return
			}
			data := request["data"].(map[string]any)
			if data["type"] != "USER_CHANGES" || data["token"] != "tbl_main" || data["route_key"] != "bas_fixture" || data["content_type"] != "gzip/base64" {
				t.Fatalf("USER_CHANGES data = %#v", data)
			}
			operations := decodeGzipBase64String(t, data["operations"].(string))
			decoded := decodeOperations(t, operations)
			if len(decoded) != 1 || decoded[0]["command"] != "SetRecord" {
				t.Fatalf("operations = %s", operations)
			}
			actions := decoded[0]["actions"].([]any)
			if len(actions) != 1 {
				t.Fatalf("operation actions = %#v", actions)
			}
			action := actions[0].(map[string]any)
			if action["action"] != "data.setRecord" || action["tableId"] != "tbl_main" || action["viewId"] != "vew_grid" || action["recordId"] != "rec_1" {
				t.Fatalf("setRecord action = %#v", action)
			}
			cell := action["data"].(map[string]any)["fld_image"].(map[string]any)
			if int(cell["type"].(float64)) != attachmentFieldType {
				t.Fatalf("setRecord cell = %#v", cell)
			}
			values := cell["value"].([]any)
			if len(values) != 1 {
				t.Fatalf("setRecord attachment values = %#v", values)
			}
			attachment := values[0].(map[string]any)
			if attachment["attachmentToken"] != "box_uploaded" || attachment["name"] != "ceph_logo.jpeg" {
				t.Fatalf("setRecord attachment = %#v", attachment)
			}
			sawUserChanges = true
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"type": "ACCEPT_COMMIT"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := Attach(AttachConfig{
		URL:         server.URL + "/base/bas_fixture?table=tbl_main&view=vew_grid",
		Field:       "Screenshot",
		RecordMatch: "Title=Image bug",
		FilePath:    file,
		Apply:       true,
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("Attach apply returned error: %v", err)
	}
	if clientVarsCalls != 2 || !sawPrepare || !sawMerge || !sawFinish || !sawUserChanges {
		t.Fatalf("apply calls clientVars=%d prepare=%t merge=%t finish=%t userChanges=%t", clientVarsCalls, sawPrepare, sawMerge, sawFinish, sawUserChanges)
	}
	if payload["ok"] != true || payload["dryRun"] != false || payload["applied"] != true || payload["uploadedFileCount"] != 1 {
		t.Fatalf("apply payload = %+v", payload)
	}
	if payload["recordId"] != "rec_1" {
		t.Fatalf("recordId = %#v, want rec_1", payload["recordId"])
	}
	verify := payload["verify"].(map[string]any)
	if verify["ok"] != true || verify["recordId"] != "rec_1" || verify["recordIndex"] != 0 {
		t.Fatalf("verify payload = %+v", verify)
	}
}

func TestVerifyAttachedRecordRequiresUploadedAttachmentToken(t *testing.T) {
	data := bitableClientVarsFixtureWithUpdatedAttachmentToken(t, "rec_1", "ceph_logo.jpeg", "old_same_name_token")
	meta, err := ParseClientVars(data, "bas_fixture")
	if err != nil {
		t.Fatalf("ParseClientVars returned error: %v", err)
	}
	field := meta.FieldByName("Screenshot")
	if field == nil {
		t.Fatal("Screenshot field not found")
	}

	_, err = verifyAttachedRecord(meta, *field, "rec_1", []bitableUploadedFile{{
		Token: "box_uploaded",
		Name:  "ceph_logo.jpeg",
	}})
	if err == nil || !strings.Contains(err.Error(), "box_uploaded") {
		t.Fatalf("verifyAttachedRecord error = %v, want uploaded token mismatch", err)
	}
}

func TestRecordCreateDryRunPlansFieldsAndAttachments(t *testing.T) {
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})
	input := writeFixtureJSON(t, "row.json", map[string]any{
		"fields": map[string]any{
			"Title":      "New image bug",
			"Module":     "Collab",
			"Screenshot": map[string]any{"file": file},
		},
	})

	payload, err := RecordCreate(RecordCreateConfig{
		URL:        "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		InputPath:  input,
		DryRun:     true,
		ClientVars: bitableClientVarsFixture(t),
	})
	if err != nil {
		t.Fatalf("RecordCreate dry-run returned error: %v", err)
	}
	if payload["ok"] != true || payload["dryRun"] != true || payload["operation"] != "bitable_record_create" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["willCreateRecord"] != true || payload["plannedAttachmentCount"] != 1 || payload["fieldCount"] != 3 {
		t.Fatalf("record create plan = %+v", payload)
	}
	if payload["insertPosition"] != "bottom" || payload["plannedRecordIndex"] != 2 {
		t.Fatalf("record create insert plan = %+v", payload)
	}
	attachments := payload["attachments"].([]map[string]any)
	if len(attachments) != 1 || attachments[0]["fieldName"] != "Screenshot" {
		t.Fatalf("attachments = %+v", attachments)
	}
	fileInfo := attachments[0]["file"].(map[string]any)
	if fileInfo["name"] != "ceph_logo.jpeg" || fileInfo["mimeType"] != "image/jpeg" {
		t.Fatalf("attachment file metadata = %+v", fileInfo)
	}
}

func TestRecordCreateDryRunExpandsTildeAttachmentPaths(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	downloadDir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(downloadDir, "ceph_logo.jpeg"), []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	input := writeFixtureJSON(t, "row.json", map[string]any{
		"fields": map[string]any{
			"Title":      "New image bug",
			"Screenshot": map[string]any{"file": "~/Downloads/ceph_logo.jpeg"},
		},
	})

	payload, err := RecordCreate(RecordCreateConfig{
		URL:        "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		InputPath:  input,
		DryRun:     true,
		ClientVars: bitableClientVarsFixture(t),
	})
	if err != nil {
		t.Fatalf("RecordCreate dry-run returned error: %v", err)
	}
	attachments := payload["attachments"].([]map[string]any)
	fileInfo := attachments[0]["file"].(map[string]any)
	if fileInfo["name"] != "ceph_logo.jpeg" || fileInfo["mimeType"] != "image/jpeg" {
		t.Fatalf("attachment file metadata = %+v", fileInfo)
	}
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	volume := filepath.VolumeName(home)
	if volume == "" {
		volume = filepath.VolumeName(filepath.Clean(home))
	}
	t.Setenv("HOMEDRIVE", volume)
	t.Setenv("HOMEPATH", strings.TrimPrefix(home, volume))
}

func TestRecordCreateDryRunRejectsUnknownField(t *testing.T) {
	input := writeFixtureJSON(t, "row.json", map[string]any{
		"fields": map[string]any{
			"Missing": "value",
		},
	})

	_, err := RecordCreate(RecordCreateConfig{
		URL:        "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		InputPath:  input,
		DryRun:     true,
		ClientVars: bitableClientVarsFixture(t),
	})
	if err == nil || !strings.Contains(err.Error(), "field \"Missing\" was not found") {
		t.Fatalf("RecordCreate unknown field error = %v", err)
	}
}

func TestRecordCreateApplyUploadsAttachmentsWritesRecordAndVerifies(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})
	input := writeFixtureJSON(t, "row.json", map[string]any{
		"fields": map[string]any{
			"Title":      "New image bug",
			"Screenshot": map[string]any{"file": file},
		},
	})

	clientVarsCalls := 0
	var sawPrepare bool
	var sawMerge bool
	var sawFinish bool
	var sawAddRecordToken bool
	var sawUserChanges bool
	createdRecordID := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			clientVarsCalls++
			// Every fetch carries the scope, the verification refetch included:
			// verifying against another table's records would prove nothing.
			if got := r.URL.Query().Get("tableID"); got != "tbl_main" {
				t.Fatalf("clientvars call %d tableID = %q, want tbl_main", clientVarsCalls, got)
			}
			data := bitableClientVarsFixture(t)
			if sawUserChanges {
				data = bitableClientVarsFixtureWithCreatedRecord(t, createdRecordID, "New image bug", "ceph_logo.jpeg")
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": data})
		case "/space/api/bitable/bas_fixture/add_record/token":
			sawAddRecordToken = true
			request := decodeJSONRequest(t, r)
			if request["tableID"] != "tbl_main" {
				t.Fatalf("add_record token tableID = %#v", request["tableID"])
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"addRecordToken": "token_fixture"}})
		case "/space/api/box/upload/prepare/":
			sawPrepare = true
			request := decodeJSONRequest(t, r)
			if request["mount_point"] != "bitable_image" || request["mount_node_token"] != "bas_fixture" || request["name"] != "ceph_logo.jpeg" {
				t.Fatalf("prepare request = %#v", request)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{
				"upload_id":  "upload_fixture",
				"block_size": float64(8),
				"num_blocks": float64(1),
			}})
		case "/space/api/box/stream/upload/merge_block/":
			sawMerge = true
			if got := r.URL.Query().Get("upload_id"); got != "upload_fixture" {
				t.Fatalf("merge upload_id = %q", got)
			}
			if got := r.URL.Query().Get("mount_point"); got != "bitable_image" {
				t.Fatalf("merge mount_point = %q", got)
			}
			if got := r.Header.Get("x-seq-list"); got != "0" {
				t.Fatalf("merge seq header = %q", got)
			}
			if got := r.Header.Get("x-block-list-checksum"); got == "" {
				t.Fatal("merge checksum header was empty")
			}
			if got := r.Header.Get("x-block-origin-size"); got != "8" {
				t.Fatalf("merge block origin size = %q", got)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"success_seq_list": []any{float64(0)}}})
		case "/space/api/box/upload/finish/":
			sawFinish = true
			request := decodeJSONRequest(t, r)
			if request["upload_id"] != "upload_fixture" || request["mount_point"] != "bitable_image" {
				t.Fatalf("finish request = %#v", request)
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"file_token": "box_uploaded"}})
		case "/space/api/rce/messages":
			request := decodeJSONRequest(t, r)
			if request["type"] != "BITABLE_TABLE" {
				writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"type": "ACCEPT_WATCH"}})
				return
			}
			data := request["data"].(map[string]any)
			if data["type"] != "USER_CHANGES" || data["token"] != "tbl_main" || data["route_key"] != "bas_fixture" || data["content_type"] != "gzip/base64" {
				t.Fatalf("USER_CHANGES data = %#v", data)
			}
			operations := decodeGzipBase64String(t, data["operations"].(string))
			if !strings.Contains(operations, "New image bug") || !strings.Contains(operations, "box_uploaded") || !strings.Contains(operations, "ceph_logo.jpeg") {
				t.Fatalf("USER_CHANGES operations missing text or attachment: %s", operations)
			}
			if index := recordIndexFromOperations(t, operations, "vew_grid"); index != 2 {
				t.Fatalf("record index = %d, want 2 for bottom append; operations=%s", index, operations)
			}
			createdRecordID = recordIDFromOperations(t, operations)
			sawUserChanges = true
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"type": "ACCEPT_COMMIT"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := RecordCreate(RecordCreateConfig{
		URL:         server.URL + "/base/bas_fixture?table=tbl_main&view=vew_grid",
		InputPath:   input,
		Apply:       true,
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("RecordCreate apply returned error: %v", err)
	}
	if !sawPrepare || !sawMerge || !sawFinish || !sawAddRecordToken || !sawUserChanges {
		t.Fatalf("apply calls prepare=%t merge=%t finish=%t addToken=%t userChanges=%t", sawPrepare, sawMerge, sawFinish, sawAddRecordToken, sawUserChanges)
	}
	if clientVarsCalls != 2 {
		t.Fatalf("clientVarsCalls = %d, want 2", clientVarsCalls)
	}
	if payload["ok"] != true || payload["dryRun"] != false || payload["applied"] != true || payload["uploadedFileCount"] != 1 {
		t.Fatalf("apply payload = %+v", payload)
	}
	if payload["insertPosition"] != "bottom" || payload["plannedRecordIndex"] != 2 {
		t.Fatalf("apply insert plan = %+v", payload)
	}
	verify := payload["verify"].(map[string]any)
	if verify["ok"] != true {
		t.Fatalf("verify payload = %+v", verify)
	}
	if verify["recordIndex"] != 2 || verify["expectedRecordIndex"] != 2 {
		t.Fatalf("verify record index = %+v", verify)
	}
}

func TestRecordCreateApplyRejectsUnsupportedFieldTypes(t *testing.T) {
	input := writeFixtureJSON(t, "row.json", map[string]any{
		"fields": map[string]any{
			"Module": "Collab",
		},
	})

	_, err := RecordCreate(RecordCreateConfig{
		URL:        "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
		InputPath:  input,
		Apply:      true,
		ClientVars: bitableClientVarsFixture(t),
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported apply field type "single_select" for field "Module"`) {
		t.Fatalf("unsupported field error = %v", err)
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
		// The fetch is scoped to the requested table, so the metadata, the
		// guard, and any write all describe one table.
		if got := r.URL.Query().Get("tableID"); got != "tbl_main" {
			t.Fatalf("clientvars tableID = %q, want tbl_main", got)
		}
		// viewID stays empty: the response does not vary by view, so sending it
		// buys nothing. See the comment in session.clientVars.
		if got := r.URL.Query().Get("viewID"); got != "" {
			t.Fatalf("clientvars viewID = %q, want it left empty", got)
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

// A ?table= naming a non-default table is served, not refused. The fixture
// server honours tableID the way the live server does, so the payload describes
// the requested table and the guard has nothing to object to.
func TestInspectHTTPServesSecondTableWhenServerHonoursRequestedScope(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	var gotTableID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/v1/bitable/bas_fixture/clientvars" {
			http.NotFound(w, r)
			return
		}
		gotTableID = r.URL.Query().Get("tableID")
		described := gotTableID
		if described == "" {
			described = "tbl_main"
		}
		writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableClientVarsFixtureDescribing(t, described)})
	}))
	defer server.Close()

	payload, err := Inspect(InspectConfig{
		URL:         server.URL + "/base/bas_fixture?table=tbl_archive&view=vew_grid",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if gotTableID != "tbl_archive" {
		t.Fatalf("clientvars tableID = %q, want tbl_archive", gotTableID)
	}
	target := payload["target"].(map[string]any)
	if target["tableId"] != "tbl_archive" {
		t.Fatalf("target = %+v, want the requested table reported", target)
	}
}

// With no ?table=, nothing is requested, so the parameter goes out empty and the
// server picks the base's default table.
func TestInspectHTTPSendsEmptyTableIDWhenNoTableWasRequested(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	sawTableIDParam := false
	gotTableID := "unset"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/v1/bitable/bas_fixture/clientvars" {
			http.NotFound(w, r)
			return
		}
		_, sawTableIDParam = r.URL.Query()["tableID"]
		gotTableID = r.URL.Query().Get("tableID")
		writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableClientVarsFixture(t)})
	}))
	defer server.Close()

	if _, err := Inspect(InspectConfig{
		URL:         server.URL + "/base/bas_fixture",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	}); err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if !sawTableIDParam || gotTableID != "" {
		t.Fatalf("tableID present=%t value=%q, want the parameter sent empty", sawTableIDParam, gotTableID)
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

// The guard covers one thing: the fetched metadata describing a table other
// than the one the caller asked for. The fetch is now scoped to the requested
// table, so a table the server honours is accepted -- including a non-default
// one, which v3.30.0 refused. A mismatch still has to be refused, because field
// names would be validated against the table that came back while the write
// targeted the requested one.
func TestCommandsRefuseOnlyWhenMetadataDescribesAnotherTable(t *testing.T) {
	file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})
	input := writeFixtureJSON(t, "record.json", map[string]any{"Title": "New issue"})

	testCases := []struct {
		name        string
		url         string
		clientVars  func(*testing.T) map[string]any
		wantTableID string
		wantRefused bool
		wantInError []string
	}{
		{
			name:        "table matching fetched metadata is accepted",
			url:         "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
			clientVars:  bitableClientVarsFixture,
			wantTableID: "tbl_main",
		},
		{
			name:        "no table parameter uses the metadata table",
			url:         "https://tenant.example/base/bas_fixture?view=vew_grid",
			clientVars:  bitableClientVarsFixture,
			wantTableID: "tbl_main",
		},
		{
			// The scoped fetch came back describing tbl_archive, so operating
			// on it is coherent: fields, records, and the write all name it.
			name: "non-default table the server described is accepted",
			url:  "https://tenant.example/base/bas_fixture?table=tbl_archive&view=vew_grid",
			clientVars: func(t *testing.T) map[string]any {
				return bitableClientVarsFixtureDescribing(t, "tbl_archive")
			},
			wantTableID: "tbl_archive",
		},
		{
			name:        "table absent from the base is refused",
			url:         "https://tenant.example/base/bas_fixture?table=tbl_ghost&view=vew_grid",
			clientVars:  bitableClientVarsFixture,
			wantRefused: true,
			wantInError: []string{"tbl_ghost", "tbl_main", "table mismatch", "not one of this base's tables"},
		},
		{
			// tbl_archive exists, but the payload describes tbl_main, so the
			// server did not honour the scope. Still refused, and the message
			// says so rather than blaming the caller's URL.
			name:        "table the server did not honour is refused",
			url:         "https://tenant.example/base/bas_fixture?table=tbl_archive&view=vew_grid",
			clientVars:  bitableClientVarsFixtureWithSecondTable,
			wantRefused: true,
			wantInError: []string{"tbl_archive", "tbl_main", "table mismatch", "did not honour the requested scope"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Every command shares one prologue; assert all three, and assert
			// the dry-run paths refuse rather than reporting a clean plan.
			commands := map[string]func() (map[string]any, error){
				"inspect": func() (map[string]any, error) {
					return Inspect(InspectConfig{URL: testCase.url, ClientVars: testCase.clientVars(t)})
				},
				"attach dry-run": func() (map[string]any, error) {
					return Attach(AttachConfig{
						URL:         testCase.url,
						Field:       "Screenshot",
						RecordMatch: "Title=Image bug",
						FilePath:    file,
						DryRun:      true,
						ClientVars:  testCase.clientVars(t),
					})
				},
				"record create dry-run": func() (map[string]any, error) {
					return RecordCreate(RecordCreateConfig{
						URL:        testCase.url,
						InputPath:  input,
						DryRun:     true,
						ClientVars: testCase.clientVars(t),
					})
				},
			}
			for label, run := range commands {
				payload, err := run()
				if !testCase.wantRefused {
					if err != nil {
						t.Fatalf("%s returned error: %v", label, err)
					}
					if payload["ok"] != true {
						t.Fatalf("%s payload = %+v, want ok", label, payload)
					}
					target := payload["target"].(map[string]any)
					if target["tableId"] != testCase.wantTableID {
						t.Fatalf("%s target = %+v, want tableId %s", label, target, testCase.wantTableID)
					}
					continue
				}
				if err == nil {
					t.Fatalf("%s accepted a mismatched table and returned %+v", label, payload)
				}
				for _, want := range testCase.wantInError {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("%s error = %q, want it to mention %q", label, err.Error(), want)
					}
				}
			}
		})
	}
}

func TestRecordCreateApplyRefusesMismatchedTableBeforeAnyRequest(t *testing.T) {
	input := writeFixtureJSON(t, "record.json", map[string]any{"Title": "New issue"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("apply issued a request to %s despite a mismatched table", r.URL.Path)
	}))
	defer server.Close()

	_, err := RecordCreate(RecordCreateConfig{
		URL:         server.URL + "/base/bas_fixture?table=tbl_ghost&view=vew_grid",
		InputPath:   input,
		Apply:       true,
		CookiesPath: writeCookieFixture(t),
		SpaceAPI:    server.URL,
		ClientVars:  bitableClientVarsFixture(t),
	})
	if err == nil || !strings.Contains(err.Error(), "tbl_ghost") {
		t.Fatalf("RecordCreate apply error = %v, want refusal naming tbl_ghost", err)
	}
}

// An embedded docx bitable populates Source.TableID from block discovery, not
// from a user query string. Scoping the fetch to that discovered table is what
// makes the path coherent: before, the metadata came from the base's default
// table while a write would have targeted the embedded one.
//
// Source.tableIDFromQuery still separates the two provenances, so the guard
// stays off here: a discovered table is not a caller assertion, and the
// embedded block can legitimately name a table the base does not default to.
func TestInspectHTTPScopesDocxDiscoveredTableAndDoesNotRefuseIt(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	var gotTableID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{
				"block_map": map[string]any{
					"dox_fixture": map[string]any{
						"data": map[string]any{"type": "page", "children": []any{"blk_bitable"}},
					},
					"blk_bitable": map[string]any{
						"data": map[string]any{
							"type":       "bitable",
							"base_token": "bas_fixture",
							"table_id":   "tbl_embedded",
							"view_id":    "vew_embedded",
						},
					},
				},
			}})
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			gotTableID = r.URL.Query().Get("tableID")
			// Answer the way the live server does: describe the table the
			// request scoped to.
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": bitableClientVarsFixtureDescribing(t, gotTableID)})
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
	if gotTableID != "tbl_embedded" {
		t.Fatalf("clientvars tableID = %q, want the docx-discovered tbl_embedded", gotTableID)
	}
	if payload["ok"] != true || payload["sourceKind"] != "docx_embedded_bitable" {
		t.Fatalf("payload = %+v", payload)
	}
}

// The guard is off for a docx-discovered table, so a server that answers with a
// different table than the embedded block named is served rather than refused.
// This records that gap deliberately: a write here would validate field names
// against the table that came back and POST the embedded table id. Scoping is
// what keeps it from happening in practice, and tableIDFromQuery is what keeps
// the refusal limited to a caller's own assertion.
func TestInspectHTTPDoesNotRefuseDocxDiscoveredTableTheServerDidNotDescribe(t *testing.T) {
	cookiesPath := writeCookieFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{
				"block_map": map[string]any{
					"dox_fixture": map[string]any{
						"data": map[string]any{"type": "page", "children": []any{"blk_bitable"}},
					},
					"blk_bitable": map[string]any{
						"data": map[string]any{
							"type":       "bitable",
							"base_token": "bas_fixture",
							"table_id":   "tbl_embedded",
						},
					},
				},
			}})
		case "/space/api/v1/bitable/bas_fixture/clientvars":
			// Ignore the scope and answer with the base's default table.
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
	if payload["sourceKind"] != "docx_embedded_bitable" {
		t.Fatalf("payload = %+v", payload)
	}
}

// bitableClientVarsFixtureWithSecondTable is a two-table base whose payload
// still describes tbl_main, the first table. It stands for a server that
// answered with a table other than the one the request scoped to.
func bitableClientVarsFixtureWithSecondTable(t *testing.T) map[string]any {
	t.Helper()
	return bitableClientVarsFixtureDescribing(t, "tbl_main")
}

// bitableClientVarsFixtureDescribing is a two-table base -- tbl_main then
// tbl_archive -- whose payload describes describedTableID. Naming tbl_archive
// makes the described table differ from the positional first table, which is
// what distinguishes reading data.table.meta.id from guessing base.tables[0].
func bitableClientVarsFixtureDescribing(t *testing.T, describedTableID string) map[string]any {
	t.Helper()
	data := bitableClientVarsFixture(t)
	schema := decodeFixtureSchema(t, data)
	base := schema["base"].(map[string]any)
	base["tables"] = []any{"tbl_main", "tbl_archive"}
	base["tableInfos"].(map[string]any)["tbl_archive"] = map[string]any{"name": "Archive"}
	table := schema["data"].(map[string]any)["table"].(map[string]any)
	table["meta"].(map[string]any)["id"] = describedTableID
	data["oldSchema"].(map[string]any)["gzipSchema"] = gzipBase64JSON(t, schema)
	return data
}

// bitableClientVarsFixtureWithoutDescribedTableID drops data.table.meta.id so
// the positional fallback is the only thing left to resolve the table.
func bitableClientVarsFixtureWithoutDescribedTableID(t *testing.T) map[string]any {
	t.Helper()
	data := bitableClientVarsFixtureDescribing(t, "tbl_archive")
	schema := decodeFixtureSchema(t, data)
	table := schema["data"].(map[string]any)["table"].(map[string]any)
	delete(table["meta"].(map[string]any), "id")
	data["oldSchema"].(map[string]any)["gzipSchema"] = gzipBase64JSON(t, schema)
	return data
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
				// The real payload names the described table at
				// data.table.meta.id, alongside rev. There is no data.table.id.
				"meta":  map[string]any{"id": "tbl_main", "rev": float64(7)},
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

func bitableClientVarsFixtureWithCreatedRecord(t *testing.T, recordID string, title string, attachmentName string) map[string]any {
	t.Helper()
	if recordID == "" {
		t.Fatal("created record id was not captured")
	}
	data := bitableClientVarsFixture(t)
	schema := decodeFixtureSchema(t, data)
	table := schema["data"].(map[string]any)["table"].(map[string]any)
	view := table["viewMap"].(map[string]any)["vew_grid"].(map[string]any)
	property := view["property"].(map[string]any)
	property["records"] = []any{"rec_1", "rec_2", recordID}
	recordMap := schema["data"].(map[string]any)["recordMap"].(map[string]any)
	recordMap[recordID] = map[string]any{
		"fld_title": map[string]any{"value": []any{map[string]any{"type": "text", "text": title}}},
		"fld_image": map[string]any{"value": []any{map[string]any{
			"id":              "box_uploaded",
			"attachmentToken": "box_uploaded",
			"name":            attachmentName,
			"mimeType":        "image/jpeg",
			"size":            float64(7),
			"timeStamp":       float64(1786718860252),
		}}},
	}
	data["oldSchema"].(map[string]any)["gzipSchema"] = gzipBase64JSON(t, schema)
	return data
}

func bitableClientVarsFixtureWithUpdatedAttachment(t *testing.T, recordID string, attachmentName string) map[string]any {
	return bitableClientVarsFixtureWithUpdatedAttachmentToken(t, recordID, attachmentName, "box_uploaded")
}

func bitableClientVarsFixtureWithUpdatedAttachmentToken(t *testing.T, recordID string, attachmentName string, attachmentToken string) map[string]any {
	t.Helper()
	data := bitableClientVarsFixture(t)
	schema := decodeFixtureSchema(t, data)
	recordMap := schema["data"].(map[string]any)["recordMap"].(map[string]any)
	record := recordMap[recordID].(map[string]any)
	record["fld_image"] = map[string]any{"value": []any{map[string]any{
		"id":              attachmentToken,
		"attachmentToken": attachmentToken,
		"name":            attachmentName,
		"mimeType":        "image/jpeg",
		"size":            float64(7),
		"timeStamp":       float64(1786718860252),
	}}}
	data["oldSchema"].(map[string]any)["gzipSchema"] = gzipBase64JSON(t, schema)
	return data
}

func decodeFixtureSchema(t *testing.T, data map[string]any) map[string]any {
	t.Helper()
	raw := decodeGzipBase64String(t, data["oldSchema"].(map[string]any)["gzipSchema"].(string))
	var schema map[string]any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return schema
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

func decodeGzipBase64String(t *testing.T, value string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	defer reader.Close()
	var decoded bytes.Buffer
	if _, err := decoded.ReadFrom(reader); err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	return decoded.String()
}

func decodeOperations(t *testing.T, operations string) []map[string]any {
	t.Helper()
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(operations), &decoded); err != nil {
		t.Fatalf("decode operations: %v", err)
	}
	return decoded
}

func decodeJSONRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatalf("decode request JSON: %v", err)
	}
	return payload
}

func recordIDFromOperations(t *testing.T, operations string) string {
	t.Helper()
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(operations), &decoded); err != nil {
		t.Fatalf("decode operations: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("operations were empty")
	}
	actions := decoded[0]["actions"].([]any)
	if len(actions) == 0 {
		t.Fatal("operation actions were empty")
	}
	recordID := actions[0].(map[string]any)["recordId"].(string)
	if recordID == "" {
		t.Fatal("recordId was empty")
	}
	return recordID
}

func recordIndexFromOperations(t *testing.T, operations string, viewID string) int {
	t.Helper()
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(operations), &decoded); err != nil {
		t.Fatalf("decode operations: %v", err)
	}
	actions := decoded[0]["actions"].([]any)
	action := actions[0].(map[string]any)
	data := action["data"].(map[string]any)
	indexes := data["indexes"].(map[string]any)
	return int(indexes[viewID].(float64))
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

func writeFixtureJSON(t *testing.T, name string, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal fixture JSON: %v", err)
	}
	return writeFixtureFile(t, name, raw)
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
