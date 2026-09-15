package docspublish

import "testing"

func TestVerifyMarkdownOutputChecksOrderedChildEdgesAndImageToken(t *testing.T) {
	blockMap := nestedVerifyBlockMap()
	session, closeServer := newVerifyFixtureSession(t, blockMap)
	defer closeServer()

	verify, err := session.verifyMarkdownOutputWithRoots(
		"doxrzPage",
		session.spaceAPI+"/docx/doxrzPage",
		[]string{"Deploy", "command"},
		nestedVerifySpecs(),
		[]string{"ordered_1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if verify["ok"] != true || verify["expectedNestedBlockCount"] != 2 || verify["nestedBlockCount"] != 2 ||
		verify["missingNestedBlockCount"] != 0 || verify["missingImageTokenCount"] != 0 {
		t.Fatalf("verify = %#v", verify)
	}
}

func TestVerifyMarkdownOutputRejectsBrokenOrderedChildTree(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		field  string
		want   int
	}{
		{
			name: "wrong parent",
			mutate: func(blockMap map[string]any) {
				dataForBlock(blockMap["code_1"])["parent_id"] = "wrong_parent"
			},
			field: "missingNestedBlockCount",
			want:  1,
		},
		{
			name: "missing child edge",
			mutate: func(blockMap map[string]any) {
				dataForBlock(blockMap["ordered_1"])["children"] = []any{"image_1"}
			},
			field: "missingNestedBlockCount",
			want:  1,
		},
		{
			name: "missing image token",
			mutate: func(blockMap map[string]any) {
				dataForBlock(blockMap["image_1"])["image"] = map[string]any{}
			},
			field: "missingImageTokenCount",
			want:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blockMap := nestedVerifyBlockMap()
			test.mutate(blockMap)
			session, closeServer := newVerifyFixtureSession(t, blockMap)
			defer closeServer()

			verify, err := session.verifyMarkdownOutputWithRoots(
				"doxrzPage",
				session.spaceAPI+"/docx/doxrzPage",
				[]string{"Deploy", "command"},
				nestedVerifySpecs(),
				[]string{"ordered_1"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if verify["ok"] != false || verify[test.field] != test.want {
				t.Fatalf("verify = %#v, want ok=false and %s=%d", verify, test.field, test.want)
			}
		})
	}
}

func TestVerifyMarkdownOutputAllowsContainerImplementationChildren(t *testing.T) {
	tests := []struct {
		name      string
		specKind  string
		blockType string
	}{
		{name: "quote", specKind: "quote", blockType: "quote_container"},
		{name: "callout", specKind: "callout", blockType: "callout"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blockMap := map[string]any{
				"doxrzPage": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":     "page",
						"children": []any{"container_1"},
					},
				},
				"container_1": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      test.blockType,
						"parent_id": "doxrzPage",
						"children":  []any{"text_1"},
					},
				},
				"text_1": map[string]any{
					"version": 1,
					"data": map[string]any{
						"type":      "text",
						"parent_id": "container_1",
						"text":      attributedCLIText("Container text"),
					},
				},
			}
			session, closeServer := newVerifyFixtureSession(t, blockMap)
			defer closeServer()

			verify, err := session.verifyMarkdownOutputWithRoots(
				"doxrzPage",
				session.spaceAPI+"/docx/doxrzPage",
				[]string{"Container text"},
				[]Spec{{Kind: test.specKind, Text: "Container text"}},
				[]string{"container_1"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if verify["ok"] != true || verify["expectedNestedBlockCount"] != 0 || verify["missingNestedBlockCount"] != 0 {
				t.Fatalf("verify = %#v", verify)
			}
		})
	}
}

func nestedVerifySpecs() []Spec {
	return []Spec{{
		Kind: "ordered",
		Text: "Deploy",
		Children: []Spec{
			{Kind: "code", Text: "command\nnext"},
			{Kind: "image", SourceKind: "mermaid", Text: "flowchart LR\nA-->B"},
		},
	}}
}

func nestedVerifyBlockMap() map[string]any {
	return map[string]any{
		"doxrzPage": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":     "page",
				"children": []any{"ordered_1"},
			},
		},
		"ordered_1": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "ordered",
				"parent_id": "doxrzPage",
				"children":  []any{"code_1", "image_1"},
				"text":      attributedCLIText("Deploy"),
			},
		},
		"code_1": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "code",
				"parent_id": "ordered_1",
				"text":      attributedCLIText("command\nnext"),
			},
		},
		"image_1": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type":      "image",
				"parent_id": "ordered_1",
				"image":     map[string]any{"token": "bound-token"},
			},
		},
	}
}
