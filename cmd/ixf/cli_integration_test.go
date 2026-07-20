package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCLIDocsPublishDryRunAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "notes.md")
	if err := os.WriteFile(source, []byte(
		"# Apply Title\n\n"+
			"Body with required text.\n\n"+
			"```bash\n"+
			"echo one\n"+
			"echo two\n"+
			"```\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLITest(t,
		"docs", "publish", source,
		"--base-url", "https://tenant.example.test",
		"--title-suffix", " - Draft",
		"--require", "required text",
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("dry-run exit code = %d, stderr=%q", code, stderr)
	}
	dryRun := decodeCLIJSON(t, stdout)
	if dryRun["ok"] != true || dryRun["dryRun"] != true || dryRun["title"] != "Apply Title - Draft" {
		t.Fatalf("dry-run payload = %+v", dryRun)
	}
	if dryRun["operation"] != "create_docx" {
		t.Fatalf("dry-run operation = %q, want create_docx", dryRun["operation"])
	}
	assertMapNumbers(t, dryRun["counts"], map[string]int{"text": 1, "code": 1})
	if stderr != "" {
		t.Fatalf("dry-run stderr = %q, want empty", stderr)
	}

	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCLICookieFixture(t, cookiesPath)
	var events []string
	wroteBlocks := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/explorer/v2/create/object/":
			if r.Method != http.MethodPost {
				t.Fatalf("create method = %s, want POST", r.Method)
			}
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			events = append(events, "create")
			if got := r.Form.Get("name"); got != "Apply Title - Published" {
				t.Fatalf("create name = %q", got)
			}
			if got := r.Form.Get("parent_token"); got != "parent_fixture" {
				t.Fatalf("parent token = %q", got)
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"obj_token": "doxrzCreatedPage"}})
		case "/space/api/docx/pages/client_vars":
			if r.Method != http.MethodGet {
				t.Fatalf("client_vars method = %s, want GET", r.Method)
			}
			events = append(events, "client_vars")
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			if got := r.URL.Query().Get("id"); got != "doxrzCreatedPage" {
				t.Fatalf("client_vars id = %q", got)
			}
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
				blockMap = map[string]any{
					"doxrzCreatedPage": map[string]any{
						"version": 8,
						"data": map[string]any{
							"type":     "page",
							"author":   "author_fixture",
							"children": []any{"text_1", "code_1"},
						},
					},
					"text_1": map[string]any{
						"version": 1,
						"data": map[string]any{
							"type": "text",
							"text": attributedCLIText("Body with required text."),
						},
					},
					"code_1": map[string]any{
						"version": 1,
						"data": map[string]any{
							"type": "code",
							"text": attributedCLIText("echo one\necho two"),
						},
					},
				}
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"block_map": blockMap}})
		case "/space/api/docx/blocks/user_change":
			if r.Method != http.MethodPost {
				t.Fatalf("user_change method = %s, want POST", r.Method)
			}
			events = append(events, "write")
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["member_id"] != "member_override" || payload["page_id"] != "doxrzCreatedPage" {
				t.Fatalf("write payload identifiers = %+v", payload)
			}
			raw, err := json.Marshal(payload["change_map"])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "Body with required text.") || !strings.Contains(string(raw), "echo one\\necho two") {
				t.Fatalf("change_map missing written content: %s", raw)
			}
			wroteBlocks = true
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr, code = runCLITest(t,
		"docs", "publish", source,
		"--base-url", server.URL,
		"--space-api", server.URL,
		"--cookies", cookiesPath,
		"--parent-token", "parent_fixture",
		"--member-id", "member_override",
		"--title-suffix", " - Published",
		"--require", "required text",
		"--apply",
	)
	if code != 0 {
		t.Fatalf("apply exit code = %d, stderr=%q", code, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["ok"] != true || payload["dryRun"] != false || payload["title"] != "Apply Title - Published" {
		t.Fatalf("apply payload = %+v", payload)
	}
	if payload["operation"] != "create_docx" {
		t.Fatalf("apply operation = %q, want create_docx", payload["operation"])
	}
	verify := payload["verify"].(map[string]any)
	if verify["ok"] != true {
		t.Fatalf("verify payload = %+v", verify)
	}
	if !reflect.DeepEqual(events, []string{"create", "client_vars", "write", "client_vars"}) {
		t.Fatalf("events = %#v", events)
	}
	if stderr != "" {
		t.Fatalf("apply stderr = %q, want empty", stderr)
	}
}

func TestCLIDocsUpdateDryRunPreflight(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "replacement.md")
	if err := os.WriteFile(source, []byte(
		"# Replacement Title\n\n"+
			"Replacement body.\n\n"+
			"## Next Section\n\n"+
			"- Replacement item\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			if r.Method != http.MethodGet {
				t.Fatalf("client_vars method = %s, want GET", r.Method)
			}
			events = append(events, "client_vars")
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
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
					},
				},
			})
		case "/space/api/docx/blocks/user_change":
			t.Fatal("dry-run must not call user_change")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr, code := runCLITest(t,
		"docs", "update", source,
		"--url", server.URL+"/docx/doxrzExistingPage?from=copy",
		"--space-api", server.URL,
		"--cookies", cookiesPath,
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("docs update dry-run exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["ok"] != true || payload["dryRun"] != true || payload["operation"] != "update_docx" ||
		payload["mode"] != "replace_body" || payload["destructive"] != true {
		t.Fatalf("docs update dry-run payload = %+v", payload)
	}
	if payload["targetToken"] != "doxrzExistingPage" || payload["currentTopLevelBlocks"] != float64(2) ||
		payload["plannedTopLevelBlocks"] != float64(3) {
		t.Fatalf("docs update target/counts payload = %+v", payload)
	}
	if payload["supportedExistingContent"] != true || payload["complexBlockCount"] != float64(0) {
		t.Fatalf("docs update complex block payload = %+v", payload)
	}
	assertMapNumbers(t, payload["counts"], map[string]int{"text": 1, "heading2": 1, "bullet": 1})
	if !reflect.DeepEqual(events, []string{"client_vars"}) {
		t.Fatalf("events = %#v, want client_vars only", events)
	}
	if stderr != "" {
		t.Fatalf("docs update dry-run stderr = %q, want empty", stderr)
	}
}

func TestCLIDocsUpdateApplyReplacesExistingBody(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "replacement.md")
	if err := os.WriteFile(source, []byte(
		"# Replacement Title\n\n"+
			"Replacement body with required text.\n\n"+
			"```bash\n"+
			"echo new\n"+
			"echo next\n"+
			"```\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	var events []string
	wroteBlocks := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			if r.Method != http.MethodGet {
				t.Fatalf("client_vars method = %s, want GET", r.Method)
			}
			events = append(events, "client_vars")
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			if got := r.URL.Query().Get("id"); got != "doxrzExistingPage" {
				t.Fatalf("client_vars id = %q", got)
			}
			blockMap := map[string]any{
				"doxrzExistingPage": map[string]any{
					"version": 12,
					"data": map[string]any{
						"type":     "page",
						"author":   "author_fixture",
						"children": []any{"old_text", "old_code"},
					},
				},
				"old_text": map[string]any{
					"version": 3,
					"data": map[string]any{
						"type":      "text",
						"parent_id": "doxrzExistingPage",
						"text":      attributedCLIText("Old body."),
					},
				},
				"old_code": map[string]any{
					"version": 4,
					"data": map[string]any{
						"type":      "code",
						"parent_id": "doxrzExistingPage",
						"text":      attributedCLIText("echo old"),
					},
				},
			}
			if wroteBlocks {
				blockMap = map[string]any{
					"doxrzExistingPage": map[string]any{
						"version": 13,
						"data": map[string]any{
							"type":     "page",
							"author":   "author_fixture",
							"children": []any{"text_1", "code_1"},
						},
					},
					"text_1": map[string]any{
						"version": 1,
						"data": map[string]any{
							"type": "text",
							"text": attributedCLIText("Replacement body with required text."),
						},
					},
					"code_1": map[string]any{
						"version": 1,
						"data": map[string]any{
							"type": "code",
							"text": attributedCLIText("echo new\necho next"),
						},
					},
				}
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"block_map": blockMap}})
		case "/space/api/docx/blocks/user_change":
			if r.Method != http.MethodPost {
				t.Fatalf("user_change method = %s, want POST", r.Method)
			}
			events = append(events, "write")
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			payload := decodeRequestBody(t, r)
			if payload["member_id"] != "author_fixture" || payload["page_id"] != "doxrzExistingPage" {
				t.Fatalf("write payload identifiers = %+v", payload)
			}
			raw, err := json.Marshal(payload["change_map"])
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			for _, expected := range []string{
				`"ld":"old_code"`,
				`"ld":"old_text"`,
				`"od"`,
				"Replacement body with required text.",
				"echo new\\necho next",
			} {
				if !strings.Contains(text, expected) {
					t.Fatalf("change_map missing %q: %s", expected, text)
				}
			}
			wroteBlocks = true
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr, code := runCLITest(t,
		"docs", "update", source,
		"--url", server.URL+"/docx/doxrzExistingPage",
		"--space-api", server.URL,
		"--cookies", cookiesPath,
		"--require", "required text",
		"--apply",
	)
	if code != 0 {
		t.Fatalf("docs update apply exit code = %d, stderr=%q stdout=%q", code, stderr, stdout)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["ok"] != true || payload["dryRun"] != false || payload["operation"] != "update_docx" ||
		payload["mode"] != "replace_body" || payload["destructive"] != true {
		t.Fatalf("docs update apply payload = %+v", payload)
	}
	verify := payload["verify"].(map[string]any)
	if verify["ok"] != true {
		t.Fatalf("verify payload = %+v", verify)
	}
	if !reflect.DeepEqual(events, []string{"client_vars", "write", "client_vars"}) {
		t.Fatalf("events = %#v", events)
	}
	if stderr != "" {
		t.Fatalf("docs update apply stderr = %q, want empty", stderr)
	}
}

func TestCLIDocsUpdateRejectsUnsupportedURLsAndComplexApply(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "replacement.md")
	if err := os.WriteFile(source, []byte("# Replacement\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLITest(t,
		"docs", "update", source,
		"--url", "https://tenant.example.test/wiki/wikiToken",
		"--dry-run",
	)
	if code != 2 || !strings.Contains(stderr, "docs update requires a direct docx URL") {
		t.Fatalf("wiki rejection code=%d stderr=%q", code, stderr)
	}

	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCLICookieFixture(t, cookiesPath)
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			events = append(events, "client_vars")
			writeTestJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"block_map": map[string]any{
						"doxrzExistingPage": map[string]any{
							"version": 12,
							"data": map[string]any{
								"type":     "page",
								"author":   "author_fixture",
								"children": []any{"old_image"},
							},
						},
						"old_image": map[string]any{
							"version": 1,
							"data": map[string]any{
								"type":      "image",
								"parent_id": "doxrzExistingPage",
							},
						},
					},
				},
			})
		case "/space/api/docx/blocks/user_change":
			t.Fatal("complex apply must not call user_change")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, stderr, code = runCLITest(t,
		"docs", "update", source,
		"--url", server.URL+"/docx/doxrzExistingPage",
		"--space-api", server.URL,
		"--cookies", cookiesPath,
		"--apply",
	)
	if code != 2 || !strings.Contains(stderr, "complex existing content") {
		t.Fatalf("complex apply rejection code=%d stderr=%q", code, stderr)
	}
	if !reflect.DeepEqual(events, []string{"client_vars"}) {
		t.Fatalf("events = %#v, want client_vars only", events)
	}
}

