package docspublish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownTablesArePreservedAsReadableBlocks(t *testing.T) {
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
		t.Fatalf("top ids = %#v, want one table fallback block", topIDs)
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"callout", "告警", "阈值", "P0", "立即处理", "P1", "尽快处理"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated table fallback missing %q: %s", expected, text)
		}
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
	if payload["tableFallbackCount"] != 1 {
		t.Fatalf("tableFallbackCount = %#v, want 1", payload["tableFallbackCount"])
	}
	if payload["tableFallbackBlockType"] != "callout" {
		t.Fatalf("tableFallbackBlockType = %#v, want callout", payload["tableFallbackBlockType"])
	}
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

	verify, err := session.verify("doxrzPage", session.spaceAPI+"/docx/doxrzPage", []string{"missing text"})
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

	verify, err := session.verify("doxrzPage", session.spaceAPI+"/docx/doxrzPage", []string{"required text"})
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
