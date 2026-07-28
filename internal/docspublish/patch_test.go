package docspublish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serialq7ic4/ixf-toolbox/internal/docxgraph"
)

func TestParseMarkdownFragmentAcceptsTableWithoutTitle(t *testing.T) {
	specs, err := ParseMarkdownFragment("| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Kind != "table" || specs[0].Rows[1][0] != "Alpha" {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestParseMarkdownFragmentDoesNotRenameDocumentForH1(t *testing.T) {
	specs, err := ParseMarkdownFragment("# Inserted Heading\n\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Kind != "heading1" || specs[0].Text != "Inserted Heading" {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestPatchInsertDryRunReportsNonDestructivePlan(t *testing.T) {
	server := patchInsertFixtureServer(t)
	defer server.Close()
	markdownPath := writeTempMarkdown(t, "| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")

	payload, err := PatchInsertMarkdown(PatchInsertConfig{
		MarkdownPath: markdownPath,
		URL:          server.URL + "/docx/page_1",
		SpaceAPI:     server.URL,
		CookiesPath:  writePatchCookieFixture(t),
		UnderHeading: "1.1 账号全集群初始化",
		Position:     "section-end",
		Apply:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["operation"] != "docs_patch_insert" || payload["mode"] != "block_insert" ||
		payload["destructive"] != false || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["tableBlockType"] != "table" || payload["tableFallbackCount"] != 0 {
		t.Fatalf("table metadata = %+v", payload)
	}
	if payload["duplicateCandidate"] != false || payload["existingBlocksTouched"] != false {
		t.Fatalf("safety metadata = %+v", payload)
	}
	if payload["insertIndex"] != 2 {
		t.Fatalf("insertIndex = %#v, want 2", payload["insertIndex"])
	}
}

func TestBuildPatchInsertChangeMapOnlyAddsNewBlocksAndRootLinks(t *testing.T) {
	root := map[string]any{"version": 7}
	topIDs := []string{"new_table", "new_text"}
	entries := []blockEntry{
		{ID: "new_table", Data: map[string]any{"type": "table", "parent_id": "page_1"}},
		{ID: "new_text", Data: map[string]any{"type": "text", "parent_id": "page_1"}},
	}
	changeMap := buildPatchInsertChangeMap("page_1", root, 2, topIDs, entries)
	raw, err := json.Marshal(changeMap)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{`"ld"`, `"od"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("insert change map touched existing blocks: %s", text)
		}
	}
	if !strings.Contains(text, `"li":"new_table"`) || !strings.Contains(text, `"li":"new_text"`) {
		t.Fatalf("missing root insert ops: %s", text)
	}
}

func TestReplaceSectionChangeMapTouchesOnlySectionChildren(t *testing.T) {
	graph := replaceSectionGraphFixture(t)
	heading, err := graph.FindHeadingByText("Replace Me")
	if err != nil {
		t.Fatal(err)
	}
	section := graph.SectionRange(heading)
	changeMap := buildReplaceSectionChangeMap("page_1", map[string]any{"version": 3}, section, []string{"new_1"}, []blockEntry{
		{ID: "new_1", Data: map[string]any{"type": "text", "parent_id": "page_1"}},
	})
	raw, err := json.Marshal(changeMap)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "outside_before") || strings.Contains(text, "outside_after") {
		t.Fatalf("outside section was touched: %s", text)
	}
	if !strings.Contains(text, `"ld":"replace_heading"`) || !strings.Contains(text, `"ld":"replace_body"`) ||
		!strings.Contains(text, `"li":"new_1"`) {
		t.Fatalf("section replacement ops missing expected ids: %s", text)
	}
}

func TestPatchReplaceSectionDryRunReportsBoundedDestructivePlan(t *testing.T) {
	server := patchSectionFixtureServer(t, false)
	defer server.Close()
	markdownPath := writeTempMarkdown(t, "## Replacement\n\nNew body with Alpha.\n")

	payload, err := PatchSectionMarkdown(PatchSectionConfig{
		MarkdownPath: markdownPath,
		URL:          server.URL + "/docx/page_1",
		SpaceAPI:     server.URL,
		CookiesPath:  writePatchCookieFixture(t),
		UnderHeading: "Replace Me",
		Apply:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["operation"] != "docs_patch_replace_section" || payload["mode"] != "section_replace" ||
		payload["destructive"] != true || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["deletedTopLevelBlocks"] != 2 || payload["plannedTopLevelBlocks"] != 2 ||
		payload["outsideSectionBlocksTouched"] != false {
		t.Fatalf("section metadata = %+v", payload)
	}
	structure := payload["structure"].(map[string]any)
	if structure["topLevelBlocks"] != 4 || structure["headingCount"] != 2 {
		t.Fatalf("section structure = %+v", structure)
	}
}

func TestPatchReplaceSectionRejectsComplexSectionWithoutOverride(t *testing.T) {
	server := patchSectionFixtureServer(t, true)
	defer server.Close()
	markdownPath := writeTempMarkdown(t, "## Replacement\n\nNew body.\n")

	_, err := PatchSectionMarkdown(PatchSectionConfig{
		MarkdownPath: markdownPath,
		URL:          server.URL + "/docx/page_1",
		SpaceAPI:     server.URL,
		CookiesPath:  writePatchCookieFixture(t),
		UnderHeading: "Replace Me",
		Apply:        false,
	})
	if err == nil || !strings.Contains(err.Error(), "complex section content") {
		t.Fatalf("err = %v, want complex section rejection", err)
	}
}

func replaceSectionGraphFixture(t *testing.T) docxgraph.Graph {
	t.Helper()
	graph, err := docxgraph.Build(map[string]any{
		"block_map": map[string]any{
			"page_1": map[string]any{
				"version": 3,
				"data": map[string]any{
					"type":     "page",
					"children": []any{"outside_before", "replace_heading", "replace_body", "outside_after"},
				},
			},
			"outside_before": map[string]any{
				"version": 1,
				"data": map[string]any{
					"type":      "text",
					"parent_id": "page_1",
					"text":      attributedCLIText("before"),
				},
			},
			"replace_heading": map[string]any{
				"version": 1,
				"data": map[string]any{
					"type":      "heading2",
					"parent_id": "page_1",
					"text":      attributedCLIText("Replace Me"),
				},
			},
			"replace_body": map[string]any{
				"version": 1,
				"data": map[string]any{
					"type":      "text",
					"parent_id": "page_1",
					"text":      attributedCLIText("old section"),
				},
			},
			"outside_after": map[string]any{
				"version": 1,
				"data": map[string]any{
					"type":      "heading2",
					"parent_id": "page_1",
					"text":      attributedCLIText("Next"),
				},
			},
		},
	}, "page_1")
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func patchSectionFixtureServer(t *testing.T, complex bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			if r.Method != http.MethodGet {
				t.Fatalf("client_vars method = %s, want GET", r.Method)
			}
			if got := r.URL.Query().Get("id"); got != "page_1" {
				t.Fatalf("client_vars id = %q, want page_1", got)
			}
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			children := []any{"outside_before", "replace_heading", "replace_body", "outside_after"}
			blockMap := map[string]any{
				"page_1": map[string]any{
					"version": 7,
					"data": map[string]any{
						"type":     "page",
						"author":   "author_fixture",
						"children": children,
					},
				},
				"outside_before": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "callout",
						"parent_id": "page_1",
						"children":  []any{"outside_before_text"},
					},
				},
				"outside_before_text": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "text",
						"parent_id": "outside_before",
						"text":      attributedCLIText("outside rich block"),
					},
				},
				"replace_heading": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "heading2",
						"parent_id": "page_1",
						"text":      attributedCLIText("Replace Me"),
					},
				},
				"replace_body": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "text",
						"parent_id": "page_1",
						"text":      attributedCLIText("old section"),
					},
				},
				"outside_after": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "heading2",
						"parent_id": "page_1",
						"text":      attributedCLIText("Next"),
					},
				},
			}
			if complex {
				blockMap["replace_body"].(map[string]any)["data"].(map[string]any)["children"] = []any{"complex_image"}
				blockMap["complex_image"] = map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "image",
						"parent_id": "replace_body",
					},
				}
			}
			writeTestJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"meta_map":  map[string]any{"page_1": map[string]any{"editor_id": "member_fixture"}},
					"block_map": blockMap,
				},
			})
		case "/space/api/docx/blocks/user_change/":
			t.Fatal("dry-run must not call user_change")
		default:
			http.NotFound(w, r)
		}
	}))
}

func patchInsertFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/docx/pages/client_vars":
			if r.Method != http.MethodGet {
				t.Fatalf("client_vars method = %s, want GET", r.Method)
			}
			if got := r.URL.Query().Get("id"); got != "page_1" {
				t.Fatalf("client_vars id = %q, want page_1", got)
			}
			assertHeader(t, r, "X-CSRFToken", "csrf-fixture")
			assertCookie(t, r, "session", "session-fixture")
			writeTestJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"meta_map": map[string]any{"page_1": map[string]any{"editor_id": "member_fixture"}},
					"block_map": map[string]any{
						"page_1": map[string]any{
							"version": 7,
							"data": map[string]any{
								"type":     "page",
								"author":   "author_fixture",
								"children": []any{"h1", "p1", "h2"},
							},
						},
						"h1": map[string]any{
							"version": 2,
							"data": map[string]any{
								"type":      "heading2",
								"parent_id": "page_1",
								"text":      attributedCLIText("1.1 账号全集群初始化"),
							},
						},
						"p1": map[string]any{
							"version": 1,
							"data": map[string]any{
								"type":      "text",
								"parent_id": "page_1",
								"text":      attributedCLIText("existing body"),
							},
						},
						"h2": map[string]any{
							"version": 2,
							"data": map[string]any{
								"type":      "heading2",
								"parent_id": "page_1",
								"text":      attributedCLIText("1.2 其他章节"),
							},
						},
					},
				},
			})
		case "/space/api/docx/blocks/user_change/":
			t.Fatal("dry-run must not call user_change")
		default:
			http.NotFound(w, r)
		}
	}))
}

func writeTempMarkdown(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fragment.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePatchCookieFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.json")
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
	return path
}
