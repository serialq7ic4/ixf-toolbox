package docspublish

import (
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendTableRowDryRunPlansNativeTableRowWithImage(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := writePatchCookieFixture(t)
	imagePath := filepath.Join(tmpDir, "ceph-logo.png")
	writePNGFixture(t, imagePath)
	inputPath := filepath.Join(tmpDir, "row.json")
	writeJSONFixture(t, inputPath, map[string]any{"fields": map[string]any{
		"Title": "Dry-run row",
		"Logo":  map[string]any{"file": imagePath},
	}})

	var sawMutation bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			writeTestJSON(t, w, map[string]any{"code": 0, "data": nativeTableClientVars("", "", false)})
		default:
			if r.Method == http.MethodPost || strings.Contains(r.URL.Path, "upload") {
				sawMutation = true
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := AppendTableRow(TableAppendRowConfig{
		URL:         server.URL + "/docx/page_1",
		InputPath:   inputPath,
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
		Apply:       false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sawMutation {
		t.Fatal("dry-run performed mutation")
	}
	for key, want := range map[string]any{
		"ok":                 true,
		"dryRun":             true,
		"operation":          "docs_table_append_row",
		"willWrite":          false,
		"tableCount":         1,
		"tableIndex":         1,
		"currentRowCount":    1,
		"columnCount":        2,
		"plannedCellCount":   2,
		"plannedTextCount":   1,
		"plannedImageCount":  1,
		"requiredTextChecks": 1,
	} {
		if payload[key] != want {
			t.Fatalf("payload[%s] = %#v, want %#v; payload=%+v", key, payload[key], want, payload)
		}
	}
	headers := payload["headers"].([]string)
	if strings.Join(headers, ",") != "Title,Logo" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestAppendTableRowApplyAddsCellsUploadsImageAndVerifies(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := writePatchCookieFixture(t)
	imagePath := filepath.Join(tmpDir, "ceph-logo.png")
	writePNGFixture(t, imagePath)
	inputPath := filepath.Join(tmpDir, "row.json")
	writeJSONFixture(t, inputPath, map[string]any{"fields": map[string]any{
		"Title": "Applied table row",
		"Logo":  map[string]any{"file": imagePath},
	}})

	wroteRow := false
	boundImage := false
	newRowID := ""
	newTextBlockID := ""
	newImageBlockID := ""
	newCellIDs := map[string]bool{}
	uploadedName := ""
	uploadedMountPoint := ""
	uploadedMountNodeToken := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			writeTestJSON(t, w, map[string]any{
				"code": 0,
				"data": nativeTableClientVars(newTextBlockID, newImageBlockID, boundImage),
			})
		case "/space/api/docx/blocks/user_change/":
			payload := decodeDocspublishJSONRequest(t, r)
			raw, err := json.Marshal(payload["change_map"])
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if strings.Contains(text, `"p":["image"]`) {
				if uploadedMountNodeToken != newImageBlockID {
					t.Fatalf("image upload mount_node_token = %q, want %q", uploadedMountNodeToken, newImageBlockID)
				}
				change := asMap(asMap(payload["change_map"])[newImageBlockID])
				ops := asSlice(asMap(change["payload"])["ops"])
				if len(ops) != 1 {
					t.Fatalf("image binding ops = %#v, want one op", ops)
				}
				action := asMap(asMap(ops[0])["action"])
				if asMap(action["oi"])["token"] != "boxr-png-token" {
					t.Fatalf("image binding action = %#v", action)
				}
				boundImage = true
				returnOKJSON(t, w)
				return
			}
			changeMap := asMap(payload["change_map"])
			tableChange := asMap(changeMap["table_1"])
			if len(tableChange) == 0 {
				t.Fatalf("table change missing: %s", text)
			}
			tableOps := asSlice(asMap(tableChange["payload"])["ops"])
			cellSetAdds := 0
			for _, rawOp := range tableOps {
				op := asMap(rawOp)
				path := asSlice(op["p"])
				action := asMap(op["action"])
				if len(path) == 2 && path[0] == "rows_id" {
					newRowID = asString(action["li"])
				}
				if len(path) == 2 && path[0] == "cell_set" {
					cellSetAdds++
					cell := asMap(action["oi"])
					newCellIDs[asString(cell["block_id"])] = true
				}
			}
			if newRowID == "" || cellSetAdds != 2 || len(newCellIDs) != 2 {
				t.Fatalf("table ops did not append one complete row: %#v", tableOps)
			}
			for id, rawChange := range changeMap {
				if id == "table_1" {
					continue
				}
				ops := asSlice(asMap(asMap(rawChange)["payload"])["ops"])
				if len(ops) == 0 {
					continue
				}
				data := asMap(asMap(ops[0])["action"])["oi"]
				inserted := asMap(data)
				switch inserted["type"] {
				case "table_cell":
					if inserted["parent_id"] != "table_1" || !newCellIDs[id] {
						t.Fatalf("table cell %s = %#v", id, inserted)
					}
				case "text":
					if textFromBlockData(inserted) == "Applied table row" {
						newTextBlockID = id
					}
				case "image":
					newImageBlockID = id
					image := asMap(inserted["image"])
					if image["token"] != nil || image["mimeType"] != "image/png" || image["name"] != "ceph-logo.png" ||
						asInt(image["width"]) != 1 || asInt(image["height"]) != 1 {
						t.Fatalf("image placeholder = %#v", image)
					}
				}
			}
			if newTextBlockID == "" || newImageBlockID == "" {
				t.Fatalf("new row blocks missing textID=%q imageID=%q payload=%s", newTextBlockID, newImageBlockID, text)
			}
			wroteRow = true
			returnOKJSON(t, w)
		case "/space/api/box/stream/upload/all/":
			if !wroteRow {
				t.Fatal("upload occurred before row write")
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatal(err)
			}
			uploadedName = r.URL.Query().Get("name")
			uploadedMountPoint = r.URL.Query().Get("mount_point")
			uploadedMountNodeToken = r.URL.Query().Get("mount_node_token")
			if uploadedName != "ceph-logo.png" || uploadedMountPoint != "docx_image" || uploadedMountNodeToken != newImageBlockID {
				t.Fatalf("upload query name=%q mount_point=%q mount_node_token=%q imageBlockID=%q", uploadedName, uploadedMountPoint, uploadedMountNodeToken, newImageBlockID)
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"token": "boxr-png-token"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := AppendTableRow(TableAppendRowConfig{
		URL:         server.URL + "/docx/page_1",
		InputPath:   inputPath,
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
		Apply:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["dryRun"] != false || payload["willWrite"] != true ||
		payload["appendedRowCount"] != 1 || payload["attachedImageCount"] != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	verify := payload["verify"].(map[string]any)
	if verify["ok"] != true {
		t.Fatalf("verify = %+v", verify)
	}
	if !wroteRow || !boundImage || uploadedName != "ceph-logo.png" {
		t.Fatalf("wroteRow=%v boundImage=%v uploadedName=%q", wroteRow, boundImage, uploadedName)
	}
}

func TestLocalRenderedImageSupportsPNGJPEGAndSVG(t *testing.T) {
	tmpDir := t.TempDir()
	pngPath := filepath.Join(tmpDir, "logo.png")
	writePNGFixture(t, pngPath)
	jpegPath := filepath.Join(tmpDir, "logo.jpeg")
	writeJPEGFixture(t, jpegPath)
	svgPath := filepath.Join(tmpDir, "logo.svg")
	if err := os.WriteFile(svgPath, []byte(`<svg width="12" height="7" xmlns="http://www.w3.org/2000/svg"></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path     string
		mimeType string
		width    int
		height   int
	}{
		{path: pngPath, mimeType: "image/png", width: 1, height: 1},
		{path: jpegPath, mimeType: "image/jpeg", width: 1, height: 1},
		{path: svgPath, mimeType: "image/svg+xml", width: 12, height: 7},
	}
	for _, test := range tests {
		image, err := localRenderedImage(test.path)
		if err != nil {
			t.Fatalf("localRenderedImage(%s): %v", test.path, err)
		}
		if image.MimeType != test.mimeType || image.Width != test.width || image.Height != test.height || image.Size <= 0 {
			t.Fatalf("image metadata = %+v, want mime=%s %dx%d", image, test.mimeType, test.width, test.height)
		}
	}
}

func nativeTableClientVars(textBlockID string, imageBlockID string, imageBound bool) map[string]any {
	rows := []any{"row_header"}
	cellSet := map[string]any{
		"row_headercol_title": map[string]any{"block_id": "cell_header_title", "merge_info": map[string]any{"row_span": 1, "col_span": 1}},
		"row_headercol_logo":  map[string]any{"block_id": "cell_header_logo", "merge_info": map[string]any{"row_span": 1, "col_span": 1}},
	}
	blockMap := map[string]any{
		"page_1": map[string]any{"version": 7, "data": map[string]any{
			"type":     "page",
			"author":   "author_fixture",
			"children": []any{"table_1"},
		}},
		"table_1": map[string]any{"version": 3, "data": map[string]any{
			"type":       "table",
			"parent_id":  "page_1",
			"author":     "author_fixture",
			"rows_id":    rows,
			"columns_id": []any{"col_title", "col_logo"},
			"column_set": map[string]any{
				"col_title": map[string]any{"column_width": 120},
				"col_logo":  map[string]any{"column_width": 120},
			},
			"cell_set": cellSet,
		}},
		"cell_header_title": map[string]any{"version": 1, "data": map[string]any{
			"type":      "table_cell",
			"parent_id": "table_1",
			"children":  []any{"text_header_title"},
		}},
		"text_header_title": map[string]any{"version": 1, "data": map[string]any{
			"type":      "text",
			"parent_id": "cell_header_title",
			"text":      attributedCLIText("Title"),
		}},
		"cell_header_logo": map[string]any{"version": 1, "data": map[string]any{
			"type":      "table_cell",
			"parent_id": "table_1",
			"children":  []any{"text_header_logo"},
		}},
		"text_header_logo": map[string]any{"version": 1, "data": map[string]any{
			"type":      "text",
			"parent_id": "cell_header_logo",
			"text":      attributedCLIText("Logo"),
		}},
	}
	if textBlockID != "" || imageBlockID != "" {
		rows = append(rows, "row_appended")
		cellSet["row_appendedcol_title"] = map[string]any{"block_id": "cell_appended_title", "merge_info": map[string]any{"row_span": 1, "col_span": 1}}
		cellSet["row_appendedcol_logo"] = map[string]any{"block_id": "cell_appended_logo", "merge_info": map[string]any{"row_span": 1, "col_span": 1}}
		blockMap["cell_appended_title"] = map[string]any{"version": 1, "data": map[string]any{
			"type":      "table_cell",
			"parent_id": "table_1",
			"children":  []any{textBlockID},
		}}
		blockMap[textBlockID] = map[string]any{"version": 1, "data": map[string]any{
			"type":      "text",
			"parent_id": "cell_appended_title",
			"text":      attributedCLIText("Applied table row"),
		}}
		imageData := map[string]any{
			"name":     "ceph-logo.png",
			"mimeType": "image/png",
			"size":     70,
			"width":    1,
			"height":   1,
			"src":      "",
			"scale":    1,
			"align":    "center",
		}
		if imageBound {
			imageData["token"] = "boxr-png-token"
		}
		blockMap["cell_appended_logo"] = map[string]any{"version": 1, "data": map[string]any{
			"type":      "table_cell",
			"parent_id": "table_1",
			"children":  []any{imageBlockID},
		}}
		blockMap[imageBlockID] = map[string]any{"version": 1, "data": map[string]any{
			"type":      "image",
			"parent_id": "cell_appended_logo",
			"image":     imageData,
		}}
	}
	return map[string]any{
		"meta_map":  map[string]any{"page_1": map[string]any{"editor_id": "member_fixture"}},
		"block_map": blockMap,
	}
}

func writePNGFixture(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 0x33, G: 0x66, B: 0x99, A: 0xff})
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func writeJPEGFixture(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 0x99, G: 0x66, B: 0x33, A: 0xff})
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatal(err)
	}
}

func writeJSONFixture(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeDocspublishJSONRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