func TestCLIDocsReadManifestCleanupAndOKRGuard(t *testing.T) {
	tmpDir := t.TempDir()
	sourceA := filepath.Join(tmpDir, "Project Plan.md")
	sourceB := filepath.Join(tmpDir, "project-plan.md")
	if err := os.WriteFile(sourceA, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceB, []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmpDir, "out")

	stdout, stderr, code := runCLITest(t,
		"docs", "read", sourceA, sourceB,
		"--out-dir", outDir,
		"--print-manifest",
	)
	if code != 0 {
		t.Fatalf("docs read manifest exit code = %d, stderr=%q", code, stderr)
	}
	manifest := decodeCLIJSON(t, stdout)
	first := manifest["local_markdown_1"].(map[string]any)
	second := manifest["local_markdown_2"].(map[string]any)
	if first["file"] != filepath.Join(outDir, "project-plan.md") ||
		second["file"] != filepath.Join(outDir, "project-plan-2.md") {
		t.Fatalf("manifest file paths = %+v", manifest)
	}
	assertFileText(t, filepath.Join(outDir, "project-plan.md"), "# A\n")
	assertFileText(t, filepath.Join(outDir, "project-plan-2.md"), "# B\n")

	_, stderr, code = runCLITest(t, "docs", "cleanup", outDir)
	if code != 0 {
		t.Fatalf("docs cleanup exit code = %d, stderr=%q", code, stderr)
	}
	if fileExists(filepath.Join(outDir, "manifest.json")) || fileExists(filepath.Join(outDir, "project-plan.md")) {
		t.Fatal("cleanup left generated manifest or markdown output")
	}

	stdout, stderr, code = runCLITest(t,
		"docs", "read",
		"https://tenant.example.test/okr/user/owner-fixture/?okrId=okr-fixture-200",
	)
	if code != 2 {
		t.Fatalf("OKR guard exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, expected := range []string{"docs read does not support OKR URLs", "ixf okr read"} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("OKR guard stderr missing %q: %s", expected, stderr)
		}
	}
	for _, forbidden := range []string{"cookie file", "okr-fixture-200"} {
		if strings.Contains(stderr, forbidden) {
			t.Fatalf("OKR guard stderr leaked %q: %s", forbidden, stderr)
		}
	}
}

