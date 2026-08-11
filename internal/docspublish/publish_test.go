package docspublish

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseMarkdownConvertsMermaidFencesToImageSpecs(t *testing.T) {
	_, specs, err := ParseMarkdown("# Title\n\n```mermaid\nflowchart LR\n  A --> B\n```\n\n```Plain\nsequenceDiagram\n  A->>B: ping\n```\n\n```bash\necho keep-code\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 {
		t.Fatalf("spec count = %d, want 3: %#v", len(specs), specs)
	}
	for index, spec := range specs[:2] {
		if spec.Kind != "image" || spec.SourceKind != "mermaid" {
			t.Fatalf("spec[%d] = %#v, want mermaid image spec", index, spec)
		}
	}
	if specs[2].Kind != "code" || specs[2].SourceKind != "" || !strings.Contains(specs[2].Text, "echo keep-code") {
		t.Fatalf("spec[2] = %#v, want ordinary code block", specs[2])
	}
}

func TestBuildBlocksCreatesMermaidImageSkeleton(t *testing.T) {
	_, specs, err := ParseMarkdown("# Title\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	topIDs, entries := buildBlocks(specs, "doxrzPage", newBlockFactory("author_fixture"))
	if len(topIDs) != 1 || len(entries) != 1 {
		t.Fatalf("blocks top=%#v entries=%#v, want one image block", topIDs, entries)
	}
	image := entries[0].Data
	if image["type"] != "image" || image["parent_id"] != "doxrzPage" || image["author"] != "author_fixture" {
		t.Fatalf("image skeleton = %#v", image)
	}
	if len(asSlice(image["children"])) != 0 {
		t.Fatalf("image children = %#v, want empty slice", image["children"])
	}
	if _, ok := image["image"]; ok {
		t.Fatalf("image skeleton contains bound image before upload: %#v", image)
	}
}

func TestMarkdownTablesBuildNativeTableBlocks(t *testing.T) {
	_, specs, err := ParseMarkdown("# Title\n\n| 告警 | 阈值 |\n|---|---|\n| P0 | 立即处理 |\n| P1 | 尽快处理 |\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("spec count = %d, want 1: %#v", len(specs), specs)
	}
	if specs[0].Kind != "table" {
		t.Fatalf("table spec kind = %q, want table", specs[0].Kind)
	}
	for _, expected := range []string{"告警", "阈值", "P0", "立即处理", "P1", "尽快处理"} {
		if !strings.Contains(specs[0].Text, expected) {
			t.Fatalf("table spec text missing %q: %#v", expected, specs[0])
		}
	}

	topIDs, entries := buildBlocks(specs, "doxrzPage", newBlockFactory("author_fixture"))
	if len(topIDs) != 1 {
		t.Fatalf("top ids = %#v, want one table block", topIDs)
	}
	dataByID := map[string]map[string]any{}
	for _, entry := range entries {
		dataByID[entry.ID] = entry.Data
	}
	table := dataByID[topIDs[0]]
	if table["type"] != "table" {
		raw, _ := json.Marshal(entries)
		t.Fatalf("top block type = %#v, want table; blocks=%s", table["type"], string(raw))
	}
	if len(asSlice(table["rows_id"])) != 3 || len(asSlice(table["columns_id"])) != 2 {
		t.Fatalf("table dimensions = rows %#v cols %#v", table["rows_id"], table["columns_id"])
	}
	if _, ok := table["children"]; ok {
		t.Fatalf("table block includes unsupported children field: %#v", table)
	}
	if _, ok := table["align"]; ok {
		t.Fatalf("table block includes unsupported align field: %#v", table)
	}
	rows := asSlice(table["rows_id"])
	columns := asSlice(table["columns_id"])
	for _, rowID := range rows {
		if !strings.HasPrefix(asString(rowID), "row") {
			t.Fatalf("row id = %#v, want row-prefixed uuid", rowID)
		}
	}
	columnSet := asMap(table["column_set"])
	if len(columnSet) != len(columns) {
		t.Fatalf("column_set len = %d, want %d: %#v", len(columnSet), len(columns), columnSet)
	}
	for _, columnID := range columns {
		id := asString(columnID)
		if !strings.HasPrefix(id, "col") {
			t.Fatalf("column id = %#v, want col-prefixed uuid", columnID)
		}
		if asInt(asMap(columnSet[id])["column_width"]) <= 0 {
			t.Fatalf("column_set[%q] = %#v, want positive column_width", id, columnSet[id])
		}
	}
	cellSet := asMap(table["cell_set"])
	if len(cellSet) != 6 {
		t.Fatalf("cell set len = %d, want 6; cell_set=%#v", len(cellSet), cellSet)
	}
	for _, entry := range entries {
		if entry.ID == topIDs[0] {
			continue
		}
		if entry.Data["type"] == "table_cell" {
			if entry.Data["vertical_align"] != "middle" {
				t.Fatalf("table cell missing vertical_align: %#v", entry.Data)
			}
			if _, ok := entry.Data["align"]; ok {
				t.Fatalf("table cell includes unsupported align field: %#v", entry.Data)
			}
			continue
		}
		if entry.Data["type"] != "text" {
			raw, _ := json.Marshal(entries)
			t.Fatalf("unexpected table child block type: %#v in %s", entry.Data["type"], string(raw))
		}
	}
	for _, rowID := range rows {
		for _, columnID := range columns {
			key := asString(rowID) + asString(columnID)
			cell := asMap(cellSet[key])
			if cell["block_id"] == "" {
				t.Fatalf("cell_set[%q] missing block_id: %#v", key, cellSet)
			}
			merge := asMap(cell["merge_info"])
			if asInt(merge["row_span"]) != 1 || asInt(merge["col_span"]) != 1 {
				t.Fatalf("cell_set[%q].merge_info = %#v, want 1x1", key, merge)
			}
		}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"告警", "阈值", "P0", "立即处理", "P1", "尽快处理"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated table missing %q: %s", expected, text)
		}
	}
}

