package docxgraph

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSafeSummaryReportsStructureWithoutRawIDs(t *testing.T) {
	graph := graphFixtureWithRootChildren(
		blockFixture("secret_heading_parent_abcdef", "heading2", "目标章节"),
		blockFixture("secret_text_body_abcdef", "text", "body text that should be summarized safely"),
		blockFixture("secret_heading_child_abcdef", "heading3", "子章节"),
		blockFixture("secret_image_abcdef", "image", ""),
		blockFixture("secret_heading_duplicate_abcdef", "heading2", "目标章节"),
	)
	graph.RootID = "secret_page_root_abcdef"
	root := graph.Blocks["page_1"]
	root.ID = graph.RootID
	graph.Blocks[graph.RootID] = root
	delete(graph.Blocks, "page_1")

	summary := graph.SafeSummary()
	if summary["topLevelBlocks"] != 5 {
		t.Fatalf("topLevelBlocks = %v, want 5", summary["topLevelBlocks"])
	}
	if summary["complexBlockCount"] != 1 {
		t.Fatalf("complexBlockCount = %v, want 1", summary["complexBlockCount"])
	}
	if got := summary["complexBlockTypes"]; !reflect.DeepEqual(got, []string{"image"}) {
		t.Fatalf("complexBlockTypes = %#v, want image", got)
	}
	if got := summary["duplicateHeadings"]; !reflect.DeepEqual(got, []string{"目标章节"}) {
		t.Fatalf("duplicateHeadings = %#v", got)
	}

	headings := summary["headings"].([]map[string]any)
	if len(headings) != 3 {
		t.Fatalf("heading count = %d, want 3", len(headings))
	}
	if headings[0]["path"] != "目标章节" || headings[0]["sectionStart"] != 0 || headings[0]["sectionEnd"] != 4 {
		t.Fatalf("first heading summary = %#v", headings[0])
	}
	if headings[1]["path"] != "目标章节 > 子章节" || headings[1]["previousSibling"].(map[string]any)["kind"] != "text" || headings[1]["nextSibling"].(map[string]any)["kind"] != "image" {
		t.Fatalf("nested heading summary = %#v", headings[1])
	}

	serialized, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	text := string(serialized)
	for _, leaked := range []string{"secret_page_root_abcdef", "secret_heading_parent_abcdef", "secret_image_abcdef"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("summary leaked raw id %q: %s", leaked, text)
		}
	}
}