func TestCLIOKRWriteDryRunAndApplyObjectiveIndex(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "okr.json")
	writeOKRInput(t, inputPath, []map[string]any{{
		"objective": "New O3",
		"krs":       []string{"New KR1", "New KR2"},
	}})

	stdout, stderr, code := runCLITest(t,
		"okr", "write",
		"--url", "https://tenant.example.test/okr/user/example/?okrId=example-okr",
		"--input", inputPath,
		"--objective-index", "3",
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("OKR dry-run exit code = %d, stderr=%q", code, stderr)
	}
	dryRun := decodeCLIJSON(t, stdout)
	if dryRun["ok"] != true || dryRun["dryRun"] != true ||
		dryRun["okrId"] != "example-okr" || dryRun["targetObjectiveIndex"] != float64(3) {
		t.Fatalf("OKR dry-run payload = %+v", dryRun)
	}

	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCLICookieFixture(t, cookiesPath)
	var events []string
	detailPayload := func(final bool) map[string]any {
		o3Name := "Old O3"
		krs := []any{map[string]any{"id": "old-kr", "content": map[string]any{"blocks": []any{map[string]any{"text": "Old KR"}}}}}
		if final {
			o3Name = "New O3"
			krs = []any{
				map[string]any{"id": "new-kr-1", "content": map[string]any{"blocks": []any{map[string]any{"text": "New KR1"}}}},
				map[string]any{"id": "new-kr-2", "content": map[string]any{"blocks": []any{map[string]any{"text": "New KR2"}}}},
			}
		}
		return map[string]any{
			"code": 0,
			"okr_detail_data": map[string]any{
				"name": "2026 Q3",
				"objective_list": []any{
					map[string]any{"id": "o1", "name": map[string]any{"blocks": []any{map[string]any{"text": "O1"}}}, "kr_list": []any{}},
					map[string]any{"id": "o2", "name": map[string]any{"blocks": []any{map[string]any{"text": "O2"}}}, "kr_list": []any{}},
					map[string]any{"id": "o3", "name": map[string]any{"blocks": []any{map[string]any{"text": o3Name}}}, "kr_list": krs},
				},
			},
		}
	}
	server := newOKRTestServer(t, &events, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/okrx/api/okr/owner/aggr_detail/":
			events = append(events, "detail")
			assertHeader(t, r, "x-lgw-csrf-token", "lgw-fixture")
			writeTestJSON(t, w, detailPayload(containsEvent(events, "publish")))
		case "/okrx/api/okr/example-okr/version/":
			events = append(events, "version")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"okr_draft_version": "version-1"}})
		case "/okrx/api/draft_v2/enable/o3/":
			events = append(events, "enable")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-2"}})
		case "/okrx/api/draft_v2/objective/o3/":
			events = append(events, "objective")
			assertJSONBodyContains(t, r, "name", "New O3")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-3"}})
		case "/okrx/api/draft_v2/kr/old-kr/":
			events = append(events, "delete_kr")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-4"}})
		case "/okrx/api/draft_v2/kr/":
			index := countEvents(events, "create_kr") + 1
			events = append(events, "create_kr")
			body := decodeRequestBody(t, r)
			if body["objective_id"] != "o3" {
				t.Fatalf("create KR objective_id = %+v", body)
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"kr_id": fmt.Sprintf("new-kr-%d", index), "draft_version": "version-5"}})
		case "/okrx/api/draft_v2/kr/new-kr-1/", "/okrx/api/draft_v2/kr/new-kr-2/":
			events = append(events, "kr_text")
			assertJSONBodyContains(t, r, "content", "New KR")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-6"}})
		case "/okrx/api/draft_v2/publish/o3/":
			events = append(events, "publish")
			body := decodeRequestBody(t, r)
			if !reflect.DeepEqual(asStringSlice(body["need_delete_kr_ids"]), []string{"old-kr"}) {
				t.Fatalf("publish delete KR ids = %+v", body["need_delete_kr_ids"])
			}
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-7"}})
		default:
			return false
		}
		return true
	})
	defer server.Close()

	stdout, stderr, code = runCLITest(t,
		"okr", "write",
		"--url", server.URL+"/okr/user/example/?okrId=example-okr",
		"--input", inputPath,
		"--objective-index", "3",
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--apply",
	)
	if code != 0 {
		t.Fatalf("OKR apply exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	target := payload["target"].(map[string]any)
	if target["objective"] != "New O3" {
		t.Fatalf("target payload = %+v", target)
	}
	assertEventSequence(t, events, []string{
		"csrf", "detail", "version", "enable", "objective", "delete_kr",
		"create_kr", "kr_text", "create_kr", "kr_text", "publish", "detail",
	})
	if stderr != "" {
		t.Fatalf("OKR apply stderr = %q, want empty", stderr)
	}
}

func TestCLIOKRWriteCreatesAndPrunesViaAPI(t *testing.T) {
	tmpDir := t.TempDir()
	cookiesPath := filepath.Join(tmpDir, "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	t.Run("create next objective by index", func(t *testing.T) {
		inputPath := filepath.Join(tmpDir, "create-okr.json")
		writeOKRInput(t, inputPath, []map[string]any{{
			"objective": "Created O3",
			"krs":       []string{"Created KR1", "Created KR2"},
		}})
		var events []string
		detailPayload := func(final bool) map[string]any {
			objectives := []any{
				map[string]any{"id": "o1", "name": map[string]any{"blocks": []any{map[string]any{"text": "O1"}}}, "kr_list": []any{}},
				map[string]any{"id": "o2", "name": map[string]any{"blocks": []any{map[string]any{"text": "O2"}}}, "kr_list": []any{}},
			}
			if final {
				objectives = append(objectives, map[string]any{
					"id":   "new-o3",
					"name": map[string]any{"blocks": []any{map[string]any{"text": "Created O3"}}},
					"kr_list": []any{
						map[string]any{"id": "new-kr-1", "content": map[string]any{"blocks": []any{map[string]any{"text": "Created KR1"}}}},
						map[string]any{"id": "new-kr-2", "content": map[string]any{"blocks": []any{map[string]any{"text": "Created KR2"}}}},
					},
				})
			}
			return map[string]any{"code": 0, "okr_detail_data": map[string]any{"name": "2026 Q3", "objective_list": objectives}}
		}
		server := newOKRTestServer(t, &events, func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case "/okrx/api/okr/owner/aggr_detail/":
				events = append(events, "detail")
				writeTestJSON(t, w, detailPayload(containsEvent(events, "publish")))
			case "/okrx/api/okr/example-okr/version/":
				events = append(events, "version")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"okr_draft_version": "version-1"}})
			case "/okrx/api/draft_v2/objective/":
				events = append(events, "create_objective")
				body := decodeRequestBody(t, r)
				if body["okr_id"] != "example-okr" {
					t.Fatalf("create objective payload = %+v", body)
				}
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"objective_id": "new-o3", "draft_version": "version-2"}})
			case "/okrx/api/draft_v2/objective/new-o3/":
				events = append(events, "objective")
				assertJSONBodyContains(t, r, "name", "Created O3")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-3"}})
			case "/okrx/api/draft_v2/kr/":
				index := countEvents(events, "create_kr") + 1
				events = append(events, "create_kr")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"kr_id": fmt.Sprintf("new-kr-%d", index), "draft_version": "version-4"}})
			case "/okrx/api/draft_v2/kr/new-kr-1/", "/okrx/api/draft_v2/kr/new-kr-2/":
				events = append(events, "kr_text")
				assertJSONBodyContains(t, r, "content", "Created KR")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-5"}})
			case "/okrx/api/draft_v2/publish/new-o3/":
				events = append(events, "publish")
				body := decodeRequestBody(t, r)
				if len(asStringSlice(body["need_delete_kr_ids"])) != 0 {
					t.Fatalf("created publish should not delete KRs: %+v", body)
				}
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-6"}})
			default:
				return false
			}
			return true
		})
		defer server.Close()

		stdout, stderr, code := runCLITest(t,
			"okr", "write",
			"--url", server.URL+"/okr/user/example/?okrId=example-okr",
			"--input", inputPath,
			"--objective-index", "3",
			"--cookies", cookiesPath,
			"--csrf-url", server.URL+"/lgw/csrf_token",
			"--apply",
		)
		if code != 0 {
			t.Fatalf("create O3 exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
		}
		assertEventSequence(t, events, []string{
			"csrf", "detail", "version", "create_objective", "objective",
			"create_kr", "kr_text", "create_kr", "kr_text", "publish", "detail",
		})
	})

	t.Run("prune deletes non-input objectives and KRs", func(t *testing.T) {
		inputPath := filepath.Join(tmpDir, "prune-okr.json")
		writeOKRInput(t, inputPath, []map[string]any{{
			"objective": "O2",
			"krs":       []string{"Pruned KR"},
		}})
		var events []string
		detailPayload := func(final bool) map[string]any {
			objectives := []any{
				map[string]any{"id": "o1", "name": map[string]any{"blocks": []any{map[string]any{"text": "Delete Me"}}}, "kr_list": []any{}},
				map[string]any{
					"id":   "o2",
					"name": map[string]any{"blocks": []any{map[string]any{"text": "O2"}}},
					"kr_list": []any{
						map[string]any{"id": "old-kr", "content": map[string]any{"blocks": []any{map[string]any{"text": "Old KR"}}}},
						map[string]any{"id": "extra-kr", "content": map[string]any{"blocks": []any{map[string]any{"text": "Extra KR"}}}},
					},
				},
			}
			if final {
				objectives = []any{map[string]any{
					"id":      "o2",
					"name":    map[string]any{"blocks": []any{map[string]any{"text": "O2"}}},
					"kr_list": []any{map[string]any{"id": "new-kr-1", "content": map[string]any{"blocks": []any{map[string]any{"text": "Pruned KR"}}}}},
				}}
			}
			return map[string]any{"code": 0, "okr_detail_data": map[string]any{"name": "2026 Q3", "objective_list": objectives}}
		}
		server := newOKRTestServer(t, &events, func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case "/okrx/api/okr/owner/aggr_detail/":
				events = append(events, "detail")
				writeTestJSON(t, w, detailPayload(containsEvent(events, "publish")))
			case "/okrx/api/okr/example-okr/version/":
				events = append(events, "version")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"okr_draft_version": "version-1"}})
			case "/okrx/api/draft_v2/enable/o1/":
				events = append(events, "enable_o1")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-2"}})
			case "/okrx/api/draft_v2/objective/o1/":
				events = append(events, "delete_o1")
				assertDeleteHasDraftVersion(t, r)
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-3"}})
			case "/okrx/api/draft_v2/enable/o2/":
				events = append(events, "enable_o2")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-4"}})
			case "/okrx/api/draft_v2/kr/old-kr/", "/okrx/api/draft_v2/kr/extra-kr/":
				events = append(events, "delete_kr")
				assertDeleteHasDraftVersion(t, r)
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-5"}})
			case "/okrx/api/draft_v2/kr/":
				events = append(events, "create_kr")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"kr_id": "new-kr-1", "draft_version": "version-6"}})
			case "/okrx/api/draft_v2/kr/new-kr-1/":
				events = append(events, "kr_text")
				assertJSONBodyContains(t, r, "content", "Pruned KR")
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-7"}})
			case "/okrx/api/draft_v2/publish/o2/":
				events = append(events, "publish")
				body := decodeRequestBody(t, r)
				if !reflect.DeepEqual(asStringSlice(body["need_delete_kr_ids"]), []string{"old-kr", "extra-kr"}) {
					t.Fatalf("prune publish delete KR ids = %+v", body["need_delete_kr_ids"])
				}
				writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "version-8"}})
			default:
				return false
			}
			return true
		})
		defer server.Close()

		stdout, stderr, code := runCLITest(t,
			"okr", "write",
			"--url", server.URL+"/okr/user/example/?okrId=example-okr",
			"--input", inputPath,
			"--cookies", cookiesPath,
			"--csrf-url", server.URL+"/lgw/csrf_token",
			"--prune",
			"--apply",
		)
		if code != 0 {
			t.Fatalf("prune exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
		}
		payload := decodeCLIJSON(t, stdout)
		objectives := payload["objectives"].([]any)
		if len(objectives) != 1 || objectives[0].(map[string]any)["objective"] != "O2" {
			t.Fatalf("prune payload objectives = %+v", objectives)
		}
		if countEvents(events, "delete_kr") != 2 || !containsEvent(events, "delete_o1") {
			t.Fatalf("prune delete events = %#v", events)
		}
	})
}