func TestPublishMarkdownDryRunReportsMermaidImageMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	markdownPath := filepath.Join(tmpDir, "mermaid.md")
	if err := os.WriteFile(markdownPath, []byte("# Title\n\n```mermaid\nflowchart LR\n  A --> B\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpDir)

	payload, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      "https://tenant.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	counts := payload["counts"].(map[string]int)
	if counts["image"] != 1 || counts["code"] != 0 {
		t.Fatalf("counts = %+v, want image=1 code=0", counts)
	}
	if payload["mermaidImageCount"] != 1 || payload["plannedImageCount"] != 1 {
		t.Fatalf("mermaid metadata = %+v", payload)
	}
	if payload["mermaidRenderer"] != "mmdc" || payload["mermaidPreferredFormat"] != "svg" || payload["mermaidFallbackFormat"] != "png" {
		t.Fatalf("renderer metadata = %+v", payload)
	}
	if payload["mermaidRendererAvailable"] != false {
		t.Fatalf("renderer availability = %#v, want false", payload["mermaidRendererAvailable"])
	}
}

func TestPublishMarkdownApplyRequiresMermaidRendererBeforeRemoteWrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)
	markdownPath := filepath.Join(tmpDir, "mermaid.md")
	if err := os.WriteFile(markdownPath, []byte("# Title\n\n```mermaid\nflowchart LR\n  A --> B\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      server.URL,
		Apply:        true,
	})
	if err == nil || !strings.Contains(err.Error(), `mermaid renderer "mmdc" not found in PATH`) {
		t.Fatalf("err = %v, want missing renderer error", err)
	}
	if requests != 0 {
		t.Fatalf("remote requests = %d, want 0 before renderer preflight", requests)
	}
}

