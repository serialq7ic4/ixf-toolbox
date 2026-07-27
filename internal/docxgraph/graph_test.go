package docxgraph

import (
	"reflect"
	"testing"
)

func TestBuildGraphPreservesRootChildrenAndHeadings(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockEntry(2, map[string]any{
				"type": "page", "children": []any{"h1", "p1", "h2", "callout_1"},
			}),
			"h1": blockEntry(1, map[string]any{
				"type": "heading2", "parent_id": "page_1", "text": attributedText("1.1 账号全集群初始化"),
			}),
			"p1": blockEntry(1, map[string]any{
				"type": "text", "parent_id": "page_1", "text": attributedText("existing body"),
			}),
			"h2": blockEntry(1, map[string]any{
				"type": "heading2", "parent_id": "page_1", "text": attributedText("1.2 其他章节"),
			}),
			"callout_1": blockEntry(1, map[string]any{
				"type": "callout", "parent_id": "page_1", "children": []any{"callout_text"},
			}),
			"callout_text": blockEntry(1, map[string]any{
				"type": "text", "parent_id": "callout_1", "text": attributedText("rich block"),
			}),
		},
	}

	graph, err := Build(clientVars, "page_1")
	if err != nil {
		t.Fatal(err)
	}
	if got := graph.RootChildren; !reflect.DeepEqual(got, []string{"h1", "p1", "h2", "callout_1"}) {
		t.Fatalf("root children = %#v", got)
	}
	heading, err := graph.FindHeadingByText("1.1 账号全集群初始化")
	if err != nil {
		t.Fatal(err)
	}
	if heading.ID != "h1" || heading.Level != 2 || heading.Index != 0 {
		t.Fatalf("heading = %#v", heading)
	}
}

func blockEntry(version int, data map[string]any) map[string]any {
	return map[string]any{
		"version": version,
		"data":    data,
	}
}

func attributedText(text string) map[string]any {
	return map[string]any{
		"initialAttributedTexts": map[string]any{
			"text": map[string]any{"0": text},
		},
	}
}

func blockFixture(id string, kind string, text string) Block {
	return Block{
		ID:       id,
		Kind:     kind,
		ParentID: "page_1",
		Text:     text,
		Raw: map[string]any{
			"type": kind,
			"text": attributedText(text),
		},
	}
}

func graphFixtureWithRootChildren(blocks ...Block) Graph {
	blockMap := map[string]Block{
		"page_1": {
			ID:       "page_1",
			Kind:     "page",
			Children: make([]string, 0, len(blocks)),
			Raw:      map[string]any{"type": "page"},
		},
	}
	rootChildren := make([]string, 0, len(blocks))
	for _, block := range blocks {
		rootChildren = append(rootChildren, block.ID)
		blockMap[block.ID] = block
	}
	root := blockMap["page_1"]
	root.Children = append([]string(nil), rootChildren...)
	blockMap["page_1"] = root
	return Graph{
		RootID:       "page_1",
		RootVersion:  1,
		Blocks:       blockMap,
		RootChildren: rootChildren,
	}
}

func graphFixtureWithTextVersion(id string, text string, version int) Graph {
	block := blockFixture(id, "text", text)
	block.Version = version
	return graphFixtureWithRootChildren(block)
}