func TestCLIUpdateSelfUsesGoReleaseArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	nextVersion := nextPatchVersion(t, version)
	artifactName := fmt.Sprintf("ixf_%s_%s_%s", nextVersion, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		artifactName += ".exe"
	}
	replacement := []byte("new-go-binary\n")
	artifact := filepath.Join(tmpDir, artifactName)
	if err := os.WriteFile(artifact, replacement, 0o755); err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(replacement))
	checksums := filepath.Join(tmpDir, fmt.Sprintf("ixf_%s_checksums.txt", nextVersion))
	if err := os.WriteFile(checksums, []byte(fmt.Sprintf("%s  %s\n", checksum, artifactName)), 0o644); err != nil {
		t.Fatal(err)
	}
	releasePath := filepath.Join(tmpDir, "latest.json")
	writeJSONFile(t, releasePath, map[string]any{
		"tag_name": "v" + nextVersion,
		"html_url": "https://github.example/releases/v" + nextVersion,
		"assets": []map[string]any{
			{"name": artifactName, "browser_download_url": fileURI(t, artifact)},
			{"name": filepath.Base(checksums), "browser_download_url": fileURI(t, checksums)},
		},
	})
	target := filepath.Join(tmpDir, "ixf-target")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := os.WriteFile(target, []byte("old-go-binary\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLITest(t,
		"update", "self",
		"--release-file", releasePath,
		"--target-path", target,
		"--apply",
		"--json",
	)
	if code != 0 {
		t.Fatalf("update self exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["applied"] != true || payload["checksumVerified"] != true || payload["artifactName"] != artifactName {
		t.Fatalf("update self payload = %+v", payload)
	}
	assertFileText(t, target, string(replacement))
}

func nextPatchVersion(t *testing.T, current string) string {
	t.Helper()
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		t.Fatalf("current version = %q, want semantic version", current)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		t.Fatalf("current patch version = %q: %v", parts[2], err)
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}

func runCLITest(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func decodeCLIJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, text)
	}
	return payload
}