func TestPublishMarkdownApplyCreatesMermaidImageWithUploadedImageData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	tmpDir := t.TempDir()
	writeMMDCRendererFixture(t, tmpDir)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	markdownPath := filepath.Join(tmpDir, "mermaid.md")
	if err := os.WriteFile(markdownPath, []byte("# Mermaid Title\n\nBody with required text.\n\n```mermaid\nflowchart LR\n  A --> B\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[
		{"name":"_csrf_token","value":"csrf-fixture"},
		{"name":"session","value":"session-fixture"}
	]`), 0o600); err != nil {
		t.Fatal(err)
	}

	var events []string
	wroteBlocks := false
	boundImage := false
	wroteImagePlaceholder := false
	textBlockID := ""
	imageBlockID := ""
	placeholderImage := map[string]any{}
	uploadedName := ""
	uploadedMountPoint := ""
	uploadedMountNodeToken := ""
	uploadedMountNodePoint := ""
	uploadedObjType := ""
	uploadedAsync := ""
	uploadedSize := ""
	uploadedFormMetadata := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/explorer/v2/create/object/":
			events = append(events, "create")
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"obj_token": "doxrzCreatedPage"}})
		case "/space/api/docx/pages/client_vars":
			events = append(events, "client_vars")
			blockMap := map[string]any{
				"doxrzCreatedPage": map[string]any{
					"version": 7,
					"data": map[string]any{
						"type":     "page",
						"author":   "author_fixture",
						"children": []any{},
					},
				},
			}
			if wroteBlocks {
				if textBlockID == "" || imageBlockID == "" {
					t.Fatal("fixture missing generated block ids")
				}
				blockMap = map[string]any{
					"doxrzCreatedPage": map[string]any{
						"version": 8,
						"data": map[string]any{
							"type":     "page",
							"author":   "author_fixture",
							"children": []any{textBlockID, imageBlockID},
						},
					},
					textBlockID: map[string]any{
						"version": 1,
						"data": map[string]any{
							"type": "text",
							"text": attributedCLIText("Body with required text."),
						},
					},
					imageBlockID: map[string]any{
						"version": 2,
						"data": map[string]any{
							"type":      "image",
							"parent_id": "doxrzCreatedPage",
							"author":    "author_fixture",
						},
					},
				}
				if len(placeholderImage) > 0 {
					asMap(asMap(blockMap[imageBlockID])["data"])["image"] = placeholderImage
				}
				if boundImage {
					asMap(asMap(blockMap[imageBlockID])["data"])["image"] = map[string]any{
						"token":    "boxr-svg-token",
						"mimeType": "image/svg+xml",
						"name":     "mermaid-001.svg",
						"width":    640,
						"height":   360,
						"size":     1234,
						"src":      "",
						"scale":    1,
						"align":    "center",
					}
				}
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"block_map": blockMap}})
		case "/space/api/docx/blocks/user_change/":
			events = append(events, "write")
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(payload["change_map"])
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if strings.Contains(text, `"type":"image"`) {
				changeMap := payload["change_map"].(map[string]any)
				for blockID, rawChange := range changeMap {
					change := rawChange.(map[string]any)
					payload := change["payload"].(map[string]any)
					ops := payload["ops"].([]any)
					for _, rawOp := range ops {
						op := rawOp.(map[string]any)
						action := op["action"].(map[string]any)
						inserted := asMap(action["oi"])
						switch inserted["type"] {
						case "text":
							textBlockID = blockID
						case "image":
							imageBlockID = blockID
							image := asMap(inserted["image"])
							if len(image) == 0 {
								continue
							}
							if image["token"] != nil || image["mimeType"] != "image/svg+xml" || image["name"] != "mermaid-001.svg" || asInt(image["width"]) != 640 || asInt(image["height"]) != 360 || asInt(image["scale"]) != 1 || image["src"] != "" || image["align"] != "center" {
								writeTestJSON(t, w, map[string]any{
									"code": 4000020,
									"msg":  "schema mismatch",
								})
								return
							}
							wroteImagePlaceholder = true
							placeholderImage = image
						}
					}
				}
				if textBlockID == "" || imageBlockID == "" {
					t.Fatalf("could not discover created text/image block IDs: %s", text)
				}
				if !wroteImagePlaceholder {
					t.Fatalf("first image write did not include placeholder metadata: %s", text)
				}
				wroteBlocks = true
				returnOKJSON(t, w)
				return
			}
			if strings.Contains(text, `"p":["image"]`) {
				if imageBlockID == "" || uploadedMountNodeToken != imageBlockID {
					writeTestJSON(t, w, map[string]any{
						"code": 4000030,
						"msg":  "relation mismatch",
					})
					return
				}
				change := asMap(payload["change_map"])[imageBlockID]
				ops := asSlice(asMap(asMap(change)["payload"])["ops"])
				if len(ops) != 1 {
					t.Fatalf("image binding ops = %#v, want one replace op", ops)
				}
				action := asMap(asMap(ops[0])["action"])
				oldImage := asMap(action["od"])
				newImage := asMap(action["oi"])
				if oldImage["token"] != nil || oldImage["mimeType"] != "image/svg+xml" || oldImage["name"] != "mermaid-001.svg" || asInt(oldImage["width"]) != 640 || asInt(oldImage["height"]) != 360 {
					writeTestJSON(t, w, map[string]any{
						"code": 4000030,
						"msg":  "relation mismatch",
					})
					return
				}
				if newImage["token"] != "boxr-svg-token" || newImage["mimeType"] != "image/svg+xml" || newImage["name"] != "mermaid-001.svg" || asInt(newImage["width"]) != 640 || asInt(newImage["height"]) != 360 {
					writeTestJSON(t, w, map[string]any{
						"code": 4000020,
						"msg":  "schema mismatch",
					})
					return
				}
				boundImage = true
				returnOKJSON(t, w)
				return
			}
			t.Fatalf("unexpected user_change payload: %s", text)
		case "/space/api/box/stream/upload/all/":
			events = append(events, "upload")
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatal(err)
			}
			query := r.URL.Query()
			uploadedName = query.Get("name")
			uploadedMountPoint = query.Get("mount_point")
			uploadedMountNodeToken = query.Get("mount_node_token")
			uploadedMountNodePoint = query.Get("mount_node_point")
			uploadedObjType = query.Get("obj_type")
			uploadedAsync = query.Get("is_asynchronous")
			uploadedSize = query.Get("size")
			for _, key := range []string{"file_name", "parent_type", "parent_node", "mount_point", "mount_node_token", "size"} {
				if _, ok := r.MultipartForm.Value[key]; ok {
					uploadedFormMetadata = true
				}
			}
			if uploadedName == "" || uploadedMountPoint == "" || uploadedMountNodeToken == "" || uploadedMountNodePoint != "" || uploadedObjType != "" || uploadedAsync != "" || uploadedSize == "" || uploadedFormMetadata {
				writeTestJSON(t, w, map[string]any{
					"code": 4000020,
					"msg":  "upload metadata must match direct upload params",
				})
				return
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "<svg") {
				t.Fatalf("uploaded body = %q, want SVG", string(body))
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"token": "boxr-svg-token"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	payload, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      server.URL,
		SpaceAPI:     server.URL,
		CookiesPath:  cookiesPath,
		RequiredText: []string{"required text"},
		Apply:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["mermaidImageCount"] != 1 || payload["attachedImageCount"] != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	verify := payload["verify"].(map[string]any)
	counts := verify["counts"].(map[string]int)
	if counts["image"] != 1 {
		t.Fatalf("verify counts = %+v, want image=1", counts)
	}
	if uploadedName != "mermaid-001.svg" || uploadedMountPoint != "docx_image" || uploadedMountNodeToken != imageBlockID || uploadedMountNodePoint != "" || uploadedObjType != "" || uploadedAsync != "" || uploadedSize == "" {
		t.Fatalf("upload query name=%q mount_point=%q mount_node_token=%q mount_node_point=%q imageBlockID=%q obj_type=%q async=%q size=%q", uploadedName, uploadedMountPoint, uploadedMountNodeToken, uploadedMountNodePoint, imageBlockID, uploadedObjType, uploadedAsync, uploadedSize)
	}
	if uploadedFormMetadata {
		t.Fatal("upload metadata was sent as multipart form fields; want query params only")
	}
	if !wroteImagePlaceholder || !boundImage {
		t.Fatalf("placeholder=%v boundImage=%v, want both stages", wroteImagePlaceholder, boundImage)
	}
	expectedEvents := []string{"create", "client_vars", "write", "client_vars", "upload", "write", "client_vars"}
	if strings.Join(events, ",") != strings.Join(expectedEvents, ",") {
		t.Fatalf("events = %#v, want %#v", events, expectedEvents)
	}
}

func TestMermaidImageUploadFallsBackToPNGWhenSVGRenderFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	tmpDir := t.TempDir()
	writeFailingSVGMMDCRendererFixture(t, tmpDir)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	uploadedName := ""
	uploadedBody := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/box/stream/upload/all/" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatal(err)
		}
		uploadedName = r.URL.Query().Get("name")
		if uploadedName == "" {
			writeTestJSON(t, w, map[string]any{
				"code": 4000020,
				"msg":  "upload metadata must be query params",
			})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		uploadedBody = string(content)
		writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"token": "boxr-png-token"}})
	}))
	defer server.Close()

	session := &publishSession{
		client:  server.Client(),
		baseURL: server.URL,
		csrf:    "csrf-fixture",
		cookies: []http.Cookie{{Name: "session", Value: "session-fixture"}},
	}
	binding, err := session.renderUploadMermaidImage("doxrzPage", blockEntry{
		ID: "image_block",
		Image: &imageSource{
			Kind:    "mermaid",
			Text:    "flowchart LR\n  A --> B",
			Ordinal: 1,
		},
	}, server.URL+"/docx/doxrzPage", tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Token != "boxr-png-token" || binding.Image.MimeType != "image/png" || binding.Image.Name != "mermaid-001.png" || binding.Image.Width != 1 || binding.Image.Height != 1 {
		t.Fatalf("binding = %+v, want png fallback token", binding)
	}
	if uploadedName != "mermaid-001.png" || !strings.HasPrefix(uploadedBody, "\x89PNG") {
		t.Fatalf("uploaded name=%q body=%q, want PNG fallback", uploadedName, uploadedBody)
	}
}

func TestPublishMarkdownDryRunCountsMarkdownTables(t *testing.T) {
	tmpDir := t.TempDir()
	markdownPath := filepath.Join(tmpDir, "table.md")
	if err := os.WriteFile(markdownPath, []byte("# Title\n\n| Name | Value |\n|---|---|\n| Alpha | 1 |\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      "https://tenant.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	counts := payload["counts"].(map[string]int)
	if counts["table"] != 1 {
		t.Fatalf("counts = %+v, want table=1", counts)
	}
	if payload["tableFallbackCount"] != 0 {
		t.Fatalf("tableFallbackCount = %#v, want 0", payload["tableFallbackCount"])
	}
	if payload["tableBlockType"] != "table" {
		t.Fatalf("tableBlockType = %#v, want table", payload["tableBlockType"])
	}
	if payload["tableCount"] != 1 {
		t.Fatalf("tableCount = %#v, want 1", payload["tableCount"])
	}
}

func writeMMDCRendererFixture(t *testing.T, dir string) {
	t.Helper()
	pngPath := filepath.Join(dir, "fixture.png")
	if err := os.WriteFile(pngPath, testPNG1x1(), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mmdc")
	script := `#!/bin/sh
set -eu
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
case "$out" in
  *.svg) printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360"><text>fixture</text></svg>' > "$out" ;;
  *.png) cp ` + shellQuote(pngPath) + ` "$out" ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFailingSVGMMDCRendererFixture(t *testing.T, dir string) {
	t.Helper()
	pngPath := filepath.Join(dir, "fixture.png")
	if err := os.WriteFile(pngPath, testPNG1x1(), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mmdc")
	script := `#!/bin/sh
set -eu
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
case "$out" in
  *.svg) echo "svg unsupported" >&2; exit 42 ;;
  *.png) cp ` + shellQuote(pngPath) + ` "$out" ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func testPNG1x1() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func returnOKJSON(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{}})
}

func TestVerifyReportsMissingRequiredText(t *testing.T) {
	session, closeServer := newVerifyFixtureSession(t, map[string]any{
		"doxrzPage": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":     "page",
				"children": []any{"body"},
			},
		},
		"body": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "text",
				"parent_id": "doxrzPage",
				"text":      attributedCLIText("present text"),
			},
		},
	})
	defer closeServer()

	verify, err := session.verify("doxrzPage", session.spaceAPI+"/docx/doxrzPage", []string{"missing text"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if verify["ok"] != false {
		t.Fatalf("verify ok = %#v, want false: %+v", verify["ok"], verify)
	}
	missing, ok := verify["missingRequiredText"].([]string)
	if !ok || len(missing) != 1 || missing[0] != "missing text" {
		t.Fatalf("missingRequiredText = %#v, want [missing text]", verify["missingRequiredText"])
	}
}

func TestVerifyFailsWhenExpectedImageBlocksAreMissing(t *testing.T) {
	session, closeServer := newVerifyFixtureSession(t, map[string]any{
		"doxrzPage": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":     "page",
				"children": []any{"body"},
			},
		},
		"body": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "text",
				"parent_id": "doxrzPage",
				"text":      attributedCLIText("required text"),
			},
		},
	})
	defer closeServer()

	verify, err := session.verify("doxrzPage", session.spaceAPI+"/docx/doxrzPage", []string{"required text"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if verify["ok"] != false {
		t.Fatalf("verify ok = %#v, want false for missing image block: %+v", verify["ok"], verify)
	}
	if verify["imageCount"] != 0 || verify["expectedImageCount"] != 1 || verify["missingImageCount"] != 1 {
		t.Fatalf("image verification metadata = %+v", verify)
	}
}

func TestVerifyFailsWhenCalloutIsEmpty(t *testing.T) {
	session, closeServer := newVerifyFixtureSession(t, map[string]any{
		"doxrzPage": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":     "page",
				"children": []any{"callout"},
			},
		},
		"callout": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "callout",
				"parent_id": "doxrzPage",
				"children":  []any{"empty_text"},
			},
		},
		"empty_text": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "text",
				"parent_id": "callout",
				"text":      attributedCLIText(""),
			},
		},
		"body": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "text",
				"parent_id": "doxrzPage",
				"text":      attributedCLIText("required text"),
			},
		},
	})
	defer closeServer()

	verify, err := session.verify("doxrzPage", session.spaceAPI+"/docx/doxrzPage", []string{"required text"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if verify["ok"] != false {
		t.Fatalf("verify ok = %#v, want false for empty callout: %+v", verify["ok"], verify)
	}
	if verify["emptyCalloutCount"] != 1 {
		t.Fatalf("emptyCalloutCount = %#v, want 1", verify["emptyCalloutCount"])
	}
}

func TestBuildReplaceBodyChangeMapLeavesOldBlocksUnmodified(t *testing.T) {
	blockMap := map[string]any{
		"doxrzExistingPage": map[string]any{
			"version": 12,
			"data": map[string]any{
				"type":     "page",
				"children": []any{"old_text", "old_code"},
			},
		},
		"old_text": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "text",
				"parent_id": "doxrzExistingPage",
				"text":      attributedCLIText("Old body."),
			},
		},
		"old_code": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "code",
				"parent_id": "doxrzExistingPage",
				"text":      attributedCLIText("echo old"),
			},
		},
	}

	topIDs, entries := buildBlocks([]Spec{{Kind: "text", Text: "Replacement body."}}, "doxrzExistingPage", newBlockFactory("editor_fixture"))
	changeMap := buildReplaceBodyChangeMap(
		"doxrzExistingPage",
		asMap(blockMap["doxrzExistingPage"]),
		[]any{"old_text", "old_code"},
		topIDs,
		entries,
	)

	if _, ok := changeMap["old_text"]; ok {
		t.Fatalf("replace_body must not submit old_text deletion ops: %+v", changeMap["old_text"])
	}
	if _, ok := changeMap["old_code"]; ok {
		t.Fatalf("replace_body must not submit old_code deletion ops: %+v", changeMap["old_code"])
	}
	raw, err := json.Marshal(changeMap)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"ld":"old_code"`, `"ld":"old_text"`, "Replacement body."} {
		if !strings.Contains(text, want) {
			t.Fatalf("change_map missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, `"od"`) {
		t.Fatalf("replace_body must not hard-delete old block objects: %s", text)
	}
}

func TestAttribForCountsUTF16CodeUnits(t *testing.T) {
	cases := map[string]string{
		"abc":       "*0+3",
		"中文":        "*0+2",
		"emoji🙂":    "*0+7",
		"中文\nabc":   "*0|1+3*0+3",
		"emoji🙂\n中": "*0|1+8*0+1",
	}
	for input, want := range cases {
		if got := attribFor(input); got != want {
			t.Fatalf("attribFor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUpdateMarkdownSurfacesWriteRejectionDetails(t *testing.T) {
	tmpDir := t.TempDir()
	markdownPath := filepath.Join(tmpDir, "update.md")
	if err := os.WriteFile(markdownPath, []byte("# Title\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	if err := os.WriteFile(cookiesPath, []byte(`[
		{"name":"_csrf_token","value":"csrf-fixture"},
		{"name":"session","value":"session-fixture"}
	]`), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			if got := r.URL.Query().Get("id"); got != "doxrzExistingPage" {
				t.Fatalf("client_vars id = %q", got)
			}
			writeTestJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"block_map": map[string]any{
						"doxrzExistingPage": map[string]any{
							"version": 12,
							"data": map[string]any{
								"type":     "page",
								"author":   "author_fixture",
								"children": []any{"old_text"},
							},
						},
						"old_text": map[string]any{
							"version": 1,
							"data": map[string]any{
								"type":      "text",
								"parent_id": "doxrzExistingPage",
								"text":      attributedCLIText("Old body."),
							},
						},
					},
				},
			})
		case "/space/api/docx/blocks/user_change/":
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			writeTestJSON(t, w, map[string]any{
				"code": 123,
				"msg":  "write rejected by server",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := UpdateMarkdown(UpdateConfig{
		MarkdownPath: markdownPath,
		URL:          server.URL + "/docx/doxrzExistingPage?from=copy",
		CookiesPath:  cookiesPath,
		SpaceAPI:     server.URL,
		Apply:        true,
	})
	if err == nil {
		t.Fatal("UpdateMarkdown accepted a rejected write")
	}
	text := err.Error()
	for _, want := range []string{"document content write failed", "code=123", "write rejected by server"} {
		if !strings.Contains(text, want) {
			t.Fatalf("error = %q, want substring %q", text, want)
		}
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func newVerifyFixtureSession(t *testing.T, blockMap map[string]any) (*publishSession, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/api/docx/pages/client_vars" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(t, w, map[string]any{
			"code": 0,
			"data": map[string]any{"block_map": blockMap},
		})
	}))
	return &publishSession{
		client:   server.Client(),
		csrf:     "csrf-fixture",
		baseURL:  server.URL,
		spaceAPI: server.URL,
	}, server.Close
}

func attributedCLIText(text string) map[string]any {
	return map[string]any{"initialAttributedTexts": map[string]any{"text": map[string]any{"0": text}}}
}

func assertHeader(t *testing.T, r *http.Request, name string, want string) {
	t.Helper()
	if got := r.Header.Get(name); got != want {
		t.Fatalf("%s header = %q, want %q", name, got, want)
	}
}

func assertCookie(t *testing.T, r *http.Request, name string, want string) {
	t.Helper()
	cookie, err := r.Cookie(name)
	if err != nil || cookie.Value != want {
		t.Fatalf("%s cookie = %#v, %v; want %q", name, cookie, err, want)
	}
}