func writeJSONFile(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeOKRInput(t *testing.T, path string, objectives []map[string]any) {
	t.Helper()
	writeJSONFile(t, path, map[string]any{"objectives": objectives})
}

func writeCLICookieFixture(t *testing.T, path string) {
	t.Helper()
	raw, err := json.Marshal([]map[string]string{
		{"name": "_csrf_token", "value": "csrf-fixture"},
		{"name": "session", "value": "session-fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
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

func assertMapNumbers(t *testing.T, raw any, want map[string]int) {
	t.Helper()
	got, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want map", raw)
	}
	for key, wantValue := range want {
		if got[key] != float64(wantValue) {
			t.Fatalf("map[%s] = %#v, want %d; all=%+v", key, got[key], wantValue, got)
		}
	}
}

func assertFileText(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", path, content, want)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func newOKRTestServer(t *testing.T, events *[]string, handler func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lgw/csrf_token" {
			*events = append(*events, "csrf")
			assertCookie(t, r, "session", "session-fixture")
			w.Header().Set("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
			_, _ = w.Write([]byte("{}"))
			return
		}
		if handler(w, r) {
			return
		}
		http.NotFound(w, r)
	}))
}

func decodeRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatalf("decode request body for %s %s: %v", r.Method, r.URL.Path, err)
	}
	return payload
}

func assertJSONBodyContains(t *testing.T, r *http.Request, key string, expected string) {
	t.Helper()
	body := decodeRequestBody(t, r)
	value, err := json.Marshal(body[key])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(value), expected) {
		t.Fatalf("%s body value = %s, want substring %q", key, value, expected)
	}
}

func asStringSlice(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func containsEvent(events []string, event string) bool {
	for _, item := range events {
		if item == event {
			return true
		}
	}
	return false
}

func countEvents(events []string, event string) int {
	count := 0
	for _, item := range events {
		if item == event {
			count++
		}
	}
	return count
}

func assertEventSequence(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func assertDeleteHasDraftVersion(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodDelete {
		t.Fatalf("method = %s, want DELETE", r.Method)
	}
	if r.URL.Query().Get("draft_version") == "" {
		t.Fatalf("delete query missing draft_version: %s", r.URL.RawQuery)
	}
}

func fileURI(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	absolute = filepath.ToSlash(absolute)
	if runtime.GOOS == "windows" && !strings.HasPrefix(absolute, "/") {
		absolute = "/" + absolute
	}
	return (&url.URL{Scheme: "file", Path: absolute}).String()
}
